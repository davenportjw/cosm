package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/mutation"
	"github.com/cosmscm/cosm/pkg/storage"
)

func TestAgentAPIServer_InProcessMutationAndQuery(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fg-api-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	blobStore, _ := storage.NewBlobStore(filepath.Join(tmpDir, "objects"))
	graphEngine, _ := storage.NewGraphEngine(filepath.Join(tmpDir, "graph.db"))
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	server := NewAgentAPIServer(universeMgr, blobStore, graphEngine)
	client := NewInProcessAgentClient(server.Handler())

	// 1. Create a universe and manifest
	universeID := "universe-swarm-1"
	_, _ = universeMgr.CreateUniverse(universeID, "")
	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-swarm",
		UniverseID:  universeID,
		Components:  []string{"comp-1"},
	}
	_, _ = universeMgr.CommitManifest(universeID, manifest)

	// 2. Query universe
	uResp, err := client.GetUniverse(universeID)
	if err != nil {
		t.Fatalf("GetUniverse failed: %v", err)
	}
	if uResp.UniverseID != universeID || uResp.ComponentCount != 1 {
		t.Fatalf("unexpected universe response: %+v", uResp)
	}

	// 3. Post an AST node mutation
	mutReq := &MutateNodeRequest{
		UniverseID: universeID,
		SymbolNode: &core.ASTSymbolNode{
			Language:   core.LangGo,
			NodeType:   "FunctionDecl",
			Identifier: "HandlePaymentCallback",
			ASTPayload: []byte("func HandlePaymentCallback() {}"),
			Lineage: core.LineageEnvelope{
				ExecutingAgentID: "subagent-payment-coder",
				Intent:           "Add callback endpoint",
				Timestamp:        time.Now().UTC(),
			},
		},
	}

	mutResp, err := client.MutateNode(mutReq)
	if err != nil {
		t.Fatalf("MutateNode failed: %v", err)
	}
	if !mutResp.Success || mutResp.NodeID == "" {
		t.Fatalf("expected mutation success, got: %+v", mutResp)
	}

	// Verify node is readable from blob store
	has, err := blobStore.Has(mutResp.NodeID)
	if err != nil || !has {
		t.Fatalf("expected node %s to exist in blob store", mutResp.NodeID)
	}
}

func TestAgentAPIServer_ApplyASTEditBatch(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fg-api-batch-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	blobStore, _ := storage.NewBlobStore(filepath.Join(tmpDir, "objects"))
	graphEngine, _ := storage.NewGraphEngine(filepath.Join(tmpDir, "graph.db"))
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	server := NewAgentAPIServer(universeMgr, blobStore, graphEngine)
	client := NewInProcessAgentClient(server.Handler())

	universeID := "universe-api-batch"
	_, _ = universeMgr.CreateUniverse(universeID, "")

	lineage := core.LineageEnvelope{
		UserID:    "agent-api",
		Timestamp: time.Now().UTC(),
	}

	sym := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "pkg/api::ServeHTTP",
		ASTPayload: []byte("func ServeHTTP() {}"),
		Lineage:    lineage,
	}
	symBytes, _ := json.Marshal(sym)
	symHash, _ := blobStore.Put(symBytes)
	sym.NodeID = symHash

	comp := &core.ComponentNode{
		Name:        "pkg/api",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{symHash},
		Lineage:     lineage,
	}
	compBytes, _ := json.Marshal(comp)
	compHash, _ := blobStore.Put(compBytes)

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-api-batch",
		UniverseID:  universeID,
		Components:  []string{compHash},
		Lineage:     lineage,
		CreatedAt:   time.Now().UTC(),
	}
	mBytes, _ := json.Marshal(manifest)
	mHash, _ := blobStore.Put(mBytes)
	manifest.MerkleRootHash = mHash
	_, _ = universeMgr.CommitManifest(universeID, manifest)

	// Test ResolveSymbol via client
	resolved, err := client.ResolveSymbol(universeID, "ServeHTTP")
	if err != nil || resolved.SymbolNode.NodeID != symHash {
		t.Fatalf("ResolveSymbol failed: %v", err)
	}

	// Test ApplyASTEditBatch via client
	batch := &mutation.ASTEditBatch{
		UniverseID: universeID,
		Lineage:    lineage,
		Operations: []mutation.ASTOperation{
			{
				Operation: mutation.OpReplaceFunctionBody,
				Target:    "ServeHTTP",
				Content:   "w.WriteHeader(http.StatusOK)",
			},
			{
				Operation: mutation.OpAddAfter,
				Target:    "ServeHTTP",
				Content:   "func HealthCheck() string { return \"OK\" }",
			},
		},
	}

	res, err := client.ApplyASTEditBatch(batch)
	if err != nil {
		t.Fatalf("ApplyASTEditBatch failed: %v", err)
	}
	if res.AppliedOperations != 2 {
		t.Fatalf("Expected 2 applied operations, got %d", res.AppliedOperations)
	}
	if len(res.ModifiedSymbols) != 1 || len(res.AddedSymbols) != 1 {
		t.Fatalf("Unexpected modified/added count: %+v", res)
	}
}

func TestAgentAPIServer_CommitWorkspaceWithTelemetry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fg-api-commit-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	blobStore, _ := storage.NewBlobStore(filepath.Join(tmpDir, "objects"))
	graphEngine, _ := storage.NewGraphEngine(filepath.Join(tmpDir, "graph.db"))
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	server := NewAgentAPIServer(universeMgr, blobStore, graphEngine)
	client := NewInProcessAgentClient(server.Handler())

	universeID := "universe-telemetry-test"
	_, _ = universeMgr.CreateUniverse(universeID, "")

	// 1. Stage a component and symbol
	sym := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "pkg/auth::ValidateRS256Token",
		ASTPayload: []byte("func ValidateRS256Token() bool { return true }"),
	}
	symBytes, _ := json.Marshal(sym)
	symHash, _ := blobStore.Put(symBytes)
	sym.NodeID = symHash

	_ = graphEngine.PutNode(storage.NodeRecord{
		NodeID:     symHash,
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "pkg/auth::ValidateRS256Token",
		MerkleHash: symHash,
		CreatedAt:  time.Now().UTC(),
	})

	comp := &core.ComponentNode{
		Name:        "pkg/auth",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{symHash},
	}
	compBytes, _ := json.Marshal(comp)
	compHash, _ := blobStore.Put(compBytes)
	comp.ComponentID = compHash

	_ = graphEngine.PutNode(storage.NodeRecord{
		NodeID:     compHash,
		Language:   core.LangGo,
		NodeType:   "service",
		Identifier: "pkg/auth",
		MerkleHash: compHash,
		CreatedAt:  time.Now().UTC(),
	})

	// 2. Commit workspace with full LineageEnvelope, Tokens, and Trace Carrier
	commitReq := &CommitWorkspaceRequest{
		UniverseID: universeID,
		Intent:     "Implement RS256 token verification",
		Lineage: core.LineageEnvelope{
			UserID:              "developer-alice",
			UserPrompt:          "Add RS256 token validation with error propagation",
			SessionID:           "sess-993821aa-8371-4209",
			OrchestratorAgentID: "agent-orchestrator-alpha",
			ExecutingAgentID:    "cosm-worker-auth-1",
			LLMVersion:          "gemini-3.7-flash",
			GenerationParams:    `{"temperature": 0.2, "seed": 42}`,
			Intent:              "Add RS256 token validation",
			Timestamp:           time.Now().UTC(),
			Tokens: core.TokenTelemetry{
				PromptTokens:     1420,
				CompletionTokens: 312,
				ReasoningTokens:  850,
				CachedTokens:     128,
				TotalTokens:      2582,
				CostUSD:          0.00184,
				LatencyMs:        640,
			},
			Trace: core.TraceCarrier{
				TraceID:    "4bf92f3577b34da6a3ce929d0e0e4736",
				SpanID:     "00f067aa0ba902b7",
				TraceFlags: "01",
				Attributes: map[string]string{
					"agent.role": "auth-engineer",
				},
			},
		},
	}

	commitResp, err := client.CommitWorkspace(commitReq)
	if err != nil {
		t.Fatalf("CommitWorkspace failed: %v", err)
	}

	if !commitResp.Success || commitResp.MerkleRootHash == "" {
		t.Fatalf("expected successful commit, got: %+v", commitResp)
	}

	if commitResp.ComponentCount != 1 {
		t.Fatalf("expected 1 component, got %d", commitResp.ComponentCount)
	}

	// 3. Verify Universe Manifest
	uniManifest, err := universeMgr.GetUniverseManifest(universeID)
	if err != nil {
		t.Fatalf("GetUniverseManifest failed: %v", err)
	}

	if uniManifest.Lineage.Tokens.TotalTokens != 2582 {
		t.Fatalf("expected 2582 total tokens, got %d", uniManifest.Lineage.Tokens.TotalTokens)
	}
	if uniManifest.Lineage.Trace.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("expected trace id 4bf92f3577b34da6a3ce929d0e0e4736, got %s", uniManifest.Lineage.Trace.TraceID)
	}
}
