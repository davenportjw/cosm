package agents_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
	"github.com/cosmscm/cosm/test/agents/rater"
	"github.com/cosmscm/cosm/test/agents/testagent"
)

// Test 1: Full-Stack Polyglot Generation & Deep Verification
func TestIntegration_PolyglotGenerationAndDeepVerification(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fg-polyglot-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp workspace: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mock := llm.NewMockProvider("mock-gemini-3.7-flash")
	mock.EnqueueToolCall("fg_init", `{"universe_id":"universe-main"}`)
	mock.EnqueueToolCall("fg_add", `{"files":["backend/main.py","frontend/src/App.tsx","infra/main.tf"],"intent":"Stage polyglot microservice"}`)
	mock.EnqueueToolCall("fg_commit", `{"intent":"Commit polyglot application manifest"}`)
	mock.EnqueueToolCall("fg_ship", `{"universe_id":"universe-main"}`)
	mock.EnqueueText("FastAPI + React + TF application successfully deployed and linked.")

	agent := testagent.NewTestAgent("agent-polyglot", mock, tempDir)
	scenario, err := testagent.GetScenario("fastapi-react-tf")
	if err != nil {
		t.Fatalf("failed fetching scenario: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := agent.RunScenario(ctx, *scenario, framework.LoopOptions{MaxSteps: 10})
	if err != nil {
		t.Fatalf("scenario execution failed: %v", err)
	}
	if session.Status != framework.StatusSuccess {
		t.Fatalf("expected SUCCESS, got %s", session.Status)
	}

	// Deep Verification Oracle Audit
	oracle := rater.NewDeepVerificationOracle()
	audit, err := oracle.AuditWorkspace(tempDir, "universe-main")
	if err != nil {
		t.Fatalf("oracle audit failed: %v", err)
	}

	if audit.SyntaxErrorsCount != 0 {
		t.Errorf("expected 0 syntax errors, got %d", audit.SyntaxErrorsCount)
	}

	if len(audit.ContractEntries) < 2 {
		t.Errorf("expected at least 2 cross-boundary contracts audited, got %d", len(audit.ContractEntries))
	}

	for _, c := range audit.ContractEntries {
		if c.Status != "VALID" {
			t.Errorf("contract %s -> %s (%s) expected VALID, got %s", c.SourceComponent, c.TargetComponent, c.Identifier, c.Status)
		}
	}

	// Verify Merkle Root calculation
	if !audit.StorageReport.MerkleRootMatch || audit.StorageReport.BitRotDetected {
		t.Errorf("storage Merkle root match failed: match=%v, bitRot=%v",
			audit.StorageReport.MerkleRootMatch, audit.StorageReport.BitRotDetected)
	}

	// Scorecard
	scorecard := rater.CalculateScorecard(audit)
	if scorecard.OverallScore < 95.0 || (scorecard.LetterGrade != rater.GradeAPlus && scorecard.LetterGrade != rater.GradeA) {
		t.Errorf("expected Grade A/A+ (>=95.0), got %.1f (%s)", scorecard.OverallScore, scorecard.LetterGrade)
	}

	// Reporter
	reporter := rater.NewReporter()
	md := reporter.GenerateMarkdownScorecard(scorecard, audit, session)
	if len(md) == 0 {
		t.Errorf("expected non-empty markdown report")
	}
}

// Test 2: Surgical Mutation & AST Symbol Deduplication
func TestIntegration_SurgicalMutationDeduplication(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fg-surgery-integration-*")
	if err != nil {
		t.Fatalf("failed creating temp workspace: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mock := llm.NewMockProvider("mock-gemini-3.7-flash")
	mock.EnqueueToolCall("fg_init", `{"universe_id":"universe-main"}`)
	mock.EnqueueToolCall("fg_add", `{"files":["backend/main.py","frontend/src/App.tsx","infra/main.tf"]}`)
	mock.EnqueueToolCall("fg_commit", `{"intent":"Initial baseline commit"}`)
	mock.EnqueueToolCall("fg_universe_create", `{"new_universe_id":"feat-surgical-opt","parent_universe_id":"universe-main"}`)
	mock.EnqueueToolCall("fg_symbol_edit", `{"universe_id":"feat-surgical-opt","symbol_id":"main.py","new_payload":"def list_users(): return [{'id':1, 'username':'alice', 'role':'superadmin'}]","intent":"Surgically update user role"}`)
	mock.EnqueueText("Surgical symbol mutation successfully executed with deduplication.")

	agent := testagent.NewTestAgent("agent-surgery", mock, tempDir)
	scenario, _ := testagent.GetScenario("fastapi-react-tf")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := agent.RunScenario(ctx, *scenario, framework.LoopOptions{MaxSteps: 10})
	if err != nil {
		t.Fatalf("surgery scenario failed: %v", err)
	}
	if session.Status != framework.StatusSuccess {
		t.Fatalf("expected SUCCESS, got %s", session.Status)
	}

	// Inspect trace to confirm symbol mutation tool was called and returned valid result
	foundSurgery := false
	for _, tr := range session.Traces {
		for _, tc := range tr.ToolCalls {
			if tc.Name == "fg_symbol_edit" {
				foundSurgery = true
			}
		}
	}
	if !foundSurgery {
		t.Errorf("expected fg_symbol_edit tool call in traces")
	}
}

// Test 3: Breaking Contract Detection & Autonomous Remediation
func TestIntegration_BreakingContractDetectionAndRemediation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fg-contract-integration-*")
	if err != nil {
		t.Fatalf("failed creating temp workspace: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Step A: Ingest baseline with mismatched/broken contract
	_ = os.MkdirAll(filepath.Join(tempDir, "backend"), 0755)
	_ = os.MkdirAll(filepath.Join(tempDir, "frontend", "src"), 0755)

	// Backend exposes /api/v2/users_v2
	_ = os.WriteFile(filepath.Join(tempDir, "backend/main.py"), []byte(`from fastapi import FastAPI
app = FastAPI()
@app.get("/api/v2/users_v2")
def get_v2_users():
    return [{"id": 1, "name": "v2_user"}]
`), 0644)

	// Frontend calls /api/v1/users (BROKEN CONTRACT)
	_ = os.WriteFile(filepath.Join(tempDir, "frontend/src/App.tsx"), []byte(`import React, { useEffect } from "react";
export const App = () => {
    useEffect(() => {
        fetch("/api/v1/users");
    }, []);
    return <div>Users</div>;
};
`), 0644)

	oracle := rater.NewDeepVerificationOracle()
	audit1, err := oracle.AuditWorkspace(tempDir, "universe-main")
	if err != nil {
		t.Fatalf("initial audit failed: %v", err)
	}

	brokenFound := false
	for _, c := range audit1.ContractEntries {
		if c.Identifier == "/api/v1/users" && c.Status == "BROKEN" {
			brokenFound = true
		}
	}
	if !brokenFound {
		t.Fatalf("expected broken contract /api/v1/users in audit")
	}

	scorecard1 := rater.CalculateScorecard(audit1)
	if scorecard1.Dimensions[rater.DimContract].Passed {
		t.Errorf("contract dimension should fail on broken contract")
	}

	// Step B: Agent receives critique remediation prompt and fixes frontend caller
	critic := rater.NewLLMCritic(nil, "critic-agent")
	critique, _ := critic.EvaluateArchitecture(context.Background(), audit1, nil)
	if critique.Verdict == "APPROVE" {
		t.Errorf("expected critique verdict REQUEST_CHANGES or REJECT, got %s", critique.Verdict)
	}

	mock := llm.NewMockProvider("mock-gemini-3.7-flash")
	mock.EnqueueToolCall("write_file", `{"path":"frontend/src/App.tsx","content":"import React, { useEffect } from \"react\";\nexport const App = () => {\n    useEffect(() => {\n        fetch(\"/api/v2/users_v2\");\n    }, []);\n    return <div>Users</div>;\n};\n"}`)
	mock.EnqueueText("Fixed frontend route contract to match /api/v2/users_v2.")

	agent := testagent.NewTestAgent("remediation-agent", mock, tempDir)
	session, err := agent.RunPrompt(context.Background(), critique.RemediationPrompt, framework.LoopOptions{MaxSteps: 5})
	if err != nil {
		t.Fatalf("remediation run failed: %v", err)
	}
	if session.Status != framework.StatusSuccess {
		t.Fatalf("remediation session failed: %s", session.Status)
	}

	// Step C: Re-Audit Workspace and verify contract is healed
	audit2, _ := oracle.AuditWorkspace(tempDir, "universe-main")
	for _, c := range audit2.ContractEntries {
		if c.Status != "VALID" {
			t.Errorf("expected all contracts valid after remediation, found %s (%s)", c.Identifier, c.Status)
		}
	}
	scorecard2 := rater.CalculateScorecard(audit2)
	if !scorecard2.Dimensions[rater.DimContract].Passed {
		t.Errorf("contract dimension expected to pass after remediation")
	}
}

// Test 4: Storage Bit-Rot & Tampering Defense
func TestIntegration_StorageBitRotAndTamperingDefense(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fg-bitrot-integration-*")
	if err != nil {
		t.Fatalf("failed creating temp workspace: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mock := llm.NewMockProvider("mock-gemini-3.7-flash")
	mock.EnqueueToolCall("fg_init", `{"universe_id":"universe-main"}`)
	mock.EnqueueToolCall("fg_add", `{"files":["backend/main.py","frontend/src/App.tsx","infra/main.tf"]}`)
	mock.EnqueueToolCall("fg_commit", `{"intent":"Initial commit"}`)
	mock.EnqueueText("Initialized and committed.")

	agent := testagent.NewTestAgent("agent-bitrot", mock, tempDir)
	scenario, _ := testagent.GetScenario("fastapi-react-tf")
	_, err = agent.RunScenario(context.Background(), *scenario, framework.LoopOptions{MaxSteps: 5})
	if err != nil {
		t.Fatalf("baseline run failed: %v", err)
	}

	// Intentionally corrupt an object in .cosm/objects/
	objectsDir := filepath.Join(tempDir, ".cosm", "objects")
	corrupted := false
	_ = filepath.Walk(objectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".bin" || corrupted {
			return nil
		}
		// Flip first byte
		data, rErr := os.ReadFile(path)
		if rErr == nil && len(data) > 0 {
			data[0] ^= 0xFF
			_ = os.WriteFile(path, data, 0644)
			corrupted = true
		}
		return nil
	})

	if !corrupted {
		t.Fatalf("failed to tamper with any object in .cosm/objects")
	}

	// Run Oracle
	oracle := rater.NewDeepVerificationOracle()
	audit, err := oracle.AuditWorkspace(tempDir, "universe-main")
	if err != nil {
		t.Fatalf("oracle audit failed: %v", err)
	}

	if !audit.StorageReport.BitRotDetected {
		t.Errorf("expected BitRotDetected == true")
	}
	if audit.StorageReport.CorruptedObjectsCount < 1 {
		t.Errorf("expected at least 1 corrupted object reported")
	}

	scorecard := rater.CalculateScorecard(audit)
	if scorecard.Dimensions[rater.DimStorage].Passed {
		t.Errorf("storage dimension should fail when bit-rot is detected")
	}
}

// Test 5: Terraform Security & IAM Governance
func TestIntegration_TerraformSecurityAndIAMGovernance(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fg-sec-integration-*")
	if err != nil {
		t.Fatalf("failed creating temp workspace: %v", err)
	}
	defer os.RemoveAll(tempDir)

	_ = os.MkdirAll(filepath.Join(tempDir, "infra"), 0755)

	// Insecure Terraform config with 0.0.0.0/0 ingress and roles/owner
	insecureTF := `terraform {
  required_version = ">= 1.5.0"
}

resource "google_compute_firewall" "insecure_ingress" {
  name    = "allow-all-ingress"
  network = "default"
  allow {
    protocol = "tcp"
    ports    = ["0-65535"]
  }
  source_ranges = ["0.0.0.0/0"]
}

resource "google_project_iam_member" "overprivileged_owner" {
  project = "fg-sec-test"
  role    = "roles/owner"
  member  = "serviceAccount:test-sa@fg-sec-test.iam.gserviceaccount.com"
}
`
	_ = os.WriteFile(filepath.Join(tempDir, "infra/main.tf"), []byte(insecureTF), 0644)

	oracle := rater.NewDeepVerificationOracle()
	audit1, err := oracle.AuditWorkspace(tempDir, "universe-main")
	if err != nil {
		t.Fatalf("security audit failed: %v", err)
	}

	var foundIngress, foundOwner bool
	for _, f := range audit1.Findings {
		if f.Dimension == "SECURITY_IAM" {
			if f.Title == "Unrestricted Ingress CIDR Block (0.0.0.0/0)" {
				foundIngress = true
			}
			if f.Title == "Overprivileged Primitive IAM Role" {
				foundOwner = true
			}
		}
	}

	if !foundIngress || !foundOwner {
		t.Errorf("expected both security findings, foundIngress=%v, foundOwner=%v", foundIngress, foundOwner)
	}

	scorecard1 := rater.CalculateScorecard(audit1)
	if scorecard1.Dimensions[rater.DimSecurity].Passed {
		t.Errorf("security dimension should fail on high severity security violations")
	}

	// Remediate with secure least-privilege configuration
	secureTF := `terraform {
  required_version = ">= 1.5.0"
}

resource "google_compute_firewall" "secure_bastion" {
  name    = "allow-bastion-ingress"
  network = "internal-vpc"
  allow {
    protocol = "tcp"
    ports    = ["443"]
  }
  source_ranges = ["10.0.0.0/16"]
}

resource "google_project_iam_member" "least_privilege" {
  project = "fg-sec-test"
  role    = "roles/run.invoker"
  member  = "serviceAccount:test-sa@fg-sec-test.iam.gserviceaccount.com"
}
`
	_ = os.WriteFile(filepath.Join(tempDir, "infra/main.tf"), []byte(secureTF), 0644)

	audit2, _ := oracle.AuditWorkspace(tempDir, "universe-main")
	for _, f := range audit2.Findings {
		if f.Dimension == "SECURITY_IAM" {
			t.Errorf("unexpected security finding after remediation: %s", f.Title)
		}
	}

	scorecard2 := rater.CalculateScorecard(audit2)
	if !scorecard2.Dimensions[rater.DimSecurity].Passed || scorecard2.Dimensions[rater.DimSecurity].Score < 95.0 {
		t.Errorf("expected 100%% security score after remediation, got %.1f", scorecard2.Dimensions[rater.DimSecurity].Score)
	}
}

// Test 6: Geometric-Style Declarative AST Operations Benchmark
func TestIntegration_DeclarativeASTBenchmark_Geometric(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fg-geometric-ast-*")
	if err != nil {
		t.Fatalf("failed creating temp workspace: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mock := llm.NewMockProvider("mock-gemini-3.7-flash")
	mock.EnqueueToolCall("fg_init", `{"universe_id":"universe-main"}`)
	mock.EnqueueToolCall("fg_add", `{"files":["backend/main.py","frontend/src/App.tsx","infra/main.tf"]}`)
	mock.EnqueueToolCall("fg_commit", `{"intent":"Baseline commit before AST operations"}`)
	mock.EnqueueToolCall("fg_ast_resolve", `{"universe_id":"universe-main","target":"read_root"}`)
	mock.EnqueueToolCall("fg_ast_edit", `{"universe_id":"universe-main","operation":"replace_function_body","target":"read_root","content":"return {\"status\": \"geometric_ast_operational\"}"}`)
	mock.EnqueueToolCall("fg_ast_edit", `{"universe_id":"universe-main","operation":"add_after","target":"read_root","content":"@app.get(\"/api/v1/metrics\")\ndef get_metrics():\n    return {\"nodes\": 42}"}`)
	mock.EnqueueText("Declarative AST edits successfully executed with 100% precision.")

	agent := testagent.NewTestAgent("agent-ast-surgeon", mock, tempDir)
	scenario, err := testagent.GetScenario("fastapi-react-tf")
	if err != nil {
		t.Fatalf("failed fetching scenario: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := agent.RunScenario(ctx, *scenario, framework.LoopOptions{MaxSteps: 10})
	if err != nil {
		t.Fatalf("scenario execution failed: %v", err)
	}
	if session.Status != framework.StatusSuccess {
		t.Fatalf("expected SUCCESS, got %s", session.Status)
	}

	oracle := rater.NewDeepVerificationOracle()
	audit, err := oracle.AuditWorkspace(tempDir, "universe-main")
	if err != nil {
		t.Fatalf("oracle audit failed: %v", err)
	}

	if audit.SyntaxErrorsCount != 0 {
		t.Errorf("expected 0 syntax errors after declarative AST edits, got %d", audit.SyntaxErrorsCount)
	}

	if !audit.StorageReport.MerkleRootMatch || audit.StorageReport.BitRotDetected {
		t.Errorf("storage Merkle root check failed: match=%v, bitRot=%v",
			audit.StorageReport.MerkleRootMatch, audit.StorageReport.BitRotDetected)
	}
}
