# GEMINI.md - Antigravity AI Pair Programming Rules for Cosm (`cosm`)

This file contains durable project rules and contextual memory for Antigravity pair programming sessions in the `future-of-git` (Cosm) codebase.

---

## 1. Project Invariants

- **Language & Runtime**: Cosm is written in pure Go (`go 1.22+`).
- **Core APIs**: Pure Go backend and gRPC/in-process client SDKs. No Python for core SCM functionality.
- **Storage**:
  - Content-addressed immutable AST object storage under `.cosm/objects/` with SHA-256 hashes and atomic `fsync` persistence.
  - Pure-Go SQLite (`modernc.org/sqlite`) in WAL mode (`.cosm/graph.db`) for semantic edges, universe heads, oplog events, and lineage records. Zero-CGO requirement.
- **Local Autonomy**: All AST parsing, hydration, shipping, lineage tracking, and Git shims must run 100% offline without mandatory external cloud services.
- **Distributed Collaboration**: Distributed synchronization is built on Radicle-style CRDT Collaborative Objects (COBs) and Jujutsu-style stacked proposals (`pkg/distributed/`).
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
   - ALWAYS update docs in `docs/` (`docs/reference/schema-and-storage.md`, `docs/reference/agent-api.md`, `docs/reference/codecs.md`, etc.) whenever data models, APIs, codecs, or CLI commands change.
   - Documentation style MUST be **very direct, precise, and concise** (zero fluff, exact type definitions, clear markdown tables, and explicit formulas).

---

## 3. Skills Available in Workspace

The following specialized skills are available in `.agents/skills/`:
- `cosm-core-dev`: Core Cosm AST DAG development, storage operations, and CLI workflows.
- `cosm-agent-harness`: Autonomous test and rater agent execution, scenarios, and scorecards.
- `polyglot-codecs-guide`: Adding and testing language codecs, hydrators, and cross-boundary contracts.
- `shipping-and-validation`: Target preview sidecar, Terraform validation, and `uv` runner.
