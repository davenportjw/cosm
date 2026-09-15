package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func setupTestWorkspace(t *testing.T) (string, *storage.BlobStore, *storage.GraphEngine) {
	tempDir := t.TempDir()
	cosmDir := filepath.Join(tempDir, ".cosm")
	if err := os.MkdirAll(filepath.Join(cosmDir, "objects"), 0755); err != nil {
		t.Fatalf("failed to create cosm objects dir: %v", err)
	}

	blobStore, err := storage.NewBlobStore(filepath.Join(cosmDir, "objects"))
	if err != nil {
		t.Fatalf("failed to create blob store: %v", err)
	}

	graphEngine, err := storage.NewGraphEngine(filepath.Join(cosmDir, "graph.db"))
	if err != nil {
		t.Fatalf("failed to create graph engine: %v", err)
	}

	universeMgr := storage.NewUniverseManager(graphEngine, blobStore)
	_, err = universeMgr.CreateUniverse("universe-main", "")
	if err != nil {
		t.Fatalf("failed to create universe-main: %v", err)
	}

	// Create sample Go file and stage it
	goFile := filepath.Join(tempDir, "calculator.go")
	initialCode := `package main

// Add returns sum of two integers
func Add(a int, b int) int {
	return a + b
}
`
	if err := os.WriteFile(goFile, []byte(initialCode), 0644); err != nil {
		t.Fatalf("failed to write calculator.go: %v", err)
	}

	parsed, err := codecs.ParseSourceFile("calculator.go", []byte(initialCode), core.LineageEnvelope{
		ExecutingAgentID: "setup-agent",
		Intent:           "Initial calculator code",
		Timestamp:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to parse calculator.go: %v", err)
	}

	comp := parsed.Component
	for _, s := range parsed.Symbols {
		sData, _ := json.Marshal(s)
		sHash, _ := blobStore.Put(sData)
		_ = graphEngine.PutNode(storage.NodeRecord{
			NodeID:     s.NodeID,
			Language:   s.Language,
			NodeType:   s.NodeType,
			MerkleHash: sHash,
			CreatedAt:  time.Now(),
		})
	}

	cData, _ := json.Marshal(comp)
	cHash, _ := blobStore.Put(cData)
	_ = graphEngine.PutNode(storage.NodeRecord{
		NodeID:     comp.ComponentID,
		Language:   comp.Language,
		NodeType:   string(comp.Type),
		MerkleHash: cHash,
		CreatedAt:  time.Now(),
	})

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "test-workspace",
		UniverseID:  "universe-main",
		Components:  []string{comp.ComponentID},
		Lineage: core.LineageEnvelope{
			ExecutingAgentID: "setup-agent",
			Intent:           "Initial staging",
			Timestamp:        time.Now().UTC(),
		},
		CreatedAt: time.Now().UTC(),
	}
	mData, _ := json.Marshal(manifest)
	mHash, _ := blobStore.Put(mData)
	manifest.MerkleRootHash = mHash
	_, _ = universeMgr.CommitManifest("universe-main", manifest)

	return tempDir, blobStore, graphEngine
}

func executeMCPRequest(t *testing.T, s *Server, req jsonRPCRequest) *jsonRPCResponse {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshaling request: %v", err)
	}
	reqBytes = append(reqBytes, '\n')

	inBuf := bytes.NewReader(reqBytes)
	outBuf := &bytes.Buffer{}

	s.in = inBuf
	s.out = outBuf

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = s.Run(ctx)

	var resp jsonRPCResponse
	if err := json.Unmarshal(bytes.TrimSpace(outBuf.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshaling response: %v, raw: %s", err, outBuf.String())
	}
	return &resp
}

func TestMCP_Initialize(t *testing.T) {
	tempDir, blobStore, graphEngine := setupTestWorkspace(t)
	s := NewServerWithStorage(tempDir, blobStore, graphEngine, nil, nil)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{}`),
	}

	resp := executeMCPRequest(t, s, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	resMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not map: %v", resp.Result)
	}

	serverInfo, ok := resMap["serverInfo"].(map[string]interface{})
	if !ok || serverInfo["name"] != "cosm-mcp" {
		t.Errorf("expected server name cosm-mcp, got %v", serverInfo)
	}
}

func TestMCP_ToolsList(t *testing.T) {
	tempDir, blobStore, graphEngine := setupTestWorkspace(t)
	s := NewServerWithStorage(tempDir, blobStore, graphEngine, nil, nil)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	resp := executeMCPRequest(t, s, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	resMap := resp.Result.(map[string]interface{})
	toolsList, ok := resMap["tools"].([]interface{})
	if !ok || len(toolsList) != 8 {
		t.Fatalf("expected 8 tools, got %d", len(toolsList))
	}

	expectedTools := map[string]bool{
		"cosm_status":          false,
		"cosm_ast_resolve":     false,
		"cosm_ast_edit":        false,
		"cosm_blast_radius":    false,
		"cosm_topology":        false,
		"cosm_universe_create": false,
		"cosm_commit":          false,
		"cosm_ship":            false,
	}

	for _, toolItem := range toolsList {
		tm := toolItem.(map[string]interface{})
		name := tm["name"].(string)
		if _, exists := expectedTools[name]; exists {
			expectedTools[name] = true
		}
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("tool %s not registered", name)
		}
	}
}

func TestMCP_ToolsCall_Status(t *testing.T) {
	tempDir, blobStore, graphEngine := setupTestWorkspace(t)
	s := NewServerWithStorage(tempDir, blobStore, graphEngine, nil, nil)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "cosm_status", "arguments": {"universe": "universe-main"}}`),
	}

	resp := executeMCPRequest(t, s, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	resMap := resp.Result.(map[string]interface{})
	content := resMap["content"].([]interface{})
	if len(content) == 0 {
		t.Fatalf("empty content in tool response")
	}

	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "universe-main") {
		t.Errorf("status output did not contain universe-main: %s", text)
	}
	if !strings.Contains(text, "tracked_components") {
		t.Errorf("status output did not contain tracked_components: %s", text)
	}
}

func TestMCP_ToolsCall_ASTEdit_WithDiskSync(t *testing.T) {
	tempDir, blobStore, graphEngine := setupTestWorkspace(t)
	s := NewServerWithStorage(tempDir, blobStore, graphEngine, nil, nil)

	// Mutate Add function body
	editArgs := map[string]interface{}{
		"op":         "replace_function_body",
		"target":     "Add",
		"content":    "\treturn a + b + 42\n",
		"universe":   "universe-main",
		"write_disk": true,
		"prompt":     "Add 42 bonus",
	}
	editArgsBytes, _ := json.Marshal(editArgs)
	callParams := fmt.Sprintf(`{"name": "cosm_ast_edit", "arguments": %s}`, string(editArgsBytes))

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  json.RawMessage(callParams),
	}

	resp := executeMCPRequest(t, s, req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	resMap := resp.Result.(map[string]interface{})
	if isErr, _ := resMap["isError"].(bool); isErr {
		t.Fatalf("tool call returned isError=true: %v", resMap)
	}

	content := resMap["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "applied_operations") {
		t.Errorf("expected applied_operations in text: %s", text)
	}

	// Verify that calculator.go on disk was automatically synchronized
	diskFile := filepath.Join(tempDir, "calculator.go")
	diskBytes, err := os.ReadFile(diskFile)
	if err != nil {
		t.Fatalf("failed to read disk file after edit: %v", err)
	}

	if !strings.Contains(string(diskBytes), "42") {
		t.Errorf("disk file did not synchronize modified function body: %s", string(diskBytes))
	}
}

func TestMCP_ToolsCall_AllRemainingTools(t *testing.T) {
	tempDir, blobStore, graphEngine := setupTestWorkspace(t)
	s := NewServerWithStorage(tempDir, blobStore, graphEngine, nil, nil)

	// 1. cosm_ast_resolve
	{
		req := jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      10,
			Method:  "tools/call",
			Params:  json.RawMessage(`{"name": "cosm_ast_resolve", "arguments": {"target": "Add", "universe": "universe-main"}}`),
		}
		resp := executeMCPRequest(t, s, req)
		if resp.Error != nil {
			t.Fatalf("resolve failed: %v", resp.Error)
		}
		resMap := resp.Result.(map[string]interface{})
		content := resMap["content"].([]interface{})
		text := content[0].(map[string]interface{})["text"].(string)
		if !strings.Contains(text, "Add") {
			t.Errorf("expected Add in resolve text: %s", text)
		}
	}

	// 2. cosm_universe_create
	{
		req := jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      11,
			Method:  "tools/call",
			Params:  json.RawMessage(`{"name": "cosm_universe_create", "arguments": {"universe_id": "universe-feature-1", "parent_universe_id": "universe-main"}}`),
		}
		resp := executeMCPRequest(t, s, req)
		if resp.Error != nil {
			t.Fatalf("universe create failed: %v", resp.Error)
		}
		resMap := resp.Result.(map[string]interface{})
		content := resMap["content"].([]interface{})
		text := content[0].(map[string]interface{})["text"].(string)
		if !strings.Contains(text, "universe-feature-1") {
			t.Errorf("expected universe-feature-1 in text: %s", text)
		}
	}

	// 3. cosm_commit
	{
		req := jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      12,
			Method:  "tools/call",
			Params:  json.RawMessage(`{"name": "cosm_commit", "arguments": {"universe": "universe-main", "intent": "MCP commit test", "prompt": "test prompt"}}`),
		}
		resp := executeMCPRequest(t, s, req)
		if resp.Error != nil {
			t.Fatalf("commit failed: %v", resp.Error)
		}
		resMap := resp.Result.(map[string]interface{})
		content := resMap["content"].([]interface{})
		text := content[0].(map[string]interface{})["text"].(string)
		if !strings.Contains(text, "merkle_root") {
			t.Errorf("expected merkle_root in commit text: %s", text)
		}
	}

	// 4. cosm_blast_radius
	{
		req := jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      13,
			Method:  "tools/call",
			Params:  json.RawMessage(`{"name": "cosm_blast_radius", "arguments": {"target": "setup-agent"}}`),
		}
		resp := executeMCPRequest(t, s, req)
		if resp.Error != nil {
			t.Fatalf("blast radius failed: %v", resp.Error)
		}
		resMap := resp.Result.(map[string]interface{})
		content := resMap["content"].([]interface{})
		text := content[0].(map[string]interface{})["text"].(string)
		if !strings.Contains(text, "risk_score") {
			t.Errorf("expected risk_score in text: %s", text)
		}
	}

	// 5. cosm_topology
	{
		req := jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      14,
			Method:  "tools/call",
			Params:  json.RawMessage(`{"name": "cosm_topology", "arguments": {"universe": "universe-main", "format": "ascii"}}`),
		}
		resp := executeMCPRequest(t, s, req)
		if resp.Error != nil {
			t.Fatalf("topology failed: %v", resp.Error)
		}
	}

	// 6. cosm_ship
	{
		req := jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      15,
			Method:  "tools/call",
			Params:  json.RawMessage(`{"name": "cosm_ship", "arguments": {"universe": "universe-main", "target": "cosm"}}`),
		}
		resp := executeMCPRequest(t, s, req)
		if resp.Error != nil {
			t.Fatalf("ship failed: %v", resp.Error)
		}
		resMap := resp.Result.(map[string]interface{})
		content := resMap["content"].([]interface{})
		text := content[0].(map[string]interface{})["text"].(string)
		if !strings.Contains(text, "READY_FOR_PREVIEW") {
			t.Errorf("expected READY_FOR_PREVIEW in ship text: %s", text)
		}
	}
}

