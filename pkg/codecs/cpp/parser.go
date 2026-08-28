package cpp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// CppField represents a member variable inside a class or struct.
type CppField struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Visibility string `json:"visibility,omitempty"` // public, protected, private
	IsStatic   bool   `json:"is_static"`
	IsConst    bool   `json:"is_const"`
	Doc        string `json:"doc,omitempty"`
}

// CppParam represents a function parameter.
type CppParam struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	DefaultValue string `json:"default_value,omitempty"`
}

// CppFunction represents an extracted function or class method.
type CppFunction struct {
	Name         string     `json:"name"`
	Namespace    string     `json:"namespace,omitempty"`
	ClassName    string     `json:"class_name,omitempty"`
	Visibility   string     `json:"visibility,omitempty"` // public, protected, private
	ReturnType   string     `json:"return_type"`
	Template     string     `json:"template,omitempty"` // template<typename T>
	Params       []CppParam `json:"params"`
	IsVirtual    bool       `json:"is_virtual"`
	IsConst      bool       `json:"is_const"`
	IsStatic     bool       `json:"is_static"`
	IsInline     bool       `json:"is_inline"`
	IsNoexcept   bool       `json:"is_noexcept"`
	Doc          string     `json:"doc,omitempty"`
	BodySource   string     `json:"body_source,omitempty"`
	CalledFuncs  []string   `json:"called_funcs,omitempty"`
}

// CppClass represents an extracted class or struct.
type CppClass struct {
	Kind         string        `json:"kind"` // class, struct
	Name         string        `json:"name"`
	Namespace    string        `json:"namespace,omitempty"`
	Template     string        `json:"template,omitempty"`
	BaseClasses  []string      `json:"base_classes,omitempty"`
	Doc          string        `json:"doc,omitempty"`
	Fields       []CppField    `json:"fields"`
	Methods      []CppFunction `json:"methods"`
}

// CppEnumVal represents an enum value.
type CppEnumVal struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	Doc   string `json:"doc,omitempty"`
}

// CppEnum represents an enum or enum class.
type CppEnum struct {
	Name      string       `json:"name"`
	Namespace string       `json:"namespace,omitempty"`
	IsScoped  bool         `json:"is_scoped"` // enum class
	Doc       string       `json:"doc,omitempty"`
	Values    []CppEnumVal `json:"values"`
}

// CppFileResult contains all extracted C/C++ symbols.
type CppFileResult struct {
	FilePath   string              `json:"file_path"`
	Includes   []string            `json:"includes"`
	Namespaces []string            `json:"namespaces"`
	Classes    []CppClass          `json:"classes"`
	Functions  []CppFunction       `json:"functions"`
	Enums      []CppEnum           `json:"enums"`
	AllSymbols []*core.ASTSymbolNode `json:"all_symbols"`
}

// CppParser parses C and C++ source/header files into structured AST symbol nodes.
type CppParser struct{}

// NewCppParser creates a new CppParser instance.
func NewCppParser() *CppParser {
	return &CppParser{}
}

// ParseSource parses C/C++ source code from bytes.
func (p *CppParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*CppFileResult, error) {
	if filename == "" {
		filename = "main.cpp"
	}

	result := &CppFileResult{
		FilePath: filename,
	}

	lines := splitCppLines(src)

	// 1. Extract Includes
	result.Includes = p.extractIncludes(lines)

	// 2. Extract Classes/Structs
	result.Classes = p.extractClasses(lines)

	// 3. Extract Top-level Functions
	result.Functions = p.extractFunctions(lines)

	// 4. Extract Enums
	result.Enums = p.extractEnums(lines)

	// 5. Convert to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a C/C++ file from disk.
func (p *CppParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*CppFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all C/C++ source and header files in a directory.
func (p *CppParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*CppFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*CppFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext == ".cpp" || ext == ".cc" || ext == ".cxx" || ext == ".c" || ext == ".h" || ext == ".hpp" || ext == ".hxx" {
			fPath := filepath.Join(dirPath, entry.Name())
			res, err := p.ParseFile(fPath, lineage)
			if err != nil {
				return nil, err
			}
			results = append(results, res)
		}
	}
	return results, nil
}

type cppLine struct {
	num  int
	text string
}

func splitCppLines(src []byte) []cppLine {
	var lines []cppLine
	scanner := bufio.NewScanner(bytes.NewReader(src))
	num := 1
	for scanner.Scan() {
		lines = append(lines, cppLine{
			num:  num,
			text: scanner.Text(),
		})
		num++
	}
	return lines
}

var (
	cppIncludeRegex  = regexp.MustCompile(`^#include\s+([<"][^>"]+[>"])`)
	cppClassRegex    = regexp.MustCompile(`^(?:template\s*<[^>]+>\s*)?(class|struct)\s+([A-Za-z0-9_]+)(?:\s*:\s*([^{]+))?`)
	cppEnumRegex     = regexp.MustCompile(`^enum\s+(?:(class|struct)\s+)?([A-Za-z0-9_]+)`)
	cppFnHeaderRegex = regexp.MustCompile(`^(?:(template\s*<[^>]+>)\s*)?(?:(inline|static|virtual|explicit|friend)\s+)*(?:([a-zA-Z0-9_:<>&*~]+)\s+)?([a-zA-Z0-9_:]+)\s*\((.*?)\)(?:\s*(const|noexcept|override|final|\=\s*0))*\s*(\{|;|:\s*)`)
)

func (p *CppParser) extractIncludes(lines []cppLine) []string {
	var includes []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := cppIncludeRegex.FindStringSubmatch(trimmed); m != nil {
			includes = append(includes, m[1])
		}
	}
	return includes
}

func (p *CppParser) extractClasses(lines []cppLine) []CppClass {
	var classes []CppClass
	var pendingDoc []string
	var pendingTemplate string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			docLine := strings.TrimPrefix(trimmed, "//")
			docLine = strings.TrimPrefix(docLine, "/*")
			docLine = strings.TrimPrefix(docLine, "*/")
			docLine = strings.TrimPrefix(docLine, "*")
			docClean := strings.TrimSpace(docLine)
			if docClean != "" {
				pendingDoc = append(pendingDoc, docClean)
			}
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "template") && strings.HasSuffix(trimmed, ">") {
			pendingTemplate = trimmed
			i++
			continue
		}

		if m := cppClassRegex.FindStringSubmatch(trimmed); m != nil && !strings.HasSuffix(trimmed, ";") && !strings.Contains(trimmed, "(") {
			kind := m[1]
			name := m[2]
			basesStr := m[3]

			var baseClasses []string
			if basesStr != "" {
				for _, b := range strings.Split(basesStr, ",") {
					b = strings.TrimSpace(b)
					if b != "" {
						baseClasses = append(baseClasses, b)
					}
				}
			}

			var fields []CppField
			var methods []CppFunction

			currentVis := "private"
			if kind == "struct" {
				currentVis = "public"
			}

			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			var memberDoc []string

			i++
			for i < len(lines) {
				mLine := strings.TrimSpace(lines[i].text)

				if strings.HasPrefix(mLine, "//") || strings.HasPrefix(mLine, "/*") || strings.HasPrefix(mLine, "*") {
					docLine := strings.TrimPrefix(mLine, "//")
					docLine = strings.TrimPrefix(docLine, "/*")
					docLine = strings.TrimPrefix(docLine, "*/")
					docLine = strings.TrimPrefix(docLine, "*")
					docClean := strings.TrimSpace(docLine)
					if docClean != "" {
						memberDoc = append(memberDoc, docClean)
					}
					i++
					continue
				}

				if mLine == "public:" {
					currentVis = "public"
					memberDoc = nil
					i++
					continue
				} else if mLine == "protected:" {
					currentVis = "protected"
					memberDoc = nil
					i++
					continue
				} else if mLine == "private:" {
					currentVis = "private"
					memberDoc = nil
					i++
					continue
				}

				// Check method
				if fm := cppFnHeaderRegex.FindStringSubmatch(mLine); fm != nil && !strings.HasPrefix(mLine, "class ") && !strings.HasPrefix(mLine, "struct ") {
					tmpl := fm[1]
					retType := strings.TrimSpace(fm[3])
					fnName := fm[4]
					paramsStr := fm[5]
					modifiers := fm[6]

					isVirtual := strings.Contains(mLine, "virtual")
					isConst := strings.Contains(modifiers, "const")
					isStatic := strings.Contains(mLine, "static")
					isInline := strings.Contains(mLine, "inline")
					isNoexcept := strings.Contains(modifiers, "noexcept")

					// Collect body if inline
					var bodyLines []string
					bodyLines = append(bodyLines, mLine)
					methodBraces := 0
					for _, ch := range mLine {
						if ch == '{' {
							methodBraces++
						} else if ch == '}' {
							methodBraces--
						}
					}

					if methodBraces > 0 {
						for i+1 < len(lines) {
							i++
							bLine := lines[i].text
							bodyLines = append(bodyLines, bLine)
							for _, ch := range bLine {
								if ch == '{' {
									methodBraces++
								} else if ch == '}' {
									methodBraces--
								}
							}
							if methodBraces <= 0 {
								break
							}
						}
					}

					methods = append(methods, CppFunction{
						Name:        fnName,
						ClassName:   name,
						Visibility:  currentVis,
						ReturnType:  retType,
						Template:    tmpl,
						Params:      parseCppParams(paramsStr),
						IsVirtual:   isVirtual,
						IsConst:     isConst,
						IsStatic:    isStatic,
						IsInline:    isInline,
						IsNoexcept:  isNoexcept,
						Doc:         strings.Join(memberDoc, "\n"),
						BodySource:  strings.Join(bodyLines, "\n"),
						CalledFuncs: extractCppCalls(strings.Join(bodyLines, "\n")),
					})
					memberDoc = nil
				} else {
					for _, ch := range mLine {
						if ch == '{' {
							braceCount++
						} else if ch == '}' {
							braceCount--
						}
					}
					if braceCount <= 0 {
						break
					}

					if strings.HasSuffix(mLine, ";") && !strings.Contains(mLine, "(") && mLine != "" {
						// Field
						fClean := strings.TrimSuffix(mLine, ";")
						parts := strings.Fields(fClean)
						if len(parts) >= 2 {
							fName := parts[len(parts)-1]
							fType := strings.Join(parts[:len(parts)-1], " ")
							fields = append(fields, CppField{
								Name:       fName,
								Type:       fType,
								Visibility: currentVis,
								IsStatic:   strings.Contains(fType, "static"),
								IsConst:    strings.Contains(fType, "const"),
								Doc:        strings.Join(memberDoc, "\n"),
							})
						}
						memberDoc = nil
					} else {
						if mLine != "" && !strings.HasPrefix(mLine, "//") {
							memberDoc = nil
						}
					}
				}
				i++
			}

			classes = append(classes, CppClass{
				Kind:        kind,
				Name:        name,
				Template:    pendingTemplate,
				BaseClasses: baseClasses,
				Doc:         strings.Join(pendingDoc, "\n"),
				Fields:      fields,
				Methods:     methods,
			})

			pendingDoc = nil
			pendingTemplate = ""
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
				pendingTemplate = ""
			}
		}
		i++
	}

	return classes
}

func (p *CppParser) extractFunctions(lines []cppLine) []CppFunction {
	var functions []CppFunction
	var pendingDoc []string
	var pendingTemplate string

	i := 0
	inClassOrStruct := 0

	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "struct ") {
			if strings.Contains(trimmed, "{") {
				inClassOrStruct++
			}
			i++
			continue
		}

		if inClassOrStruct > 0 {
			for _, ch := range trimmed {
				if ch == '{' {
					inClassOrStruct++
				} else if ch == '}' {
					inClassOrStruct--
				}
			}
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			docLine := strings.TrimPrefix(trimmed, "//")
			docLine = strings.TrimPrefix(docLine, "/*")
			docLine = strings.TrimPrefix(docLine, "*/")
			docLine = strings.TrimPrefix(docLine, "*")
			docClean := strings.TrimSpace(docLine)
			if docClean != "" {
				pendingDoc = append(pendingDoc, docClean)
			}
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "template") && strings.HasSuffix(trimmed, ">") {
			pendingTemplate = trimmed
			i++
			continue
		}

		if fm := cppFnHeaderRegex.FindStringSubmatch(trimmed); fm != nil && !strings.HasPrefix(trimmed, "class ") && !strings.HasPrefix(trimmed, "struct ") {
			tmpl := fm[1]
			if tmpl == "" {
				tmpl = pendingTemplate
			}
			retType := strings.TrimSpace(fm[3])
			fnName := fm[4]
			paramsStr := fm[5]
			modifiers := fm[6]

			isVirtual := strings.Contains(trimmed, "virtual")
			isConst := strings.Contains(modifiers, "const")
			isStatic := strings.Contains(trimmed, "static")
			isInline := strings.Contains(trimmed, "inline")
			isNoexcept := strings.Contains(modifiers, "noexcept")

			var bodyLines []string
			bodyLines = append(bodyLines, lines[i].text)
			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			if braceCount > 0 {
				for i+1 < len(lines) {
					i++
					bLine := lines[i].text
					bodyLines = append(bodyLines, bLine)
					for _, ch := range bLine {
						if ch == '{' {
							braceCount++
						} else if ch == '}' {
							braceCount--
						}
					}
					if braceCount <= 0 {
						break
					}
				}
			}

			fullBody := strings.Join(bodyLines, "\n")
			functions = append(functions, CppFunction{
				Name:        fnName,
				Visibility:  "public",
				ReturnType:  retType,
				Template:    tmpl,
				Params:      parseCppParams(paramsStr),
				IsVirtual:   isVirtual,
				IsConst:     isConst,
				IsStatic:    isStatic,
				IsInline:    isInline,
				IsNoexcept:  isNoexcept,
				Doc:         strings.Join(pendingDoc, "\n"),
				BodySource:  fullBody,
				CalledFuncs: extractCppCalls(fullBody),
			})

			pendingDoc = nil
			pendingTemplate = ""
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
				pendingTemplate = ""
			}
		}
		i++
	}

	return functions
}

func (p *CppParser) extractEnums(lines []cppLine) []CppEnum {
	var enums []CppEnum
	var pendingDoc []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			docLine := strings.TrimPrefix(trimmed, "//")
			docLine = strings.TrimPrefix(docLine, "/*")
			docLine = strings.TrimPrefix(docLine, "*/")
			docLine = strings.TrimPrefix(docLine, "*")
			docClean := strings.TrimSpace(docLine)
			if docClean != "" {
				pendingDoc = append(pendingDoc, docClean)
			}
			i++
			continue
		}

		if m := cppEnumRegex.FindStringSubmatch(trimmed); m != nil {
			isScoped := strings.TrimSpace(m[1]) != ""
			name := m[2]

			var values []CppEnumVal
			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			i++
			for i < len(lines) {
				eLine := strings.TrimSpace(lines[i].text)
				if strings.Contains(eLine, "{") {
					braceCount++
				}
				if strings.Contains(eLine, "}") {
					braceCount--
					if braceCount <= 0 {
						break
					}
				}
				if eLine != "" && !strings.HasPrefix(eLine, "//") {
					eClean := strings.TrimSuffix(eLine, ",")
					if eClean != "" && !strings.Contains(eClean, "{") && !strings.Contains(eClean, "}") {
						var vName, val string
						if eqIdx := strings.Index(eClean, "="); eqIdx != -1 {
							vName = strings.TrimSpace(eClean[:eqIdx])
							val = strings.TrimSpace(eClean[eqIdx+1:])
						} else {
							vName = eClean
						}
						values = append(values, CppEnumVal{
							Name:  vName,
							Value: val,
						})
					}
				}
				i++
			}

			enums = append(enums, CppEnum{
				Name:     name,
				IsScoped: isScoped,
				Doc:      strings.Join(pendingDoc, "\n"),
				Values:   values,
			})
			pendingDoc = nil
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
			}
		}
		i++
	}

	return enums
}

func parseCppParams(s string) []CppParam {
	var params []CppParam
	s = strings.TrimSpace(s)
	if s == "" || s == "void" {
		return params
	}

	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var defVal string
		if eqIdx := strings.Index(p, "="); eqIdx != -1 {
			defVal = strings.TrimSpace(p[eqIdx+1:])
			p = strings.TrimSpace(p[:eqIdx])
		}
		parts := strings.Fields(p)
		if len(parts) >= 2 {
			pName := parts[len(parts)-1]
			pType := strings.Join(parts[:len(parts)-1], " ")
			params = append(params, CppParam{
				Name:         pName,
				Type:         pType,
				DefaultValue: defVal,
			})
		} else if len(parts) == 1 {
			params = append(params, CppParam{
				Name:         parts[0],
				Type:         "auto",
				DefaultValue: defVal,
			})
		}
	}
	return params
}

var cppCallRegex = regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*\(`)

func extractCppCalls(src string) []string {
	var calls []string
	seen := make(map[string]bool)
	matches := cppCallRegex.FindAllStringSubmatch(src, -1)
	for _, m := range matches {
		name := m[1]
		if !seen[name] && name != "if" && name != "while" && name != "for" && name != "switch" && name != "catch" {
			seen[name] = true
			calls = append(calls, name)
		}
	}
	sort.Strings(calls)
	return calls
}

func (p *CppParser) toSymbolNodes(res *CppFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Classes
	for _, cls := range res.Classes {
		payload, err := json.Marshal(cls)
		if err != nil {
			return nil, err
		}

		var deps []string
		deps = append(deps, cls.BaseClasses...)
		for _, f := range cls.Fields {
			deps = append(deps, f.Type)
		}
		for _, m := range cls.Methods {
			deps = append(deps, m.ReturnType)
			for _, p := range m.Params {
				deps = append(deps, p.Type)
			}
		}
		sort.Strings(deps)

		node := &core.ASTSymbolNode{
			Language:          core.LangCpp,
			NodeType:          "ClassDeclaration",
			Identifier:        fmt.Sprintf("class:%s", cls.Name),
			Signature:         fmt.Sprintf("%s %s", cls.Kind, cls.Name),
			Docstring:         cls.Doc,
			Visibility:        "public",
			ASTPayload:        payload,
			ASTMetadata:       map[string]string{"kind": cls.Kind, "method_count": fmt.Sprintf("%d", len(cls.Methods))},
			LocalDependencies: deps,
			Dependencies:      deps,
			Lineage:           lineage,
		}

		nodeID, err := core.HashASTSymbolNode(node)
		if err != nil {
			return nil, err
		}
		node.NodeID = nodeID
		nodes = append(nodes, node)
	}

	// Functions
	for _, fn := range res.Functions {
		payload, err := json.Marshal(fn)
		if err != nil {
			return nil, err
		}

		sig := fmt.Sprintf("%s %s(%s)", fn.ReturnType, fn.Name, formatCppParams(fn.Params))
		if fn.Template != "" {
			sig = fmt.Sprintf("%s %s", fn.Template, sig)
		}

		node := &core.ASTSymbolNode{
			Language:          core.LangCpp,
			NodeType:          "FunctionDeclaration",
			Identifier:        fmt.Sprintf("fn:%s", fn.Name),
			Signature:         sig,
			Docstring:         fn.Doc,
			Visibility:        "public",
			ASTPayload:        payload,
			ASTMetadata:       map[string]string{"return_type": fn.ReturnType},
			LocalDependencies: fn.CalledFuncs,
			Dependencies:      fn.CalledFuncs,
			Lineage:           lineage,
		}

		nodeID, err := core.HashASTSymbolNode(node)
		if err != nil {
			return nil, err
		}
		node.NodeID = nodeID
		nodes = append(nodes, node)
	}

	// Enums
	for _, en := range res.Enums {
		payload, err := json.Marshal(en)
		if err != nil {
			return nil, err
		}

		node := &core.ASTSymbolNode{
			Language:    core.LangCpp,
			NodeType:    "EnumDeclaration",
			Identifier:  fmt.Sprintf("enum:%s", en.Name),
			Signature:   fmt.Sprintf("enum %s", en.Name),
			Docstring:   en.Doc,
			Visibility:  "public",
			ASTPayload:  payload,
			ASTMetadata: map[string]string{"val_count": fmt.Sprintf("%d", len(en.Values))},
			Lineage:     lineage,
		}

		nodeID, err := core.HashASTSymbolNode(node)
		if err != nil {
			return nil, err
		}
		enNodeID := nodeID
		node.NodeID = enNodeID
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func formatCppParams(params []CppParam) string {
	var list []string
	for _, p := range params {
		s := fmt.Sprintf("%s %s", p.Type, p.Name)
		if p.DefaultValue != "" {
			s += " = " + p.DefaultValue
		}
		list = append(list, s)
	}
	return strings.Join(list, ", ")
}

// BuildComponentNode bundles parsed C/C++ symbols into a core.ComponentNode.
func (p *CppParser) BuildComponentNode(
	res *CppFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = "cpp-module"
	}
	if !compType.IsValid() {
		compType = core.CompLibrary
	}

	symbolIDs := make([]string, 0, len(res.AllSymbols))
	for _, sym := range res.AllSymbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	metadata := map[string]string{
		"file_path":     res.FilePath,
		"class_count":   fmt.Sprintf("%d", len(res.Classes)),
		"func_count":    fmt.Sprintf("%d", len(res.Functions)),
		"symbol_count":  fmt.Sprintf("%d", len(res.AllSymbols)),
		"include_count": fmt.Sprintf("%d", len(res.Includes)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangCpp,
		SymbolNodes: symbolIDs,
		Metadata:    metadata,
		Lineage:     lineage,
	}

	compID, err := core.HashComponentNode(comp)
	if err != nil {
		return nil, fmt.Errorf("computing component hash: %w", err)
	}
	comp.ComponentID = compID
	return comp, nil
}
