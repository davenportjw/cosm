# GEMINI.md - Antigravity AI Pair Programming Rules for Cosm (`cosm`)

This file contains durable project rules and contextual memory for Antigravity pair programming sessions in the `cosm` codebase.

---

## 1. Project Invariants

- **Language & Runtime**: Cosm is written in pure Go (`go 1.22+`).
- **Core APIs**: Pure Go backend and gRPC/in-process client SDKs (`pkg/api`). No Python for core SCM functionality.
- **Storage**:
  - Content-addressed immutable AST object storage under `.cosm/objects/` with SHA-256 hashes and atomic `fsync` persistence.
  - Pure-Go zero-dependency custom Write-Ahead Log (`WAL`) graph engine (`.cosm/graph.db`) with CRC32 checksums for semantic edges, universe heads, oplog events, and lineage records. Zero-CGO requirement.
- **Local Autonomy vs. Topocosm Hub**:
  - `cosm` is 100% offline, pure-Go local engine (codecs, AST surgery, local Git shim, target compilation).
  - `topocosm` is the decentralized cloud hub and agent mesh (being spun off into `github.com/cosmscm/topocosm`).
- **Distributed Collaboration & Stacking**:
  - Radicle-style CRDT Collaborative Objects (COBs), Lamport logical clocks, and Jujutsu-style stacked proposals (`pkg/distributed/`).
- **Human PR Compatibility**: Universe proposals present code changes, AST symbol diff cards, contract topology graphs, and build badges while maintaining intuitive human review semantics (`pkg/review/`).
- **Test Agents**: Autonomous agent testing and scoring harnesses live in `test/agents/` and `cmd/cosm-agent-harness/`. Default LLM is `gemini-3.8-flash`.

---

## 2. Mandatory Workflow & Tooling Constraints

1. **Go Testing**:
   - Run tests with `go test -v ./...` or `go test -v ./pkg/...` and `go test -v ./test/agents/...`.
2. **Python Tooling**:
   - When executing or testing Python components, ALWAYS use `uv` (e.g., `uv run pytest`).
3. **Terraform / HCL Code**:
   - Always format and validate Terraform files with `terraform fmt` and `terraform validate`.
4. **Containerization / Infrastructure**:
   - Docker is NOT installed on this machine. Use local services or Apple containers for infrastructure and preview shipping.
5. **Continuous Documentation Updates & Doc-Code-Test Parity**:
   - ALWAYS update docs in `docs/` (`docs/reference/schema-and-storage.md`, `docs/reference/federation-and-multi-repo.md`, `docs/reference/topocosm.md`, `docs/roadmaps/topocosm-spin-off-plan.md`, etc.) whenever data models, APIs, codecs, or CLI commands change.
   - **Continuous Parity Invariant**: Every documented CLI command, example snippet, and workflow must have an automated test in the repository test suite (e.g. `cmd/cosm/doc_examples_test.go`). Code, documentation, and tests MUST always be kept strictly in sync and pass in CI (`go test -v ./...`).
   - Documentation style MUST be **very direct, precise, and concise** (zero fluff, exact type definitions, clear markdown tables, and explicit formulas).
6. **IDE & Workspace Auto-Synchronization**:
   - `cosm ast edit` automatically updates workspace disk files (`--write-disk` / `-w`, default `true`) so open VS Code buffers, Language Server Protocols (`gopls`, `tsserver`, `pyright`), linters, and test runners immediately see AST mutations.
7. **Graph Concurrency & Edge Write Synchronization**:
   - `GraphEngine` synchronizes `PutEdge` via `g.mu.Lock()` and framed binary WAL records with CRC32 checksums and atomic `fsync`. Edges are addressed via composite keys (`SourceID|TargetID|EdgeType`), ensuring write idempotency. Branch blocking locks are replaced by zero-copy micro-universes and non-blocking `ASTConflictNode`s.

---

## 3. Skills Available in Workspace

The following specialized skills are available in `.agents/skills/`:
- `cosm-core-dev`: Core Cosm AST DAG development, storage operations, and CLI workflows.
- `cosm-agent-harness`: Autonomous test and rater agent execution, scenarios, and scorecards.
- `polyglot-codecs-guide`: Adding and testing language codecs, hydrators, and cross-boundary contracts.
- `shipping-and-validation`: Target preview sidecar, Terraform validation, and `uv` runner.
- `topocosm-cloud-deploy`: Deploy Topocosm Hub to Google Cloud (Cloud Run, GCS CAS, Memorystore for Redis, Cloud Pub/Sub, Secret Manager) and perform zero-downtime updates, canary traffic routing, rollback, and Day-2 cloud operations.

---

## 4. Key CLI Commands

```bash
# Initialize a new Cosm repository (.cosm/)
cosm init [--universe universe-main]

# Reversal and History Operations
cosm undo [count] [-u universe-main] [-w]
cosm revert <target_hash> [-u universe-main] [-i "Revert description"] [-w]  # alias: cosm rollback
cosm reset [--hard] <target_hash> [-u universe-main] [-w]
cosm init --ledger [-u universe-main]  # Strict linear append-only audit mode
```

