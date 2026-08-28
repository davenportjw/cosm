package rust

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

// RustField represents a struct field in Rust.
type RustField struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Visibility string   `json:"visibility,omitempty"`
	Attributes []string `json:"attributes,omitempty"`
	Doc        string   `json:"doc,omitempty"`
}

// RustStruct represents an extracted Rust struct definition.
type RustStruct struct {
	Name       string      `json:"name"`
	Visibility string      `json:"visibility,omitempty"` // pub, pub(crate), private
	Generics   string      `json:"generics,omitempty"`
	Derives    []string    `json:"derives,omitempty"`    // e.g. ["Serialize", "Deserialize", "Debug", "Clone"]
	Attributes []string    `json:"attributes,omitempty"` // e.g. #[serde(rename_all = "camelCase")]
	Doc        string      `json:"doc,omitempty"`
	Fields     []RustField `json:"fields"`
	IsTuple    bool        `json:"is_tuple,omitempty"`
}

// RustEnumVariant represents an enum variant.
type RustEnumVariant struct {
	Name         string      `json:"name"`
	Doc          string      `json:"doc,omitempty"`
	Fields       []RustField `json:"fields,omitempty"`
	Discriminant string      `json:"discriminant,omitempty"`
}

// RustEnum represents an extracted Rust enum definition.
type RustEnum struct {
	Name       string            `json:"name"`
	Visibility string            `json:"visibility,omitempty"`
	Generics   string            `json:"generics,omitempty"`
	Derives    []string          `json:"derives,omitempty"`
	Attributes []string          `json:"attributes,omitempty"`
	Doc        string            `json:"doc,omitempty"`
	Variants   []RustEnumVariant `json:"variants"`
}

// RustParam represents a function parameter.
type RustParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// RustFunction represents an extracted Rust function or method definition.
type RustFunction struct {
	Name         string      `json:"name"`
	Visibility   string      `json:"visibility,omitempty"` // pub, pub(crate), private
	IsAsync      bool        `json:"is_async"`
	IsConst      bool        `json:"is_const"`
	IsUnsafe     bool        `json:"is_unsafe"`
	Generics     string      `json:"generics,omitempty"`
	Params       []RustParam `json:"params"`
	ReturnType   string      `json:"return_type,omitempty"`
	Doc          string      `json:"doc,omitempty"`
	Attributes   []string    `json:"attributes,omitempty"`
	RouteMethod  string      `json:"route_method,omitempty"` // GET, POST, etc.
	RoutePath    string      `json:"route_path,omitempty"`   // /api/v1/users
	SQLxQueries  []string    `json:"sqlx_queries,omitempty"` // Extracted SQL statements from sqlx::query!
	BodySource   string      `json:"body_source,omitempty"`
	CalledFuncs  []string    `json:"called_funcs,omitempty"`
}

// RustTraitMethod represents a method inside a trait definition.
type RustTraitMethod struct {
	Name        string      `json:"name"`
	IsAsync     bool        `json:"is_async"`
	Params      []RustParam `json:"params"`
	ReturnType  string      `json:"return_type,omitempty"`
	Doc         string      `json:"doc,omitempty"`
	DefaultBody string      `json:"default_body,omitempty"`
}

// RustTrait represents an extracted Rust trait definition.
type RustTrait struct {
	Name       string            `json:"name"`
	Visibility string            `json:"visibility,omitempty"`
	Generics   string            `json:"generics,omitempty"`
	Doc        string            `json:"doc,omitempty"`
	Attributes []string          `json:"attributes,omitempty"`
	Methods    []RustTraitMethod `json:"methods"`
}

// RustImpl represents an impl block (e.g. `impl Trait for Target` or `impl Target`).
type RustImpl struct {
	TraitName  string         `json:"trait_name,omitempty"`
	TargetType string         `json:"target_type"`
	Generics   string         `json:"generics,omitempty"`
	Methods    []RustFunction `json:"methods"`
}

// RustMacro represents a macro definition (e.g. `macro_rules!`).
type RustMacro struct {
	Name       string `json:"name"`
	Visibility string `json:"visibility,omitempty"`
	Doc        string `json:"doc,omitempty"`
	Body       string `json:"body"`
}

// RustRouteBinding represents an HTTP endpoint discovered in Rust code (Axum, Actix, Rocket).
type RustRouteBinding struct {
	Method      string `json:"method"`       // GET, POST, PUT, DELETE, PATCH, ANY
	Path        string `json:"path"`         // /api/v1/resource
	HandlerName string `json:"handler_name"` // e.g. get_user
	Framework   string `json:"framework"`    // actix, axum, rocket
	LineNumber  int    `json:"line_number"`
}

// RustFileResult contains all extracted symbols and metadata from a Rust source file.
type RustFileResult struct {
	FilePath    string              `json:"file_path"`
	PackageName string              `json:"package_name"`
	Structs     []RustStruct        `json:"structs"`
	Enums       []RustEnum          `json:"enums"`
	Traits      []RustTrait         `json:"traits"`
	Impls       []RustImpl          `json:"impls"`
	Functions   []RustFunction      `json:"functions"`
	Macros      []RustMacro         `json:"macros"`
	Routes      []RustRouteBinding  `json:"routes"`
	SQLxQueries []string            `json:"sqlx_queries"`
	Imports     []string            `json:"imports"`
	AllSymbols  []*core.ASTSymbolNode `json:"all_symbols"`
}

// RustParser provides Rust AST parsing, symbol extraction, and framework metadata extraction.
type RustParser struct{}

// NewRustParser creates a new RustParser instance.
func NewRustParser() *RustParser {
	return &RustParser{}
}

// ParseSource parses Rust source code bytes and extracts all symbols.
func (p *RustParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*RustFileResult, error) {
	if filename == "" {
		filename = "lib.rs"
	}

	result := &RustFileResult{
		FilePath:    filename,
		PackageName: p.derivePackageName(filename),
	}

	lines := splitLines(src)
	fullText := string(src)

	// 1. Extract Imports (use statements)
	result.Imports = p.extractImports(lines)

	// 2. Extract SQLx Queries across entire source
	result.SQLxQueries = p.extractSQLxQueries(fullText)

	// 3. Extract Structs
	result.Structs = p.extractStructs(lines)

	// 4. Extract Enums
	result.Enums = p.extractEnums(lines)

	// 5. Extract Traits
	result.Traits = p.extractTraits(lines)

	// 6. Extract Impl blocks
	result.Impls = p.extractImpls(lines)

	// 7. Extract Top-level Functions
	result.Functions = p.extractFunctions(lines)

	// 8. Extract Macros
	result.Macros = p.extractMacros(lines)

	// 9. Extract Route Bindings
	result.Routes = p.extractRoutes(result.Functions, lines)

	// Convert all extracted entities to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a Rust source file from disk.
func (p *RustParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*RustFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .rs files in a directory.
func (p *RustParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*RustFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*RustFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".rs" {
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

type lineInfo struct {
	num  int
	text string
}

func splitLines(src []byte) []lineInfo {
	var lines []lineInfo
	scanner := bufio.NewScanner(bytes.NewReader(src))
	num := 1
	for scanner.Scan() {
		lines = append(lines, lineInfo{
			num:  num,
			text: scanner.Text(),
		})
		num++
	}
	return lines
}

func (p *RustParser) derivePackageName(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "lib" || name == "main" || name == "mod" {
		dir := filepath.Base(filepath.Dir(filename))
		if dir != "." && dir != "/" && dir != "" {
			return dir
		}
	}
	return name
}

var (
	useRegex          = regexp.MustCompile(`^use\s+([^;]+);`)
	deriveRegex       = regexp.MustCompile(`^#\[derive\(([^)]+)\)\]`)
	actixRouteRegex   = regexp.MustCompile(`^#\[(get|post|put|delete|patch|head|options)\(["']([^"']+)["']\)\]`)
	sqlxQueryRegex    = regexp.MustCompile(`(?:sqlx::query(?:_as)?!|query(?:_as)?!)\s*\(\s*r?["']([^"']+)["']`)
	structHeaderRegex = regexp.MustCompile(`^(?:(pub(?:\([^)]+\))?)\s+)?struct\s+([A-Za-z0-9_]+)(?:<([^>]+)>)?`)
	enumHeaderRegex   = regexp.MustCompile(`^(?:(pub(?:\([^)]+\))?)\s+)?enum\s+([A-Za-z0-9_]+)(?:<([^>]+)>)?`)
	traitHeaderRegex  = regexp.MustCompile(`^(?:(pub(?:\([^)]+\))?)\s+)?trait\s+([A-Za-z0-9_]+)(?:<([^>]+)>)?`)
	implHeaderRegex   = regexp.MustCompile(`^impl(?:<([^>]+)>)?\s+(?:([A-Za-z0-9_:]+(?:<[^>]+>)?)\s+for\s+)?([A-Za-z0-9_:]+(?:<[^>]+>)?)\s*\{?`)
	fnHeaderRegex     = regexp.MustCompile(`^(?:(pub(?:\([^)]+\))?)\s+)?(?:(async)\s+)?(?:(const)\s+)?(?:(unsafe)\s+)?fn\s+([A-Za-z0-9_]+)(?:<([^>]+)>)?\s*\((.*?)\)(?:\s*->\s*([^{;]+))?`)
	macroHeaderRegex  = regexp.MustCompile(`^(?:(pub(?:\([^)]+\))?)\s+)?macro_rules!\s+([A-Za-z0-9_]+)`)
)

func (p *RustParser) extractImports(lines []lineInfo) []string {
	var imports []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := useRegex.FindStringSubmatch(trimmed); m != nil {
			imports = append(imports, strings.TrimSpace(m[1]))
		}
	}
	return imports
}

func (p *RustParser) extractSQLxQueries(text string) []string {
	var queries []string
	matches := sqlxQueryRegex.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) > 1 {
			q := strings.TrimSpace(m[1])
			if q != "" {
				queries = append(queries, q)
			}
		}
	}
	return queries
}

func (p *RustParser) extractStructs(lines []lineInfo) []RustStruct {
	var structs []RustStruct
	var pendingDoc []string
	var pendingAttrs []string
	var pendingDerives []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "//!") {
			docLine := strings.TrimPrefix(trimmed, "///")
			docLine = strings.TrimPrefix(docLine, "//!")
			pendingDoc = append(pendingDoc, strings.TrimSpace(docLine))
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "#[") {
			pendingAttrs = append(pendingAttrs, trimmed)
			if dm := deriveRegex.FindStringSubmatch(trimmed); dm != nil {
				for _, d := range strings.Split(dm[1], ",") {
					dClean := strings.TrimSpace(d)
					if dClean != "" {
						pendingDerives = append(pendingDerives, dClean)
					}
				}
			}
			i++
			continue
		}

		if m := structHeaderRegex.FindStringSubmatch(trimmed); m != nil && !strings.Contains(trimmed, "fn ") {
			vis := m[1]
			name := m[2]
			generics := m[3]

			isTuple := strings.Contains(trimmed, "(") && strings.Contains(trimmed, ");")
			var fields []RustField

			if !isTuple && strings.Contains(trimmed, "{") {
				// Parse struct fields until matching closing brace
				braceCount := 1
				i++
				for i < len(lines) && braceCount > 0 {
					fLine := strings.TrimSpace(lines[i].text)
					if strings.Contains(fLine, "{") {
						braceCount++
					}
					if strings.Contains(fLine, "}") {
						braceCount--
						if braceCount == 0 {
							break
						}
					}
					if fLine != "" && !strings.HasPrefix(fLine, "//") && !strings.HasPrefix(fLine, "#[") {
						parts := strings.SplitN(fLine, ":", 2)
						if len(parts) == 2 {
							fNamePart := strings.TrimSpace(parts[0])
							fTypePart := strings.TrimSpace(parts[1])
							fTypePart = strings.TrimSuffix(fTypePart, ",")
							fVis := ""
							if strings.HasPrefix(fNamePart, "pub ") {
								fVis = "pub"
								fNamePart = strings.TrimPrefix(fNamePart, "pub ")
							}
							fields = append(fields, RustField{
								Name:       fNamePart,
								Type:       fTypePart,
								Visibility: fVis,
							})
						}
					}
					i++
				}
			}

			docStr := strings.Join(pendingDoc, "\n")
			structs = append(structs, RustStruct{
				Name:       name,
				Visibility: vis,
				Generics:   generics,
				Derives:    pendingDerives,
				Attributes: pendingAttrs,
				Doc:        docStr,
				Fields:     fields,
				IsTuple:    isTuple,
			})

			pendingDoc = nil
			pendingAttrs = nil
			pendingDerives = nil
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
				pendingAttrs = nil
				pendingDerives = nil
			}
		}
		i++
	}

	return structs
}

func (p *RustParser) extractEnums(lines []lineInfo) []RustEnum {
	var enums []RustEnum
	var pendingDoc []string
	var pendingAttrs []string
	var pendingDerives []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "//!") {
			docLine := strings.TrimPrefix(trimmed, "///")
			docLine = strings.TrimPrefix(docLine, "//!")
			pendingDoc = append(pendingDoc, strings.TrimSpace(docLine))
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "#[") {
			pendingAttrs = append(pendingAttrs, trimmed)
			if dm := deriveRegex.FindStringSubmatch(trimmed); dm != nil {
				for _, d := range strings.Split(dm[1], ",") {
					dClean := strings.TrimSpace(d)
					if dClean != "" {
						pendingDerives = append(pendingDerives, dClean)
					}
				}
			}
			i++
			continue
		}

		if m := enumHeaderRegex.FindStringSubmatch(trimmed); m != nil {
			vis := m[1]
			name := m[2]
			generics := m[3]

			var variants []RustEnumVariant
			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			i++
			for i < len(lines) {
				vLine := strings.TrimSpace(lines[i].text)
				if strings.Contains(vLine, "{") {
					braceCount++
				}
				if strings.Contains(vLine, "}") {
					braceCount--
					if braceCount <= 0 {
						break
					}
				}
				if vLine != "" && !strings.HasPrefix(vLine, "//") && !strings.HasPrefix(vLine, "#[") {
					vClean := strings.TrimSuffix(vLine, ",")
					if vClean != "" && !strings.Contains(vClean, "{") && !strings.Contains(vClean, "}") {
						var vName string
						var discriminant string
						if eqIdx := strings.Index(vClean, "="); eqIdx != -1 {
							vName = strings.TrimSpace(vClean[:eqIdx])
							discriminant = strings.TrimSpace(vClean[eqIdx+1:])
						} else {
							vName = vClean
						}
						variants = append(variants, RustEnumVariant{
							Name:         vName,
							Discriminant: discriminant,
						})
					}
				}
				i++
			}

			enums = append(enums, RustEnum{
				Name:       name,
				Visibility: vis,
				Generics:   generics,
				Derives:    pendingDerives,
				Attributes: pendingAttrs,
				Doc:        strings.Join(pendingDoc, "\n"),
				Variants:   variants,
			})

			pendingDoc = nil
			pendingAttrs = nil
			pendingDerives = nil
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
				pendingAttrs = nil
				pendingDerives = nil
			}
		}
		i++
	}

	return enums
}

func (p *RustParser) extractTraits(lines []lineInfo) []RustTrait {
	var traits []RustTrait
	var pendingDoc []string
	var pendingAttrs []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "///") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "///")))
			i++
			continue
		}
		if strings.HasPrefix(trimmed, "#[") {
			pendingAttrs = append(pendingAttrs, trimmed)
			i++
			continue
		}

		if m := traitHeaderRegex.FindStringSubmatch(trimmed); m != nil {
			vis := m[1]
			name := m[2]
			generics := m[3]

			var methods []RustTraitMethod
			braceCount := 1
			i++
			for i < len(lines) && braceCount > 0 {
				mLine := strings.TrimSpace(lines[i].text)
				if strings.Contains(mLine, "{") {
					braceCount++
				}
				if strings.Contains(mLine, "}") {
					braceCount--
					if braceCount == 0 {
						break
					}
				}
				if fnM := fnHeaderRegex.FindStringSubmatch(mLine); fnM != nil {
					isAsync := fnM[2] == "async"
					fnName := fnM[5]
					paramsStr := fnM[7]
					retType := strings.TrimSpace(fnM[8])

					params := parseRustParams(paramsStr)
					methods = append(methods, RustTraitMethod{
						Name:       fnName,
						IsAsync:    isAsync,
						Params:     params,
						ReturnType: retType,
					})
				}
				i++
			}

			traits = append(traits, RustTrait{
				Name:       name,
				Visibility: vis,
				Generics:   generics,
				Doc:        strings.Join(pendingDoc, "\n"),
				Attributes: pendingAttrs,
				Methods:    methods,
			})

			pendingDoc = nil
			pendingAttrs = nil
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
				pendingAttrs = nil
			}
		}
		i++
	}

	return traits
}

func (p *RustParser) extractImpls(lines []lineInfo) []RustImpl {
	var impls []RustImpl
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)
		if m := implHeaderRegex.FindStringSubmatch(trimmed); m != nil && !strings.Contains(trimmed, "fn ") {
			generics := m[1]
			traitName := m[2]
			targetType := m[3]
			if targetType == "" && traitName != "" {
				targetType = traitName
				traitName = ""
			}

			var methods []RustFunction
			braceCount := 1
			i++
			for i < len(lines) && braceCount > 0 {
				mLine := strings.TrimSpace(lines[i].text)
				if strings.Contains(mLine, "{") {
					braceCount++
				}
				if strings.Contains(mLine, "}") {
					braceCount--
					if braceCount == 0 {
						break
					}
				}
				if fnM := fnHeaderRegex.FindStringSubmatch(mLine); fnM != nil {
					vis := fnM[1]
					isAsync := fnM[2] == "async"
					isConst := fnM[3] == "const"
					isUnsafe := fnM[4] == "unsafe"
					fnName := fnM[5]
					fnGenerics := fnM[6]
					paramsStr := fnM[7]
					retType := strings.TrimSpace(fnM[8])

					params := parseRustParams(paramsStr)

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
							innerLine := lines[i].text
							bodyLines = append(bodyLines, innerLine)
							if strings.Contains(innerLine, "{") {
								methodBraces++
								braceCount++
							}
							if strings.Contains(innerLine, "}") {
								methodBraces--
								braceCount--
								if methodBraces == 0 {
									break
								}
							}
						}
					}

					methods = append(methods, RustFunction{
						Name:        fnName,
						Visibility:  vis,
						IsAsync:     isAsync,
						IsConst:     isConst,
						IsUnsafe:    isUnsafe,
						Generics:    fnGenerics,
						Params:      params,
						ReturnType:  retType,
						BodySource:  strings.Join(bodyLines, "\n"),
						CalledFuncs: extractCalledFuncs(strings.Join(bodyLines, "\n")),
					})
				}
				i++
			}

			impls = append(impls, RustImpl{
				TraitName:  traitName,
				TargetType: targetType,
				Generics:   generics,
				Methods:    methods,
			})
		}
		i++
	}
	return impls
}

func (p *RustParser) extractFunctions(lines []lineInfo) []RustFunction {
	var functions []RustFunction
	var pendingDoc []string
	var pendingAttrs []string

	i := 0
	inImplOrTrait := 0

	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if (implHeaderRegex.MatchString(trimmed) || traitHeaderRegex.MatchString(trimmed)) && !strings.Contains(trimmed, "fn ") {
			if strings.Contains(trimmed, "{") {
				inImplOrTrait++
			}
			i++
			continue
		}

		if inImplOrTrait > 0 {
			for _, ch := range trimmed {
				if ch == '{' {
					inImplOrTrait++
				} else if ch == '}' {
					inImplOrTrait--
				}
			}
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "//!") {
			docLine := strings.TrimPrefix(trimmed, "///")
			docLine = strings.TrimPrefix(docLine, "//!")
			pendingDoc = append(pendingDoc, strings.TrimSpace(docLine))
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "#[") {
			pendingAttrs = append(pendingAttrs, trimmed)
			i++
			continue
		}

		if m := fnHeaderRegex.FindStringSubmatch(trimmed); m != nil {
			vis := m[1]
			isAsync := m[2] == "async"
			isConst := m[3] == "const"
			isUnsafe := m[4] == "unsafe"
			name := m[5]
			generics := m[6]
			paramsStr := m[7]
			retType := strings.TrimSpace(m[8])

			params := parseRustParams(paramsStr)

			var routeMethod, routePath string
			for _, attr := range pendingAttrs {
				if rm := actixRouteRegex.FindStringSubmatch(attr); rm != nil {
					routeMethod = strings.ToUpper(rm[1])
					routePath = rm[2]
					break
				}
			}

			// Collect function body
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
			sqlxQueries := p.extractSQLxQueries(fullBody)

			functions = append(functions, RustFunction{
				Name:        name,
				Visibility:  vis,
				IsAsync:     isAsync,
				IsConst:     isConst,
				IsUnsafe:    isUnsafe,
				Generics:    generics,
				Params:      params,
				ReturnType:  retType,
				Doc:         strings.Join(pendingDoc, "\n"),
				Attributes:  pendingAttrs,
				RouteMethod: routeMethod,
				RoutePath:   routePath,
				SQLxQueries: sqlxQueries,
				BodySource:  fullBody,
				CalledFuncs: extractCalledFuncs(fullBody),
			})

			pendingDoc = nil
			pendingAttrs = nil
		} else {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				pendingDoc = nil
				pendingAttrs = nil
			}
		}
		i++
	}

	return functions
}

func (p *RustParser) extractMacros(lines []lineInfo) []RustMacro {
	var macros []RustMacro
	var pendingDoc []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)
		if strings.HasPrefix(trimmed, "///") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "///")))
			i++
			continue
		}

		if m := macroHeaderRegex.FindStringSubmatch(trimmed); m != nil {
			vis := m[1]
			name := m[2]

			var bodyLines []string
			bodyLines = append(bodyLines, lines[i].text)
			braceCount := 0
			if strings.Contains(trimmed, "{") {
				braceCount = 1
			}

			for i+1 < len(lines) && braceCount > 0 {
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
			}

			macros = append(macros, RustMacro{
				Name:       name,
				Visibility: vis,
				Doc:        strings.Join(pendingDoc, "\n"),
				Body:       strings.Join(bodyLines, "\n"),
			})
			pendingDoc = nil
		}
		i++
	}
	return macros
}

func (p *RustParser) extractRoutes(funcs []RustFunction, lines []lineInfo) []RustRouteBinding {
	var routes []RustRouteBinding

	// 1. Extract from function attributes (Actix / Rocket)
	for _, fn := range funcs {
		if fn.RouteMethod != "" && fn.RoutePath != "" {
			routes = append(routes, RustRouteBinding{
				Method:      fn.RouteMethod,
				Path:        fn.RoutePath,
				HandlerName: fn.Name,
				Framework:   "actix-web",
				LineNumber:  1,
			})
		}
	}

	// 2. Extract Axum router patterns: .route("/path", get(handler)) or post(handler)
	axumRouteRegex := regexp.MustCompile(`\.route\(\s*["']([^"']+)["']\s*,\s*(get|post|put|delete|patch)\s*\(\s*([a-zA-Z0-9_:]+)\s*\)\)`)
	for _, l := range lines {
		if m := axumRouteRegex.FindStringSubmatch(l.text); m != nil {
			routes = append(routes, RustRouteBinding{
				Method:      strings.ToUpper(m[2]),
				Path:        m[1],
				HandlerName: m[3],
				Framework:   "axum",
				LineNumber:  l.num,
			})
		}
	}

	return routes
}

func parseRustParams(s string) []RustParam {
	var params []RustParam
	s = strings.TrimSpace(s)
	if s == "" {
		return params
	}

	parts := splitTopLevelCommas(s)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "&self" || part == "&mut self" || part == "self" {
			params = append(params, RustParam{
				Name: part,
				Type: "self",
			})
			continue
		}
		colonIdx := strings.Index(part, ":")
		if colonIdx != -1 {
			pName := strings.TrimSpace(part[:colonIdx])
			pType := strings.TrimSpace(part[colonIdx+1:])
			params = append(params, RustParam{
				Name: pName,
				Type: pType,
			})
		} else {
			params = append(params, RustParam{
				Name: part,
				Type: "any",
			})
		}
	}
	return params
}

func splitTopLevelCommas(s string) []string {
	var items []string
	var current strings.Builder
	depth := 0
	for _, ch := range s {
		switch ch {
		case '<', '(', '[', '{':
			depth++
			current.WriteRune(ch)
		case '>', ')', ']', '}':
			depth--
			current.WriteRune(ch)
		case ',':
			if depth == 0 {
				items = append(items, current.String())
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		items = append(items, current.String())
	}
	return items
}

var callRegex = regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*\(`)

func extractCalledFuncs(src string) []string {
	var funcs []string
	seen := make(map[string]bool)
	matches := callRegex.FindAllStringSubmatch(src, -1)
	for _, m := range matches {
		name := m[1]
		if !seen[name] && name != "fn" && name != "if" && name != "match" && name != "for" && name != "while" {
			seen[name] = true
			funcs = append(funcs, name)
		}
	}
	sort.Strings(funcs)
	return funcs
}

func (p *RustParser) toSymbolNodes(res *RustFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Structs
	for _, s := range res.Structs {
		node, err := StructToASTSymbolNode(s, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Enums
	for _, e := range res.Enums {
		node, err := EnumToASTSymbolNode(e, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Traits
	for _, t := range res.Traits {
		node, err := TraitToASTSymbolNode(t, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Impls
	for _, imp := range res.Impls {
		node, err := ImplToASTSymbolNode(imp, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Top-level Functions
	for _, fn := range res.Functions {
		node, err := FuncToASTSymbolNode(fn, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Macros
	for _, m := range res.Macros {
		node, err := MacroToASTSymbolNode(m, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Routes
	for _, r := range res.Routes {
		node, err := RouteToASTSymbolNode(r, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// StructToASTSymbolNode converts a RustStruct to an ASTSymbolNode.
func StructToASTSymbolNode(s RustStruct, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshaling Rust struct %s: %w", s.Name, err)
	}

	var deps []string
	for _, f := range s.Fields {
		cleanType := strings.TrimLeft(f.Type, "&*[]Option<Vec<>")
		if cleanType != "" {
			deps = append(deps, cleanType)
		}
	}

	meta := map[string]string{
		"struct_name": s.Name,
		"field_count": fmt.Sprintf("%d", len(s.Fields)),
	}
	if len(s.Derives) > 0 {
		meta["derives"] = strings.Join(s.Derives, ",")
	}

	vis := s.Visibility
	if vis == "" {
		vis = "private"
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangRust,
		NodeType:          "StructDef",
		Identifier:        fmt.Sprintf("%s::%s", pkgName, s.Name),
		Signature:         fmt.Sprintf("struct %s%s", s.Name, s.Generics),
		Docstring:         s.Doc,
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
	return node, nil
}

// EnumToASTSymbolNode converts a RustEnum to an ASTSymbolNode.
func EnumToASTSymbolNode(e RustEnum, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("marshaling Rust enum %s: %w", e.Name, err)
	}

	vis := e.Visibility
	if vis == "" {
		vis = "private"
	}

	node := &core.ASTSymbolNode{
		Language:    core.LangRust,
		NodeType:    "EnumDef",
		Identifier:  fmt.Sprintf("%s::%s", pkgName, e.Name),
		Signature:   fmt.Sprintf("enum %s%s", e.Name, e.Generics),
		Docstring:   e.Doc,
		Visibility:  vis,
		ASTPayload:  payload,
		ASTMetadata: map[string]string{"variant_count": fmt.Sprintf("%d", len(e.Variants))},
		Lineage:     lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// TraitToASTSymbolNode converts a RustTrait to an ASTSymbolNode.
func TraitToASTSymbolNode(t RustTrait, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("marshaling Rust trait %s: %w", t.Name, err)
	}

	vis := t.Visibility
	if vis == "" {
		vis = "private"
	}

	node := &core.ASTSymbolNode{
		Language:    core.LangRust,
		NodeType:    "TraitDef",
		Identifier:  fmt.Sprintf("%s::%s", pkgName, t.Name),
		Signature:   fmt.Sprintf("trait %s%s", t.Name, t.Generics),
		Docstring:   t.Doc,
		Visibility:  vis,
		ASTPayload:  payload,
		ASTMetadata: map[string]string{"method_count": fmt.Sprintf("%d", len(t.Methods))},
		Lineage:     lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// ImplToASTSymbolNode converts a RustImpl to an ASTSymbolNode.
func ImplToASTSymbolNode(imp RustImpl, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(imp)
	if err != nil {
		return nil, fmt.Errorf("marshaling Rust impl for %s: %w", imp.TargetType, err)
	}

	ident := fmt.Sprintf("%s::impl %s", pkgName, imp.TargetType)
	sig := fmt.Sprintf("impl %s", imp.TargetType)
	if imp.TraitName != "" {
		ident = fmt.Sprintf("%s::impl %s for %s", pkgName, imp.TraitName, imp.TargetType)
		sig = fmt.Sprintf("impl %s for %s", imp.TraitName, imp.TargetType)
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangRust,
		NodeType:          "ImplBlock",
		Identifier:        ident,
		Signature:         sig,
		ASTPayload:        payload,
		ASTMetadata:       map[string]string{"method_count": fmt.Sprintf("%d", len(imp.Methods))},
		LocalDependencies: []string{imp.TargetType},
		Dependencies:      []string{imp.TargetType},
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// FuncToASTSymbolNode converts a RustFunction to an ASTSymbolNode.
func FuncToASTSymbolNode(fn RustFunction, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(fn)
	if err != nil {
		return nil, fmt.Errorf("marshaling Rust func %s: %w", fn.Name, err)
	}

	vis := fn.Visibility
	if vis == "" {
		vis = "private"
	}

	meta := make(map[string]string)
	if fn.RouteMethod != "" {
		meta["route_method"] = fn.RouteMethod
		meta["route_path"] = fn.RoutePath
	}
	if len(fn.SQLxQueries) > 0 {
		meta["sql_queries"] = strings.Join(fn.SQLxQueries, ";;")
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangRust,
		NodeType:          "FnDef",
		Identifier:        fmt.Sprintf("%s::%s", pkgName, fn.Name),
		Signature:         fmt.Sprintf("fn %s%s(...) -> %s", fn.Name, fn.Generics, fn.ReturnType),
		Docstring:         fn.Doc,
		Visibility:        vis,
		ASTPayload:        payload,
		ASTMetadata:       meta,
		LocalDependencies: fn.CalledFuncs,
		Dependencies:      fn.CalledFuncs,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// MacroToASTSymbolNode converts a RustMacro to an ASTSymbolNode.
func MacroToASTSymbolNode(m RustMacro, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshaling Rust macro %s: %w", m.Name, err)
	}

	node := &core.ASTSymbolNode{
		Language:    core.LangRust,
		NodeType:    "MacroDef",
		Identifier:  fmt.Sprintf("%s::%s!", pkgName, m.Name),
		Signature:   fmt.Sprintf("macro_rules! %s", m.Name),
		Docstring:   m.Doc,
		ASTPayload:  payload,
		Lineage:     lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// RouteToASTSymbolNode converts a RustRouteBinding to an ASTSymbolNode.
func RouteToASTSymbolNode(r RustRouteBinding, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("marshaling Rust route %s: %w", r.Path, err)
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangRust,
		NodeType:          "RouteBinding",
		Identifier:        fmt.Sprintf("%s:%s %s", pkgName, r.Method, r.Path),
		Signature:         fmt.Sprintf("%s %s", r.Method, r.Path),
		ASTPayload:        payload,
		ASTMetadata:       map[string]string{"framework": r.Framework, "handler": r.HandlerName},
		LocalDependencies: []string{r.HandlerName},
		Dependencies:      []string{r.HandlerName},
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// BuildComponentNode bundles parsed Rust symbols into a core.ComponentNode.
func (p *RustParser) BuildComponentNode(
	res *RustFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = res.PackageName
	}
	if !compType.IsValid() {
		compType = core.CompService
	}

	symbolIDs := make([]string, 0, len(res.AllSymbols))
	for _, sym := range res.AllSymbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	metadata := map[string]string{
		"file_path":     res.FilePath,
		"package_name":  res.PackageName,
		"symbol_count":  fmt.Sprintf("%d", len(res.AllSymbols)),
		"route_count":   fmt.Sprintf("%d", len(res.Routes)),
		"struct_count":  fmt.Sprintf("%d", len(res.Structs)),
		"func_count":    fmt.Sprintf("%d", len(res.Functions)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangRust,
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
