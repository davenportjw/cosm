package ranger

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWorkOrderLifecycle(t *testing.T) {
	hub := NewEventHub()
	pms := NewPMSManager(hub)

	// Subscribe to event stream to verify SSE broadcast events
	sseCh := hub.Subscribe()
	defer hub.Unsubscribe(sseCh)

	// 1. Validation checks
	_, err := pms.CreateWorkOrder("cg-yosemite", "site-01", "INVALID_CAT", "HIGH", "Broken latch")
	if err == nil {
		t.Fatal("expected error for invalid category, got nil")
	}

	_, err = pms.CreateWorkOrder("cg-yosemite", "site-01", "BEAR_BOX", "SUPER_CRITICAL", "Broken latch")
	if err == nil {
		t.Fatal("expected error for invalid severity, got nil")
	}

	_, err = pms.CreateWorkOrder("", "site-01", "BEAR_BOX", "HIGH", "Broken latch")
	if err == nil {
		t.Fatal("expected error for empty campgroundID, got nil")
	}

	// 2. Create work orders
	wo1, err := pms.CreateWorkOrder("cg-yosemite", "site-upper-pines-04", "BEAR_BOX", "HIGH", "Bear box latch mechanism jammed")
	if err != nil {
		t.Fatalf("failed to create work order 1: %v", err)
	}
	if wo1.ID == "" || !strings.HasPrefix(wo1.ID, "wo-") {
		t.Errorf("unexpected work order ID: %s", wo1.ID)
	}
	if wo1.Status != StatusPending {
		t.Errorf("expected status PENDING, got %s", wo1.Status)
	}
	if wo1.ResolvedAt != nil {
		t.Errorf("expected nil ResolvedAt on new work order, got %v", wo1.ResolvedAt)
	}

	// Drain create broadcast
	select {
	case msg := <-sseCh:
		if !strings.Contains(msg, "work_order_created") || !strings.Contains(msg, wo1.ID) {
			t.Errorf("unexpected SSE broadcast for create: %s", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for create broadcast")
	}

	wo2, err := pms.CreateWorkOrder("cg-yosemite", "site-lower-pines-12", "WATER_SPIGOT", "MEDIUM", "Spigot leaking into drainage channel")
	if err != nil {
		t.Fatalf("failed to create work order 2: %v", err)
	}
	if wo2.ID == "" {
		t.Error("expected non-empty ID for wo2")
	}

	// Drain wo2 create broadcast
	select {
	case <-sseCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for wo2 broadcast")
	}

	wo3, err := pms.CreateWorkOrder("cg-tuolumne", "site-tm-08", "TREE_HAZARD", "CRITICAL", "Heavy branch suspended above tent pad")
	if err != nil {
		t.Fatalf("failed to create work order 3: %v", err)
	}
	if wo3.ID == "" {
		t.Error("expected non-empty ID for wo3")
	}

	// Drain wo3 create broadcast
	select {
	case <-sseCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for wo3 broadcast")
	}

	// 3. List work orders
	yosemiteOrders := pms.ListWorkOrders("cg-yosemite")
	if len(yosemiteOrders) != 2 {
		t.Fatalf("expected 2 work orders for cg-yosemite, got %d", len(yosemiteOrders))
	}

	tuolumneOrders := pms.ListWorkOrders("cg-tuolumne")
	if len(tuolumneOrders) != 1 {
		t.Fatalf("expected 1 work order for cg-tuolumne, got %d", len(tuolumneOrders))
	}

	allOrders := pms.ListWorkOrders("")
	if len(allOrders) != 3 {
		t.Fatalf("expected 3 total work orders, got %d", len(allOrders))
	}

	// 4. Update status -> DISPATCHED
	dispatched, err := pms.UpdateWorkOrderStatus(wo1.ID, StatusDispatched, "ranger-sarah-m")
	if err != nil {
		t.Fatalf("failed to update work order to DISPATCHED: %v", err)
	}
	if dispatched.Status != StatusDispatched {
		t.Errorf("expected status DISPATCHED, got %s", dispatched.Status)
	}
	if dispatched.AssignedRanger != "ranger-sarah-m" {
		t.Errorf("expected assigned ranger 'ranger-sarah-m', got %s", dispatched.AssignedRanger)
	}
	if dispatched.ResolvedAt != nil {
		t.Errorf("expected nil ResolvedAt when dispatched, got %v", dispatched.ResolvedAt)
	}

	// Drain dispatch broadcast
	select {
	case msg := <-sseCh:
		if !strings.Contains(msg, "work_order_updated") || !strings.Contains(msg, "DISPATCHED") {
			t.Errorf("unexpected SSE broadcast for dispatch: %s", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for dispatch broadcast")
	}

	// 5. Update status -> RESOLVED
	resolved, err := pms.UpdateWorkOrderStatus(wo1.ID, StatusResolved, "ranger-sarah-m")
	if err != nil {
		t.Fatalf("failed to update work order to RESOLVED: %v", err)
	}
	if resolved.Status != StatusResolved {
		t.Errorf("expected status RESOLVED, got %s", resolved.Status)
	}
	if resolved.ResolvedAt == nil {
		t.Fatal("expected non-nil ResolvedAt when resolved, got nil")
	}

	// Drain resolve broadcast
	select {
	case msg := <-sseCh:
		if !strings.Contains(msg, "work_order_updated") || !strings.Contains(msg, "RESOLVED") {
			t.Errorf("unexpected SSE broadcast for resolve: %s", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for resolve broadcast")
	}

	// 6. Non-existent work order check
	_, err = pms.UpdateWorkOrderStatus("non-existent-id", StatusResolved, "ranger-sarah-m")
	if err == nil {
		t.Fatal("expected error updating non-existent work order, got nil")
	}
}

func TestVehicleGateAuthorization(t *testing.T) {
	pms := NewPMSManager(nil)
	now := time.Now()

	// 1. Register valid vehicle active now
	pms.RegisterVehicle("7XYZ987", "CA", "site-upper-pines-04", "John Muir", now.Add(-2*time.Hour), now.Add(24*time.Hour))

	// 2. Register expired vehicle
	pms.RegisterVehicle("EXPIRED1", "NV", "site-lower-pines-12", "Alex Honnold", now.Add(-48*time.Hour), now.Add(-2*time.Hour))

	// 3. Register future vehicle (not yet checked in)
	pms.RegisterVehicle("FUTURE44", "OR", "site-tm-08", "Galen Rowell", now.Add(4*time.Hour), now.Add(48*time.Hour))

	// Test case A: Valid vehicle within date range -> returns true
	auth, reason := pms.AuthorizeGateEntry("7XYZ987", "CA")
	if !auth {
		t.Fatalf("expected authorized entry for valid vehicle, got false (reason: %s)", reason)
	}
	if !strings.Contains(reason, "site-upper-pines-04") || !strings.Contains(reason, "John Muir") {
		t.Errorf("unexpected reason payload: %s", reason)
	}

	// Test case B: Case-insensitivity support
	authCase, reasonCase := pms.AuthorizeGateEntry("7xyz987", "ca")
	if !authCase {
		t.Fatalf("expected case-insensitive authorized entry, got false (reason: %s)", reasonCase)
	}

	// Test case C: Expired plate -> returns false
	authExp, reasonExp := pms.AuthorizeGateEntry("EXPIRED1", "NV")
	if authExp {
		t.Fatalf("expected unauthorized entry for expired vehicle, got true (reason: %s)", reasonExp)
	}
	if !strings.Contains(reasonExp, "expired") {
		t.Errorf("expected expired message in reason, got: %s", reasonExp)
	}

	// Test case D: Future reservation -> returns false
	authFut, reasonFut := pms.AuthorizeGateEntry("FUTURE44", "OR")
	if authFut {
		t.Fatalf("expected unauthorized entry for future vehicle, got true (reason: %s)", reasonFut)
	}
	if !strings.Contains(reasonFut, "not yet active") {
		t.Errorf("expected not yet active message in reason, got: %s", reasonFut)
	}

	// Test case E: Unknown plate -> returns false
	authUnk, reasonUnk := pms.AuthorizeGateEntry("UNKNOWN9", "WA")
	if authUnk {
		t.Fatalf("expected unauthorized entry for unknown vehicle, got true (reason: %s)", reasonUnk)
	}
	if !strings.Contains(reasonUnk, "not registered") {
		t.Errorf("expected not registered message in reason, got: %s", reasonUnk)
	}
}

func TestEventHubBroadcast(t *testing.T) {
	hub := NewEventHub()

	sub1 := hub.Subscribe()
	sub2 := hub.Subscribe()

	if count := hub.SubscriberCount(); count != 2 {
		t.Fatalf("expected 2 subscribers, got %d", count)
	}

	event := "BEAR_SIGHTING"
	payload := `{"location":"Camp 4","behavior":"foraging"}`
	hub.Broadcast(event, payload)

	expectedPrefix := "event: BEAR_SIGHTING\ndata: {\"location\":\"Camp 4\",\"behavior\":\"foraging\"}\n\n"

	// Verify sub1 received
	select {
	case msg1 := <-sub1:
		if msg1 != expectedPrefix {
			t.Errorf("sub1 got %q, expected %q", msg1, expectedPrefix)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("sub1 timed out waiting for event")
	}

	// Verify sub2 received
	select {
	case msg2 := <-sub2:
		if msg2 != expectedPrefix {
			t.Errorf("sub2 got %q, expected %q", msg2, expectedPrefix)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("sub2 timed out waiting for event")
	}

	// Unsubscribe sub1 and verify channel closure
	hub.Unsubscribe(sub1)
	if count := hub.SubscriberCount(); count != 1 {
		t.Fatalf("expected 1 subscriber after unsubscribe, got %d", count)
	}

	// Verify sub1 channel is closed
	select {
	case _, ok := <-sub1:
		if ok {
			t.Error("expected closed channel for sub1")
		}
	default:
		// Try reading again to confirm closed
		_, ok := <-sub1
		if ok {
			t.Error("expected closed channel for sub1")
		}
	}

	// Broadcast again, only sub2 receives
	hub.Broadcast("TRAIL_CLEAR", "Loop C reopened")
	select {
	case msg := <-sub2:
		if !strings.Contains(msg, "TRAIL_CLEAR") {
			t.Errorf("sub2 received unexpected message: %s", msg)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("sub2 timed out waiting for second event")
	}

	hub.Unsubscribe(sub2)
}

func TestMultiLaneGateAuthorization(t *testing.T) {
	hub := NewEventHub()
	pms := NewPMSManager(hub)
	now := time.Now()

	// Register vehicle with primary tow vehicle + secondary trailer plate
	pms.RegisterVehicleExtended(
		"TRUCK-99", "CA", "site-rv-10", "Alex Honnold",
		"Ford F-250 Super Duty", "TRAILER-77", "res-rv-001",
		now.Add(-2*time.Hour), now.Add(48*time.Hour),
	)

	sseCh := hub.Subscribe()
	defer hub.Unsubscribe(sseCh)

	// 1. Primary plate in Standard Lane -> Approved
	entry1, auth1, reason1 := pms.AuthorizeGateEntryWithLane("TRUCK-99", "CA", LaneStandard)
	if !auth1 {
		t.Fatalf("expected authorized primary vehicle in standard lane, got false: %s", reason1)
	}
	if entry1.BarrierAction != "BARRIER_RAISED" {
		t.Errorf("expected barrier action BARRIER_RAISED, got %s", entry1.BarrierAction)
	}
	if entry1.Lane != LaneStandard {
		t.Errorf("expected lane %s, got %s", LaneStandard, entry1.Lane)
	}

	// Verify SSE broadcast for gate scan
	select {
	case msg := <-sseCh:
		if !strings.Contains(msg, "gate_scan_event") || !strings.Contains(msg, "TRUCK-99") {
			t.Errorf("unexpected SSE broadcast: %s", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for gate_scan_event broadcast")
	}

	// 2. Secondary trailer plate in Standard Lane -> Rejected
	entry2, auth2, reason2 := pms.AuthorizeGateEntryWithLane("TRAILER-77", "CA", LaneStandard)
	if auth2 {
		t.Fatalf("expected trailer plate in standard lane to be rejected, got authorized: %s", reason2)
	}
	if entry2.BarrierAction != "BARRIER_LOCKED" {
		t.Errorf("expected barrier action BARRIER_LOCKED, got %s", entry2.BarrierAction)
	}

	// 3. Secondary trailer plate in Oversize Trailer Lane -> Approved
	entry3, auth3, reason3 := pms.AuthorizeGateEntryWithLane("TRAILER-77", "CA", LaneTrailerOversize)
	if !auth3 {
		t.Fatalf("expected authorized trailer in oversize lane, got false: %s", reason3)
	}
	if entry3.BarrierAction != "BARRIER_RAISED" {
		t.Errorf("expected barrier action BARRIER_RAISED, got %s", entry3.BarrierAction)
	}
	if entry3.CampsiteID != "site-rv-10" {
		t.Errorf("expected campsite site-rv-10, got %s", entry3.CampsiteID)
	}

	// 4. Primary plate in Oversize Trailer Lane -> Also Approved
	entry4, auth4, reason4 := pms.AuthorizeGateEntryWithLane("TRUCK-99", "CA", LaneTrailerOversize)
	if !auth4 {
		t.Fatalf("expected authorized primary truck in oversize lane, got false: %s", reason4)
	}
	if entry4.BarrierAction != "BARRIER_RAISED" {
		t.Errorf("expected barrier action BARRIER_RAISED, got %s", entry4.BarrierAction)
	}

	// Check recent gate logs
	logs := pms.GetRecentGateLogs(10)
	if len(logs) < 4 {
		t.Fatalf("expected at least 4 gate logs, got %d", len(logs))
	}
	// logs[0] is newest (entry4)
	if logs[0].Plate != "TRUCK-99" || logs[0].Lane != LaneTrailerOversize {
		t.Errorf("expected latest log to be truck-99 on oversize lane, got %+v", logs[0])
	}
}

func TestExpiredReservationRejection(t *testing.T) {
	pms := NewPMSManager(nil)
	now := time.Now()

	// 1. Expired reservation
	pms.RegisterVehicleExtended(
		"EXP-PAST", "NV", "site-05", "Past Camper",
		"Toyota RAV4", "", "res-past",
		now.Add(-72*time.Hour), now.Add(-1*time.Hour),
	)

	entryExp, authExp, reasonExp := pms.AuthorizeGateEntryWithLane("EXP-PAST", "NV", LaneStandard)
	if authExp {
		t.Fatalf("expected expired vehicle rejection, got authorized: %s", reasonExp)
	}
	if !strings.Contains(reasonExp, "expired") {
		t.Errorf("expected reason to contain 'expired', got %s", reasonExp)
	}
	if entryExp.BarrierAction != "BARRIER_LOCKED" {
		t.Errorf("expected BARRIER_LOCKED, got %s", entryExp.BarrierAction)
	}
	if pms.GateController.State() != BarrierLockout {
		t.Errorf("expected barrier state %s, got %s", BarrierLockout, pms.GateController.State())
	}

	// 2. Future reservation
	pms.RegisterVehicleExtended(
		"FUT-EARLY", "OR", "site-06", "Early Camper",
		"Subaru Forester", "", "res-fut",
		now.Add(5*time.Hour), now.Add(48*time.Hour),
	)

	_, authFut, reasonFut := pms.AuthorizeGateEntryWithLane("FUT-EARLY", "OR", LaneStandard)
	if authFut {
		t.Fatalf("expected future vehicle rejection, got authorized: %s", reasonFut)
	}
	if !strings.Contains(reasonFut, "not yet active") {
		t.Errorf("expected reason to contain 'not yet active', got %s", reasonFut)
	}

	// 3. Revoked authorization
	pms.RegisterVehicleAuthorization(&VehicleAuthorization{
		LicensePlate: "REVOKED-1",
		State:        "CA",
		CampsiteID:   "site-07",
		GuestName:    "Revoked Camper",
		CheckIn:      now.Add(-2 * time.Hour),
		CheckOut:     now.Add(24 * time.Hour),
		Authorized:   false,
	})

	_, authRev, reasonRev := pms.AuthorizeGateEntryWithLane("REVOKED-1", "CA", LaneStandard)
	if authRev {
		t.Fatalf("expected revoked authorization rejection, got authorized: %s", reasonRev)
	}
	if !strings.Contains(reasonRev, "revoked") {
		t.Errorf("expected reason to contain 'revoked', got %s", reasonRev)
	}
}

func TestBarrierStateTransitionsAndAutoReset(t *testing.T) {
	pms := NewPMSManager(nil)
	now := time.Now()

	// Initial barrier state
	if pms.GateController.State() != BarrierLowered {
		t.Fatalf("expected initial barrier state %s, got %s", BarrierLowered, pms.GateController.State())
	}

	// Default reset duration is 6 seconds
	if pms.GateController.resetDuration != 6*time.Second {
		t.Errorf("expected default 6s reset duration, got %v", pms.GateController.resetDuration)
	}

	// Set short reset duration for fast auto-reset test
	pms.GateController.SetResetDuration(60 * time.Millisecond)

	pms.RegisterVehicle("ACTIVE-01", "CA", "site-01", "Active Camper", now.Add(-1*time.Hour), now.Add(24*time.Hour))

	// Authorize vehicle -> barrier transitions to OPEN
	_, auth, _ := pms.AuthorizeGateEntryWithLane("ACTIVE-01", "CA", LaneStandard)
	if !auth {
		t.Fatal("expected vehicle to be authorized")
	}
	if pms.GateController.State() != BarrierOpen {
		t.Fatalf("expected barrier state %s after authorization, got %s", BarrierOpen, pms.GateController.State())
	}

	// Wait for auto-clearance reset timer to fire
	time.Sleep(90 * time.Millisecond)
	if pms.GateController.State() != BarrierLowered {
		t.Fatalf("expected barrier state %s after auto-reset timer, got %s", BarrierLowered, pms.GateController.State())
	}

	// Unauthorized scan -> transitions to LOCKOUT
	_, authBad, _ := pms.AuthorizeGateEntryWithLane("INTRUDER-9", "XX", LaneStandard)
	if authBad {
		t.Fatal("expected intruder rejection")
	}
	if pms.GateController.State() != BarrierLockout {
		t.Fatalf("expected barrier state %s on unauthorized attempt, got %s", BarrierLockout, pms.GateController.State())
	}

	// Manual reset barrier -> resets to LOWERED
	resetState := pms.ResetBarrier()
	if resetState != BarrierLowered || pms.GateController.State() != BarrierLowered {
		t.Fatalf("expected barrier state %s after ResetBarrier, got %s", BarrierLowered, resetState)
	}
}

func TestConcurrentGateScans(t *testing.T) {
	pms := NewPMSManager(nil)
	now := time.Now()

	// Seed 5 registered vehicles
	for i := 0; i < 5; i++ {
		pms.RegisterVehicleExtended(
			fmt.Sprintf("FLEET-%d", i), "CA",
			fmt.Sprintf("site-%02d", i), fmt.Sprintf("Guest %d", i),
			"Van", fmt.Sprintf("TRAIL-%d", i), fmt.Sprintf("res-%d", i),
			now.Add(-2*time.Hour), now.Add(24*time.Hour),
		)
	}

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	lanes := []string{LaneStandard, LaneTrailerOversize, LaneEmergencyRanger}

	for g := 0; g < numGoroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			lane := lanes[gid%len(lanes)]

			// Alternate between primary plate, secondary trailer plate, and unknown plate
			var plate string
			if gid%3 == 0 {
				plate = fmt.Sprintf("FLEET-%d", gid%5)
			} else if gid%3 == 1 {
				plate = fmt.Sprintf("TRAIL-%d", gid%5)
			} else {
				plate = fmt.Sprintf("UNKNOWN-%d", gid)
			}

			entry, _, _ := pms.AuthorizeGateEntryWithLane(plate, "CA", lane)
			if entry == nil || entry.ID == "" {
				t.Errorf("goroutine %d: expected non-nil gate entry", gid)
			}
		}(g)
	}

	wg.Wait()

	// Verify all 50 gate logs recorded
	recentLogs := pms.GetRecentGateLogs(100)
	if len(recentLogs) != numGoroutines {
		t.Fatalf("expected %d gate logs, got %d", numGoroutines, len(recentLogs))
	}

	// Verify at least one vehicle recorded multiple gate entries
	pms.mu.RLock()
	totalEntries := 0
	for _, auth := range pms.vehicleAuthorizations {
		totalEntries += auth.GateEntryCount
		if auth.GateEntryCount > 0 && auth.LastEntryAt == nil {
			t.Errorf("expected LastEntryAt to be non-nil for vehicle with entries")
		}
	}
	pms.mu.RUnlock()

	if totalEntries == 0 {
		t.Error("expected non-zero total gate entries among registered vehicles")
	}
}

