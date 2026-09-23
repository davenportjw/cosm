package incident

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// IncidentReport records the state, severity, sector, and lifecycle of an ICS incident.
type IncidentReport struct {
	ID string `json:"id"`
	Type string `json:"type"`
	Severity string `json:"severity"`
	CampgroundID string `json:"campground_id"`
	Sector string `json:"sector"`
	GPSTrailhead string `json:"gps_trailhead"`
	Description string `json:"description"`
	Status string `json:"status"`
	AssignedRanger string `json:"assigned_ranger,omitempty"`
	Notes string `json:"notes,omitempty"`
	ReportedAt time.Time `json:"reported_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// EventBroadcaster manages thread-safe Server-Sent Events (SSE) subscriptions for real-time alerts.
type EventBroadcaster struct {
	mu sync.RWMutex
	subscribers map[chan string]struct{}
}

// ICSManager coordinates wildland incident lifecycle, dispatches, containment, and evacuation directives.
type ICSManager struct {
	mu sync.RWMutex
	incidents map[string]*IncidentReport
	broadcaster *EventBroadcaster
}

// NewEventBroadcaster initializes an EventBroadcaster instance.
func NewEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{
		subscribers: make(map[chan string]struct{}),
	}
}

// Subscribe returns an SSE formatted message channel.
func (b *EventBroadcaster) Subscribe() chan string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string, 128)
	b.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe closes and unregisters the subscriber channel.
func (b *EventBroadcaster) Unsubscribe(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.subscribers[ch]; exists {
		delete(b.subscribers, ch)
		close(ch)
	}
}

// Broadcast sends an SSE-formatted event to all registered channels.
func (b *EventBroadcaster) Broadcast(event string, payload interface{}) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(data))
	for ch := range b.subscribers {
		select {
		case ch <- msg:
		default:
			// Non-blocking drop on full subscriber buffer
		}
	}
}

// SubscriberCount returns the current active subscriber count.
func (b *EventBroadcaster) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

// NewICSManager constructs and returns an initialized ICSManager.
func NewICSManager() *ICSManager {
	return &ICSManager{
		incidents:   make(map[string]*IncidentReport),
		broadcaster: NewEventBroadcaster(),
	}
}

// Broadcaster returns the underlying SSE event broadcaster.
func (m *ICSManager) Broadcaster() *EventBroadcaster {
	return m.broadcaster
}

// Subscribe registers for SSE incident updates.
func (m *ICSManager) Subscribe() chan string {
	return m.broadcaster.Subscribe()
}

// Unsubscribe unregisters an SSE subscriber.
func (m *ICSManager) Unsubscribe(ch chan string) {
	m.broadcaster.Unsubscribe(ch)
}

// SeverityRank converts severity strings to an integer priority (higher number = more severe).
func SeverityRank(s string) int {
	switch s {
	case SeverityImmediateEvacuation:
		return 4
	case SeverityEvacuationWarning:
		return 3
	case SeverityAlert:
		return 2
	case SeverityAdvisory:
		return 1
	default:
		return 0
	}
}

// ReportIncident creates and files a new incident in REPORTED status.
func (m *ICSManager) ReportIncident(incType, severity, campgroundID, sector, gps, description string) (*IncidentReport, error) {
	if incType == "" {
		return nil, errors.New("incident type is required")
	}
	if severity == "" {
		return nil, errors.New("severity is required")
	}
	if campgroundID == "" {
		return nil, errors.New("campground ID is required")
	}
	if sector == "" {
		return nil, errors.New("sector is required")
	}
	if description == "" {
		return nil, errors.New("description is required")
	}

	randBytes := make([]byte, 6)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, fmt.Errorf("failed to generate incident id: %w", err)
	}
	id := fmt.Sprintf("INC-%s", hex.EncodeToString(randBytes))

	inc := &IncidentReport{
		ID:           id,
		Type:         incType,
		Severity:     severity,
		CampgroundID: campgroundID,
		Sector:       sector,
		GPSTrailhead: gps,
		Description:  description,
		Status:       StatusReported,
		ReportedAt:   time.Now().UTC(),
	}

	m.mu.Lock()
	m.incidents[id] = inc
	m.mu.Unlock()

	m.broadcaster.Broadcast("incident_reported", inc)

	reportCopy := *inc
	return &reportCopy, nil
}

// DispatchIncident assigns a ranger to an incident and transitions status to DISPATCHED.
func (m *ICSManager) DispatchIncident(id, rangerID string) error {
	if id == "" {
		return errors.New("incident id is required")
	}
	if rangerID == "" {
		return errors.New("ranger id is required")
	}

	m.mu.Lock()
	inc, exists := m.incidents[id]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("incident %s not found", id)
	}
	if inc.Status == StatusResolved {
		m.mu.Unlock()
		return fmt.Errorf("cannot dispatch resolved incident %s", id)
	}

	inc.AssignedRanger = rangerID
	inc.Status = StatusDispatched
	incCopy := *inc
	m.mu.Unlock()

	m.broadcaster.Broadcast("incident_dispatched", &incCopy)
	return nil
}

// ContainIncident sets status to CONTAINED with containment field notes.
func (m *ICSManager) ContainIncident(id, containmentNotes string) error {
	if id == "" {
		return errors.New("incident id is required")
	}

	m.mu.Lock()
	inc, exists := m.incidents[id]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("incident %s not found", id)
	}
	if inc.Status == StatusResolved {
		m.mu.Unlock()
		return fmt.Errorf("cannot contain resolved incident %s", id)
	}

	inc.Status = StatusContained
	if containmentNotes != "" {
		if inc.Notes != "" {
			inc.Notes = inc.Notes + "; Containment: " + containmentNotes
		} else {
			inc.Notes = "Containment: " + containmentNotes
		}
	}
	incCopy := *inc
	m.mu.Unlock()

	m.broadcaster.Broadcast("incident_contained", &incCopy)
	return nil
}

// ResolveIncident marks an incident as RESOLVED with final resolution notes and timestamp.
func (m *ICSManager) ResolveIncident(id, notes string) error {
	if id == "" {
		return errors.New("incident id is required")
	}

	m.mu.Lock()
	inc, exists := m.incidents[id]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("incident %s not found", id)
	}
	if inc.Status == StatusResolved {
		m.mu.Unlock()
		return fmt.Errorf("incident %s is already resolved", id)
	}

	now := time.Now().UTC()
	inc.Status = StatusResolved
	inc.ResolvedAt = &now
	if notes != "" {
		if inc.Notes != "" {
			inc.Notes = inc.Notes + "; Resolution: " + notes
		} else {
			inc.Notes = "Resolution: " + notes
		}
	}
	incCopy := *inc
	m.mu.Unlock()

	m.broadcaster.Broadcast("incident_resolved", &incCopy)
	return nil
}

// GetIncident returns a copy of a specific incident by ID.
func (m *ICSManager) GetIncident(id string) (*IncidentReport, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	inc, exists := m.incidents[id]
	if !exists {
		return nil, fmt.Errorf("incident %s not found", id)
	}
	incCopy := *inc
	return &incCopy, nil
}

// ListActiveIncidents returns all un-resolved incidents for a campground (or all campgrounds if empty),
// sorted by severity descending (highest hazard first), then reported timestamp descending.
func (m *ICSManager) ListActiveIncidents(campgroundID string) []*IncidentReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var active []*IncidentReport
	for _, inc := range m.incidents {
		if inc.Status == StatusResolved {
			continue
		}
		if campgroundID != "" && inc.CampgroundID != campgroundID {
			continue
		}
		cp := *inc
		active = append(active, &cp)
	}

	sort.Slice(active, func(i, j int) bool {
		rankI := SeverityRank(active[i].Severity)
		rankJ := SeverityRank(active[j].Severity)
		if rankI != rankJ {
			return rankI > rankJ // Higher severity first
		}
		if !active[i].ReportedAt.Equal(active[j].ReportedAt) {
			return active[i].ReportedAt.After(active[j].ReportedAt)
		}
		return active[i].ID < active[j].ID
	})

	return active
}

// GenerateEvacuationMusterList generates prioritized muster and evacuation directives
// for active hazardous incidents in the given campground.
func (m *ICSManager) GenerateEvacuationMusterList(campgroundID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var directives []string
	for _, inc := range m.incidents {
		if inc.Status == StatusResolved {
			continue
		}
		if campgroundID != "" && inc.CampgroundID != campgroundID {
			continue
		}

		// Directives apply to all active alert and evacuation-level incidents
		if inc.Severity == SeverityImmediateEvacuation || inc.Severity == SeverityEvacuationWarning || inc.Severity == SeverityAlert {
			action := "Prepare Evacuation Perimeter"
			if inc.Severity == SeverityImmediateEvacuation {
				action = "Mandatory Immediate Evacuation & Roll Call"
			} else if inc.Severity == SeverityEvacuationWarning {
				action = "Pre-Evacuation Warning & Staging"
			}

			directive := fmt.Sprintf("[%s] Campground %s | Sector: %s | Incident: %s (%s) | Status: %s | Action: %s | Trailhead: %s",
				inc.Severity, inc.CampgroundID, inc.Sector, inc.ID, inc.Type, inc.Status, action, inc.GPSTrailhead)
			directives = append(directives, directive)
		}
	}

	sort.Slice(directives, func(i, j int) bool {
		return directives[i] < directives[j]
	})

	return directives
}

// Incident Types
const (
	IncidentBearSighting    = "BEAR_SIGHTING"
	IncidentFoodConditioned = "FOOD_CONDITIONED_ANIMAL"
	IncidentTrailWashout    = "TRAIL_WASHOUT"
	IncidentSearchAndRescue = "SEARCH_AND_RESCUE"
	IncidentSmokeHazard     = "SMOKE_HAZARD"
)
// Incident Severities
const (
	SeverityAdvisory            = "ADVISORY"
	SeverityAlert               = "ALERT"
	SeverityEvacuationWarning   = "EVACUATION_WARNING"
	SeverityImmediateEvacuation = "IMMEDIATE_EVACUATION"
)
// Incident Statuses
const (
	StatusReported   = "REPORTED"
	StatusDispatched = "DISPATCHED"
	StatusContained  = "CONTAINED"
	StatusResolved   = "RESOLVED"
)
