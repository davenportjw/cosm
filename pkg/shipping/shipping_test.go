package shipping

import (
	"os"
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

	tfTarget := DefaultTerraformTarget("target:my-infra", []string{"infra"})
	if err := tfTarget.Validate(); err != nil {
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
	runner := NewTerraformRunner(staging)

	validHCL := map[string][]byte{
		"infra/main.tf": []byte(`resource "google_storage_bucket" "b" {
  name     = "my-bucket"
  location = "US"
}
`),
	}

	report, err := runner.CheckFiles(validHCL)
	if err != nil {
		t.Fatalf("terraform check failed: %v", err)
	}
	if !report.Valid {
		t.Fatalf("expected valid HCL, got invalid: %v", report.Diagnostics)
	}

	unformattedHCL := map[string][]byte{
		"infra/main.tf": []byte("resource \"google_storage_bucket\" \"b\" {\nname=\"my-bucket\"\n}\n"),
	}
	fixed, err := runner.FormatAndFix(unformattedHCL)
	if err != nil {
		t.Fatalf("FormatAndFix failed: %v", err)
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
