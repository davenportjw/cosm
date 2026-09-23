package backcountry

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	mathrand "math/rand"
)

// TrailheadZone defines a regulated backcountry wilderness access point.
type TrailheadZone struct {
	ID string `json:"id"`
	Name string `json:"name"`
	ElevationFt int `json:"elevation_ft"`
	DailyQuota int `json:"daily_quota"`
	BearCanisterRequired bool `json:"bear_canister_required"`
	WAGBagRequired bool `json:"wag_bag_required"`
	DifficultyRating string `json:"difficulty_rating"`
}

// LotteryApplication represents a permit lottery request submitted by a backcountry party.
type LotteryApplication struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	FullName string `json:"full_name"`
	TrailheadID string `json:"trailhead_id"`
	TargetDate time.Time `json:"target_date"`
	PartySize int `json:"party_size"`
	LeaveNoTraceCertID string `json:"leave_no_trace_cert_id"`
	SubmittedAt time.Time `json:"submitted_at"`
	Status string `json:"status"`
}

// LotteryDrawCommitment contains the cryptographic commitment for a provably fair draw.
type LotteryDrawCommitment struct {
	TrailheadID string `json:"trailhead_id"`
	DrawDate time.Time `json:"draw_date"`
	CommitmentHash string `json:"commitment_hash"`
	PublishedAt time.Time `json:"published_at"`
	RevealedSecret string `json:"revealed_secret"`
	DrawnAt *time.Time `json:"drawn_at,omitempty"`
}

// BackcountryEngine coordinates trailhead quotas, application admissions, and provably fair draws.
type BackcountryEngine struct {
	mu sync.RWMutex
	zones map[string]*TrailheadZone
	dailyQuotas map[string]int
	entries map[string][]*LotteryApplication
	appsByID map[string]*LotteryApplication
	commitments map[string]*LotteryDrawCommitment
}

// NewBackcountryEngine constructs a new thread-safe BackcountryEngine.
func NewBackcountryEngine() *BackcountryEngine {
	return &BackcountryEngine{
		zones:       make(map[string]*TrailheadZone),
		dailyQuotas: make(map[string]int),
		entries:     make(map[string][]*LotteryApplication),
		appsByID:    make(map[string]*LotteryApplication),
		commitments: make(map[string]*LotteryDrawCommitment),
	}
}

// DeriveLotterySeed generates the canonical deterministic seed string for a given trailhead and target date.
func DeriveLotterySeed(trailheadID string, targetDate time.Time) string {
	return fmt.Sprintf("%s:%s", trailheadID, targetDate.UTC().Format("2006-01-02"))
}

// ComputeCommitmentHash computes the hex-encoded SHA-256 commitment of the secret salt combined with the seed.
func ComputeCommitmentHash(secretSalt, seed string) string {
	sum := sha256.Sum256([]byte(secretSalt + ":" + seed))
	return hex.EncodeToString(sum[:])
}

// RegisterZone adds a new regulated trailhead zone to the backcountry registry.
func (b *BackcountryEngine) RegisterZone(z *TrailheadZone) error {
	if z == nil {
		return errors.New("zone cannot be nil")
	}
	if strings.TrimSpace(z.ID) == "" {
		return errors.New("zone id cannot be empty")
	}
	if z.DailyQuota <= 0 {
		return fmt.Errorf("zone %s daily quota must be positive, got %d", z.ID, z.DailyQuota)
	}

	switch z.DifficultyRating {
	case DifficultyModerate, DifficultyStrenuous, DifficultyAlpineTechnical:
		// Valid
	default:
		return fmt.Errorf("invalid difficulty rating %q; must be MODERATE, STRENUOUS, or ALPINE_TECHNICAL", z.DifficultyRating)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.zones[z.ID] = z
	return nil
}

// GetZone retrieves a registered trailhead zone by ID.
func (b *BackcountryEngine) GetZone(id string) (*TrailheadZone, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	z, ok := b.zones[id]
	if !ok {
		return nil, fmt.Errorf("zone %s not found", id)
	}
	return z, nil
}

// ListZones returns all registered trailhead zones sorted by ID.
func (b *BackcountryEngine) ListZones() []*TrailheadZone {
	b.mu.RLock()
	defer b.mu.RUnlock()

	zones := make([]*TrailheadZone, 0, len(b.zones))
	for _, z := range b.zones {
		zones = append(zones, z)
	}
	sort.Slice(zones, func(i, j int) bool {
		return zones[i].ID < zones[j].ID
	})
	return zones
}

// PublishLotteryCommitment generates and locks a cryptographic commitment hash before the lottery draw.
func (b *BackcountryEngine) PublishLotteryCommitment(trailheadID string, drawDate time.Time, secretSalt string) (*LotteryDrawCommitment, error) {
	if strings.TrimSpace(secretSalt) == "" {
		return nil, errors.New("secret salt cannot be empty")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.zones[trailheadID]; !ok {
		return nil, fmt.Errorf("trailhead zone %s not found", trailheadID)
	}

	seed := DeriveLotterySeed(trailheadID, drawDate)
	key := seed
	if existing, ok := b.commitments[key]; ok && existing.DrawnAt != nil {
		return nil, fmt.Errorf("lottery for trailhead %s on %s has already been drawn", trailheadID, drawDate.UTC().Format("2006-01-02"))
	}

	commitmentHash := ComputeCommitmentHash(secretSalt, seed)
	commitment := &LotteryDrawCommitment{
		TrailheadID:    trailheadID,
		DrawDate:       drawDate.UTC(),
		CommitmentHash: commitmentHash,
		PublishedAt:    time.Now().UTC(),
		RevealedSecret: "",
		DrawnAt:        nil,
	}

	b.commitments[key] = commitment
	return commitment, nil
}

// SubmitApplication admits a new lottery entry after validating party size and Leave No Trace certification.
func (b *BackcountryEngine) SubmitApplication(app *LotteryApplication) error {
	if app == nil {
		return errors.New("lottery application cannot be nil")
	}
	if strings.TrimSpace(app.UserID) == "" {
		return errors.New("user_id cannot be empty")
	}
	if strings.TrimSpace(app.FullName) == "" {
		return errors.New("full_name cannot be empty")
	}
	if strings.TrimSpace(app.TrailheadID) == "" {
		return errors.New("trailhead_id cannot be empty")
	}
	if app.TargetDate.IsZero() {
		return errors.New("target_date must be specified")
	}
	if app.PartySize <= 0 {
		return errors.New("party size must be greater than zero")
	}
	if strings.TrimSpace(app.LeaveNoTraceCertID) == "" {
		return errors.New("leave no trace certification ID is required")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	zone, ok := b.zones[app.TrailheadID]
	if !ok {
		return fmt.Errorf("trailhead zone %s not found", app.TrailheadID)
	}

	if app.PartySize > zone.DailyQuota {
		return fmt.Errorf("party size %d exceeds trailhead zone daily quota %d", app.PartySize, zone.DailyQuota)
	}

	if strings.TrimSpace(app.ID) == "" {
		var randBytes [8]byte
		_, _ = rand.Read(randBytes[:])
		app.ID = fmt.Sprintf("app-%s", hex.EncodeToString(randBytes[:]))
	}

	if app.SubmittedAt.IsZero() {
		app.SubmittedAt = time.Now().UTC()
	}
	app.Status = StatusPending
	app.TargetDate = app.TargetDate.UTC()

	key := DeriveLotterySeed(app.TrailheadID, app.TargetDate)
	b.entries[key] = append(b.entries[key], app)
	b.appsByID[app.ID] = app

	return nil
}

// ExecuteProvablyFairDraw verifies the cryptographic commitment, deterministically seeds a PRNG via HMAC-SHA256,
// shuffles applicant pools, awards permits up to daily quota, and marks outcomes.
func (b *BackcountryEngine) ExecuteProvablyFairDraw(trailheadID string, targetDate time.Time, secretSalt string) ([]*LotteryApplication, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	zone, ok := b.zones[trailheadID]
	if !ok {
		return nil, fmt.Errorf("trailhead zone %s not found", trailheadID)
	}

	seed := DeriveLotterySeed(trailheadID, targetDate)
	commitment, ok := b.commitments[seed]
	if !ok {
		return nil, fmt.Errorf("no published lottery commitment found for trailhead %s on %s", trailheadID, targetDate.UTC().Format("2006-01-02"))
	}

	// 1. Verify SHA-256 commitment hash against secretSalt + seed
	expectedHash := ComputeCommitmentHash(secretSalt, seed)
	if expectedHash != commitment.CommitmentHash {
		return nil, fmt.Errorf("tampered commitment: provided secret salt does not match published commitment hash (expected %s, computed %s)", commitment.CommitmentHash, expectedHash)
	}

	// 2. Reveal secret and mark draw timestamp
	now := time.Now().UTC()
	commitment.RevealedSecret = secretSalt
	commitment.DrawnAt = &now

	// 3. Extract eligible pending applicants
	apps := b.entries[seed]
	var pending []*LotteryApplication
	for _, app := range apps {
		if app.Status == StatusPending {
			pending = append(pending, app)
		}
	}

	if len(pending) == 0 {
		return []*LotteryApplication{}, nil
	}

	// 4. Sort deterministically before PRNG shuffle to eliminate Go map/slice iteration non-determinism
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].SubmittedAt.Equal(pending[j].SubmittedAt) {
			return pending[i].ID < pending[j].ID
		}
		return pending[i].SubmittedAt.Before(pending[j].SubmittedAt)
	})

	// 5. Deterministically seed PRNG with HMAC-SHA256(secretSalt, commitmentHash)
	mac := hmac.New(sha256.New, []byte(secretSalt))
	mac.Write([]byte(commitment.CommitmentHash))
	hmacBytes := mac.Sum(nil)

	seedUint := binary.BigEndian.Uint64(hmacBytes[:8])
	prng := mathrand.New(mathrand.NewSource(int64(seedUint)))

	// 6. Fair Fisher-Yates shuffle
	shuffled := make([]*LotteryApplication, len(pending))
	copy(shuffled, pending)
	prng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	// 7. Award permits up to available quota
	currentAvailableQuota := b.getAvailableQuotaLocked(trailheadID, targetDate, zone)
	var winners []*LotteryApplication

	for _, app := range shuffled {
		if app.PartySize <= currentAvailableQuota {
			app.Status = StatusWon
			currentAvailableQuota -= app.PartySize
			winners = append(winners, app)
		} else {
			app.Status = StatusUnsuccessful
		}
	}

	// Record remaining quota after lottery allocation
	b.dailyQuotas[seed] = currentAvailableQuota

	return winners, nil
}

// GetAvailableQuota returns the remaining permit capacity for a given trailhead on a specific date.
func (b *BackcountryEngine) GetAvailableQuota(trailheadID string, date time.Time) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	zone, ok := b.zones[trailheadID]
	if !ok {
		return 0
	}
	return b.getAvailableQuotaLocked(trailheadID, date, zone)
}

// DecrementQuota reduces the available capacity for a given trailhead and date (e.g., walk-up permits or group bookings).
func (b *BackcountryEngine) DecrementQuota(trailheadID string, date time.Time, count int) error {
	if count <= 0 {
		return fmt.Errorf("decrement count must be positive, got %d", count)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	zone, ok := b.zones[trailheadID]
	if !ok {
		return fmt.Errorf("trailhead zone %s not found", trailheadID)
	}

	key := DeriveLotterySeed(trailheadID, date)
	current := b.getAvailableQuotaLocked(trailheadID, date, zone)
	if current < count {
		return fmt.Errorf("insufficient quota for %s on %s: requested %d, available %d", trailheadID, date.UTC().Format("2006-01-02"), count, current)
	}

	b.dailyQuotas[key] = current - count
	return nil
}

// GetApplication looks up an individual lottery application by ID.
func (b *BackcountryEngine) GetApplication(id string) (*LotteryApplication, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	app, ok := b.appsByID[id]
	if !ok {
		return nil, fmt.Errorf("application %s not found", id)
	}
	return app, nil
}

// GetCommitment looks up the draw commitment for a trailhead and date.
func (b *BackcountryEngine) GetCommitment(trailheadID string, date time.Time) (*LotteryDrawCommitment, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	key := DeriveLotterySeed(trailheadID, date)
	com, ok := b.commitments[key]
	if !ok {
		return nil, fmt.Errorf("commitment not found for %s on %s", trailheadID, date.UTC().Format("2006-01-02"))
	}
	return com, nil
}

// getAvailableQuotaLocked retrieves remaining quota while holding either read or write lock.
func (b *BackcountryEngine) getAvailableQuotaLocked(trailheadID string, date time.Time, zone *TrailheadZone) int {
	key := DeriveLotterySeed(trailheadID, date)
	if rem, exists := b.dailyQuotas[key]; exists {
		return rem
	}
	return zone.DailyQuota
}

// Difficulty ratings for backcountry trailhead zones.
const (
	DifficultyModerate        = "MODERATE"
	DifficultyStrenuous        = "STRENUOUS"
	DifficultyAlpineTechnical = "ALPINE_TECHNICAL"
)
// Status values for backcountry lottery applications.
const (
	StatusPending      = "PENDING"
	StatusWon          = "WON"
	StatusUnsuccessful = "UNSUCCESSFUL"
	StatusConfirmed    = "CONFIRMED"
)
