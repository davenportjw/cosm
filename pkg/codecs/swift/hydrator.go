package swift

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// SwiftHydrator reconstitutes Swift AST symbol nodes back into clean, formatted Swift source code.
type SwiftHydrator struct{}

// NewSwiftHydrator creates an initialized SwiftHydrator instance.
func NewSwiftHydrator() *SwiftHydrator {
	return &SwiftHydrator{}
}

// HydrateSymbol reconstitutes a single Swift ASTSymbolNode into source text.
func (h *SwiftHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// Swift Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "StructDecl", "ClassDecl", "ProtocolDecl", "ExtensionDecl", "EnumDecl", "ActorDecl", "SwiftUIView":
		var t SwiftTypeDecl
		if err := json.Unmarshal(node.ASTPayload, &t); err != nil {
			return "", fmt.Errorf("unmarshaling SwiftTypeDecl: %w", err)
		}
		return h.renderTypeDecl(&t), nil

	case "MethodDeclaration", "FunctionDecl":
		var m SwiftMethod
		if err := json.Unmarshal(node.ASTPayload, &m); err != nil {
			return "", fmt.Errorf("unmarshaling SwiftMethod: %w", err)
		}
		return h.renderMethod(&m, 0), nil

	case "ApiClientCall":
		var api SwiftAPICall
		if err := json.Unmarshal(node.ASTPayload, &api); err != nil {
			return "", fmt.Errorf("unmarshaling SwiftAPICall: %w", err)
		}
		return fmt.Sprintf("// API Client Request: %s %s via %s\n", api.Method, api.URL, api.Caller), nil

	default:
		return string(node.ASTPayload), nil
	}
}

// HydrateFile reconstitutes a complete Swift source file from a SwiftFileResult.
func (h *SwiftHydrator) HydrateFile(res *SwiftFileResult) (string, error) {
	if res == nil {
		return "", fmt.Errorf("SwiftFileResult is nil")
	}

	var sb strings.Builder

	// Imports
	if len(res.Imports) > 0 {
		for _, imp := range res.Imports {
			sb.WriteString(fmt.Sprintf("import %s\n", imp))
		}
		sb.WriteString("\n")
	}

	// Types
	for i, t := range res.Types {
		sb.WriteString(h.renderTypeDecl(&t))
		if i < len(res.Types)-1 || len(res.TopLevelFunctions) > 0 {
			sb.WriteString("\n")
		}
	}

	// Top-level functions
	for i, fn := range res.TopLevelFunctions {
		sb.WriteString(h.renderMethod(&fn, 0))
		if i < len(res.TopLevelFunctions)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

func (h *SwiftHydrator) renderTypeDecl(t *SwiftTypeDecl) string {
	// If BodySource is present and valid, we can format it
	if t.BodySource != "" {
		return strings.TrimSpace(t.BodySource) + "\n"
	}

	var sb strings.Builder

	// Docstrings
	if t.Doc != "" {
		for _, line := range strings.Split(t.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", line))
		}
	}

	// Attributes (e.g. @MainActor, @frozen)
	for _, attr := range t.Attributes {
		sb.WriteString(fmt.Sprintf("%s\n", attr))
	}

	vis := ""
	if t.Visibility != "" && t.Visibility != "internal" {
		vis = t.Visibility + " "
	}

	kind := t.Kind
	if kind == "" {
		if t.IsSwiftUIView {
			kind = "struct"
		} else {
			kind = "struct"
		}
	}

	generics := ""
	if t.Generics != "" {
		generics = fmt.Sprintf("<%s>", t.Generics)
	}

	inherit := ""
	if len(t.Inheritance) > 0 {
		inherit = fmt.Sprintf(": %s", strings.Join(t.Inheritance, ", "))
	}

	sb.WriteString(fmt.Sprintf("%s%s %s%s%s {\n", vis, kind, t.Name, generics, inherit))

	// Render Enum Cases
	for _, ec := range t.EnumCases {
		sb.WriteString(h.renderEnumCase(&ec, 1))
	}

	// Render Properties
	for _, p := range t.Properties {
		sb.WriteString(h.renderProperty(&p, 1))
	}

	if (len(t.EnumCases) > 0 || len(t.Properties) > 0) && len(t.Methods) > 0 {
		sb.WriteString("\n")
	}

	// Render Methods
	for _, m := range t.Methods {
		sb.WriteString(h.renderMethod(&m, 1))
		sb.WriteString("\n")
	}

	// Special case for SwiftUI View without body property
	if t.IsSwiftUIView && len(t.Properties) == 0 && len(t.Methods) == 0 {
		sb.WriteString("    var body: some View {\n")
		sb.WriteString("        Text(\"Hello, World!\")\n")
		sb.WriteString("    }\n")
	}

	sb.WriteString("}\n")
	return sb.String()
}

func (h *SwiftHydrator) renderEnumCase(ec *SwiftEnumCase, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)
	var sb strings.Builder

	if ec.Doc != "" {
		for _, line := range strings.Split(ec.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s/// %s\n", indent, line))
		}
	}

	assoc := ""
	if len(ec.AssociatedValues) > 0 {
		assoc = fmt.Sprintf("(%s)", strings.Join(ec.AssociatedValues, ", "))
	}
	raw := ""
	if ec.RawValue != "" {
		raw = fmt.Sprintf(" = %s", ec.RawValue)
	}

	sb.WriteString(fmt.Sprintf("%scase %s%s%s\n", indent, ec.Name, assoc, raw))
	return sb.String()
}

func (h *SwiftHydrator) renderProperty(p *SwiftProperty, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)
	var sb strings.Builder

	if p.Doc != "" {
		for _, line := range strings.Split(p.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s/// %s\n", indent, line))
		}
	}

	for _, attr := range p.Attributes {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, attr))
	}

	vis := ""
	if p.Visibility != "" && p.Visibility != "internal" {
		vis = p.Visibility + " "
	}
	staticStr := ""
	if p.IsStatic {
		staticStr = "static "
	}
	lazyStr := ""
	if p.IsLazy {
		lazyStr = "lazy "
	}
	letVar := "var"
	if p.IsLet {
		letVar = "let"
	}

	typeStr := ""
	if p.Type != "" {
		typeStr = fmt.Sprintf(": %s", p.Type)
	}

	defVal := ""
	if p.DefaultValue != "" {
		defVal = fmt.Sprintf(" = %s", p.DefaultValue)
	}

	if p.GetterBody != "" {
		sb.WriteString(fmt.Sprintf("%s%s%s%s%s %s%s {\n%s\n%s}\n", indent, vis, staticStr, lazyStr, letVar, p.Name, typeStr, p.GetterBody, indent))
	} else {
		sb.WriteString(fmt.Sprintf("%s%s%s%s%s %s%s%s\n", indent, vis, staticStr, lazyStr, letVar, p.Name, typeStr, defVal))
	}

	return sb.String()
}

func (h *SwiftHydrator) renderMethod(m *SwiftMethod, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)

	if m.BodySource != "" {
		trimmed := strings.TrimSpace(m.BodySource)
		var sb strings.Builder
		for _, line := range strings.Split(trimmed, "\n") {
			if strings.TrimSpace(line) != "" {
				sb.WriteString(fmt.Sprintf("%s%s\n", indent, line))
			}
		}
		return sb.String()
	}

	var sb strings.Builder

	if m.Doc != "" {
		for _, line := range strings.Split(m.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s/// %s\n", indent, line))
		}
	}

	for _, attr := range m.Attributes {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, attr))
	}

	vis := ""
	if m.Visibility != "" && m.Visibility != "internal" {
		vis = m.Visibility + " "
	}
	staticStr := ""
	if m.IsClass {
		staticStr = "class "
	} else if m.IsStatic {
		staticStr = "static "
	}
	mutatingStr := ""
	if m.IsMutating {
		mutatingStr = "mutating "
	}

	var paramStrs []string
	for _, p := range m.Params {
		lbl := ""
		if p.Label != "" && p.Label != p.Name {
			lbl = p.Label + " "
		}
		defVal := ""
		if p.DefaultValue != "" {
			defVal = fmt.Sprintf(" = %s", p.DefaultValue)
		}
		attrStr := ""
		if len(p.Attributes) > 0 {
			attrStr = strings.Join(p.Attributes, " ") + " "
		}
		paramStrs = append(paramStrs, fmt.Sprintf("%s%s%s: %s%s", attrStr, lbl, p.Name, p.Type, defVal))
	}

	asyncStr := ""
	if m.IsAsync {
		asyncStr = " async"
	}
	throwsStr := ""
	if m.Throws {
		throwsStr = " throws"
	}

	retStr := ""
	if m.ReturnType != "" && m.ReturnType != "Void" && m.ReturnType != "()" {
		retStr = fmt.Sprintf(" -> %s", m.ReturnType)
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%sfunc %s(%s)%s%s%s {\n", indent, vis, staticStr, mutatingStr, m.Name, strings.Join(paramStrs, ", "), asyncStr, throwsStr, retStr))
	sb.WriteString(fmt.Sprintf("%s    // Reconstituted implementation\n", indent))
	sb.WriteString(fmt.Sprintf("%s}\n", indent))

	return sb.String()
}
