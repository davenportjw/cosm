package graphql

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// GraphQLHydrator reconstitutes GraphQL AST symbol nodes back into clean GraphQL SDL schema text.
type GraphQLHydrator struct{}

// NewGraphQLHydrator creates an initialized GraphQLHydrator instance.
func NewGraphQLHydrator() *GraphQLHydrator {
	return &GraphQLHydrator{}
}

// HydrateSymbol reconstitutes a single GraphQL ASTSymbolNode into GraphQL SDL schema text.
func (h *GraphQLHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("# GraphQL Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "SchemaDefinition":
		var s GraphQLSchemaDef
		if err := json.Unmarshal(node.ASTPayload, &s); err != nil {
			return "", fmt.Errorf("unmarshaling GraphQLSchemaDef: %w", err)
		}
		return h.renderSchema(&s), nil

	case "ObjectTypeDefinition", "InterfaceTypeDefinition", "InputObjectTypeDefinition", "EnumTypeDefinition", "UnionTypeDefinition", "ScalarTypeDefinition", "DirectiveDefinition":
		var t GraphQLType
		if err := json.Unmarshal(node.ASTPayload, &t); err != nil {
			return "", fmt.Errorf("unmarshaling GraphQLType: %w", err)
		}
		return h.renderType(&t), nil

	case "FieldDefinition":
		var f GraphQLField
		if err := json.Unmarshal(node.ASTPayload, &f); err != nil {
			return "", fmt.Errorf("unmarshaling GraphQLField: %w", err)
		}
		return h.renderField(&f, 0), nil

	default:
		return string(node.ASTPayload), nil
	}
}

func (h *GraphQLHydrator) renderSchema(s *GraphQLSchemaDef) string {
	var sb strings.Builder

	if s.Description != "" {
		sb.WriteString(h.renderDoc(s.Description, 0))
	}

	sb.WriteString("schema")
	for _, dir := range s.Directives {
		sb.WriteString(" ")
		sb.WriteString(dir.Raw)
	}
	sb.WriteString(" {\n")

	if s.Query != "" {
		sb.WriteString(fmt.Sprintf("  query: %s\n", s.Query))
	}
	if s.Mutation != "" {
		sb.WriteString(fmt.Sprintf("  mutation: %s\n", s.Mutation))
	}
	if s.Subscription != "" {
		sb.WriteString(fmt.Sprintf("  subscription: %s\n", s.Subscription))
	}

	sb.WriteString("}\n")
	return sb.String()
}

func (h *GraphQLHydrator) renderType(t *GraphQLType) string {
	var sb strings.Builder

	if t.Description != "" {
		sb.WriteString(h.renderDoc(t.Description, 0))
	}

	extendPrefix := ""
	if t.IsExtension {
		extendPrefix = "extend "
	}

	kind := t.Kind
	if kind == "" {
		kind = "type"
	}

	// 1. Scalar
	if kind == "scalar" {
		sb.WriteString(fmt.Sprintf("%sscalar %s", extendPrefix, t.Name))
		for _, dir := range t.Directives {
			sb.WriteString(" ")
			sb.WriteString(dir.Raw)
		}
		sb.WriteString("\n")
		return sb.String()
	}

	// 2. Union
	if kind == "union" {
		sb.WriteString(fmt.Sprintf("%sunion %s", extendPrefix, t.Name))
		for _, dir := range t.Directives {
			sb.WriteString(" ")
			sb.WriteString(dir.Raw)
		}
		if len(t.UnionTypes) > 0 {
			sb.WriteString(fmt.Sprintf(" = %s", strings.Join(t.UnionTypes, " | ")))
		}
		sb.WriteString("\n")
		return sb.String()
	}

	// 3. Directive Definition
	if kind == "directive" {
		sb.WriteString(fmt.Sprintf("directive @%s", t.Name))
		if len(t.Arguments) > 0 {
			sb.WriteString("(\n")
			for _, arg := range t.Arguments {
				sb.WriteString(h.renderArgument(&arg, 1))
			}
			sb.WriteString(")")
		}
		if len(t.Locations) > 0 {
			sb.WriteString(fmt.Sprintf(" on %s", strings.Join(t.Locations, " | ")))
		}
		sb.WriteString("\n")
		return sb.String()
	}

	// 4. Enum
	if kind == "enum" {
		sb.WriteString(fmt.Sprintf("%senum %s", extendPrefix, t.Name))
		for _, dir := range t.Directives {
			sb.WriteString(" ")
			sb.WriteString(dir.Raw)
		}
		sb.WriteString(" {\n")
		for _, ev := range t.EnumValues {
			if ev.Description != "" {
				sb.WriteString(h.renderDoc(ev.Description, 1))
			}
			dirStr := ""
			for _, dir := range ev.Directives {
				dirStr += " " + dir.Raw
			}
			sb.WriteString(fmt.Sprintf("  %s%s\n", ev.Name, dirStr))
		}
		sb.WriteString("}\n")
		return sb.String()
	}

	// 5. Object Type, Interface, Input
	implementsStr := ""
	if len(t.Implements) > 0 {
		implementsStr = fmt.Sprintf(" implements %s", strings.Join(t.Implements, " & "))
	}

	sb.WriteString(fmt.Sprintf("%s%s %s%s", extendPrefix, kind, t.Name, implementsStr))
	for _, dir := range t.Directives {
		sb.WriteString(" ")
		sb.WriteString(dir.Raw)
	}

	sb.WriteString(" {\n")
	for _, f := range t.Fields {
		sb.WriteString(h.renderField(&f, 1))
	}
	sb.WriteString("}\n")

	return sb.String()
}

func (h *GraphQLHydrator) renderField(f *GraphQLField, indentLevel int) string {
	var sb strings.Builder
	indent := strings.Repeat("  ", indentLevel)

	if f.Description != "" {
		sb.WriteString(h.renderDoc(f.Description, indentLevel))
	}

	sb.WriteString(indent)
	sb.WriteString(f.Name)

	if len(f.Arguments) > 0 {
		if len(f.Arguments) == 1 && f.Arguments[0].Description == "" {
			sb.WriteString(fmt.Sprintf("(%s: %s", f.Arguments[0].Name, f.Arguments[0].Type))
			if f.Arguments[0].DefaultValue != "" {
				sb.WriteString(fmt.Sprintf(" = %s", f.Arguments[0].DefaultValue))
			}
			sb.WriteString(")")
		} else {
			sb.WriteString("(\n")
			for _, arg := range f.Arguments {
				sb.WriteString(h.renderArgument(&arg, indentLevel+1))
			}
			sb.WriteString(indent)
			sb.WriteString(")")
		}
	}

	if f.Type != "" {
		sb.WriteString(fmt.Sprintf(": %s", f.Type))
	}

	if f.DefaultValue != "" {
		sb.WriteString(fmt.Sprintf(" = %s", f.DefaultValue))
	}

	for _, dir := range f.Directives {
		sb.WriteString(" ")
		sb.WriteString(dir.Raw)
	}

	sb.WriteString("\n")
	return sb.String()
}

func (h *GraphQLHydrator) renderArgument(arg *GraphQLArgument, indentLevel int) string {
	var sb strings.Builder
	indent := strings.Repeat("  ", indentLevel)

	if arg.Description != "" {
		sb.WriteString(h.renderDoc(arg.Description, indentLevel))
	}

	sb.WriteString(indent)
	sb.WriteString(arg.Name)
	sb.WriteString(fmt.Sprintf(": %s", arg.Type))

	if arg.DefaultValue != "" {
		sb.WriteString(fmt.Sprintf(" = %s", arg.DefaultValue))
	}

	for _, dir := range arg.Directives {
		sb.WriteString(" ")
		sb.WriteString(dir.Raw)
	}

	sb.WriteString("\n")
	return sb.String()
}

func (h *GraphQLHydrator) renderDoc(doc string, indentLevel int) string {
	indent := strings.Repeat("  ", indentLevel)
	trimmed := strings.TrimSpace(doc)
	if trimmed == "" {
		return ""
	}

	if !strings.Contains(trimmed, "\n") {
		return fmt.Sprintf("%s\"\"\"%s\"\"\"\n", indent, trimmed)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s\"\"\"\n", indent))
	for _, line := range strings.Split(trimmed, "\n") {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, line))
	}
	sb.WriteString(fmt.Sprintf("%s\"\"\"\n", indent))
	return sb.String()
}
