package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

const DefaultVectorDimension = 128

// VectorEntry stores a semantic embedding vector alongside its intent, prompt, and AST node association.
type VectorEntry struct {
	ID         string        `json:"id"`
	NodeID     string        `json:"node_id"`
	Intent     string        `json:"intent"`
	UserPrompt string        `json:"user_prompt,omitempty"`
	Rationale  string        `json:"rationale,omitempty"`
	Tags       []string      `json:"tags,omitempty"`
	Language   core.Language `json:"language,omitempty"`
	NodeType   string        `json:"node_type,omitempty"`
	AgentID    string        `json:"agent_id,omitempty"`
	SessionID  string        `json:"session_id,omitempty"`
	Vector     []float32     `json:"vector"`
	CreatedAt  time.Time     `json:"created_at"`
}

// VectorSearchResult holds a matched VectorEntry and its cosine similarity score.
type VectorSearchResult struct {
	Entry VectorEntry `json:"entry"`
	Score float32     `json:"score"`
}

// VectorFilter defines optional metadata filtering constraints during similarity search.
type VectorFilter struct {
	Language  core.Language `json:"language,omitempty"`
	NodeType  string        `json:"node_type,omitempty"`
	AgentID   string        `json:"agent_id,omitempty"`
	SessionID string        `json:"session_id,omitempty"`
	Tags      []string      `json:"tags,omitempty"`
	MinScore  float32       `json:"min_score,omitempty"`
}

// EmbeddingProvider converts arbitrary text into a normalized float32 vector embedding.
type EmbeddingProvider interface {
	Dimension() int
	Embed(text string) ([]float32, error)
}

// DeterministicEmbedder generates normalized feature vectors using subword character n-grams and token hashing.
type DeterministicEmbedder struct {
	dimension int
}

// NewDeterministicEmbedder creates a deterministic, offline pure-Go embedder.
func NewDeterministicEmbedder(dimension int) *DeterministicEmbedder {
	if dimension <= 0 {
		dimension = DefaultVectorDimension
	}
	return &DeterministicEmbedder{dimension: dimension}
}

// Dimension returns the vector dimensionality.
func (e *DeterministicEmbedder) Dimension() int {
	return e.dimension
}

// Embed generates a normalized TF-IDF inspired vector from input text.
func (e *DeterministicEmbedder) Embed(text string) ([]float32, error) {
	vec := make([]float32, e.dimension)
	cleaned := strings.ToLower(strings.TrimSpace(text))
	if cleaned == "" {
		return vec, nil
	}

	words := strings.Fields(cleaned)
	for _, w := range words {
		// Word-level hash bucket
		wHash := hashStringToBucket(w, e.dimension)
		vec[wHash] += 2.0

		// Character trigrams for subword robustness
		runes := []rune(w)
		if len(runes) >= 3 {
			for i := 0; i <= len(runes)-3; i++ {
				trigram := string(runes[i : i+3])
				tHash := hashStringToBucket(trigram, e.dimension)
				vec[tHash] += 1.0
			}
		}
	}

	// L2 Normalization
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v * v)
	}
	if sumSq > 0 {
		norm := float32(math.Sqrt(sumSq))
		for i := range vec {
			vec[i] /= norm
		}
	}

	return vec, nil
}

func hashStringToBucket(s string, numBuckets int) int {
	sum := sha256.Sum256([]byte(s))
	val := uint32(sum[0])<<24 | uint32(sum[1])<<16 | uint32(sum[2])<<8 | uint32(sum[3])
	return int(val % uint32(numBuckets))
}

// VectorIndex manages persistent storage, CRUD, and semantic similarity search for vectors.
type VectorIndex struct {
	dbPath    string
	dimension int
	embedder  EmbeddingProvider
	mu        sync.RWMutex
	entries   map[string]VectorEntry // entry_id -> VectorEntry
	byNodeID  map[string][]string    // node_id -> []entry_id
}

type vectorDBSnapshot struct {
	Version   int           `json:"version"`
	Dimension int           `json:"dimension"`
	Timestamp time.Time     `json:"timestamp"`
	Entries   []VectorEntry `json:"entries"`
}

// NewVectorIndex opens or initializes a VectorIndex stored at basePath (.cosm/vector.db).
func NewVectorIndex(basePath string, dimension int, embedder EmbeddingProvider) (*VectorIndex, error) {
	var dbPath string
	if filepath.Base(basePath) == "vector.db" {
		dbPath = basePath
	} else if filepath.Base(basePath) == ".cosm" || filepath.Base(basePath) == ".fg" {
		dbPath = filepath.Join(basePath, "vector.db")
	} else {
		if _, err := os.Stat(filepath.Join(basePath, ".fg", "vector.db")); err == nil {
			dbPath = filepath.Join(basePath, ".fg", "vector.db")
		} else {
			dbPath = filepath.Join(basePath, ".cosm", "vector.db")
		}
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for vector.db: %w", err)
	}

	if dimension <= 0 {
		dimension = DefaultVectorDimension
	}
	if embedder == nil {
		embedder = NewDeterministicEmbedder(dimension)
	}

	idx := &VectorIndex{
		dbPath:    dbPath,
		dimension: dimension,
		embedder:  embedder,
		entries:   make(map[string]VectorEntry),
		byNodeID:  make(map[string][]string),
	}

	if err := idx.load(); err != nil {
		return nil, fmt.Errorf("failed to load vector db: %w", err)
	}

	return idx, nil
}

func (idx *VectorIndex) load() error {
	data, err := os.ReadFile(idx.dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}

	var snap vectorDBSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("corrupted vector snapshot: %w", err)
	}

	for _, entry := range snap.Entries {
		idx.entries[entry.ID] = entry
		idx.byNodeID[entry.NodeID] = append(idx.byNodeID[entry.NodeID], entry.ID)
	}

	return nil
}

// Save persists the in-memory vector index atomically to disk.
func (idx *VectorIndex) Save() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	var entriesList []VectorEntry
	for _, e := range idx.entries {
		entriesList = append(entriesList, e)
	}
	sort.Slice(entriesList, func(i, j int) bool {
		return entriesList[i].ID < entriesList[j].ID
	})

	snap := vectorDBSnapshot{
		Version:   1,
		Dimension: idx.dimension,
		Timestamp: time.Now().UTC(),
		Entries:   entriesList,
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal vector index: %w", err)
	}

	tmpFile := idx.dbPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write tmp vector db: %w", err)
	}
	if err := os.Rename(tmpFile, idx.dbPath); err != nil {
		return fmt.Errorf("failed to replace vector db: %w", err)
	}

	return nil
}

// Insert adds or updates a vector entry, automatically generating the vector embedding if empty.
func (idx *VectorIndex) Insert(entry *VectorEntry) error {
	if entry == nil {
		return fmt.Errorf("entry is nil")
	}
	if entry.NodeID == "" {
		return fmt.Errorf("entry node_id cannot be empty")
	}

	if entry.ID == "" {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s", entry.NodeID, entry.Intent, entry.UserPrompt)))
		entry.ID = hex.EncodeToString(sum[:])[:16]
	}

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	// Generate vector if not provided
	if len(entry.Vector) == 0 {
		var textParts []string
		if entry.Intent != "" {
			textParts = append(textParts, entry.Intent)
		}
		if entry.UserPrompt != "" {
			textParts = append(textParts, entry.UserPrompt)
		}
		if entry.Rationale != "" {
			textParts = append(textParts, entry.Rationale)
		}
		for _, tag := range entry.Tags {
			textParts = append(textParts, tag)
		}

		fullText := strings.Join(textParts, " ")
		vec, err := idx.embedder.Embed(fullText)
		if err != nil {
			return fmt.Errorf("failed to generate embedding: %w", err)
		}
		entry.Vector = vec
	}

	idx.mu.Lock()
	idx.entries[entry.ID] = *entry
	idx.byNodeID[entry.NodeID] = append(idx.byNodeID[entry.NodeID], entry.ID)
	idx.mu.Unlock()

	return idx.Save()
}

// InsertBatch adds multiple vector entries.
func (idx *VectorIndex) InsertBatch(entries []*VectorEntry) error {
	for _, e := range entries {
		if err := idx.Insert(e); err != nil {
			return err
		}
	}
	return nil
}

// Get returns a single VectorEntry by its unique ID.
func (idx *VectorIndex) Get(id string) (*VectorEntry, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.entries[id]
	if !ok {
		return nil, fmt.Errorf("vector entry not found: %s", id)
	}
	entryCopy := entry
	return &entryCopy, nil
}

// GetByNodeID returns all vector entries attached to a given nodeID.
func (idx *VectorIndex) GetByNodeID(nodeID string) ([]VectorEntry, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	ids := idx.byNodeID[nodeID]
	var results []VectorEntry
	for _, id := range ids {
		if e, ok := idx.entries[id]; ok {
			results = append(results, e)
		}
	}
	return results, nil
}

// Delete removes an entry by ID.
func (idx *VectorIndex) Delete(id string) error {
	idx.mu.Lock()
	entry, ok := idx.entries[id]
	if !ok {
		idx.mu.Unlock()
		return fmt.Errorf("entry not found: %s", id)
	}
	delete(idx.entries, id)

	// Clean index by nodeID
	var filtered []string
	for _, eID := range idx.byNodeID[entry.NodeID] {
		if eID != id {
			filtered = append(filtered, eID)
		}
	}
	idx.byNodeID[entry.NodeID] = filtered
	idx.mu.Unlock()

	return idx.Save()
}

// DeleteByNodeID removes all vector entries for a node.
func (idx *VectorIndex) DeleteByNodeID(nodeID string) error {
	idx.mu.Lock()
	ids := idx.byNodeID[nodeID]
	for _, id := range ids {
		delete(idx.entries, id)
	}
	delete(idx.byNodeID, nodeID)
	idx.mu.Unlock()

	return idx.Save()
}

// Search queries the vector index using cosine similarity against queryVector.
func (idx *VectorIndex) Search(queryVector []float32, topK int, filter *VectorFilter) []VectorSearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if topK <= 0 {
		topK = 10
	}

	var results []VectorSearchResult

	for _, entry := range idx.entries {
		if filter != nil {
			if filter.Language != "" && entry.Language != filter.Language {
				continue
			}
			if filter.NodeType != "" && entry.NodeType != filter.NodeType {
				continue
			}
			if filter.AgentID != "" && entry.AgentID != filter.AgentID {
				continue
			}
			if filter.SessionID != "" && entry.SessionID != filter.SessionID {
				continue
			}
			if len(filter.Tags) > 0 {
				hasTag := false
				for _, reqTag := range filter.Tags {
					for _, eTag := range entry.Tags {
						if eTag == reqTag {
							hasTag = true
							break
						}
					}
					if hasTag {
						break
					}
				}
				if !hasTag {
					continue
				}
			}
		}

		score := CosineSimilarity(queryVector, entry.Vector)
		if filter != nil && filter.MinScore > 0 && score < filter.MinScore {
			continue
		}

		results = append(results, VectorSearchResult{
			Entry: entry,
			Score: score,
		})
	}

	// Sort descending by similarity score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// SearchByText embeds queryText on-the-fly and finds nearest vector entries.
func (idx *VectorIndex) SearchByText(queryText string, topK int, filter *VectorFilter) ([]VectorSearchResult, error) {
	vec, err := idx.embedder.Embed(queryText)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query text: %w", err)
	}
	return idx.Search(vec, topK, filter), nil
}

// SearchByIntent searches for nodes matching natural language intent descriptions.
func (idx *VectorIndex) SearchByIntent(intent string, topK int, filter *VectorFilter) ([]VectorSearchResult, error) {
	return idx.SearchByText(intent, topK, filter)
}

// List returns all vector entries in the index.
func (idx *VectorIndex) List() []VectorEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var result []VectorEntry
	for _, e := range idx.entries {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// Close saves and closes the VectorIndex.
func (idx *VectorIndex) Close() error {
	return idx.Save()
}

// CosineSimilarity calculates the cosine of the angle between two float32 vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0.0
	}

	var dotProduct float32
	var normA float32
	var normB float32

	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	denom := float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB)))
	if denom == 0 {
		return 0.0
	}

	return dotProduct / denom
}
