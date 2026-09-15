package ignore

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cosmscm/cosm/pkg/core"
)

func TestIgnoreEngine_DefaultRules(t *testing.T) {
	tempDir := t.TempDir()
	engine := NewIgnoreEngine(tempDir)

	// Default dirs
	if !engine.ShouldIgnorePath(".git", true) {
		t.Errorf("expected .git dir to be ignored")
	}
	if !engine.ShouldIgnorePath("node_modules", true) {
		t.Errorf("expected node_modules to be ignored")
	}
	if !engine.ShouldIgnorePath("vendor", true) {
		t.Errorf("expected vendor to be ignored")
	}
	if !engine.ShouldIgnorePath("dist", true) {
		t.Errorf("expected dist to be ignored")
	}

	// Default secrets
	if !engine.ShouldIgnorePath(".env", false) {
		t.Errorf("expected .env to be ignored by default")
	}
	if !engine.ShouldIgnorePath(".env.production", false) {
		t.Errorf("expected .env.production to be ignored by default")
	}
	if !engine.ShouldIgnorePath("server.key", false) {
		t.Errorf("expected server.key to be ignored by default")
	}
	if !engine.ShouldIgnorePath("id_rsa", false) {
		t.Errorf("expected id_rsa to be ignored by default")
	}
	if !engine.ShouldIgnorePath("credentials.json", false) {
		t.Errorf("expected credentials.json to be ignored by default")
	}
	if !engine.ShouldIgnorePath("terraform.tfvars", false) {
		t.Errorf("expected terraform.tfvars to be ignored by default")
	}

	// Normal source code should NOT be ignored
	if engine.ShouldIgnorePath("main.go", false) {
		t.Errorf("expected main.go to NOT be ignored")
	}
	if engine.ShouldIgnorePath("src/app.ts", false) {
		t.Errorf("expected src/app.ts to NOT be ignored")
	}
}

func TestIgnoreEngine_FileLoading(t *testing.T) {
	tempDir := t.TempDir()
	cosmIgnoreContent := `# Cosm ignore file
*.log
/local_cache/
temp/**
!temp/important.log

[edges]
ignore BINDS_ENV where env="TEST_*"
ignore CONSUMES_API where endpoint="/mock/*"
`
	cosmIgnorePath := filepath.Join(tempDir, ".cosmignore")
	if err := os.WriteFile(cosmIgnorePath, []byte(cosmIgnoreContent), 0644); err != nil {
		t.Fatalf("failed to write .cosmignore: %v", err)
	}

	engine, err := LoadWorkspaceRules(tempDir)
	if err != nil {
		t.Fatalf("failed to load workspace rules: %v", err)
	}

	// Test custom file ignore rules
	if !engine.ShouldIgnorePath("debug.log", false) {
		t.Errorf("expected debug.log to be ignored")
	}
	if !engine.ShouldIgnorePath("sub/dir/app.log", false) {
		t.Errorf("expected sub/dir/app.log to be ignored")
	}
	if !engine.ShouldIgnorePath("local_cache", true) {
		t.Errorf("expected local_cache dir to be ignored")
	}
	if !engine.ShouldIgnorePath("temp/file.txt", false) {
		t.Errorf("expected temp/file.txt to be ignored")
	}
	// Test negation
	if engine.ShouldIgnorePath("temp/important.log", false) {
		t.Errorf("expected temp/important.log to NOT be ignored due to negation")
	}

	// Test edge filter rules loaded from [edges]
	if len(engine.EdgeFilter.Rules) != 2 {
		t.Fatalf("expected 2 edge rules loaded, got %d", len(engine.EdgeFilter.Rules))
	}
	if engine.EdgeFilter.Rules[0].EdgeType != core.EdgeBindsEnv || engine.EdgeFilter.Rules[0].EnvVar != "TEST_*" {
		t.Errorf("unexpected edge rule 0: %+v", engine.EdgeFilter.Rules[0])
	}
	if engine.EdgeFilter.Rules[1].EdgeType != core.EdgeConsumesAPI || engine.EdgeFilter.Rules[1].EndpointGlob != "/mock/*" {
		t.Errorf("unexpected edge rule 1: %+v", engine.EdgeFilter.Rules[1])
	}
}

func TestSecretDetector(t *testing.T) {
	detector := NewSecretDetector()

	// High confidence API key (dynamically constructed to prevent static secret scanner alerts)
	fakeGoogleKey := fmt.Sprintf("%s%s%s", "AIza", "SyD-1234567890abcdefghijkl", "mnopqrstuv")
	contentWithKey := []byte(fmt.Sprintf("\nconst googleApiKey = %q\nconst normalVar = \"hello-world\"\n", fakeGoogleKey))
	findings := detector.DetectSecrets("config.js", contentWithKey)
	if len(findings) != 1 {
		t.Fatalf("expected 1 secret finding, got %d", len(findings))
	}
	if findings[0].Type != "Google AI / GCP API Key" {
		t.Errorf("expected Google AI Key type, got %s", findings[0].Type)
	}

	// Private key block
	privKey := []byte(`
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA0...
-----END RSA PRIVATE KEY-----
`)
	findings = detector.DetectSecrets("id_rsa", privKey)
	if len(findings) == 0 {
		t.Fatalf("expected private key finding")
	}

	// Benign / placeholder checks
	benignContent := []byte(`
auth_token = "${var.auth_token}"
password = "dummy-password"
api_key = "secret-token"
`)
	findings = detector.DetectSecrets("main.tf", benignContent)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for benign placeholders, got %d", len(findings))
	}

	// Inline suppression check
	suppressedContent := []byte(fmt.Sprintf("\nconst testKey = %q // cosm:allow-secret\n", fakeGoogleKey))
	findings = detector.DetectSecrets("test.js", suppressedContent)
	if len(findings) != 0 {
		t.Errorf("expected suppressed finding to be ignored, got %d", len(findings))
	}
}

func TestEdgeFilter_ShouldIgnoreEdge(t *testing.T) {
	filter := NewEdgeFilter()
	filter.ParseRuleLine("ignore BINDS_ENV where env=\"TEST_*\"")
	filter.ParseRuleLine("ignore CONSUMES_API where endpoint=\"/mock/*\"")
	filter.ParseRuleLine("ignore * where source=\"test/**\"")

	// 1. Matching BINDS_ENV
	edge1 := core.CrossBoundaryEdge{
		Type:     core.EdgeBindsEnv,
		Metadata: map[string]string{"env_var": "TEST_DB_PASSWORD"},
	}
	if !filter.ShouldIgnoreEdge(edge1, nil, nil, "pkg/db.go", "infra/main.tf") {
		t.Errorf("expected edge1 to be ignored by env var rule")
	}

	// Non-matching BINDS_ENV
	edgeProd := core.CrossBoundaryEdge{
		Type:     core.EdgeBindsEnv,
		Metadata: map[string]string{"env_var": "PROD_DB_PASSWORD"},
	}
	if filter.ShouldIgnoreEdge(edgeProd, nil, nil, "pkg/db.go", "infra/main.tf") {
		t.Errorf("expected edgeProd to NOT be ignored")
	}

	// 2. Matching CONSUMES_API
	edgeMock := core.CrossBoundaryEdge{
		Type:     core.EdgeConsumesAPI,
		Metadata: map[string]string{"consumer_endpoint": "/mock/users"},
	}
	if !filter.ShouldIgnoreEdge(edgeMock, nil, nil, "frontend/app.tsx", "backend/main.go") {
		t.Errorf("expected edgeMock to be ignored by endpoint rule")
	}

	// 3. Matching source path glob
	edgeTestSource := core.CrossBoundaryEdge{
		Type: core.EdgeCalls,
	}
	if !filter.ShouldIgnoreEdge(edgeTestSource, nil, nil, "test/integration/suite_test.go", "pkg/api.go") {
		t.Errorf("expected edge with test/** source to be ignored")
	}

	// 4. Inline comment metadata
	sourceNodeWithMeta := &core.ASTSymbolNode{
		NodeID:     "node-1",
		Identifier: "FetchMockData",
		ASTMetadata: map[string]string{
			"cosm:ignore-edge": "CONSUMES_API",
		},
	}
	edgeInline := core.CrossBoundaryEdge{
		SourceNodeID: "node-1",
		Type:         core.EdgeConsumesAPI,
		Metadata:     map[string]string{"consumer_endpoint": "/api/users"},
	}
	if !filter.ShouldIgnoreEdge(edgeInline, sourceNodeWithMeta, nil, "src/api.ts", "src/server.go") {
		t.Errorf("expected edgeInline to be ignored via inline ASTMetadata")
	}
}
