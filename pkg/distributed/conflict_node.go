package distributed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

// ConflictingVersion represents one concurrent agent's proposed version of a symbol or component.
type ConflictingVersion struct {
	AgentID    string               `json:"agent_id"`
	UniverseID string               `json:"universe_id"`
	ASTHash    string               `json:"ast_hash"`
	ASTPayload []byte               `json:"ast_payload"`
	Lineage    core.LineageEnvelope `json:"lineage"`
	Timestamp  time.Time            `json:"timestamp"`
}

// ASTConflictNode represents a first-class conflict state stored inside the Merkle DAG.
// Unlike Git which halts or creates text marker files (<<<< HEAD), Cosm models conflicts as graph nodes.
type ASTConflictNode struct {
	ConflictID       string               `json:"conflict_id"`
	TargetSymbolID   string               `json:"target_symbol_id"`
	TargetIdentifier string               `json:"target_identifier"`
	Language         core.Language        `json:"language"`
	BaseASTHash      string               `json:"base_ast_hash,omitempty"`
	Conflicting      []ConflictingVersion `json:"conflicting_versions"`
	Resolution       *ConflictingVersion  `json:"resolution,omitempty"` // Set when resolved
	ResolvedAt       *time.Time           `json:"resolved_at,omitempty"`
	Status           string               `json:"status"` // "UNRESOLVED", "AUTO_RESOLVED", "MANUAL_RESOLVED"
}

// NewASTConflictNode initializes a non-blocking conflict node.
func NewASTConflictNode(
	targetSymbolID, targetIdentifier string,
	lang core.Language,
	baseHash string,
	versions []ConflictingVersion,
) *ASTConflictNode {
	h := sha256.New()
	h.Write([]byte(targetSymbolID))
	for _, v := range versions {
		h.Write([]byte(v.ASTHash))
	}
	conflictID := fmt.Sprintf("conflict-%s", hex.EncodeToString(h.Sum(nil))[:12])

	return &ASTConflictNode{
		ConflictID:       conflictID,
		TargetSymbolID:   targetSymbolID,
		TargetIdentifier: targetIdentifier,
		Language:         lang,
		BaseASTHash:      baseHash,
		Conflicting:      versions,
		Status:           "UNRESOLVED",
	}
}

// Resolve marks the conflict as resolved with a selected or synthesized version.
func (c *ASTConflictNode) Resolve(winner ConflictingVersion, resolutionType string) {
	now := time.Now().UTC()
	c.Resolution = &winner
	c.ResolvedAt = &now
	c.Status = resolutionType
}

// IsResolved returns true if the conflict has been settled.
func (c *ASTConflictNode) IsResolved() bool {
	return c.Resolution != nil && c.Status != "UNRESOLVED"
}

// MarshalJSON helper for serialization
func (c *ASTConflictNode) ToJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}
