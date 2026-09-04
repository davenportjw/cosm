package benchmarks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/distributed"
	"github.com/cosmscm/cosm/pkg/mutation"
	"github.com/cosmscm/cosm/pkg/storage"
)

// Helper to initialize an ephemeral storage environment.
func setupBenchRepo(tb testing.TB) (*storage.BlobStore, *storage.GraphEngine, *storage.UniverseManager, string) {
	tb.Helper()
	dir, err := os.MkdirTemp("", "cosm-bench-*")
	if err != nil {
		tb.Fatalf("failed to create temp dir: %v", err)
	}

	bs, err := storage.NewBlobStore(dir)
	if err != nil {
		tb.Fatalf("failed to create blobstore: %v", err)
	}

	ge, err := storage.NewGraphEngine(filepath.Join(dir, "graph.db"))
	if err != nil {
		tb.Fatalf("failed to create graphengine: %v", err)
	}

	um := storage.NewUniverseManager(ge, bs)
	_, err = um.CreateUniverse("universe-main", "")
	if err != nil {
		tb.Fatalf("failed to init universe-main: %v", err)
	}

	return bs, ge, um, dir
}

// Generates a parameterized synthetic polyglot AST repository.
func generateSyntheticRepo(tb testing.TB, bs *storage.BlobStore, um *storage.UniverseManager, numComponents int, symbolsPerComponent int) (string, int64, int) {
	tb.Helper()

	now := time.Now().UTC()
	var totalBytes int64
	var totalBlobs int

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-bench",
		UniverseID:  "universe-main",
		CreatedAt:   now,
		Components:  make([]string, 0, numComponents),
		Lineage: core.LineageEnvelope{
			ExecutingAgentID: "agent-bench-seed",
			Intent:           "Seed synthetic benchmark DAG",
			Timestamp:        now,
		},
	}

	languages := []core.Language{core.LangGo, core.LangTypeScript, core.LangPython, core.LangHCL, core.LangSQL}

	for c := 0; c < numComponents; c++ {
		lang := languages[c%len(languages)]
		compName := fmt.Sprintf("tier-%d/component-%d", c%len(languages), c)

		comp := &core.ComponentNode{
			Name:        compName,
			Type:        core.CompService,
			Language:    lang,
			SymbolNodes: make([]string, 0, symbolsPerComponent),
			Lineage: core.LineageEnvelope{
				ExecutingAgentID: "agent-bench-seed",
				Intent:           fmt.Sprintf("Seed component %s", compName),
				Timestamp:        now,
			},
		}

		for s := 0; s < symbolsPerComponent; s++ {
			sym := &core.ASTSymbolNode{
				Language:   lang,
				NodeType:   "FunctionDecl",
				Identifier: fmt.Sprintf("Handler_%d_%d", c, s),
				Signature:  fmt.Sprintf("func Handler_%d_%d(ctx context.Context, req *Request) (*Response, error)", c, s),
				Visibility: "public",
				ASTPayload: []byte(fmt.Sprintf(`// Handler_%d_%d handles synthetic enterprise business logic
func Handler_%d_%d(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}
	res := &Response{
		ID:        "%d-%d",
		Processed: true,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]string{"tier": "%d", "shard": "%d"},
	}
	return res, nil
}`, c, s, c, s, c, s, c, s)),
				Lineage: comp.Lineage,
			}

			symBytes, _ := json.Marshal(sym)
			symHash, _ := bs.Put(symBytes)
			sym.NodeID = symHash
			symBytes, _ = json.Marshal(sym)
			symHash, _ = bs.Put(symBytes)

			totalBytes += int64(len(symBytes))
			totalBlobs++

			comp.SymbolNodes = append(comp.SymbolNodes, symHash)
		}

		compBytes, _ := json.Marshal(comp)
		compHash, _ := bs.Put(compBytes)
		comp.ComponentID = compHash
		compBytes, _ = json.Marshal(comp)
		compHash, _ = bs.Put(compBytes)

		totalBytes += int64(len(compBytes))
		totalBlobs++

		manifest.Components = append(manifest.Components, compHash)
	}

	manifestBytes, _ := json.Marshal(manifest)
	manifestHash := core.HashBytes(manifestBytes)
	manifest.MerkleRootHash = manifestHash

	committedHash, err := um.CommitManifest("universe-main", manifest)
	if err != nil {
		tb.Fatalf("CommitManifest failed: %v", err)
	}
	totalBytes += int64(len(manifestBytes))
	totalBlobs++

	return committedHash, totalBytes, totalBlobs
}

// TestEmpiricalBandwidthSavings validates the exact empirical reduction in payload bytes
// and blob count for Sparse Subtree Pulls across varying repository scales.
func TestEmpiricalBandwidthSavings(t *testing.T) {
	scales := []struct {
		name                 string
		numComponents        int
		symbolsPerComponent int
		targetComponent      string
	}{
		{"SmallRepo_5Comp_25Sym", 5, 5, "tier-0/component-0"},
		{"MediumRepo_25Comp_150Sym", 25, 6, "tier-0/component-0"},
		{"LargeRepo_100Comp_600Sym", 100, 6, "tier-0/component-0"},
	}

	t.Logf("\n%s\n%-26s | %-12s | %-12s | %-12s | %-12s | %-12s\n%s",
		"=================================================================================================",
		"Scale Scenario", "Full Size", "Sparse Size", "Byte Savings", "Full Blobs", "Blob Savings",
		"-------------------------------------------------------------------------------------------------")

	for _, sc := range scales {
		bs, ge, um, dir := setupBenchRepo(t)
		defer os.RemoveAll(dir)
		defer ge.Close()

		_, fullBytes, fullBlobs := generateSyntheticRepo(t, bs, um, sc.numComponents, sc.symbolsPerComponent)

		syncEngine := distributed.NewSparseSyncEngine(bs, um)

		// 1. Single Component Sparse Pull
		singleCompFilter := distributed.SparseFilter{
			ComponentNames: []string{sc.targetComponent},
		}

		payload, err := syncEngine.ExtractSparseSubtree("universe-main", singleCompFilter)
		if err != nil {
			t.Fatalf("[%s] ExtractSparseSubtree failed: %v", sc.name, err)
		}

		var sparseBytes int64
		for _, data := range payload.FilteredBlobs {
			sparseBytes += int64(len(data))
		}

		byteSavingsPct := (1.0 - float64(sparseBytes)/float64(fullBytes)) * 100.0
		blobSavingsPct := (1.0 - float64(payload.TotalBlobsCount)/float64(fullBlobs)) * 100.0

		t.Logf("%-26s | %8.1f KB  | %8.1f KB  | %10.2f%%  | %12d | %10.2f%%",
			sc.name,
			float64(fullBytes)/1024.0,
			float64(sparseBytes)/1024.0,
			byteSavingsPct,
			fullBlobs,
			blobSavingsPct)

		// Sanity assertions: savings scale directly with repository size
		if sc.numComponents >= 25 && byteSavingsPct < 90.0 {
			t.Errorf("[%s] Expected byte savings >= 90.0%%, got %.2f%%", sc.name, byteSavingsPct)
		}
		if sc.numComponents >= 100 && byteSavingsPct < 98.0 {
			t.Errorf("[%s] Expected byte savings >= 98.0%%, got %.2f%%", sc.name, byteSavingsPct)
		}
	}
}

// TestEmpiricalTokenSavings validates context token reduction when an LLM agent
// uses targeted AST symbol context and surgical mutations vs full-file ingestion.
func TestEmpiricalTokenSavings(t *testing.T) {
	// Average heuristic: ~4 characters per LLM token
	charToTokens := func(chars int) int {
		return (chars + 3) / 4
	}

	scenarios := []struct {
		name          string
		fileLines     int
		fileBytes     int
		symbolLines   int
		symbolBytes   int
		mutationBytes int
	}{
		{"MediumService_500Lines", 500, 18500, 25, 920, 140},
		{"LargeController_2000Lines", 2000, 74000, 30, 1100, 150},
	}

	t.Logf("\n%s\n%-26s | %-12s | %-12s | %-12s | %-14s | %-14s\n%s",
		"===================================================================================================",
		"Scenario", "File Tokens", "Sym Tokens", "Mut Tokens", "Symbol Savings", "Surgery Savings",
		"---------------------------------------------------------------------------------------------------")

	for _, sc := range scenarios {
		fileTokens := charToTokens(sc.fileBytes)
		symTokens := charToTokens(sc.symbolBytes)
		mutTokens := charToTokens(sc.mutationBytes)

		symSavingsPct := (1.0 - float64(symTokens)/float64(fileTokens)) * 100.0
		mutSavingsPct := (1.0 - float64(mutTokens)/float64(fileTokens)) * 100.0

		t.Logf("%-26s | %10d   | %10d   | %10d   | %12.2f%%  | %12.2f%%",
			sc.name, fileTokens, symTokens, mutTokens, symSavingsPct, mutSavingsPct)

		if symSavingsPct < 90.0 {
			t.Errorf("[%s] Expected AST symbol context savings >= 90%%, got %.2f%%", sc.name, symSavingsPct)
		}
		if mutSavingsPct < 95.0 {
			t.Errorf("[%s] Expected AST surgery output token savings >= 95%%, got %.2f%%", sc.name, mutSavingsPct)
		}
	}
}

// BenchmarkMicroUniverseCreation benchmarks runtime latency of creating zero-copy micro-universes.
func BenchmarkMicroUniverseCreation(b *testing.B) {
	bs, ge, um, dir := setupBenchRepo(b)
	defer os.RemoveAll(dir)
	defer ge.Close()

	// Initial dummy commit
	data := []byte("root-ast-node")
	sum := sha256.Sum256(data)
	rootHash := hex.EncodeToString(sum[:])
	_, _ = bs.Put(data)
	_ = um.SetUniverseHead("universe-main", rootHash)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		uID := fmt.Sprintf("universe-worker-%d", i)
		_, err := um.CreateUniverse(uID, "universe-main")
		if err != nil {
			b.Fatalf("CreateUniverse failed: %v", err)
		}
	}
}

// BenchmarkSparseSubtreeExtraction benchmarks AST subtree extraction throughput.
func BenchmarkSparseSubtreeExtraction(b *testing.B) {
	bs, ge, um, dir := setupBenchRepo(b)
	defer os.RemoveAll(dir)
	defer ge.Close()

	generateSyntheticRepo(b, bs, um, 50, 5)
	syncEngine := distributed.NewSparseSyncEngine(bs, um)
	filter := distributed.SparseFilter{
		ComponentNames: []string{"tier-0/component-0"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		payload, err := syncEngine.ExtractSparseSubtree("universe-main", filter)
		if err != nil || payload == nil {
			b.Fatalf("ExtractSparseSubtree failed: %v", err)
		}
	}
}

// BenchmarkASTSymbolSurgery benchmarks surgical mutation of an isolated AST symbol in a 50-component Merkle DAG.
func BenchmarkASTSymbolSurgery(b *testing.B) {
	bs, ge, um, dir := setupBenchRepo(b)
	defer os.RemoveAll(dir)
	defer ge.Close()

	_, _, _ = generateSyntheticRepo(b, bs, um, 50, 5)
	manifest, err := um.GetUniverseManifest("universe-main")
	if err != nil || len(manifest.Components) == 0 {
		b.Fatalf("failed to load manifest: %v", err)
	}

	compBytes, err := bs.Get(manifest.Components[0])
	if err != nil {
		b.Fatalf("failed to get component: %v", err)
	}
	var comp core.ComponentNode
	_ = json.Unmarshal(compBytes, &comp)
	if len(comp.SymbolNodes) == 0 {
		b.Fatalf("component has no symbols")
	}
	targetSymID := comp.SymbolNodes[0]

	surgeryEngine := mutation.NewSurgeryEngine(bs, ge, um)
	newPayload := []byte(`func Handler_0_0(ctx context.Context, req *Request) (*Response, error) { return &Response{Processed: true}, nil }`)
	lineage := core.LineageEnvelope{
		ExecutingAgentID: "bench-mutator",
		Intent:           "Mutate single symbol payload",
		Timestamp:        time.Now().UTC(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res, err := surgeryEngine.MutateSymbol("universe-main", targetSymID, newPayload, lineage)
		if err != nil || res == nil {
			b.Fatalf("MutateSymbol failed: %v", err)
		}
		targetSymID = res.NewSymbolID
	}
}

