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
   - `schema.go`: `ASTSymbolNode`, `ComponentNode`, `WorkspaceManifestNode`, `LineageEnvelope`, `TokenTelemetry`, `TraceCarrier`, `CrossBoundaryEdge`.
   - `hasher.go`: Deterministic SHA-256 Merkle root computation.
   - `crossboundary.go`: Inference engine linking frontend, backend, and cloud infrastructure symbols (`CONSUMES_API`, `DEPLOYS_TO`, `BINDS_ENV`).

3. **Developer CLI (`cmd/cosm/`) & Agent Commit Contract**:
   - Implements CLI commands: `init`, `add`, `commit`, `status`, `view`, `topology`, `lineage`, `blast-radius`, `universe`, `proposal`, `ship`, `git`, `dashboard`, `ast edit`, `ast resolve`.
   - Telemetry flags for agents: `--prompt`, `--session-id`, `--orchestrator-id`, `--model`, `--prompt-tokens`, `--completion-tokens`, `--reasoning-tokens`, `--cost-usd`, `--latency-ms`, `--trace-id`, `--span-id`.
   - Complete agent contract reference: `docs/reference/agent-commit-contract.md`.

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

### 5. IDE & Workspace Disk Synchronization
When AST mutations are applied via `cosm ast edit`, Cosm automatically synchronizes modified components to the workspace disk files via `materialize.NewExporter().ExportToDisk()` (default flag `--write-disk` / `-w`). This preserves real-time synchronization with developer IDEs (VS Code, Cursor) and Language Server Protocols (`gopls`, `tsserver`, `pyright`). When writing or testing commands that modify universe AST state, ensure the disk synchronization layer cleanly hydrates and exports affected components.

### 6. Storage Concurrency & Edge Write Invariants
When developing or extending `pkg/storage/graphengine.go`, `pkg/storage/universe.go`, or cross-boundary graph traversal, adhere strictly to these physical storage and edge concurrency invariants:
1. **Physical Engine Mutex Lock (`mu sync.RWMutex`)**:
   `GraphEngine` guards all write mutations with `g.mu.Lock()` in `executeMutation`. Standalone mutations write framed binary WAL records (`walRecordTxBegin`, `walRecordMutation`, `walRecordTxCommit`) with IEEE CRC32 checksums before in-memory index application:
   $$\text{Frame} = [\text{Magic}_{4\text{B}}][\text{Type}_{1\text{B}}][\text{Length}_{4\text{B}}][\text{CRC32}_{4\text{B}}][\text{Payload}]$$
   Every committed transaction executes `g.walFile.Sync()` (`fsync`) to guarantee disk durability.
2. **Composite Key Idempotency**:
   Edge records are uniquely addressed by `EdgeRecord.EdgeKey()`:
   ```go
   fmt.Sprintf("%s|%s|%s", e.SourceID, e.TargetID, e.EdgeType)
   ```
   Concurrent `PutEdge` calls on identical relation keys are safe and idempotent; the second caller updates metadata without corrupting graph topology or creating duplicate relational edges in `g.outgoingEdges` or `g.incomingEdges`.
3. **Micro-Universe Manifest Edge Union**:
   In `UniverseManager.MergeUniverse`, merging cross-boundary edges pools them into an edge set map:
   ```go
   edgeSet[fmt.Sprintf("%s|%s|%s", e.SourceNodeID, e.TargetNodeID, e.Type)] = e
   ```
   Deduplication occurs automatically across non-conflicting micro-universe heads.
4. **Semantic Conflict Reification vs. Lock Stalls**:
   Cosm core engine code must never block waiting for long-held external locks. When concurrent micro-universes introduce conflicting edge contracts (evaluated via `core.DetectContractBreakages`), the collaboration engine raises `SemanticConflict` (`SeverityError`) and persists competing versions as first-class `pkg/distributed/ASTConflictNode`s in the Merkle-DAG rather than throwing merge deadlocks or corrupting graph state.

---

## Rules to Remember

- **Pure Go Only**: Never introduce CGO dependencies (use `modernc.org/sqlite`).
- **Atomic Persistence**: Always persist AST symbol nodes to `.cosm/objects/` before recording node IDs in SQLite.
- **Deduplication**: Unchanged sibling AST symbols must share identical SHA-256 hashes and avoid redundant disk writes.
- **Edge Write Synchronization**: In `GraphEngine`, all edge mutations must be synchronized via `g.mu.Lock()` and committed through framed WAL records with CRC32 verification before updating in-memory indices.
- **IDE Disk Sync**: Commands performing AST surgery (`cosm ast edit`) must auto-synchronize changes to workspace disk files by default (`-w`) to keep VS Code and Language Servers in sync.
- **Always Update Docs**: When modifying schemas, storage formats, or CLI commands, always update `docs/reference/schema-and-storage.md`, `docs/reference/agent-api.md`, and `docs/reference/cli.md` in the same commit.
- **Direct Documentation Style**: Documentation must be strictly technical, direct, and concise (no fluff, exact equations, explicit JSON wire schemas, and copy-pasteable code).

