package graphql

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// GraphQLDirectiveApplication represents a directive applied to a type, field, or enum value.
type GraphQLDirectiveApplication struct {
	Name      string            `json:"name"`                // e.g. "deprecated", "key", "auth"
	Arguments map[string]string `json:"arguments,omitempty"` // e.g. {"reason": "Use newField", "fields": "id"}
	Raw       string            `json:"raw,omitempty"`       // full string e.g. "@deprecated(reason: \"...\")"
}

// GraphQLArgument represents an argument in a field or directive definition.
type GraphQLArgument struct {
	Name         string                        `json:"name"`
	Type         string                        `json:"type"` // e.g. "String!", "[Int!]"
	DefaultValue string                        `json:"default_value,omitempty"`
	Directives   []GraphQLDirectiveApplication `json:"directives,omitempty"`
	Description  string                        `json:"description,omitempty"`
}

// GraphQLField represents a field in an Object type, Interface, or Input type.
type GraphQLField struct {
	Name         string                        `json:"name"`
	Type         string                        `json:"type"` // e.g. "String!", "[User!]!"
	DefaultValue string                        `json:"default_value,omitempty"`
	Arguments    []GraphQLArgument             `json:"arguments,omitempty"`
	Directives   []GraphQLDirectiveApplication `json:"directives,omitempty"`
	Description  string                        `json:"description,omitempty"`
}

// GraphQLEnumValue represents a single value in an enum.
type GraphQLEnumValue struct {
	Name        string                        `json:"name"`
	Description string                        `json:"description,omitempty"`
	Directives  []GraphQLDirectiveApplication `json:"directives,omitempty"`
}

// GraphQLType represents a GraphQL type definition (type, interface, union, enum, input, scalar, directive).
type GraphQLType struct {
	Kind        string                        `json:"kind"` // type, interface, union, enum, input, scalar, directive
	Name        string                        `json:"name"`
	Implements  []string                      `json:"implements,omitempty"`  // for type: ["Node", "Entity"]
	UnionTypes  []string                      `json:"union_types,omitempty"` // for union: ["User", "Post"]
	Fields      []GraphQLField                `json:"fields,omitempty"`
	EnumValues  []GraphQLEnumValue            `json:"enum_values,omitempty"`
	Directives  []GraphQLDirectiveApplication `json:"directives,omitempty"`
	Description string                        `json:"description,omitempty"`
	Locations   []string                      `json:"locations,omitempty"` // for directive: ["FIELD_DEFINITION", "OBJECT"]
	Arguments   []GraphQLArgument             `json:"arguments,omitempty"` // for directive
	IsExtension bool                          `json:"is_extension,omitempty"`
	IsFederated bool                          `json:"is_federated,omitempty"` // @key, @extends
	KeyFields   string                        `json:"key_fields,omitempty"`
}

// GraphQLOperation represents a field on Query, Mutation, or Subscription.
type GraphQLOperation struct {
	OperationType string       `json:"operation_type"` // query, mutation, subscription
	Name          string       `json:"name"`           // e.g. "getUserById", "createUser"
	Field         GraphQLField `json:"field"`
	Description   string       `json:"description,omitempty"`
}

// GraphQLSchemaDef represents the root `schema { query: ... mutation: ... }` block.
type GraphQLSchemaDef struct {
	Query        string                        `json:"query,omitempty"`
	Mutation     string                        `json:"mutation,omitempty"`
	Subscription string                        `json:"subscription,omitempty"`
	Directives   []GraphQLDirectiveApplication `json:"directives,omitempty"`
	Description  string                        `json:"description,omitempty"`
}

// GraphQLFileResult contains all extracted GraphQL SDL symbols and schema definitions.
type GraphQLFileResult struct {
	FilePath   string                `json:"file_path"`
	Schema     *GraphQLSchemaDef     `json:"schema,omitempty"`
	Types      []GraphQLType         `json:"types"`
	Operations []GraphQLOperation    `json:"operations"`
	AllSymbols []*core.ASTSymbolNode `json:"all_symbols"`
}

// GraphQLParser parses GraphQL SDL schemas into structured AST symbol nodes.
type GraphQLParser struct{}

// NewGraphQLParser creates a new GraphQLParser instance.
func NewGraphQLParser() *GraphQLParser {
	return &GraphQLParser{}
}

// ParseSource parses raw GraphQL SDL bytes into structured AST symbols.
func (p *GraphQLParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*GraphQLFileResult, error) {
	if filename == "" {
		filename = "schema.graphql"
	}

	result := &GraphQLFileResult{
		FilePath: filename,
	}

	tokens := tokenizeGraphQL(src)
	res, err := p.parseTokens(tokens, result)
	if err != nil {
		return nil, fmt.Errorf("parsing graphql tokens in %s: %w", filename, err)
	}

	// Convert parsed elements into ASTSymbolNodes
	symbols, err := p.toSymbolNodes(res, lineage)
	if err != nil {
		return nil, fmt.Errorf("generating ASTSymbolNodes: %w", err)
	}
	res.AllSymbols = symbols

	return res, nil
}

// ParseFile parses a .graphql or .gql schema file from disk.
func (p *GraphQLParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*GraphQLFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .graphql and .gql files in a directory.
func (p *GraphQLParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*GraphQLFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*GraphQLFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext == ".graphql" || ext == ".gql" {
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

// Tokenizer & Lexer
type gqlToken struct {
	kind string // "NAME", "STRING", "PUNCT", "DIRECTIVE", "COMMENT"
	val  string
	line int
}

func tokenizeGraphQL(src []byte) []gqlToken {
	var tokens []gqlToken
	scanner := bufio.NewScanner(bytes.NewReader(src))
	lineNum := 1

	inBlockString := false
	var blockStringBuf strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if inBlockString {
			if idx := strings.Index(line, `"""`); idx != -1 {
				blockStringBuf.WriteString(line[:idx])
				tokens = append(tokens, gqlToken{kind: "STRING", val: strings.TrimSpace(blockStringBuf.String()), line: lineNum})
				blockStringBuf.Reset()
				inBlockString = false
				line = line[idx+3:]
				trimmed = strings.TrimSpace(line)
				if trimmed == "" {
					lineNum++
					continue
				}
			} else {
				blockStringBuf.WriteString(line + "\n")
				lineNum++
				continue
			}
		}

		// Check for block string start
		if strings.HasPrefix(trimmed, `"""`) {
			rest := strings.TrimPrefix(trimmed, `"""`)
			if idx := strings.Index(rest, `"""`); idx != -1 {
				// Single line block string: """doc"""
				tokens = append(tokens, gqlToken{kind: "STRING", val: strings.TrimSpace(rest[:idx]), line: lineNum})
				line = rest[idx+3:]
			} else {
				inBlockString = true
				blockStringBuf.WriteString(rest + "\n")
				lineNum++
				continue
			}
		}

		// Skip comments or extract single line doc
		if strings.HasPrefix(trimmed, "#") {
			lineNum++
			continue
		}

		// Lex line
		i := 0
		for i < len(line) {
			ch := line[i]
			if ch == ' ' || ch == '\t' || ch == '\r' || ch == ',' {
				i++
				continue
			}
			if ch == '#' {
				break
			}

			// Single line string: "..."
			if ch == '"' {
				end := i + 1
				for end < len(line) && line[end] != '"' {
					if line[end] == '\\' && end+1 < len(line) {
						end += 2
					} else {
						end++
					}
				}
				strVal := ""
				if end < len(line) {
					strVal = line[i+1 : end]
					i = end + 1
				} else {
					strVal = line[i+1:]
					i = len(line)
				}
				tokens = append(tokens, gqlToken{kind: "STRING", val: strVal, line: lineNum})
				continue
			}

			// Directive: @name
			if ch == '@' {
				end := i + 1
				for end < len(line) && (isIdentChar(line[end]) || line[end] == '.') {
					end++
				}
				tokens = append(tokens, gqlToken{kind: "DIRECTIVE", val: line[i:end], line: lineNum})
				i = end
				continue
			}

			// Punctuation
			if ch == '{' || ch == '}' || ch == '(' || ch == ')' || ch == '[' || ch == ']' || ch == ':' || ch == '=' || ch == '!' || ch == '|' || ch == '&' {
				tokens = append(tokens, gqlToken{kind: "PUNCT", val: string(ch), line: lineNum})
				i++
				continue
			}

			// Number literal: 123, -45.67, 10
			if (ch >= '0' && ch <= '9') || (ch == '-' && i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9') {
				end := i + 1
				for end < len(line) && ((line[end] >= '0' && line[end] <= '9') || line[end] == '.' || line[end] == 'e' || line[end] == 'E' || line[end] == '-' || line[end] == '+') {
					end++
				}
				tokens = append(tokens, gqlToken{kind: "NAME", val: line[i:end], line: lineNum})
				i = end
				continue
			}

			// Name / Ident
			if isIdentStart(ch) {
				end := i + 1
				for end < len(line) && isIdentChar(line[end]) {
					end++
				}
				tokens = append(tokens, gqlToken{kind: "NAME", val: line[i:end], line: lineNum})
				i = end
				continue
			}

			i++
		}

		lineNum++
	}

	return tokens
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-'
}

// Token parser
func (p *GraphQLParser) parseTokens(tokens []gqlToken, res *GraphQLFileResult) (*GraphQLFileResult, error) {
	idx := 0
	var pendingDoc string

	for idx < len(tokens) {
		tok := tokens[idx]

		if tok.kind == "STRING" {
			pendingDoc = tok.val
			idx++
			continue
		}

		if tok.kind != "NAME" {
			idx++
			continue
		}

		isExtend := false
		if tok.val == "extend" {
			isExtend = true
			idx++
			if idx >= len(tokens) {
				break
			}
			tok = tokens[idx]
		}

		switch tok.val {
		case "schema":
			idx++
			schemaDef, nextIdx := p.parseSchemaDef(tokens, idx, pendingDoc)
			res.Schema = schemaDef
			idx = nextIdx
			pendingDoc = ""

		case "type":
			idx++
			t, nextIdx := p.parseObjectType(tokens, idx, "type", isExtend, pendingDoc)
			res.Types = append(res.Types, t)
			idx = nextIdx
			pendingDoc = ""

			// If type is Query, Mutation, or Subscription, extract operations
			if t.Name == "Query" || t.Name == "Mutation" || t.Name == "Subscription" {
				opType := strings.ToLower(t.Name)
				for _, f := range t.Fields {
					res.Operations = append(res.Operations, GraphQLOperation{
						OperationType: opType,
						Name:          f.Name,
						Field:         f,
						Description:   f.Description,
					})
				}
			}

		case "interface":
			idx++
			t, nextIdx := p.parseObjectType(tokens, idx, "interface", isExtend, pendingDoc)
			res.Types = append(res.Types, t)
			idx = nextIdx
			pendingDoc = ""

		case "input":
			idx++
			t, nextIdx := p.parseObjectType(tokens, idx, "input", isExtend, pendingDoc)
			res.Types = append(res.Types, t)
			idx = nextIdx
			pendingDoc = ""

		case "enum":
			idx++
			t, nextIdx := p.parseEnumType(tokens, idx, isExtend, pendingDoc)
			res.Types = append(res.Types, t)
			idx = nextIdx
			pendingDoc = ""

		case "union":
			idx++
			t, nextIdx := p.parseUnionType(tokens, idx, isExtend, pendingDoc)
			res.Types = append(res.Types, t)
			idx = nextIdx
			pendingDoc = ""

		case "scalar":
			idx++
			t, nextIdx := p.parseScalarType(tokens, idx, isExtend, pendingDoc)
			res.Types = append(res.Types, t)
			idx = nextIdx
			pendingDoc = ""

		case "directive":
			idx++
			t, nextIdx := p.parseDirectiveDef(tokens, idx, pendingDoc)
			res.Types = append(res.Types, t)
			idx = nextIdx
			pendingDoc = ""

		default:
			idx++
		}
	}

	return res, nil
}

func (p *GraphQLParser) parseSchemaDef(tokens []gqlToken, idx int, doc string) (*GraphQLSchemaDef, int) {
	schema := &GraphQLSchemaDef{
		Description: doc,
	}

	// Directives on schema
	for idx < len(tokens) && tokens[idx].kind == "DIRECTIVE" {
		dir, nextIdx := p.parseDirectiveApplication(tokens, idx)
		schema.Directives = append(schema.Directives, dir)
		idx = nextIdx
	}

	if idx < len(tokens) && tokens[idx].val == "{" {
		idx++
		for idx < len(tokens) && tokens[idx].val != "}" {
			if tokens[idx].kind == "NAME" {
				opName := tokens[idx].val
				idx++
				if idx < len(tokens) && tokens[idx].val == ":" {
					idx++
					if idx < len(tokens) && tokens[idx].kind == "NAME" {
						typeName := tokens[idx].val
						switch opName {
						case "query":
							schema.Query = typeName
						case "mutation":
							schema.Mutation = typeName
						case "subscription":
							schema.Subscription = typeName
						}
						idx++
					}
				}
			} else {
				idx++
			}
		}
		if idx < len(tokens) && tokens[idx].val == "}" {
			idx++
		}
	}

	return schema, idx
}

func (p *GraphQLParser) parseObjectType(tokens []gqlToken, idx int, kind string, isExtend bool, doc string) (GraphQLType, int) {
	t := GraphQLType{
		Kind:        kind,
		IsExtension: isExtend,
		Description: doc,
	}

	if idx < len(tokens) && tokens[idx].kind == "NAME" {
		t.Name = tokens[idx].val
		idx++
	}

	// Check for 'implements'
	if idx < len(tokens) && tokens[idx].val == "implements" {
		idx++
		expectIface := true
		for idx < len(tokens) {
			if expectIface && tokens[idx].kind == "NAME" {
				t.Implements = append(t.Implements, tokens[idx].val)
				idx++
				expectIface = false
			} else if !expectIface && tokens[idx].val == "&" {
				idx++
				expectIface = true
			} else if expectIface && tokens[idx].val == "&" {
				idx++
			} else {
				break
			}
		}
	}

	// Directives
	for idx < len(tokens) && tokens[idx].kind == "DIRECTIVE" {
		dir, nextIdx := p.parseDirectiveApplication(tokens, idx)
		t.Directives = append(t.Directives, dir)
		if dir.Name == "key" && dir.Arguments["fields"] != "" {
			t.IsFederated = true
			t.KeyFields = dir.Arguments["fields"]
		}
		if dir.Name == "extends" {
			t.IsFederated = true
			t.IsExtension = true
		}
		idx = nextIdx
	}

	// Fields block { ... }
	if idx < len(tokens) && tokens[idx].val == "{" {
		idx++
		var fieldDoc string

		for idx < len(tokens) && tokens[idx].val != "}" {
			if tokens[idx].kind == "STRING" {
				fieldDoc = tokens[idx].val
				idx++
				continue
			}

			if tokens[idx].kind == "NAME" {
				fieldName := tokens[idx].val
				idx++

				f := GraphQLField{
					Name:        fieldName,
					Description: fieldDoc,
				}
				fieldDoc = ""

				// Field Arguments (arg1: Type, arg2: Type = default)
				if idx < len(tokens) && tokens[idx].val == "(" {
					idx++
					var argDoc string
					for idx < len(tokens) && tokens[idx].val != ")" {
						if tokens[idx].kind == "STRING" {
							argDoc = tokens[idx].val
							idx++
							continue
						}
						if tokens[idx].kind == "NAME" {
							argName := tokens[idx].val
							idx++
							argType := ""
							if idx < len(tokens) && tokens[idx].val == ":" {
								idx++
								argType, idx = p.parseTypeSignature(tokens, idx)
							}
							argDefVal := ""
							if idx < len(tokens) && tokens[idx].val == "=" {
								idx++
								if idx < len(tokens) {
									argDefVal = tokens[idx].val
									idx++
								}
							}
							var argDirs []GraphQLDirectiveApplication
							for idx < len(tokens) && tokens[idx].kind == "DIRECTIVE" {
								aDir, nextIdx := p.parseDirectiveApplication(tokens, idx)
								argDirs = append(argDirs, aDir)
								idx = nextIdx
							}

							f.Arguments = append(f.Arguments, GraphQLArgument{
								Name:         argName,
								Type:         argType,
								DefaultValue: argDefVal,
								Directives:   argDirs,
								Description:  argDoc,
							})
							argDoc = ""
						} else {
							idx++
						}
					}
					if idx < len(tokens) && tokens[idx].val == ")" {
						idx++
					}
				}

				// Field Return Type: : Type
				if idx < len(tokens) && tokens[idx].val == ":" {
					idx++
					f.Type, idx = p.parseTypeSignature(tokens, idx)
				}

				// Field default value (e.g. for input types: field: Type = default)
				if idx < len(tokens) && tokens[idx].val == "=" {
					idx++
					if idx < len(tokens) {
						f.DefaultValue = tokens[idx].val
						idx++
					}
				}

				// Field Directives
				for idx < len(tokens) && tokens[idx].kind == "DIRECTIVE" {
					fDir, nextIdx := p.parseDirectiveApplication(tokens, idx)
					f.Directives = append(f.Directives, fDir)
					idx = nextIdx
				}

				t.Fields = append(t.Fields, f)
			} else {
				idx++
			}
		}

		if idx < len(tokens) && tokens[idx].val == "}" {
			idx++
		}
	}

	return t, idx
}

func (p *GraphQLParser) parseEnumType(tokens []gqlToken, idx int, isExtend bool, doc string) (GraphQLType, int) {
	t := GraphQLType{
		Kind:        "enum",
		IsExtension: isExtend,
		Description: doc,
	}

	if idx < len(tokens) && tokens[idx].kind == "NAME" {
		t.Name = tokens[idx].val
		idx++
	}

	for idx < len(tokens) && tokens[idx].kind == "DIRECTIVE" {
		dir, nextIdx := p.parseDirectiveApplication(tokens, idx)
		t.Directives = append(t.Directives, dir)
		idx = nextIdx
	}

	if idx < len(tokens) && tokens[idx].val == "{" {
		idx++
		var valDoc string
		for idx < len(tokens) && tokens[idx].val != "}" {
			if tokens[idx].kind == "STRING" {
				valDoc = tokens[idx].val
				idx++
				continue
			}
			if tokens[idx].kind == "NAME" {
				valName := tokens[idx].val
				idx++
				var valDirs []GraphQLDirectiveApplication
				for idx < len(tokens) && tokens[idx].kind == "DIRECTIVE" {
					vDir, nextIdx := p.parseDirectiveApplication(tokens, idx)
					valDirs = append(valDirs, vDir)
					idx = nextIdx
				}
				t.EnumValues = append(t.EnumValues, GraphQLEnumValue{
					Name:        valName,
					Description: valDoc,
					Directives:  valDirs,
				})
				valDoc = ""
			} else {
				idx++
			}
		}
		if idx < len(tokens) && tokens[idx].val == "}" {
			idx++
		}
	}

	return t, idx
}

func (p *GraphQLParser) parseUnionType(tokens []gqlToken, idx int, isExtend bool, doc string) (GraphQLType, int) {
	t := GraphQLType{
		Kind:        "union",
		IsExtension: isExtend,
		Description: doc,
	}

	if idx < len(tokens) && tokens[idx].kind == "NAME" {
		t.Name = tokens[idx].val
		idx++
	}

	for idx < len(tokens) && tokens[idx].kind == "DIRECTIVE" {
		dir, nextIdx := p.parseDirectiveApplication(tokens, idx)
		t.Directives = append(t.Directives, dir)
		idx = nextIdx
	}

	if idx < len(tokens) && tokens[idx].val == "=" {
		idx++
		expectType := true
		for idx < len(tokens) {
			if expectType && tokens[idx].kind == "NAME" {
				t.UnionTypes = append(t.UnionTypes, tokens[idx].val)
				idx++
				expectType = false
			} else if !expectType && tokens[idx].val == "|" {
				idx++
				expectType = true
			} else if expectType && tokens[idx].val == "|" {
				idx++
			} else {
				break
			}
		}
	}

	return t, idx
}

func (p *GraphQLParser) parseScalarType(tokens []gqlToken, idx int, isExtend bool, doc string) (GraphQLType, int) {
	t := GraphQLType{
		Kind:        "scalar",
		IsExtension: isExtend,
		Description: doc,
	}

	if idx < len(tokens) && tokens[idx].kind == "NAME" {
		t.Name = tokens[idx].val
		idx++
	}

	for idx < len(tokens) && tokens[idx].kind == "DIRECTIVE" {
		dir, nextIdx := p.parseDirectiveApplication(tokens, idx)
		t.Directives = append(t.Directives, dir)
		idx = nextIdx
	}

	return t, idx
}

func (p *GraphQLParser) parseDirectiveDef(tokens []gqlToken, idx int, doc string) (GraphQLType, int) {
	t := GraphQLType{
		Kind:        "directive",
		Description: doc,
	}

	if idx < len(tokens) && (tokens[idx].kind == "DIRECTIVE" || tokens[idx].kind == "NAME") {
		name := tokens[idx].val
		name = strings.TrimPrefix(name, "@")
		t.Name = name
		idx++
	}

	// Arguments (arg: Type)
	if idx < len(tokens) && tokens[idx].val == "(" {
		idx++
		var argDoc string
		for idx < len(tokens) && tokens[idx].val != ")" {
			if tokens[idx].kind == "STRING" {
				argDoc = tokens[idx].val
				idx++
				continue
			}
			if tokens[idx].kind == "NAME" {
				argName := tokens[idx].val
				idx++
				argType := ""
				if idx < len(tokens) && tokens[idx].val == ":" {
					idx++
					argType, idx = p.parseTypeSignature(tokens, idx)
				}
				argDefVal := ""
				if idx < len(tokens) && tokens[idx].val == "=" {
					idx++
					if idx < len(tokens) {
						argDefVal = tokens[idx].val
						idx++
					}
				}
				t.Arguments = append(t.Arguments, GraphQLArgument{
					Name:         argName,
					Type:         argType,
					DefaultValue: argDefVal,
					Description:  argDoc,
				})
				argDoc = ""
			} else {
				idx++
			}
		}
		if idx < len(tokens) && tokens[idx].val == ")" {
			idx++
		}
	}

	// on LOCATION1 | LOCATION2
	if idx < len(tokens) && tokens[idx].val == "on" {
		idx++
		expectLoc := true
		for idx < len(tokens) {
			if expectLoc && tokens[idx].kind == "NAME" {
				t.Locations = append(t.Locations, tokens[idx].val)
				idx++
				expectLoc = false
			} else if !expectLoc && tokens[idx].val == "|" {
				idx++
				expectLoc = true
			} else if expectLoc && tokens[idx].val == "|" {
				idx++
			} else {
				break
			}
		}
	}

	return t, idx
}

func (p *GraphQLParser) parseDirectiveApplication(tokens []gqlToken, idx int) (GraphQLDirectiveApplication, int) {
	dirName := strings.TrimPrefix(tokens[idx].val, "@")
	idx++

	dir := GraphQLDirectiveApplication{
		Name:      dirName,
		Arguments: make(map[string]string),
	}

	rawParts := []string{"@" + dirName}

	if idx < len(tokens) && tokens[idx].val == "(" {
		rawParts = append(rawParts, "(")
		idx++
		for idx < len(tokens) && tokens[idx].val != ")" {
			if tokens[idx].kind == "NAME" {
				argName := tokens[idx].val
				idx++
				if idx < len(tokens) && tokens[idx].val == ":" {
					idx++
					if idx < len(tokens) {
						argVal := tokens[idx].val
						dir.Arguments[argName] = argVal
						rawParts = append(rawParts, fmt.Sprintf("%s: %s", argName, argVal))
						idx++
					}
				}
			} else {
				idx++
			}
		}
		if idx < len(tokens) && tokens[idx].val == ")" {
			rawParts = append(rawParts, ")")
			idx++
		}
	}

	dir.Raw = strings.Join(rawParts, "")
	return dir, idx
}

func (p *GraphQLParser) parseTypeSignature(tokens []gqlToken, idx int) (string, int) {
	if idx >= len(tokens) {
		return "", idx
	}

	var sb strings.Builder

	if tokens[idx].val == "[" {
		sb.WriteString("[")
		idx++
		innerType, nextIdx := p.parseTypeSignature(tokens, idx)
		sb.WriteString(innerType)
		idx = nextIdx
		if idx < len(tokens) && tokens[idx].val == "]" {
			sb.WriteString("]")
			idx++
		}
		if idx < len(tokens) && tokens[idx].val == "!" {
			sb.WriteString("!")
			idx++
		}
		return sb.String(), idx
	}

	if tokens[idx].kind == "NAME" {
		sb.WriteString(tokens[idx].val)
		idx++
		if idx < len(tokens) && tokens[idx].val == "!" {
			sb.WriteString("!")
			idx++
		}
		return sb.String(), idx
	}

	return "", idx
}

func (p *GraphQLParser) toSymbolNodes(res *GraphQLFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Schema Symbol Node if present
	if res.Schema != nil {
		schemaPayload, err := json.Marshal(res.Schema)
		if err != nil {
			return nil, err
		}

		var deps []string
		if res.Schema.Query != "" {
			deps = append(deps, res.Schema.Query)
		}
		if res.Schema.Mutation != "" {
			deps = append(deps, res.Schema.Mutation)
		}
		if res.Schema.Subscription != "" {
			deps = append(deps, res.Schema.Subscription)
		}

		schemaNode := &core.ASTSymbolNode{
			Language:          core.LangGraphQL,
			NodeType:          "SchemaDefinition",
			Identifier:        "schema",
			Signature:         "schema",
			Docstring:         res.Schema.Description,
			ASTPayload:        schemaPayload,
			ASTMetadata:       map[string]string{"query": res.Schema.Query, "mutation": res.Schema.Mutation, "subscription": res.Schema.Subscription},
			LocalDependencies: deps,
			Dependencies:      deps,
			Lineage:           lineage,
		}
		sID, err := core.HashASTSymbolNode(schemaNode)
		if err != nil {
			return nil, err
		}
		schemaNode.NodeID = sID
		nodes = append(nodes, schemaNode)
	}

	// Types
	for _, t := range res.Types {
		typePayload, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}

		var deps []string
		deps = append(deps, t.Implements...)
		deps = append(deps, t.UnionTypes...)
		for _, f := range t.Fields {
			cleanType := strings.Trim(f.Type, "[]!")
			if cleanType != "" {
				deps = append(deps, cleanType)
			}
			for _, arg := range f.Arguments {
				cleanArgType := strings.Trim(arg.Type, "[]!")
				if cleanArgType != "" {
					deps = append(deps, cleanArgType)
				}
			}
		}
		for _, arg := range t.Arguments {
			cleanArgType := strings.Trim(arg.Type, "[]!")
			if cleanArgType != "" {
				deps = append(deps, cleanArgType)
			}
		}
		sort.Strings(deps)

		nodeType := "ObjectTypeDefinition"
		switch t.Kind {
		case "interface":
			nodeType = "InterfaceTypeDefinition"
		case "union":
			nodeType = "UnionTypeDefinition"
		case "enum":
			nodeType = "EnumTypeDefinition"
		case "input":
			nodeType = "InputObjectTypeDefinition"
		case "scalar":
			nodeType = "ScalarTypeDefinition"
		case "directive":
			nodeType = "DirectiveDefinition"
		}

		meta := map[string]string{
			"kind":        t.Kind,
			"field_count": fmt.Sprintf("%d", len(t.Fields)),
		}
		if t.IsExtension {
			meta["is_extension"] = "true"
		}
		if t.IsFederated {
			meta["is_federated"] = "true"
			if t.KeyFields != "" {
				meta["key_fields"] = t.KeyFields
			}
		}

		tNode := &core.ASTSymbolNode{
			Language:          core.LangGraphQL,
			NodeType:          nodeType,
			Identifier:        t.Name,
			Signature:         fmt.Sprintf("%s %s", t.Kind, t.Name),
			Docstring:         t.Description,
			Visibility:        "public",
			ASTPayload:        typePayload,
			ASTMetadata:       meta,
			LocalDependencies: deps,
			Dependencies:      deps,
			Lineage:           lineage,
		}
		tNodeID, err := core.HashASTSymbolNode(tNode)
		if err != nil {
			return nil, err
		}
		tNode.NodeID = tNodeID
		nodes = append(nodes, tNode)

		// Create sub-symbols for operations if it's Query/Mutation/Subscription
		if t.Name == "Query" || t.Name == "Mutation" || t.Name == "Subscription" {
			for _, f := range t.Fields {
				fPayload, err := json.Marshal(f)
				if err != nil {
					continue
				}

				fMeta := map[string]string{
					"operation_type": strings.ToLower(t.Name),
					"return_type":    f.Type,
				}
				for _, dir := range f.Directives {
					fMeta["directive_"+dir.Name] = dir.Raw
				}

				var fDeps []string
				cleanRet := strings.Trim(f.Type, "[]!")
				if cleanRet != "" {
					fDeps = append(fDeps, cleanRet)
				}
				for _, arg := range f.Arguments {
					cleanArg := strings.Trim(arg.Type, "[]!")
					if cleanArg != "" {
						fDeps = append(fDeps, cleanArg)
					}
				}

				opNode := &core.ASTSymbolNode{
					Language:          core.LangGraphQL,
					NodeType:          "FieldDefinition",
					Identifier:        fmt.Sprintf("%s.%s", t.Name, f.Name),
					Signature:         fmt.Sprintf("%s(%s): %s", f.Name, formatGraphQLArgs(f.Arguments), f.Type),
					Docstring:         f.Description,
					Visibility:        "public",
					ASTPayload:        fPayload,
					ASTMetadata:       fMeta,
					LocalDependencies: fDeps,
					Dependencies:      fDeps,
					Lineage:           lineage,
				}
				opNodeID, err := core.HashASTSymbolNode(opNode)
				if err == nil {
					opNode.NodeID = opNodeID
					nodes = append(nodes, opNode)
				}
			}
		}
	}

	return nodes, nil
}

func formatGraphQLArgs(args []GraphQLArgument) string {
	var list []string
	for _, a := range args {
		def := ""
		if a.DefaultValue != "" {
			def = fmt.Sprintf(" = %s", a.DefaultValue)
		}
		list = append(list, fmt.Sprintf("%s: %s%s", a.Name, a.Type, def))
	}
	return strings.Join(list, ", ")
}

// BuildComponentNode bundles parsed GraphQL symbols into a core.ComponentNode.
func (p *GraphQLParser) BuildComponentNode(
	res *GraphQLFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = "graphql-schema"
	}

	var symbolIDs []string
	for _, sym := range res.AllSymbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangGraphQL,
		SymbolNodes: symbolIDs,
		Metadata: map[string]string{
			"file_path":   res.FilePath,
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
