package cpp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// CppHydrator converts C/C++ ASTSymbolNodes back into formatted C/C++ source code.
type CppHydrator struct{}

// NewCppHydrator creates an initialized CppHydrator instance.
func NewCppHydrator() *CppHydrator {
	return &CppHydrator{}
}

// HydrateSymbol reconstitutes an ASTSymbolNode into C/C++ code.
func (h *CppHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("// C++ Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "ClassDeclaration":
		var cls CppClass
		if err := json.Unmarshal(node.ASTPayload, &cls); err != nil {
			return "", fmt.Errorf("unmarshaling CppClass: %w", err)
		}
		return h.renderClass(&cls), nil

	case "FunctionDeclaration":
		var fn CppFunction
		if err := json.Unmarshal(node.ASTPayload, &fn); err != nil {
			return "", fmt.Errorf("unmarshaling CppFunction: %w", err)
		}
		return h.renderFunction(&fn, 0), nil

	case "EnumDeclaration":
		var en CppEnum
		if err := json.Unmarshal(node.ASTPayload, &en); err != nil {
			return "", fmt.Errorf("unmarshaling CppEnum: %w", err)
		}
		return h.renderEnum(&en), nil

	default:
		return string(node.ASTPayload), nil
	}
}

func (h *CppHydrator) renderClass(cls *CppClass) string {
	var sb strings.Builder

	if cls.Doc != "" {
		for _, line := range strings.Split(cls.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("// %s\n", line))
		}
	}

	if cls.Template != "" {
		sb.WriteString(fmt.Sprintf("%s\n", cls.Template))
	}

	kind := cls.Kind
	if kind == "" {
		kind = "class"
	}

	basesStr := ""
	if len(cls.BaseClasses) > 0 {
		basesStr = fmt.Sprintf(" : %s", strings.Join(cls.BaseClasses, ", "))
	}

	sb.WriteString(fmt.Sprintf("%s %s%s {\n", kind, cls.Name, basesStr))

	// Group members by visibility
	var publicFields, protectedFields, privateFields []CppField
	for _, f := range cls.Fields {
		switch f.Visibility {
		case "public":
			publicFields = append(publicFields, f)
		case "protected":
			protectedFields = append(protectedFields, f)
		default:
			privateFields = append(privateFields, f)
		}
	}

	var publicMethods, protectedMethods, privateMethods []CppFunction
	for _, m := range cls.Methods {
		switch m.Visibility {
		case "public":
			publicMethods = append(publicMethods, m)
		case "protected":
			protectedMethods = append(protectedMethods, m)
		default:
			privateMethods = append(privateMethods, m)
		}
	}

	// Render public section
	if len(publicFields) > 0 || len(publicMethods) > 0 {
		sb.WriteString("public:\n")
		for _, f := range publicFields {
			sb.WriteString(h.renderField(&f, 1))
		}
		for _, m := range publicMethods {
			sb.WriteString(h.renderFunction(&m, 1))
		}
	}

	// Render protected section
	if len(protectedFields) > 0 || len(protectedMethods) > 0 {
		sb.WriteString("protected:\n")
		for _, f := range protectedFields {
			sb.WriteString(h.renderField(&f, 1))
		}
		for _, m := range protectedMethods {
			sb.WriteString(h.renderFunction(&m, 1))
		}
	}

	// Render private section
	if len(privateFields) > 0 || len(privateMethods) > 0 {
		sb.WriteString("private:\n")
		for _, f := range privateFields {
			sb.WriteString(h.renderField(&f, 1))
		}
		for _, m := range privateMethods {
			sb.WriteString(h.renderFunction(&m, 1))
		}
	}

	sb.WriteString("};\n")
	return sb.String()
}

func (h *CppHydrator) renderField(f *CppField, indentLevel int) string {
	indent := strings.Repeat("    ", indentLevel)
	var sb strings.Builder
	if f.Doc != "" {
		sb.WriteString(fmt.Sprintf("%s// %s\n", indent, f.Doc))
	}
	sb.WriteString(fmt.Sprintf("%s%s %s;\n", indent, f.Type, f.Name))
	return sb.String()
}

func (h *CppHydrator) renderFunction(fn *CppFunction, indentLevel int) string {
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
		for _, line := range strings.Split(fn.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s// %s\n", indent, line))
		}
	}

	if fn.Template != "" {
		sb.WriteString(fmt.Sprintf("%s%s\n", indent, fn.Template))
	}

	virtualPrefix := ""
	if fn.IsVirtual {
		virtualPrefix = "virtual "
	}
	staticPrefix := ""
	if fn.IsStatic {
		staticPrefix = "static "
	}
	inlinePrefix := ""
	if fn.IsInline {
		inlinePrefix = "inline "
	}

	retType := fn.ReturnType
	if retType == "" {
		retType = "void"
	}

	var params []string
	for _, p := range fn.Params {
		s := fmt.Sprintf("%s %s", p.Type, p.Name)
		if p.DefaultValue != "" {
			s += " = " + p.DefaultValue
		}
		params = append(params, s)
	}

	modifiers := ""
	if fn.IsConst {
		modifiers += " const"
	}
	if fn.IsNoexcept {
		modifiers += " noexcept"
	}

	sb.WriteString(fmt.Sprintf("%s%s%s%s%s %s(%s)%s {\n", indent, inlinePrefix, staticPrefix, virtualPrefix, retType, fn.Name, strings.Join(params, ", "), modifiers))
	sb.WriteString(fmt.Sprintf("%s    // Reconstituted implementation\n", indent))
	sb.WriteString(fmt.Sprintf("%s}\n", indent))

	return sb.String()
}

func (h *CppHydrator) renderEnum(en *CppEnum) string {
	var sb strings.Builder

	if en.Doc != "" {
		for _, line := range strings.Split(en.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("// %s\n", line))
		}
	}

	scopedStr := ""
	if en.IsScoped {
		scopedStr = "class "
	}

	sb.WriteString(fmt.Sprintf("enum %s%s {\n", scopedStr, en.Name))
	for _, v := range en.Values {
		if v.Doc != "" {
			sb.WriteString(fmt.Sprintf("    // %s\n", v.Doc))
		}
		if v.Value != "" {
			sb.WriteString(fmt.Sprintf("    %s = %s,\n", v.Name, v.Value))
		} else {
			sb.WriteString(fmt.Sprintf("    %s,\n", v.Name))
		}
	}
	sb.WriteString("};\n")
	return sb.String()
}
