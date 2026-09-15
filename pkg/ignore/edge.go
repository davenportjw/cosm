package ignore

import (
	"path/filepath"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// EdgeIgnoreRule specifies matching criteria to suppress cross-boundary edges.
type EdgeIgnoreRule struct {
	EdgeType       core.EdgeType `json:"edge_type,omitempty"`
	SourcePathGlob string        `json:"source_path_glob,omitempty"`
	TargetPathGlob string        `json:"target_path_glob,omitempty"`
	SourceSymbol   string        `json:"source_symbol,omitempty"`
	TargetSymbol   string        `json:"target_symbol,omitempty"`
	EnvVar         string        `json:"env_var,omitempty"`
	EndpointGlob   string        `json:"endpoint_glob,omitempty"`
	TableGlob      string        `json:"table_glob,omitempty"`
	Raw            string        `json:"raw,omitempty"`
}

// EdgeFilter manages edge suppression rules across the workspace.
type EdgeFilter struct {
	Rules []EdgeIgnoreRule
}

// NewEdgeFilter creates an empty EdgeFilter.
func NewEdgeFilter() *EdgeFilter {
	return &EdgeFilter{
		Rules: make([]EdgeIgnoreRule, 0),
	}
}

// AddRule appends an EdgeIgnoreRule.
func (f *EdgeFilter) AddRule(rule EdgeIgnoreRule) {
	f.Rules = append(f.Rules, rule)
}

// ParseRuleLine parses declarative rule strings from .cosmignore.
// Supported formats:
//   ignore BINDS_ENV where env="TEST_*"
//   ignore CONSUMES_API where endpoint="/mock/*"
//   ignore QUERIES_TABLE where table="test_*"
//   ignore * where source="test/**"
//   edge:ignore BINDS_ENV env=TEST_*
//   edge:ignore CONSUMES_API target=/mock/*
//   edge:ignore * source=test/**
func (f *EdgeFilter) ParseRuleLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}

	clean := line
	if strings.HasPrefix(clean, "edge:") {
		clean = strings.TrimPrefix(clean, "edge:")
	}

	clean = strings.TrimSpace(clean)
	if !strings.HasPrefix(clean, "ignore") {
		return
	}

	clean = strings.TrimSpace(strings.TrimPrefix(clean, "ignore"))
	parts := strings.Fields(clean)
	if len(parts) == 0 {
		return
	}

	rule := EdgeIgnoreRule{Raw: line}
	edgeTypeStr := parts[0]
	if edgeTypeStr != "*" && edgeTypeStr != "all" {
		rule.EdgeType = core.EdgeType(edgeTypeStr)
	}

	remainder := strings.TrimSpace(strings.TrimPrefix(clean, edgeTypeStr))
	remainder = strings.TrimPrefix(remainder, "where")
	remainder = strings.TrimSpace(remainder)

	tokens := tokenizeClauses(remainder)
	for _, tok := range tokens {
		kv := strings.SplitN(tok, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.Trim(strings.TrimSpace(kv[1]), `"'`)

		switch key {
		case "source", "source_path":
			rule.SourcePathGlob = val
		case "target", "target_path":
			rule.TargetPathGlob = val
		case "source_symbol":
			rule.SourceSymbol = val
		case "target_symbol":
			rule.TargetSymbol = val
		case "env", "env_var":
			rule.EnvVar = val
		case "endpoint", "route":
			rule.EndpointGlob = val
		case "table":
			rule.TableGlob = val
		}
	}

	f.AddRule(rule)
}

func tokenizeClauses(str string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false

	for _, r := range str {
		if r == '"' || r == '\'' {
			inQuote = !inQuote
			cur.WriteRune(r)
		} else if (r == ' ' || r == '\t') && !inQuote {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// ShouldIgnoreEdge evaluates whether a cross-boundary edge should be suppressed.
func (f *EdgeFilter) ShouldIgnoreEdge(
	edge core.CrossBoundaryEdge,
	sourceNode, targetNode *core.ASTSymbolNode,
	sourcePath, targetPath string,
) bool {
	// 1. Check inline symbol metadata on source or target node
	if sourceNode != nil && checkInlineMetadata(sourceNode.ASTMetadata, edge) {
		return true
	}
	if targetNode != nil && checkInlineMetadata(targetNode.ASTMetadata, edge) {
		return true
	}

	// 2. Check workspace edge filter rules
	for _, rule := range f.Rules {
		if rule.EdgeType != "" && rule.EdgeType != edge.Type {
			continue
		}

		if rule.SourcePathGlob != "" && !globMatch(filepath.ToSlash(sourcePath), rule.SourcePathGlob) {
			continue
		}

		if rule.TargetPathGlob != "" && !globMatch(filepath.ToSlash(targetPath), rule.TargetPathGlob) {
			continue
		}

		if rule.SourceSymbol != "" && (sourceNode == nil || !globMatch(sourceNode.Identifier, rule.SourceSymbol)) {
			continue
		}

		if rule.TargetSymbol != "" && (targetNode == nil || !globMatch(targetNode.Identifier, rule.TargetSymbol)) {
			continue
		}

		if rule.EnvVar != "" {
			envVal := edge.Metadata["env_var"]
			if envVal == "" || !globMatch(envVal, rule.EnvVar) {
				continue
			}
		}

		if rule.EndpointGlob != "" {
			endpoint := edge.Metadata["consumer_endpoint"]
			if endpoint == "" {
				endpoint = edge.Metadata["backend_endpoint"]
			}
			if endpoint == "" || !globMatch(endpoint, rule.EndpointGlob) {
				continue
			}
		}

		if rule.TableGlob != "" {
			tbl := edge.Metadata["table_name"]
			if tbl == "" || !globMatch(tbl, rule.TableGlob) {
				continue
			}
		}

		return true
	}

	return false
}

func checkInlineMetadata(meta map[string]string, edge core.CrossBoundaryEdge) bool {
	if meta == nil {
		return false
	}

	directive := meta["cosm:ignore-edge"]
	if directive == "" {
		directive = meta["ignore-edge"]
	}
	if directive == "" {
		return false
	}

	// Directives can be: "ALL", "BINDS_ENV", "BINDS_ENV:TEST_KEY", "CONSUMES_API:/mock/*"
	parts := strings.Split(directive, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.EqualFold(p, "ALL") || strings.EqualFold(p, "*") {
			return true
		}
		if strings.EqualFold(p, string(edge.Type)) {
			return true
		}

		subParts := strings.SplitN(p, ":", 2)
		if len(subParts) == 2 {
			t := strings.TrimSpace(subParts[0])
			spec := strings.TrimSpace(subParts[1])
			if strings.EqualFold(t, string(edge.Type)) {
				if edge.Type == core.EdgeBindsEnv && globMatch(edge.Metadata["env_var"], spec) {
					return true
				}
				if edge.Type == core.EdgeConsumesAPI && (globMatch(edge.Metadata["consumer_endpoint"], spec) || globMatch(edge.Metadata["backend_endpoint"], spec)) {
					return true
				}
				if edge.Type == core.EdgeQueriesTable && globMatch(edge.Metadata["table_name"], spec) {
					return true
				}
			}
		}
	}

	return false
}
