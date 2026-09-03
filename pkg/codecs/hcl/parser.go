package hcl

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

// HCLAttribute represents a key-value attribute assignment in an HCL block.
type HCLAttribute struct {
	Key        string   `json:"key"`
	Value      string   `json:"value"`
	RawValue   string   `json:"raw_value"`
	References []string `json:"references,omitempty"`
	LineNumber int      `json:"line_number"`
}

// HCLBlock represents an HCL block (e.g. resource "type" "name", variable "name", etc.).
type HCLBlock struct {
	BlockType    string                  `json:"block_type"` // e.g. "resource", "variable", "output", "provider", "module", "data", "terraform"
	Labels       []string                `json:"labels"`     // e.g. ["google_cloud_run_service", "api"]
	Attributes   map[string]HCLAttribute `json:"attributes"`
	NestedBlocks []*HCLBlock             `json:"nested_blocks,omitempty"`
	LineStart    int                     `json:"line_start"`
	LineEnd      int                     `json:"line_end"`
	RawSource    string                  `json:"raw_source,omitempty"`
}

// FullIdentifier returns canonical identifier e.g. "resource.google_cloud_run_service.api".
func (b *HCLBlock) FullIdentifier() string {
	parts := []string{b.BlockType}
	parts = append(parts, b.Labels...)
	return strings.Join(parts, ".")
}

// EnvVarBindings extracts environment variable key-value pairs declared inside container blocks.
func (b *HCLBlock) EnvVarBindings() map[string]string {
	bindings := make(map[string]string)
	b.collectEnvVars(bindings)
	return bindings
}

func (b *HCLBlock) collectEnvVars(bindings map[string]string) {
	if b.BlockType == "env" {
		nameAttr, hasName := b.Attributes["name"]
		valueAttr, hasValue := b.Attributes["value"]
		if hasName && hasValue {
			cleanName := strings.Trim(nameAttr.Value, `"`)
			cleanVal := strings.Trim(valueAttr.Value, `"`)
			if cleanName != "" {
				bindings[cleanName] = cleanVal
			}
		}
	}
	for _, nested := range b.NestedBlocks {
		nested.collectEnvVars(bindings)
	}
}

// ContainerImages extracts container image references (e.g., in cloud_run or kubernetes blocks).
func (b *HCLBlock) ContainerImages() []string {
	var images []string
	if imgAttr, ok := b.Attributes["image"]; ok {
		cleanImg := strings.Trim(imgAttr.Value, `"`)
		if cleanImg != "" {
			images = append(images, cleanImg)
		}
	}
	for _, nested := range b.NestedBlocks {
		images = append(images, nested.ContainerImages()...)
	}
	return images
}

// HCLDocument represents a parsed Terraform HCL file.
type HCLDocument struct {
	FilePath string      `json:"file_path"`
	Blocks   []*HCLBlock `json:"blocks"`
}

// HCLParser provides pure-Go parsing of Terraform HCL files into AST nodes.
type HCLParser struct{}

// NewHCLParser creates an instance of HCLParser.
func NewHCLParser() *HCLParser {
	return &HCLParser{}
}

// ParseSource parses raw HCL source bytes into an HCLDocument.
func (p *HCLParser) ParseSource(filename string, src []byte) (*HCLDocument, error) {
	doc := &HCLDocument{
		FilePath: filename,
		Blocks:   make([]*HCLBlock, 0),
	}

	lines := splitLines(src)
	blocks, err := p.parseBlocks(lines, 0, len(lines))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HCL in %s: %w", filename, err)
	}
	doc.Blocks = blocks

	return doc, nil
}

// ParseFile parses an HCL file from disk.
func (p *HCLParser) ParseFile(filePath string) (*HCLDocument, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read HCL file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data)
}

// ParseDir parses all .tf files in a directory into a merged HCLDocument.
func (p *HCLParser) ParseDir(dirPath string) (*HCLDocument, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	mergedDoc := &HCLDocument{
		FilePath: dirPath,
		Blocks:   make([]*HCLBlock, 0),
	}

	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".tf") && !strings.HasSuffix(entry.Name(), ".tf.json")) {
			continue
		}
		fPath := filepath.Join(dirPath, entry.Name())
		doc, err := p.ParseFile(fPath)
		if err != nil {
			return nil, err
		}
		mergedDoc.Blocks = append(mergedDoc.Blocks, doc.Blocks...)
	}

	return mergedDoc, nil
}

type lineItem struct {
	lineNum int
	text    string
}

func splitLines(src []byte) []lineItem {
	var items []lineItem
	scanner := bufio.NewScanner(bytes.NewReader(src))
	lineNum := 1
	for scanner.Scan() {
		items = append(items, lineItem{
			lineNum: lineNum,
			text:    scanner.Text(),
		})
		lineNum++
	}
	return items
}

var blockHeaderRegex = regexp.MustCompile(`^([a-zA-Z0-9_-]+)\s*("[^"]*"\s*|'[^']*'\s*|[a-zA-Z0-9_-]+\s*)*\{`)
var attrRegex = regexp.MustCompile(`^([a-zA-Z0-9_-]+)\s*=\s*(.*)$`)
var refRegex = regexp.MustCompile(`\b([a-zA-Z0-9_]+(?:\.[a-zA-Z0-9_]+)+)\b`)

func (p *HCLParser) parseBlocks(lines []lineItem, startIdx, endIdx int) ([]*HCLBlock, error) {
	var blocks []*HCLBlock

	i := startIdx
	for i < endIdx {
		line := strings.TrimSpace(lines[i].text)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			i++
			continue
		}

		// Check if line begins a block
		if blockHeaderRegex.MatchString(line) {
			block, nextIdx, err := p.parseSingleBlock(lines, i, endIdx)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, block)
			i = nextIdx
			continue
		}

		i++
	}

	return blocks, nil
}

func (p *HCLParser) parseSingleBlock(lines []lineItem, startIdx, endIdx int) (*HCLBlock, int, error) {
	firstLine := strings.TrimSpace(lines[startIdx].text)
	blockType, labels := p.extractBlockHeader(firstLine)

	block := &HCLBlock{
		BlockType:  blockType,
		Labels:     labels,
		Attributes: make(map[string]HCLAttribute),
		LineStart:  lines[startIdx].lineNum,
	}

	braceCount := 0
	var blockLines []string
	nestedStart := -1
	i := startIdx

	for i < endIdx {
		curLine := lines[i].text
		blockLines = append(blockLines, curLine)

		for _, ch := range curLine {
			if ch == '{' {
				braceCount++
			} else if ch == '}' {
				braceCount--
			}
		}

		if i > startIdx {
			trimmed := strings.TrimSpace(curLine)

			// Sub-block detection
			if blockHeaderRegex.MatchString(trimmed) && nestedStart == -1 {
				subBlock, nextI, err := p.parseSingleBlock(lines, i, endIdx)
				if err != nil {
					return nil, i, err
				}
				block.NestedBlocks = append(block.NestedBlocks, subBlock)
				i = nextI - 1 // loop increments
			} else if attrMatch := attrRegex.FindStringSubmatch(trimmed); attrMatch != nil && nestedStart == -1 {
				key := attrMatch[1]
				val := strings.TrimSpace(attrMatch[2])

				// Extract variable/resource references
				var refs []string
				for _, match := range refRegex.FindAllString(val, -1) {
					if !strings.HasPrefix(match, `"`) {
						refs = append(refs, match)
					}
				}

				block.Attributes[key] = HCLAttribute{
					Key:        key,
					Value:      val,
					RawValue:   val,
					References: refs,
					LineNumber: lines[i].lineNum,
				}
			}
		}

		if braceCount == 0 {
			block.LineEnd = lines[i].lineNum
			block.RawSource = strings.Join(blockLines, "\n")
			return block, i + 1, nil
		}

		i++
	}

	block.LineEnd = lines[len(lines)-1].lineNum
	block.RawSource = strings.Join(blockLines, "\n")
	return block, endIdx, nil
}

func (p *HCLParser) extractBlockHeader(line string) (string, []string) {
	idx := strings.Index(line, "{")
	if idx != -1 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)

	var tokens []string
	inQuotes := false
	var current strings.Builder

	for _, ch := range line {
		if ch == '"' || ch == '\'' {
			inQuotes = !inQuotes
			continue
		}
		if (ch == ' ' || ch == '\t') && !inQuotes {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(ch)
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	if len(tokens) == 0 {
		return "unknown", nil
	}

	blockType := tokens[0]
	labels := tokens[1:]
	return blockType, labels
}

// ToASTSymbolNodes converts all HCL blocks into core.ASTSymbolNodes.
func (p *HCLParser) ToASTSymbolNodes(doc *HCLDocument, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	for _, block := range doc.Blocks {
		payload, err := json.Marshal(block)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal HCL block %s: %w", block.FullIdentifier(), err)
		}

		var nodeType string
		switch block.BlockType {
		case "resource":
			nodeType = "ResourceBlock"
		case "variable":
			nodeType = "VariableBlock"
		case "output":
			nodeType = "OutputBlock"
		case "provider":
			nodeType = "ProviderBlock"
		case "module":
			nodeType = "ModuleBlock"
		case "data":
			nodeType = "DataBlock"
		default:
			nodeType = "HCLBlock"
		}

		// Extract local dependencies (references to other variables, resources)
		var localDeps []string
		for _, attr := range block.Attributes {
			localDeps = append(localDeps, attr.References...)
		}
		sort.Strings(localDeps)

		node := &core.ASTSymbolNode{
			Language:          core.LangHCL,
			NodeType:          nodeType,
			Identifier:        block.FullIdentifier(),
			ASTPayload:        payload,
			LocalDependencies: localDeps,
			Lineage:           lineage,
		}

		nodeID, err := core.HashASTSymbolNode(node)
		if err != nil {
			return nil, fmt.Errorf("failed to compute hash for HCL node %s: %w", node.Identifier, err)
		}
		node.NodeID = nodeID

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// BuildComponentNode creates an infrastructure ComponentNode from an HCLDocument.
func (p *HCLParser) BuildComponentNode(doc *HCLDocument, compName string, lineage core.LineageEnvelope) (*core.ComponentNode, []*core.ASTSymbolNode, error) {
	if compName == "" {
		compName = "infra-root"
	}

	nodes, err := p.ToASTSymbolNodes(doc, lineage)
	if err != nil {
		return nil, nil, err
	}

	symbolIDs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		symbolIDs = append(symbolIDs, n.NodeID)
	}

	metadata := map[string]string{
		"file_path":    doc.FilePath,
		"block_count":  fmt.Sprintf("%d", len(doc.Blocks)),
		"symbol_count": fmt.Sprintf("%d", len(nodes)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        core.CompInfra,
		Language:    core.LangHCL,
		SymbolNodes: symbolIDs,
		Metadata:    metadata,
		Lineage:     lineage,
	}

	compID, err := core.HashComponentNode(comp)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute hash for HCL component node: %w", err)
	}
	comp.ComponentID = compID

	return comp, nodes, nil
}
