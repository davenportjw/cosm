package mutation

import (
	"fmt"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

// ImpactAssessment details the downstream cross-boundary blast radius of a symbol mutation.
type ImpactAssessment struct {
	TargetSymbolID     string                   `json:"target_symbol_id"`
	IncomingEdges      []core.CrossBoundaryEdge `json:"incoming_edges"`
	OutgoingEdges      []core.CrossBoundaryEdge `json:"outgoing_edges"`
	AffectedComponents []string                 `json:"affected_components"`
	RiskScore          float64                  `json:"risk_score"` // 0.0 (low) to 1.0 (critical)
	Warnings           []string                 `json:"warnings"`
}

// ContractCascadeEngine inspects graph edges to calculate contract breakages and blast radius.
type ContractCascadeEngine struct {
	graphEngine *storage.GraphEngine
}

// NewContractCascadeEngine initializes a cascade analyzer.
func NewContractCascadeEngine(graphEngine *storage.GraphEngine) *ContractCascadeEngine {
	return &ContractCascadeEngine{
		graphEngine: graphEngine,
	}
}

// AssessImpact evaluates the semantic impact of mutating targetSymbolID.
func (c *ContractCascadeEngine) AssessImpact(targetSymbolID string) (*ImpactAssessment, error) {
	incoming := c.graphEngine.ListEdges("", targetSymbolID, "")
	outgoing := c.graphEngine.ListEdges(targetSymbolID, "", "")

	assessment := &ImpactAssessment{
		TargetSymbolID:     targetSymbolID,
		IncomingEdges:      make([]core.CrossBoundaryEdge, len(incoming)),
		OutgoingEdges:      make([]core.CrossBoundaryEdge, len(outgoing)),
		AffectedComponents: []string{},
		Warnings:           []string{},
	}

	compSet := make(map[string]bool)

	for i, e := range incoming {
		edge := core.CrossBoundaryEdge{
			SourceNodeID:     e.SourceID,
			TargetNodeID:     e.TargetID,
			Type:             e.EdgeType,
			ContractSchemaID: e.ContractID,
		}
		assessment.IncomingEdges[i] = edge

		if !compSet[e.SourceID] {
			compSet[e.SourceID] = true
			assessment.AffectedComponents = append(assessment.AffectedComponents, e.SourceID)
		}

		switch e.EdgeType {
		case core.EdgeConsumesAPI:
			assessment.Warnings = append(assessment.Warnings,
				fmt.Sprintf("Incoming API consumer %s may break if route signature changes", e.SourceID))
		case core.EdgeDeploysTo:
			assessment.Warnings = append(assessment.Warnings,
				fmt.Sprintf("Infrastructure deploy contract %s depends on this symbol", e.SourceID))
		case core.EdgeBindsEnv:
			assessment.Warnings = append(assessment.Warnings,
				fmt.Sprintf("Environment binding %s depends on this symbol", e.SourceID))
		}
	}

	for i, e := range outgoing {
		edge := core.CrossBoundaryEdge{
			SourceNodeID:     e.SourceID,
			TargetNodeID:     e.TargetID,
			Type:             e.EdgeType,
			ContractSchemaID: e.ContractID,
		}
		assessment.OutgoingEdges[i] = edge

		if !compSet[e.TargetID] {
			compSet[e.TargetID] = true
			assessment.AffectedComponents = append(assessment.AffectedComponents, e.TargetID)
		}
	}

	// Compute risk score based on edge count and severity
	totalEdges := len(incoming) + len(outgoing)
	if totalEdges == 0 {
		assessment.RiskScore = 0.1
	} else if totalEdges <= 2 {
		assessment.RiskScore = 0.5
	} else {
		assessment.RiskScore = 0.9
	}

	return assessment, nil
}
