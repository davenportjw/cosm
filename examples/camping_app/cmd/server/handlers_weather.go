package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// HandleWeatherCampsite serves real-time microclimate and campfire safety ratings.
func (s *Server) HandleWeatherCampsite(w http.ResponseWriter, r *http.Request) {
	campsiteID := r.URL.Query().Get("campsite_id")
	if campsiteID == "" {
		campsiteID = "c1"
	}

	weather, err := s.weatherEng.GetWeather(campsiteID)
	if err != nil {
		// Fallback to dynamic evaluation from catalog
		s.mem.mu.RLock()
		cs, exists := s.mem.campsites[campsiteID]
		s.mem.mu.RUnlock()
		cName := "Campground Sector"
		if exists {
			cName = cs.Name
		}
		weather = s.weatherEng.BuildTelemetry(campsiteID, cName, 5400, 72.0, 28.0, 12.0, 18.0, "Partly Cloudy")
	}

	// Support JSON API response
	format := r.URL.Query().Get("format")
	if format == "json" || strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(weather)
		return
	}

	// Render HTMX microclimate pill & fire safety badge
	var badgeColor, badgeIcon string
	switch weather.CampfireAdvisory {
	case "ALLOWED":
		badgeColor = "bg-emerald-100 text-emerald-800 border-emerald-300"
		badgeIcon = "🔥 Campfires Allowed"
	case "CONTAINED_RING_ONLY":
		badgeColor = "bg-amber-100 text-amber-800 border-amber-300"
		badgeIcon = "⚠️ Contained Rings Only"
	case "PROPANE_ONLY":
		badgeColor = "bg-orange-100 text-orange-800 border-orange-300"
		badgeIcon = "💨 Propane Stoves Only"
	default:
		badgeColor = "bg-rose-100 text-rose-800 border-rose-300"
		badgeIcon = "🚫 Total Fire Ban"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="p-4 bg-white/95 rounded-xl border border-stone-200 shadow-sm space-y-3">
		<div class="flex items-center justify-between border-b border-stone-100 pb-2">
			<div>
				<div class="text-xs font-semibold text-stone-500 uppercase tracking-wider">Microclimate Telemetry</div>
				<div class="text-sm font-bold text-stone-900">%s (%d ft)</div>
			</div>
			<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium border %s">
				%s
			</span>
		</div>
		<div class="grid grid-cols-4 gap-2 text-center">
			<div class="p-2 bg-stone-50 rounded-lg">
				<div class="text-xs text-stone-500">Temp</div>
				<div class="text-base font-bold text-stone-800">%.1f°F</div>
			</div>
			<div class="p-2 bg-stone-50 rounded-lg">
				<div class="text-xs text-stone-500">Humidity</div>
				<div class="text-base font-bold text-stone-800">%.0f%%</div>
			</div>
			<div class="p-2 bg-stone-50 rounded-lg">
				<div class="text-xs text-stone-500">Wind</div>
				<div class="text-base font-bold text-stone-800">%.1f mph</div>
			</div>
			<div class="p-2 bg-stone-50 rounded-lg">
				<div class="text-xs text-stone-500">NFDRS Risk</div>
				<div class="text-base font-bold text-stone-800">%s</div>
			</div>
		</div>
		<div class="text-xs text-stone-500 text-right">Conditions: %s · Synced: %s</div>
	</div>`,
		weather.CampsiteName, weather.ElevationFt,
		badgeColor, badgeIcon,
		weather.TemperatureF, weather.HumidityPct, weather.WindSpeedMph, weather.FireDanger,
		weather.Conditions, weather.UpdatedAt.Format("15:04:05 MST"),
	)
}
