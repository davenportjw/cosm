package hcl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

const sampleHCL = `
terraform {
  required_version = ">= 1.5.0"
}

provider "google" {
  project = "future-of-git-prod"
  region  = "us-central1"
}

variable "service_name" {
  type    = string
  default = "auth-service"
}

variable "port" {
  type    = string
  default = "8080"
}

resource "google_cloud_run_service" "auth_api" {
  name     = var.service_name
  location = "us-central1"

  template {
    spec {
      containers {
        image = "gcr.io/future-of-git-prod/auth-service:v1"
        env {
          name  = "PORT"
          value = var.port
        }
        env {
          name  = "DATABASE_URL"
          value = "postgres://user:pass@localhost:5432/auth"
        }
      }
    }
  }
}

output "service_url" {
  value = google_cloud_run_service.auth_api.status[0].url
}
`

func TestHCLParser_ParseSource(t *testing.T) {
	parser := NewHCLParser()
	doc, err := parser.ParseSource("main.tf", []byte(sampleHCL))
	if err != nil {
		t.Fatalf("ParseSource failed: %v", err)
	}

	if len(doc.Blocks) != 5 {
		t.Fatalf("expected 5 root blocks, got %d", len(doc.Blocks))
	}

	blockTypes := make(map[string]*HCLBlock)
	for _, b := range doc.Blocks {
		blockTypes[b.FullIdentifier()] = b
	}

	// 1. Verify terraform block
	if _, ok := blockTypes["terraform"]; !ok {
		t.Errorf("missing terraform block")
	}

	// 2. Verify provider block
	if pBlock, ok := blockTypes["provider.google"]; !ok {
		t.Errorf("missing provider.google block")
	} else {
		if pBlock.Attributes["project"].Value != `"future-of-git-prod"` {
			t.Errorf("expected project attr future-of-git-prod, got %s", pBlock.Attributes["project"].Value)
		}
	}

	// 3. Verify variable blocks
	if _, ok := blockTypes["variable.service_name"]; !ok {
		t.Errorf("missing variable.service_name")
	}

	// 4. Verify resource block & env vars & images
	resBlock, ok := blockTypes["resource.google_cloud_run_service.auth_api"]
	if !ok {
		t.Fatalf("missing resource.google_cloud_run_service.auth_api")
	}

	images := resBlock.ContainerImages()
	if len(images) == 0 || images[0] != "gcr.io/future-of-git-prod/auth-service:v1" {
		t.Errorf("expected container image, got %v", images)
	}

	envVars := resBlock.EnvVarBindings()
	if envVars["PORT"] != "var.port" {
		t.Errorf("expected PORT=var.port, got %s", envVars["PORT"])
	}
	if envVars["DATABASE_URL"] != "postgres://user:pass@localhost:5432/auth" {
		t.Errorf("expected DATABASE_URL env var, got %s", envVars["DATABASE_URL"])
	}

	// 5. Convert to AST Symbol Nodes
	lineage := core.LineageEnvelope{
		UserID:     "user-1",
		UserPrompt: "Provision Cloud Run auth service",
		Timestamp:  time.Now().UTC(),
	}

	nodes, err := parser.ToASTSymbolNodes(doc, lineage)
	if err != nil {
		t.Fatalf("ToASTSymbolNodes failed: %v", err)
	}
	if len(nodes) != 5 {
		t.Errorf("expected 5 AST symbol nodes, got %d", len(nodes))
	}

	for _, n := range nodes {
		if n.Language != core.LangHCL {
			t.Errorf("expected LangHCL, got %s", n.Language)
		}
		if n.NodeID == "" {
			t.Errorf("empty NodeID for %s", n.Identifier)
		}
	}

	// 6. Build Component Node
	comp, compNodes, err := parser.BuildComponentNode(doc, "infra-prod", lineage)
	if err != nil {
		t.Fatalf("BuildComponentNode failed: %v", err)
	}
	if comp.Name != "infra-prod" {
		t.Errorf("expected component name infra-prod, got %s", comp.Name)
	}
	if comp.Type != core.CompInfra {
		t.Errorf("expected CompInfra, got %s", comp.Type)
	}
	if len(compNodes) != 5 {
		t.Errorf("expected 5 component symbol nodes, got %d", len(compNodes))
	}
}

func TestHCLFormatter_FormatAndValidate(t *testing.T) {
	unformatted := `
resource "google_cloud_run_service" "api" {
name = "api"
location = "us-central1"
template {
spec {
containers {
image = "gcr.io/app:v1"
}
}
}
}
`

	formatted, err := FormatHCL([]byte(unformatted))
	if err != nil {
		t.Fatalf("FormatHCL failed: %v", err)
	}

	if !strings.Contains(string(formatted), "  name     = \"api\"") {
		t.Errorf("expected aligned '=' in formatted output:\n%s", string(formatted))
	}

	// Structural validation
	if err := ValidateHCL(formatted); err != nil {
		t.Fatalf("ValidateHCL failed on formatted code: %v", err)
	}

	// Invalid HCL syntax test (unclosed brace)
	invalidHCL := `resource "test" "broken" { name = "broken"`
	if err := ValidateHCL([]byte(invalidHCL)); err == nil {
		t.Errorf("expected ValidateHCL to fail on unclosed brace")
	}
}

func TestHCL_TerraformHooks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tf-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tfFile := filepath.Join(tmpDir, "main.tf")
	if err := os.WriteFile(tfFile, []byte(sampleHCL), 0644); err != nil {
		t.Fatalf("failed to write test tf file: %v", err)
	}

	// Test RunTerraformFmt hook (works with or without terraform binary)
	fmtRes, err := RunTerraformFmt(tmpDir, false)
	if err != nil {
		t.Fatalf("RunTerraformFmt failed: %v", err)
	}
	if !fmtRes.Success {
		t.Errorf("expected fmt success")
	}

	// Test RunTerraformValidate hook (works with or without terraform binary)
	valRes, err := RunTerraformValidate(tmpDir)
	if err != nil {
		t.Fatalf("RunTerraformValidate failed: %v", err)
	}
	if !valRes.Valid {
		t.Errorf("expected valid terraform configuration: %v", valRes.Diagnostics)
	}
}
