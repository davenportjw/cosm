package main

import (
	"encoding/json"
	"fmt"
	"github.com/cosmscm/cosm/examples/camping_app/internal/sync"
	"github.com/cosmscm/cosm/examples/camping_app/internal/telemetry"
	"net/http"
	"strconv"
)

// HandleTelemetryMesh returns live readings from the elevation sensor mesh.
func (s *Server) HandleTelemetryMesh(w http.ResponseWriter, r *http.Request) {
	readings := s.sensorMesh.GetLatestReadings()

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderSensorMeshHTML(w, readings)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(readings)
}

// HandleTelemetryRothermel calculates surface fire rate of spread using USFS Rothermel (1972) equations.
func (s *Server) HandleTelemetryRothermel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FuelModelNumber int     `json:"fuel_model_number"`
		FuelMoisturePct float64 `json:"fuel_moisture_pct"`
		WindSpeedMph    float64 `json:"wind_speed_mph"`
		SlopeDeg        float64 `json:"slope_deg"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&req)
	} else {
		r.ParseForm()
		req.FuelModelNumber, _ = strconv.Atoi(r.FormValue("fuel_model_number"))
		req.FuelMoisturePct, _ = strconv.ParseFloat(r.FormValue("fuel_moisture_pct"), 64)
		req.WindSpeedMph, _ = strconv.ParseFloat(r.FormValue("wind_speed_mph"), 64)
		req.SlopeDeg, _ = strconv.ParseFloat(r.FormValue("slope_deg"), 64)
	}

	if req.FuelModelNumber <= 0 {
		req.FuelModelNumber = 1 // Standard Short Grass
	}
	if req.FuelMoisturePct <= 0 {
		req.FuelMoisturePct = 8.0 // 8% moisture
	}

	fuel, ok := telemetry.StandardModels[req.FuelModelNumber]
	if !ok {
		fuel = telemetry.StandardModels[1]
	}

	ros, fl, intBtu := telemetry.CalculateRothermelRateOfSpread(fuel, req.FuelMoisturePct, req.WindSpeedMph, req.SlopeDeg)

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
		<div class="p-3 bg-stone-900 text-amber-400 rounded font-mono text-xs space-y-1.5 border border-stone-800">
			<div class="flex items-center justify-between">
				<span class="font-bold text-stone-100">🔥 Rothermel Surface Fire Spread: Fuel Model %d (%s)</span>
				<span class="text-xs px-2 py-0.5 bg-amber-500/20 text-amber-300 rounded font-bold">R = %.2f ft/min</span>
			</div>
			<div class="grid grid-cols-3 gap-2 text-[11px] pt-1 text-stone-300 border-t border-stone-800">
				<div>Spread Rate: <strong class="text-amber-400">%.2f ft/min</strong></div>
				<div>Flame Length: <strong class="text-amber-400">%.2f ft</strong></div>
				<div>Fireline Intensity: <strong class="text-amber-400">%.2f Btu/ft-s</strong></div>
			</div>
			<div class="text-[10px] text-stone-400">Inputs: Wind=%.1f mph · Slope=%.1f° · Fuel Moisture=%.1f%%</div>
		</div>`, req.FuelModelNumber, templateEscape(fuel.FuelName), ros, ros, fl, intBtu, req.WindSpeedMph, req.SlopeDeg, req.FuelMoisturePct)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"fuel_model":             fuel.FuelName,
		"rate_of_spread_ft_min":  ros,
		"flame_length_ft":        fl,
		"fire_intensity_btu_sec": intBtu,
	})
}

// HandleTelemetryBriefing synthesizes a tactical ranger fire briefing via Vertex AI Gemini 3.8 Flash (or deterministic fallback).
func (s *Server) HandleTelemetryBriefing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	readings := s.sensorMesh.GetLatestReadings()
	activeIncidents := len(s.icsMgr.ListActiveIncidents(""))
	inboundVehicles := 7

	briefing, err := telemetry.GenerateRangerBriefing(readings, activeIncidents, inboundVehicles)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
		<div class="p-3 bg-stone-900 text-stone-200 border border-stone-800 rounded font-mono text-xs space-y-2">
			<div class="flex items-center justify-between pb-1 border-b border-stone-800">
				<span class="font-bold text-amber-400">🤖 %s Tactical Briefing</span>
				<span class="text-[10px] text-stone-400">%s</span>
			</div>
			<div class="p-2 bg-stone-950 border border-stone-800 rounded text-amber-300 font-bold text-xs">
				%s
			</div>
			<div class="text-[11px] font-sans prose prose-invert max-w-none text-stone-300 whitespace-pre-wrap">
%s
			</div>
		</div>`,
			templateEscape(briefing.ModelUsed),
			briefing.GeneratedAt.Format("15:04:05 UTC"),
			templateEscape(briefing.EvacuationAdvice),
			templateEscape(briefing.SummaryMarkdown),
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(briefing)
}

// HandleSyncWAL returns the offline gate cache WAL entries.
func (s *Server) HandleSyncWAL(w http.ResponseWriter, r *http.Request) {
	records, err := s.offlineCache.ReplayWAL()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderWALRecordsHTML(w, records)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

// HandleSyncAuthorizeOffline executes a disconnected gate clearance check against the offline cache.
func (s *Server) HandleSyncAuthorizeOffline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Plate string `json:"plate"`
		State string `json:"state"`
	}

	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&req)
	} else {
		r.ParseForm()
		req.Plate = r.FormValue("plate")
		req.State = r.FormValue("state")
	}

	granted, reason := s.offlineCache.AuthorizeOffline(req.Plate, req.State)

	if r.Header.Get("HX-Request") == "true" {
		records, _ := s.offlineCache.ReplayWAL()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		badge := "❌ OFFLINE DENIED"
		badgeClass := "bg-rose-100 text-rose-900 border-rose-300"
		if granted {
			badge = "✅ OFFLINE GRANTED"
			badgeClass = "bg-emerald-100 text-emerald-900 border-emerald-300"
		}
		fmt.Fprintf(w, `
		<div class="space-y-2">
			<div class="p-2.5 border rounded font-mono text-xs %s flex items-center justify-between">
				<span>%s: %s (%s)</span>
				<span class="text-[10px]">%s</span>
			</div>`, badgeClass, badge, templateEscape(req.Plate), templateEscape(req.State), templateEscape(reason))
		s.renderWALRecordsHTML(w, records)
		fmt.Fprint(w, `</div>`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"granted": granted,
		"reason":  reason,
	})
}

// HandleSyncReconcile reconciles offline gatehouse WAL records with cloud event streams.
func (s *Server) HandleSyncReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cloudEvents []sync.WALRecord
	if r.Header.Get("Content-Type") == "application/json" {
		json.NewDecoder(r.Body).Decode(&cloudEvents)
	}

	applied, conflicts := s.offlineCache.ReconcileWithCloud(cloudEvents)

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div class="p-2.5 bg-emerald-50 border border-emerald-300 rounded font-mono text-xs text-emerald-900">
			🔄 Reconciled with Cloud Hub: Applied %d records, Resolved %d conflict(s).
		</div>`, applied, conflicts)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"applied_count":  applied,
		"conflict_count": conflicts,
	})
}

func (s *Server) renderSensorMeshHTML(w http.ResponseWriter, readings []telemetry.SensorReading) {
	fmt.Fprint(w, `<div class="grid grid-cols-1 md:grid-cols-3 gap-3 font-mono text-xs">`)
	for _, r := range readings {
		fmt.Fprintf(w, `
		<div class="p-3 bg-white border border-stone-200 rounded space-y-1.5">
			<div class="flex items-center justify-between font-bold text-stone-900">
				<span>%s</span>
				<span class="text-stone-500 text-[10px]">%.0f ft</span>
			</div>
			<div class="grid grid-cols-2 gap-1 text-[11px] text-stone-600">
				<div>Temp: <strong>%.1f°F</strong></div>
				<div>Humidity: <strong>%.0f%%</strong></div>
				<div>Wind: <strong>%.1f mph</strong></div>
				<div>10-Hr Fuel: <strong class="text-amber-700">%.1f%%</strong></div>
			</div>
			<div class="text-[9px] text-stone-400 pt-1 border-t border-stone-100">Last Synced: %s</div>
		</div>`,
			templateEscape(r.SensorID), r.ElevationFt,
			r.TemperatureF, r.HumidityPct,
			r.WindSpeedMph, r.FuelMoisturePct,
			r.Timestamp.Format("15:04:05"),
		)
	}
	fmt.Fprint(w, `</div>`)
}

func (s *Server) renderWALRecordsHTML(w http.ResponseWriter, records []sync.WALRecord) {
	if len(records) == 0 {
		fmt.Fprint(w, `<div class="p-2 bg-stone-50 border border-stone-200 rounded text-center text-[11px] text-stone-400 font-mono">
			No offline WAL records pending sync.
		</div>`)
		return
	}

	fmt.Fprint(w, `
	<div class="border border-stone-200 rounded overflow-x-auto">
		<table class="w-full text-left font-mono text-[10px]">
			<thead class="bg-stone-100 text-stone-600 border-b border-stone-200">
				<tr>
					<th class="p-1.5">Seq</th>
					<th class="p-1.5">Entity</th>
					<th class="p-1.5">Op</th>
					<th class="p-1.5">CRC32</th>
					<th class="p-1.5">Data</th>
				</tr>
			</thead>
			<tbody class="divide-y divide-stone-100 bg-white">`)
	for _, r := range records {
		fmt.Fprintf(w, `
				<tr>
					<td class="p-1.5 text-stone-500">#%d</td>
					<td class="p-1.5 font-bold">%s</td>
					<td class="p-1.5 text-blue-700 font-bold">%s</td>
					<td class="p-1.5 text-stone-400">0x%08X</td>
					<td class="p-1.5 text-stone-600 truncate max-w-[200px]">%s</td>
				</tr>`,
			r.Sequence, templateEscape(r.EntityID),
			templateEscape(r.Operation), r.CRC32,
			templateEscape(r.DataJSON),
		)
	}
	fmt.Fprint(w, `
			</tbody>
		</table>
	</div>`)
}

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

// Route Binding: GET Content-Type -> 

// Route Binding: GET HX-Request -> 

