package review

import (
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// CritiqueItem represents a fine-grained, actionable issue targeting a specific AST symbol.
type CritiqueItem struct {
	SymbolID        string `json:"symbol_id"`
	Category        string `json:"category"` // "CONTRACT_DRIFT", "SECURITY_IAM", "SCOPE_DRIFT", "COMPILER_ERROR"
	Severity        string `json:"severity"` // "HIGH", "MEDIUM", "LOW"
	Description     string `json:"description"`
	SuggestedFix    string `json:"suggested_fix"`
	RemediationTask string `json:"remediation_task"`
}

// CritiqueReport represents the autonomous assessment produced by an AI reviewer agent.
type CritiqueReport struct {
	ProposalID        string         `json:"proposal_id"`
	ReviewerAgent     string         `json:"reviewer_agent"`
	Verdict           string         `json:"verdict"` // "APPROVE", "REQUEST_CHANGES", "REJECT"
	FitnessScore      float64        `json:"fitness_score"`
	Critiques         []CritiqueItem `json:"critiques"`
	RemediationPrompt string         `json:"remediation_prompt"`
}

// CriticEngine performs multi-vector automated fitness evaluations on Universe Proposals.
type CriticEngine struct {
	AgentID string
}

// NewCriticEngine creates a new CriticEngine instance with a given reviewer identity.
func NewCriticEngine(agentID string) *CriticEngine {
	if agentID == "" {
		agentID = "critic-agent-core"
	}
	return &CriticEngine{
		AgentID: agentID,
	}
}

// EvaluateProposal conducts autonomous multi-vector audits on a proposal presentation model.
func (c *CriticEngine) EvaluateProposal(model *ProposalPresentationModel) *CritiqueReport {
	report := &CritiqueReport{
		ProposalID:    model.ProposalID,
		ReviewerAgent: c.AgentID,
		Verdict:       "APPROVE",
		FitnessScore:  1.0,
		Critiques:     []CritiqueItem{},
	}

	var remPrompts []string

	// 1. Contract Consistency & Broken Edges Check
	for _, delta := range model.ContractDeltas {
		if delta.Status == "BROKEN" {
			crit := CritiqueItem{
				SymbolID:        delta.SourceID,
				Category:        "CONTRACT_DRIFT",
				Severity:        "HIGH",
				Description:     fmt.Sprintf("Cross-boundary %s contract to %s is broken: %s", delta.EdgeType, delta.TargetID, delta.Description),
				SuggestedFix:    fmt.Sprintf("Update caller %s to adhere to target %s contract schema.", delta.SourceID, delta.TargetID),
				RemediationTask: fmt.Sprintf("Fix cross-boundary signature mismatch between %s and %s", delta.SourceID, delta.TargetID),
			}
			report.Critiques = append(report.Critiques, crit)
			report.FitnessScore -= 0.25
			remPrompts = append(remPrompts, crit.RemediationTask)
		}
	}

	// 2. Security & IAM Audit on Terraform/HCL symbols
	for _, sym := range append(model.ModifiedSymbols, model.AddedSymbols...) {
		if sym.Language == core.LangHCL {
			snippet := sym.NewSnippet
			if strings.Contains(snippet, "0.0.0.0/0") && strings.Contains(snippet, "ingress") {
				crit := CritiqueItem{
					SymbolID:        sym.SymbolID,
					Category:        "SECURITY_IAM",
					Severity:        "HIGH",
					Description:     "Permissive 0.0.0.0/0 ingress rule detected in infrastructure configuration.",
					SuggestedFix:    "Restrict CIDR blocks to internal VPC or authorized bastion IPs.",
					RemediationTask: fmt.Sprintf("Restrict ingress CIDR block in %s", sym.Identifier),
				}
				report.Critiques = append(report.Critiques, crit)
				report.FitnessScore -= 0.30
				remPrompts = append(remPrompts, crit.RemediationTask)
			}
			if strings.Contains(snippet, "roles/owner") || strings.Contains(snippet, "roles/editor") {
				crit := CritiqueItem{
					SymbolID:        sym.SymbolID,
					Category:        "SECURITY_IAM",
					Severity:        "HIGH",
					Description:     "Overprivileged IAM basic role (roles/owner or roles/editor) found in resource binding.",
					SuggestedFix:    "Use least-privilege predefined or custom IAM roles.",
					RemediationTask: fmt.Sprintf("Replace primitive IAM role in %s with least-privilege role", sym.Identifier),
				}
				report.Critiques = append(report.Critiques, crit)
				report.FitnessScore -= 0.25
				remPrompts = append(remPrompts, crit.RemediationTask)
			}
		}
	}

	// 3. Build & Compiler Diagnostics Check
	if model.BuildStatus != "" && model.BuildStatus != "PASS" {
		crit := CritiqueItem{
			SymbolID:        "build-target",
			Category:        "COMPILER_ERROR",
			Severity:        "HIGH",
			Description:     fmt.Sprintf("Target compilation / validation failed with status: %s", model.BuildStatus),
			SuggestedFix:    "Resolve compilation diagnostics reported by sidecar build.",
			RemediationTask: "Fix compiler / typechecking errors before merging proposal",
		}
		report.Critiques = append(report.Critiques, crit)
		report.FitnessScore -= 0.40
		remPrompts = append(remPrompts, crit.RemediationTask)
	}

	// 4. Provenance & Cryptographic Signature Check
	if !model.SignatureValid {
		crit := CritiqueItem{
			SymbolID:        "lineage-envelope",
			Category:        "UNVERIFIED_SIGNATURE",
			Severity:        "MEDIUM",
			Description:     "Proposal contains unverified or missing Ed25519 agent signatures.",
			SuggestedFix:    "Re-sign lineage envelope using authorized agent Ed25519 key.",
			RemediationTask: "Sign lineage record with agent private key",
		}
		report.Critiques = append(report.Critiques, crit)
		report.FitnessScore -= 0.10
	}

	// Finalize score and verdict
	if report.FitnessScore < 0.0 {
		report.FitnessScore = 0.0
	}

	if report.FitnessScore >= 0.85 && len(report.Critiques) == 0 {
		report.Verdict = "APPROVE"
	} else if report.FitnessScore >= 0.50 {
		report.Verdict = "REQUEST_CHANGES"
	} else {
		report.Verdict = "REJECT"
	}

	if len(remPrompts) > 0 {
		report.RemediationPrompt = fmt.Sprintf(
			"Please address the following automated review items in universe %s:\n- %s",
			model.ProposedUniverse,
			strings.Join(remPrompts, "\n- "),
		)
	}

	return report
}
