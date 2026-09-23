package incident

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIncidentLifecycle(t *testing.T) {
	mgr := NewICSManager()

	// 1. Subscribe to SSE events
	ch := mgr.Subscribe()
	defer mgr.Unsubscribe(ch)

	// 2. Report incident
	inc, err := mgr.ReportIncident(
		IncidentBearSighting,
		SeverityImmediateEvacuation,
		"cg-yosemite-upper-pines",
		"Loop-B",
		"GPS:37.7401,-119.5672",
		"Aggressive sow with two cubs raiding campsites",
	)
	if err != nil {
		t.Fatalf("failed to report incident: %v", err)
	}
	if inc.ID == "" {
		t.Fatal("expected non-empty incident ID")
	}
	if inc.Status != StatusReported {
		t.Fatalf("expected status %s, got %s", StatusReported, inc.Status)
	}

	// Verify SSE broadcast for reported
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "event: incident_reported") || !strings.Contains(msg, inc.ID) {
			t.Fatalf("unexpected SSE broadcast: %s", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for incident_reported event")
	}

	// 3. Dispatch Incident
	rangerID := "ranger-smith-404"
	if err := mgr.DispatchIncident(inc.ID, rangerID); err != nil {
		t.Fatalf("failed to dispatch incident: %v", err)
	}
	updated, err := mgr.GetIncident(inc.ID)
	if err != nil {
		t.Fatalf("failed to get incident: %v", err)
	}
	if updated.Status != StatusDispatched {
		t.Fatalf("expected status %s, got %s", StatusDispatched, updated.Status)
	}
	if updated.AssignedRanger != rangerID {
		t.Fatalf("expected ranger %s, got %s", rangerID, updated.AssignedRanger)
	}

	// Verify SSE broadcast for dispatched
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "event: incident_dispatched") || !strings.Contains(msg, rangerID) {
			t.Fatalf("unexpected SSE broadcast: %s", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for incident_dispatched event")
	}

	// 4. Contain Incident
	containNotes := "Bear hazing round deployed; animals retreated across Merced River"
	if err := mgr.ContainIncident(inc.ID, containNotes); err != nil {
		t.Fatalf("failed to contain incident: %v", err)
	}
	updated, err = mgr.GetIncident(inc.ID)
	if err != nil {
		t.Fatalf("failed to get incident: %v", err)
	}
	if updated.Status != StatusContained {
		t.Fatalf("expected status %s, got %s", StatusContained, updated.Status)
	}
	if !strings.Contains(updated.Notes, containNotes) {
		t.Fatalf("expected notes to contain %q, got %q", containNotes, updated.Notes)
	}

	// Verify SSE broadcast for contained
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "event: incident_contained") {
			t.Fatalf("unexpected SSE broadcast: %s", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for incident_contained event")
	}

	// 5. Resolve Incident
	resolveNotes := "Perimeter monitored for 2 hours with no sightings. All campers accounted for."
	if err := mgr.ResolveIncident(inc.ID, resolveNotes); err != nil {
		t.Fatalf("failed to resolve incident: %v", err)
	}
	updated, err = mgr.GetIncident(inc.ID)
	if err != nil {
		t.Fatalf("failed to get incident: %v", err)
	}
	if updated.Status != StatusResolved {
		t.Fatalf("expected status %s, got %s", StatusResolved, updated.Status)
	}
	if updated.ResolvedAt == nil {
		t.Fatal("expected non-nil ResolvedAt timestamp")
	}
	if !strings.Contains(updated.Notes, resolveNotes) {
		t.Fatalf("expected notes to contain %q, got %q", resolveNotes, updated.Notes)
	}

	// Verify SSE broadcast for resolved
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "event: incident_resolved") {
			t.Fatalf("unexpected SSE broadcast: %s", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for incident_resolved event")
	}

	// Disallow dispatching or containing already resolved incidents
	if err := mgr.DispatchIncident(inc.ID, "ranger-nobody"); err == nil {
		t.Fatal("expected error dispatching resolved incident, got nil")
	}
	if err := mgr.ContainIncident(inc.ID, "new containment"); err == nil {
		t.Fatal("expected error containing resolved incident, got nil")
	}
	if err := mgr.ResolveIncident(inc.ID, "re-resolve"); err == nil {
		t.Fatal("expected error resolving already resolved incident, got nil")
	}
}

func TestSeveritySortingAndActiveFilter(t *testing.T) {
	mgr := NewICSManager()

	cg1 := "cg-joshua-tree-hidden-valley"
	cg2 := "cg-zion-watchman"

	// Create incidents with differing severities and campgrounds
	inc1, err := mgr.ReportIncident(IncidentTrailWashout, SeverityAdvisory, cg1, "Sector-North", "Trailhead-A", "Minor erosion on trail edge")
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}

	inc2, err := mgr.ReportIncident(IncidentSmokeHazard, SeverityAlert, cg1, "Sector-South", "Trailhead-B", "Smoke drifting from ridge fire")
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}

	inc3, err := mgr.ReportIncident(IncidentSearchAndRescue, SeverityImmediateEvacuation, cg1, "Sector-East", "Trailhead-C", "Flash flood inundation in canyon")
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}

	inc4, err := mgr.ReportIncident(IncidentFoodConditioned, SeverityEvacuationWarning, cg1, "Sector-West", "Trailhead-D", "Habituated bear entering tents")
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}

	// Incident in another campground
	inc5, err := mgr.ReportIncident(IncidentSmokeHazard, SeverityImmediateEvacuation, cg2, "Zion-Main", "Watchman-01", "Rockfall near campsite")
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}

	// 1. List active incidents for cg1
	activeCg1 := mgr.ListActiveIncidents(cg1)
	if len(activeCg1) != 4 {
		t.Fatalf("expected 4 active incidents for cg1, got %d", len(activeCg1))
	}

	// Verify strict severity ordering: IMMEDIATE_EVACUATION (inc3) -> EVACUATION_WARNING (inc4) -> ALERT (inc2) -> ADVISORY (inc1)
	if activeCg1[0].ID != inc3.ID || activeCg1[0].Severity != SeverityImmediateEvacuation {
		t.Fatalf("expected first incident to be %s (%s), got %s (%s)", inc3.ID, SeverityImmediateEvacuation, activeCg1[0].ID, activeCg1[0].Severity)
	}
	if activeCg1[1].ID != inc4.ID || activeCg1[1].Severity != SeverityEvacuationWarning {
		t.Fatalf("expected second incident to be %s (%s), got %s (%s)", inc4.ID, SeverityEvacuationWarning, activeCg1[1].ID, activeCg1[1].Severity)
	}
	if activeCg1[2].ID != inc2.ID || activeCg1[2].Severity != SeverityAlert {
		t.Fatalf("expected third incident to be %s (%s), got %s (%s)", inc2.ID, SeverityAlert, activeCg1[2].ID, activeCg1[2].Severity)
	}
	if activeCg1[3].ID != inc1.ID || activeCg1[3].Severity != SeverityAdvisory {
		t.Fatalf("expected fourth incident to be %s (%s), got %s (%s)", inc1.ID, SeverityAdvisory, activeCg1[3].ID, activeCg1[3].Severity)
	}

	// 2. Resolve inc3 and verify it drops from active list
	if err := mgr.ResolveIncident(inc3.ID, "Canyon drained, all parties rescued safely"); err != nil {
		t.Fatalf("failed to resolve inc3: %v", err)
	}

	activeCg1Post := mgr.ListActiveIncidents(cg1)
	if len(activeCg1Post) != 3 {
		t.Fatalf("expected 3 active incidents after resolution, got %d", len(activeCg1Post))
	}
	for _, inc := range activeCg1Post {
		if inc.ID == inc3.ID {
			t.Fatalf("resolved incident %s still found in active list", inc3.ID)
		}
	}

	// 3. Test Evacuation Muster List for cg1
	musterList := mgr.GenerateEvacuationMusterList(cg1)
	// Active evacuation severities in cg1: inc4 (EVACUATION_WARNING), inc2 (ALERT) - inc1 is ADVISORY so not muster
	if len(musterList) != 2 {
		t.Fatalf("expected 2 evacuation muster directives for cg1, got %d: %v", len(musterList), musterList)
	}
	for _, entry := range musterList {
		if !strings.Contains(entry, cg1) {
			t.Errorf("muster entry missing campground ID: %s", entry)
		}
	}

	// 4. Test Evacuation Muster List for cg2
	musterCg2 := mgr.GenerateEvacuationMusterList(cg2)
	if len(musterCg2) != 1 {
		t.Fatalf("expected 1 muster entry for cg2, got %d", len(musterCg2))
	}
	if !strings.Contains(musterCg2[0], inc5.ID) || !strings.Contains(musterCg2[0], "Immediate Evacuation") {
		t.Errorf("expected muster directive for cg2 to mention %s and immediate evacuation, got %s", inc5.ID, musterCg2[0])
	}
}

func TestConcurrentIncidentDispatching(t *testing.T) {
	mgr := NewICSManager()

	// Report 50 distinct incidents concurrently
	const goroutines = 50
	incidentIDs := make([]string, goroutines)
	var reportWg sync.WaitGroup
	reportWg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		idx := i
		go func() {
			defer reportWg.Done()
			inc, err := mgr.ReportIncident(
				IncidentBearSighting,
				SeverityAlert,
				"cg-glacier-many-glacier",
				fmt.Sprintf("Sector-%02d", idx),
				fmt.Sprintf("GPS:48.796,-113.657+%d", idx),
				fmt.Sprintf("Active grizzly bear foraging near tent cluster %d", idx),
			)
			if err != nil {
				t.Errorf("report failed for %d: %v", idx, err)
				return
			}
			incidentIDs[idx] = inc.ID
		}()
	}
	reportWg.Wait()

	// Now dispatch all 50 incidents concurrently with 50 distinct ranger IDs
	var dispatchWg sync.WaitGroup
	dispatchWg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		idx := i
		go func() {
			defer dispatchWg.Done()
			ranger := fmt.Sprintf("ranger-unit-%02d", idx)
			if err := mgr.DispatchIncident(incidentIDs[idx], ranger); err != nil {
				t.Errorf("dispatch failed for index %d (incident %s): %v", idx, incidentIDs[idx], err)
			}
		}()
	}
	dispatchWg.Wait()

	// Verify all 50 are dispatched
	active := mgr.ListActiveIncidents("cg-glacier-many-glacier")
	if len(active) != goroutines {
		t.Fatalf("expected %d active dispatched incidents, got %d", goroutines, len(active))
	}
	for _, inc := range active {
		if inc.Status != StatusDispatched {
			t.Errorf("expected incident %s to be DISPATCHED, got %s", inc.ID, inc.Status)
		}
		if !strings.HasPrefix(inc.AssignedRanger, "ranger-unit-") {
			t.Errorf("expected assigned ranger to start with ranger-unit-, got %s", inc.AssignedRanger)
		}
	}
}

