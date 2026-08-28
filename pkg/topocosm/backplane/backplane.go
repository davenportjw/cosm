package backplane

import (
	"context"
	"time"

	"github.com/cosmscm/cosm/pkg/distributed"
	"github.com/cosmscm/cosm/pkg/shipping"
	"github.com/cosmscm/cosm/pkg/storage"
)

// BlobStoreProvider defines content-addressed immutable storage operations.
type BlobStoreProvider interface {
	Put(data []byte) (string, error)
	Get(hash string) ([]byte, error)
	Has(hash string) (bool, error)
	Delete(hash string) error
	List() ([]string, error)
	Size() (int64, error)
}

// EventStreamProvider defines publish/subscribe event bus operations for oplogs and CRDT updates.
type EventStreamProvider interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Subscribe(ctx context.Context, topic string, handler func(topic string, payload []byte)) (func(), error)
	Close() error
}

// LeaseProvider manages domain locks and agent claims with TTLs.
type LeaseProvider interface {
	AcquireLease(ctx context.Context, key, ownerDID, goal string, ttl time.Duration) (bool, error)
	ReleaseLease(ctx context.Context, key, ownerDID string) error
	GetActiveLeases(ctx context.Context) (map[string]distributed.ClaimRecord, error)
}

// SandboxRunner executes isolated target builds and preview servers (Apple containers / local processes).
type SandboxRunner interface {
	ExecuteStagingBuild(ctx context.Context, targetKind shipping.TargetKind, vfsRoot string) (*SandboxResult, error)
}

// SandboxResult contains execution diagnostics from preview sandbox staging.
type SandboxResult struct {
	Success    bool          `json:"success"`
	PreviewURL string        `json:"preview_url,omitempty"`
	Duration   time.Duration `json:"duration"`
	OutputLogs string        `json:"output_logs"`
	ExitCode   int           `json:"exit_code"`
}

// CriticProvider executes autonomous critic reviews against proposals.
type CriticProvider interface {
	EvaluateProposal(ctx context.Context, proposal *distributed.ProposalCOB, diffSummary string) (*distributed.ProposalCOB, error)
}

// BackplaneProvider unifies storage, coordination, eventing, sandboxing, and criticism.
type BackplaneProvider interface {
	BlobStore() BlobStoreProvider
	GraphEngine() *storage.GraphEngine
	UniverseManager() *storage.UniverseManager
	EventStream() EventStreamProvider
	LeaseManager() LeaseProvider
	SandboxRunner() SandboxRunner
	CriticProvider() CriticProvider
	Close() error
}
