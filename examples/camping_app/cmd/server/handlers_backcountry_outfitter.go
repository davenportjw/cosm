package main

import (
	"encoding/json"
	"fmt"
	"github.com/cosmscm/cosm/examples/camping_app/internal/backcountry"
	"github.com/cosmscm/cosm/examples/camping_app/internal/outfitter"
	"net/http"
	"strconv"
	"time"
)

// HandleBackcountryZones returns registered trailhead zones and quotas.
func (s *Server) HandleBackcountryZones(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	targetDate := time.Now().Add(24 * time.Hour)
	if dateStr != "" {
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			targetDate = parsed
		}
	}

	zones := s.backcountryEng.ListZones()

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderTrailheadZonesHTML(w, zones, targetDate)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(zones)
}

// HandleBackcountryApply registers an applicant for the backcountry permit lottery.
func (s *Server) HandleBackcountryApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := s.getAuthenticatedUser(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		TrailheadID        string `json:"trailhead_id"`
		TargetDate         string `json:"target_date"`
		PartySize          int    `json:"party_size"`
		LeaveNoTraceCertID string `json:"lnt_cert_id"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&req)
	} else {
		r.ParseForm()
		req.TrailheadID = r.FormValue("trailhead_id")
		req.TargetDate = r.FormValue("target_date")
		req.PartySize, _ = strconv.Atoi(r.FormValue("party_size"))
		req.LeaveNoTraceCertID = r.FormValue("lnt_cert_id")
	}

	if req.TrailheadID == "" {
		http.Error(w, `{"error":"trailhead_id required"}`, http.StatusBadRequest)
		return
	}
	if req.PartySize <= 0 {
		req.PartySize = 2
	}
	if req.LeaveNoTraceCertID == "" {
		req.LeaveNoTraceCertID = "LNT-ONLINE-PASS-CERTIFIED"
	}

	targetDate := time.Now().Add(48 * time.Hour)
	if req.TargetDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.TargetDate); err == nil {
			targetDate = parsed
		}
	}

	app := &backcountry.LotteryApplication{
		ID:                 fmt.Sprintf("APP-%d", time.Now().UnixNano()%100000),
		UserID:             user.ID,
		FullName:           user.FullName,
		TrailheadID:        req.TrailheadID,
		TargetDate:         targetDate,
		PartySize:          req.PartySize,
		LeaveNoTraceCertID: req.LeaveNoTraceCertID,
		SubmittedAt:        time.Now(),
		Status:             "PENDING",
	}

	if err := s.backcountryEng.SubmitApplication(app); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div class="p-3 bg-emerald-50 border border-emerald-300 rounded text-xs font-mono text-emerald-900 space-y-1">
			<div class="font-bold">✅ Permit Lottery Application Staged</div>
			<div>Ref: %s · Trailhead: %s · Target: %s</div>
			<div class="text-[10px] text-emerald-700">LNT Verification: %s (Valid) · Status: PENDING DRAW</div>
		</div>`, app.ID, templateEscape(app.TrailheadID), app.TargetDate.Format("Jan 02, 2006"), app.LeaveNoTraceCertID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(app)
}

// HandleBackcountryCommit publishes the park secret commitment hash for a trailhead draw.
func (s *Server) HandleBackcountryCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TrailheadID string `json:"trailhead_id"`
		DrawDate    string `json:"draw_date"`
		SecretSalt  string `json:"secret_salt"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&req)
	} else {
		r.ParseForm()
		req.TrailheadID = r.FormValue("trailhead_id")
		req.DrawDate = r.FormValue("draw_date")
		req.SecretSalt = r.FormValue("secret_salt")
	}

	if req.TrailheadID == "" {
		req.TrailheadID = "zone-enchantments"
	}
	if req.SecretSalt == "" {
		req.SecretSalt = "alpine-ranger-salt-2026"
	}

	drawDate := time.Now().Add(24 * time.Hour)
	if req.DrawDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.DrawDate); err == nil {
			drawDate = parsed
		}
	}

	commit, err := s.backcountryEng.PublishLotteryCommitment(req.TrailheadID, drawDate, req.SecretSalt)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div class="p-3 bg-stone-100 border border-stone-300 rounded font-mono text-xs space-y-1">
			<div class="font-bold text-stone-900">🔒 Provably Fair Lottery Commitment Published</div>
			<div class="text-[10px] text-stone-600 truncate">SHA-256 Hash: %s</div>
			<div class="text-[10px] text-stone-500">Zone: %s · Draw Date: %s</div>
		</div>`, commit.CommitmentHash, commit.TrailheadID, commit.DrawDate.Format("Jan 02, 2006"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(commit)
}

// HandleBackcountryDraw executes the provably fair lottery draw.
func (s *Server) HandleBackcountryDraw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TrailheadID string `json:"trailhead_id"`
		TargetDate  string `json:"target_date"`
		SecretSalt  string `json:"secret_salt"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&req)
	} else {
		r.ParseForm()
		req.TrailheadID = r.FormValue("trailhead_id")
		req.TargetDate = r.FormValue("target_date")
		req.SecretSalt = r.FormValue("secret_salt")
	}

	if req.TrailheadID == "" {
		req.TrailheadID = "zone-enchantments"
	}
	if req.SecretSalt == "" {
		req.SecretSalt = "alpine-ranger-salt-2026"
	}

	targetDate := time.Now().Add(24 * time.Hour)
	if req.TargetDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.TargetDate); err == nil {
			targetDate = parsed
		}
	}

	winners, err := s.backcountryEng.ExecuteProvablyFairDraw(req.TrailheadID, targetDate, req.SecretSalt)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderLotteryDrawResultsHTML(w, winners, req.TrailheadID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "drawn",
		"trailhead_id":  req.TrailheadID,
		"winners_count": len(winners),
		"winners":       winners,
	})
}

// HandleOutfitterGear returns the gear rental catalog.
func (s *Server) HandleOutfitterGear(w http.ResponseWriter, r *http.Request) {
	gear := s.lockerMgr.ListAvailableGear()

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderGearCatalogHTML(w, gear)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gear)
}

// HandleOutfitterReserve reserves gear and provisions a contactless smart locker bay with a 6-digit PIN.
func (s *Server) HandleOutfitterReserve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := s.getAuthenticatedUser(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		ItemSerial    string `json:"item_serial"`
		ReservationID string `json:"reservation_id"`
		RentalDays    int    `json:"rental_days"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&req)
	} else {
		r.ParseForm()
		req.ItemSerial = r.FormValue("item_serial")
		req.ReservationID = r.FormValue("reservation_id")
		req.RentalDays, _ = strconv.Atoi(r.FormValue("rental_days"))
	}

	if req.ItemSerial == "" {
		http.Error(w, `{"error":"item_serial required"}`, http.StatusBadRequest)
		return
	}
	if req.RentalDays <= 0 {
		req.RentalDays = 3
	}
	if req.ReservationID == "" {
		req.ReservationID = fmt.Sprintf("RES-GEAR-%d", time.Now().UnixNano()%10000)
	}

	bay, err := s.lockerMgr.ReserveGearAndLocker(user.ID, req.ReservationID, req.ItemSerial, req.RentalDays)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
		<div class="p-3 bg-emerald-50 border border-emerald-300 rounded font-mono text-xs space-y-1.5 text-emerald-950">
			<div class="flex items-center justify-between font-bold text-sm">
				<span>🎒 Smart Locker Provisioned: Bay #%d</span>
				<span class="px-2 py-0.5 bg-emerald-200 rounded text-emerald-900 font-mono text-base tracking-widest">%s</span>
			</div>
			<div class="text-[11px]">Item: %s · Days: %d</div>
			<div class="text-[10px] text-emerald-700">Enter this 6-digit OTP code at the locker terminal kiosk to unlatch the electromagnetic solenoid door.</div>
		</div>`, bay.BayNumber, bay.PasscodePIN, templateEscape(bay.ItemSerialNumber), req.RentalDays)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(bay)
}

// HandleOutfitterUnlock unlocks a smart locker bay using the 6-digit OTP passcode.
func (s *Server) HandleOutfitterUnlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BayNumber int    `json:"bay_number"`
		PIN       string `json:"pin"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&req)
	} else {
		r.ParseForm()
		req.BayNumber, _ = strconv.Atoi(r.FormValue("bay_number"))
		req.PIN = r.FormValue("pin")
	}

	unlocked, serial, err := s.lockerMgr.UnlockLocker(req.BayNumber, req.PIN)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusUnauthorized)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
		<div class="p-3 bg-emerald-100 border border-emerald-300 rounded font-mono text-xs text-emerald-900">
			🔓 <strong>SOLENOID UNLATCHED</strong>: Bay #%d Door Opened. Retrieved: <strong>%s</strong>.
		</div>`, req.BayNumber, templateEscape(serial))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"unlocked":    unlocked,
		"bay_number":  req.BayNumber,
		"item_serial": serial,
	})
}

// HandleOutfitterReturn processes returning gear back into an idle locker bay.
func (s *Server) HandleOutfitterReturn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BayNumber  int    `json:"bay_number"`
		ItemSerial string `json:"item_serial"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&req)
	} else {
		r.ParseForm()
		req.BayNumber, _ = strconv.Atoi(r.FormValue("bay_number"))
		req.ItemSerial = r.FormValue("item_serial")
	}

	if err := s.lockerMgr.ReturnGearToLocker(req.BayNumber, req.ItemSerial); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div class="p-3 bg-stone-100 border border-stone-300 rounded font-mono text-xs text-stone-800">
			✅ Gear %s returned to Bay #%d. Locker locked and returned to inventory.
		</div>`, templateEscape(req.ItemSerial), req.BayNumber)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "returned",
		"bay_number":  fmt.Sprintf("%d", req.BayNumber),
		"item_serial": req.ItemSerial,
	})
}

// HandleOutfitterLockers inspects the status of the 16-bay smart locker bank.
func (s *Server) HandleOutfitterLockers(w http.ResponseWriter, r *http.Request) {
	bays := make([]*outfitter.LockerBay, 0, 16)
	for i := 1; i <= 16; i++ {
		if b, err := s.lockerMgr.GetLockerStatus(i); err == nil {
			bays = append(bays, b)
		}
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderLockerGridHTML(w, bays)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bays)
}

func (s *Server) renderTrailheadZonesHTML(w http.ResponseWriter, zones []*backcountry.TrailheadZone, date time.Time) {
	fmt.Fprintf(w, `<div class="grid grid-cols-1 md:grid-cols-3 gap-3">`)
	for _, z := range zones {
		quota := s.backcountryEng.GetAvailableQuota(z.ID, date)
		diffColor := "bg-blue-100 text-blue-800"
		if z.DifficultyRating == "STRENUOUS" {
			diffColor = "bg-amber-100 text-amber-800"
		} else if z.DifficultyRating == "ALPINE_TECHNICAL" {
			diffColor = "bg-rose-100 text-rose-800 font-bold"
		}

		fmt.Fprintf(w, `
		<div class="p-3 bg-white border border-stone-200 rounded text-xs space-y-2 flex flex-col justify-between">
			<div>
				<div class="flex items-center justify-between">
					<span class="font-bold text-stone-900">%s</span>
					<span class="px-1.5 py-0.5 rounded text-[10px] font-mono %s">%s</span>
				</div>
				<div class="text-[11px] text-stone-500 font-mono mt-0.5">Elevation: %d ft</div>
				<div class="mt-2 text-[11px] space-y-1 text-stone-600">
					<div>Daily Quota: <strong>%d permits</strong> (Avail: <strong class="text-emerald-700">%d</strong>)</div>
					<div>Bear Canister: %s</div>
				</div>
			</div>
			<div class="pt-2 border-t border-stone-100">
				<button hx-post="/api/v1/backcountry/applications" hx-vals='{"trailhead_id":"%s","party_size":"2"}' hx-target="#lottery-application-status" class="w-full py-1.5 bg-stone-900 text-stone-100 rounded text-xs font-medium hover:bg-stone-800">
					Apply for Permit
				</button>
			</div>
		</div>`,
			templateEscape(z.Name),
			diffColor, templateEscape(z.DifficultyRating),
			z.ElevationFt,
			z.DailyQuota, quota,
			func() string {
				if z.BearCanisterRequired {
					return `<span class="text-amber-800 font-bold">REQUIRED</span>`
				}
				return "Recommended"
			}(),
			templateEscape(z.ID),
		)
	}
	fmt.Fprint(w, `</div>`)
}

func (s *Server) renderLotteryDrawResultsHTML(w http.ResponseWriter, winners []*backcountry.LotteryApplication, zoneID string) {
	fmt.Fprintf(w, `
	<div class="p-3 bg-white border border-stone-300 rounded font-mono text-xs space-y-2">
		<div class="font-bold text-stone-900 flex items-center justify-between">
			<span>🎲 Provably Fair Draw Completed (%s)</span>
			<span class="text-emerald-700">%d Winners Awarded</span>
		</div>
		<div class="divide-y divide-stone-100">`, templateEscape(zoneID), len(winners))

	if len(winners) == 0 {
		fmt.Fprint(w, `<div class="py-2 text-stone-400 italic">No applicants drawn or quota exhausted.</div>`)
	}
	for _, app := range winners {
		fmt.Fprintf(w, `
		<div class="py-1.5 flex items-center justify-between">
			<span>%s (Party of %d)</span>
			<span class="px-2 py-0.5 bg-emerald-100 text-emerald-800 rounded font-bold text-[10px]">AWARDED PERMIT</span>
		</div>`, templateEscape(app.FullName), app.PartySize)
	}
	fmt.Fprint(w, `</div></div>`)
}

func (s *Server) renderGearCatalogHTML(w http.ResponseWriter, gear []*outfitter.GearItem) {
	fmt.Fprint(w, `<div class="grid grid-cols-1 md:grid-cols-2 gap-3">`)
	for _, g := range gear {
		fmt.Fprintf(w, `
		<div class="p-3 bg-white border border-stone-200 rounded text-xs space-y-2 flex flex-col justify-between">
			<div>
				<div class="flex items-center justify-between font-bold text-stone-900">
					<span>%s</span>
					<span class="font-mono text-emerald-800">$%.2f/day</span>
				</div>
				<div class="text-[10px] text-stone-500 font-mono">Category: %s · Serial: %s</div>
				<div class="text-[10px] text-stone-400">Condition: %s · Deposit: $%.2f</div>
			</div>
			<div>
				<button hx-post="/api/v1/outfitter/reserve" hx-vals='{"item_serial":"%s","rental_days":"3"}' hx-target="#locker-provision-status" class="w-full py-1.5 bg-stone-100 hover:bg-stone-200 text-stone-800 border border-stone-300 rounded text-xs font-mono font-medium">
					Reserve & Provision Locker
				</button>
			</div>
		</div>`,
			templateEscape(g.Name), float64(g.DailyRateCents)/100.0,
			templateEscape(g.Category), templateEscape(g.SerialNumber),
			templateEscape(g.Condition), float64(g.DepositCents)/100.0,
			templateEscape(g.SerialNumber),
		)
	}
	fmt.Fprint(w, `</div>`)
}

func (s *Server) renderLockerGridHTML(w http.ResponseWriter, bays []*outfitter.LockerBay) {
	fmt.Fprint(w, `<div class="grid grid-cols-4 md:grid-cols-8 gap-2 font-mono text-xs">`)
	for _, b := range bays {
		statusColor := "bg-stone-50 border-stone-300 text-stone-500"
		if b.Status == "LOADED" {
			statusColor = "bg-amber-100 border-amber-400 text-amber-900 font-bold"
		} else if b.Status == "CLAIMED" {
			statusColor = "bg-emerald-100 border-emerald-400 text-emerald-900"
		}
		fmt.Fprintf(w, `
		<div class="p-2 border rounded text-center space-y-1 %s">
			<div class="text-[10px] text-stone-400">BAY</div>
			<div class="font-bold text-sm">#%d</div>
			<div class="text-[9px] uppercase tracking-wider">%s</div>
		</div>`, statusColor, b.BayNumber, b.Status)
	}
	fmt.Fprint(w, `</div>`)
}

// Route Binding: GET date -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Request -> 

