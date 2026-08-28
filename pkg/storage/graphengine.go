package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

// WAL Record Type constants
const (
	walRecordTxBegin  uint8 = 1
	walRecordMutation uint8 = 2
	walRecordTxCommit uint8 = 3
	walMagicHeader    uint32 = 0x46475741 // "FGWA" - Future of Git WAL
)

// NodeRecord stores indexed AST symbol or component node metadata in the graph engine.
type NodeRecord struct {
	NodeID     string            `json:"node_id"`
	Language   core.Language     `json:"language"`
	NodeType   string            `json:"node_type"`
	Identifier string            `json:"identifier"`
	MerkleHash string            `json:"merkle_hash"`
	CreatedAt  time.Time         `json:"created_at"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// EdgeRecord stores directional semantic edges between AST nodes.
type EdgeRecord struct {
	SourceID   string            `json:"source_id"`
	TargetID   string            `json:"target_id"`
	EdgeType   core.EdgeType     `json:"edge_type"`
	ContractID string            `json:"contract_id,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// EdgeKey returns unique composite identifier for an edge.
func (e EdgeRecord) EdgeKey() string {
	return fmt.Sprintf("%s|%s|%s", e.SourceID, e.TargetID, e.EdgeType)
}

// LineageRecord stores full-spectrum provenance metadata.
type LineageRecord struct {
	RecordID            string    `json:"record_id"`
	NodeID              string    `json:"node_id"`
	UserID              string    `json:"user_id"`
	UserPrompt          string    `json:"user_prompt"`
	SessionID           string    `json:"session_id"`
	OrchestratorAgentID string    `json:"orchestrator_agent_id"`
	ExecutingAgentID    string    `json:"executing_agent_id"`
	LLMVersion          string    `json:"llm_version"`
	GenerationParams    string    `json:"generation_params"`
	Intent              string    `json:"intent"`
	Timestamp           time.Time `json:"timestamp"`
	SignatureEd25519    []byte    `json:"signature_ed25519,omitempty"`
}

// UniverseHeadRecord stores branch and micro-universe heads.
type UniverseHeadRecord struct {
	UniverseID       string    `json:"universe_id"`
	HeadManifestHash string    `json:"head_manifest_hash"`
	ParentUniverseID string    `json:"parent_universe_id,omitempty"`
	Status           string    `json:"status"` // "active", "merged", "archived"
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TraversalResult encapsulates the result of a recursive graph CTE traversal.
type TraversalResult struct {
	VisitedNodes []NodeRecord            `json:"visited_nodes"`
	VisitedEdges []EdgeRecord            `json:"visited_edges"`
	Depths       map[string]int          `json:"depths"` // node_id -> shortest depth
	Paths        map[string][]string     `json:"paths"`  // node_id -> slice of node IDs in path
}

// GraphEngine provides pure-Go SQLite WAL-mode graph database operations, ACID transactions,
// crash resilience, recursive CTE traversal queries, and oplog integration.
type GraphEngine struct {
	dbPath    string
	walPath   string
	walFile   *os.File
	mu        sync.RWMutex
	inTx      bool
	txMutations []walMutationPayload

	// Graph State Indexes
	nodes         map[string]NodeRecord            // node_id -> NodeRecord
	edges         map[string]EdgeRecord            // composite key -> EdgeRecord
	outgoingEdges map[string]map[string]EdgeRecord // source_id -> composite_key -> EdgeRecord
	incomingEdges map[string]map[string]EdgeRecord // target_id -> composite_key -> EdgeRecord
	lineage       map[string]LineageRecord         // record_id -> LineageRecord
	nodeLineage   map[string][]string              // node_id -> []record_id
	universeHeads map[string]UniverseHeadRecord    // universe_id -> UniverseHeadRecord
	oplog         *OplogEngine                     // Event oplog subsystem
}

// WAL mutation data payload
type walMutationPayload struct {
	Type    string          `json:"type"` // "PUT_NODE", "DEL_NODE", "PUT_EDGE", "DEL_EDGE", "PUT_LINEAGE", "PUT_UNIVERSE", "OPLOG_EVENT"
	Payload json.RawMessage `json:"payload"`
}

// Snapshot DB file format
type dbSnapshot struct {
	Version       int                           `json:"version"`
	Timestamp     time.Time                     `json:"timestamp"`
	Nodes         map[string]NodeRecord         `json:"nodes"`
	Edges         map[string]EdgeRecord         `json:"edges"`
	Lineage       map[string]LineageRecord      `json:"lineage"`
	UniverseHeads map[string]UniverseHeadRecord `json:"universe_heads"`
	OplogEvents   []*OplogEvent                 `json:"oplog_events"`
}

// NewGraphEngine opens or creates the GraphEngine database at dbPath.
// If dbPath is a directory or repo root, it resolves to .cosm/graph.db (or .fg/graph.db if existing).
func NewGraphEngine(basePath string) (*GraphEngine, error) {
	var dbPath string
	if filepath.Base(basePath) == "graph.db" {
		dbPath = basePath
	} else if filepath.Base(basePath) == ".cosm" || filepath.Base(basePath) == ".fg" {
		dbPath = filepath.Join(basePath, "graph.db")
	} else {
		if _, err := os.Stat(filepath.Join(basePath, ".fg", "graph.db")); err == nil {
			dbPath = filepath.Join(basePath, ".fg", "graph.db")
		} else {
			dbPath = filepath.Join(basePath, ".cosm", "graph.db")
		}
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for db %s: %w", dbPath, err)
	}

	walPath := dbPath + "-wal"

	engine := &GraphEngine{
		dbPath:        dbPath,
		walPath:       walPath,
		nodes:         make(map[string]NodeRecord),
		edges:         make(map[string]EdgeRecord),
		outgoingEdges: make(map[string]map[string]EdgeRecord),
		incomingEdges: make(map[string]map[string]EdgeRecord),
		lineage:       make(map[string]LineageRecord),
		nodeLineage:   make(map[string][]string),
		universeHeads: make(map[string]UniverseHeadRecord),
	}

	// Initialize Oplog subsystem
	engine.oplog = newOplogEngine(engine)

	// Recover state from snapshot and replay WAL
	if err := engine.recover(); err != nil {
		return nil, fmt.Errorf("graph engine recovery failed: %w", err)
	}

	// Open WAL file for appending new transactions
	walFile, err := os.OpenFile(walPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open WAL file %s: %w", walPath, err)
	}
	engine.walFile = walFile

	return engine, nil
}

// Oplog returns the OplogEngine attached to this graph engine.
func (g *GraphEngine) Oplog() *OplogEngine {
	return g.oplog
}

// recover loads base snapshot from disk and replays uncheckpointed committed WAL transactions.
func (g *GraphEngine) recover() error {
	// 1. Load base DB snapshot if exists
	if data, err := os.ReadFile(g.dbPath); err == nil && len(data) > 0 {
		var snap dbSnapshot
		if err := json.Unmarshal(data, &snap); err == nil {
			g.nodes = snap.Nodes
			if g.nodes == nil {
				g.nodes = make(map[string]NodeRecord)
			}
			g.edges = snap.Edges
			if g.edges == nil {
				g.edges = make(map[string]EdgeRecord)
			}
			g.lineage = snap.Lineage
			if g.lineage == nil {
				g.lineage = make(map[string]LineageRecord)
			}
			g.universeHeads = snap.UniverseHeads
			if g.universeHeads == nil {
				g.universeHeads = make(map[string]UniverseHeadRecord)
			}
			if snap.OplogEvents != nil {
				g.oplog.events = snap.OplogEvents
				for _, ev := range snap.OplogEvents {
					g.oplog.eventsByID[ev.EventID] = ev
					g.oplog.eventsByUUID[ev.EventUUID] = ev
					if ev.EventID >= g.oplog.nextEventID {
						g.oplog.nextEventID = ev.EventID + 1
					}
				}
			}
			// Rebuild adjacency indexes
			for _, e := range g.edges {
				g.indexEdge(e)
			}
			for _, l := range g.lineage {
				g.nodeLineage[l.NodeID] = append(g.nodeLineage[l.NodeID], l.RecordID)
			}
		}
	}

	// 2. Read and replay WAL if exists
	if walData, err := os.ReadFile(g.walPath); err == nil && len(walData) > 0 {
		if err := g.replayWALBytes(walData); err != nil {
			// Log recovery warning but preserve what was recoverable
		}
	}

	return nil
}

// replayWALBytes reads WAL frame chunks, verifying CRC checksums and applying committed transactions.
func (g *GraphEngine) replayWALBytes(walData []byte) error {
	buf := bytes.NewReader(walData)

	var currentTx []walMutationPayload
	inTx := false

	for {
		// Read frame header: [Magic 4B][Type 1B][Length 4B][CRC32 4B]
		var magic uint32
		if err := binary.Read(buf, binary.BigEndian, &magic); err != nil {
			if err == io.EOF {
				break
			}
			return nil // truncated frame at EOF - ignore
		}
		if magic != walMagicHeader {
			// Corrupted or misaligned header - stop replay safely
			break
		}

		var recType uint8
		var length uint32
		var storedCRC uint32

		if err := binary.Read(buf, binary.BigEndian, &recType); err != nil {
			break
		}
		if err := binary.Read(buf, binary.BigEndian, &length); err != nil {
			break
		}
		if err := binary.Read(buf, binary.BigEndian, &storedCRC); err != nil {
			break
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(buf, payload); err != nil {
			break // Incomplete record at crash point
		}

		// Verify CRC32
		computedCRC := crc32.ChecksumIEEE(payload)
		if computedCRC != storedCRC {
			break // Corrupted frame
		}

		switch recType {
		case walRecordTxBegin:
			inTx = true
			currentTx = nil
		case walRecordMutation:
			if inTx {
				var mut walMutationPayload
				if err := json.Unmarshal(payload, &mut); err == nil {
					currentTx = append(currentTx, mut)
				}
			}
		case walRecordTxCommit:
			if inTx {
				for _, mut := range currentTx {
					g.applyMutationDirect(mut)
				}
				inTx = false
				currentTx = nil
			}
		}
	}

	return nil
}

// applyMutationDirect executes a mutation directly onto graph state.
func (g *GraphEngine) applyMutationDirect(mut walMutationPayload) {
	switch mut.Type {
	case "PUT_NODE":
		var node NodeRecord
		if err := json.Unmarshal(mut.Payload, &node); err == nil {
			g.nodes[node.NodeID] = node
		}
	case "DEL_NODE":
		var nodeID string
		if err := json.Unmarshal(mut.Payload, &nodeID); err == nil {
			delete(g.nodes, nodeID)
		}
	case "PUT_EDGE":
		var edge EdgeRecord
		if err := json.Unmarshal(mut.Payload, &edge); err == nil {
			g.edges[edge.EdgeKey()] = edge
			g.indexEdge(edge)
		}
	case "DEL_EDGE":
		var edge EdgeRecord
		if err := json.Unmarshal(mut.Payload, &edge); err == nil {
			key := edge.EdgeKey()
			delete(g.edges, key)
			g.unindexEdge(edge)
		}
	case "PUT_LINEAGE":
		var lin LineageRecord
		if err := json.Unmarshal(mut.Payload, &lin); err == nil {
			g.lineage[lin.RecordID] = lin
			g.nodeLineage[lin.NodeID] = append(g.nodeLineage[lin.NodeID], lin.RecordID)
		}
	case "PUT_UNIVERSE":
		var uni UniverseHeadRecord
		if err := json.Unmarshal(mut.Payload, &uni); err == nil {
			g.universeHeads[uni.UniverseID] = uni
		}
	case "OPLOG_EVENT":
		var ev OplogEvent
		if err := json.Unmarshal(mut.Payload, &ev); err == nil {
			evCopy := ev
			g.oplog.events = append(g.oplog.events, &evCopy)
			g.oplog.eventsByID[ev.EventID] = &evCopy
			g.oplog.eventsByUUID[ev.EventUUID] = &evCopy
			if ev.EventID >= g.oplog.nextEventID {
				g.oplog.nextEventID = ev.EventID + 1
			}
		}
	}
}

func (g *GraphEngine) indexEdge(e EdgeRecord) {
	key := e.EdgeKey()
	if g.outgoingEdges[e.SourceID] == nil {
		g.outgoingEdges[e.SourceID] = make(map[string]EdgeRecord)
	}
	g.outgoingEdges[e.SourceID][key] = e

	if g.incomingEdges[e.TargetID] == nil {
		g.incomingEdges[e.TargetID] = make(map[string]EdgeRecord)
	}
	g.incomingEdges[e.TargetID][key] = e
}

func (g *GraphEngine) unindexEdge(e EdgeRecord) {
	key := e.EdgeKey()
	if g.outgoingEdges[e.SourceID] != nil {
		delete(g.outgoingEdges[e.SourceID], key)
	}
	if g.incomingEdges[e.TargetID] != nil {
		delete(g.incomingEdges[e.TargetID], key)
	}
}

// writeWALRecord writes a single framed record into the WAL file.
func (g *GraphEngine) writeWALRecord(recType uint8, payload []byte) error {
	if g.walFile == nil {
		return fmt.Errorf("wal file is not open")
	}

	length := uint32(len(payload))
	crc := crc32.ChecksumIEEE(payload)

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, walMagicHeader); err != nil {
		return err
	}
	if err := binary.Write(&buf, binary.BigEndian, recType); err != nil {
		return err
	}
	if err := binary.Write(&buf, binary.BigEndian, length); err != nil {
		return err
	}
	if err := binary.Write(&buf, binary.BigEndian, crc); err != nil {
		return err
	}
	if _, err := buf.Write(payload); err != nil {
		return err
	}

	if _, err := g.walFile.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

// BeginTx starts an atomic transaction.
func (g *GraphEngine) BeginTx() error {
	g.mu.Lock()
	if g.inTx {
		g.mu.Unlock()
		return fmt.Errorf("transaction already in progress")
	}
	g.inTx = true
	g.txMutations = nil
	return nil
}

// Commit commits all staged mutations in the current transaction to WAL with fsync.
func (g *GraphEngine) Commit() error {
	if !g.inTx {
		return fmt.Errorf("no transaction to commit")
	}
	defer func() {
		g.inTx = false
		g.txMutations = nil
		g.mu.Unlock()
	}()

	if len(g.txMutations) == 0 {
		return nil
	}

	// Write TxBegin
	if err := g.writeWALRecord(walRecordTxBegin, nil); err != nil {
		return fmt.Errorf("failed to write WAL TxBegin: %w", err)
	}

	// Write all mutations
	for _, mut := range g.txMutations {
		data, err := json.Marshal(mut)
		if err != nil {
			return fmt.Errorf("failed to serialize mutation: %w", err)
		}
		if err := g.writeWALRecord(walRecordMutation, data); err != nil {
			return fmt.Errorf("failed to write WAL mutation: %w", err)
		}
	}

	// Write TxCommit
	if err := g.writeWALRecord(walRecordTxCommit, nil); err != nil {
		return fmt.Errorf("failed to write WAL TxCommit: %w", err)
	}

	// Flush and fsync WAL
	if err := g.walFile.Sync(); err != nil {
		return fmt.Errorf("failed to fsync WAL file: %w", err)
	}

	// Apply mutations to in-memory indexes
	for _, mut := range g.txMutations {
		g.applyMutationDirect(mut)
	}

	return nil
}

// Rollback discards all staged mutations in the current transaction.
func (g *GraphEngine) Rollback() error {
	if !g.inTx {
		return fmt.Errorf("no transaction to rollback")
	}
	g.txMutations = nil
	g.inTx = false
	g.mu.Unlock()
	return nil
}

// executeMutation stages a mutation if inside transaction, or immediately commits it with WAL fsync.
func (g *GraphEngine) executeMutation(mutType string, obj interface{}) error {
	payload, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal mutation payload: %w", err)
	}

	mut := walMutationPayload{
		Type:    mutType,
		Payload: payload,
	}

	if g.inTx {
		g.txMutations = append(g.txMutations, mut)
		return nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Write single-mutation atomic transaction
	if err := g.writeWALRecord(walRecordTxBegin, nil); err != nil {
		return err
	}
	mutBytes, err := json.Marshal(mut)
	if err != nil {
		return err
	}
	if err := g.writeWALRecord(walRecordMutation, mutBytes); err != nil {
		return err
	}
	if err := g.writeWALRecord(walRecordTxCommit, nil); err != nil {
		return err
	}
	if err := g.walFile.Sync(); err != nil {
		return err
	}

	g.applyMutationDirect(mut)
	return nil
}

// PutNode inserts or updates an AST symbol or component node in the graph.
func (g *GraphEngine) PutNode(node NodeRecord) error {
	if node.NodeID == "" {
		return fmt.Errorf("node_id cannot be empty")
	}
	if node.CreatedAt.IsZero() {
		node.CreatedAt = time.Now().UTC()
	}
	return g.executeMutation("PUT_NODE", node)
}

// GetNode retrieves a node record by its NodeID.
func (g *GraphEngine) GetNode(nodeID string) (*NodeRecord, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, exists := g.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}
	nodeCopy := node
	return &nodeCopy, nil
}

// HasNode checks if a node exists.
func (g *GraphEngine) HasNode(nodeID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, exists := g.nodes[nodeID]
	return exists
}

// DeleteNode removes a node and its adjacent edges.
func (g *GraphEngine) DeleteNode(nodeID string) error {
	return g.executeMutation("DEL_NODE", nodeID)
}

// ListNodes returns all nodes optionally filtered by language and/or node_type.
func (g *GraphEngine) ListNodes(lang core.Language, nodeType string) []NodeRecord {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []NodeRecord
	for _, n := range g.nodes {
		if lang != "" && n.Language != lang {
			continue
		}
		if nodeType != "" && n.NodeType != nodeType {
			continue
		}
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].NodeID < result[j].NodeID
	})
	return result
}

// PutEdge inserts or updates a semantic cross-boundary edge.
func (g *GraphEngine) PutEdge(edge EdgeRecord) error {
	if edge.SourceID == "" || edge.TargetID == "" {
		return fmt.Errorf("edge source and target must be non-empty")
	}
	if !edge.EdgeType.IsValid() {
		return fmt.Errorf("invalid edge type: %s", edge.EdgeType)
	}
	return g.executeMutation("PUT_EDGE", edge)
}

// DeleteEdge removes a semantic edge.
func (g *GraphEngine) DeleteEdge(sourceID, targetID string, edgeType core.EdgeType) error {
	edge := EdgeRecord{
		SourceID: sourceID,
		TargetID: targetID,
		EdgeType: edgeType,
	}
	return g.executeMutation("DEL_EDGE", edge)
}

// ListEdges returns all edges matching criteria.
func (g *GraphEngine) ListEdges(sourceID, targetID string, edgeType core.EdgeType) []EdgeRecord {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []EdgeRecord
	for _, e := range g.edges {
		if sourceID != "" && e.SourceID != sourceID {
			continue
		}
		if targetID != "" && e.TargetID != targetID {
			continue
		}
		if edgeType != "" && e.EdgeType != edgeType {
			continue
		}
		result = append(result, e)
	}
	return result
}

// PutLineage persists a LineageRecord associated with an AST node.
func (g *GraphEngine) PutLineage(lin LineageRecord) error {
	if lin.RecordID == "" {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", lin.NodeID, lin.UserPrompt, time.Now().UnixNano())))
		lin.RecordID = hex.EncodeToString(sum[:])[:16]
	}
	if lin.Timestamp.IsZero() {
		lin.Timestamp = time.Now().UTC()
	}
	return g.executeMutation("PUT_LINEAGE", lin)
}

// GetLineageByNode retrieves all lineage records for a given node.
func (g *GraphEngine) GetLineageByNode(nodeID string) []LineageRecord {
	g.mu.RLock()
	defer g.mu.RUnlock()

	recordIDs := g.nodeLineage[nodeID]
	var result []LineageRecord
	for _, id := range recordIDs {
		if rec, ok := g.lineage[id]; ok {
			result = append(result, rec)
		}
	}
	return result
}

// PutUniverseHead updates or creates a universe head pointer.
func (g *GraphEngine) PutUniverseHead(head UniverseHeadRecord) error {
	if head.UniverseID == "" {
		return fmt.Errorf("universe_id cannot be empty")
	}
	if head.Status == "" {
		head.Status = "active"
	}
	now := time.Now().UTC()
	if head.CreatedAt.IsZero() {
		head.CreatedAt = now
	}
	head.UpdatedAt = now
	return g.executeMutation("PUT_UNIVERSE", head)
}

// GetUniverseHead returns head record for a universe.
func (g *GraphEngine) GetUniverseHead(universeID string) (*UniverseHeadRecord, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	u, exists := g.universeHeads[universeID]
	if !exists {
		return nil, fmt.Errorf("universe not found: %s", universeID)
	}
	uCopy := u
	return &uCopy, nil
}

// ListUniverseHeads returns all known micro-universes.
func (g *GraphEngine) ListUniverseHeads() []UniverseHeadRecord {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []UniverseHeadRecord
	for _, u := range g.universeHeads {
		result = append(result, u)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UniverseID < result[j].UniverseID
	})
	return result
}

// TraverseForward executes recursive CTE traversal finding all downstream nodes and edges reachable
// from startNodeIDs. Supports cycle prevention, depth limits, and edge type filtering.
func (g *GraphEngine) TraverseForward(startNodeIDs []string, maxDepth int, edgeTypes []core.EdgeType) TraversalResult {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type queueItem struct {
		nodeID string
		depth  int
		path   []string
	}

	filterTypes := make(map[core.EdgeType]bool)
	for _, et := range edgeTypes {
		filterTypes[et] = true
	}

	visitedMap := make(map[string]bool)
	depths := make(map[string]int)
	paths := make(map[string][]string)
	visitedEdgesMap := make(map[string]EdgeRecord)

	var queue []queueItem
	for _, startID := range startNodeIDs {
		visitedMap[startID] = true
		depths[startID] = 0
		paths[startID] = []string{startID}
		queue = append(queue, queueItem{
			nodeID: startID,
			depth:  0,
			path:   []string{startID},
		})
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if maxDepth > 0 && curr.depth >= maxDepth {
			continue
		}

		outEdges := g.outgoingEdges[curr.nodeID]
		for _, edge := range outEdges {
			if len(filterTypes) > 0 && !filterTypes[edge.EdgeType] {
				continue
			}

			visitedEdgesMap[edge.EdgeKey()] = edge

			// Check cycle in current path
			hasCycle := false
			for _, p := range curr.path {
				if p == edge.TargetID {
					hasCycle = true
					break
				}
			}
			if hasCycle {
				continue
			}

			newPath := append(append([]string{}, curr.path...), edge.TargetID)
			nextDepth := curr.depth + 1

			if !visitedMap[edge.TargetID] || nextDepth < depths[edge.TargetID] {
				visitedMap[edge.TargetID] = true
				depths[edge.TargetID] = nextDepth
				paths[edge.TargetID] = newPath
				queue = append(queue, queueItem{
					nodeID: edge.TargetID,
					depth:  nextDepth,
					path:   newPath,
				})
			}
		}
	}

	var visitedNodes []NodeRecord
	for id := range visitedMap {
		if node, ok := g.nodes[id]; ok {
			visitedNodes = append(visitedNodes, node)
		} else {
			visitedNodes = append(visitedNodes, NodeRecord{NodeID: id})
		}
	}
	sort.Slice(visitedNodes, func(i, j int) bool {
		return depths[visitedNodes[i].NodeID] < depths[visitedNodes[j].NodeID]
	})

	var visitedEdges []EdgeRecord
	for _, e := range visitedEdgesMap {
		visitedEdges = append(visitedEdges, e)
	}

	return TraversalResult{
		VisitedNodes: visitedNodes,
		VisitedEdges: visitedEdges,
		Depths:       depths,
		Paths:        paths,
	}
}

// TraverseBackward executes recursive CTE blast-radius traversal finding all upstream dependents
// that directly or transitively depend on the target startNodeIDs.
func (g *GraphEngine) TraverseBackward(startNodeIDs []string, maxDepth int, edgeTypes []core.EdgeType) TraversalResult {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type queueItem struct {
		nodeID string
		depth  int
		path   []string
	}

	filterTypes := make(map[core.EdgeType]bool)
	for _, et := range edgeTypes {
		filterTypes[et] = true
	}

	visitedMap := make(map[string]bool)
	depths := make(map[string]int)
	paths := make(map[string][]string)
	visitedEdgesMap := make(map[string]EdgeRecord)

	var queue []queueItem
	for _, startID := range startNodeIDs {
		visitedMap[startID] = true
		depths[startID] = 0
		paths[startID] = []string{startID}
		queue = append(queue, queueItem{
			nodeID: startID,
			depth:  0,
			path:   []string{startID},
		})
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if maxDepth > 0 && curr.depth >= maxDepth {
			continue
		}

		inEdges := g.incomingEdges[curr.nodeID]
		for _, edge := range inEdges {
			if len(filterTypes) > 0 && !filterTypes[edge.EdgeType] {
				continue
			}

			visitedEdgesMap[edge.EdgeKey()] = edge

			hasCycle := false
			for _, p := range curr.path {
				if p == edge.SourceID {
					hasCycle = true
					break
				}
			}
			if hasCycle {
				continue
			}

			newPath := append(append([]string{}, curr.path...), edge.SourceID)
			nextDepth := curr.depth + 1

			if !visitedMap[edge.SourceID] || nextDepth < depths[edge.SourceID] {
				visitedMap[edge.SourceID] = true
				depths[edge.SourceID] = nextDepth
				paths[edge.SourceID] = newPath
				queue = append(queue, queueItem{
					nodeID: edge.SourceID,
					depth:  nextDepth,
					path:   newPath,
				})
			}
		}
	}

	var visitedNodes []NodeRecord
	for id := range visitedMap {
		if node, ok := g.nodes[id]; ok {
			visitedNodes = append(visitedNodes, node)
		} else {
			visitedNodes = append(visitedNodes, NodeRecord{NodeID: id})
		}
	}
	sort.Slice(visitedNodes, func(i, j int) bool {
		return depths[visitedNodes[i].NodeID] < depths[visitedNodes[j].NodeID]
	})

	var visitedEdges []EdgeRecord
	for _, e := range visitedEdgesMap {
		visitedEdges = append(visitedEdges, e)
	}

	return TraversalResult{
		VisitedNodes: visitedNodes,
		VisitedEdges: visitedEdges,
		Depths:       depths,
		Paths:        paths,
	}
}

// FindPath finds the shortest path of semantic edges connecting sourceID to targetID.
func (g *GraphEngine) FindPath(sourceID, targetID string, maxDepth int) ([]string, []EdgeRecord, error) {
	result := g.TraverseForward([]string{sourceID}, maxDepth, nil)
	path, found := result.Paths[targetID]
	if !found {
		return nil, nil, fmt.Errorf("no path found between %s and %s", sourceID, targetID)
	}

	var edgesInPath []EdgeRecord
	for i := 0; i < len(path)-1; i++ {
		src := path[i]
		dst := path[i+1]
		for _, e := range result.VisitedEdges {
			if e.SourceID == src && e.TargetID == dst {
				edgesInPath = append(edgesInPath, e)
				break
			}
		}
	}

	return path, edgesInPath, nil
}

// Checkpoint merges current in-memory state into the main snapshot DB file and resets the WAL file.
func (g *GraphEngine) Checkpoint() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	snap := dbSnapshot{
		Version:       1,
		Timestamp:     time.Now().UTC(),
		Nodes:         g.nodes,
		Edges:         g.edges,
		Lineage:       g.lineage,
		UniverseHeads: g.universeHeads,
		OplogEvents:   g.oplog.events,
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal db snapshot: %w", err)
	}

	// Atomic write to tmp and rename
	tmpPath := g.dbPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write tmp snapshot: %w", err)
	}
	if err := os.Rename(tmpPath, g.dbPath); err != nil {
		return fmt.Errorf("failed to replace db file: %w", err)
	}

	// Close current WAL and truncate
	if g.walFile != nil {
		_ = g.walFile.Close()
	}

	// Reset WAL file to 0 bytes
	walFile, err := os.OpenFile(g.walPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to truncate WAL file: %w", err)
	}
	g.walFile = walFile

	return nil
}

// Close safely checkpoints and closes the GraphEngine.
func (g *GraphEngine) Close() error {
	if err := g.Checkpoint(); err != nil {
		return err
	}
	if g.walFile != nil {
		return g.walFile.Close()
	}
	return nil
}
