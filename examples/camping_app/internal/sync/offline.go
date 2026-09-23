package sync

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"strings"
	"sync"
	"time"
)

// WALRecord represents an immutable, content-addressed append-only log entry
// with an IEEE CRC32 checksum for offline data resilience.
type WALRecord struct {
	Sequence uint64 `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	EntityType string `json:"entity_type"`
	EntityID string `json:"entity_id"`
	Operation string `json:"operation"`
	DataJSON string `json:"data_json"`
	CRC32 uint32 `json:"crc32"`
}

// GateAuthorizationPayload encapsulates the details stored inside a gate authorization WAL entry.
type GateAuthorizationPayload struct {
	Plate string `json:"plate"`
	State string `json:"state"`
	Granted bool `json:"granted"`
	Reason string `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// OfflineGateCache provides an offline-first permit cache and write-ahead log (WAL) buffer
// for disconnected gatehouse check-in and entry operations.
type OfflineGateCache struct {
	mu sync.RWMutex
	permits map[string]string
	permitTimes map[string]time.Time
	offlineUsage map[string]int
	wal []WALRecord
	sequence uint64
}

// ComputeWALRecordCRC32 calculates the IEEE 802.3 CRC32 checksum over the canonical record fields.
func ComputeWALRecordCRC32(rec WALRecord) uint32 {
	payload := fmt.Sprintf("%d|%d|%s|%s|%s|%s",
		rec.Sequence,
		rec.Timestamp.UnixNano(),
		rec.EntityType,
		rec.EntityID,
		rec.Operation,
		rec.DataJSON,
	)
	return crc32.ChecksumIEEE([]byte(payload))
}

// NewOfflineGateCache creates an initialized thread-safe OfflineGateCache.
func NewOfflineGateCache() *OfflineGateCache {
	return &OfflineGateCache{
		permits:      make(map[string]string),
		permitTimes:  make(map[string]time.Time),
		offlineUsage: make(map[string]int),
		wal:          make([]WALRecord, 0),
		sequence:     0,
	}
}

func normalizeEntityKey(plate, state string) string {
	p := strings.ToUpper(strings.TrimSpace(plate))
	s := strings.ToUpper(strings.TrimSpace(state))
	if s == "" {
		return p
	}
	return fmt.Sprintf("%s:%s", p, s)
}

// LoadPermits hydrates the offline in-memory cache with valid reservations and vehicle permits.
func (c *OfflineGateCache) LoadPermits(permits map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for k, v := range permits {
		normKey := strings.ToUpper(strings.TrimSpace(k))
		c.permits[normKey] = v
		c.permitTimes[normKey] = now
	}
}

// AuthorizeOffline checks the offline permit cache for the given license plate and state.
// It grants or denies gate entry and atomically appends an immutable WALRecord with IEEE CRC32.
func (c *OfflineGateCache) AuthorizeOffline(plate, state string) (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	normPlate := strings.ToUpper(strings.TrimSpace(plate))
	normState := strings.ToUpper(strings.TrimSpace(state))
	keyWithState := fmt.Sprintf("%s:%s", normPlate, normState)

	var granted bool
	var reason string

	permitData, ok := c.permits[keyWithState]
	if !ok {
		// Fall back to plate-only key
		permitData, ok = c.permits[normPlate]
	}

	if ok {
		granted = true
		reason = fmt.Sprintf("Granted: Active permit [%s]", permitData)
		c.offlineUsage[keyWithState]++
	} else {
		granted = false
		reason = fmt.Sprintf("Denied: Vehicle %s (%s) not found in offline permit cache", normPlate, normState)
	}

	now := time.Now()
	c.sequence++

	authPayload := GateAuthorizationPayload{
		Plate:     normPlate,
		State:     normState,
		Granted:   granted,
		Reason:    reason,
		Timestamp: now,
	}
	dataBytes, _ := json.Marshal(authPayload)

	rec := WALRecord{
		Sequence:   c.sequence,
		Timestamp:  now,
		EntityType: "VehicleAuthorization",
		EntityID:   keyWithState,
		Operation:  "INSERT",
		DataJSON:   string(dataBytes),
	}
	rec.CRC32 = ComputeWALRecordCRC32(rec)

	c.wal = append(c.wal, rec)
	return granted, reason
}

// ReplayWAL audits the append-only log, verifying the IEEE CRC32 checksum for every record.
// If any checksum discrepancy is detected, it fails immediately with an integrity violation error.
func (c *OfflineGateCache) ReplayWAL() ([]WALRecord, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	replayed := make([]WALRecord, len(c.wal))
	copy(replayed, c.wal)

	for _, rec := range replayed {
		expectedCRC := ComputeWALRecordCRC32(rec)
		if rec.CRC32 != expectedCRC {
			return nil, fmt.Errorf("sync: CRC32 checksum mismatch for WAL sequence %d: got 0x%08X, expected 0x%08X",
				rec.Sequence, rec.CRC32, expectedCRC)
		}
	}

	return replayed, nil
}

// ReconcileWithCloud incorporates upstream cloud events into the local cache using CRDT / LWW semantics.
// Returns the count of successfully applied events and detected write/usage conflicts.
func (c *OfflineGateCache) ReconcileWithCloud(cloudEvents []WALRecord) (appliedCount int, conflictCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, event := range cloudEvents {
		// Optional integrity verification if checksum present
		if event.CRC32 != 0 {
			if expected := ComputeWALRecordCRC32(event); event.CRC32 != expected {
				conflictCount++
				continue
			}
		}

		key := strings.ToUpper(strings.TrimSpace(event.EntityID))
		localTime, exists := c.permitTimes[key]
		usageCount := c.offlineUsage[key]

		switch event.Operation {
		case "INSERT", "UPDATE":
			// If modified locally after the cloud event timestamp, flag a concurrent conflict
			if exists && localTime.After(event.Timestamp) {
				conflictCount++
				continue
			}
			c.permits[key] = event.DataJSON
			c.permitTimes[key] = event.Timestamp
			appliedCount++

		case "DELETE":
			// If a deleted permit was granted/used offline while disconnected, flag a conflict
			if usageCount > 0 {
				conflictCount++
			}
			delete(c.permits, key)
			delete(c.permitTimes, key)
			appliedCount++

		default:
			conflictCount++
		}
	}

	return appliedCount, conflictCount
}

