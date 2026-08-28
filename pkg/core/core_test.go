package core_test

import (
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestSchemaEnums(t *testing.T) {
	languages := []core.Language{
		core.LangGo, core.LangHCL, core.LangTypeScript, core.LangPython, core.LangOpenAPI,
	}
	for _, l := range languages {
		if !l.IsValid() {
			t.Errorf("expected language %s to be valid", l)
		}
		if l.String() == "" {
			t.Errorf("expected non-empty string for language %s", l)
		}
	}
	if core.Language("unsupported_lang").IsValid() {
		t.Errorf("expected unsupported language to be invalid")
	}

	compTypes := []core.ComponentType{
		core.CompService, core.CompInfra, core.CompFrontend, core.CompContract,
	}
	for _, ct := range compTypes {
		if !ct.IsValid() {
			t.Errorf("expected component type %s to be valid", ct)
		}
		if ct.String() == "" {
			t.Errorf("expected non-empty string for component type %s", ct)
		}
	}
	if core.ComponentType("invalid_type").IsValid() {
		t.Errorf("expected invalid component type to be invalid")
	}

	edgeTypes := []core.EdgeType{
		core.EdgeCalls, core.EdgeConsumesAPI, core.EdgeDeploysTo, core.EdgeBindsEnv, core.EdgeDependsOn,
	}
	for _, et := range edgeTypes {
		if !et.IsValid() {
			t.Errorf("expected edge type %s to be valid", et)
		}
		if et.String() == "" {
			t.Errorf("expected non-empty string for edge type %s", et)
		}
	}
	if core.EdgeType("UNKNOWN_EDGE").IsValid() {
		t.Errorf("expected unknown edge type to be invalid")
	}
}

func TestLineageEnvelope(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	sig := []byte("fake_ed25519_signature_bytes_32b")

	lineage := core.LineageEnvelope{
		UserID:              "user-123",
		UserPrompt:          "Create user authentication endpoint",
		SessionID:           "sess-abc",
		OrchestratorAgentID: "agent-arch-01",
		ExecutingAgentID:    "agent-coder-02",
		LLMVersion:          "claude-3-7-sonnet",
		GenerationParams:    `{"temperature":0.2,"seed":42}`,
		Intent:              "Implement AuthHandler function in Go",
		Timestamp:           now,
		SignatureEd25519:    sig,
	}

	clone := lineage.Clone()
	if clone.UserID != lineage.UserID || clone.Intent != lineage.Intent {
		t.Fatalf("clone does not match original: %+v vs %+v", clone, lineage)
	}
	if hex.EncodeToString(clone.SignatureEd25519) != hex.EncodeToString(lineage.SignatureEd25519) {
		t.Fatalf("clone signature does not match original")
	}

	// Mutate clone slice to verify deep copy
	clone.SignatureEd25519[0] = 0xFF
	if lineage.SignatureEd25519[0] == 0xFF {
		t.Fatalf("clone mutation affected original signature")
	}

	hash1 := core.HashLineage(&lineage)
	hash2 := core.HashLineage(&lineage)
	if hash1 != hash2 {
		t.Fatalf("deterministic hash mismatch: %s vs %s", hash1, hash2)
	}

	nilHash := core.HashLineage(nil)
	if nilHash == "" || nilHash == hash1 {
		t.Fatalf("nil lineage should produce a non-empty unique hash")
	}
}

func TestASTSymbolNodeDeterministicHashing(t *testing.T) {
	node1 := &core.ASTSymbolNode{
		Language:          core.LangGo,
		NodeType:          "FunctionDecl",
		Identifier:        "HandleLogin",
		ASTPayload:        []byte("func HandleLogin(w http.ResponseWriter, r *http.Request) {}"),
		LocalDependencies: []string{"UserStore", "TokenGenerator", "HashPassword"},
		Lineage: core.LineageEnvelope{
			UserID:     "user-1",
			UserPrompt: "Add login handler",
			Intent:     "Implement login HTTP handler",
		},
	}

	// node2 has the exact same content but LocalDependencies in reverse order
	node2 := &core.ASTSymbolNode{
		Language:          core.LangGo,
		NodeType:          "FunctionDecl",
		Identifier:        "HandleLogin",
		ASTPayload:        []byte("func HandleLogin(w http.ResponseWriter, r *http.Request) {}"),
		LocalDependencies: []string{"HashPassword", "TokenGenerator", "UserStore"},
		Lineage: core.LineageEnvelope{
			UserID:     "user-1",
			UserPrompt: "Add login handler",
			Intent:     "Implement login HTTP handler",
		},
	}

	hash1, err1 := core.HashASTSymbolNode(node1)
	if err1 != nil {
		t.Fatalf("failed to hash node1: %v", err1)
	}
	hash2, err2 := core.HashASTSymbolNode(node2)
	if err2 != nil {
		t.Fatalf("failed to hash node2: %v", err2)
	}

	if hash1 != hash2 {
		t.Fatalf("expected identical hashes regardless of LocalDependencies ordering: %s vs %s", hash1, hash2)
	}

	// Modify payload and assert hash changes
	node3 := *node1
	node3.ASTPayload = []byte("func HandleLogin(w http.ResponseWriter, r *http.Request) { log.Println() }")
	hash3, err3 := core.HashASTSymbolNode(&node3)
	if err3 != nil {
		t.Fatalf("failed to hash node3: %v", err3)
	}

	if hash1 == hash3 {
		t.Fatalf("expected different hashes for modified AST payload")
	}

	// Validate method
	if err := node1.Validate(); err != nil {
		t.Fatalf("node1 should be valid: %v", err)
	}
	invalidNode := &core.ASTSymbolNode{}
	if err := invalidNode.Validate(); err == nil {
		t.Fatalf("empty node should fail validation")
	}
}

func TestComponentNodeDeterministicHashing(t *testing.T) {
	comp1 := &core.ComponentNode{
		Name:        "auth-service",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{"node-sha-1", "node-sha-2", "node-sha-3"},
		Metadata: map[string]string{
			"port":    "8080",
			"runtime": "go1.25",
			"tier":    "backend",
		},
	}

	// comp2 has permuted symbol nodes and metadata keys
	comp2 := &core.ComponentNode{
		Name:        "auth-service",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{"node-sha-3", "node-sha-1", "node-sha-2"},
		Metadata: map[string]string{
			"tier":    "backend",
			"port":    "8080",
			"runtime": "go1.25",
		},
	}

	hash1, err := core.HashComponentNode(comp1)
	if err != nil {
		t.Fatalf("failed to hash comp1: %v", err)
	}
	hash2, err := core.HashComponentNode(comp2)
	if err != nil {
		t.Fatalf("failed to hash comp2: %v", err)
	}

	if hash1 != hash2 {
		t.Fatalf("expected identical component hashes: %s vs %s", hash1, hash2)
	}

	if err := comp1.Validate(); err != nil {
		t.Fatalf("comp1 should be valid: %v", err)
	}
	invalidComp := &core.ComponentNode{Type: "invalid"}
	if err := invalidComp.Validate(); err == nil {
		t.Fatalf("invalidComp should fail validation")
	}
}

func TestCrossBoundaryEdgeDeterministicHashing(t *testing.T) {
	edge1 := &core.CrossBoundaryEdge{
		SourceNodeID:     "frontend.App.fetchUsers",
		TargetNodeID:     "backend.HandleGetUsers",
		Type:             core.EdgeConsumesAPI,
		ContractSchemaID: "openapi.spec.v1#/paths/users/get",
		Metadata: map[string]string{
			"method": "GET",
			"path":   "/api/v1/users",
		},
	}

	edge2 := &core.CrossBoundaryEdge{
		SourceNodeID:     "frontend.App.fetchUsers",
		TargetNodeID:     "backend.HandleGetUsers",
		Type:             core.EdgeConsumesAPI,
		ContractSchemaID: "openapi.spec.v1#/paths/users/get",
		Metadata: map[string]string{
			"path":   "/api/v1/users",
			"method": "GET",
		},
	}

	hash1 := core.HashCrossBoundaryEdge(edge1)
	hash2 := core.HashCrossBoundaryEdge(edge2)
	if hash1 != hash2 {
		t.Fatalf("expected identical edge hashes: %s vs %s", hash1, hash2)
	}

	if err := edge1.Validate(); err != nil {
		t.Fatalf("edge1 should be valid: %v", err)
	}

	invalidEdge := &core.CrossBoundaryEdge{Type: "UNKNOWN"}
	if err := invalidEdge.Validate(); err == nil {
		t.Fatalf("invalid edge should fail validation")
	}
}

func TestComputeMerkleRoot(t *testing.T) {
	// Empty leaves
	emptyRoot := core.ComputeMerkleRoot([]string{})
	if emptyRoot == "" {
		t.Fatalf("empty leaves should produce valid root hash")
	}

	// Single leaf
	singleRoot := core.ComputeMerkleRoot([]string{"leaf1"})
	if singleRoot == "" {
		t.Fatalf("single leaf should produce valid root hash")
	}

	// 2 leaves
	twoRoots1 := core.ComputeMerkleRoot([]string{"leaf1", "leaf2"})
	twoRoots2 := core.ComputeMerkleRoot([]string{"leaf1", "leaf2"})
	if twoRoots1 != twoRoots2 {
		t.Fatalf("merkle root must be deterministic")
	}

	// 3 leaves (odd balance)
	oddRoot1 := core.ComputeMerkleRoot([]string{"leaf1", "leaf2", "leaf3"})
	oddRoot2 := core.ComputeMerkleRoot([]string{"leaf1", "leaf2", "leaf3"})
	if oddRoot1 != oddRoot2 {
		t.Fatalf("odd leaves merkle root must be deterministic")
	}

	// 5 leaves
	fiveRoot := core.ComputeMerkleRoot([]string{"leaf1", "leaf2", "leaf3", "leaf4", "leaf5"})
	if fiveRoot == "" {
		t.Fatalf("5 leaves should produce valid root hash")
	}
}

func TestWorkspaceManifestNodeDeterministicHashing(t *testing.T) {
	manifest1 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-polyglot-cloud",
		UniverseID:  "main",
		Components:  []string{"comp-infra-hash", "comp-backend-hash", "comp-frontend-hash"},
		CrossEdges: []core.CrossBoundaryEdge{
			{
				SourceNodeID: "comp-frontend",
				TargetNodeID: "comp-backend",
				Type:         core.EdgeConsumesAPI,
			},
			{
				SourceNodeID: "comp-backend",
				TargetNodeID: "comp-infra",
				Type:         core.EdgeDeploysTo,
			},
		},
		Lineage: core.LineageEnvelope{
			UserID:     "architect-1",
			UserPrompt: "Initial polyglot cloud setup",
			Intent:     "Bootstrap project workspace",
		},
	}

	// Permute components and cross edges
	manifest2 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-polyglot-cloud",
		UniverseID:  "main",
		Components:  []string{"comp-frontend-hash", "comp-infra-hash", "comp-backend-hash"},
		CrossEdges: []core.CrossBoundaryEdge{
			{
				SourceNodeID: "comp-backend",
				TargetNodeID: "comp-infra",
				Type:         core.EdgeDeploysTo,
			},
			{
				SourceNodeID: "comp-frontend",
				TargetNodeID: "comp-backend",
				Type:         core.EdgeConsumesAPI,
			},
		},
		Lineage: core.LineageEnvelope{
			UserID:     "architect-1",
			UserPrompt: "Initial polyglot cloud setup",
			Intent:     "Bootstrap project workspace",
		},
	}

	hash1, err1 := core.HashWorkspaceManifest(manifest1)
	if err1 != nil {
		t.Fatalf("failed to hash manifest1: %v", err1)
	}
	hash2, err2 := core.HashWorkspaceManifest(manifest2)
	if err2 != nil {
		t.Fatalf("failed to hash manifest2: %v", err2)
	}

	if hash1 != hash2 {
		t.Fatalf("expected identical manifest Merkle root hash: %s vs %s", hash1, hash2)
	}
	if manifest1.MerkleRootHash != hash1 {
		t.Fatalf("manifest1.MerkleRootHash was not updated: %s", manifest1.MerkleRootHash)
	}

	if err := manifest1.Validate(); err != nil {
		t.Fatalf("manifest1 should be valid: %v", err)
	}

	// JSON roundtrip
	data, err := json.Marshal(manifest1)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	var roundtrip core.WorkspaceManifestNode
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("failed to unmarshal manifest: %v", err)
	}
	if roundtrip.WorkspaceID != manifest1.WorkspaceID || roundtrip.MerkleRootHash != manifest1.MerkleRootHash {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", roundtrip, manifest1)
	}
}
