package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// setupDocTestEnv prepares an isolated temporary workspace for doc testing.
func setupDocTestEnv(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "cosm_doc_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}

	cleanup := func() {
		_ = os.Chdir(origWd)
		_ = os.RemoveAll(tempDir)
	}
	return tempDir, cleanup
}

// writeSampleFiles writes sample Go, Python, and raw files for doc workflow execution.
func writeSampleFiles(t *testing.T, dir string) (string, string, string) {
	t.Helper()
	goCode := `package main

import "net/http"

func ValidateToken() bool {
	return false
}

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
`
	goFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(goFile, []byte(goCode), 0644); err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	pyCode := `from fastapi import FastAPI
app = FastAPI()

@app.get("/health")
def health():
    return {"status": "ok"}
`
	pyFile := filepath.Join(dir, "app.py")
	if err := os.WriteFile(pyFile, []byte(pyCode), 0644); err != nil {
		t.Fatalf("Failed to write app.py: %v", err)
	}

	rawFile := filepath.Join(dir, "LICENSE")
	if err := os.WriteFile(rawFile, []byte("Apache-2.0 License\n"), 0644); err != nil {
		t.Fatalf("Failed to write LICENSE: %v", err)
	}

	return goFile, pyFile, rawFile
}

// TestDocExamples_HowToPRAndCollaboration verifies every example in docs/HOW_TO_PR_AND_COLLABORATION.md:
// - Stacked changes registration (Section 4.1)
// - Stack listing (Section 4.1)
// - Auto-evolution rebasing (Section 4.2)
// - Universe Proposals creation, list, view, review, merge (Sections 1-3)
// - Method 1: In-place surgical AST mutation for history repair (Section 4.3)
// - Method 2: Micro-universe fork and union merge (Section 4.3)
func TestDocExamples_HowToPRAndCollaboration(t *testing.T) {
	dir, cleanup := setupDocTestEnv(t)
	defer cleanup()

	goFile, pyFile, rawFile := writeSampleFiles(t, dir)

	// Step 1: Initialize repository
	runInit([]string{"-u", "universe-main"})

	// Step 2: Stage and commit base code
	runAdd([]string{"-u", "universe-main", "-a", "did:key:alice", "-p", "Initial setup", "-i", "chore: bootstrap project", goFile, pyFile, rawFile})
	runCommit([]string{"-u", "universe-main", "-i", "Initial bootstrap commit"})

	// Step 3: Create micro-universe branch u/auth-service
	runUniverse([]string{"create", "u/auth-service", "-p", "universe-main"})

	// Step 4: Section 4.1 - Stacked change registration
	runStack([]string{"create", "-c", "ch-auth-jwt", "-u", "u/auth-service", "-p", "universe-main", "--title", "Add JWT Auth Service"})

	// Step 5: Section 4.1 - Stack listing
	runStack([]string{"list"})

	// Step 6: Sections 1-3 - Universe Proposals (PR equivalent)
	// Create proposal
	runProposal([]string{"create", "--source", "u/auth-service", "--target", "universe-main", "--title", "Autonomous Auth Migration"})

	// List proposals
	runProposal([]string{"list"})

	// View proposal diff
	runProposal([]string{"view", "--source", "u/auth-service", "--target", "universe-main"})

	// AI Critic review
	runProposal([]string{"review", "--source", "u/auth-service", "--target", "universe-main", "--critic", "agent-critic-deepseek"})

	// Merge proposal
	runProposal([]string{"merge", "--source", "u/auth-service", "--target", "universe-main"})

	// Step 7: Section 4.2 - Stack auto-evolution (rebase downstream changes)
	runStack([]string{"evolve", "-c", "universe-main"})

	// Step 8: Section 4.3 Method 1 - Surgical AST Repair (zero history rewriting)
	runAST([]string{"edit", "--op", "replace_function_body", "--target", "ValidateToken", "--content", "func ValidateToken() bool { return true }", "-u", "universe-main", "-w"})
	runCommit([]string{"-u", "universe-main", "-i", "fix(auth): Correct token validation logic"})

	// Step 9: Section 4.3 Method 2 - Micro-Universe Isolation and Union Merge
	runUniverse([]string{"create", "u/hotfix-patch", "-p", "universe-main"})
	runAST([]string{"edit", "--op", "replace_function_body", "--target", "ValidateToken", "--content", "func ValidateToken() bool { return true }", "-u", "u/hotfix-patch", "-w"})
	runShip([]string{"-u", "u/hotfix-patch", "-t", "target:cosm"})
	runUniverse([]string{"merge", "u/hotfix-patch", "-t", "universe-main", "-s", "union"})
}

// TestDocExamples_CosmForGitHubUsers verifies the 8-step guide in docs/guides/cosm-for-github-users.md:
// - Step 1: cosm init
// - Step 2: cosm universe create
// - Step 3: cosm add
// - Step 4: cosm commit with lineage
// - Step 5: cosm status & topology
// - Step 6: cosm stack create, cosm stack evolve, cosm proposal create, cosm ast edit
// - Step 7: cosm ship
// - Step 8: cosm git init-bridge, status, log
func TestDocExamples_CosmForGitHubUsers(t *testing.T) {
	dir, cleanup := setupDocTestEnv(t)
	defer cleanup()

	goFile, pyFile, _ := writeSampleFiles(t, dir)

	// Step 1: Initialize
	runInit([]string{"-u", "universe-main"})

	// Step 2: Branch micro-universe
	runUniverse([]string{"create", "u/auth-service", "-p", "universe-main"})

	// Step 3: Stage polyglot AST symbols
	runAdd([]string{"-u", "u/auth-service", "-a", "did:key:alice", "-p", "Build authentication backend", "-i", "feat: auth middleware", goFile, pyFile})

	// Step 4: Commit with lineage envelope
	runCommit([]string{
		"-u", "u/auth-service",
		"-i", "feat(auth): Implement token validation and API endpoints",
		"-p", "Add JWT auth and health checks",
		"-a", "did:key:alice",
		"--prompt-tokens", "1250",
		"--comp-tokens", "340",
		"--cost", "0.0018",
		"--trace", "trace-github-guide-001",
	})

	// Step 5: Status and Topology inspection
	runStatus([]string{"-u", "u/auth-service"})
	runTopology([]string{"-u", "u/auth-service"})

	// Step 6: Stacking, Auto-Rebase & Proposals
	runStack([]string{"create", "-c", "ch-auth-v1", "-u", "u/auth-service", "-p", "universe-main", "--title", "Auth v1 Stack"})
	runStack([]string{"list"})
	runStack([]string{"evolve", "-c", "universe-main"})
	runProposal([]string{"create", "--source", "u/auth-service", "--target", "universe-main", "--title", "PR: Auth v1"})
	runAST([]string{"edit", "--op", "replace_function_body", "--target", "ValidateToken", "--content", "func ValidateToken() bool { return true }", "-u", "u/auth-service", "-w"})

	// Step 7: Compilation sidecar and preview
	runShip([]string{"-u", "u/auth-service", "-t", "target:cosm"})

	// Step 8: Git CLI interop shim
	runGit([]string{"init-bridge", "-u", "u/auth-service", "-d", dir})
	runGit([]string{"status"})
	runGit([]string{"log"})
}

// TestDocExamples_MicroUniverses verifies docs/guides/micro-universes.md:
// - Parallel swarm forks (u/agent-1, u/agent-2)
// - Listing universes
// - AST mutations inside micro-universes
// - Universe diffing
// - Universe union merging
// - Section 3: 5-step non-destructive history repair workflow
func TestDocExamples_MicroUniverses(t *testing.T) {
	dir, cleanup := setupDocTestEnv(t)
	defer cleanup()

	goFile, _, _ := writeSampleFiles(t, dir)

	// Base repo
	runInit([]string{"-u", "universe-main"})
	runAdd([]string{"-u", "universe-main", "-i", "init", goFile})
	runCommit([]string{"-u", "universe-main", "-i", "Initial commit"})

	// 1. Fork micro-universes
	runUniverse([]string{"create", "u/agent-1", "-p", "universe-main"})
	runUniverse([]string{"create", "u/agent-2", "-p", "universe-main"})
	runUniverse([]string{"list"})

	// 2. Perform AST surgery in parallel universe
	runAST([]string{"edit", "--op", "replace_function_body", "--target", "ValidateToken", "--content", "func ValidateToken() bool { return true }", "-u", "u/agent-1", "-w"})
	runCommit([]string{"-u", "u/agent-1", "-i", "agent-1: fix validation"})

	// 3. Diff universes
	runUniverse([]string{"diff", "universe-main", "u/agent-1"})

	// 4. Merge winner universe
	runUniverse([]string{"merge", "u/agent-1", "-t", "universe-main", "-s", "union"})

	// 5. Section 3: 5-step non-destructive history repair & mistake isolation
	// Step 1: Fork patch universe
	runUniverse([]string{"create", "u/hotfix-patch", "-p", "universe-main"})
	// Step 2: Surgically mutate the target AST node
	runAST([]string{"edit", "--op", "replace_function_body", "--target", "ValidateToken", "--content", "func ValidateToken() bool { return true }", "-u", "u/hotfix-patch", "-w"})
	// Step 3: Run isolated target compilation sidecar
	runShip([]string{"-u", "u/hotfix-patch", "-t", "target:cosm"})
	// Step 4: Union-merge patch universe into universe-main
	runUniverse([]string{"merge", "u/hotfix-patch", "-t", "universe-main", "-s", "union"})
	// Step 5: Stack auto-evolution
	runStack([]string{"create", "-c", "ch-patch-v1", "-u", "u/hotfix-patch", "-p", "universe-main", "--title", "Hotfix Patch Stack"})
	runStack([]string{"evolve", "-c", "universe-main"})
}

// TestDocExamples_ASTMutationsAndLineage verifies docs/guides/ast-mutations-and-lineage.md:
// - Symbol resolution
// - Declarative AST surgery with disk synchronization
// - Blast-radius computation
// - Lineage ancestry tracing
// - Node viewing
func TestDocExamples_ASTMutationsAndLineage(t *testing.T) {
	dir, cleanup := setupDocTestEnv(t)
	defer cleanup()

	goFile, _, _ := writeSampleFiles(t, dir)

	runInit([]string{"-u", "universe-main"})
	runAdd([]string{"-u", "universe-main", "-a", "agent-oracle", "-p", "AST Lineage Test", "-i", "feat: add ValidateToken", goFile})
	runCommit([]string{"-u", "universe-main", "-i", "Initial commit for AST surgery"})

	// 1. Resolve symbol
	runAST([]string{"resolve", "ValidateToken", "-u", "universe-main"})

	// 2. Surgical edit
	runAST([]string{"edit", "--op", "replace_function_body", "--target", "ValidateToken", "--content", "func ValidateToken() bool { return true }", "-u", "universe-main", "-w"})

	// 3. Blast radius
	runBlastRadius([]string{"agent-oracle"})

	// 4. Lineage trace
	runLineage([]string{"ValidateToken"})

	// 5. Symbol view and impact
	runSymbol([]string{"view", "ValidateToken", "-u", "universe-main"})
	runSymbol([]string{"impact", "ValidateToken", "-u", "universe-main"})
}

// TestDocExamples_GitInterop verifies docs/guides/git-interop.md:
// - Bridge initialization
// - Git status, log, and diff through Cosm shim
func TestDocExamples_GitInterop(t *testing.T) {
	dir, cleanup := setupDocTestEnv(t)
	defer cleanup()

	goFile, _, _ := writeSampleFiles(t, dir)

	runInit([]string{"-u", "universe-main"})
	runAdd([]string{"-u", "universe-main", "-i", "feat: git interop", goFile})
	runCommit([]string{"-u", "universe-main", "-i", "Commit 1 for git bridge"})

	// 1. Initialize git bridge
	runGit([]string{"init-bridge", "-u", "universe-main", "-d", dir})

	// 2. Git status
	runGit([]string{"status"})

	// 3. Git log
	runGit([]string{"log"})

	// 4. Git diff
	runGit([]string{"diff"})

	// 5. Git push shim
	runGit([]string{"push", "origin", "universe-main:main"})
}

// TestDocExamples_TargetShipping verifies docs/guides/target-shipping.md:
// - cosm ship to cosm target
// - cosm ship to cloud-run target
func TestDocExamples_TargetShipping(t *testing.T) {
	dir, cleanup := setupDocTestEnv(t)
	defer cleanup()

	goFile, pyFile, _ := writeSampleFiles(t, dir)

	runInit([]string{"-u", "universe-main"})
	runAdd([]string{"-u", "universe-main", "-i", "feat: shipping", goFile, pyFile})
	runCommit([]string{"-u", "universe-main", "-i", "Commit for shipping target"})

	// Default cosm target
	runShip([]string{"-u", "universe-main", "-t", "target:cosm"})

	// Cloud Run target
	runShip([]string{"-u", "universe-main", "-t", "target:cloud-run"})
}

// TestDocExamples_CLIReferenceAllCommands tests that all commands documented in
// docs/reference/cli.md execute without crashing or panicking.
func TestDocExamples_CLIReferenceAllCommands(t *testing.T) {
	dir, cleanup := setupDocTestEnv(t)
	defer cleanup()

	goFile, _, _ := writeSampleFiles(t, dir)

	// Command 1: cosm init
	runInit([]string{"-u", "universe-main"})

	// Command 2: cosm add
	runAdd([]string{"-u", "universe-main", "-a", "agent-doc-test", "-p", "CLI reference test", "-i", "feat: CLI test", goFile})

	// Command 3: cosm commit
	runCommit([]string{"-u", "universe-main", "-i", "CLI Reference Commit", "-p", "Doc test prompt", "-a", "agent-doc-test"})

	// Command 4: cosm status
	runStatus([]string{"-u", "universe-main"})

	// Command 5: cosm view
	runView([]string{"ValidateToken"})

	// Command 6: cosm topology
	runTopology([]string{"-u", "universe-main"})

	// Command 7: cosm blast-radius
	runBlastRadius([]string{"agent-doc-test"})

	// Command 8: cosm lineage
	runLineage([]string{"ValidateToken"})

	// Command 9: cosm ship
	runShip([]string{"-u", "universe-main", "-t", "target:cosm"})

	// Command 10: cosm universe
	runUniverse([]string{"list"})
	runUniverse([]string{"create", "u/cli-ref-branch", "-p", "universe-main"})
	runUniverse([]string{"diff", "universe-main", "u/cli-ref-branch"})
	runUniverse([]string{"merge", "u/cli-ref-branch", "-t", "universe-main", "-s", "union"})

	// Command 11: cosm proposal
	runProposal([]string{"create", "--source", "u/cli-ref-branch", "--target", "universe-main", "--title", "CLI Ref Proposal"})
	runProposal([]string{"list"})
	runProposal([]string{"view", "--source", "u/cli-ref-branch", "--target", "universe-main"})
	runProposal([]string{"review", "--source", "u/cli-ref-branch", "--target", "universe-main"})
	runProposal([]string{"merge", "--source", "u/cli-ref-branch", "--target", "universe-main"})

	// Command 12: cosm stack
	runStack([]string{"create", "-c", "ch-cli-ref", "-u", "u/cli-ref-branch", "-p", "universe-main", "--title", "CLI Stack"})
	runStack([]string{"list"})
	runStack([]string{"evolve", "-c", "universe-main"})

	// Command 13: cosm symbol
	runSymbol([]string{"view", "ValidateToken", "-u", "universe-main"})
	runSymbol([]string{"impact", "ValidateToken", "-u", "universe-main"})

	// Command 14: cosm ast
	runAST([]string{"resolve", "ValidateToken", "-u", "universe-main"})
	runAST([]string{"edit", "--op", "replace_function_body", "--target", "ValidateToken", "--content", "func ValidateToken() bool { return true }", "-u", "universe-main", "-w"})

	// Command 15: cosm export
	exportDir := filepath.Join(dir, "exported")
	runExport([]string{"-u", "universe-main", "-o", exportDir})

	// Command 16: cosm git
	runGit([]string{"init-bridge", "-u", "universe-main", "-d", dir})
	runGit([]string{"status"})
	runGit([]string{"log"})
	runGit([]string{"diff"})

	// Command 17: cosm auth
	runAuth([]string{"status"})

	// Command 18: cosm credential-helper
	runCredentialHelper([]string{"get"})

	// Command 19: cosm share
	runShare([]string{})

	// Command 20: cosm claim, blackboard & release
	runClaim([]string{})
	runBlackboard([]string{})
	runRelease([]string{})
}

// TestDocExamples_MarkdownCommandExtractorAndValidator scans markdown documentation
// files in docs/ and asserts that all documented CLI subcommands are recognized.
func TestDocExamples_MarkdownCommandExtractorAndValidator(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("Failed to resolve repo root: %v", err)
	}

	docsDir := filepath.Join(repoRoot, "docs")
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		t.Skip("docs directory not found, skipping markdown extraction test")
	}

	knownCommands := map[string]bool{
		"init":              true,
		"add":               true,
		"commit":            true,
		"status":            true,
		"topology":          true,
		"lineage":           true,
		"blast-radius":      true,
		"view":              true,
		"ship":              true,
		"universe":          true,
		"branch":            true,
		"proposal":          true,
		"pr":                true,
		"stack":             true,
		"peer":              true,
		"import":            true,
		"import-repo":       true,
		"onboard":           true,
		"symbol":            true,
		"ast":               true,
		"export":            true,
		"dashboard":         true,
		"tui":               true,
		"git":               true,
		"mcp":               true,
		"lsp":               true,
		"watch":             true,
		"auth":              true,
		"credential-helper": true,
		"share":             true,
		"topocosm":          true,
		"publish":           true,
		"clone":             true,
		"claim":             true,
		"release":           true,
		"blackboard":        true,
		"help":              true,
		"--help":            true,
		"-h":                true,
		"version":           true,
		"--version":         true,
		"-v":                true,
	}

	cosmCmdRegex := regexp.MustCompile(`^\s*cosm\s+([a-z0-9\-]+)`)

	var docFiles []string
	_ = filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") {
			docFiles = append(docFiles, path)
		}
		return nil
	})

	if len(docFiles) == 0 {
		t.Fatalf("No markdown doc files found under %s", docsDir)
	}

	checkedCommands := 0
	for _, docFile := range docFiles {
		f, err := os.Open(docFile)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		inCodeBlock := false
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if inCodeBlock {
				matches := cosmCmdRegex.FindStringSubmatch(line)
				if len(matches) > 1 {
					cmd := matches[1]
					checkedCommands++
					if !knownCommands[cmd] {
						t.Errorf("%s:%d: Documented command 'cosm %s' is not recognized in CLI router", docFile, lineNum, cmd)
					}
				}
			}
		}
		_ = f.Close()
	}

	t.Logf("Validated %d CLI command invocations across %d markdown doc files", checkedCommands, len(docFiles))
}
