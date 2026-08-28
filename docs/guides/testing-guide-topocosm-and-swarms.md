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

## 3. Validation Summary Checklist

- [ ] `cosm topocosm dev` starts cleanly without external services or Docker.
- [ ] Agent discovery endpoint `/.well-known/cosm-agent.json` returns capabilities.
- [ ] Multi-agent swarm benchmark achieves >2,000 op/s with conflict avoidance.
- [ ] Workspaces publish cleanly to the hub over HTTP.
- [ ] Sparse clone fetches isolated AST subgraphs with reported bandwidth savings.
