package onboarding

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func TestDirectoryScanner(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scanner_test_*")
	if err != nil {
		t.Fatalf("TempDir error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create subdirectories
	backendDir := filepath.Join(tempDir, "services", "backend")
	frontendDir := filepath.Join(tempDir, "web")
	infraDir := filepath.Join(tempDir, "infra")
	ignoredDir := filepath.Join(tempDir, "node_modules", "pkg")

	_ = os.MkdirAll(backendDir, 0755)
	_ = os.MkdirAll(frontendDir, 0755)
	_ = os.MkdirAll(infraDir, 0755)
	_ = os.MkdirAll(ignoredDir, 0755)

	_ = os.WriteFile(filepath.Join(backendDir, "go.mod"), []byte("module my/backend\ngo 1.22"), 0644)
	_ = os.WriteFile(filepath.Join(backendDir, "main.go"), []byte("package main\nfunc main() {}"), 0644)
	_ = os.WriteFile(filepath.Join(frontendDir, "package.json"), []byte(`{"name": "web"}`), 0644)
	_ = os.WriteFile(filepath.Join(frontendDir, "App.tsx"), []byte("export const App = () => <div>Hello</div>"), 0644)
	_ = os.WriteFile(filepath.Join(infraDir, "main.tf"), []byte(`resource "google_cloud_run_service" "app" {}`), 0644)
	_ = os.WriteFile(filepath.Join(ignoredDir, "junk.js"), []byte("junk"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# My Repo"), 0644)

	scanner := NewDirectoryScanner()
	codeFiles, rawFiles, err := scanner.ScanDirectory(tempDir)
	if err != nil {
		t.Fatalf("ScanDirectory error: %v", err)
	}

	if len(codeFiles) != 3 { // main.go, App.tsx, main.tf
		t.Errorf("Expected 3 code files, got %d (%v)", len(codeFiles), codeFiles)
	}
	if len(rawFiles) < 3 { // go.mod, package.json, README.md
		t.Errorf("Expected at least 3 raw files, got %d (%v)", len(rawFiles), rawFiles)
	}

	// Test Manifest detection
	bInfo := scanner.DetectManifests(backendDir)
	if !bInfo.HasGoMod || bInfo.ComponentType != core.CompService {
		t.Errorf("Expected backend to be detected as CompService (Go)")
	}

	fInfo := scanner.DetectManifests(frontendDir)
	if !fInfo.HasPackageJSON || fInfo.ComponentType != core.CompFrontend {
		t.Errorf("Expected frontend to be detected as CompFrontend (TypeScript)")
	}

	iInfo := scanner.DetectManifests(infraDir)
	if !iInfo.HasTerraform || iInfo.ComponentType != core.CompInfra {
		t.Errorf("Expected infra to be detected as CompInfra (HCL)")
	}
}

func TestRepositoryMigrator_FullPolyglotSnapshot(t *testing.T) {
	repoDir, err := os.MkdirTemp("", "repo_src_*")
	if err != nil {
		t.Fatalf("TempDir error: %v", err)
	}
	defer os.RemoveAll(repoDir)

	fgDir, err := os.MkdirTemp("", "fg_store_*")
	if err != nil {
		t.Fatalf("TempDir error: %v", err)
	}
	defer os.RemoveAll(fgDir)

	// Set up source repository with Go, Python, TypeScript, Terraform, and Raw files
	goDir := filepath.Join(repoDir, "server")
	pyDir := filepath.Join(repoDir, "ml_service")
	tsDir := filepath.Join(repoDir, "frontend")
	tfDir := filepath.Join(repoDir, "deploy")
	_ = os.MkdirAll(goDir, 0755)
	_ = os.MkdirAll(pyDir, 0755)
	_ = os.MkdirAll(tsDir, 0755)
	_ = os.MkdirAll(tfDir, 0755)

	goCode := `package server
import "net/http"
func HandleGetUsers(w http.ResponseWriter, r *http.Request) {
	http.HandleFunc("/api/v1/users", HandleGetUsers)
}`
	_ = os.WriteFile(filepath.Join(goDir, "main.go"), []byte(goCode), 0644)
	_ = os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module server\ngo 1.22"), 0644)

	pyCode := `from fastapi import FastAPI
app = FastAPI()
@app.get("/api/v1/predict")
def predict():
    return {"prediction": 42}`
	_ = os.WriteFile(filepath.Join(pyDir, "app.py"), []byte(pyCode), 0644)
	_ = os.WriteFile(filepath.Join(pyDir, "requirements.txt"), []byte("fastapi\nuvicorn"), 0644)

	tsCode := `export const fetchUsers = async () => {
    return await fetch('/api/v1/users');
};`
	_ = os.WriteFile(filepath.Join(tsDir, "api.ts"), []byte(tsCode), 0644)
	_ = os.WriteFile(filepath.Join(tsDir, "package.json"), []byte(`{"name":"client"}`), 0644)

	tfCode := `resource "google_cloud_run_service" "backend" {
  name = "backend-service"
  template {
    spec {
      containers {
        image = "gcr.io/test/server"
        env {
          name = "PORT"
          value = "8080"
        }
      }
    }
  }
}`
	_ = os.WriteFile(filepath.Join(tfDir, "main.tf"), []byte(tfCode), 0644)
	_ = os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Full Polyglot Architecture"), 0644)

	// Initialize Storage & Migrator
	objectsDir := filepath.Join(fgDir, "objects")
	dbPath := filepath.Join(fgDir, "graph.db")
	_ = os.MkdirAll(objectsDir, 0755)

	blobStore, err := storage.NewBlobStore(objectsDir)
	if err != nil {
		t.Fatalf("BlobStore init: %v", err)
	}
	graphEngine, err := storage.NewGraphEngine(dbPath)
	if err != nil {
		t.Fatalf("GraphEngine init: %v", err)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	_, err = universeMgr.CreateUniverse("universe-main", "")
	if err != nil {
		t.Fatalf("CreateUniverse error: %v", err)
	}

	migrator := NewRepositoryMigrator(blobStore, graphEngine, universeMgr)
	report, err := migrator.ImportRepositorySnapshot(
		repoDir,
		"universe-main",
		"Onboard full polyglot repo",
		"migrator-agent-1",
	)
	if err != nil {
		t.Fatalf("ImportRepositorySnapshot failed: %v", err)
	}

	if report.CodeFilesParsed != 4 {
		t.Errorf("Expected 4 code files parsed, got %d", report.CodeFilesParsed)
	}
	if report.RawFilesPreserved < 4 { // go.mod, requirements.txt, package.json, README.md
		t.Errorf("Expected at least 4 raw files preserved, got %d", report.RawFilesPreserved)
	}
	if report.SymbolsExtracted == 0 {
		t.Errorf("Expected symbols extracted > 0")
	}
	if report.ComponentsCreated == 0 {
		t.Errorf("Expected components created > 0")
	}
	if report.ManifestHash == "" {
		t.Errorf("Expected non-empty manifest hash")
	}

	// Verify universe head was updated
	headManifest, err := universeMgr.GetUniverseManifest("universe-main")
	if err != nil {
		t.Fatalf("GetUniverseManifest error: %v", err)
	}
	if headManifest == nil || len(headManifest.Components) == 0 {
		t.Errorf("Expected head manifest with components")
	}
}
