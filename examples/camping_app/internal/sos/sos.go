package sos

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// DistressType specifies emergency incident classification.
type DistressType string

const (
	DistressMedical  DistressType = "MEDICAL"
	DistressLost     DistressType = "LOST"
	DistressWildfire DistressType = "WILDFIRE"
	DistressInjury   DistressType = "INJURY"
	DistressWildlife DistressType = "WILDLIFE"
)

// BeaconStatus tracks active SAR (Search and Rescue) lifecycle.
type BeaconStatus string

const (
	StatusActiveSearching BeaconStatus = "ACTIVE_SEARCHING"
	StatusDispatched      BeaconStatus = "DISPATCHED"
	StatusRescueUnderway  BeaconStatus = "RESCUE_UNDERWAY"
	StatusResolved        BeaconStatus = "RESOLVED"
)

// SOSBeacon represents a high-priority distress beacon emitted from the wilderness.
type SOSBeacon struct {
	ID             string       `json:"id"`
	UserID         string       `json:"user_id"`
	UserName       string       `json:"user_name"`
	TrailheadOrSite string      `json:"trailhead_or_site"`
	Latitude       float64      `json:"latitude"`
	Longitude      float64      `json:"longitude"`
	DistressType   DistressType `json:"distress_type"`
	Description    string       `json:"description"`
	ReportedAt     time.Time    `json:"reported_at"`
	Status         BeaconStatus `json:"status"`
	AssignedRanger string       `json:"assigned_ranger,omitempty"`
	ResolvedAt     *time.Time   `json:"resolved_at,omitempty"`
}

// SOSManager coordinates emergency beacons across rangers and SAR units.
type SOSManager struct {
	mu      sync.RWMutex
	beacons map[string]*SOSBeacon
}

// NewSOSManager initializes an active emergency beacon coordinator.
func NewSOSManager() *SOSManager {
	mgr := &SOSManager{
		beacons: make(map[string]*SOSBeacon),
	}
	// Seed one historical resolved drill beacon
	drillID := "sos-drill-001"
	resolvedTime := time.Now().Add(-2 * time.Hour)
	mgr.beacons[drillID] = &SOSBeacon{
		ID:              drillID,
		UserID:          "u-ranger-lead",
		UserName:        "Lead Ranger Miller",
		TrailheadOrSite: "Wonderland Pass Trailhead",
		Latitude:        46.8523,
		Longitude:       -121.7603,
		DistressType:    DistressMedical,
		Description:     "Scheduled SAR protocol readiness exercise",
		ReportedAt:      time.Now().Add(-3 * time.Hour),
		Status:          StatusResolved,
		AssignedRanger:  "Unit Echo-1",
		ResolvedAt:      &resolvedTime,
	}
	return mgr
}

func generateID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}

// TriggerBeacon registers a new emergency beacon in the system.
func (m *SOSManager) TriggerBeacon(userID, userName, location string, lat, lon float64, dType DistressType, desc string) (*SOSBeacon, error) {
	if location == "" {
		return nil, fmt.Errorf("location is required for emergency dispatch")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	beacon := &SOSBeacon{
		ID:              generateID("sos"),
		UserID:          userID,
		UserName:        userName,
		TrailheadOrSite: location,
		Latitude:        lat,
		Longitude:       lon,
		DistressType:    dType,
		Description:     desc,
		ReportedAt:      time.Now().UTC(),
		Status:          StatusActiveSearching,
	}

	m.beacons[beacon.ID] = beacon
	return beacon, nil
}

// AssignRanger updates beacon status and assigns a responding ranger.
func (m *SOSManager) AssignRanger(id, rangerName string) (*SOSBeacon, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, exists := m.beacons[id]
	if !exists {
		return nil, fmt.Errorf("beacon %q not found", id)
	}

	b.Status = StatusDispatched
	b.AssignedRanger = rangerName
	return b, nil
}

// ResolveBeacon marks an emergency distress incident as resolved.
func (m *SOSManager) ResolveBeacon(id string) (*SOSBeacon, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, exists := m.beacons[id]
	if !exists {
		return nil, fmt.Errorf("beacon %q not found", id)
	}

	now := time.Now().UTC()
	b.Status = StatusResolved
	b.ResolvedAt = &now
	return b, nil
}

// GetBeacon returns an emergency beacon by ID.
func (m *SOSManager) GetBeacon(id string) (*SOSBeacon, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	b, exists := m.beacons[id]
	if !exists {
		return nil, fmt.Errorf("beacon %q not found", id)
	}
	return b, nil
}

// ListActive returns all unresolved emergency distress beacons.
func (m *SOSManager) ListActive() []*SOSBeacon {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var active []*SOSBeacon
	for _, b := range m.beacons {
		if b.Status != StatusResolved {
			active = append(active, b)
		}
	}
	return active
}
