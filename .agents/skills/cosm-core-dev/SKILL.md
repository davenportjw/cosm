---
name: cosm-core-dev
description: >-
  Develop, test, and debug the core Cosm AST Source Control Management (SCM) system,
  including content-addressed storage (.cosm/objects/), SQLite WAL graph engine (.cosm/graph.db),
  micro-universe branching, lineage tracing, and developer CLI commands (`cosm`).
---

# Cosm Core Development & Storage Runbook

Use this skill when modifying, extending, or debugging the core Cosm Go codebase, storage engine, CLI, or API server.

---

## Architecture Overview

1. **Storage Layer (`pkg/storage/`)**:
   - `blobstore.go`: Content-addressed immutable blob storage under `.cosm/objects/` with atomic write-and-rename (`fsync`).
   - `graphengine.go`: Pure-Go SQLite database (`modernc.org/sqlite`) running in Write-Ahead Log (`WAL`) mode with recursive CTEs.
   - `universe.go`: Zero-copy branching and micro-universe frontier manager.
   - `oplog.go`: Event-sourced append-only mutation stream.
   - `vectorindex.go`: Intent and natural language search index.

2. **Core Domain Primitives (`pkg/core/`)**:
   - `schema.go`: `ASTSymbolNode`, `ComponentNode`, `WorkspaceManifestNode`, `LineageEnvelope`, `CrossBoundaryEdge`.
   - `hasher.go`: Deterministic SHA-256 Merkle root computation.
   - `crossboundary.go`: Inference engine linking frontend, backend, and cloud infrastructure symbols (`CONSUMES_API`, `DEPLOYS_TO`, `BINDS_ENV`).

3. **Developer CLI (`cmd/cosm/`)**:
   - Implements Cobra-based CLI commands: `init`, `add`, `commit`, `status`, `view`, `topology`, `lineage`, `blast-radius`, `universe`, `proposal`, `ship`, `git`, `dashboard`.

---

## Common Development Workflows

### 1. Running Automated Tests
Always execute tests before and after making changes:
```bash
# Run all core package tests
go test -v ./pkg/...

# Run with race detector enabled
go test -v -race ./pkg/storage/... ./pkg/core/... ./pkg/mutation/...
```

### 2. Building the CLI Binary
```bash
go build -o cosm ./cmd/cosm
./cosm --help
```

### 3. Testing Storage Crash Recovery & Durability
To test SQLite WAL durability:
```go
// 1. Open workspace storage
store, graph, err := storage.OpenWorkspaceStorage(tempDir)

// 2. Put nodes and commit oplog events
// 3. Force-close or terminate process
// 4. Reopen and verify data integrity
store2, graph2, err := storage.OpenWorkspaceStorage(tempDir)
defer graph2.Close()
```

### 4. Adding or Modifying a CLI Command
1. Add command definitions in `cmd/cosm/main.go`.
2. Add corresponding integration tests in `cmd/cosm/cli_test.go`.
3. Verify CLI output formatting with Lipgloss.

---

## Rules to Remember

- **Pure Go Only**: Never introduce CGO dependencies (use `modernc.org/sqlite`).
- **Atomic Persistence**: Always persist AST symbol nodes to `.cosm/objects/` before recording node IDs in SQLite.
- **Deduplication**: Unchanged sibling AST symbols must share identical SHA-256 hashes and avoid redundant disk writes.
- **Always Update Docs**: When modifying schemas, storage formats, or CLI commands, always update `docs/reference/schema-and-storage.md`, `docs/reference/agent-api.md`, and `docs/reference/cli.md` in the same commit.
- **Direct Documentation Style**: Documentation must be strictly technical, direct, and concise (no fluff, exact equations, explicit JSON wire schemas, and copy-pasteable code).
