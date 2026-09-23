package pms

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// CampsiteAsset tracks physical campground and campsite assets, equipment serials, and inspection cycles.
type CampsiteAsset struct {
	ID string `json:"id"`
	CampgroundID string `json:"campground_id,omitempty"`
	CampsiteID string `json:"campsite_id"`
	AssetClass string `json:"asset_class"`
	SerialNumber string `json:"serial_number"`
	InstalledDate time.Time `json:"installed_date"`
	ConditionRating string `json:"condition_rating"`
	LastInspected time.Time `json:"last_inspected"`
	NextInspectionDue time.Time `json:"next_inspection_due"`
	Notes string `json:"notes,omitempty"`
}

// OccupancyRecord represents a live in-park registered party, vehicles, and emergency contact details.
type OccupancyRecord struct {
	CampsiteID string `json:"campsite_id"`
	ReservationID string `json:"reservation_id"`
	GuestName string `json:"guest_name"`
	PrimaryPlate string `json:"primary_plate"`
	TrailerPlate string `json:"trailer_plate,omitempty"`
	CheckedInAt time.Time `json:"checked_in_at"`
	ScheduledCheckOut time.Time `json:"scheduled_check_out"`
	PartySize int `json:"party_size"`
	EmergencyPhone string `json:"emergency_phone"`
}

// PMSAssetManager coordinates physical campsite asset maintenance registries and real-time in-park occupancy rosters.
type PMSAssetManager struct {
	mu sync.RWMutex
	assets map[string]*CampsiteAsset
	occupancy map[string]*OccupancyRecord
}

// NewPMSAssetManager constructs an initialized PMSAssetManager.
func NewPMSAssetManager() *PMSAssetManager {
	return &PMSAssetManager{
		assets:    make(map[string]*CampsiteAsset),
		occupancy: make(map[string]*OccupancyRecord),
	}
}

// RegisterAsset registers a campsite asset into the registry.
func (m *PMSAssetManager) RegisterAsset(asset *CampsiteAsset) error {
	if asset == nil {
		return errors.New("asset cannot be nil")
	}
	if asset.ID == "" {
		return errors.New("asset id is required")
	}
	if asset.CampsiteID == "" {
		return errors.New("campsite id is required")
	}
	if asset.AssetClass == "" {
		return errors.New("asset class is required")
	}

	assetCopy := *asset
	if assetCopy.InstalledDate.IsZero() {
		assetCopy.InstalledDate = time.Now().UTC()
	}
	if assetCopy.ConditionRating == "" {
		assetCopy.ConditionRating = ConditionGood
	}
	if assetCopy.NextInspectionDue.IsZero() {
		assetCopy.NextInspectionDue = assetCopy.InstalledDate.AddDate(0, 6, 0)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.assets[assetCopy.ID] = &assetCopy
	return nil
}

// UpdateAssetCondition updates the condition rating, records an inspection timestamp, and updates notes.
func (m *PMSAssetManager) UpdateAssetCondition(id, condition string, notes string) error {
	if id == "" {
		return errors.New("asset id is required")
	}
	switch condition {
	case ConditionExcellent, ConditionGood, ConditionNeedsRepair, ConditionCondemned:
	default:
		return fmt.Errorf("invalid condition rating: %s", condition)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	asset, exists := m.assets[id]
	if !exists {
		return fmt.Errorf("asset %s not found", id)
	}

	asset.ConditionRating = condition
	asset.LastInspected = time.Now().UTC()
	if notes != "" {
		if asset.Notes != "" {
			asset.Notes = asset.Notes + "; " + notes
		} else {
			asset.Notes = notes
		}
	}
	return nil
}

// GetAsset retrieves an asset by ID.
func (m *PMSAssetManager) GetAsset(id string) (*CampsiteAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	asset, exists := m.assets[id]
	if !exists {
		return nil, fmt.Errorf("asset %s not found", id)
	}
	cp := *asset
	return &cp, nil
}

// ListAssetsNeedingRepair returns all assets with condition NEEDS_REPAIR or CONDEMNED,
// optionally filtered by campgroundID.
func (m *PMSAssetManager) ListAssetsNeedingRepair(campgroundID string) []*CampsiteAsset {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var needingRepair []*CampsiteAsset
	for _, asset := range m.assets {
		if asset.ConditionRating != ConditionNeedsRepair && asset.ConditionRating != ConditionCondemned {
			continue
		}
		if campgroundID != "" {
			matchesCampground := asset.CampgroundID == campgroundID ||
				strings.HasPrefix(asset.CampsiteID, campgroundID)
			if !matchesCampground {
				continue
			}
		}
		cp := *asset
		needingRepair = append(needingRepair, &cp)
	}

	sort.Slice(needingRepair, func(i, j int) bool {
		if !needingRepair[i].NextInspectionDue.Equal(needingRepair[j].NextInspectionDue) {
			return needingRepair[i].NextInspectionDue.Before(needingRepair[j].NextInspectionDue)
		}
		return needingRepair[i].ID < needingRepair[j].ID
	})

	return needingRepair
}

// CheckInOccupant registers a party to an active campsite. Returns an error if the site is already occupied.
func (m *PMSAssetManager) CheckInOccupant(rec *OccupancyRecord) error {
	if rec == nil {
		return errors.New("occupancy record cannot be nil")
	}
	if rec.CampsiteID == "" {
		return errors.New("campsite id is required")
	}
	if rec.ReservationID == "" {
		return errors.New("reservation id is required")
	}
	if rec.GuestName == "" {
		return errors.New("guest name is required")
	}
	if rec.PartySize <= 0 {
		return errors.New("party size must be greater than zero")
	}

	recCopy := *rec
	if recCopy.CheckedInAt.IsZero() {
		recCopy.CheckedInAt = time.Now().UTC()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, exists := m.occupancy[recCopy.CampsiteID]; exists {
		return fmt.Errorf("campsite %s is already occupied by reservation %s (%s)",
			recCopy.CampsiteID, existing.ReservationID, existing.GuestName)
	}

	m.occupancy[recCopy.CampsiteID] = &recCopy
	return nil
}

// CheckOutOccupant removes the occupant record for a campsite upon departure.
func (m *PMSAssetManager) CheckOutOccupant(campsiteID string) error {
	if campsiteID == "" {
		return errors.New("campsite id is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.occupancy[campsiteID]; !exists {
		return fmt.Errorf("no active occupant found for campsite %s", campsiteID)
	}

	delete(m.occupancy, campsiteID)
	return nil
}

// GetInParkRoster returns a snapshot of all currently checked-in parties, sorted by campsite.
func (m *PMSAssetManager) GetInParkRoster() []*OccupancyRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var roster []*OccupancyRecord
	for _, rec := range m.occupancy {
		cp := *rec
		roster = append(roster, &cp)
	}

	sort.Slice(roster, func(i, j int) bool {
		return roster[i].CampsiteID < roster[j].CampsiteID
	})

	return roster
}

// GetTotalInParkHeadcount calculates total human campers and authorized vehicle count across all active campsites.
func (m *PMSAssetManager) GetTotalInParkHeadcount() (campers int, vehicles int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rec := range m.occupancy {
		campers += rec.PartySize
		if rec.PrimaryPlate != "" {
			vehicles++
		}
		if rec.TrailerPlate != "" {
			vehicles++
		}
	}
	return campers, vehicles
}

// Asset Classes
const (
	AssetBearBox       = "BEAR_BOX"
	AssetFireRing      = "FIRE_RING"
	AssetWaterSpigot   = "WATER_SPIGOT"
	AssetGraywaterDump = "GRAYWATER_DUMP"
	AssetSolarPedestal = "SOLAR_PEDESTAL"
)
// Condition Ratings
const (
	ConditionExcellent   = "EXCELLENT"
	ConditionGood        = "GOOD"
	ConditionNeedsRepair = "NEEDS_REPAIR"
	ConditionCondemned   = "CONDEMNED"
)
