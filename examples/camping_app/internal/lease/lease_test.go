package lease

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBasicAcquireCommitRelease(t *testing.T) {
	engine := NewEngine()

	start := time.Now().Add(48 * time.Hour)
	end := start.Add(72 * time.Hour)
	campsiteID := "campsite-basic-1"
	userID := "user-alice"

	// 1. Acquire hold
	hold, err := engine.AcquireHold(campsiteID, userID, start, end)
	if err != nil {
		t.Fatalf("Failed to acquire hold: %v", err)
	}

	if hold == nil {
		t.Fatal("Expected non-nil hold token")
	}

	if len(hold.TokenID) != 32 {
		t.Errorf("Expected 32-char hex token ID, got %d chars: %s", len(hold.TokenID), hold.TokenID)
	}

	if hold.CampsiteID != campsiteID || hold.UserID != userID {
		t.Errorf("Unexpected hold campsite (%s) or user (%s)", hold.CampsiteID, hold.UserID)
	}

	if hold.Status != StatusHeld {
		t.Errorf("Expected status %s, got %s", StatusHeld, hold.Status)
	}

	// Verify expiration is approximately 15 minutes in the future
	expectedExpiry := time.Now().Add(15 * time.Minute)
	if hold.ExpiresAt.Before(expectedExpiry.Add(-10*time.Second)) || hold.ExpiresAt.After(expectedExpiry.Add(10*time.Second)) {
		t.Errorf("ExpiresAt %v deviates from 15min expected %v", hold.ExpiresAt, expectedExpiry)
	}

	// Active holds count should be 1
	active := engine.GetActiveHolds(campsiteID)
	if len(active) != 1 {
		t.Fatalf("Expected 1 active hold, got %d", len(active))
	}
	if active[0].TokenID != hold.TokenID {
		t.Errorf("Active hold token mismatch: got %s, want %s", active[0].TokenID, hold.TokenID)
	}

	// 2. Commit hold
	if err := engine.CommitHold(hold.TokenID); err != nil {
		t.Fatalf("Failed to commit hold: %v", err)
	}

	committed, err := engine.GetHold(hold.TokenID)
	if err != nil {
		t.Fatalf("Failed to get hold: %v", err)
	}
	if committed.Status != StatusCommitted {
		t.Errorf("Expected committed status %s, got %s", StatusCommitted, committed.Status)
	}

	// Once committed, it should no longer be returned as a pending active hold
	activeAfterCommit := engine.GetActiveHolds(campsiteID)
	if len(activeAfterCommit) != 0 {
		t.Errorf("Expected 0 active holds after commitment, got %d", len(activeAfterCommit))
	}

	// 3. Release hold test on another campsite
	campsiteID2 := "campsite-basic-2"
	hold2, err := engine.AcquireHold(campsiteID2, userID, start, end)
	if err != nil {
		t.Fatalf("Failed to acquire second hold: %v", err)
	}

	if err := engine.ReleaseHold(hold2.TokenID); err != nil {
		t.Fatalf("Failed to release hold: %v", err)
	}

	released, err := engine.GetHold(hold2.TokenID)
	if err != nil {
		t.Fatalf("Failed to get released hold: %v", err)
	}
	if released.Status != StatusCancelled {
		t.Errorf("Expected status %s, got %s", StatusCancelled, released.Status)
	}

	activeAfterRelease := engine.GetActiveHolds(campsiteID2)
	if len(activeAfterRelease) != 0 {
		t.Errorf("Expected 0 active holds after release, got %d", len(activeAfterRelease))
	}
}

func TestDoubleHoldConflict(t *testing.T) {
	engine := NewEngine()

	campsiteID := "campsite-conflict"
	start := time.Now().Add(10 * 24 * time.Hour)
	end := start.Add(5 * 24 * time.Hour) // 10 to 15 days out

	// First camper acquires hold
	hold1, err := engine.AcquireHold(campsiteID, "user-bob", start, end)
	if err != nil {
		t.Fatalf("Initial hold acquisition failed: %v", err)
	}
	if hold1 == nil {
		t.Fatal("Initial hold is nil")
	}

	// 1. Exact duplicate slot contention
	_, err = engine.AcquireHold(campsiteID, "user-carol", start, end)
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Errorf("Expected ErrSlotUnavailable for exact duplicate slot, got: %v", err)
	}

	// 2. Overlapping slot: starts during existing booking
	overlapStart := start.Add(2 * 24 * time.Hour)
	overlapEnd := end.Add(2 * 24 * time.Hour)
	_, err = engine.AcquireHold(campsiteID, "user-dave", overlapStart, overlapEnd)
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Errorf("Expected ErrSlotUnavailable for late overlap, got: %v", err)
	}

	// 3. Overlapping slot: ends during existing booking
	overlapEarlyStart := start.Add(-2 * 24 * time.Hour)
	overlapEarlyEnd := start.Add(2 * 24 * time.Hour)
	_, err = engine.AcquireHold(campsiteID, "user-eve", overlapEarlyStart, overlapEarlyEnd)
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Errorf("Expected ErrSlotUnavailable for early overlap, got: %v", err)
	}

	// 4. Overlapping slot: fully encloses existing booking
	enclosingStart := start.Add(-2 * 24 * time.Hour)
	enclosingEnd := end.Add(2 * 24 * time.Hour)
	_, err = engine.AcquireHold(campsiteID, "user-frank", enclosingStart, enclosingEnd)
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Errorf("Expected ErrSlotUnavailable for enclosing slot, got: %v", err)
	}

	// 5. Non-overlapping contiguous slots MUST succeed
	// Back-to-back: checkout day matches next check-in day (end == nextStart)
	contiguousAfterStart := end
	contiguousAfterEnd := end.Add(3 * 24 * time.Hour)
	holdAfter, err := engine.AcquireHold(campsiteID, "user-grace", contiguousAfterStart, contiguousAfterEnd)
	if err != nil {
		t.Fatalf("Expected contiguous subsequent hold to succeed, got: %v", err)
	}
	if holdAfter == nil {
		t.Fatal("Expected non-nil holdAfter")
	}

	// Prior contiguous: previous checkout matches existing check-in (prevEnd == start)
	contiguousBeforeStart := start.Add(-3 * 24 * time.Hour)
	contiguousBeforeEnd := start
	holdBefore, err := engine.AcquireHold(campsiteID, "user-heidi", contiguousBeforeStart, contiguousBeforeEnd)
	if err != nil {
		t.Fatalf("Expected contiguous prior hold to succeed, got: %v", err)
	}
	if holdBefore == nil {
		t.Fatal("Expected non-nil holdBefore")
	}

	// 6. Invalid dates checks
	// End before start
	_, err = engine.AcquireHold(campsiteID, "user-invalid", end, start)
	if !errors.Is(err, ErrInvalidDates) {
		t.Errorf("Expected ErrInvalidDates when end < start, got: %v", err)
	}

	// Start in the past
	pastStart := time.Now().Add(-24 * time.Hour)
	pastEnd := time.Now().Add(24 * time.Hour)
	_, err = engine.AcquireHold(campsiteID, "user-invalid-past", pastStart, pastEnd)
	if !errors.Is(err, ErrInvalidDates) {
		t.Errorf("Expected ErrInvalidDates for past start time, got: %v", err)
	}

	// 7. Committed reservation conflict
	campsiteCommitted := "campsite-committed"
	resStart := time.Now().Add(30 * 24 * time.Hour)
	resEnd := resStart.Add(5 * 24 * time.Hour)
	hComm, err := engine.AcquireHold(campsiteCommitted, "user-comm", resStart, resEnd)
	if err != nil {
		t.Fatalf("Failed to acquire hold: %v", err)
	}
	if err := engine.CommitHold(hComm.TokenID); err != nil {
		t.Fatalf("Failed to commit hold: %v", err)
	}

	// New user tries to hold committed reservation slot
	_, err = engine.AcquireHold(campsiteCommitted, "user-intruder", resStart, resEnd)
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Errorf("Expected ErrSlotUnavailable for committed reservation conflict, got: %v", err)
	}
}

func TestConcurrentSwarmContention(t *testing.T) {
	engine := NewEngine()

	campsiteID := "campsite-swarm-race"
	start := time.Now().Add(60 * 24 * time.Hour)
	end := start.Add(5 * 24 * time.Hour)

	concurrency := 50
	var wg sync.WaitGroup
	var successCount int64
	var conflictCount int64
	var unexpectedErrors int64

	startBarrier := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		userID := fmt.Sprintf("camper-%02d", i)

		go func(uid string) {
			defer wg.Done()

			// Block until all goroutines are ready to unleash simultaneous contention
			<-startBarrier

			hold, err := engine.AcquireHold(campsiteID, uid, start, end)
			if err == nil && hold != nil {
				atomic.AddInt64(&successCount, 1)
			} else if errors.Is(err, ErrSlotUnavailable) {
				atomic.AddInt64(&conflictCount, 1)
			} else {
				atomic.AddInt64(&unexpectedErrors, 1)
			}
		}(userID)
	}

	// Unleash all 50 concurrent goroutines simultaneously
	close(startBarrier)
	wg.Wait()

	if unexpectedErrors > 0 {
		t.Fatalf("Encountered %d unexpected errors during contention race", unexpectedErrors)
	}

	if successCount != 1 {
		t.Fatalf("Expected exactly 1 winner out of %d concurrent racers, got %d", concurrency, successCount)
	}

	expectedConflicts := int64(concurrency - 1)
	if conflictCount != expectedConflicts {
		t.Fatalf("Expected %d ErrSlotUnavailable conflicts, got %d", expectedConflicts, conflictCount)
	}

	activeHolds := engine.GetActiveHolds(campsiteID)
	if len(activeHolds) != 1 {
		t.Fatalf("Expected 1 active hold in engine calendar, got %d", len(activeHolds))
	}
}

func TestTTLExpirationCleanup(t *testing.T) {
	engine := NewEngine()

	// Configure a short 60ms TTL for testing expiration
	engine.SetHoldTTL(60 * time.Millisecond)

	campsiteID := "campsite-ttl-test"
	start := time.Now().Add(90 * 24 * time.Hour)
	end := start.Add(3 * 24 * time.Hour)

	hold, err := engine.AcquireHold(campsiteID, "user-ttl-1", start, end)
	if err != nil {
		t.Fatalf("Failed to acquire initial hold: %v", err)
	}

	// Immediately after acquisition, hold is active
	activeImmediate := engine.GetActiveHolds(campsiteID)
	if len(activeImmediate) != 1 {
		t.Fatalf("Expected 1 active hold immediately, got %d", len(activeImmediate))
	}

	// Sleep past the TTL expiration
	time.Sleep(80 * time.Millisecond)

	// GetActiveHolds should now filter out expired holds
	activeAfterExpiry := engine.GetActiveHolds(campsiteID)
	if len(activeAfterExpiry) != 0 {
		t.Errorf("Expected 0 active holds after TTL expiration, got %d", len(activeAfterExpiry))
	}

	// Run background sweeper
	cleaned := engine.CleanupExpiredHolds()
	if cleaned != 1 {
		t.Errorf("Expected CleanupExpiredHolds to sweep 1 hold, swept %d", cleaned)
	}

	// Subsequent sweep cleans 0 holds
	cleanedSecond := engine.CleanupExpiredHolds()
	if cleanedSecond != 0 {
		t.Errorf("Expected second CleanupExpiredHolds to sweep 0 holds, swept %d", cleanedSecond)
	}

	// Slot should now be immediately re-acquirable by a different user
	hold2, err := engine.AcquireHold(campsiteID, "user-ttl-2", start, end)
	if err != nil {
		t.Fatalf("Expected slot to be re-acquirable after TTL expiration, got: %v", err)
	}
	if hold2 == nil || hold2.TokenID == hold.TokenID {
		t.Fatalf("Expected new distinct hold token after expiration")
	}

	// Test attempting to commit an expired hold
	campsiteID2 := "campsite-ttl-commit"
	holdExpired, err := engine.AcquireHold(campsiteID2, "user-ttl-3", start, end)
	if err != nil {
		t.Fatalf("Failed to acquire hold: %v", err)
	}

	time.Sleep(80 * time.Millisecond) // expire

	err = engine.CommitHold(holdExpired.TokenID)
	if !errors.Is(err, ErrHoldExpired) {
		t.Errorf("Expected ErrHoldExpired when committing expired hold, got: %v", err)
	}
}

