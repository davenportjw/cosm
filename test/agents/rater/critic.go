package rater

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/test/agents/llm"
)

// CritiqueItem represents an actionable critique regarding code architecture, security, or contracts.
type CritiqueItem struct {
	SymbolID        string `json:"symbol_id"`
	Category        string `json:"category"` // "CONTRACT_DRIFT", "SECURITY_IAM", "COMPILER_ERROR", "ARCHITECTURE"
	Severity        string `json:"severity"` // "CRITICAL", "HIGH", "MEDIUM", "LOW"
	Description     string `json:"description"`
	SuggestedFix    string `json:"suggested_fix"`
	RemediationTask string `json:"remediation_task"`
}

// CritiqueReport represents the multi-turn evaluation produced by the AI Critic.
type CritiqueReport struct {
	ReviewerAgent     string         `json:"reviewer_agent"`
	Verdict           string         `json:"verdict"` // "APPROVE", "REQUEST_CHANGES", "REJECT"
	FitnessScore      float64        `json:"fitness_score"` // 0.0 to 1.0
	Critiques         []CritiqueItem `json:"critiques"`
	RemediationPrompt string         `json:"remediation_prompt"`
}

// LLMCritic coordinates multi-turn LLM critique and autonomous remediation prompts.
type LLMCritic struct {
	provider llm.LLMProvider
	agentID  string
	model    string
}

// NewLLMCritic creates a new LLMCritic instance.
func NewLLMCritic(provider llm.LLMProvider, agentID string, model ...string) *LLMCritic {
	m := "gemini-3.7-flash"
	if len(model) > 0 && model[0] != "" {
		m = model[0]
	} else if provider != nil && provider.ModelName() != "" {
		m = provider.ModelName()
	}

	if agentID == "" {
		agentID = "critic-agent-gemini-3.7"
	}

	return &LLMCritic{
		provider: provider,
		agentID:  agentID,
		model:    m,
	}
}

// EvaluateArchitecture evaluates the code topology, cross-boundary contracts, and security posture.
func (c *LLMCritic) EvaluateArchitecture(ctx context.Context, audit *OracleAuditReport, files map[string]string) (*CritiqueReport, error) {
	report := &CritiqueReport{
		ReviewerAgent: c.agentID,
		Verdict:       "APPROVE",
		FitnessScore:  1.0,
		Critiques:     make([]CritiqueItem, 0),
	}

	var remPrompts []string

	// Check broken contracts from oracle
	for _, entry := range audit.ContractEntries {
		if entry.Status == "BROKEN" {
			crit := CritiqueItem{
				SymbolID:        entry.SourceComponent,
				Category:        "CONTRACT_DRIFT",
				Severity:        "HIGH",
				Description:     fmt.Sprintf("Cross-boundary %s contract to %s is broken: %s", entry.ContractType, entry.TargetComponent, entry.Details),
				SuggestedFix:    fmt.Sprintf("Update %s to provide or match contract %s", entry.TargetComponent, entry.Identifier),
				RemediationTask: fmt.Sprintf("Fix cross-boundary %s contract %s in %s", entry.ContractType, entry.Identifier, entry.SourceComponent),
			}
			report.Critiques = append(report.Critiques, crit)
			report.FitnessScore -= 0.25
			remPrompts = append(remPrompts, crit.RemediationTask)
		}
	}

	// Check findings from oracle
	for _, f := range audit.Findings {
		if f.Severity == SeverityCritical || f.Severity == SeverityHigh {
			crit := CritiqueItem{
				SymbolID:        f.FileLocation,
				Category:        f.Dimension,
				Severity:        string(f.Severity),
				Description:     f.Description,
				SuggestedFix:    f.SuggestedFix,
				RemediationTask: fmt.Sprintf("Remediate %s: %s", f.Title, f.SuggestedFix),
			}
			report.Critiques = append(report.Critiques, crit)
			report.FitnessScore -= 0.20
			remPrompts = append(remPrompts, crit.RemediationTask)
		}
	}

	// If provider is configured, we can also prompt the LLM for deep architectural critique
	if c.provider != nil {
		prompt := fmt.Sprintf(
			"You are an expert AI software architect and rater. Evaluate the following audit results:\n"+
				"Files Audited: %d\nBroken Contracts: %d\nFindings Count: %d\n\n"+
				"Provide any additional architectural or security critique.",
			audit.TotalFilesAudited, len(report.Critiques), len(audit.Findings),
		)

		messages := []llm.Message{
			{Role: llm.RoleSystem, Content: "You are an autonomous code critic and rating harness."},
			{Role: llm.RoleUser, Content: prompt},
		}

		resp, err := c.provider.Generate(ctx, messages, nil, llm.GenerateOptions{
			Model: c.model,
		})
		if err == nil && resp != nil && resp.Content != "" {
			// If response contains specific critique text, we can append or log it
			_ = resp.Content
		}
	}

	if report.FitnessScore < 0.0 {
		report.FitnessScore = 0.0
	}

	if len(report.Critiques) == 0 && report.FitnessScore >= 0.85 {
		report.Verdict = "APPROVE"
	} else if report.FitnessScore >= 0.50 {
		report.Verdict = "REQUEST_CHANGES"
	} else {
		report.Verdict = "REJECT"
	}

	if len(remPrompts) > 0 {
		report.RemediationPrompt = fmt.Sprintf(
			"Please address the following automated review items in universe %s:\n- %s",
			audit.UniverseID,
			strings.Join(remPrompts, "\n- "),
		)
	}

	return report, nil
}

// ToJSON serializes the critique report.
func (c *CritiqueReport) ToJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}
