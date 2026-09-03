package csharp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// CSharpHydrator reconstitutes C# AST symbol nodes back into clean, formatted C# source code.
type CSharpHydrator struct{}

// NewCSharpHydrator creates an initialized CSharpHydrator instance.
func NewCSharpHydrator() *CSharpHydrator {
	return &CSharpHydrator{}
}

// HydrateSymbol reconstitutes a single C# ASTSymbolNode into source text.
func (h *CSharpHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// C# Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "ClassDeclaration", "StructDeclaration", "InterfaceDeclaration", "RecordDeclaration", "EnumDeclaration":
		var t CSharpType
		if err := json.Unmarshal(node.ASTPayload, &t); err != nil {
			return "", fmt.Errorf("unmarshaling CSharpType: %w", err)
		}
		return h.renderType(&t), nil

	case "MethodDeclaration", "ConstructorDeclaration":
		var m CSharpMethod
		if err := json.Unmarshal(node.ASTPayload, &m); err != nil {
			return "", fmt.Errorf("unmarshaling CSharpMethod: %w", err)
		}
		return h.renderMethod(&m, 0), nil

	case "PropertyDeclaration":
		var p CSharpProperty
		if err := json.Unmarshal(node.ASTPayload, &p); err != nil {
			return "", fmt.Errorf("unmarshaling CSharpProperty: %w", err)
		}
		return h.renderProperty(&p, 0), nil

	case "RouteBinding":
		var r CSharpRouteBinding
		if err := json.Unmarshal(node.ASTPayload, &r); err != nil {
			return "", fmt.Errorf("unmarshaling CSharpRouteBinding: %w", err)
		}
		if r.IsMinimalAPI {
			return fmt.Sprintf("// Minimal API Route: %s %s\n// Handler: %s\n", r.Method, r.Path, r.HandlerName), nil
		}
		return fmt.Sprintf("// Controller Route: %s %s (%s)\n// Attached action: %s\n", r.Method, r.Path, r.Framework, r.HandlerName), nil

	default:
		return string(node.ASTPayload), nil
	}
}

func (h *CSharpHydrator) renderType(t *CSharpType) string {
	var sb strings.Builder

	if t.Doc != "" {
		sb.WriteString("/// <summary>\n")
		for _, line := range strings.Split(t.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", line))
		}
		sb.WriteString("/// </summary>\n")
	}

	for _, attr := range t.Attributes {
		sb.WriteString(fmt.Sprintf("%s\n", attr))
	}

	vis := ""
	if t.Visibility != "" {
		vis = t.Visibility + " "
	}

	abstractPrefix := ""
	if t.IsAbstract {
		abstractPrefix = "abstract "
	}
	sealedPrefix := ""
	if t.IsSealed {
		sealedPrefix = "sealed "
	}
	staticPrefix := ""
	if t.IsStatic {
		staticPrefix = "static "
	}
	partialPrefix := ""
	if t.IsPartial {
		partialPrefix = "partial "
	}

	kind := t.Kind
	if kind == "" {
		kind = "class"
	}

	genStr := ""
	if len(t.GenericParams) > 0 {
		genStr = fmt.Sprintf("<%s>", strings.Join(t.GenericParams, ", "))
	}

	var inheritance []string
	if t.BaseType != "" {
		inheritance = append(inheritance, t.BaseType)
	}
	inheritance = append(inheritance, t.Interfaces...)

	inheritanceStr := ""
	if len(inheritance) > 0 {
		inheritanceStr = fmt.Sprintf(" : %s", strings.Join(inheritance, ", "))
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%s%s%s %s%s%s\n{\n",
		vis, abstractPrefix, sealedPrefix, staticPrefix, partialPrefix, kind, t.Name, genStr, inheritanceStr))

	// Render Enum Members
	if kind == "enum" {
		for i, em := range t.EnumMembers {
			if em.Doc != "" {
				sb.WriteString(fmt.Sprintf("    /// <summary>\n    /// %s\n    /// </summary>\n", em.Doc))
			}
			for _, attr := range em.Attributes {
				sb.WriteString(fmt.Sprintf("    %s\n", attr))
			}
			valStr := ""
			if em.Value != "" {
				valStr = fmt.Sprintf(" = %s", em.Value)
			}
			comma := ","
			if i == len(t.EnumMembers)-1 {
				comma = ""
			}
			sb.WriteString(fmt.Sprintf("    %s%s%s\n", em.Name, valStr, comma))
		}
		sb.WriteString("}\n")
		return sb.String()
	}

	// Render fields
	for _, f := range t.Fields {
		sb.WriteString(h.renderField(&f, 1))
	}

	if len(t.Fields) > 0 && (len(t.Properties) > 0 || len(t.Methods) > 0) {
		sb.WriteString("\n")
	}

	// Render properties
	for _, p := range t.Properties {
		sb.WriteString(h.renderProperty(&p, 1))
	}

	if len(t.Properties) > 0 && len(t.Methods) > 0 {
		sb.WriteString("\n")
	}

	// Render methods
	for _, m := range t.Methods {
		sb.WriteString(h.renderMethod(&m, 1))
		sb.WriteString("\n")
	}

	sb.WriteString("}\n")
	return sb.String()
}

func (h *CSharpHydrator) renderField(f *CSharpField, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)
	var sb strings.Builder

	if f.Doc != "" {
		sb.WriteString(fmt.Sprintf("%s/// <summary>\n%s/// %s\n%s/// </summary>\n", indent, indent, f.Doc, indent))
	}

	for _, attr := range f.Attributes {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, attr))
	}

	vis := ""
	if f.Visibility != "" {
		vis = f.Visibility + " "
	}
	staticStr := ""
	if f.IsStatic {
		staticStr = "static "
	}
	readonlyStr := ""
	if f.IsReadonly {
		readonlyStr = "readonly "
	}
	constStr := ""
	if f.IsConst {
		constStr = "const "
	}

	initStr := ""
	if f.InitialValue != "" {
		initStr = fmt.Sprintf(" = %s", f.InitialValue)
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%s%s%s %s%s;\n", indent, vis, staticStr, readonlyStr, constStr, f.Type, f.Name, initStr))
	return sb.String()
}

func (h *CSharpHydrator) renderProperty(p *CSharpProperty, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)
	var sb strings.Builder

	if p.Doc != "" {
		sb.WriteString(fmt.Sprintf("%s/// <summary>\n%s/// %s\n%s/// </summary>\n", indent, indent, p.Doc, indent))
	}

	for _, attr := range p.Attributes {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, attr))
	}

	vis := ""
	if p.Visibility != "" {
		vis = p.Visibility + " "
	}
	staticStr := ""
	if p.IsStatic {
		staticStr = "static "
	}
	virtualStr := ""
	if p.IsVirtual {
		virtualStr = "virtual "
	}
	overrideStr := ""
	if p.IsOverride {
		overrideStr = "override "
	}
	abstractStr := ""
	if p.IsAbstract {
		abstractStr = "abstract "
	}

	var accessors []string
	if p.HasGetter {
		getVis := ""
		if p.GetterVisibility != "" {
			getVis = p.GetterVisibility + " "
		}
		accessors = append(accessors, fmt.Sprintf("%sget;", getVis))
	}
	if p.HasSetter {
		setVis := ""
		if p.SetterVisibility != "" {
			setVis = p.SetterVisibility + " "
		}
		accessors = append(accessors, fmt.Sprintf("%sset;", setVis))
	} else if p.HasInit {
		accessors = append(accessors, "init;")
	}

	accessorBlock := "{ get; set; }"
	if len(accessors) > 0 {
		accessorBlock = fmt.Sprintf("{ %s }", strings.Join(accessors, " "))
	}

	initStr := ""
	if p.InitialValue != "" {
		initStr = fmt.Sprintf(" = %s;", p.InitialValue)
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%s%s%s%s %s %s%s\n", indent, vis, staticStr, virtualStr, overrideStr, abstractStr, p.Type, p.Name, accessorBlock, initStr))
	return sb.String()
}

func (h *CSharpHydrator) renderMethod(m *CSharpMethod, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)

	if m.BodySource != "" && !m.IsConstructor {
		trimmed := strings.TrimSpace(m.BodySource)
		var sb strings.Builder
		for _, attr := range m.Attributes {
			if !strings.Contains(trimmed, attr) {
				sb.WriteString(fmt.Sprintf("%s%s\n", indent, attr))
			}
		}
		for _, line := range strings.Split(trimmed, "\n") {
			if strings.TrimSpace(line) != "" {
				sb.WriteString(fmt.Sprintf("%s%s\n", indent, line))
			}
		}
		return sb.String()
	}

	var sb strings.Builder

	if m.Doc != "" {
		sb.WriteString(fmt.Sprintf("%s/// <summary>\n", indent))
		for _, line := range strings.Split(m.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s/// %s\n", indent, line))
		}
		sb.WriteString(fmt.Sprintf("%s/// </summary>\n", indent))
	}

	for _, attr := range m.Attributes {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, attr))
	}

	vis := ""
	if m.Visibility != "" {
		vis = m.Visibility + " "
	}
	staticStr := ""
	if m.IsStatic {
		staticStr = "static "
	}
	asyncStr := ""
	if m.IsAsync {
		asyncStr = "async "
	}
	virtualStr := ""
	if m.IsVirtual {
		virtualStr = "virtual "
	}
	overrideStr := ""
	if m.IsOverride {
		overrideStr = "override "
	}
	abstractStr := ""
	if m.IsAbstract {
		abstractStr = "abstract "
	}

	genStr := ""
	if len(m.GenericParams) > 0 {
		genStr = fmt.Sprintf("<%s>", strings.Join(m.GenericParams, ", "))
	}

	var paramStrs []string
	for _, p := range m.Params {
		annPrefix := ""
		if len(p.Attributes) > 0 {
			annPrefix = strings.Join(p.Attributes, " ") + " "
		}
		modifier := ""
		if p.IsRef {
			modifier = "ref "
		} else if p.IsOut {
			modifier = "out "
		} else if p.IsIn {
			modifier = "in "
		} else if p.IsParams {
			modifier = "params "
		}
		defStr := ""
		if p.DefaultValue != "" {
			defStr = fmt.Sprintf(" = %s", p.DefaultValue)
		}
		paramStrs = append(paramStrs, fmt.Sprintf("%s%s%s %s%s", annPrefix, modifier, p.Type, p.Name, defStr))
	}

	if m.IsConstructor {
		sb.WriteString(fmt.Sprintf("%s%s%s(%s)\n%s{\n%s    // Constructor initialization\n%s}\n",
			indent, vis, m.Name, strings.Join(paramStrs, ", "), indent, indent, indent))
		return sb.String()
	}

	retType := m.ReturnType
	if retType == "" {
		retType = "void"
	}

	if m.IsAbstract {
		sb.WriteString(fmt.Sprintf("%s%s%s%s%s%s%s%s %s%s(%s);\n",
			indent, vis, staticStr, asyncStr, virtualStr, overrideStr, abstractStr, retType, m.Name, genStr, strings.Join(paramStrs, ", ")))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%s%s%s%s%s %s%s(%s)\n%s{\n",
		indent, vis, staticStr, asyncStr, virtualStr, overrideStr, abstractStr, retType, m.Name, genStr, strings.Join(paramStrs, ", "), indent))
	sb.WriteString(fmt.Sprintf("%s    // Reconstituted implementation\n", indent))
	sb.WriteString(fmt.Sprintf("%s}\n", indent))

	return sb.String()
}
