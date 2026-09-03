package csharp

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

// CSharpField represents a field or constant declaration in a C# class, struct, or record.
type CSharpField struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Visibility   string   `json:"visibility,omitempty"` // public, private, protected, internal, etc.
	IsStatic     bool     `json:"is_static"`
	IsReadonly   bool     `json:"is_readonly"`
	IsConst      bool     `json:"is_const"`
	Attributes   []string `json:"attributes,omitempty"`
	Doc          string   `json:"doc,omitempty"`
	InitialValue string   `json:"initial_value,omitempty"`
}

// CSharpProperty represents a C# property (auto-property, expression-bodied, or full getter/setter).
type CSharpProperty struct {
	Name             string   `json:"name"`
	Type             string   `json:"type"`
	Visibility       string   `json:"visibility,omitempty"`
	IsStatic         bool     `json:"is_static"`
	IsVirtual        bool     `json:"is_virtual"`
	IsOverride       bool     `json:"is_override"`
	IsAbstract       bool     `json:"is_abstract"`
	HasGetter        bool     `json:"has_getter"`
	HasSetter        bool     `json:"has_setter"`
	HasInit          bool     `json:"has_init"`
	GetterVisibility string   `json:"getter_visibility,omitempty"`
	SetterVisibility string   `json:"setter_visibility,omitempty"`
	Attributes       []string `json:"attributes,omitempty"`
	Doc              string   `json:"doc,omitempty"`
	InitialValue     string   `json:"initial_value,omitempty"`
	IsDbSet          bool     `json:"is_dbset"`
	DbSetEntity      string   `json:"dbset_entity,omitempty"`
}

// CSharpParam represents a method or constructor parameter.
type CSharpParam struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	DefaultValue string   `json:"default_value,omitempty"`
	Attributes   []string `json:"attributes,omitempty"`
	IsRef        bool     `json:"is_ref"`
	IsOut        bool     `json:"is_out"`
	IsIn         bool     `json:"is_in"`
	IsParams     bool     `json:"is_params"`
}

// CSharpMethod represents an extracted C# method or constructor.
type CSharpMethod struct {
	Name          string        `json:"name"`
	Visibility    string        `json:"visibility,omitempty"`
	IsStatic      bool          `json:"is_static"`
	IsAsync       bool          `json:"is_async"`
	IsVirtual     bool          `json:"is_virtual"`
	IsOverride    bool          `json:"is_override"`
	IsAbstract    bool          `json:"is_abstract"`
	ReturnType    string        `json:"return_type,omitempty"`
	GenericParams []string      `json:"generic_params,omitempty"`
	Params        []CSharpParam `json:"params"`
	Attributes    []string      `json:"attributes,omitempty"`
	Doc           string        `json:"doc,omitempty"`
	RouteMethod   string        `json:"route_method,omitempty"` // GET, POST, PUT, DELETE, PATCH, etc.
	RoutePath     string        `json:"route_path,omitempty"`   // /api/v1/users/{id}
	EFQuery       string        `json:"ef_query,omitempty"`
	BodySource    string        `json:"body_source,omitempty"`
	CalledMethods []string      `json:"called_methods,omitempty"`
	IsConstructor bool          `json:"is_constructor"`
}

// CSharpEnumMember represents an individual member in a C# enum.
type CSharpEnumMember struct {
	Name       string   `json:"name"`
	Value      string   `json:"value,omitempty"`
	Doc        string   `json:"doc,omitempty"`
	Attributes []string `json:"attributes,omitempty"`
}

// CSharpType represents an extracted C# class, struct, interface, record, or enum.
type CSharpType struct {
	Kind          string             `json:"kind"` // class, struct, interface, record, record class, record struct, enum
	Name          string             `json:"name"`
	Namespace     string             `json:"namespace"`
	Visibility    string             `json:"visibility,omitempty"`
	IsAbstract    bool               `json:"is_abstract"`
	IsSealed      bool               `json:"is_sealed"`
	IsStatic      bool               `json:"is_static"`
	IsPartial     bool               `json:"is_partial"`
	GenericParams []string           `json:"generic_params,omitempty"`
	BaseType      string             `json:"base_type,omitempty"`
	Interfaces    []string           `json:"interfaces,omitempty"`
	Attributes    []string           `json:"attributes,omitempty"`
	Doc           string             `json:"doc,omitempty"`
	Fields        []CSharpField      `json:"fields"`
	Properties    []CSharpProperty   `json:"properties"`
	Methods       []CSharpMethod     `json:"methods"`
	EnumMembers   []CSharpEnumMember `json:"enum_members,omitempty"`
	IsController  bool               `json:"is_controller"`
	BasePath      string             `json:"base_path,omitempty"` // from [Route("...")]
	IsDbContext   bool               `json:"is_dbcontext"`
	IsEntity      bool               `json:"is_entity"`
	TableName     string             `json:"table_name,omitempty"` // from [Table("...")] or DbSet
}

// CSharpRouteBinding represents an ASP.NET Core controller route or Minimal API route endpoint.
type CSharpRouteBinding struct {
	Method       string `json:"method"`       // GET, POST, PUT, DELETE, PATCH, ANY
	Path         string `json:"path"`         // /api/v1/users/{id}
	HandlerName  string `json:"handler_name"` // UsersController.GetUserById or MapGet:/api/v1/todos
	Framework    string `json:"framework"`    // aspnetcore
	IsMinimalAPI bool   `json:"is_minimal_api"`
	LineNumber   int    `json:"line_number"`
}

// CSharpFileResult contains all extracted symbols and metadata from a C# source file.
type CSharpFileResult struct {
	FilePath   string                `json:"file_path"`
	Namespace  string                `json:"namespace"`
	Usings     []string              `json:"usings"`
	Types      []CSharpType          `json:"types"`
	Routes     []CSharpRouteBinding  `json:"routes"`
	AllSymbols []*core.ASTSymbolNode `json:"all_symbols"`
}

// CSharpParser parses C# (.cs) files into structured AST symbol nodes.
type CSharpParser struct{}

// NewCSharpParser creates a new CSharpParser instance.
func NewCSharpParser() *CSharpParser {
	return &CSharpParser{}
}

// ParseSource parses raw C# source code bytes.
func (p *CSharpParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*CSharpFileResult, error) {
	if filename == "" {
		filename = "Source.cs"
	}

	result := &CSharpFileResult{
		FilePath: filename,
	}

	lines := splitCSharpLines(src)

	// 1. Extract Namespace and Usings
	result.Namespace = p.extractNamespace(lines)
	result.Usings = p.extractUsings(lines)

	// 2. Extract Types (Classes, Structs, Interfaces, Records, Enums)
	result.Types = p.extractTypes(lines, result.Namespace)

	// 3. Extract Routes (ASP.NET Core Controller routes and Minimal APIs)
	controllerRoutes := p.extractControllerRoutes(result.Types)
	minimalRoutes := p.extractMinimalAPIRoutes(lines)
	result.Routes = append(controllerRoutes, minimalRoutes...)

	// 4. Convert to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a C# source file from disk.
func (p *CSharpParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*CSharpFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .cs files in a directory.
func (p *CSharpParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*CSharpFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*CSharpFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".cs" {
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

type csharpLine struct {
	num  int
	text string
}

func splitCSharpLines(src []byte) []csharpLine {
	var lines []csharpLine
	scanner := bufio.NewScanner(bytes.NewReader(src))
	num := 1
	for scanner.Scan() {
		lines = append(lines, csharpLine{
			num:  num,
			text: scanner.Text(),
		})
		num++
	}
	return lines
}

var (
	csNamespaceRegex = regexp.MustCompile(`^namespace\s+([a-zA-Z0-9_.]+)(?:\s*;|\s*\{)?`)
	csUsingRegex     = regexp.MustCompile(`^using\s+(?:static\s+)?([a-zA-Z0-9_.]+)(?:\s*=\s*[a-zA-Z0-9_.]+)?\s*;`)
	csTableRegex     = regexp.MustCompile(`\[Table\s*\(\s*["']([^"']*)["'](?:\s*,\s*Schema\s*=\s*["']([^"']*)["'])?\s*\)\]`)
	csRouteRegex     = regexp.MustCompile(`\[Route\s*\(\s*["']([^"']*)["']\s*\)\]`)
	csHttpRegex      = regexp.MustCompile(`\[Http(Get|Post|Put|Delete|Patch|Head|Options)\s*(?:\(\s*["']([^"']*)["']\s*\)|\(\s*\))?\s*\]`)
	csDbSetRegex     = regexp.MustCompile(`(?:public|protected|internal)?\s*DbSet\s*<\s*([A-Za-z0-9_]+)\s*>\s+([A-Za-z0-9_]+)\s*\{`)
	csMinimalRegex   = regexp.MustCompile(`\b(?:app|routes|group|endpoints)\.Map(Get|Post|Put|Delete|Patch)\s*\(\s*["']([^"']*)["']`)
	csTypeHeaderReg  = regexp.MustCompile(`^(?:(public|protected|private|internal|protected\s+internal|private\s+protected)\s+)?(?:(abstract|sealed|static|partial|readonly)\s+)*(class|struct|interface|record\s+class|record\s+struct|record|enum)\s+([A-Za-z0-9_]+)(?:<([^>]+)>)?(?:\s*\((.*?)\))?(?:\s*:\s*([A-Za-z0-9_.,<>\s]+))?`)
	csFieldHeaderReg = regexp.MustCompile(`^(?:(public|protected|private|internal|protected\s+internal|private\s+protected)\s+)?(?:(static|readonly|const|volatile)\s+)*([A-Za-z0-9_<>\[\],\s\?]+?)\s+([A-Za-z0-9_]+)(?:\s*=\s*(.*?))?\s*;`)
)

func (p *CSharpParser) extractNamespace(lines []csharpLine) string {
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := csNamespaceRegex.FindStringSubmatch(trimmed); m != nil {
			return m[1]
		}
	}
	return ""
}

func (p *CSharpParser) extractUsings(lines []csharpLine) []string {
	var usings []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := csUsingRegex.FindStringSubmatch(trimmed); m != nil {
			usings = append(usings, m[1])
		}
	}
	return usings
}

func (p *CSharpParser) extractTypes(lines []csharpLine, ns string) []CSharpType {
	var types []CSharpType
	var pendingDoc []string
	var pendingAttributes []string

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i].text)

		// XML doc comments or regular comments
		if strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			docLine := trimmed
			docLine = strings.TrimPrefix(docLine, "///")
			docLine = strings.TrimPrefix(docLine, "//")
			docLine = strings.TrimPrefix(docLine, "/*")
			docLine = strings.TrimPrefix(docLine, "*/")
			docLine = strings.TrimPrefix(docLine, "*")
			docClean := strings.TrimSpace(docLine)
			if docClean != "" {
				pendingDoc = append(pendingDoc, docClean)
			}
			continue
		}

		// Attributes e.g. [ApiController], [Route("api/[controller]")]
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			pendingAttributes = append(pendingAttributes, trimmed)
			continue
		}

		// Match Class / Struct / Interface / Record / Enum header
		if m := csTypeHeaderReg.FindStringSubmatch(trimmed); m != nil && !strings.HasPrefix(trimmed, "return") && !strings.HasPrefix(trimmed, "var ") {
			vis := m[1]
			modifiers := m[2]
			kind := m[3]
			name := m[4]
			genericParamsStr := m[5]
			positionalParamsStr := m[6]
			inheritanceStr := m[7]

			var genericParams []string
			if genericParamsStr != "" {
				for _, gp := range strings.Split(genericParamsStr, ",") {
					gp = strings.TrimSpace(gp)
					if gp != "" {
						genericParams = append(genericParams, gp)
					}
				}
			}

			var baseType string
			var interfaces []string
			if inheritanceStr != "" {
				parts := strings.Split(inheritanceStr, ",")
				for idx, part := range parts {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					if idx == 0 && !strings.HasPrefix(part, "I") {
						baseType = part
					} else {
						interfaces = append(interfaces, part)
					}
				}
			}

			isAbstract := strings.Contains(modifiers, "abstract")
			isSealed := strings.Contains(modifiers, "sealed")
			isStatic := strings.Contains(modifiers, "static")
			isPartial := strings.Contains(modifiers, "partial")

			isController := false
			basePath := ""
			isDbContext := false
			isEntity := false
			tableName := ""

			if baseType == "DbContext" || strings.Contains(baseType, "DbContext") {
				isDbContext = true
			}
			for _, iface := range interfaces {
				if strings.Contains(iface, "DbContext") {
					isDbContext = true
				}
			}

			for _, attr := range pendingAttributes {
				if strings.Contains(attr, "[ApiController]") || strings.Contains(attr, "Controller") {
					isController = true
				}
				if tm := csTableRegex.FindStringSubmatch(attr); tm != nil {
					isEntity = true
					tableName = tm[1]
				}
				if rm := csRouteRegex.FindStringSubmatch(attr); rm != nil {
					routeVal := rm[1]
					// Replace [controller] token with controller name (sans Controller suffix)
					ctrlName := name
					if strings.HasSuffix(ctrlName, "Controller") {
						ctrlName = strings.TrimSuffix(ctrlName, "Controller")
					}
					routeVal = strings.ReplaceAll(routeVal, "[controller]", strings.ToLower(ctrlName))
					basePath = routeVal
				}
			}

			if strings.HasSuffix(name, "Controller") {
				isController = true
				if basePath == "" {
					ctrlName := strings.TrimSuffix(name, "Controller")
					basePath = "api/" + strings.ToLower(ctrlName)
				}
			}

			// Parse members inside type body
			var fields []CSharpField
			var properties []CSharpProperty
			var methods []CSharpMethod
			var enumMembers []CSharpEnumMember

			// If it has positional parameters (e.g. record TodoItem(int Id, string Title, bool IsComplete);)
			if positionalParamsStr != "" {
				posParams := parseCSharpParams(positionalParamsStr)
				for _, p := range posParams {
					properties = append(properties, CSharpProperty{
						Name:       p.Name,
						Type:       p.Type,
						Visibility: "public",
						HasGetter:  true,
						HasInit:    true,
					})
				}
			}

			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			isPositionalRecordOnly := positionalParamsStr != "" && strings.HasSuffix(trimmed, ";")

			if !isPositionalRecordOnly {
				var memberDoc []string
				var memberAttributes []string

				for i+1 < len(lines) {
					i++
					mLine := strings.TrimSpace(lines[i].text)

					if strings.HasPrefix(mLine, "///") || strings.HasPrefix(mLine, "//") || strings.HasPrefix(mLine, "/*") || strings.HasPrefix(mLine, "*") {
						docLine := mLine
						docLine = strings.TrimPrefix(docLine, "///")
						docLine = strings.TrimPrefix(docLine, "//")
						docLine = strings.TrimPrefix(docLine, "/*")
						docLine = strings.TrimPrefix(docLine, "*/")
						docLine = strings.TrimPrefix(docLine, "*")
						docClean := strings.TrimSpace(docLine)
						if docClean != "" {
							memberDoc = append(memberDoc, docClean)
						}
						continue
					}

					if strings.HasPrefix(mLine, "[") && strings.HasSuffix(mLine, "]") {
						memberAttributes = append(memberAttributes, mLine)
						continue
					}

					// Count braces
					for _, ch := range mLine {
						if ch == '{' {
							braceCount++
						} else if ch == '}' {
							braceCount--
						}
					}

					if braceCount <= 0 && (strings.Contains(mLine, "}") || (braceCount == 0 && mLine == "")) {
						if strings.Contains(mLine, "}") {
							break
						}
					}

					// If it's an enum, parse enum members
					if kind == "enum" {
						if mLine != "" && mLine != "{" && mLine != "}" {
							enumLine := strings.TrimSuffix(mLine, ",")
							enumParts := strings.SplitN(enumLine, "=", 2)
							memberName := strings.TrimSpace(enumParts[0])
							memberVal := ""
							if len(enumParts) == 2 {
								memberVal = strings.TrimSpace(enumParts[1])
							}
							if memberName != "" && !strings.HasPrefix(memberName, "//") {
								enumMembers = append(enumMembers, CSharpEnumMember{
									Name:       memberName,
									Value:      memberVal,
									Doc:        strings.Join(memberDoc, "\n"),
									Attributes: memberAttributes,
								})
								memberDoc = nil
								memberAttributes = nil
							}
						}
						continue
					}

					// Check for DbSet properties
					if dbm := csDbSetRegex.FindStringSubmatch(mLine); dbm != nil {
						entityType := dbm[1]
						propName := dbm[2]
						properties = append(properties, CSharpProperty{
							Name:        propName,
							Type:        fmt.Sprintf("DbSet<%s>", entityType),
							Visibility:  "public",
							HasGetter:   true,
							HasSetter:   true,
							IsDbSet:     true,
							DbSetEntity: entityType,
							Doc:         strings.Join(memberDoc, "\n"),
							Attributes:  memberAttributes,
						})
						memberDoc = nil
						memberAttributes = nil
						continue
					}

					// Check if this line is a Method or Constructor
					if mParsed, isMethod := parseCSharpMethodLine(mLine, name, memberAttributes, memberDoc); isMethod {
						// Extract full method body if applicable
						bodyLines := []string{mLine}
						methodBraceCount := 0
						for _, ch := range mLine {
							if ch == '{' {
								methodBraceCount++
							} else if ch == '}' {
								methodBraceCount--
							}
						}

						// If body hasn't opened yet (e.g. { on next line)
						if methodBraceCount == 0 && !strings.HasSuffix(mLine, ";") && !strings.Contains(mLine, "=>") {
							for i+1 < len(lines) {
								nextLine := strings.TrimSpace(lines[i+1].text)
								if nextLine == "{" || strings.HasPrefix(nextLine, "{") {
									i++
									bodyLines = append(bodyLines, lines[i].text)
									for _, ch := range lines[i].text {
										if ch == '{' {
											methodBraceCount++
											braceCount++
										} else if ch == '}' {
											methodBraceCount--
											braceCount--
										}
									}
									break
								} else if nextLine != "" && !strings.HasPrefix(nextLine, "//") {
									break
								}
								i++
							}
						}

						if methodBraceCount > 0 {
							for i+1 < len(lines) {
								i++
								bLine := lines[i].text
								bodyLines = append(bodyLines, bLine)
								for _, ch := range bLine {
									if ch == '{' {
										methodBraceCount++
										braceCount++
									} else if ch == '}' {
										methodBraceCount--
										braceCount--
									}
								}
								if methodBraceCount <= 0 {
									break
								}
							}
						}

						bodySrc := strings.Join(bodyLines, "\n")
						mParsed.BodySource = bodySrc
						mParsed.CalledMethods = extractCSharpCalls(bodySrc)

						methods = append(methods, *mParsed)
						memberDoc = nil
						memberAttributes = nil
						continue
					}

					// Check if this line is a Property
					if pParsed, isProp := parseCSharpPropertyLine(mLine, memberAttributes, memberDoc); isProp {
						properties = append(properties, *pParsed)
						memberDoc = nil
						memberAttributes = nil
						continue
					}

					// Match Fields
					if fm := csFieldHeaderReg.FindStringSubmatch(mLine); fm != nil && !strings.Contains(mLine, "(") {
						fVis := fm[1]
						fMods := fm[2]
						fType := fm[3]
						fName := fm[4]
						fVal := ""
						if len(fm) > 5 {
							fVal = fm[5]
						}

						fields = append(fields, CSharpField{
							Name:         fName,
							Type:         fType,
							Visibility:   fVis,
							IsStatic:     strings.Contains(fMods, "static"),
							IsReadonly:   strings.Contains(fMods, "readonly"),
							IsConst:      strings.Contains(fMods, "const"),
							Attributes:   memberAttributes,
							Doc:          strings.Join(memberDoc, "\n"),
							InitialValue: fVal,
						})

						memberDoc = nil
						memberAttributes = nil
						continue
					}

					if mLine != "" && !strings.HasPrefix(mLine, "//") && mLine != "{" && mLine != "}" {
						memberDoc = nil
						memberAttributes = nil
					}
				}
			}

			types = append(types, CSharpType{
				Kind:          kind,
				Name:          name,
				Namespace:     ns,
				Visibility:    vis,
				IsAbstract:    isAbstract,
				IsSealed:      isSealed,
				IsStatic:      isStatic,
				IsPartial:     isPartial,
				GenericParams: genericParams,
				BaseType:      baseType,
				Interfaces:    interfaces,
				Attributes:    pendingAttributes,
				Doc:           strings.Join(pendingDoc, "\n"),
				Fields:        fields,
				Properties:    properties,
				Methods:       methods,
				EnumMembers:   enumMembers,
				IsController:  isController,
				BasePath:      basePath,
				IsDbContext:   isDbContext,
				IsEntity:      isEntity,
				TableName:     tableName,
			})

			pendingDoc = nil
			pendingAttributes = nil
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "[") {
				pendingDoc = nil
				pendingAttributes = nil
			}
		}
	}

	return types
}

func parseCSharpMethodLine(line, enclosingTypeName string, attributes, doc []string) (*CSharpMethod, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "return ") || strings.HasPrefix(trimmed, "throw ") {
		return nil, false
	}
	if strings.Contains(trimmed, "class ") || strings.Contains(trimmed, "struct ") || strings.Contains(trimmed, "enum ") || strings.Contains(trimmed, "interface ") {
		return nil, false
	}

	openParenIdx := strings.Index(trimmed, "(")
	if openParenIdx == -1 {
		return nil, false
	}

	closeParenIdx := strings.LastIndex(trimmed, ")")
	if closeParenIdx == -1 || closeParenIdx <= openParenIdx {
		return nil, false
	}

	beforeParen := strings.TrimSpace(trimmed[:openParenIdx])
	if strings.Contains(beforeParen, "=") {
		return nil, false
	}
	paramsStr := trimmed[openParenIdx+1 : closeParenIdx]

	tokens := strings.Fields(beforeParen)
	if len(tokens) == 0 {
		return nil, false
	}

	mNameWithGen := tokens[len(tokens)-1]
	mName := mNameWithGen
	var genParams []string
	if genIdx := strings.Index(mNameWithGen, "<"); genIdx != -1 {
		mName = mNameWithGen[:genIdx]
		genStr := strings.TrimSuffix(mNameWithGen[genIdx+1:], ">")
		for _, gp := range strings.Split(genStr, ",") {
			gp = strings.TrimSpace(gp)
			if gp != "" {
				genParams = append(genParams, gp)
			}
		}
	}

	if mName == "if" || mName == "while" || mName == "for" || mName == "foreach" || mName == "switch" || mName == "catch" || mName == "using" || mName == "lock" || mName == "typeof" || mName == "nameof" {
		return nil, false
	}

	isCtor := false
	if mName == enclosingTypeName {
		isCtor = true
	}

	var vis string
	var mods []string
	var retTokens []string

	for idx, tok := range tokens[:len(tokens)-1] {
		switch tok {
		case "public", "private", "protected", "internal":
			if vis == "" {
				vis = tok
			} else {
				vis = vis + " " + tok
			}
		case "static", "virtual", "override", "abstract", "async", "partial", "new", "readonly", "extern", "unsafe":
			mods = append(mods, tok)
		default:
			retTokens = append(retTokens, tokens[idx:]...)
			break
		}
		if len(retTokens) > 0 {
			break
		}
	}

	retType := ""
	if !isCtor {
		if len(retTokens) > 0 {
			retType = strings.Join(retTokens, " ")
		} else if len(tokens) >= 2 {
			retType = tokens[len(tokens)-2]
		}
	}

	params := parseCSharpParams(paramsStr)

	routeMethod := ""
	routePath := ""
	for _, attr := range attributes {
		if hm := csHttpRegex.FindStringSubmatch(attr); hm != nil {
			routeMethod = strings.ToUpper(hm[1])
			if len(hm) > 2 {
				routePath = hm[2]
			}
		}
		if rm := csRouteRegex.FindStringSubmatch(attr); rm != nil && routePath == "" {
			routePath = rm[1]
		}
	}

	modsStr := strings.Join(mods, " ")

	return &CSharpMethod{
		Name:          mName,
		Visibility:    vis,
		IsStatic:      strings.Contains(modsStr, "static"),
		IsAsync:       strings.Contains(modsStr, "async"),
		IsVirtual:     strings.Contains(modsStr, "virtual"),
		IsOverride:    strings.Contains(modsStr, "override"),
		IsAbstract:    strings.Contains(modsStr, "abstract"),
		ReturnType:    retType,
		GenericParams: genParams,
		Params:        params,
		Attributes:    attributes,
		Doc:           strings.Join(doc, "\n"),
		RouteMethod:   routeMethod,
		RoutePath:     routePath,
		IsConstructor: isCtor,
	}, true
}

func parseCSharpPropertyLine(line string, attributes, doc []string) (*CSharpProperty, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "{") && !strings.Contains(trimmed, "=>") {
		return nil, false
	}
	if strings.Contains(trimmed, "(") && !strings.Contains(trimmed, "=>") {
		return nil, false
	}
	if !strings.Contains(trimmed, "get") && !strings.Contains(trimmed, "set") && !strings.Contains(trimmed, "init") && !strings.Contains(trimmed, "=>") {
		return nil, false
	}

	headerPart := trimmed
	if idx := strings.Index(trimmed, "{"); idx != -1 {
		headerPart = strings.TrimSpace(trimmed[:idx])
	} else if idx := strings.Index(trimmed, "=>"); idx != -1 {
		headerPart = strings.TrimSpace(trimmed[:idx])
	}

	tokens := strings.Fields(headerPart)
	if len(tokens) < 2 {
		return nil, false
	}

	pName := tokens[len(tokens)-1]
	var vis string
	var mods []string
	var typeTokens []string

	for idx, tok := range tokens[:len(tokens)-1] {
		switch tok {
		case "public", "private", "protected", "internal":
			if vis == "" {
				vis = tok
			} else {
				vis = vis + " " + tok
			}
		case "static", "virtual", "override", "abstract", "required", "new", "readonly":
			mods = append(mods, tok)
		default:
			typeTokens = append(typeTokens, tokens[idx:]...)
			break
		}
		if len(typeTokens) > 0 {
			break
		}
	}

	pType := ""
	if len(typeTokens) > 0 {
		pType = strings.Join(typeTokens, " ")
	} else {
		pType = tokens[len(tokens)-2]
	}

	hasGet := strings.Contains(trimmed, "get") || strings.Contains(trimmed, "=>")
	hasSet := strings.Contains(trimmed, "set")
	hasInit := strings.Contains(trimmed, "init")
	modsStr := strings.Join(mods, " ")

	return &CSharpProperty{
		Name:       pName,
		Type:       pType,
		Visibility: vis,
		IsStatic:   strings.Contains(modsStr, "static"),
		IsVirtual:  strings.Contains(modsStr, "virtual"),
		IsOverride: strings.Contains(modsStr, "override"),
		IsAbstract: strings.Contains(modsStr, "abstract"),
		HasGetter:  hasGet,
		HasSetter:  hasSet,
		HasInit:    hasInit,
		Attributes: attributes,
		Doc:        strings.Join(doc, "\n"),
	}, true
}

func (p *CSharpParser) extractControllerRoutes(types []CSharpType) []CSharpRouteBinding {
	var routes []CSharpRouteBinding

	for _, t := range types {
		for _, m := range t.Methods {
			if m.RouteMethod != "" {
				fullPath := t.BasePath
				if !strings.HasPrefix(fullPath, "/") && fullPath != "" {
					fullPath = "/" + fullPath
				}

				actionPath := m.RoutePath
				if actionPath != "" {
					if strings.HasPrefix(actionPath, "/") {
						fullPath = actionPath
					} else {
						if fullPath == "" || fullPath == "/" {
							fullPath = "/" + actionPath
						} else {
							fullPath = strings.TrimSuffix(fullPath, "/") + "/" + actionPath
						}
					}
				}

				if fullPath == "" {
					fullPath = "/"
				}

				routes = append(routes, CSharpRouteBinding{
					Method:       m.RouteMethod,
					Path:         fullPath,
					HandlerName:  fmt.Sprintf("%s.%s", t.Name, m.Name),
					Framework:    "aspnetcore",
					IsMinimalAPI: false,
					LineNumber:   1,
				})
			}
		}
	}

	return routes
}

func (p *CSharpParser) extractMinimalAPIRoutes(lines []csharpLine) []CSharpRouteBinding {
	var routes []CSharpRouteBinding

	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := csMinimalRegex.FindStringSubmatch(trimmed); m != nil {
			httpMethod := strings.ToUpper(m[1])
			path := m[2]
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}

			routes = append(routes, CSharpRouteBinding{
				Method:       httpMethod,
				Path:         path,
				HandlerName:  fmt.Sprintf("Map%s:%s", m[1], path),
				Framework:    "aspnetcore",
				IsMinimalAPI: true,
				LineNumber:   l.num,
			})
		}
	}

	return routes
}

func parseCSharpParams(s string) []CSharpParam {
	var params []CSharpParam
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

		isRef := false
		isOut := false
		isIn := false
		isParams := false

		tokens := strings.Fields(p)
		var attributes []string
		var cleanTokens []string

		for _, tok := range tokens {
			if strings.HasPrefix(tok, "[") && strings.HasSuffix(tok, "]") {
				attributes = append(attributes, tok)
			} else if tok == "ref" {
				isRef = true
			} else if tok == "out" {
				isOut = true
			} else if tok == "in" {
				isIn = true
			} else if tok == "params" {
				isParams = true
			} else {
				cleanTokens = append(cleanTokens, tok)
			}
		}

		if len(cleanTokens) >= 2 {
			defVal := ""
			pName := cleanTokens[len(cleanTokens)-1]
			if strings.Contains(pName, "=") {
				pParts := strings.SplitN(pName, "=", 2)
				pName = strings.TrimSpace(pParts[0])
				defVal = strings.TrimSpace(pParts[1])
			}

			pType := strings.Join(cleanTokens[:len(cleanTokens)-1], " ")
			params = append(params, CSharpParam{
				Name:         pName,
				Type:         pType,
				DefaultValue: defVal,
				Attributes:   attributes,
				IsRef:        isRef,
				IsOut:        isOut,
				IsIn:         isIn,
				IsParams:     isParams,
			})
		} else if len(cleanTokens) == 1 {
			params = append(params, CSharpParam{
				Name:       cleanTokens[0],
				Type:       "object",
				Attributes: attributes,
			})
		}
	}
	return params
}

var csCallRegex = regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*\(`)

func extractCSharpCalls(src string) []string {
	var calls []string
	seen := make(map[string]bool)
	matches := csCallRegex.FindAllStringSubmatch(src, -1)
	for _, m := range matches {
		name := m[1]
		if !seen[name] && name != "if" && name != "while" && name != "for" && name != "foreach" && name != "switch" && name != "catch" && name != "using" && name != "lock" && name != "typeof" && name != "nameof" {
			seen[name] = true
			calls = append(calls, name)
		}
	}
	sort.Strings(calls)
	return calls
}

func (p *CSharpParser) toSymbolNodes(res *CSharpFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	for _, t := range res.Types {
		typePayload, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}

		var deps []string
		if t.BaseType != "" {
			deps = append(deps, t.BaseType)
		}
		deps = append(deps, t.Interfaces...)
		for _, f := range t.Fields {
			deps = append(deps, f.Type)
		}
		for _, prop := range t.Properties {
			deps = append(deps, prop.Type)
			if prop.DbSetEntity != "" {
				deps = append(deps, prop.DbSetEntity)
			}
		}
		for _, m := range t.Methods {
			if m.ReturnType != "" {
				deps = append(deps, m.ReturnType)
			}
			for _, param := range m.Params {
				deps = append(deps, param.Type)
			}
		}
		sort.Strings(deps)

		nodeType := "ClassDeclaration"
		switch t.Kind {
		case "struct":
			nodeType = "StructDeclaration"
		case "interface":
			nodeType = "InterfaceDeclaration"
		case "record", "record class", "record struct":
			nodeType = "RecordDeclaration"
		case "enum":
			nodeType = "EnumDeclaration"
		}

		meta := map[string]string{
			"kind":           t.Kind,
			"method_count":   fmt.Sprintf("%d", len(t.Methods)),
			"property_count": fmt.Sprintf("%d", len(t.Properties)),
			"field_count":    fmt.Sprintf("%d", len(t.Fields)),
		}
		if t.IsController {
			meta["is_controller"] = "true"
			if t.BasePath != "" {
				meta["base_path"] = t.BasePath
			}
		}
		if t.IsDbContext {
			meta["is_dbcontext"] = "true"
		}
		if t.IsEntity {
			meta["is_entity"] = "true"
			if t.TableName != "" {
				meta["table_name"] = t.TableName
			}
		}

		ident := t.Name
		if t.Namespace != "" {
			ident = fmt.Sprintf("%s.%s", t.Namespace, t.Name)
		}

		vis := t.Visibility
		if vis == "" {
			vis = "internal"
		}

		node := &core.ASTSymbolNode{
			Language:          core.LangCSharp,
			NodeType:          nodeType,
			Identifier:        ident,
			Signature:         fmt.Sprintf("%s %s", t.Kind, t.Name),
			Docstring:         t.Doc,
			Visibility:        vis,
			ASTPayload:        typePayload,
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

		// Extract individual methods
		for _, m := range t.Methods {
			mPayload, err := json.Marshal(m)
			if err != nil {
				continue
			}

			mMeta := make(map[string]string)
			if m.RouteMethod != "" {
				mMeta["route_method"] = m.RouteMethod
				mMeta["route_path"] = m.RoutePath
			}
			if m.EFQuery != "" {
				mMeta["ef_query"] = m.EFQuery
			}

			mIdent := fmt.Sprintf("%s.%s", ident, m.Name)
			mVis := m.Visibility
			if mVis == "" {
				mVis = "private"
			}

			mNodeType := "MethodDeclaration"
			if m.IsConstructor {
				mNodeType = "ConstructorDeclaration"
			}

			mNode := &core.ASTSymbolNode{
				Language:          core.LangCSharp,
				NodeType:          mNodeType,
				Identifier:        mIdent,
				Signature:         fmt.Sprintf("%s %s(%s)", m.ReturnType, m.Name, formatCSharpParams(m.Params)),
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

		// Extract individual properties
		for _, prop := range t.Properties {
			pPayload, err := json.Marshal(prop)
			if err != nil {
				continue
			}

			pMeta := map[string]string{
				"property_type": prop.Type,
			}
			if prop.IsDbSet {
				pMeta["is_dbset"] = "true"
				pMeta["dbset_entity"] = prop.DbSetEntity
			}

			pIdent := fmt.Sprintf("%s.%s", ident, prop.Name)
			pVis := prop.Visibility
			if pVis == "" {
				pVis = "public"
			}

			pNode := &core.ASTSymbolNode{
				Language:    core.LangCSharp,
				NodeType:    "PropertyDeclaration",
				Identifier:  pIdent,
				Signature:   fmt.Sprintf("%s %s", prop.Type, prop.Name),
				Docstring:   prop.Doc,
				Visibility:  pVis,
				ASTPayload:  pPayload,
				ASTMetadata: pMeta,
				Lineage:     lineage,
			}
			pNodeID, err := core.HashASTSymbolNode(pNode)
			if err == nil {
				pNode.NodeID = pNodeID
				nodes = append(nodes, pNode)
			}
		}
	}

	// Routes
	for _, r := range res.Routes {
		rPayload, err := json.Marshal(r)
		if err != nil {
			continue
		}
		rIdent := fmt.Sprintf("%s:%s %s", res.Namespace, r.Method, r.Path)
		if res.Namespace == "" {
			rIdent = fmt.Sprintf("%s %s", r.Method, r.Path)
		}

		rNode := &core.ASTSymbolNode{
			Language:          core.LangCSharp,
			NodeType:          "RouteBinding",
			Identifier:        rIdent,
			Signature:         fmt.Sprintf("%s %s", r.Method, r.Path),
			ASTPayload:        rPayload,
			ASTMetadata:       map[string]string{"framework": r.Framework, "handler": r.HandlerName, "route_method": r.Method, "route_path": r.Path},
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

func formatCSharpParams(params []CSharpParam) string {
	var list []string
	for _, p := range params {
		prefix := ""
		if p.IsRef {
			prefix = "ref "
		} else if p.IsOut {
			prefix = "out "
		} else if p.IsIn {
			prefix = "in "
		} else if p.IsParams {
			prefix = "params "
		}
		defStr := ""
		if p.DefaultValue != "" {
			defStr = fmt.Sprintf(" = %s", p.DefaultValue)
		}
		list = append(list, fmt.Sprintf("%s%s %s%s", prefix, p.Type, p.Name, defStr))
	}
	return strings.Join(list, ", ")
}

// BuildComponentNode bundles parsed C# symbols into a core.ComponentNode.
func (p *CSharpParser) BuildComponentNode(
	res *CSharpFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = res.Namespace
		if compName == "" {
			compName = "csharp-service"
		}
	}

	var symbolIDs []string
	for _, sym := range res.AllSymbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangCSharp,
		SymbolNodes: symbolIDs,
		Metadata: map[string]string{
			"file_path":   res.FilePath,
			"namespace":   res.Namespace,
			"types_count": fmt.Sprintf("%d", len(res.Types)),
		},
		Lineage: lineage,
	}

	compID, err := core.HashComponentNode(comp)
	if err != nil {
		return nil, fmt.Errorf("hashing component node: %w", err)
	}
	comp.ComponentID = compID
	return comp, nil
}
