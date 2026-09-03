package php

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

// PHPParam represents a parameter in a PHP function or method.
type PHPParam struct {
	Name         string `json:"name"`
	Type         string `json:"type,omitempty"`
	DefaultValue string `json:"default_value,omitempty"`
	IsVariadic   bool   `json:"is_variadic,omitempty"`
	IsPromoted   bool   `json:"is_promoted,omitempty"`
	Visibility   string `json:"visibility,omitempty"`
}

// PHPField represents a class property in PHP.
type PHPField struct {
	Name         string   `json:"name"`
	Type         string   `json:"type,omitempty"`
	Visibility   string   `json:"visibility"` // public, protected, private
	IsStatic     bool     `json:"is_static,omitempty"`
	IsReadonly   bool     `json:"is_readonly,omitempty"`
	Attributes   []string `json:"attributes,omitempty"`
	Doc          string   `json:"doc,omitempty"`
	DefaultValue string   `json:"default_value,omitempty"`
}

// PHPMethod represents a method or function in PHP.
type PHPMethod struct {
	Name          string     `json:"name"`
	Visibility    string     `json:"visibility"` // public, protected, private
	IsStatic      bool       `json:"is_static,omitempty"`
	IsAbstract    bool       `json:"is_abstract,omitempty"`
	IsFinal       bool       `json:"is_final,omitempty"`
	Params        []PHPParam `json:"params"`
	ReturnType    string     `json:"return_type,omitempty"`
	Attributes    []string   `json:"attributes,omitempty"`
	Doc           string     `json:"doc,omitempty"`
	RouteMethod   string     `json:"route_method,omitempty"`
	RoutePath     string     `json:"route_path,omitempty"`
	BodySource    string     `json:"body_source,omitempty"`
	CalledMethods []string   `json:"called_methods,omitempty"`
}

// PHPEnumCase represents a case in a PHP 8.1+ enum.
type PHPEnumCase struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	Doc   string `json:"doc,omitempty"`
}

// PHPClass represents a PHP class, interface, enum, or trait.
type PHPClass struct {
	Kind         string        `json:"kind"` // class, interface, enum, trait
	Name         string        `json:"name"`
	Namespace    string        `json:"namespace,omitempty"`
	Visibility   string        `json:"visibility,omitempty"` // public, abstract, final, readonly
	Extends      string        `json:"extends,omitempty"`
	Implements   []string      `json:"implements,omitempty"`
	TraitsUsed   []string      `json:"traits_used,omitempty"`
	Attributes   []string      `json:"attributes,omitempty"`
	Doc          string        `json:"doc,omitempty"`
	Fields       []PHPField    `json:"fields,omitempty"`
	Methods      []PHPMethod   `json:"methods,omitempty"`
	EnumCases    []PHPEnumCase `json:"enum_cases,omitempty"`
	BackedType   string        `json:"backed_type,omitempty"`
	IsController bool          `json:"is_controller"`
	BasePath     string        `json:"base_path,omitempty"`
}

// PHPRouteBinding represents an HTTP endpoint discovered in PHP (Symfony attributes or Laravel routes).
type PHPRouteBinding struct {
	Method      string `json:"method"`       // GET, POST, PUT, PATCH, DELETE, ANY
	Path        string `json:"path"`         // /api/v1/users, /users/{id}
	HandlerName string `json:"handler_name"` // UserController::index or UserController@index
	Framework   string `json:"framework"`    // symfony, laravel, slim
	LineNumber  int    `json:"line_number"`
}

// PHPFileResult contains all extracted symbols and metadata from a PHP source file.
type PHPFileResult struct {
	FilePath   string                `json:"file_path"`
	Namespace  string                `json:"namespace"`
	Uses       []string              `json:"uses"`
	Classes    []PHPClass            `json:"classes"`
	Functions  []PHPMethod           `json:"functions"`
	Routes     []PHPRouteBinding     `json:"routes"`
	AllSymbols []*core.ASTSymbolNode `json:"all_symbols"`
}

// PHPParser provides pure-Go PHP AST parsing, symbol extraction, and framework metadata extraction.
type PHPParser struct{}

// NewPHPParser creates a new PHPParser instance.
func NewPHPParser() *PHPParser {
	return &PHPParser{}
}

// ParseSource parses PHP source bytes and extracts all symbols.
func (p *PHPParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*PHPFileResult, error) {
	if filename == "" {
		filename = "index.php"
	}

	result := &PHPFileResult{
		FilePath: filename,
	}

	lines := splitPHPLines(src)

	// 1. Extract Namespace
	result.Namespace = p.extractNamespace(lines)

	// 2. Extract Use Statements
	result.Uses = p.extractUses(lines)

	// 3. Extract Classes, Interfaces, Enums, Traits, and Functions
	result.Classes, result.Functions = p.extractDefinitions(lines, result.Namespace)

	// 4. Extract Routes (Laravel & Symfony attribute routes)
	result.Routes = p.extractRoutes(lines, result.Classes, result.Namespace)

	// Convert all extracted entities to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a PHP source file from disk.
func (p *PHPParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*PHPFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .php files in a directory.
func (p *PHPParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*PHPFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*PHPFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".php" {
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

type phpLineInfo struct {
	num  int
	text string
}

func splitPHPLines(src []byte) []phpLineInfo {
	var lines []phpLineInfo
	scanner := bufio.NewScanner(bytes.NewReader(src))
	num := 1
	for scanner.Scan() {
		lines = append(lines, phpLineInfo{
			num:  num,
			text: scanner.Text(),
		})
		num++
	}
	return lines
}

var (
	phpNamespaceRegex   = regexp.MustCompile(`^namespace\s+([A-Za-z0-9_\\]+)\s*;`)
	phpUseRegex         = regexp.MustCompile(`^use\s+([A-Za-z0-9_\\]+)(?:\s+as\s+([A-Za-z0-9_]+))?\s*;`)
	phpAttrRegex        = regexp.MustCompile(`^#\[(.*)\]`)
	phpClassRegex       = regexp.MustCompile(`^(?:(abstract|final|readonly)\s+)?(class|interface|trait|enum)\s+([A-Za-z0-9_]+)(?:\s*:\s*([A-Za-z0-9_]+))?(?:\s+extends\s+([A-Za-z0-9_\\]+))?(?:\s+implements\s+([A-Za-z0-9_\\,\s]+))?`)
	phpTraitUseRegex    = regexp.MustCompile(`^\s*use\s+([A-Za-z0-9_\\,\s]+)\s*;`)
	phpEnumCaseRegex    = regexp.MustCompile(`^\s*case\s+([A-Za-z0-9_]+)(?:\s*=\s*([^;]+))?\s*;`)
	phpFieldRegex       = regexp.MustCompile(`^\s*(public|protected|private)?(?:\s+(static))?(?:\s+(readonly))?(?:\s+([A-Za-z0-9_?\\|]+))?\s+\$([a-zA-Z0-9_]+)(?:\s*=\s*([^;]+))?\s*;`)
	phpMethodStartRegex = regexp.MustCompile(`^(?:(public|protected|private)\s+)?(?:(static)\s+)?(?:(abstract|final)\s+)?function\s+([a-zA-Z0-9_]+)\s*\(`)
	phpFullMethodRegex  = regexp.MustCompile(`(?s)^\s*(?:(public|protected|private)\s+)?(?:(static)\s+)?(?:(abstract|final)\s+)?function\s+([a-zA-Z0-9_]+)\s*\((.*?)\)(?:\s*:\s*([A-Za-z0-9_?\\|]+))?`)

	// Symfony route attribute regex
	symfonyRouteAttrRegex = regexp.MustCompile(`Route\(\s*['"]([^'"]*)['"](?:\s*,\s*(?:name:\s*['"][^'"]+['"]\s*,\s*)?methods:\s*\[([^\]]+)\])?`)

	// Laravel route facade regex
	laravelRouteRegex    = regexp.MustCompile(`Route::(get|post|put|patch|delete|any|match)\s*\(\s*['"]([^'"]+)['"]\s*,\s*(?:\[\s*([A-Za-z0-9_\\]+)::class\s*,\s*['"]([a-zA-Z0-9_]+)['"]\s*\]|['"]([A-Za-z0-9_\\]+)(?:@([a-zA-Z0-9_]+))?['"]|([a-zA-Z0-9_]+))\s*\)`)
	laravelResourceRegex = regexp.MustCompile(`Route::(?:apiResource|resource)\s*\(\s*['"]([^'"]+)['"]\s*,\s*([A-Za-z0-9_\\]+)::class\s*\)`)
)

func (p *PHPParser) extractNamespace(lines []phpLineInfo) string {
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := phpNamespaceRegex.FindStringSubmatch(trimmed); m != nil {
			return m[1]
		}
	}
	return ""
}

func (p *PHPParser) extractUses(lines []phpLineInfo) []string {
	var uses []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if phpClassRegex.MatchString(trimmed) {
			break
		}
		if m := phpUseRegex.FindStringSubmatch(trimmed); m != nil {
			uses = append(uses, m[1])
		}
	}
	return uses
}

func collectMethodSignature(lines []phpLineInfo, startIdx int) (string, int) {
	var parts []string
	i := startIdx
	parenDepth := 0
	hasSeenParen := false

	for i < len(lines) {
		line := lines[i].text
		parts = append(parts, line)
		openP := strings.Count(line, "(")
		closeP := strings.Count(line, ")")
		if openP > 0 {
			hasSeenParen = true
		}
		parenDepth += (openP - closeP)
		if hasSeenParen && parenDepth <= 0 {
			break
		}
		i++
	}
	return strings.TrimSpace(strings.Join(parts, " ")), i
}

func (p *PHPParser) extractDefinitions(lines []phpLineInfo, currentNamespace string) ([]PHPClass, []PHPMethod) {
	var classes []PHPClass
	var functions []PHPMethod

	var pendingDoc []string
	var pendingAttrs []string
	i := 0

	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		// Docblocks and comments
		if strings.HasPrefix(trimmed, "/**") || strings.HasPrefix(trimmed, "/*") {
			var docLines []string
			for i < len(lines) {
				t := strings.TrimSpace(lines[i].text)
				clean := strings.TrimPrefix(t, "/**")
				clean = strings.TrimPrefix(clean, "/*")
				clean = strings.TrimSuffix(clean, "*/")
				clean = strings.TrimPrefix(clean, "*")
				clean = strings.TrimSpace(clean)
				if clean != "" {
					docLines = append(docLines, clean)
				}
				if strings.Contains(t, "*/") {
					i++
					break
				}
				i++
			}
			pendingDoc = docLines
			continue
		} else if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			if !strings.HasPrefix(trimmed, "#[") {
				c := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "//"), "#"))
				if c != "" {
					pendingDoc = append(pendingDoc, c)
				}
				i++
				continue
			}
		}

		if trimmed == "" || trimmed == "<?php" || trimmed == "?>" {
			i++
			continue
		}

		// Check PHP 8 Attributes: #[Route(...)]
		if m := phpAttrRegex.FindStringSubmatch(trimmed); m != nil {
			pendingAttrs = append(pendingAttrs, m[1])
			i++
			continue
		}

		// Check Class / Interface / Enum / Trait
		if m := phpClassRegex.FindStringSubmatch(trimmed); m != nil {
			mod := m[1]
			kind := m[2]
			name := m[3]
			backedType := m[4]
			extends := m[5]
			implementsRaw := m[6]

			docStr := strings.Join(pendingDoc, "\n")
			attrs := pendingAttrs
			pendingDoc = nil
			pendingAttrs = nil

			var impls []string
			if implementsRaw != "" {
				for _, imp := range strings.Split(implementsRaw, ",") {
					impTrim := strings.TrimSpace(imp)
					if impTrim != "" {
						impls = append(impls, impTrim)
					}
				}
			}

			cls, nextI := p.parseClassBody(lines, i, kind, name, currentNamespace, mod, extends, backedType, impls, attrs, docStr)
			classes = append(classes, cls)
			i = nextI
			continue
		}

		// Check Top-level function
		if phpMethodStartRegex.MatchString(trimmed) {
			sig, sigEndIdx := collectMethodSignature(lines, i)
			if m := phpFullMethodRegex.FindStringSubmatch(sig); m != nil {
				docStr := strings.Join(pendingDoc, "\n")
				attrs := pendingAttrs
				pendingDoc = nil
				pendingAttrs = nil

				fn, nextI := p.parseMethodBody(lines, i, sigEndIdx, m, attrs, docStr)
				functions = append(functions, fn)
				i = nextI
				continue
			}
		}

		pendingDoc = nil
		pendingAttrs = nil
		i++
	}

	return classes, functions
}

func (p *PHPParser) parseClassBody(
	lines []phpLineInfo,
	startIdx int,
	kind, name, namespace, modifier, extends, backedType string,
	implements, attrs []string,
	doc string,
) (PHPClass, int) {
	cls := PHPClass{
		Kind:         kind,
		Name:         name,
		Namespace:    namespace,
		Visibility:   modifier,
		Extends:      extends,
		Implements:   implements,
		Attributes:   attrs,
		Doc:          doc,
		BackedType:   backedType,
		IsController: strings.HasSuffix(name, "Controller") || strings.Contains(extends, "Controller"),
	}

	// Check if class has base route attribute: #[Route('/api/v1')]
	for _, attr := range attrs {
		if m := symfonyRouteAttrRegex.FindStringSubmatch(attr); m != nil {
			cls.BasePath = m[1]
		}
	}

	var pendingDoc []string
	var pendingAttrs []string

	i := startIdx
	// Find the opening brace of class
	for i < len(lines) && !strings.Contains(lines[i].text, "{") {
		i++
	}

	braceDepth := 0
	for i < len(lines) {
		lineText := lines[i].text
		trimmed := strings.TrimSpace(lineText)

		if strings.HasPrefix(trimmed, "/**") || strings.HasPrefix(trimmed, "/*") {
			var docLines []string
			for i < len(lines) {
				t := strings.TrimSpace(lines[i].text)
				clean := strings.TrimPrefix(t, "/**")
				clean = strings.TrimPrefix(clean, "/*")
				clean = strings.TrimSuffix(clean, "*/")
				clean = strings.TrimPrefix(clean, "*")
				clean = strings.TrimSpace(clean)
				if clean != "" {
					docLines = append(docLines, clean)
				}
				if strings.Contains(t, "*/") {
					i++
					break
				}
				i++
			}
			pendingDoc = docLines
			continue
		} else if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			if !strings.HasPrefix(trimmed, "#[") {
				c := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "//"), "#"))
				if c != "" {
					pendingDoc = append(pendingDoc, c)
				}
				i++
				continue
			}
		}

		if m := phpAttrRegex.FindStringSubmatch(trimmed); m != nil {
			pendingAttrs = append(pendingAttrs, m[1])
			i++
			continue
		}

		// Check for methods BEFORE general brace counting
		if phpMethodStartRegex.MatchString(trimmed) && braceDepth >= 1 {
			sig, sigEndIdx := collectMethodSignature(lines, i)
			if m := phpFullMethodRegex.FindStringSubmatch(sig); m != nil {
				docStr := strings.Join(pendingDoc, "\n")
				attrs := pendingAttrs
				pendingDoc = nil
				pendingAttrs = nil

				method, nextI := p.parseMethodBody(lines, i, sigEndIdx, m, attrs, docStr)
				// Check if method has route attribute
				for _, attr := range attrs {
					if rm := symfonyRouteAttrRegex.FindStringSubmatch(attr); rm != nil {
						method.RoutePath = rm[1]
						if rm[2] != "" {
							methodsRaw := rm[2]
							var parsedMethods []string
							for _, pm := range strings.Split(methodsRaw, ",") {
								pmTrim := strings.ToUpper(strings.Trim(strings.TrimSpace(pm), "'\""))
								if pmTrim != "" {
									parsedMethods = append(parsedMethods, pmTrim)
								}
							}
							method.RouteMethod = strings.Join(parsedMethods, ",")
						} else {
							method.RouteMethod = "GET"
						}
					}
				}

				cls.Methods = append(cls.Methods, method)
				i = nextI
				continue
			}
		}

		openCount := strings.Count(lineText, "{")
		closeCount := strings.Count(lineText, "}")
		braceDepth += (openCount - closeCount)

		if braceDepth <= 0 && i > startIdx {
			i++
			break
		}

		// Trait use inside class
		if m := phpTraitUseRegex.FindStringSubmatch(trimmed); m != nil && braceDepth == 1 {
			for _, tr := range strings.Split(m[1], ",") {
				trTrim := strings.TrimSpace(tr)
				if trTrim != "" {
					cls.TraitsUsed = append(cls.TraitsUsed, trTrim)
				}
			}
			pendingDoc = nil
			pendingAttrs = nil
			i++
			continue
		}

		// Enum cases
		if kind == "enum" && braceDepth == 1 {
			if m := phpEnumCaseRegex.FindStringSubmatch(trimmed); m != nil {
				caseName := m[1]
				val := strings.Trim(strings.TrimSpace(m[2]), "'\"")
				cls.EnumCases = append(cls.EnumCases, PHPEnumCase{
					Name:  caseName,
					Value: val,
					Doc:   strings.Join(pendingDoc, "\n"),
				})
				pendingDoc = nil
				pendingAttrs = nil
				i++
				continue
			}
		}

		// Fields
		if m := phpFieldRegex.FindStringSubmatch(trimmed); m != nil && braceDepth == 1 && !strings.Contains(trimmed, "function") {
			vis := m[1]
			if vis == "" {
				vis = "public"
			}
			isStatic := m[2] != ""
			isReadonly := m[3] != ""
			fieldType := m[4]
			fieldName := m[5]
			defVal := strings.TrimSpace(m[6])

			cls.Fields = append(cls.Fields, PHPField{
				Name:         fieldName,
				Type:         fieldType,
				Visibility:   vis,
				IsStatic:     isStatic,
				IsReadonly:   isReadonly,
				Attributes:   pendingAttrs,
				Doc:          strings.Join(pendingDoc, "\n"),
				DefaultValue: defVal,
			})
			pendingDoc = nil
			pendingAttrs = nil
			i++
			continue
		}

		pendingDoc = nil
		pendingAttrs = nil
		i++
	}

	return cls, i
}

func (p *PHPParser) parseMethodBody(
	lines []phpLineInfo,
	startIdx, sigEndIdx int,
	m []string,
	attrs []string,
	doc string,
) (PHPMethod, int) {
	vis := m[1]
	if vis == "" {
		vis = "public"
	}
	isStatic := m[2] != ""
	modifier := m[3]
	isAbstract := modifier == "abstract"
	isFinal := modifier == "final"
	methodName := m[4]
	paramsRaw := m[5]
	returnType := m[6]

	params := parsePHPParams(paramsRaw)

	// Check if this is an interface/abstract method ending in semicolon
	isSemi := false
	for k := startIdx; k <= sigEndIdx && k < len(lines); k++ {
		if strings.Contains(lines[k].text, ";") {
			isSemi = true
			break
		}
	}

	if isAbstract || isSemi {
		return PHPMethod{
			Name:       methodName,
			Visibility: vis,
			IsStatic:   isStatic,
			IsAbstract: isAbstract,
			IsFinal:    isFinal,
			Params:     params,
			ReturnType: returnType,
			Attributes: attrs,
			Doc:        doc,
			BodySource: "",
		}, sigEndIdx + 1
	}

	var bodyLines []string
	for k := startIdx; k <= sigEndIdx && k < len(lines); k++ {
		bodyLines = append(bodyLines, lines[k].text)
	}

	i := sigEndIdx
	for i < len(lines) && !strings.Contains(lines[i].text, "{") {
		i++
		if i < len(lines) {
			bodyLines = append(bodyLines, lines[i].text)
		}
	}

	braceDepth := 0
	hasOpenedBrace := false
	for i < len(lines) {
		lineText := lines[i].text
		if i > sigEndIdx {
			bodyLines = append(bodyLines, lineText)
		}

		openCount := strings.Count(lineText, "{")
		closeCount := strings.Count(lineText, "}")
		if openCount > 0 {
			hasOpenedBrace = true
		}
		braceDepth += (openCount - closeCount)

		if hasOpenedBrace && braceDepth <= 0 {
			i++
			break
		}
		i++
	}

	fullBody := strings.Join(bodyLines, "\n")
	calledMethods := extractPHPCalls(fullBody)

	return PHPMethod{
		Name:          methodName,
		Visibility:    vis,
		IsStatic:      isStatic,
		IsAbstract:    isAbstract,
		IsFinal:       isFinal,
		Params:        params,
		ReturnType:    returnType,
		Attributes:    attrs,
		Doc:           doc,
		BodySource:    fullBody,
		CalledMethods: calledMethods,
	}, i
}

func parsePHPParams(paramsStr string) []PHPParam {
	var params []PHPParam
	paramsStr = strings.TrimSpace(paramsStr)
	if paramsStr == "" {
		return params
	}

	for _, p := range strings.Split(paramsStr, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		param := PHPParam{}

		// Check promoted properties visibility: private string $id
		if strings.HasPrefix(p, "public ") || strings.HasPrefix(p, "protected ") || strings.HasPrefix(p, "private ") {
			parts := strings.Fields(p)
			param.Visibility = parts[0]
			param.IsPromoted = true
			p = strings.TrimSpace(strings.TrimPrefix(p, parts[0]))
		}

		if strings.Contains(p, "=") {
			parts := strings.SplitN(p, "=", 2)
			param.DefaultValue = strings.TrimSpace(parts[1])
			p = strings.TrimSpace(parts[0])
		}

		if strings.Contains(p, "...") {
			param.IsVariadic = true
			p = strings.ReplaceAll(p, "...", "")
		}

		tokens := strings.Fields(p)
		if len(tokens) == 1 {
			param.Name = strings.TrimPrefix(tokens[0], "$")
		} else if len(tokens) > 1 {
			param.Type = tokens[0]
			param.Name = strings.TrimPrefix(tokens[1], "$")
		}

		params = append(params, param)
	}

	return params
}

func (p *PHPParser) extractRoutes(lines []phpLineInfo, classes []PHPClass, namespace string) []PHPRouteBinding {
	var routes []PHPRouteBinding

	// 1. Symfony Attribute routes from classes and methods
	for _, cls := range classes {
		basePath := cls.BasePath
		for _, m := range cls.Methods {
			if m.RouteMethod != "" || m.RoutePath != "" {
				fullPath := combinePHPPaths(basePath, m.RoutePath)
				methods := strings.Split(m.RouteMethod, ",")
				for _, httpMethod := range methods {
					routes = append(routes, PHPRouteBinding{
						Method:      httpMethod,
						Path:        fullPath,
						HandlerName: fmt.Sprintf("%s::%s", cls.Name, m.Name),
						Framework:   "symfony",
					})
				}
			}
		}
	}

	// 2. Laravel Route facade definitions
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)

		// Route::apiResource('users', UserController::class);
		if m := laravelResourceRegex.FindStringSubmatch(trimmed); m != nil {
			resName := m[1]
			controller := m[2]
			routes = append(routes, PHPRouteBinding{
				Method:      "ANY",
				Path:        "/" + resName,
				HandlerName: controller,
				Framework:   "laravel",
				LineNumber:  l.num,
			})
			continue
		}

		// Route::get('/users', [UserController::class, 'index']);
		if m := laravelRouteRegex.FindStringSubmatch(trimmed); m != nil {
			httpMethod := strings.ToUpper(m[1])
			path := m[2]
			controller1 := m[3]
			action1 := m[4]
			controller2 := m[5]
			action2 := m[6]
			handlerFunc := m[7]

			handler := ""
			if controller1 != "" && action1 != "" {
				handler = fmt.Sprintf("%s::%s", controller1, action1)
			} else if controller2 != "" {
				if action2 != "" {
					handler = fmt.Sprintf("%s@%s", controller2, action2)
				} else {
					handler = controller2
				}
			} else if handlerFunc != "" {
				handler = handlerFunc
			}

			routes = append(routes, PHPRouteBinding{
				Method:      httpMethod,
				Path:        path,
				HandlerName: handler,
				Framework:   "laravel",
				LineNumber:  l.num,
			})
			continue
		}
	}

	return routes
}

func combinePHPPaths(base, path string) string {
	cleanBase := strings.Trim(base, "/")
	cleanPath := strings.Trim(path, "/")
	if cleanBase == "" {
		return "/" + cleanPath
	}
	if cleanPath == "" {
		return "/" + cleanBase
	}
	return "/" + cleanBase + "/" + cleanPath
}

var phpCallRegex = regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*\(`)

func extractPHPCalls(src string) []string {
	var calls []string
	seen := make(map[string]bool)
	matches := phpCallRegex.FindAllStringSubmatch(src, -1)
	keywords := map[string]bool{
		"function": true, "class": true, "interface": true, "trait": true, "enum": true,
		"if": true, "elseif": true, "else": true, "while": true, "for": true, "foreach": true,
		"switch": true, "case": true, "return": true, "throw": true, "catch": true, "finally": true,
		"use": true, "namespace": true, "new": true, "echo": true, "print": true,
	}

	for _, m := range matches {
		name := m[1]
		if !seen[name] && !keywords[name] {
			seen[name] = true
			calls = append(calls, name)
		}
	}
	sort.Strings(calls)
	return calls
}

func (p *PHPParser) toSymbolNodes(res *PHPFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Classes / Interfaces / Enums / Traits
	for _, cls := range res.Classes {
		node, err := ClassToASTSymbolNode(cls, res.Namespace, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Standalone Functions
	for _, fn := range res.Functions {
		node, err := FunctionToASTSymbolNode(fn, res.Namespace, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Routes
	for _, r := range res.Routes {
		node, err := RouteToASTSymbolNode(r, res.Namespace, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// ClassToASTSymbolNode converts a PHPClass to an ASTSymbolNode.
func ClassToASTSymbolNode(cls PHPClass, namespace string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(cls)
	if err != nil {
		return nil, fmt.Errorf("marshaling PHP class %s: %w", cls.Name, err)
	}

	var deps []string
	if cls.Extends != "" {
		deps = append(deps, cls.Extends)
	}
	deps = append(deps, cls.Implements...)
	deps = append(deps, cls.TraitsUsed...)

	nodeType := "ClassDef"
	switch cls.Kind {
	case "interface":
		nodeType = "InterfaceDef"
	case "enum":
		nodeType = "EnumDef"
	case "trait":
		nodeType = "TraitDef"
	}

	meta := map[string]string{
		"class_name":    cls.Name,
		"kind":          cls.Kind,
		"namespace":     namespace,
		"extends":       cls.Extends,
		"method_count":  fmt.Sprintf("%d", len(cls.Methods)),
		"field_count":   fmt.Sprintf("%d", len(cls.Fields)),
		"is_controller": fmt.Sprintf("%t", cls.IsController),
	}
	if cls.BasePath != "" {
		meta["route_base"] = cls.BasePath
	}

	id := cls.Name
	if namespace != "" {
		id = fmt.Sprintf("%s\\%s", namespace, cls.Name)
	}

	sig := fmt.Sprintf("%s %s", cls.Kind, cls.Name)
	if cls.Extends != "" {
		sig = fmt.Sprintf("%s extends %s", sig, cls.Extends)
	}
	if len(cls.Implements) > 0 {
		sig = fmt.Sprintf("%s implements %s", sig, strings.Join(cls.Implements, ", "))
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangPHP,
		NodeType:          nodeType,
		Identifier:        id,
		Signature:         sig,
		Docstring:         cls.Doc,
		Visibility:        "public",
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
	return node, nil
}

// FunctionToASTSymbolNode converts a standalone PHP function to an ASTSymbolNode.
func FunctionToASTSymbolNode(fn PHPMethod, namespace string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(fn)
	if err != nil {
		return nil, fmt.Errorf("marshaling PHP function %s: %w", fn.Name, err)
	}

	id := fn.Name
	if namespace != "" {
		id = fmt.Sprintf("%s\\%s", namespace, fn.Name)
	}

	sig := fmt.Sprintf("function %s()", fn.Name)
	if fn.ReturnType != "" {
		sig = fmt.Sprintf("%s: %s", sig, fn.ReturnType)
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangPHP,
		NodeType:          "FunctionDef",
		Identifier:        id,
		Signature:         sig,
		Docstring:         fn.Doc,
		Visibility:        "public",
		ASTPayload:        payload,
		ASTMetadata:       map[string]string{"return_type": fn.ReturnType},
		LocalDependencies: fn.CalledMethods,
		Dependencies:      fn.CalledMethods,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// RouteToASTSymbolNode converts a PHPRouteBinding to an ASTSymbolNode.
func RouteToASTSymbolNode(r PHPRouteBinding, namespace string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("marshaling PHP route %s: %w", r.Path, err)
	}

	meta := map[string]string{
		"framework":   r.Framework,
		"handler":     r.HandlerName,
		"route_path":  r.Path,
		"http_method": r.Method,
	}

	var deps []string
	if r.HandlerName != "" {
		parts := strings.Split(r.HandlerName, "::")
		if len(parts) > 0 {
			deps = append(deps, parts[0])
		}
	}

	id := fmt.Sprintf("%s %s", r.Method, r.Path)
	if namespace != "" {
		id = fmt.Sprintf("%s:%s", namespace, id)
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangPHP,
		NodeType:          "RouteBinding",
		Identifier:        id,
		Signature:         fmt.Sprintf("%s %s", r.Method, r.Path),
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
	return node, nil
}

// BuildComponentNode bundles parsed PHP symbols into a core.ComponentNode.
func (p *PHPParser) BuildComponentNode(
	res *PHPFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		if res.Namespace != "" {
			compName = res.Namespace
		} else {
			compName = strings.TrimSuffix(filepath.Base(res.FilePath), filepath.Ext(res.FilePath))
		}
	}
	if !compType.IsValid() {
		compType = core.CompService
	}

	symbolIDs := make([]string, 0, len(res.AllSymbols))
	for _, sym := range res.AllSymbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	metadata := map[string]string{
		"file_path":    res.FilePath,
		"namespace":    res.Namespace,
		"symbol_count": fmt.Sprintf("%d", len(res.AllSymbols)),
		"class_count":  fmt.Sprintf("%d", len(res.Classes)),
		"route_count":  fmt.Sprintf("%d", len(res.Routes)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangPHP,
		SymbolNodes: symbolIDs,
		Metadata:    metadata,
		Lineage:     lineage,
	}

	compID, err := core.HashComponentNode(comp)
	if err != nil {
		return nil, fmt.Errorf("computing component node hash: %w", err)
	}
	comp.ComponentID = compID
	return comp, nil
}
