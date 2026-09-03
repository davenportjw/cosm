package ruby

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

// RubyParam represents a method parameter in Ruby.
type RubyParam struct {
	Name         string `json:"name"`
	Kind         string `json:"kind,omitempty"` // req, opt, key, keyreq, rest, block
	DefaultValue string `json:"default_value,omitempty"`
}

// RubyMethod represents an extracted Ruby method or class method definition.
type RubyMethod struct {
	Name          string      `json:"name"`
	IsClassMethod bool        `json:"is_class_method"`
	Visibility    string      `json:"visibility,omitempty"` // public, private, protected
	Params        []RubyParam `json:"params"`
	Doc           string      `json:"doc,omitempty"`
	BodySource    string      `json:"body_source,omitempty"`
	CalledMethods []string    `json:"called_methods,omitempty"`
}

// RubyAssociation represents an ActiveRecord association (e.g. has_many :posts, belongs_to :user).
type RubyAssociation struct {
	Kind    string            `json:"kind"` // has_many, belongs_to, has_one, has_and_belongs_to_many
	Target  string            `json:"target"`
	Options map[string]string `json:"options,omitempty"`
	Raw     string            `json:"raw,omitempty"`
}

// RubyValidation represents an ActiveRecord validation (e.g. validates :email, presence: true).
type RubyValidation struct {
	Fields  []string          `json:"fields"`
	Options map[string]string `json:"options,omitempty"`
	Raw     string            `json:"raw,omitempty"`
}

// RubyClass represents an extracted Ruby class definition.
type RubyClass struct {
	Name            string            `json:"name"`
	Superclass      string            `json:"superclass,omitempty"`
	Doc             string            `json:"doc,omitempty"`
	ModulesIncluded []string          `json:"modules_included,omitempty"`
	ModulesExtended []string          `json:"modules_extended,omitempty"`
	AttrAccessors   []string          `json:"attr_accessors,omitempty"`
	AttrReaders     []string          `json:"attr_readers,omitempty"`
	AttrWriters     []string          `json:"attr_writers,omitempty"`
	Associations    []RubyAssociation `json:"associations,omitempty"`
	Validations     []RubyValidation  `json:"validations,omitempty"`
	Methods         []RubyMethod      `json:"methods"`
	IsController    bool              `json:"is_controller"`
	IsModel         bool              `json:"is_model"`
}

// RubyModule represents an extracted Ruby module definition.
type RubyModule struct {
	Name            string       `json:"name"`
	Doc             string       `json:"doc,omitempty"`
	ModulesIncluded []string     `json:"modules_included,omitempty"`
	ModulesExtended []string     `json:"modules_extended,omitempty"`
	Methods         []RubyMethod `json:"methods"`
}

// RubyRouteBinding represents an HTTP endpoint discovered in Ruby code (Rails routes.rb or Sinatra).
type RubyRouteBinding struct {
	Method      string `json:"method"` // GET, POST, PUT, PATCH, DELETE, RESOURCES, ANY
	Path        string `json:"path"`   // /api/v1/users, /users/:id
	Controller  string `json:"controller,omitempty"`
	Action      string `json:"action,omitempty"`
	HandlerName string `json:"handler_name"` // e.g. UsersController#index
	Framework   string `json:"framework"`    // rails, sinatra
	LineNumber  int    `json:"line_number"`
}

// RubyFileResult contains all extracted symbols and metadata from a Ruby source file.
type RubyFileResult struct {
	FilePath    string                `json:"file_path"`
	PackageName string                `json:"package_name"`
	Requires    []string              `json:"requires"`
	Classes     []RubyClass           `json:"classes"`
	Modules     []RubyModule          `json:"modules"`
	Methods     []RubyMethod          `json:"methods"`
	Routes      []RubyRouteBinding    `json:"routes"`
	AllSymbols  []*core.ASTSymbolNode `json:"all_symbols"`
}

// RubyParser provides Ruby AST parsing, symbol extraction, and framework metadata extraction.
type RubyParser struct{}

// NewRubyParser creates a new RubyParser instance.
func NewRubyParser() *RubyParser {
	return &RubyParser{}
}

// ParseSource parses Ruby source code bytes and extracts all symbols.
func (p *RubyParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*RubyFileResult, error) {
	if filename == "" {
		filename = "app.rb"
	}

	result := &RubyFileResult{
		FilePath:    filename,
		PackageName: p.derivePackageName(filename),
	}

	lines := splitRubyLines(src)

	// 1. Extract Requires
	result.Requires = p.extractRequires(lines)

	// 2. Extract Classes & Modules
	result.Classes, result.Modules, result.Methods = p.extractDefinitions(lines)

	// 3. Extract Routes (Rails routes & Sinatra)
	result.Routes = p.extractRoutes(lines, filename)

	// Convert all extracted entities to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a Ruby source file from disk.
func (p *RubyParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*RubyFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .rb files in a directory.
func (p *RubyParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*RubyFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*RubyFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".rb" {
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

type rubyLineInfo struct {
	num  int
	text string
}

func splitRubyLines(src []byte) []rubyLineInfo {
	var lines []rubyLineInfo
	scanner := bufio.NewScanner(bytes.NewReader(src))
	num := 1
	for scanner.Scan() {
		lines = append(lines, rubyLineInfo{
			num:  num,
			text: scanner.Text(),
		})
		num++
	}
	return lines
}

func (p *RubyParser) derivePackageName(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	dir := filepath.Base(filepath.Dir(filename))
	if dir != "." && dir != "/" && dir != "" {
		return fmt.Sprintf("%s/%s", dir, name)
	}
	return name
}

var (
	rubyRequireRegex   = regexp.MustCompile(`^(?:require|require_relative)\s+["']([^"']+)["']`)
	rubyClassRegex     = regexp.MustCompile(`^class\s+([A-Za-z0-9_:]+)(?:\s*<\s*([A-Za-z0-9_:]+))?`)
	rubyModuleRegex    = regexp.MustCompile(`^module\s+([A-Za-z0-9_:]+)`)
	rubyDefRegex       = regexp.MustCompile(`^def\s+(self\.)?([a-zA-Z0-9_!?=]+)(?:\s*\((.*?)\)|\s+(.*?))?$`)
	rubyAssocRegex     = regexp.MustCompile(`^(has_many|belongs_to|has_one|has_and_belongs_to_many)\s+:([a-zA-Z0-9_]+)(?:,\s*(.*))?`)
	rubyValidatesRegex = regexp.MustCompile(`^validates\s+(:[a-zA-Z0-9_]+(?:\s*,\s*:[a-zA-Z0-9_]+)*)(?:,\s*(.*))?`)
	rubyAttrAccRegex   = regexp.MustCompile(`^attr_accessor\s+(.*)`)
	rubyAttrReadRegex  = regexp.MustCompile(`^attr_reader\s+(.*)`)
	rubyAttrWriteRegex = regexp.MustCompile(`^attr_writer\s+(.*)`)
	rubyIncludeRegex   = regexp.MustCompile(`^include\s+([A-Za-z0-9_:]+)`)
	rubyExtendRegex    = regexp.MustCompile(`^extend\s+([A-Za-z0-9_:]+)`)

	// Rails route regexes
	railsRouteRegex     = regexp.MustCompile(`^(get|post|put|patch|delete|match)\s+["']([^"']+)["'](?:\s*,\s*(?:to:|=>)\s*["']([^"']+)["'])?`)
	railsResourcesRegex = regexp.MustCompile(`^resources\s+:([a-zA-Z0-9_]+)`)
	railsRootRegex      = regexp.MustCompile(`^root\s+(?:to:\s*)?["']([^"']+)["']`)
	railsNamespaceRegex = regexp.MustCompile(`^namespace\s+:([a-zA-Z0-9_]+)\s+do`)
	railsScopeRegex     = regexp.MustCompile(`^scope\s+["']([^"']+)["']\s+do`)

	// Sinatra route regex
	sinatraRouteRegex = regexp.MustCompile(`^(get|post|put|patch|delete)\s+["']([^"']+)["']\s+do`)
)

func (p *RubyParser) extractRequires(lines []rubyLineInfo) []string {
	var requires []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if m := rubyRequireRegex.FindStringSubmatch(trimmed); m != nil {
			requires = append(requires, m[1])
		}
	}
	return requires
}

func (p *RubyParser) extractDefinitions(lines []rubyLineInfo) ([]RubyClass, []RubyModule, []RubyMethod) {
	var classes []RubyClass
	var modules []RubyModule
	var topMethods []RubyMethod

	var pendingDoc []string
	i := 0

	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)

		// Collect comment lines as documentation
		if strings.HasPrefix(trimmed, "#") {
			docLine := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			if !strings.HasPrefix(docLine, "frozen_string_literal") {
				pendingDoc = append(pendingDoc, docLine)
			}
			i++
			continue
		}

		if trimmed == "" {
			i++
			continue
		}

		// Check Class
		if m := rubyClassRegex.FindStringSubmatch(trimmed); m != nil {
			className := m[1]
			superclass := m[2]
			docStr := strings.Join(pendingDoc, "\n")
			pendingDoc = nil

			cls, nextI := p.parseClass(lines, i, className, superclass, docStr)
			classes = append(classes, cls)
			i = nextI
			continue
		}

		// Check Module
		if m := rubyModuleRegex.FindStringSubmatch(trimmed); m != nil {
			modName := m[1]
			docStr := strings.Join(pendingDoc, "\n")
			pendingDoc = nil

			mod, nextI := p.parseModule(lines, i, modName, docStr)
			modules = append(modules, mod)
			i = nextI
			continue
		}

		// Check Top-level Method
		if m := rubyDefRegex.FindStringSubmatch(trimmed); m != nil {
			docStr := strings.Join(pendingDoc, "\n")
			pendingDoc = nil

			method, nextI := p.parseMethod(lines, i, "public", docStr)
			topMethods = append(topMethods, method)
			i = nextI
			continue
		}

		pendingDoc = nil
		i++
	}

	return classes, modules, topMethods
}

func (p *RubyParser) parseClass(lines []rubyLineInfo, startIdx int, name, superclass, doc string) (RubyClass, int) {
	cls := RubyClass{
		Name:         name,
		Superclass:   superclass,
		Doc:          doc,
		IsController: strings.HasSuffix(name, "Controller") || strings.Contains(superclass, "Controller"),
		IsModel:      superclass == "ApplicationRecord" || superclass == "ActiveRecord::Base",
	}

	currentVisibility := "public"
	var pendingDoc []string

	i := startIdx + 1
	blockDepth := 1

	for i < len(lines) && blockDepth > 0 {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "#") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "#")))
			i++
			continue
		}

		if trimmed == "" {
			i++
			continue
		}

		// Visibility switches
		if trimmed == "private" {
			currentVisibility = "private"
			pendingDoc = nil
			i++
			continue
		} else if trimmed == "protected" {
			currentVisibility = "protected"
			pendingDoc = nil
			i++
			continue
		} else if trimmed == "public" {
			currentVisibility = "public"
			pendingDoc = nil
			i++
			continue
		}

		// Check for method definition
		if rubyDefRegex.MatchString(trimmed) {
			docStr := strings.Join(pendingDoc, "\n")
			pendingDoc = nil
			method, nextI := p.parseMethod(lines, i, currentVisibility, docStr)
			cls.Methods = append(cls.Methods, method)
			i = nextI
			continue
		}

		// General block start / end tracking for non-method constructs inside class
		if isBlockStart(trimmed) {
			blockDepth++
		}
		if isBlockEnd(trimmed) {
			blockDepth--
			if blockDepth == 0 {
				i++
				break
			}
			pendingDoc = nil
			i++
			continue
		}

		// Associations
		if m := rubyAssocRegex.FindStringSubmatch(trimmed); m != nil {
			kind := m[1]
			target := m[2]
			optStr := m[3]
			opts := parseRubyOptions(optStr)
			cls.Associations = append(cls.Associations, RubyAssociation{
				Kind:    kind,
				Target:  target,
				Options: opts,
				Raw:     trimmed,
			})
			pendingDoc = nil
			i++
			continue
		}

		// Validations
		if m := rubyValidatesRegex.FindStringSubmatch(trimmed); m != nil {
			fieldsRaw := m[1]
			optStr := m[2]
			var fields []string
			for _, f := range strings.Split(fieldsRaw, ",") {
				fClean := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(f), ":"))
				if fClean != "" {
					fields = append(fields, fClean)
				}
			}
			opts := parseRubyOptions(optStr)
			cls.Validations = append(cls.Validations, RubyValidation{
				Fields:  fields,
				Options: opts,
				Raw:     trimmed,
			})
			pendingDoc = nil
			i++
			continue
		}

		// Attr accessors
		if m := rubyAttrAccRegex.FindStringSubmatch(trimmed); m != nil {
			cls.AttrAccessors = append(cls.AttrAccessors, parseSymbolList(m[1])...)
			pendingDoc = nil
			i++
			continue
		}
		if m := rubyAttrReadRegex.FindStringSubmatch(trimmed); m != nil {
			cls.AttrReaders = append(cls.AttrReaders, parseSymbolList(m[1])...)
			pendingDoc = nil
			i++
			continue
		}
		if m := rubyAttrWriteRegex.FindStringSubmatch(trimmed); m != nil {
			cls.AttrWriters = append(cls.AttrWriters, parseSymbolList(m[1])...)
			pendingDoc = nil
			i++
			continue
		}

		// Includes / Extends
		if m := rubyIncludeRegex.FindStringSubmatch(trimmed); m != nil {
			cls.ModulesIncluded = append(cls.ModulesIncluded, m[1])
			pendingDoc = nil
			i++
			continue
		}
		if m := rubyExtendRegex.FindStringSubmatch(trimmed); m != nil {
			cls.ModulesExtended = append(cls.ModulesExtended, m[1])
			pendingDoc = nil
			i++
			continue
		}

		pendingDoc = nil
		i++
	}

	return cls, i
}

func (p *RubyParser) parseModule(lines []rubyLineInfo, startIdx int, name, doc string) (RubyModule, int) {
	mod := RubyModule{
		Name: name,
		Doc:  doc,
	}

	currentVisibility := "public"
	var pendingDoc []string

	i := startIdx + 1
	blockDepth := 1

	for i < len(lines) && blockDepth > 0 {
		trimmed := strings.TrimSpace(lines[i].text)

		if strings.HasPrefix(trimmed, "#") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "#")))
			i++
			continue
		}

		if trimmed == "" {
			i++
			continue
		}

		if trimmed == "private" {
			currentVisibility = "private"
			pendingDoc = nil
			i++
			continue
		} else if trimmed == "protected" {
			currentVisibility = "protected"
			pendingDoc = nil
			i++
			continue
		} else if trimmed == "public" {
			currentVisibility = "public"
			pendingDoc = nil
			i++
			continue
		}

		if rubyDefRegex.MatchString(trimmed) {
			docStr := strings.Join(pendingDoc, "\n")
			pendingDoc = nil
			method, nextI := p.parseMethod(lines, i, currentVisibility, docStr)
			mod.Methods = append(mod.Methods, method)
			i = nextI
			continue
		}

		if isBlockStart(trimmed) {
			blockDepth++
		}
		if isBlockEnd(trimmed) {
			blockDepth--
			if blockDepth == 0 {
				i++
				break
			}
			pendingDoc = nil
			i++
			continue
		}

		if m := rubyIncludeRegex.FindStringSubmatch(trimmed); m != nil {
			mod.ModulesIncluded = append(mod.ModulesIncluded, m[1])
			pendingDoc = nil
			i++
			continue
		}
		if m := rubyExtendRegex.FindStringSubmatch(trimmed); m != nil {
			mod.ModulesExtended = append(mod.ModulesExtended, m[1])
			pendingDoc = nil
			i++
			continue
		}

		pendingDoc = nil
		i++
	}

	return mod, i
}

func (p *RubyParser) parseMethod(lines []rubyLineInfo, startIdx int, visibility, doc string) (RubyMethod, int) {
	header := strings.TrimSpace(lines[startIdx].text)
	m := rubyDefRegex.FindStringSubmatch(header)

	isClassMethod := false
	methodName := ""
	paramsStr := ""

	if m != nil {
		isClassMethod = m[1] != ""
		methodName = m[2]
		if m[3] != "" {
			paramsStr = m[3]
		} else if m[4] != "" {
			paramsStr = m[4]
		}
	}

	params := parseRubyParams(paramsStr)

	var bodyLines []string
	bodyLines = append(bodyLines, lines[startIdx].text)

	i := startIdx + 1
	blockDepth := 1

	for i < len(lines) && blockDepth > 0 {
		trimmed := strings.TrimSpace(lines[i].text)
		bodyLines = append(bodyLines, lines[i].text)

		if isBlockStart(trimmed) {
			blockDepth++
		}
		if isBlockEnd(trimmed) {
			blockDepth--
			if blockDepth == 0 {
				i++
				break
			}
		}
		i++
	}

	fullBody := strings.Join(bodyLines, "\n")
	calledMethods := extractRubyCalls(fullBody)

	return RubyMethod{
		Name:          methodName,
		IsClassMethod: isClassMethod,
		Visibility:    visibility,
		Params:        params,
		Doc:           doc,
		BodySource:    fullBody,
		CalledMethods: calledMethods,
	}, i
}

func (p *RubyParser) extractRoutes(lines []rubyLineInfo, filename string) []RubyRouteBinding {
	var routes []RubyRouteBinding
	var currentNamespace []string
	var currentScope []string

	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)

		if m := railsNamespaceRegex.FindStringSubmatch(trimmed); m != nil {
			currentNamespace = append(currentNamespace, m[1])
			continue
		}
		if m := railsScopeRegex.FindStringSubmatch(trimmed); m != nil {
			cleanScope := strings.Trim(m[1], "/")
			currentScope = append(currentScope, cleanScope)
			continue
		}
		if trimmed == "end" {
			if len(currentNamespace) > 0 {
				currentNamespace = currentNamespace[:len(currentNamespace)-1]
			} else if len(currentScope) > 0 {
				currentScope = currentScope[:len(currentScope)-1]
			}
			continue
		}

		// Rails Route: get '/users', to: 'users#index'
		if m := railsRouteRegex.FindStringSubmatch(trimmed); m != nil {
			httpMethod := strings.ToUpper(m[1])
			path := m[2]
			target := m[3]

			fullPath := buildFullPath(currentScope, currentNamespace, path)
			controller, action := parseControllerAction(target)

			handler := target
			if handler == "" && controller != "" {
				handler = fmt.Sprintf("%s#%s", controller, action)
			}

			routes = append(routes, RubyRouteBinding{
				Method:      httpMethod,
				Path:        fullPath,
				Controller:  controller,
				Action:      action,
				HandlerName: handler,
				Framework:   "rails",
				LineNumber:  l.num,
			})
			continue
		}

		// Rails Route: resources :users
		if m := railsResourcesRegex.FindStringSubmatch(trimmed); m != nil {
			resName := m[1]
			fullPath := buildFullPath(currentScope, currentNamespace, "/"+resName)
			controller := resName
			if len(currentNamespace) > 0 {
				controller = strings.Join(currentNamespace, "/") + "/" + resName
			}

			routes = append(routes, RubyRouteBinding{
				Method:      "RESOURCES",
				Path:        fullPath,
				Controller:  controller,
				HandlerName: fmt.Sprintf("%s_controller", resName),
				Framework:   "rails",
				LineNumber:  l.num,
			})
			continue
		}

		// Rails Root Route: root 'home#index'
		if m := railsRootRegex.FindStringSubmatch(trimmed); m != nil {
			target := m[1]
			controller, action := parseControllerAction(target)
			routes = append(routes, RubyRouteBinding{
				Method:      "GET",
				Path:        "/",
				Controller:  controller,
				Action:      action,
				HandlerName: target,
				Framework:   "rails",
				LineNumber:  l.num,
			})
			continue
		}

		// Sinatra Route: get '/users' do
		if m := sinatraRouteRegex.FindStringSubmatch(trimmed); m != nil {
			httpMethod := strings.ToUpper(m[1])
			path := m[2]
			routes = append(routes, RubyRouteBinding{
				Method:      httpMethod,
				Path:        path,
				HandlerName: fmt.Sprintf("%s %s", httpMethod, path),
				Framework:   "sinatra",
				LineNumber:  l.num,
			})
			continue
		}
	}

	return routes
}

func buildFullPath(scopes, namespaces []string, path string) string {
	var parts []string
	for _, ns := range namespaces {
		parts = append(parts, ns)
	}
	for _, sc := range scopes {
		parts = append(parts, sc)
	}
	cleanPath := strings.Trim(path, "/")
	if cleanPath != "" {
		parts = append(parts, cleanPath)
	}
	return "/" + strings.Join(parts, "/")
}

func parseControllerAction(target string) (string, string) {
	if strings.Contains(target, "#") {
		parts := strings.SplitN(target, "#", 2)
		return parts[0], parts[1]
	}
	return target, ""
}

func parseRubyParams(paramsStr string) []RubyParam {
	var params []RubyParam
	paramsStr = strings.TrimSpace(paramsStr)
	if paramsStr == "" {
		return params
	}

	for _, p := range strings.Split(paramsStr, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		kind := "req"
		defVal := ""
		name := p

		if strings.HasPrefix(p, "&") {
			kind = "block"
			name = strings.TrimPrefix(p, "&")
		} else if strings.HasPrefix(p, "*") {
			kind = "rest"
			name = strings.TrimPrefix(p, "*")
		} else if strings.Contains(p, ":") {
			parts := strings.SplitN(p, ":", 2)
			name = strings.TrimSpace(parts[0])
			if strings.TrimSpace(parts[1]) != "" {
				kind = "key"
				defVal = strings.TrimSpace(parts[1])
			} else {
				kind = "keyreq"
			}
		} else if strings.Contains(p, "=") {
			parts := strings.SplitN(p, "=", 2)
			name = strings.TrimSpace(parts[0])
			defVal = strings.TrimSpace(parts[1])
			kind = "opt"
		}

		params = append(params, RubyParam{
			Name:         name,
			Kind:         kind,
			DefaultValue: defVal,
		})
	}
	return params
}

func parseRubyOptions(optStr string) map[string]string {
	opts := make(map[string]string)
	optStr = strings.TrimSpace(optStr)
	if optStr == "" {
		return opts
	}

	pairs := strings.Split(optStr, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if strings.Contains(pair, ":") {
			parts := strings.SplitN(pair, ":", 2)
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			opts[k] = v
		} else if strings.Contains(pair, "=>") {
			parts := strings.SplitN(pair, "=>", 2)
			k := strings.Trim(strings.TrimSpace(parts[0]), ":\"'")
			v := strings.TrimSpace(parts[1])
			opts[k] = v
		}
	}
	return opts
}

func parseSymbolList(raw string) []string {
	var symbols []string
	for _, part := range strings.Split(raw, ",") {
		s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), ":"))
		if s != "" {
			symbols = append(symbols, s)
		}
	}
	return symbols
}

func isBlockStart(line string) bool {
	// Matches Ruby constructs that introduce a block ending in 'end'
	if rubyDefRegex.MatchString(line) ||
		rubyClassRegex.MatchString(line) ||
		rubyModuleRegex.MatchString(line) ||
		strings.HasPrefix(line, "if ") ||
		strings.HasPrefix(line, "unless ") ||
		strings.HasPrefix(line, "case ") ||
		strings.HasPrefix(line, "while ") ||
		strings.HasPrefix(line, "until ") ||
		strings.HasPrefix(line, "for ") ||
		strings.HasPrefix(line, "begin") ||
		strings.HasSuffix(line, " do") ||
		strings.Contains(line, " do |") {
		return true
	}
	return false
}

func isBlockEnd(line string) bool {
	return line == "end" || strings.HasPrefix(line, "end ") || strings.HasPrefix(line, "end#")
}

var rubyCallRegex = regexp.MustCompile(`\b([a-zA-Z0-9_]+)\s*(?:\(|\s+[:"'])`)

func extractRubyCalls(src string) []string {
	var calls []string
	seen := make(map[string]bool)
	matches := rubyCallRegex.FindAllStringSubmatch(src, -1)
	keywords := map[string]bool{
		"def": true, "class": true, "module": true, "if": true, "unless": true,
		"case": true, "when": true, "while": true, "until": true, "begin": true,
		"rescue": true, "ensure": true, "end": true, "return": true, "require": true,
		"include": true, "extend": true, "validates": true, "has_many": true,
		"belongs_to": true, "has_one": true, "attr_accessor": true,
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

func (p *RubyParser) toSymbolNodes(res *RubyFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Classes
	for _, cls := range res.Classes {
		node, err := ClassToASTSymbolNode(cls, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Modules
	for _, mod := range res.Modules {
		node, err := ModuleToASTSymbolNode(mod, res.PackageName, lineage)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// Top-level Methods
	for _, m := range res.Methods {
		node, err := MethodToASTSymbolNode(m, res.PackageName, lineage)
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

// ClassToASTSymbolNode converts a RubyClass to an ASTSymbolNode.
func ClassToASTSymbolNode(cls RubyClass, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(cls)
	if err != nil {
		return nil, fmt.Errorf("marshaling Ruby class %s: %w", cls.Name, err)
	}

	var deps []string
	if cls.Superclass != "" {
		deps = append(deps, cls.Superclass)
	}
	deps = append(deps, cls.ModulesIncluded...)
	for _, a := range cls.Associations {
		deps = append(deps, a.Target)
	}

	meta := map[string]string{
		"class_name":        cls.Name,
		"superclass":        cls.Superclass,
		"method_count":      fmt.Sprintf("%d", len(cls.Methods)),
		"association_count": fmt.Sprintf("%d", len(cls.Associations)),
		"is_controller":     fmt.Sprintf("%t", cls.IsController),
		"is_model":          fmt.Sprintf("%t", cls.IsModel),
	}

	sig := fmt.Sprintf("class %s", cls.Name)
	if cls.Superclass != "" {
		sig = fmt.Sprintf("class %s < %s", cls.Name, cls.Superclass)
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangRuby,
		NodeType:          "ClassDef",
		Identifier:        fmt.Sprintf("%s::%s", pkgName, cls.Name),
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

// ModuleToASTSymbolNode converts a RubyModule to an ASTSymbolNode.
func ModuleToASTSymbolNode(mod RubyModule, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(mod)
	if err != nil {
		return nil, fmt.Errorf("marshaling Ruby module %s: %w", mod.Name, err)
	}

	meta := map[string]string{
		"module_name":  mod.Name,
		"method_count": fmt.Sprintf("%d", len(mod.Methods)),
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangRuby,
		NodeType:          "ModuleDef",
		Identifier:        fmt.Sprintf("%s::%s", pkgName, mod.Name),
		Signature:         fmt.Sprintf("module %s", mod.Name),
		Docstring:         mod.Doc,
		Visibility:        "public",
		ASTPayload:        payload,
		ASTMetadata:       meta,
		LocalDependencies: mod.ModulesIncluded,
		Dependencies:      mod.ModulesIncluded,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// MethodToASTSymbolNode converts a RubyMethod to an ASTSymbolNode.
func MethodToASTSymbolNode(m RubyMethod, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshaling Ruby method %s: %w", m.Name, err)
	}

	prefix := ""
	if m.IsClassMethod {
		prefix = "self."
	}
	sig := fmt.Sprintf("def %s%s", prefix, m.Name)

	vis := m.Visibility
	if vis == "" {
		vis = "public"
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangRuby,
		NodeType:          "MethodDef",
		Identifier:        fmt.Sprintf("%s#%s%s", pkgName, prefix, m.Name),
		Signature:         sig,
		Docstring:         m.Doc,
		Visibility:        vis,
		ASTPayload:        payload,
		ASTMetadata:       map[string]string{"is_class_method": fmt.Sprintf("%t", m.IsClassMethod)},
		LocalDependencies: m.CalledMethods,
		Dependencies:      m.CalledMethods,
		Lineage:           lineage,
	}

	nodeID, err := core.HashASTSymbolNode(node)
	if err != nil {
		return nil, err
	}
	node.NodeID = nodeID
	return node, nil
}

// RouteToASTSymbolNode converts a RubyRouteBinding to an ASTSymbolNode.
func RouteToASTSymbolNode(r RubyRouteBinding, pkgName string, lineage core.LineageEnvelope) (*core.ASTSymbolNode, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("marshaling Ruby route %s: %w", r.Path, err)
	}

	meta := map[string]string{
		"framework":   r.Framework,
		"handler":     r.HandlerName,
		"controller":  r.Controller,
		"action":      r.Action,
		"route_path":  r.Path,
		"http_method": r.Method,
	}

	var deps []string
	if r.Controller != "" {
		deps = append(deps, r.Controller)
	}

	node := &core.ASTSymbolNode{
		Language:          core.LangRuby,
		NodeType:          "RouteBinding",
		Identifier:        fmt.Sprintf("%s:%s %s", pkgName, r.Method, r.Path),
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

// BuildComponentNode bundles parsed Ruby symbols into a core.ComponentNode.
func (p *RubyParser) BuildComponentNode(
	res *RubyFileResult,
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
		"file_path":    res.FilePath,
		"package_name": res.PackageName,
		"symbol_count": fmt.Sprintf("%d", len(res.AllSymbols)),
		"class_count":  fmt.Sprintf("%d", len(res.Classes)),
		"module_count": fmt.Sprintf("%d", len(res.Modules)),
		"route_count":  fmt.Sprintf("%d", len(res.Routes)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangRuby,
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
