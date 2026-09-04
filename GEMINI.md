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
- **Test Agents**: Autonomous agent testing and scoring harnesses live in `test/agents/` and `cmd/cosm-agent-harness/`. Default LLM is `gemini-3.7-flash`.

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
5. **Continuous Documentation Updates & Direct Style**:
   - ALWAYS update docs in `docs/` (`docs/reference/schema-and-storage.md`, `docs/reference/federation-and-multi-repo.md`, `docs/reference/topocosm.md`, `docs/roadmaps/topocosm-spin-off-plan.md`, etc.) whenever data models, APIs, codecs, or CLI commands change.
   - Documentation style MUST be **very direct, precise, and concise** (zero fluff, exact type definitions, clear markdown tables, and explicit formulas).
6. **IDE & Workspace Auto-Synchronization**:
   - `cosm ast edit` automatically updates workspace disk files (`--write-disk` / `-w`, default `true`) so open VS Code buffers, Language Server Protocols (`gopls`, `tsserver`, `pyright`), and linters immediately see AST mutations.

---

## 3. Skills Available in Workspace

The following specialized skills are available in `.agents/skills/`:
- `cosm-core-dev`: Core Cosm AST DAG development, storage operations, and CLI workflows.
- `cosm-agent-harness`: Autonomous test and rater agent execution, scenarios, and scorecards.
- `polyglot-codecs-guide`: Adding and testing language codecs, hydrators, and cross-boundary contracts.
- `shipping-and-validation`: Target preview sidecar, Terraform validation, and `uv` runner.
- `topocosm-cloud-deploy`: Deploy Topocosm Hub to Google Cloud (Cloud Run, GCS CAS, Memorystore for Redis, Cloud Pub/Sub, Secret Manager) and perform zero-downtime updates, canary traffic routing, rollback, and Day-2 cloud operations.
