package core

import (
	"encoding/json"
	"fmt"
	"time"
)

// Language represents the programming language or schema domain of an AST node.
type Language string

const (
	LangGo         Language = "go"
	LangHCL        Language = "hcl"
	LangTypeScript Language = "typescript"
	LangPython     Language = "python"
	LangOpenAPI    Language = "openapi"
	LangRust       Language = "rust"
	LangJava       Language = "java"
	LangCpp        Language = "cpp"
	LangC          Language = "c"
	LangSQL        Language = "sql"
	LangProtobuf   Language = "protobuf"
	LangRaw        Language = "raw"
)

// IsValid checks if the language is a known supported language.
func (l Language) IsValid() bool {
	switch l {
	case LangGo, LangHCL, LangTypeScript, LangPython, LangOpenAPI,
		LangRust, LangJava, LangCpp, LangC, LangSQL, LangProtobuf, LangRaw:
		return true
	default:
		return false
	}
}

// String returns the string representation of the language.
func (l Language) String() string {
	return string(l)
}

// ComponentType designates the functional architectural tier of a component.
type ComponentType string

const (
	CompService  ComponentType = "service"
	CompInfra    ComponentType = "infra"
	CompFrontend ComponentType = "frontend"
	CompContract ComponentType = "contract"
	CompDatabase ComponentType = "database"
	CompModel    ComponentType = "model"
	CompLibrary  ComponentType = "library"
)

// IsValid checks if the component type is known.
func (c ComponentType) IsValid() bool {
	switch c {
	case CompService, CompInfra, CompFrontend, CompContract,
		CompDatabase, CompModel, CompLibrary:
		return true
	default:
		return false
	}
}

// String returns the string representation of the component type.
func (c ComponentType) String() string {
	return string(c)
}

// EdgeType defines semantic cross-boundary relationships between code symbols.
type EdgeType string

const (
	EdgeCalls         EdgeType = "CALLS"
	EdgeConsumesAPI   EdgeType = "CONSUMES_API"
	EdgeDeploysTo     EdgeType = "DEPLOYS_TO"
	EdgeBindsEnv      EdgeType = "BINDS_ENV"
	EdgeDependsOn     EdgeType = "DEPENDS_ON"
	EdgeQueriesTable  EdgeType = "QUERIES_TABLE"
	EdgeImplementsRPC EdgeType = "IMPLEMENTS_RPC"
)

// IsValid checks if the edge type is known.
func (e EdgeType) IsValid() bool {
	switch e {
	case EdgeCalls, EdgeConsumesAPI, EdgeDeploysTo, EdgeBindsEnv, EdgeDependsOn,
		EdgeQueriesTable, EdgeImplementsRPC:
		return true
	default:
		return false
	}
}

// String returns the string representation of the edge type.
func (e EdgeType) String() string {
	return string(e)
}

// LineageEnvelope records the unbroken causal pedigree from user prompt and agent sessions to code nodes.
type LineageEnvelope struct {
	UserID              string    `json:"user_id"`
	UserPrompt          string    `json:"user_prompt"`
	SessionID           string    `json:"session_id"`
	OrchestratorAgentID string    `json:"orchestrator_agent_id"`
	ExecutingAgentID    string    `json:"executing_agent_id"`
	LLMVersion          string    `json:"llm_version"`
	GenerationParams    string    `json:"generation_params"` // JSON string: temp, seed, etc.
	Intent              string    `json:"intent"`
	Timestamp           time.Time `json:"timestamp"`
	SignatureEd25519    []byte    `json:"signature_ed25519,omitempty"`
}

// Clone creates a deep copy of the LineageEnvelope.
func (l *LineageEnvelope) Clone() LineageEnvelope {
	clone := *l
	if l.SignatureEd25519 != nil {
		clone.SignatureEd25519 = make([]byte, len(l.SignatureEd25519))
		copy(clone.SignatureEd25519, l.SignatureEd25519)
	}
	return clone
}

// ASTSymbolNode represents an individual function, struct, resource block, or UI component.
type ASTSymbolNode struct {
	NodeID            string            `json:"node_id"` // Content-addressed SHA-256
	Language          Language          `json:"language"`
	NodeType          string            `json:"node_type"` // e.g., "FunctionDecl", "ResourceBlock", "StructDecl"
	Identifier        string            `json:"identifier"`
	Signature         string            `json:"signature,omitempty"`
	Docstring         string            `json:"docstring,omitempty"`
	Visibility        string            `json:"visibility,omitempty"`
	ASTPayload        []byte            `json:"ast_payload"` // Serialized AST binary or source snippet
	ASTMetadata       map[string]string `json:"ast_metadata,omitempty"`
	LocalDependencies []string          `json:"local_dependencies,omitempty"`
	Dependencies      []string          `json:"dependencies,omitempty"`
	Lineage           LineageEnvelope   `json:"lineage"`
}

// Validate checks basic structural validity of an ASTSymbolNode.
func (n *ASTSymbolNode) Validate() error {
	if n.NodeType == "" {
		return fmt.Errorf("node_type cannot be empty")
	}
	if n.Identifier == "" {
		return fmt.Errorf("identifier cannot be empty")
	}
	return nil
}

// CrossBoundaryEdge links symbols across different language and architectural domains.
type CrossBoundaryEdge struct {
	SourceNodeID     string            `json:"source_node_id"`
	TargetNodeID     string            `json:"target_node_id"`
	Type             EdgeType          `json:"type"`
	ContractSchemaID string            `json:"contract_schema_id,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// Validate checks structural validity of a CrossBoundaryEdge.
func (e *CrossBoundaryEdge) Validate() error {
	if e.SourceNodeID == "" {
		return fmt.Errorf("source_node_id cannot be empty")
	}
	if e.TargetNodeID == "" {
		return fmt.Errorf("target_node_id cannot be empty")
	}
	if !e.Type.IsValid() {
		return fmt.Errorf("invalid edge type: %s", e.Type)
	}
	return nil
}

// ComponentNode bundles symbols into an architectural module/subsystem (e.g. "auth-service", "infra-prod").
type ComponentNode struct {
	ComponentID string            `json:"component_id"` // Content-addressed SHA-256
	Name        string            `json:"name"`
	Type        ComponentType     `json:"type"`
	Language    Language          `json:"language"`
	SymbolNodes []string          `json:"symbol_nodes"` // List of ASTSymbolNode NodeIDs
	Metadata    map[string]string `json:"metadata,omitempty"`
	Lineage     LineageEnvelope   `json:"lineage"`
}

// Validate checks structural validity of a ComponentNode.
func (c *ComponentNode) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("component name cannot be empty")
	}
	if !c.Type.IsValid() {
		return fmt.Errorf("invalid component type: %s", c.Type)
	}
	return nil
}

// WorkspaceManifestNode is the content-addressed root of the repository state at a given commit/universe head.
type WorkspaceManifestNode struct {
	WorkspaceID    string              `json:"workspace_id"`
	UniverseID     string              `json:"universe_id"`
	MerkleRootHash string              `json:"merkle_root_hash"`
	Components     []string            `json:"components"` // List of ComponentNode ComponentIDs
	CrossEdges     []CrossBoundaryEdge `json:"cross_edges"`
	Lineage        LineageEnvelope     `json:"lineage"`
	CreatedAt      time.Time           `json:"created_at"`
}

// Validate checks structural validity of a WorkspaceManifestNode.
func (w *WorkspaceManifestNode) Validate() error {
	if w.WorkspaceID == "" {
		return fmt.Errorf("workspace_id cannot be empty")
	}
	if w.UniverseID == "" {
		return fmt.Errorf("universe_id cannot be empty")
	}
	return nil
}

// MarshalCanonical returns canonical JSON representation of any schema object.
func MarshalCanonical(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
