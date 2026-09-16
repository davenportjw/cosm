package agents_test

import (
	"context"
	
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/topocosm"
	"github.com/cosmscm/cosm/pkg/topocosm/backplane"
	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
	"github.com/cosmscm/cosm/test/agents/testagent"
)

// TestSwarm_MergedProposals_CompileAndRun verifies that when workers create micro-universes,
// commit polyglot modifications across backend, frontend, and infra, and submit stacked proposals,
// the proposals can be merged into universe-main, and the resulting Cosm repository compiles,
// validates, and runs all target services cleanly.
func TestSwarm_MergedProposals_CompileAndRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 1. Setup in-process Topocosm Hub server
	hubDir, err := os.MkdirTemp("", "topocosm-merge-hub-*")
	if err != nil {
		t.Fatalf("failed creating hub dir: %v", err)
	}
	defer os.RemoveAll(hubDir)

	bp, err := backplane.NewLocalBackplane(hubDir)
	if err != nil {
		t.Fatalf("failed creating backplane: %v", err)
	}
	defer bp.Close()

	hubServer := topocosm.NewHubServer(bp)
	ts := httptest.NewServer(hubServer.Handler())
	defer ts.Close()

	hubURL := ts.URL
	os.Setenv("TOPOCOSM_HUB_URL", hubURL)
	repoName := "cosm/fintech-mesh"

	baseDir, err := os.MkdirTemp("", "cosm-merge-swarm-*")
	if err != nil {
		t.Fatalf("failed creating swarm base dir: %v", err)
	}
	defer os.RemoveAll(baseDir)

	// 2. Task 0: Bootstrap Baseline Polyglot Repository
	task0Dir := filepath.Join(baseDir, "task-0-bootstrap")
	if err := os.MkdirAll(task0Dir, 0755); err != nil {
		t.Fatalf("failed creating task-0 dir: %v", err)
	}
	if err := testagent.WriteScenarioFiles(task0Dir, &testagent.ScenarioCloudRunMesh); err != nil {
		t.Fatalf("failed writing scenario files: %v", err)
	}

	mock0 := llm.NewMockProvider("mock-gemini-3.8-flash")
	mock0.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mock0.EnqueueToolCall("cosm_add", `{"files":["services/orders/main.go","apps/checkout/src/CheckoutForm.tsx","workers/fraud/analyzer.py","infra/cloudrun/main.tf"]}`)
	mock0.EnqueueToolCall("cosm_commit", `{"universe_id":"universe-main","intent":"Baseline polyglot commit"}`)
	mock0.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main"}`, repoName))
	mock0.EnqueueText("Bootstrap complete.")

	agent0 := testagent.NewTestAgent("agent-0", mock0, task0Dir, "gemini-3.8-flash")
	sess0, err := agent0.RunPrompt(ctx, "Bootstrap repo and publish to hub.", framework.LoopOptions{MaxSteps: 6})
	if err != nil || sess0.Status != framework.StatusSuccess {
		t.Fatalf("bootstrap failed: %v, sess=%v", err, sess0)
	}

	// 3. Worker Tasks: Backend, Frontend, and Infrastructure
	// Worker 1: Backend orders enhancement
	task1Dir := filepath.Join(baseDir, "task-1-backend")
	_ = os.MkdirAll(task1Dir, 0755)
	mock1 := llm.NewMockProvider("mock-gemini-3.8-flash")
	mock1.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mock1.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main"}`, repoName))
	mock1.EnqueueToolCall("cosm_universe_create", `{"universe_id":"u/agent-backend-1","parent_id":"universe-main"}`)
	mock1.EnqueueToolCall("cosm_commit", `{"universe_id":"u/agent-backend-1","intent":"Backend idempotency update"}`)
	mock1.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":"u/agent-backend-1"}`, repoName))
	mock1.EnqueueToolCall("cosm_stack_create", fmt.Sprintf(`{"repo":%q,"title":"Orders V2 Backend","source_universe":"u/agent-backend-1","target_universe":"universe-main"}`, repoName))
	mock1.EnqueueText("Backend done.")

	agent1 := testagent.NewTestAgent("agent-1-backend", mock1, task1Dir, "gemini-3.8-flash")
	if _, err := agent1.RunPrompt(ctx, "Work backend", framework.LoopOptions{MaxSteps: 8}); err != nil {
		t.Fatalf("agent 1 failed: %v", err)
	}

	// Worker 2: Frontend enhancement
	task2Dir := filepath.Join(baseDir, "task-2-frontend")
	_ = os.MkdirAll(task2Dir, 0755)
	mock2 := llm.NewMockProvider("mock-gemini-3.8-flash")
	mock2.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mock2.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main","components":["apps/checkout"]}`, repoName))
	mock2.EnqueueToolCall("cosm_universe_create", `{"universe_id":"u/agent-frontend-1","parent_id":"universe-main"}`)
	mock2.EnqueueToolCall("cosm_commit", `{"universe_id":"u/agent-frontend-1","intent":"Frontend React integration"}`)
	mock2.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":"u/agent-frontend-1"}`, repoName))
	mock2.EnqueueToolCall("cosm_stack_create", fmt.Sprintf(`{"repo":%q,"title":"Frontend React Integration","source_universe":"u/agent-frontend-1","target_universe":"universe-main"}`, repoName))
	mock2.EnqueueText("Frontend done.")

	agent2 := testagent.NewTestAgent("agent-2-frontend", mock2, task2Dir, "gemini-3.8-flash")
	if _, err := agent2.RunPrompt(ctx, "Work frontend", framework.LoopOptions{MaxSteps: 8}); err != nil {
		t.Fatalf("agent 2 failed: %v", err)
	}

	// Worker 3: Infra enhancement
	task3Dir := filepath.Join(baseDir, "task-3-infra")
	_ = os.MkdirAll(task3Dir, 0755)
	mock3 := llm.NewMockProvider("mock-gemini-3.8-flash")
	mock3.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mock3.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main","components":["infra/cloudrun"]}`, repoName))
	mock3.EnqueueToolCall("cosm_universe_create", `{"universe_id":"u/agent-infra-1","parent_id":"universe-main"}`)
	mock3.EnqueueToolCall("cosm_commit", `{"universe_id":"u/agent-infra-1","intent":"Terraform DLQ bindings"}`)
	mock3.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":"u/agent-infra-1"}`, repoName))
	mock3.EnqueueToolCall("cosm_stack_create", fmt.Sprintf(`{"repo":%q,"title":"Terraform DLQ Proposal","source_universe":"u/agent-infra-1","target_universe":"universe-main"}`, repoName))
	mock3.EnqueueText("Infra done.")

	agent3 := testagent.NewTestAgent("agent-3-infra", mock3, task3Dir, "gemini-3.8-flash")
	if _, err := agent3.RunPrompt(ctx, "Work infra", framework.LoopOptions{MaxSteps: 8}); err != nil {
		t.Fatalf("agent 3 failed: %v", err)
	}

	// 4. Merge all proposals on Topocosm Hub into universe-main
	hubClient := topocosm.NewHubClient(hubURL, "orchestrator")
	props, err := hubClient.ListProposals(ctx, "cosm", "fintech-mesh")
	if err != nil {
		t.Fatalf("failed listing proposals: %v", err)
	}
	if len(props) != 3 {
		t.Fatalf("expected 3 proposals, got %d", len(props))
	}

	for _, p := range props {
		t.Logf("Merging proposal ID=%s Title=%q Source=%s -> Target=%s", p.ID, p.Title, p.ID, p.TargetUniverse)
		_, err := hubClient.MergeProposal(ctx, "cosm", "fintech-mesh", p.ID)
		if err != nil {
			t.Fatalf("failed merging proposal %s: %v", p.ID, err)
		}
	}

	// 5. Clone universe-main with all merged worker contributions into a clean test dir
	finalDir := filepath.Join(baseDir, "final-merged-workspace")
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		t.Fatalf("failed creating final dir: %v", err)
	}

	mockFinal := llm.NewMockProvider("mock-gemini-3.8-flash")
	mockFinal.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mockFinal.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main"}`, repoName))
	mockFinal.EnqueueText("Cloned merged repository.")

	agentFinal := testagent.NewTestAgent("agent-final", mockFinal, finalDir, "gemini-3.8-flash")
	if _, err := agentFinal.RunPrompt(ctx, "Clone merged repository", framework.LoopOptions{MaxSteps: 4}); err != nil {
		t.Fatalf("final clone failed: %v", err)
	}

	// 6. Verify Compile and Run of the resulting codebase!
	// (a) Go Orders API compiles and runs
	ordersDir := filepath.Join(finalDir, "services", "orders")
	if _, err := os.Stat(filepath.Join(ordersDir, "main.go")); os.IsNotExist(err) {
		t.Fatalf("services/orders/main.go not found in final merged workspace")
	}

	// Add go.mod if not present for standalone compilation
	goModPath := filepath.Join(ordersDir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		_ = os.WriteFile(goModPath, []byte("module services/orders\n\ngo 1.22\n"), 0644)
	}

	ordersBin := filepath.Join(ordersDir, "orders-bin")
	cmdBuild := exec.CommandContext(ctx, "go", "build", "-o", ordersBin, "main.go")
	cmdBuild.Dir = ordersDir
	if out, err := cmdBuild.CombinedOutput(); err != nil {
		t.Fatalf("go build orders failed: %v\nOutput: %s", err, string(out))
	}

	cmdRun := exec.CommandContext(ctx, ordersBin)
	cmdRun.Dir = ordersDir
	cmdRun.Env = append(os.Environ(), "PORT=18080", "PAYMENT_TOPIC_ID=payment-events")
	if err := cmdRun.Start(); err != nil {
		t.Fatalf("failed starting orders binary: %v", err)
	}
	defer func() {
		if cmdRun.Process != nil {
			_ = cmdRun.Process.Kill()
		}
	}()

	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < 25; attempt++ {
		time.Sleep(100 * time.Millisecond)
		resp, lastErr = http.Post("http://127.0.0.1:18080/api/v2/orders/checkout", "application/json", strings.NewReader(`{"order_id":"ord-123","amount":99.50}`))
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		t.Fatalf("failed pinging orders HTTP server: %v", lastErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 from orders API, got %d", resp.StatusCode)
	}
	t.Logf("Orders API responded with HTTP %d", resp.StatusCode)

	// (b) Python Fraud Worker executes with uv
	fraudScript := filepath.Join(finalDir, "workers", "fraud", "analyzer.py")
	cmdPy := exec.CommandContext(ctx, "uv", "run", "python", fraudScript)
	cmdPy.Dir = finalDir
	pyOut, err := cmdPy.CombinedOutput()
	if err != nil {
		t.Fatalf("uv run python analyzer.py failed: %v\nOutput: %s", err, string(pyOut))
	}
	t.Logf("Python analyzer executed cleanly: %s", string(pyOut))

	// (c) Terraform HCL formatting and validation
	infraDir := filepath.Join(finalDir, "infra", "cloudrun")
	cmdFmt := exec.CommandContext(ctx, "terraform", "fmt", "-check")
	cmdFmt.Dir = infraDir
	if out, err := cmdFmt.CombinedOutput(); err != nil {
		t.Fatalf("terraform fmt -check failed: %v\nOutput: %s", err, string(out))
	}

	cmdInit := exec.CommandContext(ctx, "terraform", "init", "-backend=false")
	cmdInit.Dir = infraDir
	if out, err := cmdInit.CombinedOutput(); err != nil {
		t.Fatalf("terraform init failed: %v\nOutput: %s", err, string(out))
	}

	cmdVal := exec.CommandContext(ctx, "terraform", "validate")
	cmdVal.Dir = infraDir
	valOut, err := cmdVal.CombinedOutput()
	if err != nil {
		t.Fatalf("terraform validate failed: %v\nOutput: %s", err, string(valOut))
	}
	t.Logf("Terraform validation passed: %s", string(valOut))

	t.Logf("ALL polyglot components with merged worker proposals compiled, ran, and validated successfully!")
}
