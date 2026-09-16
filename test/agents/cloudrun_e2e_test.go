package agents_test

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/topocosm"
	"github.com/cosmscm/cosm/pkg/topocosm/backplane"
	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
	"github.com/cosmscm/cosm/test/agents/rater"
	"github.com/cosmscm/cosm/test/agents/testagent"
)

func TestCloudRun_PolyglotSwarmE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Setup in-process Topocosm Hub server representing the remote Cloud Run Topocosm service
	hubTempDir, err := os.MkdirTemp("", "topocosm-e2e-hub-*")
	if err != nil {
		t.Fatalf("failed to create temp dir for hub: %v", err)
	}
	defer os.RemoveAll(hubTempDir)

	bp, err := backplane.NewLocalBackplane(hubTempDir)
	if err != nil {
		t.Fatalf("failed to init local backplane: %v", err)
	}
	defer bp.Close()

	hubServer := topocosm.NewHubServer(bp)
	ts := httptest.NewServer(hubServer.Handler())
	defer ts.Close()

	hubURL := ts.URL
	os.Setenv("TOPOCOSM_HUB_URL", hubURL)
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	sessionID := "session-cloudrun-mesh-2026"
	os.Setenv("COSM_TRACE_ID", traceID)
	os.Setenv("COSM_SESSION_ID", sessionID)

	// Workspace base directory for all Cloud Run worker jobs
	baseDir, err := os.MkdirTemp("", "cosm-cloudrun-mesh-*")
	if err != nil {
		t.Fatalf("failed to create temp base dir: %v", err)
	}
	defer os.RemoveAll(baseDir)

	allSessions := make([]*framework.AgentSession, 0, 5)

	// -------------------------------------------------------------
	// 2. Agent Zero: Remote Initializer & Bootstrap Job
	// -------------------------------------------------------------
	agentZeroDir := filepath.Join(baseDir, "agent-zero")
	if err := os.MkdirAll(agentZeroDir, 0755); err != nil {
		t.Fatalf("failed creating agent-zero dir: %v", err)
	}
	if err := testagent.WriteScenarioFiles(agentZeroDir, &testagent.ScenarioCloudRunMesh); err != nil {
		t.Fatalf("failed writing scenario files: %v", err)
	}

	mockZero := llm.NewMockProvider("mock-gemini-3.8-flash")
	mockZero.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mockZero.EnqueueToolCall("cosm_add", `{"files":["services/orders/main.go","apps/checkout/CheckoutForm.tsx","workers/fraud/analyzer.py","infra/cloudrun/main.tf"],"intent":"Bootstrap polyglot mesh components"}`)
	mockZero.EnqueueToolCall("cosm_commit", `{"universe_id":"universe-main","intent":"Initial Merkle root commit on universe-main"}`)
	mockZero.EnqueueToolCall("cosm_publish", `{"repo":"cosm/fintech-mesh","universe_id":"universe-main","description":"Initial baseline publication"}`)
	mockZero.EnqueueText("Repository initialized, staged polyglot components, committed to Merkle DAG, and published to Topocosm Hub.")

	agentZero := testagent.NewTestAgent("agent-zero-initializer", mockZero, agentZeroDir, "gemini-3.8-flash")
	sessZero, err := agentZero.RunPrompt(ctx, "Initialize repository at universe-main, stage all 4 polyglot components, commit, and publish to Topocosm Hub.", framework.LoopOptions{MaxSteps: 10})
	if err != nil {
		t.Fatalf("agent-zero execution failed: %v", err)
	}
	if sessZero.Status != framework.StatusSuccess {
		t.Fatalf("expected agent-zero success, got %s", sessZero.Status)
	}
	allSessions = append(allSessions, sessZero)

	// -------------------------------------------------------------
	// 3. Agent Alpha: Backend Orders API Lead (Cloud Run Job 1)
	// -------------------------------------------------------------
	agentAlphaDir := filepath.Join(baseDir, "agent-alpha")
	if err := os.MkdirAll(agentAlphaDir, 0755); err != nil {
		t.Fatalf("failed creating agent-alpha dir: %v", err)
	}

	mockAlpha := llm.NewMockProvider("mock-gemini-3.8-flash")
	mockAlpha.EnqueueToolCall("cosm_claim", `{"repo":"cosm/fintech-mesh","domain":"services/orders","purpose":"V2 checkout idempotency refactor","ttl_sec":300}`)
	mockAlpha.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mockAlpha.EnqueueToolCall("cosm_clone", `{"repo":"cosm/fintech-mesh","universe_id":"universe-main"}`)
	mockAlpha.EnqueueToolCall("cosm_universe_create", `{"universe_id":"u/agent-alpha-orders-v2","parent_id":"universe-main"}`)
	mockAlpha.EnqueueToolCall("cosm_commit", `{"universe_id":"u/agent-alpha-orders-v2","intent":"Implement V2 checkout idempotency checks"}`)
	mockAlpha.EnqueueToolCall("cosm_publish", `{"repo":"cosm/fintech-mesh","universe_id":"u/agent-alpha-orders-v2","description":"Orders V2 checkout idempotency branch"}`)
	mockAlpha.EnqueueToolCall("cosm_stack_create", `{"repo":"cosm/fintech-mesh","title":"Orders V2 Checkout with Idempotency","source_universe":"u/agent-alpha-orders-v2","target_universe":"universe-main"}`)
	mockAlpha.EnqueueToolCall("cosm_claim", `{"repo":"cosm/fintech-mesh","domain":"services/orders","release":true}`)
	mockAlpha.EnqueueText("Claimed services/orders, branched to u/agent-alpha-orders-v2, updated logic, published, created proposal #101, and released domain.")

	agentAlpha := testagent.NewTestAgent("agent-alpha-backend", mockAlpha, agentAlphaDir, "gemini-3.8-flash")
	sessAlpha, err := agentAlpha.RunPrompt(ctx, "Claim services/orders, clone, branch to u/agent-alpha-orders-v2, commit, publish, open proposal, and release claim.", framework.LoopOptions{MaxSteps: 12})
	if err != nil {
		t.Fatalf("agent-alpha execution failed: %v", err)
	}
	if sessAlpha.Status != framework.StatusSuccess {
		t.Fatalf("expected agent-alpha success, got %s", sessAlpha.Status)
	}
	allSessions = append(allSessions, sessAlpha)

	// -------------------------------------------------------------
	// 4. Agent Beta: Frontend Checkout Specialist (Cloud Run Job 2)
	// -------------------------------------------------------------
	agentBetaDir := filepath.Join(baseDir, "agent-beta")
	if err := os.MkdirAll(agentBetaDir, 0755); err != nil {
		t.Fatalf("failed creating agent-beta dir: %v", err)
	}

	mockBeta := llm.NewMockProvider("mock-gemini-3.8-flash")
	mockBeta.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mockBeta.EnqueueToolCall("cosm_clone", `{"repo":"cosm/fintech-mesh","universe_id":"universe-main","components":["apps/checkout"]}`)
	mockBeta.EnqueueToolCall("cosm_universe_create", `{"universe_id":"u/agent-beta-frontend","parent_id":"universe-main"}`)
	mockBeta.EnqueueToolCall("cosm_commit", `{"universe_id":"u/agent-beta-frontend","intent":"Update CheckoutForm to call /api/v2/orders/checkout"}`)
	mockBeta.EnqueueToolCall("cosm_publish", `{"repo":"cosm/fintech-mesh","universe_id":"u/agent-beta-frontend","description":"Frontend V2 checkout integration"}`)
	mockBeta.EnqueueToolCall("cosm_stack_create", `{"repo":"cosm/fintech-mesh","title":"Frontend V2 Checkout Integration","source_universe":"u/agent-beta-frontend","target_universe":"universe-main","parent_proposal_id":"prop-orders-v2"}`)
	mockBeta.EnqueueText("Sparse pulled apps/checkout, branched to u/agent-beta-frontend, committed and stacked proposal on backend proposal.")

	agentBeta := testagent.NewTestAgent("agent-beta-frontend", mockBeta, agentBetaDir, "gemini-3.8-flash")
	sessBeta, err := agentBeta.RunPrompt(ctx, "Sparse pull apps/checkout, branch to u/agent-beta-frontend, commit, publish, and open stacked proposal.", framework.LoopOptions{MaxSteps: 10})
	if err != nil {
		t.Fatalf("agent-beta execution failed: %v", err)
	}
	if sessBeta.Status != framework.StatusSuccess {
		t.Fatalf("expected agent-beta success, got %s", sessBeta.Status)
	}
	allSessions = append(allSessions, sessBeta)

	// -------------------------------------------------------------
	// 5. Agent Gamma: Cloud Infrastructure Engineer (Cloud Run Job 3)
	// -------------------------------------------------------------
	agentGammaDir := filepath.Join(baseDir, "agent-gamma")
	if err := os.MkdirAll(agentGammaDir, 0755); err != nil {
		t.Fatalf("failed creating agent-gamma dir: %v", err)
	}

	mockGamma := llm.NewMockProvider("mock-gemini-3.8-flash")
	mockGamma.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mockGamma.EnqueueToolCall("cosm_clone", `{"repo":"cosm/fintech-mesh","universe_id":"universe-main","components":["infra/cloudrun"]}`)
	mockGamma.EnqueueToolCall("cosm_universe_create", `{"universe_id":"u/agent-gamma-infra","parent_id":"universe-main"}`)
	mockGamma.EnqueueToolCall("cosm_commit", `{"universe_id":"u/agent-gamma-infra","intent":"Add dead letter queue and Cloud Run environment bindings"}`)
	mockGamma.EnqueueToolCall("cosm_publish", `{"repo":"cosm/fintech-mesh","universe_id":"u/agent-gamma-infra","description":"Infrastructure updates for V2"}`)
	mockGamma.EnqueueToolCall("cosm_stack_create", `{"repo":"cosm/fintech-mesh","title":"Cloud Run Infra DLQ and Environment Bindings","source_universe":"u/agent-gamma-infra","target_universe":"universe-main"}`)
	mockGamma.EnqueueText("Sparse pulled infra, updated Terraform HCL, verified, committed, published, and created stacked proposal.")

	agentGamma := testagent.NewTestAgent("agent-gamma-infra", mockGamma, agentGammaDir, "gemini-3.8-flash")
	sessGamma, err := agentGamma.RunPrompt(ctx, "Sparse pull infra/cloudrun, branch to u/agent-gamma-infra, commit, publish, and open proposal.", framework.LoopOptions{MaxSteps: 10})
	if err != nil {
		t.Fatalf("agent-gamma execution failed: %v", err)
	}
	if sessGamma.Status != framework.StatusSuccess {
		t.Fatalf("expected agent-gamma success, got %s", sessGamma.Status)
	}
	allSessions = append(allSessions, sessGamma)

	// -------------------------------------------------------------
	// 6. Agent Delta: Concurrency Stressor (Cloud Run Job 4)
	// -------------------------------------------------------------
	agentDeltaDir := filepath.Join(baseDir, "agent-delta")
	if err := os.MkdirAll(agentDeltaDir, 0755); err != nil {
		t.Fatalf("failed creating agent-delta dir: %v", err)
	}

	mockDelta := llm.NewMockProvider("mock-gemini-3.8-flash")
	mockDelta.EnqueueToolCall("cosm_claim", `{"repo":"cosm/fintech-mesh","domain":"services/orders","purpose":"Contender test","ttl_sec":60}`)
	mockDelta.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mockDelta.EnqueueToolCall("cosm_universe_create", `{"universe_id":"u/agent-delta-contender","parent_id":"universe-main"}`)
	mockDelta.EnqueueToolCall("cosm_claim", `{"repo":"cosm/fintech-mesh","domain":"services/orders","release":true}`)
	mockDelta.EnqueueText("Validated contention handling, micro-universe isolation, and lease release.")

	agentDelta := testagent.NewTestAgent("agent-delta-contender", mockDelta, agentDeltaDir, "gemini-3.8-flash")
	sessDelta, err := agentDelta.RunPrompt(ctx, "Test concurrency lease and micro-universe isolation.", framework.LoopOptions{MaxSteps: 8})
	if err != nil {
		t.Fatalf("agent-delta execution failed: %v", err)
	}
	if sessDelta.Status != framework.StatusSuccess {
		t.Fatalf("expected agent-delta success, got %s", sessDelta.Status)
	}
	allSessions = append(allSessions, sessDelta)

	// -------------------------------------------------------------
	// 7. Rater Judge Audit (Gemini 3.8 Flash)
	// -------------------------------------------------------------
	judgeMock := llm.NewMockProvider(rater.DefaultJudgeModel)
	judgeMock.EnqueueText(`VERDICT: PASSED
CONFIDENCE: HIGH
QUALITY: 24.8 / 25
REASONING: The multi-agent swarm appropriately partitioned tasks across Cloud Run workers using Cosm micro-universes and Topocosm domain leases. Agent Zero initialized the baseline repository and published to Topocosm. Agent Alpha claimed the orders domain, branched cleanly, and opened a proposal before releasing the lease. Agent Beta and Agent Gamma utilized sparse clones and Jujutsu-style stacked proposals. Agent Delta validated concurrency resilience. Zero file write bypasses were detected.
TRACE_AUDIT: Trace ID 4bf92f3577b34da6a3ce929d0e0e4736 correctly propagated across all worker requests.
TOKEN_AUDIT: Token consumption was well within model efficiency thresholds.`)

	judge := rater.NewRaterJudge(judgeMock, rater.DefaultJudgeModel)
	evalReq := &rater.JudgeEvaluationRequest{
		ScenarioName: "cloudrun-polyglot-mesh",
		Sessions:     allSessions,
		HubMetrics: map[string]interface{}{
			"active_leases": 0,
			"proposals":     3,
		},
	}

	report, err := judge.Evaluate(ctx, evalReq)
	if err != nil {
		t.Fatalf("judge evaluation failed: %v", err)
	}

	if report.JudgeModel != rater.DefaultJudgeModel {
		t.Errorf("expected judge model %s, got %s", rater.DefaultJudgeModel, report.JudgeModel)
	}
	if report.Verdict != "PASSED" {
		t.Errorf("expected judge verdict PASSED, got %s", report.Verdict)
	}
	if report.OverallScore < 85.0 {
		t.Errorf("expected overall score >= 85.0, got %f", report.OverallScore)
	}
	if !report.LogAudit.AgentZeroInitVerified {
		t.Errorf("expected AgentZeroInitVerified to be true")
	}
	if !report.LogAudit.AgentAlphaLeaseVerified {
		t.Errorf("expected AgentAlphaLeaseVerified to be true")
	}
	if !report.LogAudit.AgentBetaSparsePullVerified {
		t.Errorf("expected AgentBetaSparsePullVerified to be true")
	}
	if !report.LogAudit.AgentBetaStackedProposalVerified {
		t.Errorf("expected AgentBetaStackedProposalVerified to be true")
	}
	if report.LogAudit.BypassCheatingDetected {
		t.Errorf("flagged bypass cheating detected: %v", report.LogAudit.FlaggedActions)
	}
	if report.LegitimateCosmRatio < 0.8 {
		t.Errorf("expected legitimate Cosm ratio >= 0.8, got %f", report.LegitimateCosmRatio)
	}

	t.Logf("Cloud Run Swarm E2E Completed: Score=%.1f (%s), Verdict=%s, CosmTools=%d/%d, Duration=%dms",
		report.OverallScore, report.LetterGrade, report.Verdict, report.CosmToolCalls, report.TotalToolCalls, report.TotalDurationMs)
}
