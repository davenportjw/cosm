# AGENTS.md - Cosm (`future-of-git`) Agent Guidelines & Architecture Rules

This document establishes durable conventions, architectural invariants, environment constraints, and workflow patterns for all AI agents working within the Cosm repository.

---

## 1. Project Vision & Core Identity

**Cosm (`cosm`)** is an AI-Native Polyglot AST Source Control Management (SCM) and Target Compilation System built in pure Go (`go 1.22+`).
- **Storage**: Software is stored as a content-addressed AST Merkle-DAG in `.cosm/objects/` with metadata, semantic dependency edges, universe heads, and event Oplogs managed in pure-Go SQLite WAL mode (`.cosm/graph.db`).
- **Branching**: Zero-copy parallel micro-universes (non-linear frontier heads).
- **Lineage**: Multi-tier causal pedigree (`User Prompt` $\rightarrow$ `Session ID` $\rightarrow$ `Agent ID` $\rightarrow$ `Model Params` $\rightarrow$ `AST Node`) with Ed25519 digital signatures.
- **Shipping**: Autonomous target compilation sidecar (`cosm ship`) executing isolated staging builds.
- **Collaboration**: Local Git CLI compatibility shim (`cosm git`) + Radicle-inspired distributed P2P Collaborative Objects (COBs) with CRDT state synchronization.
- **PR Model**: Human-understandable Universe Proposals displaying AST symbol deltas, topology changes, and build badges.

---

## 2. Invariant Architecture & Coding Rules

1. **Go-First Backend & APIs (`go 1.22+`)**:
   - All core APIs, storage engines, codecs, and CLI tools MUST be written in Go.
   - Core packages: `pkg/core`, `pkg/storage`, `pkg/codecs`, `pkg/lineage`, `pkg/materialize`, `pkg/shipping`, `pkg/target`, `pkg/gitshim`, `pkg/distributed`, `pkg/collaboration`, `pkg/review`, `pkg/onboarding`, `pkg/mutation`, `pkg/tui`, `pkg/api`.
   - Do NOT use Python or external interpreted wrappers for core Cosm functionality.

2. **Storage Durability & Zero-CGO**:
   - Uses pure-Go SQLite (`modernc.org/sqlite`) in Write-Ahead Log (`WAL`) mode.
   - Atomic write-and-rename (`fsync`) semantics for content-addressed immutable blobs in `.cosm/objects/`.
   - The database and blob store must survive sudden process termination or service restarts with zero corrupt records.

3. **Git vs. GitHub.com Disentanglement**:
   - Cosm runs 100% locally with zero required network calls or cloud dependencies.
   - Git compatibility (`cosm git`) provides a local plumbing shim for IDEs and existing tools.
   - Distributed remote sync uses Radicle-style CRDT Collaborative Objects (COBs) and Jujutsu-style stacked proposals.

4. **Preserve the Human PR Model**:
   - Proposals represent semantic AST mutations, cross-boundary contracts, and cryptographic attestations while preserving familiar human review workflows (annotating symbols, partial approvals, natural language remediation prompts).

5. **Always Update Documentation & Maintain Direct Style**:
   - Whenever schemas, storage engines, APIs, CLI commands, codecs, or distributed models are created or altered, corresponding reference documents under `docs/` (`docs/reference/schema-and-storage.md`, `docs/reference/agent-api.md`, `docs/reference/codecs.md`, `docs/reference/cli.md`, etc.) MUST be updated immediately in the same change.
   - Documentation style MUST be **very direct, concise, and technically rigorous**—avoiding filler, marketing fluff, or conversational tangents, and emphasizing exact data structures, mathematical equations, wire DTO schemas, and executable code snippets.

6. **Test Agent & Rater Harness Isolation**:
   - The external autonomous test agent and rater harness live strictly in `test/agents/` and `cmd/cosm-agent-harness/`, isolated from the application runtime.
   - Always use the latest model for LLM evaluations: `gemini-3.7-flash` is the default.
   - Hermetic CI tests use `test/agents/llm/mock.go` to run without API keys or network.

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

# Stage polyglot source code into AST symbol nodes
cosm add <file_or_dir> [--intent "description"] [--prompt "user prompt"]

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

# Universe Proposals (PR equivalent)
cosm proposal list
cosm proposal create --title "..." --source <universe_id> --target universe-main
cosm proposal view <proposal_id>

# Build and ship to target environment
cosm ship --target local-preview

# Run autonomous agent test harness
cosm-agent-harness run --scenario polyglot-fastapi-react --model gemini-3.7-flash
cosm-agent-harness rate --session <session_id>
cosm-agent-harness suite --all
```

---

## 5. Directory Structure Reference

```
future-of-git/
├── .cosm/                      # Content-addressed AST store & SQLite WAL database
├── cmd/
│   ├── cosm/                   # Developer CLI binary (`cosm`)
│   └── cosm-agent-harness/     # Standalone test/rater agent harness binary
├── pkg/
│   ├── api/                    # gRPC and in-process Go client/server SDK
│   ├── codecs/                 # Polyglot AST parsers (Go, HCL, TS, Python, Rust, Java, SQL, Protobuf, C++)
│   ├── collaboration/          # Conflict engine and multi-universe fitness evaluator
│   ├── core/                   # Schemas, Merkle hasher, and cross-boundary graph linker
│   ├── distributed/            # Radicle-inspired CRDT COBs & P2P sync
│   ├── gitshim/                # Git command interceptor and synthetic tree generator
│   ├── lineage/                # Ancestry tracer, audit engine, and Ed25519 attestations
│   ├── materialize/            # AST-to-source hydrators, terminal viewer, and VFS
│   ├── mutation/               # AST symbol surgery and cross-boundary blast radius
│   ├── onboarding/             # Snapshot & historical Git repo ingestion pipelines
│   ├── review/                 # AST proposal cards, human annotations, and AI critic protocols
│   ├── shipping/               # Target specification, ephemeral staging, and compilers
│   ├── storage/                # Blobstore, SQLite WAL graph engine, and vector index
│   ├── target/                 # Target environment projector and local preview sandbox
│   └── tui/                    # Interactive Bubble Tea terminal dashboard
├── test/
│   └── agents/                 # External autonomous test agent, LLM client, and rater oracle
├── docs/                       # Benefits-driven documentation and reference guides
└── examples/                   # Polyglot demo workspaces (Terraform + Go + React)
```
