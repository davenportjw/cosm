package framework

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cosmscm/cosm/test/agents/llm"
)

// ToolHandler executes a specific tool invocation.
type ToolHandler func(ctx context.Context, call ToolCall) (string, error)

type toolEntry struct {
	definition ToolDefinition
	handler    ToolHandler
}

// ToolRegistry manages registered tools and handles execution dispatch.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]toolEntry
}

// NewToolRegistry creates an empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]toolEntry),
	}
}

// Register registers a tool definition and corresponding handler.
func (r *ToolRegistry) Register(def ToolDefinition, handler ToolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[def.Name] = toolEntry{
		definition: def,
		handler:    handler,
	}
}

// GetDefinitions returns the list of all registered tool schemas.
func (r *ToolRegistry) GetDefinitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]ToolDefinition, 0, len(r.tools))
	for _, entry := range r.tools {
		defs = append(defs, entry.definition)
	}
	return defs
}

// Execute dispatches a ToolCall to its registered handler and returns a ToolResult.
func (r *ToolRegistry) Execute(ctx context.Context, call ToolCall) ToolResult {
	r.mu.RLock()
	entry, exists := r.tools[call.Name]
	r.mu.RUnlock()

	start := time.Now()
	if !exists {
		return ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Output:     "",
			Error:      fmt.Sprintf("unknown tool: %q", call.Name),
			Success:    false,
			Duration:   time.Since(start),
		}
	}

	output, err := entry.handler(ctx, call)
	duration := time.Since(start)

	if err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Output:     output,
			Error:      err.Error(),
			Success:    false,
			Duration:   duration,
		}
	}

	return ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Output:     output,
		Success:    true,
		Duration:   duration,
	}
}

// LoopOptions defines execution parameters for the autonomous agent loop.
type LoopOptions struct {
	MaxSteps                int
	SystemInstruction       string
	Temperature             *float64
	ReflectOnError          bool
	KeepRunningWithoutTools bool
	OnStepCallback          func(trace StepTrace)
}

// DefaultLoopOptions returns default loop execution options.
func DefaultLoopOptions() LoopOptions {
	temp := 0.2
	return LoopOptions{
		MaxSteps:          25,
		SystemInstruction: "You are an autonomous test and evaluation engineer for Cosm (cosm). Use available tools to complete the task.",
		Temperature:       &temp,
		ReflectOnError:    true,
	}
}

// ExecuteLoop coordinates the autonomous multi-turn ReAct reasoning and tool-calling execution loop.
func ExecuteLoop(
	ctx context.Context,
	session *AgentSession,
	provider llm.LLMProvider,
	registry *ToolRegistry,
	opts ...LoopOptions,
) (*AgentSession, error) {
	options := DefaultLoopOptions()
	if len(opts) > 0 {
		userOpts := opts[0]
		if userOpts.MaxSteps > 0 {
			options.MaxSteps = userOpts.MaxSteps
		}
		if userOpts.SystemInstruction != "" {
			options.SystemInstruction = userOpts.SystemInstruction
		}
		if userOpts.Temperature != nil {
			options.Temperature = userOpts.Temperature
		}
		options.ReflectOnError = userOpts.ReflectOnError
		options.KeepRunningWithoutTools = userOpts.KeepRunningWithoutTools
		if userOpts.OnStepCallback != nil {
			options.OnStepCallback = userOpts.OnStepCallback
		}
	}

	if session == nil {
		return nil, fmt.Errorf("agent session cannot be nil")
	}
	if provider == nil {
		return nil, fmt.Errorf("llm provider cannot be nil")
	}
	if registry == nil {
		registry = NewToolRegistry()
	}

	toolDefs := registry.GetDefinitions()

	for step := 1; step <= options.MaxSteps; step++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			session.Finalize(StatusTimeout, fmt.Sprintf("Execution context canceled or timed out at step %d: %v", step, ctx.Err()))
			return session, ctx.Err()
		default:
		}

		stepStart := time.Now()

		genOpts := llm.GenerateOptions{
			Model:             session.Model,
			SystemInstruction: options.SystemInstruction,
			Temperature:       options.Temperature,
		}

		// Call LLM
		resp, err := provider.Generate(ctx, session.Messages, toolDefs, genOpts)
		if err != nil {
			session.Finalize(StatusError, fmt.Sprintf("LLM Generation failed at step %d: %v", step, err))
			return session, err
		}

		// Record assistant message
		session.AppendAssistantMessage(resp.Content, resp.ToolCalls)

		trace := StepTrace{
			StepIndex:    step,
			Timestamp:    stepStart,
			InputSummary: fmt.Sprintf("Turn %d with %d messages in context", step, len(session.Messages)-1),
			ResponseText: resp.Content,
			ToolCalls:    resp.ToolCalls,
			TokensUsed:   resp.Usage,
		}

		// If no tool calls and KeepRunningWithoutTools is false (default), we've reached completion
		if !resp.HasToolCalls() {
			trace.Duration = time.Since(stepStart)
			session.RecordStep(trace)
			if options.OnStepCallback != nil {
				options.OnStepCallback(trace)
			}

			if !options.KeepRunningWithoutTools {
				session.Finalize(StatusSuccess, "Autonomous agent finished all tasks without further tool calls.")
				return session, nil
			}
			continue
		}

		// Execute tool calls
		var toolResults []ToolResult
		var reflectionNotes string

		for _, call := range resp.ToolCalls {
			res := registry.Execute(ctx, call)
			toolResults = append(toolResults, res)

			// Determine payload to feed back to LLM
			var resultPayload string
			if res.Success {
				resultPayload = res.Output
				if resultPayload == "" {
					resultPayload = `{"status":"ok"}`
				}
			} else {
				resultPayload = fmt.Sprintf(`{"status":"error","error":%q,"output":%q}`, res.Error, res.Output)
				if options.ReflectOnError {
					reflectionNotes += fmt.Sprintf("Tool %s failed: %s. Reflect on root cause and attempt corrective action.\n", call.Name, res.Error)
				}
			}

			session.AppendToolResultMessage(call.ID, call.Name, resultPayload)
		}

		// If reflection notes were generated, append guidance user message
		if reflectionNotes != "" && options.ReflectOnError {
			trace.Reflection = reflectionNotes
			session.AppendUserMessage(fmt.Sprintf("[System Reflection Feedback]:\n%s", reflectionNotes))
		}

		trace.ToolResults = toolResults
		trace.Duration = time.Since(stepStart)
		session.RecordStep(trace)

		if options.OnStepCallback != nil {
			options.OnStepCallback(trace)
		}
	}

	// Max steps exceeded
	session.Finalize(StatusMaxStepsExceeded, fmt.Sprintf("Reached maximum step limit (%d) without explicit completion.", options.MaxSteps))
	return session, nil
}
