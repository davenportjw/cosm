# Contributing to Cosm Core: Systems Architecture & Engineering Runbook

This guide specifies the invariants, coding standards, storage crash recovery rules, and verification procedures for engineers and AI agents contributing to the **Cosm core engine** (`cosm`).

---

## 1. Core Engine Invariants

1. **Pure Go 1.22+ & Zero-CGO**:
   - All core packages (`pkg/core`, `pkg/storage`, `pkg/codecs`, `pkg/mutation`, `pkg/materialize`, `pkg/mcp`, `pkg/lsp`, `pkg/gitshim`, `pkg/api`) MUST compile with `CGO_ENABLED=0`.
   - Never import packages that link to external C shared libraries. Use pure-Go SQLite (`modernc.org/sqlite`).

2. **Storage Durability & WAL Framing**:
   - Immutable AST blobs in `.cosm/objects/` are content-addressed by SHA-256 and committed using atomic write-and-rename (`fsync`).
   - The graph engine in `.cosm/graph.db` logs mutation transactions to custom Write-Ahead Log (`WAL`) records framed with IEEE CRC32 checksums:
     $$\text{Frame} = [\text{Magic}_{4\text{B}}][\text{Type}_{1\text{B}}][\text{Length}_{4\text{B}}][\text{CRC32}_{4\text{B}}][\text{Payload}]$$
   - Sudden process termination or OS crash must result in zero corrupted records upon restart.

3. **Composite Edge Key Idempotency**:
   - Edges in `GraphEngine` are uniquely identified by:
     $$\text{EdgeKey} = \text{SourceID} \parallel \text{"\|"} \parallel \text{TargetID} \parallel \text{"\|"} \parallel \text{EdgeType}$$
   - Concurrent `PutEdge` calls on identical keys are idempotent and must not create duplicate relational entries in `g.outgoingEdges` or `g.incomingEdges`.

4. **Non-Blocking Micro-Universe Branching**:
   - Never hold long-lived global locks across branches or proposals.
   - Conflicting cross-boundary AST mutations must be reified as first-class `ASTConflictNode`s in the Merkle-DAG rather than stalling the pipeline or deadlocking.

---

## 2. Directory Layout & Module Responsibilities

```
cosm/
├── cmd/
│   ├── cosm/                 # Main CLI binary (init, add, commit, ast, mcp, lsp, ship)
│   └── cosm-agent-harness/   # Autonomous test agent and rater oracle
├── pkg/
│   ├── api/                  # REST and gRPC wire DTOs and HTTP handlers
│   ├── codecs/               # Polyglot AST parsers (Go, TS, Python, HCL, Rust, etc.)
│   ├── core/                 # Schemas (ASTSymbolNode, ComponentNode), Merkle hasher, Linker
│   ├── distributed/          # Radicle-inspired CRDT COBs, stacked proposals, P2P sync
│   ├── gitshim/              # Git CLI compatibility shim and synthetic .git generator
│   ├── lineage/              # Provenance tracer, audit engine, Ed25519 signing
│   ├── lsp/                  # Language Server Protocol stdio server & diagnostics
│   ├── materialize/          # AST hydrator, terminal viewer, and MemoryVFS
│   ├── mcp/                  # Model Context Protocol stdio server (tools/list, tools/call)
│   ├── mutation/             # Declarative AST surgery engine (9 Geometric verbs)
│   ├── shipping/             # Ephemeral staging compiler and preview sidecar
│   ├── storage/              # CAS BlobStore, SQLite WAL GraphEngine, UniverseManager
│   └── watcher/              # Recursive filesystem watcher for auto-staging
├── test/
│   └── agents/               # Autonomous test agent loop, LLM providers, and oracles
└── editors/
    └── vscode/               # Official VS Code / Antigravity IDE extension (TypeScript)
```

---

## 3. Local Development & Testing Workflows

### A. Compiling the Cosm CLI Binary
```bash
cd /Users/jasondavenport/GitHub/cosm
go build -o cosm ./cmd/cosm
./cosm status
```

### B. Running Hermetic Automated Tests
Run all package tests:
```bash
go test -v ./pkg/...
```

Run race detector across storage, core, and mutation packages:
```bash
go test -v -race ./pkg/storage/... ./pkg/core/... ./pkg/mutation/... ./pkg/mcp/... ./pkg/lsp/...
```

### C. Testing Storage Crash Recovery & Durability
To test SQLite WAL durability:
```go
// 1. Open storage and insert records
store, graph, err := storage.OpenWorkspaceStorage(tempDir)
_ = graph.PutNode(testNode)

// 2. Force terminate or close
graph.Close()

// 3. Re-open and verify node exists with valid Merkle hash
store2, graph2, err := storage.OpenWorkspaceStorage(tempDir)
defer graph2.Close()
retrieved, err := graph2.GetNode(testNode.NodeID)
```

### D. Testing Autonomous Agent Harness
Execute autonomous evaluation scenarios:
```bash
go test -v ./test/agents/...
```

---

## 4. Architectural Rules for PRs & Proposals

1. **Always Update Documentation**: Any change altering schemas, storage binary formats, CLI commands, or wire protocols must update reference documentation under `docs/reference/` in the same commit.
2. **Deterministic Merkle Roots**: Ensure `core.HashWorkspaceManifest` produces identical SHA-256 hashes across platforms regardless of map iteration order (always sort component IDs and edge keys before hashing).
3. **Preserve Lineage Telemetry**: All commit paths must propagate `LineageEnvelope` containing `UserPrompt`, `SessionID`, `ExecutingAgentID`, `LLMVersion`, and `Ed25519` signature.
