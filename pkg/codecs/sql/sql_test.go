package sql

import (
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestSQLParser_CompleteDDL(t *testing.T) {
	sqlSrc := `-- Core users table
CREATE TABLE public.users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    full_name TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Orders table referencing users
CREATE TABLE public.orders (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL,
    total_amount NUMERIC(10, 2) NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',
    CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

-- Index for searching orders by user
CREATE INDEX idx_orders_user_id ON orders (user_id);

-- Active user orders view
CREATE VIEW active_orders AS
SELECT o.id, o.user_id, u.email, o.total_amount
FROM orders o
JOIN users u ON o.user_id = u.id
WHERE o.status = 'ACTIVE';
`

	lineage := core.LineageEnvelope{
		UserID:           "db-admin",
		UserPrompt:       "Design Postgres schema with users and orders",
		ExecutingAgentID: "agent-db",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewSQLParser()
	res, err := parser.ParseSource("migrations/001_init.sql", []byte(sqlSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Validate Tables
	if len(res.Tables) != 2 {
		t.Fatalf("Expected 2 tables, got %d", len(res.Tables))
	}

	usersTable := res.Tables[0]
	if usersTable.Name != "users" || usersTable.Schema != "public" {
		t.Errorf("Expected public.users, got %s.%s", usersTable.Schema, usersTable.Name)
	}
	if len(usersTable.Columns) != 4 {
		t.Errorf("Expected 4 columns in users, got %d", len(usersTable.Columns))
	}
	if len(usersTable.PrimaryKeys) != 1 || usersTable.PrimaryKeys[0] != "id" {
		t.Errorf("Expected PK 'id', got %v", usersTable.PrimaryKeys)
	}

	ordersTable := res.Tables[1]
	if ordersTable.Name != "orders" {
		t.Errorf("Expected table orders, got %s", ordersTable.Name)
	}
	if len(ordersTable.ForeignKeys) != 1 {
		t.Fatalf("Expected 1 FK in orders, got %d", len(ordersTable.ForeignKeys))
	}
	fk := ordersTable.ForeignKeys[0]
	if fk.TargetTable != "users" || fk.Columns[0] != "user_id" || fk.TargetColumns[0] != "id" {
		t.Errorf("Unexpected FK: %+v", fk)
	}

	// 2. Validate Index
	if len(res.Indexes) != 1 {
		t.Fatalf("Expected 1 index, got %d", len(res.Indexes))
	}
	idx := res.Indexes[0]
	if idx.Name != "idx_orders_user_id" || idx.TableName != "orders" {
		t.Errorf("Unexpected index: %+v", idx)
	}

	// 3. Validate View
	if len(res.Views) != 1 {
		t.Fatalf("Expected 1 view, got %d", len(res.Views))
	}
	vw := res.Views[0]
	if vw.Name != "active_orders" {
		t.Errorf("Expected active_orders view, got %s", vw.Name)
	}
	if len(vw.ReferencedTables) < 2 {
		t.Errorf("Expected referenced tables orders and users, got %v", vw.ReferencedTables)
	}

	// 4. Validate Component Node
	comp, err := parser.BuildComponentNode(res, "db-schema", core.CompDatabase, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Name != "db-schema" || comp.Type != core.CompDatabase {
		t.Errorf("Unexpected component node: %+v", comp)
	}
	if len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("Symbol count mismatch: %d vs %d", len(comp.SymbolNodes), len(res.AllSymbols))
	}

	// 5. Validate Hydrator Round-trip
	hydrator := NewSQLHydrator()
	for _, sym := range res.AllSymbols {
		code, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("HydrateSymbol failed for %s: %v", sym.Identifier, err)
		}
		if code == "" {
			t.Fatalf("Hydrated SQL is empty for %s", sym.Identifier)
		}
		if sym.NodeType == "CreateTableStatement" {
			if !strings.Contains(code, "CREATE TABLE") {
				t.Errorf("Hydrated table missing CREATE TABLE: %s", code)
			}
		}
	}
}
