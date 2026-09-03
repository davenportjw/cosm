# Cosm Agent Commit Contract & Lineage/Telemetry Specification

This document provides the canonical contract and interface definition for autonomous AI agents, tool orchestrators, LLM gateways, and CI/CD runners interacting with the Cosm AST Source Control System during a commit or AST mutation.

---

## 1. Architectural Problem & Cosm Primitives

### 1.1 The File-Diff Attribution Dilemma
Traditional VCS tools (Git) record commits as monolithic file blobs and unstructured commit messages. External telemetry tools (such as Entire.io, Datadog LLM Observability, or OpenTelemetry gateways) attempt to reconstruct commit provenance post-hoc by matching commit timestamps to span traces or file diffs. This creates severe attribution gaps:
1. **Sub-File Ambiguity**: Multiple agents modifying different functions in the same file lose granular AST symbol attribution.
2. **Disconnected Token Metrics**: Token consumption, model temperature, prompt traces, and financial cost are stored in external APM silos, decoupled from the cryptographic code Merkle-DAG.
3. **No Non-Repudiation**: Code changes lack cryptographic attestations tying the exact prompt and LLM parameters to the resulting AST nodes.

### 1.2 Cosm's Content-Addressed Merkle Lineage
Cosm embeds causal provenance and token telemetry directly inside each content-addressed `ASTSymbolNode` and `WorkspaceManifestNode`. Every commit cryptographically binds:
- The exact **AST symbol graph delta** ($\Delta \text{AST}$).
- The causal chain: $\text{User Prompt} \rightarrow \text{Session ID} \rightarrow \text{Orchestrator} \rightarrow \text{Executing Agent} \rightarrow \text{LLM Model \& Params}$.
- The **Token Telemetry**: Prompt, completion, reasoning/thinking, and cached token counts, cost in USD, latency, and W3C distributed trace context (`traceparent`).

$$\text{ManifestHash} = \text{SHA256}\Big(\text{UniverseID} \parallel \sum H(\text{Component}_i) \parallel \sum H(\text{Edge}_j) \parallel H(\text{LineageEnvelope})\Big)$$

---

## 2. Core Schema & Contract Definitions

### 2.1 Contract Tiers

| Field Tier | Field Name | Type | Description |
| :--- | :--- | :--- | :--- |
| **Required** | `universe_id` | `string` | Target micro-universe (e.g., `universe-main`, `u/feat-auth`) |
| **Required** | `intent` | `string` | Human/agent-readable summary of the intent behind the change |
| **Required** | `executing_agent_id`| `string` | Unique identifier or DID of the executing AI agent |
| **Required** | `user_prompt` | `string` | High-level natural language prompt or agent directive |
| **Optional / Recommended** | `session_id` | `string` | Unique conversation or workflow session UUID |
| **Optional / Recommended** | `llm_version` | `string` | Exact model version identifier (e.g., `gemini-3.7-flash`, `claude-3-7-sonnet`) |
| **Optional / Recommended** | `generation_params` | `string` | JSON string of inference parameters (`temperature`, `top_p`, `seed`) |
| **Optional / Recommended** | `tokens` | `TokenTelemetry` | Token consumption metrics and cost calculation |
| **Optional / Recommended** | `trace` | `TraceCarrier` | W3C Distributed Tracing context (`trace_id`, `span_id`) |
| **Optional** | `signature_ed25519` | `bytes` | Cryptographic signature of the agent attesting to the AST payload |
| **Auto-Computed** | `timestamp` | `RFC3339` | UTC timestamp of the commit / mutation |
| **Auto-Computed** | `merkle_root_hash` | `SHA-256` | Root hash of the resulting immutable workspace manifest |

---

### 2.2 Go Type Specifications (`pkg/core/schema.go`)

```go
// TokenTelemetry captures LLM token consumption, latency, and cost telemetry.
type TokenTelemetry struct {
	PromptTokens     int64   `json:"prompt_tokens,omitempty"`
	CompletionTokens int64   `json:"completion_tokens,omitempty"`
	ReasoningTokens  int64   `json:"reasoning_tokens,omitempty"`
	CachedTokens     int64   `json:"cached_tokens,omitempty"`
	TotalTokens      int64   `json:"total_tokens,omitempty"`
	CostUSD          float64 `json:"cost_usd,omitempty"`
	LatencyMs        int64   `json:"latency_ms,omitempty"`
	TTFTMs           int64   `json:"ttft_ms,omitempty"` // Time-To-First-Token
}

// TraceCarrier carries W3C Trace Context and OpenTelemetry span correlation.
type TraceCarrier struct {
	TraceID      string            `json:"trace_id,omitempty"`
	SpanID       string            `json:"span_id,omitempty"`
	TraceFlags   string            `json:"trace_flags,omitempty"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	TraceState   string            `json:"trace_state,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// LineageEnvelope stores multi-tier causal pedigree and verification metadata.
type LineageEnvelope struct {
	UserID              string          `json:"user_id,omitempty"`
	UserPrompt          string          `json:"user_prompt,omitempty"`
	SessionID           string          `json:"session_id,omitempty"`
	OrchestratorAgentID string          `json:"orchestrator_agent_id,omitempty"`
	ExecutingAgentID    string          `json:"executing_agent_id"`
	LLMVersion          string          `json:"llm_version,omitempty"`
	GenerationParams    string          `json:"generation_params,omitempty"`
	Intent              string          `json:"intent"`
	Timestamp           time.Time       `json:"timestamp"`
	Tokens              TokenTelemetry  `json:"tokens,omitempty"`
	Trace               TraceCarrier    `json:"trace,omitempty"`
	SignatureEd25519    []byte          `json:"signature_ed25519,omitempty"`
}
```

---

## 3. Interaction Modalities for Autonomous Agents

Agents can execute commits and AST mutations through three distinct interfaces:

```
                  ┌──────────────────────────────────────────────┐
                  │          Autonomous AI Agent Swarm           │
                  └───────┬──────────────┬──────────────┬────────┘
                          │              │              │
             CLI Flags    │   HTTP REST  │    In-Process│ Go Client
                          ▼              ▼              ▼
                    ┌───────────┐  ┌───────────┐  ┌───────────┐
                    │ cosm CLI  │  │ /api/v1/  │  │ AgentSDK  │
                    └─────┬─────┘  └─────┬─────┘  └─────┬─────┘
                          │              │              │
                          ▼              ▼              ▼
                  ┌──────────────────────────────────────────────┐
                  │    Cosm AST Storage & SQLite WAL Engine      │
                  │   (.cosm/objects/ + .cosm/graph.db)          │
                  └──────────────────────────────────────────────┘
```

---

### Modality 1: CLI Interface (`cosm commit`, `cosm add`, `cosm ast edit`)

When an agent executes shell commands, it provides telemetry flags:

```bash
# Staging files with agent provenance
cosm add src/auth/jwt.go \
  -u universe-feat-auth \
  -a agent-auth-builder \
  -p "Implement RS256 token verification" \
  -i "Add JWT validation function" \
  --session-id "sess-4491-a8b2" \
  -m "gemini-3.7-flash" \
  --prompt-tokens 1200 \
  --completion-tokens 340 \
  --reasoning-tokens 512 \
  --cost-usd 0.0014 \
  --trace-id "4bf92f3577b34da6a3ce929d0e0e4736" \
  --span-id "00f067aa0ba902b7"

# Committing workspace state with Merkle DAG recalculation
cosm commit \
  -u universe-feat-auth \
  -i "feat: JWT RS256 signature verification" \
  -a agent-auth-builder \
  -p "Implement RS256 token verification" \
  --session-id "sess-4491-a8b2" \
  --orchestrator-id "orchestrator-lead-01" \
  -m "gemini-3.7-flash" \
  --prompt-tokens 2450 \
  --completion-tokens 680 \
  --reasoning-tokens 1024 \
  --cost-usd 0.0031 \
  --latency-ms 840 \
  --trace-id "4bf92f3577b34da6a3ce929d0e0e4736" \
  --span-id "00f067aa0ba902b7"
```

---

### Modality 2: HTTP / REST API (`POST /api/v1/commit`)

For HTTP-based agents and microservices running against a local or remote Cosm daemon:

#### Request
```http
POST /api/v1/commit HTTP/1.1
Host: localhost:9090
Content-Type: application/json

{
  "universe_id": "universe-feat-auth",
  "intent": "Implement RS256 token verification",
  "lineage": {
    "user_id": "alice@company.com",
    "user_prompt": "Add RS256 token validation with error propagation",
    "session_id": "sess-4491-a8b2-c103",
    "orchestrator_agent_id": "orch-orchestrator-alpha",
    "executing_agent_id": "cosm-worker-auth-1",
    "llm_version": "gemini-3.7-flash",
    "generation_params": "{\"temperature\": 0.2, \"seed\": 42}",
    "intent": "Implement RS256 token verification",
    "tokens": {
      "prompt_tokens": 1420,
      "completion_tokens": 312,
      "reasoning_tokens": 850,
      "cached_tokens": 128,
      "total_tokens": 2582,
      "cost_usd": 0.00184,
      "latency_ms": 640
    },
    "trace": {
      "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
      "span_id": "00f067aa0ba902b7",
      "trace_flags": "01",
      "attributes": {
        "gen_ai.system": "gemini",
        "agent.role": "auth-engineer"
      }
    }
  }
}
```

#### Response (`200 OK`)
```json
{
  "universe_id": "universe-feat-auth",
  "merkle_root_hash": "658a10ba9570fa1f188f09dfd2af5c1b5a743240c2c6c659aef3e3c9719cab50",
  "component_count": 3,
  "success": true,
  "message": "Successfully committed workspace manifest to universe"
}
```

---

### Modality 3: Go Client SDK (`pkg/api/client.go`)

In-process Go client execution (0ms network latency):

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmscm/cosm/pkg/api"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func CommitWithTelemetry(universeMgr *storage.UniverseManager, blobStore *storage.BlobStore, graphEngine *storage.GraphEngine) error {
	server := api.NewAgentAPIServer(universeMgr, blobStore, graphEngine)
	client := api.NewInProcessAgentClient(server.Handler())

	resp, err := client.CommitWorkspace(&api.CommitWorkspaceRequest{
		UniverseID: "universe-main",
		Intent:     "Refactor database connection pool",
		Lineage: core.LineageEnvelope{
			UserID:              "developer-bob",
			UserPrompt:          "Increase DB connection pool size and configure read replicas",
			SessionID:           "sess-993821aa",
			OrchestratorAgentID: "agent-swarm-orchestrator",
			ExecutingAgentID:    "cosm-db-specialist",
			LLMVersion:          "gemini-3.7-flash",
			Intent:              "Database pool optimization",
			Timestamp:           time.Now().UTC(),
			Tokens: core.TokenTelemetry{
				PromptTokens:     890,
				CompletionTokens: 240,
				ReasoningTokens:  410,
				TotalTokens:      1540,
				CostUSD:          0.00095,
				LatencyMs:        480,
			},
			Trace: core.TraceCarrier{
				TraceID: "e4893bc4a0e98a21764b8a2e1d0943ba",
				SpanID:  "12984abce942013f",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	fmt.Printf("Committed to Universe '%s' (Merkle Hash: %s)\n", resp.UniverseID, resp.MerkleRootHash[:16])
	return nil
}
```

---

## 4. Querying Lineage & Telemetry

Once committed, lineage and token telemetry can be queried via:
1. **Lineage Tracer** (`pkg/lineage/tracer.go`): `TraceNodeAncestry(nodeID)` returns full ancestry hops with tokens and trace carrier for every symbol.
2. **Blast Radius & Audit Engine** (`pkg/lineage/audit.go`): Find all symbols modified by a specific `session_id`, `llm_version`, or `trace_id`.
3. **Graph Engine SQL WAL** (`.cosm/graph.db`):
   ```sql
   SELECT node_id, user_prompt, executing_agent_id, llm_version, timestamp 
   FROM lineage_records 
   WHERE session_id = 'sess-4491-a8b2'
   ORDER BY timestamp ASC;
   ```
