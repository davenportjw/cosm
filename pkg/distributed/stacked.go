package distributed

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

// StackedProposal represents an individual PR/Proposal in a change chain (Stack).
type StackedProposal struct {
	ChangeID       string               `json:"change_id"`   // Stable ID (e.g., "c/auth-v2", "c/billing-api")
	ProposalID     string               `json:"proposal_id"` // COB ID or proposal handle
	Title          string               `json:"title"`
	ParentChangeID string               `json:"parent_change_id"` // Direct parent in stack or "root" / base branch
	UniverseID     string               `json:"universe_id"`      // Working micro-universe for this change
	ManifestHash   string               `json:"manifest_hash"`    // Current Merkle root hash
	AuthorDID      string               `json:"author_did"`
	OrderIndex     int                  `json:"order_index"` // Position in stack (0 = base, 1 = first child, ...)
	Lineage        core.LineageEnvelope `json:"lineage"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

// StackEvolutionResult reports which descendant changes were automatically rebased.
type StackEvolutionResult struct {
	TriggerChangeID string            `json:"trigger_change_id"`
	EvolvedChanges  []string          `json:"evolved_changes"` // ChangeIDs updated
	NewRoots        map[string]string `json:"new_roots"`       // ChangeID -> New Manifest Root Hash
}

// StackManager coordinates stacked pull requests, parent-child change chains, and automatic evolution.
type StackManager struct {
	proposals   map[string]*StackedProposal
	universeMgr *storage.UniverseManager
	blobStore   *storage.BlobStore
	mu          sync.RWMutex
}

// NewStackManager creates a new StackManager.
func NewStackManager(u *storage.UniverseManager, b *storage.BlobStore) *StackManager {
	return &StackManager{
		proposals:   make(map[string]*StackedProposal),
		universeMgr: u,
		blobStore:   b,
	}
}

// RegisterChange adds or updates a change in the stack graph.
func (sm *StackManager) RegisterChange(prop *StackedProposal) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if prop == nil || prop.ChangeID == "" {
		return fmt.Errorf("invalid stacked proposal")
	}
	if prop.CreatedAt.IsZero() {
		prop.CreatedAt = time.Now().UTC()
	}
	prop.UpdatedAt = time.Now().UTC()
	sm.proposals[prop.ChangeID] = prop
	return nil
}

// SaveToFile serializes the stack graph to a JSON file.
func (sm *StackManager) SaveToFile(path string) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	data, err := json.MarshalIndent(sm.proposals, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadFromFile restores the stack graph from a JSON file.
func (sm *StackManager) LoadFromFile(path string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var props map[string]*StackedProposal
	if err := json.Unmarshal(data, &props); err != nil {
		return err
	}
	sm.proposals = props
	return nil
}

// GetChange returns a change by ChangeID.
func (sm *StackManager) GetChange(changeID string) (*StackedProposal, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	p, exists := sm.proposals[changeID]
	return p, exists
}

// ListStack returns all changes in topological / stack order.
func (sm *StackManager) ListStack() []*StackedProposal {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	res := make([]*StackedProposal, 0, len(sm.proposals))
	for _, p := range sm.proposals {
		res = append(res, p)
	}

	sort.Slice(res, func(i, j int) bool {
		if res[i].OrderIndex == res[j].OrderIndex {
			return res[i].CreatedAt.Before(res[j].CreatedAt)
		}
		return res[i].OrderIndex < res[j].OrderIndex
	})
	return res
}

// GetDescendants finds all dependent changes stacked on top of targetChangeID.
func (sm *StackManager) GetDescendants(targetChangeID string) []*StackedProposal {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	descendants := make([]*StackedProposal, 0)
	queue := []string{targetChangeID}
	visited := make(map[string]bool)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, p := range sm.proposals {
			if p.ParentChangeID == curr && !visited[p.ChangeID] {
				visited[p.ChangeID] = true
				descendants = append(descendants, p)
				queue = append(queue, p.ChangeID)
			}
		}
	}

	sort.Slice(descendants, func(i, j int) bool {
		return descendants[i].OrderIndex < descendants[j].OrderIndex
	})
	return descendants
}

// EvolveStack automatically propagates changes from a modified parent proposal to all descendant changes in the stack.
func (sm *StackManager) EvolveStack(parentChangeID string, newParentManifestHash string) (*StackEvolutionResult, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	parent, exists := sm.proposals[parentChangeID]
	if !exists {
		return nil, fmt.Errorf("parent change %s not found in stack", parentChangeID)
	}

	parent.ManifestHash = newParentManifestHash
	parent.UpdatedAt = time.Now().UTC()

	result := &StackEvolutionResult{
		TriggerChangeID: parentChangeID,
		EvolvedChanges:  make([]string, 0),
		NewRoots:        make(map[string]string),
	}

	// Find descendants
	descendants := make([]*StackedProposal, 0)
	queue := []string{parentChangeID}
	visited := make(map[string]bool)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, p := range sm.proposals {
			if p.ParentChangeID == curr && !visited[p.ChangeID] {
				visited[p.ChangeID] = true
				descendants = append(descendants, p)
				queue = append(queue, p.ChangeID)
			}
		}
	}

	sort.Slice(descendants, func(i, j int) bool {
		return descendants[i].OrderIndex < descendants[j].OrderIndex
	})

	// Rebase each descendant in order
	for _, child := range descendants {
		// In an AST Merkle-DAG, rebasing applies the child's AST delta onto the updated parent manifest
		childManifest, err := sm.universeMgr.GetUniverseManifest(child.UniverseID)
		if err != nil || childManifest == nil {
			continue
		}

		// Rebase: merge updated parent into child universe using union strategy
		mergedManifest, err := sm.universeMgr.MergeUniverse(parent.UniverseID, child.UniverseID, "union")
		if err == nil && mergedManifest != nil {
			newHash, _ := core.HashWorkspaceManifest(mergedManifest)
			child.ManifestHash = newHash
			child.UpdatedAt = time.Now().UTC()
			result.EvolvedChanges = append(result.EvolvedChanges, child.ChangeID)
			result.NewRoots[child.ChangeID] = newHash
		}
	}

	return result, nil
}
