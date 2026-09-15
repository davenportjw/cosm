package gitshim

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/materialize"
	"github.com/cosmscm/cosm/pkg/storage"
)

// GitObjectType represents Git object types (commit, tree, blob, tag).
type GitObjectType string

const (
	GitObjCommit GitObjectType = "commit"
	GitObjTree   GitObjectType = "tree"
	GitObjBlob   GitObjectType = "blob"
)

// SyntheticGitObject represents an in-memory Git object derived from AST state.
type SyntheticGitObject struct {
	Hash    string        `json:"hash"`
	Type    GitObjectType `json:"type"`
	Size    int           `json:"size"`
	Payload []byte        `json:"payload"`
}

// SyntheticGitCommit represents a virtual Git commit synthesized from an Oplog / Manifest event.
type SyntheticGitCommit struct {
	CommitHash  string    `json:"commit_hash"`
	TreeHash    string    `json:"tree_hash"`
	ParentHash  string    `json:"parent_hash,omitempty"`
	Author      string    `json:"author"`
	Committer   string    `json:"committer"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp"`
	UniverseID  string    `json:"universe_id"`
	ManifestRef string    `json:"manifest_ref"`
}

// SyntheticGitTreeEntry represents a single file or directory inside a synthetic tree.
type SyntheticGitTreeEntry struct {
	Mode     string `json:"mode"`      // "100644" for file, "040000" for dir
	Path     string `json:"path"`      // Relative path
	BlobHash string `json:"blob_hash"` // Synthetic Git blob hash
}

// SyntheticRepoAdapter transforms AST Merkle-DAGs into virtual Git objects.
type SyntheticRepoAdapter struct {
	hydrator *materialize.Hydrator
}

// NewSyntheticRepoAdapter creates a new SyntheticRepoAdapter instance.
func NewSyntheticRepoAdapter() *SyntheticRepoAdapter {
	return &SyntheticRepoAdapter{
		hydrator: materialize.NewHydrator(),
	}
}

// ComputeGitBlobHash calculates the standard Git SHA-1 hash for a blob: sha1("blob <size>\0<content>").
func ComputeGitBlobHash(content []byte) string {
	header := fmt.Sprintf("blob %d\x00", len(content))
	hasher := sha1.New()
	hasher.Write([]byte(header))
	hasher.Write(content)
	return hex.EncodeToString(hasher.Sum(nil))
}

// ComputeGitTreeHash calculates a synthetic Git tree hash from sorted tree entries.
func ComputeGitTreeHash(entries []SyntheticGitTreeEntry) string {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(fmt.Sprintf("%s %s %s\n", e.Mode, e.BlobHash, e.Path))
	}
	content := []byte(b.String())
	header := fmt.Sprintf("tree %d\x00", len(content))
	hasher := sha1.New()
	hasher.Write([]byte(header))
	hasher.Write(content)
	return hex.EncodeToString(hasher.Sum(nil))
}

// ComputeGitCommitHash calculates a synthetic Git commit hash.
func ComputeGitCommitHash(treeHash, parentHash, author, message string, ts time.Time) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("tree %s\n", treeHash))
	if parentHash != "" {
		b.WriteString(fmt.Sprintf("parent %s\n", parentHash))
	}
	b.WriteString(fmt.Sprintf("author %s %d +0000\n", author, ts.Unix()))
	b.WriteString(fmt.Sprintf("committer %s %d +0000\n", author, ts.Unix()))
	b.WriteString(fmt.Sprintf("\n%s\n", message))

	content := []byte(b.String())
	header := fmt.Sprintf("commit %d\x00", len(content))
	hasher := sha1.New()
	hasher.Write([]byte(header))
	hasher.Write(content)
	return hex.EncodeToString(hasher.Sum(nil))
}

// SynthesizeObjectsFromManifest builds synthetic Git blobs, tree, and commit from an AST WorkspaceManifest.
func (a *SyntheticRepoAdapter) SynthesizeObjectsFromManifest(
	manifest *core.WorkspaceManifestNode,
	components map[string]*core.ComponentNode,
	symbols map[string]*core.ASTSymbolNode,
	parentCommitHash string,
) (*SyntheticGitCommit, []SyntheticGitTreeEntry, map[string]*SyntheticGitObject, error) {
	if manifest == nil {
		return nil, nil, nil, fmt.Errorf("manifest cannot be nil")
	}

	hydratedFiles, err := a.hydrator.HydrateWorkspace(manifest, components, symbols)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to hydrate workspace for git synthesis: %w", err)
	}

	objects := make(map[string]*SyntheticGitObject)
	entries := make([]SyntheticGitTreeEntry, 0, len(hydratedFiles))

	for path, content := range hydratedFiles {
		blobHash := ComputeGitBlobHash(content)
		objects[blobHash] = &SyntheticGitObject{
			Hash:    blobHash,
			Type:    GitObjBlob,
			Size:    len(content),
			Payload: content,
		}
		entries = append(entries, SyntheticGitTreeEntry{
			Mode:     "100644",
			Path:     path,
			BlobHash: blobHash,
		})
	}

	treeHash := ComputeGitTreeHash(entries)
	objects[treeHash] = &SyntheticGitObject{
		Hash: treeHash,
		Type: GitObjTree,
		Size: len(entries),
	}

	author := manifest.Lineage.ExecutingAgentID
	if author == "" {
		author = "cosm-agent <agent@cosm.local>"
	} else if !strings.Contains(author, "<") {
		author = fmt.Sprintf("%s <%s@cosm.local>", author, author)
	}

	msg := manifest.Lineage.Intent
	if msg == "" {
		msg = fmt.Sprintf("AST state commit for universe %s", manifest.UniverseID)
	}

	ts := manifest.Lineage.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	commitHash := ComputeGitCommitHash(treeHash, parentCommitHash, author, msg, ts)
	commit := &SyntheticGitCommit{
		CommitHash:  commitHash,
		TreeHash:    treeHash,
		ParentHash:  parentCommitHash,
		Author:      author,
		Committer:   author,
		Message:     msg,
		Timestamp:   ts,
		UniverseID:  manifest.UniverseID,
		ManifestRef: manifest.MerkleRootHash,
	}

	return commit, entries, objects, nil
}

// InitGitBridge writes a synthetic .git directory structure (.git/HEAD, .git/config, .git/objects, refs)
// so that standard IDEs (VS Code, Cursor, Windsurf, Antigravity IDE) and git shims identify the workspace
// as a valid Git repository synchronized with the active micro-universe.
func InitGitBridge(workspaceDir, universeID string) error {
	if universeID == "" {
		universeID = "universe-main"
	}

	gitDir := filepath.Join(workspaceDir, ".git")
	dirsToCreate := []string{
		filepath.Join(gitDir, "objects", "info"),
		filepath.Join(gitDir, "objects", "pack"),
		filepath.Join(gitDir, "refs", "heads"),
		filepath.Join(gitDir, "refs", "tags"),
		filepath.Join(gitDir, "info"),
	}
	for _, d := range dirsToCreate {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}

	// 1. Write .git/HEAD pointing to refs/heads/main
	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		return fmt.Errorf("writing HEAD: %w", err)
	}

	// 2. Write .git/config
	configContent := fmt.Sprintf(`[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
	ignorecase = true
	precomposeunicode = true
[cosm]
	universe = %s
`, universeID)
	configPath := filepath.Join(gitDir, "config")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// 3. Write .git/description
	descPath := filepath.Join(gitDir, "description")
	_ = os.WriteFile(descPath, []byte("Cosm AST SCM synthetic git bridge\n"), 0644)

	// 4. Write .git/info/exclude to ignore .cosm directory
	excludePath := filepath.Join(gitDir, "info", "exclude")
	_ = os.WriteFile(excludePath, []byte(".cosm\n.cosm/*\n"), 0644)

	// 5. If .cosm storage exists, try to synthesize commit and write refs
	cosmDir := filepath.Join(workspaceDir, ".cosm")
	if _, err := os.Stat(filepath.Join(cosmDir, "graph.db")); err == nil {
		blobStore, bErr := storage.NewBlobStore(filepath.Join(cosmDir, "objects"))
		graphEngine, gErr := storage.NewGraphEngine(filepath.Join(cosmDir, "graph.db"))
		if bErr == nil && gErr == nil {
			defer graphEngine.Close()
			universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
			manifest, mErr := universeMgr.GetUniverseManifest(universeID)
			if mErr == nil && manifest != nil {
				// Load components and symbols
				compMap := make(map[string]*core.ComponentNode)
				symMap := make(map[string]*core.ASTSymbolNode)
				for _, cID := range manifest.Components {
					var compData []byte
					if node, _ := graphEngine.GetNode(cID); node != nil && node.MerkleHash != "" {
						compData, _ = blobStore.Get(node.MerkleHash)
					} else {
						compData, _ = blobStore.Get(cID)
					}
					if len(compData) > 0 {
						var c core.ComponentNode
						if e := json.Unmarshal(compData, &c); e == nil {
							cCopy := c
							compMap[c.ComponentID] = &cCopy
							compMap[cID] = &cCopy
							for _, sID := range c.SymbolNodes {
								var sData []byte
								if sNode, _ := graphEngine.GetNode(sID); sNode != nil && sNode.MerkleHash != "" {
									sData, _ = blobStore.Get(sNode.MerkleHash)
								} else {
									sData, _ = blobStore.Get(sID)
								}
								if len(sData) > 0 {
									var s core.ASTSymbolNode
									if se := json.Unmarshal(sData, &s); se == nil {
										sCopy := s
										symMap[s.NodeID] = &sCopy
									}
								}
							}
						}
					}
				}

				adapter := NewSyntheticRepoAdapter()
				commit, _, objects, synErr := adapter.SynthesizeObjectsFromManifest(manifest, compMap, symMap, "")
				if synErr == nil && commit != nil {
					// Write loose git objects
					for _, obj := range objects {
						_ = writeLooseGitObject(gitDir, obj)
					}

					// Write commit object
					var b strings.Builder
					b.WriteString(fmt.Sprintf("tree %s\n", commit.TreeHash))
					if commit.ParentHash != "" {
						b.WriteString(fmt.Sprintf("parent %s\n", commit.ParentHash))
					}
					b.WriteString(fmt.Sprintf("author %s %d +0000\n", commit.Author, commit.Timestamp.Unix()))
					b.WriteString(fmt.Sprintf("committer %s %d +0000\n", commit.Committer, commit.Timestamp.Unix()))
					b.WriteString(fmt.Sprintf("\n%s\n", commit.Message))
					commitPayload := []byte(b.String())

					commitObj := &SyntheticGitObject{
						Hash:    commit.CommitHash,
						Type:    GitObjCommit,
						Size:    len(commitPayload),
						Payload: commitPayload,
					}
					_ = writeLooseGitObject(gitDir, commitObj)

					// Write ref for universe and main
					refMain := filepath.Join(gitDir, "refs", "heads", "main")
					_ = os.WriteFile(refMain, []byte(commit.CommitHash+"\n"), 0644)
					if universeID != "main" && universeID != "universe-main" {
						refUniv := filepath.Join(gitDir, "refs", "heads", universeID)
						_ = os.WriteFile(refUniv, []byte(commit.CommitHash+"\n"), 0644)
					}
				}
			}
		}
	}

	return nil
}

func writeLooseGitObject(gitDir string, obj *SyntheticGitObject) error {
	if len(obj.Hash) < 3 {
		return fmt.Errorf("invalid object hash: %s", obj.Hash)
	}
	dir := filepath.Join(gitDir, "objects", obj.Hash[:2])
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, obj.Hash[2:])
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	var raw bytes.Buffer
	header := fmt.Sprintf("%s %d\x00", obj.Type, obj.Size)
	raw.WriteString(header)
	raw.Write(obj.Payload)

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	return os.WriteFile(path, compressed.Bytes(), 0644)
}

