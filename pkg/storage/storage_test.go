package storage_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func createTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "fg-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

// -----------------------------------------------------------------------------
// BlobStore Unit Tests
// -----------------------------------------------------------------------------

func TestBlobStoreBasicOperations(t *testing.T) {
	tmpDir := createTempDir(t)
	bs, err := storage.NewBlobStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create blob store: %v", err)
	}

	payload := []byte("func HelloWorld() string { return \"Hello, Future of Git!\" }")
	hash, err := bs.Put(payload)
	if err != nil {
		t.Fatalf("failed to put blob: %v", err)
	}

	sum := sha256.Sum256(payload)
	expectedHash := hex.EncodeToString(sum[:])
	if hash != expectedHash {
		t.Fatalf("hash mismatch: got %s, expected %s", hash, expectedHash)
	}

	// Has check
	has, err := bs.Has(hash)
	if err != nil || !has {
		t.Fatalf("expected blob %s to exist, err: %v", hash, err)
	}

	// Get check
	retrieved, err := bs.Get(hash)
	if err != nil {
		t.Fatalf("failed to get blob %s: %v", hash, err)
	}
	if string(retrieved) != string(payload) {
		t.Fatalf("retrieved payload mismatch: got %q, expected %q", string(retrieved), string(payload))
	}

	// Idempotent put
	hash2, err := bs.Put(payload)
	if err != nil || hash2 != hash {
		t.Fatalf("idempotent put failed: %v", err)
	}

	// Put with matching hash
	if err := bs.PutWithHash(hash, payload); err != nil {
		t.Fatalf("put with matching hash failed: %v", err)
	}

	// Put with incorrect hash
	if err := bs.PutWithHash("0000000000000000000000000000000000000000000000000000000000000000", payload); err == nil {
		t.Fatalf("expected error when putting with mismatched hash")
	}

	// List check
	hashes, err := bs.List()
	if err != nil || len(hashes) != 1 || hashes[0] != hash {
		t.Fatalf("list mismatch: got %v, err: %v", hashes, err)
	}

	// Size check
	size, err := bs.Size()
	if err != nil || size != int64(len(payload)) {
		t.Fatalf("size mismatch: got %d, expected %d", size, len(payload))
	}

	// Delete check
	if err := bs.Delete(hash); err != nil {
		t.Fatalf("failed to delete blob: %v", err)
	}
	hasAfterDelete, _ := bs.Has(hash)
	if hasAfterDelete {
		t.Fatalf("blob should not exist after deletion")
	}
}

func TestBlobStoreIntegrityAndBitRotDetection(t *testing.T) {
	tmpDir := createTempDir(t)
	bs, err := storage.NewBlobStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create blob store: %v", err)
	}

	data1 := []byte("valid content 1")
	data2 := []byte("valid content 2 to be corrupted")

	hash1, _ := bs.Put(data1)
	hash2, _ := bs.Put(data2)

	corruptedList, err := bs.VerifyIntegrity()
	if err != nil || len(corruptedList) != 0 {
		t.Fatalf("expected 0 corrupted blobs initially, got %v", corruptedList)
	}

	// Corrupt blob 2 directly on disk
	shardDir := filepath.Join(bs.ObjectsDir(), hash2[:2])
	filePath := filepath.Join(shardDir, hash2[2:]+".bin")
	if err := os.WriteFile(filePath, []byte("tampered data"), 0644); err != nil {
		t.Fatalf("failed to tamper file: %v", err)
	}

	// Get should fail with corruption error
	_, err = bs.Get(hash2)
	if err == nil {
		t.Fatalf("expected Get to fail on corrupted blob")
	}

	// VerifyIntegrity should identify hash2 as corrupted
	corrupted, err := bs.VerifyIntegrity()
	if err != nil || len(corrupted) != 1 || corrupted[0] != hash2 {
		t.Fatalf("expected hash2 in corrupted list, got %v", corrupted)
	}

	// Prune test: keep only hash1
	keepMap := map[string]bool{hash1: true}
	prunedCount, err := bs.Prune(keepMap)
	if err != nil || prunedCount != 1 {
		t.Fatalf("expected 1 pruned blob, got %d, err: %v", prunedCount, err)
	}

	remaining, _ := bs.List()
	if len(remaining) != 1 || remaining[0] != hash1 {
		t.Fatalf("expected only hash1 to remain, got %v", remaining)
	}
}

func TestBlobStoreConcurrentWrites(t *testing.T) {
	tmpDir := createTempDir(t)
	bs, err := storage.NewBlobStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create blob store: %v", err)
	}

	var wg sync.WaitGroup
	numRoutines := 20
	blobsPerRoutine := 10

	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			for j := 0; j < blobsPerRoutine; j++ {
				data := []byte(fmt.Sprintf("routine-%d-blob-%d", routineID, j))
				hash, err := bs.Put(data)
				if err != nil {
					t.Errorf("concurrent put failed: %v", err)
					return
				}
				readData, err := bs.Get(hash)
				if err != nil || string(readData) != string(data) {
					t.Errorf("concurrent get mismatch: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	hashes, err := bs.List()
	if err != nil || len(hashes) != numRoutines*blobsPerRoutine {
		t.Fatalf("expected %d blobs stored, got %d", numRoutines*blobsPerRoutine, len(hashes))
	}
}

// -----------------------------------------------------------------------------
// GraphEngine Unit Tests
// -----------------------------------------------------------------------------

func TestGraphEngineNodeAndEdgeCRUD(t *testing.T) {
	tmpDir := createTempDir(t)
	ge, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("failed to create graph engine: %v", err)
	}
	defer ge.Close()

	node1 := storage.NodeRecord{
		NodeID:     "auth.HandleLogin",
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "HandleLogin",
		MerkleHash: "merkle-hash-login",
		Metadata:   map[string]string{"visibility": "public"},
	}
	node2 := storage.NodeRecord{
		NodeID:     "auth.TokenService",
		Language:   core.LangGo,
		NodeType:   "StructDecl",
		Identifier: "TokenService",
		MerkleHash: "merkle-hash-token",
	}

	if err := ge.PutNode(node1); err != nil {
		t.Fatalf("failed to put node1: %v", err)
	}
	if err := ge.PutNode(node2); err != nil {
		t.Fatalf("failed to put node2: %v", err)
	}

	if !ge.HasNode("auth.HandleLogin") {
		t.Fatalf("expected node to exist")
	}

	gotNode, err := ge.GetNode("auth.HandleLogin")
	if err != nil || gotNode.Identifier != "HandleLogin" {
		t.Fatalf("get node mismatch: %+v, err: %v", gotNode, err)
	}

	nodes := ge.ListNodes(core.LangGo, "FunctionDecl")
	if len(nodes) != 1 || nodes[0].NodeID != "auth.HandleLogin" {
		t.Fatalf("list nodes filter mismatch: %+v", nodes)
	}

	// Edge CRUD
	edge := storage.EdgeRecord{
		SourceID:   "auth.HandleLogin",
		TargetID:   "auth.TokenService",
		EdgeType:   core.EdgeCalls,
		ContractID: "internal/auth",
	}
	if err := ge.PutEdge(edge); err != nil {
		t.Fatalf("failed to put edge: %v", err)
	}

	edges := ge.ListEdges("auth.HandleLogin", "", "")
	if len(edges) != 1 || edges[0].TargetID != "auth.TokenService" {
		t.Fatalf("list edges mismatch: %+v", edges)
	}

	// Lineage CRUD
	lin := storage.LineageRecord{
		NodeID:     "auth.HandleLogin",
		UserID:     "eng-1",
		UserPrompt: "Implement login endpoint",
		Intent:     "Handle JWT generation",
	}
	if err := ge.PutLineage(lin); err != nil {
		t.Fatalf("failed to put lineage: %v", err)
	}

	lines := ge.GetLineageByNode("auth.HandleLogin")
	if len(lines) != 1 || lines[0].UserPrompt != "Implement login endpoint" {
		t.Fatalf("get lineage mismatch: %+v", lines)
	}

	// Universe Head CRUD
	uHead := storage.UniverseHeadRecord{
		UniverseID:       "feature/auth-jwt",
		HeadManifestHash: "hash-manifest-1",
		Status:           "active",
	}
	if err := ge.PutUniverseHead(uHead); err != nil {
		t.Fatalf("failed to put universe head: %v", err)
	}

	gotHead, err := ge.GetUniverseHead("feature/auth-jwt")
	if err != nil || gotHead.HeadManifestHash != "hash-manifest-1" {
		t.Fatalf("get universe head mismatch: %+v, err: %v", gotHead, err)
	}

	allHeads := ge.ListUniverseHeads()
	if len(allHeads) != 1 || allHeads[0].UniverseID != "feature/auth-jwt" {
		t.Fatalf("list universe heads mismatch: %+v", allHeads)
	}
}

func TestGraphEngineRecursiveCTEAndBlastRadius(t *testing.T) {
	tmpDir := createTempDir(t)
	ge, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("failed to create graph engine: %v", err)
	}
	defer ge.Close()

	// Build a cross-domain dependency graph:
	// App.tsx (TS) --[CONSUMES_API]--> Server.go (Go) --[CALLS]--> DB.go (Go) --[DEPLOYS_TO]--> Postgres (HCL)
	// Additional branch: App.tsx --[CONSUMES_API]--> Metrics.go
	nodes := []storage.NodeRecord{
		{NodeID: "ui.App", Language: core.LangTypeScript, NodeType: "ComponentDecl", Identifier: "App"},
		{NodeID: "api.Server", Language: core.LangGo, NodeType: "FunctionDecl", Identifier: "HandleAPI"},
		{NodeID: "db.PostgresStore", Language: core.LangGo, NodeType: "StructDecl", Identifier: "PostgresStore"},
		{NodeID: "infra.PostgresDB", Language: core.LangHCL, NodeType: "ResourceBlock", Identifier: "aws_db_instance"},
		{NodeID: "api.Metrics", Language: core.LangGo, NodeType: "FunctionDecl", Identifier: "HandleMetrics"},
	}
	for _, n := range nodes {
		_ = ge.PutNode(n)
	}

	edges := []storage.EdgeRecord{
		{SourceID: "ui.App", TargetID: "api.Server", EdgeType: core.EdgeConsumesAPI},
		{SourceID: "ui.App", TargetID: "api.Metrics", EdgeType: core.EdgeConsumesAPI},
		{SourceID: "api.Server", TargetID: "db.PostgresStore", EdgeType: core.EdgeCalls},
		{SourceID: "db.PostgresStore", TargetID: "infra.PostgresDB", EdgeType: core.EdgeDeploysTo},
	}
	for _, e := range edges {
		_ = ge.PutEdge(e)
	}

	// 1. Forward Traversal from ui.App (Full downstream dependency tree)
	fwdResult := ge.TraverseForward([]string{"ui.App"}, 0, nil)
	if len(fwdResult.VisitedNodes) != 5 {
		t.Fatalf("expected 5 reachable nodes from ui.App, got %d: %+v", len(fwdResult.VisitedNodes), fwdResult.VisitedNodes)
	}
	if fwdResult.Depths["infra.PostgresDB"] != 3 {
		t.Fatalf("expected infra.PostgresDB at depth 3, got %d", fwdResult.Depths["infra.PostgresDB"])
	}

	// Depth limited forward traversal (maxDepth = 1)
	depth1Result := ge.TraverseForward([]string{"ui.App"}, 1, nil)
	if len(depth1Result.VisitedNodes) != 3 { // ui.App, api.Server, api.Metrics
		t.Fatalf("expected 3 nodes at depth <= 1, got %d", len(depth1Result.VisitedNodes))
	}

	// Filtered forward traversal (only CONSUMES_API)
	filteredResult := ge.TraverseForward([]string{"ui.App"}, 0, []core.EdgeType{core.EdgeConsumesAPI})
	if len(filteredResult.VisitedNodes) != 3 {
		t.Fatalf("expected only CONSUMES_API targets (3 nodes), got %d", len(filteredResult.VisitedNodes))
	}

	// 2. Backward Traversal / Blast-Radius Analysis:
	// If infra.PostgresDB changes, who is impacted?
	blastResult := ge.TraverseBackward([]string{"infra.PostgresDB"}, 0, nil)
	if len(blastResult.VisitedNodes) != 4 { // PostgresDB, db.PostgresStore, api.Server, ui.App
		t.Fatalf("expected 4 impacted nodes in blast radius, got %d", len(blastResult.VisitedNodes))
	}
	if blastResult.Depths["ui.App"] != 3 {
		t.Fatalf("expected ui.App at distance 3 from PostgresDB, got %d", blastResult.Depths["ui.App"])
	}

	// 3. Find Shortest Path
	path, pathEdges, err := ge.FindPath("ui.App", "infra.PostgresDB", 5)
	if err != nil {
		t.Fatalf("failed to find path: %v", err)
	}
	if len(path) != 4 || path[0] != "ui.App" || path[3] != "infra.PostgresDB" {
		t.Fatalf("unexpected path: %v", path)
	}
	if len(pathEdges) != 3 {
		t.Fatalf("expected 3 edges in path, got %d", len(pathEdges))
	}

	// 4. Cycle Handling: Add cycle A -> B -> A
	_ = ge.PutEdge(storage.EdgeRecord{SourceID: "db.PostgresStore", TargetID: "ui.App", EdgeType: core.EdgeCalls})
	cycleResult := ge.TraverseForward([]string{"ui.App"}, 0, nil)
	if len(cycleResult.VisitedNodes) != 5 {
		t.Fatalf("cycle traversal should terminate safely with 5 nodes, got %d", len(cycleResult.VisitedNodes))
	}
}

func TestGraphEngineACIDTransactions(t *testing.T) {
	tmpDir := createTempDir(t)
	ge, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("failed to create graph engine: %v", err)
	}
	defer ge.Close()

	// 1. Rollback test
	if err := ge.BeginTx(); err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	_ = ge.PutNode(storage.NodeRecord{NodeID: "temp.Node1", Language: core.LangGo})
	_ = ge.PutNode(storage.NodeRecord{NodeID: "temp.Node2", Language: core.LangGo})
	if err := ge.Rollback(); err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	if ge.HasNode("temp.Node1") || ge.HasNode("temp.Node2") {
		t.Fatalf("rolled back nodes should not exist")
	}

	// 2. Commit test
	if err := ge.BeginTx(); err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	_ = ge.PutNode(storage.NodeRecord{NodeID: "perm.Node1", Language: core.LangGo})
	_ = ge.PutNode(storage.NodeRecord{NodeID: "perm.Node2", Language: core.LangGo})
	_ = ge.PutEdge(storage.EdgeRecord{SourceID: "perm.Node1", TargetID: "perm.Node2", EdgeType: core.EdgeCalls})
	if err := ge.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	if !ge.HasNode("perm.Node1") || !ge.HasNode("perm.Node2") {
		t.Fatalf("committed nodes must exist")
	}
	edges := ge.ListEdges("perm.Node1", "perm.Node2", core.EdgeCalls)
	if len(edges) != 1 {
		t.Fatalf("committed edge must exist")
	}
}

func TestGraphEngineCrashResilienceAndWALRecovery(t *testing.T) {
	tmpDir := createTempDir(t)

	// Step 1: Open DB, write committed transactions directly to WAL, DO NOT call Checkpoint/Close
	ge1, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("failed to open ge1: %v", err)
	}

	_ = ge1.PutNode(storage.NodeRecord{NodeID: "node.Alpha", Language: core.LangGo, Identifier: "Alpha"})
	_ = ge1.PutNode(storage.NodeRecord{NodeID: "node.Beta", Language: core.LangTypeScript, Identifier: "Beta"})
	_ = ge1.PutEdge(storage.EdgeRecord{SourceID: "node.Alpha", TargetID: "node.Beta", EdgeType: core.EdgeCalls})
	_ = ge1.PutUniverseHead(storage.UniverseHeadRecord{UniverseID: "dev-branch", HeadManifestHash: "manifest-xyz"})

	// Record an oplog event
	_, _ = ge1.Oplog().RecordNodeCreation("dev-branch", storage.NodeRecord{NodeID: "node.Alpha"})

	// Simulate sudden crash without calling ge1.Close() (WAL is flushed, DB snapshot is un-checkpointed)

	// Step 2: Open second engine instance from same directory -> tests crash recovery
	ge2, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("failed to open ge2 (recovery failed): %v", err)
	}
	defer ge2.Close()

	if !ge2.HasNode("node.Alpha") || !ge2.HasNode("node.Beta") {
		t.Fatalf("failed to recover nodes from WAL")
	}
	edges := ge2.ListEdges("node.Alpha", "node.Beta", core.EdgeCalls)
	if len(edges) != 1 {
		t.Fatalf("failed to recover edges from WAL")
	}
	head, err := ge2.GetUniverseHead("dev-branch")
	if err != nil || head.HeadManifestHash != "manifest-xyz" {
		t.Fatalf("failed to recover universe head from WAL")
	}

	events, err := ge2.Oplog().GetEvents("dev-branch", 1, 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("failed to recover oplog events from WAL: got %v", events)
	}

	// Step 3: Checkpoint and verify WAL truncation
	if err := ge2.Checkpoint(); err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}

	// Step 4: Reopen ge3 after checkpoint
	ge3, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("failed to open ge3: %v", err)
	}
	defer ge3.Close()

	if !ge3.HasNode("node.Alpha") {
		t.Fatalf("data missing after checkpoint recovery")
	}
}

func TestGraphEngineTornWriteRecovery(t *testing.T) {
	tmpDir := createTempDir(t)

	ge, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("failed to open ge: %v", err)
	}
	_ = ge.PutNode(storage.NodeRecord{NodeID: "valid.Node", Language: core.LangGo})
	_ = ge.Close()

	// Append corrupt torn-write garbage bytes to WAL file
	walPath := filepath.Join(tmpDir, ".cosm", "graph.db-wal")
	walFile, err := os.OpenFile(walPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open WAL: %v", err)
	}
	_, _ = walFile.Write([]byte{0x46, 0x47, 0x57, 0x41, 0x02, 0x00, 0x00, 0x10, 0xDE, 0xAD, 0xBE, 0xEF, 0x12, 0x34})
	_ = walFile.Close()

	// Reopen DB: must safely recover valid records and cleanly discard torn frame
	geRecovered, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("reopening DB with torn WAL should not fail: %v", err)
	}
	defer geRecovered.Close()

	if !geRecovered.HasNode("valid.Node") {
		t.Fatalf("valid node must survive torn WAL recovery")
	}
}

// -----------------------------------------------------------------------------
// OplogEngine Unit Tests
// -----------------------------------------------------------------------------

func TestOplogUndoAndRollback(t *testing.T) {
	tmpDir := createTempDir(t)
	ge, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("failed to create graph engine: %v", err)
	}
	defer ge.Close()

	oplog := ge.Oplog()

	// 1. Create Node with Oplog tracking
	node := storage.NodeRecord{NodeID: "service.UserService", Language: core.LangGo, Identifier: "UserService"}
	_ = ge.PutNode(node)
	ev1, err := oplog.RecordNodeCreation("main", node)
	if err != nil {
		t.Fatalf("failed to record node creation: %v", err)
	}

	// 2. Add Edge with Oplog tracking
	edge := storage.EdgeRecord{SourceID: "service.UserService", TargetID: "service.AuthService", EdgeType: core.EdgeCalls}
	_ = ge.PutEdge(edge)
	ev2, err := oplog.RecordEdgeCreation("main", edge)
	if err != nil {
		t.Fatalf("failed to record edge creation: %v", err)
	}

	// 3. Update Universe Head
	oldHead := storage.UniverseHeadRecord{UniverseID: "main", HeadManifestHash: "hash-v1"}
	newHead := storage.UniverseHeadRecord{UniverseID: "main", HeadManifestHash: "hash-v2"}
	_ = ge.PutUniverseHead(newHead)
	ev3, err := oplog.RecordUniverseHeadUpdate("main", oldHead, newHead)
	if err != nil {
		t.Fatalf("failed to record universe head update: %v", err)
	}

	// Verify events were recorded
	events, _ := oplog.GetEvents("main", 1, 10)
	if len(events) != 3 {
		t.Fatalf("expected 3 oplog events, got %d", len(events))
	}

	// Get by UUID
	byUUID, err := oplog.GetEventByUUID(ev1.EventUUID)
	if err != nil || byUUID.EntityID != "service.UserService" {
		t.Fatalf("get by UUID mismatch: %v", err)
	}

	// History check
	history, _ := oplog.GetHistory("service.UserService")
	if len(history) != 1 {
		t.Fatalf("history mismatch: %+v", history)
	}

	// 4. Undo step 3 (Universe Head update)
	undone, err := oplog.Undo(1)
	if err != nil || len(undone) != 1 || undone[0].EventID != ev3.EventID {
		t.Fatalf("undo universe head failed: %v", err)
	}
	curHead, _ := ge.GetUniverseHead("main")
	if curHead.HeadManifestHash != "hash-v1" {
		t.Fatalf("universe head was not restored to hash-v1, got %s", curHead.HeadManifestHash)
	}

	// 5. Undo step 2 (Edge addition)
	undone, err = oplog.Undo(1)
	if err != nil || len(undone) != 1 || undone[0].EventID != ev2.EventID {
		t.Fatalf("undo edge creation failed: %v", err)
	}
	edges := ge.ListEdges("service.UserService", "service.AuthService", core.EdgeCalls)
	if len(edges) != 0 {
		t.Fatalf("edge was not removed after undo")
	}

	// 6. Undo step 1 (Node creation)
	undone, err = oplog.Undo(1)
	if err != nil || len(undone) != 1 || undone[0].EventID != ev1.EventID {
		t.Fatalf("undo node creation failed: %v", err)
	}
	if ge.HasNode("service.UserService") {
		t.Fatalf("node was not removed after undo")
	}
}

func TestOplogForwardReplay(t *testing.T) {
	tmpDir1 := createTempDir(t)
	ge1, _ := storage.NewGraphEngine(tmpDir1)

	// Perform operations on ge1
	n1 := storage.NodeRecord{NodeID: "n1", Language: core.LangGo, Identifier: "Func1"}
	n2 := storage.NodeRecord{NodeID: "n2", Language: core.LangGo, Identifier: "Func2"}
	edge := storage.EdgeRecord{SourceID: "n1", TargetID: "n2", EdgeType: core.EdgeCalls}

	_ = ge1.PutNode(n1)
	_, _ = ge1.Oplog().RecordNodeCreation("main", n1)
	_ = ge1.PutNode(n2)
	_, _ = ge1.Oplog().RecordNodeCreation("main", n2)
	_ = ge1.PutEdge(edge)
	_, _ = ge1.Oplog().RecordEdgeCreation("main", edge)

	events, _ := ge1.Oplog().GetEvents("main", 1, 100)
	_ = ge1.Close()

	// Replay onto clean ge2
	tmpDir2 := createTempDir(t)
	ge2, _ := storage.NewGraphEngine(tmpDir2)
	defer ge2.Close()

	if err := ge2.Oplog().ReplayFromOplog(events); err != nil {
		t.Fatalf("failed to replay from oplog: %v", err)
	}

	if !ge2.HasNode("n1") || !ge2.HasNode("n2") {
		t.Fatalf("replayed nodes missing")
	}
	edges := ge2.ListEdges("n1", "n2", core.EdgeCalls)
	if len(edges) != 1 {
		t.Fatalf("replayed edges missing")
	}
}

// -----------------------------------------------------------------------------
// UniverseManager Unit Tests
// -----------------------------------------------------------------------------

func TestUniverseManagerBranchingAndCommits(t *testing.T) {
	tmpDir := createTempDir(t)
	bs, err := storage.NewBlobStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create blob store: %v", err)
	}
	ge, err := storage.NewGraphEngine(tmpDir)
	if err != nil {
		t.Fatalf("failed to create graph engine: %v", err)
	}
	defer ge.Close()

	um := storage.NewUniverseManager(ge, bs)

	// 1. Create main universe
	mainHead, err := um.CreateUniverse("main", "")
	if err != nil || mainHead.UniverseID != "main" {
		t.Fatalf("failed to create main universe: %v", err)
	}

	// 2. Commit Manifest 1 to main
	manifest1 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-test",
		UniverseID:  "main",
		Components:  []string{"comp-auth-v1", "comp-db-v1"},
		CrossEdges: []core.CrossBoundaryEdge{
			{SourceNodeID: "comp-auth-v1", TargetNodeID: "comp-db-v1", Type: core.EdgeCalls},
		},
		Lineage: core.LineageEnvelope{
			Intent: "Initial commit on main",
		},
	}

	hash1, err := um.CommitManifest("main", manifest1)
	if err != nil || hash1 == "" {
		t.Fatalf("failed to commit manifest1: %v", err)
	}

	// Verify head was updated
	head, _ := um.GetUniverse("main")
	if head.HeadManifestHash != hash1 {
		t.Fatalf("head manifest hash mismatch: got %s, expected %s", head.HeadManifestHash, hash1)
	}

	// Read back manifest
	readManifest, err := um.GetUniverseManifest("main")
	if err != nil || len(readManifest.Components) != 2 {
		t.Fatalf("failed to get universe manifest: %v", err)
	}

	// 3. Fork feature universe from main
	featHead, err := um.CreateUniverse("feature/auth-oauth", "main")
	if err != nil {
		t.Fatalf("failed to fork universe: %v", err)
	}
	if featHead.HeadManifestHash != hash1 {
		t.Fatalf("forked universe head must inherit parent head manifest: got %s, expected %s", featHead.HeadManifestHash, hash1)
	}

	// 4. Commit Manifest 2 to feature universe (add new component)
	manifest2 := &core.WorkspaceManifestNode{
		WorkspaceID: "ws-test",
		UniverseID:  "feature/auth-oauth",
		Components:  []string{"comp-auth-v1", "comp-db-v1", "comp-oauth-v1"},
		CrossEdges: []core.CrossBoundaryEdge{
			{SourceNodeID: "comp-auth-v1", TargetNodeID: "comp-db-v1", Type: core.EdgeCalls},
			{SourceNodeID: "comp-auth-v1", TargetNodeID: "comp-oauth-v1", Type: core.EdgeCalls},
		},
		Lineage: core.LineageEnvelope{
			Intent: "Add OAuth component",
		},
	}

	hash2, err := um.CommitManifest("feature/auth-oauth", manifest2)
	if err != nil || hash2 == hash1 {
		t.Fatalf("failed to commit manifest2 on feature branch: %v", err)
	}

	// 5. Diff universes (main vs feature/auth-oauth)
	diff, err := um.DiffUniverses("main", "feature/auth-oauth")
	if err != nil {
		t.Fatalf("failed to diff universes: %v", err)
	}
	if diff.Identical {
		t.Fatalf("universes should differ")
	}
	if len(diff.AddedComponents) != 1 || diff.AddedComponents[0] != "comp-oauth-v1" {
		t.Fatalf("diff added components mismatch: %+v", diff.AddedComponents)
	}
	if len(diff.AddedEdges) != 1 || diff.AddedEdges[0].TargetNodeID != "comp-oauth-v1" {
		t.Fatalf("diff added edges mismatch: %+v", diff.AddedEdges)
	}

	// 6. Merge feature into main (fast-forward or union)
	merged, err := um.MergeUniverse("feature/auth-oauth", "main", "union")
	if err != nil {
		t.Fatalf("failed to merge universe: %v", err)
	}
	if len(merged.Components) != 3 {
		t.Fatalf("merged manifest should contain 3 components, got %d", len(merged.Components))
	}

	// Assert main head was updated to merged manifest
	mainHeadAfterMerge, _ := um.GetUniverse("main")
	if mainHeadAfterMerge.HeadManifestHash == hash1 {
		t.Fatalf("main head was not updated after merge")
	}
}
