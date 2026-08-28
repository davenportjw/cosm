package rust

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// RustHydrator reconstitutes Rust AST symbol nodes back into clean, formatted Rust source code.
type RustHydrator struct{}

// NewRustHydrator creates a new RustHydrator instance.
func NewRustHydrator() *RustHydrator {
	return &RustHydrator{}
}

// HydrateSymbol reconstitutes a single Rust ASTSymbolNode into source code.
func (h *RustHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// Rust Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "StructDef":
		var s RustStruct
		if err := json.Unmarshal(node.ASTPayload, &s); err != nil {
			return "", fmt.Errorf("unmarshaling RustStruct: %w", err)
		}
		return h.renderStruct(&s), nil

	case "EnumDef":
		var e RustEnum
		if err := json.Unmarshal(node.ASTPayload, &e); err != nil {
			return "", fmt.Errorf("unmarshaling RustEnum: %w", err)
		}
		return h.renderEnum(&e), nil

	case "TraitDef":
		var t RustTrait
		if err := json.Unmarshal(node.ASTPayload, &t); err != nil {
			return "", fmt.Errorf("unmarshaling RustTrait: %w", err)
		}
		return h.renderTrait(&t), nil

	case "ImplBlock":
		var imp RustImpl
		if err := json.Unmarshal(node.ASTPayload, &imp); err != nil {
			return "", fmt.Errorf("unmarshaling RustImpl: %w", err)
		}
		return h.renderImpl(&imp), nil

	case "FnDef":
		var fn RustFunction
		if err := json.Unmarshal(node.ASTPayload, &fn); err != nil {
			return "", fmt.Errorf("unmarshaling RustFunction: %w", err)
		}
		return h.renderFunction(&fn), nil

	case "MacroDef":
		var m RustMacro
		if err := json.Unmarshal(node.ASTPayload, &m); err != nil {
			return "", fmt.Errorf("unmarshaling RustMacro: %w", err)
		}
		return h.renderMacro(&m), nil

	case "RouteBinding":
		var r RustRouteBinding
		if err := json.Unmarshal(node.ASTPayload, &r); err != nil {
			return "", fmt.Errorf("unmarshaling RustRouteBinding: %w", err)
		}
		return fmt.Sprintf("// Route: %s %s (%s)\n// Handler: %s\n", r.Method, r.Path, r.Framework, r.HandlerName), nil

	default:
		return string(node.ASTPayload), nil
	}
}

func (h *RustHydrator) renderStruct(s *RustStruct) string {
	var sb strings.Builder

	if s.Doc != "" {
		for _, line := range strings.Split(s.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", line))
		}
	}

	if len(s.Derives) > 0 {
		sb.WriteString(fmt.Sprintf("#[derive(%s)]\n", strings.Join(s.Derives, ", ")))
	}

	for _, attr := range s.Attributes {
		if !strings.HasPrefix(attr, "#[derive") {
			sb.WriteString(fmt.Sprintf("%s\n", attr))
		}
	}

	vis := ""
	if s.Visibility != "" && s.Visibility != "private" {
		vis = s.Visibility + " "
	}

	generics := ""
	if s.Generics != "" {
		generics = fmt.Sprintf("<%s>", s.Generics)
	}

	if s.IsTuple {
		var fieldTypes []string
		for _, f := range s.Fields {
			fVis := ""
			if f.Visibility != "" && f.Visibility != "private" {
				fVis = f.Visibility + " "
			}
			fieldTypes = append(fieldTypes, fmt.Sprintf("%s%s", fVis, f.Type))
		}
		sb.WriteString(fmt.Sprintf("%sstruct %s%s(%s);\n", vis, s.Name, generics, strings.Join(fieldTypes, ", ")))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("%sstruct %s%s {\n", vis, s.Name, generics))
	for _, f := range s.Fields {
		fVis := ""
		if f.Visibility != "" && f.Visibility != "private" {
			fVis = f.Visibility + " "
		}
		sb.WriteString(fmt.Sprintf("    %s%s: %s,\n", fVis, f.Name, f.Type))
	}
	sb.WriteString("}\n")

	return sb.String()
}

func (h *RustHydrator) renderEnum(e *RustEnum) string {
	var sb strings.Builder

	if e.Doc != "" {
		for _, line := range strings.Split(e.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", line))
		}
	}

	if len(e.Derives) > 0 {
		sb.WriteString(fmt.Sprintf("#[derive(%s)]\n", strings.Join(e.Derives, ", ")))
	}

	for _, attr := range e.Attributes {
		if !strings.HasPrefix(attr, "#[derive") {
			sb.WriteString(fmt.Sprintf("%s\n", attr))
		}
	}

	vis := ""
	if e.Visibility != "" && e.Visibility != "private" {
		vis = e.Visibility + " "
	}

	generics := ""
	if e.Generics != "" {
		generics = fmt.Sprintf("<%s>", e.Generics)
	}

	sb.WriteString(fmt.Sprintf("%senum %s%s {\n", vis, e.Name, generics))
	for _, v := range e.Variants {
		if v.Doc != "" {
			sb.WriteString(fmt.Sprintf("    /// %s\n", v.Doc))
		}
		if v.Discriminant != "" {
			sb.WriteString(fmt.Sprintf("    %s = %s,\n", v.Name, v.Discriminant))
		} else {
			sb.WriteString(fmt.Sprintf("    %s,\n", v.Name))
		}
	}
	sb.WriteString("}\n")

	return sb.String()
}

func (h *RustHydrator) renderTrait(t *RustTrait) string {
	var sb strings.Builder

	if t.Doc != "" {
		for _, line := range strings.Split(t.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", line))
		}
	}

	for _, attr := range t.Attributes {
		sb.WriteString(fmt.Sprintf("%s\n", attr))
	}

	vis := ""
	if t.Visibility != "" && t.Visibility != "private" {
		vis = t.Visibility + " "
	}

	generics := ""
	if t.Generics != "" {
		generics = fmt.Sprintf("<%s>", t.Generics)
	}

	sb.WriteString(fmt.Sprintf("%strait %s%s {\n", vis, t.Name, generics))
	for _, m := range t.Methods {
		if m.Doc != "" {
			sb.WriteString(fmt.Sprintf("    /// %s\n", m.Doc))
		}
		asyncPrefix := ""
		if m.IsAsync {
			asyncPrefix = "async "
		}
		var paramStrs []string
		for _, p := range m.Params {
			if p.Type == "self" {
				paramStrs = append(paramStrs, p.Name)
			} else {
				paramStrs = append(paramStrs, fmt.Sprintf("%s: %s", p.Name, p.Type))
			}
		}
		retStr := ""
		if m.ReturnType != "" {
			retStr = fmt.Sprintf(" -> %s", m.ReturnType)
		}

		if m.DefaultBody != "" {
			sb.WriteString(fmt.Sprintf("    %sfn %s(%s)%s {\n%s\n    }\n", asyncPrefix, m.Name, strings.Join(paramStrs, ", "), retStr, m.DefaultBody))
		} else {
			sb.WriteString(fmt.Sprintf("    %sfn %s(%s)%s;\n", asyncPrefix, m.Name, strings.Join(paramStrs, ", "), retStr))
		}
	}
	sb.WriteString("}\n")

	return sb.String()
}

func (h *RustHydrator) renderImpl(imp *RustImpl) string {
	var sb strings.Builder

	generics := ""
	if imp.Generics != "" {
		generics = fmt.Sprintf("<%s>", imp.Generics)
	}

	if imp.TraitName != "" {
		sb.WriteString(fmt.Sprintf("impl%s %s for %s {\n", generics, imp.TraitName, imp.TargetType))
	} else {
		sb.WriteString(fmt.Sprintf("impl%s %s {\n", generics, imp.TargetType))
	}

	for _, m := range imp.Methods {
		methodCode := h.renderFunction(&m)
		// Indent method lines
		for _, line := range strings.Split(methodCode, "\n") {
			if strings.TrimSpace(line) != "" {
				sb.WriteString(fmt.Sprintf("    %s\n", line))
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString("}\n")

	return sb.String()
}

func (h *RustHydrator) renderFunction(fn *RustFunction) string {
	if fn.BodySource != "" {
		trimmed := strings.TrimSpace(fn.BodySource)
		if strings.Contains(trimmed, "fn ") {
			return trimmed + "\n"
		}
	}

	var sb strings.Builder

	if fn.Doc != "" {
		for _, line := range strings.Split(fn.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", line))
		}
	}

	for _, attr := range fn.Attributes {
		sb.WriteString(fmt.Sprintf("%s\n", attr))
	}

	vis := ""
	if fn.Visibility != "" && fn.Visibility != "private" {
		vis = fn.Visibility + " "
	}

	asyncPrefix := ""
	if fn.IsAsync {
		asyncPrefix = "async "
	}
	constPrefix := ""
	if fn.IsConst {
		constPrefix = "const "
	}
	unsafePrefix := ""
	if fn.IsUnsafe {
		unsafePrefix = "unsafe "
	}

	generics := ""
	if fn.Generics != "" {
		generics = fmt.Sprintf("<%s>", fn.Generics)
	}

	var paramStrs []string
	for _, p := range fn.Params {
		if p.Type == "self" {
			paramStrs = append(paramStrs, p.Name)
		} else {
			paramStrs = append(paramStrs, fmt.Sprintf("%s: %s", p.Name, p.Type))
		}
	}

	retStr := ""
	if fn.ReturnType != "" {
		retStr = fmt.Sprintf(" -> %s", fn.ReturnType)
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%sfn %s%s(%s)%s {\n", vis, asyncPrefix, constPrefix, unsafePrefix, fn.Name, generics, strings.Join(paramStrs, ", "), retStr))
	if fn.BodySource != "" && !strings.Contains(fn.BodySource, "fn ") {
		sb.WriteString(fmt.Sprintf("    %s\n", strings.TrimSpace(fn.BodySource)))
	} else {
		sb.WriteString("    // Reconstituted implementation\n")
	}
	sb.WriteString("}\n")

	return sb.String()
}

func (h *RustHydrator) renderMacro(m *RustMacro) string {
	if m.Body != "" {
		return m.Body + "\n"
	}
	var sb strings.Builder
	if m.Doc != "" {
		for _, line := range strings.Split(m.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", line))
		}
	}
	sb.WriteString(fmt.Sprintf("macro_rules! %s {\n    () => {};\n}\n", m.Name))
	return sb.String()
}
