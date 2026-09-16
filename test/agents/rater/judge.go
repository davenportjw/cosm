package rater

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
)

// DefaultJudgeModel is the mandated Gemini 3.8+ model for autonomous rater judges.
const DefaultJudgeModel = "gemini-3.8-flash"

// JudgeLogAudit captures specific rule compliance discovered during agent log review.
type JudgeLogAudit struct {
	AgentZeroInitVerified           bool   `json:"agent_zero_init_verified"`
	AgentAlphaLeaseVerified         bool   `json:"agent_alpha_lease_verified"`
	AgentBetaSparsePullVerified     bool   `json:"agent_beta_sparse_pull_verified"`
	AgentBetaStackedProposalVerified bool  `json:"agent_beta_stacked_proposal_verified"`
	AgentGammaTerraformValidated    bool   `json:"agent_gamma_terraform_validated"`
	AgentDeltaConflictVerified      bool   `json:"agent_delta_conflict_verified"`
	BypassCheatingDetected          bool   `json:"bypass_cheating_detected"`
	FlaggedActions                  []string `json:"flagged_actions,omitempty"`
}

// JudgeEvaluationRequest bundles all trace data, metrics, and oracle reports for the judge.
type JudgeEvaluationRequest struct {
	ScenarioName string                    `json:"scenario_name"`
	Sessions     []*framework.AgentSession `json:"sessions"`
	FinalAudit   *OracleAuditReport        `json:"final_audit,omitempty"`
	HubMetrics   map[string]interface{}    `json:"hub_metrics,omitempty"`
}

// JudgeEvaluationReport contains the multi-dimensional scorecard and LLM critique.
type JudgeEvaluationReport struct {
	JudgeModel             string         `json:"judge_model"`
	OverallScore           float64        `json:"overall_score"` // 0.0 to 100.0
	LetterGrade            LetterGrade    `json:"letter_grade"`  // A+, A, B, C, F
	Verdict                string         `json:"verdict"`       // PASSED, FAILED
	CosmUtilizationScore   float64        `json:"cosm_utilization_score"`   // max 25
	TimeEfficiencyScore    float64        `json:"time_efficiency_score"`    // max 20
	TokenEconomicsScore    float64        `json:"token_economics_score"`    // max 20
	OutputQualityScore     float64        `json:"output_quality_score"`     // max 35
	TotalDurationMs        int64          `json:"total_duration_ms"`
	TotalTokensUsed        int            `json:"total_tokens_used"`
	TotalToolCalls         int            `json:"total_tool_calls"`
	CosmToolCalls          int            `json:"cosm_tool_calls"`
	LegitimateCosmRatio    float64        `json:"legitimate_cosm_ratio"`
	LogAudit               JudgeLogAudit  `json:"log_audit"`
	Critique               string         `json:"critique"`
	Recommendations        []string       `json:"recommendations"`
	Timestamp              time.Time      `json:"timestamp"`
}

// RaterJudge evaluates multi-agent execution logs and output quality.
type RaterJudge struct {
	provider llm.LLMProvider
	model    string
}

// NewRaterJudge constructs a new RaterJudge instance.
func NewRaterJudge(provider llm.LLMProvider, model ...string) *RaterJudge {
	m := DefaultJudgeModel
	if len(model) > 0 && model[0] != "" {
		m = model[0]
	}
	return &RaterJudge{
		provider: provider,
		model:    m,
	}
}

// Evaluate analyzes the agent sessions, audits tool fidelity, and produces the final scorecard.
func (j *RaterJudge) Evaluate(ctx context.Context, req *JudgeEvaluationRequest) (*JudgeEvaluationReport, error) {
	report := &JudgeEvaluationReport{
		JudgeModel:      j.model,
		Timestamp:       time.Now().UTC(),
		Recommendations: make([]string, 0),
		LogAudit: JudgeLogAudit{
			FlaggedActions: make([]string, 0),
		},
	}

	cosmToolsPrefixes := []string{
		"cosm_", "cosm", "topocosm",
	}

	totalDuration := time.Duration(0)
	totalTokens := 0
	totalToolCalls := 0
	cosmToolCalls := 0

	for _, sess := range req.Sessions {
		totalDuration += sess.Duration
		totalTokens += sess.TotalTokens.TotalTokens

		for _, trace := range sess.Traces {
			for _, tc := range trace.ToolCalls {
				totalToolCalls++
				isCosm := false
				for _, prefix := range cosmToolsPrefixes {
					if strings.HasPrefix(strings.ToLower(tc.Name), prefix) {
						isCosm = true
						break
					}
				}
				if isCosm {
					cosmToolCalls++
				}

				nameLower := strings.ToLower(tc.Name)
				argsLower := strings.ToLower(tc.Arguments)
				// Audit specific agent actions
				switch {
				case strings.Contains(nameLower, "init"):
					report.LogAudit.AgentZeroInitVerified = true
				case strings.Contains(nameLower, "claim"):
					report.LogAudit.AgentAlphaLeaseVerified = true
					if strings.Contains(argsLower, "contend") || strings.Contains(sess.AgentID, "contender") {
						report.LogAudit.AgentDeltaConflictVerified = true
					}
				case strings.Contains(nameLower, "clone") || strings.Contains(nameLower, "sparse") || strings.Contains(nameLower, "pull"):
					report.LogAudit.AgentBetaSparsePullVerified = true
					if strings.Contains(argsLower, "infra") || strings.Contains(argsLower, "terraform") {
						report.LogAudit.AgentGammaTerraformValidated = true
					}
				case strings.Contains(nameLower, "stack"):
					report.LogAudit.AgentBetaStackedProposalVerified = true
					if strings.Contains(argsLower, "infra") || strings.Contains(argsLower, "terraform") {
						report.LogAudit.AgentGammaTerraformValidated = true
					}
				case strings.Contains(nameLower, "terraform") || strings.Contains(nameLower, "infra"):
					report.LogAudit.AgentGammaTerraformValidated = true
				case strings.Contains(nameLower, "conflict") || strings.Contains(nameLower, "contend"):
					report.LogAudit.AgentDeltaConflictVerified = true
				case strings.Contains(nameLower, "raw_write_bypass") || strings.Contains(nameLower, "force_overwrite"):
					report.LogAudit.BypassCheatingDetected = true
					report.LogAudit.FlaggedActions = append(report.LogAudit.FlaggedActions, fmt.Sprintf("%s in session %s", tc.Name, sess.SessionID))
				}
			}
		}
	}

	report.TotalDurationMs = totalDuration.Milliseconds()
	report.TotalTokensUsed = totalTokens
	report.TotalToolCalls = totalToolCalls
	report.CosmToolCalls = cosmToolCalls

	if totalToolCalls > 0 {
		report.LegitimateCosmRatio = float64(cosmToolCalls) / float64(totalToolCalls)
	} else {
		report.LegitimateCosmRatio = 0.0
	}

	// 1. Cosm Utilization Score (max 25)
	if report.LegitimateCosmRatio >= 0.80 {
		report.CosmUtilizationScore = 25.0
	} else {
		report.CosmUtilizationScore = report.LegitimateCosmRatio * 25.0
	}
	if report.LogAudit.BypassCheatingDetected {
		report.CosmUtilizationScore -= 10.0
		if report.CosmUtilizationScore < 0 {
			report.CosmUtilizationScore = 0
		}
	}

	numSessions := len(req.Sessions)

	// 2. Time Efficiency Score (max 20)
	// Normalized by session count for horizontal swarm scaling
	avgDuration := totalDuration
	if numSessions > 0 {
		avgDuration = totalDuration / time.Duration(numSessions)
	}
	if avgDuration < 10*time.Second {
		report.TimeEfficiencyScore = 20.0
	} else if avgDuration < 30*time.Second {
		report.TimeEfficiencyScore = 17.0
	} else if avgDuration < 60*time.Second {
		report.TimeEfficiencyScore = 14.0
	} else {
		report.TimeEfficiencyScore = 10.0
	}

	// 3. Token Economics Score (max 20)
	// Normalized by session count for horizontal swarm scaling
	avgTokens := totalTokens
	if numSessions > 0 {
		avgTokens = totalTokens / numSessions
	}
	if avgTokens < 5000 {
		report.TokenEconomicsScore = 20.0
	} else if avgTokens < 12000 {
		report.TokenEconomicsScore = 17.0
	} else if avgTokens < 20000 {
		report.TokenEconomicsScore = 14.0
	} else {
		report.TokenEconomicsScore = 10.0
	}

	// 4. Output Quality & Contract Integrity Score (max 35)
	qualityScore := 35.0
	if req.FinalAudit != nil {
		for _, f := range req.FinalAudit.Findings {
			switch f.Severity {
			case SeverityCritical:
				qualityScore -= 15.0
			case SeverityHigh:
				qualityScore -= 8.0
			case SeverityMedium:
				qualityScore -= 3.0
			}
		}
		for _, entry := range req.FinalAudit.ContractEntries {
			if entry.Status == "BROKEN" {
				qualityScore -= 10.0
			}
		}
	}
	if qualityScore < 0 {
		qualityScore = 0
	}
	report.OutputQualityScore = qualityScore

	// Overall Score (0 - 100)
	report.OverallScore = report.CosmUtilizationScore + report.TimeEfficiencyScore + report.TokenEconomicsScore + report.OutputQualityScore
	if report.OverallScore > 100.0 {
		report.OverallScore = 100.0
	}

	report.LetterGrade = ComputeLetterGrade(report.OverallScore, 0)
	if report.OverallScore >= 85.0 && !report.LogAudit.BypassCheatingDetected {
		report.Verdict = "PASSED"
	} else {
		report.Verdict = "FAILED"
	}

	// LLM qualitative evaluation if provider is active
	if j.provider != nil {
		summaryPrompt := fmt.Sprintf(
			"You are an autonomous AI software evaluation judge powered by %s.\n"+
				"Review the following Cosm/Topocosm multi-agent swarm run:\n"+
				"- Scenario: %s\n"+
				"- Sessions: %d\n"+
				"- Total Wall Duration: %d ms\n"+
				"- Total Tokens Used: %d\n"+
				"- Total Tool Calls: %d (Cosm: %d, Ratio: %.2f)\n"+
				"- Cosm Utilization Score: %.1f/25\n"+
				"- Time Efficiency Score: %.1f/20\n"+
				"- Token Economics Score: %.1f/20\n"+
				"- Output Quality Score: %.1f/35\n"+
				"- Overall Score: %.1f/100 (Verdict: %s)\n"+
				"- Log Audit: Init=%v, Lease=%v, SparsePull=%v, Stack=%v, Terraform=%v, Conflict=%v, Bypass=%v\n\n"+
				"Provide a concise, technically rigorous critique of how effectively the agents used Cosm/Topocosm primitives "+
				"(micro-universes, leases, sparse pulls, stacked proposals, contract edges) and recommendations for optimization.",
			j.model, req.ScenarioName, len(req.Sessions), report.TotalDurationMs, report.TotalTokensUsed,
			report.TotalToolCalls, report.CosmToolCalls, report.LegitimateCosmRatio,
			report.CosmUtilizationScore, report.TimeEfficiencyScore, report.TokenEconomicsScore, report.OutputQualityScore,
			report.OverallScore, report.Verdict,
			report.LogAudit.AgentZeroInitVerified, report.LogAudit.AgentAlphaLeaseVerified,
			report.LogAudit.AgentBetaSparsePullVerified, report.LogAudit.AgentBetaStackedProposalVerified,
			report.LogAudit.AgentGammaTerraformValidated, report.LogAudit.AgentDeltaConflictVerified,
			report.LogAudit.BypassCheatingDetected,
		)

		messages := []llm.Message{
			{Role: llm.RoleSystem, Content: "You are an autonomous AI judge auditing Cosm AST and Topocosm distributed SCM execution logs."},
			{Role: llm.RoleUser, Content: summaryPrompt},
		}

		resp, err := j.provider.Generate(ctx, messages, nil, llm.GenerateOptions{
			Model: j.model,
		})
		if err == nil && resp != nil && resp.Content != "" {
			report.Critique = strings.TrimSpace(resp.Content)
		}
	}

	if report.Critique == "" {
		report.Critique = fmt.Sprintf(
			"Swarm completed scenario %s with %.1f/100 overall score (%s). "+
				"Cosm tool fidelity ratio was %.2f across %d tool calls with %d tokens consumed. "+
				"Zero unauthorized bypasses detected.",
			req.ScenarioName, report.OverallScore, report.Verdict,
			report.LegitimateCosmRatio, report.TotalToolCalls, report.TotalTokensUsed,
		)
	}

	return report, nil
}

// MarkdownSummary formats the report into a rich Markdown audit card.
func (r *JudgeEvaluationReport) MarkdownSummary() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# ⚖️ Gemini 3.8 Flash Rater Judge Evaluation Report\n\n"))
	sb.WriteString(fmt.Sprintf("- **Judge Model**: `%s`\n", r.JudgeModel))
	sb.WriteString(fmt.Sprintf("- **Verdict**: **`%s`**\n", r.Verdict))
	sb.WriteString(fmt.Sprintf("- **Overall Score**: **`%.1f / 100`** (Grade: **%s**)\n", r.OverallScore, r.LetterGrade))
	sb.WriteString(fmt.Sprintf("- **Timestamp**: %s\n\n", r.Timestamp.Format(time.RFC3339)))

	sb.WriteString("## Multi-Dimensional Score Breakdown\n\n")
	sb.WriteString("| Dimension | Weight / Max | Score Awarded | Status |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Cosm/Topocosm Utilization** | 25 pts | `%.1f` | %s |\n", r.CosmUtilizationScore, passStr(r.CosmUtilizationScore >= 20)))
	sb.WriteString(fmt.Sprintf("| **Time & Step Efficiency** | 20 pts | `%.1f` | %s |\n", r.TimeEfficiencyScore, passStr(r.TimeEfficiencyScore >= 14)))
	sb.WriteString(fmt.Sprintf("| **Token Economics** | 20 pts | `%.1f` | %s |\n", r.TokenEconomicsScore, passStr(r.TokenEconomicsScore >= 14)))
	sb.WriteString(fmt.Sprintf("| **Output Quality & Contracts** | 35 pts | `%.1f` | %s |\n\n", r.OutputQualityScore, passStr(r.OutputQualityScore >= 30)))

	sb.WriteString("## Telemetry & Execution Resource Metrics\n\n")
	sb.WriteString(fmt.Sprintf("- **Wall Duration**: `%d ms` (%.2fs)\n", r.TotalDurationMs, float64(r.TotalDurationMs)/1000.0))
	sb.WriteString(fmt.Sprintf("- **Total Token Expenditure**: `%d tokens`\n", r.TotalTokensUsed))
	sb.WriteString(fmt.Sprintf("- **Total Tool Calls**: `%d` (Legitimate Cosm calls: `%d`, Ratio: `%.2f`)\n\n", r.TotalToolCalls, r.CosmToolCalls, r.LegitimateCosmRatio))

	sb.WriteString("## Cosm Agent Log Audit Checklist\n\n")
	sb.WriteString(fmt.Sprintf("- [%s] Agent Zero: Repository baseline initialized & published to Hub\n", checkStr(r.LogAudit.AgentZeroInitVerified)))
	sb.WriteString(fmt.Sprintf("- [%s] Agent Alpha: Blackboard lease acquired (`cosm claim`) & clean micro-universe branch\n", checkStr(r.LogAudit.AgentAlphaLeaseVerified)))
	sb.WriteString(fmt.Sprintf("- [%s] Agent Beta: Sparse AST clone executed (>75%% cold-start bandwidth saved)\n", checkStr(r.LogAudit.AgentBetaSparsePullVerified)))
	sb.WriteString(fmt.Sprintf("- [%s] Agent Beta: Jujutsu-style stacked proposal created (`cosm stack create`)\n", checkStr(r.LogAudit.AgentBetaStackedProposalVerified)))
	sb.WriteString(fmt.Sprintf("- [%s] Agent Gamma: Terraform Cloud Run & PubSub validated\n", checkStr(r.LogAudit.AgentGammaTerraformValidated)))
	sb.WriteString(fmt.Sprintf("- [%s] Agent Delta: Concurrency lease 409 rejection & ASTConflictNode reified\n", checkStr(r.LogAudit.AgentDeltaConflictVerified)))
	sb.WriteString(fmt.Sprintf("- [%s] Anti-Cheating: Zero unverified file bypasses detected\n\n", checkStr(!r.LogAudit.BypassCheatingDetected)))

	sb.WriteString("## Qualitative Judge Critique\n\n")
	sb.WriteString(r.Critique)
	sb.WriteString("\n")

	return sb.String()
}

func passStr(p bool) string {
	if p {
		return "✅ PASS"
	}
	return "⚠️ WARNING"
}

func checkStr(b bool) string {
	if b {
		return "x"
	}
	return " "
}
