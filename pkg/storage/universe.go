package storage

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

// UniverseDiff encapsulates differences between two micro-universes.
type UniverseDiff struct {
	UniverseA         string                   `json:"universe_a"`
	UniverseB         string                   `json:"universe_b"`
	ManifestHashA     string                   `json:"manifest_hash_a"`
	ManifestHashB     string                   `json:"manifest_hash_b"`
	Identical         bool                     `json:"identical"`
	AddedComponents   []string                 `json:"added_components"`   // In B but not A
	RemovedComponents []string                 `json:"removed_components"` // In A but not B
	AddedEdges        []core.CrossBoundaryEdge `json:"added_edges"`        // In B but not A
	RemovedEdges      []core.CrossBoundaryEdge `json:"removed_edges"`      // In A but not B
}

// UniverseManager coordinates parallel micro-universes, branch head pointers, and manifest commits.
type UniverseManager struct {
	graphEngine *GraphEngine
	blobStore   *BlobStore
	mu          sync.RWMutex
}

// NewUniverseManager constructs a UniverseManager with the provided GraphEngine and BlobStore.
func NewUniverseManager(ge *GraphEngine, bs *BlobStore) *UniverseManager {
	return &UniverseManager{
		graphEngine: ge,
		blobStore:   bs,
	}
}

// CreateUniverse forks or initializes a new micro-universe.
// If parentUniverseID is specified, the new universe branches from the parent's current head manifest.
func (u *UniverseManager) CreateUniverse(universeID, parentUniverseID string) (*UniverseHeadRecord, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if universeID == "" {
		return nil, fmt.Errorf("universe_id cannot be empty")
	}

	// Check if universe already exists
	if existing, err := u.graphEngine.GetUniverseHead(universeID); err == nil && existing != nil {
		return nil, fmt.Errorf("universe already exists: %s", universeID)
	}

	var headManifestHash string
	if parentUniverseID != "" {
		parentHead, err := u.graphEngine.GetUniverseHead(parentUniverseID)
		if err != nil {
			return nil, fmt.Errorf("parent universe not found %s: %w", parentUniverseID, err)
		}
		headManifestHash = parentHead.HeadManifestHash
	}

	headRecord := UniverseHeadRecord{
		UniverseID:       universeID,
		HeadManifestHash: headManifestHash,
		ParentUniverseID: parentUniverseID,
		Status:           "active",
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}

	if err := u.graphEngine.PutUniverseHead(headRecord); err != nil {
		return nil, fmt.Errorf("failed to save universe head: %w", err)
	}

	// Record in Oplog
	payloadBytes, _ := json.Marshal(headRecord)
	ev := &OplogEvent{
		UniverseID:  universeID,
		Action:      ActionCreateUniverse,
		EntityType:  EntityUniverse,
		EntityID:    universeID,
		Payload:     string(payloadBytes),
		UndoPayload: "",
	}
	_ = u.graphEngine.Oplog().AppendEvent(ev)

	return &headRecord, nil
}

// GetUniverse retrieves universe metadata and head pointer.
func (u *UniverseManager) GetUniverse(universeID string) (*UniverseHeadRecord, error) {
	return u.graphEngine.GetUniverseHead(universeID)
}

// ListUniverses returns all registered micro-universes.
func (u *UniverseManager) ListUniverses() []UniverseHeadRecord {
	return u.graphEngine.ListUniverseHeads()
}

// SetUniverseHead explicitly moves a universe head pointer to a new manifest hash.
func (u *UniverseManager) SetUniverseHead(universeID, manifestHash string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	head, err := u.graphEngine.GetUniverseHead(universeID)
	if err != nil {
		return fmt.Errorf("universe not found: %s", universeID)
	}

	oldHead := *head
	head.HeadManifestHash = manifestHash
	head.UpdatedAt = time.Now().UTC()

	if err := u.graphEngine.PutUniverseHead(*head); err != nil {
		return err
	}

	_, _ = u.graphEngine.Oplog().RecordUniverseHeadUpdate(universeID, oldHead, *head)
	return nil
}

// CommitManifest hashes the WorkspaceManifestNode, persists it in BlobStore, updates the universe head pointer,
// indices edges in GraphEngine, and logs the commit event.
func (u *UniverseManager) CommitManifest(universeID string, manifest *core.WorkspaceManifestNode) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if manifest == nil {
		return "", fmt.Errorf("manifest cannot be nil")
	}

	manifest.UniverseID = universeID
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = time.Now().UTC()
	}

	// 1. Calculate deterministic Merkle root hash
	manifestHash, err := core.HashWorkspaceManifest(manifest)
	if err != nil {
		return "", fmt.Errorf("failed to compute manifest merkle hash: %w", err)
	}

	// 2. Persist serialized manifest to content-addressed BlobStore
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("failed to serialize manifest: %w", err)
	}

	blobHash, err := u.blobStore.Put(manifestJSON)
	if err != nil {
		return "", fmt.Errorf("failed to persist manifest blob: %w", err)
	}

	// 3. Index cross-boundary edges in GraphEngine
	for _, ce := range manifest.CrossEdges {
		edgeRec := EdgeRecord{
			SourceID:   ce.SourceNodeID,
			TargetID:   ce.TargetNodeID,
			EdgeType:   ce.Type,
			ContractID: ce.ContractSchemaID,
			Metadata:   ce.Metadata,
		}
		_ = u.graphEngine.PutEdge(edgeRec)
	}

	// 4. Update Universe Head
	var oldHead UniverseHeadRecord
	head, err := u.graphEngine.GetUniverseHead(universeID)
	if err != nil {
		// Auto-initialize universe if not exists
		newHead := UniverseHeadRecord{
			UniverseID:       universeID,
			HeadManifestHash: blobHash,
			Status:           "active",
			CreatedAt:        time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
		}
		if err := u.graphEngine.PutUniverseHead(newHead); err != nil {
			return "", err
		}
	} else {
		oldHead = *head
		head.HeadManifestHash = blobHash
		head.UpdatedAt = time.Now().UTC()
		if err := u.graphEngine.PutUniverseHead(*head); err != nil {
			return "", err
		}
		_, _ = u.graphEngine.Oplog().RecordUniverseHeadUpdate(universeID, oldHead, *head)
	}

	// 5. Append commit event to Oplog
	ev := &OplogEvent{
		UniverseID:  universeID,
		Action:      ActionCommitManifest,
		EntityType:  EntityManifest,
		EntityID:    manifestHash,
		Payload:     string(manifestJSON),
		UndoPayload: oldHead.HeadManifestHash,
	}
	_ = u.graphEngine.Oplog().AppendEvent(ev)

	return blobHash, nil
}

// GetUniverseManifest loads and unmarshals the active WorkspaceManifestNode for a universe.
func (u *UniverseManager) GetUniverseManifest(universeID string) (*core.WorkspaceManifestNode, error) {
	head, err := u.graphEngine.GetUniverseHead(universeID)
	if err != nil {
		return nil, fmt.Errorf("universe not found: %s", universeID)
	}

	if head.HeadManifestHash == "" {
		return nil, fmt.Errorf("universe %s has no committed manifest", universeID)
	}

	blobData, err := u.blobStore.Get(head.HeadManifestHash)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest blob %s: %w", head.HeadManifestHash, err)
	}

	var manifest core.WorkspaceManifestNode
	if err := json.Unmarshal(blobData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest blob: %w", err)
	}

	return &manifest, nil
}

// DiffUniverses compares the head manifests of two micro-universes.
func (u *UniverseManager) DiffUniverses(universeAID, universeBID string) (*UniverseDiff, error) {
	headA, err := u.graphEngine.GetUniverseHead(universeAID)
	if err != nil {
		return nil, fmt.Errorf("universe A (%s) not found: %w", universeAID, err)
	}
	headB, err := u.graphEngine.GetUniverseHead(universeBID)
	if err != nil {
		return nil, fmt.Errorf("universe B (%s) not found: %w", universeBID, err)
	}

	diff := &UniverseDiff{
		UniverseA:     universeAID,
		UniverseB:     universeBID,
		ManifestHashA: headA.HeadManifestHash,
		ManifestHashB: headB.HeadManifestHash,
		Identical:     headA.HeadManifestHash == headB.HeadManifestHash && headA.HeadManifestHash != "",
	}

	if diff.Identical {
		return diff, nil
	}

	var manifestA, manifestB *core.WorkspaceManifestNode
	if headA.HeadManifestHash != "" {
		manifestA, _ = u.GetUniverseManifest(universeAID)
	}
	if headB.HeadManifestHash != "" {
		manifestB, _ = u.GetUniverseManifest(universeBID)
	}

	// Compare Components
	compMapA := make(map[string]bool)
	if manifestA != nil {
		for _, c := range manifestA.Components {
			compMapA[c] = true
		}
	}

	compMapB := make(map[string]bool)
	if manifestB != nil {
		for _, c := range manifestB.Components {
			compMapB[c] = true
		}
	}

	for c := range compMapB {
		if !compMapA[c] {
			diff.AddedComponents = append(diff.AddedComponents, c)
		}
	}
	sort.Strings(diff.AddedComponents)

	for c := range compMapA {
		if !compMapB[c] {
			diff.RemovedComponents = append(diff.RemovedComponents, c)
		}
	}
	sort.Strings(diff.RemovedComponents)

	// Compare Edges
	edgeMapA := make(map[string]core.CrossBoundaryEdge)
	if manifestA != nil {
		for _, e := range manifestA.CrossEdges {
			edgeMapA[fmt.Sprintf("%s|%s|%s", e.SourceNodeID, e.TargetNodeID, e.Type)] = e
		}
	}

	edgeMapB := make(map[string]core.CrossBoundaryEdge)
	if manifestB != nil {
		for _, e := range manifestB.CrossEdges {
			edgeMapB[fmt.Sprintf("%s|%s|%s", e.SourceNodeID, e.TargetNodeID, e.Type)] = e
		}
	}

	for k, e := range edgeMapB {
		if _, ok := edgeMapA[k]; !ok {
			diff.AddedEdges = append(diff.AddedEdges, e)
		}
	}

	for k, e := range edgeMapA {
		if _, ok := edgeMapB[k]; !ok {
			diff.RemovedEdges = append(diff.RemovedEdges, e)
		}
	}

	return diff, nil
}

// MergeUniverse merges the source universe into the target universe using either "fast-forward" or "union".
func (u *UniverseManager) MergeUniverse(sourceUniverseID, targetUniverseID string, strategy string) (*core.WorkspaceManifestNode, error) {
	diff, err := u.DiffUniverses(targetUniverseID, sourceUniverseID)
	if err != nil {
		return nil, err
	}

	if diff.Identical {
		return u.GetUniverseManifest(targetUniverseID)
	}

	sourceManifest, err := u.GetUniverseManifest(sourceUniverseID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source universe manifest: %w", err)
	}

	targetHead, err := u.graphEngine.GetUniverseHead(targetUniverseID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target universe head: %w", err)
	}

	// If fast-forward strategy or target is empty, fast-forward directly
	if strategy == "fast-forward" || targetHead.HeadManifestHash == "" {
		mergedManifest := *sourceManifest
		mergedManifest.UniverseID = targetUniverseID
		mergedManifest.CreatedAt = time.Now().UTC()
		_, err := u.CommitManifest(targetUniverseID, &mergedManifest)
		if err != nil {
			return nil, err
		}
		return &mergedManifest, nil
	}

	targetManifest, err := u.GetUniverseManifest(targetUniverseID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target universe manifest: %w", err)
	}

	// Union merge strategy
	compSet := make(map[string]bool)
	for _, c := range targetManifest.Components {
		compSet[c] = true
	}
	for _, c := range sourceManifest.Components {
		compSet[c] = true
	}

	var mergedComponents []string
	for c := range compSet {
		mergedComponents = append(mergedComponents, c)
	}
	sort.Strings(mergedComponents)

	edgeSet := make(map[string]core.CrossBoundaryEdge)
	for _, e := range targetManifest.CrossEdges {
		edgeSet[fmt.Sprintf("%s|%s|%s", e.SourceNodeID, e.TargetNodeID, e.Type)] = e
	}
	for _, e := range sourceManifest.CrossEdges {
		edgeSet[fmt.Sprintf("%s|%s|%s", e.SourceNodeID, e.TargetNodeID, e.Type)] = e
	}

	var mergedEdges []core.CrossBoundaryEdge
	for _, e := range edgeSet {
		mergedEdges = append(mergedEdges, e)
	}

	mergedManifest := &core.WorkspaceManifestNode{
		WorkspaceID: targetManifest.WorkspaceID,
		UniverseID:  targetUniverseID,
		Components:  mergedComponents,
		CrossEdges:  mergedEdges,
		Lineage: core.LineageEnvelope{
			Intent:    fmt.Sprintf("Merge universe %s into %s", sourceUniverseID, targetUniverseID),
			Timestamp: time.Now().UTC(),
		},
		CreatedAt: time.Now().UTC(),
	}

	_, err = u.CommitManifest(targetUniverseID, mergedManifest)
	if err != nil {
		return nil, fmt.Errorf("failed to commit merged manifest: %w", err)
	}

	return mergedManifest, nil
}
