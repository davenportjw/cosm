package java

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// JavaHydrator reconstitutes Java AST symbol nodes back into clean, formatted Java source code.
type JavaHydrator struct{}

// NewJavaHydrator creates an initialized JavaHydrator instance.
func NewJavaHydrator() *JavaHydrator {
	return &JavaHydrator{}
}

// HydrateSymbol reconstitutes a single Java ASTSymbolNode into source text.
func (h *JavaHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// Java Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "ClassOrInterfaceDeclaration":
		var cls JavaClass
		if err := json.Unmarshal(node.ASTPayload, &cls); err != nil {
			return "", fmt.Errorf("unmarshaling JavaClass: %w", err)
		}
		return h.renderClass(&cls), nil

	case "MethodDeclaration":
		var m JavaMethod
		if err := json.Unmarshal(node.ASTPayload, &m); err != nil {
			return "", fmt.Errorf("unmarshaling JavaMethod: %w", err)
		}
		return h.renderMethod(&m, 0), nil

	case "FieldDeclaration":
		var f JavaField
		if err := json.Unmarshal(node.ASTPayload, &f); err != nil {
			return "", fmt.Errorf("unmarshaling JavaField: %w", err)
		}
		return h.renderField(&f, 0), nil

	case "RouteBinding":
		var r JavaRouteBinding
		if err := json.Unmarshal(node.ASTPayload, &r); err != nil {
			return "", fmt.Errorf("unmarshaling JavaRouteBinding: %w", err)
		}
		return fmt.Sprintf("// Route: %s %s (%s)\n// Attached handler: %s\n", r.Method, r.Path, r.Framework, r.HandlerName), nil

	default:
		return string(node.ASTPayload), nil
	}
}

func (h *JavaHydrator) renderClass(cls *JavaClass) string {
	var sb strings.Builder

	if cls.Doc != "" {
		sb.WriteString("/**\n")
		for _, line := range strings.Split(cls.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(" * %s\n", line))
		}
		sb.WriteString(" */\n")
	}

	for _, ann := range cls.Annotations {
		sb.WriteString(fmt.Sprintf("%s\n", ann))
	}

	vis := ""
	if cls.Visibility != "" && cls.Visibility != "package-private" {
		vis = cls.Visibility + " "
	}

	abstractPrefix := ""
	if cls.IsAbstract {
		abstractPrefix = "abstract "
	}
	finalPrefix := ""
	if cls.IsFinal {
		finalPrefix = "final "
	}
	staticPrefix := ""
	if cls.IsStatic {
		staticPrefix = "static "
	}

	kind := cls.Kind
	if kind == "" {
		kind = "class"
	}

	extendsStr := ""
	if cls.Extends != "" {
		extendsStr = fmt.Sprintf(" extends %s", cls.Extends)
	}

	implementsStr := ""
	if len(cls.Implements) > 0 {
		implementsStr = fmt.Sprintf(" implements %s", strings.Join(cls.Implements, ", "))
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%s%s %s%s%s {\n", vis, abstractPrefix, finalPrefix, staticPrefix, kind, cls.Name, extendsStr, implementsStr))

	// Render fields
	for _, f := range cls.Fields {
		sb.WriteString(h.renderField(&f, 1))
	}

	if len(cls.Fields) > 0 && len(cls.Methods) > 0 {
		sb.WriteString("\n")
	}

	// Render methods
	for _, m := range cls.Methods {
		sb.WriteString(h.renderMethod(&m, 1))
		sb.WriteString("\n")
	}

	sb.WriteString("}\n")
	return sb.String()
}

func (h *JavaHydrator) renderField(f *JavaField, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)
	var sb strings.Builder

	if f.Doc != "" {
		sb.WriteString(fmt.Sprintf("%s/** %s */\n", indent, f.Doc))
	}

	for _, ann := range f.Annotations {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, ann))
	}

	vis := ""
	if f.Visibility != "" && f.Visibility != "package-private" {
		vis = f.Visibility + " "
	}
	staticStr := ""
	if f.IsStatic {
		staticStr = "static "
	}
	finalStr := ""
	if f.IsFinal {
		finalStr = "final "
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%s%s %s;\n", indent, vis, staticStr, finalStr, f.Type, f.Name))
	return sb.String()
}

func (h *JavaHydrator) renderMethod(m *JavaMethod, indentLevel int) string {
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
		sb.WriteString(fmt.Sprintf("%s/**\n", indent))
		for _, line := range strings.Split(m.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s * %s\n", indent, line))
		}
		sb.WriteString(fmt.Sprintf("%s */\n", indent))
	}

	for _, ann := range m.Annotations {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, ann))
	}

	vis := ""
	if m.Visibility != "" && m.Visibility != "package-private" {
		vis = m.Visibility + " "
	}
	staticStr := ""
	if m.IsStatic {
		staticStr = "static "
	}
	abstractStr := ""
	if m.IsAbstract {
		abstractStr = "abstract "
	}
	finalStr := ""
	if m.IsFinal {
		finalStr = "final "
	}

	var paramStrs []string
	for _, p := range m.Params {
		annPrefix := ""
		if len(p.Annotations) > 0 {
			annPrefix = strings.Join(p.Annotations, " ") + " "
		}
		paramStrs = append(paramStrs, fmt.Sprintf("%s%s %s", annPrefix, p.Type, p.Name))
	}

	throwsStr := ""
	if len(m.Throws) > 0 {
		throwsStr = fmt.Sprintf(" throws %s", strings.Join(m.Throws, ", "))
	}

	retType := m.ReturnType
	if retType == "" {
		retType = "void"
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%s%s%s %s(%s)%s {\n", indent, vis, staticStr, abstractStr, finalStr, retType, m.Name, strings.Join(paramStrs, ", "), throwsStr))
	sb.WriteString(fmt.Sprintf("%s    // Reconstituted implementation\n", indent))
	sb.WriteString(fmt.Sprintf("%s}\n", indent))

	return sb.String()
}
