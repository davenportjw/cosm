package lineage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

// AuditFilter defines criteria for auditing AST nodes and calculating blast radius.
type AuditFilter struct {
	ExecutingAgentID    string    `json:"executing_agent_id,omitempty"`
	OrchestratorAgentID string    `json:"orchestrator_agent_id,omitempty"`
	LLMVersion          string    `json:"llm_version,omitempty"`
	SessionID           string    `json:"session_id,omitempty"`
	PromptKeyword       string    `json:"prompt_keyword,omitempty"`
	PromptHash          string    `json:"prompt_hash,omitempty"`
	TimeRangeStart      time.Time `json:"time_range_start,omitempty"`
	TimeRangeEnd        time.Time `json:"time_range_end,omitempty"`
}

// BlastRadiusReport details all directly generated and transitively impacted graph nodes.
type BlastRadiusReport struct {
	Filter                  AuditFilter          `json:"filter"`
	DirectlyTouchedNodes    []storage.NodeRecord `json:"directly_touched_nodes"`
	DownstreamImpactedNodes []storage.NodeRecord `json:"downstream_impacted_nodes"`
	ImpactedEdges           []storage.EdgeRecord `json:"impacted_edges"`
	AffectedLanguages       []core.Language      `json:"affected_languages"`
	TotalDirectNodes        int                  `json:"total_direct_nodes"`
	TotalDownstreamNodes    int                  `json:"total_downstream_nodes"`
	TotalImpactedNodes      int                  `json:"total_impacted_nodes"`
	RiskScore               float64              `json:"risk_score"`
	Summary                 string               `json:"summary"`
}

// AgentAuditSummary aggregates overall contributions and footprint of a single agent.
type AgentAuditSummary struct {
	AgentID             string          `json:"agent_id"`
	TotalNodesCreated   int             `json:"total_nodes_created"`
	LanguagesTouched    []core.Language `json:"languages_touched"`
	AssociatedSessions  []string        `json:"associated_sessions"`
	AssociatedLLMModels []string        `json:"associated_llm_models"`
	FirstSeen           time.Time       `json:"first_seen"`
	LastSeen            time.Time       `json:"last_seen"`
}

// ModelAuditSummary aggregates overall contributions from an LLM model version.
type ModelAuditSummary struct {
	LLMVersion        string          `json:"llm_version"`
	TotalNodesCreated int             `json:"total_nodes_created"`
	LanguagesTouched  []core.Language `json:"languages_touched"`
	AssociatedAgents  []string        `json:"associated_agents"`
	FirstSeen         time.Time       `json:"first_seen"`
	LastSeen          time.Time       `json:"last_seen"`
}

// AuditEngine queries lineage graphs to perform provenance audits and blast-radius impact analysis.
type AuditEngine struct {
	graph *storage.GraphEngine
}

// NewAuditEngine creates an AuditEngine instance.
func NewAuditEngine(graph *storage.GraphEngine) *AuditEngine {
	return &AuditEngine{
		graph: graph,
	}
}

// QueryNodes returns all graph nodes that match the specified AuditFilter.
func (a *AuditEngine) QueryNodes(filter AuditFilter) ([]storage.NodeRecord, error) {
	if a.graph == nil {
		return nil, fmt.Errorf("graph engine is nil")
	}

	allNodes := a.graph.ListNodes("", "")
	var matchingNodes []storage.NodeRecord

	for _, n := range allNodes {
		records := a.graph.GetLineageByNode(n.NodeID)
		if len(records) == 0 {
			continue
		}

		matched := false
		for _, r := range records {
			if filter.ExecutingAgentID != "" && r.ExecutingAgentID != filter.ExecutingAgentID {
				continue
			}
			if filter.OrchestratorAgentID != "" && r.OrchestratorAgentID != filter.OrchestratorAgentID {
				continue
			}
			if filter.LLMVersion != "" && r.LLMVersion != filter.LLMVersion {
				continue
			}
			if filter.SessionID != "" && r.SessionID != filter.SessionID {
				continue
			}
			if filter.PromptKeyword != "" && !strings.Contains(strings.ToLower(r.UserPrompt), strings.ToLower(filter.PromptKeyword)) {
				continue
			}
			if filter.PromptHash != "" {
				pSum := sha256.Sum256([]byte(r.UserPrompt))
				pHash := hex.EncodeToString(pSum[:])
				if pHash != filter.PromptHash {
					continue
				}
			}
			if !filter.TimeRangeStart.IsZero() && r.Timestamp.Before(filter.TimeRangeStart) {
				continue
			}
			if !filter.TimeRangeEnd.IsZero() && r.Timestamp.After(filter.TimeRangeEnd) {
				continue
			}

			matched = true
			break
		}

		if matched {
			matchingNodes = append(matchingNodes, n)
		}
	}

	return matchingNodes, nil
}

// ComputeBlastRadius determines the full blast-radius impact of modifying or removing all nodes matching filter.
func (a *AuditEngine) ComputeBlastRadius(filter AuditFilter, maxDepth int) (*BlastRadiusReport, error) {
	directNodes, err := a.QueryNodes(filter)
	if err != nil {
		return nil, err
	}

	if len(directNodes) == 0 {
		return &BlastRadiusReport{
			Filter:  filter,
			Summary: "No nodes match the given audit criteria. Blast radius is 0.",
		}, nil
	}

	directNodeIDs := make([]string, len(directNodes))
	directMap := make(map[string]bool)
	for i, n := range directNodes {
		directNodeIDs[i] = n.NodeID
		directMap[n.NodeID] = true
	}

	// Traverse forward across all edges to find downstream dependents
	traversal := a.graph.TraverseForward(directNodeIDs, maxDepth, nil)

	var downstreamNodes []storage.NodeRecord
	langSet := make(map[core.Language]bool)

	for _, n := range directNodes {
		langSet[n.Language] = true
	}

	for _, n := range traversal.VisitedNodes {
		if !directMap[n.NodeID] {
			downstreamNodes = append(downstreamNodes, n)
			langSet[n.Language] = true
		}
	}

	var affectedLangs []core.Language
	for lang := range langSet {
		if lang != "" {
			affectedLangs = append(affectedLangs, lang)
		}
	}

	totalDirect := len(directNodes)
	totalDownstream := len(downstreamNodes)
	totalImpacted := totalDirect + totalDownstream

	// Calculate risk score: base score proportional to nodes + language boundaries crossed
	riskScore := float64(totalDirect)*1.0 + float64(totalDownstream)*1.5 + float64(len(affectedLangs))*2.0

	summary := fmt.Sprintf("Audit matched %d direct node(s), impacting %d downstream node(s) across %d language domain(s).",
		totalDirect, totalDownstream, len(affectedLangs))

	return &BlastRadiusReport{
		Filter:                  filter,
		DirectlyTouchedNodes:    directNodes,
		DownstreamImpactedNodes: downstreamNodes,
		ImpactedEdges:           traversal.VisitedEdges,
		AffectedLanguages:       affectedLangs,
		TotalDirectNodes:        totalDirect,
		TotalDownstreamNodes:    totalDownstream,
		TotalImpactedNodes:      totalImpacted,
		RiskScore:               riskScore,
		Summary:                 summary,
	}, nil
}

// AuditAgentContributions summarizes the entire historical footprint of a specific agent ID.
func (a *AuditEngine) AuditAgentContributions(agentID string) (*AgentAuditSummary, error) {
	nodes, err := a.QueryNodes(AuditFilter{ExecutingAgentID: agentID})
	if err != nil {
		return nil, err
	}

	langMap := make(map[core.Language]bool)
	sessionMap := make(map[string]bool)
	modelMap := make(map[string]bool)
	var firstSeen, lastSeen time.Time

	for _, n := range nodes {
		langMap[n.Language] = true
		for _, r := range a.graph.GetLineageByNode(n.NodeID) {
			if r.ExecutingAgentID == agentID {
				if r.SessionID != "" {
					sessionMap[r.SessionID] = true
				}
				if r.LLMVersion != "" {
					modelMap[r.LLMVersion] = true
				}
				if firstSeen.IsZero() || r.Timestamp.Before(firstSeen) {
					firstSeen = r.Timestamp
				}
				if lastSeen.IsZero() || r.Timestamp.After(lastSeen) {
					lastSeen = r.Timestamp
				}
			}
		}
	}

	var langs []core.Language
	for l := range langMap {
		if l != "" {
			langs = append(langs, l)
		}
	}

	var sessions []string
	for s := range sessionMap {
		sessions = append(sessions, s)
	}

	var models []string
	for m := range modelMap {
		models = append(models, m)
	}

	return &AgentAuditSummary{
		AgentID:             agentID,
		TotalNodesCreated:   len(nodes),
		LanguagesTouched:    langs,
		AssociatedSessions:  sessions,
		AssociatedLLMModels: models,
		FirstSeen:           firstSeen,
		LastSeen:            lastSeen,
	}, nil
}

// AuditModelContributions summarizes the contributions of a specific LLM version.
func (a *AuditEngine) AuditModelContributions(llmVersion string) (*ModelAuditSummary, error) {
	nodes, err := a.QueryNodes(AuditFilter{LLMVersion: llmVersion})
	if err != nil {
		return nil, err
	}

	langMap := make(map[core.Language]bool)
	agentMap := make(map[string]bool)
	var firstSeen, lastSeen time.Time

	for _, n := range nodes {
		langMap[n.Language] = true
		for _, r := range a.graph.GetLineageByNode(n.NodeID) {
			if r.LLMVersion == llmVersion {
				if r.ExecutingAgentID != "" {
					agentMap[r.ExecutingAgentID] = true
				}
				if firstSeen.IsZero() || r.Timestamp.Before(firstSeen) {
					firstSeen = r.Timestamp
				}
				if lastSeen.IsZero() || r.Timestamp.After(lastSeen) {
					lastSeen = r.Timestamp
				}
			}
		}
	}

	var langs []core.Language
	for l := range langMap {
		if l != "" {
			langs = append(langs, l)
		}
	}

	var agents []string
	for ag := range agentMap {
		agents = append(agents, ag)
	}

	return &ModelAuditSummary{
		LLMVersion:        llmVersion,
		TotalNodesCreated: len(nodes),
		LanguagesTouched:  langs,
		AssociatedAgents:  agents,
		FirstSeen:         firstSeen,
		LastSeen:          lastSeen,
	}, nil
}
