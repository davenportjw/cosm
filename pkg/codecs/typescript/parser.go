package typescript

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

// TSAPICall represents an HTTP fetch or API client request discovered in TypeScript/TSX.
type TSAPICall struct {
	Method     string `json:"method"` // e.g. "GET", "POST", "PUT", "DELETE"
	URL        string `json:"url"`    // e.g. "/api/v1/users", "/api/auth/login"
	Caller     string `json:"caller"` // e.g. "fetch", "axios.get", "apiClient.post"
	LineNumber int    `json:"line_number"`
}

// TSComponent represents a React UI Component (functional, arrow, or class).
type TSComponent struct {
	Name          string      `json:"name"`
	PropsType     string      `json:"props_type,omitempty"`
	HooksUsed     []string    `json:"hooks_used,omitempty"`
	SubComponents []string    `json:"sub_components,omitempty"`
	APICalls      []TSAPICall `json:"api_calls,omitempty"`
	IsExported    bool        `json:"is_exported"`
	LineStart     int         `json:"line_start"`
	LineEnd       int         `json:"line_end"`
	BodySource    string      `json:"body_source,omitempty"`
}

// TSHook represents a React hook definition (e.g. useAuth, useUsers).
type TSHook struct {
	Name       string   `json:"name"`
	Params     []string `json:"params,omitempty"`
	HooksUsed  []string `json:"hooks_used,omitempty"`
	IsExported bool     `json:"is_exported"`
	LineNumber int      `json:"line_number"`
}

// TSInterface represents a TypeScript interface declaration.
type TSInterface struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
	Extends    []string          `json:"extends,omitempty"`
	IsExported bool              `json:"is_exported"`
	LineNumber int               `json:"line_number"`
}

// TSTypeAlias represents a TypeScript type alias.
type TSTypeAlias struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	IsExported bool   `json:"is_exported"`
	LineNumber int    `json:"line_number"`
}

// TSFunction represents an exported or top-level TypeScript function.
type TSFunction struct {
	Name       string   `json:"name"`
	Params     []string `json:"params,omitempty"`
	ReturnType string   `json:"return_type,omitempty"`
	IsAsync    bool     `json:"is_async"`
	IsExported bool     `json:"is_exported"`
	LineNumber int      `json:"line_number"`
}

// TSImport represents an import statement.
type TSImport struct {
	Specifiers []string `json:"specifiers"`
	Source     string   `json:"source"`
	IsDefault  bool     `json:"is_default"`
	LineNumber int      `json:"line_number"`
}

// TSFileResult contains all extracted symbols from a TypeScript / TSX file.
type TSFileResult struct {
	FilePath   string                `json:"file_path"`
	Components []TSComponent         `json:"components"`
	Hooks      []TSHook              `json:"hooks"`
	Interfaces []TSInterface         `json:"interfaces"`
	Types      []TSTypeAlias         `json:"types"`
	Functions  []TSFunction          `json:"functions"`
	APICalls   []TSAPICall           `json:"api_calls"`
	Imports    []TSImport            `json:"imports"`
	Exports    []string              `json:"exports"`
	AllSymbols []*core.ASTSymbolNode `json:"all_symbols"`
	Trivia     *core.TriviaEnvelope  `json:"trivia,omitempty"`
}

// TSParser extracts symbols, components, hooks, interfaces, and API calls from TypeScript/TSX.
type TSParser struct{}

// NewTSParser creates an initialized TSParser instance.
func NewTSParser() *TSParser {
	return &TSParser{}
}

// ParseSource parses raw TypeScript / TSX source bytes.
func (p *TSParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*TSFileResult, error) {
	if filename == "" {
		filename = "Component.tsx"
	}

	lines := splitTSLines(src)
	result := &TSFileResult{
		FilePath: filename,
		Trivia:   extractTSTrivia(lines),
	}

	// 1. Extract Imports
	result.Imports = p.extractImports(lines)

	// 2. Extract Interfaces and Type Aliases
	result.Interfaces = p.extractInterfaces(lines)
	result.Types = p.extractTypes(lines)

	// 3. Extract API Calls across entire file
	result.APICalls = p.extractAPICalls(lines)

	// 4. Extract Hooks
	result.Hooks = p.extractHooks(lines)

	// 5. Extract Components
	result.Components = p.extractComponents(lines, result.APICalls)

	// 6. Extract Functions
	result.Functions = p.extractFunctions(lines)

	// Convert extracted symbols to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, err
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a TypeScript/TSX file from disk.
func (p *TSParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*TSFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read TypeScript file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .ts and .tsx files in a directory.
func (p *TSParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*TSFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	var results []*TSFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" {
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

type tsLine struct {
	lineNum int
	text    string
}

func splitTSLines(src []byte) []tsLine {
	var items []tsLine
	scanner := bufio.NewScanner(bytes.NewReader(src))
	lineNum := 1
	for scanner.Scan() {
		items = append(items, tsLine{
			lineNum: lineNum,
			text:    scanner.Text(),
		})
		lineNum++
	}
	return items
}

var importRegex = regexp.MustCompile(`import\s+(?:\{([^}]+)\}|\*\s+as\s+(\w+)|(\w+))\s+from\s+['"]([^'"]+)['"]`)
var interfaceHeaderRegex = regexp.MustCompile(`(?:export\s+)?interface\s+([A-Za-z0-9_]+)(?:\s+extends\s+([A-Za-z0-9_,\s]+))?\s*\{`)
var typeAliasRegex = regexp.MustCompile(`(?:export\s+)?type\s+([A-Za-z0-9_]+)\s*=\s*(.+);?`)
var hookDeclRegex = regexp.MustCompile(`(?:export\s+)?(?:function|const)\s+(use[A-Z][A-Za-z0-9_]*)\s*(?:=\s*(?:\([^)]*\)|async\s*\([^)]*\))\s*=>|\([^)]*\))`)
var hookUsageRegex = regexp.MustCompile(`\b(use[A-Z][A-Za-z0-9_]*)\b`)
var reactCompRegex = regexp.MustCompile(`(?:export\s+(?:default\s+)?)?(?:function|const)\s+([A-Z][A-Za-z0-9_]*)\s*(?::\s*React\.FC(?:<[^>]+>)?\s*=\s*|\s*=\s*(?:\([^)]*\)|async\s*\([^)]*\))\s*=>|\([^)]*\))`)
var fetchCallRegex = regexp.MustCompile(`fetch\s*\(\s*['"\x60]([^'"\x60]+)['"\x60](?:\s*,\s*\{[^}]*method:\s*['"]([A-Z]+)['"])?`)
var axiosCallRegex = regexp.MustCompile(`(?:axios|apiClient|client|api)\.(get|post|put|delete|patch)\s*\(\s*['"\x60]([^'"\x60]+)['"\x60]`)
var jsxElementRegex = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_])<([A-Z][A-Za-z0-9_]*)(?:\s+[^>]*|\s*\/?>)`)
var genericTypeRegex = regexp.MustCompile(`[a-zA-Z0-9_]+<[A-Z][A-Za-z0-9_]*`)

func (p *TSParser) extractImports(lines []tsLine) []TSImport {
	var imports []TSImport

	combinedImportRegex := regexp.MustCompile(`import\s+(?:type\s+)?(?:(\w+)\s*,\s*)?(?:\{([^}]+)\}|\*\s+as\s+(\w+)|(\w+))?\s*from\s*['"]([^'"]+)['"]`)
	sideEffectImportRegex := regexp.MustCompile(`import\s+['"]([^'"]+)['"]`)

	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		if match := combinedImportRegex.FindStringSubmatch(trimmed); match != nil {
			var specifiers []string
			// Default specifier before comma (e.g. React in `import React, { useState } from 'react'`)
			if match[1] != "" {
				specifiers = append(specifiers, match[1])
			}
			// Named specifiers { a, b }
			if match[2] != "" {
				for _, s := range strings.Split(match[2], ",") {
					spec := strings.TrimSpace(s)
					if spec != "" {
						specifiers = append(specifiers, spec)
					}
				}
			}
			// Star namespace import (* as Foo)
			if match[3] != "" {
				specifiers = append(specifiers, match[3])
			}
			// Standalone default import (import Foo from 'foo')
			if match[4] != "" && match[1] == "" {
				specifiers = append(specifiers, match[4])
			}

			source := match[5]
			imports = append(imports, TSImport{
				Specifiers: specifiers,
				Source:     source,
				IsDefault:  match[1] != "" || match[4] != "",
				LineNumber: l.lineNum,
			})
		} else if match := sideEffectImportRegex.FindStringSubmatch(trimmed); match != nil {
			imports = append(imports, TSImport{
				Specifiers: nil,
				Source:     match[1],
				IsDefault:  false,
				LineNumber: l.lineNum,
			})
		}
	}
	return imports
}

func (p *TSParser) extractInterfaces(lines []tsLine) []TSInterface {
	var interfaces []TSInterface
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)
		if match := interfaceHeaderRegex.FindStringSubmatch(trimmed); match != nil {
			name := match[1]
			var extends []string
			if match[2] != "" {
				for _, ext := range strings.Split(match[2], ",") {
					ext = strings.TrimSpace(ext)
					if ext != "" {
						extends = append(extends, ext)
					}
				}
			}
			isExported := strings.HasPrefix(trimmed, "export")

			props := make(map[string]string)
			i++
			for i < len(lines) {
				propLine := strings.TrimSpace(lines[i].text)
				if strings.HasPrefix(propLine, "}") {
					break
				}
				if colIdx := strings.Index(propLine, ":"); colIdx != -1 {
					key := strings.TrimSpace(propLine[:colIdx])
					key = strings.TrimSuffix(key, "?")
					val := strings.TrimSpace(propLine[colIdx+1:])
					val = strings.TrimSuffix(val, ";")
					val = strings.TrimSuffix(val, ",")
					if key != "" && val != "" {
						props[key] = val
					}
				}
				i++
			}

			interfaces = append(interfaces, TSInterface{
				Name:       name,
				Properties: props,
				Extends:    extends,
				IsExported: isExported,
				LineNumber: lines[i-1].lineNum,
			})
		}
		i++
	}
	return interfaces
}

func (p *TSParser) extractTypes(lines []tsLine) []TSTypeAlias {
	var types []TSTypeAlias
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if match := typeAliasRegex.FindStringSubmatch(trimmed); match != nil {
			types = append(types, TSTypeAlias{
				Name:       match[1],
				Definition: strings.TrimSpace(match[2]),
				IsExported: strings.HasPrefix(trimmed, "export"),
				LineNumber: l.lineNum,
			})
		}
	}
	return types
}

func (p *TSParser) extractAPICalls(lines []tsLine) []TSAPICall {
	var calls []TSAPICall
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Check fetch(...) calls
		if match := fetchCallRegex.FindStringSubmatch(trimmed); match != nil {
			url := match[1]
			method := "GET"
			if match[2] != "" {
				method = match[2]
			}
			calls = append(calls, TSAPICall{
				Method:     method,
				URL:        url,
				Caller:     "fetch",
				LineNumber: l.lineNum,
			})
		}

		// Check axios / apiClient calls: axios.get("/api/..."), apiClient.post(...)
		if match := axiosCallRegex.FindStringSubmatch(trimmed); match != nil {
			method := strings.ToUpper(match[1])
			url := match[2]
			caller := "axios." + match[1]
			calls = append(calls, TSAPICall{
				Method:     method,
				URL:        url,
				Caller:     caller,
				LineNumber: l.lineNum,
			})
		}
	}
	return calls
}

func (p *TSParser) extractHooks(lines []tsLine) []TSHook {
	var hooks []TSHook
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if match := hookDeclRegex.FindStringSubmatch(trimmed); match != nil {
			hooks = append(hooks, TSHook{
				Name:       match[1],
				IsExported: strings.HasPrefix(trimmed, "export"),
				LineNumber: l.lineNum,
			})
		}
	}
	return hooks
}

func (p *TSParser) extractComponents(lines []tsLine, apiCalls []TSAPICall) []TSComponent {
	var components []TSComponent

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i].text)
		if match := reactCompRegex.FindStringSubmatch(trimmed); match != nil {
			name := match[1]
			if strings.HasPrefix(name, "use") {
				i++
				continue // Skip hooks misclassified as components
			}

			startLine := lines[i].lineNum
			isExported := strings.HasPrefix(trimmed, "export")

			// Collect component body until matching closing brace
			braceCount := 0
			var bodyLines []string
			var hooksUsed []string
			var subComps []string
			seenHooks := make(map[string]bool)
			seenSubComps := make(map[string]bool)

			for i < len(lines) {
				lineText := lines[i].text
				bodyLines = append(bodyLines, lineText)

				for _, ch := range lineText {
					if ch == '{' {
						braceCount++
					} else if ch == '}' {
						braceCount--
					}
				}

				// Find hooks used inside component
				for _, hMatch := range hookUsageRegex.FindAllString(lineText, -1) {
					if !seenHooks[hMatch] && hMatch != name {
						seenHooks[hMatch] = true
						hooksUsed = append(hooksUsed, hMatch)
					}
				}

				// Find rendered JSX child components
				for _, subMatch := range jsxElementRegex.FindAllStringSubmatch(lineText, -1) {
					subName := subMatch[1]
					if subName != name && !seenSubComps[subName] {
						seenSubComps[subName] = true
						subComps = append(subComps, subName)
					}
				}

				if braceCount == 0 && len(bodyLines) > 0 && strings.Contains(lineText, "}") {
					break
				}
				i++
			}

			endLine := startLine
			if i < len(lines) {
				endLine = lines[i].lineNum
			}

			// Filter API calls made within this component's line boundaries
			var compAPICalls []TSAPICall
			for _, call := range apiCalls {
				if call.LineNumber >= startLine && call.LineNumber <= endLine {
					compAPICalls = append(compAPICalls, call)
				}
			}

			components = append(components, TSComponent{
				Name:          name,
				HooksUsed:     hooksUsed,
				SubComponents: subComps,
				APICalls:      compAPICalls,
				IsExported:    isExported,
				LineStart:     startLine,
				LineEnd:       endLine,
				BodySource:    strings.Join(bodyLines, "\n"),
			})
		}
		i++
	}

	return components
}

var funcDeclRegex = regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+([a-z][A-Za-z0-9_]*)\s*\(`)

func (p *TSParser) extractFunctions(lines []tsLine) []TSFunction {
	var functions []TSFunction
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if match := funcDeclRegex.FindStringSubmatch(trimmed); match != nil {
			functions = append(functions, TSFunction{
				Name:       match[1],
				IsAsync:    strings.Contains(trimmed, "async"),
				IsExported: strings.HasPrefix(trimmed, "export"),
				LineNumber: l.lineNum,
			})
		}
	}
	return functions
}

func (p *TSParser) toSymbolNodes(res *TSFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Components
	for _, c := range res.Components {
		payload, err := json.Marshal(c)
		if err != nil {
			return nil, err
		}
		var localDeps []string
		localDeps = append(localDeps, c.HooksUsed...)
		localDeps = append(localDeps, c.SubComponents...)
		sort.Strings(localDeps)

		node := &core.ASTSymbolNode{
			Language:          core.LangTypeScript,
			NodeType:          "ReactComponent",
			Identifier:        fmt.Sprintf("%s:%s", res.FilePath, c.Name),
			ASTPayload:        payload,
			LocalDependencies: localDeps,
			Lineage:           lineage,
		}
		nodeID, err := core.HashASTSymbolNode(node)
		if err != nil {
			return nil, err
		}
		node.NodeID = nodeID
		nodes = append(nodes, node)
	}

	// Interfaces
	for _, iface := range res.Interfaces {
		payload, err := json.Marshal(iface)
		if err != nil {
			return nil, err
		}
		node := &core.ASTSymbolNode{
			Language:          core.LangTypeScript,
			NodeType:          "InterfaceDecl",
			Identifier:        fmt.Sprintf("%s:%s", res.FilePath, iface.Name),
			ASTPayload:        payload,
			LocalDependencies: iface.Extends,
			Lineage:           lineage,
		}
		nodeID, err := core.HashASTSymbolNode(node)
		if err != nil {
			return nil, err
		}
		node.NodeID = nodeID
		nodes = append(nodes, node)
	}

	// API Client Calls
	for _, api := range res.APICalls {
		payload, err := json.Marshal(api)
		if err != nil {
			return nil, err
		}
		node := &core.ASTSymbolNode{
			Language:          core.LangTypeScript,
			NodeType:          "ApiClientCall",
			Identifier:        fmt.Sprintf("%s:%s %s", res.FilePath, api.Method, api.URL),
			ASTPayload:        payload,
			LocalDependencies: nil,
			Lineage:           lineage,
		}
		nodeID, err := core.HashASTSymbolNode(node)
		if err != nil {
			return nil, err
		}
		node.NodeID = nodeID
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// BuildComponentNode bundles frontend symbols into a core.ComponentNode.
func (p *TSParser) BuildComponentNode(res *TSFileResult, compName string, lineage core.LineageEnvelope) (*core.ComponentNode, []*core.ASTSymbolNode, error) {
	if compName == "" {
		compName = "frontend-ui"
	}

	symbolIDs := make([]string, 0, len(res.AllSymbols))
	for _, s := range res.AllSymbols {
		symbolIDs = append(symbolIDs, s.NodeID)
	}

	metadata := map[string]string{
		"file_path":       res.FilePath,
		"component_count": fmt.Sprintf("%d", len(res.Components)),
		"api_call_count":  fmt.Sprintf("%d", len(res.APICalls)),
		"symbol_count":    fmt.Sprintf("%d", len(res.AllSymbols)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        core.CompFrontend,
		Language:    core.LangTypeScript,
		SymbolNodes: symbolIDs,
		Trivia:      res.Trivia,
		Metadata:    metadata,
		Lineage:     lineage,
	}

	compID, err := core.HashComponentNode(comp)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute hash for TS component: %w", err)
	}
	comp.ComponentID = compID

	return comp, res.AllSymbols, nil
}

func extractTSTrivia(lines []tsLine) *core.TriviaEnvelope {
	var directives []string
	var licenseLines []string
	inBlock := false
	var blockLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line.text)
		if trimmed == "" {
			continue
		}
		if inBlock {
			blockLines = append(blockLines, line.text)
			if strings.Contains(trimmed, "*/") {
				inBlock = false
				joined := strings.Join(blockLines, "\n")
				if isTSLicenseText(joined) {
					licenseLines = append(licenseLines, joined)
				}
				blockLines = nil
			}
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			if strings.Contains(trimmed, "*/") {
				if isTSLicenseText(trimmed) {
					licenseLines = append(licenseLines, trimmed)
				}
			} else {
				inBlock = true
				blockLines = append(blockLines, line.text)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "// @ts-") ||
			strings.HasPrefix(trimmed, "/// <reference") ||
			trimmed == `"use client";` || trimmed == `'use client';` || trimmed == `"use client"` || trimmed == `'use client'` ||
			trimmed == `"use server";` || trimmed == `'use server';` || trimmed == `"use server"` || trimmed == `'use server'` {
			directives = append(directives, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			if isTSLicenseText(trimmed) {
				licenseLines = append(licenseLines, trimmed)
			}
			continue
		}
		break
	}

	var lic string
	if len(licenseLines) > 0 {
		lic = strings.TrimSpace(strings.Join(licenseLines, "\n"))
	}
	if len(directives) == 0 && lic == "" {
		return nil
	}
	return &core.TriviaEnvelope{
		HeaderDirectives: directives,
		LicenseHeader:    lic,
	}
}

func isTSLicenseText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "copyright") ||
		strings.Contains(lower, "license") ||
		strings.Contains(lower, "licensed") ||
		strings.Contains(lower, "apache") ||
		strings.Contains(lower, "mit license") ||
		strings.Contains(lower, "spdx-license-identifier") ||
		strings.Contains(lower, "all rights reserved")
}
