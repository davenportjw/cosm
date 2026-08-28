package topocosm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/distributed"
)

// SeedOptions configures the mock data generation parameters.
type SeedOptions struct {
	NumUsers     int
	NumCosms     int
	NumProposals int
}

// DefaultSeedOptions provides standard defaults.
func DefaultSeedOptions() SeedOptions {
	return SeedOptions{
		NumUsers:     3,
		NumCosms:     3,
		NumProposals: 4,
	}
}

// SeedLocalHub populates a Topocosm Hub server with realistic polyglot fixtures.
func SeedLocalHub(ctx context.Context, server *HubServer, opts SeedOptions) error {
	client := NewInProcessHubClient(server.Handler(), "did:key:z6MkuSeeder")
	defer client.Close()

	now := time.Now().UTC()

	// 1. Enroll Users
	users := []struct {
		did      string
		username string
		email    string
		name     string
	}{
		{"did:key:z6MkuDevJason", "jason", "jason@topocosm.dev", "Jason Davenport"},
		{"did:key:z6MkuDevAlice", "alice", "alice@topocosm.dev", "Alice Smith"},
		{"did:key:z6MkuAgentBilling", "agent-billing", "agent@topocosm.dev", "Autonomous Billing Builder"},
	}

	for _, u := range users {
		_ = client.Enroll(ctx, u.did, u.username, u.email, u.name, "ed25519-mock-key-"+u.username, false)
	}

	// 2. Create Polyglot Cosm Repositories
	repo1Payload := createPolyglotPayload("demo-org", "cloud-platform", "Enterprise Polyglot Microservices & Cloud Infrastructure")
	_, err := client.PublishCosm(ctx, repo1Payload)
	if err != nil {
		return fmt.Errorf("failed to seed cloud-platform: %w", err)
	}

	repo2Payload := createAuthServicePayload("acme-corp", "auth-gateway", "Zero-Trust Ed25519 JWT Authentication Gateway")
	_, err = client.PublishCosm(ctx, repo2Payload)
	if err != nil {
		return fmt.Errorf("failed to seed auth-gateway: %w", err)
	}

	// 3. Create Sample Universe Proposals
	lineage := core.LineageEnvelope{
		UserID:              "user-jason",
		UserPrompt:          "Add Stripe billing webhook handler and Cloud PubSub topic",
		SessionID:           "sess-seed-001",
		OrchestratorAgentID: "orchestrator-main",
		ExecutingAgentID:    "agent-builder-01",
		LLMVersion:          "gemini-3.7-flash",
		Intent:              "Implement Stripe webhook endpoint",
		Timestamp:           now,
	}

	prop1, err := client.CreateProposal(ctx, "demo-org", "cloud-platform", "Feature: Stripe Billing Webhook & PubSub Topic", "universe-main", "universe-main", lineage)
	if err == nil && prop1 != nil {
		// Add human reviewer approval
		_, _ = client.SubmitReview(ctx, "demo-org", "cloud-platform", &ReviewSubmission{
			ProposalID:   prop1.ID,
			ReviewerDID:  "did:key:z6MkuDevJason",
			Approved:     true,
			FitnessScore: 98.5,
			Comments: []distributed.ReviewComment{
				{
					CommentID: "comm-1",
					AuthorDID: "did:key:z6MkuDevJason",
					Body:      "Code is well-structured and cross-boundary edges are validated against terraform infra.",
					Severity:  "INFO",
					Timestamp: now,
				},
			},
		})
	}

	// 4. Create Active Blackboard Domain Claims
	_, _ = client.ClaimDomain(ctx, "demo-org", "cloud-platform", "services/billing", "Refactoring payment receipt generator", 600)
	_, _ = client.ClaimDomain(ctx, "acme-corp", "auth-gateway", "services/auth/jwt", "Migrating JWT tokens to Ed25519 asymmetric keys", 600)

	return nil
}

func createPolyglotPayload(org, name, desc string) *PublishPayload {
	now := time.Now().UTC()

	// 1. AST Symbol Nodes
	symGo := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "HandleStripeWebhook",
		Signature:  "func(w http.ResponseWriter, r *http.Request)",
		Visibility: "public",
		ASTPayload: []byte("func HandleStripeWebhook(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }"),
		Lineage: core.LineageEnvelope{
			ExecutingAgentID: "agent-seed",
			Intent:           "Create webhook handler",
			Timestamp:        now,
		},
	}
	symGoBytes, _ := json.Marshal(symGo)
	symGoHash := core.HashBytes(symGoBytes)
	symGo.NodeID = symGoHash
	symGoBytes, _ = json.Marshal(symGo)
	symGoHash = core.HashBytes(symGoBytes)

	symHCL := &core.ASTSymbolNode{
		Language:   core.LangHCL,
		NodeType:   "ResourceBlock",
		Identifier: "google_pubsub_topic.billing_events",
		Visibility: "public",
		ASTPayload: []byte(`resource "google_pubsub_topic" "billing_events" { name = "billing-events" }`),
		Lineage: core.LineageEnvelope{
			ExecutingAgentID: "agent-seed",
			Intent:           "Define pubsub topic",
			Timestamp:        now,
		},
	}
	symHCLBytes, _ := json.Marshal(symHCL)
	symHCLHash := core.HashBytes(symHCLBytes)
	symHCL.NodeID = symHCLHash
	symHCLBytes, _ = json.Marshal(symHCL)
	symHCLHash = core.HashBytes(symHCLBytes)

	symTS := &core.ASTSymbolNode{
		Language:   core.LangTypeScript,
		NodeType:   "FunctionDecl",
		Identifier: "BillingDashboard",
		Signature:  "export function BillingDashboard(): JSX.Element",
		Visibility: "public",
		ASTPayload: []byte("export function BillingDashboard() { return <div>Billing Active</div>; }"),
		Lineage: core.LineageEnvelope{
			ExecutingAgentID: "agent-seed",
			Intent:           "Create React UI",
			Timestamp:        now,
		},
	}
	symTSBytes, _ := json.Marshal(symTS)
	symTSHash := core.HashBytes(symTSBytes)
	symTS.NodeID = symTSHash
	symTSBytes, _ = json.Marshal(symTS)
	symTSHash = core.HashBytes(symTSBytes)

	// 2. Component Nodes
	compGo := &core.ComponentNode{
		Name:        "services/billing",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{symGoHash},
		Lineage:     symGo.Lineage,
	}
	compGoBytes, _ := json.Marshal(compGo)
	compGoHash := core.HashBytes(compGoBytes)
	compGo.ComponentID = compGoHash
	compGoBytes, _ = json.Marshal(compGo)
	compGoHash = core.HashBytes(compGoBytes)

	compHCL := &core.ComponentNode{
		Name:        "infra/pubsub",
		Type:        core.CompInfra,
		Language:    core.LangHCL,
		SymbolNodes: []string{symHCLHash},
		Lineage:     symHCL.Lineage,
	}
	compHCLBytes, _ := json.Marshal(compHCL)
	compHCLHash := core.HashBytes(compHCLBytes)
	compHCL.ComponentID = compHCLHash
	compHCLBytes, _ = json.Marshal(compHCL)
	compHCLHash = core.HashBytes(compHCLBytes)

	compTS := &core.ComponentNode{
		Name:        "web/billing",
		Type:        core.CompFrontend,
		Language:    core.LangTypeScript,
		SymbolNodes: []string{symTSHash},
		Lineage:     symTS.Lineage,
	}
	compTSBytes, _ := json.Marshal(compTS)
	compTSHash := core.HashBytes(compTSBytes)
	compTS.ComponentID = compTSHash
	compTSBytes, _ = json.Marshal(compTS)
	compTSHash = core.HashBytes(compTSBytes)

	// 3. Cross-Boundary Edges
	edge1 := core.CrossBoundaryEdge{
		SourceNodeID: symGoHash,
		TargetNodeID: symHCLHash,
		Type:         core.EdgeDeploysTo,
		Metadata:     map[string]string{"binding": "pubsub_topic"},
	}
	edge2 := core.CrossBoundaryEdge{
		SourceNodeID: symTSHash,
		TargetNodeID: symGoHash,
		Type:         core.EdgeConsumesAPI,
		Metadata:     map[string]string{"endpoint": "/api/v1/billing"},
	}

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-cloud-platform",
		UniverseID:  "universe-main",
		Components:  []string{compGoHash, compHCLHash, compTSHash},
		CrossEdges:  []core.CrossBoundaryEdge{edge1, edge2},
		Lineage: core.LineageEnvelope{
			Intent:    "Initial polyglot workspace manifest",
			Timestamp: now,
		},
		CreatedAt: now,
	}
	merkleRoot, _ := core.HashWorkspaceManifest(manifest)
	manifest.MerkleRootHash = merkleRoot

	blobs := map[string][]byte{
		symGoHash:   symGoBytes,
		symHCLHash:  symHCLBytes,
		symTSHash:   symTSBytes,
		compGoHash:  compGoBytes,
		compHCLHash: compHCLBytes,
		compTSHash:  compTSBytes,
	}

	return &PublishPayload{
		OrgSlug:     org,
		CosmName:    name,
		UniverseID:  "universe-main",
		Manifest:    manifest,
		Components:  map[string]*core.ComponentNode{compGoHash: compGo, compHCLHash: compHCL, compTSHash: compTS},
		Symbols:     map[string]*core.ASTSymbolNode{symGoHash: symGo, symHCLHash: symHCL, symTSHash: symTS},
		Blobs:       blobs,
		Description: desc,
		Visibility:  VisibilityPublic,
		IsInitial:   true,
	}
}

func createAuthServicePayload(org, name, desc string) *PublishPayload {
	now := time.Now().UTC()

	symAuth := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "ValidateEd25519Token",
		Signature:  "func(tokenString string) (*Claims, error)",
		Visibility: "public",
		ASTPayload: []byte("func ValidateEd25519Token(tokenString string) (*Claims, error) { return &Claims{Valid: true}, nil }"),
		Lineage: core.LineageEnvelope{
			ExecutingAgentID: "agent-seed",
			Intent:           "Implement Ed25519 token validator",
			Timestamp:        now,
		},
	}
	symAuthBytes, _ := json.Marshal(symAuth)
	symAuthHash := core.HashBytes(symAuthBytes)
	symAuth.NodeID = symAuthHash
	symAuthBytes, _ = json.Marshal(symAuth)
	symAuthHash = core.HashBytes(symAuthBytes)

	compAuth := &core.ComponentNode{
		Name:        "services/auth",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{symAuthHash},
		Lineage:     symAuth.Lineage,
	}
	compAuthBytes, _ := json.Marshal(compAuth)
	compAuthHash := core.HashBytes(compAuthBytes)
	compAuth.ComponentID = compAuthHash
	compAuthBytes, _ = json.Marshal(compAuth)
	compAuthHash = core.HashBytes(compAuthBytes)

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-auth-gateway",
		UniverseID:  "universe-main",
		Components:  []string{compAuthHash},
		CrossEdges:  []core.CrossBoundaryEdge{},
		Lineage: core.LineageEnvelope{
			Intent:    "Initial auth gateway manifest",
			Timestamp: now,
		},
		CreatedAt: now,
	}
	merkleRoot, _ := core.HashWorkspaceManifest(manifest)
	manifest.MerkleRootHash = merkleRoot

	blobs := map[string][]byte{
		symAuthHash:  symAuthBytes,
		compAuthHash: compAuthBytes,
	}

	return &PublishPayload{
		OrgSlug:     org,
		CosmName:    name,
		UniverseID:  "universe-main",
		Manifest:    manifest,
		Components:  map[string]*core.ComponentNode{compAuthHash: compAuth},
		Symbols:     map[string]*core.ASTSymbolNode{symAuthHash: symAuth},
		Blobs:       blobs,
		Description: desc,
		Visibility:  VisibilityPublic,
		IsInitial:   true,
	}
}
