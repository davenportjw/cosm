package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	DefaultGeminiModel   = "gemini-3.8-flash"
	DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	DefaultVertexBaseURL = "https://aiplatform.googleapis.com/v1"
)

// GeminiConfig holds options for initializing a GeminiProvider.
type GeminiConfig struct {
	APIKey     string
	ProjectID  string
	Location   string
	OAuthToken string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
	Timeout    time.Duration
	UseVertex  bool
}

// GeminiProvider implements LLMProvider for the Google Gemini API (supporting both AI Studio API keys and Google Cloud Project Vertex AI Auth).
type GeminiProvider struct {
	apiKey      string
	projectID   string
	location    string
	oauthToken  string
	useVertex   bool
	model       string
	baseURL     string
	httpClient  *http.Client
	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// NewGeminiProvider creates a new GeminiProvider using environment, Google Cloud Project Auth (Vertex AI), or explicit configuration.
func NewGeminiProvider(cfg ...GeminiConfig) (*GeminiProvider, error) {
	var c GeminiConfig
	if len(cfg) > 0 {
		c = cfg[0]
	}

	apiKey := c.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}

	model := c.Model
	if model == "" {
		model = os.Getenv("GEMINI_MODEL")
	}
	if model == "" {
		model = DefaultGeminiModel
	}

	useVertex := c.UseVertex || apiKey == "" || os.Getenv("GOOGLE_GENAI_USE_VERTEXAI") == "true"
	var (
		projectID  string
		location   string
		oauthToken string
		baseURL    string
	)

	if useVertex {
		projectID = c.ProjectID
		if projectID == "" {
			projectID = os.Getenv("VERTEX_PROJECT")
		}
		if projectID == "" {
			projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
		}
		if projectID == "" {
			projectID = os.Getenv("GCP_PROJECT_ID")
		}
		if projectID == "" {
			projectID = os.Getenv("PROJECT_ID")
		}
		if projectID == "" {
			// Try metadata server (Cloud Run / GCE)
			req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/project/project-id", nil)
			req.Header.Set("Metadata-Flavor", "Google")
			client := &http.Client{Timeout: 500 * time.Millisecond}
			if resp, err := client.Do(req); err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				if b, err := io.ReadAll(resp.Body); err == nil && len(b) > 0 {
					projectID = strings.TrimSpace(string(b))
				}
			}
		}
		if projectID == "" {
			out, err := exec.Command("gcloud", "config", "get-value", "project").Output()
			if err == nil && len(out) > 0 {
				projectID = strings.TrimSpace(string(out))
			}
		}
		if projectID == "" {
			projectID = "davenport-boutique"
		}

		location = c.Location
		if location == "" {
			location = os.Getenv("VERTEX_LOCATION")
		}
		if location == "" {
			location = os.Getenv("GOOGLE_CLOUD_LOCATION")
		}
		if location == "" {
			location = "us" // Default multi-region location hosting Gemini 3.8 Flash
		}

		oauthToken = c.OAuthToken
		baseURL = c.BaseURL
		if baseURL == "" {
			baseURL = DefaultVertexBaseURL
		}
	} else {
		baseURL = c.BaseURL
		if baseURL == "" {
			baseURL = DefaultGeminiBaseURL
		}
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		timeout := c.Timeout
		if timeout == 0 {
			timeout = 90 * time.Second
		}
		httpClient = &http.Client{
			Timeout: timeout,
		}
	}

	return &GeminiProvider{
		apiKey:     apiKey,
		projectID:  projectID,
		location:   location,
		oauthToken: oauthToken,
		useVertex:  useVertex,
		model:      model,
		baseURL:    baseURL,
		httpClient: httpClient,
	}, nil
}

// getToken retrieves a valid Google Cloud OAuth bearer token using metadata server, environment, or gcloud CLI.
func (p *GeminiProvider) getToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.oauthToken != "" {
		return p.oauthToken, nil
	}

	if p.cachedToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.cachedToken, nil
	}

	// 1. Check environment variable
	if envTok := os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN"); envTok != "" {
		p.cachedToken = envTok
		p.tokenExpiry = time.Now().Add(5 * time.Minute)
		return envTok, nil
	}

	// 2. Check metadata server (Cloud Run / Compute Engine / GKE)
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	req.Header.Set("Metadata-Flavor", "Google")
	client := &http.Client{Timeout: 800 * time.Millisecond}
	if resp, err := client.Do(req); err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var metaToken struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&metaToken); err == nil && metaToken.AccessToken != "" {
			p.cachedToken = metaToken.AccessToken
			expSec := metaToken.ExpiresIn - 60
			if expSec < 60 {
				expSec = 60
			}
			p.tokenExpiry = time.Now().Add(time.Duration(expSec) * time.Second)
			return p.cachedToken, nil
		}
	}

	// 3. Check local gcloud ADC (Application Default Credentials) first
	if out, err := exec.CommandContext(ctx, "gcloud", "auth", "application-default", "print-access-token").Output(); err == nil && len(out) > 0 {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if strings.HasPrefix(line, "ya29.") {
				p.cachedToken = line
				p.tokenExpiry = time.Now().Add(10 * time.Minute)
				return line, nil
			}
		}
	}

	// 4. Fallback to standard gcloud CLI access token
	if out, err := exec.CommandContext(ctx, "gcloud", "auth", "print-access-token").Output(); err == nil && len(out) > 0 {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if strings.HasPrefix(line, "ya29.") {
				p.cachedToken = line
				p.tokenExpiry = time.Now().Add(10 * time.Minute)
				return line, nil
			}
		}
	}

	return "", fmt.Errorf("could not resolve Google Cloud OAuth token from environment, metadata server, or gcloud CLI")
}

// ModelName returns the configured default model.
func (p *GeminiProvider) ModelName() string {
	return p.model
}

// Gemini API request & response types

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiToolDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []geminiToolDeclaration `json:"functionDeclarations,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type geminiRequest struct {
	SystemInstruction *geminiSystemInstruction `json:"systemInstruction,omitempty"`
	Contents          []geminiContent          `json:"contents"`
	Tools             []geminiTool             `json:"tools,omitempty"`
	GenerationConfig  *geminiGenerationConfig  `json:"generationConfig,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
	Error         *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// Generate executes a completion request against Google Gemini REST API or Google Cloud Vertex AI.
func (p *GeminiProvider) Generate(ctx context.Context, messages []Message, tools []ToolDefinition, opts GenerateOptions) (*Response, error) {
	if !p.useVertex && p.apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable or Google Cloud project auth is required")
	}

	model := opts.Model
	if model == "" {
		model = p.model
	}

	reqBody := &geminiRequest{}

	// System Instruction
	if opts.SystemInstruction != "" {
		reqBody.SystemInstruction = &geminiSystemInstruction{
			Parts: []geminiPart{
				{Text: opts.SystemInstruction},
			},
		}
	}

	// Tools / Function Declarations
	if len(tools) > 0 {
		var decls []geminiToolDeclaration
		for _, t := range tools {
			decls = append(decls, geminiToolDeclaration{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  cleanParametersSchema(t.Parameters),
			})
		}
		reqBody.Tools = []geminiTool{
			{FunctionDeclarations: decls},
		}
	}

	// Generation Config
	reqBody.GenerationConfig = &geminiGenerationConfig{
		Temperature:     opts.Temperature,
		TopP:            opts.TopP,
		MaxOutputTokens: opts.MaxTokens,
		StopSequences:   opts.StopSequences,
	}

	// Message History
	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			if reqBody.SystemInstruction == nil {
				reqBody.SystemInstruction = &geminiSystemInstruction{
					Parts: []geminiPart{{Text: msg.Content}},
				}
			} else {
				reqBody.SystemInstruction.Parts = append(reqBody.SystemInstruction.Parts, geminiPart{Text: msg.Content})
			}

		case RoleUser:
			reqBody.Contents = append(reqBody.Contents, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: msg.Content}},
			})

		case RoleAssistant:
			var parts []geminiPart
			if msg.Content != "" {
				parts = append(parts, geminiPart{Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var argsMap map[string]interface{}
				if tc.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Arguments), &argsMap)
				}
				if argsMap == nil {
					argsMap = make(map[string]interface{})
				}
				parts = append(parts, geminiPart{
					FunctionCall: &geminiFunctionCall{
						Name: tc.Name,
						Args: argsMap,
					},
				})
			}
			if len(parts) == 0 {
				parts = append(parts, geminiPart{Text: ""})
			}
			reqBody.Contents = append(reqBody.Contents, geminiContent{
				Role:  "model",
				Parts: parts,
			})

		case RoleTool:
			respMap := map[string]interface{}{
				"result": msg.Content,
			}
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Content), &parsed); err == nil {
				respMap = parsed
			}
			reqBody.Contents = append(reqBody.Contents, geminiContent{
				Role: "user",
				Parts: []geminiPart{
					{
						FunctionResponse: &geminiFunctionResponse{
							Name:     msg.Name,
							Response: respMap,
						},
					},
				},
			})
		}
	}

	reqPayload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Gemini request payload: %w", err)
	}

	var url string
	if p.useVertex {
		url = fmt.Sprintf("%s/projects/%s/locations/%s/publishers/google/models/%s:generateContent", p.baseURL, p.projectID, p.location, model)
	} else {
		url = fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.baseURL, model, p.apiKey)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to build HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.useVertex {
		token, err := p.getToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed resolving Google Cloud auth token: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini API request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Gemini response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini API error (HTTP %d): %s", httpResp.StatusCode, string(respBytes))
	}

	var gResp geminiResponse
	if err := json.Unmarshal(respBytes, &gResp); err != nil {
		return nil, fmt.Errorf("failed to decode Gemini JSON response: %w", err)
	}

	if gResp.Error != nil {
		return nil, fmt.Errorf("gemini API error (%d - %s): %s", gResp.Error.Code, gResp.Error.Status, gResp.Error.Message)
	}

	res := &Response{
		Model:      model,
		RawPayload: string(respBytes),
	}

	if gResp.UsageMetadata != nil {
		res.Usage = TokenUsage{
			PromptTokens:     gResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: gResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gResp.UsageMetadata.TotalTokenCount,
		}
	}

	if len(gResp.Candidates) > 0 {
		cand := gResp.Candidates[0]
		res.FinishReason = cand.FinishReason

		var textParts []string
		for i, part := range cand.Content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				res.ToolCalls = append(res.ToolCalls, ToolCall{
					ID:        fmt.Sprintf("call_%s_%d_%d", part.FunctionCall.Name, time.Now().UnixNano(), i),
					Name:      part.FunctionCall.Name,
					Arguments: string(argsJSON),
				})
			}
		}
		res.Content = strings.Join(textParts, "\n")
	}

	return res, nil
}

// cleanParametersSchema cleans up unsupported json-schema keywords for Gemini function calling.
func cleanParametersSchema(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return map[string]interface{}{
			"type": "object",
		}
	}
	cleaned := make(map[string]interface{})
	for k, v := range params {
		if k == "$schema" || k == "additionalProperties" {
			continue
		}
		if nested, ok := v.(map[string]interface{}); ok {
			cleaned[k] = cleanParametersSchema(nested)
		} else {
			cleaned[k] = v
		}
	}
	return cleaned
}
