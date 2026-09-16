package rater

import (
	"context"
	"testing"
	"time"

	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
)

func TestJudgeEngine_EvaluateSuccess(t *testing.T) {
	ctx := context.Background()

	mockLLM := llm.NewMockProvider(DefaultJudgeModel)
	judgeResp := `VERDICT: PASSED
CONFIDENCE: HIGH
QUALITY: 24.5 / 25
REASONING: The multi-agent swarm appropriately partitioned tasks using Cosm micro-universes and Topocosm domain leases. Agent Zero initialized the baseline, Agent Alpha secured the auth domain, and subsequent agents used Jujutsu-style stacked proposals without file write bypasses.
TRACE_AUDIT: Clean trace ID propagation across all Cloud Run tasks.
TOKEN_AUDIT: Total tokens consumed were within acceptable efficiency margins.`

	mockLLM.EnqueueText(judgeResp)

	judge := NewRaterJudge(mockLLM, DefaultJudgeModel)

	sessZero := &framework.AgentSession{
		SessionID:    "session-zero-101",
		ScenarioName: "ScenarioCloudRunMesh",
		AgentID:      "agent-zero-initializer",
		Duration:     10 * time.Second,
		TotalTokens: llm.TokenUsage{
			PromptTokens:     3000,
			CompletionTokens: 500,
			TotalTokens:      3500,
		},
		Traces: []framework.StepTrace{
			{
				StepIndex: 1,
				ToolCalls: []llm.ToolCall{
					{Name: "cosm_init", Arguments: `{"universe_id":"universe-main"}`},
					{Name: "cosm_publish", Arguments: `{"repo":"cosmscm/swarm-preview"}`},
				},
			},
		},
	}

	sessAlpha := &framework.AgentSession{
		SessionID:    "session-alpha-101",
		ScenarioName: "ScenarioCloudRunMesh",
		AgentID:      "agent-alpha-auth",
		Duration:     12 * time.Second,
		TotalTokens: llm.TokenUsage{
			PromptTokens:     4000,
			CompletionTokens: 800,
			TotalTokens:      4800,
		},
		Traces: []framework.StepTrace{
			{
				StepIndex: 1,
				ToolCalls: []llm.ToolCall{
					{Name: "cosm_claim", Arguments: `{"repo":"cosmscm/swarm-preview","domain":"auth"}`},
					{Name: "cosm_ast_edit", Arguments: `{"universe_id":"universe-auth","target":"AuthenticateUser"}`},
					{Name: "cosm_stack_create", Arguments: `{"repo":"cosmscm/swarm-preview","title":"Add auth"}`},
				},
			},
		},
	}

	req := &JudgeEvaluationRequest{
		ScenarioName: "ScenarioCloudRunMesh",
		Sessions:     []*framework.AgentSession{sessZero, sessAlpha},
		HubMetrics: map[string]interface{}{
			"active_leases": 1,
		},
	}

	report, err := judge.Evaluate(ctx, req)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}

	if report.JudgeModel != DefaultJudgeModel {
		t.Errorf("expected model %s, got %s", DefaultJudgeModel, report.JudgeModel)
	}
	if report.OverallScore < 85.0 {
		t.Errorf("expected score >= 85.0, got %f", report.OverallScore)
	}
	if report.Verdict != "PASSED" {
		t.Errorf("expected verdict PASSED, got %s", report.Verdict)
	}
	if !report.LogAudit.AgentZeroInitVerified {
		t.Errorf("expected AgentZeroInitVerified to be true")
	}
	if !report.LogAudit.AgentAlphaLeaseVerified {
		t.Errorf("expected AgentAlphaLeaseVerified to be true")
	}
	if report.LogAudit.BypassCheatingDetected {
		t.Errorf("expected 0 direct file bypasses, got cheating flagged: %v", report.LogAudit.FlaggedActions)
	}
}

func TestJudgeEngine_DetectsDirectFileBypass(t *testing.T) {
	ctx := context.Background()

	mockLLM := llm.NewMockProvider(DefaultJudgeModel)
	mockLLM.EnqueueText("VERDICT: FAILED\nCONFIDENCE: HIGH\nQUALITY: 10 / 25\nREASONING: Direct file bypass detected.")

	judge := NewRaterJudge(mockLLM, DefaultJudgeModel)

	sessBypass := &framework.AgentSession{
		SessionID:    "session-bad-101",
		ScenarioName: "ScenarioCloudRunMesh",
		AgentID:      "agent-bad",
		Duration:     5 * time.Second,
		Traces: []framework.StepTrace{
			{
				StepIndex: 1,
				ToolCalls: []llm.ToolCall{
					{Name: "raw_write_bypass", Arguments: `{"path":"src/main.go"}`},
				},
			},
		},
	}

	req := &JudgeEvaluationRequest{
		ScenarioName: "ScenarioCloudRunMesh",
		Sessions:     []*framework.AgentSession{sessBypass},
	}

	report, err := judge.Evaluate(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.LogAudit.BypassCheatingDetected {
		t.Errorf("expected bypass cheating detection, got false")
	}
	if report.Verdict == "PASSED" {
		t.Errorf("expected verdict to be FAILED due to bypass, got %s", report.Verdict)
	}
}
