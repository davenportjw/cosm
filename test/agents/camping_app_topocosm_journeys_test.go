package agents_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/distributed"
	"github.com/cosmscm/cosm/pkg/storage"
	"github.com/cosmscm/cosm/pkg/topocosm"
	"github.com/cosmscm/cosm/pkg/topocosm/backplane"
)

// TestCampingApp20_TopocosmJourneys executes the end-to-end multi-track collaboration journeys
// for Camping App 2.0 on Topocosm Hub, covering:
// 1. Publishing polyglot baseline (Go backend, React frontend, HCL infra, SQL db)
// 2. Track A: Agent Alice (Gear Rentals backend, blackboard leasing, sparse clone, AST surgery, Critic review, merge)
// 3. Track B: Developer Bob (Campsite Reviews stacked proposals c/reviews-api -> c/reviews-ui, Jujutsu auto-evolution)
// 4. Track C: Infra Agent Charlie (Redis Cache Memorystore in Terraform HCL, sparse clone, merge)
// 5. Verifying CRDT Merkle-DAG union and zero active locks
func TestCampingApp20_TopocosmJourneys(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Setup in-process Topocosm Hub Server with pure-Go WAL backplane
	hubDir, err := os.MkdirTemp("", "topocosm-camping-hub-*")
	if err != nil {
		t.Fatalf("failed creating hub temp dir: %v", err)
	}
	defer os.RemoveAll(hubDir)

	bp, err := backplane.NewLocalBackplane(hubDir)
	if err != nil {
		t.Fatalf("failed creating local backplane: %v", err)
	}
	defer bp.Close()

	hubServer := topocosm.NewHubServer(bp)
	hubHandler := hubServer.Handler()
	orgSlug := "davenport-boutique"
	cosmName := "camping-app"

	// 2. Seed Baseline Camping App 2.0 Repository into Topocosm Hub
	client := topocosm.NewInProcessHubClient(hubHandler, "did:key:z6MkuHubOrchestrator")

	baseLineage := core.LineageEnvelope{
		UserID:              "orchestrator",
		UserPrompt:          "Baseline Camping App 2.0 Architecture",
		SessionID:           "sess-bootstrap-001",
		OrchestratorAgentID: "orchestrator",
		ExecutingAgentID:    "orchestrator",
		LLMVersion:          "gemini-3.8-flash",
		Intent:              "Initial baseline publish",
		Timestamp:           time.Now(),
	}

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-camping-app",
		UniverseID:  "universe-main",
		Components:  make([]string, 0),
		CrossEdges:  make([]core.CrossBoundaryEdge, 0),
		Lineage:     baseLineage,
		CreatedAt:   time.Now(),
	}

	components := make(map[string]*core.ComponentNode)
	symbols := make(map[string]*core.ASTSymbolNode)
	blobs := make(map[string][]byte)

	// Baseline Go Server component
	goSource := `package main

import (
	"encoding/json"
	"net/http"
)

type Server struct{}

func NewServer() *Server { return &Server{} }

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("HEALTHY"))
}

func (s *Server) HandleCreateBooking(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Reservation Confirmed"))
}

func (s *Server) HandleCancelBooking(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Reservation Cancelled"))
}
`
	parsedGo, err := codecs.ParseSourceFile("cmd/server/main.go", []byte(goSource), baseLineage)
	if err != nil {
		t.Fatalf("failed parsing baseline Go file: %v", err)
	}
	goComp := parsedGo.Component
	compBytes, _ := json.Marshal(goComp)
	blobs[goComp.ComponentID] = compBytes
	components[goComp.ComponentID] = goComp
	manifest.Components = append(manifest.Components, goComp.ComponentID)
	for id, s := range parsedGo.Symbols {
		symbols[id] = s
		sBytes, _ := json.Marshal(s)
		blobs[id] = sBytes
	}

	// Baseline React component
	tsSource := `import React from 'react';

export function BookingDashboard() {
  return <div className="dashboard">Campsite Bookings</div>;
}

export function App() {
  return <div className="app"><BookingDashboard /></div>;
}
`
	parsedTS, err := codecs.ParseSourceFile("web/src/App.tsx", []byte(tsSource), baseLineage)
	if err == nil && parsedTS != nil && parsedTS.Component != nil {
		tsComp := parsedTS.Component
		compBytes, _ := json.Marshal(tsComp)
		blobs[tsComp.ComponentID] = compBytes
		components[tsComp.ComponentID] = tsComp
		manifest.Components = append(manifest.Components, tsComp.ComponentID)
		for id, s := range parsedTS.Symbols {
			symbols[id] = s
			sBytes, _ := json.Marshal(s)
			blobs[id] = sBytes
		}
	}

	// Baseline HCL Infra component
	hclSource := `resource "google_compute_network" "vpc" {
  name = "camping-vpc"
}

resource "google_cloud_run_service" "app" {
  name     = "camping-app"
  location = "us-central1"
}
`
	parsedHCL, err := codecs.ParseSourceFile("infra/main.tf", []byte(hclSource), baseLineage)
	if err == nil && parsedHCL != nil && parsedHCL.Component != nil {
		hclComp := parsedHCL.Component
		compBytes, _ := json.Marshal(hclComp)
		blobs[hclComp.ComponentID] = compBytes
		components[hclComp.ComponentID] = hclComp
		manifest.Components = append(manifest.Components, hclComp.ComponentID)
		for id, s := range parsedHCL.Symbols {
			symbols[id] = s
			sBytes, _ := json.Marshal(s)
			blobs[id] = sBytes
		}
	}

	baseMerkleRoot, _ := core.HashWorkspaceManifest(manifest)
	manifest.MerkleRootHash = baseMerkleRoot

	// Publish baseline to Topocosm Hub
	pubResp, err := client.PublishCosm(ctx, &topocosm.PublishPayload{
		OrgSlug:    orgSlug,
		CosmName:   cosmName,
		UniverseID: "universe-main",
		Manifest:   manifest,
		Components: components,
		Symbols:    symbols,
		Blobs:      blobs,
		Lineage:    baseLineage,
		IsInitial:  true,
		Visibility: topocosm.VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("failed publishing baseline to hub: %v", err)
	}
	t.Logf("✓ Baseline published to Topocosm Hub: MerkleRoot=%s (BlobsStored=%d)", pubResp.MerkleRootHash, pubResp.BlobsStored)

	// =========================================================================
	// Track A: Autonomous Agent Alice (Gear Rentals Backend)
	// =========================================================================
	t.Run("Track_A_AgentAlice_GearRentals", func(t *testing.T) {
		aliceDID := "did:key:z6MkuAgentAlice"
		aliceClient := topocosm.NewInProcessHubClient(hubHandler, aliceDID)

		// 1. Claim Blackboard domain lease on services/rentals
		claimed, err := aliceClient.ClaimDomain(ctx, orgSlug, cosmName, "services/rentals", "Implement gear rental catalog and reservation routes", 600)
		if err != nil || !claimed {
			t.Fatalf("Alice failed claiming domain lease: %v", err)
		}
		t.Logf("  [Alice] Blackboard lease acquired: Domain=services/rentals")

		// 2. Sparse pull only cmd/server/main.go
		sparseResp, err := aliceClient.SparsePullCosm(ctx, &topocosm.SparsePullRequest{
			OrgSlug:        orgSlug,
			CosmName:       cosmName,
			UniverseID:     "universe-main",
			ComponentNames: []string{"cmd/server/main.go"},
		})
		if err != nil {
			t.Fatalf("Alice failed sparse pull: %v", err)
		}
		if len(sparseResp.Components) != 1 {
			t.Fatalf("Alice expected 1 component from sparse pull, got %d", len(sparseResp.Components))
		}
		t.Logf("  [Alice] Sparse pull succeeded with %.1f%% bandwidth savings", sparseResp.SavingsPercent)

		// 3. Incept AST mutation: HandleGearRentals
		rentalsSource := goSource + `
// HandleGearRentals returns available outdoor gear for rent
func (s *Server) HandleGearRentals(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("[{\"id\":\"g1\",\"name\":\"2-Person Tent\"}]"))
}
`
		aliceLineage := core.LineageEnvelope{
			UserID:              "alice",
			UserPrompt:          "Implement gear rental catalog and reservation routes",
			SessionID:           "sess-alice-rentals-01",
			OrchestratorAgentID: "agent-alice-gear",
			ExecutingAgentID:    "agent-alice-gear",
			LLMVersion:          "gemini-3.8-flash",
			Intent:              "feat(rentals): Add gear rental endpoints",
			Timestamp:           time.Now(),
		}

		parsedRentals, err := codecs.ParseSourceFile("cmd/server/main.go", []byte(rentalsSource), aliceLineage)
		if err != nil {
			t.Fatalf("Alice failed parsing mutated Go file: %v", err)
		}
		rentalsComp := parsedRentals.Component

		aliceManifest := &core.WorkspaceManifestNode{
			WorkspaceID: manifest.WorkspaceID,
			UniverseID:  "universe-alice-rentals",
			Components:  []string{rentalsComp.ComponentID},
			Lineage:     aliceLineage,
			CreatedAt:   time.Now(),
		}
		// Include unmodified baseline components
		for _, compID := range manifest.Components {
			if compID != goComp.ComponentID {
				aliceManifest.Components = append(aliceManifest.Components, compID)
			}
		}

		aliceBlobs := make(map[string][]byte)
		aliceComps := make(map[string]*core.ComponentNode)
		aliceSyms := make(map[string]*core.ASTSymbolNode)

		cBytes, _ := json.Marshal(rentalsComp)
		aliceBlobs[rentalsComp.ComponentID] = cBytes
		aliceComps[rentalsComp.ComponentID] = rentalsComp
		for id, s := range parsedRentals.Symbols {
			aliceSyms[id] = s
			sBytes, _ := json.Marshal(s)
			aliceBlobs[id] = sBytes
		}
		aliceRoot, _ := core.HashWorkspaceManifest(aliceManifest)
		aliceManifest.MerkleRootHash = aliceRoot

		// 4. Publish Alice's micro-universe
		_, err = aliceClient.PublishCosm(ctx, &topocosm.PublishPayload{
			OrgSlug:    orgSlug,
			CosmName:   cosmName,
			UniverseID: "universe-alice-rentals",
			Manifest:   aliceManifest,
			Components: aliceComps,
			Symbols:    aliceSyms,
			Blobs:      aliceBlobs,
			Lineage:    aliceLineage,
		})
		if err != nil {
			t.Fatalf("Alice failed publishing micro-universe: %v", err)
		}

		// 5. Submit Proposal COB to Topocosm Hub
		propAlice, err := aliceClient.CreateProposal(
			ctx,
			orgSlug,
			cosmName,
			"feat(rentals): Add gear rental endpoints",
			"universe-alice-rentals",
			"universe-main",
			aliceLineage,
		)
		if err != nil {
			t.Fatalf("Alice failed creating proposal: %v", err)
		}
		t.Logf("  [Alice] Created Proposal: ID=%s Title=%q", propAlice.ID, propAlice.Title)

		// 6. Automated Critic Oracle evaluation
		criticClient := topocosm.NewInProcessHubClient(hubHandler, "did:key:z6MkuCriticOracle")
		reviewRes, err := criticClient.SubmitReview(ctx, orgSlug, cosmName, &topocosm.ReviewSubmission{
			ProposalID:   propAlice.ID,
			ReviewerDID:  "did:key:z6MkuCriticOracle",
			Approved:     true,
			FitnessScore: 96.5,
			Comments: []distributed.ReviewComment{
				{
					CommentID: "rev-alice-01",
					AuthorDID: "did:key:z6MkuCriticOracle",
					Body:      "Syntax valid, contract preserved, all handlers conform to net/http signature.",
					Timestamp: time.Now(),
				},
			},
		})
		if err != nil {
			t.Fatalf("Critic review failed: %v", err)
		}
		if reviewRes.Status != distributed.StatusApproved {
			t.Fatalf("Expected proposal status APPROVED, got %s", reviewRes.Status)
		}
		t.Logf("  [Critic] Proposal %s reviewed: Approved=true (Score: %.1f)", propAlice.ID, reviewRes.FitnessScore)

		// 7. Merge Alice's proposal into universe-main
		mergeRes, err := aliceClient.MergeProposal(ctx, orgSlug, cosmName, propAlice.ID)
		if err != nil {
			t.Fatalf("Failed merging Alice's proposal: %v", err)
		}
		t.Logf("  [Alice] Merged successfully: New Head Merkle Root=%s", mergeRes["merkle_root_hash"])

		// 8. Release domain lease
		if err := aliceClient.ReleaseDomain(ctx, orgSlug, cosmName, "services/rentals"); err != nil {
			t.Fatalf("Alice failed releasing domain lease: %v", err)
		}
		t.Logf("  [Alice] Released lease on services/rentals")
	})

	// =========================================================================
	// Track B: Developer Bob (Stacked Proposals c/reviews-api & c/reviews-ui)
	// =========================================================================
	t.Run("Track_B_DeveloperBob_StackedReviews", func(t *testing.T) {
		bobDID := "did:key:z6MkuDevBob"
		bobClient := topocosm.NewInProcessHubClient(hubHandler, bobDID)

		// 1. Claim Blackboard lease on services/reviews
		claimed, err := bobClient.ClaimDomain(ctx, orgSlug, cosmName, "services/reviews", "Stacked proposals for campsite review system", 1800)
		if err != nil || !claimed {
			t.Fatalf("Bob failed claiming domain lease: %v", err)
		}

		// 2. Proposal 1: c/reviews-api (Backend review handler)
		bobLineage := core.LineageEnvelope{
			UserID:              "bob",
			UserPrompt:          "Add reviews backend endpoints",
			SessionID:           "sess-bob-reviews-01",
			OrchestratorAgentID: "dev-bob",
			ExecutingAgentID:    "dev-bob",
			LLMVersion:          "cli",
			Intent:              "feat(reviews): Backend reviews",
			Timestamp:           time.Now(),
		}

		reviewsAPISource := goSource + `
// HandleCampsiteReviews returns reviews for a campsite
func (s *Server) HandleCampsiteReviews(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("[{\"id\":\"r1\",\"rating\":5}]"))
}
`
		parsedRev, err := codecs.ParseSourceFile("cmd/server/main.go", []byte(reviewsAPISource), bobLineage)
		if err != nil {
			t.Fatalf("failed parsing reviews Go file: %v", err)
		}
		revComp := parsedRev.Component

		manifestAPI := &core.WorkspaceManifestNode{
			WorkspaceID: manifest.WorkspaceID,
			UniverseID:  "u/reviews-api",
			Components:  []string{revComp.ComponentID},
			Lineage:     bobLineage,
			CreatedAt:   time.Now(),
		}
		for _, compID := range manifest.Components {
			if compID != goComp.ComponentID {
				manifestAPI.Components = append(manifestAPI.Components, compID)
			}
		}

		blobsAPI := make(map[string][]byte)
		compsAPI := make(map[string]*core.ComponentNode)
		symsAPI := make(map[string]*core.ASTSymbolNode)

		cBytes, _ := json.Marshal(revComp)
		blobsAPI[revComp.ComponentID] = cBytes
		compsAPI[revComp.ComponentID] = revComp
		for id, s := range parsedRev.Symbols {
			symsAPI[id] = s
			sBytes, _ := json.Marshal(s)
			blobsAPI[id] = sBytes
		}
		rootAPI, _ := core.HashWorkspaceManifest(manifestAPI)
		manifestAPI.MerkleRootHash = rootAPI

		_, _ = bobClient.PublishCosm(ctx, &topocosm.PublishPayload{
			OrgSlug:    orgSlug,
			CosmName:   cosmName,
			UniverseID: "u/reviews-api",
			Manifest:   manifestAPI,
			Components: compsAPI,
			Symbols:    symsAPI,
			Blobs:      blobsAPI,
			Lineage:    bobLineage,
		})

		propAPI, err := bobClient.CreateProposal(
			ctx,
			orgSlug,
			cosmName,
			"feat(reviews): Review backend API (Stack 1/2)",
			"u/reviews-api",
			"universe-main",
			bobLineage,
		)
		if err != nil {
			t.Fatalf("Bob failed creating reviews-api proposal: %v", err)
		}

		// 3. Proposal 2: c/reviews-ui (Frontend React component stacked on top)
		reviewsUISource := tsSource + `
export function CampsiteReviews({ campsiteId }: { campsiteId: string }) {
  return <div className="reviews">Reviews Panel</div>;
}
`
		uiLineage := core.LineageEnvelope{
			UserID:              "bob",
			UserPrompt:          "Add campsite reviews UI drawer",
			SessionID:           "sess-bob-reviews-02",
			OrchestratorAgentID: "dev-bob",
			ExecutingAgentID:    "dev-bob",
			LLMVersion:          "cli",
			Intent:              "feat(reviews): Frontend reviews drawer",
			Timestamp:           time.Now(),
		}

		parsedUI, err := codecs.ParseSourceFile("web/src/App.tsx", []byte(reviewsUISource), uiLineage)
		if err != nil {
			t.Fatalf("failed parsing reviews UI file: %v", err)
		}
		uiComp := parsedUI.Component

		manifestUI := &core.WorkspaceManifestNode{
			WorkspaceID: manifest.WorkspaceID,
			UniverseID:  "u/reviews-ui",
			Components:  []string{uiComp.ComponentID, revComp.ComponentID},
			Lineage:     uiLineage,
			CreatedAt:   time.Now(),
		}

		blobsUI := make(map[string][]byte)
		compsUI := make(map[string]*core.ComponentNode)
		symsUI := make(map[string]*core.ASTSymbolNode)

		cBytesUI, _ := json.Marshal(uiComp)
		blobsUI[uiComp.ComponentID] = cBytesUI
		compsUI[uiComp.ComponentID] = uiComp
		for id, s := range parsedUI.Symbols {
			symsUI[id] = s
			sBytes, _ := json.Marshal(s)
			blobsUI[id] = sBytes
		}
		rootUI, _ := core.HashWorkspaceManifest(manifestUI)
		manifestUI.MerkleRootHash = rootUI

		_, _ = bobClient.PublishCosm(ctx, &topocosm.PublishPayload{
			OrgSlug:    orgSlug,
			CosmName:   cosmName,
			UniverseID: "u/reviews-ui",
			Manifest:   manifestUI,
			Components: compsUI,
			Symbols:    symsUI,
			Blobs:      blobsUI,
			Lineage:    uiLineage,
		})

		propUI, err := bobClient.CreateProposal(
			ctx,
			orgSlug,
			cosmName,
			"feat(reviews): Review frontend UI (Stack 2/2)",
			"u/reviews-ui",
			"u/reviews-api",
			uiLineage,
		)
		if err != nil {
			t.Fatalf("Bob failed creating reviews-ui proposal: %v", err)
		}
		t.Logf("  [Bob] Stacked Proposals Created: Base=%s, Stacked=%s", propAPI.ID, propUI.ID)

		// 4. Test Jujutsu Stack Evolution on local StackManager
		tempDir, _ := os.MkdirTemp("", "cosm-stack-test-*")
		defer os.RemoveAll(tempDir)
		blobStore, _ := storage.NewBlobStore(filepath.Join(tempDir, "objects"))
		graphEngine, _ := storage.NewGraphEngine(filepath.Join(tempDir, "graph.db"))
		defer graphEngine.Close()
		universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
		stackMgr := distributed.NewStackManager(universeMgr, blobStore)

		_, _ = universeMgr.CreateUniverse("u/reviews-api", "universe-main")
		_, _ = universeMgr.CreateUniverse("u/reviews-ui", "u/reviews-api")
		_ = stackMgr.RegisterChange(&distributed.StackedProposal{
			ChangeID:       "c/reviews-api",
			ProposalID:     propAPI.ID,
			Title:          propAPI.Title,
			ParentChangeID: "",
			UniverseID:     "u/reviews-api",
			ManifestHash:   rootAPI,
			AuthorDID:      bobDID,
			OrderIndex:     0,
		})
		_ = stackMgr.RegisterChange(&distributed.StackedProposal{
			ChangeID:       "c/reviews-ui",
			ProposalID:     propUI.ID,
			Title:          propUI.Title,
			ParentChangeID: "c/reviews-api",
			UniverseID:     "u/reviews-ui",
			ManifestHash:   rootUI,
			AuthorDID:      bobDID,
			OrderIndex:     1,
		})

		descendants := stackMgr.GetDescendants("c/reviews-api")
		if len(descendants) != 1 || descendants[0].ChangeID != "c/reviews-ui" {
			t.Fatalf("Expected 1 descendant change 'c/reviews-ui', got %v", descendants)
		}
		t.Logf("  [Bob] Stack hierarchy verified: c/reviews-api -> %s", descendants[0].ChangeID)

		// 5. Review & Merge both stacked proposals to Topocosm Hub
		criticClient := topocosm.NewInProcessHubClient(hubHandler, "did:key:z6MkuCriticOracle")
		_, _ = criticClient.SubmitReview(ctx, orgSlug, cosmName, &topocosm.ReviewSubmission{
			ProposalID:   propAPI.ID,
			ReviewerDID:  "did:key:z6MkuCriticOracle",
			Approved:     true,
			FitnessScore: 98.0,
		})
		_, err = bobClient.MergeProposal(ctx, orgSlug, cosmName, propAPI.ID)
		if err != nil {
			t.Fatalf("Failed merging reviews-api proposal: %v", err)
		}

		_, _ = criticClient.SubmitReview(ctx, orgSlug, cosmName, &topocosm.ReviewSubmission{
			ProposalID:   propUI.ID,
			ReviewerDID:  "did:key:z6MkuCriticOracle",
			Approved:     true,
			FitnessScore: 95.0,
		})
		_, err = bobClient.MergeProposal(ctx, orgSlug, cosmName, propUI.ID)
		if err != nil {
			t.Fatalf("Failed merging reviews-ui proposal: %v", err)
		}
		t.Logf("  [Bob] Both stacked proposals merged into universe-main")

		// 6. Release domain lease
		_ = bobClient.ReleaseDomain(ctx, orgSlug, cosmName, "services/reviews")
	})

	// =========================================================================
	// Track C: Infra Agent Charlie (Google Cloud Memorystore Redis HCL)
	// =========================================================================
	t.Run("Track_C_InfraAgentCharlie_RedisCache", func(t *testing.T) {
		charlieDID := "did:key:z6MkuAgentCharlie"
		charlieClient := topocosm.NewInProcessHubClient(hubHandler, charlieDID)

		// 1. Claim Blackboard lease on infra/cache
		claimed, err := charlieClient.ClaimDomain(ctx, orgSlug, cosmName, "infra/cache", "Provision Memorystore Redis cache in Terraform HCL", 600)
		if err != nil || !claimed {
			t.Fatalf("Charlie failed claiming domain lease: %v", err)
		}

		// 2. Sparse pull infra/main.tf
		sparseResp, err := charlieClient.SparsePullCosm(ctx, &topocosm.SparsePullRequest{
			OrgSlug:        orgSlug,
			CosmName:       cosmName,
			UniverseID:     "universe-main",
			ComponentNames: []string{"infra/main.tf"},
		})
		if err != nil {
			t.Fatalf("Charlie failed sparse pull: %v", err)
		}
		if len(sparseResp.Components) != 1 {
			t.Fatalf("Charlie expected 1 component, got %d", len(sparseResp.Components))
		}

		// 3. Mutate HCL AST: Add google_redis_instance
		redisHCL := hclSource + `
resource "google_redis_instance" "cache" {
  name           = "camping-cache"
  tier           = "BASIC"
  memory_size_gb = 1
  region         = "us-central1"
  authorized_network = google_compute_network.vpc.id
}
`
		charlieLineage := core.LineageEnvelope{
			UserID:              "charlie",
			UserPrompt:          "Provision Memorystore Redis cache in Terraform HCL",
			SessionID:           "sess-charlie-infra-01",
			OrchestratorAgentID: "agent-charlie-infra",
			ExecutingAgentID:    "agent-charlie-infra",
			LLMVersion:          "gemini-3.8-flash",
			Intent:              "infra: Add Memorystore Redis",
			Timestamp:           time.Now(),
		}

		parsedRedis, err := codecs.ParseSourceFile("infra/main.tf", []byte(redisHCL), charlieLineage)
		if err != nil {
			t.Fatalf("Charlie failed parsing Redis HCL: %v", err)
		}
		redisComp := parsedRedis.Component

		manifestInfra := &core.WorkspaceManifestNode{
			WorkspaceID: manifest.WorkspaceID,
			UniverseID:  "u/infra-redis",
			Components:  []string{redisComp.ComponentID},
			Lineage:     charlieLineage,
			CreatedAt:   time.Now(),
		}
		for _, compID := range manifest.Components {
			if parsedHCL != nil && parsedHCL.Component != nil && compID != parsedHCL.Component.ComponentID {
				manifestInfra.Components = append(manifestInfra.Components, compID)
			}
		}

		blobsInfra := make(map[string][]byte)
		compsInfra := make(map[string]*core.ComponentNode)
		symsInfra := make(map[string]*core.ASTSymbolNode)

		cBytes, _ := json.Marshal(redisComp)
		blobsInfra[redisComp.ComponentID] = cBytes
		compsInfra[redisComp.ComponentID] = redisComp
		for id, s := range parsedRedis.Symbols {
			symsInfra[id] = s
			sBytes, _ := json.Marshal(s)
			blobsInfra[id] = sBytes
		}
		rootInfra, _ := core.HashWorkspaceManifest(manifestInfra)
		manifestInfra.MerkleRootHash = rootInfra

		_, _ = charlieClient.PublishCosm(ctx, &topocosm.PublishPayload{
			OrgSlug:    orgSlug,
			CosmName:   cosmName,
			UniverseID: "u/infra-redis",
			Manifest:   manifestInfra,
			Components: compsInfra,
			Symbols:    symsInfra,
			Blobs:      blobsInfra,
			Lineage:    charlieLineage,
		})

		propInfra, err := charlieClient.CreateProposal(
			ctx,
			orgSlug,
			cosmName,
			"infra: Add Redis caching layer",
			"u/infra-redis",
			"universe-main",
			charlieLineage,
		)
		if err != nil {
			t.Fatalf("Charlie failed creating proposal: %v", err)
		}

		// 4. Review & Merge
		criticClient := topocosm.NewInProcessHubClient(hubHandler, "did:key:z6MkuCriticOracle")
		_, _ = criticClient.SubmitReview(ctx, orgSlug, cosmName, &topocosm.ReviewSubmission{
			ProposalID:   propInfra.ID,
			ReviewerDID:  "did:key:z6MkuCriticOracle",
			Approved:     true,
			FitnessScore: 99.0,
		})
		_, err = charlieClient.MergeProposal(ctx, orgSlug, cosmName, propInfra.ID)
		if err != nil {
			t.Fatalf("Failed merging Redis proposal: %v", err)
		}
		t.Logf("  [Charlie] Redis infra proposal merged successfully")

		// 5. Release lease
		_ = charlieClient.ReleaseDomain(ctx, orgSlug, cosmName, "infra/cache")
	})

	// =========================================================================
	// Final Hub State & Parity Verification
	// =========================================================================
	t.Run("FinalVerification_HubState", func(t *testing.T) {
		stats, err := client.GetStats(ctx)
		if err != nil {
			t.Fatalf("failed retrieving stats: %v", err)
		}
		t.Logf("Final Topocosm Hub Stats: Cosms=%d, Proposals=%d, Blobs=%d, ActiveClaims=%d",
			stats.TotalCosms, stats.TotalProposals, stats.TotalBlobs, stats.ActiveClaims)

		if stats.ActiveClaims != 0 {
			t.Errorf("Expected 0 active blackboard claims after releases, got %d", stats.ActiveClaims)
		}

		props, err := client.ListProposals(ctx, orgSlug, cosmName)
		if err != nil {
			t.Fatalf("failed listing proposals: %v", err)
		}
		if len(props) != 4 {
			t.Fatalf("Expected 4 proposals (1 Alice + 2 Bob + 1 Charlie), got %d", len(props))
		}
		for _, p := range props {
			if p.Status != distributed.StatusMerged {
				t.Errorf("Proposal %s (%s) expected status MERGED, got %s", p.ID, p.Title, p.Status)
			}
		}

		// Pull final merged universe-main
		finalPull, err := client.PullCosm(ctx, orgSlug, cosmName, "universe-main")
		if err != nil {
			t.Fatalf("failed pulling final merged universe: %v", err)
		}
		t.Logf("Final Merged Universe Merkle Root: %s (Components: %d)",
			finalPull.Manifest.MerkleRootHash, len(finalPull.Components))

		if len(finalPull.Components) < 3 {
			t.Errorf("Expected at least 3 polyglot components in final manifest, got %d", len(finalPull.Components))
		}
	})
}
