package backcountry

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestZoneRegistrationAndQuotaDecrementing(t *testing.T) {
	engine := NewBackcountryEngine()

	zone := &TrailheadZone{
		ID:                   "th-enchantments-core",
		Name:                 "Enchantment Core Zone",
		ElevationFt:          7800,
		DailyQuota:           16,
		BearCanisterRequired: true,
		WAGBagRequired:       true,
		DifficultyRating:     DifficultyAlpineTechnical,
	}

	err := engine.RegisterZone(zone)
	if err != nil {
		t.Fatalf("expected zone registration to succeed, got %v", err)
	}

	// Verify retrieved zone
	retrieved, err := engine.GetZone(zone.ID)
	if err != nil {
		t.Fatalf("failed to retrieve registered zone: %v", err)
	}
	if retrieved.DailyQuota != 16 || retrieved.DifficultyRating != DifficultyAlpineTechnical {
		t.Fatalf("zone data mismatch: got %+v", retrieved)
	}

	// Test available quota for target date
	targetDate := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	initialQuota := engine.GetAvailableQuota(zone.ID, targetDate)
	if initialQuota != 16 {
		t.Fatalf("expected initial available quota 16, got %d", initialQuota)
	}

	// Decrement quota by 6
	err = engine.DecrementQuota(zone.ID, targetDate, 6)
	if err != nil {
		t.Fatalf("expected successful decrement, got %v", err)
	}

	postQuota := engine.GetAvailableQuota(zone.ID, targetDate)
	if postQuota != 10 {
		t.Fatalf("expected remaining quota 10, got %d", postQuota)
	}

	// Decrement by 10
	err = engine.DecrementQuota(zone.ID, targetDate, 10)
	if err != nil {
		t.Fatalf("expected successful decrement, got %v", err)
	}

	exhaustedQuota := engine.GetAvailableQuota(zone.ID, targetDate)
	if exhaustedQuota != 0 {
		t.Fatalf("expected 0 quota remaining, got %d", exhaustedQuota)
	}

	// Decrement exceeding quota should return error
	err = engine.DecrementQuota(zone.ID, targetDate, 1)
	if err == nil {
		t.Fatal("expected error on decrementing beyond available quota, got nil")
	}

	// Decrement on unregistered zone should return error
	err = engine.DecrementQuota("non-existent-zone", targetDate, 1)
	if err == nil {
		t.Fatal("expected error for non-existent zone, got nil")
	}

	// Invalid difficulty rating registration test
	badZone := &TrailheadZone{
		ID:               "th-invalid",
		Name:             "Invalid Zone",
		DailyQuota:       5,
		DifficultyRating: "EXTREME_CHAOS",
	}
	if err := engine.RegisterZone(badZone); err == nil {
		t.Fatal("expected error for invalid difficulty rating, got nil")
	}
}

func TestProvablyFairLotteryDrawDeterministicReproducibility(t *testing.T) {
	// We run two completely independent engines with identical zones, salt, and applications.
	// The provably fair PRNG seeded with HMAC-SHA256(secretSalt, commitmentHash) MUST produce
	// the exact same order of winners and identical winner IDs.
	zoneID := "th-mt-whitney-main"
	targetDate := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	secretSalt := "usfs-inyo-secret-salt-alpha-99281-x99"

	buildEngine := func() (*BackcountryEngine, error) {
		eng := NewBackcountryEngine()
		err := eng.RegisterZone(&TrailheadZone{
			ID:                   zoneID,
			Name:                 "Mt. Whitney Trailhead Main",
			ElevationFt:          8360,
			DailyQuota:           10, // Quota allows several parties
			BearCanisterRequired: true,
			WAGBagRequired:       true,
			DifficultyRating:     DifficultyStrenuous,
		})
		if err != nil {
			return nil, err
		}

		// Publish commitment
		_, err = eng.PublishLotteryCommitment(zoneID, targetDate, secretSalt)
		if err != nil {
			return nil, err
		}

		// Submit fixed set of applications with fixed IDs and party sizes
		apps := []struct {
			id        string
			name      string
			partySize int
			lntCert   string
		}{
			{"app-whitney-01", "Alex Honnold", 2, "LNT-CERT-7701"},
			{"app-whitney-02", "Tommy Caldwell", 3, "LNT-CERT-7702"},
			{"app-whitney-03", "Lynn Hill", 4, "LNT-CERT-7703"},
			{"app-whitney-04", "Conrad Anker", 2, "LNT-CERT-7704"},
			{"app-whitney-05", "Jimmy Chin", 1, "LNT-CERT-7705"},
			{"app-whitney-06", "Beth Rodden", 4, "LNT-CERT-7706"},
			{"app-whitney-07", "Emily Harrington", 3, "LNT-CERT-7707"},
		}

		for _, a := range apps {
			err := eng.SubmitApplication(&LotteryApplication{
				ID:                 a.id,
				UserID:             fmt.Sprintf("user-%s", a.id),
				FullName:           a.name,
				TrailheadID:        zoneID,
				TargetDate:         targetDate,
				PartySize:          a.partySize,
				LeaveNoTraceCertID: a.lntCert,
				SubmittedAt:        time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			})
			if err != nil {
				return nil, err
			}
		}

		return eng, nil
	}

	eng1, err := buildEngine()
	if err != nil {
		t.Fatalf("failed building engine 1: %v", err)
	}
	eng2, err := buildEngine()
	if err != nil {
		t.Fatalf("failed building engine 2: %v", err)
	}

	winners1, err := eng1.ExecuteProvablyFairDraw(zoneID, targetDate, secretSalt)
	if err != nil {
		t.Fatalf("eng1 draw failed: %v", err)
	}

	winners2, err := eng2.ExecuteProvablyFairDraw(zoneID, targetDate, secretSalt)
	if err != nil {
		t.Fatalf("eng2 draw failed: %v", err)
	}

	if len(winners1) == 0 {
		t.Fatal("expected non-zero winners from lottery draw")
	}

	if len(winners1) != len(winners2) {
		t.Fatalf("winner count mismatch between runs: run1=%d, run2=%d", len(winners1), len(winners2))
	}

	// Verify exact ordering match
	for i := range winners1 {
		if winners1[i].ID != winners2[i].ID {
			t.Fatalf("winner[%d] mismatch: run1=%s, run2=%s", i, winners1[i].ID, winners2[i].ID)
		}
		if winners1[i].Status != StatusWon || winners2[i].Status != StatusWon {
			t.Fatalf("winner[%d] status not WON: run1=%s, run2=%s", i, winners1[i].Status, winners2[i].Status)
		}
	}

	// Check total allocated quota
	totalAwarded := 0
	for _, w := range winners1 {
		totalAwarded += w.PartySize
	}
	if totalAwarded > 10 {
		t.Fatalf("total awarded party size %d exceeded daily quota 10", totalAwarded)
	}

	remainingQuota := eng1.GetAvailableQuota(zoneID, targetDate)
	if remainingQuota != 10-totalAwarded {
		t.Fatalf("expected remaining quota %d, got %d", 10-totalAwarded, remainingQuota)
	}

	// Check commitment revealed secret
	com, err := eng1.GetCommitment(zoneID, targetDate)
	if err != nil {
		t.Fatalf("failed to get commitment: %v", err)
	}
	if com.RevealedSecret != secretSalt {
		t.Fatalf("expected revealed secret %s, got %s", secretSalt, com.RevealedSecret)
	}
	if com.DrawnAt == nil {
		t.Fatal("expected DrawnAt timestamp to be set")
	}
}

func TestTamperingDetection(t *testing.T) {
	engine := NewBackcountryEngine()
	zoneID := "th-grand-canyon-rim"
	targetDate := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	legitimateSalt := "nps-grand-canyon-auth-salt-genuine-993"
	tamperedSalt := "hacker-attempting-to-rig-lottery-seed-123"

	err := engine.RegisterZone(&TrailheadZone{
		ID:                   zoneID,
		Name:                 "Bright Angel Trailhead",
		ElevationFt:          6860,
		DailyQuota:           8,
		BearCanisterRequired: false,
		WAGBagRequired:       false,
		DifficultyRating:     DifficultyModerate,
	})
	if err != nil {
		t.Fatalf("failed to register zone: %v", err)
	}

	// Publish commitment using legitimate salt
	com, err := engine.PublishLotteryCommitment(zoneID, targetDate, legitimateSalt)
	if err != nil {
		t.Fatalf("failed to publish commitment: %v", err)
	}

	if com.CommitmentHash == "" {
		t.Fatal("commitment hash should not be empty")
	}

	// Submit applicant
	err = engine.SubmitApplication(&LotteryApplication{
		ID:                 "app-gc-1",
		UserID:             "user-101",
		FullName:           "John Wesley Powell",
		TrailheadID:        zoneID,
		TargetDate:         targetDate,
		PartySize:          2,
		LeaveNoTraceCertID: "LNT-CERT-GC-1",
	})
	if err != nil {
		t.Fatalf("failed to submit application: %v", err)
	}

	// Attempt draw with tampered salt -> must fail!
	_, err = engine.ExecuteProvablyFairDraw(zoneID, targetDate, tamperedSalt)
	if err == nil {
		t.Fatal("expected draw to fail due to tampered secret salt, but it succeeded")
	}

	// Now execute with legitimate salt -> must succeed
	winners, err := engine.ExecuteProvablyFairDraw(zoneID, targetDate, legitimateSalt)
	if err != nil {
		t.Fatalf("expected legitimate draw to succeed, got %v", err)
	}
	if len(winners) != 1 || winners[0].ID != "app-gc-1" {
		t.Fatalf("expected 1 winner app-gc-1, got %v", winners)
	}
}

func TestConcurrentApplications(t *testing.T) {
	engine := NewBackcountryEngine()
	zoneID := "th-olympic-hoh"
	targetDate := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	err := engine.RegisterZone(&TrailheadZone{
		ID:                   zoneID,
		Name:                 "Hoh River Trailhead",
		ElevationFt:          600,
		DailyQuota:           100, // Large quota
		BearCanisterRequired: true,
		WAGBagRequired:       false,
		DifficultyRating:     DifficultyModerate,
	})
	if err != nil {
		t.Fatalf("failed to register zone: %v", err)
	}

	numGoroutines := 50
	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			app := &LotteryApplication{
				ID:                 fmt.Sprintf("app-concurrent-%03d", idx),
				UserID:             fmt.Sprintf("user-%03d", idx),
				FullName:           fmt.Sprintf("Hiker Number %03d", idx),
				TrailheadID:        zoneID,
				TargetDate:         targetDate,
				PartySize:          1 + (idx % 3),
				LeaveNoTraceCertID: fmt.Sprintf("LNT-CERT-%03d", idx),
			}

			if err := engine.SubmitApplication(app); err != nil {
				errCh <- fmt.Errorf("goroutine %d submit failed: %w", idx, err)
				return
			}

			// Interleaved read of available quota
			_ = engine.GetAvailableQuota(zoneID, targetDate)
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent application error: %v", err)
	}

	// Verify all 50 entries registered properly
	for i := 0; i < numGoroutines; i++ {
		appID := fmt.Sprintf("app-concurrent-%03d", i)
		app, err := engine.GetApplication(appID)
		if err != nil {
			t.Fatalf("missing application %s: %v", appID, err)
		}
		if app.Status != StatusPending {
			t.Fatalf("application %s expected PENDING status, got %s", appID, app.Status)
		}
	}
}

func TestApplicationValidations(t *testing.T) {
	engine := NewBackcountryEngine()
	zoneID := "th-zion-subway"
	targetDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	_ = engine.RegisterZone(&TrailheadZone{
		ID:                   zoneID,
		Name:                 "The Subway (Left Fork)",
		ElevationFt:          4500,
		DailyQuota:           12,
		BearCanisterRequired: false,
		WAGBagRequired:       true,
		DifficultyRating:     DifficultyStrenuous,
	})

	// Missing LNT cert
	err := engine.SubmitApplication(&LotteryApplication{
		UserID:             "user-1",
		FullName:           "Alice",
		TrailheadID:        zoneID,
		TargetDate:         targetDate,
		PartySize:          2,
		LeaveNoTraceCertID: "", // empty
	})
	if err == nil {
		t.Fatal("expected error on missing LNT cert, got nil")
	}

	// Party size exceeding daily quota
	err = engine.SubmitApplication(&LotteryApplication{
		UserID:             "user-2",
		FullName:           "Bob",
		TrailheadID:        zoneID,
		TargetDate:         targetDate,
		PartySize:          15, // exceeds 12
		LeaveNoTraceCertID: "LNT-CERT-123",
	})
	if err == nil {
		t.Fatal("expected error on party size exceeding zone quota, got nil")
	}

	// Non-existent zone
	err = engine.SubmitApplication(&LotteryApplication{
		UserID:             "user-3",
		FullName:           "Charlie",
		TrailheadID:        "ghost-zone",
		TargetDate:         targetDate,
		PartySize:          2,
		LeaveNoTraceCertID: "LNT-CERT-123",
	})
	if err == nil {
		t.Fatal("expected error on unknown zone, got nil")
	}
}

