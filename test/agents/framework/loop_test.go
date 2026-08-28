package framework_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
)

func TestExecuteLoop_Success(t *testing.T) {
	mock := llm.NewMockProvider("mock-gemini-3.7-flash")
	mock.SetAutoDefault(false)

	// Step 1: Agent calls fg_init
	mock.EnqueueToolCall("fg_init", `{"universe_id":"u-main"}`)
	// Step 2: Agent calls fg_commit
	mock.EnqueueToolCall("fg_commit", `{"intent":"Initial commit"}`)
	// Step 3: Agent finishes with final summary
	mock.EnqueueText("All actions completed successfully.")

	registry := framework.NewToolRegistry()
	initCalled := false
	commitCalled := false

	registry.Register(llm.ToolDefinition{
		Name:        "fg_init",
		Description: "Initialize repository",
	}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		initCalled = true
		return `{"status":"initialized"}`, nil
	})

	registry.Register(llm.ToolDefinition{
		Name:        "fg_commit",
		Description: "Commit manifest",
	}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		commitCalled = true
		return `{"status":"committed","merkle_root":"abc1234"}`, nil
	})

	session := framework.NewAgentSession("test-polyglot", "agent-007", "gemini-3.7-flash", "/tmp/test-workdir")
	session.AppendUserMessage("Build and initialize the repository")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	completedSession, err := framework.ExecuteLoop(ctx, session, mock, registry, framework.LoopOptions{
		MaxSteps: 10,
	})

	if err != nil {
		t.Fatalf("unexpected error in ExecuteLoop: %v", err)
	}

	if completedSession.Status != framework.StatusSuccess {
		t.Errorf("expected status SUCCESS, got %s", completedSession.Status)
	}

	if !initCalled || !commitCalled {
		t.Errorf("expected both tools to be called, init: %v, commit: %v", initCalled, commitCalled)
	}

	if completedSession.TotalSteps != 3 {
		t.Errorf("expected 3 steps in traces, got %d", completedSession.TotalSteps)
	}

	if completedSession.TotalTokens.TotalTokens <= 0 {
		t.Errorf("expected non-zero token metrics")
	}
}

func TestExecuteLoop_ErrorReflection(t *testing.T) {
	mock := llm.NewMockProvider("mock-gemini-3.7-flash")
	mock.SetAutoDefault(false)

	// Step 1: Agent calls faulty tool
	mock.EnqueueToolCall("faulty_tool", `{}`)
	// Step 2: Agent reacts to reflection feedback and fixes action
	mock.EnqueueText("Recognized tool error and recovered.")

	registry := framework.NewToolRegistry()
	registry.Register(llm.ToolDefinition{
		Name: "faulty_tool",
	}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		return "", errors.New("simulated disk error")
	})

	session := framework.NewAgentSession("test-reflection", "agent-fixer", "gemini-3.7-flash", "/tmp/test")
	session.AppendUserMessage("Execute faulty action")

	completedSession, err := framework.ExecuteLoop(context.Background(), session, mock, registry, framework.LoopOptions{
		MaxSteps:       5,
		ReflectOnError: true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if completedSession.Status != framework.StatusSuccess {
		t.Errorf("expected status SUCCESS, got %s", completedSession.Status)
	}

	if completedSession.Traces[0].Reflection == "" {
		t.Errorf("expected reflection note recorded on step 1")
	}
}

func TestExecuteLoop_MaxStepsExceeded(t *testing.T) {
	mock := llm.NewMockProvider("mock-gemini-3.7-flash")
	mock.SetAutoDefault(false)

	// Enqueue 5 continuous tool calls
	for i := 0; i < 5; i++ {
		mock.EnqueueToolCall("dummy_tool", `{}`)
	}

	registry := framework.NewToolRegistry()
	registry.Register(llm.ToolDefinition{Name: "dummy_tool"}, func(ctx context.Context, call llm.ToolCall) (string, error) {
		return `{"ok":true}`, nil
	})

	session := framework.NewAgentSession("test-max-steps", "agent-loop", "gemini-3.7-flash", "/tmp/test")
	session.AppendUserMessage("Run loop")

	completedSession, err := framework.ExecuteLoop(context.Background(), session, mock, registry, framework.LoopOptions{
		MaxSteps: 3, // Limit to 3 steps
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if completedSession.Status != framework.StatusMaxStepsExceeded {
		t.Errorf("expected MAX_STEPS_EXCEEDED, got %s", completedSession.Status)
	}
}
