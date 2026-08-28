package topocosm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/distributed"
)

type inProcessRoundTripper struct {
	handler http.Handler
}

func (rt *inProcessRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	rt.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// HubClient provides client SDK bindings for interacting with topocosm.dev or a local hub server.
type HubClient struct {
	baseURL    string
	callerDID  string
	httpClient *http.Client
	handler    http.Handler
}

// NewHubClient creates a new HTTP network client connecting to a remote Topocosm Hub.
func NewHubClient(baseURL, callerDID string) *HubClient {
	if callerDID == "" {
		callerDID = "did:key:z6MkuAnonymousDev"
	}
	return &HubClient{
		baseURL:    baseURL,
		callerDID:  callerDID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewInProcessHubClient creates a zero-latency in-process client against an HTTP handler.
func NewInProcessHubClient(handler http.Handler, callerDID string) *HubClient {
	if callerDID == "" {
		callerDID = "did:key:z6MkuInProcessDev"
	}
	return &HubClient{
		baseURL:    "http://in-process.topocosm.local",
		callerDID:  callerDID,
		httpClient: &http.Client{Transport: &inProcessRoundTripper{handler: handler}},
		handler:    handler,
	}
}

// Handler returns the underlying handler if in-process.
func (c *HubClient) Handler() http.Handler {
	return c.handler
}

// Close terminates any active in-process test servers.
func (c *HubClient) Close() {
}

func (c *HubClient) doRequest(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request failed: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Cosm-DID", c.callerDID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBytes))
	}

	if out != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, out); err != nil {
			return fmt.Errorf("unmarshal response failed: %w (raw: %s)", err, string(respBytes))
		}
	}

	return nil
}

// GetAgentDiscoveryManifest retrieves the machine-readable manifest at /.well-known/cosm-agent.json.
func (c *HubClient) GetAgentDiscoveryManifest(ctx context.Context) (*AgentDiscoveryManifest, error) {
	var manifest AgentDiscoveryManifest
	if err := c.doRequest(ctx, http.MethodGet, "/.well-known/cosm-agent.json", nil, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// GetHealth returns system status and uptime.
func (c *HubClient) GetHealth(ctx context.Context) (map[string]any, error) {
	var res map[string]any
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/health", nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetStats returns platform metrics.
func (c *HubClient) GetStats(ctx context.Context) (*HubStats, error) {
	var stats HubStats
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/stats", nil, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// Enroll registers a new user or agent DID.
func (c *HubClient) Enroll(ctx context.Context, did, username, email, displayName, pubKey string, isAgent bool) error {
	req := map[string]any{
		"did":          did,
		"username":     username,
		"email":        email,
		"display_name": displayName,
		"public_key":   pubKey,
		"is_agent":     isAgent,
	}
	return c.doRequest(ctx, http.MethodPost, "/api/v1/auth/enroll", req, nil)
}

// WhoAmI retrieves the authenticated identity.
func (c *HubClient) WhoAmI(ctx context.Context) (map[string]any, error) {
	var res map[string]any
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/auth/whoami", nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// CreateCosm registers a new Cosm repository.
func (c *HubClient) CreateCosm(ctx context.Context, orgSlug, cosmName, desc string, visibility Visibility) (*CosmRepository, error) {
	req := map[string]any{
		"org_slug":    orgSlug,
		"cosm_name":   cosmName,
		"description": desc,
		"visibility":  visibility,
	}
	var repo CosmRepository
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/cosms", req, &repo); err != nil {
		return nil, err
	}
	return &repo, nil
}

// ListCosms lists all available cosms.
func (c *HubClient) ListCosms(ctx context.Context) ([]*CosmRepository, error) {
	var list []*CosmRepository
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/cosms", nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetCosm retrieves details of a specific cosm.
func (c *HubClient) GetCosm(ctx context.Context, orgSlug, cosmName string) (*CosmRepository, error) {
	var repo CosmRepository
	path := fmt.Sprintf("/api/v1/cosms/%s/%s", orgSlug, cosmName)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &repo); err != nil {
		return nil, err
	}
	return &repo, nil
}

// PublishCosm streams and commits an AST DAG to Topocosm Hub.
func (c *HubClient) PublishCosm(ctx context.Context, payload *PublishPayload) (*PublishResponse, error) {
	var resp PublishResponse
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/publish", payload.OrgSlug, payload.CosmName)
	if err := c.doRequest(ctx, http.MethodPost, path, payload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PullCosm pulls the full manifest and all AST blobs for a universe.
func (c *HubClient) PullCosm(ctx context.Context, orgSlug, cosmName, universeID string) (*SparsePullResponse, error) {
	var resp SparsePullResponse
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/pull?universe=%s", orgSlug, cosmName, universeID)
	if err := c.doRequest(ctx, http.MethodPost, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SparsePullCosm performs agent-filtered AST subtree extraction.
func (c *HubClient) SparsePullCosm(ctx context.Context, req *SparsePullRequest) (*SparsePullResponse, error) {
	var resp SparsePullResponse
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/sparse-pull", req.OrgSlug, req.CosmName)
	if err := c.doRequest(ctx, http.MethodPost, path, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateProposal opens a new Universe Proposal (PR equivalent).
func (c *HubClient) CreateProposal(ctx context.Context, orgSlug, cosmName, title, srcUni, tgtUni string, lineage core.LineageEnvelope) (*distributed.ProposalCOB, error) {
	req := map[string]any{
		"title":           title,
		"source_universe": srcUni,
		"target_universe": tgtUni,
		"author_did":      c.callerDID,
		"lineage":         lineage,
	}
	var cob distributed.ProposalCOB
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/proposals", orgSlug, cosmName)
	if err := c.doRequest(ctx, http.MethodPost, path, req, &cob); err != nil {
		return nil, err
	}
	return &cob, nil
}

// ListProposals returns all active proposals for a cosm.
func (c *HubClient) ListProposals(ctx context.Context, orgSlug, cosmName string) ([]*distributed.ProposalCOB, error) {
	var list []*distributed.ProposalCOB
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/proposals", orgSlug, cosmName)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetProposal retrieves a single proposal by ID.
func (c *HubClient) GetProposal(ctx context.Context, orgSlug, cosmName, propID string) (*distributed.ProposalCOB, error) {
	var cob distributed.ProposalCOB
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/proposals/%s", orgSlug, cosmName, propID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &cob); err != nil {
		return nil, err
	}
	return &cob, nil
}

// SubmitReview submits human or critic approval/comments on a proposal.
func (c *HubClient) SubmitReview(ctx context.Context, orgSlug, cosmName string, review *ReviewSubmission) (*distributed.ProposalCOB, error) {
	var cob distributed.ProposalCOB
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/proposals/%s/review", orgSlug, cosmName, review.ProposalID)
	if err := c.doRequest(ctx, http.MethodPost, path, review, &cob); err != nil {
		return nil, err
	}
	return &cob, nil
}

// MergeProposal merges a proposal into its target universe head.
func (c *HubClient) MergeProposal(ctx context.Context, orgSlug, cosmName, propID string) (map[string]any, error) {
	var res map[string]any
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/proposals/%s/merge", orgSlug, cosmName, propID)
	if err := c.doRequest(ctx, http.MethodPost, path, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// ClaimDomain acquires an agent lease on a domain.
func (c *HubClient) ClaimDomain(ctx context.Context, orgSlug, cosmName, domain, goal string, ttlSec int) (bool, error) {
	req := BlackboardClaimRequest{
		OrgSlug:     orgSlug,
		CosmName:    cosmName,
		Domain:      domain,
		AgentDID:    c.callerDID,
		Goal:        goal,
		LeaseTTLSec: ttlSec,
	}
	var res map[string]any
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/blackboard/claim", orgSlug, cosmName)
	if err := c.doRequest(ctx, http.MethodPost, path, req, &res); err != nil {
		return false, err
	}
	return res["success"] == true, nil
}

// ReleaseDomain releases an agent lease on a domain.
func (c *HubClient) ReleaseDomain(ctx context.Context, orgSlug, cosmName, domain string) error {
	req := BlackboardReleaseRequest{
		OrgSlug:  orgSlug,
		CosmName: cosmName,
		Domain:   domain,
		AgentDID: c.callerDID,
	}
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/blackboard/release", orgSlug, cosmName)
	return c.doRequest(ctx, http.MethodPost, path, req, nil)
}

// GetBlackboard returns active domain claims and goals.
func (c *HubClient) GetBlackboard(ctx context.Context, orgSlug, cosmName string) (map[string]any, error) {
	var res map[string]any
	path := fmt.Sprintf("/api/v1/cosms/%s/%s/blackboard", orgSlug, cosmName)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}
