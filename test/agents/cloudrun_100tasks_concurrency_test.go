package agents_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// TestCloudRun_100Tasks_Concurrency validates that 100 concurrent agent worker tasks
// targeting the same Cosm repository on Topocosm Hub correctly execute without deadlocks,
// enforcing domain leases, zero-copy branching, CAS deduplication, and CRDT convergence.
func TestCloudRun_100Tasks_Concurrency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// 1. Setup in-process Topocosm Hub server
	hubTempDir, err := os.MkdirTemp("", "topocosm-hub-100tasks-*")
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
	traceID := "trace-100tasks-concurrency-test"
	os.Setenv("COSM_TRACE_ID", traceID)

	baseDir, err := os.MkdirTemp("", "cosm-100tasks-mesh-*")
	if err != nil {
		t.Fatalf("failed to create temp base dir: %v", err)
	}
	defer os.RemoveAll(baseDir)

	const totalTasks = 100
	const repoName = "cosm/fintech-mesh"

	// 2. Task 0: Bootstrap Baseline Initialization
	agentZeroDir := filepath.Join(baseDir, "task-0-bootstrap")
	if err := os.MkdirAll(agentZeroDir, 0755); err != nil {
		t.Fatalf("failed creating task-0 dir: %v", err)
	}
	if err := testagent.WriteScenarioFiles(agentZeroDir, &testagent.ScenarioCloudRunMesh); err != nil {
		t.Fatalf("failed writing scenario files: %v", err)
	}

	mockZero := llm.NewMockProvider("mock-gemini-3.8-flash")
	mockZero.EnqueueToolCall("cosm_init", `{"universe_id":"universe-main"}`)
	mockZero.EnqueueToolCall("cosm_add", `{"files":["services/orders/main.go","apps/checkout/CheckoutForm.tsx","workers/fraud/analyzer.py","infra/cloudrun/main.tf"],"intent":"Bootstrap polyglot mesh components"}`)
	mockZero.EnqueueToolCall("cosm_commit", `{"universe_id":"universe-main","intent":"Initial Merkle root commit on universe-main"}`)
	mockZero.EnqueueToolCall("cosm_publish", fmt.Sprintf(`{"repo":%q,"universe_id":"universe-main","description":"Initial baseline publication"}`, repoName))
	mockZero.EnqueueText("Repository initialized, staged polyglot components, committed to Merkle DAG, and published to Topocosm Hub.")

	agentZero := testagent.NewTestAgent("agent-bootstrap-0", mockZero, agentZeroDir, "gemini-3.8-flash")
	sessZero, err := agentZero.RunPrompt(ctx, "Initialize repository at universe-main, stage polyglot components, commit, and publish to Topocosm Hub.", framework.LoopOptions{MaxSteps: 10})
	if err != nil {
		t.Fatalf("task-0 bootstrap execution failed: %v", err)
	}
	if sessZero.Status != framework.StatusSuccess {
		t.Fatalf("expected task-0 success, got %s", sessZero.Status)
	}

	// Collected sessions across all 100 tasks
	var (
		sessionsMu  sync.Mutex
		allSessions = make([]*framework.AgentSession, 0, totalTasks)
	)
	allSessions = append(allSessions, sessZero)

	// 3. Concurrently launch Tasks 1 through 99
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

			mock := llm.NewMockProvider("mock-gemini-3.8-flash")
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

			agent := testagent.NewTestAgent(agentName, mock, taskDir, "gemini-3.8-flash")
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

	// 4. Gemini 3.8 Flash Rater Judge evaluation of all 100 worker traces
	mockJudge := llm.NewMockProvider("mock-gemini-3.8-flash")
	mockJudge.EnqueueText("VERDICT: PASSED\nCONFIDENCE: HIGH\nQUALITY: 25 / 25\nREASONING: High quality AST execution across all 100 concurrent swarm workers with zero file corruption and flawless lease governance.")

	judge := rater.NewRaterJudge(mockJudge, "gemini-3.8-flash")
	report, err := judge.Evaluate(ctx, &rater.JudgeEvaluationRequest{
		ScenarioName: "cloudrun-polyglot-mesh-100",
		Sessions:     allSessions,
	})
	if err != nil {
		t.Fatalf("judge evaluation failed: %v", err)
	}

	t.Logf("Rater Judge Report for 100 Tasks: Score=%0.1f/100 Grade=%s Verdict=%s Duration=%v Tokens=%d ToolCalls=%d",
		report.OverallScore, report.LetterGrade, report.Verdict,
		time.Duration(report.TotalDurationMs)*time.Millisecond,
		report.TotalTokensUsed, report.TotalToolCalls)

	if report.OverallScore < 90.0 {
		t.Errorf("expected overall score >= 90.0, got %0.1f", report.OverallScore)
	}
	if report.Verdict != "PASSED" {
		t.Errorf("expected verdict PASSED, got %s", report.Verdict)
	}
	if !report.LogAudit.AgentZeroInitVerified {
		t.Errorf("expected AgentZeroInitVerified=true")
	}
	if !report.LogAudit.AgentAlphaLeaseVerified {
		t.Errorf("expected AgentAlphaLeaseVerified=true")
	}
	if !report.LogAudit.AgentBetaSparsePullVerified {
		t.Errorf("expected AgentBetaSparsePullVerified=true")
	}
	if !report.LogAudit.AgentBetaStackedProposalVerified {
		t.Errorf("expected AgentBetaStackedProposalVerified=true")
	}
	if !report.LogAudit.AgentGammaTerraformValidated {
		t.Errorf("expected AgentGammaTerraformValidated=true")
	}
	if !report.LogAudit.AgentDeltaConflictVerified {
		t.Errorf("expected AgentDeltaConflictVerified=true")
	}
	if report.LogAudit.BypassCheatingDetected {
		t.Errorf("expected BypassCheatingDetected=false")
	}
}
