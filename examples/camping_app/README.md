# Alpine Escapes: Camping & Cabin Reservation App
> **Cosm Customer Zero Demonstration Workspace**

Welcome to the **Alpine Escapes** reservation service. This project serves as **Customer Zero** for [Cosm](https://github.com/cosmscm/cosm)—a production-grade, full-stack Go web application with SQL migrations, Terraform cloud infrastructure, and container definitions managed **exclusively** by Cosm's AI-native polyglot AST source control system.

There is **no `.git/` folder** in this project. Cosm is the sole, autonomous source control and compilation engine.

---

## 🚀 Quickstart: Compiling the Camping App into Cosm (from Git)

When you download or clone Cosm from GitHub/Git, Git tracks the polyglot source code files on disk (`cmd/`, `migrations/`, `deploy/`, `Dockerfile`), but Cosm's internal content-addressed AST Merkle-DAG and SQLite WAL engine (`.cosm/`) are intentionally gitignored.

To **compile the Camping App into Cosm** and activate the AST Merkle-DAG:

### Option A: 3 Canonical Cosm CLI Commands

Run these commands inside `examples/camping_app`:

```bash
# 1. Initialize empty Cosm repository on universe-main
cosm init --universe universe-main

# 2. Parse all polyglot files and stage into typed AST symbol nodes
cosm add .

# 3. Commit the Merkle root with causal pedigree lineage
cosm commit -u universe-main -i "Compile camping app into cosm"
```

### Option B: One-Command Bootstrap Script

Alternatively, execute the bundled compilation script:

```bash
./compile_into_cosm.sh
```

### Verify Compilation into Cosm

Once compiled into Cosm, inspect the active universe and Merkle-DAG:

```bash
# 1. Inspect universe status, Merkle root hash, and lens sync
cosm status

# 2. Visualize the cross-domain topology graph (Backend -> SQL -> Cloud Infra)
cosm topology

# 3. Browse the complete AST symbol hierarchy
cosm ast tree
```

### Ship & Run

```bash
# Ship directly from the AST Merkle-DAG to an ephemeral preview sandbox
cosm ship

# Run tests and launch the local server
go test -v ./...
go run ./cmd/server
```
Navigate to `http://localhost:8080` to interact with the live Alpine Escapes reservation system!

---

## 1. The Core Question: Why Are Files in the Folder Structure?

If Cosm stores code as a **content-addressed AST Merkle-DAG** in `.cosm/objects/` and `.cosm/graph.db`, why do files like `cmd/server/main.go`, `migrations/001_initial_schema.sql`, and `deploy/main.tf` exist on disk?

The answer lies in Cosm's foundational **Dual-Plane Architecture**:

```
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │                    PLANE 1: CANONICAL AST MERKLE-DAG                        │
 │  • Content-addressed CAS (.cosm/objects/) & WAL Graph Engine (.cosm/graph.db)│
 │  • Symbol nodes, semantic contract edges, causal lineage, micro-universes    │
 │  • Ground truth for all commits, AST surgery, shipping, and federated sync   │
 └──────────────────────────────────────┬──────────────────────────────────────┘
                                        │
                         Bidirectional Hydration & Sync
                  (cosm export / cosm add / cosm ast edit -w)
                                        │
                                        ▼
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │                PLANE 2: MATERIALIZED WORKING TREE LENS                      │
 │  • Ephemeral disk files (cmd/server/main.go, deploy/main.tf, etc.)          │
 │  • Host toolchain execution: 'go build', 'go test ./...', 'terraform validate'│
 │  • IDE editor integration: Language Server Protocols (gopls, pyright), linters│
 └─────────────────────────────────────────────────────────────────────────────┘
```

### The Dual Planes

1. **Plane 1: Canonical AST Merkle-DAG (`.cosm/`)**
   - The **single source of truth**. Software is decomposed into typed AST symbol nodes (`FunctionDecl`, `StructDecl`, `RouteBinding`, `ResourceBlock`, `TableSchema`), cross-boundary dependency edges, and causal lineage chains.
   - Micro-universes (`cosm universe`) branch with zero file copies by simply advancing root pointers.
   - Builds (`cosm ship`) compile directly from ephemeral AST projections hydrated in isolated sandboxes, entirely decoupled from whatever happens to be on your local disk.

2. **Plane 2: Materialized Working Tree Lens (Workspace Disk Files)**
   - The files you see in this folder (`cmd/`, `migrations/`, `deploy/`, `Dockerfile`) form a **materialized lens**—a high-fidelity projection of the canonical AST DAG into standard POSIX directory hierarchies.
   - **Why keep this lens materialized?**
     - **Host Toolchain Compatibility**: Go compilers (`go test ./...`, `go build`), Terraform engines (`terraform fmt`), and container builders require POSIX files.
     - **IDE & Language Server Protocol (LSP)**: VS Code, `gopls`, and linters operate on active file buffers and inotify events.
     - **Human Developer Familiarity**: Developers can browse, inspect, and reason about code in conventional layouts without cognitive dissonance.

Cosm bridges these two planes continuously. When an AI agent performs surgical AST mutations (`cosm ast edit`), Cosm automatically updates the materialized lens on disk (`--write-disk=true`) so your open editor buffers and `gopls` update in real time.

---

## 2. Inspecting the Dual Planes with Cosm

### Checking Working Tree Lens Status

Run `cosm status` to inspect both the canonical AST Merkle root and the state of the materialized disk lens:

```bash
cosm status
```

Output:
```text
On universe: universe-main
Head Merkle Root: 1c170a7199e6ca461bacbffaa78e33df68d51256fe41739b0d404930a3b1f3c8
Components tracked: 9 (AST Symbol DAG in .cosm/)
Cross-boundary edges: 0
Working Tree Lens: Materialized projection in sync with Merkle root (5 projected file(s))
```

If you edit or delete a file on disk without staging it into the AST DAG, `cosm status` detects the drift immediately:

```text
Working Tree Lens: File drift detected (1 file(s) drifted from AST DAG)
   • cmd/server/main.go (modified on disk)
   -> Run 'cosm add' to stage disk edits into AST DAG, or 'cosm export -d .' to reset disk projection to Merkle root.
```

Machine runtimes can also inspect the lens programmatically via `cosm status -f json`:

```json
{
  "status": "SUCCESS",
  "universe_id": "universe-main",
  "merkle_root": "1c170a7199e6ca461bacbffaa78e33df68d51256fe41739b0d404930a3b1f3c8",
  "components_count": 9,
  "working_tree_lens": {
    "status": "clean",
    "projected_files_count": 5,
    "drifted_files": []
  }
}
```

---

## 3. Hands-On Verification: Proof of AST Autonomy

To prove that the files on disk are merely a materialized lens and not the source of truth, try these exercises:

### Exercise 1: Reconstitute the Workspace from Pure AST

1. Delete all source code and configuration files from the folder (leaving only `.cosm/`):
   ```bash
   rm -rf cmd deploy migrations Dockerfile
   ```
2. Verify that `cosm status` detects the missing files:
   ```bash
   cosm status
   ```
   *Notice that the AST Merkle root and 9 tracked components remain 100% intact!*

3. Re-materialize the entire workspace instantly from the Merkle DAG:
   ```bash
   cosm export -d .
   ```
4. Verify your files are completely restored and in sync:
   ```bash
   cosm status
   go test -v ./...
   ```

### Exercise 2: Shipping Directly from the AST DAG

When shipping targets with `cosm ship`, Cosm does not package the workspace disk files. Instead, it pulls the canonical AST nodes directly from `.cosm/objects/`, hydrates them ephemerally into an isolated build sandbox, and compiles the target artifact:

```bash
cosm ship
```

Output:
```text
🚢 Shipping Target: target:cosm (Universe: universe-main)
   • Source:  Ephemeral AST Hydration from Merkle Root 1c170a7199e6 (5 file(s))
   • Staging: Isolated sandbox (decoupled from workspace disk drift)
🚀 Shipping Sidecar Execution Succeeded!
   Package Size: 8676138 bytes (Artifact: 5533e8b74342034c)
   Artifact Path: dist/cosm.tar.gz
```

### Exercise 3: Inspecting the AST DAG Without Disk Files

You can navigate and query code directly at the AST symbol level:

```bash
# View the full AST hierarchy (services, schemas, routes, infra)
cosm ast tree

# View the blast radius of modifying a specific symbol
cosm blast-radius sym-HandleCreateBooking-main

# View causal lineage and author prompt for any symbol
cosm view sym-HandleCreateBooking-main --format raw
```

---

## 4. Tracked Components in Universe `universe-main`

The Camping App consists of 45 polyglot AST components spanning 3 architectural tiers in `universe-main`:

| Architectural Tier | Primary Languages | Materialized Disk Pathways | Semantic AST Symbol Capabilities |
| :--- | :--- | :--- | :--- |
| **Backend Core & Domain Engines** | Go | `cmd/server/`, `internal/weather/`, `internal/sos/`, `internal/astronomy/`, `internal/sync/`, `internal/telemetry/`, `internal/profile/`, `internal/ranger/`, `internal/outfitter/`, `internal/backcountry/`, `internal/environmental/`, `internal/pms/`, `internal/lease/` | HTTP handlers, booking transactions, NFDRS fire safety, SAR emergency beacons, ALPR gate cache, outfitter lockers |
| **Database Schemas & Migrations** | SQL | `migrations/001_*.sql` through `008_emergency_sos_beacon.sql` | Table definitions, foreign key constraints, spatial/status indexes |
| **Cloud Infrastructure & Packaging** | HCL (Terraform) & Dockerfile | `deploy/main.tf`, `Dockerfile` | Cloud Run v2 service, Memorystore for Redis, IAM bindings, multi-stage Alpine build |

---

## 5. Running the Application

Because the materialized working tree lens is maintained in sync with the AST DAG, all standard Go development commands work out of the box:

```bash
# Run unit and concurrency tests
go test -v ./...

# Run the local server
go run ./cmd/server
```

Once running, navigate to `http://localhost:8080` to experience:
- Campsite search and live inventory filtering
- Real-time HTMX booking and reservation management
- Simulated concurrency contention tests
- Health probe at `/health`

---

## 6. Camping App 2.0: Multi-Agent Journeys with Topocosm Hub

Camping App 2.0 demonstrates real-world concurrent collaboration between human engineers and autonomous AI agent swarms using **Topocosm Hub** (`topocosm.dev`), Cosm's decentralized distribution backplane and CRDT coordination mesh.

### The 3 Parallel Tracks

```
                        ┌────────────────────────────────────────────────────────┐
                        │              Topocosm Hub Backplane                    │
                        │       (http://127.0.0.1:51204/davenport-boutique)      │
                        │   • Blackboard Coordination (Redis Leases)             │
                        │   • CAS Merkle Blobstore & Proposal Registry           │
                        └─────────────────────────┬──────────────────────────────┘
                                                  │
          ┌───────────────────────────────────────┼───────────────────────────────────────┐
          │                                       │                                       │
          ▼                                       ▼                                       ▼
  [Track A: Agent Alice]                  [Track B: Developer Bob]               [Track C: Infra Charlie]
  • Lease: 'services/rentals'             • Lease: 'services/reviews'            • Lease: 'infra/cache'
  • Sparse Pull: cmd/server/main.go       • Jujutsu Stacked Proposals:           • Sparse Pull: deploy/main.tf
  • AST Surgery: Gear Rentals API           - Stack 1: c/reviews-api (Go backend)• AST Surgery: Redis Memorystore
  • Critic Oracle: Score 96.5               - Stack 2: c/reviews-ui (TypeScript) • Critic Oracle: Score 99.0
  • CRDT Merge -> universe-main           • Jujutsu Evolve: Auto-rebase child    • CRDT Merge -> universe-main
                                          • Merge in Topological Order
```

### Running the End-to-End Automated Journey

A fully self-contained bash script executes all three tracks against a local Topocosm Hub daemon, verifying AST mutations, Jujutsu stacked proposal evolution, AI Critic Oracle approvals, and CRDT convergence:

```bash
./examples/camping_app/run_topocosm_journeys.sh
```

### Comprehensive Tutorial Guide

For an in-depth walkthrough of each command, wire DTO schemas, and the 11 friction points resolved during implementation, see the complete tutorial:
📖 [Camping App 2.0: Multi-Agent Collaboration with Topocosm Hub](../../docs/guides/camping-app-2.0-topocosm-journeys.md)

---

## 7. Interactive Agent Demo Playbook & Production Cloud Run Service

### 🚀 Live Cloud Run Service
Alpine Escapes is fully deployed to Google Cloud Run in `davenport-boutique` (`us-central1`):
- **Live URL**: `https://cosm-camping-app-txgsracloq-uc.a.run.app`
- **Active Revision**: `cosm-camping-app-00005-zjl`
- **Health Check**: `https://cosm-camping-app-txgsracloq-uc.a.run.app/health`

### 🎮 Live Features & Hands-On Agent Exercises
For a complete, tool-agnostic agent walkthrough (compatible with Antigravity, Claude, and OpenAI Codex), see:
📖 [**Cosm Interactive Demo Guide & Agent Playbook**](DEMO_INSTRUCTIONS.md)

### 🛠️ Installing Cosm CLI & Activating Agent Skills

#### 1. Compile & Install the Cosm Binary
Cosm is written in pure Go (`go 1.22+`) with zero CGO dependencies:
```bash
# Global system installation (recommended):
go build -o /usr/local/bin/cosm ./cmd/cosm

# Or local repository build:
go build -o bin/cosm ./cmd/cosm && export PATH="$(pwd)/bin:$PATH"
```

#### 2. Installing Agent Skills for AI Coding Agents
Cosm includes **6 specialized agent skills** located in [`.agents/skills/`](../../.agents/skills/):
- **`cosm-core-dev`**: Merkle-DAG storage, WAL engine, micro-universes, and CLI workflows.
- **`cosm-ast-surgeon`**: Precision AST mutations using 9 Geometric AST verbs without full-file overwrites.
- **`cosm-agent-harness`**: Autonomous test agent execution, benchmarks, and 6-dimension scoring oracles.
- **`polyglot-codecs-guide`**: AST codecs, hydrators, and cross-boundary contract inferencers.
- **`shipping-and-validation`**: Ephemeral target sandbox builds (`cosm ship`), Terraform validation, and `uv`.
- **`topocosm-cloud-deploy`**: Cloud Run, GCS CAS, Memorystore for Redis deployment and live updates.

**Setup by Agent Environment**:
- **Google Antigravity (AGY)**: Eagerly auto-detected in workspace, or install globally via: `mkdir -p ~/.gemini/antigravity/skills && cp -r .agents/skills/* ~/.gemini/antigravity/skills/`.
- **Anthropic Claude Code**: Auto-discovered via `CLAUDE.md`, or symlink via: `mkdir -p .claude/skills && ln -sf $(pwd)/.agents/skills/* .claude/skills/`.
- **OpenAI Codex / Cursor / Copilot**: System rules in [`AGENTS.md`](../../AGENTS.md) and [`GEMINI.md`](../../GEMINI.md) index these skills for on-demand file inspection.

For complete copy-paste walkthroughs and command references, see [**DEMO_INSTRUCTIONS.md (Section 2)**](DEMO_INSTRUCTIONS.md#2-installing-cosm--agent-skills).


