package collaboration

import (
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// ConflictSeverity indicates how severe a semantic merge conflict is.
type ConflictSeverity string

const (
	SeverityError   ConflictSeverity = "ERROR"   // Blocks commit/merge
	SeverityWarning ConflictSeverity = "WARNING" // Informational warning
)

// SemanticConflict describes a logical contradiction across agents or universes.
type SemanticConflict struct {
	ConflictType string           `json:"conflict_type"` // e.g., "ROUTE_CONTRACT_BROKEN", "CONCURRENT_MUTATION"
	Severity     ConflictSeverity `json:"severity"`
	SourceNodeID string           `json:"source_node_id"`
	TargetNodeID string           `json:"target_node_id"`
	Message      string           `json:"message"`
	Remediation  string           `json:"remediation"`
}

// ConflictReport summarizes all detected semantic contradictions.
type ConflictReport struct {
	HasErrors bool                `json:"has_errors"`
	Conflicts []*SemanticConflict `json:"conflicts"`
}

// ConflictEngine detects semantic collisions across concurrent agent modifications.
type ConflictEngine struct{}

// NewConflictEngine creates a new ConflictEngine instance.
func NewConflictEngine() *ConflictEngine {
	return &ConflictEngine{}
}

// CheckConflicts evaluates two concurrent workspace states for semantic contradictions.
func (ce *ConflictEngine) CheckConflicts(
	base *core.WorkspaceManifestNode,
	branchA *core.WorkspaceManifestNode,
	branchB *core.WorkspaceManifestNode,
	compMapA map[string]*core.ComponentNode,
	compMapB map[string]*core.ComponentNode,
	symMapA map[string]*core.ASTSymbolNode,
	symMapB map[string]*core.ASTSymbolNode,
) *ConflictReport {
	report := &ConflictReport{
		HasErrors: false,
		Conflicts: make([]*SemanticConflict, 0),
	}

	// 1. Detect concurrent mutations on the same symbol with divergent contents
	for nodeIDA, symA := range symMapA {
		if symB, exists := symMapB[nodeIDA]; exists {
			if string(symA.ASTPayload) != string(symB.ASTPayload) {
				report.Conflicts = append(report.Conflicts, &SemanticConflict{
					ConflictType: "CONCURRENT_SYMBOL_MUTATION",
					Severity:     SeverityError,
					SourceNodeID: symA.NodeID,
					TargetNodeID: symB.NodeID,
					Message: fmt.Sprintf("Symbol %q (%s) modified concurrently in both branches with differing logic",
						symA.Identifier, symA.NodeType),
					Remediation: "Resolve symbol divergence by picking one universe implementation or auto-clustering.",
				})
				report.HasErrors = true
			}
		}
	}

	// 2. Detect cross-boundary contract breakages if base edges exist
	var baseEdges []core.CrossBoundaryEdge
	if base != nil {
		baseEdges = base.CrossEdges
	}

	breakagesA := core.DetectContractBreakages(baseEdges, branchA.CrossEdges, symMapA, symMapA)
	for _, b := range breakagesA {
		report.Conflicts = append(report.Conflicts, &SemanticConflict{
			ConflictType: string(b.Type),
			Severity:     SeverityError,
			SourceNodeID: b.SourceNodeID,
			TargetNodeID: b.TargetNodeID,
			Message:      b.Description,
			Remediation:  "Align client route or infra configuration with updated backend symbol.",
		})
		report.HasErrors = true
	}

	breakagesB := core.DetectContractBreakages(baseEdges, branchB.CrossEdges, symMapB, symMapB)
	for _, b := range breakagesB {
		report.Conflicts = append(report.Conflicts, &SemanticConflict{
			ConflictType: string(b.Type),
			Severity:     SeverityError,
			SourceNodeID: b.SourceNodeID,
			TargetNodeID: b.TargetNodeID,
			Message:      b.Description,
			Remediation:  "Align client route or infra configuration with updated backend symbol.",
		})
		report.HasErrors = true
	}

	return report
}

// FormatConflictReport prints human-readable conflict diagnostics.
func FormatConflictReport(report *ConflictReport) string {
	if report == nil || len(report.Conflicts) == 0 {
		return "No semantic conflicts detected. Clean state."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("=== Semantic Conflict Report (%d conflicts detected) ===\n", len(report.Conflicts)))
	for idx, c := range report.Conflicts {
		b.WriteString(fmt.Sprintf("[%d] [%s] %s\n", idx+1, c.Severity, c.ConflictType))
		b.WriteString(fmt.Sprintf("    Details:     %s\n", c.Message))
		b.WriteString(fmt.Sprintf("    Remediation: %s\n", c.Remediation))
	}
	return b.String()
}
