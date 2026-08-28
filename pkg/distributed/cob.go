package distributed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

// COBType represents the kind of collaborative object (Proposal, Review, Blackboard).
type COBType string

const (
	COBTypeProposal   COBType = "proposal"
	COBTypeReview     COBType = "review"
	COBTypeBlackboard COBType = "blackboard"
)

// ReviewStatus represents the state of a proposal review.
type ReviewStatus string

const (
	StatusOpen            ReviewStatus = "OPEN"
	StatusUnderReview     ReviewStatus = "UNDER_REVIEW"
	StatusChangesRequested ReviewStatus = "CHANGES_REQUESTED"
	StatusApproved        ReviewStatus = "APPROVED"
	StatusMerged          ReviewStatus = "MERGED"
	StatusClosed          ReviewStatus = "CLOSED"
)

// ReviewComment represents a human or AI critic comment on a proposal or specific AST symbol.
type ReviewComment struct {
	CommentID   string    `json:"comment_id"`
	AuthorDID   string    `json:"author_did"` // Decentralized Identifier (e.g. did:key:z6Mku...)
	SymbolID    string    `json:"symbol_id,omitempty"` // Target AST symbol if targeted comment
	Body        string    `json:"body"`
	Severity    string    `json:"severity,omitempty"` // "INFO", "WARNING", "BLOCKER"
	Timestamp   time.Time `json:"timestamp"`
	Signature   []byte    `json:"signature,omitempty"`
}

// ProposalRevision represents an immutable snapshot/revision in the proposal's history.
type ProposalRevision struct {
	RevisionID     string    `json:"revision_id"`
	SourceUniverse string    `json:"source_universe"`
	ManifestHash   string    `json:"manifest_hash"`
	AuthorDID      string    `json:"author_did"`
	Description    string    `json:"description"`
	Timestamp      time.Time `json:"timestamp"`
}

// ProposalCOB is a Conflict-Free Replicated Data Type (CRDT) representing a Pull Request / Universe Proposal.
type ProposalCOB struct {
	ID             string              `json:"id"` // Stable Change / Proposal ID (e.g. prop-auth-v1)
	Type           COBType             `json:"type"`
	Title          string              `json:"title"`
	TargetUniverse string              `json:"target_universe"` // Base branch (e.g. universe-main)
	AuthorDID      string              `json:"author_did"`
	Status         ReviewStatus        `json:"status"`
	CreatedAt      time.Time           `json:"created_at"`
	Revisions      []ProposalRevision  `json:"revisions"`
	Comments       []ReviewComment     `json:"comments"`
	Approvals      map[string]bool     `json:"approvals"` // DID -> Approved (true/false)
	FitnessScore   float64             `json:"fitness_score"`
	Lineage        core.LineageEnvelope `json:"lineage"`
	LamportClock   uint64              `json:"lamport_clock"`
	mu             sync.RWMutex
}

// NewProposalCOB creates a new decentralized Proposal Collaborative Object.
func NewProposalCOB(
	id, title, targetUniverse, authorDID string,
	initialSourceUniverse, initialManifestHash string,
	lineage core.LineageEnvelope,
) *ProposalCOB {
	now := time.Now().UTC()
	revID := fmt.Sprintf("rev-1-%s", shortenHash(initialManifestHash))

	p := &ProposalCOB{
		ID:             id,
		Type:           COBTypeProposal,
		Title:          title,
		TargetUniverse: targetUniverse,
		AuthorDID:      authorDID,
		Status:         StatusOpen,
		CreatedAt:      now,
		Revisions: []ProposalRevision{
			{
				RevisionID:     revID,
				SourceUniverse: initialSourceUniverse,
				ManifestHash:   initialManifestHash,
				AuthorDID:      authorDID,
				Description:    "Initial proposal revision",
				Timestamp:      now,
			},
		},
		Comments:     make([]ReviewComment, 0),
		Approvals:    make(map[string]bool),
		FitnessScore: 1.0,
		Lineage:      lineage,
		LamportClock: 1,
	}
	return p
}

// AddRevision appends a new state revision to the proposal (e.g. after agent updates or human edits).
func (p *ProposalCOB) AddRevision(sourceUniverse, manifestHash, authorDID, desc string) ProposalRevision {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.LamportClock++
	revID := fmt.Sprintf("rev-%d-%s", len(p.Revisions)+1, shortenHash(manifestHash))
	rev := ProposalRevision{
		RevisionID:     revID,
		SourceUniverse: sourceUniverse,
		ManifestHash:   manifestHash,
		AuthorDID:      authorDID,
		Description:    desc,
		Timestamp:      time.Now().UTC(),
	}
	p.Revisions = append(p.Revisions, rev)
	if p.Status == StatusChangesRequested {
		p.Status = StatusUnderReview
	}
	return rev
}

// AddComment records a review comment or critique on the proposal.
func (p *ProposalCOB) AddComment(comment ReviewComment) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.LamportClock++
	if comment.CommentID == "" {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", comment.AuthorDID, comment.Body, time.Now().UnixNano())))
		comment.CommentID = hex.EncodeToString(h[:8])
	}
	if comment.Timestamp.IsZero() {
		comment.Timestamp = time.Now().UTC()
	}
	p.Comments = append(p.Comments, comment)
}

// SetApproval records a peer/critic approval or rejection.
func (p *ProposalCOB) SetApproval(reviewerDID string, approved bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.LamportClock++
	p.Approvals[reviewerDID] = approved

	// Update high-level status based on approvals
	if approved {
		allApproved := true
		for _, ok := range p.Approvals {
			if !ok {
				allApproved = false
				break
			}
		}
		if allApproved && len(p.Approvals) > 0 {
			p.Status = StatusApproved
		}
	} else {
		p.Status = StatusChangesRequested
	}
}

// MergeCRDT performs deterministic state union (join operator) with another replica of this ProposalCOB.
func (p *ProposalCOB) MergeCRDT(other *ProposalCOB) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if other == nil || p.ID != other.ID {
		return
	}

	// 1. Advance Lamport Clock
	if other.LamportClock > p.LamportClock {
		p.LamportClock = other.LamportClock
	}
	p.LamportClock++

	// 2. Union revisions by RevisionID
	revMap := make(map[string]ProposalRevision)
	for _, r := range p.Revisions {
		revMap[r.RevisionID] = r
	}
	for _, r := range other.Revisions {
		revMap[r.RevisionID] = r
	}
	allRevs := make([]ProposalRevision, 0, len(revMap))
	for _, r := range revMap {
		allRevs = append(allRevs, r)
	}
	sort.Slice(allRevs, func(i, j int) bool {
		return allRevs[i].Timestamp.Before(allRevs[j].Timestamp)
	})
	p.Revisions = allRevs

	// 3. Union comments by CommentID
	commMap := make(map[string]ReviewComment)
	for _, c := range p.Comments {
		commMap[c.CommentID] = c
	}
	for _, c := range other.Comments {
		commMap[c.CommentID] = c
	}
	allComms := make([]ReviewComment, 0, len(commMap))
	for _, c := range commMap {
		allComms = append(allComms, c)
	}
	sort.Slice(allComms, func(i, j int) bool {
		return allComms[i].Timestamp.Before(allComms[j].Timestamp)
	})
	p.Comments = allComms

	// 4. Union approvals (last-write-wins by clock/presence)
	for did, ok := range other.Approvals {
		p.Approvals[did] = ok
	}

	// 5. Converge status: MERGED > APPROVED > CHANGES_REQUESTED > UNDER_REVIEW > OPEN
	if other.Status == StatusMerged || p.Status == StatusMerged {
		p.Status = StatusMerged
	} else if other.Status == StatusApproved && p.Status != StatusMerged {
		p.Status = StatusApproved
	} else if other.Status == StatusChangesRequested && p.Status == StatusOpen {
		p.Status = StatusChangesRequested
	}
}

// ClaimRecord represents an active domain lease held by an agent.
type ClaimRecord struct {
	Domain      string    `json:"domain"`
	AgentDID    string    `json:"agent_did"`
	Goal        string    `json:"goal"`
	ClaimedAt   time.Time `json:"claimed_at"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

// BlackboardCOB provides real-time shared memory coordination across autonomous agents.
type BlackboardCOB struct {
	DomainClaims map[string]string `json:"domain_claims"` // AST Identifier / Domain -> Agent DID
	ActiveGoals  map[string]string `json:"active_goals"`  // Agent DID -> Goal Description
	LamportClock uint64            `json:"lamport_clock"`
	mu           sync.RWMutex
}

// NewBlackboardCOB initializes a shared blackboard.
func NewBlackboardCOB() *BlackboardCOB {
	return &BlackboardCOB{
		DomainClaims: make(map[string]string),
		ActiveGoals:  make(map[string]string),
		LamportClock: 1,
	}
}

// ClaimDomain registers an agent's intent to mutate a domain/symbol, preventing race conditions.
func (b *BlackboardCOB) ClaimDomain(domain, agentDID, goal string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.LamportClock++
	if existing, claimed := b.DomainClaims[domain]; claimed && existing != agentDID {
		return false // Already claimed by another agent
	}
	b.DomainClaims[domain] = agentDID
	b.ActiveGoals[agentDID] = goal
	return true
}

// ReleaseDomain releases a previously claimed domain.
func (b *BlackboardCOB) ReleaseDomain(domain, agentDID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.LamportClock++
	if b.DomainClaims[domain] == agentDID {
		delete(b.DomainClaims, domain)
	}
}

// MergeCRDT joins two blackboard states across peer swarms.
func (b *BlackboardCOB) MergeCRDT(other *BlackboardCOB) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if other == nil {
		return
	}
	if other.LamportClock > b.LamportClock {
		b.LamportClock = other.LamportClock
	}
	b.LamportClock++

	for domain, agent := range other.DomainClaims {
		if _, exists := b.DomainClaims[domain]; !exists {
			b.DomainClaims[domain] = agent
		}
	}
	for agent, goal := range other.ActiveGoals {
		b.ActiveGoals[agent] = goal
	}
}

// MarshalJSON helper for ProposalCOB
func (p *ProposalCOB) ToJSON() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return json.MarshalIndent(p, "", "  ")
}

func shortenHash(h string) string {
	if len(h) <= 8 {
		return h
	}
	return h[:8]
}
