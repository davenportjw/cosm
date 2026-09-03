# What is Cosm?

**Cosm** is an AI-native, polyglot Source Control Management (SCM) system and target compilation engine. 

Instead of tracking software as raw, unstructured text files and line-by-line diffs (`+` / `-` lines), Cosm models and stores repositories as an immutable, content-addressed **Abstract Syntax Tree (AST) Merkle-DAG** with first-class **semantic cross-domain edges**.

```
┌────────────────────────────────────────────────────────────────────────┐
│                        COSM WORKSPACE MANIFEST                         │
├───────────────────┬───────────────────────────────┬────────────────────┤
│   FRONTEND TIER   │         BACKEND TIER          │     INFRA TIER     │
│  React / TSX AST  │     Go SSA / Python AST       │   Terraform HCL    │
│  (Button onClick) ───[CONSUMES_API]───> (Route) ───[DEPLOYS_TO]───> (Cloud) │
└───────────────────┴───────────────────────────────┴────────────────────┘
```

---

## Cosm vs. Topocosm Architecture Division

Cosm splits software source control and distribution into two distinct, decoupled systems:

```
┌────────────────────────────────────────────────────────────────────────┐
│               TOPOCOSM (The Decentralized Hub / Mesh)                  │
│               Standalone Repo: github.com/cosmscm/topocosm             │
│                                                                        │
│  • Agent Discovery (/.well-known/cosm-agent.json)                      │
│  • Multi-Repo Catalog & Registry (/api/v1/cosms/)                      │
│  • Distributed Blackboard Domain Leases (Parallel Swarm Locks)         │
│  • Sparse Subgraph Distribution & CAS Object Replication               │
│  • Multi-Peer CRDT Proposal Convergence (COBs)                         │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
               Wire Protocol & DTOs (pkg/api)
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                  COSM (The Local Core Engine / CLI)                    │
│                  Local Monorepo: github.com/cosmscm/cosm               │
│                                                                        │
│  • Polyglot AST Codecs (Go, TS, Py, HCL, Rust, Protobuf, SQL)          │
│  • Content-Addressed Blobstore (.cosm/objects/)                        │
│  • Pure-Go SQLite WAL Graph Engine (.cosm/graph.db)                    │
│  • Declarative AST Surgery (9 Geometric Verbs)                         │
│  • AST-to-Source Hydration & Local Git CLI Shim                        │
│  • Jujutsu-Style Stacked Proposals & Auto-Evolution                    │
│  • Embedded Target Shipping Sidecar (Sub-second local preview)         │
└────────────────────────────────────────────────────────────────────────┘
```

---

## Core Problems Solved & Direct Benefits

### 1. Eliminates Silent Semantic Collisions
* **Traditional Git Problem**: Git merges code based on line adjacency. If Agent A changes a function's return type in `api.go` and Agent B adds a caller in `client.go`, Git merges cleanly without conflict, shipping a broken build to CI.
* **Cosm Benefit**: Cosm parses code into AST symbols and tracks typed cross-boundary contracts (`CONSUMES_API`, `DEPLOYS_TO`, `BINDS_ENV`). If a change alters or breaks an API contract, Cosm detects the collision immediately at commit time before code is pushed.

### 2. Zero-Copy Parallel Micro-Universes for AI Agent Swarms
* **Traditional Git Problem**: Running 10 concurrent AI agents requires cloning or checking out 10 isolated disk worktrees, creating massive disk I/O, storage bloat, and painful rebase storms.
* **Cosm Benefit**: Cosm provides zero-copy **Micro-Universes**. Swarms of autonomous agents fork lightweight, memory-efficient universes (`cosm universe create u/agent-1`), mutate specific AST nodes in parallel, evaluate their solution quality, and auto-collapse the winning universe using Monte Carlo Tree Search (MCTS).

### 3. Unbroken Cryptographic Causal Lineage
* **Traditional Git Problem**: Git commit authors are unverified email strings with arbitrary commit messages that provide no insight into why code was generated or which prompt produced it.
* **Cosm Benefit**: Every symbol in Cosm contains a cryptographic `LineageEnvelope` signed with Ed25519 that records:
  $$\text{User Prompt} \longrightarrow \text{Session ID} \longrightarrow \text{Agent ID} \longrightarrow \text{LLM Model} \longrightarrow \text{AST Node}$$
  You can trace any single line of code in production directly back to the originating prompt and agent that created it via `cosm lineage <node_id>`.

### 4. Sub-Second Target Previews & Shipping Sidecar
* **Traditional Git Problem**: Code review happens in isolation from deployment. Reviewers must wait for remote CI/CD pipelines to build containers and spin up staging environments.
* **Cosm Benefit**: Cosm embeds a Go shipping sidecar. Running `cosm ship` compiles backend services, executes automated `terraform fmt` / `terraform validate` hooks, packages the polyglot stack, and boots an ephemeral local preview sandbox in under 1 second.

---

## Human Mental Models for Cosm

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. THE GOOGLE DOCS ANALOGY                                                  │
│    Git is like emailing Word documents back and forth. Cosm is like Google  │
│    Docs with track-changes: everyone has a local copy, and edits merge      │
│    together smoothly in the background using mathematical CRDTs.            │
├─────────────────────────────────────────────────────────────────────────────┤
│ 2. THE AST SYMBOL vs. LINE ADJACENCY ANALOGY                                │
│    Git panics if you and a teammate edit adjacent lines in the same file.   │
│    Cosm knows you changed function 'login()' and they changed 'logout()'.   │
│    Because they are distinct AST symbols, there is zero merge conflict.    │
├─────────────────────────────────────────────────────────────────────────────┤
│ 3. THE 3-STORY HOUSE (STACKING) ANALOGY                                     │
│    Building a feature is like building a 3-story house:                     │
│    Floor 1 (DB Schema) -> Floor 2 (API Route) -> Floor 3 (Frontend UI).     │
│    In Git, modifying Floor 1 collapses upper floors (rebase hell).          │
│    In Cosm, modifying Floor 1 automatically adjusts the floors above it.    │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Comparison: Git vs. GitHub vs. Jujutsu vs. Radicle vs. Cosm

| Capability | Traditional Git | GitHub.com | Jujutsu (`jj`) | Radicle | Cosm (`cosm`) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Primary Scope** | Local VCS & DAG | Centralized Cloud Forge | Local VCS & Stacked Diffs | P2P Code Collaboration | **AI-Native AST SCM & Swarm Mesh** |
| **Unit of Storage** | Text Blobs | Relational DB + Git | Text Commits | Git Storage + CRDTs | **Content-Addressed AST Merkle-DAG** |
| **Cross-Domain Contracts**| None | None (CI deferred) | None | None | **Typed Semantic Edges (`CONSUMES_API`, etc.)** |
| **Pull Request / Review** | Raw Patch Emails | Web PRs (Text lines) | None (defers to forge) | P2P Patch COBs | **Dual View (Text Diff + AST Cards + Topology)** |
| **Stacked Changes** | Manual rebase hell | Poor (rebases break) | First-class Stacked Diffs| Standard patch stacks | **Native Stacked Proposals & Auto-Evolution** |
| **Multi-Agent Swarm Sync**| High conflict rates | Central ref locks | Good conflict tracking | P2P Gossip Sync | **Sparse AST Sync + MCTS Fitness Collapsing** |
| **Lineage & Provenance** | Unverified author | OAuth User Account | Git Commit Metadata | Cryptographic DIDs | **Ed25519 Prompt-to-Code Causal Lineage** |
| **Local Compilation / Build**| External | Central CI | External | External | **Embedded Target Shipping Sidecar** |

---

## High-Level Storage Layout

Cosm stores all repository state under `.cosm/`:

```
my-project/
├── .cosm/
│   ├── config.json             # Workspace targets, default universe, active policy
│   ├── objects/                # Content-addressed immutable AST object store
│   │   ├── 4f/8a2e...bin       # Serialized AST symbol & component nodes
│   │   └── e1/9b4c...bin
│   ├── graph.db                # Pure-Go SQLite WAL: nodes, semantic edges, and oplog
│   ├── graph.db-wal            # Write-Ahead Log ensuring crash durability
│   ├── vector.db               # Embedded vector embeddings for semantic prompt memory
│   └── attestations/           # In-toto / Ed25519 signatures for verified nodes
```
