package collaboration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func TestConflictEngine_DetectCollisions(t *testing.T) {
	engine := NewConflictEngine()

	symA := &core.ASTSymbolNode{
		NodeID:     "func-1",
		NodeType:   "FunctionDecl",
		Identifier: "ProcessPayment",
		ASTPayload: []byte("func ProcessPayment(amount int) bool { return true }"),
	}
	symB := &core.ASTSymbolNode{
		NodeID:     "func-1",
		NodeType:   "FunctionDecl",
		Identifier: "ProcessPayment",
		ASTPayload: []byte("func ProcessPayment(amount float64) error { return nil }"),
	}

	branchA := &core.WorkspaceManifestNode{UniverseID: "univ-a", Components: []string{}}
	branchB := &core.WorkspaceManifestNode{UniverseID: "univ-b", Components: []string{}}

	report := engine.CheckConflicts(
		nil,
		branchA,
		branchB,
		map[string]*core.ComponentNode{},
		map[string]*core.ComponentNode{},
		map[string]*core.ASTSymbolNode{"func-1": symA},
		map[string]*core.ASTSymbolNode{"func-1": symB},
	)

	if !report.HasErrors {
		t.Fatalf("expected conflict errors for divergent symbol payloads")
	}
	if len(report.Conflicts) == 0 {
		t.Fatalf("expected at least 1 conflict, got 0")
	}

	formatted := FormatConflictReport(report)
	if len(formatted) == 0 {
		t.Fatalf("expected formatted conflict string")
	}
}

func TestMultiUniverseEvaluator_AutoCollapse(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fg-eval-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	blobStore, _ := storage.NewBlobStore(filepath.Join(tmpDir, "objects"))
	graphEngine, _ := storage.NewGraphEngine(filepath.Join(tmpDir, "graph.db"))
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	evaluator := NewMultiUniverseEvaluator(universeMgr, blobStore, graphEngine)

	_, _ = universeMgr.CreateUniverse("universe-main", "")
	_, _ = universeMgr.CreateUniverse("universe-candidate-1", "universe-main")
	_, _ = universeMgr.CreateUniverse("universe-candidate-2", "universe-main")

	// Commit candidate manifests
	m1 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-1",
		UniverseID:  "universe-candidate-1",
		Components:  []string{},
		Lineage: core.LineageEnvelope{
			ExecutingAgentID: "agent-1",
			Intent:           "Approach 1",
			Timestamp:        time.Now().UTC(),
		},
	}
	_, _ = universeMgr.CommitManifest("universe-candidate-1", m1)

	m2 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-2",
		UniverseID:  "universe-candidate-2",
		Components:  []string{},
		Lineage: core.LineageEnvelope{
			ExecutingAgentID: "agent-2",
			Intent:           "Approach 2 - High fitness",
			Timestamp:        time.Now().UTC(),
		},
	}
	_, _ = universeMgr.CommitManifest("universe-candidate-2", m2)

	candidates := []*UniverseCandidate{
		{
			UniverseID:    "universe-candidate-1",
			AgentID:       "agent-1",
			TestPassRate:  0.80,
			SecurityScore: 0.70,
			LatencyMs:     250,
			TokenCost:     1200,
		},
		{
			UniverseID:    "universe-candidate-2",
			AgentID:       "agent-2",
			TestPassRate:  1.00,
			SecurityScore: 0.98,
			LatencyMs:     50,
			TokenCost:     450,
		},
	}

	winner, err := evaluator.EvaluateAndCollapse("universe-main", candidates)
	if err != nil {
		t.Fatalf("EvaluateAndCollapse failed: %v", err)
	}

	if winner.UniverseID != "universe-candidate-2" {
		t.Fatalf("expected candidate-2 to win, got %s (score: %.3f)", winner.UniverseID, winner.CompositeScore)
	}

	// Verify universe-main was updated
	head, err := universeMgr.GetUniverseManifest("universe-main")
	if err != nil || head == nil {
		t.Fatalf("failed to read collapsed universe-main head: %v", err)
	}
	if head.Lineage.ExecutingAgentID != "multi-universe-evaluator" {
		t.Fatalf("expected evaluator lineage on collapsed head, got: %+v", head.Lineage)
	}
}
