package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func setupTestServer() *Server {
	return NewServer("") // In-memory isolation engine for fast unit tests
}

func TestHealthCheck(t *testing.T) {
	s := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	s.HandleHealth(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "HEALTHY") {
		t.Errorf("Expected HEALTHY in body, got: %s", w.Body.String())
	}
}

func TestUserSignupAndLoginFlow(t *testing.T) {
	s := setupTestServer()

	// 1. Signup
	form := url.Values{
		"full_name": {"Diana Prince"},
		"email":     {"diana@camping.local"},
		"password":  {"secret456"},
	}
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.HandleSignup(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Signup failed with status %d: %s", w.Code, w.Body.String())
	}

	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "cosm_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("Expected cosm_session cookie to be set")
	}

	// 2. Duplicate signup rejection
	wDup := httptest.NewRecorder()
	reqDup := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
	reqDup.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.HandleSignup(wDup, reqDup)
	if wDup.Code != http.StatusConflict {
		t.Errorf("Expected duplicate signup to return 409 Conflict, got %d", wDup.Code)
	}

	// 3. Login with correct credentials
	loginForm := url.Values{
		"email":    {"diana@camping.local"},
		"password": {"secret456"},
	}
	reqLogin := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(loginForm.Encode()))
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wLogin := httptest.NewRecorder()
	s.HandleLogin(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Fatalf("Login failed with status %d: %s", wLogin.Code, wLogin.Body.String())
	}

	// 4. Login with bad credentials
	badForm := url.Values{
		"email":    {"diana@camping.local"},
		"password": {"wrongpass"},
	}
	reqBad := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(badForm.Encode()))
	reqBad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wBad := httptest.NewRecorder()
	s.HandleLogin(wBad, reqBad)
	if wBad.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for bad password, got %d", wBad.Code)
	}
}

func TestBookingCreationAndDoubleBookingConflict(t *testing.T) {
	s := setupTestServer()

	// Use Alice persona session cookie
	startDate := time.Now().Add(14 * 24 * time.Hour).Format("2006-01-02")
	endDate := time.Now().Add(18 * 24 * time.Hour).Format("2006-01-02")

	form := url.Values{
		"campsite_id": {"c1"},
		"start_date":  {startDate},
		"end_date":    {endDate},
	}

	req := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
	w := httptest.NewRecorder()

	s.HandleCreateBooking(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("First booking failed with status %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Reservation Confirmed") {
		t.Errorf("Expected confirmation in body, got: %s", w.Body.String())
	}

	// Attempt exact overlapping booking (Double Booking)
	wConflict := httptest.NewRecorder()
	reqConflict := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(form.Encode()))
	reqConflict.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqConflict.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
	s.HandleCreateBooking(wConflict, reqConflict)

	if wConflict.Code != http.StatusConflict {
		t.Fatalf("Expected 409 Conflict for overlapping reservation, got %d: %s", wConflict.Code, wConflict.Body.String())
	}
	if !strings.Contains(wConflict.Body.String(), "Concurrency Conflict") {
		t.Errorf("Expected Concurrency Conflict message, got: %s", wConflict.Body.String())
	}
}

func TestBookingCancellationAndInventoryRestoration(t *testing.T) {
	s := setupTestServer()

	startDate := time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02")
	endDate := time.Now().Add(33 * 24 * time.Hour).Format("2006-01-02")

	form := url.Values{
		"campsite_id": {"c2"},
		"start_date":  {startDate},
		"end_date":    {endDate},
	}

	req := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
	w := httptest.NewRecorder()
	s.HandleCreateBooking(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Booking failed: %s", w.Body.String())
	}

	// Retrieve Alice's bookings to get ID
	reqMy := httptest.NewRequest(http.MethodGet, "/my-bookings", nil)
	reqMy.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
	wMy := httptest.NewRecorder()
	s.HandleMyBookings(wMy, reqMy)

	// Find the created reservation ID
	s.mem.mu.RLock()
	var resID string
	for id, r := range s.mem.reservations {
		if r.CampsiteID == "c2" && r.Status == "confirmed" {
			resID = id
			break
		}
	}
	s.mem.mu.RUnlock()

	if resID == "" {
		t.Fatal("Could not locate created reservation ID")
	}

	// Cancel the reservation
	cancelForm := url.Values{"booking_id": {resID}}
	reqCancel := httptest.NewRequest(http.MethodPost, "/cancel-booking", strings.NewReader(cancelForm.Encode()))
	reqCancel.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqCancel.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
	wCancel := httptest.NewRecorder()
	s.HandleCancelBooking(wCancel, reqCancel)

	if wCancel.Code != http.StatusOK {
		t.Fatalf("Cancellation failed: %d", wCancel.Code)
	}

	// Now re-booking the EXACT SAME DATES must succeed!
	wRebook := httptest.NewRecorder()
	reqRebook := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(form.Encode()))
	reqRebook.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqRebook.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
	s.HandleCreateBooking(wRebook, reqRebook)

	if wRebook.Code != http.StatusOK {
		t.Fatalf("Re-booking after cancellation failed (%d): %s", wRebook.Code, wRebook.Body.String())
	}
}

func TestConcurrentSwarmContention(t *testing.T) {
	s := setupTestServer()

	var wg sync.WaitGroup
	results := make([]int, 5)

	startDate := time.Now().Add(60 * 24 * time.Hour).Format("2006-01-02")
	endDate := time.Now().Add(65 * 24 * time.Hour).Format("2006-01-02")

	// Fire 5 goroutines competing for the same slot
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			form := url.Values{
				"campsite_id": {"c3"},
				"start_date":  {startDate},
				"end_date":    {endDate},
			}
			req := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
			w := httptest.NewRecorder()
			s.HandleCreateBooking(w, req)
			results[idx] = w.Code
		}(i)
	}

	wg.Wait()

	successCount := 0
	conflictCount := 0
	for _, code := range results {
		if code == http.StatusOK {
			successCount++
		} else if code == http.StatusConflict {
			conflictCount++
		}
	}

	if successCount != 1 {
		t.Fatalf("Expected exactly 1 winner in race, got %d", successCount)
	}
	if conflictCount != 4 {
		t.Fatalf("Expected 4 conflicts in race, got %d", conflictCount)
	}
}

func TestTopoLeaseHoldLifecycle(t *testing.T) {
	s := setupTestServer()

	// 1. Acquire hold
	form := url.Values{
		"campsite_id": {"c1"},
		"start_date":  {"2026-11-01"},
		"end_date":    {"2026-11-05"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/holds", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
	w := httptest.NewRecorder()

	s.HandleHold(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for hold, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Query active holds
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/holds?campsite_id=c1", nil)
	wList := httptest.NewRecorder()
	s.HandleHold(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for list holds, got %d", wList.Code)
	}

	// 3. Second hold on same campsite and dates should conflict
	wConflict := httptest.NewRecorder()
	reqConflict := httptest.NewRequest(http.MethodPost, "/api/v1/holds", strings.NewReader(form.Encode()))
	reqConflict.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqConflict.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-bob-demo"})
	s.HandleHold(wConflict, reqConflict)
	if wConflict.Code != http.StatusConflict {
		t.Fatalf("Expected 409 Conflict for overlapping hold, got %d", wConflict.Code)
	}
}

func TestRangerPMSAndGateAuthEndpoints(t *testing.T) {
	s := setupTestServer()

	// 1. Create work order
	orderForm := url.Values{
		"campground_id": {"cg-olympic"},
		"campsite_id":   {"c1"},
		"category":      {"BEAR_BOX"},
		"severity":      {"CRITICAL"},
		"description":   {"Bear box latch bent by wildlife"},
	}
	reqOrder := httptest.NewRequest(http.MethodPost, "/api/v1/ranger/workorders", strings.NewReader(orderForm.Encode()))
	reqOrder.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wOrder := httptest.NewRecorder()
	s.HandleRangerWorkOrders(wOrder, reqOrder)
	if wOrder.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for work order, got %d: %s", wOrder.Code, wOrder.Body.String())
	}

	// 2. Register vehicle
	vehForm := url.Values{
		"plate":       {"WA-BEAR-01"},
		"state":       {"WA"},
		"campsite_id": {"c1"},
		"guest_name":  {"Alice Ranger"},
	}
	reqVeh := httptest.NewRequest(http.MethodPost, "/api/v1/ranger/vehicles", strings.NewReader(vehForm.Encode()))
	reqVeh.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wVeh := httptest.NewRecorder()
	s.HandleRangerVehicles(wVeh, reqVeh)
	if wVeh.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for vehicle reg, got %d: %s", wVeh.Code, wVeh.Body.String())
	}

	// 3. Check gate authorization for registered vehicle
	reqGate := httptest.NewRequest(http.MethodGet, "/api/v1/ranger/gate/authorize?plate=WA-BEAR-01&state=WA", nil)
	wGate := httptest.NewRecorder()
	s.HandleRangerGateAuthorize(wGate, reqGate)
	if wGate.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for gate check, got %d", wGate.Code)
	}
	if !strings.Contains(wGate.Body.String(), "AUTHORIZED") {
		t.Fatalf("Expected AUTHORIZED in gate response, got %s", wGate.Body.String())
	}

	// 4. Check gate authorization for unknown vehicle
	reqUnknown := httptest.NewRequest(http.MethodGet, "/api/v1/ranger/gate/authorize?plate=UNKNOWN99&state=OR", nil)
	wUnknown := httptest.NewRecorder()
	s.HandleRangerGateAuthorize(wUnknown, reqUnknown)
	if wUnknown.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for unknown gate check, got %d", wUnknown.Code)
	}
	if !strings.Contains(wUnknown.Body.String(), "DENIED") {
		t.Fatalf("Expected DENIED in unknown gate response, got %s", wUnknown.Body.String())
	}
}

func TestCamperProfileAndVehicleManagement(t *testing.T) {
	s := setupTestServer()
	aliceCookie := &http.Cookie{Name: "cosm_session", Value: "session-alice-demo"}

	// 1. Update camper profile
	profForm := url.Values{
		"full_name":               {"Alice Mountaineer"},
		"phone":                   {"+1-206-555-0199"},
		"emergency_contact_name":  {"Bob Backpacker"},
		"emergency_contact_phone": {"+1-206-555-0188"},
		"wilderness_pass_id":      {"PASS-WA-7890"},
		"notifications_enabled":   {"true"},
	}
	reqProf := httptest.NewRequest(http.MethodPost, "/api/v1/profile", strings.NewReader(profForm.Encode()))
	reqProf.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqProf.AddCookie(aliceCookie)
	wProf := httptest.NewRecorder()
	s.HandleCamperProfile(wProf, reqProf)
	if wProf.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for profile update, got %d: %s", wProf.Code, wProf.Body.String())
	}

	// 2. Add vehicle to fleet
	vehForm := url.Values{
		"plate":      {"WA-CAMP-77"},
		"state":      {"WA"},
		"make_model": {"Toyota Tacoma TRD"},
		"color":      {"Quicksand"},
		"is_primary": {"true"},
	}
	reqVeh := httptest.NewRequest(http.MethodPost, "/api/v1/profile/vehicles", strings.NewReader(vehForm.Encode()))
	reqVeh.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqVeh.AddCookie(aliceCookie)
	wVeh := httptest.NewRecorder()
	s.HandleCamperVehicles(wVeh, reqVeh)
	if wVeh.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for adding vehicle, got %d: %s", wVeh.Code, wVeh.Body.String())
	}

	// 3. Inspect profile via JSON
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
	reqGet.AddCookie(aliceCookie)
	wGet := httptest.NewRecorder()
	s.HandleCamperProfile(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for GET profile, got %d", wGet.Code)
	}
	if !strings.Contains(wGet.Body.String(), "WA-CAMP-77") {
		t.Fatalf("Expected WA-CAMP-77 in profile vehicle fleet, got %s", wGet.Body.String())
	}
}

func TestMultiLaneGateScanAndAutoReset(t *testing.T) {
	s := setupTestServer()

	// 1. Standard lane scan for authorized vehicle WA-ALPINE1
	scanForm := url.Values{
		"plate": {"WA-ALPINE1"},
		"state": {"WA"},
		"lane":  {"STANDARD"},
	}
	reqScan := httptest.NewRequest(http.MethodPost, "/api/v1/gate/scan", strings.NewReader(scanForm.Encode()))
	reqScan.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wScan := httptest.NewRecorder()
	s.HandleGateScan(wScan, reqScan)
	if wScan.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for standard gate scan, got %d: %s", wScan.Code, wScan.Body.String())
	}
	if !strings.Contains(wScan.Body.String(), "true") {
		t.Fatalf("Expected authorized=true for WA-ALPINE1, got %s", wScan.Body.String())
	}

	// 2. Trailer oversize lane scan matching secondary plate WA-TRL-44
	trailerForm := url.Values{
		"plate": {"WA-TRL-44"},
		"state": {"WA"},
		"lane":  {"TRAILER_OVERSIZE"},
	}
	reqTrailer := httptest.NewRequest(http.MethodPost, "/api/v1/gate/scan", strings.NewReader(trailerForm.Encode()))
	reqTrailer.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wTrailer := httptest.NewRecorder()
	s.HandleGateScan(wTrailer, reqTrailer)
	if wTrailer.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for trailer gate scan, got %d: %s", wTrailer.Code, wTrailer.Body.String())
	}
	if !strings.Contains(wTrailer.Body.String(), "true") {
		t.Fatalf("Expected authorized=true for trailer plate, got %s", wTrailer.Body.String())
	}

	// 3. Inspect recent gate logs
	reqLogs := httptest.NewRequest(http.MethodGet, "/api/v1/gate/logs", nil)
	wLogs := httptest.NewRecorder()
	s.HandleGateLogs(wLogs, reqLogs)
	if wLogs.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for gate logs, got %d", wLogs.Code)
	}

	// 4. Manually reset barrier
	reqReset := httptest.NewRequest(http.MethodPost, "/api/v1/gate/reset", nil)
	wReset := httptest.NewRecorder()
	s.HandleGateReset(wReset, reqReset)
	if wReset.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for gate reset, got %d", wReset.Code)
	}
}

func TestICSIncidentsAndPMSWorkOrders(t *testing.T) {
	s := setupTestServer()

	// 1. Declare ICS incident
	incForm := url.Values{
		"type":        {"BEAR_SIGHTING"},
		"severity":    {"HIGH"},
		"description": {"Confirmed sighting within 50 yards of campsites 12 and 14."},
		"sector":      {"Loop C"},
	}
	reqInc := httptest.NewRequest(http.MethodPost, "/api/v1/incidents", strings.NewReader(incForm.Encode()))
	reqInc.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wInc := httptest.NewRecorder()
	s.HandleIncidents(wInc, reqInc)
	if wInc.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for incident creation, got %d: %s", wInc.Code, wInc.Body.String())
	}

	// Extract incident ID
	var incResp map[string]interface{}
	if err := json.Unmarshal(wInc.Body.Bytes(), &incResp); err != nil {
		t.Fatalf("Failed to decode incident response: %v", err)
	}
	incID, ok := incResp["id"].(string)
	if !ok || incID == "" {
		t.Fatalf("Missing incident id in response: %v", incResp)
	}

	// 2. Dispatch personnel to incident
	dispForm := url.Values{
		"incident_id": {incID},
		"ranger_id":   {"RGR-DAVE-04"},
		"notes":       {"Deploying non-lethal hazing and signage."},
	}
	reqDisp := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/dispatch", strings.NewReader(dispForm.Encode()))
	reqDisp.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wDisp := httptest.NewRecorder()
	s.HandleIncidentDispatch(wDisp, reqDisp)
	if wDisp.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for incident dispatch, got %d: %s", wDisp.Code, wDisp.Body.String())
	}

	// 3. Inspect muster list and headcount
	reqMuster := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/muster", nil)
	wMuster := httptest.NewRecorder()
	s.HandleIncidentMuster(wMuster, reqMuster)
	if wMuster.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for muster list, got %d", wMuster.Code)
	}

	// 4. Inspect PMS physical assets
	reqAssets := httptest.NewRequest(http.MethodGet, "/api/v1/pms/assets", nil)
	wAssets := httptest.NewRecorder()
	s.HandlePMSAssets(wAssets, reqAssets)
	if wAssets.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for PMS assets, got %d", wAssets.Code)
	}
}

func TestBackcountryZonesAndLotteryDraw(t *testing.T) {
	s := setupTestServer()

	// 1. List backcountry zones
	reqZones := httptest.NewRequest(http.MethodGet, "/api/v1/backcountry/zones", nil)
	wZones := httptest.NewRecorder()
	s.HandleBackcountryZones(wZones, reqZones)
	if wZones.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for backcountry zones, got %d", wZones.Code)
	}
	if !strings.Contains(wZones.Body.String(), "zone-enchantments") {
		t.Fatalf("Expected zone-enchantments in zones list, got %s", wZones.Body.String())
	}

	// 2. Publish cryptographic commitment hash
	targetDate := time.Now().Add(24 * time.Hour).Format("2006-01-02")
	commitForm := url.Values{
		"trailhead_id": {"zone-enchantments"},
		"draw_date":    {targetDate},
		"secret_salt":  {"topocosm-salt-999"},
	}
	reqCommit := httptest.NewRequest(http.MethodPost, "/api/v1/backcountry/commitment", strings.NewReader(commitForm.Encode()))
	reqCommit.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wCommit := httptest.NewRecorder()
	s.HandleBackcountryCommit(wCommit, reqCommit)
	if wCommit.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for commitment publish, got %d: %s", wCommit.Code, wCommit.Body.String())
	}

	// 3. Submit lottery application
	appForm := url.Values{
		"trailhead_id": {"zone-enchantments"},
		"target_date":  {targetDate},
		"party_size":   {"2"},
		"lnt_cert_id":  {"LNT-998877"},
	}
	reqApp := httptest.NewRequest(http.MethodPost, "/api/v1/backcountry/applications", strings.NewReader(appForm.Encode()))
	reqApp.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqApp.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
	wApp := httptest.NewRecorder()
	s.HandleBackcountryApply(wApp, reqApp)
	if wApp.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for lottery application, got %d: %s", wApp.Code, wApp.Body.String())
	}

	// 4. Perform provably fair lottery draw
	drawForm := url.Values{
		"trailhead_id": {"zone-enchantments"},
		"target_date":  {targetDate},
		"secret_salt":  {"topocosm-salt-999"},
	}
	reqDraw := httptest.NewRequest(http.MethodPost, "/api/v1/backcountry/draw", strings.NewReader(drawForm.Encode()))
	reqDraw.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wDraw := httptest.NewRecorder()
	s.HandleBackcountryDraw(wDraw, reqDraw)
	if wDraw.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for lottery draw, got %d: %s", wDraw.Code, wDraw.Body.String())
	}
	if !strings.Contains(wDraw.Body.String(), "WON") {
		t.Fatalf("Expected WON in lottery draw results, got %s", wDraw.Body.String())
	}
}

func TestOutfitterGearAndContactlessLockers(t *testing.T) {
	s := setupTestServer()

	// 1. Get gear catalog
	reqGear := httptest.NewRequest(http.MethodGet, "/api/v1/outfitter/gear", nil)
	wGear := httptest.NewRecorder()
	s.HandleOutfitterGear(wGear, reqGear)
	if wGear.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for gear catalog, got %d", wGear.Code)
	}

	// 2. Reserve bear canister gear
	resForm := url.Values{
		"item_serial": {"BC-GARCIA-001"},
		"rental_days": {"3"},
	}
	reqRes := httptest.NewRequest(http.MethodPost, "/api/v1/outfitter/reserve", strings.NewReader(resForm.Encode()))
	reqRes.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqRes.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
	wRes := httptest.NewRecorder()
	s.HandleOutfitterReserve(wRes, reqRes)
	if wRes.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for gear reservation, got %d: %s", wRes.Code, wRes.Body.String())
	}

	var bayResp map[string]interface{}
	if err := json.Unmarshal(wRes.Body.Bytes(), &bayResp); err != nil {
		t.Fatalf("Failed to parse gear reservation: %v", err)
	}
	bayNumVal, ok := bayResp["bay_number"].(float64)
	if !ok {
		t.Fatalf("Missing bay_number in response: %v", bayResp)
	}
	bayNum := int(bayNumVal)
	pin, _ := bayResp["passcode_pin"].(string)
	serial, _ := bayResp["item_serial_number"].(string)

	if pin == "" || serial == "" {
		t.Fatalf("Missing pin or serial in gear reservation: %v", bayResp)
	}

	// 3. Unlock locker bay with 6-digit OTP
	unlockForm := url.Values{
		"bay_number": {fmt.Sprintf("%d", bayNum)},
		"pin":        {pin},
	}
	reqUnlock := httptest.NewRequest(http.MethodPost, "/api/v1/outfitter/unlock", strings.NewReader(unlockForm.Encode()))
	reqUnlock.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wUnlock := httptest.NewRecorder()
	s.HandleOutfitterUnlock(wUnlock, reqUnlock)
	if wUnlock.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for locker unlock, got %d: %s", wUnlock.Code, wUnlock.Body.String())
	}

	// 4. Return gear into locker bay
	returnForm := url.Values{
		"bay_number":  {fmt.Sprintf("%d", bayNum)},
		"item_serial": {serial},
	}
	reqReturn := httptest.NewRequest(http.MethodPost, "/api/v1/outfitter/return", strings.NewReader(returnForm.Encode()))
	reqReturn.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wReturn := httptest.NewRecorder()
	s.HandleOutfitterReturn(wReturn, reqReturn)
	if wReturn.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for gear return, got %d: %s", wReturn.Code, wReturn.Body.String())
	}

	// 5. Inspect 16-bay smart locker grid
	reqLockers := httptest.NewRequest(http.MethodGet, "/api/v1/outfitter/lockers", nil)
	wLockers := httptest.NewRecorder()
	s.HandleOutfitterLockers(wLockers, reqLockers)
	if wLockers.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for lockers list, got %d", wLockers.Code)
	}
}

func TestRothermelRateOfSpreadAndBriefing(t *testing.T) {
	s := setupTestServer()

	// 1. Calculate Rothermel surface rate of spread for Fuel Model 1
	calcForm := url.Values{
		"fuel_model_number": {"1"},
		"fuel_moisture_pct": {"8.0"},
		"wind_speed_mph":    {"15.0"},
		"slope_deg":         {"10.0"},
	}
	reqCalc := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry/rothermel", strings.NewReader(calcForm.Encode()))
	reqCalc.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wCalc := httptest.NewRecorder()
	s.HandleTelemetryRothermel(wCalc, reqCalc)
	if wCalc.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for Rothermel calc, got %d: %s", wCalc.Code, wCalc.Body.String())
	}

	var rothResp map[string]interface{}
	if err := json.Unmarshal(wCalc.Body.Bytes(), &rothResp); err != nil {
		t.Fatalf("Failed to parse Rothermel response: %v", err)
	}
	ros, ok := rothResp["rate_of_spread_ft_min"].(float64)
	if !ok || ros <= 0 {
		t.Fatalf("Expected positive rate of spread, got %v", rothResp)
	}

	// 2. Fetch elevation sensor mesh
	reqMesh := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/mesh", nil)
	wMesh := httptest.NewRecorder()
	s.HandleTelemetryMesh(wMesh, reqMesh)
	if wMesh.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for sensor mesh, got %d", wMesh.Code)
	}

	// 3. Generate tactical ranger fire briefing
	reqBrief := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry/briefing", nil)
	wBrief := httptest.NewRecorder()
	s.HandleTelemetryBriefing(wBrief, reqBrief)
	if wBrief.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for briefing, got %d: %s", wBrief.Code, wBrief.Body.String())
	}
	if !strings.Contains(wBrief.Body.String(), "Fire Briefing") && !strings.Contains(wBrief.Body.String(), "fire_spread_predictions") {
		t.Fatalf("Expected briefing content in response, got %s", wBrief.Body.String())
	}
}

func TestOfflineGateCacheWALReconciliation(t *testing.T) {
	s := setupTestServer()

	// 1. Authorize offline access through local cache
	authForm := url.Values{
		"permit_hash": {"PERMIT-OFFLINE-ALPHA"},
	}
	reqAuth := httptest.NewRequest(http.MethodPost, "/api/v1/sync/authorize-offline", strings.NewReader(authForm.Encode()))
	reqAuth.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wAuth := httptest.NewRecorder()
	s.HandleSyncAuthorizeOffline(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for offline authorize, got %d: %s", wAuth.Code, wAuth.Body.String())
	}

	// 2. Inspect offline gatehouse WAL
	reqWAL := httptest.NewRequest(http.MethodGet, "/api/v1/sync/wal", nil)
	wWAL := httptest.NewRecorder()
	s.HandleSyncWAL(wWAL, reqWAL)
	if wWAL.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for sync WAL, got %d", wWAL.Code)
	}

	// 3. Reconcile offline WAL with cloud hub
	reqReconcile := httptest.NewRequest(http.MethodPost, "/api/v1/sync/reconcile", nil)
	wReconcile := httptest.NewRecorder()
	s.HandleSyncReconcile(wReconcile, reqReconcile)
	if wReconcile.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for reconciliation, got %d: %s", wReconcile.Code, wReconcile.Body.String())
	}
}

func TestWeatherCampsiteAndSafetyAdvisory(t *testing.T) {
	s := setupTestServer()

	// 1. Query campsite weather in JSON format
	reqJSON := httptest.NewRequest(http.MethodGet, "/api/v1/weather/campsite?campsite_id=c1&format=json", nil)
	wJSON := httptest.NewRecorder()
	s.HandleWeatherCampsite(wJSON, reqJSON)
	if wJSON.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for weather JSON, got %d: %s", wJSON.Code, wJSON.Body.String())
	}
	if !strings.Contains(wJSON.Body.String(), "Pine Ridge Site 4") || !strings.Contains(wJSON.Body.String(), "campfire_advisory") {
		t.Errorf("Unexpected weather JSON payload: %s", wJSON.Body.String())
	}

	// 2. Query campsite weather as HTMX widget snippet
	reqHTML := httptest.NewRequest(http.MethodGet, "/api/v1/weather/campsite?campsite_id=c1", nil)
	wHTML := httptest.NewRecorder()
	s.HandleWeatherCampsite(wHTML, reqHTML)
	if wHTML.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for weather HTML snippet, got %d", wHTML.Code)
	}
	if !strings.Contains(wHTML.Body.String(), "Microclimate Telemetry") {
		t.Errorf("Expected Microclimate Telemetry widget in response, got: %s", wHTML.Body.String())
	}
}

func TestSOSBeaconTriggerAndLifecycle(t *testing.T) {
	s := setupTestServer()

	// 1. Dispatch emergency SOS beacon
	form := url.Values{
		"location":      {"Enchantments Pass Mile 11"},
		"latitude":      {"47.4892"},
		"longitude":     {"-120.7812"},
		"distress_type": {"INJURY"},
		"description":   {"Fractured fibula on steep scree field, immobilized"},
		"format":        {"json"},
	}
	reqTrigger := httptest.NewRequest(http.MethodPost, "/api/v1/sos/beacon", strings.NewReader(form.Encode()))
	reqTrigger.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqTrigger.AddCookie(&http.Cookie{Name: "cosm_session", Value: "session-alice-demo"})
	wTrigger := httptest.NewRecorder()
	s.HandleSOSBeaconTrigger(wTrigger, reqTrigger)
	if wTrigger.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for SOS trigger, got %d: %s", wTrigger.Code, wTrigger.Body.String())
	}

	var beacon struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(wTrigger.Body.Bytes(), &beacon); err != nil {
		t.Fatalf("Failed parsing beacon response: %v", err)
	}
	if beacon.ID == "" || beacon.Status != "ACTIVE_SEARCHING" {
		t.Errorf("Unexpected beacon response state: %+v", beacon)
	}

	// 2. Query active beacons
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/sos/active", nil)
	wList := httptest.NewRecorder()
	s.HandleSOSBeaconList(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for SOS active list, got %d", wList.Code)
	}
	if !strings.Contains(wList.Body.String(), beacon.ID) {
		t.Errorf("Expected beacon %s in active list, got: %s", beacon.ID, wList.Body.String())
	}

	// 3. Resolve beacon
	resForm := url.Values{
		"beacon_id": {beacon.ID},
	}
	reqRes := httptest.NewRequest(http.MethodPost, "/api/v1/sos/resolve", strings.NewReader(resForm.Encode()))
	reqRes.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wRes := httptest.NewRecorder()
	s.HandleSOSBeaconResolve(wRes, reqRes)
	if wRes.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for beacon resolve, got %d: %s", wRes.Code, wRes.Body.String())
	}
}


