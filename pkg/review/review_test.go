package review

import (
	"strings"
	"testing"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestProposalPresenter_RenderMarkdownAndTerminal(t *testing.T) {
	presenter := NewProposalPresenter()

	model := &ProposalPresentationModel{
		ProposalID:           "prop-101",
		BaseUniverse:         "universe-main",
		ProposedUniverse:     "u/feat-auth",
		BaseManifestHash:     "1111222233334444",
		ProposedManifestHash: "5555666677778888",
		Author:               "agent-coder-1",
		Intent:               "Implement rate limiter",
		UserPrompt:           "Add token bucket rate limiter to API",
		ExecutingAgentID:     "agent-coder-1",
		SignatureValid:       true,
		BuildStatus:          "PASS",
		AddedSymbols: []SymbolDiffCard{
			{
				SymbolID:   "go:server:RateLimiter",
				Identifier: "server.RateLimiter",
				Language:   core.LangGo,
				NodeType:   "FunctionDecl",
				DiffType:   "ADDED",
				NewSnippet: "func RateLimiter() http.Handler { ... }",
			},
		},
		ModifiedSymbols: []SymbolDiffCard{
			{
				SymbolID:    "go:server:HandleGetUsers",
				Identifier:  "server.HandleGetUsers",
				Language:    core.LangGo,
				NodeType:    "FunctionDecl",
				DiffType:    "MODIFIED",
				NewSnippet:  "func HandleGetUsers(w http.ResponseWriter, r *http.Request) { RateLimiter(); ... }",
				ImpactLevel: "MEDIUM",
			},
		},
		ContractDeltas: []ContractDelta{
			{
				SourceID:    "ts:web:fetchUsers",
				TargetID:    "go:server:HandleGetUsers",
				EdgeType:    core.EdgeConsumesAPI,
				Status:      "MODIFIED",
				Description: "API route updated with rate limit header",
			},
		},
	}

	md := presenter.RenderMarkdown(model)
	if !strings.Contains(md, "Proposal: u/feat-auth -> universe-main") {
		t.Errorf("Expected markdown title in output")
	}
	if !strings.Contains(md, "server.RateLimiter") || !strings.Contains(md, "server.HandleGetUsers") {
		t.Errorf("Expected symbols in markdown output")
	}
	if !strings.Contains(md, "CONSUMES_API") {
		t.Errorf("Expected contract delta in markdown output")
	}

	term := presenter.RenderTerminalCards(model)
	if !strings.Contains(term, "PROPOSAL: u/feat-auth -> universe-main") {
		t.Errorf("Expected terminal header in output")
	}
}

func TestCriticEngine_AutonomousReviewApproval(t *testing.T) {
	critic := NewCriticEngine("critic-agent-security")

	cleanModel := &ProposalPresentationModel{
		ProposalID:           "prop-clean-01",
		BaseUniverse:         "universe-main",
		ProposedUniverse:     "u/feat-clean",
		BaseManifestHash:     "1111222233334444",
		ProposedManifestHash: "5555666677778888",
		Author:               "dev-user",
		Intent:               "Add unit tests",
		UserPrompt:           "Write unit test for user service",
		ExecutingAgentID:     "agent-test-writer",
		SignatureValid:       true,
		BuildStatus:          "PASS",
		AddedSymbols: []SymbolDiffCard{
			{
				SymbolID:   "go:server:TestUsers",
				Identifier: "server.TestUsers",
				Language:   core.LangGo,
				NodeType:   "FunctionDecl",
				DiffType:   "ADDED",
				NewSnippet: "func TestUsers(t *testing.T) { ... }",
			},
		},
	}

	report := critic.EvaluateProposal(cleanModel)
	if report.Verdict != "APPROVE" {
		t.Errorf("Expected APPROVE verdict, got %s", report.Verdict)
	}
	if report.FitnessScore < 0.85 {
		t.Errorf("Expected FitnessScore >= 0.85, got %f", report.FitnessScore)
	}
	if len(report.Critiques) != 0 {
		t.Errorf("Expected 0 critiques for clean proposal, got %d", len(report.Critiques))
	}
}

func TestCriticEngine_ContractDriftAndSecurityFlagging(t *testing.T) {
	critic := NewCriticEngine("critic-agent-security")

	problemModel := &ProposalPresentationModel{
		ProposalID:           "prop-bad-01",
		BaseUniverse:         "universe-main",
		ProposedUniverse:     "u/feat-risky",
		BaseManifestHash:     "1111222233334444",
		ProposedManifestHash: "5555666677778888",
		Author:               "agent-hack",
		Intent:               "Deploy open gateway",
		UserPrompt:           "Open gateway ports",
		ExecutingAgentID:     "agent-hack",
		SignatureValid:       false,
		BuildStatus:          "FAIL (go build error)",
		AddedSymbols: []SymbolDiffCard{
			{
				SymbolID:   "tf:deploy:security_group",
				Identifier: "deploy.security_group",
				Language:   core.LangHCL,
				NodeType:   "ResourceBlock",
				DiffType:   "ADDED",
				NewSnippet: `resource "google_compute_firewall" "allow_all" {
  ingress {
    cidr_blocks = ["0.0.0.0/0"]
  }
}`,
			},
		},
		ContractDeltas: []ContractDelta{
			{
				SourceID:    "ts:web:fetchUsers",
				TargetID:    "go:server:HandleGetUsers",
				EdgeType:    core.EdgeConsumesAPI,
				Status:      "BROKEN",
				Description: "Route path changed without updating client",
			},
		},
	}

	report := critic.EvaluateProposal(problemModel)
	if report.Verdict == "APPROVE" {
		t.Errorf("Expected non-APPROVE verdict for problem proposal, got %s", report.Verdict)
	}
	if len(report.Critiques) < 3 { // Broken contract, security 0.0.0.0/0, failed build, unverified signature
		t.Errorf("Expected at least 3 critiques, got %d", len(report.Critiques))
	}
	if report.RemediationPrompt == "" {
		t.Errorf("Expected non-empty remediation prompt for automated agent loop")
	}
}
