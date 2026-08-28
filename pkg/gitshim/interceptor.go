package gitshim

import (
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
	compMap := make(map[string]*core.ComponentNode)
	symMap := make(map[string]*core.ASTSymbolNode)
	for _, cHash := range manifest.Components {
		data, err := i.blobStore.Get(cHash)
		if err == nil {
			var comp core.ComponentNode
			if e := json.Unmarshal(data, &comp); e == nil {
				compMap[comp.ComponentID] = &comp
				for _, sHash := range comp.SymbolNodes {
					sData, sErr := i.blobStore.Get(sHash)
					if sErr == nil {
						var sym core.ASTSymbolNode
						if se := json.Unmarshal(sData, &sym); se == nil {
							symMap[sym.NodeID] = &sym
						}
					}
				}
			}
		}
	}

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

	compMap := make(map[string]*core.ComponentNode)
	symMap := make(map[string]*core.ASTSymbolNode)
	for _, cHash := range manifest.Components {
		data, err := i.blobStore.Get(cHash)
		if err == nil {
			var comp core.ComponentNode
			if e := json.Unmarshal(data, &comp); e == nil {
				compMap[comp.ComponentID] = &comp
				for _, sHash := range comp.SymbolNodes {
					sData, sErr := i.blobStore.Get(sHash)
					if sErr == nil {
						var sym core.ASTSymbolNode
						if se := json.Unmarshal(sData, &sym); se == nil {
							symMap[sym.NodeID] = &sym
						}
					}
				}
			}
		}
	}

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

	compMap := make(map[string]*core.ComponentNode)
	symMap := make(map[string]*core.ASTSymbolNode)
	for _, cHash := range manifest.Components {
		data, err := i.blobStore.Get(cHash)
		if err == nil {
			var comp core.ComponentNode
			if e := json.Unmarshal(data, &comp); e == nil {
				compMap[comp.ComponentID] = &comp
				for _, sHash := range comp.SymbolNodes {
					sData, sErr := i.blobStore.Get(sHash)
					if sErr == nil {
						var sym core.ASTSymbolNode
						if se := json.Unmarshal(sData, &sym); se == nil {
							symMap[sym.NodeID] = &sym
						}
					}
				}
			}
		}
	}

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
		if evt.Action == storage.ActionCommitManifest {
			author := "cosm-agent <agent@cosm.local>"
			msg := fmt.Sprintf("Commit event %d", evt.EventID)
			entries = append(entries, &GitLogEntry{
				CommitHash: fmt.Sprintf("sha1-%s", evt.EventUUID[:12]),
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
