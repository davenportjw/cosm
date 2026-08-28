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
