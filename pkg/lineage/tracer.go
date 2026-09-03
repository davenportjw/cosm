package lineage

import (
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

// AncestryHop represents a single point in the causal lineage provenance pedigree.
type AncestryHop struct {
	HopIndex            int                  `json:"hop_index"`
	NodeID              string               `json:"node_id"`
	Lineage             core.LineageEnvelope `json:"lineage"`
	Timestamp           time.Time            `json:"timestamp"`
	UserPrompt          string               `json:"user_prompt"`
	SessionID           string               `json:"session_id"`
	OrchestratorAgentID string               `json:"orchestrator_agent_id"`
	ExecutingAgentID    string               `json:"executing_agent_id"`
	LLMVersion          string               `json:"llm_version"`
	Intent              string               `json:"intent"`
	HasSignature        bool                 `json:"has_signature"`
}

// AncestryChain represents the full backward causal chain from an AST node to its root prompt.
type AncestryChain struct {
	TargetNodeID       string        `json:"target_node_id"`
	Hops               []AncestryHop `json:"hops"`
	RootPrompt         string        `json:"root_prompt"`
	RootSessionID      string        `json:"root_session_id"`
	RootOrchestratorID string        `json:"root_orchestrator_id"`
	PrimaryExecutorID  string        `json:"primary_executor_id"`
	LLMVersion         string        `json:"llm_version"`
	IntactChain        bool          `json:"intact_chain"`
	TotalHops          int           `json:"total_hops"`
}

// LineageTracer resolves full-stack ancestry graphs connecting AST nodes back to user prompts and agent sessions.
type LineageTracer struct {
	graph *storage.GraphEngine
}

// NewLineageTracer creates a new LineageTracer using the provided GraphEngine.
func NewLineageTracer(graph *storage.GraphEngine) *LineageTracer {
	return &LineageTracer{
		graph: graph,
	}
}

// TraceNodeAncestry traces the causal history of a given nodeID.
func (t *LineageTracer) TraceNodeAncestry(nodeID string) (*AncestryChain, error) {
	if t.graph == nil {
		return nil, fmt.Errorf("graph engine is nil")
	}

	records := t.graph.GetLineageByNode(nodeID)
	if len(records) == 0 {
		// Attempt to check if node exists
		node, err := t.graph.GetNode(nodeID)
		if err != nil {
			return nil, fmt.Errorf("node not found in graph: %s: %w", nodeID, err)
		}

		return &AncestryChain{
			TargetNodeID: node.NodeID,
			Hops:         nil,
			IntactChain:  false,
			TotalHops:    0,
		}, nil
	}

	var hops []AncestryHop
	for idx, r := range records {
		env := core.LineageEnvelope{
			UserID:              r.UserID,
			UserPrompt:          r.UserPrompt,
			SessionID:           r.SessionID,
			OrchestratorAgentID: r.OrchestratorAgentID,
			ExecutingAgentID:    r.ExecutingAgentID,
			LLMVersion:          r.LLMVersion,
			GenerationParams:    r.GenerationParams,
			Intent:              r.Intent,
			Timestamp:           r.Timestamp,
			Tokens:              r.Tokens,
			Trace:               r.Trace,
			SignatureEd25519:    r.SignatureEd25519,
		}

		hops = append(hops, AncestryHop{
			HopIndex:            idx,
			NodeID:              r.NodeID,
			Lineage:             env,
			Timestamp:           r.Timestamp,
			UserPrompt:          r.UserPrompt,
			SessionID:           r.SessionID,
			OrchestratorAgentID: r.OrchestratorAgentID,
			ExecutingAgentID:    r.ExecutingAgentID,
			LLMVersion:          r.LLMVersion,
			Intent:              r.Intent,
			HasSignature:        len(r.SignatureEd25519) > 0,
		})
	}

	root := hops[0]
	latest := hops[len(hops)-1]

	chain := &AncestryChain{
		TargetNodeID:       nodeID,
		Hops:               hops,
		RootPrompt:         root.UserPrompt,
		RootSessionID:      root.SessionID,
		RootOrchestratorID: root.OrchestratorAgentID,
		PrimaryExecutorID:  latest.ExecutingAgentID,
		LLMVersion:         latest.LLMVersion,
		IntactChain:        true,
		TotalHops:          len(hops),
	}

	return chain, nil
}

// TraceComponentAncestry traces all symbol nodes belonging to a ComponentNode.
func (t *LineageTracer) TraceComponentAncestry(comp *core.ComponentNode) (map[string]*AncestryChain, error) {
	if comp == nil {
		return nil, fmt.Errorf("component is nil")
	}

	chains := make(map[string]*AncestryChain)
	for _, symID := range comp.SymbolNodes {
		chain, err := t.TraceNodeAncestry(symID)
		if err != nil {
			return nil, fmt.Errorf("failed to trace ancestry for symbol %s: %w", symID, err)
		}
		chains[symID] = chain
	}

	return chains, nil
}

// VerifyAncestryIntegrity validates that all hops in the ancestry chain have valid Ed25519 cryptographic signatures.
func (t *LineageTracer) VerifyAncestryIntegrity(chain *AncestryChain, pubKey ed25519.PublicKey) (bool, []error) {
	if chain == nil || len(chain.Hops) == 0 {
		return false, []error{fmt.Errorf("chain is empty")}
	}

	var errs []error
	for _, hop := range chain.Hops {
		if !hop.HasSignature {
			errs = append(errs, fmt.Errorf("hop %d (node %s) has no signature", hop.HopIndex, hop.NodeID))
			continue
		}

		valid, err := VerifyEnvelope(&hop.Lineage, pubKey)
		if err != nil || !valid {
			errs = append(errs, fmt.Errorf("hop %d (node %s) signature verification failed: %v", hop.HopIndex, hop.NodeID, err))
		}
	}

	return len(errs) == 0, errs
}
