package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func setupTestLSPWorkspace(t *testing.T) (string, *storage.BlobStore, *storage.GraphEngine) {
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

	// 1. Create Go backend file: services/billing/main.go
	billingDir := filepath.Join(tempDir, "services", "billing")
	_ = os.MkdirAll(billingDir, 0755)
	billingFile := filepath.Join(billingDir, "main.go")
	billingCode := `package main

// ProcessPayment handles charges
func ProcessPayment(amount int) bool {
	return amount > 0
}
`
	_ = os.WriteFile(billingFile, []byte(billingCode), 0644)

	parsedBackend, err := codecs.ParseSourceFile("services/billing/main.go", []byte(billingCode), core.LineageEnvelope{
		ExecutingAgentID: "backend-agent",
		Intent:           "Add ProcessPayment handler",
		Timestamp:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to parse backend: %v", err)
	}

	for _, s := range parsedBackend.Symbols {
		sData, _ := json.Marshal(s)
		sHash, _ := blobStore.Put(sData)
		_ = graphEngine.PutNode(storage.NodeRecord{
			NodeID:     s.NodeID,
			Language:   s.Language,
			NodeType:   s.NodeType,
			MerkleHash: sHash,
			CreatedAt:  time.Now(),
		})
		_ = graphEngine.PutLineage(storage.LineageRecord{
			RecordID:         s.NodeID + "-lin",
			NodeID:           s.NodeID,
			ExecutingAgentID: "backend-agent",
			Intent:           "Add ProcessPayment handler",
			Timestamp:        time.Now().UTC(),
		})
	}

	cData, _ := json.Marshal(parsedBackend.Component)
	cHash, _ := blobStore.Put(cData)
	_ = graphEngine.PutNode(storage.NodeRecord{
		NodeID:     parsedBackend.Component.ComponentID,
		Language:   parsedBackend.Component.Language,
		NodeType:   string(parsedBackend.Component.Type),
		MerkleHash: cHash,
		CreatedAt:  time.Now(),
	})

	// 2. Create Frontend file: frontend/api.ts
	feDir := filepath.Join(tempDir, "frontend")
	_ = os.MkdirAll(feDir, 0755)
	feFile := filepath.Join(feDir, "api.ts")
	feCode := `export const checkout = async (amt: number) => {
	return fetch("/api/billing/charge", { method: "POST" });
};
`
	_ = os.WriteFile(feFile, []byte(feCode), 0644)

	parsedFE, err := codecs.ParseSourceFile("frontend/api.ts", []byte(feCode), core.LineageEnvelope{
		ExecutingAgentID: "frontend-agent",
		Intent:           "Checkout frontend call",
		Timestamp:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to parse frontend: %v", err)
	}

	for _, s := range parsedFE.Symbols {
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

	feCData, _ := json.Marshal(parsedFE.Component)
	feCHash, _ := blobStore.Put(feCData)
	_ = graphEngine.PutNode(storage.NodeRecord{
		NodeID:     parsedFE.Component.ComponentID,
		Language:   parsedFE.Component.Language,
		NodeType:   string(parsedFE.Component.Type),
		MerkleHash: feCHash,
		CreatedAt:  time.Now(),
	})

	// Add Cross-boundary edge connecting frontend to backend
	var feSymID, beSymID string
	for _, s := range parsedFE.Symbols {
		feSymID = s.NodeID
		break
	}
	for _, s := range parsedBackend.Symbols {
		beSymID = s.NodeID
		break
	}

	crossEdge := core.CrossBoundaryEdge{
		SourceNodeID: feSymID,
		TargetNodeID: beSymID,
		Type:         core.EdgeConsumesAPI,
		Metadata: map[string]string{
			"route":  "/api/billing/charge",
			"method": "POST",
		},
	}

	_ = graphEngine.PutEdge(storage.EdgeRecord{
		SourceID: feSymID,
		TargetID: beSymID,
		EdgeType: core.EdgeConsumesAPI,
	})

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "test-workspace",
		UniverseID:  "universe-main",
		Components:  []string{parsedBackend.Component.ComponentID, parsedFE.Component.ComponentID},
		CrossEdges:  []core.CrossBoundaryEdge{crossEdge},
		CreatedAt:   time.Now().UTC(),
	}

	_, _ = universeMgr.CommitManifest("universe-main", manifest)
	return tempDir, blobStore, graphEngine
}

func sendLSPRequest(inBuf *bytes.Buffer, req lspRequest) {
	data, _ := json.Marshal(req)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	inBuf.WriteString(header)
	inBuf.Write(data)
}

func readLSPResponses(outBuf *bytes.Buffer) []map[string]interface{} {
	var responses []map[string]interface{}
	reader := bytes.NewReader(outBuf.Bytes())
	bufReader := &bytes.Buffer{}
	_, _ = io.Copy(bufReader, reader)
	allBytes := bufReader.Bytes()

	for len(allBytes) > 0 {
		clIdx := bytes.Index(allBytes, []byte("Content-Length: "))
		if clIdx == -1 {
			break
		}
		bodyStart := bytes.Index(allBytes[clIdx:], []byte("\r\n\r\n"))
		if bodyStart == -1 {
			break
		}
		headerEnd := clIdx + bodyStart + 4
		header := string(allBytes[clIdx : clIdx+bodyStart])
		var cl int
		_, _ = fmt.Sscanf(header, "Content-Length: %d", &cl)
		if cl <= 0 || headerEnd+cl > len(allBytes) {
			break
		}
		body := allBytes[headerEnd : headerEnd+cl]
		var item map[string]interface{}
		if err := json.Unmarshal(body, &item); err == nil {
			responses = append(responses, item)
		}
		allBytes = allBytes[headerEnd+cl:]
	}
	return responses
}

func TestLSP_Initialize(t *testing.T) {
	tempDir, blobStore, graphEngine := setupTestLSPWorkspace(t)
	inBuf := &bytes.Buffer{}
	outBuf := &bytes.Buffer{}

	server := NewServerWithStorage(tempDir, blobStore, graphEngine, inBuf, outBuf)

	sendLSPRequest(inBuf, lspRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{}`),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = server.Run(ctx)

	responses := readLSPResponses(outBuf)
	if len(responses) == 0 {
		t.Fatalf("no response received from LSP server")
	}

	initResp := responses[0]
	result := initResp["result"].(map[string]interface{})
	caps := result["capabilities"].(map[string]interface{})

	if defProv, ok := caps["definitionProvider"].(bool); !ok || !defProv {
		t.Errorf("expected definitionProvider: true, got %v", caps["definitionProvider"])
	}

	if _, ok := caps["codeLensProvider"]; !ok {
		t.Errorf("expected codeLensProvider in capabilities")
	}
}

func TestLSP_CodeLens(t *testing.T) {
	tempDir, blobStore, graphEngine := setupTestLSPWorkspace(t)
	inBuf := &bytes.Buffer{}
	outBuf := &bytes.Buffer{}

	server := NewServerWithStorage(tempDir, blobStore, graphEngine, inBuf, outBuf)

	uri := fmt.Sprintf("file://%s/services/billing/main.go", tempDir)
	params := fmt.Sprintf(`{"textDocument": {"uri": "%s"}}`, uri)

	sendLSPRequest(inBuf, lspRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "textDocument/codeLens",
		Params:  json.RawMessage(params),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = server.Run(ctx)

	responses := readLSPResponses(outBuf)
	if len(responses) == 0 {
		t.Fatalf("no response received for codeLens")
	}

	res := responses[0]["result"].([]interface{})
	if len(res) == 0 {
		t.Fatalf("expected at least 1 CodeLens for services/billing/main.go")
	}

	lens := res[0].(map[string]interface{})
	cmd := lens["command"].(map[string]interface{})
	title := cmd["title"].(string)

	if !strings.Contains(title, "Cosm Lineage") || !strings.Contains(title, "backend-agent") {
		t.Errorf("expected Cosm Lineage with backend-agent in CodeLens title, got: %s", title)
	}
}

func TestLSP_Definition_CrossLanguage(t *testing.T) {
	tempDir, blobStore, graphEngine := setupTestLSPWorkspace(t)
	inBuf := &bytes.Buffer{}
	outBuf := &bytes.Buffer{}

	server := NewServerWithStorage(tempDir, blobStore, graphEngine, inBuf, outBuf)

	uri := fmt.Sprintf("file://%s/frontend/api.ts", tempDir)
	params := fmt.Sprintf(`{
		"textDocument": {"uri": "%s"},
		"position": {"line": 0, "character": 5}
	}`, uri)

	sendLSPRequest(inBuf, lspRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "textDocument/definition",
		Params:  json.RawMessage(params),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = server.Run(ctx)

	responses := readLSPResponses(outBuf)
	if len(responses) == 0 {
		t.Fatalf("no response received for definition")
	}

	res, ok := responses[0]["result"].(map[string]interface{})
	if !ok || res == nil {
		t.Fatalf("definition result is nil: %v", responses[0])
	}

	targetURI := res["uri"].(string)
	if !strings.Contains(targetURI, "services/billing/main.go") {
		t.Errorf("expected jump to services/billing/main.go, got: %s", targetURI)
	}
}

func TestLSP_DidSave_Diagnostics(t *testing.T) {
	tempDir, blobStore, graphEngine := setupTestLSPWorkspace(t)
	inBuf := &bytes.Buffer{}
	outBuf := &bytes.Buffer{}

	server := NewServerWithStorage(tempDir, blobStore, graphEngine, inBuf, outBuf)

	uri := fmt.Sprintf("file://%s/services/billing/main.go", tempDir)
	// Delete the ProcessPayment function so the contract breaks!
	brokenCode := `package main

// Empty file breaking ProcessPayment endpoint
`
	params := fmt.Sprintf(`{
		"textDocument": {"uri": "%s"},
		"text": %q
	}`, uri, brokenCode)

	sendLSPRequest(inBuf, lspRequest{
		JSONRPC: "2.0",
		Method:  "textDocument/didSave",
		Params:  json.RawMessage(params),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = server.Run(ctx)

	responses := readLSPResponses(outBuf)
	if len(responses) == 0 {
		t.Fatalf("no notification received for didSave")
	}

	diagNotif := responses[0]
	if diagNotif["method"] != "textDocument/publishDiagnostics" {
		t.Errorf("expected method textDocument/publishDiagnostics, got: %v", diagNotif["method"])
	}

	notifParams := diagNotif["params"].(map[string]interface{})
	diagnostics := notifParams["diagnostics"].([]interface{})
	if len(diagnostics) == 0 {
		t.Fatalf("expected contract breakage diagnostic after removing endpoint")
	}

	d := diagnostics[0].(map[string]interface{})
	msg := d["message"].(string)
	if !strings.Contains(msg, "removed or altered") {
		t.Errorf("expected contract breakage message, got: %s", msg)
	}
}
