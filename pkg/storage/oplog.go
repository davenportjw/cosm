package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Action constants for Oplog events
const (
	ActionCreateNode      = "CREATE_NODE"
	ActionDeleteNode      = "DELETE_NODE"
	ActionUpdateNode      = "UPDATE_NODE"
	ActionAddEdge         = "ADD_EDGE"
	ActionRemoveEdge      = "REMOVE_EDGE"
	ActionCreateUniverse  = "CREATE_UNIVERSE"
	ActionSetUniverseHead = "SET_UNIVERSE_HEAD"
	ActionRecordLineage   = "RECORD_LINEAGE"
	ActionCommitManifest  = "COMMIT_MANIFEST"
)

// EntityType constants
const (
	EntityNode     = "NODE"
	EntityEdge     = "EDGE"
	EntityUniverse = "UNIVERSE"
	EntityLineage  = "LINEAGE"
	EntityManifest = "MANIFEST"
)

// OplogEvent represents an immutable, append-only event-sourced log record.
type OplogEvent struct {
	EventID     int64  `json:"event_id"`     // Monotonically increasing sequence number
	EventUUID   string `json:"event_uuid"`   // Globally unique event identifier
	TimestampMs int64  `json:"timestamp_ms"` // Epoch timestamp in milliseconds
	UniverseID  string `json:"universe_id"`  // Micro-universe context
	Action      string `json:"action"`       // Action performed
	EntityType  string `json:"entity_type"`  // Type of entity affected
	EntityID    string `json:"entity_id"`    // ID of entity affected
	Payload     string `json:"payload"`      // JSON payload of the applied state
	UndoPayload string `json:"undo_payload"` // JSON payload required to reverse this action
}

// OplogEngine manages the append-only event stream, transaction-safe recordings, and undo/redo replays.
type OplogEngine struct {
	graphEngine  *GraphEngine
	events       []*OplogEvent
	eventsByID   map[int64]*OplogEvent
	eventsByUUID map[string]*OplogEvent
	nextEventID  int64
	mu           sync.RWMutex
}

// newOplogEngine initializes an Oplog subsystem bound to the graph engine.
func newOplogEngine(ge *GraphEngine) *OplogEngine {
	return &OplogEngine{
		graphEngine:  ge,
		events:       make([]*OplogEvent, 0),
		eventsByID:   make(map[int64]*OplogEvent),
		eventsByUUID: make(map[string]*OplogEvent),
		nextEventID:  1,
	}
}

// generateUUID creates a cryptographic random hex UUID.
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// AppendEvent appends a new event to the oplog, assigning an EventID and persisting it to WAL.
func (o *OplogEngine) AppendEvent(ev *OplogEvent) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if ev.EventUUID == "" {
		ev.EventUUID = generateUUID()
	}
	if ev.TimestampMs == 0 {
		ev.TimestampMs = time.Now().UnixMilli()
	}

	ev.EventID = o.nextEventID
	o.nextEventID++

	evCopy := *ev

	// Persist to GraphEngine WAL and apply mutation
	return o.graphEngine.executeMutation("OPLOG_EVENT", evCopy)
}

// RecordNodeCreation records the creation of an AST node with an inverse delete undo payload.
func (o *OplogEngine) RecordNodeCreation(universeID string, node NodeRecord) (*OplogEvent, error) {
	payloadBytes, err := json.Marshal(node)
	if err != nil {
		return nil, err
	}
	undoBytes, err := json.Marshal(map[string]string{"node_id": node.NodeID})
	if err != nil {
		return nil, err
	}

	ev := &OplogEvent{
		UniverseID:  universeID,
		Action:      ActionCreateNode,
		EntityType:  EntityNode,
		EntityID:    node.NodeID,
		Payload:     string(payloadBytes),
		UndoPayload: string(undoBytes),
	}

	if err := o.AppendEvent(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// RecordNodeDeletion records node deletion with full node reconstruction undo payload.
func (o *OplogEngine) RecordNodeDeletion(universeID string, node NodeRecord) (*OplogEvent, error) {
	payloadBytes, err := json.Marshal(map[string]string{"node_id": node.NodeID})
	if err != nil {
		return nil, err
	}
	undoBytes, err := json.Marshal(node)
	if err != nil {
		return nil, err
	}

	ev := &OplogEvent{
		UniverseID:  universeID,
		Action:      ActionDeleteNode,
		EntityType:  EntityNode,
		EntityID:    node.NodeID,
		Payload:     string(payloadBytes),
		UndoPayload: string(undoBytes),
	}

	if err := o.AppendEvent(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// RecordEdgeCreation records the addition of an edge with inverse delete payload.
func (o *OplogEngine) RecordEdgeCreation(universeID string, edge EdgeRecord) (*OplogEvent, error) {
	payloadBytes, err := json.Marshal(edge)
	if err != nil {
		return nil, err
	}
	undoBytes, err := json.Marshal(edge)
	if err != nil {
		return nil, err
	}

	ev := &OplogEvent{
		UniverseID:  universeID,
		Action:      ActionAddEdge,
		EntityType:  EntityEdge,
		EntityID:    edge.EdgeKey(),
		Payload:     string(payloadBytes),
		UndoPayload: string(undoBytes),
	}

	if err := o.AppendEvent(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// RecordEdgeDeletion records the deletion of an edge with edge reconstruction undo payload.
func (o *OplogEngine) RecordEdgeDeletion(universeID string, edge EdgeRecord) (*OplogEvent, error) {
	payloadBytes, err := json.Marshal(edge)
	if err != nil {
		return nil, err
	}
	undoBytes, err := json.Marshal(edge)
	if err != nil {
		return nil, err
	}

	ev := &OplogEvent{
		UniverseID:  universeID,
		Action:      ActionRemoveEdge,
		EntityType:  EntityEdge,
		EntityID:    edge.EdgeKey(),
		Payload:     string(payloadBytes),
		UndoPayload: string(undoBytes),
	}

	if err := o.AppendEvent(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// RecordUniverseHeadUpdate records universe head updates with rollback to old head.
func (o *OplogEngine) RecordUniverseHeadUpdate(universeID string, oldHead, newHead UniverseHeadRecord) (*OplogEvent, error) {
	payloadBytes, err := json.Marshal(newHead)
	if err != nil {
		return nil, err
	}
	undoBytes, err := json.Marshal(oldHead)
	if err != nil {
		return nil, err
	}

	ev := &OplogEvent{
		UniverseID:  universeID,
		Action:      ActionSetUniverseHead,
		EntityType:  EntityUniverse,
		EntityID:    universeID,
		Payload:     string(payloadBytes),
		UndoPayload: string(undoBytes),
	}

	if err := o.AppendEvent(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// RecordLineage records a lineage envelope attached to an AST node.
func (o *OplogEngine) RecordLineage(universeID string, lin LineageRecord) (*OplogEvent, error) {
	payloadBytes, err := json.Marshal(lin)
	if err != nil {
		return nil, err
	}

	ev := &OplogEvent{
		UniverseID:  universeID,
		Action:      ActionRecordLineage,
		EntityType:  EntityLineage,
		EntityID:    lin.RecordID,
		Payload:     string(payloadBytes),
		UndoPayload: "",
	}

	if err := o.AppendEvent(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// GetEvents retrieves recorded events filtered by universe and starting event ID.
func (o *OplogEngine) GetEvents(universeID string, fromEventID int64, limit int) ([]*OplogEvent, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var result []*OplogEvent
	for _, ev := range o.events {
		if ev.EventID < fromEventID {
			continue
		}
		if universeID != "" && ev.UniverseID != universeID {
			continue
		}
		evCopy := *ev
		result = append(result, &evCopy)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

// GetEventByUUID retrieves an event by its unique UUID.
func (o *OplogEngine) GetEventByUUID(uuid string) (*OplogEvent, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	ev, exists := o.eventsByUUID[uuid]
	if !exists {
		return nil, fmt.Errorf("oplog event not found for uuid: %s", uuid)
	}
	evCopy := *ev
	return &evCopy, nil
}

// GetHistory retrieves all oplog events relating to a specific entity ID.
func (o *OplogEngine) GetHistory(entityID string) ([]*OplogEvent, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var result []*OplogEvent
	for _, ev := range o.events {
		if ev.EntityID == entityID {
			evCopy := *ev
			result = append(result, &evCopy)
		}
	}
	return result, nil
}

// Undo unrolls the last `count` events in reverse chronological order, executing inverse operations.
func (o *OplogEngine) Undo(count int) ([]*OplogEvent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if count <= 0 || len(o.events) == 0 {
		return nil, nil
	}

	if count > len(o.events) {
		count = len(o.events)
	}

	var undoneEvents []*OplogEvent

	for i := 0; i < count; i++ {
		lastIdx := len(o.events) - 1
		ev := o.events[lastIdx]

		if err := o.applyUndoOperation(ev); err != nil {
			return undoneEvents, fmt.Errorf("failed to undo event %d (%s): %w", ev.EventID, ev.Action, err)
		}

		delete(o.eventsByID, ev.EventID)
		delete(o.eventsByUUID, ev.EventUUID)
		o.events = o.events[:lastIdx]
		undoneEvents = append(undoneEvents, ev)
	}

	return undoneEvents, nil
}

// UndoToEventID rolls back all events back down to (and not including) targetEventID.
func (o *OplogEngine) UndoToEventID(targetEventID int64) ([]*OplogEvent, error) {
	o.mu.RLock()
	count := 0
	for i := len(o.events) - 1; i >= 0; i-- {
		if o.events[i].EventID > targetEventID {
			count++
		} else {
			break
		}
	}
	o.mu.RUnlock()

	return o.Undo(count)
}

// applyUndoOperation applies the exact inverse payload of an event.
func (o *OplogEngine) applyUndoOperation(ev *OplogEvent) error {
	switch ev.Action {
	case ActionCreateNode:
		// Inverse: delete node
		var m map[string]string
		if err := json.Unmarshal([]byte(ev.UndoPayload), &m); err != nil {
			return err
		}
		return o.graphEngine.DeleteNode(m["node_id"])

	case ActionDeleteNode:
		// Inverse: re-insert node
		var node NodeRecord
		if err := json.Unmarshal([]byte(ev.UndoPayload), &node); err != nil {
			return err
		}
		return o.graphEngine.PutNode(node)

	case ActionAddEdge:
		// Inverse: remove edge
		var edge EdgeRecord
		if err := json.Unmarshal([]byte(ev.UndoPayload), &edge); err != nil {
			return err
		}
		return o.graphEngine.DeleteEdge(edge.SourceID, edge.TargetID, edge.EdgeType)

	case ActionRemoveEdge:
		// Inverse: re-add edge
		var edge EdgeRecord
		if err := json.Unmarshal([]byte(ev.UndoPayload), &edge); err != nil {
			return err
		}
		return o.graphEngine.PutEdge(edge)

	case ActionSetUniverseHead:
		// Inverse: restore previous head
		var oldHead UniverseHeadRecord
		if err := json.Unmarshal([]byte(ev.UndoPayload), &oldHead); err != nil {
			return err
		}
		if oldHead.UniverseID != "" {
			return o.graphEngine.PutUniverseHead(oldHead)
		}
		return nil

	default:
		return nil
	}
}

// ReplayFromOplog deterministically reconstructs graph engine state by replaying an event sequence from scratch.
func (o *OplogEngine) ReplayFromOplog(events []*OplogEvent) error {
	for _, ev := range events {
		switch ev.Action {
		case ActionCreateNode, ActionUpdateNode:
			var node NodeRecord
			if err := json.Unmarshal([]byte(ev.Payload), &node); err != nil {
				return err
			}
			if err := o.graphEngine.PutNode(node); err != nil {
				return err
			}

		case ActionDeleteNode:
			var m map[string]string
			if err := json.Unmarshal([]byte(ev.Payload), &m); err != nil {
				return err
			}
			if err := o.graphEngine.DeleteNode(m["node_id"]); err != nil {
				return err
			}

		case ActionAddEdge:
			var edge EdgeRecord
			if err := json.Unmarshal([]byte(ev.Payload), &edge); err != nil {
				return err
			}
			if err := o.graphEngine.PutEdge(edge); err != nil {
				return err
			}

		case ActionRemoveEdge:
			var edge EdgeRecord
			if err := json.Unmarshal([]byte(ev.Payload), &edge); err != nil {
				return err
			}
			if err := o.graphEngine.DeleteEdge(edge.SourceID, edge.TargetID, edge.EdgeType); err != nil {
				return err
			}

		case ActionSetUniverseHead, ActionCreateUniverse:
			var head UniverseHeadRecord
			if err := json.Unmarshal([]byte(ev.Payload), &head); err != nil {
				return err
			}
			if err := o.graphEngine.PutUniverseHead(head); err != nil {
				return err
			}

		case ActionRecordLineage:
			var lin LineageRecord
			if err := json.Unmarshal([]byte(ev.Payload), &lin); err != nil {
				return err
			}
			if err := o.graphEngine.PutLineage(lin); err != nil {
				return err
			}
		}
	}
	return nil
}
