# Guide 3: Topocosm Hub, Multi-Agent Swarms & Sparse Replication

This guide covers testing the **Topocosm Hub** (`topocosm.dev` local daemon), verifying machine-first agent discovery manifests, benchmarking concurrent multi-agent swarm simulations, and testing sparse AST subgraph replication.

---

## 1. Conceptual Invariants

* **Zero-Docker Local Hub**: Runs 100% locally with pure-Go SQLite WAL persistence and in-memory TTL lease management.
* **Agent Blackboard Leases**: Atomic domain locking for multi-agent swarms to prevent conflicting mutations.
* **Sparse AST Replication**: Agents download only the AST subgraphs they need for their specific tasks, saving bandwidth and memory.

---

## 2. Step-by-Step Validation Walkthrough

### Step 1: Start the Local Topocosm Hub Daemon
Open a dedicated terminal window:
```bash
cosm topocosm dev --port 51204 --dir .topocosm
```
**Expected Output:**
```
=====================================================================
  🪐 TOPOCOSM HUB LOCAL DAEMON (Zero-Docker / Pure-Go)
=====================================================================
  • Server listening on:       http://127.0.0.1:51204
  • Agent Discovery Manifest:  http://127.0.0.1:51204/.well-known/cosm-agent.json
  • REST & gRPC API Root:      http://127.0.0.1:51204/api/v1/
  • Git Smart-HTTP Shim:       http://127.0.0.1:51204/git/{org}/{cosm}.git
  • Local CAS & SQLite Store:  .topocosm/
=====================================================================
Ready for AI Agent Swarms and Developer CLI operations. Press Ctrl+C to stop.
```

### Step 2: Validate Agent Discovery Manifest & Hub Status
In a second terminal window:
```bash
# Query the machine-first discovery manifest
curl -s http://127.0.0.1:51204/.well-known/cosm-agent.json

# Check Hub statistics & active blackboard claims
cosm topocosm status --url http://127.0.0.1:51204
```
**Expected Output for `topocosm status`:**
```
=====================================================================
  🪐 TOPOCOSM HUB STATUS (http://127.0.0.1:51204)
=====================================================================
  • Registered Cosms:        ...
  • Active Agent DIDs:       ...
  • Open Proposals:          ...
  • Active Blackboard Locks: ...
  • Content Blobs in Store:  ...
  • Server Uptime:           ... seconds
=====================================================================
```

### Step 3: Run the Multi-Agent Swarm Simulation Benchmark
Simulate 25 concurrent virtual agent workers competing for blackboard domain claims, pulling sparse AST subgraphs, and merging CRDT proposals:
```bash
cosm topocosm test-swarm --agents 25 --duration 3s
```
**Expected Output:**
```
🚀 Starting Multi-Agent Swarm Simulation (25 agents, 3s duration)...
=====================================================================
  ⚡ MULTI-AGENT SWARM SIMULATION BENCHMARK REPORT
=====================================================================
  • Total Operations Executed: >6000
  • Operations / Second:       >2000 op/s
  • Sparse Subtree Pulls:      ...
  • Blackboard Claims Won:     ...
  • Claim Conflicts Avoided:   ...
  • CRDT Proposals Merged:     ...
  • Simulation Duration:       3s
=====================================================================
```
* **Validation Check**: Verify that throughput exceeds 2,000 op/s with zero deadlock or data corruption.

### Step 4: Publish a Local Workspace to the Hub
From your test workspace (`~/cosm-test-workspace`), publish your universe to Topocosm Hub:
```bash
cd ~/cosm-test-workspace
cosm publish http://127.0.0.1:51204/demo-org/cloud-platform -u universe-main -i "Publish platform to Topocosm"
```
**Expected Output:**
```
🚀 Successfully published demo-org/cloud-platform to Topocosm Hub!
   • Merkle Root: ...
   • Blobs Synced: 2
   • Universe:     universe-main
```

### Step 5: Test Sparse AST Clone
Clone only a specific component to a new directory without downloading unrelated repository files:
```bash
cd ~
cosm clone http://127.0.0.1:51204/demo-org/cloud-platform ./sparse-workspace --sparse "server.go"
```
**Expected Output:**
```
✅ Successfully cloned demo-org/cloud-platform to ./sparse-workspace!
   • Merkle Root:     ...
   • Components:      1
   • AST Symbols:     ...
   • Sparse Bandwidth Savings: ...%
```
* **Validation Check**: Verify that `~/sparse-workspace/server.go` exists while unrequested components are omitted.

---

## 3. Distributed Cloud Run Swarm & Gemini 3.8 Flash Rater Judge

When operating at enterprise scale, autonomous code agents do not run on a developer laptop—they execute inside isolated **Google Cloud Run Jobs** targeting a shared **Topocosm Cloud Hub** service.

### Multi-Agent Swarm Topology
```
┌────────────────────────────────────────────────────────┐
│               Topocosm Cloud Hub (Cloud Run)           │
│    - GCS Content-Addressed Blob Storage (CAS)          │
│    - Memorystore for Redis Blackboard Domain Leases    │
│    - Cloud Pub/Sub CRDT Event Notification Stream      │
└────────▲──────────────────▲──────────────────▲─────────┘
         │                  │                  │
         │ HTTP / REST      │ HTTP / REST      │ HTTP / REST
         │ (W3C Tracing)    │ (W3C Tracing)    │ (W3C Tracing)
┌────────┴────────┐┌────────┴────────┐┌────────┴────────┐
│  Cloud Run Job  ││  Cloud Run Job  ││  Cloud Run Job  │
│  (Task 0)       ││  (Task 1)       ││  (Task 2)       │
│  Agent Zero     ││  Agent Alpha    ││  Agent Beta     │
│  [Bootstrap]    ││  [Backend Orders││  [Frontend UI]  │
└─────────────────┘└─────────────────┘└─────────────────┘
```

### Swarm Execution via `run_cloudrun_agent_swarm.sh`
```bash
# Execute Cloud Run multi-agent job swarm (100 parallel worker tasks)
export TOPOCOSM_HUB_URL="https://topocosm-hub-uc.a.run.app"
export COSM_REPO="cosm/fintech-mesh"
export GEMINI_MODEL="gemini-3.8-flash"
export TASK_COUNT=100
export AGENT_JOB_NAME="cosm-agent-swarm-100"
./deploy/scripts/run_cloudrun_agent_swarm.sh
```

### Two-Tier Multi-Agent Testing Architecture

To ensure both thorough backplane stress testing and authentic AI model verification without wasting API quotas or masking failures, Cosm enforces a strict **Two-Tier Testing Model**:

| Tier | Name | LLM Mode | Purpose & Focus | Execution Command |
|---|---|---|---|---|
| **Tier 1** | **Deterministic SCM Concurrency & Backplane Benchmark** | Deterministic Scripted (`--mock=true`) | Stresses high-concurrency SCM invariants: Blackboard mutual-exclusion domain leases (`services/orders-0..3`), atomic CAS blob store deduplication, and CRDT semilattice proposal merges across 100 concurrent workers without exhausting Vertex AI rate limits (~1,000 live calls avoided). | `go test -v ./test/agents/ -run TestCloudRun_100Tasks_Concurrency` |
| **Tier 2** | **Live Scaled Agent Efficacy & Judge Audits** | Unmocked Live (`--mock=false`) | Audits true autonomous model efficacy: Unmocked Gemini 3.8 Flash agents receive natural-language directives, dynamically plan, invoke Cosm AST tools, stage code, and push stacked proposals. Unmocked Gemini 3.8 Flash Rater Judge audits tool fidelity, token economy, and awards qualitative scorecards. | `go test -v ./test/agents/ -run TestLiveVertex_AgentEfficacy_RealScale` |

#### Zero-Mock Enforcement in Live Execution
When executing in live mode (`--mock=false` / default in production Cloud Run jobs), Cosm strictly enforces the **STRICT NEVER MOCK DIRECTIVE**:
- Worker tasks automatically persist their authentic `AgentSession` telemetry to `$COSM_SESSIONS_DIR/session-task-<idx>.json`.
- The standalone Rater Judge scans `$COSM_SESSIONS_DIR` for completed worker sessions. If no authentic session files are detected, the judge immediately aborts with a fatal error:
  `FATAL: no genuine worker sessions found in /tmp/cosm-sessions: live judge requires authentic execution sessions from completed workers (STRICT NEVER MOCK DIRECTIVE)`.
- Synthetic or pre-canned fallbacks are prohibited during live runs.

### High-Scale 100-Task Swarm Concurrency Suite
Cosm and Topocosm include dedicated 100-task end-to-end concurrency test suites:
- `cosm`: `test/agents/cloudrun_100tasks_concurrency_test.go`
- `topocosm`: `pkg/server/e2e_cloudrun_100tasks_test.go`

The 100 tasks are partitioned across 6 specialized agent cohorts targeting the same repository:
1. **Task 0 (Bootstrap)**: Initializes `universe-main`, stages polyglot AST symbols, and publishes initial Merkle root.
2. **Tasks 1..20 (Backend)**: Contends for domain leases (`services/orders-0..3`), creates isolated zero-copy micro-universes (`u/agent-backend-X`), commits Go AST mutations, publishes, and stacks proposals.
3. **Tasks 21..40 (Frontend)**: Sparse-pulls `apps/checkout`, branches zero-copy micro-universes (`u/agent-frontend-X`), commits TypeScript components, and opens stacked proposals.
4. **Tasks 41..60 (Infra)**: Sparse-pulls `infra/cloudrun`, branches to `u/agent-infra-X`, validates Terraform HCL syntax, commits, and opens stacked proposals.
5. **Tasks 61..80 (Contenders)**: Floods mutual-exclusion lease requests on `services/orders-0..3` to validate atomic lease locks and HTTP 409 conflict handling.
6. **Tasks 81..99 (Observers)**: Concurrently queries proposal CRDTs and inspects Merkle DAG topologies.

### End-to-End Evaluation Oracle: Gemini 3.8 Flash Rater Judge
At the conclusion of a swarm run, `rater.NewRaterJudge` inspects the complete execution trace:
1. **Cosm Utilization (Max 25 pts)**: Verifies agents used official AST tools (`cosm_claim`, `cosm_clone`, `cosm_ast_edit`, `cosm_publish`, `cosm_stack_create`). Flags any direct file write bypasses (`write_file`, `overwrite_file`).
2. **Time Efficiency (Max 20 pts)**: Normalized per session (`avgDuration = totalDuration / numSessions`) to accommodate 100+ concurrent workers without false positive timeouts.
3. **Token Economics (Max 20 pts)**: Normalized per session (`avgTokens = totalTokens / numSessions`) to audit token consumption against complexity thresholds.
4. **Output Quality & Contract Integrity (Max 35 pts)**: Validates AST syntax, cross-boundary contract bindings (`CONSUMES_API`, `BINDS_ENV`), and Merkle DAG integrity.

To execute the automated end-to-end swarm tests:
```bash
# Cosm 5-agent baseline E2E
go test -v ./test/agents/ -run TestCloudRun_PolyglotSwarmE2E

# Cosm Tier 1 100-agent deterministic concurrency suite
go test -v ./test/agents/ -run TestCloudRun_100Tasks_Concurrency

# Cosm Tier 2 Live Vertex AI Gemini 3.8 Flash Agent Efficacy suite
go test -v ./test/agents/ -run TestLiveVertex_AgentEfficacy_RealScale

# Topocosm 100-agent concurrency suite
go test -v ./pkg/server/ -run TestE2E_CloudRun_100Tasks_Concurrency
```

---

## 4. Validation Summary Checklist

- [ ] `cosm topocosm dev` starts cleanly without external services or Docker.
- [ ] Agent discovery endpoint `/.well-known/cosm-agent.json` returns capabilities.
- [ ] Multi-agent swarm benchmark achieves >2,000 op/s with conflict avoidance.
- [ ] Workspaces publish cleanly to the hub over HTTP.
- [ ] Sparse clone fetches isolated AST subgraphs with reported bandwidth savings.
- [ ] Multi-agent Cloud Run swarm executes with W3C distributed trace correlation.
- [ ] Tier 1 100-task concurrency suite validates mutual-exclusion domain leases and HTTP 409 conflict handling.
- [ ] Tier 2 Live Gemini 3.8 Flash Rater Judge audits authentic tool fidelity, normalized token economy, and awards Score >= 85.0 (Grade A/A+).
- [ ] Live execution strictly enforces genuine session telemetry, rejecting mock fallbacks under STRICT NEVER MOCK DIRECTIVE.


