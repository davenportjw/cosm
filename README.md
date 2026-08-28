# Cosm (`cosm`) & Topocosm (`topocosm.dev`)

> **AI-Native Polyglot AST Source Control Management (SCM) & Target Compilation System**

Cosm is an AST-level Source Control Management system and target compiler written in pure Go (`go 1.22+`). Instead of tracking lines of text with unstructured string diffs, Cosm parses source code into an immutable, content-addressed AST Merkle-DAG, models parallel branching as zero-copy micro-universes, enforces cross-boundary contracts, and compiles directly into verified target environments.

---

## 🌟 Key Architecture & Capabilities

1. **AST Merkle-DAG Storage**: Code is stored as content-addressed AST symbol nodes in `.cosm/objects/` with semantic dependency edges, universe heads, and event Oplogs managed via pure-Go SQLite WAL mode (`.cosm/graph.db`).
2. **Zero-Copy Micro-Universes**: Parallel branching creates instantaneous micro-universes with non-linear frontier heads, allowing concurrent human and AI agent exploration without workspace pollution.
3. **Multi-Tier Causal Pedigree**: Cryptographic Ed25519 attestations track causality end-to-end:
   $$\text{User Prompt} \longrightarrow \text{Session ID} \longrightarrow \text{Agent ID} \longrightarrow \text{Model Parameters} \longrightarrow \text{AST Symbol Node}$$
4. **Cross-Domain Topology & Blast Radius**: Real-time dependency mapping linking Frontend components, Backend handlers, Data schemas, and Cloud Infrastructure (HCL/Terraform).
5. **Target Compilation Sidecar (`cosm ship`)**: Projects AST states into target staging sandboxes with automated build validation.
6. **Local Git Shim & CRDT Distributed Collaboration**: Operates 100% offline with zero cloud dependencies; provides a local Git plumbing shim (`cosm git`) alongside Radicle-inspired CRDT Collaborative Objects (COBs).
7. **Universe Proposals (Human PR Model)**: Proposal cards display semantic symbol deltas, topology changes, and verification badges while preserving human review workflows.

---

## 🔄 Self-Hosting Roadmap: Cosm on Cosm

Cosm is designed to achieve self-hosting dogfooding: **maintaining Cosm using Cosm itself**.

```mermaid
graph LR
    A["Cosm Go Source AST"] --> B["cosm add / stage"]
    B --> C["Micro-Universe Branching"]
    C --> D["AST Surgery & Lineage Attestations"]
    D --> E["Universe Proposals & Blast Radius Checks"]
    E --> F["cosm ship Target Compilation"]
    F --> G["Self-Hosted cosm Binary"]
```

- **Phase 1: Git Shim Bootstrapping**: Git-compatible plumbing allows standard IDEs and git tools to interact seamlessly with the Cosm repository.
- **Phase 2: Polyglot Codec Dogfooding**: Cosm parses its own Go AST, configuration files, and schema models into the Merkle-DAG store.
- **Phase 3: Autonomous Agent Swarms**: Parallel agents operate in isolated micro-universes, generating AST mutations with verified cryptographic lineage.
- **Phase 4: Full Native Self-Hosting**: All code versioning, reviews, merges, and target compilations for Cosm are managed natively via `cosm`.

---

## 🚀 Quickstart

### 1. Build the Binary
```bash
go build -o cosm ./cmd/cosm
```

### 2. Initialize a Repository
```bash
# Initialize a new Cosm repository with a default universe
cosm init -u universe-main
```

### 3. Stage Polyglot Code into AST Symbol Nodes
```bash
# Stage Go, TypeScript, Python, HCL, or SQL source files
cosm add -p "Initial setup" -i "Staged Go backend and HCL infra" server.go infra/main.tf
```

### 4. Commit with Causal Lineage
```bash
cosm commit -i "Initial backend & infrastructure setup"
```

### 5. Inspect Workspace Status & Cross-Domain Topology
```bash
# Check staged symbol nodes and frontier heads
cosm status

# Visualize cross-boundary dependency graph (Frontend -> Backend -> Infra)
cosm topology
```

### 6. Zero-Copy Micro-Universes & Proposals
```bash
# Create an isolated micro-universe branched from universe-main
cosm universe create universe-feature-auth -p universe-main

# View AST symbol differences across universes
cosm universe diff universe-main universe-feature-auth

# Create a Universe Proposal (PR equivalent)
cosm proposal create --title "Add OAuth2 Flow" --source universe-feature-auth --target universe-main

# View proposal cards with semantic deltas
cosm proposal view prop-1
```

### 7. Compile & Ship to Target Preview
```bash
cosm ship --target local-preview
```

### 8. Interactive Terminal Dashboard
```bash
cosm dashboard
```

---

## 📁 Repository Structure

```
future-of-git/
├── cmd/
│   ├── cosm/                   # Developer CLI binary (`cosm`)
│   └── cosm-agent-harness/     # Standalone autonomous test & rater agent harness
├── pkg/
│   ├── api/                    # gRPC & in-process client/server SDK
│   ├── codecs/                 # Polyglot AST parsers (Go, TS, Python, HCL, Rust, Java, SQL, Proto, C++)
│   ├── collaboration/          # Conflict engine & multi-universe fitness evaluator
│   ├── core/                   # Core schemas, Merkle hasher, and graph linker
│   ├── distributed/            # Radicle-inspired CRDT COBs & P2P sync
│   ├── gitshim/                # Git command interceptor & synthetic tree generator
│   ├── lineage/                # Ancestry tracer, audit engine, & Ed25519 attestations
│   ├── materialize/            # AST-to-source hydrators, terminal viewer, & VFS
│   ├── mutation/               # AST symbol surgery & cross-boundary blast radius
│   ├── onboarding/             # Snapshot & historical Git repo ingestion pipelines
│   ├── review/                 # AST proposal cards, human annotations, & AI critic protocols
│   ├── shipping/               # Target specification, staging engine, & compilers
│   ├── storage/                # Blobstore, SQLite WAL graph engine, & vector index
│   ├── target/                 # Target environment projector & local preview sandbox
│   ├── topocosm/               # Topocosm Hub daemon, swarm simulator, & sparse replication
│   └── tui/                    # Bubble Tea interactive terminal dashboard
├── docs/                       # Architecture specs, guides, and reference manuals
├── examples/                   # Polyglot demo workspaces (Terraform + Go + React)
└── test/
    └── agents/                 # Autonomous agent test suites, LLM harness, & rater oracles
```

---

## 🧪 Testing & Validation Guides

For testing Cosm and Topocosm, refer to the 3 step-by-step testing guides:

1. **[Guide 1: IDE Setup & Polyglot AST Core Workflow](docs/guides/testing-guide-ide-and-core.md)**
   * Setting up IDEs (VS Code, Cursor, JetBrains, Zed, Neovim) with Cosm
   * Workspace initialization (`cosm init`)
   * Polyglot AST symbol parsing and staging (`cosm add`)
   * Multi-domain dependency topology visualization (`cosm topology`)
   * Ephemeral preview compilation and shipping (`cosm ship`)

2. **[Guide 2: Micro-Universes, AST Surgery & Universe Proposals](docs/guides/testing-guide-universes-and-proposals.md)**
   * Instant zero-copy micro-universe branching (`cosm universe create`)
   * AST-level symbol diffing (`cosm universe diff`)
   * Human-understandable Universe Proposal review cards (`cosm proposal view`)
   * CRDT-based universe merging (`cosm universe merge`)
   * Interactive Bubble Tea TUI dashboard (`cosm dashboard`)

3. **[Guide 3: Topocosm Hub, Multi-Agent Swarms & Sparse Replication](docs/guides/testing-guide-topocosm-and-swarms.md)**
   * Running the zero-Docker Topocosm Hub daemon (`cosm topocosm dev`)
   * Machine-first agent discovery manifest (`/.well-known/cosm-agent.json`)
   * Multi-agent concurrent swarm simulation benchmark (`cosm topocosm test-swarm`)
   * Publishing workspaces and sparse AST subgraph cloning (`cosm clone --sparse`)

---

## 📚 Complete Documentation Index

| Guide | Description |
| :--- | :--- |
| [What is Cosm?](docs/what-is-cosm.md) | Architectural philosophy, problem space, and comparative analysis |
| [5-Minute Quickstart](docs/getting-started.md) | Rapid onboarding and initial setup guide |
| [Micro-Universes & Parallel Swarms](docs/guides/micro-universes.md) | Non-linear branching and zero-copy micro-universes |
| [AI-Native Proposals & Code Review](docs/guides/proposals-and-reviews.md) | Universe Proposals, human annotations, and AI critic checks |
| [Target Shipping & Ephemeral Previews](docs/guides/target-shipping.md) | Isolated compilation targets and preview sidecar |
| [Surgical AST Mutations & Lineage](docs/guides/ast-mutations-and-lineage.md) | 9 Geometric AST verbs and causal pedigree tracing |
| [Git Interoperability & Remote Sync](docs/guides/git-interop.md) | Local Git plumbing shim and Radicle-style CRDT COBs |
| [CLI Reference Manual](docs/reference/cli.md) | Complete flag, subcommand, and argument specifications |
| [Agent API & SDK Reference](docs/reference/agent-api.md) | In-process and gRPC SDK for autonomous agents |
| [Schema & Durable Storage Spec](docs/reference/schema-and-storage.md) | Content-addressed blobstore and SQLite WAL schema |
| [Language Codecs & Contract Inference](docs/reference/codecs.md) | Polyglot parser implementations and symbol boundary contracts |
| [Topocosm Hub & Agent Distribution](docs/reference/topocosm.md) | Machine discovery manifests, sparse cloning, and hub federation |
| [PR & Collaboration Workflow](docs/HOW_TO_PR_AND_COLLABORATION.md) | Human reviewer and agent collaboration protocol |

---

## 🛠️ Testing & Verification

Run the test suite:
```bash
# Run all Go package unit and integration tests
go test -v ./pkg/...

# Run autonomous agent harness hermetic tests
go test -v ./test/agents/...
```

---

## 📄 License

Licensed under the Apache License, Version 2.0. See LICENSE for details.
