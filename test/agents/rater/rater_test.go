package rater_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/rater"
)

func TestLanguageSyntacticValidators(t *testing.T) {
	// 1. Go
	validGo := []byte(`package main
func main() {}`)
	if err := rater.ValidateGoSyntax("main.go", validGo); err != nil {
		t.Errorf("expected valid Go, got %v", err)
	}
	invalidGo := []byte(`package main
func main( {`)
	if err := rater.ValidateGoSyntax("main.go", invalidGo); err == nil {
		t.Errorf("expected syntax error on invalid Go")
	}

	// 2. Python
	validPy := []byte(`def hello():
    return "world"`)
	if err := rater.ValidatePythonSyntax("main.py", validPy); err != nil {
		t.Errorf("expected valid Python, got %v", err)
	}
	invalidPy := []byte(`def hello()
    return "world"`)
	if err := rater.ValidatePythonSyntax("main.py", invalidPy); err == nil {
		t.Errorf("expected syntax error on invalid Python")
	}

	// 3. TypeScript
	validTS := []byte(`export interface User { id: string; }`)
	if err := rater.ValidateTypeScriptSyntax("App.tsx", validTS); err != nil {
		t.Errorf("expected valid TS, got %v", err)
	}
	invalidTS := []byte(`export interface User { id: string; `)
	if err := rater.ValidateTypeScriptSyntax("App.tsx", invalidTS); err == nil {
		t.Errorf("expected syntax error on invalid TS")
	}

	// 4. Rust
	validRust := []byte(`fn main() { println!("hello"); }`)
	if err := rater.ValidateRustSyntax("main.rs", validRust); err != nil {
		t.Errorf("expected valid Rust, got %v", err)
	}

	// 5. SQL
	validSQL := []byte(`CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);`)
	if err := rater.ValidateSQLSyntax("schema.sql", validSQL); err != nil {
		t.Errorf("expected valid SQL, got %v", err)
	}

	// 6. Protobuf
	validProto := []byte(`syntax = "proto3"; message User { string id = 1; }`)
	if err := rater.ValidateProtobufSyntax("user.proto", validProto); err != nil {
		t.Errorf("expected valid Protobuf, got %v", err)
	}

	// 7. HCL
	validHCL := []byte(`resource "google_cloud_run_service" "app" { name = "app" }`)
	if err := rater.ValidateHCLSyntax("main.tf", validHCL); err != nil {
		t.Errorf("expected valid HCL, got %v", err)
	}
}

func TestScorecardCalculationAndReporting(t *testing.T) {
	audit := &rater.OracleAuditReport{
		WorkspaceDir:      "/tmp/test-ws",
		UniverseID:        "universe-main",
		Timestamp:         time.Now().UTC(),
		TotalFilesAudited: 4,
		SyntaxValidCount:  4,
		SyntaxErrorsCount: 0,
		ContractEntries: []rater.ContractAuditEntry{
			{
				SourceComponent: "frontend/App.tsx",
				TargetComponent: "backend/main.go",
				ContractType:    "API_ROUTE",
				Identifier:      "/api/v1/items",
				Status:          "VALID",
			},
		},
		StorageReport: rater.StorageIntegrityReport{
			TotalObjectsChecked: 10,
			ValidObjectsCount:   10,
			BitRotDetected:      false,
			MerkleRootMatch:     true,
			ManifestMerkleRoot:  "root1234",
			RecalculatedRoot:    "root1234",
		},
		LineageReport: rater.LineageAuditReport{
			TotalNodesAudited: 10,
			IntactChainsCount: 10,
			AllSignaturesOK:   true,
		},
	}

	critic := rater.NewLLMCritic(nil, "critic-test")
	critique, err := critic.EvaluateArchitecture(context.Background(), audit, nil)
	if err != nil {
		t.Fatalf("critic failed: %v", err)
	}

	sc := rater.CalculateScorecard(audit, critique)

	if sc.OverallScore < 85.0 {
		t.Errorf("expected A or A+ score, got %.2f", sc.OverallScore)
	}
	if sc.LetterGrade != rater.GradeA && sc.LetterGrade != rater.GradeAPlus {
		t.Errorf("expected letter grade A or A+, got %s", sc.LetterGrade)
	}

	reporter := rater.NewReporter()
	session := framework.NewAgentSession("test-scenario", "agent-1", "gemini-3.7-flash", "/tmp/ws")
	session.Finalize(framework.StatusSuccess, "All done")

	md := reporter.GenerateMarkdownScorecard(sc, audit, session)
	if len(md) == 0 {
		t.Errorf("expected non-empty markdown output")
	}

	remediation := reporter.GenerateRemediationReport(sc)
	if len(remediation) == 0 {
		t.Errorf("expected remediation output")
	}
}

func TestDeepVerificationOracle_AuditWorkspace(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "oracle-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Write valid Go backend and React frontend
	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(`package main
import "github.com/gin-gonic/gin"
func main() {
	r := gin.Default()
	r.GET("/api/v1/test", nil)
}`), 0644)

	_ = os.WriteFile(filepath.Join(tempDir, "App.tsx"), []byte(`import React from "react";
export const App = () => {
	fetch("/api/v1/test");
	return <div>App</div>;
};`), 0644)

	oracle := rater.NewDeepVerificationOracle()
	report, err := oracle.AuditWorkspace(tempDir, "universe-main")
	if err != nil {
		t.Fatalf("failed oracle audit: %v", err)
	}

	if report.TotalFilesAudited != 2 {
		t.Errorf("expected 2 files audited, got %d", report.TotalFilesAudited)
	}
	if report.SyntaxErrorsCount != 0 {
		t.Errorf("expected 0 syntax errors, got %d", report.SyntaxErrorsCount)
	}
	if len(report.ContractEntries) != 1 {
		t.Fatalf("expected 1 contract entry, got %d", len(report.ContractEntries))
	}
	if report.ContractEntries[0].Status != "VALID" {
		t.Errorf("expected contract VALID, got %s", report.ContractEntries[0].Status)
	}
}
