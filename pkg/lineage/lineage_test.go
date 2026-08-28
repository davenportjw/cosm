package lineage

import (
	"crypto/ed25519"
	"os"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func setupTestGraph(t *testing.T) (*storage.GraphEngine, string) {
	tmpDir, err := os.MkdirTemp("", "fg-lineage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	graph, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("NewGraphEngine failed: %v", err)
	}

	return graph, tmpDir
}

func TestLineageTracer_TraceNodeAndComponentAncestry(t *testing.T) {
	graph, tmpDir := setupTestGraph(t)
	defer os.RemoveAll(tmpDir)
	defer graph.Close()

	pubKey, privKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	now := time.Now().UTC()
	env := core.LineageEnvelope{
		UserID:              "developer-alice",
		UserPrompt:          "Create high-performance authentication service in Go",
		SessionID:           "session-auth-001",
		OrchestratorAgentID: "orchestrator-main",
		ExecutingAgentID:    "golang-coder-agent",
		LLMVersion:          "claude-3-5-sonnet",
		GenerationParams:    `{"temperature": 0.2, "seed": 42}`,
		Intent:              "Implement JWT auth verification",
		Timestamp:           now,
	}

	if err := SignEnvelope(&env, privKey); err != nil {
		t.Fatalf("SignEnvelope failed: %v", err)
	}

	valid, err := VerifyEnvelope(&env, pubKey)
	if err != nil || !valid {
		t.Fatalf("VerifyEnvelope failed: valid=%v, err=%v", valid, err)
	}

	// Insert Node and Lineage into GraphEngine
	nodeID := "node-auth-jwt-001"
	nodeRec := storage.NodeRecord{
		NodeID:     nodeID,
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "auth.VerifyJWT",
		MerkleHash: "merkle-hash-jwt",
		CreatedAt:  now,
	}
	if err := graph.PutNode(nodeRec); err != nil {
		t.Fatalf("PutNode failed: %v", err)
	}

	lineageRec := storage.LineageRecord{
		RecordID:            "lin-rec-001",
		NodeID:              nodeID,
		UserID:              env.UserID,
		UserPrompt:          env.UserPrompt,
		SessionID:           env.SessionID,
		OrchestratorAgentID: env.OrchestratorAgentID,
		ExecutingAgentID:    env.ExecutingAgentID,
		LLMVersion:          env.LLMVersion,
		GenerationParams:    env.GenerationParams,
		Intent:              env.Intent,
		Timestamp:           env.Timestamp,
		SignatureEd25519:    env.SignatureEd25519,
	}
	if err := graph.PutLineage(lineageRec); err != nil {
		t.Fatalf("PutLineage failed: %v", err)
	}

	tracer := NewLineageTracer(graph)
	chain, err := tracer.TraceNodeAncestry(nodeID)
	if err != nil {
		t.Fatalf("TraceNodeAncestry failed: %v", err)
	}

	if !chain.IntactChain {
		t.Errorf("expected intact ancestry chain")
	}
	if chain.RootPrompt != "Create high-performance authentication service in Go" {
		t.Errorf("unexpected root prompt: %s", chain.RootPrompt)
	}
	if chain.RootSessionID != "session-auth-001" {
		t.Errorf("unexpected session id: %s", chain.RootSessionID)
	}
	if chain.PrimaryExecutorID != "golang-coder-agent" {
		t.Errorf("unexpected executor: %s", chain.PrimaryExecutorID)
	}
	if chain.LLMVersion != "claude-3-5-sonnet" {
		t.Errorf("unexpected llm version: %s", chain.LLMVersion)
	}

	// Verify cryptographic signature on the traced chain
	intact, signErrs := tracer.VerifyAncestryIntegrity(chain, pubKey)
	if !intact || len(signErrs) > 0 {
		t.Fatalf("VerifyAncestryIntegrity failed: %v", signErrs)
	}

	// Test Component ancestry tracing
	compNode := &core.ComponentNode{
		Name:        "auth-service",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{nodeID},
	}
	compChains, err := tracer.TraceComponentAncestry(compNode)
	if err != nil {
		t.Fatalf("TraceComponentAncestry failed: %v", err)
	}
	if len(compChains) != 1 || compChains[nodeID] == nil {
		t.Fatalf("expected component chain for %s", nodeID)
	}
}

func TestLineageAudit_BlastRadiusAndModelQueries(t *testing.T) {
	graph, tmpDir := setupTestGraph(t)
	defer os.RemoveAll(tmpDir)
	defer graph.Close()

	now := time.Now().UTC()

	// Setup nodes:
	// Node 1: Go Handler (created by Agent A, Claude 3.5)
	// Node 2: TS Client Call (created by Agent B, GPT-4o) - calls Node 1
	// Node 3: React UI Page (created by Agent B, GPT-4o) - uses Node 2
	n1 := storage.NodeRecord{
		NodeID:     "node-go-handler",
		Language:   core.LangGo,
		NodeType:   "RouteBinding",
		Identifier: "service:GET /api/v1/orders",
		CreatedAt:  now,
	}
	n2 := storage.NodeRecord{
		NodeID:     "node-ts-api",
		Language:   core.LangTypeScript,
		NodeType:   "ApiClientCall",
		Identifier: "Orders.tsx:GET /api/v1/orders",
		CreatedAt:  now,
	}
	n3 := storage.NodeRecord{
		NodeID:     "node-ts-ui",
		Language:   core.LangTypeScript,
		NodeType:   "ReactComponent",
		Identifier: "Orders.tsx:OrderList",
		CreatedAt:  now,
	}

	_ = graph.PutNode(n1)
	_ = graph.PutNode(n2)
	_ = graph.PutNode(n3)

	// Edges: n1 -> n2 (Go route serves TS API) and n2 -> n3 (TS API serves UI component)
	_ = graph.PutEdge(storage.EdgeRecord{
		SourceID: n1.NodeID,
		TargetID: n2.NodeID,
		EdgeType: core.EdgeConsumesAPI,
	})
	_ = graph.PutEdge(storage.EdgeRecord{
		SourceID: n2.NodeID,
		TargetID: n3.NodeID,
		EdgeType: core.EdgeCalls,
	})

	// Put lineage
	_ = graph.PutLineage(storage.LineageRecord{
		RecordID:         "lin-1",
		NodeID:           n1.NodeID,
		ExecutingAgentID: "agent-backend",
		LLMVersion:       "claude-3-5-sonnet",
		UserPrompt:       "Create orders backend API",
		SessionID:        "session-1",
		Timestamp:        now,
	})
	_ = graph.PutLineage(storage.LineageRecord{
		RecordID:         "lin-2",
		NodeID:           n2.NodeID,
		ExecutingAgentID: "agent-frontend",
		LLMVersion:       "gpt-4o",
		UserPrompt:       "Create orders frontend UI",
		SessionID:        "session-2",
		Timestamp:        now,
	})

	auditor := NewAuditEngine(graph)

	// 1. Query by Agent
	backendNodes, err := auditor.QueryNodes(AuditFilter{ExecutingAgentID: "agent-backend"})
	if err != nil || len(backendNodes) != 1 {
		t.Fatalf("expected 1 backend node, got %d (err: %v)", len(backendNodes), err)
	}

	// 2. Query by LLM Version
	gptNodes, err := auditor.QueryNodes(AuditFilter{LLMVersion: "gpt-4o"})
	if err != nil || len(gptNodes) != 1 {
		t.Fatalf("expected 1 gpt-4o node, got %d (err: %v)", len(gptNodes), err)
	}

	// 3. Compute Blast Radius for backend changes
	blast, err := auditor.ComputeBlastRadius(AuditFilter{ExecutingAgentID: "agent-backend"}, 5)
	if err != nil {
		t.Fatalf("ComputeBlastRadius failed: %v", err)
	}

	if blast.TotalDirectNodes != 1 {
		t.Errorf("expected 1 direct node, got %d", blast.TotalDirectNodes)
	}
	if blast.TotalDownstreamNodes != 2 {
		t.Errorf("expected 2 downstream impacted nodes (n2 and n3), got %d", blast.TotalDownstreamNodes)
	}
	if blast.TotalImpactedNodes != 3 {
		t.Errorf("expected 3 total impacted nodes, got %d", blast.TotalImpactedNodes)
	}
	if len(blast.AffectedLanguages) != 2 {
		t.Errorf("expected 2 affected languages (Go and TypeScript), got %d", len(blast.AffectedLanguages))
	}
	if blast.RiskScore <= 0 {
		t.Errorf("expected positive risk score, got %f", blast.RiskScore)
	}

	// 4. Summaries
	agentSumm, err := auditor.AuditAgentContributions("agent-backend")
	if err != nil || agentSumm.TotalNodesCreated != 1 {
		t.Errorf("unexpected agent summary: %+v, err: %v", agentSumm, err)
	}

	modelSumm, err := auditor.AuditModelContributions("claude-3-5-sonnet")
	if err != nil || modelSumm.TotalNodesCreated != 1 {
		t.Errorf("unexpected model summary: %+v, err: %v", modelSumm, err)
	}
}

func TestLineageAttestation_InTotoSigningAndVerification(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fg-attestation-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pubKey, privKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Test Key Serialization roundtrip
	pubHex := PublicKeyToHex(pubKey)
	privHex := PrivateKeyToHex(privKey)

	recoveredPub, err := HexToPublicKey(pubHex)
	if err != nil {
		t.Fatalf("HexToPublicKey failed: %v", err)
	}
	recoveredPriv, err := HexToPrivateKey(privHex)
	if err != nil {
		t.Fatalf("HexToPrivateKey failed: %v", err)
	}

	// Verify key roundtrip signing
	msg := []byte("test-data-for-signing")
	sig := ed25519.Sign(recoveredPriv, msg)
	if !ed25519.Verify(recoveredPub, msg, sig) {
		t.Errorf("recovered key signature verification failed")
	}

	// Create ASTSymbolNode
	lineageEnv := core.LineageEnvelope{
		UserID:           "architect-1",
		UserPrompt:       "Provision scalable Redis cluster in Terraform",
		SessionID:        "sess-infra-99",
		ExecutingAgentID: "terraform-bot",
		LLMVersion:       "claude-3-5-sonnet",
		Intent:           "Provision Redis memory store",
		Timestamp:        time.Now().UTC(),
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangHCL,
		NodeType:          "ResourceBlock",
		Identifier:        "resource.google_redis_instance.cache",
		ASTPayload:        []byte(`resource "google_redis_instance" "cache" { memory_size_gb = 5 }`),
		LocalDependencies: []string{"var.project_id"},
		Lineage:           lineageEnv,
	}
	node.NodeID, _ = core.HashASTSymbolNode(node)

	// Create and sign in-toto attestation
	att, err := CreateAttestation(node, privKey, "key-admin-1")
	if err != nil {
		t.Fatalf("CreateAttestation failed: %v", err)
	}

	if att.Type != InTotoStatementV1 {
		t.Errorf("expected _type %s, got %s", InTotoStatementV1, att.Type)
	}
	if att.PredicateType != ProvenancePredicateType {
		t.Errorf("expected predicateType %s, got %s", ProvenancePredicateType, att.PredicateType)
	}

	// Verify valid attestation
	valid, err := VerifyAttestation(att, pubKey)
	if err != nil || !valid {
		t.Fatalf("VerifyAttestation failed: valid=%v, err=%v", valid, err)
	}

	// Save attestation to disk
	savedPath, err := SaveAttestation(tmpDir, att)
	if err != nil {
		t.Fatalf("SaveAttestation failed: %v", err)
	}
	if _, err := os.Stat(savedPath); err != nil {
		t.Fatalf("attestation file not written: %s", savedPath)
	}

	// Load and verify saved attestation
	loadedAtt, err := LoadAttestation(savedPath)
	if err != nil {
		t.Fatalf("LoadAttestation failed: %v", err)
	}
	validLoaded, err := VerifyAttestation(loadedAtt, pubKey)
	if err != nil || !validLoaded {
		t.Fatalf("VerifyAttestation on loaded attestation failed: valid=%v, err=%v", validLoaded, err)
	}

	// Verify tampered attestation fails
	tamperedAtt := *loadedAtt
	tamperedAtt.Predicate.Recipe.UserPrompt = "Tampered prompt text"
	tamperedValid, _ := VerifyAttestation(&tamperedAtt, pubKey)
	if tamperedValid {
		t.Errorf("expected tampered attestation verification to fail")
	}
}
