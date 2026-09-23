package outfitter

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
)

// GearItem represents a tracked piece of expedition equipment available for rental.
type GearItem struct {
	SerialNumber string `json:"serial_number"`
	Name string `json:"name"`
	Category string `json:"category"`
	DailyRateCents int `json:"daily_rate_cents"`
	DepositCents int `json:"deposit_cents"`
	Condition string `json:"condition"`
	Status string `json:"status"`
}

// LockerBay represents an individual contactless electronic locker compartment (bays 1..16).
type LockerBay struct {
	BayNumber int `json:"bay_number"`
	ItemSerialNumber string `json:"item_serial_number"`
	AssignedUserID string `json:"assigned_user_id"`
	ReservationID string `json:"reservation_id"`
	PasscodePIN string `json:"passcode_pin"`
	Status string `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// LockerManager manages inventory and contactless locker bay access control.
type LockerManager struct {
	mu sync.RWMutex
	inventory map[string]*GearItem
	bays map[int]*LockerBay
}

// NewLockerManager constructs a new LockerManager with 16 idle locker bays.
func NewLockerManager() *LockerManager {
	mgr := &LockerManager{
		inventory: make(map[string]*GearItem),
		bays:      make(map[int]*LockerBay, 16),
	}
	for i := 1; i <= 16; i++ {
		mgr.bays[i] = &LockerBay{
			BayNumber: i,
			Status:    LockerStatusIdle,
		}
	}
	return mgr
}

// SeedDefaultGear populates expedition rental inventory across all 5 core categories.
func (m *LockerManager) SeedDefaultGear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	defaultGear := []*GearItem{
		// Bear Canisters
		{
			SerialNumber:   "BC-GARCIA-001",
			Name:           "Garcia Backpacker's Cache 812",
			Category:       CategoryBearCanister,
			DailyRateCents: 800,
			DepositCents:   7500,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "BC-BV500-002",
			Name:           "BearVault BV500 Journey Canister",
			Category:       CategoryBearCanister,
			DailyRateCents: 900,
			DepositCents:   8500,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "BC-BV450-003",
			Name:           "BearVault BV450 Jaunt Solo Canister",
			Category:       CategoryBearCanister,
			DailyRateCents: 800,
			DepositCents:   7000,
			Condition:      ConditionGood,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "BC-LID1-004",
			Name:           "Wildcat Carbon Fiber Pro Canister",
			Category:       CategoryBearCanister,
			DailyRateCents: 1400,
			DepositCents:   22000,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		// Satellite Beacons
		{
			SerialNumber:   "SB-INREACH-101",
			Name:           "Garmin inReach Mini 2 Satellite Communicator",
			Category:       CategorySatelliteBeacon,
			DailyRateCents: 1500,
			DepositCents:   35000,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "SB-ZOLEO-102",
			Name:           "ZOLEO Satellite Communicator Beacon",
			Category:       CategorySatelliteBeacon,
			DailyRateCents: 1200,
			DepositCents:   20000,
			Condition:      ConditionGood,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "SB-SPOT4-103",
			Name:           "SPOT Gen4 Satellite GPS Messenger",
			Category:       CategorySatelliteBeacon,
			DailyRateCents: 1000,
			DepositCents:   15000,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "SB-ACR-104",
			Name:           "ACR Bivy Stick 2-Way Satellite Beacon",
			Category:       CategorySatelliteBeacon,
			DailyRateCents: 1300,
			DepositCents:   25000,
			Condition:      ConditionGood,
			Status:         GearStatusAvailable,
		},
		// Four Season Tents
		{
			SerialNumber:   "TN-TRANGO-201",
			Name:           "Mountain Hardwear Trango 2 Mountaineering Tent",
			Category:       CategoryFourSeasonTent,
			DailyRateCents: 3500,
			DepositCents:   65000,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "TN-GEODOME-202",
			Name:           "The North Face Mountain 25 Expedition Tent",
			Category:       CategoryFourSeasonTent,
			DailyRateCents: 3200,
			DepositCents:   60000,
			Condition:      ConditionGood,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "TN-HILLE-203",
			Name:           "Hilleberg Nammatj 2 GT 4-Season Tunnel Tent",
			Category:       CategoryFourSeasonTent,
			DailyRateCents: 4500,
			DepositCents:   95000,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "TN-MSR-204",
			Name:           "MSR Remote 2 Expedition Winter Tent",
			Category:       CategoryFourSeasonTent,
			DailyRateCents: 3400,
			DepositCents:   62000,
			Condition:      ConditionGood,
			Status:         GearStatusAvailable,
		},
		// Snowshoes
		{
			SerialNumber:   "SS-LIGHTNING-301",
			Name:           "MSR Lightning Ascent 25 Snowshoes",
			Category:       CategorySnowshoes,
			DailyRateCents: 1400,
			DepositCents:   32000,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "SS-TUBBS-302",
			Name:           "Tubbs Flex VRT Backcountry Snowshoes",
			Category:       CategorySnowshoes,
			DailyRateCents: 1300,
			DepositCents:   28000,
			Condition:      ConditionGood,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "SS-ATLAS-303",
			Name:           "Atlas Range-MTN Backcountry Snowshoes",
			Category:       CategorySnowshoes,
			DailyRateCents: 1200,
			DepositCents:   26000,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "SS-REVO-304",
			Name:           "MSR Revo Explore Snowshoes 22",
			Category:       CategorySnowshoes,
			DailyRateCents: 1100,
			DepositCents:   22000,
			Condition:      ConditionGood,
			Status:         GearStatusAvailable,
		},
		// Water Filters
		{
			SerialNumber:   "WF-GUARDIAN-401",
			Name:           "MSR Guardian Military-Grade Purifier",
			Category:       CategoryWaterFilter,
			DailyRateCents: 2000,
			DepositCents:   38000,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "WF-KATADYN-402",
			Name:           "Katadyn BeFree 1.0L Microfilter Fast-Flow",
			Category:       CategoryWaterFilter,
			DailyRateCents: 700,
			DepositCents:   4500,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "WF-SAWYER-403",
			Name:           "Sawyer Squeeze Gravity Filtration System",
			Category:       CategoryWaterFilter,
			DailyRateCents: 600,
			DepositCents:   4000,
			Condition:      ConditionGood,
			Status:         GearStatusAvailable,
		},
		{
			SerialNumber:   "WF-PLATY-404",
			Name:           "Platypus GravityWorks 4.0L Expedition Filter",
			Category:       CategoryWaterFilter,
			DailyRateCents: 1200,
			DepositCents:   12000,
			Condition:      ConditionMint,
			Status:         GearStatusAvailable,
		},
	}

	for _, item := range defaultGear {
		m.inventory[item.SerialNumber] = item
	}
}

// AddGearItem registers a new custom gear item into the inventory.
func (m *LockerManager) AddGearItem(item *GearItem) error {
	if item == nil {
		return errors.New("item cannot be nil")
	}
	if strings.TrimSpace(item.SerialNumber) == "" {
		return errors.New("item serial number cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.inventory[item.SerialNumber] = item
	return nil
}

// GetGearItem retrieves a gear item by its unique serial number.
func (m *LockerManager) GetGearItem(serial string) (*GearItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, ok := m.inventory[serial]
	if !ok {
		return nil, fmt.Errorf("gear item %s not found", serial)
	}
	return item, nil
}

// generateSecurePIN creates a cryptographically secure 6-digit numeric passcode PIN.
func generateSecurePIN() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// ReserveGearAndLocker reserves an available gear item, allocates an idle locker bay,
// generates a 6-digit PIN, and sets an expiration timestamp.
func (m *LockerManager) ReserveGearAndLocker(userID, reservationID, itemSerial string, rentalDays int) (*LockerBay, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("userID cannot be empty")
	}
	if strings.TrimSpace(reservationID) == "" {
		return nil, errors.New("reservationID cannot be empty")
	}
	if strings.TrimSpace(itemSerial) == "" {
		return nil, errors.New("itemSerial cannot be empty")
	}
	if rentalDays <= 0 {
		return nil, errors.New("rentalDays must be greater than zero")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.inventory[itemSerial]
	if !ok {
		return nil, fmt.Errorf("gear item %s not found in inventory", itemSerial)
	}
	if item.Status != GearStatusAvailable {
		return nil, fmt.Errorf("gear item %s is not available (current status: %s)", itemSerial, item.Status)
	}

	// Find the lowest numbered idle locker bay
	var targetBay *LockerBay
	for bayNum := 1; bayNum <= 16; bayNum++ {
		bay := m.bays[bayNum]
		if bay != nil && bay.Status == LockerStatusIdle {
			targetBay = bay
			break
		}
	}

	if targetBay == nil {
		return nil, errors.New("all locker bays are currently occupied")
	}

	pin, err := generateSecurePIN()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secure PIN: %w", err)
	}

	// Update gear item status
	item.Status = GearStatusReserved

	// Populate and lock bay
	targetBay.ItemSerialNumber = itemSerial
	targetBay.AssignedUserID = userID
	targetBay.ReservationID = reservationID
	targetBay.PasscodePIN = pin
	targetBay.Status = LockerStatusLoaded
	targetBay.ExpiresAt = time.Now().UTC().Add(time.Duration(rentalDays) * 24 * time.Hour)

	// Return a copy to prevent mutation outside locks
	bayCopy := *targetBay
	return &bayCopy, nil
}

// UnlockLocker verifies the bay number and passcode PIN using constant-time comparison,
// unlocks the locker door, transitions the bay to CLAIMED, and changes gear to CHECKED_OUT.
func (m *LockerManager) UnlockLocker(bayNumber int, pin string) (bool, string, error) {
	if bayNumber < 1 || bayNumber > 16 {
		return false, "", fmt.Errorf("invalid bay number: %d (must be 1..16)", bayNumber)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	bay, ok := m.bays[bayNumber]
	if !ok || bay == nil {
		return false, "", fmt.Errorf("locker bay %d not found", bayNumber)
	}

	if bay.Status != LockerStatusLoaded {
		return false, "", fmt.Errorf("bay %d is not loaded (current status: %s)", bayNumber, bay.Status)
	}

	// Cryptographically secure constant-time PIN comparison
	if subtle.ConstantTimeCompare([]byte(bay.PasscodePIN), []byte(pin)) != 1 {
		return false, "", fmt.Errorf("invalid passcode PIN for bay %d", bayNumber)
	}

	bay.Status = LockerStatusClaimed
	itemSerial := bay.ItemSerialNumber

	if item, exists := m.inventory[itemSerial]; exists {
		item.Status = GearStatusCheckedOut
	}

	return true, itemSerial, nil
}

// ReturnGearToLocker accepts an expedition gear return into the assigned smart locker bay,
// transitioning the bay to RETURNED and restoring gear inventory to AVAILABLE.
func (m *LockerManager) ReturnGearToLocker(bayNumber int, itemSerial string) error {
	if bayNumber < 1 || bayNumber > 16 {
		return fmt.Errorf("invalid bay number: %d (must be 1..16)", bayNumber)
	}
	if strings.TrimSpace(itemSerial) == "" {
		return errors.New("itemSerial cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	bay, ok := m.bays[bayNumber]
	if !ok || bay == nil {
		return fmt.Errorf("locker bay %d not found", bayNumber)
	}

	if bay.ItemSerialNumber != itemSerial {
		return fmt.Errorf("bay %d item serial mismatch: expected %s, got %s", bayNumber, bay.ItemSerialNumber, itemSerial)
	}

	if bay.Status != LockerStatusClaimed {
		return fmt.Errorf("bay %d cannot receive return from status %s", bayNumber, bay.Status)
	}

	bay.Status = LockerStatusReturned

	if item, exists := m.inventory[itemSerial]; exists {
		item.Status = GearStatusAvailable
	}

	return nil
}

// ResetLockerBay clears a returned locker bay back to IDLE status after inspection.
func (m *LockerManager) ResetLockerBay(bayNumber int) error {
	if bayNumber < 1 || bayNumber > 16 {
		return fmt.Errorf("invalid bay number: %d (must be 1..16)", bayNumber)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	bay, ok := m.bays[bayNumber]
	if !ok || bay == nil {
		return fmt.Errorf("locker bay %d not found", bayNumber)
	}

	bay.ItemSerialNumber = ""
	bay.AssignedUserID = ""
	bay.ReservationID = ""
	bay.PasscodePIN = ""
	bay.Status = LockerStatusIdle
	bay.ExpiresAt = time.Time{}

	return nil
}

// ListAvailableGear returns all rental items currently in AVAILABLE status, sorted by serial number.
func (m *LockerManager) ListAvailableGear() []*GearItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var available []*GearItem
	for _, item := range m.inventory {
		if item.Status == GearStatusAvailable {
			itemCopy := *item
			available = append(available, &itemCopy)
		}
	}

	sort.Slice(available, func(i, j int) bool {
		return available[i].SerialNumber < available[j].SerialNumber
	})

	return available
}

// GetLockerStatus returns the current status and metadata of a specific locker bay.
func (m *LockerManager) GetLockerStatus(bayNumber int) (*LockerBay, error) {
	if bayNumber < 1 || bayNumber > 16 {
		return nil, fmt.Errorf("invalid bay number: %d (must be 1..16)", bayNumber)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	bay, ok := m.bays[bayNumber]
	if !ok || bay == nil {
		return nil, fmt.Errorf("locker bay %d not found", bayNumber)
	}

	bayCopy := *bay
	return &bayCopy, nil
}

// Gear categories for backcountry expeditions.
const (
	CategoryBearCanister    = "BEAR_CANISTER"
	CategorySatelliteBeacon = "SATELLITE_BEACON"
	CategoryFourSeasonTent  = "FOUR_SEASON_TENT"
	CategorySnowshoes       = "SNOWSHOES"
	CategoryWaterFilter     = "WATER_FILTER"
)
// Physical condition ratings for gear.
const (
	ConditionMint               = "MINT"
	ConditionGood               = "GOOD"
	ConditionInspectionRequired = "INSPECTION_REQUIRED"
)
// Operational status of rental inventory.
const (
	GearStatusAvailable   = "AVAILABLE"
	GearStatusReserved    = "RESERVED"
	GearStatusCheckedOut  = "CHECKED_OUT"
	GearStatusMaintenance = "MAINTENANCE"
)
// Operational lifecycle status of smart locker bays.
const (
	LockerStatusIdle     = "IDLE"
	LockerStatusLoaded   = "LOADED"
	LockerStatusClaimed  = "CLAIMED"
	LockerStatusReturned = "RETURNED"
)
