package topocosm

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/distributed"
	"github.com/cosmscm/cosm/pkg/topocosm/backplane"
)

func setupTestHub(t *testing.T) (*HubServer, *HubClient, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "cosm-topocosm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	bp, err := backplane.NewLocalBackplane(tempDir)
	if err != nil {
		t.Fatalf("failed to init local backplane: %v", err)
	}

	server := NewHubServer(bp)
	client := NewInProcessHubClient(server.Handler(), "did:key:z6MkuTestUser")

	cleanup := func() {
		client.Close()
		_ = bp.Close()
		_ = os.RemoveAll(tempDir)
	}

	return server, client, cleanup
}

func TestAgentDiscovery(t *testing.T) {
	_, client, cleanup := setupTestHub(t)
	defer cleanup()

	ctx := context.Background()
	manifest, err := client.GetAgentDiscoveryManifest(ctx)
	if err != nil {
		t.Fatalf("GetAgentDiscoveryManifest failed: %v", err)
	}

	if manifest.CosmVersion == "" {
		t.Errorf("expected non-empty cosm_version")
	}
	if !manifest.Capabilities["sparse_sync"] {
		t.Errorf("expected sparse_sync capability to be true")
	}
	if !manifest.Capabilities["blackboard_crdt"] {
		t.Errorf("expected blackboard_crdt capability to be true")
	}
	if manifest.AttestationPolicy.MinCriticScore < 80.0 {
		t.Errorf("expected MinCriticScore >= 80.0, got %f", manifest.AttestationPolicy.MinCriticScore)
	}
}

func TestUserEnrollmentAndSettings(t *testing.T) {
	_, client, cleanup := setupTestHub(t)
	defer cleanup()

	ctx := context.Background()
	userDID := "did:key:z6MkuJasonDev"
	err := client.Enroll(ctx, userDID, "jason", "jason@test.com", "Jason D", "ed25519-mock-pub", false)
	if err != nil {
		t.Fatalf("Enroll failed: %v", err)
	}

	// Authenticate as Jason
	jasonClient := NewInProcessHubClient(client.Handler(), userDID)
	defer jasonClient.Close()

	whoami, err := jasonClient.WhoAmI(ctx)
	if err != nil {
		t.Fatalf("WhoAmI failed: %v", err)
	}
	if whoami["type"] != "user" {
		t.Errorf("expected identity type user, got %v", whoami["type"])
	}
}

func TestPublishAndPullRoundtrip(t *testing.T) {
	_, client, cleanup := setupTestHub(t)
	defer cleanup()

	ctx := context.Background()
	payload := createPolyglotPayload("demo-org", "cloud-platform", "Enterprise Polyglot Microservices")

	pubResp, err := client.PublishCosm(ctx, payload)
	if err != nil {
		t.Fatalf("PublishCosm failed: %v", err)
	}

	if !pubResp.Success {
		t.Errorf("expected publish to succeed")
	}
	if pubResp.MerkleRootHash == "" {
		t.Errorf("expected non-empty MerkleRootHash")
	}

	// Pull Full Cosm
	pullResp, err := client.PullCosm(ctx, "demo-org", "cloud-platform", "universe-main")
	if err != nil {
		t.Fatalf("PullCosm failed: %v", err)
	}

	if pullResp.Manifest == nil {
		t.Fatalf("expected non-nil manifest in pull response")
	}
	if len(pullResp.Components) != 3 {
		t.Errorf("expected 3 components, got %d", len(pullResp.Components))
	}
	if len(pullResp.Symbols) != 3 {
		t.Errorf("expected 3 symbols, got %d", len(pullResp.Symbols))
	}
}

func TestSparseSubtreePull(t *testing.T) {
	_, client, cleanup := setupTestHub(t)
	defer cleanup()

	ctx := context.Background()
	payload := createPolyglotPayload("demo-org", "cloud-platform", "Enterprise Polyglot Microservices")
	_, err := client.PublishCosm(ctx, payload)
	if err != nil {
		t.Fatalf("PublishCosm failed: %v", err)
	}

	// Sparse Pull requesting only Go services/billing
	sparseReq := &SparsePullRequest{
		OrgSlug:        "demo-org",
		CosmName:       "cloud-platform",
		UniverseID:     "universe-main",
		ComponentNames: []string{"services/billing"},
	}

	sparseResp, err := client.SparsePullCosm(ctx, sparseReq)
	if err != nil {
		t.Fatalf("SparsePullCosm failed: %v", err)
	}

	if len(sparseResp.Components) != 1 {
		t.Errorf("expected 1 filtered component, got %d", len(sparseResp.Components))
	}
	if sparseResp.SavingsPercent <= 0 {
		t.Errorf("expected positive savings percentage, got %f", sparseResp.SavingsPercent)
	}
}

func TestProposalsAndCriticReview(t *testing.T) {
	_, client, cleanup := setupTestHub(t)
	defer cleanup()

	ctx := context.Background()
	payload := createPolyglotPayload("demo-org", "cloud-platform", "Enterprise Polyglot Microservices")
	_, err := client.PublishCosm(ctx, payload)
	if err != nil {
		t.Fatalf("PublishCosm failed: %v", err)
	}

	// Create Proposal
	lineage := core.LineageEnvelope{
		ExecutingAgentID: "agent-01",
		Intent:           "Add stripe webhook handler",
		Timestamp:        time.Now().UTC(),
	}

	prop, err := client.CreateProposal(ctx, "demo-org", "cloud-platform", "Feature: Stripe Billing Webhook", "universe-main", "universe-main", lineage)
	if err != nil {
		t.Fatalf("CreateProposal failed: %v", err)
	}

	if prop.ID == "" {
		t.Errorf("expected non-empty proposal ID")
	}
	if prop.FitnessScore < 80.0 {
		t.Errorf("expected fitness score >= 80 from critic oracle, got %f", prop.FitnessScore)
	}

	// Submit Human Review
	review := &ReviewSubmission{
		ProposalID:   prop.ID,
		ReviewerDID:  "did:key:z6MkuHumanReviewer",
		Approved:     true,
		FitnessScore: 99.0,
		Comments: []distributed.ReviewComment{
			{
				CommentID: "comm-test-1",
				AuthorDID: "did:key:z6MkuHumanReviewer",
				Body:      "Looks great. Approved for merge.",
				Severity:  "INFO",
				Timestamp: time.Now().UTC(),
			},
		},
	}

	reviewedProp, err := client.SubmitReview(ctx, "demo-org", "cloud-platform", review)
	if err != nil {
		t.Fatalf("SubmitReview failed: %v", err)
	}
	if !reviewedProp.Approvals["did:key:z6MkuHumanReviewer"] {
		t.Errorf("expected human approval to be recorded")
	}

	// Merge Proposal
	mergeRes, err := client.MergeProposal(ctx, "demo-org", "cloud-platform", prop.ID)
	if err != nil {
		t.Fatalf("MergeProposal failed: %v", err)
	}
	if mergeRes["status"] != "MERGED" {
		t.Errorf("expected status MERGED, got %v", mergeRes["status"])
	}
}

func TestBlackboardClaimAndConflict(t *testing.T) {
	_, client, cleanup := setupTestHub(t)
	defer cleanup()

	ctx := context.Background()
	payload := createPolyglotPayload("demo-org", "cloud-platform", "Enterprise Polyglot Microservices")
	_, err := client.PublishCosm(ctx, payload)
	if err != nil {
		t.Fatalf("PublishCosm failed: %v", err)
	}

	agent1Client := NewInProcessHubClient(client.Handler(), "did:key:z6MkuAgent001")
	defer agent1Client.Close()

	agent2Client := NewInProcessHubClient(client.Handler(), "did:key:z6MkuAgent002")
	defer agent2Client.Close()

	// Agent 1 claims domain
	ok, err := agent1Client.ClaimDomain(ctx, "demo-org", "cloud-platform", "services/billing", "Migrating database schemas", 60)
	if err != nil || !ok {
		t.Fatalf("Agent 1 claim failed: %v", err)
	}

	// Agent 2 attempts to claim same domain -> must fail
	ok2, _ := agent2Client.ClaimDomain(ctx, "demo-org", "cloud-platform", "services/billing", "Conflicting refactor", 60)
	if ok2 {
		t.Errorf("expected Agent 2 claim to be rejected due to conflict")
	}

	// Agent 1 releases claim
	err = agent1Client.ReleaseDomain(ctx, "demo-org", "cloud-platform", "services/billing")
	if err != nil {
		t.Fatalf("Agent 1 release failed: %v", err)
	}

	// Agent 2 now succeeds
	ok3, err := agent2Client.ClaimDomain(ctx, "demo-org", "cloud-platform", "services/billing", "Acquiring after release", 60)
	if err != nil || !ok3 {
		t.Fatalf("Agent 2 claim after release failed: %v", err)
	}
}

func TestSeederAndSwarmSimulation(t *testing.T) {
	server, client, cleanup := setupTestHub(t)
	defer cleanup()

	ctx := context.Background()

	// Run Seeder
	err := SeedLocalHub(ctx, server, DefaultSeedOptions())
	if err != nil {
		t.Fatalf("SeedLocalHub failed: %v", err)
	}

	// Verify cosms seeded
	cosms, err := client.ListCosms(ctx)
	if err != nil {
		t.Fatalf("ListCosms failed: %v", err)
	}
	if len(cosms) < 2 {
		t.Errorf("expected at least 2 seeded cosms, got %d", len(cosms))
	}

	// Run Swarm Simulation
	simCfg := SwarmSimulationConfig{
		NumAgents:   10,
		Concurrency: 4,
		Duration:    1 * time.Second,
		OrgSlug:     "demo-org",
		CosmName:    "cloud-platform",
	}

	res, err := RunSwarmSimulation(ctx, server, simCfg)
	if err != nil {
		t.Fatalf("RunSwarmSimulation failed: %v", err)
	}

	if res.TotalOperations <= 0 {
		t.Errorf("expected positive total operations, got %d", res.TotalOperations)
	}
	if res.OpsPerSecond <= 0 {
		t.Errorf("expected positive ops per second")
	}
}
