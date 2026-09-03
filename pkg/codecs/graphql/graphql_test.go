package graphql

import (
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestGraphQLParser_CompleteSchema(t *testing.T) {
	gqlSrc := `"""
Root Schema Configuration
"""
schema {
  query: Query
  mutation: Mutation
  subscription: Subscription
}

"""
Custom DateTime Scalar
"""
scalar DateTime

"""
User role access control enum
"""
enum Role {
  """Administrator with full access"""
  ADMIN
  """Standard verified customer"""
  USER
  """Guest read-only visitor"""
  GUEST
}

"""
Common Node Interface
"""
interface Node {
  id: ID!
}

"""
User Account Entity
"""
type User implements Node @key(fields: "id") {
  id: ID!
  username: String!
  email: String!
  role: Role!
  createdAt: DateTime!
  orders(limit: Int = 10, offset: Int = 0): [Order!]!
}

type Order implements Node {
  id: ID!
  userId: ID!
  total: Float!
  items: [String!]!
}

union SearchResult = User | Order

input CreateUserInput {
  username: String!
  email: String!
  role: Role = USER
}

directive @auth(role: Role = ADMIN) on FIELD_DEFINITION | OBJECT

type Query {
  me: User
  getUser(id: ID!): User
  search(query: String!): [SearchResult!]!
}

type Mutation {
  createUser(input: CreateUserInput!): User! @auth(role: ADMIN)
  deleteUser(id: ID!): Boolean!
}

type Subscription {
  userCreated: User!
}
`

	lineage := core.LineageEnvelope{
		UserID:           "gql-dev",
		UserPrompt:       "Design federated GraphQL schema",
		ExecutingAgentID: "agent-graphql",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewGraphQLParser()
	res, err := parser.ParseSource("schema.graphql", []byte(gqlSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	// 1. Schema check
	if res.Schema == nil {
		t.Fatalf("Expected Schema to be parsed, got nil")
	}
	if res.Schema.Query != "Query" || res.Schema.Mutation != "Mutation" || res.Schema.Subscription != "Subscription" {
		t.Errorf("Schema root types mismatch: %+v", res.Schema)
	}

	// 2. Types check
	// DateTime, Role, Node, User, Order, SearchResult, CreateUserInput, @auth, Query, Mutation, Subscription = 11 types
	if len(res.Types) != 11 {
		t.Fatalf("Expected 11 types/definitions, got %d", len(res.Types))
	}

	typeMap := make(map[string]GraphQLType)
	for _, typ := range res.Types {
		typeMap[typ.Name] = typ
	}

	// Verify Scalar
	if dt, ok := typeMap["DateTime"]; !ok || dt.Kind != "scalar" {
		t.Errorf("Expected scalar DateTime, got %+v", dt)
	}

	// Verify Enum
	if role, ok := typeMap["Role"]; !ok || role.Kind != "enum" {
		t.Errorf("Expected enum Role, got %+v", role)
	} else if len(role.EnumValues) != 3 {
		t.Errorf("Expected 3 enum values in Role, got %d", len(role.EnumValues))
	}

	// Verify Interface
	if node, ok := typeMap["Node"]; !ok || node.Kind != "interface" {
		t.Errorf("Expected interface Node, got %+v", node)
	} else if len(node.Fields) != 1 || node.Fields[0].Name != "id" {
		t.Errorf("Interface Node fields mismatch: %+v", node.Fields)
	}

	// Verify User Object Type with Federation & Directives
	if user, ok := typeMap["User"]; !ok || user.Kind != "type" {
		t.Errorf("Expected type User, got %+v", user)
	} else {
		if !user.IsFederated || user.KeyFields != "id" {
			t.Errorf("Expected federated User with key fields 'id', got isFed=%v key=%s", user.IsFederated, user.KeyFields)
		}
		if len(user.Implements) != 1 || user.Implements[0] != "Node" {
			t.Errorf("Expected User implements [Node], got %+v", user.Implements)
		}
		if len(user.Fields) != 6 {
			t.Errorf("Expected 6 fields on User, got %d", len(user.Fields))
		}
		// Check orders field arguments
		ordersField := user.Fields[5]
		if ordersField.Name != "orders" || len(ordersField.Arguments) != 2 {
			t.Errorf("Expected orders field with 2 arguments, got %+v", ordersField)
		}
	}

	// Verify Union
	if sr, ok := typeMap["SearchResult"]; !ok || sr.Kind != "union" {
		t.Errorf("Expected union SearchResult, got %+v", sr)
	} else if len(sr.UnionTypes) != 2 || sr.UnionTypes[0] != "User" || sr.UnionTypes[1] != "Order" {
		t.Errorf("Union SearchResult types mismatch: %+v", sr.UnionTypes)
	}

	// Verify Input
	if inp, ok := typeMap["CreateUserInput"]; !ok || inp.Kind != "input" {
		t.Errorf("Expected input CreateUserInput, got %+v", inp)
	} else if len(inp.Fields) != 3 {
		t.Errorf("Expected 3 fields on CreateUserInput, got %d", len(inp.Fields))
	}

	// Verify Directive Definition
	if auth, ok := typeMap["auth"]; !ok || auth.Kind != "directive" {
		t.Errorf("Expected directive @auth, got %+v", auth)
	} else {
		if len(auth.Arguments) != 1 || auth.Arguments[0].Name != "role" {
			t.Errorf("Expected 1 argument 'role' on @auth, got %+v", auth.Arguments)
		}
		if len(auth.Locations) != 2 {
			t.Errorf("Expected 2 locations on @auth, got %d", len(auth.Locations))
		}
	}

	// 3. Operations Check (3 Query + 2 Mutation + 1 Subscription = 6 operations)
	if len(res.Operations) != 6 {
		t.Fatalf("Expected 6 operations, got %d", len(res.Operations))
	}

	opNames := make(map[string]bool)
	for _, op := range res.Operations {
		opNames[op.Name] = true
	}
	expectedOps := []string{"me", "getUser", "search", "createUser", "deleteUser", "userCreated"}
	for _, exp := range expectedOps {
		if !opNames[exp] {
			t.Errorf("Expected operation '%s' to be extracted", exp)
		}
	}

	// 4. Component Node Check
	comp, err := parser.BuildComponentNode(res, "user-graphql", core.CompContract, lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Language != core.LangGraphQL {
		t.Errorf("Expected LangGraphQL, got %s", comp.Language)
	}
	if len(comp.SymbolNodes) != len(res.AllSymbols) {
		t.Errorf("Component symbols mismatch: %d vs %d", len(comp.SymbolNodes), len(res.AllSymbols))
	}

	// 5. Hydration Round-Trip Check
	hydrator := NewGraphQLHydrator()
	var hydratedSchema strings.Builder
	for _, sym := range res.AllSymbols {
		code, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("Failed to hydrate GraphQL symbol %s: %v", sym.Identifier, err)
		}
		if code == "" {
			t.Fatalf("Hydrated GraphQL code is empty for %s", sym.Identifier)
		}

		if sym.NodeType == "ObjectTypeDefinition" && sym.Identifier == "User" {
			if !strings.Contains(code, "type User implements Node") {
				t.Errorf("Hydrated User missing header: %s", code)
			}
			if !strings.Contains(code, "id: ID!") || !strings.Contains(code, "orders") {
				t.Errorf("Hydrated User missing fields: %s", code)
			}
		}

		if sym.NodeType == "SchemaDefinition" {
			if !strings.Contains(code, "schema {") || !strings.Contains(code, "query: Query") {
				t.Errorf("Hydrated Schema mismatch: %s", code)
			}
		}

		// Re-assemble non-subfield symbols
		if sym.NodeType != "FieldDefinition" {
			hydratedSchema.WriteString(code)
			hydratedSchema.WriteString("\n")
		}
	}

	// 6. Roundtrip Re-parse test
	reParsed, err := parser.ParseSource("rehydrated.graphql", []byte(hydratedSchema.String()), lineage)
	if err != nil {
		t.Fatalf("Failed to re-parse hydrated GraphQL schema: %v\nSchema:\n%s", err, hydratedSchema.String())
	}
	if len(reParsed.Types) != len(res.Types) {
		t.Errorf("Re-parsed type count mismatch: %d vs %d", len(reParsed.Types), len(res.Types))
	}
	if len(reParsed.Operations) != len(res.Operations) {
		t.Errorf("Re-parsed operation count mismatch: %d vs %d", len(reParsed.Operations), len(res.Operations))
	}
}

func TestGraphQLParser_FederationAndExtensions(t *testing.T) {
	fedSrc := `
extend type Product @key(fields: "upc") {
  upc: String! @external
  weight: Int @external
  price: Int @external
  inStock: Boolean
  shippingEstimate: Int @requires(fields: "price weight")
}

extend type Query {
  topProducts(first: Int = 5): [Product]
}
`

	lineage := core.LineageEnvelope{
		UserID:           "gql-fed",
		UserPrompt:       "Federated subgraph extension",
		ExecutingAgentID: "agent-subgraph",
		Timestamp:        time.Now().UTC(),
	}

	parser := NewGraphQLParser()
	res, err := parser.ParseSource("federation.graphql", []byte(fedSrc), lineage)
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if len(res.Types) != 2 {
		t.Fatalf("Expected 2 types, got %d", len(res.Types))
	}

	prod := res.Types[0]
	if prod.Name != "Product" || !prod.IsExtension || !prod.IsFederated {
		t.Errorf("Product type mismatch: %+v", prod)
	}
	if prod.KeyFields != "upc" {
		t.Errorf("Expected Product key field 'upc', got '%s'", prod.KeyFields)
	}
	if len(prod.Fields) != 5 {
		t.Errorf("Expected 5 fields on Product, got %d", len(prod.Fields))
	}

	// Verify Query extension
	queryExt := res.Types[1]
	if queryExt.Name != "Query" || !queryExt.IsExtension {
		t.Errorf("Query extension mismatch: %+v", queryExt)
	}
	if len(res.Operations) != 1 || res.Operations[0].Name != "topProducts" {
		t.Errorf("Expected topProducts operation, got %+v", res.Operations)
	}
}
