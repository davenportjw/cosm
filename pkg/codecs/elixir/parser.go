package elixir

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// ElixirParam represents a parameter in an Elixir function signature.
type ElixirParam struct {
	Pattern      string `json:"pattern"`
	DefaultValue string `json:"default_value,omitempty"`
}

// ElixirFunction represents a function, private function, or macro in Elixir.
type ElixirFunction struct {
	Name            string        `json:"name"`
	Arity           int           `json:"arity"`
	Kind            string        `json:"kind"` // "def", "defp", "defmacro", "defmacrop"
	Params          []ElixirParam `json:"params"`
	GuardClause     string        `json:"guard_clause,omitempty"`
	Spec            string        `json:"spec,omitempty"`
	Doc             string        `json:"doc,omitempty"`
	BodySource      string        `json:"body_source,omitempty"`
	CalledFunctions []string      `json:"called_functions,omitempty"`
	LineNumber      int           `json:"line_number,omitempty"`
}

// ElixirSchemaField represents an Ecto schema field or association.
type ElixirSchemaField struct {
	Name            string `json:"name"`
	Type            string `json:"type,omitempty"`
	AssociationType string `json:"association_type,omitempty"` // "has_many", "belongs_to", "has_one", "many_to_many"
	TargetSchema    string `json:"target_schema,omitempty"`
}

// ElixirModule represents a defmodule definition in Elixir.
type ElixirModule struct {
	Name         string              `json:"name"`
	Doc          string              `json:"doc,omitempty"`
	Uses         []string            `json:"uses,omitempty"`
	Imports      []string            `json:"imports,omitempty"`
	Aliases      []string            `json:"aliases,omitempty"`
	Requires     []string            `json:"requires,omitempty"`
	Behaviours   []string            `json:"behaviours,omitempty"`
	Plugs        []string            `json:"plugs,omitempty"`
	StructFields []string            `json:"struct_fields,omitempty"`
	SchemaTable  string              `json:"schema_table,omitempty"`
	SchemaFields []ElixirSchemaField `json:"schema_fields,omitempty"`
	Functions    []ElixirFunction    `json:"functions,omitempty"`
	IsRouter     bool                `json:"is_router"`
	IsController bool                `json:"is_controller"`
	IsSchema     bool                `json:"is_schema"`
	LineNumber   int                 `json:"line_number,omitempty"`
}

// ElixirRouteBinding represents a Phoenix router route binding.
type ElixirRouteBinding struct {
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Controller  string   `json:"controller"`
	Action      string   `json:"action"`
	PipeThrough []string `json:"pipe_through,omitempty"`
	Framework   string   `json:"framework"`
	LineNumber  int      `json:"line_number,omitempty"`
}

// ElixirParseResult contains all parsed entities from an Elixir source file.
type ElixirParseResult struct {
	FilePath   string                `json:"file_path"`
	Modules    []ElixirModule        `json:"modules"`
	Routes     []ElixirRouteBinding  `json:"routes"`
	AllSymbols []*core.ASTSymbolNode `json:"all_symbols"`
}

// ElixirParser parses Elixir source code (.ex, .exs) into structured AST Symbol Nodes.
type ElixirParser struct{}

// NewElixirParser creates a new ElixirParser instance.
func NewElixirParser() *ElixirParser {
	return &ElixirParser{}
}

type elixirLineInfo struct {
	text string
	num  int
}

func (p *ElixirParser) splitLines(content []byte) []elixirLineInfo {
	raw := strings.Split(string(content), "\n")
	var lines []elixirLineInfo
	num := 1
	for _, l := range raw {
		lines = append(lines, elixirLineInfo{
			text: l,
			num:  num,
		})
		num++
	}
	return lines
}

var (
	elixirModuleRegex   = regexp.MustCompile(`^defmodule\s+([A-Za-z0-9_\.]+)\s+do`)
	elixirUseRegex      = regexp.MustCompile(`^\s*use\s+([A-Za-z0-9_\.\,\:\s\[\]\{\}=>]+)`)
	elixirImportRegex   = regexp.MustCompile(`^\s*import\s+([A-Za-z0-9_\.\,\:\s\[\]\{\}=>]+)`)
	elixirAliasRegex    = regexp.MustCompile(`^\s*alias\s+([A-Za-z0-9_\.\,\:\s\[\]\{\}=>]+)`)
	elixirRequireRegex  = regexp.MustCompile(`^\s*require\s+([A-Za-z0-9_\.\,\:\s\[\]\{\}=>]+)`)
	elixirBehaviorRegex = regexp.MustCompile(`^\s*@behaviour\s+([A-Za-z0-9_\.]+)`)
	elixirPlugRegex     = regexp.MustCompile(`^\s*plug\s+:?([A-Za-z0-9_\.]+)`)
	elixirSpecRegex     = regexp.MustCompile(`^\s*@spec\s+([a-zA-Z0-9_!\?]+)\s*\((.*?)\)\s*::\s*(.*)`)
	elixirSchemaRegex   = regexp.MustCompile(`^\s*schema\s+['"]([^'"]+)['"]\s+do`)
	elixirFieldRegex    = regexp.MustCompile(`^\s*field\s+:([a-zA-Z0-9_]+)(?:\s*,\s*(:[a-zA-Z0-9_]+|\{.*\}|[A-Za-z0-9_\.]+))?`)
	elixirAssocRegex    = regexp.MustCompile(`^\s*(has_many|belongs_to|has_one|many_to_many)\s+:([a-zA-Z0-9_]+)\s*,\s*([A-Za-z0-9_\.]+)`)
	elixirDefstruct     = regexp.MustCompile(`^\s*defstruct\s+(?:\[(.*)\]|(.*))`)

	// Functions and macros
	elixirFuncStartRegex = regexp.MustCompile(`^\s*(def|defp|defmacro|defmacrop)\s+([a-zA-Z0-9_!\?]+)(?:\s*\(|\s+do|\s*,|\s*$)`)
	elixirFullFuncRegex  = regexp.MustCompile(`(?s)^\s*(def|defp|defmacro|defmacrop)\s+([a-zA-Z0-9_!\?]+)(?:\s*\((.*?)\))?(?:\s*,\s*do:\s*(.*)|\s+when\s+(.*?)\s+do|\s+do|\s*$)`)

	// Phoenix router regexes
	phoenixScopeRegex       = regexp.MustCompile(`^\s*scope\s+['"]([^'"]+)['"](?:\s*,\s*([A-Za-z0-9_\.]+))?`)
	phoenixPipeThroughRegex = regexp.MustCompile(`^\s*pipe_through\s+(?:\[(.*)\]|:([a-zA-Z0-9_]+))`)
	phoenixRouteRegex       = regexp.MustCompile(`^\s*(get|post|put|patch|delete|options|head|connect|trace)\s+['"]([^'"]+)['"]\s*,\s*([A-Za-z0-9_\.]+)\s*,\s*:([a-zA-Z0-9_]+)`)
	phoenixResourceRegex    = regexp.MustCompile(`^\s*resources\s+['"]([^'"]+)['"]\s*,\s*([A-Za-z0-9_\.]+)`)
)

// ParseSource parses an Elixir source file and constructs Merkle-addressable AST Symbol Nodes.
func (p *ElixirParser) ParseSource(filePath string, content []byte, lineage core.LineageEnvelope) (*ElixirParseResult, error) {
	lines := p.splitLines(content)

	result := &ElixirParseResult{
		FilePath: filePath,
	}

	// 1. Extract Modules
	modules := p.extractModules(lines)
	result.Modules = modules

	// 2. Extract Phoenix Routes if router
	routes := p.extractPhoenixRoutes(lines)
	result.Routes = routes

	// 3. Convert extracted entities to Merkle-addressed AST Symbol Nodes
	for _, mod := range result.Modules {
		node, err := ModuleToASTSymbolNode(mod, lineage)
		if err != nil {
			return nil, fmt.Errorf("converting module %s to AST symbol: %w", mod.Name, err)
		}
		result.AllSymbols = append(result.AllSymbols, node)

		for _, fn := range mod.Functions {
			fnNode, err := FunctionToASTSymbolNode(fn, mod.Name, lineage)
			if err != nil {
				return nil, fmt.Errorf("converting function %s/%d to AST symbol: %w", fn.Name, fn.Arity, err)
			}
			result.AllSymbols = append(result.AllSymbols, fnNode)
		}
	}

	for _, r := range result.Routes {
		rNode, err := RouteToASTSymbolNode(r, lineage)
		if err != nil {
			return nil, fmt.Errorf("converting route %s %s to AST symbol: %w", r.Method, r.Path, err)
		}
		result.AllSymbols = append(result.AllSymbols, rNode)
	}

	return result, nil
}

func (p *ElixirParser) extractModules(lines []elixirLineInfo) []ElixirModule {
	var modules []ElixirModule
	var pendingModuledoc string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		// Comments
		if strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		// @moduledoc
		if strings.HasPrefix(trimmed, "@moduledoc") {
			doc, nextI := extractElixirDocString(lines, i, "@moduledoc")
			pendingModuledoc = doc
			i = nextI
			continue
		}

		// Module definition: defmodule MyApp.Accounts do
		if m := elixirModuleRegex.FindStringSubmatch(trimmed); m != nil {
			modName := m[1]
			doc := pendingModuledoc
			pendingModuledoc = ""

			mod, nextI := p.parseModuleBody(lines, i, modName, doc)
			modules = append(modules, mod)
			i = nextI
			continue
		}

		i++
	}

	return modules
}

func extractElixirDocString(lines []elixirLineInfo, startIdx int, prefix string) (string, int) {
	line := strings.TrimSpace(lines[startIdx].text)
	rest := strings.TrimPrefix(line, prefix)
	rest = strings.TrimSpace(rest)

	if rest == "false" {
		return "", startIdx + 1
	}

	// Triple-quoted docstring: """..."""
	if strings.HasPrefix(rest, `"""`) {
		after := strings.TrimPrefix(rest, `"""`)
		if strings.HasSuffix(after, `"""`) && len(after) >= 3 {
			return strings.TrimSuffix(after, `"""`), startIdx + 1
		}
		var docLines []string
		if after != "" {
			docLines = append(docLines, after)
		}
		i := startIdx + 1
		for i < len(lines) {
			t := lines[i].text
			if strings.Contains(t, `"""`) {
				clean := strings.Split(t, `"""`)[0]
				if clean != "" {
					docLines = append(docLines, clean)
				}
				i++
				break
			}
			docLines = append(docLines, t)
			i++
		}
		return strings.TrimSpace(strings.Join(docLines, "\n")), i
	}

	// Single quoted string: "..."
	if strings.HasPrefix(rest, `"`) && strings.HasSuffix(rest, `"`) {
		return strings.Trim(rest, `"`), startIdx + 1
	}

	return rest, startIdx + 1
}

func collectElixirFuncSignature(lines []elixirLineInfo, startIdx int) (string, int) {
	var parts []string
	i := startIdx
	for i < len(lines) {
		line := lines[i].text
		parts = append(parts, line)
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, "do") || strings.Contains(trimmed, "do:") || strings.Contains(trimmed, "defstruct") {
			break
		}
		// Single line without do block or ending line
		if strings.Contains(line, ")") && !strings.Contains(line, "when") {
			break
		}
		i++
	}
	return strings.TrimSpace(strings.Join(parts, " ")), i
}

func (p *ElixirParser) parseModuleBody(lines []elixirLineInfo, startIdx int, name, doc string) (ElixirModule, int) {
	mod := ElixirModule{
		Name:       name,
		Doc:        doc,
		LineNumber: lines[startIdx].num,
	}

	var pendingDoc string
	var pendingSpec string

	i := startIdx + 1
	blockDepth := 1

	for i < len(lines) && blockDepth > 0 {
		lineText := lines[i].text
		trimmed := strings.TrimSpace(lineText)

		if strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		if trimmed == "" {
			i++
			continue
		}

		// @moduledoc
		if strings.HasPrefix(trimmed, "@moduledoc") && blockDepth == 1 {
			d, nextI := extractElixirDocString(lines, i, "@moduledoc")
			mod.Doc = d
			i = nextI
			continue
		}

		// @doc
		if strings.HasPrefix(trimmed, "@doc") {
			d, nextI := extractElixirDocString(lines, i, "@doc")
			pendingDoc = d
			i = nextI
			continue
		}

		// @spec
		if strings.HasPrefix(trimmed, "@spec") {
			pendingSpec = strings.TrimPrefix(trimmed, "@spec")
			pendingSpec = strings.TrimSpace(pendingSpec)
			i++
			continue
		}

		// uses
		if m := elixirUseRegex.FindStringSubmatch(trimmed); m != nil && blockDepth == 1 {
			mod.Uses = append(mod.Uses, m[1])
			if strings.Contains(m[1], "Router") {
				mod.IsRouter = true
			}
			if strings.Contains(m[1], "Controller") {
				mod.IsController = true
			}
			if strings.Contains(m[1], "Schema") {
				mod.IsSchema = true
			}
			i++
			continue
		}

		// imports
		if m := elixirImportRegex.FindStringSubmatch(trimmed); m != nil && blockDepth == 1 {
			mod.Imports = append(mod.Imports, m[1])
			i++
			continue
		}

		// aliases
		if m := elixirAliasRegex.FindStringSubmatch(trimmed); m != nil && blockDepth == 1 {
			mod.Aliases = append(mod.Aliases, m[1])
			i++
			continue
		}

		// requires
		if m := elixirRequireRegex.FindStringSubmatch(trimmed); m != nil && blockDepth == 1 {
			mod.Requires = append(mod.Requires, m[1])
			i++
			continue
		}

		// behaviours
		if m := elixirBehaviorRegex.FindStringSubmatch(trimmed); m != nil && blockDepth == 1 {
			mod.Behaviours = append(mod.Behaviours, m[1])
			i++
			continue
		}

		// plugs
		if m := elixirPlugRegex.FindStringSubmatch(trimmed); m != nil && blockDepth == 1 {
			mod.Plugs = append(mod.Plugs, m[1])
			i++
			continue
		}

		// defstruct
		if m := elixirDefstruct.FindStringSubmatch(trimmed); m != nil && blockDepth == 1 {
			raw := m[1]
			if raw == "" {
				raw = m[2]
			}
			for _, f := range strings.Split(raw, ",") {
				fTrim := strings.TrimSpace(f)
				if fTrim != "" {
					mod.StructFields = append(mod.StructFields, fTrim)
				}
			}
			i++
			continue
		}

		// Ecto schema: schema "users" do ... end
		if m := elixirSchemaRegex.FindStringSubmatch(trimmed); m != nil && blockDepth == 1 {
			mod.IsSchema = true
			mod.SchemaTable = m[1]
			schemaFields, nextI := p.parseSchemaFields(lines, i)
			mod.SchemaFields = schemaFields
			i = nextI
			continue
		}

		// Functions / Macros: def, defp, defmacro, defmacrop
		if elixirFuncStartRegex.MatchString(trimmed) && blockDepth == 1 {
			sig, sigEndIdx := collectElixirFuncSignature(lines, i)
			fn, nextI := p.parseFunction(lines, i, sigEndIdx, sig, pendingDoc, pendingSpec)
			mod.Functions = append(mod.Functions, fn)
			pendingDoc = ""
			pendingSpec = ""
			i = nextI
			continue
		}

		// Block depth tracking for module
		if isElixirBlockStart(trimmed) {
			blockDepth++
		}
		if trimmed == "end" || strings.HasSuffix(trimmed, " end") {
			blockDepth--
			if blockDepth == 0 {
				i++
				break
			}
		}

		pendingDoc = ""
		pendingSpec = ""
		i++
	}

	return mod, i
}

func isElixirBlockStart(line string) bool {
	if strings.HasSuffix(line, " do") || line == "do" {
		return true
	}
	if strings.HasPrefix(line, "defmodule ") ||
		strings.HasPrefix(line, "def ") ||
		strings.HasPrefix(line, "defp ") ||
		strings.HasPrefix(line, "defmacro ") ||
		strings.HasPrefix(line, "defmacrop ") ||
		strings.HasPrefix(line, "schema ") ||
		strings.HasPrefix(line, "scope ") ||
		strings.HasPrefix(line, "pipeline ") ||
		strings.HasPrefix(line, "case ") ||
		strings.HasPrefix(line, "cond ") ||
		strings.HasPrefix(line, "with ") ||
		strings.HasPrefix(line, "for ") ||
		strings.HasPrefix(line, "if ") ||
		strings.HasPrefix(line, "unless ") {
		return strings.Contains(line, " do")
	}
	return false
}

func (p *ElixirParser) parseSchemaFields(lines []elixirLineInfo, startIdx int) ([]ElixirSchemaField, int) {
	var fields []ElixirSchemaField
	i := startIdx + 1
	blockDepth := 1

	for i < len(lines) && blockDepth > 0 {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "field ") {
			if m := elixirFieldRegex.FindStringSubmatch(trimmed); m != nil {
				fName := m[1]
				fType := strings.TrimSpace(m[2])
				if fType == "" {
					fType = ":string"
				}
				fields = append(fields, ElixirSchemaField{
					Name: fName,
					Type: fType,
				})
			}
		} else if m := elixirAssocRegex.FindStringSubmatch(trimmed); m != nil {
			assocKind := m[1]
			assocName := m[2]
			targetSchema := m[3]
			fields = append(fields, ElixirSchemaField{
				Name:            assocName,
				AssociationType: assocKind,
				TargetSchema:    targetSchema,
			})
		}

		if isElixirBlockStart(trimmed) {
			blockDepth++
		}
		if trimmed == "end" || strings.HasSuffix(trimmed, " end") {
			blockDepth--
			if blockDepth == 0 {
				i++
				break
			}
		}
		i++
	}

	return fields, i
}

func (p *ElixirParser) parseFunction(
	lines []elixirLineInfo,
	startIdx, sigEndIdx int,
	sig, doc, spec string,
) (ElixirFunction, int) {
	fn := ElixirFunction{
		Doc:        doc,
		Spec:       spec,
		LineNumber: lines[startIdx].num,
	}

	// Parse header from sig: def show(conn, %{"id" => id}) when is_binary(id) do
	kind := "def"
	if strings.HasPrefix(sig, "defp") {
		kind = "defp"
	} else if strings.HasPrefix(sig, "defmacrop") {
		kind = "defmacrop"
	} else if strings.HasPrefix(sig, "defmacro") {
		kind = "defmacro"
	}
	fn.Kind = kind

	cleanSig := strings.TrimPrefix(sig, kind)
	cleanSig = strings.TrimSpace(cleanSig)

	// Guard clause: when ...
	if strings.Contains(cleanSig, " when ") {
		parts := strings.SplitN(cleanSig, " when ", 2)
		cleanSig = parts[0]
		guardPart := parts[1]
		guardPart = strings.TrimSuffix(guardPart, " do")
		guardPart = strings.TrimSuffix(guardPart, "do")
		fn.GuardClause = strings.TrimSpace(guardPart)
	}

	// Function name & params
	if openIdx := strings.Index(cleanSig, "("); openIdx != -1 {
		fn.Name = strings.TrimSpace(cleanSig[:openIdx])
		closeIdx := strings.LastIndex(cleanSig, ")")
		if closeIdx != -1 && closeIdx > openIdx {
			paramStr := cleanSig[openIdx+1 : closeIdx]
			fn.Params = parseElixirParams(paramStr)
		}
	} else {
		// No parens: def hello do or def hello, do: :world
		nameAndRest := strings.TrimSuffix(cleanSig, " do")
		nameAndRest = strings.TrimSuffix(nameAndRest, "do")
		if commaIdx := strings.Index(nameAndRest, ","); commaIdx != -1 {
			nameAndRest = nameAndRest[:commaIdx]
		}
		fn.Name = strings.TrimSpace(nameAndRest)
	}
	fn.Arity = len(fn.Params)

	// Single line function: def hello, do: :world
	if strings.Contains(sig, "do:") {
		fn.BodySource = sig
		fn.CalledFunctions = extractElixirCalls(sig)
		return fn, sigEndIdx + 1
	}

	// Multi-line block function
	var bodyLines []string
	i := startIdx
	blockDepth := 0
	hasSeenDo := false

	for i < len(lines) {
		lineText := lines[i].text
		trimmed := strings.TrimSpace(lineText)
		bodyLines = append(bodyLines, lineText)

		if strings.HasSuffix(trimmed, " do") || trimmed == "do" || strings.Contains(trimmed, " do ") {
			blockDepth++
			hasSeenDo = true
		} else if isElixirBlockStart(trimmed) {
			blockDepth++
		}

		if trimmed == "end" || strings.HasSuffix(trimmed, " end") {
			blockDepth--
			if hasSeenDo && blockDepth <= 0 {
				i++
				break
			}
		}
		i++
	}

	fullBody := strings.Join(bodyLines, "\n")
	fn.BodySource = fullBody
	fn.CalledFunctions = extractElixirCalls(fullBody)

	return fn, i
}

func parseElixirParams(paramStr string) []ElixirParam {
	var params []ElixirParam
	if strings.TrimSpace(paramStr) == "" {
		return params
	}

	// Handle commas inside patterns %{...}, [...], etc.
	var current []rune
	depth := 0
	inString := false

	for _, r := range paramStr {
		if r == '"' {
			inString = !inString
		}
		if !inString {
			if r == '{' || r == '[' || r == '(' {
				depth++
			} else if r == '}' || r == ']' || r == ')' {
				depth--
			} else if r == ',' && depth == 0 {
				p := strings.TrimSpace(string(current))
				if p != "" {
					params = append(params, parseSingleElixirParam(p))
				}
				current = nil
				continue
			}
		}
		current = append(current, r)
	}

	if len(current) > 0 {
		p := strings.TrimSpace(string(current))
		if p != "" {
			params = append(params, parseSingleElixirParam(p))
		}
	}

	return params
}

func parseSingleElixirParam(p string) ElixirParam {
	// Default value: param \\ default_value
	if strings.Contains(p, `\\`) {
		parts := strings.SplitN(p, `\\`, 2)
		return ElixirParam{
			Pattern:      strings.TrimSpace(parts[0]),
			DefaultValue: strings.TrimSpace(parts[1]),
		}
	}
	return ElixirParam{
		Pattern: strings.TrimSpace(p),
	}
}

func extractElixirCalls(body string) []ElixirCalls {
	callRegex := regexp.MustCompile(`(?:([A-Za-z0-9_\.]+)\.)?([a-zA-Z0-9_!\?]+)\s*\(`)
	matches := callRegex.FindAllStringSubmatch(body, -1)
	var calls []string
	seen := make(map[string]bool)

	for _, m := range matches {
		mod := m[1]
		fn := m[2]
		if fn == "def" || fn == "defp" || fn == "schema" || fn == "field" || fn == "use" || fn == "import" {
			continue
		}
		full := fn
		if mod != "" {
			full = fmt.Sprintf("%s.%s", mod, fn)
		}
		if !seen[full] {
			seen[full] = true
			calls = append(calls, full)
		}
	}
	return calls
}

type ElixirCalls = string

func (p *ElixirParser) extractPhoenixRoutes(lines []elixirLineInfo) []ElixirRouteBinding {
	var routes []ElixirRouteBinding

	var scopeStack []string
	var currentPipes []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		// scope "/api/v1", MyAppWeb do
		if m := phoenixScopeRegex.FindStringSubmatch(trimmed); m != nil {
			prefix := m[1]
			scopeStack = append(scopeStack, prefix)
		}

		// pipe_through [:api, :authenticated]
		if m := phoenixPipeThroughRegex.FindStringSubmatch(trimmed); m != nil {
			pipeList := m[1]
			singlePipe := m[2]
			var pipes []string
			if pipeList != "" {
				for _, p := range strings.Split(pipeList, ",") {
					pTrim := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(p), ":"))
					if pTrim != "" {
						pipes = append(pipes, pTrim)
					}
				}
			} else if singlePipe != "" {
				pipes = append(pipes, singlePipe)
			}
			currentPipes = pipes
		}

		// get "/users/:id", UserController, :show
		if m := phoenixRouteRegex.FindStringSubmatch(trimmed); m != nil {
			httpMethod := strings.ToUpper(m[1])
			path := m[2]
			controller := m[3]
			action := m[4]

			fullPath := combineElixirPaths(strings.Join(scopeStack, ""), path)
			routes = append(routes, ElixirRouteBinding{
				Method:      httpMethod,
				Path:        fullPath,
				Controller:  controller,
				Action:      action,
				PipeThrough: currentPipes,
				Framework:   "phoenix",
				LineNumber:  lines[i].num,
			})
		}

		// resources "/users", UserController
		if m := phoenixResourceRegex.FindStringSubmatch(trimmed); m != nil {
			resPath := m[1]
			controller := m[2]
			fullPath := combineElixirPaths(strings.Join(scopeStack, ""), resPath)

			// Phoenix resources creates standard REST endpoints: index, show, create, update, delete
			routes = append(routes,
				ElixirRouteBinding{
					Method:      "GET",
					Path:        fullPath,
					Controller:  controller,
					Action:      "index",
					PipeThrough: currentPipes,
					Framework:   "phoenix",
					LineNumber:  lines[i].num,
				},
				ElixirRouteBinding{
					Method:      "GET",
					Path:        combineElixirPaths(fullPath, ":id"),
					Controller:  controller,
					Action:      "show",
					PipeThrough: currentPipes,
					Framework:   "phoenix",
					LineNumber:  lines[i].num,
				},
				ElixirRouteBinding{
					Method:      "POST",
					Path:        fullPath,
					Controller:  controller,
					Action:      "create",
					PipeThrough: currentPipes,
					Framework:   "phoenix",
					LineNumber:  lines[i].num,
				},
			)
		}

		// End scope
		if (trimmed == "end" || strings.HasSuffix(trimmed, " end")) && len(scopeStack) > 0 {
			scopeStack = scopeStack[:len(scopeStack)-1]
		}

		i++
	}

	return routes
}

func combineElixirPaths(base, path string) string {
	base = strings.TrimSuffix(base, "/")
	path = strings.TrimPrefix(path, "/")
	if base == "" {
		return "/" + path
	}
	if path == "" {
		return base
	}
	return base + "/" + path
}

// ModuleToASTSymbolNode converts an ElixirModule into a Merkle-addressable core.ASTSymbolNode.
func ModuleToASTSymbolNode(m ElixirModule, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshaling ElixirModule: %w", err)
	}

	var deps []string
	deps = append(deps, m.Uses...)
	deps = append(deps, m.Imports...)
	deps = append(deps, m.Aliases...)
	deps = append(deps, m.Requires...)
	deps = append(deps, m.Behaviours...)

	meta := map[string]string{
		"module_name":    m.Name,
		"is_router":      fmt.Sprintf("%t", m.IsRouter),
		"is_controller":  fmt.Sprintf("%t", m.IsController),
		"is_schema":      fmt.Sprintf("%t", m.IsSchema),
		"schema_table":   m.SchemaTable,
		"function_count": fmt.Sprintf("%d", len(m.Functions)),
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangElixir,
		NodeType:          "ModuleDef",
		Identifier:        m.Name,
		Signature:         fmt.Sprintf("defmodule %s", m.Name),
		Docstring:         m.Doc,
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

// FunctionToASTSymbolNode converts an ElixirFunction into a Merkle-addressable core.ASTSymbolNode.
func FunctionToASTSymbolNode(fn ElixirFunction, moduleName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(fn)
	if err != nil {
		return nil, fmt.Errorf("marshaling ElixirFunction: %w", err)
	}

	ident := fmt.Sprintf("%s.%s/%d", moduleName, fn.Name, fn.Arity)
	nodeType := "FunctionDef"
	if fn.Kind == "defmacro" || fn.Kind == "defmacrop" {
		nodeType = "MacroDef"
	}

	vis := "public"
	if fn.Kind == "defp" || fn.Kind == "defmacrop" {
		vis = "private"
	}

	meta := map[string]string{
		"kind":         fn.Kind,
		"arity":        fmt.Sprintf("%d", fn.Arity),
		"module":       moduleName,
		"guard_clause": fn.GuardClause,
		"spec":         fn.Spec,
	}

	sig := fmt.Sprintf("%s %s/%d", fn.Kind, fn.Name, fn.Arity)
	if fn.Spec != "" {
		sig = fn.Spec
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangElixir,
		NodeType:          nodeType,
		Identifier:        ident,
		Signature:         sig,
		Docstring:         fn.Doc,
		Visibility:        vis,
		ASTPayload:        payload,
		ASTMetadata:       meta,
		LocalDependencies: fn.CalledFunctions,
		Dependencies:      fn.CalledFunctions,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// RouteToASTSymbolNode converts an ElixirRouteBinding into a Merkle-addressable core.ASTSymbolNode.
func RouteToASTSymbolNode(r ElixirRouteBinding, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("marshaling ElixirRouteBinding: %w", err)
	}

	meta := map[string]string{
		"http_method":  r.Method,
		"route_path":   r.Path,
		"controller":   r.Controller,
		"action":       r.Action,
		"pipe_through": strings.Join(r.PipeThrough, ","),
		"framework":    r.Framework,
	}

	var deps []string
	if r.Controller != "" {
		deps = append(deps, r.Controller)
	}

	id := fmt.Sprintf("%s %s", r.Method, r.Path)

	node := &core.ASTSymbolNode{
		Language:          core.LangElixir,
		NodeType:          "RouteBinding",
		Identifier:        id,
		Signature:         fmt.Sprintf("%s %s -> %s.%s", r.Method, r.Path, r.Controller, r.Action),
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

// BuildComponentNode bundles an ElixirParseResult into a Merkle-addressable core.ComponentNode.
func (p *ElixirParser) BuildComponentNode(
	res *ElixirParseResult,
	componentName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if res == nil {
		return nil, fmt.Errorf("parse result is nil")
	}

	if componentName == "" {
		componentName = strings.TrimSuffix(filepath.Base(res.FilePath), filepath.Ext(res.FilePath))
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
		"module_count": fmt.Sprintf("%d", len(res.Modules)),
		"route_count":  fmt.Sprintf("%d", len(res.Routes)),
		"symbol_count": fmt.Sprintf("%d", len(res.AllSymbols)),
	}

	comp := &core.ComponentNode{
		Name:        componentName,
		Type:        compType,
		Language:    core.LangElixir,
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
