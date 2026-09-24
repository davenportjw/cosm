# Cosm Interactive Demo Guide & Agent Playbook

> **Target Audience**: AI Agents (Antigravity, Claude 3.5/3.7, OpenAI Codex / GPT-4o / o1) and human pair programmers demoing or evaluating the **Cosm AST Source Control Management (SCM)** system.  
> **Repository Context**: `examples/camping_app/` within Cosm.

---

## 1. What is Cosm? (Mental Model)

> [!NOTE]
> **First-Class Local Deployment Option**:
> While Alpine Escapes is live on Google Cloud Run, it can be deployed 100% locally with zero cloud dependencies using `./deploy/deploy_local.sh`. This provides an automated daemon, PID tracking, and health checks on `http://localhost:8080`—ideal for offline development, local agent evaluations, CI pipelines, and environments without Google Cloud credentials.

Cosm is an **AI-Native Polyglot AST Source Control & Target Compilation System**. Unlike legacy VCS (Git) which tracks unstructured flat text lines and file diffs:

1. **AST Merkle-DAG**: Code is stored as language-aware semantic AST symbol nodes (FunctionDecl, StructDecl, InterfaceDecl, RouteBinding, HCL blocks) in `.cosm/objects/` with custom Write-Ahead Log (WAL) graphs in `.cosm/graph.db`.
2. **Zero-Copy Micro-Universes**: Parallel branching (`cosm universe create`) creates zero-copy non-linear heads. Multiple agents can work concurrently without branch lock contention.
3. **Semantic Blast Radius**: Cosm queries incoming and outgoing contract edges to calculate the ripple impact ($0.00 - 1.00$) of mutating any symbol before merging (`cosm blast-radius <symbol_or_id>`).
4. **Surgical Reconstitution**: Cosm reconstitutes pure source files directly from the Merkle DAG with full provenance pedigree (`User Prompt` $\rightarrow$ `Agent ID` $\rightarrow$ `Model Params` $\rightarrow$ `AST Node`).
5. **Jujutsu-Style Stacked Proposals**: Dependent micro-changes chain cleanly (`cosm stack create`, `cosm stack evolve`) with automatic parent rebase.
6. **Ephemeral Target Shipping**: `cosm ship` builds isolated sandbox targets directly from Merkle roots, decoupled from dirty working trees.

---

## 2. Installing Cosm & Agent Skills

### A. Installing the Cosm CLI (`cosm`)

Cosm requires **Go 1.22+** and has zero CGO dependencies. You can install it system-wide or build it inside the repository:

#### Option 1: Global System Installation (Recommended)
Compile the `cosm` binary directly into your system `$PATH` (e.g. `/usr/local/bin` or `$GOPATH/bin`):
```bash
# Build from the repository root:
go build -o /usr/local/bin/cosm ./cmd/cosm

# Verify global installation:
cosm --version
# Expected: cosm version 0.1.0-dev (AST Source Control Management & Compiler)
```

#### Option 2: Local Workspace Build
Compile `cosm` into the project `bin/` directory and prepend it to your active shell `$PATH`:
```bash
# From repository root:
go build -o bin/cosm ./cmd/cosm
export PATH="$(pwd)/bin:$PATH"

# Or from inside examples/camping_app:
go build -o ../../bin/cosm ../../cmd/cosm
export PATH="$(cd ../.. && pwd)/bin:$PATH"
```
*(When added to `$PATH`, you can invoke `cosm` directly from any directory; otherwise, reference `../../bin/cosm` when in `examples/camping_app`).*

---

### B. Quick Primer: How to Use Cosm

Cosm operates on AST components rather than unstructured string diffs:

| Workflow Phase | CLI Command | Architectural Operation |
| :--- | :--- | :--- |
| **Inception** | `cosm init` | Creates `.cosm/` AST store, WAL database, and default `universe-main`. |
| **Staging** | `cosm add <path>` | Recursively parses source files into typed AST symbol Merkle nodes. |
| **Commit** | `cosm commit -u <universe> -i "<intent>"` | Commits staged AST symbols into a micro-universe with causal lineage. |
| **Reconstitution**| `cosm view <symbol>` | Reconstitutes source code and causal pedigree directly from Merkle DAG. |
| **Risk Analysis** | `cosm blast-radius <symbol>` | Traverses semantic contract edges and calculates risk impact score (0.00 - 1.00). |
| **Micro-Universe**| `cosm universe create <id> -p <parent>` | Creates an isolated non-linear micro-universe head instantly (zero-copy). |
| **Convergence** | `cosm universe merge <src> -t <target>` | Merges micro-universe without git rebase conflict stalls. |
| **Proposals** | `cosm proposal create -s <src> -t <target>` | Generates a Universe Proposal card with symbol diffs and build badges. |
| **Stacked Changes**| `cosm stack create` / `cosm stack evolve` | Manages Jujutsu-style stacked proposal chains with auto-rebase. |
| **Target Shipping**| `cosm ship -t target:cloud-run` | Hydrates ephemeral sandbox build directly from Merkle root. |

---

### C. Installing & Activating Cosm Agent Skills

Cosm includes **6 specialized agent skills** located in `.agents/skills/` to provide AI coding agents (Antigravity, Claude, Codex, Cursor) with expert capabilities for AST operations, surgery, and compilation:

| Skill | Path | Description & When to Use |
| :--- | :--- | :--- |
| **`cosm-core-dev`** | [`.agents/skills/cosm-core-dev`](../../.agents/skills/cosm-core-dev/SKILL.md) | Core Cosm AST DAG development, storage engines (`.cosm/objects/`, `.cosm/graph.db`), and CLI workflows. |
| **`cosm-ast-surgeon`** | [`.agents/skills/cosm-ast-surgeon`](../../.agents/skills/cosm-ast-surgeon/SKILL.md) | Surgical AST code mutations using 9 Geometric AST verbs without full-file overwrites or hallucination drift. |
| **`cosm-agent-harness`** | [`.agents/skills/cosm-agent-harness`](../../.agents/skills/cosm-agent-harness/SKILL.md) | Autonomous test agent and rater oracle execution (`cmd/cosm-agent-harness`), scenario benchmarks, and scorecards. |
| **`polyglot-codecs-guide`** | [`.agents/skills/polyglot-codecs-guide`](../../.agents/skills/polyglot-codecs-guide/SKILL.md) | Extending AST parsers, hydrators, and cross-boundary contract inferencers for all 19 supported languages. |
| **`shipping-and-validation`** | [`.agents/skills/shipping-and-validation`](../../.agents/skills/shipping-and-validation/SKILL.md) | Target compilation sidecar (`cosm ship`), Terraform validation, `uv` Python runner, and isolated preview builds. |
| **`topocosm-cloud-deploy`** | [`.agents/skills/topocosm-cloud-deploy`](../../.agents/skills/topocosm-cloud-deploy/SKILL.md) | Cloud distribution hub deployment (Google Cloud Run, GCS CAS, Memorystore for Redis, Cloud Pub/Sub). |

#### How to Install / Activate Agent Skills by Environment:

1. **Google Antigravity (AGY)**:
   - **Automatic Loading**: When opening the `cosm` workspace, Antigravity automatically detects and loads all skills from `.agents/skills/`.
   - **Global Install** (to use across all projects on your machine):
     ```bash
     mkdir -p ~/.gemini/antigravity/skills
     cp -r .agents/skills/* ~/.gemini/antigravity/skills/
     ```

2. **Anthropic Claude Code**:
   - **Automatic**: Claude Code reads repository instructions in `CLAUDE.md` and discovers markdown documents in `.agents/skills/`.
   - **Symlink Setup** (to register directly in `.claude/skills`):
     ```bash
     mkdir -p .claude/skills
     ln -sf $(pwd)/.agents/skills/* .claude/skills/
     ```

3. **OpenAI Codex, GitHub Copilot, Cursor & Windsurf**:
   - **Rules File Indexing**: Both [`AGENTS.md`](../../AGENTS.md) and [`GEMINI.md`](../../GEMINI.md) in the repository root explicitly index these skills. Agents automatically read them when prompted.
   - **On-Demand Loading**: Agents can inspect any skill file directly using standard file inspection:
     ```bash
     # View specific skill instructions on demand:
     cat .agents/skills/cosm-ast-surgeon/SKILL.md
     cat .agents/skills/cosm-core-dev/SKILL.md
     ```

---

## 3. Pre-Flight Setup & Environment Invariants

Ensure you are working in `examples/camping_app/`:

```bash
cd examples/camping_app

# 1. If .cosm/ does not exist yet (e.g. fresh git clone), bootstrap the AST DAG:
# ./compile_into_cosm.sh

# 2. Verify the Cosm binary (build if not already present)
# If ../../bin/cosm doesn't exist yet, build it: go build -o ../../bin/cosm ../../cmd/cosm
../../bin/cosm --version
# Expected: cosm version 0.1.0-dev (AST Source Control Management & Compiler)

# 3. Inspect workspace status and the 3-tier Merkle DAG
../../bin/cosm status
../../bin/cosm topology
```

> **Testing Rule**: When running Go tests inside `examples/camping_app`, always pass `GOWORK=off` to isolate package boundaries:
> ```bash
> GOWORK=off go test ./...
> ```
> *(Note: If invoking `go test` from the root repository directory instead, pass `-C examples/camping_app`)*.
>
> **Offline Operation**: All Cosm AST operations (`cosm view`, `cosm blast-radius`, `cosm ship`, `cosm stack`, etc.) and Go unit tests run 100% offline with zero external network or cloud dependencies. The Cloud Run curl commands in Section 4 are optional live production probes.

---

## 4. Tour of Pre-Built & Live Deployed Features

Two features have already been developed, committed into Cosm AST micro-universes, merged, and deployed to live Google Cloud Run:

### Feature 1: Campsite Microclimate & NFDRS Campfire Safety Engine
- **Source Code**: [`internal/weather/weather.go`](file:///Users/jasondavenport/GitHub/cosm/examples/camping_app/internal/weather/weather.go)
- **Unit Tests**: [`internal/weather/weather_test.go`](file:///Users/jasondavenport/GitHub/cosm/examples/camping_app/internal/weather/weather_test.go)
- **HTTP Handler**: [`cmd/server/handlers_weather.go`](file:///Users/jasondavenport/GitHub/cosm/examples/camping_app/cmd/server/handlers_weather.go)

**What to View in Cosm:**
```bash
# View the reconstituted AST symbol for the fire safety function:
../../bin/cosm view weather.EvaluateFireSafety

# Query the blast radius and contract impact of this symbol:
../../bin/cosm blast-radius weather.EvaluateFireSafety
```

**Probe the Endpoint (Local vs. Cloud Run):**

```bash
# Option A: Local Deployment Probe (Zero-Cloud / Offline)
curl -s "http://localhost:8080/health"
curl -s "http://localhost:8080/api/v1/weather/campsite?campsite_id=c1&format=json"

# Option B: Google Cloud Run Live Probe (Serverless Production)
TOKEN=$(gcloud auth print-identity-token)
curl -s -H "Authorization: Bearer ${TOKEN}" \
  "https://cosm-camping-app-txgsracloq-uc.a.run.app/api/v1/weather/campsite?campsite_id=c1&format=json"
```

---

### Feature 2: Emergency Wilderness SOS & SAR Dispatch Beacon
- **Source Code**: [`internal/sos/sos.go`](file:///Users/jasondavenport/GitHub/cosm/examples/camping_app/internal/sos/sos.go)
- **Unit Tests**: [`internal/sos/sos_test.go`](file:///Users/jasondavenport/GitHub/cosm/examples/camping_app/internal/sos/sos_test.go)
- **SQL Migration**: [`migrations/008_emergency_sos_beacon.sql`](file:///Users/jasondavenport/GitHub/cosm/examples/camping_app/migrations/008_emergency_sos_beacon.sql)
- **HTTP Handlers**: [`cmd/server/handlers_sos.go`](file:///Users/jasondavenport/GitHub/cosm/examples/camping_app/cmd/server/handlers_sos.go)

**What to View in Cosm:**
```bash
# View the SOS manager struct definition from the Merkle DAG:
../../bin/cosm view sos.SOSManager

# Query active micro-universes and proposals:
../../bin/cosm proposal list
```

**Probe Live Distress Dispatching (Local vs. Cloud Run):**

```bash
# Option A: Local Deployment Probes (Zero-Cloud / Offline)
# Trigger an emergency beacon:
curl -s -X POST \
  -d "location=Enchantments Pass Mile 11&latitude=47.4892&longitude=-120.7812&distress_type=INJURY&description=Fall on scree&format=json" \
  "http://localhost:8080/api/v1/sos/beacon"

# Inspect active Search & Rescue dispatch queue:
curl -s "http://localhost:8080/api/v1/sos/active"

# Option B: Google Cloud Run Live Probes (Serverless Production)
TOKEN=$(gcloud auth print-identity-token)
# Trigger an emergency beacon:
curl -s -X POST -H "Authorization: Bearer ${TOKEN}" \
  -d "location=Enchantments Pass Mile 11&latitude=47.4892&longitude=-120.7812&distress_type=INJURY&description=Fall on scree&format=json" \
  "https://cosm-camping-app-txgsracloq-uc.a.run.app/api/v1/sos/beacon"

# Inspect active Search & Rescue dispatch queue:
curl -s -H "Authorization: Bearer ${TOKEN}" \
  "https://cosm-camping-app-txgsracloq-uc.a.run.app/api/v1/sos/active"
```

---

## 5. Hands-On Demo Exercise 1: Build & Launch Feature 3 (Stargazing Observation Forecast)

This exercise is designed for an agent (or presenter) to walk through live to demonstrate Cosm's end-to-end AST workflow.

### Scenario Objective
Add a **Stargazing & Astronomical Observation Forecast Engine** to Alpine OS. Campers want to know celestial viewing clarity based on:
- Elevation (ft)
- Cloud cover (%)
- Bortle Dark-Sky Scale (Class 1 to 9, where Class 1 is pristine black sky)
- Astronomical Seeing Index (0 to 100)

### Step 1: Create a Zero-Copy Micro-Universe Branch
Isolate your changes in a new universe without blocking other agents:
```bash
../../bin/cosm universe create u/feature-astronomy -p universe-main
```
*Expected Output*: `✨ Created micro-universe 'u/feature-astronomy' branched from 'universe-main' (Head: <hash>)`

---

### Step 2: Implement the Feature Code

Create `internal/astronomy/astronomy.go`:
```go
package astronomy

import (
	"sync"
	"time"
)

type BortleClass int

const (
	BortleClass1 BortleClass = 1 // Excellent dark-sky site
	BortleClass2 BortleClass = 2 // Typical truly dark site
	BortleClass3 BortleClass = 3 // Rural sky
	BortleClass4 BortleClass = 4 // Rural/suburban transition
	BortleClass5 BortleClass = 5 // Suburban sky
)

type StargazingForecast struct {
	CampsiteID     string      `json:"campsite_id"`
	CampsiteName   string      `json:"campsite_name"`
	Bortle         BortleClass `json:"bortle_class"`
	CloudCoverPct  float64     `json:"cloud_cover_pct"`
	SeeingScore    int         `json:"seeing_score"` // 0-100
	ViewingRating  string      `json:"viewing_rating"` // OPTIMAL, GOOD, FAIR, POOR
	VisibleObjects []string    `json:"visible_objects"`
	ForecastAt     time.Time   `json:"forecast_at"`
}

type AstronomyEngine struct {
	mu        sync.RWMutex
	forecasts map[string]*StargazingForecast
}

func NewAstronomyEngine() *AstronomyEngine {
	return &AstronomyEngine{
		forecasts: make(map[string]*StargazingForecast),
	}
}

func CalculateObservationRating(cloudCover float64, seeingScore int, bortle BortleClass) string {
	if cloudCover > 60.0 || seeingScore < 30 {
		return "POOR"
	}
	if cloudCover > 30.0 || bortle > BortleClass4 {
		return "FAIR"
	}
	if seeingScore >= 75 && bortle <= BortleClass3 {
		return "OPTIMAL"
	}
	return "GOOD"
}
```

Create `internal/astronomy/astronomy_test.go`:
```go
package astronomy

import "testing"

func TestObservationRating(t *testing.T) {
	if rating := CalculateObservationRating(10.0, 90, BortleClass1); rating != "OPTIMAL" {
		t.Errorf("expected OPTIMAL, got %s", rating)
	}
	if rating := CalculateObservationRating(80.0, 90, BortleClass1); rating != "POOR" {
		t.Errorf("expected POOR, got %s", rating)
	}
}
```

Verify tests pass hermetically:
```bash
GOWORK=off go test -v ./internal/astronomy
```

---

### Step 3: Stage into Cosm AST Working Tree Lens
Cosm automatically parses the Go code into language-aware AST symbol nodes:
```bash
../../bin/cosm add internal/astronomy
```
*Expected Output*:
```
✓ Staged AST Component: astronomy.go (go, 6 symbols, hash: ...)
✓ Staged AST Component: astronomy_test.go (go, 1 symbols, hash: ...)
```

---

### Step 4: Commit into the Micro-Universe
Commit with full causal intent:
```bash
../../bin/cosm commit -u u/feature-astronomy -i "feat: Add Bortle dark-sky stargazing forecast engine"
```

---

### Step 5: Inspect Reconstituted AST Symbols
Reconstitute the symbol directly from the Merkle DAG:
```bash
../../bin/cosm view -u u/feature-astronomy astronomy.CalculateObservationRating
```

**What to Look For**:
- Node ID (content-addressed SHA-256 hash)
- Language (`go`) and NodeType (`FunctionDecl`)
- Provenance Pedigree (timestamp, agent identity, commit intent)
- Reconstituted source code exactly matching the function AST

---

### Step 6: Query Semantic Blast Radius
Check downstream impact across backend and cloud tiers:
```bash
../../bin/cosm blast-radius -u u/feature-astronomy astronomy.CalculateObservationRating
```
*Expected Output*:
```
🎯 Blast-Radius & Contract Impact for 'astronomy.CalculateObservationRating':
   • Risk Score:          0.10 / 1.00
   • Incoming Contracts:  0
   • Outgoing Contracts:  0
   • Affected Components: 0
```

---

### Step 7: Create a Universe Proposal & View Review Card
Generate a formal proposal from `u/feature-astronomy` targeting `universe-main`:
```bash
../../bin/cosm proposal create -s u/feature-astronomy -t universe-main -i "Add Stargazing & Astronomical Observation Forecast Engine"
```

Inspect the Proposal Card:
```bash
../../bin/cosm proposal view u/feature-astronomy
```

---

### Step 8: Merge Micro-Universe into `universe-main`
Merge without git rebase/merge conflicts:
```bash
../../bin/cosm universe merge u/feature-astronomy -t universe-main
```
*Expected Output*: `🔀 Successfully merged 'u/feature-astronomy' into 'universe-main' (New Head: <hash>)`

---

### Step 9: Ship Isolated Target Preview
Run Cosm's compiler sidecar to produce an ephemeral staging build:
```bash
../../bin/cosm ship -t target:cloud-run
```
*Expected Output*:
```
🚢 Shipping Target: target:cloud-run (Universe: universe-main)
   • Source:  Ephemeral AST Hydration from Merkle Root <hash>
   • Staging: Isolated sandbox (decoupled from workspace disk drift)
🚀 Shipping Sidecar Execution Succeeded!
   Package Size: ... bytes
   Artifact Path: dist/cloud-run-bundle.tar.gz
   Health: true
```

---

## 6. Hands-On Demo Exercise 2: Jujutsu-Style Stacked Proposals

Cosm supports Jujutsu-style stacked changes where dependent micro-proposals can be created, evolved, and rebased automatically.

### Scenario Objective
Demonstrate stacked changes for the Gear Outfitter:
- **Change 1 (`c/bear-bundles`)**: Introduce bear-proof food storage bundle SKU in the outfitter catalog.
- **Change 2 (`c/firewood-guard`)**: Stacked on Change 1, enforce NFDRS fire safety check during bundle checkout (prevent firewood purchase during `TOTAL_FIRE_BAN`).

### Commands to Run:
```bash
# 1. Register base stacked change:
../../bin/cosm stack create -c c/bear-bundles -u universe-main -t "Add Outfitter Bear Bundle SKU"

# 2. Register dependent stacked change:
../../bin/cosm stack create -c c/firewood-guard -u universe-main -p c/bear-bundles -t "Enforce Fire Ban in Bundle Checkout"

# 3. View the active stacked proposals:
../../bin/cosm stack list
```

**Expected Stack Output**:
```
🥞 Jujutsu-Style Stacked Proposals & Change Chains:
   [1] 🔹 c/bear-bundles       (Universe: universe-main, Head: ...)
       Parent: universe-main | Auto-Rebase: Active
   [2] 🔹 c/firewood-guard     (Universe: universe-main, Head: ...)
       Parent: c/bear-bundles | Auto-Rebase: Active
```

### Evolve the Stack:
When `c/bear-bundles` is modified or updated, run `stack evolve` to cascade changes down the chain without conflicts:
```bash
../../bin/cosm stack evolve -c c/bear-bundles
```
*Expected Output*:
```
⚡ Auto-evolved 1 descendant changes in stack:
   ✓ Rebased c/firewood-guard onto new parent AST root without conflict
```

---

## 7. Summary of Key Inspection Commands

| Command | Purpose |
| :--- | :--- |
| `cosm status` | Inspect active micro-universe, Merkle root, tracked components, and working tree lens drift |
| `cosm topology` | View cross-domain dependency graph (Frontend UI $\rightarrow$ Backend Go $\rightarrow$ Cloud Infra HCL) |
| `cosm view <symbol_id>` | Reconstitute source code and causal provenance pedigree from Merkle DAG |
| `cosm blast-radius <symbol>` | Compute topological ripple effect and semantic contract risk score |
| `cosm universe list` | Display all zero-copy parallel micro-universes |
| `cosm universe create <id> -p <parent>` | Branch zero-copy micro-universe |
| `cosm universe merge <src> [-t <target>]` | Merge micro-universe into target branch (defaults to universe-main) |
| `cosm proposal list` | List active Universe Proposals |
| `cosm proposal view <id>` | Display AST symbol deltas, topology changes, and build badges |
| `cosm stack list` | Inspect Jujutsu-style stacked proposal chains |
| `cosm stack evolve -c <parent>` | Auto-rebase dependent changes across the stack |
| `cosm ship -t target:cloud-run` | Build and stage isolated targets from Merkle roots |

---

## 8. Deployment Options & Verification (Local vs. Cloud Run)

Whenever code changes are finalized and merged to `universe-main`, verify deployment using either target:

### Option A: Local Deployment (Zero-Cloud & Automated Daemon)
Ideal for offline development, rapid local verification, CI pipelines, and autonomous agent test harness execution without GCP credentials:

```bash
# 1. Launch local server daemon with automated PID tracking and health probe:
./deploy/deploy_local.sh

# 2. Check daemon status and health:
./deploy/deploy_local.sh status

# 3. Verify local health endpoint directly:
curl -s http://localhost:8080/health

# 4. Stop the local server daemon when finished:
./deploy/deploy_local.sh stop
```
- **Local Service URL**: **`http://localhost:8080`**
- **Health Endpoint**: **`http://localhost:8080/health`**
- **Process & Logs**: PID tracked in `.server.pid`, logs streamed to `.server.log`

### Option B: Google Cloud Run Deployment (Serverless Production)
For public demonstration, multi-region staging, and production infrastructure parity:

```bash
# Run the automated build, deploy, and live HTTP health probe:
./deploy/deploy_cloudrun.sh
```
- **Deployed Service URL**: **`https://cosm-camping-app-txgsracloq-uc.a.run.app`**
- **Health Endpoint**: **`https://cosm-camping-app-txgsracloq-uc.a.run.app/health`**

> [!NOTE]
> **Evaluation & Testing Recommendation**:
> For offline evaluation, unit/integration verification, and autonomous agent benchmarking (`cosm-agent-harness`), agents should use **`./deploy/deploy_local.sh`**. This guarantees zero cloud dependency stalls, eliminates token or authentication friction, and runs with sub-second feedback loops. Use `./deploy/deploy_cloudrun.sh` when publishing live demonstration endpoints or testing production container builds.
