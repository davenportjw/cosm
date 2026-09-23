# Getting Started with Cosm

This guide walks you through installing Cosm, initializing your first repository, staging polyglot code into AST symbol nodes, and running your first target preview in under 5 minutes.

---

## 1. Installation

Cosm requires **Go 1.22+** and has zero CGO dependencies.

### Option A: Direct Go Install (Recommended)
Compile and install the `cosm` binary directly into your Go bin directory (`$GOPATH/bin` or `~/go/bin`):
```bash
go install github.com/cosmscm/cosm/cmd/cosm@latest
```
*Make sure `$(go env GOPATH)/bin` or `~/go/bin` is in your `$PATH`.*

### Option B: Build from Source
```bash
git clone https://github.com/cosmscm/cosm.git
cd cosm
go build -o cosm ./cmd/cosm
sudo mv cosm /usr/local/bin/

# Optional: Build autonomous agent test harness binary
go build -o cosm-agent-harness ./cmd/cosm-agent-harness
sudo mv cosm-agent-harness /usr/local/bin/
```

### Option C: Verify Installation
```bash
cosm --help
cosm version
```

---

## 2. Quickstart: 5-Minute Tour

### Step 1: Initialize a Cosm Repository
Run `cosm init` in your project root:
```bash
mkdir my-cloud-app && cd my-cloud-app
cosm init
```
**Output:**
```
✨ Initialized empty Cosm repository in .cosm/
   Active Universe: universe-main
```

### Step 2: Create Polyglot Application Code
Create a Go API service and a Terraform infrastructure configuration:

**`server.go`**:
```go
package main

import "net/http"

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy"}`))
}
```

**`infra/main.tf`**:
```hcl
resource "google_cloud_run_service" "api" {
  name     = "my-api"
  location = "us-central1"
}
```

### Step 3: Stage Files (AST Symbols & Raw Artifacts)
`cosm add` parses polyglot code into structured AST symbol nodes and captures non-code files (like `README.md`, `LICENSE`, configs) as zero-loss `RawBlobNode`s:
```bash
# Create project documentation and license
echo "# My Cloud App" > README.md
echo "Apache-2.0 License" > LICENSE

# Stage code, infrastructure, license, and docs
cosm add server.go infra/main.tf LICENSE README.md \
  -p "Implement health endpoint, Cloud Run service, license, and docs" \
  -i "Initial project bootstrap with license and docs"
```
**Output:**
```
✓ Staged AST Component: server.go (go, 1 symbols, hash: 3a9e102f)
✓ Staged AST Component: infra/main.tf (hcl, 1 symbols, hash: d9e71b40)
✓ Staged AST Component: LICENSE (raw, 1 symbols, hash: comp-raw-c69)
✓ Staged AST Component: README.md (raw, 1 symbols, hash: comp-raw-8f2)
```

### Step 4: Commit with Cryptographic Lineage
```bash
cosm commit -i "Initial backend, infrastructure, license, and documentation setup"
```
**Output:**
```
🌟 Committed to universe 'universe-main' (Merkle Root: a83b92f7e02c)
   Components: 4 | Cross-Domain Edges: 1
```

### Step 5: Verify the Multi-Domain Topology
Inspect the cross-boundary dependencies across your frontend, backend, and infrastructure tiers:
```bash
cosm topology
```
**Output:**
```
================================================================================
                           COSM: TOPOLOGY MAP                                   
================================================================================

┌── [1] FRONTEND TIER (TypeScript / React)
│   (No frontend components detected)
│
├── [2] BACKEND / API TIER (Go Microservices)
│   └── server.HandleHealth (FunctionDecl)
│
└── [3] CLOUD INFRASTRUCTURE TIER (Terraform HCL)
    └── google_cloud_run_service.api (ResourceBlock)

Summary: 2 Nodes, 1 Cross-Domain Edges
```

### Step 6: Ship & Launch Ephemeral Live Preview
Execute the shipping sidecar to validate Terraform, build binaries, and launch an ephemeral local preview sandbox:
```bash
cosm ship
```
**Output:**
```
🚀 Shipping Sidecar Execution Succeeded!
   • Universe:     universe-main
   • Package Size: 1824 bytes
   • Preview URL:  http://127.0.0.1:54912
   • Health Check: PASS
```

---

## 3. Onboarding an Existing Git Repository

To onboard an existing polyglot codebase into Cosm with automated AST extraction and contract inference:

```bash
cosm import-repo -d ./existing-repo -u universe-main
```
**Output:**
```
🚀 Onboarding repository at './existing-repo' into universe 'universe-main'...

✅ Repository Successfully Onboarded!
   • Total Files Scanned:   48 (Code: 42, Raw: 6)
   • AST Symbols Extracted: 184
   • Components Created:    48
   • Cross-Edges Linked:    31
   • Merkle Root Hash:      4f2b90d81a9e
```

---

## 4. Compiling the Customer Zero Camping App into Cosm (from Git)

Cosm bundles a complete production-grade polyglot application in [`examples/camping_app`](../examples/camping_app/): **Alpine Escapes Camping & Cabin Reservation App** (Go HTTP backend, HTMX interactive UI, PostgreSQL DDL migrations, and Terraform HCL cloud infrastructure).

Because Git only stores flat text files and `.cosm/` is intentionally gitignored, compile the Camping App into Cosm's AST Merkle-DAG to start:

```bash
# 1. Enter the example directory
cd examples/camping_app

# 2. Compile into Cosm's AST Merkle-DAG (or run ./compile_into_cosm.sh)
cosm init --universe universe-main
cosm add .
cosm commit -u universe-main -i "Compile camping app into cosm"

# 3. Verify universe status and clean working tree lens
cosm status

# 4. Visualize cross-domain topology (Backend -> SQL -> Cloud Infra)
cosm topology

# 5. Compile target preview and launch server
cosm ship
go run ./cmd/server
```

Open `http://localhost:8080` to experience the live application! See [`examples/camping_app/README.md`](../examples/camping_app/README.md) for full documentation and multi-agent Topocosm Hub journeys.

---

## 5. Interactive Terminal Dashboard

Launch the full-screen Bubble Tea TUI dashboard to inspect micro-universes, review AST graphs, and browse prompt lineage interactively:

```bash
cosm dashboard
```

---

## 6. AI Agent Setup & Antigravity Skills

Cosm is built from the ground up for AI agent pair programming. While legacy SCM systems force agents to manipulate lines of raw text (often resulting in hallucinated indentation, malformed brackets, or git merge conflicts), Cosm allows agents to perform **direct semantic AST mutations** and project changes into isolated micro-universes.

### Bundled Antigravity Skills

Antigravity agent workflows in Cosm are guided by modular skills located under [`.agents/skills/`](../.agents/skills/):

* **[`cosm-core-dev`](../.agents/skills/cosm-core-dev/SKILL.md)**: Repository initialization, SQLite WAL graph inspection (`.cosm/graph.db`), zero-copy micro-universe branching, and standard CLI subcommands.
* **[`cosm-ast-surgeon`](../.agents/skills/cosm-ast-surgeon/SKILL.md)**: Precise geometric AST modifications using 9 declarative verbs (`insert_symbol`, `replace_body`, `delete_symbol`, `wrap_symbol`, etc.) without full-file rewrites.
* **[`shipping-and-validation`](../.agents/skills/shipping-and-validation/SKILL.md)**: Target staging compilation (`cosm ship`), Terraform formatting and validation, Python `uv` test execution, and Apple container previews.
* **[`polyglot-codecs-guide`](../.agents/skills/polyglot-codecs-guide/SKILL.md)**: AST codecs and cross-boundary contract inferencers across 19 supported languages.
* **[`cosm-agent-harness`](../.agents/skills/cosm-agent-harness/SKILL.md)**: Autonomous test harness, verification oracles (0–100 scorecards), and benchmark evaluations.

### How Antigravity Loads Skills

Antigravity uses a **hierarchical discovery** and **progressive disclosure** mechanism:

1. **Discovery**: Antigravity automatically scans for `.agents/skills/*/SKILL.md` from the current working directory upwards to the workspace root.
2. **Indexing (Zero Token Waste)**: At conversation startup, only the skill name and short description from each `SKILL.md` frontmatter are registered into the prompt catalog.
3. **Progressive Disclosure**: When a prompt or task matches a skill's purpose, the agent dynamically invokes `view_file` to load the full runbook instructions into active context.
4. **Hard Invariants**: Project rules in [`AGENTS.md`](../AGENTS.md) and [`GEMINI.md`](../GEMINI.md) are loaded unconditionally to enforce architectural invariants (pure Go WAL engine, zero-CGO, local-first autonomy).

### Enabling Cosm Skills in Your Own Application Project

If you are using Cosm as the source control system for your own project:

```bash
# Option A: Workspace scope (shared with your team in Git/Cosm)
mkdir -p .agents/skills
cp -r /path/to/cosm/.agents/skills/cosm-* .agents/skills/

# Option B: Global user scope (available across all workspaces on your machine)
mkdir -p ~/.gemini/config/skills
cp -r /path/to/cosm/.agents/skills/cosm-ast-surgeon ~/.gemini/config/skills/
cp -r /path/to/cosm/.agents/skills/cosm-core-dev ~/.gemini/config/skills/
```

