package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/cosmscm/cosm/pkg/mutation"
)

// AgentClient provides programmatic Go client bindings for agent swarms.
type AgentClient struct {
	baseURL    string
	httpClient *http.Client
	handler    http.Handler // Optional in-process handler fallback
}

// NewAgentClient creates a client connecting to a remote HTTP agent server.
func NewAgentClient(baseURL string) *AgentClient {
	return &AgentClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NewInProcessAgentClient creates an in-process direct memory client without network hops.
func NewInProcessAgentClient(handler http.Handler) *AgentClient {
	return &AgentClient{
		handler: handler,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// MutateNode posts an AST mutation to the SCM backend.
func (c *AgentClient) MutateNode(req *MutateNodeRequest) (*MutateNodeResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if c.handler != nil {
		httpReq, _ := http.NewRequest(http.MethodPost, "/api/v1/mutate", bytes.NewReader(data))
		httpReq.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c.handler.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusOK {
			return nil, fmt.Errorf("in-process request failed (%d): %s", rec.Code, rec.Body.String())
		}

		var resp MutateNodeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &resp, nil
	}

	url := fmt.Sprintf("%s/api/v1/mutate", c.baseURL)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned non-200 code: %d", resp.StatusCode)
	}

	var res MutateNodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &res, nil
}

// GetUniverse queries metadata for a universe.
func (c *AgentClient) GetUniverse(universeID string) (*QueryUniverseResponse, error) {
	path := fmt.Sprintf("/api/v1/universe/%s", universeID)
	if c.handler != nil {
		httpReq, _ := http.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		c.handler.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusOK {
			return nil, fmt.Errorf("in-process query failed (%d): %s", rec.Code, rec.Body.String())
		}

		var resp QueryUniverseResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			return nil, fmt.Errorf("failed to decode query response: %w", err)
		}
		return &resp, nil
	}

	url := fmt.Sprintf("%s%s", c.baseURL, path)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http query failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server error: %d", resp.StatusCode)
	}

	var res QueryUniverseResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &res, nil
}

// ApplyASTEditBatch applies a declarative AST mutation batch via HTTP or in-process.
func (c *AgentClient) ApplyASTEditBatch(batch *mutation.ASTEditBatch) (*mutation.BatchMutationResult, error) {
	data, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch: %w", err)
	}

	if c.handler != nil {
		httpReq, _ := http.NewRequest(http.MethodPost, "/api/v1/ast/edit", bytes.NewReader(data))
		httpReq.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c.handler.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusOK {
			return nil, fmt.Errorf("in-process AST edit failed (%d): %s", rec.Code, rec.Body.String())
		}

		var resp mutation.BatchMutationResult
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &resp, nil
	}

	url := fmt.Sprintf("%s/api/v1/ast/edit", c.baseURL)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned non-200 code: %d", resp.StatusCode)
	}

	var res mutation.BatchMutationResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &res, nil
}

// ResolveSymbol resolves a symbol in a universe by identifier or scoped path.
func (c *AgentClient) ResolveSymbol(universeID, target string) (*mutation.ResolvedSymbol, error) {
	reqPayload := map[string]string{
		"universe_id": universeID,
		"target":      target,
	}
	data, _ := json.Marshal(reqPayload)

	if c.handler != nil {
		httpReq, _ := http.NewRequest(http.MethodPost, "/api/v1/ast/resolve", bytes.NewReader(data))
		httpReq.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c.handler.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusOK {
			return nil, fmt.Errorf("in-process resolve failed (%d): %s", rec.Code, rec.Body.String())
		}

		var resp mutation.ResolvedSymbol
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &resp, nil
	}

	url := fmt.Sprintf("%s/api/v1/ast/resolve", c.baseURL)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned non-200 code: %d", resp.StatusCode)
	}

	var res mutation.ResolvedSymbol
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &res, nil
}
