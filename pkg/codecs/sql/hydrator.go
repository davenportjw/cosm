package sql

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmscm/cosm/pkg/core"
)

// SQLHydrator converts SQL ASTSymbolNodes back into canonical formatted SQL DDL source code.
type SQLHydrator struct{}

// NewSQLHydrator creates a new SQLHydrator instance.
func NewSQLHydrator() *SQLHydrator {
	return &SQLHydrator{}
}

// HydrateSymbol reconstitutes an ASTSymbolNode into SQL DDL text.
func (h *SQLHydrator) HydrateSymbol(node *core.ASTSymbolNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}

	if len(node.ASTPayload) == 0 {
		return fmt.Sprintf("-- SQL Symbol: %s (%s)\n", node.Identifier, node.NodeType), nil
	}

	switch node.NodeType {
	case "CreateTableStatement":
		var tbl SQLTable
		if err := json.Unmarshal(node.ASTPayload, &tbl); err != nil {
			return "", fmt.Errorf("unmarshaling SQLTable: %w", err)
		}
		return h.renderTable(&tbl), nil

	case "CreateIndexStatement":
		var idx SQLIndex
		if err := json.Unmarshal(node.ASTPayload, &idx); err != nil {
			return "", fmt.Errorf("unmarshaling SQLIndex: %w", err)
		}
		return h.renderIndex(&idx), nil

	case "ViewDefinition":
		var vw SQLView
		if err := json.Unmarshal(node.ASTPayload, &vw); err != nil {
			return "", fmt.Errorf("unmarshaling SQLView: %w", err)
		}
		return h.renderView(&vw), nil

	case "AlterTableStatement":
		var alter SQLAlterTable
		if err := json.Unmarshal(node.ASTPayload, &alter); err != nil {
			return "", fmt.Errorf("unmarshaling SQLAlterTable: %w", err)
		}
		return h.renderAlterTable(&alter), nil

	default:
		return string(node.ASTPayload), nil
	}
}

func (h *SQLHydrator) renderTable(tbl *SQLTable) string {
	if tbl.RawDDL != "" {
		return tbl.RawDDL + "\n"
	}

	var sb strings.Builder

	if tbl.Doc != "" {
		for _, line := range strings.Split(tbl.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("-- %s\n", line))
		}
	}

	tableName := tbl.Name
	if tbl.Schema != "" {
		tableName = fmt.Sprintf("%s.%s", tbl.Schema, tbl.Name)
	}

	sb.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", tableName))

	var definitions []string

	// Columns
	for _, col := range tbl.Columns {
		var parts []string
		parts = append(parts, fmt.Sprintf("    %s %s", col.Name, col.DataType))
		if col.IsPrimaryKey && len(tbl.PrimaryKeys) <= 1 {
			parts = append(parts, "PRIMARY KEY")
		} else if !col.IsNullable {
			parts = append(parts, "NOT NULL")
		}
		if col.IsUnique {
			parts = append(parts, "UNIQUE")
		}
		if col.DefaultValue != "" {
			parts = append(parts, fmt.Sprintf("DEFAULT %s", col.DefaultValue))
		}
		definitions = append(definitions, strings.Join(parts, " "))
	}

	// Composite Primary Key if multiple columns
	if len(tbl.PrimaryKeys) > 1 {
		definitions = append(definitions, fmt.Sprintf("    PRIMARY KEY (%s)", strings.Join(tbl.PrimaryKeys, ", ")))
	}

	// Foreign Keys
	for _, fk := range tbl.ForeignKeys {
		var fkParts []string
		fkParts = append(fkParts, "   ")
		if fk.ConstraintName != "" {
			fkParts = append(fkParts, fmt.Sprintf("CONSTRAINT %s", fk.ConstraintName))
		}
		fkParts = append(fkParts, fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s (%s)",
			strings.Join(fk.Columns, ", "),
			fk.TargetTable,
			strings.Join(fk.TargetColumns, ", "),
		))
		if fk.OnDelete != "" {
			fkParts = append(fkParts, fmt.Sprintf("ON DELETE %s", fk.OnDelete))
		}
		if fk.OnUpdate != "" {
			fkParts = append(fkParts, fmt.Sprintf("ON UPDATE %s", fk.OnUpdate))
		}
		definitions = append(definitions, strings.Join(fkParts, " "))
	}

	sb.WriteString(strings.Join(definitions, ",\n"))
	sb.WriteString("\n);\n")

	return sb.String()
}

func (h *SQLHydrator) renderIndex(idx *SQLIndex) string {
	if idx.RawDDL != "" {
		return idx.RawDDL + "\n"
	}

	var sb strings.Builder
	if idx.Doc != "" {
		for _, line := range strings.Split(idx.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("-- %s\n", line))
		}
	}

	uniqueStr := ""
	if idx.IsUnique {
		uniqueStr = "UNIQUE "
	}

	sb.WriteString(fmt.Sprintf("CREATE %sINDEX %s ON %s (%s);\n",
		uniqueStr, idx.Name, idx.TableName, strings.Join(idx.Columns, ", ")))
	return sb.String()
}

func (h *SQLHydrator) renderView(vw *SQLView) string {
	if vw.RawDDL != "" {
		return vw.RawDDL + "\n"
	}

	var sb strings.Builder
	if vw.Doc != "" {
		for _, line := range strings.Split(vw.Doc, "\n") {
			sb.WriteString(fmt.Sprintf("-- %s\n", line))
		}
	}

	sb.WriteString(fmt.Sprintf("CREATE VIEW %s AS\n%s;\n", vw.Name, vw.Query))
	return sb.String()
}

func (h *SQLHydrator) renderAlterTable(alter *SQLAlterTable) string {
	if alter.RawDDL != "" {
		return alter.RawDDL + "\n"
	}
	return fmt.Sprintf("ALTER TABLE %s %s;\n", alter.TableName, alter.Action)
}
