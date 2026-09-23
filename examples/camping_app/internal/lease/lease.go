package lease

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// HoldToken encapsulates a temporary cryptographic lease on a campsite slot.
type HoldToken struct {
	TokenID string `json:"token_id"`
	CampsiteID string `json:"campsite_id"`
	UserID string `json:"user_id"`
	StartDate time.Time `json:"start_date"`
	EndDate time.Time `json:"end_date"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Status HoldStatus `json:"status"`
}

// Engine provides thread-safe in-memory hold contention, calendar indexing, and lease management.
type Engine struct {
	mu sync.RWMutex
	holds map[string]*HoldToken
	byCampsite map[string][]*HoldToken
	ttl time.Duration
}

// NewEngine initializes a new distributed lease engine with the default 15-minute hold TTL.
func NewEngine() *Engine {
	return &Engine{
		holds:      make(map[string]*HoldToken),
		byCampsite: make(map[string][]*HoldToken),
		ttl:        DefaultHoldTTL,
	}
}

// SetHoldTTL configures the hold TTL for the engine (used in tests or custom configuration).
func (e *Engine) SetHoldTTL(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ttl = d
}

// AcquireHold requests a temporary 15-minute hold on a campsite for the specified date range.
// Checks date validity and slot availability against active holds and committed reservations.
func (e *Engine) AcquireHold(campsiteID, userID string, start, end time.Time) (*HoldToken, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()

	// 1. Date validity check: start before end, and not in the past (with a 1-minute grace margin)
	if !start.Before(end) {
		return nil, ErrInvalidDates
	}
	if start.Before(now.Add(-1 * time.Minute)) {
		return nil, ErrInvalidDates
	}

	// 2. Check for overlap against active holds and committed reservations
	for _, existing := range e.byCampsite[campsiteID] {
		isActiveHold := existing.Status == StatusHeld && now.Before(existing.ExpiresAt)
		isCommitted := existing.Status == StatusCommitted

		if !isActiveHold && !isCommitted {
			if existing.Status == StatusHeld && !now.Before(existing.ExpiresAt) {
				existing.Status = StatusExpired
			}
			continue
		}

		// Half-open interval overlap check: [start, end) overlaps [existing.StartDate, existing.EndDate)
		if start.Before(existing.EndDate) && end.After(existing.StartDate) {
			return nil, ErrSlotUnavailable
		}
	}

	// 3. Generate cryptographic hold token (16-byte hex)
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate cryptographic hold token: %w", err)
	}
	tokenID := hex.EncodeToString(tokenBytes)

	// 4. Calculate expiration TTL
	ttl := e.ttl
	if ttl <= 0 {
		ttl = DefaultHoldTTL
	}
	expiresAt := now.Add(ttl)

	hold := &HoldToken{
		TokenID:    tokenID,
		CampsiteID: campsiteID,
		UserID:     userID,
		StartDate:  start,
		EndDate:    end,
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
		Status:     StatusHeld,
	}

	e.holds[tokenID] = hold
	e.byCampsite[campsiteID] = append(e.byCampsite[campsiteID], hold)

	result := *hold
	return &result, nil
}

// CommitHold transitions an active hold to COMMITTED, securing the reservation permanently.
func (e *Engine) CommitHold(tokenID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	hold, ok := e.holds[tokenID]
	if !ok {
		return ErrHoldNotFound
	}

	if hold.Status != StatusHeld {
		return ErrHoldNotActive
	}

	if !time.Now().Before(hold.ExpiresAt) {
		hold.Status = StatusExpired
		return ErrHoldExpired
	}

	hold.Status = StatusCommitted
	return nil
}

// ReleaseHold cancels and releases an active hold immediately, freeing the calendar slot.
func (e *Engine) ReleaseHold(tokenID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	hold, ok := e.holds[tokenID]
	if !ok {
		return ErrHoldNotFound
	}

	if hold.Status == StatusCancelled {
		return nil
	}

	hold.Status = StatusCancelled
	return nil
}

// GetActiveHolds returns all currently unexpired, active holds for a given campsite.
func (e *Engine) GetActiveHolds(campsiteID string) []*HoldToken {
	e.mu.RLock()
	defer e.mu.RUnlock()

	now := time.Now()
	active := make([]*HoldToken, 0)
	for _, h := range e.byCampsite[campsiteID] {
		if h.Status == StatusHeld && now.Before(h.ExpiresAt) {
			copy := *h
			active = append(active, &copy)
		}
	}
	return active
}

// CleanupExpiredHolds sweeps through all holds, transitions expired ones to EXPIRED, and returns count.
func (e *Engine) CleanupExpiredHolds() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	cleaned := 0
	for _, h := range e.holds {
		if h.Status == StatusHeld && !now.Before(h.ExpiresAt) {
			h.Status = StatusExpired
			cleaned++
		}
	}
	return cleaned
}

// GetHold retrieves an immutable snapshot of a hold by its token ID.
func (e *Engine) GetHold(tokenID string) (*HoldToken, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	h, ok := e.holds[tokenID]
	if !ok {
		return nil, ErrHoldNotFound
	}
	copy := *h
	return &copy, nil
}

// Standard lease errors
var (
	ErrSlotUnavailable = errors.New("slot unavailable: requested dates overlap with an active hold or committed reservation")
	ErrInvalidDates    = errors.New("invalid date range: start must be before end and not in the past")
	ErrHoldNotFound    = errors.New("hold token not found")
	ErrHoldExpired     = errors.New("hold token has expired")
	ErrHoldNotActive   = errors.New("hold is not active")
)
// HoldStatus represents the lifecycle state of a campsite inventory hold.
type HoldStatus string
const (
	StatusHeld      HoldStatus = "HELD"
	StatusCommitted HoldStatus = "COMMITTED"
	StatusExpired   HoldStatus = "EXPIRED"
	StatusCancelled HoldStatus = "CANCELLED"
)
// DefaultHoldTTL is the standard duration for temporary cart holds (15 minutes).
const DefaultHoldTTL = 15 * time.Minute
