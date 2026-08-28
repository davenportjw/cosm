# Topocosm (`topocosm.dev`) Hub & Agent Distribution Reference

**Topocosm (`topocosm.dev`)** is the decentralized distribution platform, agent registry, and collaborative sharing hub for Cosm micro-universes.

Topocosm provides machine-first agent discovery, sparse AST subgraph replication, distributed blackboard lease coordination, CRDT proposal convergence, and local Git Smart-HTTP compatibility shims.

---

## 1. System Architecture

```
                                  Agent Swarms / Cosm CLI
                                             │
                      ┌──────────────────────┴──────────────────────┐
                      │                                             │
             HTTP / REST / gRPC                             Git Smart-HTTP
         (Agent / Developer SDK)                       (IDE & CLI Compatibility)
                      │                                             │
                      ▼                                             ▼
       ┌──────────────────────────────────────────────────────────────────┐
       │                       Topocosm Hub Server                        │
       │                                                                  │
       │  • /.well-known/cosm-agent.json  • CAS AST Publishing & Pull     │
       │  • Ed25519 DID & Policy Engine   • Sparse Subtree Extraction     │
       │  • Blackboard Lease Coordination • CRDT Proposal Merging (COBs)  │
       └──────────────────────────────────┬───────────────────────────────┘
                                          │
                            Pluggable Backplane Interface
                        (BackplaneProvider / LocalBackplane)
                                          │
                 ┌────────────────────────┴────────────────────────┐
                 │                                                 │
          Local Dev / Edge                               Cloud Cluster Backplane
      (100% Pure-Go / SQLite WAL)                    (Distributed Cloud Services)
                 │                                                 │
      • BlobStore (.cosm/objects/)                      • S3 / GCS / R2 Object Store
      • SQLite WAL (.cosm/graph.db)                     • CockroachDB / DynamoDB Metadata
      • In-Memory Go Channels                           • NATS JetStream Event Fabric
      • In-Process Mutex Leases                         • Redis / DynamoDB Distributed Locks
```

---

## 2. Pluggable Backplane Interface (`pkg/topocosm/backplane/`)

Topocosm abstracts all persistence and runtime infrastructure behind the `BackplaneProvider` interface:

```go
type BackplaneProvider interface {
    BlobStore() BlobStoreProvider
    GraphEngine() *storage.GraphEngine
    UniverseManager() *storage.UniverseManager
    EventStream() EventStreamProvider
    LeaseManager() LeaseProvider
    SandboxRunner() SandboxRunner
    CriticProvider() CriticProvider
    Close() error
}
```

### Local Dev Backplane (`LocalBackplane`)
- **Blob Storage**: Pure-Go atomic `fsync` content-addressed storage (`pkg/storage/blobstore.go`).
- **Graph & Universe Engine**: Pure-Go SQLite in Write-Ahead Log (`WAL`) mode (`pkg/storage/graphengine.go`).
- **Event Bus**: Go channel publish-subscribe with non-blocking broadcast.
- **Lease Manager**: Thread-safe TTL-expiring in-process domain lease coordinator.
- **Docker Requirement**: **Zero**. Runs 100% locally and hermetically.

### Cloud Cluster Requirements
When hosted at `topocosm.dev`, the cloud backplane provisions:
1. **CAS Object Store**: Global S3/GCS bucket with edge CDN caching for immutable AST blobs.
2. **Metadata DAG Engine**: CockroachDB / Spanner multi-region SQL for cross-boundary edges and universe manifests.
3. **Event Mesh**: NATS JetStream cluster for real-time agent swarms and proposal sync.
4. **Lease Coordination**: Redis / DynamoDB conditional writes with TTLs for blackboard claims.
5. **Preview Execution**: Firecracker microVMs / Apple containers for sandboxed compilation.

---

## 3. Agent Discovery Manifest (`/.well-known/cosm-agent.json`)

Topocosm exposes an agent discovery endpoint at `GET /.well-known/cosm-agent.json`:

```json
{
  "name": "Topocosm Hub",
  "version": "1.0.0",
  "hub_did": "did:web:topocosm.dev",
  "api_version": "v1",
  "capabilities": {
    "ast_level_diff": true,
    "blackboard_crdt": true,
    "critic_evaluations": true,
    "cross_boundary_contracts": true,
    "git_smart_http": true,
    "lineage_attestations": true,
    "sparse_sync": true,
    "stacked_proposals": true
  },
  "endpoints": {
    "auth_enroll": "/api/v1/auth/enroll",
    "blackboard_claim": "/api/v1/cosms/{org}/{cosm}/blackboard/claim",
    "cosms_root": "/api/v1/cosms",
    "git_shim": "/git/{org}/{cosm}.git",
    "health": "/api/v1/health",
    "proposals_root": "/api/v1/cosms/{org}/{cosm}/proposals",
    "publish": "/api/v1/cosms/{org}/{cosm}/publish",
    "sparse_pull": "/api/v1/cosms/{org}/{cosm}/sparse-pull",
    "stats": "/api/v1/stats"
  },
  "attestation_policy": {
    "require_signatures": true,
    "require_critic_review": true,
    "min_critic_score": 80.0,
    "allowed_target_kinds": [
      "local-service",
      "cloud-run",
      "static-web",
      "terraform-infra",
      "composite"
    ]
  }
}
```

---

## 4. HTTP / REST API Endpoints (`pkg/topocosm/server.go`)

### Authentication & DID Identity
- `POST /api/v1/auth/enroll`: Enrolls a user or AI agent DID (`did:key:...`) with public key attestation.
- `GET /api/v1/auth/whoami`: Returns the authenticated DID and authorization scope from `X-Cosm-DID`.

### User & Organization Settings
- `GET /api/v1/settings/user`: Inspects the authenticated user account and enrolled agent keys.
- `PUT /api/v1/settings/user`: Updates profile metadata and default attestation policies.
- `GET /api/v1/settings/orgs/{org}`: Queries organization membership and repository access tiers.
- `GET /api/v1/settings/agents`: Lists all registered agent DIDs and verification statuses.

### Cosm Publishing & Replication
- `GET /api/v1/cosms`: Lists all public and authorized private cosms.
- `POST /api/v1/cosms`: Creates a new cosm metadata record.
- `GET /api/v1/cosms/{org}/{cosm}`: Returns repository metadata, active universe heads, and Merkle root.
- `POST /api/v1/cosms/{org}/{cosm}/publish`: Receives workspace manifests, components, AST symbol nodes, and raw CAS payloads.
- `POST /api/v1/cosms/{org}/{cosm}/pull?universe={id}`: Downloads the complete workspace manifest and all referenced AST blobs.

### Sparse Subtree Pull Protocol
- `POST /api/v1/cosms/{org}/{cosm}/sparse-pull`: Downloads a filtered AST subgraph based on component names or languages, minimizing agent bandwidth overhead.
  ```json
  {
    "org_slug": "demo-org",
    "cosm_name": "cloud-platform",
    "universe_id": "universe-main",
    "component_names": ["services/billing"]
  }
  ```

### Stacked Proposals & Critic Reviews
- `GET /api/v1/cosms/{org}/{cosm}/proposals`: Lists active CRDT proposals.
- `POST /api/v1/cosms/{org}/{cosm}/proposals`: Creates a new proposal with causal lineage and target universe.
- `GET /api/v1/cosms/{org}/{cosm}/proposals/{id}`: Inspects proposal AST symbol deltas, target preview status, and reviewer scorecards.
- `POST /api/v1/cosms/{org}/{cosm}/proposals/{id}/review`: Submits a human approval or AI critic scorecard (0–100).
- `POST /api/v1/cosms/{org}/{cosm}/proposals/{id}/merge`: Merges the proposal AST DAG into the target universe head using CRDT semilattice union.

### Blackboard Lease Coordination
- `GET /api/v1/cosms/{org}/{cosm}/blackboard`: Lists active domain claims, holders, goals, and TTL expiration timestamps.
- `POST /api/v1/cosms/{org}/{cosm}/blackboard/claim`: Acquires an exclusive domain lock for an agent task (e.g. `services/billing`).
- `POST /api/v1/cosms/{org}/{cosm}/blackboard/release`: Releases a domain claim upon task completion or cancellation.

### Git Smart-HTTP Compatibility Shim
- `GET /git/{org}/{cosm}.git/info/refs?service=git-upload-pack`: Returns synthetic Git refs advertisement for IDEs and legacy git clients.

---

## 5. Developer CLI Workflows

```bash
# 1. Start local zero-Docker Topocosm Hub daemon
cosm topocosm dev --port 51204 --dir .topocosm

# 2. Seed mock polyglot cosms, proposals, and blackboard claims
cosm topocosm seed --dir .topocosm

# 3. Inspect local hub status and active claims
cosm topocosm status --url http://127.0.0.1:51204

# 4. Benchmark concurrent agent swarm throughput
cosm topocosm test-swarm --agents 25 --duration 5s

# 5. Publish local universe AST DAG to Topocosm Hub
cosm publish http://127.0.0.1:51204/demo-org/cloud-platform --universe universe-main --intent "Ship payment API"

# 6. Clone or sparse-pull cosm to local disk
cosm clone http://127.0.0.1:51204/demo-org/cloud-platform ./cloud-platform
cosm clone http://127.0.0.1:51204/demo-org/cloud-platform ./billing-subgraph --sparse "services/billing"
```
