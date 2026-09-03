package mutation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs/golang"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func setupTestEnvironment(t *testing.T, universeID string) (*storage.BlobStore, *storage.GraphEngine, *storage.UniverseManager, string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "cosm_op_test_*")
	if err != nil {
		t.Fatalf("TempDir: %v", err)
	}

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

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	_, err = universeMgr.CreateUniverse(universeID, "")
	if err != nil {
		t.Fatalf("CreateUniverse: %v", err)
	}

	cleanup := func() {
		graphEngine.Close()
		os.RemoveAll(tempDir)
	}

	return blobStore, graphEngine, universeMgr, tempDir, cleanup
}

func TestOperations_ResolveSymbol(t *testing.T) {
	blobStore, graphEngine, universeMgr, _, cleanup := setupTestEnvironment(t, "universe-test-resolve")
	defer cleanup()

	lineageEnv := core.LineageEnvelope{
		UserID:    "test-user",
		Timestamp: time.Now().UTC(),
	}

	// Create Go Function Symbol with structured GoFuncSymbol payload
	fnSymbol := golang.GoFuncSymbol{
		Name:         "ProcessPayment",
		Doc:          "ProcessPayment processes an incoming payment.",
		IsMethod:     true,
		ReceiverName: "s",
		ReceiverType: "*BillingService",
		Params: []golang.GoFuncParam{
			{Name: "amount", Type: "float64"},
		},
		Results:    []string{"error"},
		BodySource: "return nil",
	}
	fnPayload, _ := json.Marshal(fnSymbol)

	sym1 := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "services/billing::BillingService.ProcessPayment",
		Signature:  "func (s *BillingService) ProcessPayment(amount float64) error",
		Docstring:  "ProcessPayment processes an incoming payment.",
		ASTPayload: fnPayload,
		Lineage:    lineageEnv,
	}
	d1, _ := json.Marshal(sym1)
	sym1Hash, _ := blobStore.Put(d1)
	sym1.NodeID = sym1Hash

	comp := &core.ComponentNode{
		Name:        "services/billing",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{sym1Hash},
		Metadata: map[string]string{
			"dir_path": "services/billing",
		},
		Lineage: lineageEnv,
	}
	cBytes, _ := json.Marshal(comp)
	cHash, _ := blobStore.Put(cBytes)
	comp.ComponentID = cHash

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-test",
		UniverseID:  "universe-test-resolve",
		Components:  []string{cHash},
		Lineage:     lineageEnv,
		CreatedAt:   time.Now().UTC(),
	}
	mBytes, _ := json.Marshal(manifest)
	mHash, _ := blobStore.Put(mBytes)
	manifest.MerkleRootHash = mHash
	_, _ = universeMgr.CommitManifest("universe-test-resolve", manifest)

	surgeryEngine := NewSurgeryEngine(blobStore, graphEngine, universeMgr)

	// Test 1: Resolve by full Node ID
	r1, err := surgeryEngine.ResolveSymbol("universe-test-resolve", sym1Hash)
	if err != nil || r1.SymbolNode.NodeID != sym1Hash {
		t.Errorf("Resolve by node ID failed: %v", err)
	}

	// Test 2: Resolve by scoped path "services/billing::BillingService.ProcessPayment"
	r2, err := surgeryEngine.ResolveSymbol("universe-test-resolve", "services/billing::BillingService.ProcessPayment")
	if err != nil || r2.SymbolNode.NodeID != sym1Hash {
		t.Errorf("Resolve by scoped path failed: %v", err)
	}

	// Test 3: Resolve by dotted suffix "BillingService.ProcessPayment"
	r3, err := surgeryEngine.ResolveSymbol("universe-test-resolve", "BillingService.ProcessPayment")
	if err != nil || r3.SymbolNode.NodeID != sym1Hash {
		t.Errorf("Resolve by dotted suffix failed: %v", err)
	}

	// Test 4: Resolve by bare method name "ProcessPayment"
	r4, err := surgeryEngine.ResolveSymbol("universe-test-resolve", "ProcessPayment")
	if err != nil || r4.SymbolNode.NodeID != sym1Hash {
		t.Errorf("Resolve by bare method name failed: %v", err)
	}
}

func TestOperations_ApplyBatch_ReplaceFunctionBodyAndAdd(t *testing.T) {
	blobStore, graphEngine, universeMgr, _, cleanup := setupTestEnvironment(t, "universe-batch-test")
	defer cleanup()

	lineageEnv := core.LineageEnvelope{
		UserID:    "agent-surgeon",
		Timestamp: time.Now().UTC(),
	}

	// Setup Go Function Symbol
	fnSymbol := golang.GoFuncSymbol{
		Name:       "CalculateTotal",
		Doc:        "CalculateTotal calculates subtotal.",
		Params:     []golang.GoFuncParam{{Name: "items", Type: "[]int"}},
		Results:    []string{"int"},
		BodySource: "return 0",
	}
	fnPayload, _ := json.Marshal(fnSymbol)

	sym1 := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "pkg/calc::CalculateTotal",
		Signature:  "func CalculateTotal(items []int) int",
		Docstring:  "CalculateTotal calculates subtotal.",
		ASTPayload: fnPayload,
		Lineage:    lineageEnv,
	}
	d1, _ := json.Marshal(sym1)
	sym1Hash, _ := blobStore.Put(d1)
	sym1.NodeID = sym1Hash

	comp := &core.ComponentNode{
		Name:        "pkg/calc",
		Type:        core.CompLibrary,
		Language:    core.LangGo,
		SymbolNodes: []string{sym1Hash},
		Lineage:     lineageEnv,
	}
	cBytes, _ := json.Marshal(comp)
	cHash, _ := blobStore.Put(cBytes)
	comp.ComponentID = cHash

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-batch",
		UniverseID:  "universe-batch-test",
		Components:  []string{cHash},
		Lineage:     lineageEnv,
		CreatedAt:   time.Now().UTC(),
	}
	mBytes, _ := json.Marshal(manifest)
	mHash, _ := blobStore.Put(mBytes)
	manifest.MerkleRootHash = mHash
	_, _ = universeMgr.CommitManifest("universe-batch-test", manifest)

	surgeryEngine := NewSurgeryEngine(blobStore, graphEngine, universeMgr)

	// Execute Batch with 3 operations:
	// 1. replace_function_body on CalculateTotal
	// 2. add_after CalculateTotal -> new helper function
	// 3. add_import "fmt"
	batch := &ASTEditBatch{
		UniverseID: "universe-batch-test",
		Lineage:    lineageEnv,
		Operations: []ASTOperation{
			{
				Operation: OpReplaceFunctionBody,
				Target:    "CalculateTotal",
				Content:   "sum := 0\nfor _, v := range items {\n\tsum += v\n}\nreturn sum",
			},
			{
				Operation: OpAddAfter,
				Target:    "CalculateTotal",
				Content:   "func PrintTotal(total int) {\n\tprintln(total)\n}",
			},
			{
				Operation: OpAddImport,
				Target:    "pkg/calc",
				Content:   "\"fmt\"",
			},
		},
	}

	result, err := surgeryEngine.ApplyASTEditBatch(batch)
	if err != nil {
		t.Fatalf("ApplyASTEditBatch failed: %v", err)
	}

	if result.AppliedOperations != 3 {
		t.Errorf("Expected 3 applied operations, got %d", result.AppliedOperations)
	}
	if result.NewManifestHash == result.OldManifestHash {
		t.Errorf("Expected manifest hash to change after batch operations")
	}
	if len(result.ModifiedSymbols) != 1 {
		t.Errorf("Expected 1 modified symbol, got %d", len(result.ModifiedSymbols))
	}
	if len(result.AddedSymbols) != 1 {
		t.Errorf("Expected 1 added symbol, got %d", len(result.AddedSymbols))
	}

	// Verify the updated function body in BlobStore
	updatedSymBytes, err := blobStore.Get(result.ModifiedSymbols[0])
	if err != nil {
		t.Fatalf("Failed to fetch modified symbol: %v", err)
	}
	var updatedSym core.ASTSymbolNode
	_ = json.Unmarshal(updatedSymBytes, &updatedSym)

	var updatedFn golang.GoFuncSymbol
	_ = json.Unmarshal(updatedSym.ASTPayload, &updatedFn)

	if updatedFn.Name != "CalculateTotal" {
		t.Errorf("Expected signature name preserved 'CalculateTotal', got %s", updatedFn.Name)
	}
	if updatedFn.BodySource != "sum := 0\nfor _, v := range items {\n\tsum += v\n}\nreturn sum" {
		t.Errorf("Unexpected body source: %s", updatedFn.BodySource)
	}
}

func TestOperations_PythonFunctionBodySplicing(t *testing.T) {
	blobStore, graphEngine, universeMgr, _, cleanup := setupTestEnvironment(t, "universe-py-test")
	defer cleanup()

	lineageEnv := core.LineageEnvelope{
		UserID:    "python-surgeon",
		Timestamp: time.Now().UTC(),
	}

	initialPythonCode := `def fetch_user_profile(user_id: str) -> dict:
    # Legacy placeholder
    return {}`

	pySym := &core.ASTSymbolNode{
		Language:   core.LangPython,
		NodeType:   "FunctionDecl",
		Identifier: "services/user.py::fetch_user_profile",
		ASTPayload: []byte(initialPythonCode),
		Lineage:    lineageEnv,
	}
	d, _ := json.Marshal(pySym)
	symHash, _ := blobStore.Put(d)
	pySym.NodeID = symHash

	comp := &core.ComponentNode{
		Name:        "services/user",
		Type:        core.CompService,
		Language:    core.LangPython,
		SymbolNodes: []string{symHash},
		Lineage:     lineageEnv,
	}
	cBytes, _ := json.Marshal(comp)
	cHash, _ := blobStore.Put(cBytes)
	comp.ComponentID = cHash

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-py",
		UniverseID:  "universe-py-test",
		Components:  []string{cHash},
		Lineage:     lineageEnv,
		CreatedAt:   time.Now().UTC(),
	}
	mBytes, _ := json.Marshal(manifest)
	mHash, _ := blobStore.Put(mBytes)
	manifest.MerkleRootHash = mHash
	_, _ = universeMgr.CommitManifest("universe-py-test", manifest)

	surgeryEngine := NewSurgeryEngine(blobStore, graphEngine, universeMgr)

	// Perform replace_function_body
	batch := &ASTEditBatch{
		UniverseID: "universe-py-test",
		Lineage:    lineageEnv,
		Operations: []ASTOperation{
			{
				Operation: OpReplaceFunctionBody,
				Target:    "fetch_user_profile",
				Content:   "user = db.get(user_id)\nif not user:\n    raise NotFoundError()\nreturn user.to_dict()",
			},
		},
	}

	res, err := surgeryEngine.ApplyASTEditBatch(batch)
	if err != nil {
		t.Fatalf("ApplyASTEditBatch Python failed: %v", err)
	}

	updatedBytes, err := blobStore.Get(res.ModifiedSymbols[0])
	if err != nil {
		t.Fatalf("Failed to fetch mutated python symbol: %v", err)
	}
	var updatedSym core.ASTSymbolNode
	_ = json.Unmarshal(updatedBytes, &updatedSym)

	updatedCode := string(updatedSym.ASTPayload)
	expectedContains := "def fetch_user_profile(user_id: str) -> dict:\n    user = db.get(user_id)"
	if !containsSubstring(updatedCode, expectedContains) {
		t.Errorf("Expected updated python code to contain %q, got:\n%s", expectedContains, updatedCode)
	}
}

func TestOperations_AllOperationsSuite(t *testing.T) {
	blobStore, graphEngine, universeMgr, _, cleanup := setupTestEnvironment(t, "universe-suite-test")
	defer cleanup()

	lineageEnv := core.LineageEnvelope{
		UserID:    "suite-agent",
		Timestamp: time.Now().UTC(),
	}

	sym1 := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "pkg/service::Func1",
		ASTPayload: []byte("func Func1() {}"),
		Lineage:    lineageEnv,
	}
	d1, _ := json.Marshal(sym1)
	sym1Hash, _ := blobStore.Put(d1)
	sym1.NodeID = sym1Hash

	sym2 := &core.ASTSymbolNode{
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "pkg/service::Func2",
		ASTPayload: []byte("func Func2() {}"),
		Lineage:    lineageEnv,
	}
	d2, _ := json.Marshal(sym2)
	sym2Hash, _ := blobStore.Put(d2)
	sym2.NodeID = sym2Hash

	comp := &core.ComponentNode{
		Name:        "pkg/service",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{sym1Hash, sym2Hash},
		Lineage:     lineageEnv,
	}
	cBytes, _ := json.Marshal(comp)
	cHash, _ := blobStore.Put(cBytes)
	comp.ComponentID = cHash

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-suite",
		UniverseID:  "universe-suite-test",
		Components:  []string{cHash},
		Lineage:     lineageEnv,
		CreatedAt:   time.Now().UTC(),
	}
	mBytes, _ := json.Marshal(manifest)
	mHash, _ := blobStore.Put(mBytes)
	manifest.MerkleRootHash = mHash
	_, _ = universeMgr.CommitManifest("universe-suite-test", manifest)

	surgeryEngine := NewSurgeryEngine(blobStore, graphEngine, universeMgr)

	// 1. Test OpAddBefore (inserts before Func2)
	batch1 := &ASTEditBatch{
		UniverseID: "universe-suite-test",
		Lineage:    lineageEnv,
		Operations: []ASTOperation{
			{
				Operation: OpAddBefore,
				Target:    "Func2",
				Content:   "func FuncInter() {}",
			},
		},
	}
	res1, err := surgeryEngine.ApplyASTEditBatch(batch1)
	if err != nil {
		t.Fatalf("OpAddBefore failed: %v", err)
	}
	if len(res1.AddedSymbols) != 1 {
		t.Fatalf("Expected 1 added symbol, got %d", len(res1.AddedSymbols))
	}

	// 2. Test OpReplaceFunction (replaces Func1 entirely)
	batch2 := &ASTEditBatch{
		UniverseID: "universe-suite-test",
		Lineage:    lineageEnv,
		Operations: []ASTOperation{
			{
				Operation: OpReplaceFunction,
				Target:    "Func1",
				Content:   "func Func1(ctx context.Context) error { return nil }",
			},
		},
	}
	res2, err := surgeryEngine.ApplyASTEditBatch(batch2)
	if err != nil {
		t.Fatalf("OpReplaceFunction failed: %v", err)
	}
	if len(res2.ModifiedSymbols) != 1 {
		t.Fatalf("Expected 1 modified symbol, got %d", len(res2.ModifiedSymbols))
	}

	// 3. Test OpReplaceImports and OpDelete
	batch3 := &ASTEditBatch{
		UniverseID: "universe-suite-test",
		Lineage:    lineageEnv,
		Operations: []ASTOperation{
			{
				Operation: OpReplaceImports,
				Target:    "pkg/service",
				Content:   "\"context\",\"fmt\"",
			},
			{
				Operation: OpDelete,
				Target:    "Func2",
				Content:   "",
			},
		},
	}
	res3, err := surgeryEngine.ApplyASTEditBatch(batch3)
	if err != nil {
		t.Fatalf("Batch 3 failed: %v", err)
	}
	if len(res3.DeletedSymbols) != 1 {
		t.Fatalf("Expected 1 deleted symbol, got %d", len(res3.DeletedSymbols))
	}

	// Verify final manifest has 2 symbols (modified Func1 and FuncInter)
	finalManifest, err := universeMgr.GetUniverseManifest("universe-suite-test")
	if err != nil {
		t.Fatalf("GetUniverseManifest failed: %v", err)
	}
	finalCompBytes, _ := blobStore.Get(finalManifest.Components[0])
	var finalComp core.ComponentNode
	_ = json.Unmarshal(finalCompBytes, &finalComp)

	if len(finalComp.SymbolNodes) != 2 {
		t.Errorf("Expected 2 symbols in final component, got %d", len(finalComp.SymbolNodes))
	}
	if finalComp.Metadata["imports"] != "\"context\",\"fmt\"" {
		t.Errorf("Expected replaced imports, got %s", finalComp.Metadata["imports"])
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || filepath.Base(s) != "" && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
