# Micro-Universes & Parallel Swarms

In Cosm, branches are modeled as **Micro-Universes**. A micro-universe is a lightweight, zero-copy pointer to a content-addressed `WorkspaceManifestNode`.

---

## Why Micro-Universes Matter for AI Swarms

Traditional Git branches require full file-system checkout operations or heavy disk worktrees (`git worktree add`). When orchestrating swarms of 5, 10, or 50 autonomous AI coding agents:
1. Disk space balloons rapidly.
2. File-locking conflicts prevent concurrent mutations.
3. Merging parallel agent outputs results in messy rebase cascades and silent logical bugs.

Cosm solves this by storing software as an immutable AST Merkle-DAG. Agents create micro-universes in memory with zero disk overhead, execute localized AST mutations in parallel, and evaluate competing solutions objectively before collapsing the winner into production.

---

## 1. Managing Micro-Universes

### List Active Universes
```bash
cosm universe list
```
**Output:**
```
🌌 Active Micro-Universes:
  * universe-main (Head: 4f2b90d81a9e, Status: active)
```

### Fork a New Micro-Universe
Create an isolated universe branched from `universe-main`:
```bash
cosm universe create u/agent-auth-fix -p universe-main
```
**Output:**
```
✨ Created micro-universe 'u/agent-auth-fix' branched from 'universe-main' (Head: 4f2b90d81a9e)
```

### Compare Universes
Diff the AST components and cross-boundary contracts between two micro-universes:
```bash
cosm universe diff --source u/agent-auth-fix --target universe-main
```
**Output:**
```
=== Universe Diff: universe-main <-> u/agent-auth-fix ===
Added Components:    [services/auth/jwt.go]
Removed Components:  []
Modified Symbols:    [services/auth.ValidateToken, infra/iam.tf.role_auth]
Cross-Edge Delta:    +2 edges (CONSUMES_API, BINDS_ENV)
```

---

## 2. Autonomous Swarm Workflow & Auto-Collapsing

```mermaid
graph TD
    Main["universe-main<br/>(Canonical Timeline)"]
    
    Main -->|Zero-Copy Fork| U1["u/agent-1<br/>(JWT Refactor)"]
    Main -->|Zero-Copy Fork| U2["u/agent-2<br/>(OAuth2 Refactor)"]
    Main -->|Zero-Copy Fork| U3["u/agent-3<br/>(Session Token Refactor)"]

    U1 -->|Benchmark & Tests| E1["Fitness: 0.82<br/>(Fails 1 integration test)"]
    U2 -->|Benchmark & Tests| E2["Fitness: 0.98<br/>(All tests pass, lowest latency)"]
    U3 -->|Benchmark & Tests| E3["Fitness: 0.65<br/>(Contract drift detected)"]

    E2 -->|MCTS Tree Collapse| Collapsed["Merged into universe-main"]
```

### Executing Swarm Collaboration
1. **Orchestrator Agent Forks 3 Parallel Universes**:
   ```bash
   cosm universe create u/agent-1 -p universe-main
   cosm universe create u/agent-2 -p universe-main
   cosm universe create u/agent-3 -p universe-main
   ```
2. **Subagents Mutate AST Nodes Concurrently**:
   Each agent targets their specific universe ID (`-u u/agent-1`, `-u u/agent-2`, etc.) via CLI or high-speed in-process gRPC/REST APIs.
3. **Automated Fitness Evaluation**:
   Cosm evaluates build status, test pass rates, benchmark latency, and contract safety.
4. **Merge Winner into Canonical Timeline**:
   ```bash
   cosm universe merge --source u/agent-2 --target universe-main
   ```
