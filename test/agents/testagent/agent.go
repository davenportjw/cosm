package testagent

import (
	"context"
	"fmt"
	"os"

	"github.com/cosmscm/cosm/test/agents/framework"
	"github.com/cosmscm/cosm/test/agents/llm"
)

// TestAgent orchestrates autonomous multi-turn test scenario execution.
type TestAgent struct {
	AgentID  string
	Provider llm.LLMProvider
	WorkDir  string
	Model    string
}

// NewTestAgent creates an initialized TestAgent instance.
func NewTestAgent(agentID string, provider llm.LLMProvider, workDir string, model ...string) *TestAgent {
	m := "gemini-3.7-flash"
	if len(model) > 0 && model[0] != "" {
		m = model[0]
	} else if provider != nil && provider.ModelName() != "" {
		m = provider.ModelName()
	}

	if agentID == "" {
		agentID = "cosm-autonomous-test-agent"
	}

	return &TestAgent{
		AgentID:  agentID,
		Provider: provider,
		WorkDir:  workDir,
		Model:    m,
	}
}

// RunScenario executes a polyglot test application scenario within the agent workspace.
func (a *TestAgent) RunScenario(ctx context.Context, scenario Scenario, opts ...framework.LoopOptions) (*framework.AgentSession, error) {
	if err := os.MkdirAll(a.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create agent work directory %s: %w", a.WorkDir, err)
	}

	// Write scenario blueprint files to workspace
	if err := WriteScenarioFiles(a.WorkDir, &scenario); err != nil {
		return nil, fmt.Errorf("failed writing scenario files: %w", err)
	}

	session := framework.NewAgentSession(scenario.Name, a.AgentID, a.Model, a.WorkDir)
	session.AppendUserMessage(fmt.Sprintf("Scenario: %s\nDescription: %s\n\nTask Instructions:\n%s",
		scenario.Name, scenario.Description, scenario.Prompt))

	registry := framework.NewToolRegistry()
	RegisterAllFGTools(registry, a.WorkDir, a.AgentID)

	loopOpts := framework.DefaultLoopOptions()
	if len(opts) > 0 {
		loopOpts = opts[0]
	}
	loopOpts.SystemInstruction = fmt.Sprintf(
		"You are an autonomous Test and Evaluation Agent for Future of Git (fg). "+
			"Execute the required fg tools (fg_init, fg_add, fg_commit, fg_ship, etc.) to successfully stage, "+
			"link cross-boundary AST contracts, and verify the workspace for scenario %q.", scenario.Name)

	return framework.ExecuteLoop(ctx, session, a.Provider, registry, loopOpts)
}

// RunPrompt executes an arbitrary instruction prompt within the agent workspace.
func (a *TestAgent) RunPrompt(ctx context.Context, prompt string, opts ...framework.LoopOptions) (*framework.AgentSession, error) {
	if err := os.MkdirAll(a.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create agent work directory %s: %w", a.WorkDir, err)
	}

	session := framework.NewAgentSession("custom-prompt", a.AgentID, a.Model, a.WorkDir)
	session.AppendUserMessage(prompt)

	registry := framework.NewToolRegistry()
	RegisterAllFGTools(registry, a.WorkDir, a.AgentID)

	loopOpts := framework.DefaultLoopOptions()
	if len(opts) > 0 {
		loopOpts = opts[0]
	}

	return framework.ExecuteLoop(ctx, session, a.Provider, registry, loopOpts)
}
