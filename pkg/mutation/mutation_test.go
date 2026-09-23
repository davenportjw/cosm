package mutation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func TestSurgeryEngine_MutateSymbolAndDeduplicate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mutation_test_*")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	objectsDir := filepath.Join(tempDir, "objects")
	dbPath := filepath.Join(tempDir, "graph.db")

	blobStore, err := storage.NewBlobStore(objectsDir)
	if err != nil {
		t.Fatalf("BlobStore init: %v", err)
	}
	graphEngine, err := storage.NewGraphEngine(dbPath)
	if err != nil {
		t.Fatalf("GraphEngine init: %v", err)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	universeID := "universe-main"
	_, err = universeMgr.CreateUniverse(universeID, "")
	if err != nil {
		t.Fatalf("CreateUniverse: %v", err)
	}

	lineageEnv := core.LineageEnvelope{
		UserID:           "test-user",
		UserPrompt:       "Initial setup",
		ExecutingAgentID: "agent-init",
		Intent:           "Seed symbols",
		Timestamp:        time.Now().UTC(),
	}

	// Create 3 symbols
	sym1 := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "main.FuncA",
		ASTPayload: []byte("func FuncA() { println(1) }"),
		Lineage:    lineageEnv,
	}
	d1, _ := json.Marshal(sym1)
	sym1Hash, _ := blobStore.Put(d1)
	sym1.NodeID = sym1Hash

	sym2 := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "main.FuncB",
		ASTPayload: []byte("func FuncB() { println(2) }"),
		Lineage:    lineageEnv,
	}
	d2, _ := json.Marshal(sym2)
	sym2Hash, _ := blobStore.Put(d2)
	sym2.NodeID = sym2Hash

	sym3 := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "main.FuncC",
		ASTPayload: []byte("func FuncC() { println(3) }"),
		Lineage:    lineageEnv,
	}
	d3, _ := json.Marshal(sym3)
	sym3Hash, _ := blobStore.Put(d3)
	sym3.NodeID = sym3Hash

	// Create Component containing all 3 symbols
	comp := &core.ComponentNode{
		Name:        "service-core",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{sym1Hash, sym2Hash, sym3Hash},
		Lineage:     lineageEnv,
	}
	compBytes, _ := json.Marshal(comp)
	compHash, _ := blobStore.Put(compBytes)
	comp.ComponentID = compHash

	// Create Manifest
	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-test",
		UniverseID:  universeID,
		Components:  []string{compHash},
		Lineage:     lineageEnv,
		CreatedAt:   time.Now().UTC(),
	}
	manBytes, _ := json.Marshal(manifest)
	manHash, _ := blobStore.Put(manBytes)
	manifest.MerkleRootHash = manHash
	_, err = universeMgr.CommitManifest(universeID, manifest)
	if err != nil {
		t.Fatalf("CommitManifest: %v", err)
	}

	// Execute Surgical Mutation on Sym2
	surgeryEngine := NewSurgeryEngine(blobStore, graphEngine, universeMgr)
	mutLineage := core.LineageEnvelope{
		UserID:           "agent-user",
		UserPrompt:       "Update FuncB to return error",
		ExecutingAgentID: "agent-coder-1",
		Intent:           "Mutate FuncB",
		Timestamp:        time.Now().UTC(),
	}

	newPayload := []byte("func FuncB() error { return nil }")
	res, err := surgeryEngine.MutateSymbol(universeID, sym2Hash, newPayload, mutLineage)
	if err != nil {
		t.Fatalf("MutateSymbol failed: %v", err)
	}

	if res.OldSymbolID != sym2Hash {
		t.Errorf("Expected OldSymbolID %s, got %s", sym2Hash, res.OldSymbolID)
	}
	if res.NewSymbolID == sym2Hash {
		t.Errorf("Expected new symbol hash to differ from old hash")
	}
	if res.SymbolIdentifier != "main.FuncB" {
		t.Errorf("Expected SymbolIdentifier main.FuncB, got %s", res.SymbolIdentifier)
	}
	if res.ComponentName != "service-core" {
		t.Errorf("Expected ComponentName service-core, got %s", res.ComponentName)
	}
	if res.DeduplicatedSymbolsCount != 2 {
		t.Errorf("Expected 2 deduplicated sibling symbols, got %d", res.DeduplicatedSymbolsCount)
	}
	if res.NewManifestHash == res.OldManifestHash {
		t.Errorf("Expected new manifest hash to differ after mutation")
	}

	// Verify new symbol has non-empty NodeID in object storage
	newSymBytes, err := blobStore.Get(res.NewSymbolID)
	if err != nil {
		t.Fatalf("Get mutated symbol blob: %v", err)
	}
	var loadedSym core.ASTSymbolNode
	if err := json.Unmarshal(newSymBytes, &loadedSym); err != nil {
		t.Fatalf("Unmarshal mutated symbol: %v", err)
	}
	if loadedSym.NodeID == "" {
		t.Errorf("Expected loaded symbol NodeID to be non-empty")
	}

	// Verify new component in storage has [sym1, newSym2, sym3]
	updatedCompBytes, err := blobStore.Get(res.NewComponentID)
	if err != nil {
		t.Fatalf("Get updated component: %v", err)
	}
	var updatedComp core.ComponentNode
	_ = json.Unmarshal(updatedCompBytes, &updatedComp)

	if len(updatedComp.SymbolNodes) != 3 {
		t.Fatalf("Expected 3 symbols in updated component, got %d", len(updatedComp.SymbolNodes))
	}
	if updatedComp.SymbolNodes[0] != sym1Hash || updatedComp.SymbolNodes[1] != res.NewSymbolID || updatedComp.SymbolNodes[2] != sym3Hash {
		t.Errorf("Unexpected symbol list in component: %v", updatedComp.SymbolNodes)
	}
}

func TestSurgeryEngine_MutateSymbolFromCode_Polyglot(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "polyglot_mutation_*")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	objectsDir := filepath.Join(tempDir, "objects")
	dbPath := filepath.Join(tempDir, "graph.db")

	blobStore, err := storage.NewBlobStore(objectsDir)
	if err != nil {
		t.Fatalf("BlobStore init: %v", err)
	}
	graphEngine, err := storage.NewGraphEngine(dbPath)
	if err != nil {
		t.Fatalf("GraphEngine init: %v", err)
	}
	defer graphEngine.Close()

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	universeID := "universe-polyglot"
	_, err = universeMgr.CreateUniverse(universeID, "")
	if err != nil {
		t.Fatalf("CreateUniverse: %v", err)
	}

	lineageEnv := core.LineageEnvelope{
		UserID:    "rust-architect",
		Timestamp: time.Now().UTC(),
	}

	// 1. Setup initial Rust Symbol
	initialRustCode := `pub struct UserModel {
    pub id: String,
    pub name: String,
}`
	rustSym := &core.ASTSymbolNode{
		Language:   core.LangRust,
		NodeType:   "StructDef",
		Identifier: "UserModel",
		ASTPayload: []byte(initialRustCode),
		Lineage:    lineageEnv,
	}
	d, _ := json.Marshal(rustSym)
	rustSymHash, _ := blobStore.Put(d)
	rustSym.NodeID = rustSymHash

	comp := &core.ComponentNode{
		Name:        "rust-user-lib",
		Type:        core.CompLibrary,
		Language:    core.LangRust,
		SymbolNodes: []string{rustSymHash},
		Lineage:     lineageEnv,
	}
	cBytes, _ := json.Marshal(comp)
	cHash, _ := blobStore.Put(cBytes)
	comp.ComponentID = cHash

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-polyglot",
		UniverseID:  universeID,
		Components:  []string{cHash},
		Lineage:     lineageEnv,
		CreatedAt:   time.Now().UTC(),
	}
	mBytes, _ := json.Marshal(manifest)
	mHash, _ := blobStore.Put(mBytes)
	manifest.MerkleRootHash = mHash
	_, _ = universeMgr.CommitManifest(universeID, manifest)

	// 2. Perform Code Mutation on Rust struct
	updatedRustCode := `#[derive(Debug, Serialize, Deserialize)]
pub struct UserModel {
    pub id: String,
    pub email: String,
    pub is_active: bool,
}`
	surgeryEngine := NewSurgeryEngine(blobStore, graphEngine, universeMgr)
	res, err := surgeryEngine.MutateSymbolFromCode(universeID, rustSymHash, updatedRustCode, lineageEnv)
	if err != nil {
		t.Fatalf("MutateSymbolFromCode failed: %v", err)
	}

	if res.NewSymbolID == rustSymHash {
		t.Errorf("Expected new symbol hash to differ after mutation")
	}
	if res.NewManifestHash == mHash {
		t.Errorf("Expected new manifest hash to differ")
	}
}

func TestContractCascadeEngine_AssessImpact(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cascade_test_*")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "graph.db")
	graphEngine, err := storage.NewGraphEngine(dbPath)
	if err != nil {
		t.Fatalf("GraphEngine init: %v", err)
	}
	defer graphEngine.Close()

	routeSymID := "go:server:HandleGetUsers"
	feCallerID := "ts:web:fetchUsers"
	infraID := "tf:deploy:backend_service"

	// Seed edges
	_ = graphEngine.PutEdge(storage.EdgeRecord{
		SourceID: feCallerID,
		TargetID: routeSymID,
		EdgeType: core.EdgeConsumesAPI,
	})
	_ = graphEngine.PutEdge(storage.EdgeRecord{
		SourceID: infraID,
		TargetID: routeSymID,
		EdgeType: core.EdgeDeploysTo,
	})

	cascadeEngine := NewContractCascadeEngine(graphEngine)
	assessment, err := cascadeEngine.AssessImpact(routeSymID)
	if err != nil {
		t.Fatalf("AssessImpact failed: %v", err)
	}

	if len(assessment.IncomingEdges) != 2 {
		t.Errorf("Expected 2 incoming edges, got %d", len(assessment.IncomingEdges))
	}
	if len(assessment.Warnings) != 2 {
		t.Errorf("Expected 2 warnings, got %d", len(assessment.Warnings))
	}
	if assessment.RiskScore < 0.5 {
		t.Errorf("Expected RiskScore >= 0.5, got %f", assessment.RiskScore)
	}
}
