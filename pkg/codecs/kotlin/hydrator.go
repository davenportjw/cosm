package kotlin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// KotlinHydrator reconstitutes Kotlin AST symbol nodes back into clean, formatted Kotlin source code.
type KotlinHydrator struct{}

// NewKotlinHydrator creates an initialized KotlinHydrator instance.
func NewKotlinHydrator() *KotlinHydrator {
	return &KotlinHydrator{}
}

// HydrateSymbol reconstitutes a single Kotlin ASTSymbolNode into source text.
func (h *KotlinHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// Kotlin Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "ClassDecl", "DataClassDecl", "InterfaceDecl", "ObjectDecl", "EnumClassDecl", "SealedClassDecl":
		var cls KotlinClass
		if err := json.Unmarshal(node.ASTPayload, &cls); err != nil {
			return "", fmt.Errorf("unmarshaling KotlinClass: %w", err)
		}
		return h.renderClass(&cls), nil

	case "MethodDeclaration", "FunctionDecl", "ComposableFunction":
		var fn KotlinFunction
		if err := json.Unmarshal(node.ASTPayload, &fn); err != nil {
			return "", fmt.Errorf("unmarshaling KotlinFunction: %w", err)
		}
		return h.renderFunction(&fn, 0), nil

	case "RouteBinding":
		var r KotlinRouteBinding
		if err := json.Unmarshal(node.ASTPayload, &r); err != nil {
			return "", fmt.Errorf("unmarshaling KotlinRouteBinding: %w", err)
		}
		return fmt.Sprintf("// Route Binding: %s %s (%s) -> %s\n", r.Method, r.Path, r.Framework, r.HandlerName), nil

	case "ApiClientCall":
		var api KotlinAPICall
		if err := json.Unmarshal(node.ASTPayload, &api); err != nil {
			return "", fmt.Errorf("unmarshaling KotlinAPICall: %w", err)
		}
		return fmt.Sprintf("// API Client Request: %s %s via %s\n", api.Method, api.URL, api.Caller), nil

	default:
		return string(node.ASTPayload), nil
	}
}

// HydrateFile reconstitutes a complete Kotlin source file from a KotlinFileResult.
func (h *KotlinHydrator) HydrateFile(res *KotlinFileResult) (string, error) {
	if res == nil {
		return "", fmt.Errorf("KotlinFileResult is nil")
	}

	var sb strings.Builder

	// Package
	if res.PackageName != "" {
		sb.WriteString(fmt.Sprintf("package %s\n\n", res.PackageName))
	}

	// Imports
	if len(res.Imports) > 0 {
		for _, imp := range res.Imports {
			sb.WriteString(fmt.Sprintf("import %s\n", imp))
		}
		sb.WriteString("\n")
	}

	// Classes
	for i, cls := range res.Classes {
		sb.WriteString(h.renderClass(&cls))
		if i < len(res.Classes)-1 || len(res.TopLevelFunctions) > 0 {
			sb.WriteString("\n")
		}
	}

	// Top-level functions
	for i, fn := range res.TopLevelFunctions {
		sb.WriteString(h.renderFunction(&fn, 0))
		if i < len(res.TopLevelFunctions)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

func (h *KotlinHydrator) renderClass(cls *KotlinClass) string {
	if cls.BodySource != "" {
		return strings.TrimSpace(cls.BodySource) + "\n"
	}

	var sb strings.Builder

	// Docstrings
	if cls.Doc != "" {
		sb.WriteString("/**\n")
		for _, line := range strings.Split(cls.Doc, "\n") {
			sb.WriteString(fmt.Sprintf(" * %s\n", line))
		}
		sb.WriteString(" */\n")
	}

	// Annotations
	for _, ann := range cls.Annotations {
		sb.WriteString(fmt.Sprintf("%s\n", ann))
	}

	vis := ""
	if cls.Visibility != "" && cls.Visibility != "public" {
		vis = cls.Visibility + " "
	}

	mod := ""
	if cls.IsSealed && !strings.Contains(cls.Kind, "sealed") {
		mod = "sealed "
	} else if cls.IsAbstract && !strings.Contains(cls.Kind, "abstract") && cls.Kind != "interface" {
		mod = "abstract "
	} else if cls.IsOpen && !strings.Contains(cls.Kind, "open") {
		mod = "open "
	}

	kind := cls.Kind
	if kind == "" {
		kind = "class"
	}

	primaryParams := ""
	if len(cls.PrimaryConstructorParams) > 0 {
		primaryParams = fmt.Sprintf("(%s)", formatKotlinParams(cls.PrimaryConstructorParams))
	}

	superTypes := ""
	if len(cls.SuperTypes) > 0 {
		superTypes = fmt.Sprintf(" : %s", strings.Join(cls.SuperTypes, ", "))
	}

	hasBody := len(cls.Properties) > 0 || len(cls.Functions) > 0

	if !hasBody {
		sb.WriteString(fmt.Sprintf("%s%s%s %s%s%s\n", vis, mod, kind, cls.Name, primaryParams, superTypes))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("%s%s%s %s%s%s {\n", vis, mod, kind, cls.Name, primaryParams, superTypes))

	// Properties
	for _, p := range cls.Properties {
		sb.WriteString(h.renderProperty(&p, 1))
	}

	if len(cls.Properties) > 0 && len(cls.Functions) > 0 {
		sb.WriteString("\n")
	}

	// Functions
	for _, fn := range cls.Functions {
		sb.WriteString(h.renderFunction(&fn, 1))
		sb.WriteString("\n")
	}

	sb.WriteString("}\n")
	return sb.String()
}

func (h *KotlinHydrator) renderProperty(p *KotlinProperty, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)
	var sb strings.Builder

	if p.Doc != "" {
		sb.WriteString(fmt.Sprintf("%s/** %s */\n", indent, p.Doc))
	}

	for _, ann := range p.Annotations {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, ann))
	}

	vis := ""
	if p.Visibility != "" && p.Visibility != "public" {
		vis = p.Visibility + " "
	}
	overrideStr := ""
	if p.IsOverride {
		overrideStr = "override "
	}
	lateinitStr := ""
	if p.IsLateinit {
		lateinitStr = "lateinit "
	}
	valVar := "val"
	if p.IsVar {
		valVar = "var"
	}

	typeStr := ""
	if p.Type != "" {
		typeStr = fmt.Sprintf(": %s", p.Type)
	}

	defVal := ""
	if p.DefaultValue != "" {
		defVal = fmt.Sprintf(" = %s", p.DefaultValue)
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%s%s %s%s%s\n", indent, vis, overrideStr, lateinitStr, valVar, p.Name, typeStr, defVal))
	return sb.String()
}

func (h *KotlinHydrator) renderFunction(fn *KotlinFunction, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)

	if fn.BodySource != "" {
		trimmed := strings.TrimSpace(fn.BodySource)
		var sb strings.Builder
		for _, line := range strings.Split(trimmed, "\n") {
			if strings.TrimSpace(line) != "" {
				sb.WriteString(fmt.Sprintf("%s%s\n", indent, line))
			}
		}
		return sb.String()
	}

	var sb strings.Builder

	if fn.Doc != "" {
		sb.WriteString(fmt.Sprintf("%s/**\n", indent))
		for _, line := range strings.Split(fn.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s * %s\n", indent, line))
		}
		sb.WriteString(fmt.Sprintf("%s */\n", indent))
	}

	for _, ann := range fn.Annotations {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, ann))
	}

	if fn.IsComposable && !containsAnnotation(fn.Annotations, "@Composable") {
		sb.WriteString(fmt.Sprintf("%s@Composable\n", indent))
	}

	vis := ""
	if fn.Visibility != "" && fn.Visibility != "public" {
		vis = fn.Visibility + " "
	}
	overrideStr := ""
	if fn.IsOverride {
		overrideStr = "override "
	}
	suspendStr := ""
	if fn.IsSuspend {
		suspendStr = "suspend "
	}
	inlineStr := ""
	if fn.IsInline {
		inlineStr = "inline "
	}
	openStr := ""
	if fn.IsOpen {
		openStr = "open "
	}

	retStr := ""
	if fn.ReturnType != "" && fn.ReturnType != "Unit" {
		retStr = fmt.Sprintf(": %s", fn.ReturnType)
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%s%s%sfun %s(%s)%s {\n", indent, vis, overrideStr, suspendStr, inlineStr, openStr, fn.Name, formatKotlinParams(fn.Params), retStr))
	sb.WriteString(fmt.Sprintf("%s    // Reconstituted implementation\n", indent))
	sb.WriteString(fmt.Sprintf("%s}\n", indent))

	return sb.String()
}

func containsAnnotation(annotations []string, target string) bool {
	for _, a := range annotations {
		if strings.Contains(a, target) {
			return true
		}
	}
	return false
}
