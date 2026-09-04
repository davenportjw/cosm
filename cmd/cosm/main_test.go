package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveHubTarget(t *testing.T) {
	tests := []struct {
		input    string
		wantHub  string
		wantOrg  string
		wantCosm string
	}{
		{
			input:    "",
			wantHub:  "http://127.0.0.1:51204",
			wantOrg:  "demo-org",
			wantCosm: "cloud-platform",
		},
		{
			input:    "my-app",
			wantHub:  "http://127.0.0.1:51204",
			wantOrg:  "demo-org",
			wantCosm: "my-app",
		},
		{
			input:    "acme/service",
			wantHub:  "http://127.0.0.1:51204",
			wantOrg:  "acme",
			wantCosm: "service",
		},
		{
			input:    "http://127.0.0.1:51204/acme/service",
			wantHub:  "http://127.0.0.1:51204",
			wantOrg:  "acme",
			wantCosm: "service",
		},
		{
			input:    "https://topocosm.dev/my-org/core-platform",
			wantHub:  "https://topocosm.dev",
			wantOrg:  "my-org",
			wantCosm: "core-platform",
		},
	}

	for _, tt := range tests {
		hub, org, cosm := resolveHubTarget(tt.input)
		if hub != tt.wantHub || org != tt.wantOrg || cosm != tt.wantCosm {
			t.Errorf("resolveHubTarget(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.input, hub, org, cosm, tt.wantHub, tt.wantOrg, tt.wantCosm)
		}
	}
}

func TestCLIBlackboardCommandsWithMockServer(t *testing.T) {
	claims := make(map[string]map[string]any)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/cosms/demo-org/cloud-platform/blackboard/claim":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			domain := req["domain"].(string)
			claims[domain] = req
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"domain":  domain,
			})
		case "/api/v1/cosms/demo-org/cloud-platform/blackboard/release":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			domain := req["domain"].(string)
			delete(claims, domain)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"domain":  domain,
			})
		case "/api/v1/cosms/demo-org/cloud-platform/blackboard":
			var claimList []map[string]any
			for d, c := range claims {
				claimList = append(claimList, map[string]any{
					"domain":       d,
					"agent_did":    c["agent_did"],
					"goal":         c["goal"],
					"seconds_left": 300,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"claims": claimList,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// 1. Claim domain
	runClaim([]string{"--url", server.URL, "--goal", "Testing unit lease", "services/billing"})

	// 2. View blackboard
	runBlackboard([]string{"--url", server.URL})

	// 3. Release domain
	runRelease([]string{"--url", server.URL, "services/billing"})

	// 4. View blackboard again (empty)
	runBlackboard([]string{"--url", server.URL})
}
