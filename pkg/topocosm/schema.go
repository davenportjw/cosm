package topocosm

import (
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/distributed"
)

// Visibility defines whether a Cosm repository is public or private.
type Visibility string

const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

// Role defines user/agent permissions within an organization or Cosm.
type Role string

const (
	RoleOwner     Role = "owner"
	RoleCommitter Role = "committer"
	RoleReviewer  Role = "reviewer"
	RoleCritic    Role = "critic"
	RoleReader    Role = "reader"
)

// CosmRepository represents a published Cosm hosted on topocosm.dev.
type CosmRepository struct {
	OrgSlug         string                      `json:"org_slug"`         // e.g. "acme-corp"
	CosmName        string                      `json:"cosm_name"`        // e.g. "cloud-billing"
	Description     string                      `json:"description"`
	Visibility      Visibility                  `json:"visibility"`
	DefaultUniverse string                      `json:"default_universe"` // e.g. "universe-main"
	OwnerDID        string                      `json:"owner_did"`        // did:key:z6M...
	MerkleRootHash  string                      `json:"merkle_root_hash"` // Current head Merkle root hash
	Tags            []string                    `json:"tags,omitempty"`
	StarCount       int                         `json:"star_count"`
	CreatedAt       time.Time                   `json:"created_at"`
	UpdatedAt       time.Time                   `json:"updated_at"`
	Manifest        *core.WorkspaceManifestNode `json:"manifest,omitempty"`
}

// FullSlug returns "org/cosm" identifier.
func (c *CosmRepository) FullSlug() string {
	return c.OrgSlug + "/" + c.CosmName
}

// UserAccount represents a registered human user on topocosm.dev.
type UserAccount struct {
	UserDID     string    `json:"user_did"`    // did:key:z6Mku...
	Username    string    `json:"username"`    // e.g. "jason"
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	PublicKeys  []string  `json:"public_keys"` // Ed25519 hex/base64 public keys
	SSHKeys     []string  `json:"ssh_keys"`    // OpenSSH public keys
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Organization represents a shared workspace / company namespace.
type Organization struct {
	OrgID         string          `json:"org_id"`
	Slug          string          `json:"slug"`         // e.g. "acme-corp"
	DisplayName   string          `json:"display_name"`
	PlanType      string          `json:"plan_type"`    // "free", "team", "enterprise"
	Members       map[string]Role `json:"members"`      // DID -> Role
	ComputeBudget int             `json:"compute_budget"` // Sandbox seconds / month
	CreatedAt     time.Time       `json:"created_at"`
}

// AgentIdentity represents a registered machine / autonomous agent worker.
type AgentIdentity struct {
	AgentDID            string    `json:"agent_did"`
	AgentName           string    `json:"agent_name"`
	ModelVersion        string    `json:"model_version"` // e.g. "gemini-3.7-flash"
	CapabilityToken     string    `json:"capability_token"`
	ScopedComponents    []string  `json:"scoped_components,omitempty"` // e.g. ["services/billing/*"]
	MaxConcurrentClaims int       `json:"max_concurrent_claims"`
	LeaseTimeoutSec     int       `json:"lease_timeout_sec"`
	CreatedAt           time.Time `json:"created_at"`
}

// AttestationPolicy defines cryptographic verification rules for incoming proposals.
type AttestationPolicy struct {
	RequirePromptProvenance     bool    `json:"require_prompt_provenance"`
	RequireModelSignatures      bool    `json:"require_model_signatures"`
	RequireCriticScoreThreshold bool    `json:"require_critic_score_threshold"`
	MinCriticScore              float64 `json:"min_critic_score"` // 0.0 - 100.0
}

// AgentDiscoveryManifest is served at /.well-known/cosm-agent.json for automated agent discovery.
type AgentDiscoveryManifest struct {
	CosmVersion        string            `json:"cosm_version"`
	HubURL             string            `json:"hub_url"`
	GRPCEndpoint       string            `json:"grpc_endpoint"`
	RESTEndpoint       string            `json:"rest_endpoint"`
	MCPEndpoint        string            `json:"mcp_endpoint"`
	Capabilities       map[string]bool   `json:"capabilities"`
	SupportedLanguages []string          `json:"supported_languages"`
	AttestationPolicy  AttestationPolicy `json:"attestation_policy"`
	Timestamp          time.Time         `json:"timestamp"`
}

// PublishPayload contains the AST DAG data transmitted during a publish or push operation.
type PublishPayload struct {
	OrgSlug     string                         `json:"org_slug"`
	CosmName    string                         `json:"cosm_name"`
	UniverseID  string                         `json:"universe_id"`
	Manifest    *core.WorkspaceManifestNode    `json:"manifest"`
	Components  map[string]*core.ComponentNode `json:"components"`
	Symbols     map[string]*core.ASTSymbolNode `json:"symbols"`
	Blobs       map[string][]byte              `json:"blobs"` // Hash -> Raw bytes
	Lineage     core.LineageEnvelope           `json:"lineage"`
	IsInitial   bool                           `json:"is_initial"`
	Visibility  Visibility                     `json:"visibility"`
	Description string                         `json:"description"`
}

// PublishResponse is returned after a successful publish or push.
type PublishResponse struct {
	OrgSlug        string    `json:"org_slug"`
	CosmName       string    `json:"cosm_name"`
	UniverseID     string    `json:"universe_id"`
	MerkleRootHash string    `json:"merkle_root_hash"`
	BlobsStored    int       `json:"blobs_stored"`
	Success        bool      `json:"success"`
	Message        string    `json:"message"`
	Timestamp      time.Time `json:"timestamp"`
}

// SparsePullRequest specifies which components, languages, or contract edges to extract.
type SparsePullRequest struct {
	OrgSlug          string          `json:"org_slug"`
	CosmName         string          `json:"cosm_name"`
	UniverseID       string          `json:"universe_id"`
	ComponentNames   []string        `json:"component_names,omitempty"`
	Languages        []core.Language `json:"languages,omitempty"`
	IncludeContracts bool            `json:"include_contracts"`
}

// SparsePullResponse packages the minimal content-addressed subtree.
type SparsePullResponse struct {
	Manifest        *core.WorkspaceManifestNode    `json:"manifest"`
	Components      map[string]*core.ComponentNode `json:"components"`
	Symbols         map[string]*core.ASTSymbolNode `json:"symbols"`
	Blobs           map[string][]byte              `json:"blobs"`
	TotalBlobsCount int                            `json:"total_blobs_count"`
	SavingsPercent  float64                        `json:"savings_percent"`
}

// BlackboardClaimRequest is used by an agent to claim a domain lease.
type BlackboardClaimRequest struct {
	OrgSlug     string `json:"org_slug"`
	CosmName    string `json:"cosm_name"`
	Domain      string `json:"domain"` // e.g. "services/billing"
	AgentDID    string `json:"agent_did"`
	Goal        string `json:"goal"`
	LeaseTTLSec int    `json:"lease_ttl_sec,omitempty"`
}

// BlackboardReleaseRequest is used by an agent to release a claimed domain lease.
type BlackboardReleaseRequest struct {
	OrgSlug  string `json:"org_slug"`
	CosmName string `json:"cosm_name"`
	Domain   string `json:"domain"`
	AgentDID string `json:"agent_did"`
}

// ReviewSubmission represents a human or AI critic review submission for a proposal.
type ReviewSubmission struct {
	ProposalID   string                      `json:"proposal_id"`
	ReviewerDID  string                      `json:"reviewer_did"`
	Approved     bool                        `json:"approved"`
	FitnessScore float64                     `json:"fitness_score"`
	Comments     []distributed.ReviewComment `json:"comments"`
}

// HubStats provides high-level health and volume statistics.
type HubStats struct {
	TotalCosms       int       `json:"total_cosms"`
	TotalBlobs       int       `json:"total_blobs"`
	TotalProposals   int       `json:"total_proposals"`
	ActiveClaims     int       `json:"active_claims"`
	RegisteredAgents int       `json:"registered_agents"`
	UptimeSec        int64     `json:"uptime_sec"`
	Timestamp        time.Time `json:"timestamp"`
}
