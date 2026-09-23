package sql

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

// SQLColumn represents a column definition in a table.
type SQLColumn struct {
	Name         string `json:"name"`
	DataType     string `json:"data_type"`
	IsPrimaryKey bool   `json:"is_primary_key"`
	IsNullable   bool   `json:"is_nullable"`
	IsUnique     bool   `json:"is_unique"`
	DefaultValue string `json:"default_value,omitempty"`
	Doc          string `json:"doc,omitempty"`
}

// SQLForeignKey represents a foreign key constraint.
type SQLForeignKey struct {
	ConstraintName string   `json:"constraint_name,omitempty"`
	Columns        []string `json:"columns"`
	TargetTable    string   `json:"target_table"`
	TargetColumns  []string `json:"target_columns"`
	OnDelete       string   `json:"on_delete,omitempty"`
	OnUpdate       string   `json:"on_update,omitempty"`
}

// SQLTable represents an extracted CREATE TABLE statement.
type SQLTable struct {
	Schema      string          `json:"schema,omitempty"`
	Name        string          `json:"name"`
	Columns     []SQLColumn     `json:"columns"`
	PrimaryKeys []string        `json:"primary_keys,omitempty"`
	ForeignKeys []SQLForeignKey `json:"foreign_keys,omitempty"`
	Doc         string          `json:"doc,omitempty"`
	RawDDL      string          `json:"raw_ddl,omitempty"`
}

// SQLIndex represents an extracted CREATE INDEX statement.
type SQLIndex struct {
	Name      string   `json:"name"`
	TableName string   `json:"table_name"`
	Columns   []string `json:"columns"`
	IsUnique  bool     `json:"is_unique"`
	Doc       string   `json:"doc,omitempty"`
	RawDDL    string   `json:"raw_ddl,omitempty"`
}

// SQLView represents an extracted CREATE VIEW statement.
type SQLView struct {
	Name             string   `json:"name"`
	Query            string   `json:"query"`
	ReferencedTables []string `json:"referenced_tables,omitempty"`
	Doc              string   `json:"doc,omitempty"`
	RawDDL           string   `json:"raw_ddl,omitempty"`
}

// SQLAlterTable represents an ALTER TABLE statement.
type SQLAlterTable struct {
	TableName string `json:"table_name"`
	Action    string `json:"action"` // ADD COLUMN, DROP COLUMN, ADD CONSTRAINT, etc.
	Details   string `json:"details"`
	RawDDL    string `json:"raw_ddl,omitempty"`
}

// SQLFileResult contains all extracted SQL DDL statements and entities.
type SQLFileResult struct {
	FilePath   string                `json:"file_path"`
	Tables     []SQLTable            `json:"tables"`
	Indexes    []SQLIndex            `json:"indexes"`
	Views      []SQLView             `json:"views"`
	AlterTable []SQLAlterTable       `json:"alter_table"`
	AllSymbols []*core.ASTSymbolNode `json:"all_symbols"`
	Trivia     *core.TriviaEnvelope  `json:"trivia,omitempty"`
}

// SQLParser parses SQL DDL files and extracts structured schema symbol nodes.
type SQLParser struct{}

// NewSQLParser creates a new SQLParser instance.
func NewSQLParser() *SQLParser {
	return &SQLParser{}
}

// ParseSource parses SQL DDL statements from bytes.
func (p *SQLParser) ParseSource(filename string, src []byte, lineage core.LineageEnvelope) (*SQLFileResult, error) {
	if filename == "" {
		filename = "schema.sql"
	}

	result := &SQLFileResult{
		FilePath: filename,
		Trivia:   extractSQLTrivia(src),
	}

	statements := splitSQLStatements(src)

	for _, stmt := range statements {
		stmtText := strings.TrimSpace(stmt.text)
		if stmtText == "" {
			continue
		}

		upper := strings.ToUpper(stmtText)
		if strings.HasPrefix(upper, "CREATE TABLE") || strings.HasPrefix(upper, "CREATE UNLOGGED TABLE") {
			table := p.parseCreateTable(stmtText, stmt.doc)
			result.Tables = append(result.Tables, table)
		} else if strings.HasPrefix(upper, "CREATE INDEX") || strings.HasPrefix(upper, "CREATE UNIQUE INDEX") {
			idx := p.parseCreateIndex(stmtText, stmt.doc)
			result.Indexes = append(result.Indexes, idx)
		} else if strings.HasPrefix(upper, "CREATE VIEW") || strings.HasPrefix(upper, "CREATE OR REPLACE VIEW") || strings.HasPrefix(upper, "CREATE MATERIALIZED VIEW") {
			view := p.parseCreateView(stmtText, stmt.doc)
			result.Views = append(result.Views, view)
		} else if strings.HasPrefix(upper, "ALTER TABLE") {
			alter := p.parseAlterTable(stmtText)
			result.AlterTable = append(result.AlterTable, alter)
		}
	}

	// Convert all statements to ASTSymbolNodes
	symbols, err := p.toSymbolNodes(result, lineage)
	if err != nil {
		return nil, fmt.Errorf("converting to ASTSymbolNodes: %w", err)
	}
	result.AllSymbols = symbols

	return result, nil
}

// ParseFile parses a SQL file from disk.
func (p *SQLParser) ParseFile(filePath string, lineage core.LineageEnvelope) (*SQLFileResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}
	return p.ParseSource(filePath, data, lineage)
}

// ParseDir parses all .sql files in a directory.
func (p *SQLParser) ParseDir(dirPath string, lineage core.LineageEnvelope) ([]*SQLFileResult, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dirPath, err)
	}

	var results []*SQLFileResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".sql" {
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

type sqlStmt struct {
	text string
	doc  string
}

func splitSQLStatements(src []byte) []sqlStmt {
	var stmts []sqlStmt
	scanner := bufio.NewScanner(bytes.NewReader(src))

	var current strings.Builder
	var currentDoc []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "--") {
			docLine := strings.TrimSpace(strings.TrimPrefix(trimmed, "--"))
			if docLine != "" {
				currentDoc = append(currentDoc, docLine)
			}
			continue
		}

		if trimmed == "" {
			continue
		}

		current.WriteString(line)
		current.WriteString("\n")

		if strings.HasSuffix(trimmed, ";") {
			stmts = append(stmts, sqlStmt{
				text: current.String(),
				doc:  strings.Join(currentDoc, "\n"),
			})
			current.Reset()
			currentDoc = nil
		}
	}

	if current.Len() > 0 {
		stmts = append(stmts, sqlStmt{
			text: current.String(),
			doc:  strings.Join(currentDoc, "\n"),
		})
	}

	return stmts
}

var (
	createTableRegex = regexp.MustCompile(`(?i)^CREATE\s+(?:UNLOGGED\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:([a-zA-Z0-9_]+)\.)?([a-zA-Z0-9_]+)\s*\(([\s\S]+)\)\s*;?`)
	createIndexRegex = regexp.MustCompile(`(?i)^CREATE\s+(UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z0-9_]+)\s+ON\s+([a-zA-Z0-9_.]+)\s*(?:USING\s+[a-zA-Z0-9_]+\s*)?\(([\s\S]+?)\)\s*;?`)
	createViewRegex  = regexp.MustCompile(`(?i)^CREATE\s+(?:OR\s+REPLACE\s+|MATERIALIZED\s+)?VIEW\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z0-9_.]+)\s+AS\s+([\s\S]+);?`)
	alterTableRegex  = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?([a-zA-Z0-9_.]+)\s+([\s\S]+);?`)
	fkInlineRegex    = regexp.MustCompile(`(?i)REFERENCES\s+([a-zA-Z0-9_]+)\s*\(([^)]+)\)`)
	fkTableRegex     = regexp.MustCompile(`(?i)(?:CONSTRAINT\s+([a-zA-Z0-9_]+)\s+)?FOREIGN\s+KEY\s*\(([^)]+)\)\s+REFERENCES\s+([a-zA-Z0-9_]+)\s*\(([^)]+)\)(?:\s+ON\s+DELETE\s+([a-zA-Z\s]+))?(?:\s+ON\s+UPDATE\s+([a-zA-Z\s]+))?`)
	pkTableRegex     = regexp.MustCompile(`(?i)(?:CONSTRAINT\s+[a-zA-Z0-9_]+\s+)?PRIMARY\s+KEY\s*\(([^)]+)\)`)
)

func (p *SQLParser) parseCreateTable(raw, doc string) SQLTable {
	table := SQLTable{
		RawDDL: strings.TrimSpace(raw),
		Doc:    doc,
	}

	m := createTableRegex.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		// Fallback for simple names
		parts := strings.Fields(raw)
		if len(parts) >= 3 {
			table.Name = strings.Trim(parts[2], `(;"'`)
		}
		return table
	}

	table.Schema = m[1]
	table.Name = m[2]
	body := m[3]

	// Split body items by commas outside parentheses
	items := splitSQLBodyItems(body)

	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		itemUpper := strings.ToUpper(item)

		// Table-level PRIMARY KEY constraint
		if pkm := pkTableRegex.FindStringSubmatch(item); pkm != nil {
			for _, pkCol := range strings.Split(pkm[1], ",") {
				pkCol = strings.TrimSpace(strings.Trim(pkCol, `"'`))
				if pkCol != "" {
					table.PrimaryKeys = append(table.PrimaryKeys, pkCol)
				}
			}
			continue
		}

		// Table-level FOREIGN KEY constraint
		if fkm := fkTableRegex.FindStringSubmatch(item); fkm != nil {
			cName := fkm[1]
			srcCols := splitAndClean(fkm[2])
			targetTable := strings.Trim(fkm[3], `"'`)
			targetCols := splitAndClean(fkm[4])
			onDel := strings.TrimSpace(fkm[5])
			onUpd := strings.TrimSpace(fkm[6])

			table.ForeignKeys = append(table.ForeignKeys, SQLForeignKey{
				ConstraintName: cName,
				Columns:        srcCols,
				TargetTable:    targetTable,
				TargetColumns:  targetCols,
				OnDelete:       onDel,
				OnUpdate:       onUpd,
			})
			continue
		}

		// Column definition
		tokens := strings.Fields(item)
		if len(tokens) >= 2 {
			colName := strings.Trim(tokens[0], `"'`)
			colType := tokens[1]
			isPK := strings.Contains(itemUpper, "PRIMARY KEY")
			isNotNull := strings.Contains(itemUpper, "NOT NULL")
			isUnique := strings.Contains(itemUpper, "UNIQUE")

			var defVal string
			if defIdx := strings.Index(itemUpper, "DEFAULT "); defIdx != -1 {
				defPart := item[defIdx+8:]
				defTokens := strings.Fields(defPart)
				if len(defTokens) > 0 {
					defVal = defTokens[0]
				}
			}

			// Check inline foreign key
			if fki := fkInlineRegex.FindStringSubmatch(item); fki != nil {
				targetTable := strings.Trim(fki[1], `"'`)
				targetCols := splitAndClean(fki[2])
				table.ForeignKeys = append(table.ForeignKeys, SQLForeignKey{
					Columns:       []string{colName},
					TargetTable:   targetTable,
					TargetColumns: targetCols,
				})
			}

			if isPK {
				table.PrimaryKeys = append(table.PrimaryKeys, colName)
			}

			table.Columns = append(table.Columns, SQLColumn{
				Name:         colName,
				DataType:     colType,
				IsPrimaryKey: isPK,
				IsNullable:   !isNotNull && !isPK,
				IsUnique:     isUnique,
				DefaultValue: defVal,
			})
		}
	}

	return table
}

func (p *SQLParser) parseCreateIndex(raw, doc string) SQLIndex {
	idx := SQLIndex{
		RawDDL: strings.TrimSpace(raw),
		Doc:    doc,
	}

	m := createIndexRegex.FindStringSubmatch(strings.TrimSpace(raw))
	if m != nil {
		idx.IsUnique = strings.TrimSpace(m[1]) != ""
		idx.Name = m[2]
		idx.TableName = m[3]
		idx.Columns = splitAndClean(m[4])
	}
	return idx
}

func (p *SQLParser) parseCreateView(raw, doc string) SQLView {
	view := SQLView{
		RawDDL: strings.TrimSpace(raw),
		Doc:    doc,
	}

	m := createViewRegex.FindStringSubmatch(strings.TrimSpace(raw))
	if m != nil {
		view.Name = m[1]
		view.Query = strings.TrimSpace(m[2])

		// Extract referenced tables from query (e.g. FROM users, JOIN orders)
		fromRegex := regexp.MustCompile(`(?i)(?:FROM|JOIN)\s+([a-zA-Z0-9_.]+)`)
		fMatches := fromRegex.FindAllStringSubmatch(view.Query, -1)
		seen := make(map[string]bool)
		for _, fm := range fMatches {
			tName := strings.Trim(fm[1], `"'`)
			if !seen[tName] {
				seen[tName] = true
				view.ReferencedTables = append(view.ReferencedTables, tName)
			}
		}
	}
	return view
}

func (p *SQLParser) parseAlterTable(raw string) SQLAlterTable {
	alter := SQLAlterTable{
		RawDDL: strings.TrimSpace(raw),
	}
	m := alterTableRegex.FindStringSubmatch(strings.TrimSpace(raw))
	if m != nil {
		alter.TableName = m[1]
		alter.Action = strings.TrimSpace(m[2])
	}
	return alter
}

func splitSQLBodyItems(body string) []string {
	var items []string
	var current strings.Builder
	depth := 0

	for _, ch := range body {
		switch ch {
		case '(':
			depth++
			current.WriteRune(ch)
		case ')':
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

func splitAndClean(s string) []string {
	var res []string
	for _, p := range strings.Split(s, ",") {
		c := strings.TrimSpace(strings.Trim(p, `"'`))
		if c != "" {
			res = append(res, c)
		}
	}
	return res
}

func (p *SQLParser) toSymbolNodes(res *SQLFileResult, lineage core.LineageEnvelope) ([]*core.ASTSymbolNode, error) {
	var nodes []*core.ASTSymbolNode

	// Tables
	for _, tbl := range res.Tables {
		tblPayload, err := json.Marshal(tbl)
		if err != nil {
			return nil, err
		}

		var deps []string
		for _, fk := range tbl.ForeignKeys {
			if fk.TargetTable != "" {
				deps = append(deps, fk.TargetTable)
			}
		}
		sort.Strings(deps)

		meta := map[string]string{
			"table_name":   tbl.Name,
			"column_count": fmt.Sprintf("%d", len(tbl.Columns)),
		}
		if len(tbl.PrimaryKeys) > 0 {
			meta["primary_keys"] = strings.Join(tbl.PrimaryKeys, ",")
		}
		if len(tbl.ForeignKeys) > 0 {
			meta["foreign_keys_count"] = fmt.Sprintf("%d", len(tbl.ForeignKeys))
		}

		ident := tbl.Name
		if tbl.Schema != "" {
			ident = fmt.Sprintf("%s.%s", tbl.Schema, tbl.Name)
		}

		node := &core.ASTSymbolNode{
			Language:          core.LangSQL,
			NodeType:          "CreateTableStatement",
			Identifier:        fmt.Sprintf("table:%s", ident),
			Signature:         fmt.Sprintf("CREATE TABLE %s", ident),
			Docstring:         tbl.Doc,
			Visibility:        "public",
			ASTPayload:        tblPayload,
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
	}

	// Indexes
	for _, idx := range res.Indexes {
		idxPayload, err := json.Marshal(idx)
		if err != nil {
			return nil, err
		}

		node := &core.ASTSymbolNode{
			Language:          core.LangSQL,
			NodeType:          "CreateIndexStatement",
			Identifier:        fmt.Sprintf("index:%s", idx.Name),
			Signature:         fmt.Sprintf("CREATE INDEX %s ON %s", idx.Name, idx.TableName),
			Docstring:         idx.Doc,
			Visibility:        "public",
			ASTPayload:        idxPayload,
			ASTMetadata:       map[string]string{"table": idx.TableName, "unique": fmt.Sprintf("%t", idx.IsUnique)},
			LocalDependencies: []string{idx.TableName},
			Dependencies:      []string{idx.TableName},
			Lineage:           lineage,
		}

		nodeID, err := core.HashASTSymbolNode(node)
		if err != nil {
			return nil, err
		}
		node.NodeID = nodeID
		nodes = append(nodes, node)
	}

	// Views
	for _, vw := range res.Views {
		vwPayload, err := json.Marshal(vw)
		if err != nil {
			return nil, err
		}

		node := &core.ASTSymbolNode{
			Language:          core.LangSQL,
			NodeType:          "ViewDefinition",
			Identifier:        fmt.Sprintf("view:%s", vw.Name),
			Signature:         fmt.Sprintf("CREATE VIEW %s", vw.Name),
			Docstring:         vw.Doc,
			Visibility:        "public",
			ASTPayload:        vwPayload,
			ASTMetadata:       map[string]string{"view_name": vw.Name},
			LocalDependencies: vw.ReferencedTables,
			Dependencies:      vw.ReferencedTables,
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

// BuildComponentNode bundles parsed SQL symbols into a core.ComponentNode.
func (p *SQLParser) BuildComponentNode(
	res *SQLFileResult,
	compName string,
	compType core.ComponentType,
	lineage core.LineageEnvelope,
) (*core.ComponentNode, error) {
	if compName == "" {
		compName = "database-schema"
	}
	if !compType.IsValid() {
		compType = core.CompDatabase
	}

	symbolIDs := make([]string, 0, len(res.AllSymbols))
	for _, sym := range res.AllSymbols {
		symbolIDs = append(symbolIDs, sym.NodeID)
	}

	metadata := map[string]string{
		"file_path":    res.FilePath,
		"table_count":  fmt.Sprintf("%d", len(res.Tables)),
		"index_count":  fmt.Sprintf("%d", len(res.Indexes)),
		"view_count":   fmt.Sprintf("%d", len(res.Views)),
		"symbol_count": fmt.Sprintf("%d", len(res.AllSymbols)),
	}

	comp := &core.ComponentNode{
		Name:        compName,
		Type:        compType,
		Language:    core.LangSQL,
		SymbolNodes: symbolIDs,
		Trivia:      res.Trivia,
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

func extractSQLTrivia(src []byte) *core.TriviaEnvelope {
	lines := strings.Split(string(src), "\n")
	var directives []string
	var licenseLines []string
	inBlock := false
	var blockLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if inBlock {
			blockLines = append(blockLines, line)
			if strings.Contains(trimmed, "*/") {
				inBlock = false
				joined := strings.Join(blockLines, "\n")
				if isSQLLicenseText(joined) {
					licenseLines = append(licenseLines, joined)
				}
				blockLines = nil
			}
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			if strings.Contains(trimmed, "*/") {
				if isSQLLicenseText(trimmed) {
					licenseLines = append(licenseLines, trimmed)
				}
			} else {
				inBlock = true
				blockLines = append(blockLines, line)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "-- dialect:") ||
			strings.HasPrefix(trimmed, "-- name:") ||
			strings.HasPrefix(trimmed, "-- cosm:") {
			directives = append(directives, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "--") {
			if isSQLLicenseText(trimmed) {
				licenseLines = append(licenseLines, trimmed)
			}
			continue
		}
		// First non-comment statement
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

func isSQLLicenseText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "copyright") ||
		strings.Contains(lower, "license") ||
		strings.Contains(lower, "licensed") ||
		strings.Contains(lower, "apache") ||
		strings.Contains(lower, "mit license") ||
		strings.Contains(lower, "spdx-license-identifier") ||
		strings.Contains(lower, "all rights reserved")
}
