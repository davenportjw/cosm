package gitshim

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs/golang"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func TestSyntheticRepoAdapter_SynthesizeObjects(t *testing.T) {
	goCode := `package main
import "fmt"
func Hello() { fmt.Println("Hello Git Shim") }
`
	goParser := golang.NewGoParser()
	pkgRes, err := goParser.ParseSource("main.go", []byte(goCode), core.LineageEnvelope{
		ExecutingAgentID: "agent-maker",
		Intent:           "Add hello func",
		Timestamp:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	compGo, err := golang.BuildComponentNode("main", core.CompService, pkgRes, pkgRes.AllSymbols[0].Lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}

	symMap := make(map[string]*core.ASTSymbolNode)
	for _, s := range pkgRes.AllSymbols {
		symMap[s.NodeID] = s
	}

	compMap := map[string]*core.ComponentNode{
		compGo.ComponentID: compGo,
	}

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-test",
		UniverseID:  "universe-main",
		Components:  []string{compGo.ComponentID},
		Lineage: core.LineageEnvelope{
			ExecutingAgentID: "agent-maker",
			Intent:           "Initial synthetic commit",
			Timestamp:        time.Now().UTC(),
		},
	}

	adapter := NewSyntheticRepoAdapter()
	commit, entries, objects, err := adapter.SynthesizeObjectsFromManifest(manifest, compMap, symMap, "")
	if err != nil {
		t.Fatalf("SynthesizeObjectsFromManifest failed: %v", err)
	}

	if commit.CommitHash == "" || commit.TreeHash == "" {
		t.Fatalf("expected commit and tree hashes, got: %+v", commit)
	}
	if len(entries) == 0 {
		t.Fatalf("expected at least 1 tree entry, got %d", len(entries))
	}
	if len(objects) < 2 { // At least 1 blob and 1 tree
		t.Fatalf("expected objects >= 2, got %d", len(objects))
	}
}

func TestGitShimInterceptor_StatusDiffAndCommit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fg-gitshim-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	blobStore, err := storage.NewBlobStore(filepath.Join(tmpDir, "objects"))
	if err != nil {
		t.Fatalf("NewBlobStore failed: %v", err)
	}
	graphEngine, err := storage.NewGraphEngine(filepath.Join(tmpDir, "graph.db"))
	if err != nil {
		t.Fatalf("NewGraphEngine failed: %v", err)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	interceptor := NewGitShimInterceptor(universeMgr, blobStore, graphEngine)

	universeID := "universe-main"
	if _, err := universeMgr.CreateUniverse(universeID, ""); err != nil {
		t.Fatalf("CreateUniverse failed: %v", err)
	}

	// 1. Check status on empty repo
	workTree := map[string][]byte{
		"main.go": []byte("package main\nfunc main() {}\n"),
	}
	status, err := interceptor.Status(universeID, workTree)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Clean {
		t.Fatalf("expected dirty status for untracked files")
	}

	// 2. Commit a manifest
	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-1",
		UniverseID:  universeID,
		Components:  []string{},
		CrossEdges:  []core.CrossBoundaryEdge{},
	}

	commit, err := interceptor.Commit(universeID, manifest, GitCommitOptions{
		Message:          "feat: initial commit",
		ExecutingAgentID: "agent-01",
		UserPrompt:       "Build initial service",
	})
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	if commit.CommitHash == "" {
		t.Fatalf("expected valid commit hash")
	}

	// 3. Check diff
	diff, err := interceptor.Diff(universeID, workTree)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if len(diff) == 0 {
		t.Fatalf("expected non-empty diff")
	}

	// 4. Check log
	logs, err := interceptor.Log(universeID)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected log entries > 0, got %d", len(logs))
	}
}

func TestRemoteBridge_PushAndPull(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fg-remote-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	blobStore, err := storage.NewBlobStore(filepath.Join(tmpDir, "objects"))
	if err != nil {
		t.Fatalf("NewBlobStore failed: %v", err)
	}
	graphEngine, err := storage.NewGraphEngine(filepath.Join(tmpDir, "graph.db"))
	if err != nil {
		t.Fatalf("NewGraphEngine failed: %v", err)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	bridge := NewRemoteBridge(universeMgr, blobStore)

	err = bridge.AddRemote(&GitRemoteConfig{
		Name:     "origin",
		URL:      "https://github.com/example/repo.git",
		Branch:   "main",
		AuthType: "none",
	})
	if err != nil {
		t.Fatalf("AddRemote failed: %v", err)
	}

	universeID := "universe-prod"
	_, _ = universeMgr.CreateUniverse(universeID, "")
	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-prod",
		UniverseID:  universeID,
		Components:  []string{},
	}
	_, _ = universeMgr.CommitManifest(universeID, manifest)

	pushRes, err := bridge.PushUniverse("origin", universeID)
	if err != nil {
		t.Fatalf("PushUniverse failed: %v", err)
	}
	if !pushRes.Success || pushRes.RemoteName != "origin" {
		t.Fatalf("unexpected push result: %+v", pushRes)
	}

	pullRes, err := bridge.ImportFilesToUniverse("universe-imported", map[string][]byte{"readme.md": []byte("# Test")}, "agent-sync", "sync remote")
	if err != nil {
		t.Fatalf("ImportFilesToUniverse failed: %v", err)
	}
	if pullRes.UniverseID != "universe-imported" {
		t.Fatalf("unexpected pull result: %+v", pullRes)
	}
}
