package zig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// ZigHydrator reconstitutes Zig AST symbol nodes back into clean, idiomatic Zig source code.
type ZigHydrator struct{}

// NewZigHydrator creates a new ZigHydrator instance.
func NewZigHydrator() *ZigHydrator {
	return &ZigHydrator{}
}

// HydrateSymbol reconstitutes a single ASTSymbolNode into clean Zig source.
func (h *ZigHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// Zig Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "StructDef":
		var st ZigStruct
		if err := json.Unmarshal(node.ASTPayload, &st); err != nil {
			return "", fmt.Errorf("unmarshaling ZigStruct: %w", err)
		}
		return h.renderStruct(&st), nil

	case "EnumDef":
		var en ZigEnum
		if err := json.Unmarshal(node.ASTPayload, &en); err != nil {
			return "", fmt.Errorf("unmarshaling ZigEnum: %w", err)
		}
		return h.renderEnum(&en), nil

	case "UnionDef":
		var un ZigUnion
		if err := json.Unmarshal(node.ASTPayload, &un); err != nil {
			return "", fmt.Errorf("unmarshaling ZigUnion: %w", err)
		}
		return h.renderUnion(&un), nil

	case "FnDef", "FunctionDecl":
		var fn ZigFunction
		if err := json.Unmarshal(node.ASTPayload, &fn); err != nil {
			return "", fmt.Errorf("unmarshaling ZigFunction: %w", err)
		}
		return h.renderFunction(&fn), nil

	case "VarDef":
		var v ZigVar
		if err := json.Unmarshal(node.ASTPayload, &v); err != nil {
			return "", fmt.Errorf("unmarshaling ZigVar: %w", err)
		}
		return h.renderVar(&v), nil

	case "ErrorSetDef":
		var errSet ZigErrorSet
		if err := json.Unmarshal(node.ASTPayload, &errSet); err != nil {
			return "", fmt.Errorf("unmarshaling ZigErrorSet: %w", err)
		}
		return h.renderErrorSet(&errSet), nil

	case "TestDef":
		var tst ZigTest
		if err := json.Unmarshal(node.ASTPayload, &tst); err != nil {
			return "", fmt.Errorf("unmarshaling ZigTest: %w", err)
		}
		return h.renderTest(&tst), nil

	default:
		return string(node.ASTPayload), nil
	}
}

// HydrateFile reconstitutes a complete .zig source file from a parsed ZigFileResult.
func (h *ZigHydrator) HydrateFile(res *ZigFileResult) string {
	var sb strings.Builder

	// 1. Module-level doc comments `//!`
	if res.Docstring != "" {
		for _, line := range strings.Split(res.Docstring, "\n") {
			sb.WriteString(fmt.Sprintf("//! %s\n", line))
		}
		sb.WriteString("\n")
	}

	// 2. Imports
	if len(res.Imports) > 0 {
		for _, imp := range res.Imports {
			sb.WriteString(h.renderImport(&imp) + "\n")
		}
		sb.WriteString("\n")
	}

	// 3. Error Sets
	for _, errSet := range res.ErrorSets {
		sb.WriteString(h.renderErrorSet(&errSet) + "\n\n")
	}

	// 4. Structs
	for _, st := range res.Structs {
		sb.WriteString(h.renderStruct(&st) + "\n\n")
	}

	// 5. Enums
	for _, en := range res.Enums {
		sb.WriteString(h.renderEnum(&en) + "\n\n")
	}

	// 6. Unions
	for _, un := range res.Unions {
		sb.WriteString(h.renderUnion(&un) + "\n\n")
	}

	// 7. Top-level Variables / Constants
	for _, v := range res.Variables {
		sb.WriteString(h.renderVar(&v) + "\n")
	}
	if len(res.Variables) > 0 {
		sb.WriteString("\n")
	}

	// 8. Functions
	for _, fn := range res.Functions {
		sb.WriteString(h.renderFunction(&fn) + "\n\n")
	}

	// 9. Tests
	for _, tst := range res.Tests {
		sb.WriteString(h.renderTest(&tst) + "\n\n")
	}

	return strings.TrimSpace(sb.String()) + "\n"
}

// ----------------------------------------------------------------------------
// Render Helpers
// ----------------------------------------------------------------------------

func (h *ZigHydrator) renderImport(imp *ZigImport) string {
	prefix := ""
	if imp.Visibility == "public" {
		prefix = "pub "
	}
	if imp.IsCImport {
		if imp.CSource != "" {
			return fmt.Sprintf("%sconst %s = @cImport(%s);", prefix, imp.Alias, imp.CSource)
		}
		return fmt.Sprintf("%sconst %s = @cImport({});", prefix, imp.Alias)
	}
	return fmt.Sprintf("%sconst %s = @import(%q);", prefix, imp.Alias, imp.Path)
}

func (h *ZigHydrator) renderErrorSet(errSet *ZigErrorSet) string {
	var sb strings.Builder
	if errSet.Doc != "" {
		for _, doc := range strings.Split(errSet.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", doc))
		}
	}
	prefix := ""
	if errSet.Visibility == "public" {
		prefix = "pub "
	}
	sb.WriteString(fmt.Sprintf("%sconst %s = error{\n", prefix, errSet.Name))
	for _, e := range errSet.Errors {
		sb.WriteString(fmt.Sprintf("    %s,\n", e))
	}
	sb.WriteString("};")
	return sb.String()
}

func (h *ZigHydrator) renderStruct(st *ZigStruct) string {
	var sb strings.Builder
	if st.Doc != "" {
		for _, doc := range strings.Split(st.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", doc))
		}
	}
	prefix := ""
	if st.Visibility == "public" {
		prefix = "pub "
	}
	kind := st.Kind
	if kind == "" {
		kind = "struct"
	}
	sb.WriteString(fmt.Sprintf("%sconst %s = %s {\n", prefix, st.Name, kind))

	// Fields
	for _, f := range st.Fields {
		if f.Doc != "" {
			for _, doc := range strings.Split(f.Doc, "\n") {
				sb.WriteString(fmt.Sprintf("    /// %s\n", doc))
			}
		}
		defStr := ""
		if f.DefaultValue != "" {
			defStr = fmt.Sprintf(" = %s", f.DefaultValue)
		}
		sb.WriteString(fmt.Sprintf("    %s: %s%s,\n", f.Name, f.Type, defStr))
	}

	if len(st.Fields) > 0 && len(st.Methods) > 0 {
		sb.WriteString("\n")
	}

	// Methods
	for i, m := range st.Methods {
		mStr := h.renderFunction(&m)
		for _, line := range strings.Split(mStr, "\n") {
			sb.WriteString("    " + line + "\n")
		}
		if i < len(st.Methods)-1 {
			sb.WriteString("\n")
		}
	}

	sb.WriteString("};")
	return sb.String()
}

func (h *ZigHydrator) renderEnum(en *ZigEnum) string {
	var sb strings.Builder
	if en.Doc != "" {
		for _, doc := range strings.Split(en.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", doc))
		}
	}
	prefix := ""
	if en.Visibility == "public" {
		prefix = "pub "
	}
	tagStr := ""
	if en.TagType != "" {
		tagStr = fmt.Sprintf("(%s)", en.TagType)
	}
	sb.WriteString(fmt.Sprintf("%sconst %s = enum%s {\n", prefix, en.Name, tagStr))

	for _, f := range en.Fields {
		if f.Doc != "" {
			for _, doc := range strings.Split(f.Doc, "\n") {
				sb.WriteString(fmt.Sprintf("    /// %s\n", doc))
			}
		}
		valStr := ""
		if f.TagValue != "" {
			valStr = fmt.Sprintf(" = %s", f.TagValue)
		}
		sb.WriteString(fmt.Sprintf("    %s%s,\n", f.Name, valStr))
	}

	if len(en.Fields) > 0 && len(en.Methods) > 0 {
		sb.WriteString("\n")
	}

	for i, m := range en.Methods {
		mStr := h.renderFunction(&m)
		for _, line := range strings.Split(mStr, "\n") {
			sb.WriteString("    " + line + "\n")
		}
		if i < len(en.Methods)-1 {
			sb.WriteString("\n")
		}
	}

	sb.WriteString("};")
	return sb.String()
}

func (h *ZigHydrator) renderUnion(un *ZigUnion) string {
	var sb strings.Builder
	if un.Doc != "" {
		for _, doc := range strings.Split(un.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", doc))
		}
	}
	prefix := ""
	if un.Visibility == "public" {
		prefix = "pub "
	}
	tagStr := ""
	if un.TagType != "" {
		tagStr = fmt.Sprintf("(%s)", un.TagType)
	}
	sb.WriteString(fmt.Sprintf("%sconst %s = union%s {\n", prefix, un.Name, tagStr))

	for _, f := range un.Fields {
		if f.Doc != "" {
			for _, doc := range strings.Split(f.Doc, "\n") {
				sb.WriteString(fmt.Sprintf("    /// %s\n", doc))
			}
		}
		sb.WriteString(fmt.Sprintf("    %s: %s,\n", f.Name, f.Type))
	}

	if len(un.Fields) > 0 && len(un.Methods) > 0 {
		sb.WriteString("\n")
	}

	for i, m := range un.Methods {
		mStr := h.renderFunction(&m)
		for _, line := range strings.Split(mStr, "\n") {
			sb.WriteString("    " + line + "\n")
		}
		if i < len(un.Methods)-1 {
			sb.WriteString("\n")
		}
	}

	sb.WriteString("};")
	return sb.String()
}

func (h *ZigHydrator) renderFunction(fn *ZigFunction) string {
	var sb strings.Builder
	if fn.Doc != "" {
		for _, doc := range strings.Split(fn.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", doc))
		}
	}

	var declKeywords []string
	if fn.Visibility == "public" {
		declKeywords = append(declKeywords, "pub")
	}
	if fn.IsExport {
		declKeywords = append(declKeywords, "export")
	} else if fn.IsExtern {
		declKeywords = append(declKeywords, "extern")
	}
	if fn.IsInline {
		declKeywords = append(declKeywords, "inline")
	}
	declKeywords = append(declKeywords, "fn")

	var paramParts []string
	for _, p := range fn.Params {
		pStr := p.Name
		if p.Comptime {
			pStr = "comptime " + pStr
		}
		if p.Noalias {
			pStr = "noalias " + pStr
		}
		if p.Type != "" {
			pStr += ": " + p.Type
		}
		if p.Default != "" {
			pStr += " = " + p.Default
		}
		paramParts = append(paramParts, pStr)
	}

	callConvStr := ""
	if fn.CallConv != "" {
		callConvStr = fmt.Sprintf(" callconv(%s)", fn.CallConv)
	}

	retType := fn.ReturnType
	if retType == "" {
		retType = "void"
	}

	sb.WriteString(fmt.Sprintf("%s %s(%s)%s %s", strings.Join(declKeywords, " "), fn.Name, strings.Join(paramParts, ", "), callConvStr, retType))

	if fn.BodySource != "" {
		trimmedBody := strings.TrimSpace(fn.BodySource)
		if strings.HasPrefix(trimmedBody, "{") {
			sb.WriteString(" " + trimmedBody)
		} else {
			sb.WriteString(" {\n")
			for _, line := range strings.Split(trimmedBody, "\n") {
				sb.WriteString("    " + line + "\n")
			}
			sb.WriteString("}")
		}
	} else if fn.IsExtern {
		sb.WriteString(";")
	} else {
		sb.WriteString(" {}")
	}

	return sb.String()
}

func (h *ZigHydrator) renderVar(v *ZigVar) string {
	var sb strings.Builder
	if v.Doc != "" {
		for _, doc := range strings.Split(v.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", doc))
		}
	}

	var kw []string
	if v.Visibility == "public" {
		kw = append(kw, "pub")
	}
	if v.IsExtern {
		kw = append(kw, "extern")
	}
	if v.IsComptime {
		kw = append(kw, "comptime")
	}
	if v.IsConst {
		kw = append(kw, "const")
	} else {
		kw = append(kw, "var")
	}

	typeStr := ""
	if v.Type != "" {
		typeStr = fmt.Sprintf(": %s", v.Type)
	}
	valStr := ""
	if v.Value != "" {
		valStr = fmt.Sprintf(" = %s", v.Value)
	}

	sb.WriteString(fmt.Sprintf("%s %s%s%s;", strings.Join(kw, " "), v.Name, typeStr, valStr))
	return sb.String()
}

func (h *ZigHydrator) renderTest(tst *ZigTest) string {
	var sb strings.Builder
	if tst.Doc != "" {
		for _, doc := range strings.Split(tst.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("/// %s\n", doc))
		}
	}
	nameStr := ""
	if tst.Name != "" && tst.Name != "anonymous" {
		nameStr = fmt.Sprintf(" %q", tst.Name)
	}
	sb.WriteString(fmt.Sprintf("test%s ", nameStr))
	trimmedBody := strings.TrimSpace(tst.BodySource)
	if strings.HasPrefix(trimmedBody, "{") {
		sb.WriteString(trimmedBody)
	} else {
		sb.WriteString("{\n")
		for _, line := range strings.Split(trimmedBody, "\n") {
			sb.WriteString("    " + line + "\n")
		}
		sb.WriteString("}")
	}
	return sb.String()
}
