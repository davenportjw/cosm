# Topocosm (`topocosm.dev`) Hub & Agent Distribution Reference

**Topocosm (`topocosm.dev`)** is the decentralized distribution platform, agent registry, and collaborative sharing hub for Cosm micro-universes.

Topocosm provides machine-first agent discovery, sparse AST subgraph replication, distributed blackboard lease coordination, CRDT proposal convergence, and local Git Smart-HTTP compatibility shims.

---

## 1. System Division of Concerns: `cosm` vs. `topocosm`

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                                 TOPOCOSM (The Hub / Mesh)                              │
│                                                                                        │
│  • Agent Discovery (/.well-known/cosm-agent.json)   • Multi-Repo Catalog & Registry    │
│  • Distributed Blackboard Leases (Domain Locks)     • Sparse Subgraph CAS Distribution │
│  • Multi-Peer Proposal Gossip & CRDT Convergence    • Remote Git Smart-HTTP Server     │
└───────────────────────────────────────────┬────────────────────────────────────────────┘
                                            │
                        Network Pull / Push Protocol (CAS Blobs + COBs)
                                            │
                                            ▼
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                              COSM (The Local Core Engine / CLI)                        │
│                                                                                        │
│  • Polyglot AST Codecs (Go, TS, Py, HCL, Rust, etc) • AST Hydration to Disk (Files)   │
│  • Local CAS Blobstore (.cosm/objects/)              • SQLite WAL Graph (.cosm/graph.db)│
│  • Declarative AST Surgery (9 Geometric Verbs)       • Stack Auto-Evolution (jj-style) │
│  • Cross-Boundary Linker & Blast Radius Engine       • Embedded Shipping Sidecar (Ship)│
│  • Local Git Interceptor & Synthetic Commit Adapter  • Micro-Universe Branching        │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

### Architectural Division Matrix

| Responsibility | Handled by `cosm` | Handled by `topocosm` |
| :--- | :---: | :---: |
| **AST Parsing, Slicing & Surgery** | **YES** (`pkg/codecs/`, `pkg/mutation/`) | **NO** (Client-side execution) |
| **AST Hydration to Disk ("Cloning")** | **YES** (`pkg/materialize/`, `pkg/gitshim/`) | **NO** (Only streams CAS blobs) |
| **Project vs. Non-Project Boundaries** | **YES** (`pkg/core/crossboundary.go`) | **NO** (Stores indexed edges) |
| **Stack Auto-Evolution (`cosm stack evolve`)** | **YES** (`pkg/distributed/stacked.go`) | **NO** (Propagates proposal COBs) |
| **Lamport Clocks & CRDT Joining** | **YES** (Local clock tick) | **YES** (State convergence join $\sqcup$) |
| **Agent Domain Leases (Blackboard)** | **SDK Client** | **Server Authority / Lock Engine** |
| **Sparse Subtree Replication** | **Ingest Engine** | **Filtered Packaging Service** |
| **Git Interoperability** | **Local CLI Interceptor** | **Remote Smart-HTTP Endpoint** |

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
When hosted at `topocosm.dev`, the cloud backplane provisions Google Cloud Platform infrastructure:
1. **CAS Object Store**: Global Google Cloud Storage (GCS) bucket with Cloud CDN caching for immutable AST blobs.
2. **Metadata DAG Engine**: Cloud Spanner / SQLite WAL for cross-boundary edges and universe manifests.
3. **Event Mesh**: Google Cloud Pub/Sub for real-time agent swarms and proposal sync.
4. **Lease Coordination**: Google Cloud Memorystore for Redis conditional writes with TTLs for blackboard claims.
5. **Preview Execution**: Google Cloud Run / Apple containers for sandboxed compilation and local preview.

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

### Web UI Dashboard & Administration (`/cosms`, `/federation`, `/admin`, `/settings`)
- `GET /cosms`: Polyglot Cosm repository catalog with Merkle root hashes and component metrics.
- `GET /cosms/{org}/{cosm}`: 4-tier AST Merkle-DAG workspace, zero-copy micro-universe switcher, and cross-boundary topology graph.
- `GET /cosms/{org}/{cosm}/symbols/{id}`: Split-pane AST symbol inspector, hydrated polyglot source code, and Ed25519 causal lineage card.
- `GET /cosms/{org}/{cosm}/proposals/{id}`: Semantic AST symbol deltas, AI Critic Oracle reviews, and CRDT merge actions.
- `GET /cosms/{org}/{cosm}/stacks`: Jujutsu-style stacked proposals with auto-evolution timeline.
- `GET /federation`: Multi-Cosm AST Super-DAG continuum and Sparse Subtree Replication Simulator.
- `GET /admin/users`: User & access governance table, admin invitation modal, and role assigner (`admin`, `maintainer`, `contributor`, `viewer`).
- `GET /admin/blackboard`: Live micro-domain lease coordinator with active agent goals and force-evict actions.
- `GET /admin/benchmarks`: Multi-agent concurrent swarm simulation controller with live throughput telemetry.
- `GET /settings/tokens`: Developer Personal Access Token (PAT) management with one-time copy reveals and SHA-256 one-way hashing.
- `POST /auth/session`: Federated Social Sign-in (Google / GitHub / Passkeys) and session cookie issuer with `LOCAL_DEV` bypass.

---

## 5. Teamwork, Envelope Encryption & Developer Authentication

### Zero Plaintext Storage Invariant
- Topocosm **never** stores plaintext user passwords, private keys, or raw PATs.
- Personal Access Tokens use the `tp_pat_` prefix and are persisted strictly as `sha256(raw_token)` hashes.
- Authenticated requests pass `Authorization: Bearer tp_pat_<secret>` or an active session cookie.

### Client-Side Multi-Recipient Envelope Encryption
1. **Data Encryption Key (DEK)**: 256-bit AES-GCM random symmetric key generated client-side per workspace/universe.
2. **Key Encryption Key (KEK)**: Derived via ECDH (`crypto/ecdh.X25519()`) between an ephemeral keypair and each recipient's public key, finalized with HKDF-SHA256.
3. **Sharing & Access Grants**: The repository owner seals the DEK for collaborator public keys retrieved from `GET /api/v1/users/{email}/pubkey` and stores access grants via `POST /api/v1/cosms/{org}/{cosm}/share`.
4. **Revocation**: Removing a collaborator envelope immediately revokes decryption capability without requiring re-encryption of past immutable CAS commits.

---

## 6. Standalone Repository Spin-Off Roadmap

Topocosm is scheduled for decoupling into an independent repository (`github.com/cosmscm/topocosm`). 
For the full migration timeline, package extraction maps, and protocol versioning, refer to the [Topocosm Spin-Off Roadmap](../roadmaps/topocosm-spin-off-plan.md).

---

## 7. Developer CLI Workflows

```bash
# 1. Start local zero-Docker Topocosm Hub daemon
cosm topocosm dev --port 51204 --dir .topocosm

# 2. Authenticate CLI & Manage Personal Access Tokens (PATs)
cosm auth login --token tp_pat_9a4f21b7c8e901...
cosm auth whoami
cosm auth token list
cosm auth token create my-laptop-pat --days 90

# 3. Seed mock polyglot cosms, proposals, and blackboard claims
cosm topocosm seed --dir .topocosm

# 4. Inspect local hub status and active claims
cosm topocosm status --url http://127.0.0.1:51204

# 5. Benchmark concurrent agent swarm throughput
cosm topocosm test-swarm --agents 25 --duration 5s

# 6. Publish local universe AST DAG to Topocosm Hub
cosm publish http://127.0.0.1:51204/demo-org/cloud-platform --universe universe-main --intent "Ship payment API"

# 7. Grant collaborator access with client-side envelope encryption
cosm share demo-org/cloud-platform --user collaborator@company.com --access maintainer

# 8. Clone or sparse-pull cosm to local disk
cosm clone http://127.0.0.1:51204/demo-org/cloud-platform ./cloud-platform
cosm clone http://127.0.0.1:51204/demo-org/cloud-platform ./billing-subgraph --sparse "services/billing"
```

