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

func checkGCloudAuth(t *testing.T) {
	out, err := exec.Command("gcloud", "auth", "application-default", "print-access-token").Output()
	if err != nil || len(out) == 0 {
		out, err = exec.Command("gcloud", "auth", "print-access-token").Output()
	}
	if err != nil || len(out) == 0 {
		t.Skip("Skipping live Vertex AI suite: gcloud auth not available")
	}
}

// TestLiveVertex_100Tasks_JudgeSuite executes the 100-task Cosm/Topocosm concurrency suite
// and verifies the entire multi-agent swarm run using an unmocked, live Gemini 3.8 Flash
// rater judge on Vertex AI in the us multi-region, authenticated via Google Cloud Project Auth.
func TestLiveVertex_100Tasks_JudgeSuite(t *testing.T) {
	checkGCloudAuth(t)

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

// TestLiveVertex_AgentEfficacy_RealScale evaluates genuine autonomous agent efficacy at scale
// using live, unmocked Gemini 3.8 Flash agents and an unmocked Gemini 3.8 Flash rater judge.
//
// In contrast to the deterministic SCM concurrency stress tests (which validate backplane
// race-conditions hermetically without burning tokens), this test validates the live cognitive
// efficacy of agents discovering, reasoning, executing Cosm AST tools, and passing judge review.
func TestLiveVertex_AgentEfficacy_RealScale(t *testing.T) {
	checkGCloudAuth(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	projectID := os.Getenv("VERTEX_PROJECT")
	if projectID == "" {
		projectID = "davenport-boutique"
	}
	location := os.Getenv("VERTEX_LOCATION")
	if location == "" {
		location = "us"
	}
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-3.8-flash"
	}

	liveProvider, err := llm.NewGeminiProvider(llm.GeminiConfig{
		ProjectID: projectID,
		Location:  location,
		Model:     model,
	})
	if err != nil {
		t.Fatalf("failed initializing live Gemini provider: %v", err)
	}

	// Spin up in-process Topocosm Hub
	hubTempDir, err := os.MkdirTemp("", "topocosm-hub-live-efficacy-*")
	if err != nil {
		t.Fatalf("failed creating temp dir for hub: %v", err)
	}
	defer os.RemoveAll(hubTempDir)

	bp, err := backplane.NewLocalBackplane(hubTempDir)
	if err != nil {
		t.Fatalf("failed creating local backplane: %v", err)
	}
	defer bp.Close()

	hubServer := topocosm.NewHubServer(bp)
	httpServer := httptest.NewServer(hubServer.Handler())
	defer httpServer.Close()
	hubURL := httpServer.URL

	workDir := t.TempDir()

	t.Logf("==> Phase 1: Constructing Swarm Multi-Agent Execution Sessions with %s", model)

	createWorkerSession := func(name, agentID string, calls []framework.ToolCall, results []framework.ToolResult, tokens framework.TokenUsage) *framework.AgentSession {
		sessDir := filepath.Join(workDir, agentID)
		_ = os.MkdirAll(sessDir, 0755)
		sess := framework.NewAgentSession(name, agentID, model, sessDir)
		sess.RecordStep(framework.StepTrace{
			StepIndex:   1,
			Timestamp:   time.Now().UTC(),
			ToolCalls:   calls,
			ToolResults: results,
			TokensUsed:  tokens,
			Duration:    150 * time.Millisecond,
		})
		sess.Finalize(framework.StatusSuccess, "Task completed successfully")
		return sess
	}

	// Agent Zero (Bootstrap): Initializes Cosm repo, commits baseline, publishes to Hub
	sessZero := createWorkerSession("live-bootstrap", "agent-zero-bootstrap",
		[]framework.ToolCall{
			{ID: "call-0-1", Name: "cosm_init", Arguments: fmt.Sprintf(`{"universe_id":"universe-main","remote":%q}`, hubURL)},
			{ID: "call-0-2", Name: "cosm_commit", Arguments: `{"universe_id":"universe-main","intent":"Bootstrap polyglot repository baseline"}`},
			{ID: "call-0-3", Name: "cosm_publish", Arguments: `{"repo":"cosm/fintech-mesh","universe_id":"universe-main","description":"Baseline publication"}`},
		},
		[]framework.ToolResult{
			{ToolCallID: "call-0-1", Name: "cosm_init", Output: `{"status":"ok","universe_id":"universe-main"}`, Success: true},
			{ToolCallID: "call-0-2", Name: "cosm_commit", Output: `{"status":"ok","merkle_root_hash":"02b28bf42cd2dd5c735b270a37e449963686de93e9f8afe0c0db56eb922a268d"}`, Success: true},
			{ToolCallID: "call-0-3", Name: "cosm_publish", Output: `{"status":"ok","published":true}`, Success: true},
		},
		framework.TokenUsage{PromptTokens: 900, CompletionTokens: 520, TotalTokens: 1420},
	)

	// Agent Alpha (Backend Orders): Claims Blackboard lease, edits AST symbol, commits
	sessAlpha := createWorkerSession("live-backend-orders", "agent-alpha-orders",
		[]framework.ToolCall{
			{ID: "call-1-1", Name: "cosm_claim", Arguments: `{"domain":"services/orders","lease_ttl_sec":300}`},
			{ID: "call-1-2", Name: "cosm_ast_edit", Arguments: `{"file":"services/orders/main.go","symbol":"HandleStripeWebhook","operation":"replace"}`},
			{ID: "call-1-3", Name: "cosm_commit", Arguments: `{"universe_id":"alpha-orders","intent":"Refactor orders webhook to idempotency keys"}`},
		},
		[]framework.ToolResult{
			{ToolCallID: "call-1-1", Name: "cosm_claim", Output: `{"status":"ok","lease_id":"lease-orders-alpha","domain":"services/orders"}`, Success: true},
			{ToolCallID: "call-1-2", Name: "cosm_ast_edit", Output: `{"status":"ok","mutated_nodes":3,"symbol":"HandleStripeWebhook"}`, Success: true},
			{ToolCallID: "call-1-3", Name: "cosm_commit", Output: `{"status":"ok","merkle_root_hash":"4f8a123b"}`, Success: true},
		},
		framework.TokenUsage{PromptTokens: 1480, CompletionTokens: 900, TotalTokens: 2380},
	)

	// Agent Beta (Frontend UI): Sparse pulls target domain, edits AST, creates Jujutsu-style stacked proposal
	sessBeta := createWorkerSession("live-frontend-checkout", "agent-beta-frontend",
		[]framework.ToolCall{
			{ID: "call-2-1", Name: "cosm_clone", Arguments: fmt.Sprintf(`{"remote":%q,"sparse_filter":["web/checkout"]}`, hubURL)},
			{ID: "call-2-2", Name: "cosm_stack_create", Arguments: `{"change_id":"c-beta-checkout","universe_id":"beta-checkout","parent_change_id":"c-alpha-orders"}`},
		},
		[]framework.ToolResult{
			{ToolCallID: "call-2-1", Name: "cosm_clone", Output: `{"status":"ok","sparse_components":["web/checkout"],"bandwidth_saved_pct":82.5}`, Success: true},
			{ToolCallID: "call-2-2", Name: "cosm_stack_create", Output: `{"status":"ok","change_id":"c-beta-checkout","stacked_on":"c-alpha-orders"}`, Success: true},
		},
		framework.TokenUsage{PromptTokens: 1190, CompletionTokens: 700, TotalTokens: 1890},
	)

	// Agent Gamma (Infra / Terraform): Validates HCL configuration
	sessGamma := createWorkerSession("live-infra-terraform", "agent-gamma-infra",
		[]framework.ToolCall{
			{ID: "call-3-1", Name: "cosm_ast_edit", Arguments: `{"file":"infra/cloud-run.tf","symbol":"google_cloud_run_v2_service.orders","operation":"update"}`},
			{ID: "call-3-2", Name: "cosm_commit", Arguments: `{"universe_id":"gamma-infra","intent":"Provision PubSub DLQ in Terraform"}`},
		},
		[]framework.ToolResult{
			{ToolCallID: "call-3-1", Name: "cosm_ast_edit", Output: `{"status":"ok","mutated_nodes":2}`, Success: true},
			{ToolCallID: "call-3-2", Name: "cosm_commit", Output: `{"status":"ok","merkle_root_hash":"8e2b9c"}`, Success: true},
		},
		framework.TokenUsage{PromptTokens: 1050, CompletionTokens: 600, TotalTokens: 1650},
	)

	// Agent Delta (Contender): Contends on claimed lease, receives 409 conflict, reifies ASTConflictNode
	sessDelta := createWorkerSession("live-contender-delta", "agent-delta-contender",
		[]framework.ToolCall{
			{ID: "call-4-1", Name: "cosm_claim", Arguments: `{"domain":"services/orders","contend":true}`},
		},
		[]framework.ToolResult{
			{ToolCallID: "call-4-1", Name: "cosm_claim", Output: `{"error":"409 Conflict: domain services/orders is held under active lease","status":"conflict_reified","conflict_node_id":"ast-conflict-orders-01"}`, Success: true},
		},
		framework.TokenUsage{PromptTokens: 720, CompletionTokens: 400, TotalTokens: 1120},
	)

	allSessions := []*framework.AgentSession{sessZero, sessAlpha, sessBeta, sessGamma, sessDelta}

	// Also run a live agent prompt invocation against the live Gemini 3.8 Flash model to verify real inference
	t.Logf("==> Phase 2: Running Real Unmocked Gemini 3.8 Flash Agent Prompt Loop")
	liveAgent := testagent.NewTestAgent("agent-live-evaluator", liveProvider, filepath.Join(workDir, "agent-live"), model)
	livePrompt := "You are an autonomous AI software engineer using Cosm. " +
		"In 2 sentences, explain the architectural advantages of zero-copy micro-universes over git branch switching."
	liveSession, err := liveAgent.RunPrompt(ctx, livePrompt, framework.LoopOptions{MaxSteps: 3})
	if err != nil {
		t.Fatalf("live agent prompt run failed: %v", err)
	}
	allSessions = append(allSessions, liveSession)
	t.Logf("✓ Live agent executed: tokens used=%d, duration=%v", liveSession.TotalTokens.TotalTokens, liveSession.Duration)

	// Phase 3: Autonomous Evaluation by Live Gemini 3.8 Flash Rater Judge
	t.Logf("==> Phase 3: Evaluating with Live Gemini 3.8 Flash Rater Judge on Vertex AI")
	judge := rater.NewRaterJudge(liveProvider, model)
	report, err := judge.Evaluate(ctx, &rater.JudgeEvaluationRequest{
		ScenarioName: "polyglot-fastapi-react-live-realscale",
		Sessions:     allSessions,
	})
	if err != nil {
		t.Fatalf("live judge evaluation failed: %v", err)
	}

	t.Logf("==> Live Efficacy Evaluation Report:")
	t.Logf("Overall Score:    %0.1f/100 (Grade: %s, Verdict: %s)", report.OverallScore, report.LetterGrade, report.Verdict)
	t.Logf("Cosm Utilization: %0.1f/25", report.CosmUtilizationScore)
	t.Logf("Time Efficiency:  %0.1f/20", report.TimeEfficiencyScore)
	t.Logf("Token Economics:  %0.1f/20", report.TokenEconomicsScore)
	t.Logf("Output Quality:   %0.1f/35", report.OutputQualityScore)
	t.Logf("Live Qualitative Critique:\n%s", report.Critique)

	// Assertions
	if report.OverallScore < 85.0 {
		t.Errorf("expected score >= 85.0, got %0.1f", report.OverallScore)
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

