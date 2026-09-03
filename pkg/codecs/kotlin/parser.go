package kotlin

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

// KotlinProperty represents a property declaration in Kotlin.
type KotlinProperty struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Visibility   string   `json:"visibility,omitempty"` // public, private, protected, internal
	IsVal        bool     `json:"is_val"`
	IsVar        bool     `json:"is_var"`
	IsOverride   bool     `json:"is_override"`
	IsLateinit   bool     `json:"is_lateinit"`
	Annotations  []string `json:"annotations,omitempty"`
	Doc          string   `json:"doc,omitempty"`
	DefaultValue string   `json:"default_value,omitempty"`
}

// KotlinParam represents a function or constructor parameter.
type KotlinParam struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	DefaultValue string   `json:"default_value,omitempty"`
	IsVal        bool     `json:"is_val,omitempty"`
	IsVar        bool     `json:"is_var,omitempty"`
	Annotations  []string `json:"annotations,omitempty"`
}

// KotlinFunction represents a function or method.
type KotlinFunction struct {
	Name            string        `json:"name"`
	Visibility      string        `json:"visibility,omitempty"`
	IsSuspend       bool          `json:"is_suspend"`
	IsInline        bool          `json:"is_inline"`
	IsOverride      bool          `json:"is_override"`
	IsOpen          bool          `json:"is_open"`
	IsAbstract      bool          `json:"is_abstract"`
	IsComposable    bool          `json:"is_composable"`
	ReturnType      string        `json:"return_type,omitempty"`
	Params          []KotlinParam `json:"params"`
	Annotations     []string      `json:"annotations,omitempty"`
	Doc             string        `json:"doc,omitempty"`
	BodySource      string        `json:"body_source,omitempty"`
	CalledFunctions []string      `json:"called_functions,omitempty"`
	RouteMethod     string        `json:"route_method,omitempty"` // GET, POST, PUT, DELETE, PATCH
	RoutePath       string        `json:"route_path,omitempty"`
	Framework       string        `json:"framework,omitempty"`
}

// KotlinClass represents a class, data class, interface, object, enum class, or sealed class.
type KotlinClass struct {
	Kind                     string           `json:"kind"` // class, data class, interface, object, enum class, sealed class
	Name                     string           `json:"name"`
	PackageName              string           `json:"package_name"`
	Visibility               string           `json:"visibility,omitempty"`
	IsOpen                   bool             `json:"is_open"`
	IsAbstract               bool             `json:"is_abstract"`
	IsSealed                 bool             `json:"is_sealed"`
	PrimaryConstructorParams []KotlinParam    `json:"primary_constructor_params,omitempty"`
	SuperTypes               []string         `json:"super_types,omitempty"`
	Annotations              []string         `json:"annotations,omitempty"`
	Doc                      string           `json:"doc,omitempty"`
	Properties               []KotlinProperty `json:"properties"`
	Functions                []KotlinFunction `json:"functions"`
	IsController             bool             `json:"is_controller"`
	BasePath                 string           `json:"base_path,omitempty"`
	BodySource               string           `json:"body_source,omitempty"`
}

// KotlinRouteBinding represents a Spring Boot or Ktor REST route endpoint.
type KotlinRouteBinding struct {
	Method      string `json:"method"`       // GET, POST, PUT, DELETE, PATCH, ANY
	Path        string `json:"path"`         // /api/v1/users
	HandlerName string `json:"handler_name"` // UserController.getUser or ktor route
	Framework   string `json:"framework"`    // spring-boot, ktor
	LineNumber  int    `json:"line_number"`
}

// KotlinAPICall represents an HTTP/REST endpoint consumer (Ktor client, Retrofit, OkHttp).
type KotlinAPICall struct {
	Method     string `json:"method"` // GET, POST, PUT, DELETE
	URL        string `json:"url"`    // /api/v1/users
	Caller     string `json:"caller"` // httpClient.get, @GET, etc.
	LineNumber int    `json:"line_number"`
}

// KotlinFileResult contains all extracted symbols and metadata from a Kotlin source file.
type KotlinFileResult struct {
	FilePath           string                `json:"file_path"`
	PackageName        string                `json:"package_name"`
	Imports            []string              `json:"imports"`
	Classes            []KotlinClass         `json:"classes"`
	TopLevelFunctions  []KotlinFunction      `json:"top_level_functions"`
	TopLevelProperties []KotlinProperty      `json:"top_level_properties"`
	Routes             []KotlinRouteBinding  `json:"routes"`
	APICalls           []KotlinAPICall       `json:"api_calls"`
	AllSymbols         []*core.ASTSymbolNode `json:"all_symbols"`
}

// KotlinParser extracts Kotlin AST symbols, Compose UI functions, Spring / Ktor routes.
type KotlinParser struct{}

// NewKotlinParser creates an initialized KotlinParser instance.
func NewKotlinParser() *KotlinParser {
	return &KotlinParser{}
}

// ParseSource parses Kotlin source code bytes.
func (p *KotlinParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*KotlinFileResult, error) {
	if filename == "" {
		filename = "Source.kt"
	}

	result := &KotlinFileResult{
		FilePath: filename,
	}

	lines := splitKotlinLines(src)

	// 1. Extract Package and Imports
	result.PackageName = p.extractPackage(lines)
	result.Imports = p.extractImports(lines)

	// 2. Extract API client calls (Retrofit / Ktor Client)
	result.APICalls = p.extractAPICalls(lines)

	// 3. Extract Classes/Interfaces/Objects/Data Classes
	result.Classes = p.extractClasses(lines, result.PackageName)

	// 4. Extract Top-level Functions
	result.TopLevelFunctions = p.extractTopLevelFunctions(lines)

	// 5. Extract Routes (Spring Boot & Ktor)
	result.Routes = p.extractRoutes(result.Classes, lines)

	// 6. Convert to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a Kotlin source file from disk.
func (p *KotlinParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*KotlinFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .kt and .kts files in a directory.
func (p *KotlinParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*KotlinFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*KotlinFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext == ".kt" || ext == ".kts" {
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

type kotlinLine struct {
	num  int
	text string
}

func splitKotlinLines(src []byte) []kotlinLine {
	var lines []kotlinLine
	scanner := bufio.NewScanner(bytes.NewReader(src))
	num := 1
	for scanner.Scan() {
		lines = append(lines, kotlinLine{
			num:  num,
			text: scanner.Text(),
		})
		num++
	}
	return lines
}

var (
	ktPackageRegex   = regexp.MustCompile(`^package\s+([a-zA-Z0-9_.]+)(?:\s*;)?`)
	ktImportRegex    = regexp.MustCompile(`^import\s+([a-zA-Z0-9_.*]+)(?:\s*;)?`)
	ktClassHeaderReg = regexp.MustCompile(`^(?:(public|private|protected|internal)\s+)?(?:(abstract|final|open|sealed|data|enum|value|annotation|inline)\s+)*(class|interface|object|enum\s+class|data\s+class|sealed\s+class|sealed\s+interface)\s+([A-Za-z0-9_]+)(?:<[^>]+>)?(?:\s*\((.*?)\))?(?:\s*:\s*([A-Za-z0-9_.,\s<>():]+))?`)
	ktFuncHeaderReg  = regexp.MustCompile(`^(?:(public|private|protected|internal)\s+)?(?:(abstract|final|open|override|suspend|inline|operator|tailrec)\s+)*fun\s+(?:<[^>]+>\s+)?([A-Za-z0-9_]+)\s*\((.*?)\)(?:\s*:\s*([A-Za-z0-9_?<>\[\]:.\s]+?))?\s*(\{|=\s*|$)`)
	ktPropHeaderReg  = regexp.MustCompile(`^(?:(public|private|protected|internal)\s+)?(?:(override|lateinit|const|open|final)\s+)*(val|var)\s+([A-Za-z0-9_]+)(?:\s*:\s*([A-Za-z0-9_?<>\[\]:.\s]+?))?(?:\s*=\s*(.+))?$`)
	ktReqMapRegex    = regexp.MustCompile(`@(RequestMapping|GetMapping|PostMapping|PutMapping|DeleteMapping|PatchMapping)\s*(?:\(\s*(?:(?:value|path)\s*=\s*)?["']([^"']*)["']|\s*\(\s*["']([^"']*)["']|\s*\(\s*\)|\s*$)`)
	ktKtorRouteRegex = regexp.MustCompile(`\b(get|post|put|delete|patch|route)\s*\(\s*["']([^"']+)["']\s*\)`)
	ktRetrofitRegex  = regexp.MustCompile(`@(GET|POST|PUT|DELETE|PATCH)\s*\(\s*["']([^"']+)["']\s*\)`)
	ktKtorClientReg  = regexp.MustCompile(`(?:client|httpClient)\s*\.\s*(get|post|put|delete|patch)\s*(?:<[^>]+>)?\s*\(\s*["']([^"']+)["']`)
)

func (p *KotlinParser) extractPackage(lines []kotlinLine) string {
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := ktPackageRegex.FindStringSubmatch(trimmed); m != nil {
			return m[1]
		}
	}
	return ""
}

func (p *KotlinParser) extractImports(lines []kotlinLine) []string {
	var imports []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := ktImportRegex.FindStringSubmatch(trimmed); m != nil {
			imports = append(imports, m[1])
		}
	}
	return imports
}

func (p *KotlinParser) extractAPICalls(lines []kotlinLine) []KotlinAPICall {
	var calls []KotlinAPICall

	for _, l := range lines {
		text := l.text
		// 1. Ktor client call: client.get("...")
		if m := ktKtorClientReg.FindStringSubmatch(text); m != nil {
			calls = append(calls, KotlinAPICall{
				Method:     strings.ToUpper(m[1]),
				URL:        m[2],
				Caller:     "KtorHttpClient",
				LineNumber: l.num,
			})
		}
		// 2. Retrofit annotation: @GET("/api/v1/users")
		if m := ktRetrofitRegex.FindStringSubmatch(text); m != nil {
			calls = append(calls, KotlinAPICall{
				Method:     strings.ToUpper(m[1]),
				URL:        m[2],
				Caller:     "Retrofit",
				LineNumber: l.num,
			})
		}
	}

	return calls
}

func (p *KotlinParser) extractClasses(lines []kotlinLine, pkgName string) []KotlinClass {
	var classes []KotlinClass
	var pendingDoc []string
	var pendingAnnotations []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		// Comments / docstrings
		if strings.HasPrefix(trimmed, "/**") || strings.HasPrefix(trimmed, "*") || (strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "///")) {
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

		// Annotations
		if strings.HasPrefix(trimmed, "@") && !strings.Contains(trimmed, "fun ") && !strings.Contains(trimmed, "val ") && !strings.Contains(trimmed, "var ") {
			pendingAnnotations = append(pendingAnnotations, trimmed)
			i++
			continue
		}

		if (strings.Contains(trimmed, "class ") || strings.Contains(trimmed, "interface ") || strings.Contains(trimmed, "object ")) && !strings.HasPrefix(trimmed, "fun ") {
			// Collect multi-line header until we find opening '{' or end of primary constructor/inheritance
			headerLines := []string{lines[i].text}
			headerText := trimmed

			parenCount := strings.Count(trimmed, "(") - strings.Count(trimmed, ")")
			hasBrace := strings.Contains(trimmed, "{")

			for (parenCount > 0 || (!hasBrace && !strings.HasSuffix(headerText, ")") && !strings.Contains(headerText, "interface ") && !strings.Contains(headerText, "object "))) && i+1 < len(lines) {
				nextLine := lines[i+1].text
				nextTrim := strings.TrimSpace(nextLine)
				// Break if next line is clearly another declaration or annotation
				if strings.HasPrefix(nextTrim, "@") || strings.HasPrefix(nextTrim, "fun ") || strings.HasPrefix(nextTrim, "class ") || strings.HasPrefix(nextTrim, "data class ") {
					break
				}
				i++
				headerLines = append(headerLines, nextLine)
				headerText = headerText + " " + nextTrim
				parenCount += strings.Count(nextTrim, "(") - strings.Count(nextTrim, ")")
				if strings.Contains(nextTrim, "{") {
					hasBrace = true
					break
				}
				if parenCount == 0 && (strings.HasSuffix(nextTrim, ")") || strings.Contains(nextTrim, ":")) {
					// Check if next line has '{'
					if i+1 < len(lines) && strings.TrimSpace(lines[i+1].text) == "{" {
						i++
						headerLines = append(headerLines, lines[i].text)
						hasBrace = true
					}
					break
				}
			}

			// Parse header
			kind := "class"
			if strings.Contains(headerText, "data class") {
				kind = "data class"
			} else if strings.Contains(headerText, "sealed class") {
				kind = "sealed class"
			} else if strings.Contains(headerText, "enum class") {
				kind = "enum class"
			} else if strings.Contains(headerText, "sealed interface") {
				kind = "sealed interface"
			} else if strings.Contains(headerText, "interface") {
				kind = "interface"
			} else if strings.Contains(headerText, "object") {
				kind = "object"
			}

			name := ""
			words := strings.Fields(headerText)
			for idx, w := range words {
				if w == "class" || w == "interface" || w == "object" {
					if idx+1 < len(words) {
						target := words[idx+1]
						target = strings.Split(target, "(")[0]
						target = strings.Split(target, "<")[0]
						target = strings.Split(target, ":")[0]
						target = strings.TrimSpace(target)
						name = target
						break
					}
				}
			}

			if name == "" {
				i++
				continue
			}

			vis := ""
			for _, v := range []string{"public", "private", "protected", "internal"} {
				if strings.Contains(headerText, v+" ") {
					vis = v
					break
				}
			}

			// Extract constructor parameters from headerText
			var constructorParams []KotlinParam
			if strings.Contains(headerText, "(") && strings.Contains(headerText, ")") {
				startIdx := strings.Index(headerText, "(")
				endIdx := strings.LastIndex(headerText, ")")
				if endIdx > startIdx {
					paramStr := headerText[startIdx+1 : endIdx]
					constructorParams = parseKotlinParams(paramStr)
				}
			}

			// Super types
			var superTypes []string
			if strings.Contains(headerText, ":") {
				colonIdx := strings.Index(headerText, ":")
				afterColon := headerText[colonIdx+1:]
				afterColon = strings.TrimSuffix(strings.TrimSpace(afterColon), "{")
				for _, st := range strings.Split(afterColon, ",") {
					st = strings.TrimSpace(st)
					if st != "" {
						superTypes = append(superTypes, st)
					}
				}
			}

			isController := false
			basePath := ""
			for _, ann := range pendingAnnotations {
				if strings.Contains(ann, "@RestController") || strings.Contains(ann, "@Controller") {
					isController = true
				}
				if rm := ktReqMapRegex.FindStringSubmatch(ann); rm != nil {
					if rm[2] != "" {
						basePath = rm[2]
					} else if rm[3] != "" {
						basePath = rm[3]
					}
				}
			}

			var bodyLines []string
			bodyLines = append(bodyLines, headerLines...)

			var properties []KotlinProperty
			var functions []KotlinFunction

			var memberDoc []string
			var memberAnnotations []string

			braceCount := 0
			for _, hl := range headerLines {
				braceCount += strings.Count(hl, "{") - strings.Count(hl, "}")
			}

			if braceCount > 0 {
				i++
				for i < len(lines) {
					mLine := lines[i].text
					mTrim := strings.TrimSpace(mLine)

					if strings.HasPrefix(mTrim, "/**") || strings.HasPrefix(mTrim, "*") || strings.HasPrefix(mTrim, "//") {
						docLine := strings.TrimPrefix(mTrim, "/**")
						docLine = strings.TrimPrefix(docLine, "*/")
						docLine = strings.TrimPrefix(docLine, "*")
						docLine = strings.TrimPrefix(docLine, "//")
						docClean := strings.TrimSpace(docLine)
						if docClean != "" {
							memberDoc = append(memberDoc, docClean)
						}
						bodyLines = append(bodyLines, mLine)
						i++
						continue
					}

					if strings.HasPrefix(mTrim, "@") && !strings.Contains(mTrim, "fun ") && !strings.Contains(mTrim, "val ") && !strings.Contains(mTrim, "var ") {
						memberAnnotations = append(memberAnnotations, mTrim)
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

					// Method / Function
					if fm := ktFuncHeaderReg.FindStringSubmatch(mTrim); fm != nil {
						fVis := fm[1]
						fName := fm[3]
						fParamsStr := fm[4]
						fRetType := strings.TrimSpace(fm[5])

						isSuspend := strings.Contains(mTrim, "suspend ")
						isInline := strings.Contains(mTrim, "inline ")
						isOverride := strings.Contains(mTrim, "override ")
						isComposable := false

						var routeMethod, routePath string
						for _, ann := range memberAnnotations {
							if strings.Contains(ann, "@Composable") {
								isComposable = true
							}
							if rm := ktReqMapRegex.FindStringSubmatch(ann); rm != nil {
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
						}

						var fnBodyLines []string
						fnBraces := 0
						if strings.Contains(mTrim, "{") {
							fnBraces = strings.Count(mTrim, "{") - strings.Count(mTrim, "}")
						}
						fnBodyLines = append(fnBodyLines, mLine)

						if fnBraces > 0 {
							for i+1 < len(lines) {
								i++
								fbLine := lines[i].text
								fnBodyLines = append(fnBodyLines, fbLine)
								bodyLines = append(bodyLines, fbLine)

								if strings.Contains(fbLine, "{") {
									fnBraces += strings.Count(fbLine, "{")
									braceCount += strings.Count(fbLine, "{")
								}
								if strings.Contains(fbLine, "}") {
									fnBraces -= strings.Count(fbLine, "}")
									braceCount -= strings.Count(fbLine, "}")
									if fnBraces <= 0 {
										break
									}
								}
							}
						}

						fullFnBody := strings.Join(fnBodyLines, "\n")
						framework := ""
						if routeMethod != "" {
							framework = "spring-boot"
						}

						functions = append(functions, KotlinFunction{
							Name:            fName,
							Visibility:      fVis,
							IsSuspend:       isSuspend,
							IsInline:        isInline,
							IsOverride:      isOverride,
							IsComposable:    isComposable,
							ReturnType:      fRetType,
							Params:          parseKotlinParams(fParamsStr),
							Annotations:     memberAnnotations,
							Doc:             strings.Join(memberDoc, "\n"),
							BodySource:      fullFnBody,
							CalledFunctions: extractKotlinCalls(fullFnBody),
							RouteMethod:     routeMethod,
							RoutePath:       routePath,
							Framework:       framework,
						})

						memberDoc = nil
						memberAnnotations = nil
						i++
						continue
					}

					// Property
					if pm := ktPropHeaderReg.FindStringSubmatch(mTrim); pm != nil {
						pVis := pm[1]
						pMod := pm[2]
						pValVar := pm[3]
						pName := pm[4]
						pType := strings.TrimSpace(pm[5])
						pDefVal := strings.TrimSpace(pm[6])

						properties = append(properties, KotlinProperty{
							Name:         pName,
							Type:         pType,
							Visibility:   pVis,
							IsVal:        pValVar == "val",
							IsVar:        pValVar == "var",
							IsOverride:   strings.Contains(pMod, "override"),
							IsLateinit:   strings.Contains(pMod, "lateinit"),
							Annotations:  memberAnnotations,
							Doc:          strings.Join(memberDoc, "\n"),
							DefaultValue: pDefVal,
						})

						memberDoc = nil
						memberAnnotations = nil
						i++
						continue
					}

					memberDoc = nil
					memberAnnotations = nil
					i++
				}
			}

			classes = append(classes, KotlinClass{
				Kind:                     kind,
				Name:                     name,
				PackageName:              pkgName,
				Visibility:               vis,
				IsOpen:                   strings.Contains(headerText, "open "),
				IsAbstract:               strings.Contains(headerText, "abstract "),
				IsSealed:                 strings.Contains(headerText, "sealed "),
				PrimaryConstructorParams: constructorParams,
				SuperTypes:               superTypes,
				Annotations:              pendingAnnotations,
				Doc:                      strings.Join(pendingDoc, "\n"),
				Properties:               properties,
				Functions:                functions,
				IsController:             isController,
				BasePath:                 basePath,
				BodySource:               strings.Join(bodyLines, "\n"),
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

func (p *KotlinParser) extractTopLevelFunctions(lines []kotlinLine) []KotlinFunction {
	var funcs []KotlinFunction
	var pendingDoc []string
	var pendingAnnotations []string

	i := 0
	braceDepth := 0
	for i < len(lines) {
		line := lines[i].text
		trimmed := strings.TrimSpace(line)

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

		if strings.HasPrefix(trimmed, "@") && !strings.Contains(trimmed, "fun ") {
			pendingAnnotations = append(pendingAnnotations, trimmed)
			i++
			continue
		}

		// Top-level function can only be declared when braceDepth == 0
		if braceDepth == 0 {
			if fm := ktFuncHeaderReg.FindStringSubmatch(trimmed); fm != nil {
				fVis := fm[1]
				fName := fm[3]
				fParamsStr := fm[4]
				fRetType := strings.TrimSpace(fm[5])

				isSuspend := strings.Contains(trimmed, "suspend ")
				isInline := strings.Contains(trimmed, "inline ")
				isComposable := false

				for _, ann := range pendingAnnotations {
					if strings.Contains(ann, "@Composable") {
						isComposable = true
					}
				}

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
				funcs = append(funcs, KotlinFunction{
					Name:            fName,
					Visibility:      fVis,
					IsSuspend:       isSuspend,
					IsInline:        isInline,
					IsComposable:    isComposable,
					ReturnType:      fRetType,
					Params:          parseKotlinParams(fParamsStr),
					Annotations:     pendingAnnotations,
					Doc:             strings.Join(pendingDoc, "\n"),
					BodySource:      fullBody,
					CalledFunctions: extractKotlinCalls(fullBody),
				})

				pendingDoc = nil
				pendingAnnotations = nil
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
		pendingAnnotations = nil
		i++
	}

	return funcs
}

func (p *KotlinParser) extractRoutes(classes []KotlinClass, lines []kotlinLine) []KotlinRouteBinding {
	var routes []KotlinRouteBinding

	// 1. Spring Boot Routes
	for _, cls := range classes {
		for _, fn := range cls.Functions {
			if fn.RouteMethod != "" {
				fullPath := cls.BasePath
				if fn.RoutePath != "" {
					if !strings.HasPrefix(fn.RoutePath, "/") && fullPath != "" {
						fullPath = fullPath + "/" + fn.RoutePath
					} else {
						fullPath = fullPath + fn.RoutePath
					}
				}
				if fullPath == "" {
					fullPath = "/"
				}

				routes = append(routes, KotlinRouteBinding{
					Method:      fn.RouteMethod,
					Path:        fullPath,
					HandlerName: fmt.Sprintf("%s.%s", cls.Name, fn.Name),
					Framework:   "spring-boot",
					LineNumber:  1,
				})
			}
		}
	}

	// 2. Ktor Routing DSL (e.g. get("/api/v1/users") { ... })
	for _, l := range lines {
		if km := ktKtorRouteRegex.FindStringSubmatch(l.text); km != nil {
			method := strings.ToUpper(km[1])
			path := km[2]
			if method == "ROUTE" {
				continue
			}
			routes = append(routes, KotlinRouteBinding{
				Method:      method,
				Path:        path,
				HandlerName: fmt.Sprintf("ktor_%s_%s", strings.ToLower(method), strings.ReplaceAll(strings.Trim(path, "/"), "/", "_")),
				Framework:   "ktor",
				LineNumber:  l.num,
			})
		}
	}

	return routes
}

func parseKotlinParams(s string) []KotlinParam {
	var params []KotlinParam
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

		var annotations []string
		tokens := strings.Fields(p)
		var nonAttrTokens []string
		for _, tok := range tokens {
			if strings.HasPrefix(tok, "@") {
				annotations = append(annotations, tok)
			} else {
				nonAttrTokens = append(nonAttrTokens, tok)
			}
		}

		joined := strings.Join(nonAttrTokens, " ")
		isVal := strings.Contains(joined, "val ")
		isVar := strings.Contains(joined, "var ")
		joined = strings.TrimPrefix(joined, "val ")
		joined = strings.TrimPrefix(joined, "var ")
		joined = strings.TrimSpace(joined)

		colonParts := strings.SplitN(joined, ":", 2)
		if len(colonParts) == 2 {
			name := strings.TrimSpace(colonParts[0])
			typeAndDef := strings.TrimSpace(colonParts[1])
			defVal := ""
			pType := typeAndDef
			if strings.Contains(typeAndDef, "=") {
				eqParts := strings.SplitN(typeAndDef, "=", 2)
				pType = strings.TrimSpace(eqParts[0])
				defVal = strings.TrimSpace(eqParts[1])
			}

			params = append(params, KotlinParam{
				Name:         name,
				Type:         pType,
				DefaultValue: defVal,
				IsVal:        isVal,
				IsVar:        isVar,
				Annotations:  annotations,
			})
		} else if len(colonParts) == 1 && colonParts[0] != "" {
			params = append(params, KotlinParam{
				Name:        colonParts[0],
				Type:        "Any",
				Annotations: annotations,
			})
		}
	}
	return params
}

var ktCallRegex = regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*\(`)

func extractKotlinCalls(src string) []string {
	var calls []string
	seen := make(map[string]bool)
	matches := ktCallRegex.FindAllStringSubmatch(src, -1)
	for _, m := range matches {
		name := m[1]
		if !seen[name] && name != "if" && name != "while" && name != "for" && name != "when" && name != "catch" {
			seen[name] = true
			calls = append(calls, name)
		}
	}
	sort.Strings(calls)
	return calls
}

func (p *KotlinParser) toSymbolNodes(res *KotlinFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Classes
	for _, cls := range res.Classes {
		payload, err := json.Marshal(cls)
		if err != nil {
			return nil, err
		}

		var deps []string
		deps = append(deps, cls.SuperTypes...)
		for _, prop := range cls.Properties {
			if prop.Type != "" {
				deps = append(deps, prop.Type)
			}
		}
		for _, fn := range cls.Functions {
			if fn.ReturnType != "" {
				deps = append(deps, fn.ReturnType)
			}
			for _, param := range fn.Params {
				deps = append(deps, param.Type)
			}
			deps = append(deps, fn.CalledFunctions...)
		}
		sort.Strings(deps)

		nodeType := "ClassDecl"
		switch cls.Kind {
		case "data class":
			nodeType = "DataClassDecl"
		case "interface", "sealed interface":
			nodeType = "InterfaceDecl"
		case "object":
			nodeType = "ObjectDecl"
		case "enum class":
			nodeType = "EnumClassDecl"
		case "sealed class":
			nodeType = "SealedClassDecl"
		}

		meta := map[string]string{
			"kind":           cls.Kind,
			"property_count": fmt.Sprintf("%d", len(cls.Properties)),
			"function_count": fmt.Sprintf("%d", len(cls.Functions)),
		}
		if cls.IsController {
			meta["is_controller"] = "true"
			if cls.BasePath != "" {
				meta["base_path"] = cls.BasePath
			}
		}

		ident := cls.Name
		if cls.PackageName != "" {
			ident = fmt.Sprintf("%s.%s", cls.PackageName, cls.Name)
		}

		vis := cls.Visibility
		if vis == "" {
			vis = "public"
		}

		node := &core.ASTSymbolNode{
			Language:          core.LangKotlin,
			NodeType:          nodeType,
			Identifier:        ident,
			Signature:         fmt.Sprintf("%s %s", cls.Kind, cls.Name),
			Docstring:         cls.Doc,
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

		// Member Functions
		for _, fn := range cls.Functions {
			fnPayload, err := json.Marshal(fn)
			if err != nil {
				continue
			}

			fnMeta := make(map[string]string)
			if fn.RouteMethod != "" {
				fnMeta["route_method"] = fn.RouteMethod
				fnMeta["route_path"] = fn.RoutePath
			}
			if fn.IsComposable {
				fnMeta["is_composable"] = "true"
				fnMeta["framework"] = "jetpack-compose"
			}

			fnNodeType := "MethodDeclaration"
			if fn.IsComposable {
				fnNodeType = "ComposableFunction"
			}

			fnVis := fn.Visibility
			if fnVis == "" {
				fnVis = "public"
			}

			fnIdent := fmt.Sprintf("%s.%s", ident, fn.Name)
			fnNode := &core.ASTSymbolNode{
				Language:          core.LangKotlin,
				NodeType:          fnNodeType,
				Identifier:        fnIdent,
				Signature:         fmt.Sprintf("fun %s(%s)", fn.Name, formatKotlinParams(fn.Params)),
				Docstring:         fn.Doc,
				Visibility:        fnVis,
				ASTPayload:        fnPayload,
				ASTMetadata:       fnMeta,
				LocalDependencies: fn.CalledFunctions,
				Dependencies:      fn.CalledFunctions,
				Lineage:           lineage,
			}

			fnNodeID, err := core.HashASTSymbolNode(fnNode)
			if err == nil {
				fnNode.NodeID = fnNodeID
				nodes = append(nodes, fnNode)
			}
		}
	}

	// Top-level functions
	for _, fn := range res.TopLevelFunctions {
		payload, err := json.Marshal(fn)
		if err != nil {
			continue
		}

		meta := map[string]string{"is_top_level": "true"}
		nodeType := "FunctionDecl"
		if fn.IsComposable {
			meta["is_composable"] = "true"
			meta["framework"] = "jetpack-compose"
			nodeType = "ComposableFunction"
		}

		ident := fn.Name
		if res.PackageName != "" {
			ident = fmt.Sprintf("%s.%s", res.PackageName, fn.Name)
		}

		vis := fn.Visibility
		if vis == "" {
			vis = "public"
		}

		node := &core.ASTSymbolNode{
			Language:          core.LangKotlin,
			NodeType:          nodeType,
			Identifier:        ident,
			Signature:         fmt.Sprintf("fun %s(%s)", fn.Name, formatKotlinParams(fn.Params)),
			Docstring:         fn.Doc,
			Visibility:        vis,
			ASTPayload:        payload,
			ASTMetadata:       meta,
			LocalDependencies: fn.CalledFunctions,
			Dependencies:      fn.CalledFunctions,
			Lineage:           lineage,
		}
		nodeID, err := core.HashASTSymbolNode(node)
		if err == nil {
			node.NodeID = nodeID
			nodes = append(nodes, node)
		}
	}

	// Routes
	for _, r := range res.Routes {
		rPayload, err := json.Marshal(r)
		if err != nil {
			continue
		}
		rNode := &core.ASTSymbolNode{
			Language:   core.LangKotlin,
			NodeType:   "RouteBinding",
			Identifier: fmt.Sprintf("%s:%s %s", res.PackageName, r.Method, r.Path),
			Signature:  fmt.Sprintf("%s %s", r.Method, r.Path),
			ASTPayload: rPayload,
			ASTMetadata: map[string]string{
				"framework":    r.Framework,
				"handler":      r.HandlerName,
				"route_method": r.Method,
				"route_path":   r.Path,
			},
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

	// API Client Calls
	for _, api := range res.APICalls {
		apiPayload, err := json.Marshal(api)
		if err != nil {
			continue
		}
		apiNode := &core.ASTSymbolNode{
			Language:   core.LangKotlin,
			NodeType:   "ApiClientCall",
			Identifier: fmt.Sprintf("%s:%s %s", res.FilePath, api.Method, api.URL),
			Signature:  fmt.Sprintf("%s %s", api.Method, api.URL),
			ASTPayload: apiPayload,
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
		apiNodeID, err := core.HashASTSymbolNode(apiNode)
		if err == nil {
			apiNode.NodeID = apiNodeID
			nodes = append(nodes, apiNode)
		}
	}

	return nodes, nil
}

func formatKotlinParams(params []KotlinParam) string {
	var list []string
	for _, p := range params {
		valVar := ""
		if p.IsVal {
			valVar = "val "
		} else if p.IsVar {
			valVar = "var "
		}
		defVal := ""
		if p.DefaultValue != "" {
			defVal = fmt.Sprintf(" = %s", p.DefaultValue)
		}
		list = append(list, fmt.Sprintf("%s%s: %s%s", valVar, p.Name, p.Type, defVal))
	}
	return strings.Join(list, ", ")
}

// BuildComponentNode bundles parsed Kotlin symbols into a core.ComponentNode.
func (p *KotlinParser) BuildComponentNode(
	res *KotlinFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = res.PackageName
		if compName == "" {
			compName = "kotlin-service"
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
		Language:    core.LangKotlin,
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
