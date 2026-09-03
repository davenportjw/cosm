package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// HashBytes computes the SHA-256 hex string of raw bytes.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HashString computes the SHA-256 hex string of a UTF-8 string.
func HashString(s string) string {
	return HashBytes([]byte(s))
}

// HashLineage computes a deterministic SHA-256 digest of a LineageEnvelope.
func HashLineage(l *LineageEnvelope) string {
	if l == nil {
		return HashString("nil_lineage")
	}

	var tsStr string
	if !l.Timestamp.IsZero() {
		tsStr = l.Timestamp.UTC().Format(time.RFC3339Nano)
	}

	sigHex := ""
	if len(l.SignatureEd25519) > 0 {
		sigHex = hex.EncodeToString(l.SignatureEd25519)
	}

	var builder strings.Builder
	builder.WriteString("lineage:v1\n")
	builder.WriteString(fmt.Sprintf("user_id:%s\n", l.UserID))
	builder.WriteString(fmt.Sprintf("user_prompt:%s\n", l.UserPrompt))
	builder.WriteString(fmt.Sprintf("session_id:%s\n", l.SessionID))
	builder.WriteString(fmt.Sprintf("orchestrator_agent_id:%s\n", l.OrchestratorAgentID))
	builder.WriteString(fmt.Sprintf("executing_agent_id:%s\n", l.ExecutingAgentID))
	builder.WriteString(fmt.Sprintf("llm_version:%s\n", l.LLMVersion))
	builder.WriteString(fmt.Sprintf("generation_params:%s\n", l.GenerationParams))
	builder.WriteString(fmt.Sprintf("intent:%s\n", l.Intent))
	builder.WriteString(fmt.Sprintf("timestamp:%s\n", tsStr))
	builder.WriteString(fmt.Sprintf("signature_ed25519:%s\n", sigHex))

	return HashString(builder.String())
}

// HashLineageContent computes the deterministic content hash for lineage attributes excluding volatile timestamps.
// This is used for AST symbol node content addressing to preserve deduplication across commits.
func HashLineageContent(l *LineageEnvelope) string {
	if l == nil {
		return HashString("nil_lineage_content")
	}

	var builder strings.Builder
	builder.WriteString("lineage_content:v1\n")
	builder.WriteString(fmt.Sprintf("user_id:%s\n", l.UserID))
	builder.WriteString(fmt.Sprintf("user_prompt:%s\n", l.UserPrompt))
	builder.WriteString(fmt.Sprintf("orchestrator_agent_id:%s\n", l.OrchestratorAgentID))
	builder.WriteString(fmt.Sprintf("executing_agent_id:%s\n", l.ExecutingAgentID))
	builder.WriteString(fmt.Sprintf("llm_version:%s\n", l.LLMVersion))
	builder.WriteString(fmt.Sprintf("generation_params:%s\n", l.GenerationParams))
	builder.WriteString(fmt.Sprintf("intent:%s\n", l.Intent))

	return HashString(builder.String())
}

// HashASTSymbolNode computes the deterministic SHA-256 Merkle hash for an AST symbol node.
func HashASTSymbolNode(node *ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("ast symbol node cannot be nil")
	}

	payloadHash := HashBytes(node.ASTPayload)
	lineageHash := HashLineageContent(&node.Lineage)

	// Combine and sort dependencies for deterministic order
	depSet := make(map[string]bool)
	for _, d := range node.LocalDependencies {
		if d != "" {
			depSet[d] = true
		}
	}
	for _, d := range node.Dependencies {
		if d != "" {
			depSet[d] = true
		}
	}
	deps := make([]string, 0, len(depSet))
	for d := range depSet {
		deps = append(deps, d)
	}
	sort.Strings(deps)

	var builder strings.Builder
	builder.WriteString("ast_symbol_node:v1\n")
	builder.WriteString(fmt.Sprintf("language:%s\n", node.Language))
	builder.WriteString(fmt.Sprintf("node_type:%s\n", node.NodeType))
	builder.WriteString(fmt.Sprintf("identifier:%s\n", node.Identifier))
	if node.Signature != "" {
		builder.WriteString(fmt.Sprintf("signature:%s\n", node.Signature))
	}
	if node.Docstring != "" {
		builder.WriteString(fmt.Sprintf("docstring:%s\n", node.Docstring))
	}
	if node.Visibility != "" {
		builder.WriteString(fmt.Sprintf("visibility:%s\n", node.Visibility))
	}
	if len(node.ASTMetadata) > 0 {
		metaKeys := make([]string, 0, len(node.ASTMetadata))
		for k := range node.ASTMetadata {
			metaKeys = append(metaKeys, k)
		}
		sort.Strings(metaKeys)
		var metaPairs []string
		for _, k := range metaKeys {
			metaPairs = append(metaPairs, fmt.Sprintf("%s=%s", k, node.ASTMetadata[k]))
		}
		builder.WriteString(fmt.Sprintf("metadata:%s\n", strings.Join(metaPairs, ";")))
	}
	builder.WriteString(fmt.Sprintf("payload_sha256:%s\n", payloadHash))
	builder.WriteString(fmt.Sprintf("local_dependencies:%s\n", strings.Join(deps, ",")))
	builder.WriteString(fmt.Sprintf("lineage_hash:%s\n", lineageHash))

	hash := HashString(builder.String())
	return hash, nil
}

// HashCrossBoundaryEdge computes a deterministic SHA-256 digest of a CrossBoundaryEdge.
func HashCrossBoundaryEdge(edge *CrossBoundaryEdge) string {
	if edge == nil {
		return HashString("nil_edge")
	}

	var metaPairs []string
	if len(edge.Metadata) > 0 {
		keys := make([]string, 0, len(edge.Metadata))
		for k := range edge.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			metaPairs = append(metaPairs, fmt.Sprintf("%s=%s", k, edge.Metadata[k]))
		}
	}

	var builder strings.Builder
	builder.WriteString("cross_boundary_edge:v1\n")
	builder.WriteString(fmt.Sprintf("source:%s\n", edge.SourceNodeID))
	builder.WriteString(fmt.Sprintf("target:%s\n", edge.TargetNodeID))
	builder.WriteString(fmt.Sprintf("type:%s\n", edge.Type))
	builder.WriteString(fmt.Sprintf("contract_schema_id:%s\n", edge.ContractSchemaID))
	builder.WriteString(fmt.Sprintf("metadata:%s\n", strings.Join(metaPairs, ";")))

	return HashString(builder.String())
}

// HashComponentNode computes the deterministic SHA-256 Merkle hash for a ComponentNode.
func HashComponentNode(comp *ComponentNode) (string, error) {
	if comp == nil {
		return "", fmt.Errorf("component node cannot be nil")
	}

	// Sort symbol nodes for determinism
	symbols := make([]string, len(comp.SymbolNodes))
	copy(symbols, comp.SymbolNodes)
	sort.Strings(symbols)

	// Sort metadata keys
	var metaPairs []string
	if len(comp.Metadata) > 0 {
		keys := make([]string, 0, len(comp.Metadata))
		for k := range comp.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			metaPairs = append(metaPairs, fmt.Sprintf("%s=%s", k, comp.Metadata[k]))
		}
	}

	symbolsMerkleRoot := ComputeMerkleRoot(symbols)
	lineageHash := HashLineage(&comp.Lineage)

	var builder strings.Builder
	builder.WriteString("component_node:v1\n")
	builder.WriteString(fmt.Sprintf("name:%s\n", comp.Name))
	builder.WriteString(fmt.Sprintf("type:%s\n", comp.Type))
	builder.WriteString(fmt.Sprintf("language:%s\n", comp.Language))
	builder.WriteString(fmt.Sprintf("symbols_merkle_root:%s\n", symbolsMerkleRoot))
	builder.WriteString(fmt.Sprintf("metadata:%s\n", strings.Join(metaPairs, ";")))
	builder.WriteString(fmt.Sprintf("lineage_hash:%s\n", lineageHash))

	hash := HashString(builder.String())
	return hash, nil
}

// ComputeMerkleRoot calculates the deterministic pairwise binary Merkle root hash from a slice of leaf hashes.
func ComputeMerkleRoot(leafHashes []string) string {
	if len(leafHashes) == 0 {
		return HashString("merkle_empty")
	}
	if len(leafHashes) == 1 {
		return HashString("merkle_leaf:" + leafHashes[0])
	}

	currentLevel := make([]string, len(leafHashes))
	copy(currentLevel, leafHashes)

	for len(currentLevel) > 1 {
		var nextLevel []string
		for i := 0; i < len(currentLevel); i += 2 {
			if i+1 < len(currentLevel) {
				combined := currentLevel[i] + ":" + currentLevel[i+1]
				nextLevel = append(nextLevel, HashString("merkle_node:"+combined))
			} else {
				// Odd leaf: pair with itself to balance the tree
				combined := currentLevel[i] + ":" + currentLevel[i]
				nextLevel = append(nextLevel, HashString("merkle_node:"+combined))
			}
		}
		currentLevel = nextLevel
	}

	return currentLevel[0]
}

// HashWorkspaceManifest computes the deterministic Merkle root hash for a WorkspaceManifestNode.
func HashWorkspaceManifest(manifest *WorkspaceManifestNode) (string, error) {
	if manifest == nil {
		return "", fmt.Errorf("workspace manifest cannot be nil")
	}

	// Sort component IDs
	comps := make([]string, len(manifest.Components))
	copy(comps, manifest.Components)
	sort.Strings(comps)
	componentsRoot := ComputeMerkleRoot(comps)

	// Canonicalize and sort edge hashes
	edgeHashes := make([]string, len(manifest.CrossEdges))
	for i := range manifest.CrossEdges {
		edgeHashes[i] = HashCrossBoundaryEdge(&manifest.CrossEdges[i])
	}
	sort.Strings(edgeHashes)
	edgesRoot := ComputeMerkleRoot(edgeHashes)

	lineageHash := HashLineage(&manifest.Lineage)

	var builder strings.Builder
	builder.WriteString("workspace_manifest:v1\n")
	builder.WriteString(fmt.Sprintf("workspace_id:%s\n", manifest.WorkspaceID))
	builder.WriteString(fmt.Sprintf("universe_id:%s\n", manifest.UniverseID))
	builder.WriteString(fmt.Sprintf("components_merkle_root:%s\n", componentsRoot))
	builder.WriteString(fmt.Sprintf("edges_merkle_root:%s\n", edgesRoot))
	builder.WriteString(fmt.Sprintf("lineage_hash:%s\n", lineageHash))

	hash := HashString(builder.String())
	manifest.MerkleRootHash = hash
	return hash, nil
}
