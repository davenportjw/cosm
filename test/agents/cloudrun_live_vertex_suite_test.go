package agents_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/topocosm"
	"github.com/cosmscm/cosm/pkg/topocosm/backplane"
	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
	"github.com/cosmscm/cosm/test/agents/rater"
	"github.com/cosmscm/cosm/test/agents/testagent"
)

// TestLiveVertex_100Tasks_JudgeSuite executes the 100-task Cosm/Topocosm concurrency suite
// and verifies the entire multi-agent swarm run using an unmocked, live Gemini 3.8 Flash
// rater judge on Vertex AI in the us multi-region, authenticated via Google Cloud Project Auth.
func TestLiveVertex_100Tasks_JudgeSuite(t *testing.T) {
	out, err := exec.Command("gcloud", "auth", "print-access-token").Output()
	if err != nil || len(out) == 0 {
		t.Skip("Skipping live Vertex AI suite: gcloud auth not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	projectID := os.Getenv("VERTEX_PROJECT")
	if projectID == "" {
		projectID = "davenport-boutique"
	}
	location := os.Getenv("VERTEX_LOCATION")
	if location == "" {
		location = "us" // Gemini 3.8 Flash multi-region location
	}
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-3.8-flash"
	}

	t.Logf("Initializing Live Vertex AI Gemini Provider (project=%s, location=%s, model=%s)", projectID, location, model)
	liveJudgeProvider, err := llm.NewGeminiProvider(llm.GeminiConfig{
		ProjectID: projectID,
		Location:  location,
		Model:     model,
	})
	if err != nil {
		t.Fatalf("failed initializing live Gemini provider: %v", err)
	}

	// 1. Setup in-process Topocosm Hub server for the 100-task concurrency test
	hubTempDir, err := os.MkdirTemp("", "topocosm-hub-live-100-*")
	if err != nil {
		t.Fatalf("failed creating hub temp dir: %v", err)
	}
	defer os.RemoveAll(hubTempDir)

	bp, err := backplane.NewLocalBackplane(hubTempDir)
	if err != nil {
		t.Fatalf("failed initializing backplane: %v", err)
	}
	defer bp.Close()

	hubServer := topocosm.NewHubServer(bp)
	ts := httptest.NewServer(hubServer.Handler())
	defer ts.Close()

	hubURL := ts.URL
	os.Setenv("TOPOCOSM_HUB_URL", hubURL)
	traceID := fmt.Sprintf("trace-live-vertex-100-%d", time.Now().UnixNano())
	os.Setenv("COSM_TRACE_ID", traceID)

	baseDir, err := os.MkdirTemp("", "cosm-live-100-mesh-*")
	if err != nil {
		t.Fatalf("failed creating workspace base dir: %v", err)
	}
	defer os.RemoveAll(baseDir)

	const totalTasks = 100
	const repoName = "cosm/fintech-mesh"

	// 2. Task 0: Bootstrap Repo & Baseline AST
	agentZeroDir := filepath.Join(baseDir, "task-0-bootstrap")
	if err := os.MkdirAll(agentZeroDir, 0755); err != nil {
		t.Fatalf("failed creating task-0 dir: %v", err)
	}
	if err := testagent.WriteScenarioFiles(agentZeroDir, &testagent.ScenarioCloudRunMesh); err != nil {
		t.Fatalf("failed writing scenario files: %v", err)
	}

	mockZero := llm.NewMockProvider(model)
	mockZero.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mockZero.EnqueueToolCall("cosm_add", `{"files":["services/orders/main.go","apps/checkout/CheckoutForm.tsx","workers/fraud/analyzer.py","infra/cloudrun/main.tf"],"intent":"Bootstrap polyglot mesh components"}`)
	mockZero.EnqueueToolCall("cosm_commit", `{"universe_id":"universe-main","intent":"Initial Merkle root commit on universe-main"}`)
	mockZero.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main","description":"Initial baseline publication"}`, repoName))
	mockZero.EnqueueText("Repository initialized, staged polyglot components, committed to Merkle DAG, and published to Topocosm Hub.")

	agentZero := testagent.NewTestAgent("agent-bootstrap-0", mockZero, agentZeroDir, model)
	sessZero, err := agentZero.RunPrompt(ctx, "Initialize repository at universe-main, stage polyglot components, commit, and publish to Topocosm Hub.", framework.LoopOptions{MaxSteps: 10})
	if err != nil {
		t.Fatalf("task-0 bootstrap execution failed: %v", err)
	}
	if sessZero.Status != framework.StatusSuccess {
		t.Fatalf("expected task-0 success, got %s", sessZero.Status)
	}

	var (
		sessionsMu  sync.Mutex
		allSessions = make([]*framework.AgentSession, 0, totalTasks)
	)
	allSessions = append(allSessions, sessZero)

	// 3. Dispatch Tasks 1..99 concurrently
	var wg sync.WaitGroup
	errCh := make(chan error, totalTasks)

	for i := 1; i < totalTasks; i++ {
		wg.Add(1)
		taskIdx := i

		go func(idx int) {
			defer wg.Done()

			taskDir := filepath.Join(baseDir, fmt.Sprintf("task-%d", idx))
			if err := os.MkdirAll(taskDir, 0755); err != nil {
				errCh <- fmt.Errorf("task %d failed mkdir: %w", idx, err)
				return
			}

			var role string
			switch {
			case idx >= 1 && idx <= 20:
				role = "backend"
			case idx >= 21 && idx <= 40:
				role = "frontend"
			case idx >= 41 && idx <= 60:
				role = "infra"
			case idx >= 61 && idx <= 80:
				role = "contender"
			case idx >= 81 && idx <= 99:
				role = "observer"
			}

			domainIdx := idx % 4
			domain := fmt.Sprintf("services/orders-%d", domainIdx)
			uID := fmt.Sprintf("u/agent-%s-%d", role, idx)
			agentName := fmt.Sprintf("agent-%s-%d", role, idx)

			mock := llm.NewMockProvider(model)
			prompt := ""

			switch role {
			case "backend":
				mock.EnqueueToolCall("cosm_claim", fmt.Sprintf(`{"repo":%q,"domain":%q,"purpose":"V2 Checkout Batch %d","ttl_sec":300}`, repoName, domain, idx))
				mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
				mock.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main"}`, repoName))
				mock.EnqueueToolCall("cosm_universe_create", fmt.Sprintf(`{"universe_id":%q,"parent_id":"universe-main"}`, uID))
				mock.EnqueueToolCall("cosm_commit", fmt.Sprintf(`{"universe_id":%q,"intent":"V2 checkout idempotency batch %d"}`, uID, idx))
				mock.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":%q}`, repoName, uID))
				mock.EnqueueToolCall("cosm_stack_create", fmt.Sprintf(`{"repo":%q,"title":"Orders V2 Checkout Batch %d","source_universe":%q,"target_universe":"universe-main"}`, repoName, idx, uID))
				mock.EnqueueToolCall("cosm_claim", fmt.Sprintf(`{"repo":%q,"domain":%q,"release":true}`, repoName, domain))
				mock.EnqueueText("Backend changes committed, published, and proposal opened.")
				prompt = fmt.Sprintf("Claim %s, clone, branch to %s, commit, publish, open proposal, and release claim.", domain, uID)

			case "frontend":
				mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
				mock.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main","components":["apps/checkout"]}`, repoName))
				mock.EnqueueToolCall("cosm_universe_create", fmt.Sprintf(`{"universe_id":%q,"parent_id":"universe-main"}`, uID))
				mock.EnqueueToolCall("cosm_commit", fmt.Sprintf(`{"universe_id":%q,"intent":"Update CheckoutForm %d"}`, uID, idx))
				mock.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":%q}`, repoName, uID))
				mock.EnqueueToolCall("cosm_stack_create", fmt.Sprintf(`{"repo":%q,"title":"Frontend V2 Integration %d","source_universe":%q,"target_universe":"universe-main"}`, repoName, idx, uID))
				mock.EnqueueText("Frontend changes committed, published, and proposal stacked.")
				prompt = fmt.Sprintf("Sparse pull apps/checkout, branch to %s, commit, publish, and open stacked proposal.", uID)

			case "infra":
				mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
				mock.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main","components":["infra/cloudrun"]}`, repoName))
				mock.EnqueueToolCall("cosm_universe_create", fmt.Sprintf(`{"universe_id":%q,"parent_id":"universe-main"}`, uID))
				mock.EnqueueToolCall("cosm_commit", fmt.Sprintf(`{"universe_id":%q,"intent":"Terraform DLQ bindings %d"}`, uID, idx))
				mock.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":%q}`, repoName, uID))
				mock.EnqueueToolCall("cosm_stack_create", fmt.Sprintf(`{"repo":%q,"title":"Terraform DLQ Proposal %d","source_universe":%q,"target_universe":"universe-main"}`, repoName, idx, uID))
				mock.EnqueueText("Infrastructure changes validated, committed, and proposal opened.")
				prompt = fmt.Sprintf("Sparse pull infra/cloudrun, branch to %s, validate Terraform, commit, publish, and open stacked proposal.", uID)

			case "contender":
				mock.EnqueueToolCall("cosm_claim", fmt.Sprintf(`{"repo":%q,"domain":%q,"purpose":"Contender lease stress %d"}`, repoName, domain, idx))
				mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
				mock.EnqueueToolCall("cosm_universe_create", fmt.Sprintf(`{"universe_id":%q,"parent_id":"universe-main"}`, uID))
				mock.EnqueueToolCall("cosm_claim", fmt.Sprintf(`{"repo":%q,"domain":%q,"release":true}`, repoName, domain))
				mock.EnqueueText("Contender test validated.")
				prompt = fmt.Sprintf("Validate concurrency domain leases on %s under contention.", domain)

			case "observer":
				mock.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
				mock.EnqueueToolCall("cosm_clone", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main"}`, repoName))
				mock.EnqueueText("Observer topology inspection complete.")
				prompt = "Query proposals and inspect Merkle DAG topology across the mesh."
			}

			agent := testagent.NewTestAgent(agentName, mock, taskDir, model)
			sess, err := agent.RunPrompt(ctx, prompt, framework.LoopOptions{MaxSteps: 12})
			if err != nil {
				errCh <- fmt.Errorf("task %d (%s) failed: %w", idx, role, err)
				return
			}
			if sess.Status != framework.StatusSuccess {
				errCh <- fmt.Errorf("task %d (%s) expected success, got %s", idx, role, sess.Status)
				return
			}

			sessionsMu.Lock()
			allSessions = append(allSessions, sess)
			sessionsMu.Unlock()
		}(taskIdx)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent task execution failure: %v", err)
	}

	if len(allSessions) != totalTasks {
		t.Fatalf("expected %d completed sessions, got %d", totalTasks, len(allSessions))
	}

	// 4. Live Gemini 3.8 Flash Rater Judge Evaluation
	t.Logf("Dispatching 100-session audit to Live Vertex AI Gemini 3.8 Flash Judge (traceID=%s)...", traceID)
	judge := rater.NewRaterJudge(liveJudgeProvider, model)
	report, err := judge.Evaluate(ctx, &rater.JudgeEvaluationRequest{
		ScenarioName: "cloudrun-live-vertex-100",
		Sessions:     allSessions,
	})
	if err != nil {
		t.Fatalf("live Gemini 3.8 Flash judge evaluation failed: %v", err)
	}

	t.Logf("=== Live Gemini 3.8 Flash Scorecard ===")
	t.Logf("Judge Model: %s", report.JudgeModel)
	t.Logf("Overall Score: %0.1f/100 (Grade: %s, Verdict: %s)", report.OverallScore, report.LetterGrade, report.Verdict)
	t.Logf("Cosm Utilization: %0.1f/25", report.CosmUtilizationScore)
	t.Logf("Time Efficiency:  %0.1f/20", report.TimeEfficiencyScore)
	t.Logf("Token Economics:  %0.1f/20", report.TokenEconomicsScore)
	t.Logf("Output Quality:   %0.1f/35", report.OutputQualityScore)
	t.Logf("Total Tool Calls: %d (Cosm: %d, Ratio: %0.2f)", report.TotalToolCalls, report.CosmToolCalls, report.LegitimateCosmRatio)
	t.Logf("Live Qualitative Critique:\n%s", report.Critique)

	// Assertions
	if report.OverallScore < 90.0 {
		t.Errorf("expected score >= 90.0, got %0.1f", report.OverallScore)
	}
	if report.Verdict != "PASSED" {
		t.Errorf("expected verdict PASSED, got %s", report.Verdict)
	}
	if report.Critique == "" {
		t.Errorf("expected non-empty live critique from Gemini 3.8 Flash")
	}
	if strings.Contains(report.Critique, "mock") {
		t.Errorf("critique contains 'mock', expected real evaluation")
	}
	if report.LogAudit.BypassCheatingDetected {
		t.Errorf("expected BypassCheatingDetected=false")
	}
}
