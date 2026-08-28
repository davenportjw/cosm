package testagent_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
	"github.com/cosmscm/cosm/test/agents/testagent"
)

func TestTestAgent_RunFastAPIScenario(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fg-agent-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mock := llm.NewMockProvider("mock-gemini-3.7-flash")

	// 1. Initialize repo
	mock.EnqueueToolCall("fg_init", `{"universe_id":"universe-main"}`)
	// 2. Stage files
	mock.EnqueueToolCall("fg_add", `{"files":["backend/main.py","frontend/src/App.tsx","infra/main.tf"]}`)
	// 3. Commit universe head
	mock.EnqueueToolCall("fg_commit", `{"intent":"Ingest FastAPI polyglot application"}`)
	// 4. Ship preview
	mock.EnqueueToolCall("fg_ship", `{"universe_id":"universe-main"}`)
	// 5. Final summary
	mock.EnqueueText("FastAPI + React + TF application successfully ingested, linked, and verified.")

	agent := testagent.NewTestAgent("agent-test", mock, tempDir)
	scenario, err := testagent.GetScenario("fastapi-react-tf")
	if err != nil {
		t.Fatalf("failed getting scenario: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := agent.RunScenario(ctx, *scenario, framework.LoopOptions{
		MaxSteps: 10,
	})
	if err != nil {
		t.Fatalf("scenario execution failed: %v", err)
	}

	if session.Status != framework.StatusSuccess {
		t.Errorf("expected session status SUCCESS, got %s", session.Status)
	}

	// Verify files written to disk
	if _, err := os.Stat(filepath.Join(tempDir, "backend/main.py")); err != nil {
		t.Errorf("backend/main.py not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".cosm/objects")); err != nil {
		t.Errorf(".cosm/objects not created: %v", err)
	}

	if session.TotalSteps < 5 {
		t.Errorf("expected at least 5 steps, got %d", session.TotalSteps)
	}
}

func TestListAndGetScenarios(t *testing.T) {
	scenarios := testagent.ListScenarios()
	if len(scenarios) < 3 {
		t.Errorf("expected at least 3 scenarios, got %d", len(scenarios))
	}

	s, err := testagent.GetScenario("go-gin-vue-tf")
	if err != nil || s == nil {
		t.Fatalf("failed getting go-gin-vue-tf: %v", err)
	}
	if len(s.Files) == 0 {
		t.Errorf("expected files in scenario")
	}

	_, err = testagent.GetScenario("non-existent-scenario")
	if err == nil {
		t.Errorf("expected error on non-existent scenario")
	}
}
