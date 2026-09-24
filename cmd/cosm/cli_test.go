package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func TestIsWorkspaceLedgerMode(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Default should be false
	if isWorkspaceLedgerMode(tmpDir, false) {
		t.Errorf("expected isWorkspaceLedgerMode to be false by default")
	}

	// 2. CLI flag true -> true
	if !isWorkspaceLedgerMode(tmpDir, true) {
		t.Errorf("expected isWorkspaceLedgerMode to be true when cliFlag is true")
	}

	// 3. Env var true -> true
	t.Setenv("COSM_LEDGER_MODE", "true")
	if !isWorkspaceLedgerMode(tmpDir, false) {
		t.Errorf("expected isWorkspaceLedgerMode to be true when COSM_LEDGER_MODE is true")
	}
	t.Setenv("COSM_LEDGER_MODE", "false")

	// 4. Config file true -> true
	cfg := &storage.WorkspaceConfig{
		LedgerMode:      true,
		DefaultUniverse: "universe-main",
		CreatedAt:       time.Now().UTC(),
	}
	if err := storage.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
	if !isWorkspaceLedgerMode(tmpDir, false) {
		t.Errorf("expected isWorkspaceLedgerMode to be true when config LedgerMode is true")
	}
}

func TestCleanEmptyParents(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "sub1", "sub2", "sub3")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Clean nested
	cleanEmptyParents(tmpDir, nested)

	// sub1 should be deleted, tmpDir must remain intact
	if _, err := os.Stat(filepath.Join(tmpDir, "sub1")); !os.IsNotExist(err) {
		t.Errorf("expected sub1 to be deleted")
	}
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Errorf("expected tmpDir to still exist")
	}

	// If directory contains a file, cleanEmptyParents should not delete it
	keepDir := filepath.Join(tmpDir, "keep", "child")
	if err := os.MkdirAll(keepDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	testFile := filepath.Join(tmpDir, "keep", "keep.txt")
	if err := os.WriteFile(testFile, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	cleanEmptyParents(tmpDir, keepDir)
	if _, err := os.Stat(filepath.Join(tmpDir, "keep")); os.IsNotExist(err) {
		t.Errorf("expected keep dir to still exist because it has a file")
	}
}

func TestSyncWorkspaceDisk(t *testing.T) {
	tmpDir := t.TempDir()
	cosmDir := filepath.Join(tmpDir, ".cosm")
	objectsDir := filepath.Join(cosmDir, "objects")
	if err := os.MkdirAll(objectsDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	blobStore, err := storage.NewBlobStore(objectsDir)
	if err != nil {
		t.Fatalf("NewBlobStore failed: %v", err)
	}

	// Create symbols and component 1
	codeA := "package test\nfunc Hello() {}\n"
	symA := &core.ASTSymbolNode{
		NodeID:     "sym-hello",
		Language:   core.LangRaw,
		Identifier: "pkg/a/hello.go",
		NodeType:   "RawBlobNode",
		ASTPayload: []byte(codeA),
	}
	symABytes, _ := json.Marshal(symA)
	symAHash, _ := blobStore.Put(symABytes)

	compA := &core.ComponentNode{
		ComponentID: "comp-a",
		Name:        "pkg/a/hello.go",
		Language:    core.LangRaw,
		Type:        core.CompService,
		SymbolNodes: []string{symAHash},
		Metadata: map[string]string{
			"file_path": "pkg/a/hello.go",
		},
	}
	compABytes, _ := json.Marshal(compA)
	compAHash, _ := blobStore.Put(compABytes)

	manifest1 := &core.WorkspaceManifestNode{
		WorkspaceID:    "ws-test",
		UniverseID:     "universe-main",
		Components:     []string{compAHash},
		MerkleRootHash: "root-1",
	}

	// 1. Sync manifest1 to disk
	restored, removed, err := syncWorkspaceDisk(tmpDir, manifest1, nil, blobStore)
	if err != nil {
		t.Fatalf("syncWorkspaceDisk manifest1 failed: %v", err)
	}
	if restored != 1 || removed != 0 {
		t.Fatalf("expected restored=1, removed=0; got restored=%d, removed=%d", restored, removed)
	}

	diskA, err := os.ReadFile(filepath.Join(tmpDir, "pkg/a/hello.go"))
	if err != nil {
		t.Fatalf("expected pkg/a/hello.go on disk: %v", err)
	}
	if string(diskA) != codeA {
		t.Fatalf("content mismatch on disk: got %q, want %q", string(diskA), codeA)
	}

	// 2. Create component 2 and new manifest replacing component 1
	codeB := "package test\nfunc World() {}\n"
	symB := &core.ASTSymbolNode{
		NodeID:     "sym-world",
		Language:   core.LangRaw,
		Identifier: "pkg/b/world.go",
		NodeType:   "RawBlobNode",
		ASTPayload: []byte(codeB),
	}
	symBBytes, _ := json.Marshal(symB)
	symBHash, _ := blobStore.Put(symBBytes)

	compB := &core.ComponentNode{
		ComponentID: "comp-b",
		Name:        "pkg/b/world.go",
		Language:    core.LangRaw,
		Type:        core.CompService,
		SymbolNodes: []string{symBHash},
		Metadata: map[string]string{
			"file_path": "pkg/b/world.go",
		},
	}
	compBBytes, _ := json.Marshal(compB)
	compBHash, _ := blobStore.Put(compBBytes)

	manifest2 := &core.WorkspaceManifestNode{
		WorkspaceID:    "ws-test",
		UniverseID:     "universe-main",
		Components:     []string{compBHash},
		MerkleRootHash: "root-2",
	}

	// Sync manifest2 with prevManifest=manifest1
	restored, removed, err = syncWorkspaceDisk(tmpDir, manifest2, manifest1, blobStore)
	if err != nil {
		t.Fatalf("syncWorkspaceDisk manifest2 failed: %v", err)
	}
	if restored != 1 || removed != 1 {
		t.Fatalf("expected restored=1, removed=1; got restored=%d, removed=%d", restored, removed)
	}

	// pkg/a/hello.go should be deleted, pkg/b/world.go should exist
	if _, err := os.Stat(filepath.Join(tmpDir, "pkg/a/hello.go")); !os.IsNotExist(err) {
		t.Errorf("expected pkg/a/hello.go to be removed from disk")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "pkg/b/world.go")); err != nil {
		t.Errorf("expected pkg/b/world.go to exist on disk: %v", err)
	}
}

func TestCLIInitLedgerAndUniverseLifecycle(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	// 1. Test runInit with --ledger
	runInit([]string{"--ledger", "-u", "test-universe"})

	cfg, err := storage.LoadConfig(".cosm")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg.LedgerMode {
		t.Fatalf("expected LedgerMode to be true in config")
	}
	if cfg.DefaultUniverse != "test-universe" {
		t.Fatalf("expected DefaultUniverse to be 'test-universe', got %q", cfg.DefaultUniverse)
	}
	if !isWorkspaceLedgerMode(".cosm", false) {
		t.Fatalf("expected isWorkspaceLedgerMode to be true")
	}

	// Verify universe head exists
	blobStore, graphEngine, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage failed: %v", err)
	}
	defer graphEngine.Close()

	mgr := storage.NewUniverseManager(graphEngine, blobStore)
	manifest, err := mgr.GetUniverseManifest("test-universe")
	if err != nil {
		t.Fatalf("GetUniverseManifest failed: %v", err)
	}
	if manifest == nil || manifest.MerkleRootHash == "" {
		t.Fatalf("expected non-empty initial manifest")
	}
}

func TestCLIUndoStandardAndLedger(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	// 1. Initialize repo in standard mode
	runInit([]string{"-u", "universe-main"})

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage failed: %v", err)
	}
	mgr := storage.NewUniverseManager(graphEngine, blobStore)
	initialHead, _ := mgr.GetUniverse("universe-main")

	// Create and commit a component
	code := "package main\nfunc Test() {}\n"
	sym := &core.ASTSymbolNode{
		NodeID:     "sym-1",
		Language:   core.LangRaw,
		Identifier: "pkg/test.go",
		NodeType:   "RawBlobNode",
		ASTPayload: []byte(code),
	}
	symBytes, _ := json.Marshal(sym)
	symHash, _ := blobStore.Put(symBytes)

	comp := &core.ComponentNode{
		ComponentID: "comp-1",
		Name:        "pkg/test.go",
		Language:    core.LangRaw,
		Type:        core.CompService,
		SymbolNodes: []string{symHash},
		Metadata: map[string]string{
			"file_path": "pkg/test.go",
		},
	}
	compBytes, _ := json.Marshal(comp)
	compHash, _ := blobStore.Put(compBytes)

	manifest1 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-universe-main",
		UniverseID:  "universe-main",
		Components:  []string{compHash},
		CreatedAt:   time.Now().UTC(),
		Lineage: core.LineageEnvelope{
			Intent: "Commit 1",
		},
	}
	h1, _ := core.HashWorkspaceManifest(manifest1)
	manifest1.MerkleRootHash = h1
	_, err = mgr.CommitManifest("universe-main", manifest1)
	if err != nil {
		t.Fatalf("CommitManifest failed: %v", err)
	}
	graphEngine.Close()

	// Run cosm undo (standard mode)
	runUndo([]string{"-u", "universe-main", "-w"})

	// Check head restored
	blobStore2, graphEngine2, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage 2 failed: %v", err)
	}
	mgr2 := storage.NewUniverseManager(graphEngine2, blobStore2)
	headAfterUndo, _ := mgr2.GetUniverse("universe-main")
	if headAfterUndo.HeadManifestHash != initialHead.HeadManifestHash {
		t.Errorf("expected head to be restored to %s, got %s", initialHead.HeadManifestHash, headAfterUndo.HeadManifestHash)
	}
	graphEngine2.Close()
}

func TestCLIRevert(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	runInit([]string{"-u", "universe-main"})

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage failed: %v", err)
	}
	mgr := storage.NewUniverseManager(graphEngine, blobStore)

	// Commit 1
	code := "package main\nfunc Test() {}\n"
	sym := &core.ASTSymbolNode{
		NodeID:     "sym-1",
		Language:   core.LangRaw,
		Identifier: "pkg/test.go",
		NodeType:   "RawBlobNode",
		ASTPayload: []byte(code),
	}
	symBytes, _ := json.Marshal(sym)
	symHash, _ := blobStore.Put(symBytes)

	comp := &core.ComponentNode{
		ComponentID: "comp-1",
		Name:        "pkg/test.go",
		Language:    core.LangRaw,
		Type:        core.CompService,
		SymbolNodes: []string{symHash},
		Metadata: map[string]string{
			"file_path": "pkg/test.go",
		},
	}
	compBytes, _ := json.Marshal(comp)
	compHash, _ := blobStore.Put(compBytes)

	manifest1 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-universe-main",
		UniverseID:  "universe-main",
		Components:  []string{compHash},
		CreatedAt:   time.Now().UTC(),
		Lineage: core.LineageEnvelope{
			Intent: "Add test.go",
		},
	}
	h1, _ := core.HashWorkspaceManifest(manifest1)
	manifest1.MerkleRootHash = h1
	_, err = mgr.CommitManifest("universe-main", manifest1)
	if err != nil {
		t.Fatalf("CommitManifest failed: %v", err)
	}
	graphEngine.Close()

	// Revert commit 1
	runRevert([]string{"-u", "universe-main", "-i", "Revert test.go", h1, "-w"})

	// Check that a new commit was appended
	blobStore2, graphEngine2, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage 2 failed: %v", err)
	}
	mgr2 := storage.NewUniverseManager(graphEngine2, blobStore2)
	headAfterRevert, _ := mgr2.GetUniverse("universe-main")
	if headAfterRevert.HeadManifestHash == h1 {
		t.Errorf("expected head to change after revert, got %s", headAfterRevert.HeadManifestHash)
	}
	revManifest, err := mgr2.GetUniverseManifest("universe-main")
	if err != nil {
		t.Fatalf("GetUniverseManifest failed: %v", err)
	}
	if len(revManifest.Components) != 0 {
		t.Errorf("expected 0 components after reverting only component, got %d", len(revManifest.Components))
	}
	graphEngine2.Close()
}

func TestCLIResetStandard(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	runInit([]string{"-u", "universe-main"})

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage failed: %v", err)
	}
	mgr := storage.NewUniverseManager(graphEngine, blobStore)
	initialHead, _ := mgr.GetUniverse("universe-main")

	// Commit 1
	comp := &core.ComponentNode{
		ComponentID: "comp-reset",
		Name:        "pkg/reset.go",
		Language:    core.LangRaw,
		Type:        core.CompService,
	}
	compBytes, _ := json.Marshal(comp)
	compHash, _ := blobStore.Put(compBytes)

	manifest1 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-universe-main",
		UniverseID:  "universe-main",
		Components:  []string{compHash},
		CreatedAt:   time.Now().UTC(),
	}
	h1, _ := core.HashWorkspaceManifest(manifest1)
	manifest1.MerkleRootHash = h1
	_, _ = mgr.CommitManifest("universe-main", manifest1)
	graphEngine.Close()

	// Reset head to initialHead.HeadManifestHash
	runReset([]string{"--hard", "-u", "universe-main", initialHead.HeadManifestHash})

	blobStore2, graphEngine2, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage 2 failed: %v", err)
	}
	mgr2 := storage.NewUniverseManager(graphEngine2, blobStore2)
	headAfterReset, _ := mgr2.GetUniverse("universe-main")
	if headAfterReset.HeadManifestHash != initialHead.HeadManifestHash {
		t.Errorf("expected head to be reset to %s, got %s", initialHead.HeadManifestHash, headAfterReset.HeadManifestHash)
	}
	graphEngine2.Close()
}

func TestCLIGitRevertAndReset(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	runInit([]string{"-u", "universe-main"})

	blobStore, graphEngine, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage failed: %v", err)
	}
	mgr := storage.NewUniverseManager(graphEngine, blobStore)
	initialHead, _ := mgr.GetUniverse("universe-main")

	// Commit 1
	code := "package main\nfunc GitTest() {}\n"
	sym := &core.ASTSymbolNode{
		NodeID:     "sym-git-1",
		Language:   core.LangRaw,
		Identifier: "pkg/git.go",
		NodeType:   "RawBlobNode",
		ASTPayload: []byte(code),
	}
	symBytes, _ := json.Marshal(sym)
	symHash, _ := blobStore.Put(symBytes)

	comp := &core.ComponentNode{
		ComponentID: "comp-git-1",
		Name:        "pkg/git.go",
		Language:    core.LangRaw,
		Type:        core.CompService,
		SymbolNodes: []string{symHash},
		Metadata: map[string]string{
			"file_path": "pkg/git.go",
		},
	}
	compBytes, _ := json.Marshal(comp)
	compHash, _ := blobStore.Put(compBytes)

	manifest1 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-universe-main",
		UniverseID:  "universe-main",
		Components:  []string{compHash},
		CreatedAt:   time.Now().UTC(),
		Lineage: core.LineageEnvelope{
			Intent: "Add git.go",
		},
	}
	h1, _ := core.HashWorkspaceManifest(manifest1)
	manifest1.MerkleRootHash = h1
	_, err = mgr.CommitManifest("universe-main", manifest1)
	if err != nil {
		t.Fatalf("CommitManifest failed: %v", err)
	}
	graphEngine.Close()

	// 1. Test cosm git revert
	runGit([]string{"revert", "-m", "Revert git.go", h1})

	blobStore2, graphEngine2, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage 2 failed: %v", err)
	}
	mgr2 := storage.NewUniverseManager(graphEngine2, blobStore2)
	headAfterRevert, _ := mgr2.GetUniverse("universe-main")
	if headAfterRevert.HeadManifestHash == h1 {
		t.Errorf("expected git revert to update head manifest hash")
	}
	graphEngine2.Close()

	// 2. Test cosm git reset --hard back to initialHead
	runGit([]string{"reset", "--hard", initialHead.HeadManifestHash})

	blobStore3, graphEngine3, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage 3 failed: %v", err)
	}
	mgr3 := storage.NewUniverseManager(graphEngine3, blobStore3)
	headAfterReset, _ := mgr3.GetUniverse("universe-main")
	if headAfterReset.HeadManifestHash != initialHead.HeadManifestHash {
		t.Errorf("expected git reset to restore head to %s, got %s", initialHead.HeadManifestHash, headAfterReset.HeadManifestHash)
	}
	graphEngine3.Close()
}

func TestCLIPolyglotMultiFileUndo(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	// 1. Initialize repository
	runInit([]string{"-u", "universe-main"})

	// 2. Writes polyglot files: Go (main.go), Python (app.py), Terraform (main.tf)
	mainGoV1 := `package main

import (
	"fmt"
)

func Version() string {
	return "1.0.0"
}

func main() {
	fmt.Println(Version())
}
`
	appPyV1 := `def get_status():
    return {"status": "ok", "service": "api"}
`
	mainTfV1 := `# Terraform Infrastructure Configuration
# Component: main.tf

resource "local_file" "config" {
  filename = "output.txt"
  content  = "database_url=postgres://localhost/db"
}
`
	if err := os.WriteFile("main.go", []byte(mainGoV1), 0644); err != nil {
		t.Fatalf("WriteFile main.go failed: %v", err)
	}
	if err := os.WriteFile("app.py", []byte(appPyV1), 0644); err != nil {
		t.Fatalf("WriteFile app.py failed: %v", err)
	}
	if err := os.WriteFile("main.tf", []byte(mainTfV1), 0644); err != nil {
		t.Fatalf("WriteFile main.tf failed: %v", err)
	}

	// 3. Commits v1
	runAdd([]string{"."})
	runCommit([]string{"-u", "universe-main", "-i", "v1 polyglot base stack"})

	// Verify initial commit state
	blobStore, graphEngine, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage failed: %v", err)
	}
	mgr := storage.NewUniverseManager(graphEngine, blobStore)
	headV1, err := mgr.GetUniverse("universe-main")
	if err != nil {
		t.Fatalf("GetUniverse failed: %v", err)
	}
	v1Hash := headV1.HeadManifestHash
	graphEngine.Close()

	// 4. Adds new file worker.py, modifies main.go, commits v2
	workerPyV2 := `def run_worker_task():
    return "task processed"
`
	mainGoV2 := `package main

import "fmt"

func Version() string {
	return "2.0.0"
}

func main() {
	fmt.Println(Version())
}
`
	if err := os.WriteFile("worker.py", []byte(workerPyV2), 0644); err != nil {
		t.Fatalf("WriteFile worker.py failed: %v", err)
	}
	if err := os.WriteFile("main.go", []byte(mainGoV2), 0644); err != nil {
		t.Fatalf("WriteFile main.go modified failed: %v", err)
	}

	runAdd([]string{"."})
	runCommit([]string{"-u", "universe-main", "-i", "v2 add worker.py and update main.go"})

	if _, err := os.Stat("worker.py"); os.IsNotExist(err) {
		t.Fatalf("worker.py should exist before undo")
	}

	// 5. Runs cosm undo -w
	runUndo([]string{"-u", "universe-main", "-w"})

	// 6. Asserts that worker.py is removed from disk
	if _, err := os.Stat("worker.py"); !os.IsNotExist(err) {
		t.Fatalf("expected worker.py to be removed from disk after undo, got err: %v", err)
	}

	// 7. Asserts that main.go content matches v1
	mainGoAfter, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("ReadFile main.go failed: %v", err)
	}
	if strings.TrimSpace(string(mainGoAfter)) != strings.TrimSpace(mainGoV1) {
		t.Fatalf("expected main.go content to match v1, got:\n%s", string(mainGoAfter))
	}

	// 8. Asserts that main.tf remains intact
	mainTfAfter, err := os.ReadFile("main.tf")
	if err != nil {
		t.Fatalf("ReadFile main.tf failed: %v", err)
	}
	if strings.TrimSpace(string(mainTfAfter)) != strings.TrimSpace(mainTfV1) {
		t.Fatalf("expected main.tf content to match v1, got:\n%s", string(mainTfAfter))
	}

	// Asserts that universe head is restored to v1Hash
	blobStoreAfter, graphEngineAfter, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage after undo failed: %v", err)
	}
	defer graphEngineAfter.Close()
	mgrAfter := storage.NewUniverseManager(graphEngineAfter, blobStoreAfter)
	headAfter, err := mgrAfter.GetUniverse("universe-main")
	if err != nil {
		t.Fatalf("GetUniverse after undo failed: %v", err)
	}
	if headAfter.HeadManifestHash != v1Hash {
		t.Fatalf("expected universe head after undo to be %s, got %s", v1Hash, headAfter.HeadManifestHash)
	}
}

