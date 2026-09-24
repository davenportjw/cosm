package gitshim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestInitGitBridge(t *testing.T) {
	tempDir := t.TempDir()

	err := InitGitBridge(tempDir, "universe-main")
	if err != nil {
		t.Fatalf("InitGitBridge failed: %v", err)
	}

	// Verify standard .git directory structure
	gitDir := filepath.Join(tempDir, ".git")
	headPath := filepath.Join(gitDir, "HEAD")
	if _, err := os.Stat(headPath); err != nil {
		t.Fatalf("expected .git/HEAD to exist: %v", err)
	}

	headContent, err := os.ReadFile(headPath)
	if err != nil || string(headContent) != "ref: refs/heads/main\n" {
		t.Fatalf("unexpected .git/HEAD content: %s", string(headContent))
	}

	configPath := filepath.Join(gitDir, "config")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected .git/config to exist: %v", err)
	}

	excludePath := filepath.Join(gitDir, "info", "exclude")
	if _, err := os.Stat(excludePath); err != nil {
		t.Fatalf("expected .git/info/exclude to exist: %v", err)
	}
}

func TestGitShimInterceptor_Revert(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cosm-gitshim-revert-test-*")
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

	goParser := golang.NewGoParser()

	// 1. Commit initial Go file (v1)
	goCodeV1 := "package main\n\nfunc Version() string { return \"1.0.0\" }\n"
	pkgRes1, err := goParser.ParseSource("main.go", []byte(goCodeV1), core.LineageEnvelope{
		ExecutingAgentID: "agent-01",
		Intent:           "Initial Go service v1",
		Timestamp:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ParseSource v1 failed: %v", err)
	}

	comp1, err := golang.BuildComponentNode("main.go", core.CompService, pkgRes1, pkgRes1.AllSymbols[0].Lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode v1 failed: %v", err)
	}

	var symHashes1 []string
	for _, sym := range pkgRes1.AllSymbols {
		sData, err := json.Marshal(sym)
		if err != nil {
			t.Fatalf("Marshal symbol failed: %v", err)
		}
		sHash, err := blobStore.Put(sData)
		if err != nil {
			t.Fatalf("blobStore.Put symbol failed: %v", err)
		}
		symHashes1 = append(symHashes1, sHash)
	}
	comp1.SymbolNodes = symHashes1

	cData1, err := json.Marshal(comp1)
	if err != nil {
		t.Fatalf("Marshal comp1 failed: %v", err)
	}
	cHash1, err := blobStore.Put(cData1)
	if err != nil {
		t.Fatalf("blobStore.Put comp1 failed: %v", err)
	}

	manifest1 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-1",
		UniverseID:  universeID,
		Components:  []string{cHash1},
	}
	commit1, err := interceptor.Commit(universeID, manifest1, GitCommitOptions{
		Message:          "feat: initial v1",
		ExecutingAgentID: "agent-01",
	})
	if err != nil {
		t.Fatalf("Commit v1 failed: %v", err)
	}
	if commit1.CommitHash == "" {
		t.Fatalf("expected valid commit hash for v1")
	}

	// 2. Commit updated Go file (v2)
	goCodeV2 := "package main\n\nfunc Version() string { return \"2.0.0\" }\n"
	pkgRes2, err := goParser.ParseSource("main.go", []byte(goCodeV2), core.LineageEnvelope{
		ExecutingAgentID: "agent-02",
		Intent:           "Update Go service to v2",
		Timestamp:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ParseSource v2 failed: %v", err)
	}

	comp2, err := golang.BuildComponentNode("main.go", core.CompService, pkgRes2, pkgRes2.AllSymbols[0].Lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode v2 failed: %v", err)
	}

	var symHashes2 []string
	for _, sym := range pkgRes2.AllSymbols {
		sData, err := json.Marshal(sym)
		if err != nil {
			t.Fatalf("Marshal symbol v2 failed: %v", err)
		}
		sHash, err := blobStore.Put(sData)
		if err != nil {
			t.Fatalf("blobStore.Put symbol v2 failed: %v", err)
		}
		symHashes2 = append(symHashes2, sHash)
	}
	comp2.SymbolNodes = symHashes2

	cData2, err := json.Marshal(comp2)
	if err != nil {
		t.Fatalf("Marshal comp2 failed: %v", err)
	}
	cHash2, err := blobStore.Put(cData2)
	if err != nil {
		t.Fatalf("blobStore.Put comp2 failed: %v", err)
	}

	manifest2 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-1",
		UniverseID:  universeID,
		Components:  []string{cHash2},
	}
	commit2, err := interceptor.Commit(universeID, manifest2, GitCommitOptions{
		Message:          "feat: update to v2",
		ExecutingAgentID: "agent-02",
	})
	if err != nil {
		t.Fatalf("Commit v2 failed: %v", err)
	}
	if commit2.CommitHash == "" {
		t.Fatalf("expected valid commit hash for v2")
	}

	// 3. Inspect logs and get synthetic commit hash for v2
	logs, err := interceptor.Log(universeID)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if len(logs) < 2 {
		t.Fatalf("expected at least 2 log entries, got %d", len(logs))
	}
	v2CommitHash := logs[0].CommitHash

	// 4. Revert commit v2 using synthetic commit hash
	revertCommit, err := interceptor.Revert(universeID, v2CommitHash, GitCommitOptions{
		Message:          "revert: rollback v2",
		ExecutingAgentID: "agent-reverter",
	})
	if err != nil {
		t.Fatalf("Revert failed: %v", err)
	}
	if revertCommit == nil || revertCommit.CommitHash == "" {
		t.Fatalf("expected valid revert synthetic commit")
	}

	// 5. Verify universe head manifest matches the reverted state (v1)
	headManifest, err := universeMgr.GetUniverseManifest(universeID)
	if err != nil {
		t.Fatalf("GetUniverseManifest failed: %v", err)
	}
	if len(headManifest.Components) != 1 {
		t.Fatalf("expected 1 component in reverted manifest, got %d", len(headManifest.Components))
	}
	if headManifest.Components[0] != cHash1 {
		t.Fatalf("expected reverted head manifest component %s, got %s", cHash1, headManifest.Components[0])
	}

	// Hydrate workspace to verify file content is back to v1
	compMap, symMap := interceptor.loadManifestNodes(headManifest)
	hydrated, err := interceptor.hydrator.HydrateWorkspace(headManifest, compMap, symMap)
	if err != nil {
		t.Fatalf("HydrateWorkspace failed: %v", err)
	}
	if strings.TrimSpace(string(hydrated["main.go"])) != strings.TrimSpace(goCodeV1) {
		t.Fatalf("expected main.go content to match v1, got: %s", string(hydrated["main.go"]))
	}
	if !strings.Contains(string(hydrated["main.go"]), "1.0.0") || strings.Contains(string(hydrated["main.go"]), "2.0.0") {
		t.Fatalf("expected hydrated content to have v1 and not v2, got: %s", string(hydrated["main.go"]))
	}
}

func TestGitShimInterceptor_Reset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cosm-gitshim-reset-test-*")
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

	goParser := golang.NewGoParser()

	// 1. Commit initial Go file (v1)
	goCodeV1 := "package main\n\nfunc Version() string { return \"1.0.0\" }\n"
	pkgRes1, err := goParser.ParseSource("main.go", []byte(goCodeV1), core.LineageEnvelope{
		ExecutingAgentID: "agent-01",
		Intent:           "Initial Go service v1",
		Timestamp:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ParseSource v1 failed: %v", err)
	}

	comp1, err := golang.BuildComponentNode("main.go", core.CompService, pkgRes1, pkgRes1.AllSymbols[0].Lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode v1 failed: %v", err)
	}

	var symHashes1 []string
	for _, sym := range pkgRes1.AllSymbols {
		sData, _ := json.Marshal(sym)
		sHash, _ := blobStore.Put(sData)
		symHashes1 = append(symHashes1, sHash)
	}
	comp1.SymbolNodes = symHashes1

	cData1, _ := json.Marshal(comp1)
	cHash1, _ := blobStore.Put(cData1)

	manifest1 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-1",
		UniverseID:  universeID,
		Components:  []string{cHash1},
	}
	_, err = interceptor.Commit(universeID, manifest1, GitCommitOptions{
		Message:          "feat: initial v1",
		ExecutingAgentID: "agent-01",
	})
	if err != nil {
		t.Fatalf("Commit v1 failed: %v", err)
	}

	// 2. Commit updated Go file (v2)
	goCodeV2 := "package main\n\nfunc Version() string { return \"2.0.0\" }\n"
	pkgRes2, err := goParser.ParseSource("main.go", []byte(goCodeV2), core.LineageEnvelope{
		ExecutingAgentID: "agent-02",
		Intent:           "Update Go service to v2",
		Timestamp:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ParseSource v2 failed: %v", err)
	}

	comp2, err := golang.BuildComponentNode("main.go", core.CompService, pkgRes2, pkgRes2.AllSymbols[0].Lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode v2 failed: %v", err)
	}

	var symHashes2 []string
	for _, sym := range pkgRes2.AllSymbols {
		sData, _ := json.Marshal(sym)
		sHash, _ := blobStore.Put(sData)
		symHashes2 = append(symHashes2, sHash)
	}
	comp2.SymbolNodes = symHashes2

	cData2, _ := json.Marshal(comp2)
	cHash2, _ := blobStore.Put(cData2)

	manifest2 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-1",
		UniverseID:  universeID,
		Components:  []string{cHash2},
	}
	_, err = interceptor.Commit(universeID, manifest2, GitCommitOptions{
		Message:          "feat: update to v2",
		ExecutingAgentID: "agent-02",
	})
	if err != nil {
		t.Fatalf("Commit v2 failed: %v", err)
	}

	// Setup working tree with v2 and untracked file
	workingTree := map[string][]byte{
		"main.go":       []byte(goCodeV2),
		"untracked.txt": []byte("should be deleted on hard reset"),
	}

	// 3. Reset back to initial commit in standard mode (succeeds)
	// Retrieve v1 commit hash from log
	logs, err := interceptor.Log(universeID)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if len(logs) < 2 {
		t.Fatalf("expected at least 2 log entries, got %d", len(logs))
	}
	v1CommitHash := logs[1].CommitHash

	// Reset using prefix of synthetic commit hash
	if err := interceptor.Reset(universeID, v1CommitHash[:10], true, workingTree); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Verify head manifest is reset back to v1
	headManifest, err := universeMgr.GetUniverseManifest(universeID)
	if err != nil {
		t.Fatalf("GetUniverseManifest failed: %v", err)
	}
	if headManifest.Components[0] != cHash1 {
		t.Fatalf("expected reset head manifest to have component %s, got %s", cHash1, headManifest.Components[0])
	}

	// Verify workingTree was synchronized
	if _, exists := workingTree["untracked.txt"]; exists {
		t.Fatalf("expected untracked.txt to be removed on hard reset")
	}
	if strings.TrimSpace(string(workingTree["main.go"])) != strings.TrimSpace(goCodeV1) {
		t.Fatalf("expected main.go to be reset to v1 content, got: %s", string(workingTree["main.go"]))
	}
	if !strings.Contains(string(workingTree["main.go"]), "1.0.0") || strings.Contains(string(workingTree["main.go"]), "2.0.0") {
		t.Fatalf("expected reset content to have v1 and not v2, got: %s", string(workingTree["main.go"]))
	}

	// 4. Test ledger mode rejection
	universeMgr.SetLedgerMode(true)
	err = interceptor.Reset(universeID, v1CommitHash, false, nil)
	if err != storage.ErrLedgerLinearityViolation {
		t.Fatalf("expected ErrLedgerLinearityViolation in ledger mode, got: %v", err)
	}

	// 5. Test nonexistent commit
	universeMgr.SetLedgerMode(false)
	err = interceptor.Reset(universeID, "nonexistent-hash-404", false, nil)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

