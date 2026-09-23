package profile

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// VehicleRecord represents a registered vehicle in a camper's fleet.
type VehicleRecord struct {
	Plate string `json:"plate"`
	State string `json:"state"`
	MakeModel string `json:"make_model"`
	Color string `json:"color"`
	IsEV bool `json:"is_ev"`
	IsPrimary bool `json:"is_primary"`
	AddedAt time.Time `json:"added_at"`
}

// CamperProfile models guest identity, contact information, wilderness permits,
// and registered fleet vehicles.
type CamperProfile struct {
	UserID string `json:"user_id"`
	FullName string `json:"full_name"`
	Phone string `json:"phone"`
	EmergencyContactName string `json:"emergency_contact_name"`
	EmergencyContactPhone string `json:"emergency_contact_phone"`
	WildernessPassID string `json:"wilderness_pass_id"`
	NotificationsEnabled bool `json:"notifications_enabled"`
	Vehicles []VehicleRecord `json:"vehicles"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProfileManager provides thread-safe access and mutations for camper profiles.
type ProfileManager struct {
	mu sync.RWMutex
	profiles map[string]*CamperProfile
}

// NewProfileManager instantiates a thread-safe ProfileManager.
func NewProfileManager() *ProfileManager {
	return &ProfileManager{
		profiles: make(map[string]*CamperProfile),
	}
}

// validatePhone validates standard phone number format and length.
func validatePhone(phone string) error {
	trimmed := strings.TrimSpace(phone)
	if trimmed == "" {
		return fmt.Errorf("phone number cannot be empty")
	}

	digits := 0
	for i, r := range trimmed {
		if r >= '0' && r <= '9' {
			digits++
		} else if r == '+' {
			if i != 0 {
				return fmt.Errorf("invalid phone number: '+' only permitted at beginning")
			}
		} else if r == '-' || r == '(' || r == ')' || r == '.' || r == ' ' {
			continue
		} else {
			return fmt.Errorf("invalid character %q in phone number", r)
		}
	}

	if digits < 10 || digits > 15 {
		return fmt.Errorf("invalid phone number length: must contain 10 to 15 digits, found %d", digits)
	}

	return nil
}

// GetProfile retrieves a camper profile by user ID.
func (m *ProfileManager) GetProfile(userID string) (*CamperProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userID = strings.TrimSpace(userID)
	profile, exists := m.profiles[userID]
	if !exists {
		return nil, fmt.Errorf("profile for user %q not found", userID)
	}

	// Return deep copy
	res := *profile
	res.Vehicles = make([]VehicleRecord, len(profile.Vehicles))
	copy(res.Vehicles, profile.Vehicles)
	return &res, nil
}

// UpsertProfile inserts or updates a camper profile, validating phone numbers
// and formatting license plates to uppercase.
func (m *ProfileManager) UpsertProfile(p *CamperProfile) error {
	if p == nil {
		return fmt.Errorf("profile cannot be nil")
	}

	userID := strings.TrimSpace(p.UserID)
	if userID == "" {
		return fmt.Errorf("user_id cannot be empty")
	}

	if err := validatePhone(p.Phone); err != nil {
		return fmt.Errorf("invalid phone: %w", err)
	}

	if strings.TrimSpace(p.EmergencyContactPhone) != "" {
		if err := validatePhone(p.EmergencyContactPhone); err != nil {
			return fmt.Errorf("invalid emergency contact phone: %w", err)
		}
	}

	// Normalize vehicle plates and states
	hasPrimary := false
	for i := range p.Vehicles {
		p.Vehicles[i].Plate = strings.ToUpper(strings.TrimSpace(p.Vehicles[i].Plate))
		p.Vehicles[i].State = strings.ToUpper(strings.TrimSpace(p.Vehicles[i].State))
		if p.Vehicles[i].AddedAt.IsZero() {
			p.Vehicles[i].AddedAt = time.Now().UTC()
		}
		if p.Vehicles[i].IsPrimary {
			if hasPrimary {
				p.Vehicles[i].IsPrimary = false
			} else {
				hasPrimary = true
			}
		}
	}

	// Default first vehicle to primary if none is explicitly designated
	if len(p.Vehicles) > 0 && !hasPrimary {
		p.Vehicles[0].IsPrimary = true
	}

	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.profiles[userID]
	profileCopy := *p
	profileCopy.UserID = userID
	profileCopy.FullName = strings.TrimSpace(p.FullName)
	profileCopy.Phone = strings.TrimSpace(p.Phone)
	profileCopy.EmergencyContactName = strings.TrimSpace(p.EmergencyContactName)
	profileCopy.EmergencyContactPhone = strings.TrimSpace(p.EmergencyContactPhone)
	profileCopy.WildernessPassID = strings.TrimSpace(p.WildernessPassID)

	profileCopy.Vehicles = make([]VehicleRecord, len(p.Vehicles))
	copy(profileCopy.Vehicles, p.Vehicles)

	if exists {
		profileCopy.CreatedAt = existing.CreatedAt
		profileCopy.UpdatedAt = now
	} else {
		if profileCopy.CreatedAt.IsZero() {
			profileCopy.CreatedAt = now
		}
		profileCopy.UpdatedAt = now
	}

	m.profiles[userID] = &profileCopy
	return nil
}

// AddVehicle adds a new vehicle record to a user profile, checking for duplicates
// and updating the primary vehicle status if required.
func (m *ProfileManager) AddVehicle(userID string, v VehicleRecord) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user_id cannot be empty")
	}

	plate := strings.ToUpper(strings.TrimSpace(v.Plate))
	state := strings.ToUpper(strings.TrimSpace(v.State))
	if plate == "" {
		return fmt.Errorf("plate cannot be empty")
	}
	if state == "" {
		return fmt.Errorf("state cannot be empty")
	}

	v.Plate = plate
	v.State = state
	v.MakeModel = strings.TrimSpace(v.MakeModel)
	v.Color = strings.TrimSpace(v.Color)
	if v.AddedAt.IsZero() {
		v.AddedAt = time.Now().UTC()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	profile, exists := m.profiles[userID]
	if !exists {
		return fmt.Errorf("profile for user %q not found", userID)
	}

	// Check duplicates
	for _, existing := range profile.Vehicles {
		if existing.Plate == v.Plate && existing.State == v.State {
			return fmt.Errorf("vehicle %s (%s) already registered for user %s", v.Plate, v.State, userID)
		}
	}

	// Manage primary flag
	if v.IsPrimary {
		for i := range profile.Vehicles {
			profile.Vehicles[i].IsPrimary = false
		}
	} else if len(profile.Vehicles) == 0 {
		v.IsPrimary = true
	}

	profile.Vehicles = append(profile.Vehicles, v)
	profile.UpdatedAt = time.Now().UTC()
	return nil
}

// RemoveVehicle removes a vehicle record identified by plate and state.
// If the removed vehicle was primary, the first remaining vehicle is promoted to primary.
func (m *ProfileManager) RemoveVehicle(userID, plate, state string) error {
	userID = strings.TrimSpace(userID)
	plate = strings.ToUpper(strings.TrimSpace(plate))
	state = strings.ToUpper(strings.TrimSpace(state))

	m.mu.Lock()
	defer m.mu.Unlock()

	profile, exists := m.profiles[userID]
	if !exists {
		return fmt.Errorf("profile for user %q not found", userID)
	}

	idx := -1
	for i, v := range profile.Vehicles {
		if v.Plate == plate && v.State == state {
			idx = i
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("vehicle %s (%s) not found for user %s", plate, state, userID)
	}

	wasPrimary := profile.Vehicles[idx].IsPrimary
	profile.Vehicles = append(profile.Vehicles[:idx], profile.Vehicles[idx+1:]...)

	if wasPrimary && len(profile.Vehicles) > 0 {
		profile.Vehicles[0].IsPrimary = true
	}

	profile.UpdatedAt = time.Now().UTC()
	return nil
}

// GetVehicles returns all registered vehicles for a user.
func (m *ProfileManager) GetVehicles(userID string) []VehicleRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	profile, exists := m.profiles[strings.TrimSpace(userID)]
	if !exists || len(profile.Vehicles) == 0 {
		return []VehicleRecord{}
	}

	res := make([]VehicleRecord, len(profile.Vehicles))
	copy(res, profile.Vehicles)
	return res
}

