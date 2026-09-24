package main

import (
	"encoding/json"
	"fmt"
	"github.com/cosmscm/cosm/examples/camping_app/internal/profile"
	"github.com/cosmscm/cosm/examples/camping_app/internal/ranger"
	"net/http"
	"strings"
	"time"
)

// HandleCamperProfile handles GET and POST requests for camper profiles.
func (s *Server) HandleCamperProfile(w http.ResponseWriter, r *http.Request) {
	user := s.getAuthenticatedUser(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, err := s.profileMgr.GetProfile(user.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)

	case http.MethodPost:
		var req struct {
			FullName              string `json:"full_name"`
			Phone                 string `json:"phone"`
			EmergencyContactName  string `json:"emergency_contact_name"`
			EmergencyContactPhone string `json:"emergency_contact_phone"`
			WildernessPassID      string `json:"wilderness_pass_id"`
			NotificationsEnabled  bool   `json:"notifications_enabled"`
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
			req.FullName = r.FormValue("full_name")
			req.Phone = r.FormValue("phone")
			req.EmergencyContactName = r.FormValue("emergency_contact_name")
			req.EmergencyContactPhone = r.FormValue("emergency_contact_phone")
			req.WildernessPassID = r.FormValue("wilderness_pass_id")
			req.NotificationsEnabled = r.FormValue("notifications_enabled") == "on" || r.FormValue("notifications_enabled") == "true"
		}

		p, _ := s.profileMgr.GetProfile(user.ID)
		if p == nil {
			p = &profile.CamperProfile{
				UserID:    user.ID,
				Vehicles:  make([]profile.VehicleRecord, 0),
				CreatedAt: time.Now().UTC(),
			}
		}
		if req.FullName != "" {
			p.FullName = req.FullName
		} else if p.FullName == "" {
			p.FullName = user.FullName
		}
		p.Phone = req.Phone
		p.EmergencyContactName = req.EmergencyContactName
		p.EmergencyContactPhone = req.EmergencyContactPhone
		p.WildernessPassID = req.WildernessPassID
		p.NotificationsEnabled = req.NotificationsEnabled

		if err := s.profileMgr.UpsertProfile(p); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<div class="p-3 bg-emerald-50 border border-emerald-200 rounded text-emerald-800 text-xs font-mono">
				✅ Profile saved: %s · Emergency Contact: %s (%s)
			</div>`, templateEscape(p.FullName), templateEscape(p.EmergencyContactName), templateEscape(p.EmergencyContactPhone))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleCamperVehicles handles adding and removing vehicles from the user's fleet.
func (s *Server) HandleCamperVehicles(w http.ResponseWriter, r *http.Request) {
	user := s.getAuthenticatedUser(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodPost:
		var v profile.VehicleRecord
		if r.Header.Get("Content-Type") == "application/json" {
			if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
				http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
				return
			}
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, `{"error":"failed parsing form"}`, http.StatusBadRequest)
				return
			}
			v.Plate = r.FormValue("plate")
			v.State = r.FormValue("state")
			v.MakeModel = r.FormValue("make_model")
			v.Color = r.FormValue("color")
			v.IsEV = r.FormValue("is_ev") == "on" || r.FormValue("is_ev") == "true"
			v.IsPrimary = r.FormValue("is_primary") == "on" || r.FormValue("is_primary") == "true"
		}

		if err := s.profileMgr.AddVehicle(user.ID, v); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		// Also register with gate controller for ALPR entry
		normPlate := strings.ToUpper(strings.TrimSpace(v.Plate))
		normState := strings.ToUpper(strings.TrimSpace(v.State))
		s.rangerPMS.RegisterVehicle(
			normPlate,
			normState,
			"FLEET-ACTIVE",
			user.FullName,
			time.Now().Add(-1*time.Hour),
			time.Now().Add(72*time.Hour),
		)

		// Also register in offline gate cache
		s.offlineCache.LoadPermits(map[string]string{
			fmt.Sprintf("%s:%s", normPlate, normState): fmt.Sprintf("Fleet-Permit (%s)", user.FullName),
		})

		if r.Header.Get("HX-Request") == "true" {
			vehicles := s.profileMgr.GetVehicles(user.ID)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			s.renderVehicleListHTML(w, vehicles)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "created",
			"vehicle": v,
		})

	case http.MethodDelete:
		plate := r.URL.Query().Get("plate")
		state := r.URL.Query().Get("state")
		if plate == "" {
			var body struct {
				Plate string `json:"plate"`
				State string `json:"state"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				plate = body.Plate
				state = body.State
			}
		}
		if plate == "" {
			http.Error(w, `{"error":"plate parameter required"}`, http.StatusBadRequest)
			return
		}

		if err := s.profileMgr.RemoveVehicle(user.ID, plate, state); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if r.Header.Get("HX-Request") == "true" {
			vehicles := s.profileMgr.GetVehicles(user.ID)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			s.renderVehicleListHTML(w, vehicles)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "plate": plate})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleGateScan executes a multi-lane ALPR camera scan with barrier control.
func (s *Server) HandleGateScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Plate string `json:"plate"`
		State string `json:"state"`
		Lane  string `json:"lane"`
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
		req.Plate = r.FormValue("plate")
		req.State = r.FormValue("state")
		req.Lane = r.FormValue("lane")
	}

	if req.Plate == "" {
		http.Error(w, `{"error":"license plate is required"}`, http.StatusBadRequest)
		return
	}
	if req.Lane == "" {
		req.Lane = ranger.LaneStandard
	}

	logEntry, authorized, reason := s.gateCtrl.AuthorizeGateEntryWithLane(req.Plate, req.State, req.Lane)

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderGateScanResultHTML(w, logEntry, authorized, reason)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"authorized":     authorized,
		"reason":         reason,
		"barrier_state":  s.gateCtrl.State(),
		"gate_log_entry": logEntry,
	})
}

// HandleGateLogs returns the recent gate access audit log.
func (s *Server) HandleGateLogs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	logs := s.gateCtrl.GetRecentGateLogs(limit)

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderGateLogsTableHTML(w, logs)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// HandleGateReset manually resets the barrier arm to LOWERED.
func (s *Server) HandleGateReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := s.gateCtrl.ResetBarrier()

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div class="p-2 bg-stone-100 border border-stone-300 rounded text-stone-700 text-xs font-mono">
			Barrier Arm Reset: <span class="font-bold">%s</span>
		</div>`, state)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":        "reset",
		"barrier_state": state,
	})
}

func (s *Server) renderVehicleListHTML(w http.ResponseWriter, vehicles []profile.VehicleRecord) {
	if len(vehicles) == 0 {
		fmt.Fprint(w, `<div class="p-4 bg-stone-50 border border-stone-200 rounded text-center text-xs text-stone-500">
			No vehicles registered in fleet yet. Add a vehicle below to enable automated ALPR gate clearance.
		</div>`)
		return
	}

	fmt.Fprint(w, `<div class="space-y-2">`)
	for _, v := range vehicles {
		primaryBadge := ""
		if v.IsPrimary {
			primaryBadge = `<span class="px-1.5 py-0.5 bg-amber-100 text-amber-800 text-[10px] font-bold rounded">PRIMARY</span>`
		}
		evBadge := ""
		if v.IsEV {
			evBadge = `<span class="px-1.5 py-0.5 bg-emerald-100 text-emerald-800 text-[10px] font-bold rounded">EV</span>`
		}

		fmt.Fprintf(w, `
		<div class="flex items-center justify-between p-3 bg-white border border-stone-200 rounded text-xs">
			<div class="flex items-center space-x-3">
				<div class="font-mono font-bold text-sm bg-stone-100 px-2.5 py-1 border border-stone-300 rounded text-stone-900">
					%s <span class="text-[10px] text-stone-500 font-normal">%s</span>
				</div>
				<div>
					<div class="font-medium text-stone-800 flex items-center space-x-2">
						<span>%s (%s)</span>
						%s %s
					</div>
					<div class="text-[10px] text-stone-400">Added: %s</div>
				</div>
			</div>
			<div>
				<button hx-delete="/api/v1/profile/vehicles?plate=%s&state=%s" hx-target="#fleet-vehicles-list" class="text-[11px] text-rose-700 hover:text-rose-900 px-2.5 py-1 border border-rose-200 rounded hover:bg-rose-50 font-medium">
					Remove
				</button>
			</div>
		</div>`,
			templateEscape(v.Plate), templateEscape(v.State),
			templateEscape(v.MakeModel), templateEscape(v.Color),
			primaryBadge, evBadge,
			v.AddedAt.Format("Jan 02, 2006"),
			templateEscape(v.Plate), templateEscape(v.State),
		)
	}
	fmt.Fprint(w, `</div>`)
}

func (s *Server) renderGateScanResultHTML(w http.ResponseWriter, logEntry *ranger.GateLogEntry, authorized bool, reason string) {
	badgeClass := "bg-rose-100 text-rose-800 border-rose-300"
	badgeIcon := "❌ DENIED"
	if authorized {
		badgeClass = "bg-emerald-100 text-emerald-800 border-emerald-300"
		badgeIcon = "✅ AUTHORIZED"
	}

	fmt.Fprintf(w, `
	<div class="p-3 border rounded font-mono text-xs space-y-1.5 %s">
		<div class="flex items-center justify-between">
			<span class="font-bold text-sm">%s</span>
			<span class="px-2 py-0.5 rounded text-[10px] font-bold uppercase bg-white/60">%s</span>
		</div>
		<div class="text-[11px]">Plate: <strong>%s (%s)</strong> · Lane: <strong>%s</strong></div>
		<div class="text-[11px]">Reason: %s</div>
		<div class="text-[10px] text-stone-600 flex items-center justify-between border-t border-stone-200/50 pt-1">
			<span>Barrier Action: %s</span>
			<span>Current State: %s</span>
		</div>
	</div>`,
		badgeClass, badgeIcon, logEntry.Lane,
		templateEscape(logEntry.Plate), templateEscape(logEntry.State),
		templateEscape(logEntry.Lane),
		templateEscape(reason),
		templateEscape(logEntry.BarrierAction),
		s.gateCtrl.State(),
	)
}

func (s *Server) renderGateLogsTableHTML(w http.ResponseWriter, logs []*ranger.GateLogEntry) {
	if len(logs) == 0 {
		fmt.Fprint(w, `<div class="p-4 bg-stone-50 border border-stone-200 rounded text-center text-xs text-stone-500">
			No gatehouse access events recorded yet.
		</div>`)
		return
	}

	fmt.Fprint(w, `
	<div class="overflow-x-auto">
		<table class="w-full text-left text-xs font-mono">
			<thead class="bg-stone-100 text-stone-600 border-b border-stone-200">
				<tr>
					<th class="p-2">Time</th>
					<th class="p-2">Plate</th>
					<th class="p-2">Lane</th>
					<th class="p-2">Status</th>
					<th class="p-2">Action</th>
					<th class="p-2">Notes</th>
				</tr>
			</thead>
			<tbody class="divide-y divide-stone-100">`)
	for _, l := range logs {
		statusColor := "text-rose-700"
		statusText := "DENIED"
		if l.Authorized {
			statusColor = "text-emerald-700 font-bold"
			statusText = "GRANTED"
		}
		fmt.Fprintf(w, `
				<tr class="hover:bg-stone-50">
					<td class="p-2 text-[10px] text-stone-500">%s</td>
					<td class="p-2 font-bold">%s <span class="text-[10px] text-stone-400 font-normal">%s</span></td>
					<td class="p-2 text-[10px]">%s</td>
					<td class="p-2 %s">%s</td>
					<td class="p-2 text-[10px]">%s</td>
					<td class="p-2 text-[10px] text-stone-600 truncate max-w-[180px]">%s</td>
				</tr>`,
			l.Timestamp.Format("15:04:05"),
			templateEscape(l.Plate), templateEscape(l.State),
			templateEscape(l.Lane),
			statusColor, statusText,
			templateEscape(l.BarrierAction),
			templateEscape(l.Reason),
		)
	}
	fmt.Fprint(w, `
			</tbody>
		</table>
	</div>`)
}

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET plate -> 

// Route Binding: GET state -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Request -> 

