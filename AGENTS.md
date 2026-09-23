# AGENTS.md - Cosm (`cosm`) Agent Guidelines & Architecture Rules

This document establishes durable conventions, architectural invariants, environment constraints, and workflow patterns for all AI agents working within the Cosm repository.

---

## 1. Project Vision & Core Identity

**Cosm (`cosm`)** is an AI-Native Polyglot AST Source Control Management (SCM) and Target Compilation System built in pure Go (`go 1.22+`).
- **Storage**: Software is stored as a content-addressed AST Merkle-DAG in `.cosm/objects/` with metadata, semantic dependency edges, universe heads, and event Oplogs managed in pure-Go custom WAL binary format (`.cosm/graph.db`).
- **Branching**: Zero-copy parallel micro-universes (non-linear frontier heads).
- **Lineage**: Multi-tier causal pedigree (`User Prompt` $\rightarrow$ `Session ID` $\rightarrow$ `Agent ID` $\rightarrow$ `Model Params` $\rightarrow$ `AST Node`) with Ed25519 digital signatures.
- **Shipping**: Autonomous target compilation sidecar (`cosm ship`) executing isolated staging builds.
- **Collaboration**: Local Git CLI compatibility shim (`cosm git`) + Radicle-inspired distributed P2P Collaborative Objects (COBs) with CRDT state synchronization.
- **PR Model**: Human-understandable Universe Proposals displaying AST symbol deltas, topology changes, and build badges.
- **Federation**: Multi-repo AST chaining and sparse replication via Topocosm (`topocosm.dev`), which is decoupled as an independent cloud hub.

---

## 2. Invariant Architecture & Coding Rules

1. **Go-First Backend & APIs (`go 1.22+`)**:
   - All core APIs, storage engines, codecs, and CLI tools MUST be written in Go.
   - Core packages: `pkg/core`, `pkg/storage`, `pkg/codecs`, `pkg/lineage`, `pkg/materialize`, `pkg/shipping`, `pkg/target`, `pkg/gitshim`, `pkg/distributed`, `pkg/collaboration`, `pkg/review`, `pkg/onboarding`, `pkg/mutation`, `pkg/tui`, `pkg/api`.
   - Do NOT use Python or external interpreted wrappers for core Cosm functionality.

2. **Storage Durability & Zero-CGO**:
   - Uses zero-dependency pure-Go custom Write-Ahead Log (`WAL`) graph engine with CRC32 checksums (`pkg/storage/graphengine.go`).
   - Atomic write-and-rename (`fsync`) semantics for content-addressed immutable blobs in `.cosm/objects/`.
   - The database and blob store must survive sudden process termination or service restarts with zero corrupt records.

3. **Cosm Engine vs. Topocosm Hub Separation**:
   - `cosm` is the 100% offline, pure-Go local AST engine, compiler, and developer CLI.
   - `topocosm` is the decentralized distribution hub, agent registry, and cloud backplane (being spun off into `github.com/cosmscm/topocosm`).
   - Communication between `cosm` and `topocosm` occurs strictly over versioned wire protocols (`/api/v1/`, gRPC) defined in `pkg/api/`.

4. **Multi-Repo Federation & Causal Time**:
   - Cross-repository contracts use content-addressed URIs (`cosm://org/repo/component@hash`).
   - Replication orders events via Lamport Logical Clocks + DID tiebreakers, converging via CRDT semilattice joins ($\sqcup$).

5. **Always Update Documentation & Maintain Direct Style (Doc-Code-Test Parity)**:
   - Whenever schemas, storage engines, APIs, CLI commands, codecs, or distributed models are created or altered, corresponding reference documents under `docs/` (`docs/reference/schema-and-storage.md`, `docs/reference/agent-api.md`, `docs/reference/codecs.md`, `docs/reference/federation-and-multi-repo.md`, `docs/reference/topocosm.md`, etc.) MUST be updated immediately in the same change.
   - **Continuous Parity Invariant**: Every documented CLI command, example snippet, and workflow must have an automated test in the repository test suite (e.g. `cmd/cosm/doc_examples_test.go`). Code, documentation, and tests MUST always be kept strictly in sync and pass in CI (`go test -v ./...`).
   - Documentation style MUST be **very direct, concise, and technically rigorous**—avoiding filler, marketing fluff, or conversational tangents, and emphasizing exact data structures, mathematical equations, wire DTO schemas, and executable code snippets.

6. **Test Agent & Rater Harness Isolation**:
   - The external autonomous test agent and rater harness live strictly in `test/agents/` and `cmd/cosm-agent-harness/`, isolated from the application runtime.
   - Always use the latest model for LLM evaluations: `gemini-3.8-flash` is the default.
   - Hermetic CI tests use `test/agents/llm/mock.go` to run without API keys or network.

7. **IDE & Workspace Disk Synchronization**:
   - `cosm ast edit` automatically updates enclosing source files on disk using `--write-disk` (default `true` / `-w`).
   - This ensures open VS Code buffers, Language Server Protocols (`gopls`, `tsserver`, `pyright`), linters, and test runners instantly observe mutations without requiring manual export.
   - Headless background batch pipelines may pass `--write-disk=false` when only DAG manipulation is required.

8. **Graph Concurrency, Edge Write Synchronization & Non-Blocking Conflict Reification**:
   - In `pkg/storage/graphengine.go`, edge and node mutations (`PutEdge`) are strictly synchronized via `g.mu.Lock()` and committed through framed binary WAL records with IEEE CRC32 checksums and atomic `fsync`.
   - Edges are composite-addressed via `EdgeKey() = SourceID|TargetID|EdgeType` guaranteeing idempotency under concurrent writes.
   - Long-lived blocking locks across branches are prohibited: agents work in isolated micro-universes.
   - Cross-agent edge contract discrepancies are detected via `core.DetectContractBreakages` and reified as first-class `ASTConflictNode`s in the Merkle-DAG rather than stalling the pipeline.

---

## 3. Environment & Tooling Guidelines

- **Go Testing**: Run unit and integration tests using `go test -v ./...` (or `go test -v ./pkg/...` and `go test -v ./test/agents/...`).
- **Python**: When running or testing Python code, always use `uv` (e.g. `uv run pytest`).
- **Terraform / HCL**: Always execute `terraform fmt` and `terraform validate` when modifying Terraform files.
- **Containers / Infrastructure**: Docker is NOT installed on the host system; use local services / apple containers for testing and shipping infrastructure.

---

## 4. Key CLI Commands

```bash
# Initialize a new Cosm repository (.cosm/)
cosm init [--universe universe-main]

# --- Paradigm A: Direct AST Inception (Agent / AST-First, No Files Required) ---
# Incept component directly into the DAG with auto-scaffolding
cosm ast create -c <component_name> --lang <go|python|typescript|sql> [-w]
# Or incept component with explicit source code
cosm ast create -c <component_name> -f <path> --code "..." [-w]
# Perform surgical AST mutations directly on symbols
cosm ast edit --op replace_function_body --target <symbol> --content "..." [-w]
# Inspect AST Merkle hierarchy
cosm ast tree

# --- Paradigm B: Filesystem Staging Lens (Disk / Hybrid) ---
# Stage all workspace files recursively into AST symbol DAG
cosm add .
# Or stage specific files or subdirectories
cosm add <file_or_dir...> [--intent "description"] [--prompt "user prompt"]

# Commit staged AST symbols with causal lineage
cosm commit [--universe universe-main] [--intent "Commit message"]

# Inspect workspace status and staged components
cosm status

# View AST symbol details, source hydration, and lineage
cosm view <node_id> [--format source|ast|raw]

# Cross-domain topology visualization (Frontend -> Backend -> Cloud Infra)
cosm topology

# Query blast radius of a symbol or model modification
cosm blast-radius <node_id>

# Manage micro-universes (zero-copy branching)
cosm universe list
cosm universe create <universe_id> [--parent <parent_id>]

# Jujutsu-style stacked proposals
cosm stack create -c <change_id> -u <universe_id> -p <parent_change_id>
cosm stack evolve -c <change_id>

# Universe Proposals (PR equivalent)
cosm proposal list
cosm proposal create --title "..." --source <universe_id> --target universe-main
cosm proposal view <proposal_id>

# Build and ship to target environment
cosm ship --target local-preview

# Run autonomous agent test harness
cosm-agent-harness run --scenario polyglot-fastapi-react --model gemini-3.8-flash
cosm-agent-harness rate --session <session_id>
cosm-agent-harness suite --all
```

---

## 5. Directory Structure Reference

```
cosm/
├── .cosm/                      # Content-addressed AST store & SQLite WAL database
├── cmd/
│   ├── cosm/                   # Developer CLI binary (`cosm`)
│   └── cosm-agent-harness/     # Standalone test/rater agent harness binary
├── pkg/
│   ├── api/                    # gRPC and REST wire DTOs and client SDK
│   ├── codecs/                 # Polyglot AST parsers (Go, HCL, TS, Python, Rust, Java, SQL, Protobuf, C++)
│   ├── collaboration/          # Conflict engine and multi-universe fitness evaluator
│   ├── core/                   # Schemas, Merkle hasher, and cross-boundary graph linker
│   ├── distributed/            # Radicle-inspired CRDT COBs, stacked proposals & P2P sync
│   ├── gitshim/                # Git command interceptor and synthetic tree generator
│   ├── lineage/                # Ancestry tracer, audit engine, and Ed25519 attestations
│   ├── materialize/            # AST-to-source hydrators, terminal viewer, and VFS
│   ├── mutation/               # AST symbol surgery and cross-boundary blast radius
│   ├── onboarding/             # Snapshot & historical Git repo ingestion pipelines
│   ├── review/                 # AST proposal cards, human annotations, and AI critic protocols
│   ├── shipping/               # Target specification, ephemeral staging, and compilers
│   ├── storage/                # Blobstore, SQLite WAL graph engine, and vector index
│   ├── target/                 # Target environment projector and local preview sandbox
│   ├── topocosm/               # Distribution hub backplane (migrating to standalone repo)
│   └── tui/                    # Interactive Bubble Tea terminal dashboard
├── test/
│   └── agents/                 # External autonomous test agent, LLM client, and rater oracle
├── docs/
│   ├── reference/              # Technical specifications (schema, federation, topocosm, codecs)
│   ├── roadmaps/               # Engineering roadmaps (Topocosm spin-off plan)
│   └── guides/                 # Developer and testing walkthroughs
└── examples/                   # Polyglot demo workspaces (Terraform + Go + React)
```

---

## 6. Skills Available in Workspace

- `cosm-core-dev`: Core Cosm AST DAG development, storage operations, and CLI workflows.
- `cosm-agent-harness`: Autonomous test and rater agent execution, scenarios, and scorecards.
- `cosm-ast-surgeon`: Declarative AST mutations using the 9 Geometric AST verbs.
- `polyglot-codecs-guide`: Adding and testing language codecs, hydrators, and cross-boundary contracts.
- `shipping-and-validation`: Target preview sidecar, Terraform validation, and `uv` runner.
- `topocosm-cloud-deploy`: Deploy Topocosm Hub to Google Cloud (Cloud Run, GCS CAS, Memorystore for Redis, Cloud Pub/Sub, Secret Manager) and perform zero-downtime updates, canary traffic routing, rollback, and Day-2 cloud operations.
