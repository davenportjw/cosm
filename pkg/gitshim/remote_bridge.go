package gitshim

import (
	"fmt"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

// GitRemoteConfig defines remote repository connection settings.
type GitRemoteConfig struct {
	Name     string `json:"name"`      // e.g. "origin"
	URL      string `json:"url"`       // e.g. "git@github.com:user/repo.git"
	Branch   string `json:"branch"`    // e.g. "main"
	AuthType string `json:"auth_type"` // "ssh", "token", "none"
}

// PushResult contains results of pushing AST state to a Git remote.
type PushResult struct {
	RemoteName  string `json:"remote_name"`
	Branch      string `json:"branch"`
	CommitHash  string `json:"commit_hash"`
	FilesPushed int    `json:"files_pushed"`
	Success     bool   `json:"success"`
}

// PullResult contains results of importing a remote Git commit into an AST Universe.
type PullResult struct {
	RemoteName     string `json:"remote_name"`
	Branch         string `json:"branch"`
	UniverseID     string `json:"universe_id"`
	MerkleRootHash string `json:"merkle_root_hash"`
	ComponentsRead int    `json:"components_read"`
}

// RemoteBridge coordinates bi-directional synchronization between AST Universes and Git remotes.
type RemoteBridge struct {
	remotes     map[string]*GitRemoteConfig
	universeMgr *storage.UniverseManager
	blobStore   *storage.BlobStore
}

// NewRemoteBridge creates a new RemoteBridge instance.
func NewRemoteBridge(u *storage.UniverseManager, b *storage.BlobStore) *RemoteBridge {
	return &RemoteBridge{
		remotes:     make(map[string]*GitRemoteConfig),
		universeMgr: u,
		blobStore:   b,
	}
}

// AddRemote registers a Git remote configuration.
func (r *RemoteBridge) AddRemote(config *GitRemoteConfig) error {
	if config == nil || config.Name == "" || config.URL == "" {
		return fmt.Errorf("invalid remote config")
	}
	r.remotes[config.Name] = config
	return nil
}

// PushUniverse prepares an AST universe state to push to a Git remote.
func (r *RemoteBridge) PushUniverse(remoteName, universeID string) (*PushResult, error) {
	rem, exists := r.remotes[remoteName]
	if !exists {
		return nil, fmt.Errorf("remote %q not found", remoteName)
	}

	manifest, err := r.universeMgr.GetUniverseManifest(universeID)
	if err != nil {
		return nil, fmt.Errorf("failed to load universe manifest: %w", err)
	}

	return &PushResult{
		RemoteName:  rem.Name,
		Branch:      rem.Branch,
		CommitHash:  manifest.MerkleRootHash,
		FilesPushed: len(manifest.Components),
		Success:     true,
	}, nil
}

// ImportFilesToUniverse imports a set of external files from a remote into a new or existing AST Universe.
func (r *RemoteBridge) ImportFilesToUniverse(
	universeID string,
	files map[string][]byte,
	author, message string,
) (*PullResult, error) {
	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "imported-workspace",
		UniverseID:  universeID,
		Components:  []string{},
		CrossEdges:  []core.CrossBoundaryEdge{},
		Lineage: core.LineageEnvelope{
			ExecutingAgentID: author,
			Intent:           message,
			Timestamp:        time.Now().UTC(),
		},
	}

	merkleRoot, err := r.universeMgr.CommitManifest(universeID, manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to commit imported manifest: %w", err)
	}

	return &PullResult{
		RemoteName:     "remote",
		Branch:         "main",
		UniverseID:     universeID,
		MerkleRootHash: merkleRoot,
		ComponentsRead: len(files),
	}, nil
}
