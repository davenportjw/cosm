package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cosmscm/cosm/examples/camping_app/internal/sos"
)

// HandleSOSBeaconTrigger registers a new emergency Search & Rescue distress call.
func (s *Server) HandleSOSBeaconTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := s.authenticateUser(w, r)
	userID := "u-anonymous"
	userName := "Backcountry Hiker"
	if user != nil {
		userID = user.ID
		userName = user.FullName
	}

	location := r.FormValue("location")
	if location == "" {
		location = "Wilderness Corridor"
	}

	lat, _ := strconv.ParseFloat(r.FormValue("latitude"), 64)
	if lat == 0 {
		lat = 46.8523
	}
	lon, _ := strconv.ParseFloat(r.FormValue("longitude"), 64)
	if lon == 0 {
		lon = -121.7603
	}

	distressStr := r.FormValue("distress_type")
	var dType sos.DistressType
	switch strings.ToUpper(distressStr) {
	case "LOST":
		dType = sos.DistressLost
	case "WILDFIRE":
		dType = sos.DistressWildfire
	case "INJURY":
		dType = sos.DistressInjury
	case "WILDLIFE":
		dType = sos.DistressWildlife
	default:
		dType = sos.DistressMedical
	}

	desc := r.FormValue("description")
	if desc == "" {
		desc = "Emergency distress beacon dispatched from backcountry"
	}

	beacon, err := s.sosMgr.TriggerBeacon(userID, userName, location, lat, lon, dType, desc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Broadcast alert to Ranger Incident Command Hub
	s.rangerHub.Broadcast("SOS_BEACON_ACTIVATED", fmt.Sprintf(`{"beacon_id":"%s","user":"%s","location":"%s","type":"%s"}`,
		beacon.ID, beacon.UserName, beacon.TrailheadOrSite, beacon.DistressType))

	if strings.Contains(r.Header.Get("Accept"), "application/json") || r.FormValue("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(beacon)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `<div class="p-4 bg-red-600 text-white rounded-xl shadow-lg border border-red-700 animate-pulse">
		<div class="flex items-center space-x-2">
			<span class="text-xl">🚨</span>
			<div class="font-black tracking-wide uppercase">Emergency SOS Beacon Dispatched</div>
		</div>
		<div class="text-sm mt-1">SAR Units and Ranger Operations notified. Beacon ID: <strong>%s</strong></div>
		<div class="text-xs text-red-100 mt-2">Location: %s (%.4f, %.4f) · Status: ACTIVE_SEARCHING</div>
	</div>`, beacon.ID, beacon.TrailheadOrSite, beacon.Latitude, beacon.Longitude)
}

// HandleSOSBeaconList returns all currently active emergency SAR distress beacons.
func (s *Server) HandleSOSBeaconList(w http.ResponseWriter, r *http.Request) {
	active := s.sosMgr.ListActive()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(active)
}

// HandleSOSBeaconResolve marks a resolved emergency incident.
func (s *Server) HandleSOSBeaconResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	beaconID := r.FormValue("beacon_id")
	if beaconID == "" {
		http.Error(w, "beacon_id required", http.StatusBadRequest)
		return
	}

	resolved, err := s.sosMgr.ResolveBeacon(beaconID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	s.rangerHub.Broadcast("SOS_BEACON_RESOLVED", fmt.Sprintf(`{"beacon_id":"%s","status":"RESOLVED"}`, beaconID))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resolved)
}
