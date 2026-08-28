package python

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// PythonParser parses Python source code into AST symbol nodes.
type PythonParser struct {
	funcRegex    *regexp.Regexp
	classRegex   *regexp.Regexp
	routeRegex   *regexp.Regexp
	envRegex     *regexp.Regexp
	importRegex  *regexp.Regexp
}

// NewPythonParser constructs a new Python AST parser.
func NewPythonParser() *PythonParser {
	return &PythonParser{
		funcRegex:   regexp.MustCompile(`(?m)^ *(?:async +)?def +([a-zA-Z0-9_]+)\s*\((.*?)\)(?:\s*->\s*([^:]+))?:`),
		classRegex:  regexp.MustCompile(`(?m)^ *class +([a-zA-Z0-9_]+)(?:\((.*?)\))?:`),
		routeRegex:  regexp.MustCompile(`(?m)@(app|router)\.(get|post|put|delete|patch|options|head)\s*\(\s*["']([^"']+)["']`),
		envRegex:    regexp.MustCompile(`(?:os\.(?:environ\.get|getenv)\s*\(\s*|os\.environ\s*\[\s*)["']([a-zA-Z0-9_]+)["']`),
		importRegex: regexp.MustCompile(`(?m)^(?:from +([a-zA-Z0-9_.]+) +import +([a-zA-Z0-9_,\s*]+)|import +([a-zA-Z0-9_.]+))`),
	}
}

// PythonParseResult contains parsed symbols and metadata from a Python source file.
type PythonParseResult struct {
	Filename       string
	Functions      []*core.ASTSymbolNode
	Classes        []*core.ASTSymbolNode
	Routes         []*core.ASTSymbolNode
	Imports        []string
	EnvVars        []string
	AllSymbols     []*core.ASTSymbolNode
}

// ParseSource parses Python source bytes into symbol nodes with attached lineage.
func (p *PythonParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*PythonParseResult, error) {
	content := string(src)
	lines := strings.Split(content, "\n")
	res := &PythonParseResult{
		Filename: filename,
	}

	// 1. Extract Imports
	importMatches := p.importRegex.FindAllStringSubmatch(content, -1)
	for _, m := range importMatches {
		if m[1] != "" {
			res.Imports = append(res.Imports, fmt.Sprintf("%s.%s", m[1], strings.TrimSpace(m[2])))
		} else if m[3] != "" {
			res.Imports = append(res.Imports, strings.TrimSpace(m[3]))
		}
	}

	// 2. Extract Environment Variable Access
	envMatches := p.envRegex.FindAllStringSubmatch(content, -1)
	for _, m := range envMatches {
		if len(m) > 1 {
			res.EnvVars = append(res.EnvVars, m[1])
		}
	}

	// 3. Extract Routes with decorators
	routeMatches := p.routeRegex.FindAllStringSubmatchIndex(content, -1)
	for _, loc := range routeMatches {
		routeMethod := strings.ToUpper(content[loc[4]:loc[5]])
		routePath := content[loc[6]:loc[7]]
		identifier := fmt.Sprintf("%s:%s %s", filename, routeMethod, routePath)

		sym := &core.ASTSymbolNode{
			Language:   core.LangPython,
			NodeType:   "RouteBinding",
			Identifier: identifier,
			ASTPayload: []byte(content[loc[0]:loc[1]]),
			Lineage:    lineage,
		}
		symID, err := core.HashASTSymbolNode(sym)
		if err == nil {
			sym.NodeID = symID
			res.Routes = append(res.Routes, sym)
			res.AllSymbols = append(res.AllSymbols, sym)
		}
	}

	// 4. Extract Classes
	classMatches := p.classRegex.FindAllStringSubmatchIndex(content, -1)
	for _, loc := range classMatches {
		className := content[loc[2]:loc[3]]
		classBody := extractBlock(lines, loc[0], content)
		identifier := fmt.Sprintf("%s.%s", filename, className)

		sym := &core.ASTSymbolNode{
			Language:   core.LangPython,
			NodeType:   "ClassDecl",
			Identifier: identifier,
			ASTPayload: []byte(classBody),
			Lineage:    lineage,
		}
		symID, err := core.HashASTSymbolNode(sym)
		if err == nil {
			sym.NodeID = symID
			res.Classes = append(res.Classes, sym)
			res.AllSymbols = append(res.AllSymbols, sym)
		}
	}

	// 5. Extract Functions
	funcMatches := p.funcRegex.FindAllStringSubmatchIndex(content, -1)
	for _, loc := range funcMatches {
		funcName := content[loc[2]:loc[3]]
		funcBody := extractBlock(lines, loc[0], content)
		identifier := fmt.Sprintf("%s.%s", filename, funcName)

		sym := &core.ASTSymbolNode{
			Language:   core.LangPython,
			NodeType:   "FunctionDecl",
			Identifier: identifier,
			ASTPayload: []byte(funcBody),
			Lineage:    lineage,
		}
		symID, err := core.HashASTSymbolNode(sym)
		if err == nil {
			sym.NodeID = symID
			res.Functions = append(res.Functions, sym)
			res.AllSymbols = append(res.AllSymbols, sym)
		}
	}

	return res, nil
}

// BuildComponentNode packages parsed Python symbols into a ComponentNode.
func (p *PythonParser) BuildComponentNode(
	res *PythonParseResult,
	compName string,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, []*core.ASTSymbolNode, error) {
	var symIDs []string
	for _, s := range res.AllSymbols {
		symIDs = append(symIDs, s.NodeID)
	}

	meta := map[string]string{
		"filename": res.Filename,
		"language": string(core.LangPython),
	}
	if len(res.EnvVars) > 0 {
		meta["env_vars"] = strings.Join(res.EnvVars, ",")
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        core.CompService,
		Language:    core.LangPython,
		SymbolNodes: symIDs,
		Metadata:    meta,
		Lineage:     lineage,
	}

	compID, err := core.HashComponentNode(comp)
	if err != nil {
		return nil, nil, err
	}
	comp.ComponentID = compID

	return comp, res.AllSymbols, nil
}

func extractBlock(lines []string, charOffset int, fullText string) string {
	currentOffset := 0
	startLine := 0
	for i, l := range lines {
		if currentOffset+len(l) >= charOffset {
			startLine = i
			break
		}
		currentOffset += len(l) + 1 // newline
	}

	if startLine >= len(lines) {
		return ""
	}

	baseIndent := getIndent(lines[startLine])
	var blockLines []string
	blockLines = append(blockLines, lines[startLine])

	for i := startLine + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			blockLines = append(blockLines, line)
			continue
		}
		lineIndent := getIndent(line)
		if lineIndent <= baseIndent {
			break
		}
		blockLines = append(blockLines, line)
	}

	return strings.Join(blockLines, "\n")
}

func getIndent(line string) int {
	indent := 0
	for _, ch := range line {
		if ch == ' ' {
			indent++
		} else if ch == '\t' {
			indent += 4
		} else {
			break
		}
	}
	return indent
}
