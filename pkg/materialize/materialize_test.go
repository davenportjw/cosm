package materialize

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs/golang"
	"github.com/cosmscm/cosm/pkg/codecs/hcl"
	"github.com/cosmscm/cosm/pkg/codecs/typescript"
	"github.com/cosmscm/cosm/pkg/core"
)

func TestHydrator_GoHCLAndTypeScript(t *testing.T) {
	hydrator := NewHydrator()

	lineage := core.LineageEnvelope{
		UserID:              "user_dev_01",
		UserPrompt:          "Create auth service and cloud run infra",
		SessionID:           "sess_123",
		OrchestratorAgentID: "agent_orchestrator",
		ExecutingAgentID:    "agent_coder",
		LLMVersion:          "claude-3-7-sonnet",
		Intent:              "Generate service components",
		Timestamp:           time.Now().UTC(),
	}

	// 1. Go Symbol Hydration
	goSrc := `package auth
type User struct {
	ID string ` + "`json:\"id\"`" + `
	Email string ` + "`json:\"email\"`" + `
}

type AuthService interface {
	Login(ctx context.Context, email string) (string, error)
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}
`
	goParser := golang.NewGoParser()
	goRes, err := goParser.ParseSource("auth/main.go", []byte(goSrc), lineage)
	if err != nil {
		t.Fatalf("failed to parse Go: %v", err)
	}

	for _, sym := range goRes.AllSymbols {
		hydrated, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("failed to hydrate Go symbol %s: %v", sym.Identifier, err)
		}
		if hydrated == "" {
			t.Fatalf("hydrated Go code is empty for %s", sym.Identifier)
		}
	}

	// 2. HCL Symbol Hydration
	hclSrc := `resource "google_cloud_run_service" "auth_api" {
  name     = "auth-service"
  location = "us-central1"
  template {
    spec {
      containers {
        image = "gcr.io/my-project/auth:v1"
      }
    }
  }
}
`
	hclParser := hcl.NewHCLParser()
	hclDoc, err := hclParser.ParseSource("infra/main.tf", []byte(hclSrc))
	if err != nil {
		t.Fatalf("failed to parse HCL: %v", err)
	}

	hclSymbols, err := hclParser.ToASTSymbolNodes(hclDoc, lineage)
	if err != nil {
		t.Fatalf("failed to convert HCL: %v", err)
	}

	for _, sym := range hclSymbols {
		hydrated, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("failed to hydrate HCL symbol %s: %v", sym.Identifier, err)
		}
		if !strings.Contains(hydrated, "google_cloud_run_service") {
			t.Fatalf("hydrated HCL missing resource: %s", hydrated)
		}
	}

	// 3. TypeScript Symbol Hydration
	tsSrc := `import React, { useState } from 'react';

export interface UserProfile {
	id: string;
	name: string;
}

export const AuthView: React.FC = () => {
	const [user, setUser] = useState(null);
	return (
		<div>
			<h1>Auth View</h1>
		</div>
	);
};
`
	tsParser := typescript.NewTSParser()
	tsRes, err := tsParser.ParseSource("src/AuthView.tsx", []byte(tsSrc), lineage)
	if err != nil {
		t.Fatalf("failed to parse TS: %v", err)
	}

	for _, sym := range tsRes.AllSymbols {
		hydrated, err := hydrator.HydrateSymbol(sym)
		if err != nil {
			t.Fatalf("failed to hydrate TS symbol %s: %v", sym.Identifier, err)
		}
		if hydrated == "" {
			t.Fatalf("hydrated TS is empty for %s", sym.Identifier)
		}
	}
}

func TestHydrator_ComponentAndWorkspace(t *testing.T) {
	hydrator := NewHydrator()
	lineage := core.LineageEnvelope{
		UserPrompt: "Build full cloud project",
		Timestamp:  time.Now().UTC(),
	}

	goParser := golang.NewGoParser()
	goRes, _ := goParser.ParseSource("server/main.go", []byte("package main\nfunc Run() {}"), lineage)
	compGo, _ := golang.BuildComponentNode("server", core.CompService, goRes, lineage)

	hclParser := hcl.NewHCLParser()
	hclDoc, _ := hclParser.ParseSource("infra/main.tf", []byte("resource \"google_storage_bucket\" \"data\" { name = \"my-data\" }"))
	compHCL, hclSyms, _ := hclParser.BuildComponentNode(hclDoc, "infra", lineage)

	symbolMap := make(map[string]*core.ASTSymbolNode)
	for _, s := range goRes.AllSymbols {
		symbolMap[s.NodeID] = s
	}
	for _, s := range hclSyms {
		symbolMap[s.NodeID] = s
	}

	compMap := map[string]*core.ComponentNode{
		compGo.ComponentID:  compGo,
		compHCL.ComponentID: compHCL,
	}

	rawSym := &core.ASTSymbolNode{
		NodeID:     "raw:lic",
		Language:   core.LangRaw,
		NodeType:   "RawBlobNode",
		Identifier: "LICENSE",
		ASTPayload: []byte("MIT License\nCopyright (c) 2026"),
		Lineage:    lineage,
	}
	compRaw := &core.ComponentNode{
		ComponentID: "comp-raw-lic",
		Name:        "LICENSE",
		Type:        core.CompService,
		Language:    core.LangRaw,
		SymbolNodes: []string{"raw:lic"},
		Metadata:    map[string]string{"file_path": "LICENSE"},
		Lineage:     lineage,
	}
	symbolMap["raw:lic"] = rawSym
	compMap[compRaw.ComponentID] = compRaw

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws_test_01",
		UniverseID:  "main",
		Components:  []string{compGo.ComponentID, compHCL.ComponentID, compRaw.ComponentID},
		Lineage:     lineage,
	}

	workspaceFiles, err := hydrator.HydrateWorkspace(manifest, compMap, symbolMap)
	if err != nil {
		t.Fatalf("HydrateWorkspace failed: %v", err)
	}

	if len(workspaceFiles) < 3 {
		t.Fatalf("expected at least 3 files, got %d", len(workspaceFiles))
	}
	if string(workspaceFiles["LICENSE"]) != "MIT License\nCopyright (c) 2026" {
		t.Fatalf("unexpected LICENSE content: %s", string(workspaceFiles["LICENSE"]))
	}
}

func TestHydrator_PolyglotImportsAndPathPreservation(t *testing.T) {
	hydrator := NewHydrator()
	lineage := core.LineageEnvelope{
		UserPrompt: "Test polyglot hydration",
		Timestamp:  time.Now().UTC(),
	}

	// 1. Go with explicit file_path and custom imports
	goSym := &core.ASTSymbolNode{
		NodeID:     "sym:go:1",
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "Run",
		ASTPayload: []byte(`{"name":"Run","body_source":"func Run() {\n\tfmt.Println(time.Now())\n}"}`),
		Lineage:    lineage,
	}
	compGo := &core.ComponentNode{
		ComponentID: "comp-go-1",
		Name:        "cmd/api/main.go",
		Type:        core.CompService,
		Language:    core.LangGo,
		SymbolNodes: []string{"sym:go:1"},
		Metadata: map[string]string{
			"file_path":    "cmd/api/main.go",
			"package_name": "main",
			"imports":      "fmt,time",
		},
		Lineage: lineage,
	}
	symMap := map[string]*core.ASTSymbolNode{"sym:go:1": goSym}
	files, err := hydrator.HydrateComponent(compGo, symMap)
	if err != nil {
		t.Fatalf("HydrateComponent Go failed: %v", err)
	}
	goCode, ok := files["cmd/api/main.go"]
	if !ok {
		t.Fatalf("Expected cmd/api/main.go in hydrated files, got: %v", files)
	}
	if !strings.Contains(string(goCode), "\"fmt\"") || !strings.Contains(string(goCode), "\"time\"") {
		t.Errorf("Expected fmt and time imports in Go output: %s", string(goCode))
	}

	// 2. Python with FastAPI
	pySym := &core.ASTSymbolNode{
		NodeID:     "sym:py:1",
		Language:   core.LangPython,
		NodeType:   "Route",
		Identifier: "get_users",
		ASTPayload: []byte("@app.get(\"/users\")\ndef get_users():\n    return []"),
		Lineage:    lineage,
	}
	compPy := &core.ComponentNode{
		ComponentID: "comp-py-1",
		Name:        "users-api",
		Type:        core.CompService,
		Language:    core.LangPython,
		SymbolNodes: []string{"sym:py:1"},
		Metadata: map[string]string{
			"file_path": "services/users/main.py",
		},
		Lineage: lineage,
	}
	pyMap := map[string]*core.ASTSymbolNode{"sym:py:1": pySym}
	pyFiles, err := hydrator.HydrateComponent(compPy, pyMap)
	if err != nil {
		t.Fatalf("HydrateComponent Python failed: %v", err)
	}
	pyCode, ok := pyFiles["services/users/main.py"]
	if !ok {
		t.Fatalf("Expected services/users/main.py in hydrated files")
	}
	if !strings.Contains(string(pyCode), "from fastapi import FastAPI") {
		t.Errorf("Expected FastAPI import in python output: %s", string(pyCode))
	}
}

func TestMemoryVFS_Operations(t *testing.T) {
	vfs := NewMemoryVFS()

	err := vfs.WriteFile("src/main.go", []byte("package main"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err = vfs.WriteFile("src/utils/math.go", []byte("package utils"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	data, err := vfs.ReadFile("src/main.go")
	if err != nil || string(data) != "package main" {
		t.Fatalf("ReadFile failed: data=%s, err=%v", string(data), err)
	}

	entries, err := vfs.ReadDir("src")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 2 { // main.go and utils dir
		t.Fatalf("expected 2 entries in 'src', got %d", len(entries))
	}

	allFiles := vfs.ListFiles()
	if len(allFiles) != 2 {
		t.Fatalf("expected 2 files in VFS, got %d", len(allFiles))
	}

	fileMap := vfs.ToMap()
	if len(fileMap) != 2 {
		t.Fatalf("expected 2 map entries, got %d", len(fileMap))
	}
}

func TestExporter_ExportToDisk(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fg_export_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	files := map[string][]byte{
		"server/main.go": []byte("package main\nfunc main() {}\n"),
		"infra/main.tf":  []byte("resource \"aws_s3_bucket\" \"b\" {}\n"),
	}

	exporter := NewExporter()
	report, err := exporter.ExportToDisk(tmpDir, files, true)
	if err != nil {
		t.Fatalf("ExportToDisk failed: %v", err)
	}

	if report.FilesWritten != 2 {
		t.Fatalf("expected 2 files written, got %d", report.FilesWritten)
	}

	serverPath := filepath.Join(tmpDir, "server", "main.go")
	if _, err := os.Stat(serverPath); err != nil {
		t.Fatalf("expected server/main.go to exist on disk: %v", err)
	}
}

func TestExporter_TarStream(t *testing.T) {
	files := map[string][]byte{
		"server/main.go": []byte("package main\nfunc main() {}\n"),
		"infra/main.tf":  []byte("resource \"aws_s3_bucket\" \"b\" {}\n"),
	}

	var buf bytes.Buffer
	exporter := NewExporter()
	report, err := exporter.ExportToTarStream(&buf, files)
	if err != nil {
		t.Fatalf("ExportToTarStream failed: %v", err)
	}

	if report.FilesWritten != 2 {
		t.Fatalf("expected 2 files written, got %d", report.FilesWritten)
	}

	// Decompress and verify tar entries
	gr, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	readEntries := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read tar entry: %v", err)
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("failed to read tar entry content: %v", err)
		}
		readEntries[hdr.Name] = content
	}

	if len(readEntries) != 2 {
		t.Fatalf("expected 2 tar entries, got %d", len(readEntries))
	}
	if string(readEntries["server/main.go"]) != "package main\nfunc main() {}\n" {
		t.Fatalf("mismatched content for server/main.go: %s", string(readEntries["server/main.go"]))
	}
}

func TestViewer_Rendering(t *testing.T) {
	viewer := NewViewer()

	node := &core.ASTSymbolNode{
		NodeID:     "node_1234567890abcdef",
		Language:   core.LangGo,
		NodeType:   "FunctionDecl",
		Identifier: "auth.HandleLogin",
		Lineage: core.LineageEnvelope{
			UserPrompt:       "Implement auth handler",
			ExecutingAgentID: "agent_007",
			LLMVersion:       "gpt-4o",
			Timestamp:        time.Now().UTC(),
		},
		ASTPayload: []byte(`{"name":"HandleLogin","params":[],"results":[]}`),
	}

	terminalView := viewer.RenderSymbolNode(node, FormatTerminal)
	if !strings.Contains(terminalView, "HandleLogin") {
		t.Fatalf("terminal view missing symbol name: %s", terminalView)
	}

	markdownView := viewer.RenderSymbolNode(node, FormatMarkdown)
	if !strings.Contains(markdownView, "```go") {
		t.Fatalf("markdown view missing go block: %s", markdownView)
	}

	diffView := viewer.RenderDiff("func Old() {}", "func New() {}", "main.go", FormatTerminal)
	if !strings.Contains(diffView, "main.go") {
		t.Fatalf("diff view missing filename: %s", diffView)
	}
}
