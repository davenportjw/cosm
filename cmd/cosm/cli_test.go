package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	// 11. Test symbol edit and impact
	runSymbol([]string{"edit", "-id", "sample-symbol-id", "--body", "func DoService() string { return \"updated\" }", "-u", "universe-onboarded"})
	runSymbol([]string{"impact", "-id", "sample-symbol-id"})

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
