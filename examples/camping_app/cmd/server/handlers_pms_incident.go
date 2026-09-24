package main

import (
	"encoding/json"
	"fmt"
	"github.com/cosmscm/cosm/examples/camping_app/internal/incident"
	"github.com/cosmscm/cosm/examples/camping_app/internal/pms"
	"net/http"
	"time"
)

// HandleIncidents handles listing active incidents and reporting new wilderness incidents.
func (s *Server) HandleIncidents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		campgroundID := r.URL.Query().Get("campground_id")
		list := s.icsMgr.ListActiveIncidents(campgroundID)

		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			s.renderIncidentListHTML(w, list)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)

	case http.MethodPost:
		var req struct {
			Type         string `json:"type"`
			Severity     string `json:"severity"`
			CampgroundID string `json:"campground_id"`
			Sector       string `json:"sector"`
			GPSTrailhead string `json:"gps_trailhead"`
			Description  string `json:"description"`
		}

		if r.Header.Get("Content-Type") == "application/json" {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
				return
			}
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, `{"error":"failed parsing form"}`, http.StatusBadRequest)
				return
			}
			req.Type = r.FormValue("type")
			req.Severity = r.FormValue("severity")
			req.CampgroundID = r.FormValue("campground_id")
			req.Sector = r.FormValue("sector")
			req.GPSTrailhead = r.FormValue("gps_trailhead")
			req.Description = r.FormValue("description")
		}

		if req.CampgroundID == "" {
			req.CampgroundID = "CAMP-PACIFIC-01"
		}

		inc, err := s.icsMgr.ReportIncident(req.Type, req.Severity, req.CampgroundID, req.Sector, req.GPSTrailhead, req.Description)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if r.Header.Get("HX-Request") == "true" {
			list := s.icsMgr.ListActiveIncidents("")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			s.renderIncidentListHTML(w, list)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(inc)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleIncidentDispatch assigns a ranger unit to an incident.
func (s *Server) HandleIncidentDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IncidentID string `json:"incident_id"`
		RangerID   string `json:"ranger_id"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&req)
	} else {
		r.ParseForm()
		req.IncidentID = r.FormValue("incident_id")
		req.RangerID = r.FormValue("ranger_id")
	}

	if req.IncidentID == "" || req.RangerID == "" {
		http.Error(w, `{"error":"incident_id and ranger_id required"}`, http.StatusBadRequest)
		return
	}

	if err := s.icsMgr.DispatchIncident(req.IncidentID, req.RangerID); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		list := s.icsMgr.ListActiveIncidents("")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderIncidentListHTML(w, list)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "dispatched", "incident_id": req.IncidentID, "ranger_id": req.RangerID})
}

// HandleIncidentResolve marks an incident resolved.
func (s *Server) HandleIncidentResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IncidentID string `json:"incident_id"`
		Notes      string `json:"notes"`
	}
	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&req)
	} else {
		r.ParseForm()
		req.IncidentID = r.FormValue("incident_id")
		req.Notes = r.FormValue("notes")
	}

	if err := s.icsMgr.ResolveIncident(req.IncidentID, req.Notes); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		list := s.icsMgr.ListActiveIncidents("")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderIncidentListHTML(w, list)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "resolved", "incident_id": req.IncidentID})
}

// HandleIncidentMuster returns the evacuation muster roll for the campground.
func (s *Server) HandleIncidentMuster(w http.ResponseWriter, r *http.Request) {
	campgroundID := r.URL.Query().Get("campground_id")
	if campgroundID == "" {
		campgroundID = "CAMP-PACIFIC-01"
	}

	musterList := s.icsMgr.GenerateEvacuationMusterList(campgroundID)
	roster := s.pmsAssetMgr.GetInParkRoster()
	campers, vehicles := s.pmsAssetMgr.GetTotalInParkHeadcount()

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
		<div class="p-3 bg-rose-50 border border-rose-300 rounded font-mono text-xs space-y-2">
			<div class="font-bold text-rose-900 text-sm flex items-center justify-between">
				<span>🚨 ICS Evacuation Muster Roll: %s</span>
				<span>%d Campers · %d Vehicles</span>
			</div>
			<div class="text-[11px] text-rose-700">Priority contact order generated across active campsite sectors:</div>
			<ol class="list-decimal list-inside space-y-1 text-[11px] text-stone-800">`,
			templateEscape(campgroundID), campers, vehicles)
		for _, m := range musterList {
			fmt.Fprintf(w, `<li>%s</li>`, templateEscape(m))
		}
		if len(musterList) == 0 {
			fmt.Fprint(w, `<li class="italic text-stone-500">No active campsites occupied in this sector.</li>`)
		}
		fmt.Fprint(w, `</ol></div>`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"campground_id":   campgroundID,
		"total_campers":   campers,
		"total_vehicles":  vehicles,
		"muster_priority": musterList,
		"active_roster":   roster,
	})
}

// HandlePMSAssets lists and reports condition changes on physical campsite infrastructure.
func (s *Server) HandlePMSAssets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		campgroundID := r.URL.Query().Get("campground_id")
		damaged := s.pmsAssetMgr.ListAssetsNeedingRepair(campgroundID)

		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			s.renderAssetRepairListHTML(w, damaged)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(damaged)

	case http.MethodPost:
		var req struct {
			AssetID   string `json:"asset_id"`
			Condition string `json:"condition"`
			Notes     string `json:"notes"`
		}

		if r.Header.Get("Content-Type") == "application/json" {
			json.NewDecoder(r.Body).Decode(&req)
		} else {
			r.ParseForm()
			req.AssetID = r.FormValue("asset_id")
			req.Condition = r.FormValue("condition")
			req.Notes = r.FormValue("notes")
		}

		if req.AssetID == "" || req.Condition == "" {
			http.Error(w, `{"error":"asset_id and condition required"}`, http.StatusBadRequest)
			return
		}

		if err := s.pmsAssetMgr.UpdateAssetCondition(req.AssetID, req.Condition, req.Notes); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if r.Header.Get("HX-Request") == "true" {
			damaged := s.pmsAssetMgr.ListAssetsNeedingRepair("")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			s.renderAssetRepairListHTML(w, damaged)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":    "updated",
			"asset_id":  req.AssetID,
			"condition": req.Condition,
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandlePMSOccupancy returns real-time in-park headcount and occupant roster.
func (s *Server) HandlePMSOccupancy(w http.ResponseWriter, r *http.Request) {
	roster := s.pmsAssetMgr.GetInParkRoster()
	campers, vehicles := s.pmsAssetMgr.GetTotalInParkHeadcount()

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderOccupancyRosterHTML(w, roster, campers, vehicles)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_campers":  campers,
		"total_vehicles": vehicles,
		"occupants":      roster,
	})
}

// HandlePMSCheckIn checks in an arrival at the campsite.
func (s *Server) HandlePMSCheckIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rec pms.OccupancyRecord
	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&rec)
	} else {
		r.ParseForm()
		rec.CampsiteID = r.FormValue("campsite_id")
		rec.ReservationID = r.FormValue("reservation_id")
		rec.GuestName = r.FormValue("guest_name")
		rec.PrimaryPlate = r.FormValue("primary_plate")
		rec.TrailerPlate = r.FormValue("trailer_plate")
		rec.EmergencyPhone = r.FormValue("emergency_phone")
		rec.PartySize = 2
	}

	if rec.CampsiteID == "" || rec.GuestName == "" {
		http.Error(w, `{"error":"campsite_id and guest_name required"}`, http.StatusBadRequest)
		return
	}
	rec.CheckedInAt = time.Now()
	rec.ScheduledCheckOut = time.Now().Add(48 * time.Hour)

	if err := s.pmsAssetMgr.CheckInOccupant(&rec); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		roster := s.pmsAssetMgr.GetInParkRoster()
		campers, vehicles := s.pmsAssetMgr.GetTotalInParkHeadcount()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderOccupancyRosterHTML(w, roster, campers, vehicles)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "checked_in",
		"occupancy": rec,
	})
}

// HandlePMSCheckOut checks out an occupant.
func (s *Server) HandlePMSCheckOut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	campsiteID := r.URL.Query().Get("campsite_id")
	if campsiteID == "" {
		r.ParseForm()
		campsiteID = r.FormValue("campsite_id")
	}

	if err := s.pmsAssetMgr.CheckOutOccupant(campsiteID); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		roster := s.pmsAssetMgr.GetInParkRoster()
		campers, vehicles := s.pmsAssetMgr.GetTotalInParkHeadcount()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderOccupancyRosterHTML(w, roster, campers, vehicles)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "checked_out", "campsite_id": campsiteID})
}

func (s *Server) renderIncidentListHTML(w http.ResponseWriter, list []*incident.IncidentReport) {
	if len(list) == 0 {
		fmt.Fprint(w, `<div class="p-4 bg-emerald-50 border border-emerald-200 rounded text-center text-xs text-emerald-800">
			✅ No active wilderness incidents or hazards reported in this jurisdiction.
		</div>`)
		return
	}

	fmt.Fprint(w, `<div class="space-y-2">`)
	for _, inc := range list {
		sevColor := "bg-amber-100 text-amber-900 border-amber-300"
		if inc.Severity == incident.SeverityImmediateEvacuation || inc.Severity == incident.SeverityEvacuationWarning {
			sevColor = "bg-rose-100 text-rose-900 border-rose-300 font-bold"
		}

		statusBadge := "bg-stone-100 text-stone-700"
		if inc.Status == "DISPATCHED" {
			statusBadge = "bg-blue-100 text-blue-800"
		} else if inc.Status == "CONTAINED" {
			statusBadge = "bg-amber-100 text-amber-800"
		}

		fmt.Fprintf(w, `
		<div class="p-3 bg-white border border-stone-200 rounded text-xs space-y-1.5">
			<div class="flex items-center justify-between">
				<div class="flex items-center space-x-2">
					<span class="px-2 py-0.5 border rounded text-[10px] %s">%s</span>
					<span class="font-bold text-stone-900">%s</span>
				</div>
				<span class="px-2 py-0.5 rounded text-[10px] font-mono %s">%s</span>
			</div>
			<div class="text-[11px] text-stone-600">%s · GPS: <span class="font-mono">%s</span></div>
			<div class="text-xs text-stone-800 font-serif">%s</div>
			<div class="flex items-center justify-between pt-1 border-t border-stone-100 text-[10px] text-stone-400">
				<span>Assigned: <strong>%s</strong></span>
				<div class="space-x-1">
					%s
					%s
				</div>
			</div>
		</div>`,
			sevColor, inc.Severity,
			templateEscape(inc.Type),
			statusBadge, inc.Status,
			templateEscape(inc.Sector), templateEscape(inc.GPSTrailhead),
			templateEscape(inc.Description),
			templateEscape(inc.AssignedRanger),
			func() string {
				if inc.Status == "REPORTED" {
					return fmt.Sprintf(`<button hx-post="/api/v1/incidents/dispatch" hx-vals='{"incident_id":"%s","ranger_id":"RANGER-DELTA-01"}' hx-target="#ics-active-incidents" class="px-2 py-0.5 bg-blue-50 text-blue-700 border border-blue-200 rounded hover:bg-blue-100">Dispatch Unit</button>`, inc.ID)
				}
				return ""
			}(),
			func() string {
				if inc.Status != "RESOLVED" {
					return fmt.Sprintf(`<button hx-post="/api/v1/incidents/resolve" hx-vals='{"incident_id":"%s","notes":"Resolved by field ranger"}' hx-target="#ics-active-incidents" class="px-2 py-0.5 bg-stone-100 text-stone-700 border border-stone-200 rounded hover:bg-stone-200">Resolve</button>`, inc.ID)
				}
				return ""
			}(),
		)
	}
	fmt.Fprint(w, `</div>`)
}

func (s *Server) renderAssetRepairListHTML(w http.ResponseWriter, damaged []*pms.CampsiteAsset) {
	if len(damaged) == 0 {
		fmt.Fprint(w, `<div class="p-4 bg-stone-50 border border-stone-200 rounded text-center text-xs text-stone-500">
			All physical campsite infrastructure (bear boxes, spigots, fire rings) in good condition.
		</div>`)
		return
	}

	fmt.Fprint(w, `<div class="space-y-2">`)
	for _, a := range damaged {
		fmt.Fprintf(w, `
		<div class="flex items-center justify-between p-2.5 bg-amber-50/60 border border-amber-200 rounded text-xs font-mono">
			<div>
				<div class="font-bold text-amber-950">%s · Site: %s</div>
				<div class="text-[10px] text-amber-800">Serial: %s · Last Inspected: %s</div>
			</div>
			<div class="flex items-center space-x-2">
				<span class="px-2 py-0.5 bg-amber-200 text-amber-900 rounded text-[10px] font-bold">%s</span>
				<button hx-post="/api/v1/pms/assets" hx-vals='{"asset_id":"%s","condition":"GOOD","notes":"Repaired and verified"}' hx-target="#pms-damaged-assets" class="px-2 py-1 bg-white text-emerald-800 border border-emerald-300 rounded hover:bg-emerald-50 text-[10px]">
					Mark Repaired
				</button>
			</div>
		</div>`,
			templateEscape(a.AssetClass), templateEscape(a.CampsiteID),
			templateEscape(a.SerialNumber), a.LastInspected.Format("Jan 02"),
			templateEscape(a.ConditionRating),
			templateEscape(a.ID),
		)
	}
	fmt.Fprint(w, `</div>`)
}

func (s *Server) renderOccupancyRosterHTML(w http.ResponseWriter, roster []*pms.OccupancyRecord, campers, vehicles int) {
	fmt.Fprintf(w, `
	<div class="space-y-3">
		<div class="flex items-center justify-between p-2 bg-stone-100 rounded text-xs font-mono font-bold text-stone-800">
			<span>In-Park Headcount: %d Campers</span>
			<span>Registered Vehicles: %d</span>
		</div>`, campers, vehicles)

	if len(roster) == 0 {
		fmt.Fprint(w, `<div class="p-3 bg-stone-50 border border-stone-200 rounded text-center text-xs text-stone-500">
			No active campers checked in to park campsites at this hour.
		</div></div>`)
		return
	}

	fmt.Fprint(w, `<div class="divide-y divide-stone-100 border border-stone-200 rounded bg-white">`)
	for _, r := range roster {
		fmt.Fprintf(w, `
		<div class="p-2.5 flex items-center justify-between text-xs">
			<div>
				<div class="font-bold text-stone-900">%s · <span class="font-mono text-stone-500">%s</span></div>
				<div class="text-[10px] text-stone-500">Plate: %s | Party: %d | Checked In: %s</div>
			</div>
			<div>
				<button hx-post="/api/v1/pms/checkout?campsite_id=%s" hx-target="#pms-occupancy-roster" class="px-2 py-1 text-stone-600 hover:text-stone-900 border border-stone-200 rounded text-[10px]">
					Check Out
				</button>
			</div>
		</div>`,
			templateEscape(r.GuestName), templateEscape(r.CampsiteID),
			templateEscape(r.PrimaryPlate), r.PartySize, r.CheckedInAt.Format("15:04"),
			templateEscape(r.CampsiteID),
		)
	}
	fmt.Fprint(w, `</div></div>`)
}

// Route Binding: GET campground_id -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET campground_id -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET campground_id -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET campsite_id -> 

// Route Binding: GET HX-Request -> 

