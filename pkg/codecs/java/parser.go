package java

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

// JavaField represents a field/member variable declaration in a Java class or record.
type JavaField struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Visibility  string   `json:"visibility,omitempty"` // public, private, protected, package-private
	IsStatic    bool     `json:"is_static"`
	IsFinal     bool     `json:"is_final"`
	Annotations []string `json:"annotations,omitempty"`
	Doc         string   `json:"doc,omitempty"`
}

// JavaParam represents a method parameter.
type JavaParam struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Annotations []string `json:"annotations,omitempty"`
}

// JavaMethod represents an extracted Java method or constructor.
type JavaMethod struct {
	Name          string      `json:"name"`
	Visibility    string      `json:"visibility,omitempty"`
	IsStatic      bool        `json:"is_static"`
	IsAbstract    bool        `json:"is_abstract"`
	IsFinal       bool        `json:"is_final"`
	ReturnType    string      `json:"return_type,omitempty"`
	Params        []JavaParam `json:"params"`
	Annotations   []string    `json:"annotations,omitempty"`
	Throws        []string    `json:"throws,omitempty"`
	Doc           string      `json:"doc,omitempty"`
	RouteMethod   string      `json:"route_method,omitempty"` // GET, POST, PUT, DELETE, PATCH
	RoutePath     string      `json:"route_path,omitempty"`   // /api/v1/users
	JPAQuery      string      `json:"jpa_query,omitempty"`    // SELECT u FROM User u
	BodySource    string      `json:"body_source,omitempty"`
	CalledMethods []string    `json:"called_methods,omitempty"`
}

// JavaClass represents an extracted Java class, interface, enum, or record.
type JavaClass struct {
	Kind           string       `json:"kind"`                 // class, interface, enum, record
	Name           string       `json:"name"`
	PackageName    string       `json:"package_name"`
	Visibility     string       `json:"visibility,omitempty"` // public, private, protected, package-private
	IsAbstract     bool         `json:"is_abstract"`
	IsFinal        bool         `json:"is_final"`
	IsStatic       bool         `json:"is_static"`
	Extends        string       `json:"extends,omitempty"`
	Implements     []string     `json:"implements,omitempty"`
	Annotations    []string     `json:"annotations,omitempty"` // e.g. @RestController, @Entity, @Table(name="users")
	Doc            string       `json:"doc,omitempty"`
	Fields         []JavaField  `json:"fields"`
	Methods        []JavaMethod `json:"methods"`
	IsController   bool         `json:"is_controller"`
	BasePath       string       `json:"base_path,omitempty"`  // from @RequestMapping
	IsEntity       bool         `json:"is_entity"`
	TableName      string       `json:"table_name,omitempty"` // from @Table(name="...")
}

// JavaRouteBinding represents a Spring Boot or JAX-RS REST route.
type JavaRouteBinding struct {
	Method      string `json:"method"`       // GET, POST, PUT, DELETE, PATCH, ANY
	Path        string `json:"path"`         // /api/v1/users
	HandlerName string `json:"handler_name"` // UserController.getUser
	Framework   string `json:"framework"`    // spring-boot, jax-rs
	LineNumber  int    `json:"line_number"`
}

// JavaFileResult contains all extracted symbols and metadata from a Java source file.
type JavaFileResult struct {
	FilePath    string              `json:"file_path"`
	PackageName string              `json:"package_name"`
	Imports     []string            `json:"imports"`
	Classes     []JavaClass         `json:"classes"`
	Routes      []JavaRouteBinding  `json:"routes"`
	AllSymbols  []*core.ASTSymbolNode `json:"all_symbols"`
}

// JavaParser extracts classes, methods, fields, Spring Boot routes, and JPA metadata from Java files.
type JavaParser struct{}

// NewJavaParser creates an initialized JavaParser instance.
func NewJavaParser() *JavaParser {
	return &JavaParser{}
}

// ParseSource parses raw Java source code bytes.
func (p *JavaParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*JavaFileResult, error) {
	if filename == "" {
		filename = "Source.java"
	}

	result := &JavaFileResult{
		FilePath: filename,
	}

	lines := splitJavaLines(src)

	// 1. Extract Package and Imports
	result.PackageName = p.extractPackage(lines)
	result.Imports = p.extractImports(lines)

	// 2. Extract Classes/Interfaces/Records
	result.Classes = p.extractClasses(lines, result.PackageName)

	// 3. Extract Spring Boot / JAX-RS Routes
	result.Routes = p.extractRoutes(result.Classes)

	// 4. Convert to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a Java source file from disk.
func (p *JavaParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*JavaFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .java files in a directory.
func (p *JavaParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*JavaFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*JavaFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".java" {
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

type javaLine struct {
	num  int
	text string
}

func splitJavaLines(src []byte) []javaLine {
	var lines []javaLine
	scanner := bufio.NewScanner(bytes.NewReader(src))
	num := 1
	for scanner.Scan() {
		lines = append(lines, javaLine{
			num:  num,
			text: scanner.Text(),
		})
		num++
	}
	return lines
}

var (
	packageRegex     = regexp.MustCompile(`^package\s+([a-zA-Z0-9_.]+)\s*;`)
	importRegex      = regexp.MustCompile(`^import\s+(?:static\s+)?([a-zA-Z0-9_.*]+)\s*;`)
	classHeaderRegex = regexp.MustCompile(`^(?:(public|protected|private)\s+)?(?:(abstract|final|static)\s+)*(class|interface|enum|record)\s+([A-Za-z0-9_]+)(?:<[^>]+>)?(?:\s+extends\s+([A-Za-z0-9_.]+))?(?:\s+implements\s+([A-Za-z0-9_.,\s]+))?`)
	methodHeaderReg  = regexp.MustCompile(`^(?:(public|protected|private)\s+)?(?:(static|final|abstract|synchronized)\s+)*(?:<[^>]+>\s+)?([A-Za-z0-9_<>\[\],\s]+?)\s+([A-Za-z0-9_]+)\s*\((.*?)\)(?:\s*throws\s+([A-Za-z0-9_,\s]+))?\s*(\{|;)`)
	tableAnnotRegex  = regexp.MustCompile(`@Table\s*\(\s*(?:name\s*=\s*)?["']([^"']*)["']`)
	reqMapRegex      = regexp.MustCompile(`@(RequestMapping|GetMapping|PostMapping|PutMapping|DeleteMapping|PatchMapping)\s*(?:\(\s*(?:(?:value|path)\s*=\s*)?["']([^"']*)["']|\s*\(\s*["']([^"']*)["']|\s*\(\s*\)|\s*$)`)
	jpaQueryRegex    = regexp.MustCompile(`@Query\s*\(\s*(?:value\s*=\s*)?["']([^"']*)["']`)
)

func (p *JavaParser) extractPackage(lines []javaLine) string {
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := packageRegex.FindStringSubmatch(trimmed); m != nil {
			return m[1]
		}
	}
	return ""
}

func (p *JavaParser) extractImports(lines []javaLine) []string {
	var imports []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := importRegex.FindStringSubmatch(trimmed); m != nil {
			imports = append(imports, m[1])
		}
	}
	return imports
}

func (p *JavaParser) extractClasses(lines []javaLine, pkgName string) []JavaClass {
	var classes []JavaClass
	var pendingDoc []string
	var pendingAnnotations []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "/**") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "//") {
			docLine := strings.TrimPrefix(trimmed, "/**")
			docLine = strings.TrimPrefix(docLine, "*/")
			docLine = strings.TrimPrefix(docLine, "*")
			docLine = strings.TrimPrefix(docLine, "//")
			docClean := strings.TrimSpace(docLine)
			if docClean != "" {
				pendingDoc = append(pendingDoc, docClean)
			}
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "@") {
			pendingAnnotations = append(pendingAnnotations, trimmed)
			i++
			continue
		}

		if m := classHeaderRegex.FindStringSubmatch(trimmed); m != nil && !strings.Contains(trimmed, "(") && !strings.Contains(trimmed, ";") {
			vis := m[1]
			kind := m[3]
			name := m[4]
			extends := m[5]
			var implements []string
			if m[6] != "" {
				for _, imp := range strings.Split(m[6], ",") {
					imp = strings.TrimSpace(imp)
					if imp != "" {
						implements = append(implements, imp)
					}
				}
			}

			isController := false
			basePath := ""
			isEntity := false
			tableName := ""

			for _, ann := range pendingAnnotations {
				if strings.Contains(ann, "@RestController") || strings.Contains(ann, "@Controller") {
					isController = true
				}
				if strings.Contains(ann, "@Entity") {
					isEntity = true
				}
				if tm := tableAnnotRegex.FindStringSubmatch(ann); tm != nil {
					tableName = tm[1]
				}
				if rm := reqMapRegex.FindStringSubmatch(ann); rm != nil {
					if rm[2] != "" {
						basePath = rm[2]
					} else if rm[3] != "" {
						basePath = rm[3]
					}
				}
			}

			// Parse fields and methods inside class body
			var fields []JavaField
			var methods []JavaMethod

			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			var memberDoc []string
			var memberAnnotations []string

			i++
			for i < len(lines) {
				mLine := strings.TrimSpace(lines[i].text)

				if strings.HasPrefix(mLine, "/**") || strings.HasPrefix(mLine, "*") || strings.HasPrefix(mLine, "//") {
					docLine := strings.TrimPrefix(mLine, "/**")
					docLine = strings.TrimPrefix(docLine, "*/")
					docLine = strings.TrimPrefix(docLine, "*")
					docLine = strings.TrimPrefix(docLine, "//")
					docClean := strings.TrimSpace(docLine)
					if docClean != "" {
						memberDoc = append(memberDoc, docClean)
					}
					i++
					continue
				}

				if strings.HasPrefix(mLine, "@") {
					memberAnnotations = append(memberAnnotations, mLine)
					i++
					continue
				}

				if strings.Contains(mLine, "{") {
					braceCount++
				}
				if strings.Contains(mLine, "}") {
					braceCount--
					if braceCount <= 0 {
						break
					}
				}

				// Check for method declaration
				if mm := methodHeaderReg.FindStringSubmatch(mLine); mm != nil && !strings.HasPrefix(mLine, "class ") && !strings.HasPrefix(mLine, "interface ") {
					mVis := mm[1]
					mRetType := strings.TrimSpace(mm[3])
					mName := mm[4]
					mParamsStr := mm[5]

					var mThrows []string
					if mm[6] != "" {
						for _, t := range strings.Split(mm[6], ",") {
							t = strings.TrimSpace(t)
							if t != "" {
								mThrows = append(mThrows, t)
							}
						}
					}

					var routeMethod, routePath string
					var jpaQuery string

					for _, ann := range memberAnnotations {
						if rm := reqMapRegex.FindStringSubmatch(ann); rm != nil {
							mType := rm[1]
							pathVal := rm[2]
							if pathVal == "" {
								pathVal = rm[3]
							}
							switch mType {
							case "GetMapping":
								routeMethod = "GET"
							case "PostMapping":
								routeMethod = "POST"
							case "PutMapping":
								routeMethod = "PUT"
							case "DeleteMapping":
								routeMethod = "DELETE"
							case "PatchMapping":
								routeMethod = "PATCH"
							case "RequestMapping":
								routeMethod = "ANY"
							}
							routePath = pathVal
						}
						if qm := jpaQueryRegex.FindStringSubmatch(ann); qm != nil {
							jpaQuery = qm[1]
						}
					}

					// Collect method body
					var bodyLines []string
					methodBraces := 0
					if strings.Contains(mLine, "{") {
						methodBraces = 1
					}
					bodyLines = append(bodyLines, mLine)

					if methodBraces > 0 {
						for i+1 < len(lines) {
							i++
							bLine := lines[i].text
							bodyLines = append(bodyLines, bLine)
							for _, ch := range bLine {
								if ch == '{' {
									methodBraces++
									braceCount++
								} else if ch == '}' {
									methodBraces--
									braceCount--
									if methodBraces <= 0 {
										break
									}
								}
							}
							if methodBraces <= 0 {
								break
							}
						}
					}

					methods = append(methods, JavaMethod{
						Name:          mName,
						Visibility:    mVis,
						ReturnType:    mRetType,
						Params:        parseJavaParams(mParamsStr),
						Annotations:   memberAnnotations,
						Throws:        mThrows,
						Doc:           strings.Join(memberDoc, "\n"),
						RouteMethod:   routeMethod,
						RoutePath:     routePath,
						JPAQuery:      jpaQuery,
						BodySource:    strings.Join(bodyLines, "\n"),
						CalledMethods: extractJavaCalls(strings.Join(bodyLines, "\n")),
					})

					memberDoc = nil
					memberAnnotations = nil
				} else if strings.HasSuffix(mLine, ";") && !strings.Contains(mLine, "(") && mLine != "" {
					// Field declaration
					fClean := strings.TrimSuffix(mLine, ";")
					parts := strings.Fields(fClean)
					if len(parts) >= 2 {
						fName := parts[len(parts)-1]
						fType := parts[len(parts)-2]
						fVis := "package-private"
						isStatic := strings.Contains(fClean, "static")
						isFinal := strings.Contains(fClean, "final")
						if strings.Contains(fClean, "public") {
							fVis = "public"
						} else if strings.Contains(fClean, "private") {
							fVis = "private"
						} else if strings.Contains(fClean, "protected") {
							fVis = "protected"
						}

						fields = append(fields, JavaField{
							Name:        fName,
							Type:        fType,
							Visibility:  fVis,
							IsStatic:    isStatic,
							IsFinal:     isFinal,
							Annotations: memberAnnotations,
							Doc:         strings.Join(memberDoc, "\n"),
						})
					}
					memberDoc = nil
					memberAnnotations = nil
				} else {
					if mLine != "" && !strings.HasPrefix(mLine, "//") {
						memberDoc = nil
						memberAnnotations = nil
					}
				}
				i++
			}

			classes = append(classes, JavaClass{
				Kind:         kind,
				Name:         name,
				PackageName:  pkgName,
				Visibility:   vis,
				Extends:      extends,
				Implements:   implements,
				Annotations:  pendingAnnotations,
				Doc:          strings.Join(pendingDoc, "\n"),
				Fields:       fields,
				Methods:      methods,
				IsController: isController,
				BasePath:     basePath,
				IsEntity:     isEntity,
				TableName:    tableName,
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

	return classes
}

func (p *JavaParser) extractRoutes(classes []JavaClass) []JavaRouteBinding {
	var routes []JavaRouteBinding

	for _, cls := range classes {
		for _, m := range cls.Methods {
			if m.RouteMethod != "" {
				fullPath := cls.BasePath
				if m.RoutePath != "" {
					if !strings.HasPrefix(m.RoutePath, "/") && fullPath != "" {
						fullPath = fullPath + "/" + m.RoutePath
					} else {
						fullPath = fullPath + m.RoutePath
					}
				}
				if fullPath == "" {
					fullPath = "/"
				}

				routes = append(routes, JavaRouteBinding{
					Method:      m.RouteMethod,
					Path:        fullPath,
					HandlerName: fmt.Sprintf("%s.%s", cls.Name, m.Name),
					Framework:   "spring-boot",
					LineNumber:  1,
				})
			}
		}
	}

	return routes
}

func parseJavaParams(s string) []JavaParam {
	var params []JavaParam
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
		tokens := strings.Fields(p)
		var annotations []string
		var typeTokens []string
		for _, tok := range tokens {
			if strings.HasPrefix(tok, "@") {
				annotations = append(annotations, tok)
			} else {
				typeTokens = append(typeTokens, tok)
			}
		}
		if len(typeTokens) >= 2 {
			pName := typeTokens[len(typeTokens)-1]
			pType := strings.Join(typeTokens[:len(typeTokens)-1], " ")
			params = append(params, JavaParam{
				Name:        pName,
				Type:        pType,
				Annotations: annotations,
			})
		} else if len(typeTokens) == 1 {
			params = append(params, JavaParam{
				Name:        typeTokens[0],
				Type:        "Object",
				Annotations: annotations,
			})
		}
	}
	return params
}

var javaCallRegex = regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*\(`)

func extractJavaCalls(src string) []string {
	var calls []string
	seen := make(map[string]bool)
	matches := javaCallRegex.FindAllStringSubmatch(src, -1)
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

func (p *JavaParser) toSymbolNodes(res *JavaFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	for _, cls := range res.Classes {
		classPayload, err := json.Marshal(cls)
		if err != nil {
			return nil, err
		}

		var deps []string
		if cls.Extends != "" {
			deps = append(deps, cls.Extends)
		}
		deps = append(deps, cls.Implements...)
		for _, f := range cls.Fields {
			deps = append(deps, f.Type)
		}
		for _, m := range cls.Methods {
			deps = append(deps, m.ReturnType)
			for _, param := range m.Params {
				deps = append(deps, param.Type)
			}
		}
		sort.Strings(deps)

		meta := map[string]string{
			"kind":         cls.Kind,
			"method_count": fmt.Sprintf("%d", len(cls.Methods)),
			"field_count":  fmt.Sprintf("%d", len(cls.Fields)),
		}
		if cls.IsController {
			meta["is_controller"] = "true"
			if cls.BasePath != "" {
				meta["base_path"] = cls.BasePath
			}
		}
		if cls.IsEntity {
			meta["is_entity"] = "true"
			if cls.TableName != "" {
				meta["table_name"] = cls.TableName
			}
		}

		ident := cls.Name
		if cls.PackageName != "" {
			ident = fmt.Sprintf("%s.%s", cls.PackageName, cls.Name)
		}

		vis := cls.Visibility
		if vis == "" {
			vis = "package-private"
		}

		node := &core.ASTSymbolNode{
			Language:          core.LangJava,
			NodeType:          "ClassOrInterfaceDeclaration",
			Identifier:        ident,
			Signature:         fmt.Sprintf("%s %s", cls.Kind, cls.Name),
			Docstring:         cls.Doc,
			Visibility:        vis,
			ASTPayload:        classPayload,
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

		// Also extract individual methods as sub-symbol nodes for fine-grained mutation
		for _, m := range cls.Methods {
			mPayload, err := json.Marshal(m)
			if err != nil {
				continue
			}

			mMeta := make(map[string]string)
			if m.RouteMethod != "" {
				mMeta["route_method"] = m.RouteMethod
				mMeta["route_path"] = m.RoutePath
			}
			if m.JPAQuery != "" {
				mMeta["jpa_query"] = m.JPAQuery
			}

			mIdent := fmt.Sprintf("%s.%s", ident, m.Name)
			mVis := m.Visibility
			if mVis == "" {
				mVis = "package-private"
			}

			mNode := &core.ASTSymbolNode{
				Language:          core.LangJava,
				NodeType:          "MethodDeclaration",
				Identifier:        mIdent,
				Signature:         fmt.Sprintf("%s %s(%s)", m.ReturnType, m.Name, formatJavaParams(m.Params)),
				Docstring:         m.Doc,
				Visibility:        mVis,
				ASTPayload:        mPayload,
				ASTMetadata:       mMeta,
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

	// Routes
	for _, r := range res.Routes {
		rPayload, err := json.Marshal(r)
		if err != nil {
			continue
		}
		rNode := &core.ASTSymbolNode{
			Language:          core.LangJava,
			NodeType:          "RouteBinding",
			Identifier:        fmt.Sprintf("%s:%s %s", res.PackageName, r.Method, r.Path),
			Signature:         fmt.Sprintf("%s %s", r.Method, r.Path),
			ASTPayload:        rPayload,
			ASTMetadata:       map[string]string{"framework": r.Framework, "handler": r.HandlerName},
			LocalDependencies: []string{r.HandlerName},
			Dependencies:      []string{r.HandlerName},
			Lineage:           lineage,
		}
		rNodeID, err := core.HashASTSymbolNode(rNode)
		if err == nil {
			rNode.NodeID = rNodeID
			nodes = append(nodes, rNode)
		}
	}

	return nodes, nil
}

func formatJavaParams(params []JavaParam) string {
	var list []string
	for _, p := range params {
		list = append(list, fmt.Sprintf("%s %s", p.Type, p.Name))
	}
	return strings.Join(list, ", ")
}

// BuildComponentNode bundles parsed Java symbols into a core.ComponentNode.
func (p *JavaParser) BuildComponentNode(
	res *JavaFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = res.PackageName
		if compName == "" {
			compName = "java-service"
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
		"package_name": res.PackageName,
		"class_count":  fmt.Sprintf("%d", len(res.Classes)),
		"symbol_count": fmt.Sprintf("%d", len(res.AllSymbols)),
		"route_count":  fmt.Sprintf("%d", len(res.Routes)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangJava,
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
