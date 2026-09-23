package sos

import (
	"testing"
)

func TestSOSManagerLifecycle(t *testing.T) {
	mgr := NewSOSManager()

	// 1. Trigger beacon
	beacon, err := mgr.TriggerBeacon(
		"u-camper-1",
		"John Hiker",
		"Granite Peak Pass Mile 7",
		44.1800,
		-114.9300,
		DistressMedical,
		"Severe hypothermia and sprained ankle, unable to descend",
	)
	if err != nil {
		t.Fatalf("unexpected error triggering beacon: %v", err)
	}
	if beacon.Status != StatusActiveSearching {
		t.Errorf("expected status %v, got %v", StatusActiveSearching, beacon.Status)
	}

	// 2. Query active beacons
	active := mgr.ListActive()
	if len(active) != 1 {
		t.Fatalf("expected 1 active beacon, got %d", len(active))
	}
	if active[0].ID != beacon.ID {
		t.Errorf("expected active beacon ID %s, got %s", beacon.ID, active[0].ID)
	}

	// 3. Dispatch Ranger
	dispatched, err := mgr.AssignRanger(beacon.ID, "Ranger Sarah Connor")
	if err != nil {
		t.Fatalf("unexpected error dispatching ranger: %v", err)
	}
	if dispatched.Status != StatusDispatched || dispatched.AssignedRanger != "Ranger Sarah Connor" {
		t.Errorf("unexpected dispatched state: %+v", dispatched)
	}

	// 4. Resolve Beacon
	resolved, err := mgr.ResolveBeacon(beacon.ID)
	if err != nil {
		t.Fatalf("unexpected error resolving beacon: %v", err)
	}
	if resolved.Status != StatusResolved || resolved.ResolvedAt == nil {
		t.Errorf("expected resolved status with timestamp, got %+v", resolved)
	}

	// 5. Active list should now be empty
	if len(mgr.ListActive()) != 0 {
		t.Errorf("expected 0 active beacons after resolution, got %d", len(mgr.ListActive()))
	}
}
