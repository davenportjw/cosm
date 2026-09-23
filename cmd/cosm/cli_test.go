package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func TestCLI_Workflow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cosm_cli_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	// 1. Test init
	runInit([]string{"-u", "universe-test"})
	if _, err := os.Stat(".cosm/objects"); os.IsNotExist(err) {
		t.Errorf("Expected .cosm/objects directory to exist")
	}

	// 2. Write sample Go & Python files
	goCode := `package main
import "net/http"
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}`
	goFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(goFile, []byte(goCode), 0644); err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	pyCode := `from fastapi import FastAPI
app = FastAPI()
@app.get("/health")
def health():
    return {"status": "ok"}`
	pyFile := filepath.Join(tempDir, "app.py")
	if err := os.WriteFile(pyFile, []byte(pyCode), 0644); err != nil {
		t.Fatalf("Failed to write app.py: %v", err)
	}

	licenseFile := filepath.Join(tempDir, "LICENSE")
	if err := os.WriteFile(licenseFile, []byte("Apache-2.0 License"), 0644); err != nil {
		t.Fatalf("Failed to write LICENSE: %v", err)
	}

	// 3. Test add
	runAdd([]string{"-u", "universe-test", "-a", "test-agent", "-p", "add health endpoints and license", "-i", "feat: health and license", goFile, pyFile, licenseFile})

	// 4. Test commit with full telemetry & lineage
	runCommit([]string{
		"-u", "universe-test",
		"-i", "Initial test commit",
		"-a", "test-agent",
		"-p", "Add initial health endpoints",
		"--session-id", "sess-test-123",
		"--orchestrator-id", "orch-main",
		"-m", "gemini-3.7-flash",
		"--prompt-tokens", "1050",
		"--completion-tokens", "210",
		"--reasoning-tokens", "400",
		"--cost-usd", "0.0012",
		"--latency-ms", "520",
		"--trace-id", "4bf92f3577b34da6a3ce929d0e0e4736",
		"--span-id", "00f067aa0ba902b7",
	})

	// 5. Test status
	runStatus([]string{"-u", "universe-test"})

	// 6. Test topology
	runTopology([]string{"-u", "universe-test", "-f", "ascii"})

	// 7. Test universe commands
	runUniverse([]string{"list"})
	runUniverse([]string{"create", "-p", "universe-test", "u/branch-1"})
	runUniverse([]string{"diff", "universe-test", "u/branch-1"})

	// 8. Test proposal commands
	runProposal([]string{"list"})
	runProposal([]string{"create", "--source", "u/branch-1", "--target", "universe-test", "--title", "Test PR"})
	runProposal([]string{"view", "--source", "u/branch-1", "--target", "universe-test"})
	runProposal([]string{"review", "--source", "u/branch-1", "--target", "universe-test"})

	// 9. Test export
	exportDir := filepath.Join(tempDir, "exported")
	runExport([]string{"-u", "universe-test", "-d", exportDir})
	if _, err := os.Stat(exportDir); os.IsNotExist(err) {
		t.Errorf("Expected export directory %s to exist", exportDir)
	}
	if exportedLic, err := os.ReadFile(filepath.Join(exportDir, "LICENSE")); err != nil || string(exportedLic) != "Apache-2.0 License" {
		t.Errorf("Expected exported LICENSE to match, got err=%v, content=%s", err, string(exportedLic))
	}

	// 10. Test import-repo onboarding
	onboardDir := filepath.Join(tempDir, "sample_repo")
	_ = os.MkdirAll(onboardDir, 0755)
	_ = os.WriteFile(filepath.Join(onboardDir, "service.go"), []byte(`package service
func DoService() string { return "ok" }`), 0644)
	_ = os.WriteFile(filepath.Join(onboardDir, "README.md"), []byte("# Service Docs"), 0644)

	runImportRepo([]string{"-d", onboardDir, "-u", "universe-onboarded", "-p", "Onboard sample repo"})

	// 11. Test symbol view and blast radius impact
	runView([]string{"HandleHealth", "-u", "universe-test"})
	runBlastRadius([]string{"HandleHealth", "-u", "universe-test"})

	// 12. Test AST resolve and AST edit with disk synchronization (-w)
	runAST([]string{"resolve", "-u", "universe-test", "--target", "HandleHealth"})
	runAST([]string{
		"edit",
		"-u", "universe-test",
		"--op", "replace_function_body",
		"--target", "HandleHealth",
		"--content", "w.WriteHeader(http.StatusNoContent)",
		"-w",
	})

	updatedGo, err := os.ReadFile(goFile)
	if err != nil {
		t.Fatalf("Failed to read updated main.go from disk: %v", err)
	}
	if !strings.Contains(string(updatedGo), "StatusNoContent") {
		t.Errorf("Expected main.go on disk to reflect AST edit, got: %s", string(updatedGo))
	}
}

func TestCLI_IgnoreAndSecrets(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cosm_cli_ignore_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	// 1. Initialize repository
	runInit([]string{"-u", "universe-test"})

	// 2. Create .cosmignore
	ignoreContent := `# Ignore patterns
custom.log
temp_data/
*.ignored
`
	if err := os.WriteFile(".cosmignore", []byte(ignoreContent), 0644); err != nil {
		t.Fatalf("Failed to write .cosmignore: %v", err)
	}

	// 3. Create test files
	_ = os.WriteFile("custom.log", []byte("application logs"), 0644)
	_ = os.WriteFile("file.ignored", []byte("ignore me"), 0644)
	_ = os.WriteFile(".env", []byte("DB_HOST=localhost"), 0644) // Default secret filename
	_ = os.WriteFile("clean.go", []byte("package main\nfunc Clean() {}\n"), 0644)

	// 4. Adding ignored file without --force should fail
	err = runAddE([]string{"custom.log"})
	if err == nil || !strings.Contains(err.Error(), "ignored") {
		t.Errorf("Expected runAdd on custom.log to fail with ignored error, got %v", err)
	}

	// 5. Adding default-ignored file without --force should fail
	err = runAddE([]string{".env"})
	if err == nil || !strings.Contains(err.Error(), "ignored") {
		t.Errorf("Expected runAdd on .env to fail with ignored error, got %v", err)
	}

	// 6. Adding with --force should succeed for non-secret ignored file
	err = runAddE([]string{"--force", "custom.log"})
	if err != nil {
		t.Errorf("Expected runAdd --force on custom.log to succeed, got %v", err)
	}

	// Dynamically assemble synthetic secret fixture at test runtime to ensure
	// static repository secret scanners (GitHub push protection, TruffleHog, GitGuardian)
	// do not falsely flag test files as containing leaked credentials.
	dummySecretKey := fmt.Sprintf("%s%s%s", "AIza", "SyTestSecretFixtureKeyOnly", "123456789")

	// 7. Adding file with plaintext secret should fail
	secretFile := "secret.go"
	secretCode := fmt.Sprintf("package main\nvar apiKey = %q\n", dummySecretKey)
	_ = os.WriteFile(secretFile, []byte(secretCode), 0644)

	err = runAddE([]string{secretFile})
	if err == nil || !strings.Contains(err.Error(), "secret detected") {
		t.Errorf("Expected runAdd on secret.go to fail with secret detected, got %v", err)
	}

	// 8. Adding file with secret and --allow-secrets should succeed
	err = runAddE([]string{"--allow-secrets", secretFile})
	if err != nil {
		t.Errorf("Expected runAdd --allow-secrets on secret.go to succeed, got %v", err)
	}

	// 9. Adding file with inline cosm:allow-secret suppression should succeed without flag
	suppressedFile := "suppressed.go"
	suppressedCode := fmt.Sprintf("package main\nvar apiKey = %q // cosm:allow-secret\n", dummySecretKey)
	_ = os.WriteFile(suppressedFile, []byte(suppressedCode), 0644)
	err = runAddE([]string{suppressedFile})
	if err != nil {
		t.Errorf("Expected runAdd on suppressed.go to succeed with inline annotation, got %v", err)
	}

	// 10. Clean file should succeed
	err = runAddE([]string{"clean.go"})
	if err != nil {
		t.Errorf("Expected runAdd on clean.go to succeed, got %v", err)
	}
}

func TestCLI_ASTTree(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cosm_cli_ast_tree_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	// 1. Initialize repo
	runInit([]string{"-u", "universe-test"})

	// 2. Create sample source files
	goCode := `package main

import "net/http"

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func ValidateToken(token string) bool {
	return token != ""
}`
	goFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(goFile, []byte(goCode), 0644); err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	pyCode := `def process_item(item_id: str):
    return {"id": item_id, "status": "processed"}`
	pyFile := filepath.Join(tempDir, "service.py")
	if err := os.WriteFile(pyFile, []byte(pyCode), 0644); err != nil {
		t.Fatalf("Failed to write service.py: %v", err)
	}

	// 3. Stage & Commit to universe-test
	runAdd([]string{"-u", "universe-test", "-a", "test-agent", "-p", "add endpoints", goFile, pyFile})
	runCommit([]string{"-u", "universe-test", "-i", "feat: initial commit for ast tree test", "-a", "test-agent", "-p", "setup test services"})

	// 4. Attach a synthetic cross-boundary edge with a contract schema to test contract visualization
	blobStore, graphEngine, err := openStorage()
	if err != nil {
		t.Fatalf("openStorage failed: %v", err)
	}
	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	head, err := universeMgr.GetUniverseManifest("universe-test")
	if err != nil {
		t.Fatalf("GetUniverseManifest failed: %v", err)
	}
	_, symMap := loadManifestState(blobStore, graphEngine, head)

	var healthNodeID, validateNodeID string
	for _, sym := range symMap {
		if strings.Contains(sym.Identifier, "HandleHealth") {
			healthNodeID = sym.NodeID
		}
		if strings.Contains(sym.Identifier, "ValidateToken") {
			validateNodeID = sym.NodeID
		}
	}
	if healthNodeID != "" && validateNodeID != "" {
		head.CrossEdges = append(head.CrossEdges, core.CrossBoundaryEdge{
			SourceNodeID:     healthNodeID,
			TargetNodeID:     validateNodeID,
			Type:             core.EdgeCalls,
			ContractSchemaID: "auth.v1.ValidateToken",
		})
		_, err = universeMgr.CommitManifest("universe-test", head)
		if err != nil {
			t.Fatalf("Failed to commit manifest with cross-edge: %v", err)
		}
	}
	graphEngine.Close()

	// Helper to capture stdout
	captureOutput := func(f func()) string {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		outChan := make(chan string)
		go func() {
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			outChan <- buf.String()
		}()

		f()

		_ = w.Close()
		os.Stdout = oldStdout
		out := <-outChan
		_ = r.Close()
		return out
	}

	// 5. Test text output via 'cosm ast tree'
	textOut := captureOutput(func() {
		runAST([]string{"tree", "-u", "universe-test", "-f", "text"})
	})

	if !strings.Contains(textOut, "universe-test") {
		t.Errorf("Expected text output to contain universe name 'universe-test', got:\n%s", textOut)
	}
	if !strings.Contains(textOut, "main.go") {
		t.Errorf("Expected text output to contain 'main.go', got:\n%s", textOut)
	}
	if !strings.Contains(textOut, "HandleHealth") {
		t.Errorf("Expected text output to contain 'HandleHealth', got:\n%s", textOut)
	}
	if !strings.Contains(textOut, "ValidateToken") {
		t.Errorf("Expected text output to contain 'ValidateToken', got:\n%s", textOut)
	}
	if !strings.Contains(textOut, "Signature:") {
		t.Errorf("Expected text output to contain 'Signature:', got:\n%s", textOut)
	}
	if !strings.Contains(textOut, "service.py") {
		t.Errorf("Expected text output to contain 'service.py', got:\n%s", textOut)
	}
	if !strings.Contains(textOut, "process_item") {
		t.Errorf("Expected text output to contain 'process_item', got:\n%s", textOut)
	}
	if !strings.Contains(textOut, "auth.v1.ValidateToken") {
		t.Errorf("Expected text output to contain contract 'auth.v1.ValidateToken', got:\n%s", textOut)
	}

	// 6. Test JSON output via 'cosm ast tree --format json'
	jsonOut := captureOutput(func() {
		runAST([]string{"tree", "-u", "universe-test", "--format", "json"})
	})

	var treeGraph ASTTreeGraph
	if err := json.Unmarshal([]byte(jsonOut), &treeGraph); err != nil {
		t.Fatalf("Failed to parse JSON output: %v\nOutput was:\n%s", err, jsonOut)
	}

	if treeGraph.UniverseID != "universe-test" {
		t.Errorf("Expected UniverseID 'universe-test', got %q", treeGraph.UniverseID)
	}
	if treeGraph.TotalComponents != 2 {
		t.Errorf("Expected 2 components, got %d", treeGraph.TotalComponents)
	}
	if treeGraph.TotalSymbols < 3 {
		t.Errorf("Expected at least 3 symbols, got %d", treeGraph.TotalSymbols)
	}

	var foundHealth bool
	for _, comp := range treeGraph.Components {
		for _, sym := range comp.Symbols {
			if strings.Contains(sym.Identifier, "HandleHealth") {
				foundHealth = true
				if sym.Signature == "" {
					t.Errorf("Expected HandleHealth to have a non-empty Signature")
				}
				if len(sym.OutgoingEdges) == 0 {
					t.Errorf("Expected HandleHealth to have outgoing edges")
				} else {
					edge := sym.OutgoingEdges[0]
					if edge.ContractSchemaID != "auth.v1.ValidateToken" {
						t.Errorf("Expected contract 'auth.v1.ValidateToken', got %q", edge.ContractSchemaID)
					}
					if edge.EdgeType != core.EdgeCalls {
						t.Errorf("Expected EdgeType CALLS, got %q", edge.EdgeType)
					}
				}
			}
		}
	}
	if !foundHealth {
		t.Errorf("HandleHealth symbol not found in JSON graph components")
	}

	// 7. Also verify -f json flag variant
	jsonOut2 := captureOutput(func() {
		runASTTree([]string{"-u", "universe-test", "-f", "json"})
	})
	var treeGraph2 ASTTreeGraph
	if err := json.Unmarshal([]byte(jsonOut2), &treeGraph2); err != nil {
		t.Fatalf("Failed to parse -f json output: %v\nOutput was:\n%s", err, jsonOut2)
	}
	if treeGraph2.UniverseID != "universe-test" {
		t.Errorf("Expected UniverseID 'universe-test', got %q", treeGraph2.UniverseID)
	}
}

func TestCLI_ErgonomicsAndLog(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cosm_cli_ergonomics_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	captureStdout := func(f func()) string {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		f()
		_ = w.Close()
		os.Stdout = oldStdout
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		return buf.String()
	}

	// 1. Init repository with custom universe
	runInit([]string{"-u", "universe-main"})

	// 2. Stage Go source code
	goCode := `package main

import "net/http"

type Server struct{}

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func ValidateToken() bool {
	return true
}
`
	if err := os.WriteFile("main.go", []byte(goCode), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	runAdd([]string{"-u", "universe-main", "-a", "agent-builder-007", "-p", "Add health check handler", "-i", "feat: initial health handler", "main.go"})
	runCommit([]string{
		"-u", "universe-main",
		"-i", "feat: initial commit with Server.HandleHealth",
		"-p", "Add health check handler",
		"-a", "agent-builder-007",
		"-m", "gemini-3.8-flash",
	})

	// 3. Test cosm view with human symbol identifier path
	viewOut := captureStdout(func() {
		runView([]string{"HandleHealth", "-u", "universe-main", "--format", "terminal"})
	})
	if !strings.Contains(viewOut, "HandleHealth") {
		t.Errorf("Expected cosm view to resolve and display HandleHealth, got:\n%s", viewOut)
	}

	// Test cosm view with qualified symbol identifier path
	viewQualOut := captureStdout(func() {
		runView([]string{"main.go::HandleHealth", "-u", "universe-main"})
	})
	if !strings.Contains(viewQualOut, "HandleHealth") {
		t.Errorf("Expected cosm view with main.go::HandleHealth to resolve, got:\n%s", viewQualOut)
	}

	// Test cosm lineage with human symbol identifier path
	lineageOut := captureStdout(func() {
		runLineage([]string{"HandleHealth", "-u", "universe-main"})
	})
	if !strings.Contains(lineageOut, "Lineage Provenance Pedigree") || !strings.Contains(lineageOut, "agent-builder-007") {
		t.Errorf("Expected cosm lineage to resolve HandleHealth, got:\n%s", lineageOut)
	}

	// Test cosm blast-radius with human symbol identifier path
	blastOut := captureStdout(func() {
		runBlastRadius([]string{"HandleHealth", "-u", "universe-main"})
	})
	if !strings.Contains(blastOut, "Blast-Radius") && !strings.Contains(blastOut, "Contract Impact") {
		t.Errorf("Expected cosm blast-radius to resolve HandleHealth, got:\n%s", blastOut)
	}

	// 4. Create micro-universe branch and test proposal create with short flags -s and -t
	runUniverse([]string{"create", "-p", "universe-main", "u/feature-auth"})

	// Make a second commit in u/feature-auth
	updatedGoCode := goCode + "\nfunc AuthenticateRequest() bool { return true }\n"
	if err := os.WriteFile("main.go", []byte(updatedGoCode), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	runAdd([]string{"-u", "u/feature-auth", "-i", "feat: add AuthenticateRequest", "main.go"})
	runCommit([]string{
		"-u", "u/feature-auth",
		"-i", "feat: add auth endpoint in feature branch",
		"-p", "Implement token validation",
		"-a", "agent-auth-001",
	})

	propOut := captureStdout(func() {
		runProposal([]string{"create", "-s", "u/feature-auth", "-t", "universe-main", "-title", "Feature Auth Proposal"})
	})
	if !strings.Contains(propOut, "Feature Auth Proposal") || !strings.Contains(propOut, "u/feature-auth -> Target: universe-main") {
		t.Errorf("Expected proposal create with -s and -t to succeed, got:\n%s", propOut)
	}

	// Test proposal view with -s and -t
	propViewOut := captureStdout(func() {
		runProposal([]string{"view", "-s", "u/feature-auth", "-t", "universe-main"})
	})
	if !strings.Contains(propViewOut, "Proposal Review") && !strings.Contains(propViewOut, "u/feature-auth") {
		t.Errorf("Expected proposal view with -s and -t to succeed, got:\n%s", propViewOut)
	}

	// 5. Test cosm log command (terminal format)
	logOut := captureStdout(func() {
		runLog([]string{"-u", "universe-main"})
	})
	if !strings.Contains(logOut, "commit ") || !strings.Contains(logOut, "initial commit with Server.HandleHealth") {
		t.Errorf("Expected cosm log output to contain commit and intent, got:\n%s", logOut)
	}
	if !strings.Contains(logOut, "HandleHealth") {
		t.Errorf("Expected cosm log output to contain symbol changes, got:\n%s", logOut)
	}

	// 6. Test cosm log command (json format)
	logJSONOut := captureStdout(func() {
		runLog([]string{"-u", "universe-main", "--format", "json"})
	})
	var entries []CommitLogEntry
	if err := json.Unmarshal([]byte(logJSONOut), &entries); err != nil {
		t.Fatalf("Failed to parse cosm log json output: %v\nOutput was:\n%s", err, logJSONOut)
	}
	if len(entries) == 0 {
		t.Fatalf("Expected at least 1 commit log entry, got 0")
	}
	if entries[0].UniverseID != "universe-main" {
		t.Errorf("Expected entry UniverseID 'universe-main', got %q", entries[0].UniverseID)
	}
	if !strings.Contains(entries[0].Intent, "HandleHealth") {
		t.Errorf("Expected intent to mention HandleHealth, got %q", entries[0].Intent)
	}
}

func TestCLIStatusWorkingTreeLens(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cosm_status_lens_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	captureStdout := func(f func()) string {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		f()
		_ = w.Close()
		os.Stdout = oldStdout
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		return buf.String()
	}

	// 1. Initialize repository
	runInit([]string{"-u", "universe-main"})

	// Status on empty repo
	statusEmpty := captureStdout(func() {
		runStatus([]string{"-u", "universe-main"})
	})
	if !strings.Contains(statusEmpty, "No AST components tracked yet") {
		t.Errorf("Expected clean empty status, got:\n%s", statusEmpty)
	}

	// 2. Create sample files
	pkgDir := filepath.Join(tempDir, "pkg")
	_ = os.MkdirAll(pkgDir, 0755)
	file1 := filepath.Join(pkgDir, "service.go")
	_ = os.WriteFile(file1, []byte("package pkg\n\nfunc Run() {}\n"), 0644)

	// Add and commit
	runAdd([]string{"-u", "universe-main", "pkg/service.go"})
	runCommit([]string{"-u", "universe-main", "-i", "initial service commit"})

	// 3. Status should show clean working tree lens
	statusClean := captureStdout(func() {
		runStatus([]string{"-u", "universe-main"})
	})
	if !strings.Contains(statusClean, "Working Tree Lens: Materialized projection in sync with Merkle root") {
		t.Errorf("Expected in sync working tree lens, got:\n%s", statusClean)
	}

	// 4. Modify file on disk -> drift detected
	_ = os.WriteFile(file1, []byte("package pkg\nfunc Run() { println(\"drift\") }\n"), 0644)
	statusDrift := captureStdout(func() {
		runStatus([]string{"-u", "universe-main"})
	})
	if !strings.Contains(statusDrift, "Working Tree Lens: File drift detected") || !strings.Contains(statusDrift, "modified on disk") {
		t.Errorf("Expected drift detected (modified on disk), got:\n%s", statusDrift)
	}

	// 5. Test JSON status format with drift
	statusJSON := captureStdout(func() {
		runStatus([]string{"-u", "universe-main", "-f", "json"})
	})
	var jsonMap map[string]interface{}
	if err := json.Unmarshal([]byte(statusJSON), &jsonMap); err != nil {
		t.Fatalf("Failed to parse json status: %v", err)
	}
	lens, ok := jsonMap["working_tree_lens"].(map[string]interface{})
	if !ok {
		t.Fatalf("Missing working_tree_lens in json status")
	}
	if lens["status"] != "drift_detected" {
		t.Errorf("Expected lens status 'drift_detected', got %v", lens["status"])
	}

	// 6. Delete file on disk -> missing on disk drift
	_ = os.Remove(file1)
	statusMissing := captureStdout(func() {
		runStatus([]string{"-u", "universe-main"})
	})
	if !strings.Contains(statusMissing, "missing on disk") {
		t.Errorf("Expected missing on disk drift, got:\n%s", statusMissing)
	}

	// 7. Reconstitute via cosm export -d . -> in sync again
	runExport([]string{"-u", "universe-main", "-d", "."})
	statusRestored := captureStdout(func() {
		runStatus([]string{"-u", "universe-main"})
	})
	if !strings.Contains(statusRestored, "Working Tree Lens: Materialized projection in sync with Merkle root") {
		t.Errorf("Expected restored in sync working tree lens, got:\n%s", statusRestored)
	}
}

// TestCLI_InitAndAddSimplification verifies the onboarding improvements:
// - cosm init outputs clear next-steps guidance (AST-first vs filesystem staging)
// - cosm add with no args prints helpful usage
// - cosm ast create --lang scaffolds components directly into the AST DAG
// - cosm add . recursively expands directories and stages files
func TestCLI_InitAndAddSimplification(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cosm_init_add_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	captureStdout := func(f func()) string {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		f()
		_ = w.Close()
		os.Stdout = oldStdout
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		return buf.String()
	}

	// 1. Verify cosm init output guidance
	initOut := captureStdout(func() {
		runInit([]string{"-u", "universe-main"})
	})
	if !strings.Contains(initOut, "Next Steps") || !strings.Contains(initOut, "Native AST Inception") || !strings.Contains(initOut, "Filesystem Staging Lens") {
		t.Errorf("Expected cosm init to display next steps guidance, got:\n%s", initOut)
	}

	// 2. Verify cosm add with no args prints clear usage
	addNoArgsOut := captureStdout(func() {
		runAdd([]string{})
	})
	if !strings.Contains(addNoArgsOut, "cosm add <file_or_dir...>") || !strings.Contains(addNoArgsOut, "cosm add .") {
		t.Errorf("Expected cosm add usage guidance, got:\n%s", addNoArgsOut)
	}

	// 3. Verify cosm ast create with --lang scaffolding
	createOut := captureStdout(func() {
		runAST([]string{"create", "-c", "services/billing", "--lang", "go", "-u", "universe-main", "-w"})
	})
	if !strings.Contains(createOut, "AST Component Inception Complete") {
		t.Fatalf("Expected ast create to succeed, got:\n%s", createOut)
	}

	// Verify disk file was synchronized
	if _, err := os.Stat("services/billing.go"); os.IsNotExist(err) {
		t.Fatalf("Expected services/billing.go to exist on disk after ast create -w")
	}

	// 4. Verify cosm add . recursively expands directory and stages
	// Create another file in a sub-sub directory
	_ = os.MkdirAll("pkg/math", 0755)
	_ = os.WriteFile("pkg/math/calc.py", []byte("def add(a, b):\n    return a + b\n"), 0644)

	addDotOut := captureStdout(func() {
		runAdd([]string{"-u", "universe-main", "-i", "stage all files", "."})
	})
	if !strings.Contains(addDotOut, "Staged AST Component") {
		t.Errorf("Expected cosm add . to stage files, got:\n%s", addDotOut)
	}

	// 5. Commit staged changes
	commitOut := captureStdout(func() {
		runCommit([]string{"-u", "universe-main", "-i", "feat: initial workspace commit"})
	})
	if !strings.Contains(commitOut, "Committed to universe") {
		t.Errorf("Expected successful commit, got:\n%s", commitOut)
	}
}

// TestCLI_ASTEditAndPositionIndependentFlags verifies:
// - cosm ast edit prints modified symbol details (NodeID prefix, identifier, type)
// - cosm view parses flags correctly regardless of whether -u comes before or after the target
// - cosm ast resolve parses flags correctly regardless of flag position
func TestCLI_ASTEditAndPositionIndependentFlags(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cosm_ast_flags_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	captureStdout := func(f func()) string {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		f()
		_ = w.Close()
		os.Stdout = oldStdout
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		return buf.String()
	}

	// 1. Initialize repo
	runInit([]string{"-u", "universe-main"})

	// 2. Incept a Go component with HandleHealth
	goSrc := `package main

import "net/http"

type Server struct{}

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
`
	runAST([]string{"create", "-c", "cmd/server", "-f", "cmd/server/main.go", "--code", goSrc, "-u", "universe-main", "-w"})

	// 3. Create micro-universe
	runUniverse([]string{"create", "universe-agent-health", "--parent", "universe-main"})

	// 4. Perform AST edit in universe-agent-health
	newBody := `w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusOK)
w.Write([]byte("{\"status\":\"healthy\",\"version\":\"v2.0\"}"))`
	editOut := captureStdout(func() {
		runAST([]string{"edit", "--op", "replace_function_body", "--target", "HandleHealth", "--content", newBody, "-u", "universe-agent-health"})
	})

	if !strings.Contains(editOut, "Declarative AST Mutation Applied") {
		t.Fatalf("Expected edit to succeed, got:\n%s", editOut)
	}
	if !strings.Contains(editOut, "Modified Symbols:   1") {
		t.Errorf("Expected 1 modified symbol in output, got:\n%s", editOut)
	}
	if !strings.Contains(editOut, "HandleHealth") || !strings.Contains(editOut, "└─") {
		t.Errorf("Expected modified symbol details with Node ID and HandleHealth, got:\n%s", editOut)
	}

	// 5. Test cosm view with trailing -u flag (HandleHealth -u universe-agent-health)
	viewOutTrailing := captureStdout(func() {
		runView([]string{"HandleHealth", "-u", "universe-agent-health", "-format", "source"})
	})
	if !strings.Contains(viewOutTrailing, "status") || !strings.Contains(viewOutTrailing, "healthy") {
		t.Errorf("Expected view with trailing -u flag to show mutated body, got:\n%s", viewOutTrailing)
	}

	// 6. Test cosm view with leading -u flag (-u universe-agent-health HandleHealth)
	viewOutLeading := captureStdout(func() {
		runView([]string{"-u", "universe-agent-health", "HandleHealth", "-format", "source"})
	})
	if !strings.Contains(viewOutLeading, "status") || !strings.Contains(viewOutLeading, "healthy") {
		t.Errorf("Expected view with leading -u flag to show mutated body, got:\n%s", viewOutLeading)
	}

	// 7. Test cosm ast resolve with trailing -u flag
	resolveTrailing := captureStdout(func() {
		runAST([]string{"resolve", "HandleHealth", "-u", "universe-agent-health"})
	})
	if !strings.Contains(resolveTrailing, "Resolved Symbol:") || !strings.Contains(resolveTrailing, "HandleHealth") {
		t.Errorf("Expected ast resolve with trailing -u flag to succeed, got:\n%s", resolveTrailing)
	}

	// 8. Test cosm ast resolve with leading -u flag
	resolveLeading := captureStdout(func() {
		runAST([]string{"resolve", "-u", "universe-agent-health", "HandleHealth"})
	})
	if !strings.Contains(resolveLeading, "Resolved Symbol:") || !strings.Contains(resolveLeading, "HandleHealth") {
		t.Errorf("Expected ast resolve with leading -u flag to succeed, got:\n%s", resolveLeading)
	}
}


