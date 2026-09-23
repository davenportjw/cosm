package sync

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOfflineCacheLoadingAndGateClearance(t *testing.T) {
	cache := NewOfflineGateCache()

	permits := map[string]string{
		"ALPINE-1:CO": "Permit-RES-101 (Site 4A)",
		"TREK-88:CA":  "Permit-RES-102 (Site 12B)",
		"YOSE-99":     "Permit-RES-103 (Site 01A)",
	}
	cache.LoadPermits(permits)

	// 1. Authorized vehicle with state
	granted, reason := cache.AuthorizeOffline("ALPINE-1", "CO")
	if !granted {
		t.Fatalf("expected vehicle ALPINE-1:CO to be authorized, got denied: %s", reason)
	}
	if !strings.Contains(reason, "Permit-RES-101") {
		t.Fatalf("expected permit reason to reference Permit-RES-101, got: %s", reason)
	}

	// 2. Case-insensitivity and trim whitespace
	granted2, _ := cache.AuthorizeOffline("  trek-88 ", "ca ")
	if !granted2 {
		t.Fatalf("expected lowercase whitespace-padded TREK-88 to be authorized")
	}

	// 3. Plate-only match
	granted3, _ := cache.AuthorizeOffline("YOSE-99", "")
	if !granted3 {
		t.Fatalf("expected plate-only YOSE-99 to be authorized")
	}

	// 4. Unauthorized vehicle
	granted4, reason4 := cache.AuthorizeOffline("UNKNOWN-404", "UT")
	if granted4 {
		t.Fatalf("expected UNKNOWN-404:UT to be denied")
	}
	if !strings.Contains(reason4, "not found in offline permit cache") {
		t.Fatalf("unexpected denial reason: %s", reason4)
	}
}

func TestWALAppendAndCRC32Verification(t *testing.T) {
	cache := NewOfflineGateCache()
	cache.LoadPermits(map[string]string{
		"TEST-01:CO": "Permit-1",
		"TEST-02:CO": "Permit-2",
	})

	cache.AuthorizeOffline("TEST-01", "CO")
	cache.AuthorizeOffline("TEST-02", "CO")
	cache.AuthorizeOffline("ROGUE-99", "WY")

	records, err := cache.ReplayWAL()
	if err != nil {
		t.Fatalf("failed replaying WAL: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 WAL records, got %d", len(records))
	}

	for i, r := range records {
		expectedCRC := ComputeWALRecordCRC32(r)
		if r.CRC32 != expectedCRC {
			t.Fatalf("record %d CRC32 mismatch: got 0x%08X, want 0x%08X", i, r.CRC32, expectedCRC)
		}
		if r.Sequence != uint64(i+1) {
			t.Fatalf("record %d sequence mismatch: got %d, want %d", i, r.Sequence, i+1)
		}
	}

	// Intentionally corrupt record 1 CRC32 to test detection
	cache.mu.Lock()
	cache.wal[1].CRC32 ^= 0xFFFFFFFF
	cache.mu.Unlock()

	_, errCorrupt := cache.ReplayWAL()
	if errCorrupt == nil {
		t.Fatalf("expected CRC32 checksum mismatch error, got nil")
	}
	if !strings.Contains(errCorrupt.Error(), "CRC32 checksum mismatch") {
		t.Fatalf("expected error message to mention checksum mismatch, got: %v", errCorrupt)
	}
}

func TestWALReplayAndReconciliation(t *testing.T) {
	cache := NewOfflineGateCache()
	cache.LoadPermits(map[string]string{
		"REVOKED-1:CO": "Permit-Revoked-A",
		"STATIC-1:CO":  "Permit-Static-B",
	})

	// Authorize REVOKED-1 while offline
	cache.AuthorizeOffline("REVOKED-1", "CO")

	now := time.Now()

	// Cloud sends updates:
	// 1. Valid new permit INSERT
	insertRec := WALRecord{
		Sequence:   100,
		Timestamp:  now,
		EntityType: "Permit",
		EntityID:   "NEW-PERMIT:CA",
		Operation:  "INSERT",
		DataJSON:   "Permit-Cloud-New",
	}
	insertRec.CRC32 = ComputeWALRecordCRC32(insertRec)

	// 2. DELETE for a permit that was used offline (REVOKED-1:CO) -> conflict!
	deleteRec := WALRecord{
		Sequence:   101,
		Timestamp:  now,
		EntityType: "Permit",
		EntityID:   "REVOKED-1:CO",
		Operation:  "DELETE",
		DataJSON:   "",
	}
	deleteRec.CRC32 = ComputeWALRecordCRC32(deleteRec)

	// 3. Stale UPDATE with timestamp in the past -> conflict
	staleRec := WALRecord{
		Sequence:   102,
		Timestamp:  now.Add(-2 * time.Hour),
		EntityType: "Permit",
		EntityID:   "STATIC-1:CO",
		Operation:  "UPDATE",
		DataJSON:   "Permit-Stale",
	}
	staleRec.CRC32 = ComputeWALRecordCRC32(staleRec)

	cloudEvents := []WALRecord{insertRec, deleteRec, staleRec}
	applied, conflicts := cache.ReconcileWithCloud(cloudEvents)

	if applied != 2 { // insert and delete applied to permit cache
		t.Fatalf("expected 2 applied events, got %d", applied)
	}
	if conflicts != 2 { // deleted permit had offline usage + stale update
		t.Fatalf("expected 2 conflicts, got %d", conflicts)
	}

	// Verify newly inserted permit is now authorizable offline
	granted, _ := cache.AuthorizeOffline("NEW-PERMIT", "CA")
	if !granted {
		t.Fatalf("expected newly reconciled permit NEW-PERMIT:CA to be authorized")
	}

	// Verify revoked permit is deleted
	grantedRevoked, _ := cache.AuthorizeOffline("REVOKED-1", "CO")
	if grantedRevoked {
		t.Fatalf("expected revoked permit REVOKED-1:CO to be denied after cloud delete")
	}
}

func TestConcurrentOfflineAuthorizations(t *testing.T) {
	cache := NewOfflineGateCache()

	const numGoroutines = 50
	permits := make(map[string]string, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		key := fmt.Sprintf("CONCURRENT-%02d:CO", i)
		permits[key] = fmt.Sprintf("Permit-Concurrent-%02d", i)
	}
	cache.LoadPermits(permits)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		gid := i
		go func() {
			defer wg.Done()
			plate := fmt.Sprintf("CONCURRENT-%02d", gid)
			granted, _ := cache.AuthorizeOffline(plate, "CO")
			if !granted {
				t.Errorf("goroutine %d failed to authorize %s", gid, plate)
			}

			// Also attempt an unauthorized check
			unauthPlate := fmt.Sprintf("UNAUTH-%02d", gid)
			denied, _ := cache.AuthorizeOffline(unauthPlate, "NV")
			if denied {
				t.Errorf("goroutine %d unexpectedly authorized %s", gid, unauthPlate)
			}
		}()
	}

	wg.Wait()

	// Verify WAL integrity across all 100 entries (50 granted + 50 denied)
	records, err := cache.ReplayWAL()
	if err != nil {
		t.Fatalf("concurrent WAL replay failed: %v", err)
	}
	if len(records) != numGoroutines*2 {
		t.Fatalf("expected %d records in WAL, got %d", numGoroutines*2, len(records))
	}

	// Check strict monotonic sequence numbers
	for i, r := range records {
		if r.Sequence != uint64(i+1) {
			t.Fatalf("WAL sequence gap at %d: got %d, want %d", i, r.Sequence, i+1)
		}
	}
}

