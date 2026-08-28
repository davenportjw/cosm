package llm

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockCallRecord records the arguments of a single Generate call.
type MockCallRecord struct {
	Timestamp time.Time
	Messages  []Message
	Tools     []ToolDefinition
	Options   GenerateOptions
}

// MockProvider provides deterministic, scriptable LLM responses for hermetic testing.
type MockProvider struct {
	mu            sync.Mutex
	model         string
	responses     []*Response
	responseIndex int
	customHandler func(ctx context.Context, messages []Message, tools []ToolDefinition, opts GenerateOptions) (*Response, error)
	recordedCalls []MockCallRecord
	autoDefault   bool
}

// NewMockProvider creates an empty MockProvider with default model.
func NewMockProvider(model ...string) *MockProvider {
	m := "mock-gemini-3.7-flash"
	if len(model) > 0 && model[0] != "" {
		m = model[0]
	}
	return &MockProvider{
		model:         m,
		responses:     make([]*Response, 0),
		recordedCalls: make([]MockCallRecord, 0),
		autoDefault:   true,
	}
}

// ModelName returns the mock model name.
func (m *MockProvider) ModelName() string {
	return m.model
}

// EnqueueResponse adds a predetermined response to the FIFO queue.
func (m *MockProvider) EnqueueResponse(resp *Response) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, resp)
	return m
}

// EnqueueText adds a simple text response to the queue.
func (m *MockProvider) EnqueueText(content string) *MockProvider {
	return m.EnqueueResponse(&Response{
		Content:      content,
		FinishReason: "STOP",
		Model:        m.model,
		Usage: TokenUsage{
			PromptTokens:     50,
			CompletionTokens: 30,
			TotalTokens:      80,
		},
	})
}

// EnqueueToolCall adds a tool execution request response to the queue.
func (m *MockProvider) EnqueueToolCall(name string, argsJSON string) *MockProvider {
	return m.EnqueueResponse(&Response{
		ToolCalls: []ToolCall{
			{
				ID:        fmt.Sprintf("call_%s_%d", name, time.Now().UnixNano()),
				Name:      name,
				Arguments: argsJSON,
			},
		},
		FinishReason: "TOOL_CALLS",
		Model:        m.model,
		Usage: TokenUsage{
			PromptTokens:     40,
			CompletionTokens: 25,
			TotalTokens:      65,
		},
	})
}

// EnqueueMultiToolCalls adds multiple tool execution requests in a single response turn.
func (m *MockProvider) EnqueueMultiToolCalls(calls []ToolCall) *MockProvider {
	return m.EnqueueResponse(&Response{
		ToolCalls:    calls,
		FinishReason: "TOOL_CALLS",
		Model:        m.model,
		Usage: TokenUsage{
			PromptTokens:     60,
			CompletionTokens: 40,
			TotalTokens:      100,
		},
	})
}

// SetCustomHandler assigns a dynamic function to compute responses on the fly.
func (m *MockProvider) SetCustomHandler(fn func(ctx context.Context, messages []Message, tools []ToolDefinition, opts GenerateOptions) (*Response, error)) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customHandler = fn
	return m
}

// SetAutoDefault configures whether to return a default completion when the queue is exhausted.
func (m *MockProvider) SetAutoDefault(enabled bool) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoDefault = enabled
	return m
}

// RecordedCalls returns a slice of all calls intercepted by this mock provider.
func (m *MockProvider) RecordedCalls() []MockCallRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]MockCallRecord, len(m.recordedCalls))
	copy(copied, m.recordedCalls)
	return copied
}

// CallCount returns the total number of calls received.
func (m *MockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.recordedCalls)
}

// Reset clears the recorded calls and resets the response index.
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseIndex = 0
	m.recordedCalls = nil
}

// Generate implements LLMProvider.
func (m *MockProvider) Generate(ctx context.Context, messages []Message, tools []ToolDefinition, opts GenerateOptions) (*Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Record call
	m.recordedCalls = append(m.recordedCalls, MockCallRecord{
		Timestamp: time.Now(),
		Messages:  messages,
		Tools:     tools,
		Options:   opts,
	})

	// Custom handler takes precedence if set
	if m.customHandler != nil {
		return m.customHandler(ctx, messages, tools, opts)
	}

	// Replay from scripted queue
	if m.responseIndex < len(m.responses) {
		resp := m.responses[m.responseIndex]
		m.responseIndex++
		if resp.Model == "" {
			resp.Model = m.model
		}
		return resp, nil
	}

	if m.autoDefault {
		// Provide a graceful termination response
		return &Response{
			Content:      "Task completed successfully.",
			FinishReason: "STOP",
			Model:        m.model,
			Usage: TokenUsage{
				PromptTokens:     20,
				CompletionTokens: 10,
				TotalTokens:      30,
			},
		}, nil
	}

	return nil, fmt.Errorf("mock provider: no more scripted responses in queue (exhausted %d responses)", len(m.responses))
}
