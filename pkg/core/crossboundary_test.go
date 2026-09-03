package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCrossBoundaryLinker_FullPipeline(t *testing.T) {
	lineage := LineageEnvelope{
		UserID:     "user-1",
		UserPrompt: "Build cloud service with UI and Terraform",
		Timestamp:  time.Now().UTC(),
	}

	// 1. Setup Go Route Node
	goRoutePayload, _ := json.Marshal(map[string]interface{}{
		"method":       "GET",
		"path":         "/api/v1/users/{id}",
		"handler_name": "HandleGetUser",
	})
	goRouteNode := &ASTSymbolNode{
		Language:   LangGo,
		NodeType:   "RouteBinding",
		Identifier: "service:GET /api/v1/users/{id}",
		ASTPayload: goRoutePayload,
		Lineage:    lineage,
	}
	goRouteNode.NodeID, _ = HashASTSymbolNode(goRouteNode)

	// 2. Setup Go Function with Env Access
	goFuncPayload, _ := json.Marshal(map[string]interface{}{
		"name":              "NewServer",
		"env_vars_accessed": []string{"PORT", "DATABASE_URL"},
	})
	goFuncNode := &ASTSymbolNode{
		Language:   LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "service.NewServer",
		ASTPayload: goFuncPayload,
		Lineage:    lineage,
	}
	goFuncNode.NodeID, _ = HashASTSymbolNode(goFuncNode)

	// 3. Setup Go Component Node
	goComp := &ComponentNode{
		Name:        "auth-service",
		Type:        CompService,
		Language:    LangGo,
		SymbolNodes: []string{goRouteNode.NodeID, goFuncNode.NodeID},
		Lineage:     lineage,
	}
	goComp.ComponentID, _ = HashComponentNode(goComp)

	// 4. Setup TypeScript API Call Node
	tsCallPayload, _ := json.Marshal(map[string]interface{}{
		"method": "GET",
		"url":    "/api/v1/users/:id",
		"caller": "fetch",
	})
	tsCallNode := &ASTSymbolNode{
		Language:   LangTypeScript,
		NodeType:   "ApiClientCall",
		Identifier: "UserCard.tsx:GET /api/v1/users/:id",
		ASTPayload: tsCallPayload,
		Lineage:    lineage,
	}
	tsCallNode.NodeID, _ = HashASTSymbolNode(tsCallNode)

	// 5. Setup HCL Cloud Run Resource Node
	hclPayload, _ := json.Marshal(map[string]interface{}{
		"block_type": "resource",
		"labels":     []string{"google_cloud_run_service", "auth_service"},
		"attributes": map[string]interface{}{
			"image": map[string]interface{}{"value": `"gcr.io/app/auth-service:v1"`},
		},
		"nested_blocks": []interface{}{
			map[string]interface{}{
				"block_type": "env",
				"attributes": map[string]interface{}{
					"name":  map[string]interface{}{"value": `"PORT"`},
					"value": map[string]interface{}{"value": `"8080"`},
				},
			},
			map[string]interface{}{
				"block_type": "env",
				"attributes": map[string]interface{}{
					"name":  map[string]interface{}{"value": `"DATABASE_URL"`},
					"value": map[string]interface{}{"value": `"postgres://localhost/db"`},
				},
			},
		},
	})
	hclResNode := &ASTSymbolNode{
		Language:   LangHCL,
		NodeType:   "ResourceBlock",
		Identifier: "resource.google_cloud_run_service.auth_service",
		ASTPayload: hclPayload,
		Lineage:    lineage,
	}
	hclResNode.NodeID, _ = HashASTSymbolNode(hclResNode)

	// Build symbol map and component slice
	symbolNodes := map[string]*ASTSymbolNode{
		goRouteNode.NodeID: goRouteNode,
		goFuncNode.NodeID:  goFuncNode,
		tsCallNode.NodeID:  tsCallNode,
		hclResNode.NodeID:  hclResNode,
	}
	components := []*ComponentNode{goComp}

	// Link workspace
	linker := NewCrossBoundaryLinker()
	edges, err := linker.LinkWorkspace(components, symbolNodes)
	if err != nil {
		t.Fatalf("LinkWorkspace failed: %v", err)
	}

	if len(edges) < 3 {
		t.Fatalf("expected at least 3 cross-boundary edges (CONSUMES_API, DEPLOYS_TO, BINDS_ENV), got %d", len(edges))
	}

	edgeTypeMap := make(map[EdgeType][]CrossBoundaryEdge)
	for _, e := range edges {
		edgeTypeMap[e.Type] = append(edgeTypeMap[e.Type], e)
	}

	// Check CONSUMES_API
	if len(edgeTypeMap[EdgeConsumesAPI]) != 1 {
		t.Errorf("expected 1 CONSUMES_API edge, got %d", len(edgeTypeMap[EdgeConsumesAPI]))
	} else {
		e := edgeTypeMap[EdgeConsumesAPI][0]
		if e.SourceNodeID != tsCallNode.NodeID || e.TargetNodeID != goRouteNode.NodeID {
			t.Errorf("CONSUMES_API edge endpoints mismatch: %v", e)
		}
	}

	// Check DEPLOYS_TO
	if len(edgeTypeMap[EdgeDeploysTo]) != 1 {
		t.Errorf("expected 1 DEPLOYS_TO edge, got %d", len(edgeTypeMap[EdgeDeploysTo]))
	} else {
		e := edgeTypeMap[EdgeDeploysTo][0]
		if e.SourceNodeID != goComp.ComponentID || e.TargetNodeID != hclResNode.NodeID {
			t.Errorf("DEPLOYS_TO edge endpoints mismatch: %v", e)
		}
	}

	// Check BINDS_ENV
	if len(edgeTypeMap[EdgeBindsEnv]) != 2 {
		t.Errorf("expected 2 BINDS_ENV edges (PORT and DATABASE_URL), got %d", len(edgeTypeMap[EdgeBindsEnv]))
	}

	// 6. Test Contract Breakage Detection
	// Simulate mutation where Go route is removed and PORT env var is removed from HCL
	newSymbolNodes := map[string]*ASTSymbolNode{
		goFuncNode.NodeID: goFuncNode,
		tsCallNode.NodeID: tsCallNode,
	}
	var newEdges []CrossBoundaryEdge // all edges removed

	breakages := DetectContractBreakages(edges, newEdges, symbolNodes, newSymbolNodes)
	if len(breakages) == 0 {
		t.Fatalf("expected contract breakages to be detected")
	}

	hasRouteBreakage := false
	hasEnvBreakage := false
	for _, b := range breakages {
		if b.Type == BreakageRouteRemoved {
			hasRouteBreakage = true
		}
		if b.Type == BreakageEnvVarMissing {
			hasEnvBreakage = true
		}
	}

	if !hasRouteBreakage {
		t.Errorf("expected BreakageRouteRemoved to be detected")
	}
	if !hasEnvBreakage {
		t.Errorf("expected BreakageEnvVarMissing to be detected")
	}
}

func TestCrossBoundaryLinker_PythonFastAPIAndTerraform(t *testing.T) {
	lineage := LineageEnvelope{
		UserID:           "agent-py",
		ExecutingAgentID: "py-agent-01",
		Timestamp:        time.Now().UTC(),
	}

	pyRoute := &ASTSymbolNode{
		Language:   LangPython,
		NodeType:   "RouteBinding",
		Identifier: "api.py:GET /api/v1/items/{item_id}",
		ASTPayload: []byte("@app.get('/api/v1/items/{item_id}')"),
		Lineage:    lineage,
	}
	pyRoute.NodeID, _ = HashASTSymbolNode(pyRoute)

	tsCall := &ASTSymbolNode{
		Language:   LangTypeScript,
		NodeType:   "ApiClientCall",
		Identifier: "ItemView.tsx:GET /api/v1/items/:item_id",
		ASTPayload: []byte("fetch('/api/v1/items/' + id)"),
		Lineage:    lineage,
	}
	tsCall.NodeID, _ = HashASTSymbolNode(tsCall)

	symbolNodes := map[string]*ASTSymbolNode{
		pyRoute.NodeID: pyRoute,
		tsCall.NodeID:  tsCall,
	}

	linker := NewCrossBoundaryLinker()
	edges, err := linker.LinkWorkspace(nil, symbolNodes)
	if err != nil {
		t.Fatalf("LinkWorkspace failed: %v", err)
	}

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].Type != EdgeConsumesAPI || edges[0].SourceNodeID != tsCall.NodeID || edges[0].TargetNodeID != pyRoute.NodeID {
		t.Errorf("Unexpected edge: %+v", edges[0])
	}
}

func TestCrossBoundaryLinker_PolyglotDBAndRPC(t *testing.T) {
	lineage := LineageEnvelope{
		UserID:    "polyglot-architect",
		Timestamp: time.Now().UTC(),
	}

	// 1. SQL Table
	sqlTablePayload, _ := json.Marshal(map[string]interface{}{
		"name": "users",
	})
	sqlTableNode := &ASTSymbolNode{
		Language:    LangSQL,
		NodeType:    "CreateTableStatement",
		Identifier:  "table:public.users",
		ASTPayload:  sqlTablePayload,
		ASTMetadata: map[string]string{"table_name": "users"},
		Lineage:     lineage,
	}
	sqlTableNode.NodeID, _ = HashASTSymbolNode(sqlTableNode)

	// 2. Java JPA Entity querying users table
	javaEntityPayload, _ := json.Marshal(map[string]interface{}{
		"name":       "UserEntity",
		"table_name": "users",
	})
	javaEntityNode := &ASTSymbolNode{
		Language:    LangJava,
		NodeType:    "ClassOrInterfaceDeclaration",
		Identifier:  "com.example.UserEntity",
		ASTPayload:  javaEntityPayload,
		ASTMetadata: map[string]string{"table_name": "users", "is_entity": "true"},
		Lineage:     lineage,
	}
	javaEntityNode.NodeID, _ = HashASTSymbolNode(javaEntityNode)

	// 3. Protobuf RPC Method
	protoRpcPayload, _ := json.Marshal(map[string]interface{}{
		"name":        "GetUser",
		"input_type":  "GetUserRequest",
		"output_type": "User",
	})
	protoRpcNode := &ASTSymbolNode{
		Language:    LangProtobuf,
		NodeType:    "RPCMethod",
		Identifier:  "rpc:user.v1.UserService.GetUser",
		ASTPayload:  protoRpcPayload,
		ASTMetadata: map[string]string{"service": "user.v1.UserService"},
		Lineage:     lineage,
	}
	protoRpcNode.NodeID, _ = HashASTSymbolNode(protoRpcNode)

	// 4. Rust gRPC Handler implementing GetUser
	rustHandlerPayload, _ := json.Marshal(map[string]interface{}{
		"name": "get_user",
	})
	rustHandlerNode := &ASTSymbolNode{
		Language:    LangRust,
		NodeType:    "FnDef",
		Identifier:  "UserServiceServer::get_user",
		ASTPayload:  rustHandlerPayload,
		ASTMetadata: map[string]string{"implements_rpc": "GetUser"},
		Lineage:     lineage,
	}
	rustHandlerNode.NodeID, _ = HashASTSymbolNode(rustHandlerNode)

	symbolNodes := map[string]*ASTSymbolNode{
		sqlTableNode.NodeID:    sqlTableNode,
		javaEntityNode.NodeID:  javaEntityNode,
		protoRpcNode.NodeID:    protoRpcNode,
		rustHandlerNode.NodeID: rustHandlerNode,
	}

	linker := NewCrossBoundaryLinker()
	edges, err := linker.LinkWorkspace(nil, symbolNodes)
	if err != nil {
		t.Fatalf("LinkWorkspace failed: %v", err)
	}

	if len(edges) != 2 {
		t.Fatalf("Expected 2 edges (QUERIES_TABLE and IMPLEMENTS_RPC), got %d: %+v", len(edges), edges)
	}

	hasQueriesTable := false
	hasImplementsRPC := false
	for _, e := range edges {
		if e.Type == EdgeQueriesTable && e.SourceNodeID == javaEntityNode.NodeID && e.TargetNodeID == sqlTableNode.NodeID {
			hasQueriesTable = true
		}
		if e.Type == EdgeImplementsRPC && e.SourceNodeID == rustHandlerNode.NodeID && e.TargetNodeID == protoRpcNode.NodeID {
			hasImplementsRPC = true
		}
	}

	if !hasQueriesTable {
		t.Errorf("Expected EdgeQueriesTable from Java entity to SQL table")
	}
	if !hasImplementsRPC {
		t.Errorf("Expected EdgeImplementsRPC from Rust handler to Proto RPC")
	}
}
