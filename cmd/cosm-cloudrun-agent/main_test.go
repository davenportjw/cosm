package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
)

func TestReplayMockBenchmarkSessions(t *testing.T) {
	sessions := replayMockBenchmarkSessions(10)
	if len(sessions) != 10 {
		t.Fatalf("expected 10 sessions, got %d", len(sessions))
	}

	expectedRoles := []string{
		"bootstrap", "backend", "backend", "backend",
		"backend", "backend", "backend", "backend",
		"backend", "backend",
	}

	for i, sess := range sessions {
		expectedID := fmt.Sprintf("cloudrun-session-%s-%d", expectedRoles[i], i)
		if sess.SessionID != expectedID {
			t.Errorf("session[%d] ID = %s, want %s", i, sess.SessionID, expectedID)
		}
		if sess.Status != framework.StatusSuccess {
			t.Errorf("session[%d] Status = %s, want SUCCESS", i, sess.Status)
		}
		if len(sess.Traces) == 0 {
			t.Errorf("session[%d] has no step traces", i)
		}
	}
}

func TestLoadWorkerSessions_Empty(t *testing.T) {
	tempDir := t.TempDir()
	sessions, err := loadWorkerSessions(tempDir)
	if err != nil {
		t.Fatalf("unexpected error loading empty sessions dir: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadWorkerSessions_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Write three session files out of order: task 10, task 2, task 0
	taskIndices := []int{10, 2, 0}
	for _, idx := range taskIndices {
		sess := &framework.AgentSession{
			SessionID:    fmt.Sprintf("test-sess-%d", idx),
			ScenarioName: "cloudrun-polyglot-mesh-100",
			AgentID:      fmt.Sprintf("agent-backend-%d", idx),
			Model:        "gemini-3.8-flash",
			StartTime:    time.Now().UTC(),
			EndTime:      time.Now().UTC().Add(time.Second),
			Duration:     time.Second,
			Status:       framework.StatusSuccess,
			TotalSteps:   5,
			TotalTokens:  framework.TokenUsage{TotalTokens: 500},
			Messages:     make([]framework.AgentMessage, 0),
			Traces: []framework.StepTrace{
				{
					StepIndex: 1,
					ToolCalls: []llm.ToolCall{{Name: "cosm_commit"}},
				},
			},
		}

		data, err := json.MarshalIndent(sess, "", "  ")
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}

		filePath := filepath.Join(tempDir, fmt.Sprintf("session-task-%d.json", idx))
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			t.Fatalf("write file error: %v", err)
		}
	}

	loaded, err := loadWorkerSessions(tempDir)
	if err != nil {
		t.Fatalf("unexpected error loading sessions: %v", err)
	}

	if len(loaded) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(loaded))
	}

	// Verify numerical sort order: task 0, task 2, task 10
	expectedOrder := []string{"test-sess-0", "test-sess-2", "test-sess-10"}
	for i, sess := range loaded {
		if sess.SessionID != expectedOrder[i] {
			t.Errorf("loaded[%d] ID = %s, want %s", i, sess.SessionID, expectedOrder[i])
		}
	}
}

func TestFatalErrorDirectiveMessage(t *testing.T) {
	sessionsDir := "/tmp/cosm-sessions"
	expectedMsg := fmt.Sprintf("no genuine worker sessions found in %s: live judge requires authentic execution sessions from completed workers (STRICT NEVER MOCK DIRECTIVE)", sessionsDir)
	want := "no genuine worker sessions found in /tmp/cosm-sessions: live judge requires authentic execution sessions from completed workers (STRICT NEVER MOCK DIRECTIVE)"
	if expectedMsg != want {
		t.Errorf("error message mismatch: got %q, want %q", expectedMsg, want)
	}
}
