package lineage

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

const (
	InTotoStatementV1       = "https://in-toto.io/Statement/v1"
	ProvenancePredicateType = "https://future-of-git.dev/attestation/provenance/v1"
)

// ProvenanceSubject identifies the artifact being attested.
type ProvenanceSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"` // algorithm -> hex digest (e.g. "sha256": "...")
}

// ProvenanceBuilder identifies the human or AI agent that built/generated the artifact.
type ProvenanceBuilder struct {
	ID        string `json:"id"`
	AgentType string `json:"agent_type,omitempty"`
	Version   string `json:"version,omitempty"`
}

// ProvenanceRecipe records the prompt, model params, and instructions used.
type ProvenanceRecipe struct {
	UserPrompt       string `json:"user_prompt"`
	Intent           string `json:"intent,omitempty"`
	LLMVersion       string `json:"llm_version,omitempty"`
	GenerationParams string `json:"generation_params,omitempty"`
}

// ProvenanceMetadata records timing and session context.
type ProvenanceMetadata struct {
	BuildStartedOn  time.Time `json:"build_started_on"`
	BuildFinishedOn time.Time `json:"build_finished_on"`
	SessionID       string    `json:"session_id,omitempty"`
	Completeness    bool      `json:"completeness"`
	Reproducible    bool      `json:"reproducible"`
}

// ProvenancePredicate constitutes the predicate payload of the attestation statement.
type ProvenancePredicate struct {
	Builder   ProvenanceBuilder   `json:"builder"`
	Recipe    ProvenanceRecipe    `json:"recipe"`
	Materials []string            `json:"materials,omitempty"` // Local dependencies / hashes
	Metadata  ProvenanceMetadata  `json:"metadata"`
}

// AttestationSignature records the Ed25519 signature over the canonical JSON statement.
type AttestationSignature struct {
	KeyID     string    `json:"key_id"`
	Signature string    `json:"signature"` // hex encoded Ed25519 signature
	SignedAt  time.Time `json:"signed_at"`
}

// ProvenanceAttestation is the standard in-toto v1 formatted cryptographic statement.
type ProvenanceAttestation struct {
	Type          string                 `json:"_type"`
	Subject       []ProvenanceSubject    `json:"subject"`
	PredicateType string                 `json:"predicateType"`
	Predicate     ProvenancePredicate    `json:"predicate"`
	Signatures    []AttestationSignature `json:"signatures"`
}

// GenerateKeyPair generates a new Ed25519 keypair for cryptographic signing.
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate Ed25519 keypair: %w", err)
	}
	return pub, priv, nil
}

// PublicKeyToHex encodes an Ed25519 public key as a hexadecimal string.
func PublicKeyToHex(pub ed25519.PublicKey) string {
	return hex.EncodeToString(pub)
}

// PrivateKeyToHex encodes an Ed25519 private key as a hexadecimal string.
func PrivateKeyToHex(priv ed25519.PrivateKey) string {
	return hex.EncodeToString(priv)
}

// HexToPublicKey decodes a hex string to an Ed25519 public key.
func HexToPublicKey(hexStr string) (ed25519.PublicKey, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %w", err)
	}
	if len(bytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size: expected %d bytes, got %d", ed25519.PublicKeySize, len(bytes))
	}
	return ed25519.PublicKey(bytes), nil
}

// HexToPrivateKey decodes a hex string to an Ed25519 private key.
func HexToPrivateKey(hexStr string) (ed25519.PrivateKey, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %w", err)
	}
	if len(bytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size: expected %d bytes, got %d", ed25519.PrivateKeySize, len(bytes))
	}
	return ed25519.PrivateKey(bytes), nil
}

// SignEnvelope signs the canonical hash of a LineageEnvelope using Ed25519 and attaches the signature.
func SignEnvelope(env *core.LineageEnvelope, privKey ed25519.PrivateKey) error {
	if env == nil {
		return fmt.Errorf("lineage envelope is nil")
	}

	// Create envelope copy without existing signature for deterministic signing
	envCopy := env.Clone()
	envCopy.SignatureEd25519 = nil

	digest := core.HashLineage(&envCopy)
	digestBytes := []byte(digest)

	sig := ed25519.Sign(privKey, digestBytes)
	env.SignatureEd25519 = sig

	return nil
}

// VerifyEnvelope verifies an Ed25519 digital signature on a LineageEnvelope.
func VerifyEnvelope(env *core.LineageEnvelope, pubKey ed25519.PublicKey) (bool, error) {
	if env == nil {
		return false, fmt.Errorf("lineage envelope is nil")
	}
	if len(env.SignatureEd25519) == 0 {
		return false, fmt.Errorf("lineage envelope has no signature")
	}

	envCopy := env.Clone()
	sig := envCopy.SignatureEd25519
	envCopy.SignatureEd25519 = nil

	digest := core.HashLineage(&envCopy)
	digestBytes := []byte(digest)

	valid := ed25519.Verify(pubKey, digestBytes, sig)
	return valid, nil
}

// CreateAttestation creates and signs an in-toto style provenance attestation for an ASTSymbolNode.
func CreateAttestation(node *core.ASTSymbolNode, privKey ed25519.PrivateKey, keyID string) (*ProvenanceAttestation, error) {
	if node == nil {
		return nil, fmt.Errorf("node is nil")
	}

	payloadHash := sha256.Sum256(node.ASTPayload)
	payloadHashHex := hex.EncodeToString(payloadHash[:])

	subject := ProvenanceSubject{
		Name: node.Identifier,
		Digest: map[string]string{
			"sha256": payloadHashHex,
			"nodeID": node.NodeID,
		},
	}

	now := time.Now().UTC()
	predicate := ProvenancePredicate{
		Builder: ProvenanceBuilder{
			ID:        node.Lineage.ExecutingAgentID,
			AgentType: "ai_code_generator",
			Version:   node.Lineage.LLMVersion,
		},
		Recipe: ProvenanceRecipe{
			UserPrompt:       node.Lineage.UserPrompt,
			Intent:           node.Lineage.Intent,
			LLMVersion:       node.Lineage.LLMVersion,
			GenerationParams: node.Lineage.GenerationParams,
		},
		Materials: node.LocalDependencies,
		Metadata: ProvenanceMetadata{
			BuildStartedOn:  node.Lineage.Timestamp,
			BuildFinishedOn: now,
			SessionID:       node.Lineage.SessionID,
			Completeness:    true,
			Reproducible:    false,
		},
	}

	att := &ProvenanceAttestation{
		Type:          InTotoStatementV1,
		Subject:       []ProvenanceSubject{subject},
		PredicateType: ProvenancePredicateType,
		Predicate:     predicate,
	}

	// Sign the statement
	statementBytes, err := json.Marshal(struct {
		Type          string              `json:"_type"`
		Subject       []ProvenanceSubject `json:"subject"`
		PredicateType string              `json:"predicateType"`
		Predicate     ProvenancePredicate `json:"predicate"`
	}{
		Type:          att.Type,
		Subject:       att.Subject,
		PredicateType: att.PredicateType,
		Predicate:     att.Predicate,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal attestation statement: %w", err)
	}

	sigBytes := ed25519.Sign(privKey, statementBytes)
	att.Signatures = []AttestationSignature{
		{
			KeyID:     keyID,
			Signature: hex.EncodeToString(sigBytes),
			SignedAt:  now,
		},
	}

	return att, nil
}

// VerifyAttestation validates the Ed25519 cryptographic signature of a ProvenanceAttestation.
func VerifyAttestation(att *ProvenanceAttestation, pubKey ed25519.PublicKey) (bool, error) {
	if att == nil {
		return false, fmt.Errorf("attestation is nil")
	}
	if len(att.Signatures) == 0 {
		return false, fmt.Errorf("attestation has no signatures")
	}

	statementBytes, err := json.Marshal(struct {
		Type          string              `json:"_type"`
		Subject       []ProvenanceSubject `json:"subject"`
		PredicateType string              `json:"predicateType"`
		Predicate     ProvenancePredicate `json:"predicate"`
	}{
		Type:          att.Type,
		Subject:       att.Subject,
		PredicateType: att.PredicateType,
		Predicate:     att.Predicate,
	})
	if err != nil {
		return false, fmt.Errorf("failed to marshal attestation for verification: %w", err)
	}

	for _, sig := range att.Signatures {
		sigBytes, err := hex.DecodeString(sig.Signature)
		if err != nil {
			return false, fmt.Errorf("failed to decode signature hex: %w", err)
		}

		if ed25519.Verify(pubKey, statementBytes, sigBytes) {
			return true, nil
		}
	}

	return false, fmt.Errorf("no matching valid signature found")
}

// SaveAttestation writes a signed attestation JSON file under .cosm/attestations/.
func SaveAttestation(baseDir string, att *ProvenanceAttestation) (string, error) {
	if att == nil || len(att.Subject) == 0 {
		return "", fmt.Errorf("invalid attestation")
	}

	attDir := filepath.Join(baseDir, ".cosm", "attestations")
	if _, err := os.Stat(filepath.Join(baseDir, ".fg", "attestations")); err == nil {
		if _, err := os.Stat(filepath.Join(baseDir, ".cosm")); os.IsNotExist(err) {
			attDir = filepath.Join(baseDir, ".fg", "attestations")
		}
	}
	if err := os.MkdirAll(attDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create attestation directory: %w", err)
	}

	nodeID := att.Subject[0].Digest["nodeID"]
	if nodeID == "" {
		nodeID = att.Subject[0].Digest["sha256"]
	}
	fileName := fmt.Sprintf("%s.attestation.json", nodeID)
	filePath := filepath.Join(attDir, fileName)

	data, err := json.MarshalIndent(att, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal attestation: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write attestation file: %w", err)
	}

	return filePath, nil
}

// LoadAttestation reads a ProvenanceAttestation JSON file from disk.
func LoadAttestation(filePath string) (*ProvenanceAttestation, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read attestation file %s: %w", filePath, err)
	}

	var att ProvenanceAttestation
	if err := json.Unmarshal(data, &att); err != nil {
		return nil, fmt.Errorf("failed to unmarshal attestation: %w", err)
	}

	return &att, nil
}
