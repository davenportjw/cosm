package ruby

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// RubyHydrator reconstitutes Ruby AST symbol nodes back into clean, formatted Ruby source code.
type RubyHydrator struct{}

// NewRubyHydrator creates a new RubyHydrator instance.
func NewRubyHydrator() *RubyHydrator {
	return &RubyHydrator{}
}

// HydrateSymbol reconstitutes a single Ruby ASTSymbolNode into source code.
func (h *RubyHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("# Ruby Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "ClassDef":
		var cls RubyClass
		if err := json.Unmarshal(node.ASTPayload, &cls); err != nil {
			return "", fmt.Errorf("unmarshaling RubyClass: %w", err)
		}
		return h.renderClass(&cls), nil

	case "ModuleDef":
		var mod RubyModule
		if err := json.Unmarshal(node.ASTPayload, &mod); err != nil {
			return "", fmt.Errorf("unmarshaling RubyModule: %w", err)
		}
		return h.renderModule(&mod), nil

	case "MethodDef":
		var m RubyMethod
		if err := json.Unmarshal(node.ASTPayload, &m); err != nil {
			return "", fmt.Errorf("unmarshaling RubyMethod: %w", err)
		}
		return h.renderMethod(&m, 0), nil

	case "RouteBinding":
		var r RubyRouteBinding
		if err := json.Unmarshal(node.ASTPayload, &r); err != nil {
			return "", fmt.Errorf("unmarshaling RubyRouteBinding: %w", err)
		}
		return h.renderRoute(&r), nil

	default:
		return string(node.ASTPayload), nil
	}
}

func (h *RubyHydrator) renderClass(cls *RubyClass) string {
	var sb strings.Builder

	if cls.Doc != "" {
		for _, line := range strings.Split(cls.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("# %s\n", line))
		}
	}

	if cls.Superclass != "" {
		sb.WriteString(fmt.Sprintf("class %s < %s\n", cls.Name, cls.Superclass))
	} else {
		sb.WriteString(fmt.Sprintf("class %s\n", cls.Name))
	}

	// Modules Included
	for _, inc := range cls.ModulesIncluded {
		sb.WriteString(fmt.Sprintf("  include %s\n", inc))
	}

	// Modules Extended
	for _, ext := range cls.ModulesExtended {
		sb.WriteString(fmt.Sprintf("  extend %s\n", ext))
	}

	if len(cls.ModulesIncluded) > 0 || len(cls.ModulesExtended) > 0 {
		sb.WriteString("\n")
	}

	// Attr accessors
	if len(cls.AttrAccessors) > 0 {
		var syms []string
		for _, a := range cls.AttrAccessors {
			syms = append(syms, ":"+a)
		}
		sb.WriteString(fmt.Sprintf("  attr_accessor %s\n", strings.Join(syms, ", ")))
	}
	if len(cls.AttrReaders) > 0 {
		var syms []string
		for _, a := range cls.AttrReaders {
			syms = append(syms, ":"+a)
		}
		sb.WriteString(fmt.Sprintf("  attr_reader %s\n", strings.Join(syms, ", ")))
	}
	if len(cls.AttrWriters) > 0 {
		var syms []string
		for _, a := range cls.AttrWriters {
			syms = append(syms, ":"+a)
		}
		sb.WriteString(fmt.Sprintf("  attr_writer %s\n", strings.Join(syms, ", ")))
	}

	if len(cls.AttrAccessors) > 0 || len(cls.AttrReaders) > 0 || len(cls.AttrWriters) > 0 {
		sb.WriteString("\n")
	}

	// Associations
	for _, assoc := range cls.Associations {
		if assoc.Raw != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", assoc.Raw))
		} else {
			optStr := renderRubyOptions(assoc.Options)
			if optStr != "" {
				sb.WriteString(fmt.Sprintf("  %s :%s, %s\n", assoc.Kind, assoc.Target, optStr))
			} else {
				sb.WriteString(fmt.Sprintf("  %s :%s\n", assoc.Kind, assoc.Target))
			}
		}
	}
	if len(cls.Associations) > 0 {
		sb.WriteString("\n")
	}

	// Validations
	for _, val := range cls.Validations {
		if val.Raw != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", val.Raw))
		} else {
			var fieldSyms []string
			for _, f := range val.Fields {
				fieldSyms = append(fieldSyms, ":"+f)
			}
			optStr := renderRubyOptions(val.Options)
			if optStr != "" {
				sb.WriteString(fmt.Sprintf("  validates %s, %s\n", strings.Join(fieldSyms, ", "), optStr))
			} else {
				sb.WriteString(fmt.Sprintf("  validates %s\n", strings.Join(fieldSyms, ", ")))
			}
		}
	}
	if len(cls.Validations) > 0 {
		sb.WriteString("\n")
	}

	// Group methods by visibility
	var publicMethods, protectedMethods, privateMethods []RubyMethod
	for _, m := range cls.Methods {
		switch m.Visibility {
		case "private":
			privateMethods = append(privateMethods, m)
		case "protected":
			protectedMethods = append(protectedMethods, m)
		default:
			publicMethods = append(publicMethods, m)
		}
	}

	// Public methods
	for _, m := range publicMethods {
		sb.WriteString(h.renderMethod(&m, 1))
		sb.WriteString("\n")
	}

	// Protected methods
	if len(protectedMethods) > 0 {
		sb.WriteString("  protected\n\n")
		for _, m := range protectedMethods {
			sb.WriteString(h.renderMethod(&m, 1))
			sb.WriteString("\n")
		}
	}

	// Private methods
	if len(privateMethods) > 0 {
		sb.WriteString("  private\n\n")
		for _, m := range privateMethods {
			sb.WriteString(h.renderMethod(&m, 1))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("end\n")
	return sb.String()
}

func (h *RubyHydrator) renderModule(mod *RubyModule) string {
	var sb strings.Builder

	if mod.Doc != "" {
		for _, line := range strings.Split(mod.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("# %s\n", line))
		}
	}

	sb.WriteString(fmt.Sprintf("module %s\n", mod.Name))

	for _, inc := range mod.ModulesIncluded {
		sb.WriteString(fmt.Sprintf("  include %s\n", inc))
	}
	for _, ext := range mod.ModulesExtended {
		sb.WriteString(fmt.Sprintf("  extend %s\n", ext))
	}
	if len(mod.ModulesIncluded) > 0 || len(mod.ModulesExtended) > 0 {
		sb.WriteString("\n")
	}

	for _, m := range mod.Methods {
		sb.WriteString(h.renderMethod(&m, 1))
		sb.WriteString("\n")
	}

	sb.WriteString("end\n")
	return sb.String()
}

func (h *RubyHydrator) renderMethod(m *RubyMethod, indentLevel int) string {
	indent := strings.Repeat("  ", indentLevel)
	var sb strings.Builder

	if m.Doc != "" {
		for _, line := range strings.Split(m.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("%s# %s\n", indent, line))
		}
	}

	prefix := ""
	if m.IsClassMethod {
		prefix = "self."
	}

	var paramParts []string
	for _, p := range m.Params {
		switch p.Kind {
		case "block":
			paramParts = append(paramParts, "&"+p.Name)
		case "rest":
			paramParts = append(paramParts, "*"+p.Name)
		case "key":
			paramParts = append(paramParts, fmt.Sprintf("%s: %s", p.Name, p.DefaultValue))
		case "keyreq":
			paramParts = append(paramParts, fmt.Sprintf("%s:", p.Name))
		case "opt":
			paramParts = append(paramParts, fmt.Sprintf("%s = %s", p.Name, p.DefaultValue))
		default:
			paramParts = append(paramParts, p.Name)
		}
	}

	paramsStr := ""
	if len(paramParts) > 0 {
		paramsStr = fmt.Sprintf("(%s)", strings.Join(paramParts, ", "))
	}

	sb.WriteString(fmt.Sprintf("%sdef %s%s%s\n", indent, prefix, m.Name, paramsStr))

	if m.BodySource != "" {
		// Clean lines from existing body
		bodyLines := strings.Split(m.BodySource, "\n")
		var innerLines []string
		for idx, line := range bodyLines {
			if idx == 0 && (strings.Contains(line, "def ") || strings.Contains(line, "def\t")) {
				continue
			}
			if idx == len(bodyLines)-1 && strings.TrimSpace(line) == "end" {
				continue
			}
			innerLines = append(innerLines, line)
		}

		if len(innerLines) > 0 {
			for _, line := range innerLines {
				sb.WriteString(fmt.Sprintf("%s  %s\n", indent, strings.TrimSpace(line)))
			}
		} else {
			sb.WriteString(fmt.Sprintf("%s  # implementation\n", indent))
		}
	} else {
		sb.WriteString(fmt.Sprintf("%s  # implementation\n", indent))
	}

	sb.WriteString(fmt.Sprintf("%send\n", indent))
	return sb.String()
}

func (h *RubyHydrator) renderRoute(r *RubyRouteBinding) string {
	if r.Framework == "sinatra" {
		return fmt.Sprintf("%s '%s' do\n  # %s\nend\n", strings.ToLower(r.Method), r.Path, r.HandlerName)
	}

	if r.Method == "RESOURCES" {
		return fmt.Sprintf("resources :%s\n", strings.TrimPrefix(r.Path, "/"))
	}

	if r.Path == "/" && r.Controller != "" && r.Action != "" {
		return fmt.Sprintf("root to: '%s#%s'\n", r.Controller, r.Action)
	}

	if r.Controller != "" && r.Action != "" {
		return fmt.Sprintf("%s '%s', to: '%s#%s'\n", strings.ToLower(r.Method), r.Path, r.Controller, r.Action)
	}

	return fmt.Sprintf("%s '%s'\n", strings.ToLower(r.Method), r.Path)
}

func renderRubyOptions(opts map[string]string) string {
	if len(opts) == 0 {
		return ""
	}
	var keys []string
	for k := range opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var pairs []string
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s: %s", k, opts[k]))
	}
	return strings.Join(pairs, ", ")
}
