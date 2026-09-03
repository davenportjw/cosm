package php

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// PHPHydrator reconstitutes PHP AST symbol nodes back into clean, formatted PHP source code.
type PHPHydrator struct{}

// NewPHPHydrator creates a new PHPHydrator instance.
func NewPHPHydrator() *PHPHydrator {
	return &PHPHydrator{}
}

// HydrateSymbol reconstitutes a single PHP ASTSymbolNode into source code.
func (h *PHPHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// PHP Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "ClassDef", "InterfaceDef", "EnumDef", "TraitDef":
		var cls PHPClass
		if err := json.Unmarshal(node.ASTPayload, &cls); err != nil {
			return "", fmt.Errorf("unmarshaling PHPClass: %w", err)
		}
		return h.renderClass(&cls), nil

	case "FunctionDef", "MethodDef":
		var fn PHPMethod
		if err := json.Unmarshal(node.ASTPayload, &fn); err != nil {
			return "", fmt.Errorf("unmarshaling PHPMethod: %w", err)
		}
		return h.renderMethod(&fn, 0), nil

	case "RouteBinding":
		var r PHPRouteBinding
		if err := json.Unmarshal(node.ASTPayload, &r); err != nil {
			return "", fmt.Errorf("unmarshaling PHPRouteBinding: %w", err)
		}
		return h.renderRoute(&r), nil

	default:
		return string(node.ASTPayload), nil
	}
}

func (h *PHPHydrator) renderClass(cls *PHPClass) string {
	var sb strings.Builder

	// Docblock
	if cls.Doc != "" {
		sb.WriteString("/**\n")
		for _, line := range strings.Split(cls.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(" * %s\n", line))
		}
		sb.WriteString(" */\n")
	}

	// Attributes
	for _, attr := range cls.Attributes {
		sb.WriteString(fmt.Sprintf("#[%s]\n", attr))
	}

	// Class Header
	var headerParts []string
	if cls.Visibility != "" && cls.Visibility != "public" {
		headerParts = append(headerParts, cls.Visibility)
	}

	kind := cls.Kind
	if kind == "" {
		kind = "class"
	}
	headerParts = append(headerParts, kind, cls.Name)

	if cls.Kind == "enum" && cls.BackedType != "" {
		headerParts = append(headerParts, ":", cls.BackedType)
	}

	if cls.Extends != "" {
		headerParts = append(headerParts, "extends", cls.Extends)
	}

	if len(cls.Implements) > 0 {
		headerParts = append(headerParts, "implements", strings.Join(cls.Implements, ", "))
	}

	sb.WriteString(fmt.Sprintf("%s\n{\n", strings.Join(headerParts, " ")))

	// Traits used
	for _, tr := range cls.TraitsUsed {
		sb.WriteString(fmt.Sprintf("    use %s;\n", tr))
	}
	if len(cls.TraitsUsed) > 0 {
		sb.WriteString("\n")
	}

	// Enum cases
	for _, ec := range cls.EnumCases {
		if ec.Doc != "" {
			sb.WriteString(fmt.Sprintf("    // %s\n", ec.Doc))
		}
		if ec.Value != "" {
			sb.WriteString(fmt.Sprintf("    case %s = '%s';\n", ec.Name, ec.Value))
		} else {
			sb.WriteString(fmt.Sprintf("    case %s;\n", ec.Name))
		}
	}
	if len(cls.EnumCases) > 0 {
		sb.WriteString("\n")
	}

	// Fields
	for _, f := range cls.Fields {
		sb.WriteString(h.renderField(&f))
	}
	if len(cls.Fields) > 0 {
		sb.WriteString("\n")
	}

	// Methods
	for _, m := range cls.Methods {
		sb.WriteString(h.renderMethod(&m, 1))
		sb.WriteString("\n")
	}

	sb.WriteString("}\n")
	return sb.String()
}

func (h *PHPHydrator) renderField(f *PHPField) string {
	var sb strings.Builder

	if f.Doc != "" {
		sb.WriteString(fmt.Sprintf("    /**\n     * %s\n     */\n", f.Doc))
	}

	for _, attr := range f.Attributes {
		sb.WriteString(fmt.Sprintf("    #[%s]\n", attr))
	}

	var parts []string
	parts = append(parts, "   ", f.Visibility)
	if f.IsStatic {
		parts = append(parts, "static")
	}
	if f.IsReadonly {
		parts = append(parts, "readonly")
	}
	if f.Type != "" {
		parts = append(parts, f.Type)
	}
	namePart := fmt.Sprintf("$%s", f.Name)
	if f.DefaultValue != "" {
		namePart = fmt.Sprintf("$%s = %s", f.Name, f.DefaultValue)
	}
	parts = append(parts, namePart+";")

	sb.WriteString(strings.Join(parts, " ") + "\n")
	return sb.String()
}

func (h *PHPHydrator) renderMethod(m *PHPMethod, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)
	var sb strings.Builder

	if m.Doc != "" {
		sb.WriteString(fmt.Sprintf("%s/**\n", indent))
		for _, line := range strings.Split(m.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s * %s\n", indent, line))
		}
		sb.WriteString(fmt.Sprintf("%s */\n", indent))
	}

	for _, attr := range m.Attributes {
		sb.WriteString(fmt.Sprintf("%s#[%s]\n", indent, attr))
	}

	var parts []string
	if m.IsAbstract {
		parts = append(parts, "abstract")
	}
	if m.IsFinal {
		parts = append(parts, "final")
	}
	if m.Visibility != "" {
		parts = append(parts, m.Visibility)
	} else if !m.IsAbstract && indentLevel > 0 {
		parts = append(parts, "public")
	}
	if m.IsStatic {
		parts = append(parts, "static")
	}
	parts = append(parts, "function", m.Name)

	var paramParts []string
	for _, p := range m.Params {
		var pParts []string
		if p.IsPromoted && p.Visibility != "" {
			pParts = append(pParts, p.Visibility)
		}
		if p.Type != "" {
			pParts = append(pParts, p.Type)
		}
		name := "$" + p.Name
		if p.IsVariadic {
			name = "..." + name
		}
		if p.DefaultValue != "" {
			name = fmt.Sprintf("%s = %s", name, p.DefaultValue)
		}
		pParts = append(pParts, name)
		paramParts = append(paramParts, strings.Join(pParts, " "))
	}

	sig := fmt.Sprintf("%s(%s)", strings.Join(parts, " "), strings.Join(paramParts, ", "))
	if m.ReturnType != "" {
		sig = fmt.Sprintf("%s: %s", sig, m.ReturnType)
	}

	if m.IsAbstract || m.BodySource == "" {
		sb.WriteString(fmt.Sprintf("%s%s;\n", indent, sig))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("%s%s\n%s{\n", indent, sig, indent))

	// Clean body source
	bodyLines := strings.Split(m.BodySource, "\n")
	var innerLines []string
	for idx, line := range bodyLines {
		if idx == 0 && strings.Contains(line, "function") {
			continue
		}
		if strings.TrimSpace(line) == "{" || strings.TrimSpace(line) == "}" {
			continue
		}
		innerLines = append(innerLines, line)
	}

	if len(innerLines) > 0 {
		for _, line := range innerLines {
			sb.WriteString(fmt.Sprintf("%s    %s\n", indent, strings.TrimSpace(line)))
		}
	} else {
		sb.WriteString(fmt.Sprintf("%s    // implementation\n", indent))
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
	return sb.String()
}

func (h *PHPHydrator) renderRoute(r *PHPRouteBinding) string {
	if r.Framework == "laravel" {
		if r.Method == "ANY" {
			return fmt.Sprintf("Route::apiResource('%s', %s::class);\n", strings.TrimPrefix(r.Path, "/"), r.HandlerName)
		}
		return fmt.Sprintf("Route::%s('%s', '%s');\n", strings.ToLower(r.Method), r.Path, r.HandlerName)
	}

	// Symfony attribute style comment
	return fmt.Sprintf("#[Route('%s', methods: ['%s'])] // %s\n", r.Path, r.Method, r.HandlerName)
}
