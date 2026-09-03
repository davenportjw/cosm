package framework

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cosmscm/cosm/test/agents/llm"
)

// Aliases to fundamental LLM types for convenience
type AgentMessage = llm.Message
type ToolDefinition = llm.ToolDefinition
type ToolCall = llm.ToolCall
type ToolResult = llm.ToolResult
type LLMResponse = llm.Response
type TokenUsage = llm.TokenUsage

// SessionStatus represents the operational state of an agent session.
type SessionStatus string

const (
	StatusRunning          SessionStatus = "RUNNING"
	StatusSuccess          SessionStatus = "SUCCESS"
	StatusFailed           SessionStatus = "FAILED"
	StatusMaxStepsExceeded SessionStatus = "MAX_STEPS_EXCEEDED"
	StatusTimeout          SessionStatus = "TIMEOUT"
	StatusError            SessionStatus = "ERROR"
)

// StepTrace captures fine-grained telemetry for a single reasoning & tool execution turn.
type StepTrace struct {
	StepIndex    int           `json:"step_index"`
	Timestamp    time.Time     `json:"timestamp"`
	InputSummary string        `json:"input_summary"`
	ResponseText string        `json:"response_text,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolResults  []ToolResult  `json:"tool_results,omitempty"`
	Reflection   string        `json:"reflection,omitempty"`
	TokensUsed   TokenUsage    `json:"tokens_used"`
	Duration     time.Duration `json:"duration"`
}

// AgentSession encapsulates the persistent context and full execution history of an agent run.
type AgentSession struct {
	SessionID     string                 `json:"session_id"`
	ScenarioName  string                 `json:"scenario_name"`
	AgentID       string                 `json:"agent_id"`
	Model         string                 `json:"model"`
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time"`
	Duration      time.Duration          `json:"duration"`
	Status        SessionStatus          `json:"status"`
	StatusMessage string                 `json:"status_message,omitempty"`
	WorkspaceDir  string                 `json:"workspace_dir"`
	Messages      []AgentMessage         `json:"messages"`
	Traces        []StepTrace            `json:"traces"`
	TotalSteps    int                    `json:"total_steps"`
	TotalTokens   TokenUsage             `json:"total_tokens"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// NewAgentSession initializes a new AgentSession instance.
func NewAgentSession(scenarioName, agentID, model, workDir string) *AgentSession {
	return &AgentSession{
		SessionID:    fmt.Sprintf("session-%d", time.Now().UnixNano()),
		ScenarioName: scenarioName,
		AgentID:      agentID,
		Model:        model,
		StartTime:    time.Now().UTC(),
		Status:       StatusRunning,
		WorkspaceDir: workDir,
		Messages:     make([]AgentMessage, 0),
		Traces:       make([]StepTrace, 0),
		Metadata:     make(map[string]interface{}),
	}
}

// AppendUserMessage appends a user prompt message to the session.
func (s *AgentSession) AppendUserMessage(content string) {
	s.Messages = append(s.Messages, AgentMessage{
		Role:      llm.RoleUser,
		Content:   content,
		Timestamp: time.Now().UTC(),
	})
}

// AppendAssistantMessage appends a model response message to the session.
func (s *AgentSession) AppendAssistantMessage(content string, toolCalls []ToolCall) {
	s.Messages = append(s.Messages, AgentMessage{
		Role:      llm.RoleAssistant,
		Content:   content,
		ToolCalls: toolCalls,
		Timestamp: time.Now().UTC(),
	})
}

// AppendToolResultMessage appends a tool execution result message to the session.
func (s *AgentSession) AppendToolResultMessage(toolCallID, name, output string) {
	s.Messages = append(s.Messages, AgentMessage{
		Role:       llm.RoleTool,
		Name:       name,
		ToolCallID: toolCallID,
		Content:    output,
		Timestamp:  time.Now().UTC(),
	})
}

// RecordStep appends a step trace to the session history and updates cumulative counters.
func (s *AgentSession) RecordStep(trace StepTrace) {
	s.Traces = append(s.Traces, trace)
	s.TotalSteps++
	s.TotalTokens.PromptTokens += trace.TokensUsed.PromptTokens
	s.TotalTokens.CompletionTokens += trace.TokensUsed.CompletionTokens
	s.TotalTokens.TotalTokens += trace.TokensUsed.TotalTokens
}

// Finalize marks the session as finished and computes duration.
func (s *AgentSession) Finalize(status SessionStatus, message string) {
	s.EndTime = time.Now().UTC()
	s.Duration = s.EndTime.Sub(s.StartTime)
	s.Status = status
	s.StatusMessage = message
}

// ToJSON exports the full session as formatted JSON.
func (s *AgentSession) ToJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}
