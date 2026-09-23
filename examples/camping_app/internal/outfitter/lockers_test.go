package outfitter

import (
	"fmt"
	"regexp"
	"sync"
	"testing"
)

func TestGearReservationLockerAllocationAndPINGeneration(t *testing.T) {
	mgr := NewLockerManager()
	mgr.SeedDefaultGear()

	available := mgr.ListAvailableGear()
	if len(available) == 0 {
		t.Fatal("expected seeded gear to be available")
	}

	targetItem := available[0]
	userID := "user-climber-42"
	reservationID := "res-alpine-991"
	rentalDays := 3

	bay, err := mgr.ReserveGearAndLocker(userID, reservationID, targetItem.SerialNumber, rentalDays)
	if err != nil {
		t.Fatalf("failed to reserve gear and locker: %v", err)
	}

	// Verify bay number is within 1..16
	if bay.BayNumber < 1 || bay.BayNumber > 16 {
		t.Fatalf("allocated bay number %d outside valid 1..16 range", bay.BayNumber)
	}

	// Verify 6-digit OTP PIN
	if !sixDigitRegex.MatchString(bay.PasscodePIN) {
		t.Fatalf("generated passcode PIN %q is not a valid 6-digit numeric string", bay.PasscodePIN)
	}

	// Verify bay metadata
	if bay.AssignedUserID != userID || bay.ReservationID != reservationID || bay.ItemSerialNumber != targetItem.SerialNumber {
		t.Fatalf("bay metadata mismatch: %+v", bay)
	}
	if bay.Status != LockerStatusLoaded {
		t.Fatalf("expected bay status %s, got %s", LockerStatusLoaded, bay.Status)
	}

	// Verify gear status changed to RESERVED
	item, err := mgr.GetGearItem(targetItem.SerialNumber)
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}
	if item.Status != GearStatusReserved {
		t.Fatalf("expected gear status %s, got %s", GearStatusReserved, item.Status)
	}

	// Attempting to reserve the same gear item again must fail
	_, err = mgr.ReserveGearAndLocker("user-other", "res-other", targetItem.SerialNumber, 1)
	if err == nil {
		t.Fatal("expected reservation to fail for already reserved gear, got nil")
	}
}

func TestPINVerificationAndLockerUnlock(t *testing.T) {
	mgr := NewLockerManager()
	mgr.SeedDefaultGear()

	itemSerial := "SB-INREACH-101"
	bay, err := mgr.ReserveGearAndLocker("user-trekker-01", "res-expedition-55", itemSerial, 4)
	if err != nil {
		t.Fatalf("reservation failed: %v", err)
	}

	// Try with wrong PINs
	wrongPINs := []string{"00000", "999999", "123456", "ABCDEF", "0000000"}
	for _, wp := range wrongPINs {
		if wp == bay.PasscodePIN {
			continue // avoid accidental collision
		}
		unlocked, _, err := mgr.UnlockLocker(bay.BayNumber, wp)
		if unlocked || err == nil {
			t.Fatalf("expected unlock to fail with incorrect PIN %q", wp)
		}
	}

	// Verify bay is still LOADED
	status, err := mgr.GetLockerStatus(bay.BayNumber)
	if err != nil {
		t.Fatalf("failed to get locker status: %v", err)
	}
	if status.Status != LockerStatusLoaded {
		t.Fatalf("expected bay to remain LOADED, got %s", status.Status)
	}

	// Unlock with correct PIN
	unlocked, unlockedSerial, err := mgr.UnlockLocker(bay.BayNumber, bay.PasscodePIN)
	if err != nil {
		t.Fatalf("unlock failed with correct PIN: %v", err)
	}
	if !unlocked {
		t.Fatal("expected unlocked to be true")
	}
	if unlockedSerial != itemSerial {
		t.Fatalf("expected unlocked serial %s, got %s", itemSerial, unlockedSerial)
	}

	// Verify bay status transitioned to CLAIMED
	postStatus, err := mgr.GetLockerStatus(bay.BayNumber)
	if err != nil {
		t.Fatalf("failed to get locker status: %v", err)
	}
	if postStatus.Status != LockerStatusClaimed {
		t.Fatalf("expected bay status %s, got %s", LockerStatusClaimed, postStatus.Status)
	}

	// Verify gear item status transitioned to CHECKED_OUT
	item, err := mgr.GetGearItem(itemSerial)
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}
	if item.Status != GearStatusCheckedOut {
		t.Fatalf("expected gear status %s, got %s", GearStatusCheckedOut, item.Status)
	}

	// Attempting to unlock an already CLAIMED locker must fail
	_, _, err = mgr.UnlockLocker(bay.BayNumber, bay.PasscodePIN)
	if err == nil {
		t.Fatal("expected unlock on already CLAIMED locker to fail, got nil")
	}
}

func TestReturnCycle(t *testing.T) {
	mgr := NewLockerManager()
	mgr.SeedDefaultGear()

	itemSerial := "TN-TRANGO-201"
	bay, err := mgr.ReserveGearAndLocker("user-mountaineer", "res-rainier-44", itemSerial, 5)
	if err != nil {
		t.Fatalf("reservation failed: %v", err)
	}

	// Unlock and claim gear
	_, _, err = mgr.UnlockLocker(bay.BayNumber, bay.PasscodePIN)
	if err != nil {
		t.Fatalf("unlock failed: %v", err)
	}

	// Return gear to locker
	err = mgr.ReturnGearToLocker(bay.BayNumber, itemSerial)
	if err != nil {
		t.Fatalf("return gear to locker failed: %v", err)
	}

	// Verify bay status is RETURNED
	bayStatus, err := mgr.GetLockerStatus(bay.BayNumber)
	if err != nil {
		t.Fatalf("failed to get locker status: %v", err)
	}
	if bayStatus.Status != LockerStatusReturned {
		t.Fatalf("expected bay status %s, got %s", LockerStatusReturned, bayStatus.Status)
	}

	// Verify gear item status is restored to AVAILABLE
	item, err := mgr.GetGearItem(itemSerial)
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}
	if item.Status != GearStatusAvailable {
		t.Fatalf("expected gear item status %s, got %s", GearStatusAvailable, item.Status)
	}

	// Return with wrong item serial must fail
	err = mgr.ReturnGearToLocker(bay.BayNumber, "WRONG-SERIAL")
	if err == nil {
		t.Fatal("expected error on return with mismatched serial, got nil")
	}

	// Reset bay back to IDLE
	err = mgr.ResetLockerBay(bay.BayNumber)
	if err != nil {
		t.Fatalf("failed to reset locker bay: %v", err)
	}
	resetStatus, err := mgr.GetLockerStatus(bay.BayNumber)
	if err != nil {
		t.Fatalf("failed to get reset locker status: %v", err)
	}
	if resetStatus.Status != LockerStatusIdle {
		t.Fatalf("expected bay status %s, got %s", LockerStatusIdle, resetStatus.Status)
	}
}

func TestConcurrentLockerReservationsAcross50Goroutines(t *testing.T) {
	mgr := NewLockerManager()

	// Seed 50 individual gear items so equipment inventory does not exhaust before locker bays
	for i := 1; i <= 50; i++ {
		err := mgr.AddGearItem(&GearItem{
			SerialNumber:   fmt.Sprintf("GEAR-EXP-%03d", i),
			Name:           fmt.Sprintf("Expedition Item %03d", i),
			Category:       CategoryFourSeasonTent,
			DailyRateCents: 2000,
			DepositCents:   40000,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		})
		if err != nil {
			t.Fatalf("failed to add gear item: %v", err)
		}
	}

	numGoroutines := 50
	var wg sync.WaitGroup
	type resResult struct {
		bay *LockerBay
		err error
	}
	results := make([]resResult, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			itemSerial := fmt.Sprintf("GEAR-EXP-%03d", idx+1)
			userID := fmt.Sprintf("user-hiker-%03d", idx+1)
			reservationID := fmt.Sprintf("res-%03d", idx+1)

			bay, err := mgr.ReserveGearAndLocker(userID, reservationID, itemSerial, 2)
			results[idx] = resResult{bay: bay, err: err}
		}(i)
	}

	wg.Wait()

	// Exactly 16 bays should be allocated (1..16), and exactly 34 should fail due to full locker bank
	allocatedBays := make(map[int]string) // bayNumber -> itemSerial
	var successCount int
	var failureCount int

	for i, res := range results {
		if res.err == nil {
			successCount++
			bayNum := res.bay.BayNumber
			if bayNum < 1 || bayNum > 16 {
				t.Errorf("goroutine %d assigned invalid bay number %d", i, bayNum)
			}
			if prevSerial, exists := allocatedBays[bayNum]; exists {
				t.Fatalf("DOUBLE ASSIGNMENT DETECTED! Bay %d was assigned to both %s and %s", bayNum, prevSerial, res.bay.ItemSerialNumber)
			}
			allocatedBays[bayNum] = res.bay.ItemSerialNumber
		} else {
			failureCount++
		}
	}

	if successCount != 16 {
		t.Fatalf("expected exactly 16 successful reservations (matching 16 bays), got %d", successCount)
	}
	if failureCount != 34 {
		t.Fatalf("expected exactly 34 rejected reservations, got %d", failureCount)
	}

	// Verify all 16 locker bays are unique and in LOADED status
	for bayNum := 1; bayNum <= 16; bayNum++ {
		status, err := mgr.GetLockerStatus(bayNum)
		if err != nil {
			t.Fatalf("failed to query bay %d: %v", bayNum, err)
		}
		if status.Status != LockerStatusLoaded {
			t.Fatalf("expected bay %d to be LOADED, got %s", bayNum, status.Status)
		}
	}
}

var sixDigitRegex = regexp.MustCompile(`^[0-9]{6}$`)
