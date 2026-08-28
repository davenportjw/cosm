package gitshim

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/materialize"
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
	Mode     string `json:"mode"`     // "100644" for file, "040000" for dir
	Path     string `json:"path"`     // Relative path
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
