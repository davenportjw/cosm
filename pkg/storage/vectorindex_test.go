package storage

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestVectorIndex_EmbedderAndCosineSimilarity(t *testing.T) {
	embedder := NewDeterministicEmbedder(128)

	vec1, err := embedder.Embed("implement user authentication jwt handler")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(vec1) != 128 {
		t.Fatalf("expected vector length 128, got %d", len(vec1))
	}

	vec2, err := embedder.Embed("implement user authentication jwt handler")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	// Determinism check
	simIdentical := CosineSimilarity(vec1, vec2)
	if simIdentical < 0.999 {
		t.Errorf("expected near 1.0 for identical text, got %f", simIdentical)
	}

	// Semantic similarity check
	vecSimilar, _ := embedder.Embed("user login and jwt token authorization")
	simSimilar := CosineSimilarity(vec1, vecSimilar)

	vecUnrelated, _ := embedder.Embed("provision kubernetes terraform cluster in aws")
	simUnrelated := CosineSimilarity(vec1, vecUnrelated)

	if simSimilar <= simUnrelated {
		t.Errorf("expected similar text score (%f) to exceed unrelated text score (%f)", simSimilar, simUnrelated)
	}
}

func TestVectorIndex_CRUDAndPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fg-vector-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	idx, err := NewVectorIndex(tmpDir, 128, nil)
	if err != nil {
		t.Fatalf("NewVectorIndex failed: %v", err)
	}

	entry1 := &VectorEntry{
		NodeID:     "node-auth-jwt",
		Intent:     "Implement user authentication handler",
		UserPrompt: "Add JWT authentication handler in Go",
		Rationale:  "Security requirement for API endpoints",
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		AgentID:    "agent-backend",
		SessionID:  "sess-001",
		Tags:       []string{"auth", "jwt", "security"},
		CreatedAt:  time.Now().UTC(),
	}

	entry2 := &VectorEntry{
		NodeID:     "node-infra-db",
		Intent:     "Provision PostgreSQL Cloud SQL database in Terraform",
		UserPrompt: "Add terraform config for postgres",
		Language:   core.LangHCL,
		NodeType:   "ResourceBlock",
		AgentID:    "agent-infra",
		SessionID:  "sess-002",
		Tags:       []string{"database", "postgres", "infra"},
		CreatedAt:  time.Now().UTC(),
	}

	entry3 := &VectorEntry{
		NodeID:     "node-ui-login",
		Intent:     "Create login form component in React TSX",
		UserPrompt: "Build user login form UI",
		Language:   core.LangTypeScript,
		NodeType:   "ReactComponent",
		AgentID:    "agent-frontend",
		SessionID:  "sess-003",
		Tags:       []string{"auth", "ui", "login"},
		CreatedAt:  time.Now().UTC(),
	}

	if err := idx.InsertBatch([]*VectorEntry{entry1, entry2, entry3}); err != nil {
		t.Fatalf("InsertBatch failed: %v", err)
	}

	// Verify retrieval by ID
	retrieved, err := idx.Get(entry1.ID)
	if err != nil || retrieved.NodeID != "node-auth-jwt" {
		t.Fatalf("Get failed: %v, got %+v", err, retrieved)
	}

	// Verify retrieval by NodeID
	nodeEntries, err := idx.GetByNodeID("node-auth-jwt")
	if err != nil || len(nodeEntries) != 1 {
		t.Fatalf("GetByNodeID failed: %v, count=%d", err, len(nodeEntries))
	}

	// Test Search by Intent
	results, err := idx.SearchByIntent("user login auth token", 5, nil)
	if err != nil {
		t.Fatalf("SearchByIntent failed: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	// Top result should be auth or login related
	if results[0].Entry.NodeID != "node-auth-jwt" && results[0].Entry.NodeID != "node-ui-login" {
		t.Errorf("expected top result to be auth related, got %s", results[0].Entry.NodeID)
	}

	// Test Filtered Search
	goFilter := &VectorFilter{Language: core.LangGo}
	goResults, err := idx.SearchByText("authentication", 5, goFilter)
	if err != nil {
		t.Fatalf("SearchByText failed: %v", err)
	}
	for _, r := range goResults {
		if r.Entry.Language != core.LangGo {
			t.Errorf("expected only Go results, got %s", r.Entry.Language)
		}
	}

	// Close and reopen index from disk (test crash persistence)
	_ = idx.Close()

	reloadedIdx, err := NewVectorIndex(tmpDir, 128, nil)
	if err != nil {
		t.Fatalf("reloading VectorIndex failed: %v", err)
	}

	if len(reloadedIdx.List()) != 3 {
		t.Errorf("expected 3 entries in reloaded index, got %d", len(reloadedIdx.List()))
	}

	// Test Delete
	if err := reloadedIdx.Delete(entry1.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if len(reloadedIdx.List()) != 2 {
		t.Errorf("expected 2 entries after delete, got %d", len(reloadedIdx.List()))
	}
}

func TestVectorIndex_ConcurrentAccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fg-vector-concurrent-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	idx, err := NewVectorIndex(tmpDir, 128, nil)
	if err != nil {
		t.Fatalf("NewVectorIndex failed: %v", err)
	}
	defer idx.Close()

	var wg sync.WaitGroup
	numWorkers := 10

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			entry := &VectorEntry{
				NodeID:     "node-worker",
				Intent:     "Worker task intent execution",
				UserPrompt: "Execute prompt for worker",
				Language:   core.LangGo,
				AgentID:    "agent-worker",
			}
			_ = idx.Insert(entry)
			_, _ = idx.SearchByIntent("worker task", 5, nil)
			_ = idx.List()
			if workerID%2 == 0 {
				_ = idx.Save()
			}
		}()
	}

	wg.Wait()
}
