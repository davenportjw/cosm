package review

import (
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// SymbolDiffCard represents a rendered diff for an individual AST symbol.
type SymbolDiffCard struct {
	SymbolID    string        `json:"symbol_id"`
	Identifier  string        `json:"identifier"`
	Language    core.Language `json:"language"`
	NodeType    string        `json:"node_type"`
	DiffType    string        `json:"diff_type"` // "ADDED", "MODIFIED", "DELETED"
	OldSnippet  string        `json:"old_snippet,omitempty"`
	NewSnippet  string        `json:"new_snippet"`
	Callers     []string      `json:"callers,omitempty"`
	ImpactLevel string        `json:"impact_level"` // "LOW", "MEDIUM", "HIGH"
}

// ContractDelta details a semantic cross-boundary contract addition, modification, or deletion.
type ContractDelta struct {
	SourceID    string        `json:"source_id"`
	TargetID    string        `json:"target_id"`
	EdgeType    core.EdgeType `json:"edge_type"`
	Status      string        `json:"status"` // "NEW", "MODIFIED", "BROKEN"
	Description string        `json:"description"`
}

// ProposalPresentationModel encapsulates all human and agent visual fields for a Universe Proposal.
type ProposalPresentationModel struct {
	ProposalID           string           `json:"proposal_id"`
	BaseUniverse         string           `json:"base_universe"`
	ProposedUniverse     string           `json:"proposed_universe"`
	BaseManifestHash     string           `json:"base_manifest_hash"`
	ProposedManifestHash string           `json:"proposed_manifest_hash"`
	Author               string           `json:"author"`
	Intent               string           `json:"intent"`
	UserPrompt           string           `json:"user_prompt"`
	ExecutingAgentID     string           `json:"executing_agent_id"`
	SignatureValid       bool             `json:"signature_valid"`
	BuildStatus          string           `json:"build_status"`
	AddedSymbols         []SymbolDiffCard `json:"added_symbols"`
	ModifiedSymbols      []SymbolDiffCard `json:"modified_symbols"`
	DeletedSymbols       []SymbolDiffCard `json:"deleted_symbols"`
	ContractDeltas       []ContractDelta  `json:"contract_deltas"`
}

// ProposalPresenter generates rich Markdown and Terminal presentation views for Pull Requests / Proposals.
type ProposalPresenter struct{}

// NewProposalPresenter initializes a new presenter.
func NewProposalPresenter() *ProposalPresenter {
	return &ProposalPresenter{}
}

// RenderMarkdown produces GitHub Flavored Markdown for PR bodies and review dashboards.
func (p *ProposalPresenter) RenderMarkdown(model *ProposalPresentationModel) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Proposal: %s -> %s\n\n", model.ProposedUniverse, model.BaseUniverse))

	// 1. Executive Summary Banner
	b.WriteString("## 1. Executive Summary & Provenance\n\n")
	b.WriteString(fmt.Sprintf("- **Intent**: %s\n", model.Intent))
	b.WriteString(fmt.Sprintf("- **Prompt**: `%s`\n", model.UserPrompt))
	b.WriteString(fmt.Sprintf("- **Agent**: `%s` (Signed: %t)\n", model.ExecutingAgentID, model.SignatureValid))
	b.WriteString(fmt.Sprintf("- **Merkle Delta**: `%s` -> `%s`\n", shortenHash(model.BaseManifestHash), shortenHash(model.ProposedManifestHash)))
	b.WriteString(fmt.Sprintf("- **Build Status**: `%s`\n\n", model.BuildStatus))

	// 2. Topology & Contract Deltas
	b.WriteString("## 2. Cross-Domain Topology & Contract Deltas\n\n")
	if len(model.ContractDeltas) == 0 {
		b.WriteString("_No cross-boundary contract changes._\n\n")
	} else {
		b.WriteString("| Source | Target | Edge Type | Contract Status |\n")
		b.WriteString("| :--- | :--- | :--- | :--- |\n")
		for _, c := range model.ContractDeltas {
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | **%s** |\n", c.SourceID, c.TargetID, c.EdgeType, c.Status))
		}
		b.WriteString("\n")
	}

	// 3. AST Symbol Cards
	b.WriteString("## 3. AST Symbol Changes\n\n")
	if len(model.ModifiedSymbols) > 0 {
		b.WriteString("### Modified Symbols\n\n")
		for _, s := range model.ModifiedSymbols {
			b.WriteString(fmt.Sprintf("#### 📝 `%s` (%s %s) - Impact: %s\n\n", s.Identifier, s.Language, s.NodeType, s.ImpactLevel))
			b.WriteString(fmt.Sprintf("```%s\n%s\n```\n\n", s.Language, s.NewSnippet))
		}
	}

	if len(model.AddedSymbols) > 0 {
		b.WriteString("### Added Symbols\n\n")
		for _, s := range model.AddedSymbols {
			b.WriteString(fmt.Sprintf("#### ✨ `%s` (%s %s)\n\n", s.Identifier, s.Language, s.NodeType))
			b.WriteString(fmt.Sprintf("```%s\n%s\n```\n\n", s.Language, s.NewSnippet))
		}
	}

	if len(model.DeletedSymbols) > 0 {
		b.WriteString("### Deleted Symbols\n\n")
		for _, s := range model.DeletedSymbols {
			b.WriteString(fmt.Sprintf("- ❌ `%s` (%s %s)\n", s.Identifier, s.Language, s.NodeType))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// RenderTerminalCards produces formatted text for CLI display (`fg proposal view`).
func (p *ProposalPresenter) RenderTerminalCards(model *ProposalPresentationModel) string {
	var b strings.Builder
	sep := strings.Repeat("═", 70)
	subsep := strings.Repeat("─", 70)

	b.WriteString(fmt.Sprintf("%s\n", sep))
	b.WriteString(fmt.Sprintf(" 🌌 PROPOSAL: %s -> %s\n", model.ProposedUniverse, model.BaseUniverse))
	b.WriteString(fmt.Sprintf(" Intent:  %s\n", model.Intent))
	b.WriteString(fmt.Sprintf(" Agent:   %s | Signed: %t | Build: %s\n", model.ExecutingAgentID, model.SignatureValid, model.BuildStatus))
	b.WriteString(fmt.Sprintf("%s\n\n", sep))

	if len(model.ContractDeltas) > 0 {
		b.WriteString(" 🔗 CROSS-BOUNDARY CONTRACT DELTAS:\n")
		for _, c := range model.ContractDeltas {
			b.WriteString(fmt.Sprintf("   [%s] %s -> %s (%s)\n", c.Status, c.SourceID, c.TargetID, c.EdgeType))
		}
		b.WriteString("\n")
	}

	b.WriteString(" 📦 AST SYMBOL MODIFICATIONS:\n")
	for _, s := range model.ModifiedSymbols {
		b.WriteString(fmt.Sprintf("%s\n", subsep))
		b.WriteString(fmt.Sprintf("  • [%s] %s (%s %s) | Impact: %s\n", s.DiffType, s.Identifier, s.Language, s.NodeType, s.ImpactLevel))
		b.WriteString(fmt.Sprintf("%s\n", subsep))
		for _, line := range strings.Split(s.NewSnippet, "\n") {
			b.WriteString(fmt.Sprintf("    %s\n", line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func shortenHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}
