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
