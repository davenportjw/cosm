# Empirical Benchmarks, Bandwidth & Runtime Savings Reference

This document provides mathematically rigorous, empirically measured performance benchmarks and resource savings for the Cosm AST Source Control System and Topocosm Distribution Hub.

All metrics documented below are directly reproducible via hermetic Go unit tests and benchmarks located in [`pkg/benchmarks/savings_test.go`](file:///Users/jasondavenport/GitHub/cosm/pkg/benchmarks/savings_test.go) and [`topocosm/pkg/benchmarks/swarm_test.go`](file:///Users/jasondavenport/GitHub/topocosm/pkg/benchmarks/swarm_test.go).

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

### 2.2 LLM Token Consumption: AST Symbols vs. Whole Files

Measurements obtained via `TestEmpiricalTokenSavings` assuming standard sub-word tokenization (~4 characters per token):

| File Scenario | File Size (Bytes / Lines) | Full File Context (Tokens) | AST Symbol Context (Tokens) | AST Context Savings | Surgical Mutation Output (Tokens) | Output Token Savings |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Medium Service Component** | 18.5 KB / 500 lines | 4,625 tokens | 230 tokens | **95.03%** | 35 tokens | **99.24%** |
| **Large Controller / Router** | 74.0 KB / 2,000 lines | 18,500 tokens | 275 tokens | **98.51%** | 38 tokens | **99.79%** |

* **Context Savings**: Ingesting isolated AST symbol signatures saves **95.0% to 98.5%** of input prompt tokens compared to reading the full file.
* **Output Savings**: Outputting a surgical mutation payload (`replace_function_body`) saves **99.2% to 99.8%** of output generation tokens compared to rewriting the full file, eliminating model truncation and mid-file hallucination.

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
# 1. Run empirical bandwidth and token savings tests in Cosm
cd cosm
go test -v ./pkg/benchmarks -run TestEmpirical

# 2. Run execution latency microbenchmarks
go test -v -bench=. -run=^# -benchtime=50x ./pkg/benchmarks

# 3. Run multi-agent concurrent swarm simulation in Topocosm
cd ../topocosm
go test -v ./pkg/benchmarks -run TestSwarmSimulation
```
