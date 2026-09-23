package profile

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProfileCreationAndPhoneValidation(t *testing.T) {
	mgr := NewProfileManager()

	// 1. Phone validation failure cases
	invalidPhones := []struct {
		name  string
		phone string
	}{
		{"empty", ""},
		{"too_short", "123456"},
		{"letters", "555-GET-AWAY"},
		{"invalid_plus_position", "555+1234567"},
		{"symbols", "555#123@4567"},
	}

	for _, tc := range invalidPhones {
		t.Run("invalid_phone_"+tc.name, func(t *testing.T) {
			p := &CamperProfile{
				UserID:   "usr-bad-phone",
				FullName: "Jane Doe",
				Phone:    tc.phone,
			}
			err := mgr.UpsertProfile(p)
			if err == nil {
				t.Fatalf("expected error for invalid phone %q, got nil", tc.phone)
			}
		})
	}

	// 2. Empty user ID validation failure
	err := mgr.UpsertProfile(&CamperProfile{
		UserID: "",
		Phone:  "+15551234567",
	})
	if err == nil {
		t.Fatal("expected error for empty user_id, got nil")
	}

	// 3. Valid profile creation with multiple phone formats
	validPhones := []string{
		"+15551234567",
		"555-123-4567",
		"(555) 123-4567",
		"555.123.4567",
		"+44 20 7123 4567",
	}

	for i, phone := range validPhones {
		uid := fmt.Sprintf("usr-%d", i)
		p := &CamperProfile{
			UserID:                uid,
			FullName:              fmt.Sprintf("Camper %d", i),
			Phone:                 phone,
			EmergencyContactName:  "Emergency Contact",
			EmergencyContactPhone: "+15559876543",
			WildernessPassID:      fmt.Sprintf("PASS-2026-%d", i),
			NotificationsEnabled:  true,
		}
		if err := mgr.UpsertProfile(p); err != nil {
			t.Fatalf("failed to upsert profile with valid phone %q: %v", phone, err)
		}

		retrieved, err := mgr.GetProfile(uid)
		if err != nil {
			t.Fatalf("failed to retrieve profile %q: %v", uid, err)
		}
		if retrieved.FullName != p.FullName {
			t.Errorf("expected FullName %q, got %q", p.FullName, retrieved.FullName)
		}
		if retrieved.CreatedAt.IsZero() {
			t.Errorf("expected non-zero CreatedAt")
		}
		if retrieved.UpdatedAt.IsZero() {
			t.Errorf("expected non-zero UpdatedAt")
		}
	}

	// 4. Non-existent profile retrieval returns error
	_, err = mgr.GetProfile("non-existent-user")
	if err == nil {
		t.Fatal("expected error retrieving non-existent user, got nil")
	}
}

func TestAddVehicleAndPrimaryFlag(t *testing.T) {
	mgr := NewProfileManager()

	userID := "usr-fleet-01"
	p := &CamperProfile{
		UserID:   userID,
		FullName: "Alex Honnold",
		Phone:    "+15559098080",
	}
	if err := mgr.UpsertProfile(p); err != nil {
		t.Fatalf("failed to create camper profile: %v", err)
	}

	// 1. Add primary vehicle (van)
	v1 := VehicleRecord{
		Plate:     "ca-van01",
		State:     "ca",
		MakeModel: "Mercedes Sprinter 4x4",
		Color:     "Pebble Gray",
		IsEV:      false,
		IsPrimary: true,
	}
	if err := mgr.AddVehicle(userID, v1); err != nil {
		t.Fatalf("failed to add primary vehicle: %v", err)
	}

	vehicles := mgr.GetVehicles(userID)
	if len(vehicles) != 1 {
		t.Fatalf("expected 1 vehicle, got %d", len(vehicles))
	}
	if vehicles[0].Plate != "CA-VAN01" {
		t.Errorf("expected plate CA-VAN01, got %s", vehicles[0].Plate)
	}
	if vehicles[0].State != "CA" {
		t.Errorf("expected state CA, got %s", vehicles[0].State)
	}
	if !vehicles[0].IsPrimary {
		t.Errorf("expected vehicle to be primary")
	}

	// 2. Add towed trailer vehicle (secondary)
	v2 := VehicleRecord{
		Plate:     "trl-9988",
		State:     "ca",
		MakeModel: "Airstream Basecamp 20X",
		Color:     "Silver",
		IsEV:      false,
		IsPrimary: false,
	}
	if err := mgr.AddVehicle(userID, v2); err != nil {
		t.Fatalf("failed to add trailer vehicle: %v", err)
	}

	vehicles = mgr.GetVehicles(userID)
	if len(vehicles) != 2 {
		t.Fatalf("expected 2 vehicles, got %d", len(vehicles))
	}
	if !vehicles[0].IsPrimary {
		t.Errorf("expected vehicle 0 to remain primary")
	}
	if vehicles[1].IsPrimary {
		t.Errorf("expected trailer vehicle 1 to NOT be primary")
	}

	// 3. Add third vehicle marked as primary -> previous primary must be demoted
	v3 := VehicleRecord{
		Plate:     "ev-rivian",
		State:     "ca",
		MakeModel: "Rivian R1T",
		Color:     "Forest Green",
		IsEV:      true,
		IsPrimary: true,
	}
	if err := mgr.AddVehicle(userID, v3); err != nil {
		t.Fatalf("failed to add new primary vehicle: %v", err)
	}

	vehicles = mgr.GetVehicles(userID)
	if len(vehicles) != 3 {
		t.Fatalf("expected 3 vehicles, got %d", len(vehicles))
	}
	if vehicles[0].IsPrimary {
		t.Errorf("expected vehicle 0 to be demoted from primary")
	}
	if vehicles[1].IsPrimary {
		t.Errorf("expected vehicle 1 to NOT be primary")
	}
	if !vehicles[2].IsPrimary {
		t.Errorf("expected vehicle 2 to be primary")
	}
}

func TestVehicleDeduplication(t *testing.T) {
	mgr := NewProfileManager()

	userID := "usr-dedup-01"
	p := &CamperProfile{
		UserID:   userID,
		FullName: "Tommy Caldwell",
		Phone:    "+15553034040",
	}
	if err := mgr.UpsertProfile(p); err != nil {
		t.Fatalf("failed to create camper profile: %v", err)
	}

	v := VehicleRecord{
		Plate:     "7XYZ987",
		State:     "CA",
		MakeModel: "Subaru Outback",
		Color:     "Blue",
	}
	if err := mgr.AddVehicle(userID, v); err != nil {
		t.Fatalf("failed to add initial vehicle: %v", err)
	}

	// Attempt to add duplicate plate & state with different casing
	duplicate := VehicleRecord{
		Plate:     "7xyz987",
		State:     "ca",
		MakeModel: "Subaru Outback 2.5i",
		Color:     "Blue",
	}
	err := mgr.AddVehicle(userID, duplicate)
	if err == nil {
		t.Fatal("expected duplicate vehicle error, got nil")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Adding same plate for a DIFFERENT user is allowed
	user2 := "usr-dedup-02"
	p2 := &CamperProfile{
		UserID:   user2,
		FullName: "Beth Rodden",
		Phone:    "+15553034041",
	}
	if err := mgr.UpsertProfile(p2); err != nil {
		t.Fatalf("failed to create profile 2: %v", err)
	}
	if err := mgr.AddVehicle(user2, v); err != nil {
		t.Fatalf("expected different user to register vehicle: %v", err)
	}
}

func TestRemoveVehicle(t *testing.T) {
	mgr := NewProfileManager()

	userID := "usr-rm-01"
	p := &CamperProfile{
		UserID:   userID,
		FullName: "Lynn Hill",
		Phone:    "+15558889999",
	}
	if err := mgr.UpsertProfile(p); err != nil {
		t.Fatalf("failed to create profile: %v", err)
	}

	mgr.AddVehicle(userID, VehicleRecord{Plate: "P1", State: "CA", IsPrimary: true})
	mgr.AddVehicle(userID, VehicleRecord{Plate: "P2", State: "CA", IsPrimary: false})
	mgr.AddVehicle(userID, VehicleRecord{Plate: "P3", State: "CA", IsPrimary: false})

	// Remove non-existent vehicle
	err := mgr.RemoveVehicle(userID, "NONEXIST", "CA")
	if err == nil {
		t.Fatal("expected error removing non-existent vehicle, got nil")
	}

	// Remove primary vehicle P1 -> P2 should become primary
	if err := mgr.RemoveVehicle(userID, "p1", "ca"); err != nil {
		t.Fatalf("failed to remove vehicle P1: %v", err)
	}

	vehicles := mgr.GetVehicles(userID)
	if len(vehicles) != 2 {
		t.Fatalf("expected 2 vehicles remaining, got %d", len(vehicles))
	}
	if vehicles[0].Plate != "P2" || !vehicles[0].IsPrimary {
		t.Errorf("expected P2 to be promoted to primary, got %+v", vehicles[0])
	}
}

func TestConcurrentFleetQueries(t *testing.T) {
	mgr := NewProfileManager()

	// Seed 10 camper profiles
	for i := 0; i < 10; i++ {
		uid := fmt.Sprintf("concurrent-usr-%d", i)
		err := mgr.UpsertProfile(&CamperProfile{
			UserID:   uid,
			FullName: fmt.Sprintf("Concurrent User %d", i),
			Phone:    fmt.Sprintf("+1555000%04d", i),
			Vehicles: []VehicleRecord{
				{
					Plate:     fmt.Sprintf("BASE%d", i),
					State:     "CA",
					MakeModel: "Toyota Tacoma",
					IsPrimary: true,
				},
			},
		})
		if err != nil {
			t.Fatalf("failed seeding profile %d: %v", i, err)
		}
	}

	// Run 50 concurrent goroutines querying and modifying fleet records
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			uid := fmt.Sprintf("concurrent-usr-%d", gid%10)

			// 1. Read profile
			prof, err := mgr.GetProfile(uid)
			if err != nil || prof == nil {
				t.Errorf("goroutine %d failed to get profile: %v", gid, err)
				return
			}

			// 2. Query vehicles
			vehicles := mgr.GetVehicles(uid)
			if len(vehicles) == 0 {
				t.Errorf("goroutine %d expected vehicles for user %s", gid, uid)
				return
			}

			// 3. Add a temporary vehicle
			tempPlate := fmt.Sprintf("G%02d-%04d", gid, time.Now().UnixNano()%10000)
			err = mgr.AddVehicle(uid, VehicleRecord{
				Plate:     tempPlate,
				State:     "NV",
				MakeModel: "Subaru Crosstrek",
				IsPrimary: false,
			})
			if err != nil {
				t.Errorf("goroutine %d failed adding vehicle: %v", gid, err)
				return
			}

			// 4. Verify added vehicle exists
			updatedVehicles := mgr.GetVehicles(uid)
			found := false
			for _, v := range updatedVehicles {
				if v.Plate == strings.ToUpper(tempPlate) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("goroutine %d did not find newly added vehicle %s", gid, tempPlate)
			}
		}(g)
	}

	wg.Wait()
}

