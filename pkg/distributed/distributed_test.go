package distributed

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func TestProposalCOB_CRDTMerge(t *testing.T) {
	lineage := core.LineageEnvelope{
		UserID:           "dev-alice",
		UserPrompt:       "Add auth service middleware",
		ExecutingAgentID: "agent-alpha",
		Timestamp:        time.Now().UTC(),
	}

	// Replica 1: Agent creates proposal
	cobA := NewProposalCOB("prop-101", "Add Auth Service", "universe-main", "did:key:agent-alpha", "u/auth-1", "manifest-hash-1", lineage)

	// Replica 2: Peer receives cobA
	cobB := NewProposalCOB("prop-101", "Add Auth Service", "universe-main", "did:key:agent-alpha", "u/auth-1", "manifest-hash-1", lineage)

	// Replica 1 adds a comment and an approval
	cobA.AddComment(ReviewComment{
		AuthorDID: "did:key:reviewer-bob",
		Body:      "Looks solid, verify token expiration.",
		Severity:  "INFO",
	})
	cobA.SetApproval("did:key:reviewer-bob", true)

	// Replica 2 concurrently adds a revision
	cobB.AddRevision("u/auth-1-v2", "manifest-hash-2", "did:key:agent-alpha", "Add token refresh handler")

	// Merge both ways
	cobA.MergeCRDT(cobB)
	cobB.MergeCRDT(cobA)

	// Assert convergence
	if len(cobA.Revisions) != 2 || len(cobB.Revisions) != 2 {
		t.Fatalf("expected 2 revisions on both replicas, got A=%d, B=%d", len(cobA.Revisions), len(cobB.Revisions))
	}
	if len(cobA.Comments) != 1 || len(cobB.Comments) != 1 {
		t.Fatalf("expected 1 comment on both replicas, got A=%d, B=%d", len(cobA.Comments), len(cobB.Comments))
	}
	if !cobA.Approvals["did:key:reviewer-bob"] || !cobB.Approvals["did:key:reviewer-bob"] {
		t.Fatalf("expected approval to be converged on both replicas")
	}
}

func TestBlackboardCOB_ConcurrencyControl(t *testing.T) {
	bbA := NewBlackboardCOB()
	bbB := NewBlackboardCOB()

	// Agent 1 claims services/billing
	ok1 := bbA.ClaimDomain("services/billing", "did:key:agent-1", "Implement Stripe Webhook")
	if !ok1 {
		t.Fatalf("expected first claim to succeed")
	}

	// Another agent on same blackboard tries to claim same domain
	ok2 := bbA.ClaimDomain("services/billing", "did:key:agent-2", "Refactor Billing Routes")
	if ok2 {
		t.Fatalf("expected concurrent claim on same domain to be rejected")
	}

	// Agent 2 claims frontend
	bbB.ClaimDomain("web/BillingView.tsx", "did:key:agent-2", "Add Billing UI Component")

	// Merge blackboards
	bbA.MergeCRDT(bbB)

	if bbA.DomainClaims["web/BillingView.tsx"] != "did:key:agent-2" {
		t.Fatalf("expected merged blackboard to contain frontend claim")
	}
	if bbA.DomainClaims["services/billing"] != "did:key:agent-1" {
		t.Fatalf("expected merged blackboard to retain billing claim")
	}
}

func TestStackedProposals_AutoEvolution(t *testing.T) {
	tmpDir := t.TempDir()
	blobStore, err := storage.NewBlobStore(filepath.Join(tmpDir, "objects"))
	if err != nil {
		t.Fatalf("failed to create blobstore: %v", err)
	}
	graphEngine, err := storage.NewGraphEngine(filepath.Join(tmpDir, "graph.db"))
	if err != nil {
		t.Fatalf("failed to create graphengine: %v", err)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)

	// Create root universe and 3 stacked universes
	_, _ = universeMgr.CreateUniverse("universe-main", "")
	_, _ = universeMgr.CreateUniverse("u/change-1", "universe-main")
	_, _ = universeMgr.CreateUniverse("u/change-2", "u/change-1")
	_, _ = universeMgr.CreateUniverse("u/change-3", "u/change-2")

	stackMgr := NewStackManager(universeMgr, blobStore)

	// Register 3 stacked proposals
	_ = stackMgr.RegisterChange(&StackedProposal{
		ChangeID:       "c/auth-base",
		ProposalID:     "prop-1",
		Title:          "Change 1: Auth Base",
		ParentChangeID: "universe-main",
		UniverseID:     "u/change-1",
		ManifestHash:   "hash-root-1",
		OrderIndex:     0,
	})
	_ = stackMgr.RegisterChange(&StackedProposal{
		ChangeID:       "c/auth-middleware",
		ProposalID:     "prop-2",
		Title:          "Change 2: Auth Middleware",
		ParentChangeID: "c/auth-base",
		UniverseID:     "u/change-2",
		ManifestHash:   "hash-root-2",
		OrderIndex:     1,
	})
	_ = stackMgr.RegisterChange(&StackedProposal{
		ChangeID:       "c/auth-routes",
		ProposalID:     "prop-3",
		Title:          "Change 3: Auth Routes",
		ParentChangeID: "c/auth-middleware",
		UniverseID:     "u/change-3",
		ManifestHash:   "hash-root-3",
		OrderIndex:     2,
	})

	stackList := stackMgr.ListStack()
	if len(stackList) != 3 {
		t.Fatalf("expected 3 stacked changes, got %d", len(stackList))
	}

	// Verify descendants
	desc := stackMgr.GetDescendants("c/auth-base")
	if len(desc) != 2 {
		t.Fatalf("expected 2 descendants of c/auth-base, got %d", len(desc))
	}
	if desc[0].ChangeID != "c/auth-middleware" || desc[1].ChangeID != "c/auth-routes" {
		t.Fatalf("unexpected descendant order: %v, %v", desc[0].ChangeID, desc[1].ChangeID)
	}

	// Trigger Auto-Evolution of the stack when c/auth-base is modified
	evolveRes, err := stackMgr.EvolveStack("c/auth-base", "hash-root-1-updated")
	if err != nil {
		t.Fatalf("evolve stack failed: %v", err)
	}
	if evolveRes.TriggerChangeID != "c/auth-base" {
		t.Fatalf("unexpected trigger change ID: %s", evolveRes.TriggerChangeID)
	}
}

func TestASTConflictNode_RepresentationAndResolution(t *testing.T) {
	conflict := NewASTConflictNode(
		"sym-calc-tax-10",
		"services/billing.CalculateTax",
		core.LangGo,
		"base-hash-99",
		[]ConflictingVersion{
			{
				AgentID:    "agent-speed",
				UniverseID: "u/speed-opt",
				ASTHash:    "ast-hash-speed",
				ASTPayload: []byte("func CalculateTax() float64 { return rate * 1.05 }"),
				Timestamp:  time.Now().UTC(),
			},
			{
				AgentID:    "agent-precision",
				UniverseID: "u/precision-opt",
				ASTHash:    "ast-hash-precision",
				ASTPayload: []byte("func CalculateTax() decimal.Decimal { return rate.Mul(1.05) }"),
				Timestamp:  time.Now().UTC(),
			},
		},
	)

	if conflict.IsResolved() {
		t.Fatalf("conflict should start unresolved")
	}
	if len(conflict.Conflicting) != 2 {
		t.Fatalf("expected 2 conflicting versions")
	}

	// Resolve with precision agent
	conflict.Resolve(conflict.Conflicting[1], "AUTO_RESOLVED")
	if !conflict.IsResolved() {
		t.Fatalf("conflict should be resolved")
	}
	if conflict.Resolution.AgentID != "agent-precision" {
		t.Fatalf("expected agent-precision winner")
	}
}
