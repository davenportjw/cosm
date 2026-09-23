# Empirical Benchmarks, Bandwidth & Runtime Savings Reference

This document provides mathematically rigorous, empirically measured performance benchmarks and resource savings for the Cosm AST Source Control System and Topocosm Distribution Hub.

All metrics documented below are directly reproducible via hermetic Go unit tests and benchmarks located in [`pkg/benchmarks/savings_test.go`](../../pkg/benchmarks/savings_test.go) and [`topocosm/pkg/benchmarks/swarm_test.go`](https://github.com/cosmscm/topocosm/blob/main/pkg/benchmarks/swarm_test.go).

---

## 1. Mathematical Savings Formulation

### 1.1 Sparse Subtree Pull Bandwidth Savings

When an autonomous agent or developer workspace requests a scoped AST slice (e.g., a single component or module), Cosm filters the Merkle DAG to retrieve only the matching `ComponentNode`, its constituent `ASTSymbolNode` instances, and required 1-hop cross-boundary contracts, omitting unreferenced sibling trees.

$$\text{BandwidthSavings}(\%) = \left(1.0 - \frac{|\mathcal{B}_{\text{sparse}}|}{|\mathcal{B}_{\text{full}}|}\right) \times 100\%$$

where:
* $|\mathcal{B}_{\text{sparse}}|$ is the total byte payload of the filtered components, constituent symbols, and workspace manifest slice.
* $|\mathcal{B}_{\text{full}}|$ is the total byte payload of the entire repository Merkle-DAG.

Because repository size scales with the number of components $C$ and symbols per component $S$, the asymptotic savings for retrieving $k$ components from an $N$-component repository is:

$$\lim_{N \to \infty} \text{BandwidthSavings} = \left(1.0 - \frac{k}{N}\right) \times 100\%$$

### 1.2 Agent Token Context Savings (Input Window)

In traditional Git workflows, LLMs must ingest entire source files (including unrelated classes, imports, and private helpers) to make an edit. In Cosm, agents request only the targeted symbol node and its 1-hop signature dependencies.

$$\text{TokenContextSavings}(\%) = \left(1.0 - \frac{T_{\text{symbol}} + T_{\text{contracts}}}{T_{\text{file}}}\right) \times 100\%$$

### 1.3 Agent Surgical Output Token Savings (Generation Window)

To modify 3–10 lines of code in a 500–2,000 line file, a standard LLM agent outputs a full file rewrite or expansive unified diff. In Cosm, the agent issues a declarative AST mutation verb (`replace_function_body`, `insert_parameter`) containing only the modified symbol payload.

$$\text{OutputTokenSavings}(\%) = \left(1.0 - \frac{T_{\text{mutation\_payload}}}{T_{\text{file\_rewrite}}}\right) \times 100\%$$

---

## 2. Empirical Benchmark Results

### 2.1 Bandwidth Savings Across Repository Scales

Measurements obtained via `TestEmpiricalBandwidthSavings` running against synthetic polyglot AST repositories (Go, TypeScript, Python, HCL, SQL):

| Repository Scale | Topology (Components / Symbols) | Full Repo Payload | Sparse Subtree Payload | Byte Savings | Full Blobs Count | Sparse Blobs Count | Blob Count Savings |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Small Workspace** | 5 components / 25 symbols | 30.1 KB | 5.8 KB | **80.55%** | 31 | 6 | **80.65%** |
| **Medium Workspace** | 25 components / 150 symbols | 176.4 KB | 6.9 KB | **96.07%** | 176 | 7 | **96.02%** |
| **Large Multi-Service** | 100 components / 600 symbols | 705.7 KB | 6.9 KB | **99.02%** | 701 | 7 | **99.00%** |
| **Reference Monorepo Slice** | `demo-org/cloud-platform` | 350.0 KB | 4.2 KB | **98.80%** | 85 | 3 | **96.47%** |

> **Key Grounding**: Claims of "~98.8% savings" correspond specifically to retrieving an isolated service slice from a medium-to-large multi-tier architecture (~85–100 components). On smaller 5-component workspaces, the reduction is ~80.6%. Savings scale monotonically with repository size.

---

### 2.2 LLM Token Consumption: AST Symbols vs. Whole Files (Git vs. Cosm)

Measurements obtained via [`pkg/benchmarks/token_git_vs_cosm_test.go`](../../pkg/benchmarks/token_git_vs_cosm_test.go) and [`pkg/benchmarks/savings_test.go`](../../pkg/benchmarks/savings_test.go) running against actual repository components (`cmd/server/main.go`, `internal/environmental/environmental.go`, `internal/lease/lease.go`, `internal/ranger/ranger.go`):

#### 2.2.1 Real-File Single-Symbol Surgery vs. Whole-File Git Context

| Real Repository Component | Git Prompt (Whole File) | Cosm Prompt (AST Symbol) | Prompt Savings | Git Output (Diff) | Cosm Output (AST Verb) | Total Token Savings |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `cmd/server/main.go` (1,729 lines) | 22,657 tokens | 1,880 tokens | **91.70%** | 163 tokens | 121 tokens | **91.23%** |
| `internal/environmental/` (550 lines) | 5,880 tokens | 63 tokens | **98.93%** | 174 tokens | 120 tokens | **96.98%** |
| `internal/ranger/` (380 lines) | 2,585 tokens | 299 tokens | **88.43%** | 167 tokens | 120 tokens | **84.77%** |
| `internal/lease/` (220 lines) | 1,639 tokens | 535 tokens | **67.36%** | 163 tokens | 117 tokens | **63.82%** |

* **Whole-File Rewrite Elimination**: For coding agents that perform whole-file overwrites (to prevent unified diff line-drift hallucinations), Git generation costs **22,611 tokens** vs Cosm's **121 tokens** (**99.46% output token reduction**), cutting generation latency from ~20s to <1s.

#### 2.2.2 Multi-Turn Autonomous Agent Trajectory (5 Iterations)

When an agent iterates through a 5-step debugging loop (Prompt $\rightarrow$ Syntax Error $\rightarrow$ Fix $\rightarrow$ Linter Rule $\rightarrow$ Telemetry Instrumentation), Git resends the updated whole file on each turn, causing rapid context ballooning:

| Iteration | Git Turn Tokens | Git Cumulative | Cosm AST Turn Tokens | Cosm AST Cumulative | Turn Token Savings |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Turn 1** | 22,831 tokens | 22,831 tokens | 380 tokens | 380 tokens | **98.34%** |
| **Turn 2** | 22,931 tokens | 45,762 tokens | 420 tokens | 800 tokens | **98.17%** |
| **Turn 3** | 23,031 tokens | 68,793 tokens | 460 tokens | 1,260 tokens | **98.00%** |
| **Turn 4** | 23,131 tokens | 91,924 tokens | 500 tokens | 1,760 tokens | **97.84%** |
| **Turn 5** | 23,231 tokens | 115,155 tokens | 540 tokens | 2,300 tokens | **97.68%** |
| **Total Trajectory** | — | **115,155 tokens** | — | **2,300 tokens** | **98.00%** |

* **Cost Reduction (Gemini 3.8 Flash)**: Git = **\$0.00877** vs Cosm = **\$0.00022** (**97.46% cost savings**).
* **Cost Reduction (Gemini 3.8 Pro)**: Git = **\$0.14620** vs Cosm = **\$0.00370** (**97.46% cost savings**).

#### 2.2.3 Merge Conflict Resolution: Git Hunks vs. AST Conflict Nodes

* **Git 3-Way Conflict**: Sending two conflicting branches with line markers (`<<<<<<< HEAD`, `=======`, `>>>>>>>`) plus surrounding file context consumes **5,606 tokens**.
* **Cosm `ASTConflictNode`**: Reified conflict node transmits only the structured Base, Side A, and Side B AST symbol payloads, consuming **150 tokens** (**97.32% savings**).
* **Disjoint Symbol Modifications**: When two agents edit different functions within the same file, Git requires line reconciliation, whereas Cosm performs a 100% automated CRDT join consuming **0 conflict tokens** (**100% savings**).

#### 2.2.4 128k Context Window Headroom & Saturation

* **Git Whole-File Exhaustion**: An agent editing a ~1,700-line service exhausts a standard 128,000-token context window in **7 iterations** before requiring context truncation or compression.
* **Cosm AST Headroom**: The same agent can sustain **320 iterative surgical mutations** (**45.7x greater operational longevity**) before saturating the context window.

---

#### 2.2.5 Live Antigravity Subagent Transcript Telemetry (Empirical Session Recording)

To audit actual LLM context window ingestion and generation in a real agentic runtime, two autonomous subagents executed the identical modification task on `examples/camping_app/cmd/server/main.go` (1,730 lines, 71,233 bytes) targeting `Server.HandleHealth`:
1. **Subagent A (File-Based / Traditional)**: Inspected the full file using `view_file` and applied edits via string replacement.
2. **Subagent B (Cosm AST Surgeon)**: Inspected only the target symbol using `cosm view "main.(Server).HandleHealth"` and applied in-place AST surgery via `cosm ast edit --op replace_function_body -w`.

The actual Antigravity JSONL session transcripts (`.system_generated/logs/transcript_full.jsonl`) were harvested and parsed by `test/agents/log_token_harvester.py` and audited via `TestEmpiricalSubagentTranscripts_FileVsAST`:

| Empirical Metric | File-Based Agent | Cosm AST Surgeon | Delta / Reduction |
| :--- | :--- | :--- | :--- |
| **Total Subagent Steps** | 22 steps | 38 steps | +16 steps |
| **LLM Invocations (Turns)** | 11 turns | 19 turns | +8 turns |
| **Tool Output Ingestion Payload** | 50,527 chars | 17,759 chars | **-64.85%** |
| **Average Context Window per Turn** | 43,800 chars (12,167 tok) | 18,257 chars (5,071 tok) | **-58.32%** |
| **Final Context Window at Completion** | 55,492 chars (15,414 tok) | 27,604 chars (7,668 tok) | **-50.26%** |
| **Cumulative Prompt Ingested** | 482,057 chars (133,905 tok) | 345,780 chars (96,050 tok) | **-28.27%** |
| **Cumulative Total Billed Tokens** | 135,426 tokens | 99,783 tokens | **-26.32%** |

##### Turn-by-Turn Prompt Accumulation Dynamics

| Turn # | File-Based Prompt Context | Cosm AST Prompt Context | Turn Context Delta | Notes |
| :--- | :--- | :--- | :--- | :--- |
| **Turn 1** | 999 chars | 1,403 chars | +40.4% | Initial user prompt with instructions |
| **Turn 2** | 33,858 chars | 12,617 chars | **-62.7%** | File agent reads `main.go` (+3,289% context explosion) |
| **Turn 4** | 48,270 chars | 15,100 chars | **-68.7%** | File agent reads test file context |
| **Turns 5–11** | 50,046 – 55,325 chars | 15,674 – 19,729 chars | **-64.0% to -68.7%** | Sustained context savings across iterative execution |

*Even with 8 additional verification and fresh-cache compilation cycles (+72% more turns)*, the Cosm AST surgeon consumed **35,643 fewer total billed tokens** (-26.32%) because each individual turn operated over a 58.32% smaller context window.

---

### 2.3 Runtime Execution Latencies & Throughput

Benchmarked on Apple Silicon (M3, pure Go 1.22, zero CGO, durable SQLite Write-Ahead Log + SHA-256 blobstore with atomic `fsync` persistence):

| Operation | Benchmark Name | Operations Tested | Latency per Op | Memory Allocated | Allocs / Op | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Micro-Universe Creation** | `BenchmarkMicroUniverseCreation` | 50 | **5.67 ms** | 12.9 KB | 88 allocs | Creates zero-copy branched micro-universe, updates head pointer, and records durable WAL Oplog event. |
| **Sparse Subtree Extraction** | `BenchmarkSparseSubtreeExtraction` | 50 | **4.58 ms** | 199.8 KB | 1,895 allocs | Traverses 50-component Merkle DAG, extracts component and constituent symbols, generates sparse payload. |
| **AST Symbol Surgery** | `BenchmarkASTSymbolSurgery` | 50 | **40.91 ms** | 152.1 KB | 853 allocs | Loads target symbol, mutates payload, recalculates Merkle root cascade, indexes semantic edges, persists blobs. |

#### Comparison vs. Legacy Git Operations

| Metric | Legacy Git CLI (Disk Worktrees / Clones) | Cosm Micro-Universes & AST Store | Advantage |
| :--- | :--- | :--- | :--- |
| **Branch Creation Time** | 50 ms – 500 ms (creates physical directory trees and locks) | **5.67 ms** (in-database pointer in WAL) | **~10x – 90x faster** |
| **Disk Overhead per Branch** | Full workspace footprint (10 MB – 2 GB per worktree) | **0 bytes** (zero-copy until mutated) | **100% disk deduplication** |
| **Subtree Sync Latency** | 200 ms – 2,000 ms (`git clone --depth 1` + checkout) | **4.58 ms** (in-memory AST filter) | **~40x – 400x faster** |

---

### 2.4 Multi-Agent Swarm Simulation Throughput

Tested via `topocosm/pkg/benchmarks/swarm_test.go` (`TestSwarmSimulation`) with concurrent agent workers executing sparse subtree pulls, blackboard domain leases, proposal COB authoring, and CRDT merges:

| Parameter | Value |
| :--- | :--- |
| **Active Virtual Agents** | 5 – 25 concurrent workers |
| **Hub Concurrency Limit** | 3 – 10 concurrent threads |
| **Sustained Operations Throughput** | **1,436.1 operations / sec** |
| **Sparse Pull Requests Processed** | 213 pulls / 500ms run |
| **Measured Bandwidth Savings** | **66.66%** (1 of 3 components selected) |
| **Proposal Creation Latency** | < 1.2 ms |
| **CRDT Convergence Failure Rate** | **0.00%** (0 unresolvable conflict errors) |

---

## 3. How to Reproduce

Execute the test and benchmark suites locally:

```bash
# 1. Run empirical bandwidth, token savings, and subagent transcript tests in Cosm
cd cosm
go test -v ./pkg/benchmarks -run TestEmpirical

# 2. Run real-file Git vs. Cosm AST token comparison benchmarks
go test -v ./pkg/benchmarks -run "TestTokenComparison_.*"

# 3. Run empirical Antigravity subagent log harvester
python3 test/agents/log_token_harvester.py test/agents/transcripts/subagent_a_file_full.jsonl test/agents/transcripts/subagent_b_ast_full.jsonl

# 4. Run execution latency microbenchmarks
go test -v -bench=. -run=^# -benchtime=50x ./pkg/benchmarks

# 5. Run multi-agent concurrent swarm simulation in Topocosm
cd ../topocosm
go test -v ./pkg/benchmarks -run TestSwarmSimulation
```

