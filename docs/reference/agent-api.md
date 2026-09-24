# Agent API & In-Process SDK Reference

Cosm provides high-speed HTTP REST APIs and zero-latency in-process Go client bindings designed for autonomous AI agent swarms, tool orchestrators, and CI validation runners.

---

## 1. REST API Endpoints (`pkg/api/server.go`)

### `GET /api/v1/health`
Checks server health, uptime, and RFC3339 system timestamp.

- **Request**:
  ```http
  GET /api/v1/health HTTP/1.1
  Host: localhost:9090
  Accept: application/json
  ```
- **Response `200 OK`**:
  ```json
  {
    "status": "ok",
    "time": "2026-08-27T20:50:00Z"
  }
  ```

---

### `GET /api/v1/universe/{universe_id}`
Queries the active state, head Merkle root hash, and component list of a micro-universe.

- **Request**:
  ```http
  GET /api/v1/universe/universe-feat-auth HTTP/1.1
  Host: localhost:9090
  Accept: application/json
  ```
- **Response `200 OK`** (`QueryUniverseResponse`):
  ```json
  {
    "universe_id": "universe-feat-auth",
    "merkle_root_hash": "a4f89d3c1e2b5a6f7890123456789abcdef0123456789abcdef0123456789abc",
    "component_count": 3,
    "components": [
      "comp-7a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b",
      "comp-8b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c",
      "comp-9c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d"
    ]
  }
  ```
- **Error Responses**:
  - `400 Bad Request`: `{"error":"universe_id required"}`
  - `404 Not Found`: `{"error":"universe universe-xyz not found"}`

---

### `POST /api/v1/mutate`
Submits an in-memory AST symbol mutation to be stored in the content-addressed immutable object store.

- **Request**:
  ```http
  POST /api/v1/mutate HTTP/1.1
  Host: localhost:9090
  Content-Type: application/json

  {
    "universe_id": "universe-main",
    "component_id": "comp-auth-service",
    "symbol_node": {
      "language": "go",
      "node_type": "FunctionDecl",
      "identifier": "AuthenticateUser",
      "signature": "func(ctx context.Context, token string) (*User, error)",
      "visibility": "public",
      "ast_payload": "eyJuYW1lIjoiQXV0aGVudGljYXRlVXNlciJ9",
      "local_dependencies": ["ValidateToken", "User"],
      "lineage": {
        "user_id": "user-123",
        "user_prompt": "Refactor authentication to support JWT expiration",
        "session_id": "sess-456",
        "executing_agent_id": "agent-auth-builder",
        "llm_version": "gemini-3.7-flash",
        "intent": "Add token expiration check",
        "timestamp": "2026-08-27T20:50:00Z"
      }
    }
  }
  ```
- **Response `200 OK`** (`MutateNodeResponse`):
  ```json
  {
    "node_id": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "universe_id": "universe-main",
    "success": true,
    "message": "AST node successfully persisted in object store"
  }
  ```
- **Error Responses**:
  - `400 Bad Request`: `{"error":"symbol_node is required"}` or JSON decoding error
  - `405 Method Not Allowed`: `{"error":"method not allowed"}`
  - `500 Internal Server Error`: `{"error":"blobstore put failed: ..."}`

---

### `POST /api/v1/commit`
Commits current workspace state into an immutable Merkle root manifest on the specified universe, recording full multi-tier causal lineage, OpenTelemetry W3C trace context, and token consumption metrics.

- **Request**:
  ```http
  POST /api/v1/commit HTTP/1.1
  Host: localhost:9090
  Content-Type: application/json

  {
    "universe_id": "universe-main",
    "intent": "Implement RS256 token verification",
    "lineage": {
      "user_id": "user-123",
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
- **Response `200 OK`** (`CommitWorkspaceResponse`):
  ```json
  {
    "universe_id": "universe-main",
    "merkle_root_hash": "658a10ba9570fa1f188f09dfd2af5c1b5a743240c2c6c659aef3e3c9719cab50",
    "component_count": 3,
    "success": true,
    "message": "Successfully committed workspace manifest to universe"
  }
  ```
- **Error Responses**:
  - `400 Bad Request`: `{"error":"invalid commit payload: ..."}`
  - `405 Method Not Allowed`: `{"error":"method not allowed"}`
  - `500 Internal Server Error`: `{"error":"commit failed: ..."}`

---

### `POST /api/v1/ast/edit`
Applies an atomic batch of declarative AST mutations (`replace_function_body`, `replace_function`, `add_method`, `add_before`, `add_after`, `delete`, `add_import`, `replace_imports`, `replace_global`) directly to AST Merkle DAG nodes.

- **Request**:
  ```http
  POST /api/v1/ast/edit HTTP/1.1
  Host: localhost:9090
  Content-Type: application/json

  {
    "universe_id": "universe-main",
    "operations": [
      {
        "operation": "replace_function_body",
        "target": "BillingService.ProcessPayment",
        "content": "w.WriteHeader(http.StatusOK)\nreturn nil"
      },
      {
        "operation": "add_after",
        "target": "BillingService.ProcessPayment",
        "content": "func (s *BillingService) RefundPayment() error { return nil }"
      }
    ],
    "lineage": {
      "user_prompt": "Add refund support and fix payment handler",
      "executing_agent_id": "agent-ast-surgeon",
      "timestamp": "2026-08-27T21:00:00Z"
    }
  }
  ```
- **Response `200 OK`** (`BatchMutationResult`):
  ```json
  {
    "universe_id": "universe-main",
    "applied_operations": 2,
    "modified_symbols": ["c1a2b3..."],
    "added_symbols": ["d4e5f6..."],
    "deleted_symbols": [],
    "deduplicated_symbols": 8,
    "old_manifest_hash": "a1b2c3d4e5f6...",
    "new_manifest_hash": "f6e5d4c3b2a1...",
    "duration_ns": 4500000
  }
  ```

---

### `POST /api/v1/ast/resolve`
Resolves an AST symbol and its enclosing component by scoped dotted identifier, bare name, or hash prefix.

- **Request**:
  ```http
  POST /api/v1/ast/resolve HTTP/1.1
  Host: localhost:9090
  Content-Type: application/json

  {
    "universe_id": "universe-main",
    "target": "services/billing::BillingService.ProcessPayment"
  }
  ```
- **Response `200 OK`** (`ResolvedSymbol`):
  ```json
  {
    "symbol_node": {
      "node_id": "c1a2b3d4...",
      "language": "go",
      "node_type": "FunctionDecl",
      "identifier": "services/billing::BillingService.ProcessPayment",
      "signature": "func (s *BillingService) ProcessPayment(w http.ResponseWriter, r *http.Request) error"
    },
    "component": {
      "component_id": "comp-billing...",
      "name": "services/billing",
      "language": "go"
    },
    "symbol_index": 2
  }
  ```

---

### `GET /api/v1/ast/tree` & Agent CLI Tree Query
Queries the complete hierarchical AST Merkle Tree projection across components, symbols, and cross-domain contract edges for agent context ingestion and navigation.

#### Agent CLI Invocation
```bash
cosm ast tree [-u universe] [--format text|json]
```

- `-u`, `--universe <id>`: Target micro-universe (defaults to `universe-main`).
- `--format`, `-f <text|json>`: Output format (defaults to `text`). Use `--format json` for structured agent reasoning.

#### Example Agent Invocations
```bash
# Human-readable formatted ASCII hierarchy
cosm ast tree -u universe-main

# Agent JSON ingestion for full symbol Merkle DAG graph
cosm ast tree -u universe-main --format json
```

#### JSON Response Schema (`ASTTreeGraph`)
```json
{
  "universe_id": "universe-main",
  "merkle_root": "673bc252841f2bb3965ed5ba48a1cbd3470e398e136031ab31332edc70916198",
  "manifest_hash": "673bc252841f2bb3965ed5ba48a1cbd3470e398e136031ab31332edc70916198",
  "total_components": 3,
  "total_symbols": 8,
  "total_edges": 4,
  "components": [
    {
      "component_id": "services/auth.go",
      "name": "services/auth.go",
      "language": "go",
      "type": "Backend",
      "symbols": [
        {
          "node_id": "9f83a48e89f81d830b0f924373a4b791",
          "identifier": "ValidateToken",
          "node_type": "FunctionDecl",
          "language": "go",
          "signature": "func ValidateToken(token string) bool",
          "outgoing_edges": [
            {
              "target_id": "services/db.go:QueryUser",
              "edge_type": "CALLS",
              "label": "CALLS QueryUser"
            }
          ],
          "lineage": {
            "user_prompt": "Implement JWT validation logic",
            "executing_agent_id": "gemini-3.8-flash",
            "intent": "feat(auth): validate token signature",
            "timestamp": "2026-09-17T12:00:00Z"
          }
        }
      ]
    }
  ]
}
```

---

### Agent CLI Reversal, History & Ledger Operations

Autonomous agents executing self-healing loops, rollbacks, and history navigation can invoke the following CLI operations:

#### 1. Undo Recent Mutations (`cosm undo`)
Reverses the most recent commit(s) and synchronizes workspace files on disk:
```bash
cosm undo [count] [-u <universe>] [-w|--write-disk=true|false]
```
- **Arguments**: `[count]` specifies the number of commits to unroll (default: `1`).
- `-u, --universe <universe>`: Target micro-universe (default: `universe-main`).
- `-w, --write-disk` (default: `true`): Updates disk files to match restored manifest.
- **Mode Semantics**:
  - In standard mode: unrolls universe head pointer and reverses Oplog events.
  - In ledger mode: automatically creates and appends a forward compensating revert commit with full causal lineage.
- **Exit Code**: `0` on success, `1` on error.

#### 2. Revert Specific Commit (`cosm revert` / `cosm rollback`)
Appends a forward compensating commit that inverts the changes of a specified commit hash:
```bash
cosm revert <target_hash> [-u <universe>] [-i|--intent <msg>] [-w|--write-disk=true|false]
cosm rollback <target_hash> [-u <universe>] [-i|--intent <msg>] [-w|--write-disk=true|false]
```
- `<target_hash>`: Commit Merkle root hash or prefix to reverse.
- `-i, --intent <msg>`: Causal intent description stamped in `LineageEnvelope`.
- `-w, --write-disk` (default: `true`): Synchronizes workspace files on disk.
- **Audit Invariant**: Preserves 100% linear history, appending `ActionRevertManifest` to `.cosm/graph.db`.

#### 3. Reposition Head Pointer (`cosm reset`)
Moves the active micro-universe head pointer directly to a specified target commit hash:
```bash
cosm reset [--hard|--soft] <target_hash> [-u <universe>] [-w|--write-disk=true|false]
```
- `--hard`: Moves head pointer and synchronizes workspace disk files.
- `--soft`: Moves head pointer only, preserving disk working tree.
- **Strict Ledger Mode Invariant**: `cosm reset` is strictly prohibited in repositories initialized with `--ledger` (or `"ledger_mode": true` in `.cosm/config.json`). Fails immediately with exit code `1` and `ErrLedgerLinearityViolation`. In ledger mode, agents must use `cosm revert` to maintain append-only provenance.

#### 4. Strict Ledger Initialization (`cosm init --ledger`)
Initializes the workspace with strict linear append-only constraints:
```bash
cosm init --ledger [-u <universe>]
```
- Writes `"ledger_mode": true` into `.cosm/config.json`.
- Prohibits destructive history rewrites, ensuring all future commits strictly satisfy $C_{N+1} = \text{Commit}(\text{Parent} = C_N, \dots)$.

---

## 2. Ephemeral Preview Sandbox Endpoints (`pkg/target/sandbox.go`)

When launching ephemeral preview sandboxes (`cosm ship` or `pkg/target/sandbox.go`), local HTTP preview servers bind to loopback sockets and serve:

| Route | HTTP Method | Description |
| :--- | :--- | :--- |
| `/healthz` | `GET` | Health check endpoint returning `{"status":"ok","target":"...","time":"..."}` |
| `/_cosm/preview/info` | `GET` | Returns target specification details and materialized file counts |
| `/*` | `GET` | Serves hydrated virtual files directly from the in-memory VFS |

---

## 3. Go Client SDK (`pkg/api/client.go`)

The Cosm client SDK supports both remote network connections and **zero-latency in-process direct memory invocations**.

### In-Process Execution (0ms Overhead for Agent Swarms)

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cosmscm/cosm/pkg/api"
	"github.com/cosmscm/cosm/pkg/core"
	"github.com/cosmscm/cosm/pkg/storage"
)

func main() {
	// 1. Initialize Pure-Go SQLite WAL & Content-Addressed Blob Storage
	blobStore, err := storage.NewBlobStore(".cosm")
	if err != nil {
		log.Fatalf("Failed to init blobstore: %v", err)
	}

	graphEngine, err := storage.NewGraphEngine(".cosm/storage/graph.db")
	if err != nil {
		log.Fatalf("Failed to init graphengine: %v", err)
	}

	universeMgr := storage.NewUniverseManager(blobStore, graphEngine)

	// 2. Initialize In-Process Server & Client (Direct Memory Handlers)
	server := api.NewAgentAPIServer(universeMgr, blobStore, graphEngine)
	client := api.NewInProcessAgentClient(server.Handler())

	// 3. Mutate an AST Node with Cryptographic Lineage
	resp, err := client.MutateNode(&api.MutateNodeRequest{
		UniverseID:  "universe-main",
		ComponentID: "services/billing",
		SymbolNode: &core.ASTSymbolNode{
			Language:   core.LangGo,
			NodeType:   "FunctionDecl",
			Identifier: "ValidateSession",
			Signature:  "func(sessionID string) bool",
			ASTPayload: []byte("func ValidateSession(sessionID string) bool { return true }"),
			Lineage: core.LineageEnvelope{
				UserID:           "user-42",
				UserPrompt:       "Add session validation logic",
				SessionID:        "sess-889",
				ExecutingAgentID: "agent-session-worker",
				LLMVersion:       "gemini-3.7-flash",
				Intent:           "In-process AST surgery",
				Timestamp:        time.Now().UTC(),
			},
		},
	})
	if err != nil {
		log.Fatalf("Mutation failed: %v", err)
	}

	fmt.Printf("Persisted AST Node ID: %s (Success: %t)\n", resp.NodeID, resp.Success)

	// 4. Query Current Universe State
	uniState, err := client.GetUniverse("universe-main")
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}

	fmt.Printf("Universe: %s | Merkle Root: %s | Components: %d\n",
		uniState.UniverseID, uniState.MerkleRootHash, uniState.ComponentCount)
}
```

---

### Remote HTTP Client Initialization

```go
// Connect to a standalone Cosm daemon or remote swarm server
remoteClient := api.NewAgentClient("http://127.0.0.1:9090")

uni, err := remoteClient.GetUniverse("universe-main")
if err != nil {
    log.Fatalf("Remote query failed: %v", err)
}
```

---

## 3. Topocosm Hub SDK (`pkg/topocosm/client.go`)

For distributed multi-cosm operations, agent enrollment, sparse subtree replication, and blackboard lease coordination, use `topocosm.HubClient`:

```go
import "github.com/cosmscm/cosm/pkg/topocosm"

// 1. Initialize client with Ed25519 Agent DID
client := topocosm.NewHubClient("http://127.0.0.1:51204", "did:key:z6MkuAutonomousBuilder")

// 2. Discover hub capabilities and endpoints
manifest, err := client.GetAgentDiscoveryManifest(ctx)

// 3. Perform agent-filtered sparse AST download
pullResp, err := client.SparsePullCosm(ctx, &topocosm.SparsePullRequest{
    OrgSlug:        "demo-org",
    CosmName:       "cloud-platform",
    UniverseID:     "universe-main",
    ComponentNames: []string{"services/billing"},
})

// 4. Coordinate with concurrent agents via Blackboard domain leases
claimed, err := client.ClaimDomain(ctx, "demo-org", "cloud-platform", "services/billing", "Refactoring Stripe handler", 300)
```

See [Topocosm Reference Manual](topocosm.md) for full endpoint specifications.

---

## 4. Autonomous Swarm Concurrency & Edge Locking Protocol

Autonomous swarms interacting with Cosm via HTTP REST or client SDKs follow a strict non-blocking concurrency protocol:

### Wire Protocol DTOs

#### `POST /api/v1/cosms/{org}/{cosm}/blackboard/claim`
Acquires an exclusive mutation lease on a domain or architectural boundary before generating code.

- **Request DTO (`BlackboardClaimRequest`)**:
  ```json
  {
    "domain": "services/billing",
    "agent_did": "did:key:z6MkuAutonomousBuilder",
    "goal": "Refactoring Stripe webhook handler",
    "ttl_seconds": 600
  }
  ```
- **Response `200 OK` (`BlackboardClaimResponse`)**:
  ```json
  {
    "success": true,
    "domain": "services/billing",
    "owner_did": "did:key:z6MkuAutonomousBuilder",
    "expires_at": "2026-09-09T17:40:00Z"
  }
  ```
- **Response `409 Conflict`**:
  ```json
  {
    "success": false,
    "error": "domain currently leased by did:key:z6MkuOtherWorker until 2026-09-09T17:35:12Z"
  }
  ```

#### `POST /api/v1/cosms/{org}/{cosm}/blackboard/release`
Releases an active lease upon task completion or cancellation.

- **Request DTO (`BlackboardReleaseRequest`)**:
  ```json
  {
    "domain": "services/billing",
    "agent_did": "did:key:z6MkuAutonomousBuilder"
  }
  ```

### Complete Go SDK Multi-Agent Concurrency Pattern

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/cosmscm/cosm/pkg/topocosm"
)

func runAgentTask(ctx context.Context, client *topocosm.HubClient) error {
    domain := "services/billing"
    goal := "Upgrade Stripe webhook signature verification"

    // 1. Acquire pre-mutation swarm lock
    claimed, err := client.ClaimDomain(ctx, "demo-org", "cloud-platform", domain, goal, 300)
    if err != nil || !claimed {
        return log.Output(1, "Domain busy, deferring or re-queuing task")
    }
    defer client.ReleaseDomain(ctx, "demo-org", "cloud-platform", domain)

    // 2. Fetch sparse AST subgraph without full repo clone
    subgraph, err := client.SparsePullCosm(ctx, &topocosm.SparsePullRequest{
        OrgSlug:        "demo-org",
        CosmName:       "cloud-platform",
        UniverseID:     "universe-main",
        ComponentNames: []string{domain},
    })
    if err != nil {
        return err
    }

    // 3. Mutate AST in an isolated micro-universe and submit Proposal COB
    // (Mutations in separate micro-universes never block each other)
    return nil
}
```

See [Multi-Agent Concurrency & Edge Locking Guide](../guides/multi-agent-concurrency-and-locking.md) for full operational lifecycle and collision recovery.

---

## 5. Model Context Protocol (MCP) Server (`cosm mcp`)

For AI agents running in Antigravity IDE, Cursor, Claude Code, and Windsurf, Cosm provides a stdio JSON-RPC 2.0 MCP server.

Launch with:
```bash
cosm mcp [--universe universe-main]
```

### Supported MCP Tools

| Tool Name | Parameters | Description |
| :--- | :--- | :--- |
| `cosm_status` | `{ "universe_id"?: string }` | Returns active universe head, staged/unstaged components, and cross-boundary edges. |
| `cosm_ast_resolve` | `{ "target": string, "universe_id"?: string }` | Resolves symbol metadata, type, signature, and SHA-256 node ID. |
| `cosm_ast_edit` | `{ "operation": string, "target": string, "content": string, "universe_id"?: string }` | Executes surgical AST mutation using the 9 Geometric verbs and syncs disk. |
| `cosm_blast_radius` | `{ "node_id": string }` | Calculates downstream contract breakages and impact risk score ($0.0 \dots 1.0$). |
| `cosm_topology` | `{ "format"?: "text" \| "json" \| "mermaid" }` | Outputs the 3-tier cross-domain dependency graph. |
| `cosm_universe_create` | `{ "universe_id": string, "parent"?: string }` | Creates a zero-copy micro-universe branch. |
| `cosm_commit` | `{ "intent": string, "prompt"?: string, "universe_id"?: string }` | Commits staged AST symbols with causal lineage and token telemetry. |
| `cosm_ship` | `{ "target"?: string, "universe_id"?: string }` | Compiles target package and launches ephemeral sandbox preview. |
| `cosm_ast_tree` | `{ "universe_id"?: string, "format"?: "text" \| "json" }` | Returns complete hierarchical AST Merkle tree, components, symbols, and cross-domain contract edges. |

---

## 6. Language Server Protocol (LSP) AST Bridge (`cosm lsp`)

The Cosm LSP server exposes standard `Content-Length` framed JSON-RPC 2.0 endpoints for VS Code, Cursor, Windsurf, and Antigravity IDE.

Launch with:
```bash
cosm lsp [-d <dir>]
```

### Supported LSP Lifecycle & Capabilities
- **Lifecycle**: `initialize` (returns ServerCapabilities with textDocumentSync, definitionProvider, codeLensProvider), `initialized`, `shutdown`, `exit`.
- **Diagnostics (`textDocument/publishDiagnostics`)**:
  On `textDocument/didSave`, the server parses the updated document into AST symbols, runs `core.DetectContractBreakages` across cross-boundary edges (e.g. backend route removed while frontend API client still references it), and emits LSP diagnostic errors pinpointing exact line and column locations.
- **Definition Provider (`textDocument/definition`)**:
  Performs polyglot cross-language symbol navigation. Clicking on a frontend endpoint string (e.g. `/api/v1/billing`) jumps directly to the backend Go/Python handler, and from the handler jumps to the underlying cloud infrastructure definition (e.g. Terraform HCL resource).
- **CodeLens Provider (`textDocument/codeLens`)**:
  Calculates and displays causal lineage attestations (Agent DID, Intent description, timestamp) directly above function and type definitions in the editor.

---

## 7. Filesystem Watcher & Synthetic Git Bridge

### Background Auto-Staging Watcher (`cosm watch`)
Monitors the workspace filesystem, computes SHA-256 content hashes, parses modified files via polyglot codecs (`codecs.ParseSourceFile`), re-links cross-boundary graph edges, and commits updated manifests to the active universe working state.

### Synthetic Git Bridge (`cosm git init-bridge`)
Generates standard Git object and reference hierarchies (`.git/HEAD`, `.git/config`, `.git/objects/`, `refs/heads/`) derived from the Cosm AST Merkle-DAG. This allows third-party tools (VS Code Source Control tab, standard Git CLI, Git diff lenses) to operate transparently on Cosm workspaces without requiring git emulation daemons.

