package ranger

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// WorkOrder defines maintenance and hazard mitigation requests in campgrounds.
type WorkOrder struct {
	ID string `json:"id"`
	CampgroundID string `json:"campground_id"`
	CampsiteID string `json:"campsite_id"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	Status string `json:"status"`
	Description string `json:"description"`
	AssignedRanger string `json:"assigned_ranger,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// VehicleAuthorization records vehicle ALPR registration for gate access control.
type VehicleAuthorization struct {
	ReservationID string `json:"reservation_id,omitempty"`
	LicensePlate string `json:"license_plate"`
	State string `json:"state"`
	MakeModel string `json:"make_model,omitempty"`
	SecondaryPlate string `json:"secondary_plate,omitempty"`
	CampsiteID string `json:"campsite_id"`
	GuestName string `json:"guest_name"`
	CheckIn time.Time `json:"check_in"`
	CheckOut time.Time `json:"check_out"`
	Authorized bool `json:"authorized"`
	GateEntryCount int `json:"gate_entry_count"`
	LastEntryAt *time.Time `json:"last_entry_at,omitempty"`
}

// GateLogEntry records each access attempt through automated ALPR gatehouses.
type GateLogEntry struct {
	ID string `json:"id"`
	Plate string `json:"plate"`
	State string `json:"state"`
	Lane string `json:"lane"`
	Timestamp time.Time `json:"timestamp"`
	Authorized bool `json:"authorized"`
	Reason string `json:"reason"`
	CampsiteID string `json:"campsite_id,omitempty"`
	GuestName string `json:"guest_name,omitempty"`
	BarrierAction string `json:"barrier_action"`
}

// EventHub is a thread-safe broker broadcasting live events to SSE subscriber channels.
type EventHub struct {
	mu sync.RWMutex
	subscribers map[chan string]struct{}
}

// GateController manages multi-lane ALPR gate access, physical barrier state transitions,
// and automatic clearance timers.
type GateController struct {
	mu sync.RWMutex
	barrierState string
	resetTimer *time.Timer
	resetDuration time.Duration
	logs []*GateLogEntry
	maxLogs int
	pms *PMSManager
	hub *EventHub
}

// PMSManager coordinates wild-pms facilities management, work order dispatch,
// vehicle ALPR gate authorizations, multi-lane gatehouse, and live SSE broadcasts.
type PMSManager struct {
	mu sync.RWMutex
	EventHub *EventHub
	GateController *GateController
	workOrders map[string]*WorkOrder
	vehicleAuthorizations map[string]*VehicleAuthorization
}

// NewEventHub creates and initializes an EventHub.
func NewEventHub() *EventHub {
	return &EventHub{
		subscribers: make(map[chan string]struct{}),
	}
}

// Subscribe registers a new subscriber and returns a channel receiving SSE formatted strings.
func (h *EventHub) Subscribe() chan string {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan string, 64)
	h.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
func (h *EventHub) Unsubscribe(ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subscribers[ch]; ok {
		delete(h.subscribers, ch)
		close(ch)
	}
}

// Broadcast sends an SSE-formatted event to all active subscribers.
func (h *EventHub) Broadcast(event, payload string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, payload)
	for ch := range h.subscribers {
		select {
		case ch <- msg:
		default:
			// Buffer full, drop to avoid blocking other subscribers
		}
	}
}

// SubscriberCount returns the current count of active subscribers.
func (h *EventHub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}

// NewGateController constructs a GateController linked to PMSManager and EventHub.
func NewGateController(pms *PMSManager, hub *EventHub) *GateController {
	if hub == nil && pms != nil {
		hub = pms.EventHub
	}
	return &GateController{
		barrierState:  BarrierLowered,
		resetDuration: 6 * time.Second,
		maxLogs:       1000,
		pms:           pms,
		hub:           hub,
		logs:          make([]*GateLogEntry, 0, 100),
	}
}

// SetResetDuration allows customizing the barrier auto-clearance reset timer duration (e.g. for testing).
func (gc *GateController) SetResetDuration(d time.Duration) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.resetDuration = d
}

// State returns the current barrier state.
func (gc *GateController) State() string {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	return gc.barrierState
}

// ResetBarrier resets the gate barrier to BarrierLowered and clears any active timers.
func (gc *GateController) ResetBarrier() string {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	if gc.resetTimer != nil {
		gc.resetTimer.Stop()
		gc.resetTimer = nil
	}
	gc.barrierState = BarrierLowered
	return gc.barrierState
}

// AuthorizeGateEntryWithLane evaluates ALPR access through a specified lane,
// manages barrier transitions with a 6-second clearance reset timer, updates entry counters,
// and broadcasts a gate_scan_event via EventHub.
func (gc *GateController) AuthorizeGateEntryWithLane(plate, state, lane string) (*GateLogEntry, bool, string) {
	normPlate := strings.ToUpper(strings.TrimSpace(plate))
	normState := strings.ToUpper(strings.TrimSpace(state))
	normLane := strings.ToUpper(strings.TrimSpace(lane))
	if normLane == "" {
		normLane = LaneStandard
	}

	// 1. Set scanning state
	gc.mu.Lock()
	gc.barrierState = BarrierScanning
	gc.mu.Unlock()

	var auth *VehicleAuthorization
	now := time.Now()

	// 2. Lookup authorization in PMSManager
	if gc.pms != nil {
		gc.pms.mu.Lock()
		key := vehicleKey(normPlate, normState)
		if existing, ok := gc.pms.vehicleAuthorizations[key]; ok {
			auth = existing
		} else if normLane == LaneTrailerOversize {
			// In oversize/trailer lane, match either primary plate or secondary trailer plate
			for _, v := range gc.pms.vehicleAuthorizations {
				if v.SecondaryPlate == normPlate || v.LicensePlate == normPlate {
					auth = v
					break
				}
			}
		}
		gc.pms.mu.Unlock()
	}

	// 3. Evaluate access rules
	authorized := false
	var reason string
	var barrierAction string
	var campsiteID string
	var guestName string

	if auth == nil {
		authorized = false
		reason = fmt.Sprintf("Vehicle %s (%s) not registered", normPlate, normState)
		barrierAction = "BARRIER_LOCKED"
	} else {
		campsiteID = auth.CampsiteID
		guestName = auth.GuestName

		if now.Before(auth.CheckIn) {
			authorized = false
			reason = fmt.Sprintf("Reservation for %s not yet active (starts %s)", auth.GuestName, auth.CheckIn.Format(time.RFC3339))
			barrierAction = "BARRIER_LOCKED"
		} else if now.After(auth.CheckOut) {
			authorized = false
			reason = fmt.Sprintf("Reservation for %s expired at %s", auth.GuestName, auth.CheckOut.Format(time.RFC3339))
			barrierAction = "BARRIER_LOCKED"
		} else if !auth.Authorized {
			authorized = false
			reason = fmt.Sprintf("Vehicle authorization revoked for %s", auth.GuestName)
			barrierAction = "BARRIER_LOCKED"
		} else {
			authorized = true
			if gc.pms != nil {
				gc.pms.mu.Lock()
				auth.GateEntryCount++
				nowUTC := now.UTC()
				auth.LastEntryAt = &nowUTC
				gc.pms.mu.Unlock()
			}
			reason = fmt.Sprintf("Authorized for campsite %s (Guest: %s, Lane: %s)", auth.CampsiteID, auth.GuestName, normLane)
			barrierAction = "BARRIER_RAISED"
		}
	}

	// 4. Update barrier state machine and reset timer
	gc.mu.Lock()
	if authorized {
		gc.barrierState = BarrierOpen
		if gc.resetTimer != nil {
			gc.resetTimer.Stop()
		}
		gc.resetTimer = time.AfterFunc(gc.resetDuration, func() {
			gc.mu.Lock()
			defer gc.mu.Unlock()
			gc.barrierState = BarrierLowered
		})
	} else {
		gc.barrierState = BarrierLockout
		if gc.resetTimer != nil {
			gc.resetTimer.Stop()
			gc.resetTimer = nil
		}
	}

	entry := &GateLogEntry{
		ID:            generateID("gate"),
		Plate:         normPlate,
		State:         normState,
		Lane:          normLane,
		Timestamp:     now.UTC(),
		Authorized:    authorized,
		Reason:        reason,
		CampsiteID:    campsiteID,
		GuestName:     guestName,
		BarrierAction: barrierAction,
	}

	gc.logs = append(gc.logs, entry)
	if len(gc.logs) > gc.maxLogs {
		gc.logs = gc.logs[len(gc.logs)-gc.maxLogs:]
	}
	gc.mu.Unlock()

	// 5. Broadcast gate_scan_event via EventHub
	if gc.hub != nil {
		payload, _ := json.Marshal(entry)
		gc.hub.Broadcast("gate_scan_event", string(payload))
	}

	return entry, authorized, reason
}

// GetRecentGateLogs returns the most recent gate access log entries (newest first).
func (gc *GateController) GetRecentGateLogs(limit int) []*GateLogEntry {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	n := len(gc.logs)
	if limit <= 0 || limit > n {
		limit = n
	}

	result := make([]*GateLogEntry, limit)
	for i := 0; i < limit; i++ {
		result[i] = gc.logs[n-1-i]
	}
	return result
}

// NewPMSManager constructs a PMSManager with an EventHub, GateController, and thread-safe data stores.
func NewPMSManager(hub *EventHub) *PMSManager {
	if hub == nil {
		hub = NewEventHub()
	}
	pms := &PMSManager{
		EventHub:              hub,
		workOrders:            make(map[string]*WorkOrder),
		vehicleAuthorizations: make(map[string]*VehicleAuthorization),
	}
	pms.GateController = NewGateController(pms, hub)
	return pms
}

// AuthorizeGateEntryWithLane delegates gate authorization to the integrated GateController.
func (m *PMSManager) AuthorizeGateEntryWithLane(plate, state, lane string) (*GateLogEntry, bool, string) {
	return m.GateController.AuthorizeGateEntryWithLane(plate, state, lane)
}

// GetRecentGateLogs delegates recent log retrieval to the integrated GateController.
func (m *PMSManager) GetRecentGateLogs(limit int) []*GateLogEntry {
	return m.GateController.GetRecentGateLogs(limit)
}

// ResetBarrier delegates barrier reset to the integrated GateController.
func (m *PMSManager) ResetBarrier() string {
	return m.GateController.ResetBarrier()
}

// generateID returns a random hex-encoded prefixed identifier.
func generateID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}

func vehicleKey(plate, state string) string {
	return fmt.Sprintf("%s:%s", strings.ToUpper(strings.TrimSpace(plate)), strings.ToUpper(strings.TrimSpace(state)))
}

// CreateWorkOrder validates and stores a new work order, broadcasting an event via EventHub.
func (m *PMSManager) CreateWorkOrder(campgroundID, campsiteID, category, severity, description string) (*WorkOrder, error) {
	category = strings.ToUpper(strings.TrimSpace(category))
	if !validCategories[category] {
		return nil, fmt.Errorf("invalid category: %q, must be one of BEAR_BOX, WATER_SPIGOT, TREE_HAZARD, SANITATION", category)
	}

	severity = strings.ToUpper(strings.TrimSpace(severity))
	if !validSeverities[severity] {
		return nil, fmt.Errorf("invalid severity: %q, must be one of LOW, MEDIUM, HIGH, CRITICAL", severity)
	}

	if strings.TrimSpace(campgroundID) == "" {
		return nil, fmt.Errorf("campgroundID cannot be empty")
	}

	wo := &WorkOrder{
		ID:           generateID("wo"),
		CampgroundID: strings.TrimSpace(campgroundID),
		CampsiteID:   strings.TrimSpace(campsiteID),
		Category:     category,
		Severity:     severity,
		Status:       StatusPending,
		Description:  strings.TrimSpace(description),
		CreatedAt:    time.Now().UTC(),
	}

	m.mu.Lock()
	m.workOrders[wo.ID] = wo
	m.mu.Unlock()

	if m.EventHub != nil {
		payload, _ := json.Marshal(wo)
		m.EventHub.Broadcast("work_order_created", string(payload))
	}

	return wo, nil
}

// UpdateWorkOrderStatus updates the status and assigned ranger of a work order,
// setting ResolvedAt on resolution, and broadcasts an update event via EventHub.
func (m *PMSManager) UpdateWorkOrderStatus(orderID, status, rangerID string) (*WorkOrder, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	if !validStatuses[status] {
		return nil, fmt.Errorf("invalid status: %q, must be one of PENDING, DISPATCHED, RESOLVED", status)
	}

	m.mu.Lock()
	wo, exists := m.workOrders[orderID]
	if !exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("work order %q not found", orderID)
	}

	wo.Status = status
	if strings.TrimSpace(rangerID) != "" {
		wo.AssignedRanger = strings.TrimSpace(rangerID)
	}

	if status == StatusResolved {
		now := time.Now().UTC()
		wo.ResolvedAt = &now
	}

	updated := *wo
	m.mu.Unlock()

	if m.EventHub != nil {
		payload, _ := json.Marshal(&updated)
		m.EventHub.Broadcast("work_order_updated", string(payload))
	}

	return wo, nil
}

// ListWorkOrders returns work orders filtered by campgroundID (or all if campgroundID is empty),
// sorted chronologically by CreatedAt.
func (m *PMSManager) ListWorkOrders(campgroundID string) []*WorkOrder {
	m.mu.RLock()
	defer m.mu.RUnlock()

	campgroundID = strings.TrimSpace(campgroundID)
	var list []*WorkOrder
	for _, wo := range m.workOrders {
		if campgroundID == "" || wo.CampgroundID == campgroundID {
			list = append(list, wo)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})

	return list
}

// RegisterVehicle stores an authorized vehicle registration for ALPR gate access control.
func (m *PMSManager) RegisterVehicle(plate, state, campsiteID, guestName string, checkIn, checkOut time.Time) {
	m.RegisterVehicleAuthorization(&VehicleAuthorization{
		LicensePlate: strings.ToUpper(strings.TrimSpace(plate)),
		State:        strings.ToUpper(strings.TrimSpace(state)),
		CampsiteID:   strings.TrimSpace(campsiteID),
		GuestName:    strings.TrimSpace(guestName),
		CheckIn:      checkIn,
		CheckOut:     checkOut,
		Authorized:   true,
	})
}

// RegisterVehicleAuthorization stores an extended VehicleAuthorization struct.
func (m *PMSManager) RegisterVehicleAuthorization(auth *VehicleAuthorization) {
	if auth == nil {
		return
	}
	auth.LicensePlate = strings.ToUpper(strings.TrimSpace(auth.LicensePlate))
	auth.State = strings.ToUpper(strings.TrimSpace(auth.State))
	auth.SecondaryPlate = strings.ToUpper(strings.TrimSpace(auth.SecondaryPlate))
	auth.CampsiteID = strings.TrimSpace(auth.CampsiteID)
	auth.GuestName = strings.TrimSpace(auth.GuestName)
	auth.ReservationID = strings.TrimSpace(auth.ReservationID)
	auth.MakeModel = strings.TrimSpace(auth.MakeModel)

	key := vehicleKey(auth.LicensePlate, auth.State)

	m.mu.Lock()
	m.vehicleAuthorizations[key] = auth
	m.mu.Unlock()

	if m.EventHub != nil {
		payload, _ := json.Marshal(auth)
		m.EventHub.Broadcast("vehicle_registered", string(payload))
	}
}

// RegisterVehicleExtended registers a vehicle with secondary trailer plate, make/model, and reservation ID.
func (m *PMSManager) RegisterVehicleExtended(plate, state, campsiteID, guestName, makeModel, secondaryPlate, reservationID string, checkIn, checkOut time.Time) *VehicleAuthorization {
	auth := &VehicleAuthorization{
		LicensePlate:   plate,
		State:          state,
		CampsiteID:     campsiteID,
		GuestName:      guestName,
		MakeModel:      makeModel,
		SecondaryPlate: secondaryPlate,
		ReservationID:  reservationID,
		CheckIn:        checkIn,
		CheckOut:       checkOut,
		Authorized:     true,
	}
	m.RegisterVehicleAuthorization(auth)
	return auth
}

// AuthorizeGateEntry evaluates vehicle access against active registrations for current time on standard lane.
func (m *PMSManager) AuthorizeGateEntry(plate, state string) (bool, string) {
	_, auth, reason := m.AuthorizeGateEntryWithLane(plate, state, LaneStandard)
	return auth, reason
}

// GateLane constants identify designated campground entrance corridors.
const (
	LaneStandard        = "STANDARD"
	LaneTrailerOversize = "OVERSIZE_TRAILER"
	LaneEmergencyRanger = "EMERGENCY_RANGER"
)
// BarrierState constants model the physical ALPR gatehouse barrier arm states.
const (
	BarrierLowered  = "LOWERED"
	BarrierScanning = "SCANNING"
	BarrierRaising  = "RAISING"
	BarrierOpen     = "OPEN"
	BarrierLowering = "LOWERING"
	BarrierLockout  = "LOCKOUT"
)
// Allowed Category, Severity, and Status values.
const (
	CategoryBearBox     = "BEAR_BOX"
	CategoryWaterSpigot = "WATER_SPIGOT"
	CategoryTreeHazard  = "TREE_HAZARD"
	CategorySanitation  = "SANITATION"

	SeverityLow      = "LOW"
	SeverityMedium   = "MEDIUM"
	SeverityHigh     = "HIGH"
	SeverityCritical = "CRITICAL"

	StatusPending    = "PENDING"
	StatusDispatched = "DISPATCHED"
	StatusResolved   = "RESOLVED"
)
var validCategories = map[string]bool{
	CategoryBearBox:     true,
	CategoryWaterSpigot: true,
	CategoryTreeHazard:  true,
	CategorySanitation:  true,
}
var validSeverities = map[string]bool{
	SeverityLow:      true,
	SeverityMedium:   true,
	SeverityHigh:     true,
	SeverityCritical: true,
}
var validStatuses = map[string]bool{
	StatusPending:    true,
	StatusDispatched: true,
	StatusResolved:   true,
}
