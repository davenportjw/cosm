package elixir

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// ElixirHydrator reconstitutes Elixir AST symbol nodes back into clean, idiomatic Elixir source code.
type ElixirHydrator struct{}

// NewElixirHydrator creates a new ElixirHydrator instance.
func NewElixirHydrator() *ElixirHydrator {
	return &ElixirHydrator{}
}

// HydrateSymbol reconstitutes a single Elixir ASTSymbolNode into source code.
func (h *ElixirHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("# Elixir Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "ModuleDef":
		var mod ElixirModule
		if err := json.Unmarshal(node.ASTPayload, &mod); err != nil {
			return "", fmt.Errorf("unmarshaling ElixirModule: %w", err)
		}
		return h.renderModule(&mod), nil

	case "FunctionDef", "MacroDef":
		var fn ElixirFunction
		if err := json.Unmarshal(node.ASTPayload, &fn); err != nil {
			return "", fmt.Errorf("unmarshaling ElixirFunction: %w", err)
		}
		return h.renderFunction(&fn, 0), nil

	case "RouteBinding":
		var r ElixirRouteBinding
		if err := json.Unmarshal(node.ASTPayload, &r); err != nil {
			return "", fmt.Errorf("unmarshaling ElixirRouteBinding: %w", err)
		}
		return h.renderRoute(&r), nil

	default:
		return string(node.ASTPayload), nil
	}
}

func (h *ElixirHydrator) renderModule(m *ElixirModule) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("defmodule %s do\n", m.Name))

	// Moduledoc
	if m.Doc != "" {
		if strings.Contains(m.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("  @moduledoc \"\"\"\n  %s\n  \"\"\"\n\n", strings.ReplaceAll(m.Doc, "\n", "\n  ")))
		} else {
			sb.WriteString(fmt.Sprintf("  @moduledoc \"%s\"\n\n", m.Doc))
		}
	}

	// Uses
	for _, u := range m.Uses {
		sb.WriteString(fmt.Sprintf("  use %s\n", u))
	}

	// Imports
	for _, imp := range m.Imports {
		sb.WriteString(fmt.Sprintf("  import %s\n", imp))
	}

	// Aliases
	for _, a := range m.Aliases {
		sb.WriteString(fmt.Sprintf("  alias %s\n", a))
	}

	// Requires
	for _, r := range m.Requires {
		sb.WriteString(fmt.Sprintf("  require %s\n", r))
	}

	// Behaviours
	for _, b := range m.Behaviours {
		sb.WriteString(fmt.Sprintf("  @behaviour %s\n", b))
	}

	// Plugs
	for _, p := range m.Plugs {
		sb.WriteString(fmt.Sprintf("  plug %s\n", p))
	}

	if len(m.Uses) > 0 || len(m.Imports) > 0 || len(m.Aliases) > 0 || len(m.Requires) > 0 || len(m.Behaviours) > 0 || len(m.Plugs) > 0 {
		sb.WriteString("\n")
	}

	// Defstruct
	if len(m.StructFields) > 0 {
		sb.WriteString(fmt.Sprintf("  defstruct [%s]\n\n", strings.Join(m.StructFields, ", ")))
	}

	// Ecto Schema
	if m.IsSchema && m.SchemaTable != "" {
		sb.WriteString(fmt.Sprintf("  schema \"%s\" do\n", m.SchemaTable))
		for _, f := range m.SchemaFields {
			if f.AssociationType != "" {
				sb.WriteString(fmt.Sprintf("    %s :%s, %s\n", f.AssociationType, f.Name, f.TargetSchema))
			} else {
				sb.WriteString(fmt.Sprintf("    field :%s, %s\n", f.Name, f.Type))
			}
		}
		sb.WriteString("  end\n\n")
	}

	// Functions
	for _, fn := range m.Functions {
		sb.WriteString(h.renderFunction(&fn, 1))
		sb.WriteString("\n")
	}

	sb.WriteString("end\n")
	return sb.String()
}

func (h *ElixirHydrator) renderFunction(fn *ElixirFunction, indentLevel int) string {
	indent := strings.Repeat("  ", indentLevel)
	var sb strings.Builder

	// @doc
	if fn.Doc != "" {
		if strings.Contains(fn.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s@doc \"\"\"\n%s  %s\n%s\"\"\"\n", indent, indent, strings.ReplaceAll(fn.Doc, "\n", "\n  "), indent))
		} else {
			sb.WriteString(fmt.Sprintf("%s@doc \"%s\"\n", indent, fn.Doc))
		}
	}

	// @spec
	if fn.Spec != "" {
		sb.WriteString(fmt.Sprintf("%s@spec %s\n", indent, fn.Spec))
	}

	kind := fn.Kind
	if kind == "" {
		kind = "def"
	}

	var paramStrs []string
	for _, p := range fn.Params {
		if p.DefaultValue != "" {
			paramStrs = append(paramStrs, fmt.Sprintf("%s \\\\ %s", p.Pattern, p.DefaultValue))
		} else {
			paramStrs = append(paramStrs, p.Pattern)
		}
	}

	paramPart := ""
	if len(paramStrs) > 0 || fn.Arity > 0 {
		paramPart = fmt.Sprintf("(%s)", strings.Join(paramStrs, ", "))
	}

	header := fmt.Sprintf("%s %s%s", kind, fn.Name, paramPart)
	if fn.GuardClause != "" {
		header = fmt.Sprintf("%s when %s", header, fn.GuardClause)
	}

	if fn.BodySource == "" {
		sb.WriteString(fmt.Sprintf("%s%s, do: nil\n", indent, header))
		return sb.String()
	}

	// If single line do:
	if strings.Contains(fn.BodySource, "do:") {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, strings.TrimSpace(fn.BodySource)))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("%s%s do\n", indent, header))

	// Clean body lines
	bodyLines := strings.Split(fn.BodySource, "\n")
	var innerLines []string
	for idx, l := range bodyLines {
		t := strings.TrimSpace(l)
		if idx == 0 && (strings.HasPrefix(t, "def") || strings.HasPrefix(t, "defp")) {
			continue
		}
		if t == "do" || t == "end" {
			continue
		}
		if t != "" {
			innerLines = append(innerLines, t)
		}
	}

	if len(innerLines) > 0 {
		for _, l := range innerLines {
			sb.WriteString(fmt.Sprintf("%s  %s\n", indent, l))
		}
	} else {
		sb.WriteString(fmt.Sprintf("%s  :ok\n", indent))
	}

	sb.WriteString(fmt.Sprintf("%send\n", indent))
	return sb.String()
}

func (h *ElixirHydrator) renderRoute(r *ElixirRouteBinding) string {
	methodLower := strings.ToLower(r.Method)
	return fmt.Sprintf("%s \"%s\", %s, :%s\n", methodLower, r.Path, r.Controller, r.Action)
}
