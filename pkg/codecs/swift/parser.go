package swift

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

// SwiftProperty represents a member variable or property in a Swift type.
type SwiftProperty struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Visibility   string   `json:"visibility,omitempty"` // public, private, internal, fileprivate, open
	IsStatic     bool     `json:"is_static"`
	IsLet        bool     `json:"is_let"` // let vs var
	IsLazy       bool     `json:"is_lazy"`
	Attributes   []string `json:"attributes,omitempty"` // @State, @Binding, @EnvironmentObject, @Published, etc.
	Doc          string   `json:"doc,omitempty"`
	DefaultValue string   `json:"default_value,omitempty"`
	GetterBody   string   `json:"getter_body,omitempty"`
}

// SwiftParam represents a function/method parameter.
type SwiftParam struct {
	Label        string   `json:"label,omitempty"` // External argument label, e.g. "_" or "with"
	Name         string   `json:"name"`            // Internal parameter name
	Type         string   `json:"type"`
	DefaultValue string   `json:"default_value,omitempty"`
	Attributes   []string `json:"attributes,omitempty"` // @escaping, inout, etc.
}

// SwiftMethod represents a method, initializer, or function.
type SwiftMethod struct {
	Name          string       `json:"name"`
	Visibility    string       `json:"visibility,omitempty"` // public, private, internal, fileprivate, open
	IsStatic      bool         `json:"is_static"`
	IsClass       bool         `json:"is_class"` // class func
	IsMutating    bool         `json:"is_mutating"`
	IsAsync       bool         `json:"is_async"`
	Throws        bool         `json:"throws"`
	ReturnType    string       `json:"return_type,omitempty"`
	Params        []SwiftParam `json:"params"`
	Attributes    []string     `json:"attributes,omitempty"` // @MainActor, @discardableResult, @objc, @IBAction
	Doc           string       `json:"doc,omitempty"`
	BodySource    string       `json:"body_source,omitempty"`
	CalledMethods []string     `json:"called_methods,omitempty"`
}

// SwiftEnumCase represents a case in a Swift enum.
type SwiftEnumCase struct {
	Name             string   `json:"name"`
	AssociatedValues []string `json:"associated_values,omitempty"`
	RawValue         string   `json:"raw_value,omitempty"`
	Doc              string   `json:"doc,omitempty"`
}

// SwiftTypeDecl represents a Swift struct, class, protocol, extension, enum, or actor.
type SwiftTypeDecl struct {
	Kind          string          `json:"kind"`                  // struct, class, protocol, extension, enum, actor
	Name          string          `json:"name"`                  // e.g. ContentView, UserService
	Visibility    string          `json:"visibility,omitempty"`  // public, private, internal, fileprivate, open
	Inheritance   []string        `json:"inheritance,omitempty"` // View, ObservableObject, Codable, etc.
	Generics      string          `json:"generics,omitempty"`
	Attributes    []string        `json:"attributes,omitempty"` // @MainActor, @frozen, @objc
	Doc           string          `json:"doc,omitempty"`
	Properties    []SwiftProperty `json:"properties"`
	Methods       []SwiftMethod   `json:"methods"`
	EnumCases     []SwiftEnumCase `json:"enum_cases,omitempty"`
	IsSwiftUIView bool            `json:"is_swiftui_view"`
	BodySource    string          `json:"body_source,omitempty"`
}

// SwiftAPICall represents an HTTP/REST endpoint consumer discovered via URLSession / URLRequest.
type SwiftAPICall struct {
	Method     string `json:"method"` // GET, POST, PUT, DELETE, PATCH, etc.
	URL        string `json:"url"`    // /api/v1/users or https://...
	Caller     string `json:"caller"` // URLSession.shared.data, URLSession.shared.dataTask, URLRequest
	LineNumber int    `json:"line_number"`
}

// SwiftFileResult contains all extracted symbols and metadata from a Swift source file.
type SwiftFileResult struct {
	FilePath          string                `json:"file_path"`
	Imports           []string              `json:"imports"`
	Types             []SwiftTypeDecl       `json:"types"`
	TopLevelFunctions []SwiftMethod         `json:"top_level_functions"`
	APICalls          []SwiftAPICall        `json:"api_calls"`
	AllSymbols        []*core.ASTSymbolNode `json:"all_symbols"`
}

// SwiftParser extracts Swift AST symbols, SwiftUI views, protocols, and URLSession endpoints.
type SwiftParser struct{}

// NewSwiftParser creates an initialized SwiftParser instance.
func NewSwiftParser() *SwiftParser {
	return &SwiftParser{}
}

// ParseSource parses Swift source code bytes.
func (p *SwiftParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*SwiftFileResult, error) {
	if filename == "" {
		filename = "Source.swift"
	}

	result := &SwiftFileResult{
		FilePath: filename,
	}

	lines := splitSwiftLines(src)

	// 1. Extract Imports
	result.Imports = p.extractImports(lines)

	// 2. Extract API calls across the entire source
	result.APICalls = p.extractAPICalls(lines)

	// 3. Extract Types (Structs, Classes, Protocols, Extensions, Enums, Actors)
	result.Types = p.extractTypes(lines)

	// 4. Extract Top-level Functions
	result.TopLevelFunctions = p.extractTopLevelFunctions(lines)

	// 5. Convert to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a Swift file from disk.
func (p *SwiftParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*SwiftFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .swift files in a directory.
func (p *SwiftParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*SwiftFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*SwiftFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".swift" {
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

type swiftLine struct {
	num  int
	text string
}

func splitSwiftLines(src []byte) []swiftLine {
	var lines []swiftLine
	scanner := bufio.NewScanner(bytes.NewReader(src))
	num := 1
	for scanner.Scan() {
		lines = append(lines, swiftLine{
			num:  num,
			text: scanner.Text(),
		})
		num++
	}
	return lines
}

var (
	swiftImportRegex = regexp.MustCompile(`^import\s+([A-Za-z0-9_.]+)(?:\s*;)?`)
	swiftTypeHeader  = regexp.MustCompile(`^(?:(public|private|internal|fileprivate|open)\s+)?(?:(final|indirect)\s+)*(struct|class|protocol|extension|enum|actor)\s+([A-Za-z0-9_]+)(?:<([^>]+)>)?(?:\s*:\s*([A-Za-z0-9_.,\s<>]+))?`)
	swiftFuncHeader  = regexp.MustCompile(`^(?:(public|private|internal|fileprivate|open)\s+)?(?:(static|class|mutating|override)\s+)*func\s+([A-Za-z0-9_]+)(?:<[^>]+>)?\s*\((.*?)\)(?:\s+(async))?(?:\s+(throws))?(?:\s*->\s*([A-Za-z0-9_?<>\[\]:.\s]+?))?\s*(\{|$)`)
	swiftPropHeader  = regexp.MustCompile(`^(?:(public|private|internal|fileprivate|open)\s+)?(?:(static|class|lazy|weak|unowned)\s+)*(let|var)\s+([A-Za-z0-9_]+)(?:\s*:\s*([A-Za-z0-9_?<>\[\]:.\s]+?))?(?:\s*=\s*(.+))?$`)
	swiftEnumCaseReg = regexp.MustCompile(`^case\s+([A-Za-z0-9_]+)(?:\((.*?)\))?(?:\s*=\s*([^,]+))?`)
)

func (p *SwiftParser) extractImports(lines []swiftLine) []string {
	var imports []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := swiftImportRegex.FindStringSubmatch(trimmed); m != nil {
			imports = append(imports, m[1])
		}
	}
	return imports
}

func (p *SwiftParser) extractTypes(lines []swiftLine) []SwiftTypeDecl {
	var types []SwiftTypeDecl
	var pendingDoc []string
	var pendingAnnotations []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		// Docstrings / comments
		if strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "/**") || strings.HasPrefix(trimmed, "*") || (strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "///")) {
			docLine := strings.TrimPrefix(trimmed, "///")
			docLine = strings.TrimPrefix(docLine, "/**")
			docLine = strings.TrimPrefix(docLine, "*/")
			docLine = strings.TrimPrefix(docLine, "*")
			docLine = strings.TrimPrefix(docLine, "//")
			docClean := strings.TrimSpace(docLine)
			if docClean != "" && !strings.HasPrefix(trimmed, "// MARK:") {
				pendingDoc = append(pendingDoc, docClean)
			}
			i++
			continue
		}

		// Annotations / Property wrappers like @MainActor, @frozen, @objc, @available(...)
		if strings.HasPrefix(trimmed, "@") && !strings.Contains(trimmed, "func ") && !strings.Contains(trimmed, "var ") && !strings.Contains(trimmed, "let ") {
			pendingAnnotations = append(pendingAnnotations, trimmed)
			i++
			continue
		}

		if m := swiftTypeHeader.FindStringSubmatch(trimmed); m != nil {
			vis := m[1]
			kind := m[3]
			name := m[4]
			generics := m[5]
			rawInherit := m[6]

			var inheritance []string
			if rawInherit != "" {
				// Strip trailing brace if present
				rawInherit = strings.TrimSuffix(strings.TrimSpace(rawInherit), "{")
				for _, inh := range strings.Split(rawInherit, ",") {
					inh = strings.TrimSpace(inh)
					if inh != "" {
						inheritance = append(inheritance, inh)
					}
				}
			}

			isSwiftUIView := false
			for _, inh := range inheritance {
				if inh == "View" || inh == "App" || inh == "Scene" {
					isSwiftUIView = true
					break
				}
			}

			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			var bodyLines []string
			bodyLines = append(bodyLines, lines[i].text)

			var properties []SwiftProperty
			var methods []SwiftMethod
			var enumCases []SwiftEnumCase

			var memberDoc []string
			var memberAttrs []string

			i++
			for i < len(lines) {
				mLine := lines[i].text
				mTrim := strings.TrimSpace(mLine)

				if strings.HasPrefix(mTrim, "///") || strings.HasPrefix(mTrim, "/**") || strings.HasPrefix(mTrim, "*") || (strings.HasPrefix(mTrim, "//") && !strings.HasPrefix(mTrim, "///")) {
					docLine := strings.TrimPrefix(mTrim, "///")
					docLine = strings.TrimPrefix(docLine, "/**")
					docLine = strings.TrimPrefix(docLine, "*/")
					docLine = strings.TrimPrefix(docLine, "*")
					docLine = strings.TrimPrefix(docLine, "//")
					docClean := strings.TrimSpace(docLine)
					if docClean != "" && !strings.HasPrefix(mTrim, "// MARK:") {
						memberDoc = append(memberDoc, docClean)
					}
					bodyLines = append(bodyLines, mLine)
					i++
					continue
				}

				if strings.HasPrefix(mTrim, "@") && !strings.Contains(mTrim, "func ") && !strings.Contains(mTrim, "var ") && !strings.Contains(mTrim, "let ") {
					memberAttrs = append(memberAttrs, mTrim)
					bodyLines = append(bodyLines, mLine)
					i++
					continue
				}

				if strings.Contains(mLine, "{") {
					braceCount += strings.Count(mLine, "{")
				}
				if strings.Contains(mLine, "}") {
					braceCount -= strings.Count(mLine, "}")
					if braceCount <= 0 {
						bodyLines = append(bodyLines, mLine)
						break
					}
				}
				bodyLines = append(bodyLines, mLine)

				// Enum cases
				if kind == "enum" && strings.HasPrefix(mTrim, "case ") {
					if cm := swiftEnumCaseReg.FindStringSubmatch(mTrim); cm != nil {
						cName := cm[1]
						var assocVals []string
						if cm[2] != "" {
							for _, v := range strings.Split(cm[2], ",") {
								v = strings.TrimSpace(v)
								if v != "" {
									assocVals = append(assocVals, v)
								}
							}
						}
						rawVal := strings.TrimSpace(cm[3])
						enumCases = append(enumCases, SwiftEnumCase{
							Name:             cName,
							AssociatedValues: assocVals,
							RawValue:         rawVal,
							Doc:              strings.Join(memberDoc, "\n"),
						})
					}
					memberDoc = nil
					memberAttrs = nil
					i++
					continue
				}

				// Methods
				if fm := swiftFuncHeader.FindStringSubmatch(mTrim); fm != nil {
					fVis := fm[1]
					fMod := fm[2]
					fName := fm[3]
					fParamsStr := fm[4]
					fAsync := fm[5] == "async"
					fThrows := fm[6] == "throws"
					fRetType := strings.TrimSpace(fm[7])

					isStatic := fMod == "static"
					isClass := fMod == "class"
					isMutating := fMod == "mutating"

					var methodBodyLines []string
					methodBraces := 0
					if strings.Contains(mTrim, "{") {
						methodBraces = strings.Count(mTrim, "{") - strings.Count(mTrim, "}")
					}
					methodBodyLines = append(methodBodyLines, mLine)

					if methodBraces > 0 {
						for i+1 < len(lines) {
							i++
							mbLine := lines[i].text
							methodBodyLines = append(methodBodyLines, mbLine)
							bodyLines = append(bodyLines, mbLine)

							if strings.Contains(mbLine, "{") {
								methodBraces += strings.Count(mbLine, "{")
								braceCount += strings.Count(mbLine, "{")
							}
							if strings.Contains(mbLine, "}") {
								methodBraces -= strings.Count(mbLine, "}")
								braceCount -= strings.Count(mbLine, "}")
								if methodBraces <= 0 {
									break
								}
							}
						}
					}

					fullMethodBody := strings.Join(methodBodyLines, "\n")
					methods = append(methods, SwiftMethod{
						Name:          fName,
						Visibility:    fVis,
						IsStatic:      isStatic,
						IsClass:       isClass,
						IsMutating:    isMutating,
						IsAsync:       fAsync,
						Throws:        fThrows,
						ReturnType:    fRetType,
						Params:        parseSwiftParams(fParamsStr),
						Attributes:    memberAttrs,
						Doc:           strings.Join(memberDoc, "\n"),
						BodySource:    fullMethodBody,
						CalledMethods: extractSwiftCalls(fullMethodBody),
					})

					memberDoc = nil
					memberAttrs = nil
					i++
					continue
				}

				// Properties (var / let)
				// Check inline attributes like @State var isPresented: Bool = false
				propLine := mTrim
				var inlineAttrs []string
				for strings.HasPrefix(propLine, "@") {
					parts := strings.SplitN(propLine, " ", 2)
					inlineAttrs = append(inlineAttrs, parts[0])
					if len(parts) > 1 {
						propLine = strings.TrimSpace(parts[1])
					} else {
						break
					}
				}

				if pm := swiftPropHeader.FindStringSubmatch(propLine); pm != nil {
					pVis := pm[1]
					pMod := pm[2]
					pLetVar := pm[3]
					pName := pm[4]
					pType := strings.TrimSpace(pm[5])
					pDefVal := strings.TrimSpace(pm[6])

					// Check if computed property (e.g. var body: some View { ... })
					if strings.Contains(pType, "{") {
						pType = strings.TrimSpace(strings.Split(pType, "{")[0])
					}
					if pName == "body" && strings.Contains(pType, "View") {
						isSwiftUIView = true
					}

					allAttrs := append(memberAttrs, inlineAttrs...)
					properties = append(properties, SwiftProperty{
						Name:         pName,
						Type:         pType,
						Visibility:   pVis,
						IsStatic:     pMod == "static" || pMod == "class",
						IsLet:        pLetVar == "let",
						IsLazy:       pMod == "lazy",
						Attributes:   allAttrs,
						Doc:          strings.Join(memberDoc, "\n"),
						DefaultValue: pDefVal,
					})

					memberDoc = nil
					memberAttrs = nil
					i++
					continue
				}

				memberDoc = nil
				memberAttrs = nil
				i++
			}

			types = append(types, SwiftTypeDecl{
				Kind:          kind,
				Name:          name,
				Visibility:    vis,
				Inheritance:   inheritance,
				Generics:      generics,
				Attributes:    pendingAnnotations,
				Doc:           strings.Join(pendingDoc, "\n"),
				Properties:    properties,
				Methods:       methods,
				EnumCases:     enumCases,
				IsSwiftUIView: isSwiftUIView,
				BodySource:    strings.Join(bodyLines, "\n"),
			})

			pendingDoc = nil
			pendingAnnotations = nil
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
				pendingAnnotations = nil
			}
		}
		i++
	}

	return types
}

func (p *SwiftParser) extractTopLevelFunctions(lines []swiftLine) []SwiftMethod {
	var funcs []SwiftMethod
	var pendingDoc []string
	var pendingAttrs []string

	i := 0
	braceDepth := 0
	for i < len(lines) {
		line := lines[i].text
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "/**") || strings.HasPrefix(trimmed, "*") || (strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "///")) {
			docLine := strings.TrimPrefix(trimmed, "///")
			docLine = strings.TrimPrefix(docLine, "/**")
			docLine = strings.TrimPrefix(docLine, "*/")
			docLine = strings.TrimPrefix(docLine, "*")
			docLine = strings.TrimPrefix(docLine, "//")
			docClean := strings.TrimSpace(docLine)
			if docClean != "" && !strings.HasPrefix(trimmed, "// MARK:") {
				pendingDoc = append(pendingDoc, docClean)
			}
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "@") && !strings.Contains(trimmed, "func ") {
			pendingAttrs = append(pendingAttrs, trimmed)
			i++
			continue
		}

		// Top-level function can only be declared when braceDepth == 0
		if braceDepth == 0 {
			if fm := swiftFuncHeader.FindStringSubmatch(trimmed); fm != nil {
				fVis := fm[1]
				fName := fm[3]
				fParamsStr := fm[4]
				fAsync := fm[5] == "async"
				fThrows := fm[6] == "throws"
				fRetType := strings.TrimSpace(fm[7])

				var bodyLines []string
				bodyLines = append(bodyLines, line)
				fnBraces := 0
				if strings.Contains(line, "{") {
					fnBraces = strings.Count(line, "{") - strings.Count(line, "}")
				}
				if fnBraces > 0 {
					for i+1 < len(lines) {
						i++
						bLine := lines[i].text
						bodyLines = append(bodyLines, bLine)
						if strings.Contains(bLine, "{") {
							fnBraces += strings.Count(bLine, "{")
						}
						if strings.Contains(bLine, "}") {
							fnBraces -= strings.Count(bLine, "}")
							if fnBraces <= 0 {
								break
							}
						}
					}
				}

				fullBody := strings.Join(bodyLines, "\n")
				funcs = append(funcs, SwiftMethod{
					Name:          fName,
					Visibility:    fVis,
					IsAsync:       fAsync,
					Throws:        fThrows,
					ReturnType:    fRetType,
					Params:        parseSwiftParams(fParamsStr),
					Attributes:    pendingAttrs,
					Doc:           strings.Join(pendingDoc, "\n"),
					BodySource:    fullBody,
					CalledMethods: extractSwiftCalls(fullBody),
				})

				pendingDoc = nil
				pendingAttrs = nil
				i++
				continue
			}
		}

		if strings.Contains(line, "{") {
			braceDepth += strings.Count(line, "{")
		}
		if strings.Contains(line, "}") {
			braceDepth -= strings.Count(line, "}")
			if braceDepth < 0 {
				braceDepth = 0
			}
		}

		pendingDoc = nil
		pendingAttrs = nil
		i++
	}

	return funcs
}

var (
	// Matches URL(string: "...") or URL(string: "/api/v1/...")
	urlInitRegex = regexp.MustCompile(`URL\s*\(\s*string:\s*["']([^"']+)["']\s*\)`)
	// Matches URLRequest(url: ...) with httpMethod = "..."
	httpMethodRegex = regexp.MustCompile(`(?:httpMethod|\.method)\s*=\s*["']([A-Z]+)["']`)
	// Matches URLSession.shared.data(from: ...), URLSession.shared.dataTask(with: ...), URLSession.shared.upload(...)
	urlSessionRegex = regexp.MustCompile(`URLSession\s*\.\s*(?:shared|[a-zA-Z0-9_]+)\s*\.\s*(dataTask|data|upload|download)\s*\(`)
)

func (p *SwiftParser) extractAPICalls(lines []swiftLine) []SwiftAPICall {
	var calls []SwiftAPICall

	for i, l := range lines {
		text := l.text
		if urlMatch := urlInitRegex.FindStringSubmatch(text); urlMatch != nil {
			rawURL := urlMatch[1]
			method := "GET"
			caller := "URLSession"

			// Check surrounding lines (look forward/backward up to 5 lines) for HTTP method or URLSession
			start := i - 3
			if start < 0 {
				start = 0
			}
			end := i + 5
			if end > len(lines) {
				end = len(lines)
			}

			window := ""
			for k := start; k < end; k++ {
				window += lines[k].text + "\n"
			}

			if mm := httpMethodRegex.FindStringSubmatch(window); mm != nil {
				method = mm[1]
			}
			if sm := urlSessionRegex.FindStringSubmatch(window); sm != nil {
				caller = fmt.Sprintf("URLSession.%s", sm[1])
			}

			// Clean path or URL
			cleanURL := rawURL
			calls = append(calls, SwiftAPICall{
				Method:     method,
				URL:        cleanURL,
				Caller:     caller,
				LineNumber: l.num,
			})
		}
	}

	return calls
}

func parseSwiftParams(s string) []SwiftParam {
	var params []SwiftParam
	s = strings.TrimSpace(s)
	if s == "" {
		return params
	}

	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		var attrs []string
		tokens := strings.Fields(p)
		var nonAttrTokens []string
		for _, tok := range tokens {
			if strings.HasPrefix(tok, "@") || tok == "inout" {
				attrs = append(attrs, tok)
			} else {
				nonAttrTokens = append(nonAttrTokens, tok)
			}
		}

		joined := strings.Join(nonAttrTokens, " ")
		colonParts := strings.SplitN(joined, ":", 2)
		if len(colonParts) == 2 {
			names := strings.Fields(strings.TrimSpace(colonParts[0]))
			typeAndDef := strings.TrimSpace(colonParts[1])
			defVal := ""
			pType := typeAndDef
			if strings.Contains(typeAndDef, "=") {
				eqParts := strings.SplitN(typeAndDef, "=", 2)
				pType = strings.TrimSpace(eqParts[0])
				defVal = strings.TrimSpace(eqParts[1])
			}

			var label, name string
			if len(names) == 2 {
				label = names[0]
				name = names[1]
			} else if len(names) == 1 {
				name = names[0]
			}

			params = append(params, SwiftParam{
				Label:        label,
				Name:         name,
				Type:         pType,
				DefaultValue: defVal,
				Attributes:   attrs,
			})
		}
	}
	return params
}

var swiftCallRegex = regexp.MustCompile(`\b([A-Za-z0-9_]+)\s*\(`)

func extractSwiftCalls(src string) []string {
	var calls []string
	seen := make(map[string]bool)
	matches := swiftCallRegex.FindAllStringSubmatch(src, -1)
	for _, m := range matches {
		name := m[1]
		if !seen[name] && name != "if" && name != "guard" && name != "for" && name != "while" && name != "switch" && name != "catch" {
			seen[name] = true
			calls = append(calls, name)
		}
	}
	sort.Strings(calls)
	return calls
}

func (p *SwiftParser) toSymbolNodes(res *SwiftFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	for _, t := range res.Types {
		payload, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}

		var deps []string
		deps = append(deps, t.Inheritance...)
		for _, prop := range t.Properties {
			if prop.Type != "" {
				deps = append(deps, prop.Type)
			}
		}
		for _, m := range t.Methods {
			if m.ReturnType != "" {
				deps = append(deps, m.ReturnType)
			}
			for _, param := range m.Params {
				deps = append(deps, param.Type)
			}
			deps = append(deps, m.CalledMethods...)
		}
		sort.Strings(deps)

		nodeType := "StructDecl"
		switch t.Kind {
		case "class":
			nodeType = "ClassDecl"
		case "protocol":
			nodeType = "ProtocolDecl"
		case "extension":
			nodeType = "ExtensionDecl"
		case "enum":
			nodeType = "EnumDecl"
		case "actor":
			nodeType = "ActorDecl"
		}
		if t.IsSwiftUIView {
			nodeType = "SwiftUIView"
		}

		meta := map[string]string{
			"kind":           t.Kind,
			"property_count": fmt.Sprintf("%d", len(t.Properties)),
			"method_count":   fmt.Sprintf("%d", len(t.Methods)),
		}
		if t.IsSwiftUIView {
			meta["is_view"] = "true"
			meta["framework"] = "swiftui"
		}
		if len(t.Inheritance) > 0 {
			meta["inheritance"] = strings.Join(t.Inheritance, ",")
		}

		vis := t.Visibility
		if vis == "" {
			vis = "internal"
		}

		node := &core.ASTSymbolNode{
			Language:          core.LangSwift,
			NodeType:          nodeType,
			Identifier:        fmt.Sprintf("%s:%s", res.FilePath, t.Name),
			Signature:         fmt.Sprintf("%s %s", t.Kind, t.Name),
			Docstring:         t.Doc,
			Visibility:        vis,
			ASTPayload:        payload,
			ASTMetadata:       meta,
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

		// Extract individual methods as sub-symbol nodes for fine-grained mutation
		for _, m := range t.Methods {
			mPayload, err := json.Marshal(m)
			if err != nil {
				continue
			}
			mVis := m.Visibility
			if mVis == "" {
				mVis = "internal"
			}
			mIdent := fmt.Sprintf("%s:%s.%s", res.FilePath, t.Name, m.Name)
			mNode := &core.ASTSymbolNode{
				Language:          core.LangSwift,
				NodeType:          "MethodDeclaration",
				Identifier:        mIdent,
				Signature:         fmt.Sprintf("func %s(%s)", m.Name, formatSwiftParams(m.Params)),
				Docstring:         m.Doc,
				Visibility:        mVis,
				ASTPayload:        mPayload,
				ASTMetadata:       map[string]string{"parent_type": t.Name},
				LocalDependencies: m.CalledMethods,
				Dependencies:      m.CalledMethods,
				Lineage:           lineage,
			}
			mNodeID, err := core.HashASTSymbolNode(mNode)
			if err == nil {
				mNode.NodeID = mNodeID
				nodes = append(nodes, mNode)
			}
		}
	}

	// Top-level functions
	for _, fn := range res.TopLevelFunctions {
		payload, err := json.Marshal(fn)
		if err != nil {
			continue
		}
		vis := fn.Visibility
		if vis == "" {
			vis = "internal"
		}
		node := &core.ASTSymbolNode{
			Language:          core.LangSwift,
			NodeType:          "FunctionDecl",
			Identifier:        fmt.Sprintf("%s:%s", res.FilePath, fn.Name),
			Signature:         fmt.Sprintf("func %s(%s)", fn.Name, formatSwiftParams(fn.Params)),
			Docstring:         fn.Doc,
			Visibility:        vis,
			ASTPayload:        payload,
			ASTMetadata:       map[string]string{"is_top_level": "true"},
			LocalDependencies: fn.CalledMethods,
			Dependencies:      fn.CalledMethods,
			Lineage:           lineage,
		}
		nodeID, err := core.HashASTSymbolNode(node)
		if err == nil {
			node.NodeID = nodeID
			nodes = append(nodes, node)
		}
	}

	// API Client Calls (URLSession -> CONSUMES_API)
	for _, api := range res.APICalls {
		payload, err := json.Marshal(api)
		if err != nil {
			continue
		}
		node := &core.ASTSymbolNode{
			Language:   core.LangSwift,
			NodeType:   "ApiClientCall",
			Identifier: fmt.Sprintf("%s:%s %s", res.FilePath, api.Method, api.URL),
			Signature:  fmt.Sprintf("%s %s", api.Method, api.URL),
			ASTPayload: payload,
			ASTMetadata: map[string]string{
				"caller":       api.Caller,
				"url":          api.URL,
				"http_method":  api.Method,
				"consumes_api": "true",
			},
			LocalDependencies: nil,
			Dependencies:      nil,
			Lineage:           lineage,
		}
		nodeID, err := core.HashASTSymbolNode(node)
		if err == nil {
			node.NodeID = nodeID
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

func formatSwiftParams(params []SwiftParam) string {
	var list []string
	for _, p := range params {
		lbl := ""
		if p.Label != "" && p.Label != p.Name {
			lbl = p.Label + " "
		}
		list = append(list, fmt.Sprintf("%s%s: %s", lbl, p.Name, p.Type))
	}
	return strings.Join(list, ", ")
}

// BuildComponentNode bundles parsed Swift symbols into a core.ComponentNode.
func (p *SwiftParser) BuildComponentNode(
	res *SwiftFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = "swift-module"
	}
	if !compType.IsValid() {
		compType = core.CompFrontend
	}

	symbolIDs := make([]string, 0, len(res.AllSymbols))
	for _, sym := range res.AllSymbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	metadata := map[string]string{
		"file_path":      res.FilePath,
		"type_count":     fmt.Sprintf("%d", len(res.Types)),
		"symbol_count":   fmt.Sprintf("%d", len(res.AllSymbols)),
		"api_call_count": fmt.Sprintf("%d", len(res.APICalls)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangSwift,
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
