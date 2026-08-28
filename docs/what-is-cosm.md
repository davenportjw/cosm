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

## High-Level Architecture

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
