package gitshim

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/materialize"
	"github.com/cosmscm/cosm/pkg/storage"
)

// GitStatusResult holds status output from the Git shim.
type GitStatusResult struct {
	UniverseID     string   `json:"universe_id"`
	StagedFiles    []string `json:"staged_files"`
	ModifiedFiles  []string `json:"modified_files"`
	UntrackedFiles []string `json:"untracked_files"`
	Clean          bool     `json:"clean"`
}

// GitCommitOptions defines parameters for a git commit simulation.
type GitCommitOptions struct {
	Message             string
	Author              string
	UserID              string
	UserPrompt          string
	SessionID           string
	OrchestratorAgentID string
	ExecutingAgentID    string
	LLMVersion          string
}

// GitLogEntry represents a log commit entry.
type GitLogEntry struct {
	CommitHash string    `json:"commit_hash"`
	Author     string    `json:"author"`
	Date       time.Time `json:"date"`
	Message    string    `json:"message"`
	UniverseID string    `json:"universe_id"`
	MerkleRoot string    `json:"merkle_root"`
}

// GitShimInterceptor translates standard Git CLI semantics into AST Merkle-DAG operations.
type GitShimInterceptor struct {
	universeMgr *storage.UniverseManager
	blobStore   *storage.BlobStore
	graphEngine *storage.GraphEngine
	adapter     *SyntheticRepoAdapter
	hydrator    *materialize.Hydrator
}

// NewGitShimInterceptor creates a new GitShimInterceptor instance.
func NewGitShimInterceptor(
	u *storage.UniverseManager,
	b *storage.BlobStore,
	g *storage.GraphEngine,
) *GitShimInterceptor {
	return &GitShimInterceptor{
		universeMgr: u,
		blobStore:   b,
		graphEngine: g,
		adapter:     NewSyntheticRepoAdapter(),
		hydrator:    materialize.NewHydrator(),
	}
}

// Status checks working tree vs head manifest for a universe.
func (i *GitShimInterceptor) Status(universeID string, workingTree map[string][]byte) (*GitStatusResult, error) {
	manifest, err := i.universeMgr.GetUniverseManifest(universeID)
	if err != nil && !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "no committed manifest") {
		return nil, fmt.Errorf("failed to get universe manifest: %w", err)
	}

	res := &GitStatusResult{
		UniverseID:     universeID,
		StagedFiles:    []string{},
		ModifiedFiles:  []string{},
		UntrackedFiles: []string{},
		Clean:          true,
	}

	if manifest == nil {
		for f := range workingTree {
			res.UntrackedFiles = append(res.UntrackedFiles, f)
		}
		sort.Strings(res.UntrackedFiles)
		res.Clean = len(res.UntrackedFiles) == 0
		return res, nil
	}

	// Fetch head components and symbols
	compMap, symMap := i.loadManifestNodes(manifest)

	hydrated, err := i.hydrator.HydrateWorkspace(manifest, compMap, symMap)
	if err != nil {
		hydrated = make(map[string][]byte)
	}

	for path, workContent := range workingTree {
		headContent, exists := hydrated[path]
		if !exists {
			res.UntrackedFiles = append(res.UntrackedFiles, path)
		} else if string(headContent) != string(workContent) {
			res.ModifiedFiles = append(res.ModifiedFiles, path)
		}
	}

	sort.Strings(res.ModifiedFiles)
	sort.Strings(res.UntrackedFiles)
	res.Clean = len(res.ModifiedFiles) == 0 && len(res.UntrackedFiles) == 0
	return res, nil
}

// Diff returns a unified textual diff between universe head and given working tree.
func (i *GitShimInterceptor) Diff(universeID string, workingTree map[string][]byte) (string, error) {
	manifest, err := i.universeMgr.GetUniverseManifest(universeID)
	if err != nil {
		return "", fmt.Errorf("failed to get head manifest: %w", err)
	}

	compMap, symMap := i.loadManifestNodes(manifest)

	hydrated, err := i.hydrator.HydrateWorkspace(manifest, compMap, symMap)
	if err != nil {
		hydrated = make(map[string][]byte)
	}

	var diffBuilder strings.Builder
	for path, newContent := range workingTree {
		oldContent, exists := hydrated[path]
		if !exists {
			diffBuilder.WriteString(fmt.Sprintf("diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1 @@\n+%s\n", path, path, path, string(newContent)))
		} else if string(oldContent) != string(newContent) {
			diffBuilder.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n- %s\n+ %s\n", path, path, path, path, string(oldContent), string(newContent)))
		}
	}

	return diffBuilder.String(), nil
}

// Commit records an AST manifest commit to the universe and generates a synthetic Git commit.
func (i *GitShimInterceptor) Commit(
	universeID string,
	manifest *core.WorkspaceManifestNode,
	opts GitCommitOptions,
) (*SyntheticGitCommit, error) {
	if manifest == nil {
		return nil, fmt.Errorf("manifest cannot be nil")
	}

	manifest.UniverseID = universeID
	manifest.Lineage = core.LineageEnvelope{
		UserID:              opts.UserID,
		UserPrompt:          opts.UserPrompt,
		SessionID:           opts.SessionID,
		OrchestratorAgentID: opts.OrchestratorAgentID,
		ExecutingAgentID:    opts.ExecutingAgentID,
		LLMVersion:          opts.LLMVersion,
		Intent:              opts.Message,
		Timestamp:           time.Now().UTC(),
	}

	compMap, symMap := i.loadManifestNodes(manifest)

	_, err := i.universeMgr.CommitManifest(universeID, manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to commit manifest in universe manager: %w", err)
	}

	parentCommitHash := ""
	commit, _, _, err := i.adapter.SynthesizeObjectsFromManifest(manifest, compMap, symMap, parentCommitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to synthesize git commit: %w", err)
	}

	return commit, nil
}

// Log lists commits for a universe from the event Oplog.
func (i *GitShimInterceptor) Log(universeID string) ([]*GitLogEntry, error) {
	events, err := i.graphEngine.Oplog().GetEvents(universeID, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get oplog events: %w", err)
	}

	entries := make([]*GitLogEntry, 0)
	for _, evt := range events {
		if evt.Action == storage.ActionCommitManifest || evt.Action == storage.ActionRevertManifest {
			author := "cosm-agent <agent@cosm.local>"
			msg := fmt.Sprintf("Commit event %d", evt.EventID)
			if evt.Action == storage.ActionRevertManifest {
				msg = fmt.Sprintf("Revert event %d", evt.EventID)
			}
			shortUUID := evt.EventUUID
			if len(shortUUID) > 12 {
				shortUUID = shortUUID[:12]
			}
			entries = append(entries, &GitLogEntry{
				CommitHash: fmt.Sprintf("sha1-%s", shortUUID),
				Author:     author,
				Date:       time.UnixMilli(evt.TimestampMs),
				Message:    msg,
				UniverseID: evt.UniverseID,
				MerkleRoot: evt.Payload,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date.After(entries[j].Date)
	})
	return entries, nil
}

// Revert creates an inverse commit that undoes the changes of the specified commit.
func (i *GitShimInterceptor) Revert(
	universeID string,
	commitHash string,
	opts GitCommitOptions,
) (*SyntheticGitCommit, error) {
	targetManifestHash, err := i.resolveCommitHash(universeID, commitHash)
	if err != nil {
		return nil, err
	}

	lineage := core.LineageEnvelope{
		UserID:              opts.UserID,
		UserPrompt:          opts.UserPrompt,
		SessionID:           opts.SessionID,
		OrchestratorAgentID: opts.OrchestratorAgentID,
		ExecutingAgentID:    opts.ExecutingAgentID,
		LLMVersion:          opts.LLMVersion,
		Intent:              opts.Message,
		Timestamp:           time.Now().UTC(),
	}

	revertedManifest, err := i.universeMgr.RevertCommit(universeID, targetManifestHash, opts.Message, lineage)
	if err != nil {
		return nil, err
	}

	compMap, symMap := i.loadManifestNodes(revertedManifest)
	parentCommitHash := ""
	commit, _, _, err := i.adapter.SynthesizeObjectsFromManifest(revertedManifest, compMap, symMap, parentCommitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to synthesize git commit: %w", err)
	}

	return commit, nil
}

// Reset points the universe head to the target commit manifest.
func (i *GitShimInterceptor) Reset(
	universeID string,
	commitHash string,
	hard bool,
	workingTree map[string][]byte,
) error {
	if i.universeMgr.IsLedgerMode() {
		return storage.ErrLedgerLinearityViolation
	}

	targetManifestHash, err := i.resolveCommitHash(universeID, commitHash)
	if err != nil {
		return err
	}

	manifest, err := i.universeMgr.ResetHead(universeID, targetManifestHash, false)
	if err != nil {
		return err
	}

	if hard && workingTree != nil {
		compMap, symMap := i.loadManifestNodes(manifest)
		hydrated, err := i.hydrator.HydrateWorkspace(manifest, compMap, symMap)
		if err != nil {
			hydrated = make(map[string][]byte)
		}

		for k := range workingTree {
			if _, exists := hydrated[k]; !exists {
				delete(workingTree, k)
			}
		}
		for k, v := range hydrated {
			workingTree[k] = v
		}
	}

	return nil
}

// resolveCommitHash resolves a commit hash (synthetic sha1, short hex prefix, or Merkle root hash)
// to the target manifest hash.
func (i *GitShimInterceptor) resolveCommitHash(universeID string, commitHash string) (string, error) {
	if commitHash == "" {
		return "", fmt.Errorf("target commit '%s' not found in universe '%s'", commitHash, universeID)
	}

	events, err := i.graphEngine.Oplog().GetEvents(universeID, 0, 0)
	if err == nil {
		for j := len(events) - 1; j >= 0; j-- {
			evt := events[j]
			var shortUUID string
			if len(evt.EventUUID) >= 12 {
				shortUUID = fmt.Sprintf("sha1-%s", evt.EventUUID[:12])
			} else if len(evt.EventUUID) > 0 {
				shortUUID = fmt.Sprintf("sha1-%s", evt.EventUUID)
			}

			var bHash string
			if evt.Payload != "" {
				sum := sha256.Sum256([]byte(evt.Payload))
				bHash = hex.EncodeToString(sum[:])
			}

			matches := evt.EntityID == commitHash ||
				evt.Payload == commitHash ||
				(len(evt.EntityID) > 0 && strings.HasPrefix(evt.EntityID, commitHash)) ||
				(shortUUID != "" && strings.HasPrefix(shortUUID, commitHash)) ||
				evt.EventUUID == commitHash ||
				(len(evt.EventUUID) > 0 && strings.HasPrefix(evt.EventUUID, commitHash)) ||
				(strings.HasPrefix(commitHash, "sha1-") && strings.HasPrefix(evt.EventUUID, strings.TrimPrefix(commitHash, "sha1-"))) ||
				bHash == commitHash ||
				(len(bHash) > 0 && strings.HasPrefix(bHash, commitHash))

			if matches {
				if evt.EntityID != "" {
					return evt.EntityID, nil
				}
				if evt.Payload != "" {
					var m core.WorkspaceManifestNode
					if err := json.Unmarshal([]byte(evt.Payload), &m); err == nil && m.MerkleRootHash != "" {
						return m.MerkleRootHash, nil
					}
					return evt.Payload, nil
				}
			}
		}
	}

	if has, _ := i.blobStore.Has(commitHash); has {
		return commitHash, nil
	}
	if len(commitHash) < 64 {
		if hashes, err := i.blobStore.List(); err == nil {
			for _, h := range hashes {
				if strings.HasPrefix(h, commitHash) {
					return h, nil
				}
			}
		}
	}

	return "", fmt.Errorf("target commit '%s' not found in universe '%s'", commitHash, universeID)
}

// loadManifestNodes retrieves components and symbols referenced in a workspace manifest.
func (i *GitShimInterceptor) loadManifestNodes(manifest *core.WorkspaceManifestNode) (map[string]*core.ComponentNode, map[string]*core.ASTSymbolNode) {
	compMap := make(map[string]*core.ComponentNode)
	symMap := make(map[string]*core.ASTSymbolNode)
	if manifest == nil {
		return compMap, symMap
	}

	for _, cHash := range manifest.Components {
		data, err := i.blobStore.Get(cHash)
		if err != nil && i.graphEngine != nil {
			if compRec, recErr := i.graphEngine.GetNode(cHash); recErr == nil && compRec != nil && compRec.MerkleHash != "" {
				data, err = i.blobStore.Get(compRec.MerkleHash)
			}
		}
		if err == nil {
			var comp core.ComponentNode
			if e := json.Unmarshal(data, &comp); e == nil {
				compCopy := comp
				compMap[compCopy.ComponentID] = &compCopy
				compMap[cHash] = &compCopy
				for _, sHash := range compCopy.SymbolNodes {
					sData, sErr := i.blobStore.Get(sHash)
					if sErr != nil && i.graphEngine != nil {
						if symRec, recErr := i.graphEngine.GetNode(sHash); recErr == nil && symRec != nil && symRec.MerkleHash != "" {
							sData, sErr = i.blobStore.Get(symRec.MerkleHash)
						}
					}
					if sErr == nil {
						var sym core.ASTSymbolNode
						if se := json.Unmarshal(sData, &sym); se == nil {
							symCopy := sym
							symMap[symCopy.NodeID] = &symCopy
							symMap[sHash] = &symCopy
						}
					}
				}
			}
		}
	}

	return compMap, symMap
}
