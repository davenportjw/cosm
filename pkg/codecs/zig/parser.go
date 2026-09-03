package zig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// ZigParam represents a function parameter in Zig.
type ZigParam struct {
	Comptime bool   `json:"comptime,omitempty"`
	Noalias  bool   `json:"noalias,omitempty"`
	Anytype  bool   `json:"anytype,omitempty"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Default  string `json:"default,omitempty"`
}

// ZigField represents a field inside a struct, union, or enum in Zig.
type ZigField struct {
	Name         string `json:"name"`
	Type         string `json:"type,omitempty"`
	DefaultValue string `json:"default_value,omitempty"`
	TagValue     string `json:"tag_value,omitempty"`
	Doc          string `json:"doc,omitempty"`
}

// ZigFunction represents a function or method declaration in Zig.
type ZigFunction struct {
	Name         string     `json:"name"`
	Visibility   string     `json:"visibility"` // "pub", "private"
	IsExport     bool       `json:"is_export,omitempty"`
	IsExtern     bool       `json:"is_extern,omitempty"`
	IsInline     bool       `json:"is_inline,omitempty"`
	CallConv     string     `json:"callconv,omitempty"`
	Params       []ZigParam `json:"params,omitempty"`
	ReturnType   string     `json:"return_type"`
	BodySource   string     `json:"body_source,omitempty"`
	Doc          string     `json:"doc,omitempty"`
	Dependencies []string   `json:"dependencies,omitempty"`
}

// ZigStruct represents a struct declaration in Zig (`struct`, `packed struct`, `extern struct`).
type ZigStruct struct {
	Name       string        `json:"name"`
	Visibility string        `json:"visibility"` // "pub", "private"
	Kind       string        `json:"kind"`       // "struct", "packed struct", "extern struct"
	Fields     []ZigField    `json:"fields,omitempty"`
	Methods    []ZigFunction `json:"methods,omitempty"`
	Doc        string        `json:"doc,omitempty"`
}

// ZigEnum represents an enum declaration in Zig (`enum`, `enum(u8)`).
type ZigEnum struct {
	Name       string        `json:"name"`
	Visibility string        `json:"visibility"` // "pub", "private"
	TagType    string        `json:"tag_type,omitempty"`
	Fields     []ZigField    `json:"fields,omitempty"`
	Methods    []ZigFunction `json:"methods,omitempty"`
	Doc        string        `json:"doc,omitempty"`
}

// ZigUnion represents a union or tagged union in Zig (`union`, `union(enum)`).
type ZigUnion struct {
	Name       string        `json:"name"`
	Visibility string        `json:"visibility"` // "pub", "private"
	TagType    string        `json:"tag_type,omitempty"`
	Fields     []ZigField    `json:"fields,omitempty"`
	Methods    []ZigFunction `json:"methods,omitempty"`
	Doc        string        `json:"doc,omitempty"`
}

// ZigVar represents a top-level constant or variable (`const`, `var`, `comptime var`).
type ZigVar struct {
	Name       string `json:"name"`
	Visibility string `json:"visibility"` // "pub", "private"
	IsConst    bool   `json:"is_const"`
	IsComptime bool   `json:"is_comptime,omitempty"`
	IsExtern   bool   `json:"is_extern,omitempty"`
	Type       string `json:"type,omitempty"`
	Value      string `json:"value,omitempty"`
	Doc        string `json:"doc,omitempty"`
}

// ZigErrorSet represents an error set declaration in Zig (`error { A, B }`).
type ZigErrorSet struct {
	Name       string   `json:"name"`
	Visibility string   `json:"visibility"` // "pub", "private"
	Errors     []string `json:"errors"`
	Doc        string   `json:"doc,omitempty"`
}

// ZigTest represents a test block in Zig (`test "name" { ... }`).
type ZigTest struct {
	Name       string `json:"name"`
	BodySource string `json:"body_source"`
	Doc        string `json:"doc,omitempty"`
}

// ZigImport represents an `@import(...)` or `@cImport(...)` directive.
type ZigImport struct {
	Alias      string `json:"alias"`
	Path       string `json:"path"`
	IsCImport  bool   `json:"is_cimport,omitempty"`
	CSource    string `json:"c_source,omitempty"`
	Visibility string `json:"visibility"`
}

// ZigFileResult holds all extracted symbols and metadata from a Zig source file.
type ZigFileResult struct {
	FilePath    string                `json:"file_path"`
	PackageName string                `json:"package_name"`
	Docstring   string                `json:"docstring,omitempty"`
	Imports     []ZigImport           `json:"imports,omitempty"`
	Structs     []ZigStruct           `json:"structs,omitempty"`
	Enums       []ZigEnum             `json:"enums,omitempty"`
	Unions      []ZigUnion            `json:"unions,omitempty"`
	Functions   []ZigFunction         `json:"functions,omitempty"`
	Variables   []ZigVar              `json:"variables,omitempty"`
	ErrorSets   []ZigErrorSet         `json:"error_sets,omitempty"`
	Tests       []ZigTest             `json:"tests,omitempty"`
	AllSymbols  []*core.ASTSymbolNode `json:"all_symbols"`
}

// ZigParser parses Zig source code into structured AST nodes.
type ZigParser struct{}

// NewZigParser creates a new ZigParser instance.
func NewZigParser() *ZigParser {
	return &ZigParser{}
}

// ParseSource parses Zig source code string/bytes and extracts AST symbols.
func (p *ZigParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*ZigFileResult, error) {
	if filename == "" {
		filename = "main.zig"
	}

	result := &ZigFileResult{
		FilePath:    filename,
		PackageName: p.derivePackageName(filename),
	}

	text := string(src)
	lines := strings.Split(text, "\n")

	// 1. Extract file-level doc comments `//!`
	var fileDoc []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "//!") {
			fileDoc = append(fileDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "//!")))
		} else if trimmed != "" {
			break
		}
	}
	result.Docstring = strings.Join(fileDoc, "\n")

	// 2. Tokenize and extract top-level blocks
	if err := p.parseDeclarations(text, result); err != nil {
		return nil, fmt.Errorf("parsing Zig source %s: %w", filename, err)
	}

	// 3. Convert all extracted declarations into ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting Zig symbols to AST nodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a .zig file from disk.
func (p *ZigParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*ZigFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .zig files in a directory.
func (p *ZigParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*ZigFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*ZigFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".zig" {
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

func (p *ZigParser) derivePackageName(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" || name == "main" || name == "root" || name == "lib" {
		dir := filepath.Base(filepath.Dir(filename))
		if dir != "." && dir != "/" && dir != "" {
			return dir
		}
		return "zig"
	}
	return name
}

// ----------------------------------------------------------------------------
// Declaration Parsing Engine
// ----------------------------------------------------------------------------

func (p *ZigParser) parseDeclarations(src string, res *ZigFileResult) error {
	lines := strings.Split(src, "\n")
	var pendingDoc []string

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Accumulate doc comments `///`
		if strings.HasPrefix(trimmed, "///") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "///")))
			i++
			continue
		}

		// Skip module comments `//!` and empty lines or line comments `//`
		if strings.HasPrefix(trimmed, "//!") || strings.HasPrefix(trimmed, "//") || trimmed == "" {
			i++
			continue
		}

		doc := strings.Join(pendingDoc, "\n")

		// 1. Check for imports: `(pub) const x = @import("...");` or `@cImport(...)`
		if (strings.Contains(trimmed, "@import(") || strings.Contains(trimmed, "@cImport(")) && strings.Contains(trimmed, "=") {
			imp, nextIdx := p.parseImport(lines, i)
			if imp != nil {
				res.Imports = append(res.Imports, *imp)
				pendingDoc = nil
				i = nextIdx
				continue
			}
		}

		// 2. Check for Error Sets: `(pub) const Name = error { ... };`
		if strings.Contains(trimmed, "= error {") || strings.Contains(trimmed, "= error{") {
			errSet, nextIdx := p.parseErrorSet(lines, i, doc)
			if errSet != nil {
				res.ErrorSets = append(res.ErrorSets, *errSet)
				pendingDoc = nil
				i = nextIdx
				continue
			}
		}

		// 3. Check for Structs: `(pub) const Name = (packed | extern) struct { ... };`
		if strings.Contains(trimmed, "struct {") || strings.Contains(trimmed, "struct{") {
			st, nextIdx := p.parseStruct(lines, i, doc)
			if st != nil {
				res.Structs = append(res.Structs, *st)
				pendingDoc = nil
				i = nextIdx
				continue
			}
		}

		// 4. Check for Enums: `(pub) const Name = enum(type) { ... };` or `enum { ... };`
		if (strings.Contains(trimmed, "= enum {") || strings.Contains(trimmed, "= enum{") || strings.Contains(trimmed, "= enum(")) && !strings.Contains(trimmed, "union(enum") {
			en, nextIdx := p.parseEnum(lines, i, doc)
			if en != nil {
				res.Enums = append(res.Enums, *en)
				pendingDoc = nil
				i = nextIdx
				continue
			}
		}

		// 5. Check for Unions: `(pub) const Name = union(enum) { ... };` or `union { ... };`
		if strings.Contains(trimmed, "= union {") || strings.Contains(trimmed, "= union{") || strings.Contains(trimmed, "= union(") {
			un, nextIdx := p.parseUnion(lines, i, doc)
			if un != nil {
				res.Unions = append(res.Unions, *un)
				pendingDoc = nil
				i = nextIdx
				continue
			}
		}

		// 6. Check for Functions: `(pub | export | extern | inline) fn name(...) ... {`
		if p.isFunctionDecl(trimmed) {
			fn, nextIdx := p.parseFunction(lines, i, doc)
			if fn != nil {
				res.Functions = append(res.Functions, *fn)
				pendingDoc = nil
				i = nextIdx
				continue
			}
		}

		// 7. Check for Tests: `test "name" { ... }` or `test { ... }`
		if strings.HasPrefix(trimmed, "test ") || strings.HasPrefix(trimmed, "test{") {
			tst, nextIdx := p.parseTest(lines, i, doc)
			if tst != nil {
				res.Tests = append(res.Tests, *tst)
				pendingDoc = nil
				i = nextIdx
				continue
			}
		}

		// 8. Check for Top-Level Variables/Constants: `(pub) (comptime) (const|var) name (: type) = ...;`
		if p.isVariableDecl(trimmed) {
			v, nextIdx := p.parseVariable(lines, i, doc)
			if v != nil {
				res.Variables = append(res.Variables, *v)
				pendingDoc = nil
				i = nextIdx
				continue
			}
		}

		pendingDoc = nil
		i++
	}

	return nil
}

func (p *ZigParser) isFunctionDecl(line string) bool {
	// Matches: `pub fn foo(`, `fn foo(`, `export fn foo(`, `pub inline fn foo(`, `extern fn foo(`
	patterns := []string{"fn ", "fn\t"}
	for _, pat := range patterns {
		if strings.Contains(line, pat) && strings.Contains(line, "(") {
			prefix := line[:strings.Index(line, pat)]
			prefix = strings.TrimSpace(prefix)
			if prefix == "" || prefix == "pub" || prefix == "export" || prefix == "extern" ||
				prefix == "inline" || prefix == "pub inline" || prefix == "pub export" ||
				prefix == "export inline" {
				return true
			}
		}
	}
	return false
}

func (p *ZigParser) isVariableDecl(line string) bool {
	return strings.HasPrefix(line, "const ") || strings.HasPrefix(line, "var ") ||
		strings.HasPrefix(line, "pub const ") || strings.HasPrefix(line, "pub var ") ||
		strings.HasPrefix(line, "comptime var ") || strings.HasPrefix(line, "pub comptime var ") ||
		strings.HasPrefix(line, "export const ") || strings.HasPrefix(line, "extern const ") ||
		strings.HasPrefix(line, "extern var ")
}

// ----------------------------------------------------------------------------
// Detailed Parsers
// ----------------------------------------------------------------------------

func (p *ZigParser) parseImport(lines []string, startIdx int) (*ZigImport, int) {
	line := strings.TrimSpace(lines[startIdx])
	isPub := strings.HasPrefix(line, "pub ")
	vis := "private"
	if isPub {
		vis = "public"
		line = strings.TrimPrefix(line, "pub ")
	}

	if strings.Contains(line, "@cImport(") {
		// Collect entire cImport block
		block, nextIdx := extractBraceOrParenBlock(lines, startIdx, '(', ')')
		alias := extractDeclName(lines[startIdx])
		return &ZigImport{
			Alias:      alias,
			IsCImport:  true,
			CSource:    block,
			Visibility: vis,
		}, nextIdx
	}

	alias := extractDeclName(line)
	path := ""
	if open := strings.Index(line, "@import(\""); open != -1 {
		close := strings.Index(line[open+9:], "\")")
		if close != -1 {
			path = line[open+9 : open+9+close]
		}
	}

	return &ZigImport{
		Alias:      alias,
		Path:       path,
		Visibility: vis,
	}, startIdx + 1
}

func (p *ZigParser) parseErrorSet(lines []string, startIdx int, doc string) (*ZigErrorSet, int) {
	line := strings.TrimSpace(lines[startIdx])
	vis := "private"
	if strings.HasPrefix(line, "pub ") {
		vis = "public"
	}
	name := extractDeclName(line)

	body, nextIdx := extractBraceBlock(lines, startIdx)
	var errors []string
	for _, l := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(l)
		trimmed = strings.TrimSuffix(trimmed, ",")
		if strings.HasPrefix(trimmed, "//") || trimmed == "{" || trimmed == "}" || trimmed == "};" || trimmed == "" ||
			strings.Contains(trimmed, "error{") || strings.Contains(trimmed, "error {") {
			continue
		}
		errors = append(errors, trimmed)
	}

	return &ZigErrorSet{
		Name:       name,
		Visibility: vis,
		Errors:     errors,
		Doc:        doc,
	}, nextIdx
}

func (p *ZigParser) parseStruct(lines []string, startIdx int, doc string) (*ZigStruct, int) {
	line := strings.TrimSpace(lines[startIdx])
	vis := "private"
	if strings.HasPrefix(line, "pub ") {
		vis = "public"
	}
	name := extractDeclName(line)

	kind := "struct"
	if strings.Contains(line, "packed struct") {
		kind = "packed struct"
	} else if strings.Contains(line, "extern struct") {
		kind = "extern struct"
	}

	body, nextIdx := extractBraceBlock(lines, startIdx)
	fields, methods := p.parseStructOrUnionBody(body)

	return &ZigStruct{
		Name:       name,
		Visibility: vis,
		Kind:       kind,
		Fields:     fields,
		Methods:    methods,
		Doc:        doc,
	}, nextIdx
}

func (p *ZigParser) parseEnum(lines []string, startIdx int, doc string) (*ZigEnum, int) {
	line := strings.TrimSpace(lines[startIdx])
	vis := "private"
	if strings.HasPrefix(line, "pub ") {
		vis = "public"
	}
	name := extractDeclName(line)

	tagType := ""
	if open := strings.Index(line, "enum("); open != -1 {
		close := strings.Index(line[open+5:], ")")
		if close != -1 {
			tagType = strings.TrimSpace(line[open+5 : open+5+close])
		}
	}

	body, nextIdx := extractBraceBlock(lines, startIdx)
	fields, methods := p.parseEnumBody(body)

	return &ZigEnum{
		Name:       name,
		Visibility: vis,
		TagType:    tagType,
		Fields:     fields,
		Methods:    methods,
		Doc:        doc,
	}, nextIdx
}

func (p *ZigParser) parseUnion(lines []string, startIdx int, doc string) (*ZigUnion, int) {
	line := strings.TrimSpace(lines[startIdx])
	vis := "private"
	if strings.HasPrefix(line, "pub ") {
		vis = "public"
	}
	name := extractDeclName(line)

	tagType := ""
	if open := strings.Index(line, "union("); open != -1 {
		close := strings.Index(line[open+6:], ")")
		if close != -1 {
			tagType = strings.TrimSpace(line[open+6 : open+6+close])
		}
	}

	body, nextIdx := extractBraceBlock(lines, startIdx)
	fields, methods := p.parseStructOrUnionBody(body)

	return &ZigUnion{
		Name:       name,
		Visibility: vis,
		TagType:    tagType,
		Fields:     fields,
		Methods:    methods,
		Doc:        doc,
	}, nextIdx
}

func (p *ZigParser) parseFunction(lines []string, startIdx int, doc string) (*ZigFunction, int) {
	headerLines := []string{lines[startIdx]}
	cur := startIdx
	for !strings.Contains(lines[cur], "{") && !strings.Contains(lines[cur], ";") && cur+1 < len(lines) {
		cur++
		headerLines = append(headerLines, lines[cur])
	}
	header := strings.Join(headerLines, " ")

	vis := "private"
	isExport := false
	isExtern := false
	isInline := false

	if strings.Contains(header, "pub ") {
		vis = "public"
	}
	if strings.Contains(header, "export ") {
		isExport = true
	}
	if strings.Contains(header, "extern ") {
		isExtern = true
	}
	if strings.Contains(header, "inline ") {
		isInline = true
	}

	callConv := ""
	if idx := strings.Index(header, "callconv("); idx != -1 {
		closeIdx := strings.Index(header[idx+9:], ")")
		if closeIdx != -1 {
			callConv = strings.TrimSpace(header[idx+9 : idx+9+closeIdx])
		}
	}

	fnIdx := strings.Index(header, "fn ")
	afterFn := strings.TrimSpace(header[fnIdx+3:])

	openParen := strings.Index(afterFn, "(")
	fnName := ""
	if openParen != -1 {
		fnName = strings.TrimSpace(afterFn[:openParen])
	}

	closeParen := findMatchingParen(afterFn, openParen)
	paramsStr := ""
	retType := ""
	if openParen != -1 && closeParen != -1 {
		paramsStr = afterFn[openParen+1 : closeParen]
		afterParams := strings.TrimSpace(afterFn[closeParen+1:])
		if braceIdx := strings.Index(afterParams, "{"); braceIdx != -1 {
			retType = strings.TrimSpace(afterParams[:braceIdx])
		} else if semiIdx := strings.Index(afterParams, ";"); semiIdx != -1 {
			retType = strings.TrimSpace(afterParams[:semiIdx])
		} else {
			retType = afterParams
		}
	}

	params := p.parseParams(paramsStr)

	var bodySource string
	var nextIdx int
	if strings.Contains(lines[cur], "{") {
		bodySource, nextIdx = extractBraceBlock(lines, startIdx)
	} else {
		nextIdx = cur + 1
	}

	deps := extractDependencies(bodySource)

	return &ZigFunction{
		Name:         fnName,
		Visibility:   vis,
		IsExport:     isExport,
		IsExtern:     isExtern,
		IsInline:     isInline,
		CallConv:     callConv,
		Params:       params,
		ReturnType:   retType,
		BodySource:   bodySource,
		Doc:          doc,
		Dependencies: deps,
	}, nextIdx
}

func (p *ZigParser) parseTest(lines []string, startIdx int, doc string) (*ZigTest, int) {
	line := strings.TrimSpace(lines[startIdx])
	name := "anonymous"
	if openQ := strings.Index(line, "\""); openQ != -1 {
		closeQ := strings.Index(line[openQ+1:], "\"")
		if closeQ != -1 {
			name = line[openQ+1 : openQ+1+closeQ]
		}
	}

	body, nextIdx := extractBraceBlock(lines, startIdx)
	return &ZigTest{
		Name:       name,
		BodySource: body,
		Doc:        doc,
	}, nextIdx
}

func (p *ZigParser) parseVariable(lines []string, startIdx int, doc string) (*ZigVar, int) {
	line := strings.TrimSpace(lines[startIdx])
	vis := "private"
	if strings.HasPrefix(line, "pub ") {
		vis = "public"
		line = strings.TrimPrefix(line, "pub ")
	}

	isComptime := false
	if strings.HasPrefix(line, "comptime ") {
		isComptime = true
		line = strings.TrimPrefix(line, "comptime ")
	}

	isExtern := false
	if strings.HasPrefix(line, "extern ") {
		isExtern = true
		line = strings.TrimPrefix(line, "extern ")
	}

	isConst := strings.HasPrefix(line, "const ")
	if isConst {
		line = strings.TrimPrefix(line, "const ")
	} else {
		line = strings.TrimPrefix(line, "var ")
	}

	line = strings.TrimSuffix(line, ";")

	var varName, varType, varVal string
	if eqIdx := strings.Index(line, "="); eqIdx != -1 {
		lhs := strings.TrimSpace(line[:eqIdx])
		varVal = strings.TrimSpace(line[eqIdx+1:])

		if colIdx := strings.Index(lhs, ":"); colIdx != -1 {
			varName = strings.TrimSpace(lhs[:colIdx])
			varType = strings.TrimSpace(lhs[colIdx+1:])
		} else {
			varName = lhs
		}
	} else if colIdx := strings.Index(line, ":"); colIdx != -1 {
		varName = strings.TrimSpace(line[:colIdx])
		varType = strings.TrimSpace(line[colIdx+1:])
	} else {
		varName = strings.TrimSpace(line)
	}

	return &ZigVar{
		Name:       varName,
		Visibility: vis,
		IsConst:    isConst,
		IsComptime: isComptime,
		IsExtern:   isExtern,
		Type:       varType,
		Value:      varVal,
		Doc:        doc,
	}, startIdx + 1
}

// ----------------------------------------------------------------------------
// Body & Helper Parsers
// ----------------------------------------------------------------------------

func (p *ZigParser) parseParams(paramStr string) []ZigParam {
	trimmed := strings.TrimSpace(paramStr)
	if trimmed == "" {
		return nil
	}

	parts := splitTopLevelCommas(trimmed)
	var params []ZigParam

	for _, part := range parts {
		pStr := strings.TrimSpace(part)
		if pStr == "" {
			continue
		}

		isComptime := false
		if strings.HasPrefix(pStr, "comptime ") {
			isComptime = true
			pStr = strings.TrimPrefix(pStr, "comptime ")
		}

		isNoalias := false
		if strings.HasPrefix(pStr, "noalias ") {
			isNoalias = true
			pStr = strings.TrimPrefix(pStr, "noalias ")
		}

		isAnytype := strings.Contains(pStr, "anytype")

		var pName, pType, pDefault string
		if eqIdx := strings.Index(pStr, "="); eqIdx != -1 {
			pDefault = strings.TrimSpace(pStr[eqIdx+1:])
			pStr = strings.TrimSpace(pStr[:eqIdx])
		}

		if colIdx := strings.Index(pStr, ":"); colIdx != -1 {
			pName = strings.TrimSpace(pStr[:colIdx])
			pType = strings.TrimSpace(pStr[colIdx+1:])
		} else {
			pName = pStr
		}

		params = append(params, ZigParam{
			Comptime: isComptime,
			Noalias:  isNoalias,
			Anytype:  isAnytype,
			Name:     pName,
			Type:     pType,
			Default:  pDefault,
		})
	}

	return params
}

func (p *ZigParser) parseStructOrUnionBody(body string) ([]ZigField, []ZigFunction) {
	lines := strings.Split(body, "\n")
	var fields []ZigField
	var methods []ZigFunction
	var pendingDoc []string

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "///") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "///")))
			i++
			continue
		}
		if strings.HasPrefix(trimmed, "//") || trimmed == "{" || trimmed == "}" || trimmed == "};" || trimmed == "" ||
			strings.Contains(trimmed, "struct {") || strings.Contains(trimmed, "struct{") ||
			strings.Contains(trimmed, "union(") || strings.Contains(trimmed, "union{") || strings.Contains(trimmed, "union {") {
			i++
			continue
		}

		doc := strings.Join(pendingDoc, "\n")

		if p.isFunctionDecl(trimmed) {
			fn, nextIdx := p.parseFunction(lines, i, doc)
			if fn != nil {
				methods = append(methods, *fn)
				pendingDoc = nil
				i = nextIdx
				continue
			}
		}

		// Field declaration: `name: type = default,` or `name: type,`
		if strings.Contains(trimmed, ":") {
			cleanLine := strings.TrimSuffix(trimmed, ",")
			colIdx := strings.Index(cleanLine, ":")
			fName := strings.TrimSpace(cleanLine[:colIdx])
			rest := strings.TrimSpace(cleanLine[colIdx+1:])

			var fType, fDef string
			if eqIdx := strings.Index(rest, "="); eqIdx != -1 {
				fType = strings.TrimSpace(rest[:eqIdx])
				fDef = strings.TrimSpace(rest[eqIdx+1:])
			} else {
				fType = rest
			}

			fields = append(fields, ZigField{
				Name:         fName,
				Type:         fType,
				DefaultValue: fDef,
				Doc:          doc,
			})
		}

		pendingDoc = nil
		i++
	}

	return fields, methods
}

func (p *ZigParser) parseEnumBody(body string) ([]ZigField, []ZigFunction) {
	lines := strings.Split(body, "\n")
	var fields []ZigField
	var methods []ZigFunction
	var pendingDoc []string

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "///") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "///")))
			i++
			continue
		}
		if strings.HasPrefix(trimmed, "//") || trimmed == "{" || trimmed == "}" || trimmed == "};" || trimmed == "" ||
			strings.Contains(trimmed, "enum(") || strings.Contains(trimmed, "enum {") || strings.Contains(trimmed, "enum{") {
			i++
			continue
		}

		doc := strings.Join(pendingDoc, "\n")

		if p.isFunctionDecl(trimmed) {
			fn, nextIdx := p.parseFunction(lines, i, doc)
			if fn != nil {
				methods = append(methods, *fn)
				pendingDoc = nil
				i = nextIdx
				continue
			}
		}

		// Enum case: `case_name = val,` or `case_name,`
		cleanLine := strings.TrimSuffix(trimmed, ",")
		var cName, cVal string
		if eqIdx := strings.Index(cleanLine, "="); eqIdx != -1 {
			cName = strings.TrimSpace(cleanLine[:eqIdx])
			cVal = strings.TrimSpace(cleanLine[eqIdx+1:])
		} else {
			cName = cleanLine
		}

		if cName != "" && cName != "}" && cName != "};" && !strings.Contains(cName, "pub ") && !strings.Contains(cName, "fn ") {
			fields = append(fields, ZigField{
				Name:     cName,
				TagValue: cVal,
				Doc:      doc,
			})
		}

		pendingDoc = nil
		i++
	}

	return fields, methods
}

// ----------------------------------------------------------------------------
// AST Node Conversions
// ----------------------------------------------------------------------------

func (p *ZigParser) toSymbolNodes(res *ZigFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Structs
	for _, st := range res.Structs {
		node, err := StructToASTSymbolNode(st, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Enums
	for _, en := range res.Enums {
		node, err := EnumToASTSymbolNode(en, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Unions
	for _, un := range res.Unions {
		node, err := UnionToASTSymbolNode(un, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Functions
	for _, fn := range res.Functions {
		node, err := FuncToASTSymbolNode(fn, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Variables & Constants
	for _, v := range res.Variables {
		node, err := VarToASTSymbolNode(v, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Error Sets
	for _, errSet := range res.ErrorSets {
		node, err := ErrorSetToASTSymbolNode(errSet, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Tests
	for _, tst := range res.Tests {
		node, err := TestToASTSymbolNode(tst, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// StructToASTSymbolNode converts a ZigStruct to an ASTSymbolNode.
func StructToASTSymbolNode(st ZigStruct, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(st)
	if err != nil {
		return nil, fmt.Errorf("marshaling Zig struct %s: %w", st.Name, err)
	}

	sig := fmt.Sprintf("%s %s", st.Kind, st.Name)
	if st.Visibility == "public" {
		sig = "pub " + sig
	}

	var methodNames []string
	for _, m := range st.Methods {
		methodNames = append(methodNames, m.Name)
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangZig,
		NodeType:          "StructDef",
		Identifier:        fmt.Sprintf("%s::%s", pkgName, st.Name),
		Signature:         sig,
		Docstring:         st.Doc,
		Visibility:        st.Visibility,
		ASTPayload:        payload,
		ASTMetadata:       map[string]string{"kind": st.Kind, "fields": fmt.Sprintf("%d", len(st.Fields))},
		LocalDependencies: methodNames,
		Dependencies:      methodNames,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// EnumToASTSymbolNode converts a ZigEnum to an ASTSymbolNode.
func EnumToASTSymbolNode(en ZigEnum, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(en)
	if err != nil {
		return nil, fmt.Errorf("marshaling Zig enum %s: %w", en.Name, err)
	}

	sig := fmt.Sprintf("enum %s", en.Name)
	if en.TagType != "" {
		sig = fmt.Sprintf("enum(%s) %s", en.TagType, en.Name)
	}
	if en.Visibility == "public" {
		sig = "pub " + sig
	}

	node := &core.ASTSymbolNode{
		Language:    core.LangZig,
		NodeType:    "EnumDef",
		Identifier:  fmt.Sprintf("%s::%s", pkgName, en.Name),
		Signature:   sig,
		Docstring:   en.Doc,
		Visibility:  en.Visibility,
		ASTPayload:  payload,
		ASTMetadata: map[string]string{"tag_type": en.TagType, "cases": fmt.Sprintf("%d", len(en.Fields))},
		Lineage:     lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// UnionToASTSymbolNode converts a ZigUnion to an ASTSymbolNode.
func UnionToASTSymbolNode(un ZigUnion, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(un)
	if err != nil {
		return nil, fmt.Errorf("marshaling Zig union %s: %w", un.Name, err)
	}

	sig := fmt.Sprintf("union %s", un.Name)
	if un.TagType != "" {
		sig = fmt.Sprintf("union(%s) %s", un.TagType, un.Name)
	}
	if un.Visibility == "public" {
		sig = "pub " + sig
	}

	node := &core.ASTSymbolNode{
		Language:    core.LangZig,
		NodeType:    "UnionDef",
		Identifier:  fmt.Sprintf("%s::%s", pkgName, un.Name),
		Signature:   sig,
		Docstring:   un.Doc,
		Visibility:  un.Visibility,
		ASTPayload:  payload,
		ASTMetadata: map[string]string{"tag_type": un.TagType, "fields": fmt.Sprintf("%d", len(un.Fields))},
		Lineage:     lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// FuncToASTSymbolNode converts a ZigFunction to an ASTSymbolNode.
func FuncToASTSymbolNode(fn ZigFunction, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(fn)
	if err != nil {
		return nil, fmt.Errorf("marshaling Zig function %s: %w", fn.Name, err)
	}

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

	sig := fmt.Sprintf("fn %s(%s) %s", fn.Name, strings.Join(paramParts, ", "), fn.ReturnType)
	if fn.IsInline {
		sig = "inline " + sig
	}
	if fn.IsExport {
		sig = "export " + sig
	} else if fn.IsExtern {
		sig = "extern " + sig
	}
	if fn.Visibility == "public" {
		sig = "pub " + sig
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangZig,
		NodeType:          "FnDef",
		Identifier:        fmt.Sprintf("%s::%s", pkgName, fn.Name),
		Signature:         sig,
		Docstring:         fn.Doc,
		Visibility:        fn.Visibility,
		ASTPayload:        payload,
		ASTMetadata:       map[string]string{"return_type": fn.ReturnType, "callconv": fn.CallConv},
		LocalDependencies: fn.Dependencies,
		Dependencies:      fn.Dependencies,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// VarToASTSymbolNode converts a ZigVar to an ASTSymbolNode.
func VarToASTSymbolNode(v ZigVar, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling Zig variable %s: %w", v.Name, err)
	}

	kw := "var"
	if v.IsConst {
		kw = "const"
	}
	if v.IsComptime {
		kw = "comptime " + kw
	}
	if v.IsExtern {
		kw = "extern " + kw
	}
	if v.Visibility == "public" {
		kw = "pub " + kw
	}

	sig := fmt.Sprintf("%s %s", kw, v.Name)
	if v.Type != "" {
		sig += ": " + v.Type
	}
	if v.Value != "" {
		sig += " = " + v.Value
	}

	node := &core.ASTSymbolNode{
		Language:    core.LangZig,
		NodeType:    "VarDef",
		Identifier:  fmt.Sprintf("%s::%s", pkgName, v.Name),
		Signature:   sig,
		Docstring:   v.Doc,
		Visibility:  v.Visibility,
		ASTPayload:  payload,
		ASTMetadata: map[string]string{"type": v.Type, "is_const": fmt.Sprintf("%v", v.IsConst)},
		Lineage:     lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// ErrorSetToASTSymbolNode converts a ZigErrorSet to an ASTSymbolNode.
func ErrorSetToASTSymbolNode(errSet ZigErrorSet, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(errSet)
	if err != nil {
		return nil, fmt.Errorf("marshaling Zig error set %s: %w", errSet.Name, err)
	}

	sig := fmt.Sprintf("const %s = error { ... }", errSet.Name)
	if errSet.Visibility == "public" {
		sig = "pub " + sig
	}

	node := &core.ASTSymbolNode{
		Language:    core.LangZig,
		NodeType:    "ErrorSetDef",
		Identifier:  fmt.Sprintf("%s::error::%s", pkgName, errSet.Name),
		Signature:   sig,
		Docstring:   errSet.Doc,
		Visibility:  errSet.Visibility,
		ASTPayload:  payload,
		ASTMetadata: map[string]string{"errors_count": fmt.Sprintf("%d", len(errSet.Errors))},
		Lineage:     lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// TestToASTSymbolNode converts a ZigTest to an ASTSymbolNode.
func TestToASTSymbolNode(tst ZigTest, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(tst)
	if err != nil {
		return nil, fmt.Errorf("marshaling Zig test %s: %w", tst.Name, err)
	}

	node := &core.ASTSymbolNode{
		Language:    core.LangZig,
		NodeType:    "TestDef",
		Identifier:  fmt.Sprintf("%s::test::%s", pkgName, tst.Name),
		Signature:   fmt.Sprintf("test %q", tst.Name),
		Docstring:   tst.Doc,
		Visibility:  "private",
		ASTPayload:  payload,
		ASTMetadata: map[string]string{"test_name": tst.Name},
		Lineage:     lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// BuildComponentNode bundles parsed Zig symbols into a core.ComponentNode.
func (p *ZigParser) BuildComponentNode(
	res *ZigFileResult,
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
		"file_path":      res.FilePath,
		"package_name":   res.PackageName,
		"symbol_count":   fmt.Sprintf("%d", len(res.AllSymbols)),
		"struct_count":   fmt.Sprintf("%d", len(res.Structs)),
		"function_count": fmt.Sprintf("%d", len(res.Functions)),
		"enum_count":     fmt.Sprintf("%d", len(res.Enums)),
		"test_count":     fmt.Sprintf("%d", len(res.Tests)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangZig,
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

// ----------------------------------------------------------------------------
// Extraction Utilities
// ----------------------------------------------------------------------------

func extractDeclName(line string) string {
	clean := line
	for _, prefix := range []string{"pub ", "export ", "extern ", "comptime ", "const ", "var "} {
		clean = strings.TrimPrefix(clean, prefix)
	}
	clean = strings.TrimSpace(clean)

	for _, sep := range []string{":", "=", "{"} {
		if idx := strings.Index(clean, sep); idx != -1 {
			clean = clean[:idx]
		}
	}
	return strings.TrimSpace(clean)
}

func extractBraceBlock(lines []string, startIdx int) (string, int) {
	var blockLines []string
	depth := 0
	foundOpen := false

	for i := startIdx; i < len(lines); i++ {
		line := lines[i]
		blockLines = append(blockLines, line)

		for _, ch := range line {
			if ch == '{' {
				depth++
				foundOpen = true
			} else if ch == '}' {
				depth--
				if foundOpen && depth == 0 {
					return strings.Join(blockLines, "\n"), i + 1
				}
			}
		}

		if !foundOpen && strings.HasSuffix(strings.TrimSpace(line), ";") {
			return strings.Join(blockLines, "\n"), i + 1
		}
	}

	return strings.Join(blockLines, "\n"), len(lines)
}

func extractBraceOrParenBlock(lines []string, startIdx int, openChar, closeChar rune) (string, int) {
	var blockLines []string
	depth := 0
	foundOpen := false

	for i := startIdx; i < len(lines); i++ {
		line := lines[i]
		blockLines = append(blockLines, line)

		for _, ch := range line {
			if ch == openChar {
				depth++
				foundOpen = true
			} else if ch == closeChar {
				depth--
				if foundOpen && depth == 0 {
					return strings.Join(blockLines, "\n"), i + 1
				}
			}
		}
	}

	return strings.Join(blockLines, "\n"), len(lines)
}

func findMatchingParen(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitTopLevelCommas(s string) []string {
	var parts []string
	depthParen := 0
	depthBrace := 0
	depthBracket := 0
	start := 0

	for i, r := range s {
		switch r {
		case '(':
			depthParen++
		case ')':
			depthParen--
		case '{':
			depthBrace++
		case '}':
			depthBrace--
		case '[':
			depthBracket++
		case ']':
			depthBracket--
		case ',':
			if depthParen == 0 && depthBrace == 0 && depthBracket == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}

	if start < len(s) {
		parts = append(parts, s[start:])
	}

	return parts
}

func extractDependencies(body string) []string {
	if body == "" {
		return nil
	}
	re := regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	matches := re.FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool)
	var deps []string

	keywords := map[string]bool{
		"if": true, "while": true, "for": true, "switch": true,
		"return": true, "catch": true, "try": true, "orelse": true,
		"expect": true, "print": true, "assert": true,
	}

	for _, m := range matches {
		name := m[1]
		if !keywords[name] && !seen[name] && !strings.HasPrefix(name, "@") {
			seen[name] = true
			deps = append(deps, name)
		}
	}

	sort.Strings(deps)
	return deps
}
