package collaboration

import (
	"fmt"
	"sort"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

// UniverseCandidate represents an explored parallel micro-universe in MCTS tree.
type UniverseCandidate struct {
	UniverseID     string   `json:"universe_id"`
	AgentID        string   `json:"agent_id"`
	PromptIntent   string   `json:"prompt_intent"`
	TestPassRate   float64  `json:"test_pass_rate"`   // 0.0 to 1.0
	LatencyMs      int64    `json:"latency_ms"`       // Benchmark execution time
	TokenCost      int      `json:"token_cost"`       // Tokens consumed
	SecurityScore  float64  `json:"security_score"`   // 0.0 to 1.0
	CompositeScore float64  `json:"composite_score"`  // Weighted overall fitness
	Selected       bool     `json:"selected"`
}

// MultiUniverseEvaluator benchmarks candidate micro-universes and auto-collapses to the winner.
type MultiUniverseEvaluator struct {
	universeMgr *storage.UniverseManager
	blobStore   *storage.BlobStore
	graphEngine *storage.GraphEngine
}

// NewMultiUniverseEvaluator creates a new MultiUniverseEvaluator.
func NewMultiUniverseEvaluator(
	u *storage.UniverseManager,
	b *storage.BlobStore,
	g *storage.GraphEngine,
) *MultiUniverseEvaluator {
	return &MultiUniverseEvaluator{
		universeMgr: u,
		blobStore:   b,
		graphEngine: g,
	}
}

// ScoreCandidate calculates the weighted composite fitness score for a candidate universe.
func (e *MultiUniverseEvaluator) ScoreCandidate(c *UniverseCandidate) float64 {
	// Weights: Test Correctness (50%), Security (30%), Performance Latency (10%), Token Efficiency (10%)
	testWeight := 0.50
	secWeight := 0.30
	latencyWeight := 0.10
	tokenWeight := 0.10

	// Latency score: 1000ms -> 0.0, 0ms -> 1.0
	normLatency := 1.0 - (float64(c.LatencyMs) / 1000.0)
	if normLatency < 0 {
		normLatency = 0
	}

	// Token score: 10000 tokens -> 0.0, 0 tokens -> 1.0
	normTokens := 1.0 - (float64(c.TokenCost) / 10000.0)
	if normTokens < 0 {
		normTokens = 0
	}

	score := (c.TestPassRate * testWeight) +
		(c.SecurityScore * secWeight) +
		(normLatency * latencyWeight) +
		(normTokens * tokenWeight)

	c.CompositeScore = score
	return score
}

// EvaluateAndCollapse evaluates N micro-universes, ranks them, and collapses the active head to the winner.
func (e *MultiUniverseEvaluator) EvaluateAndCollapse(
	targetUniverseID string,
	candidates []*UniverseCandidate,
) (*UniverseCandidate, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates provided for evaluation")
	}

	for _, c := range candidates {
		e.ScoreCandidate(c)
	}

	// Rank descending by composite score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CompositeScore > candidates[j].CompositeScore
	})

	winner := candidates[0]
	winner.Selected = true

	// If winner is from another universe, collapse targetUniverseID to winner's head manifest
	winnerManifest, err := e.universeMgr.GetUniverseManifest(winner.UniverseID)
	if err == nil && winnerManifest != nil {
		collapsedManifest := &core.WorkspaceManifestNode{
			WorkspaceID: winnerManifest.WorkspaceID,
			UniverseID:  targetUniverseID,
			Components:  winnerManifest.Components,
			CrossEdges:  winnerManifest.CrossEdges,
			Lineage: core.LineageEnvelope{
				ExecutingAgentID: "multi-universe-evaluator",
				Intent: fmt.Sprintf("Auto-collapsed %d universes to winner %s (score: %.3f)",
					len(candidates), winner.UniverseID, winner.CompositeScore),
				Timestamp: time.Now().UTC(),
			},
		}
		_, _ = e.universeMgr.CommitManifest(targetUniverseID, collapsedManifest)
	}

	return winner, nil
}
