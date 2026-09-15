package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/storage"
)

func setupTestWatcher(t *testing.T) (string, *Watcher, *storage.UniverseManager) {
	tempDir := t.TempDir()
	cosmDir := filepath.Join(tempDir, ".cosm")
	if err := os.MkdirAll(filepath.Join(cosmDir, "objects"), 0755); err != nil {
		t.Fatalf("failed to create cosm objects dir: %v", err)
	}

	blobStore, err := storage.NewBlobStore(filepath.Join(cosmDir, "objects"))
	if err != nil {
		t.Fatalf("failed to create blob store: %v", err)
	}

	graphEngine, err := storage.NewGraphEngine(filepath.Join(cosmDir, "graph.db"))
	if err != nil {
		t.Fatalf("failed to create graph engine: %v", err)
	}

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	_, err = universeMgr.CreateUniverse("universe-main", "")
	if err != nil {
		t.Fatalf("failed to create universe-main: %v", err)
	}

	w := NewWatcherWithStorage(tempDir, "universe-main", blobStore, graphEngine, 50*time.Millisecond)
	return tempDir, w, universeMgr
}

func TestWatcher_DetectAddAndModify(t *testing.T) {
	tempDir, w, universeMgr := setupTestWatcher(t)

	// 1. Initial scan with empty workspace
	rep1, err := w.ScanOnce()
	if err != nil {
		t.Fatalf("ScanOnce failed: %v", err)
	}
	if len(rep1.AddedFiles) != 0 {
		t.Fatalf("expected 0 added files initially, got: %d", len(rep1.AddedFiles))
	}

	// 2. Add a new Go file
	svcDir := filepath.Join(tempDir, "service")
	_ = os.MkdirAll(svcDir, 0755)
	mainGo := filepath.Join(svcDir, "main.go")
	code1 := `package main

func Multiply(a int, b int) int {
	return a * b
}
`
	if err := os.WriteFile(mainGo, []byte(code1), 0644); err != nil {
		t.Fatalf("writing main.go failed: %v", err)
	}

	rep2, err := w.ScanOnce()
	if err != nil {
		t.Fatalf("ScanOnce failed: %v", err)
	}
	if len(rep2.AddedFiles) != 1 || rep2.AddedFiles[0] != filepath.Join("service", "main.go") {
		t.Fatalf("expected added file service/main.go, got: %v", rep2.AddedFiles)
	}

	// Verify universe manifest was committed
	head1, err := universeMgr.GetUniverseManifest("universe-main")
	if err != nil || head1 == nil {
		t.Fatalf("failed to retrieve universe-main manifest: %v", err)
	}
	if len(head1.Components) != 1 {
		t.Fatalf("expected 1 component in manifest, got: %d", len(head1.Components))
	}

	// 3. Modify the Go file
	time.Sleep(10 * time.Millisecond) // Ensure timestamp advances
	code2 := `package main

func Multiply(a int, b int) int {
	return a * b * 2
}
`
	if err := os.WriteFile(mainGo, []byte(code2), 0644); err != nil {
		t.Fatalf("updating main.go failed: %v", err)
	}

	rep3, err := w.ScanOnce()
	if err != nil {
		t.Fatalf("ScanOnce modify failed: %v", err)
	}
	if len(rep3.ModifiedFiles) != 1 || rep3.ModifiedFiles[0] != filepath.Join("service", "main.go") {
		t.Fatalf("expected 1 modified file, got: %v", rep3.ModifiedFiles)
	}

	head2, err := universeMgr.GetUniverseManifest("universe-main")
	if err != nil || head2 == nil {
		t.Fatalf("failed to retrieve universe-main manifest after edit: %v", err)
	}
	if len(head2.Components) != 1 {
		t.Fatalf("expected 1 component in manifest after modify, got: %d", len(head2.Components))
	}

	// 4. Delete the Go file
	_ = os.Remove(mainGo)
	rep4, err := w.ScanOnce()
	if err != nil {
		t.Fatalf("ScanOnce delete failed: %v", err)
	}
	if len(rep4.DeletedFiles) != 1 || rep4.DeletedFiles[0] != filepath.Join("service", "main.go") {
		t.Fatalf("expected 1 deleted file, got: %v", rep4.DeletedFiles)
	}

	head3, err := universeMgr.GetUniverseManifest("universe-main")
	if err != nil || head3 == nil {
		t.Fatalf("failed to retrieve universe-main manifest after delete: %v", err)
	}
	if len(head3.Components) != 0 {
		t.Fatalf("expected 0 components in manifest after delete, got: %d", len(head3.Components))
	}
}

func TestWatcher_StartContextCancellation(t *testing.T) {
	_, w, _ := setupTestWatcher(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := w.Start(ctx)
	if err != context.DeadlineExceeded && err != nil && err != context.Canceled {
		t.Fatalf("expected context cancellation, got: %v", err)
	}
}
