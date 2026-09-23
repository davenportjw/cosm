package pms

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAssetRegistrationConditionAndSchedule(t *testing.T) {
	mgr := NewPMSAssetManager()

	cgID := "cg-yosemite-upper-pines"
	now := time.Now().UTC()

	// 1. Register multiple assets
	asset1 := &CampsiteAsset{
		ID:                "ASSET-BB-101",
		CampgroundID:      cgID,
		CampsiteID:        "cs-up-101",
		AssetClass:        AssetBearBox,
		SerialNumber:      "BB-YOS-2024-001",
		InstalledDate:     now.AddDate(-1, 0, 0),
		ConditionRating:   ConditionGood,
		LastInspected:     now.AddDate(0, -3, 0),
		NextInspectionDue: now.AddDate(0, 3, 0),
	}

	asset2 := &CampsiteAsset{
		ID:                "ASSET-FR-101",
		CampgroundID:      cgID,
		CampsiteID:        "cs-up-101",
		AssetClass:        AssetFireRing,
		SerialNumber:      "FR-YOS-2023-088",
		InstalledDate:     now.AddDate(-2, 0, 0),
		ConditionRating:   ConditionNeedsRepair,
		LastInspected:     now.AddDate(0, -1, 0),
		NextInspectionDue: now.AddDate(0, 1, 0),
		Notes:             "Cracked steel grate hinges",
	}

	asset3 := &CampsiteAsset{
		ID:                "ASSET-WS-202",
		CampgroundID:      cgID,
		CampsiteID:        "cs-up-202",
		AssetClass:        AssetWaterSpigot,
		SerialNumber:      "WS-YOS-2022-014",
		InstalledDate:     now.AddDate(-3, 0, 0),
		ConditionRating:   ConditionCondemned,
		LastInspected:     now.AddDate(0, -2, 0),
		NextInspectionDue: now.AddDate(0, 0, 5), // due very soon
		Notes:             "Backflow preventer failure; shut off at main valve",
	}

	// Another campground asset
	asset4 := &CampsiteAsset{
		ID:                "ASSET-SP-501",
		CampgroundID:      "cg-zion-south",
		CampsiteID:        "cs-zion-501",
		AssetClass:        AssetSolarPedestal,
		SerialNumber:      "SP-ZN-2024-009",
		InstalledDate:     now.AddDate(0, -5, 0),
		ConditionRating:   ConditionNeedsRepair,
		LastInspected:     now.AddDate(0, -1, 0),
		NextInspectionDue: now.AddDate(0, 2, 0),
	}

	for _, a := range []*CampsiteAsset{asset1, asset2, asset3, asset4} {
		if err := mgr.RegisterAsset(a); err != nil {
			t.Fatalf("failed to register asset %s: %v", a.ID, err)
		}
	}

	// 2. Query assets needing repair for cgID
	needingRepair := mgr.ListAssetsNeedingRepair(cgID)
	if len(needingRepair) != 2 {
		t.Fatalf("expected 2 assets needing repair for %s, got %d", cgID, len(needingRepair))
	}
	// Verify sorted by inspection due date: asset3 due sooner than asset2
	if needingRepair[0].ID != asset3.ID {
		t.Fatalf("expected first asset needing repair to be %s (due sooner), got %s", asset3.ID, needingRepair[0].ID)
	}
	if needingRepair[1].ID != asset2.ID {
		t.Fatalf("expected second asset to be %s, got %s", asset2.ID, needingRepair[1].ID)
	}

	// 3. Update asset condition
	repairNote := "Replaced damaged fire ring grate with heavy-duty cast iron"
	if err := mgr.UpdateAssetCondition(asset2.ID, ConditionExcellent, repairNote); err != nil {
		t.Fatalf("failed to update asset condition: %v", err)
	}

	updated, err := mgr.GetAsset(asset2.ID)
	if err != nil {
		t.Fatalf("failed to get asset: %v", err)
	}
	if updated.ConditionRating != ConditionExcellent {
		t.Fatalf("expected condition %s, got %s", ConditionExcellent, updated.ConditionRating)
	}
	if !strings.Contains(updated.Notes, repairNote) {
		t.Fatalf("expected notes to contain repair note, got %s", updated.Notes)
	}

	// 4. Verify asset2 dropped out of needing repair list
	postRepair := mgr.ListAssetsNeedingRepair(cgID)
	if len(postRepair) != 1 || postRepair[0].ID != asset3.ID {
		t.Fatalf("expected only asset3 needing repair after fix, got %d items", len(postRepair))
	}
}

func TestCheckInCheckOutAndHeadcount(t *testing.T) {
	mgr := NewPMSAssetManager()
	now := time.Now().UTC()

	// Party 1: 4 campers, 1 car, 1 trailer
	rec1 := &OccupancyRecord{
		CampsiteID:        "cs-101",
		ReservationID:     "res-alpha-001",
		GuestName:         "Alice Walker",
		PrimaryPlate:      "CA-7XYZ123",
		TrailerPlate:      "TR-9988CA",
		CheckedInAt:       now,
		ScheduledCheckOut: now.Add(48 * time.Hour),
		PartySize:         4,
		EmergencyPhone:    "555-019-2831",
	}

	// Party 2: 2 campers, 1 car, no trailer
	rec2 := &OccupancyRecord{
		CampsiteID:        "cs-102",
		ReservationID:     "res-beta-002",
		GuestName:         "Bob Muir",
		PrimaryPlate:      "NV-4B5991",
		CheckedInAt:       now,
		ScheduledCheckOut: now.Add(24 * time.Hour),
		PartySize:         2,
		EmergencyPhone:    "555-014-9922",
	}

	// Check-in Party 1
	if err := mgr.CheckInOccupant(rec1); err != nil {
		t.Fatalf("failed to check in party 1: %v", err)
	}

	// Double check-in on occupied campsite should fail
	conflictRec := &OccupancyRecord{
		CampsiteID:        "cs-101",
		ReservationID:     "res-conflict-999",
		GuestName:         "Eve Hacker",
		PartySize:         1,
		ScheduledCheckOut: now.Add(24 * time.Hour),
	}
	if err := mgr.CheckInOccupant(conflictRec); err == nil {
		t.Fatal("expected error on duplicate campsite check-in, got nil")
	}

	// Check-in Party 2
	if err := mgr.CheckInOccupant(rec2); err != nil {
		t.Fatalf("failed to check in party 2: %v", err)
	}

	// Verify live headcount: 4 + 2 = 6 campers, 2 cars + 1 trailer = 3 vehicles
	campers, vehicles := mgr.GetTotalInParkHeadcount()
	if campers != 6 {
		t.Fatalf("expected 6 campers, got %d", campers)
	}
	if vehicles != 3 {
		t.Fatalf("expected 3 vehicles, got %d", vehicles)
	}

	// Verify roster
	roster := mgr.GetInParkRoster()
	if len(roster) != 2 {
		t.Fatalf("expected roster of 2, got %d", len(roster))
	}
	if roster[0].CampsiteID != "cs-101" || roster[1].CampsiteID != "cs-102" {
		t.Fatalf("unexpected roster order: %+v", roster)
	}

	// Check out Party 1
	if err := mgr.CheckOutOccupant("cs-101"); err != nil {
		t.Fatalf("failed to check out party 1: %v", err)
	}

	// Headcount after checkout: 2 campers, 1 vehicle
	campersPost, vehiclesPost := mgr.GetTotalInParkHeadcount()
	if campersPost != 2 {
		t.Fatalf("expected 2 campers post checkout, got %d", campersPost)
	}
	if vehiclesPost != 1 {
		t.Fatalf("expected 1 vehicle post checkout, got %d", vehiclesPost)
	}

	// Checking out non-existent or vacated campsite should fail
	if err := mgr.CheckOutOccupant("cs-101"); err == nil {
		t.Fatal("expected error checking out already vacated site, got nil")
	}
}

func TestConcurrentCheckInCheckOut(t *testing.T) {
	mgr := NewPMSAssetManager()
	now := time.Now().UTC()

	const sitesCount = 50

	// 50 concurrent check-ins
	var checkInWg sync.WaitGroup
	checkInWg.Add(sitesCount)

	for i := 0; i < sitesCount; i++ {
		idx := i
		go func() {
			defer checkInWg.Done()
			rec := &OccupancyRecord{
				CampsiteID:        fmt.Sprintf("cs-conc-%03d", idx),
				ReservationID:     fmt.Sprintf("res-conc-%03d", idx),
				GuestName:         fmt.Sprintf("Camper %d", idx),
				PrimaryPlate:      fmt.Sprintf("LIC-%03d", idx),
				CheckedInAt:       now,
				ScheduledCheckOut: now.Add(48 * time.Hour),
				PartySize:         2,
				EmergencyPhone:    "555-010-0000",
			}
			if err := mgr.CheckInOccupant(rec); err != nil {
				t.Errorf("concurrent check-in failed for site %d: %v", idx, err)
			}
		}()
	}
	checkInWg.Wait()

	// Verify headcount: 50 * 2 = 100 campers, 50 vehicles
	campers, vehicles := mgr.GetTotalInParkHeadcount()
	if campers != 100 {
		t.Fatalf("expected 100 campers, got %d", campers)
	}
	if vehicles != 50 {
		t.Fatalf("expected 50 vehicles, got %d", vehicles)
	}

	// Check out half of the sites concurrently (first 25)
	const checkoutCount = 25
	var checkOutWg sync.WaitGroup
	checkOutWg.Add(checkoutCount)

	for i := 0; i < checkoutCount; i++ {
		idx := i
		go func() {
			defer checkOutWg.Done()
			siteID := fmt.Sprintf("cs-conc-%03d", idx)
			if err := mgr.CheckOutOccupant(siteID); err != nil {
				t.Errorf("concurrent checkout failed for %s: %v", siteID, err)
			}
		}()
	}
	checkOutWg.Wait()

	// Verify remaining headcount: 25 * 2 = 50 campers, 25 vehicles
	campersPost, vehiclesPost := mgr.GetTotalInParkHeadcount()
	if campersPost != 50 {
		t.Fatalf("expected 50 campers post checkout, got %d", campersPost)
	}
	if vehiclesPost != 25 {
		t.Fatalf("expected 25 vehicles post checkout, got %d", vehiclesPost)
	}

	roster := mgr.GetInParkRoster()
	if len(roster) != 25 {
		t.Fatalf("expected 25 active sites in roster, got %d", len(roster))
	}
}

