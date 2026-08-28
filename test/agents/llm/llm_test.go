package llm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/cosmscm/cosm/test/agents/llm"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestMockProvider_QueueAndRecording(t *testing.T) {
	mock := llm.NewMockProvider("mock-gemini-3.7-flash")

	// Enqueue tool call followed by final text
	mock.EnqueueToolCall("fg_init", `{"universe_id":"universe-test"}`)
	mock.EnqueueText("Initialized workspace")

	ctx := context.Background()
	tools := []llm.ToolDefinition{
		{
			Name:        "fg_init",
			Description: "Initialize repo",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"universe_id": map[string]interface{}{"type": "string"},
				},
			},
		},
	}

	// First call -> returns tool call
	resp1, err := mock.Generate(ctx, []llm.Message{{Role: llm.RoleUser, Content: "Initialize repo"}}, tools, llm.GenerateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp1.HasToolCalls() || len(resp1.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp1.ToolCalls))
	}
	if resp1.ToolCalls[0].Name != "fg_init" {
		t.Errorf("expected tool fg_init, got %s", resp1.ToolCalls[0].Name)
	}

	var args map[string]string
	if err := resp1.ToolCalls[0].ParseArguments(&args); err != nil || args["universe_id"] != "universe-test" {
		t.Errorf("failed parsing tool arguments: %v, args: %v", err, args)
	}

	// Second call -> returns text
	resp2, err := mock.Generate(ctx, []llm.Message{
		{Role: llm.RoleUser, Content: "Initialize repo"},
		{Role: llm.RoleAssistant, ToolCalls: resp1.ToolCalls},
		{Role: llm.RoleTool, Name: "fg_init", Content: `{"success":true}`},
	}, tools, llm.GenerateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp2.Content != "Initialized workspace" {
		t.Errorf("expected 'Initialized workspace', got %q", resp2.Content)
	}

	// Third call -> autoDefault fallback
	resp3, err := mock.Generate(ctx, []llm.Message{}, tools, llm.GenerateOptions{})
	if err != nil {
		t.Fatalf("unexpected error on autoDefault: %v", err)
	}
	if resp3.Content == "" {
		t.Errorf("expected default response content")
	}

	// Assert recording
	if mock.CallCount() != 3 {
		t.Errorf("expected 3 recorded calls, got %d", mock.CallCount())
	}
	records := mock.RecordedCalls()
	if len(records) != 3 {
		t.Fatalf("expected 3 recorded calls")
	}
	if records[0].Tools[0].Name != "fg_init" {
		t.Errorf("expected recorded tool definition")
	}
}

func TestGeminiProvider_HTTPRoundTrip(t *testing.T) {
	mockTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("key") != "test-api-key" {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(`{"error":{"code":401,"message":"Unauthorized"}}`)),
				Header:     make(http.Header),
			}, nil
		}

		responseJSON := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"role": "model",
						"parts": []map[string]interface{}{
							{
								"functionCall": map[string]interface{}{
									"name": "fg_add",
									"args": map[string]interface{}{
										"files": []string{"main.go"},
									},
								},
							},
						},
					},
					"finishReason": "TOOL_CALLS",
				},
			},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":     120,
				"candidatesTokenCount": 45,
				"totalTokenCount":      165,
			},
		}

		data, _ := json.Marshal(responseJSON)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBuffer(data)),
			Header:     make(http.Header),
		}, nil
	})

	httpClient := &http.Client{
		Transport: mockTransport,
		Timeout:   5 * time.Second,
	}

	provider, err := llm.NewGeminiProvider(llm.GeminiConfig{
		APIKey:     "test-api-key",
		Model:      "gemini-3.7-flash",
		BaseURL:    "https://generativelanguage.googleapis.com/v1beta",
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("failed to create Gemini provider: %v", err)
	}

	if provider.ModelName() != "gemini-3.7-flash" {
		t.Errorf("expected gemini-3.7-flash, got %s", provider.ModelName())
	}

	ctx := context.Background()
	resp, err := provider.Generate(ctx, []llm.Message{
		{Role: llm.RoleUser, Content: "Add main.go"},
	}, []llm.ToolDefinition{
		{
			Name:        "fg_add",
			Description: "Stage files",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"files": map[string]interface{}{
						"type":  "array",
						"items": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}, llm.GenerateOptions{
		SystemInstruction: "You are an autonomous test agent.",
	})

	if err != nil {
		t.Fatalf("failed generate call: %v", err)
	}
	if !resp.HasToolCalls() {
		t.Fatalf("expected tool call in response")
	}
	if resp.ToolCalls[0].Name != "fg_add" {
		t.Errorf("expected fg_add tool call, got %s", resp.ToolCalls[0].Name)
	}
	if resp.Usage.TotalTokens != 165 {
		t.Errorf("expected 165 tokens, got %d", resp.Usage.TotalTokens)
	}
}
