package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

// ErrLedgerLinearityViolation is returned when an interactive rollback or destructive history rewrite
// is attempted while in append-only ledger mode.
var ErrLedgerLinearityViolation = fmt.Errorf("operation prohibited in ledger mode: history must remain strictly linear and append-only")

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
	ledgerMode  bool
	mu          sync.RWMutex
}

// NewUniverseManager constructs a UniverseManager with the provided GraphEngine and BlobStore.
func NewUniverseManager(ge *GraphEngine, bs *BlobStore) *UniverseManager {
	return &UniverseManager{
		graphEngine: ge,
		blobStore:   bs,
	}
}

// SetLedgerMode enables or disables append-only strict ledger mode.
func (u *UniverseManager) SetLedgerMode(mode bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.ledgerMode = mode
}

// IsLedgerMode returns true if ledger mode is enabled on the UniverseManager.
func (u *UniverseManager) IsLedgerMode() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.ledgerMode
}

// GraphEngine returns the underlying GraphEngine.
func (u *UniverseManager) GraphEngine() *GraphEngine {
	if u == nil {
		return nil
	}
	return u.graphEngine
}

// BlobStore returns the underlying BlobStore.
func (u *UniverseManager) BlobStore() *BlobStore {
	if u == nil {
		return nil
	}
	return u.blobStore
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

// RevertCommit creates an inverse commit that restores the parent state of targetHash while
// maintaining append-only lineage and recording an ActionRevertManifest oplog entry.
func (u *UniverseManager) RevertCommit(universeID, targetHash, customIntent string, lineage core.LineageEnvelope) (*core.WorkspaceManifestNode, error) {
	// 1. Fetch target commit manifest from blobStore or oplog (supporting exact hash or prefix)
	events, err := u.graphEngine.Oplog().GetEvents(universeID, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read oplog for universe %s: %w", universeID, err)
	}

	var targetEvent *OplogEvent
	var targetManifest *core.WorkspaceManifestNode

	// Search oplog in reverse chronological order
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Action != ActionCommitManifest && ev.Action != ActionRevertManifest {
			continue
		}
		// Match by EntityID (MerkleRootHash) or prefix
		if ev.EntityID == targetHash || strings.HasPrefix(ev.EntityID, targetHash) {
			targetEvent = ev
			break
		}
		// Match by Payload SHA-256 (blob hash)
		sum := sha256.Sum256([]byte(ev.Payload))
		bHash := hex.EncodeToString(sum[:])
		if bHash == targetHash || strings.HasPrefix(bHash, targetHash) {
			targetEvent = ev
			break
		}
	}

	// If not found in oplog, check blobStore
	if targetEvent == nil {
		var rawBlob []byte
		if len(targetHash) == 64 {
			if has, _ := u.blobStore.Has(targetHash); has {
				rawBlob, _ = u.blobStore.Get(targetHash)
			}
		}
		if len(rawBlob) == 0 {
			if hashes, err := u.blobStore.List(); err == nil {
				for _, h := range hashes {
					if strings.HasPrefix(h, targetHash) {
						rawBlob, _ = u.blobStore.Get(h)
						break
					}
				}
			}
		}
		if len(rawBlob) > 0 {
			var m core.WorkspaceManifestNode
			if err := json.Unmarshal(rawBlob, &m); err == nil {
				targetManifest = &m
				for i := len(events) - 1; i >= 0; i-- {
					ev := events[i]
					if (ev.Action == ActionCommitManifest || ev.Action == ActionRevertManifest) &&
						(ev.EntityID == m.MerkleRootHash || strings.HasPrefix(ev.EntityID, m.MerkleRootHash)) {
						targetEvent = ev
						break
					}
				}
			}
		}
	}

	if targetManifest == nil && targetEvent != nil && targetEvent.Payload != "" {
		var m core.WorkspaceManifestNode
		if err := json.Unmarshal([]byte(targetEvent.Payload), &m); err == nil {
			targetManifest = &m
		}
	}

	if targetManifest == nil {
		return nil, fmt.Errorf("target commit manifest not found: %s", targetHash)
	}

	// 2. Resolve parent manifest hash from the commit event's UndoPayload
	var parentHash string
	if targetEvent != nil {
		parentHash = targetEvent.UndoPayload
	}

	var parentManifest *core.WorkspaceManifestNode
	if parentHash != "" {
		parentData, err := u.blobStore.Get(parentHash)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch parent manifest blob %s: %w", parentHash, err)
		}
		var pm core.WorkspaceManifestNode
		if err := json.Unmarshal(parentData, &pm); err != nil {
			return nil, fmt.Errorf("failed to unmarshal parent manifest blob: %w", err)
		}
		parentManifest = &pm
	}

	// 3. Construct new WorkspaceManifestNode
	var comps []string
	var edges []core.CrossBoundaryEdge
	if parentManifest != nil {
		if len(parentManifest.Components) > 0 {
			comps = make([]string, len(parentManifest.Components))
			copy(comps, parentManifest.Components)
		}
		if len(parentManifest.CrossEdges) > 0 {
			edges = make([]core.CrossBoundaryEdge, len(parentManifest.CrossEdges))
			copy(edges, parentManifest.CrossEdges)
		}
	}
	if comps == nil {
		comps = []string{}
	}
	if edges == nil {
		edges = []core.CrossBoundaryEdge{}
	}

	intent := customIntent
	if intent == "" {
		if targetManifest.Lineage.Intent != "" {
			intent = fmt.Sprintf("Revert: %s", targetManifest.Lineage.Intent)
		} else {
			intent = fmt.Sprintf("Revert: %s", targetHash)
		}
	}

	newLin := lineage
	newLin.Intent = intent
	newLin.Timestamp = time.Now().UTC()
	if newLin.Trace.Attributes == nil {
		newLin.Trace.Attributes = make(map[string]string)
	}
	newLin.Trace.Attributes["reverted_commit"] = targetHash

	newManifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-" + universeID,
		UniverseID:  universeID,
		Components:  comps,
		CrossEdges:  edges,
		Lineage:     newLin,
		CreatedAt:   time.Now().UTC(),
	}

	// 4. Capture current head before commit
	oldHead, err := u.graphEngine.GetUniverseHead(universeID)
	oldHeadHash := ""
	if err == nil && oldHead != nil {
		oldHeadHash = oldHead.HeadManifestHash
	}

	// 5. Commit via CommitManifest
	_, err = u.CommitManifest(universeID, newManifest)
	if err != nil {
		return nil, fmt.Errorf("failed to commit revert manifest: %w", err)
	}

	manifestJSON, err := json.Marshal(newManifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal revert manifest: %w", err)
	}

	// 6. Append ActionRevertManifest to Oplog
	revertEv := &OplogEvent{
		UniverseID:  universeID,
		Action:      ActionRevertManifest,
		EntityType:  EntityManifest,
		EntityID:    newManifest.MerkleRootHash,
		UndoPayload: oldHeadHash,
		Payload:     string(manifestJSON),
	}
	if err := u.graphEngine.Oplog().AppendEvent(revertEv); err != nil {
		return nil, fmt.Errorf("failed to append revert event to oplog: %w", err)
	}

	return newManifest, nil
}

// ResetHead moves the universe head directly to targetHash in non-ledger mode.
// In ledger mode, it returns ErrLedgerLinearityViolation.
func (u *UniverseManager) ResetHead(universeID, targetHash string, isLedgerMode bool) (*core.WorkspaceManifestNode, error) {
	if isLedgerMode || u.IsLedgerMode() {
		return nil, ErrLedgerLinearityViolation
	}

	// Validate targetHash exists in blobStore (supporting exact hash, prefix, or oplog reference)
	resolvedBlobHash := targetHash
	has, _ := u.blobStore.Has(targetHash)
	if !has {
		if len(targetHash) < 64 {
			hashes, _ := u.blobStore.List()
			var matches []string
			for _, h := range hashes {
				if strings.HasPrefix(h, targetHash) {
					matches = append(matches, h)
				}
			}
			if len(matches) == 1 {
				resolvedBlobHash = matches[0]
				has = true
			} else if len(matches) > 1 {
				return nil, fmt.Errorf("ambiguous target hash: %s", targetHash)
			}
		}
	}
	if !has {
		events, _ := u.graphEngine.Oplog().GetEvents(universeID, 0, 0)
		for _, ev := range events {
			if (ev.Action == ActionCommitManifest || ev.Action == ActionRevertManifest) &&
				(ev.EntityID == targetHash || strings.HasPrefix(ev.EntityID, targetHash)) {
				sum := sha256.Sum256([]byte(ev.Payload))
				bHash := hex.EncodeToString(sum[:])
				if h, _ := u.blobStore.Has(bHash); h {
					resolvedBlobHash = bHash
					has = true
					break
				}
			}
		}
	}
	if !has {
		return nil, fmt.Errorf("target hash %s does not exist in blobstore", targetHash)
	}

	oldHead, err := u.graphEngine.GetUniverseHead(universeID)
	if err != nil || oldHead == nil {
		return nil, fmt.Errorf("universe head not found: %s", universeID)
	}

	newHead := *oldHead
	newHead.HeadManifestHash = resolvedBlobHash
	newHead.UpdatedAt = time.Now().UTC()

	if err := u.graphEngine.PutUniverseHead(newHead); err != nil {
		return nil, fmt.Errorf("failed to update universe head: %w", err)
	}

	// Record ActionSetUniverseHead in Oplog with UndoPayload: oldHead.HeadManifestHash
	payloadBytes, _ := json.Marshal(newHead)
	ev := &OplogEvent{
		UniverseID:  universeID,
		Action:      ActionSetUniverseHead,
		EntityType:  EntityUniverse,
		EntityID:    universeID,
		Payload:     string(payloadBytes),
		UndoPayload: oldHead.HeadManifestHash,
	}
	if err := u.graphEngine.Oplog().AppendEvent(ev); err != nil {
		return nil, fmt.Errorf("failed to record set universe head event in oplog: %w", err)
	}

	blobData, err := u.blobStore.Get(resolvedBlobHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load manifest at %s: %w", resolvedBlobHash, err)
	}

	var manifest core.WorkspaceManifestNode
	if err := json.Unmarshal(blobData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest at %s: %w", resolvedBlobHash, err)
	}

	return &manifest, nil
}

// UndoLastCommit reverses the most recent commit. In ledger mode, it performs an append-only
// RevertCommit. In standard mode, it rolls back graph state and universe heads via the oplog.
func (u *UniverseManager) UndoLastCommit(universeID string, isLedgerMode bool, lineage core.LineageEnvelope) (*core.WorkspaceManifestNode, []*OplogEvent, error) {
	if isLedgerMode || u.IsLedgerMode() {
		currentHead, err := u.GetUniverseManifest(universeID)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot undo: failed to retrieve current universe manifest: %w", err)
		}

		events, err := u.graphEngine.Oplog().GetEvents(universeID, 0, 0)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot undo: failed to retrieve oplog events: %w", err)
		}

		var lastCommitEv *OplogEvent
		for i := len(events) - 1; i >= 0; i-- {
			ev := events[i]
			if ev.Action == ActionCommitManifest || ev.Action == ActionRevertManifest {
				lastCommitEv = ev
				break
			}
		}

		if lastCommitEv == nil || lastCommitEv.UndoPayload == "" {
			return nil, nil, fmt.Errorf("cannot undo initial commit")
		}

		revertManifest, err := u.RevertCommit(universeID, currentHead.MerkleRootHash, "Undo: Revert to previous ledger state", lineage)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to revert in ledger mode: %w", err)
		}

		return revertManifest, nil, nil
	}

	// Standard Mode: unroll oplog down to previous commit
	events, err := u.graphEngine.Oplog().GetEvents(universeID, 0, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot undo: failed to retrieve oplog events: %w", err)
	}

	var lastCommitEv *OplogEvent
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Action == ActionCommitManifest || ev.Action == ActionRevertManifest {
			lastCommitEv = ev
			break
		}
	}

	if lastCommitEv == nil {
		return nil, nil, fmt.Errorf("cannot undo: no commits found for universe %s", universeID)
	}

	u.graphEngine.Oplog().mu.RLock()
	allEvents := u.graphEngine.Oplog().events
	lastCommitIdx := -1
	for i := len(allEvents) - 1; i >= 0; i-- {
		if allEvents[i].EventID == lastCommitEv.EventID {
			lastCommitIdx = i
			break
		}
	}
	if lastCommitIdx == -1 {
		u.graphEngine.Oplog().mu.RUnlock()
		return nil, nil, fmt.Errorf("cannot undo: commit event not found in oplog")
	}

	startIdx := lastCommitIdx
	if allEvents[startIdx].Action == ActionRevertManifest && startIdx > 0 &&
		allEvents[startIdx-1].Action == ActionCommitManifest &&
		allEvents[startIdx-1].UniverseID == universeID {
		startIdx--
	}
	if startIdx > 0 && allEvents[startIdx-1].Action == ActionSetUniverseHead &&
		allEvents[startIdx-1].UniverseID == universeID {
		startIdx--
	}

	undoCount := len(allEvents) - startIdx
	u.graphEngine.Oplog().mu.RUnlock()

	undoneEvents, err := u.graphEngine.Oplog().Undo(undoCount)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unroll oplog: %w", err)
	}

	restoredHead, err := u.graphEngine.GetUniverseHead(universeID)
	var restoredManifest *core.WorkspaceManifestNode
	if err == nil && restoredHead != nil && restoredHead.HeadManifestHash != "" {
		restoredManifest, _ = u.GetUniverseManifest(universeID)
	}

	return restoredManifest, undoneEvents, nil
}
