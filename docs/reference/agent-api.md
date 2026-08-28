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

See [Topocosm Reference Manual](file:///Users/jasondavenport/GitHub/future-of-git/docs/reference/topocosm.md) for full endpoint specifications.
