package shipping

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/codecs/golang"
	"github.com/cosmscm/cosm/pkg/codecs/hcl"
	"github.com/cosmscm/cosm/pkg/core"
)

func TestTargetSpec_DefaultsAndValidation(t *testing.T) {
	localTarget := DefaultLocalServiceTarget("target:my-service", []string{"service"})
	if err := localTarget.Validate(); err != nil {
		t.Fatalf("local target validation failed: %v", err)
	}

	cloudRunTarget := DefaultCloudRunTarget("target:my-cloud-run", []string{"service", "infra"})
	if err := cloudRunTarget.Validate(); err != nil {
		t.Fatalf("cloud run target validation failed: %v", err)
	}

	staticWebTarget := DefaultStaticWebTarget("target:my-web", []string{"frontend"})
	if err := staticWebTarget.Validate(); err != nil {
		t.Fatalf("static web target validation failed: %v", err)
	}

	terraformTarget := DefaultTerraformTarget("target:my-infra", []string{"infra"})
	if err := terraformTarget.Validate(); err != nil {
		t.Fatalf("terraform target validation failed: %v", err)
	}
}

func TestStagingManager_PrepareAndExecute(t *testing.T) {
	staging := NewStagingManager("")

	target := DefaultLocalServiceTarget("target:test-service", nil)
	files := map[string][]byte{
		"server/main.go": []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"Hello\") }\n"),
	}

	workspace, err := staging.Prepare(target, files)
	if err != nil {
		t.Fatalf("staging prepare failed: %v", err)
	}
	defer func() {
		_ = workspace.Cleanup()
	}()

	readBytes, err := workspace.ReadFile("server/main.go")
	if err != nil || !strings.Contains(string(readBytes), "package main") {
		t.Fatalf("staging ReadFile failed: %v", err)
	}

	fileList, err := workspace.ListFiles()
	if err != nil || len(fileList) == 0 {
		t.Fatalf("staging ListFiles failed: %v", err)
	}
}

func TestTerraformRunner_FmtAndValidate(t *testing.T) {
	staging := NewStagingManager("")
	tfRunner := NewTerraformRunner(staging)

	tfFiles := map[string][]byte{
		"infra/main.tf": []byte(`
resource "google_storage_bucket" "test_bucket" {
name="my-bucket"
location="US"
}
`),
	}

	report, err := tfRunner.CheckFiles(tfFiles)
	if err != nil {
		t.Fatalf("terraform check files failed: %v", err)
	}

	if !report.Valid {
		t.Fatalf("expected valid terraform, got diagnostics: %v", report.Diagnostics)
	}

	fixed, err := tfRunner.FormatAndFix(tfFiles)
	if err != nil {
		t.Fatalf("terraform format files failed: %v", err)
	}

	if !strings.Contains(string(fixed["infra/main.tf"]), "  name = \"my-bucket\"") &&
		!strings.Contains(string(fixed["infra/main.tf"]), "  name     = \"my-bucket\"") {
		t.Fatalf("expected formatted indentation, got: %s", string(fixed["infra/main.tf"]))
	}
}

func TestCompiler_GoAndFrontend(t *testing.T) {
	staging := NewStagingManager("")
	compiler := NewCompiler(staging)

	// Go compile test
	goTarget := DefaultLocalServiceTarget("target:service", nil)
	goFiles := map[string][]byte{
		"main.go": []byte("package main\nfunc main() {}\n"),
	}
	goWorkspace, err := staging.Prepare(goTarget, goFiles)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	defer func() {
		_ = goWorkspace.Cleanup()
	}()

	goRes, err := compiler.CompileGo(goWorkspace, "server")
	if err != nil || !goRes.Successful {
		t.Fatalf("Go compilation failed: err=%v, res=%+v", err, goRes)
	}

	// Verify real binary was produced (STRICT NEVER MOCK DIRECTIVE)
	if goRes.BinarySizeBytes < 1000 {
		t.Fatalf("expected real binary with size > 1000 bytes, got %d bytes", goRes.BinarySizeBytes)
	}
	binBytes, err := os.ReadFile(goRes.ArtifactPath)
	if err != nil {
		t.Fatalf("failed to read binary artifact: %v", err)
	}
	if strings.HasPrefix(string(binBytes), "#!/bin/sh") {
		t.Fatalf("detected forbidden synthetic shell script binary fallback!")
	}

	// Frontend compile test
	feTarget := DefaultStaticWebTarget("target:web", nil)
	feFiles := map[string][]byte{
		"src/App.tsx": []byte("import React from 'react';\nexport const App: React.FC = () => { return <div>App</div>; };\n"),
	}
	feWorkspace, err := staging.Prepare(feTarget, feFiles)
	if err != nil {
		t.Fatalf("prepare fe failed: %v", err)
	}
	defer func() {
		_ = feWorkspace.Cleanup()
	}()

	feRes, err := compiler.CompileFrontend(feWorkspace, "bundle.js")
	if err != nil || !feRes.Successful {
		t.Fatalf("Frontend bundling failed: err=%v, res=%+v", err, feRes)
	}
}

func TestCompiler_Go_SyntaxErrorDiagnostics(t *testing.T) {
	staging := NewStagingManager("")
	compiler := NewCompiler(staging)

	goTarget := DefaultLocalServiceTarget("target:service-err", nil)
	goFiles := map[string][]byte{
		"main.go": []byte("package main\nfunc main( {\n"),
	}
	goWorkspace, err := staging.Prepare(goTarget, goFiles)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	defer func() {
		_ = goWorkspace.Cleanup()
	}()

	goRes, err := compiler.CompileGo(goWorkspace, "server")
	if err == nil {
		t.Fatalf("expected syntax error compilation failure, got nil error")
	}
	if goRes == nil {
		t.Fatalf("expected non-nil BuildResult on error")
	}
	if goRes.Successful {
		t.Fatalf("expected Successful=false on syntax error")
	}
	if !strings.Contains(goRes.OutputLogs, "syntax error") {
		t.Fatalf("expected OutputLogs to contain syntax diagnostics, got: %s", goRes.OutputLogs)
	}
	if len(goRes.Errors) == 0 {
		t.Fatalf("expected non-empty Errors slice")
	}
}

func TestCompiler_Go_CompileErrorDiagnostics(t *testing.T) {
	staging := NewStagingManager("")
	compiler := NewCompiler(staging)

	goTarget := DefaultLocalServiceTarget("target:service-type-err", nil)
	goFiles := map[string][]byte{
		"main.go": []byte("package main\nfunc main() {\n    undeclaredFunctionCallForTest()\n}\n"),
	}
	goWorkspace, err := staging.Prepare(goTarget, goFiles)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	defer func() {
		_ = goWorkspace.Cleanup()
	}()

	goRes, err := compiler.CompileGo(goWorkspace, "server")
	if err == nil {
		t.Fatalf("expected compile failure on undefined function, got nil error")
	}
	if goRes == nil {
		t.Fatalf("expected non-nil BuildResult on error")
	}
	if goRes.Successful {
		t.Fatalf("expected Successful=false on compiler error")
	}
	if !strings.Contains(goRes.OutputLogs, "undeclaredFunctionCallForTest") {
		t.Fatalf("expected OutputLogs to contain compiler diagnostics, got: %s", goRes.OutputLogs)
	}

	// Verify no synthetic shell script was emitted
	if goRes.ArtifactPath != "" {
		if binBytes, readErr := os.ReadFile(goRes.ArtifactPath); readErr == nil {
			if strings.HasPrefix(string(binBytes), "#!/bin/sh") {
				t.Fatalf("detected forbidden synthetic shell script fallback on compiler error!")
			}
		}
	}
}

func TestCompiler_Go_AutoStagedModule_RealBinaryExecution(t *testing.T) {
	staging := NewStagingManager("")
	compiler := NewCompiler(staging)

	goTarget := DefaultLocalServiceTarget("target:exec-test", nil)
	goFiles := map[string][]byte{
		"main.go": []byte("package main\nimport \"fmt\"\nfunc main() {\n    fmt.Println(\"cosm_real_binary_probe_ok\")\n}\n"),
	}
	goWorkspace, err := staging.Prepare(goTarget, goFiles)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	defer func() {
		_ = goWorkspace.Cleanup()
	}()

	// Verify go.mod was auto-staged
	modContent, err := goWorkspace.ReadFile("go.mod")
	if err != nil || !strings.Contains(string(modContent), "module github.com/cosmscm/cosm") {
		t.Fatalf("expected auto-staged go.mod with cosm module, got err=%v", err)
	}

	goRes, err := compiler.CompileGo(goWorkspace, "probe_app")
	if err != nil || !goRes.Successful {
		t.Fatalf("expected successful Go compilation with auto-staged go.mod: err=%v, res=%+v", err, goRes)
	}

	// Verify genuine executable binary
	if goRes.BinarySizeBytes < 1000 {
		t.Fatalf("expected real binary with size > 1000, got %d", goRes.BinarySizeBytes)
	}
	binBytes, err := os.ReadFile(goRes.ArtifactPath)
	if err != nil {
		t.Fatalf("failed to read binary artifact: %v", err)
	}
	if strings.HasPrefix(string(binBytes), "#!/bin/sh") {
		t.Fatalf("artifact is a synthetic shell script, NOT a real binary!")
	}

	// Execute the binary and probe output
	cmd := exec.Command(goRes.ArtifactPath)
	output, execErr := cmd.CombinedOutput()
	if execErr != nil {
		t.Fatalf("failed to execute compiled binary: %v, output: %s", execErr, string(output))
	}
	if !strings.Contains(string(output), "cosm_real_binary_probe_ok") {
		t.Fatalf("unexpected binary output: %s", string(output))
	}
}

func TestStaging_AutoStageDependencies_NodeAndGo(t *testing.T) {
	tempRoot, err := os.MkdirTemp("", "cosm_mock_repo_*")
	if err != nil {
		t.Fatalf("failed to create temp repo: %v", err)
	}
	defer os.RemoveAll(tempRoot)

	// Write mock repo files
	_ = os.WriteFile(filepath.Join(tempRoot, "go.mod"), []byte("module example.com/autostage\n\ngo 1.22\n"), 0644)
	_ = os.WriteFile(filepath.Join(tempRoot, "go.sum"), []byte("example.com/autostage v0.1.0 h1:checksum=\n"), 0644)
	_ = os.WriteFile(filepath.Join(tempRoot, "package.json"), []byte("{\"name\": \"@cosm/web\", \"version\": \"1.0.0\"}\n"), 0644)
	_ = os.WriteFile(filepath.Join(tempRoot, "tsconfig.json"), []byte("{\"compilerOptions\": {\"target\": \"ES2022\"}}\n"), 0644)

	staging := NewStagingManagerWithRepo("", tempRoot)
	packager := NewPackager(staging)
	packager.SetRepoRoot(tempRoot)

	// Test 1: StagingManager.Prepare auto-stages into workspace
	target := DefaultLocalServiceTarget("target:autostage-test", nil)
	files := map[string][]byte{
		"server/main.go": []byte("package main\nfunc main() {}\n"),
	}
	ws, err := staging.Prepare(target, files)
	if err != nil {
		t.Fatalf("staging prepare failed: %v", err)
	}
	defer func() {
		_ = ws.Cleanup()
	}()

	// Verify all 4 files are auto-staged
	for _, rel := range []string{"go.mod", "go.sum", "package.json", "tsconfig.json"} {
		data, readErr := ws.ReadFile(rel)
		if readErr != nil {
			t.Fatalf("expected %s to be auto-staged in workspace, got error: %v", rel, readErr)
		}
		if len(data) == 0 {
			t.Fatalf("expected non-empty data in auto-staged %s", rel)
		}
	}

	// Test 2: Packager.AutoStageDependencies populates files map
	pkgFiles := map[string][]byte{
		"src/index.ts": []byte("export const a = 1;"),
	}
	if err := packager.AutoStageDependencies(target, pkgFiles); err != nil {
		t.Fatalf("Packager.AutoStageDependencies failed: %v", err)
	}
	if _, ok := pkgFiles["package.json"]; !ok {
		t.Fatalf("expected package.json in pkgFiles")
	}
	if _, ok := pkgFiles["tsconfig.json"]; !ok {
		t.Fatalf("expected tsconfig.json in pkgFiles")
	}
	if _, ok := pkgFiles["go.mod"]; !ok {
		t.Fatalf("expected go.mod in pkgFiles")
	}
}

func TestPackager_CompositeShip(t *testing.T) {
	distDir, err := os.MkdirTemp("", "fg_packager_dist_*")
	if err != nil {
		t.Fatalf("failed to create temp dist: %v", err)
	}
	defer os.RemoveAll(distDir)

	staging := NewStagingManager("")
	packager := NewPackager(staging)

	lineage := core.LineageEnvelope{
		UserPrompt: "Ship multi-tier target",
		Timestamp:  time.Now().UTC(),
	}

	goParser := golang.NewGoParser()
	goRes, _ := goParser.ParseSource("server/main.go", []byte("package main\nfunc main() {}"), lineage)
	compGo, _ := golang.BuildComponentNode("server", core.CompService, goRes, lineage)

	hclParser := hcl.NewHCLParser()
	hclDoc, _ := hclParser.ParseSource("infra/main.tf", []byte("resource \"google_storage_bucket\" \"b\" { name = \"b\" }"))
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

	manifest := &core.WorkspaceManifestNode{
		WorkspaceID: "ws_packager_01",
		UniverseID:  "main",
		Components:  []string{compGo.ComponentID, compHCL.ComponentID},
		Lineage:     lineage,
	}

	targets := []*TargetSpec{
		DefaultLocalServiceTarget("target:local-service", []string{"server"}),
		DefaultTerraformTarget("target:infra", []string{"infra"}),
	}

	artifacts, err := packager.ShipWorkspace(manifest, compMap, symbolMap, targets, distDir)
	if err != nil {
		t.Fatalf("ShipWorkspace failed: %v", err)
	}

	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}

	for _, art := range artifacts {
		if art.SizeBytes == 0 {
			t.Fatalf("artifact %s size is 0", art.TargetName)
		}
		if _, err := os.Stat(art.ArtifactPath); err != nil {
			t.Fatalf("artifact file not found at %s: %v", art.ArtifactPath, err)
		}
	}
}

func TestCompiler_PolyglotComponents(t *testing.T) {
	staging := NewStagingManager("")
	compiler := NewCompiler(staging)

	testCases := []struct {
		lang     core.Language
		filename string
		content  string
	}{
		{core.LangGo, "main.go", "package main\nfunc main() {}"},
		{core.LangTypeScript, "src/App.tsx", "import React from 'react';\nexport const App = () => <div>App</div>;"},
		{core.LangPython, "app.py", "def main():\n    print('Hello')\nif __name__ == '__main__':\n    main()"},
		{core.LangHCL, "infra/main.tf", "resource \"google_storage_bucket\" \"b\" { name = \"test\" }"},
		{core.LangRust, "src/main.rs", "fn main() { println!(\"Hello\"); }"},
		{core.LangJava, "src/App.java", "public class App { public static void main(String[] args) {} }"},
		{core.LangCpp, "src/main.cpp", "#include <iostream>\nint main() { return 0; }"},
		{core.LangC, "src/main.c", "#include <stdio.h>\nint main() { return 0; }"},
		{core.LangSQL, "db/schema.sql", "CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL);"},
		{core.LangProtobuf, "proto/service.proto", "syntax = \"proto3\";\nservice UserSvc { rpc GetUser(UserReq) returns (UserRes); }"},
		{core.LangGraphQL, "schema.graphql", "type User { id: ID! email: String! }"},
		{core.LangOpenAPI, "openapi.json", "{\"openapi\": \"3.0.0\", \"info\": {\"title\": \"API\", \"version\": \"1.0\"}}"},
		{core.LangSwift, "Sources/App.swift", "struct App { func run() {} }"},
		{core.LangKotlin, "src/main/kotlin/App.kt", "class App { fun run() {} }"},
		{core.LangCSharp, "src/Program.cs", "class Program { static void Main() {} }"},
		{core.LangWasm, "wasm/module.wat", "(module (func $main))"},
		{core.LangZig, "src/main.zig", "pub fn main() void {}"},
		{core.LangRuby, "lib/app.rb", "class App; end"},
		{core.LangPHP, "src/index.php", "<?php echo 'Hello';"},
		{core.LangElixir, "lib/app.ex", "defmodule App do; end"},
		{core.LangDockerfile, "Dockerfile", "FROM alpine:3.19\nCMD [\"sh\"]"},
		{core.LangRaw, "assets/logo.png", "raw-binary-bytes"},
	}

	for _, tc := range testCases {
		target := &TargetSpec{
			Name:           "target:" + string(tc.lang),
			Kind:           TargetLocalService,
			ComponentNames: []string{"app"},
			OutputDir:      "bin",
		}
		files := map[string][]byte{
			tc.filename: []byte(tc.content),
		}
		ws, err := staging.Prepare(target, files)
		if err != nil {
			t.Fatalf("Prepare for %s failed: %v", tc.lang, err)
		}

		res, err := compiler.CompileComponent(ws, tc.lang, "app")
		_ = ws.Cleanup()

		if err != nil {
			t.Fatalf("CompileComponent for %s failed: %v", tc.lang, err)
		}
		if !res.Successful {
			t.Fatalf("CompileComponent for %s was not successful", tc.lang)
		}
		if res.BinarySizeBytes == 0 {
			t.Fatalf("CompileComponent for %s produced 0 bytes", tc.lang)
		}
	}
}

func TestTargetSpec_PolyglotPresets(t *testing.T) {
	pyTarget := DefaultPythonTarget("target:py", []string{"service"})
	if err := pyTarget.Validate(); err != nil {
		t.Fatalf("python target validation failed: %v", err)
	}

	rustTarget := DefaultRustTarget("target:rust", []string{"service"})
	if err := rustTarget.Validate(); err != nil {
		t.Fatalf("rust target validation failed: %v", err)
	}

	javaTarget := DefaultJavaTarget("target:java", []string{"service"})
	if err := javaTarget.Validate(); err != nil {
		t.Fatalf("java target validation failed: %v", err)
	}

	cppTarget := DefaultCppTarget("target:cpp", []string{"service"})
	if err := cppTarget.Validate(); err != nil {
		t.Fatalf("cpp target validation failed: %v", err)
	}
}
