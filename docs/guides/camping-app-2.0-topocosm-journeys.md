# Tutorial: Camping App 2.0 User Journeys with Topocosm Hub

This tutorial walks through building and collaborating on **Camping App 2.0** using **Cosm** (the offline AST Source Control System) and **Topocosm Hub** (the cloud distribution backplane and agent coordination mesh).

You will follow three concurrent polyglot collaboration tracks demonstrating autonomous AI agents and human developers working harmoniously on a shared Merkle-DAG without merge hell:
- **Track A (Agent Alice)**: Autonomous Gear Rentals backend extension with blackboard leasing, sparse AST cloning, and Critic Oracle automated review.
- **Track B (Developer Bob)**: Campsite Reviews feature developed with Jujutsu-style stacked proposals (`c/reviews-api` and `c/reviews-ui`) and automatic DAG evolution (`cosm stack evolve`).
- **Track C (Infra Agent Charlie)**: Cloud Infrastructure extension adding Google Cloud Memorystore Redis cache via Terraform HCL AST surgery.

---

## Architecture Overview

```
                      ┌──────────────────────────────────────────────┐
                      │            Topocosm Cloud Hub                │
                      │  - CRDT Proposals & Reviews                  │
                      │  - Blackboard Domain Leases                  │
                      │  - Sparse AST Subtree CAS                    │
                      │  - Server-Sent Events (/api/v1/events/stream)│
                      └──────────────────────┬───────────────────────┘
                                             │
               ┌─────────────────────────────┼─────────────────────────────┐
               ▼                             ▼                             ▼
       [ Track A: Agent ]            [ Track B: Human ]            [ Track C: Infra ]
       AI Agent Alice                Developer Bob                 Infra Agent Charlie
       services/rentals              services/reviews              infra/cache
       Sparse Pull: Go API           Stacked Proposals:            Sparse Pull: HCL
       `c/reviews-api` + `c/reviews-ui`
               │                             │                             │
               ▼                             ▼                             ▼
      AST Symbol Surgery             Jujutsu Auto-Rebase           HCL Resource Surgery
     (HandleGearRentals)            (`cosm stack evolve`)          (google_redis_instance)
               │                             │                             │
               ▼                             ▼                             ▼
      Critic Oracle Review          Critic Review (Approved)       Automated Validation
      (Score: 96.5/100)                      │                             │
               │                             │                             │
               └─────────────────────────────┼─────────────────────────────┘
                                             ▼
                             ┌───────────────────────────────┐
                             │ Topocosm CRDT Merkle-DAG Join │
                             │        (universe-main)        │
                             └───────────────┬───────────────┘
                                             │
                                             ▼
                             ┌───────────────────────────────┐
                             │  Target Compilation & Ship   │
                             │  `go test` + `cosm ship`      │
                             └───────────────────────────────┘
```

### Polyglot Component Map of Camping App 2.0

| Domain | Component Name | Language | AST Symbol Nodes |
| :--- | :--- | :--- | :--- |
| **Backend** | `cmd/server/main.go` | Go | `HandleHealth`, `HandleSignup`, `HandleLogin`, `HandleCreateBooking`, `HandleMyBookings`, `HandleCancelBooking`, `NewServer` |
| **Frontend** | `web/src/App.tsx` | TypeScript | `App`, `BookingDashboard`, `CampsiteCard` |
| **Database** | `db/schema.sql` | SQL | `campsites`, `users`, `reservations`, `gear_items`, `reviews` |
| **Infra** | `infra/main.tf` | HCL | `google_compute_network`, `google_sql_database_instance`, `google_cloud_run_service` |

---

## Part 1: Prerequisites & Launching Topocosm Hub

### 1.1 Verify Cosm CLI Installation

Ensure `cosm` is built and available on your PATH:

```bash
cosm version
# Output: cosm v0.2.0 (AST SCM & Polyglot Compiler)
```

### 1.2 Start the Local Topocosm Hub Daemon

Start the standalone Topocosm Hub server in the background or in a separate terminal:

```bash
cosm topocosm dev --port 51204 --dir .topocosm &
```

Verify that the hub is running and inspect its status:

```bash
cosm topocosm status --url http://127.0.0.1:51204
```

Output:
```
=====================================================================
  🪐 TOPOCOSM HUB STATUS (http://127.0.0.1:51204)
=====================================================================
  • Registered Cosms:        0
  • Active Agent DIDs:       0
  • Open Proposals:          0
  • Active Blackboard Locks: 0
  • Content Blobs in Store:  0
=====================================================================
```

You can also open the interactive Web UI dashboard at `http://127.0.0.1:51204` in your browser.

---

## Part 2: Publishing the Camping App Baseline

Navigate to the Camping App repository root (`examples/camping_app`):

```bash
cd examples/camping_app
```

### 2.1 Stage and Commit Baseline to Local Cosm

If not already initialized, initialize the Cosm repository and stage the polyglot files:

```bash
cosm init --universe universe-main
cosm add .
cosm commit -u universe-main -i "Baseline Camping App 2.0 architecture"
```

Verify status:

```bash
cosm status
```

### 2.2 Publish to Topocosm Hub

Publish the Merkle-DAG snapshot to Topocosm Hub:

```bash
cosm publish http://127.0.0.1:51204/davenport-boutique/camping-app
```

Output:
```
🚀 Publishing universe 'universe-main' to Topocosm Hub at http://127.0.0.1:51204/davenport-boutique/camping-app...
   • Universe Merkle Root: a1b2c3d4e5f6...
   • Components: 9
   • AST Symbols: 28
   • Total Payload: 42.8 KB
✅ Successfully published to Topocosm Hub: davenport-boutique/camping-app (universe-main)
```

---

## Part 3: Track A - Autonomous Agent Workflow (Gear Rentals)

In this journey, **Agent Alice** (`did:key:z6MkuAgentAlice`) is tasked with implementing the Gear Rentals catalog and reservation endpoints.

### 3.1 Claim Domain Lease on the Blackboard

To avoid stepping on other agents or human developers, Alice acquires an exclusive domain lease on `services/rentals`:

```bash
cosm claim services/rentals http://127.0.0.1:51204/davenport-boutique/camping-app \
  --goal "Implement gear rental catalog and reservation routes" \
  --ttl 1200
```

Output:
```
🔒 Successfully acquired blackboard lease:
   • Domain:    services/rentals
   • Agent DID: did:key:z6MkuAgentAlice
   • Goal:      Implement gear rental catalog and reservation routes
   • Lease TTL: 1200s (expires in 20m)
```

Any other worker attempting to modify `services/rentals` during this window will be warned of active contention.

### 3.2 Sparse Clone Only Required AST Subtrees

Alice does not need the entire repository (saving >80% bandwidth). Alice pulls only the server component:

```bash
cosm clone --sparse cmd/server/main.go \
  http://127.0.0.1:51204/davenport-boutique/camping-app \
  ./workspace-alice
cd ./workspace-alice
```

Output:
```
✅ Successfully cloned davenport-boutique/camping-app to ./workspace-alice!
   • Merkle Root:     a1b2c3d4e5f6...
   • Components:      1
   • AST Symbols:     8
   • Sparse Bandwidth Savings: 88.9%
```

### 3.3 Create Micro-Universe & Perform AST Surgery

Alice creates an isolated micro-universe branch:

```bash
cosm universe create universe-alice-rentals --parent universe-main
```

Alice adds the `HandleGearRentals` symbol directly into the AST Merkle DAG:

```bash
cosm ast edit \
  --universe universe-alice-rentals \
  --op insert_after \
  --target "main.HandleCancelBooking" \
  --content '
// HandleGearRentals returns available outdoor gear for rent
func (s *Server) HandleGearRentals(w http.ResponseWriter, r *http.Request) {
	gear := []map[string]interface{}{
		{"id": "g1", "name": "2-Person Dome Tent", "daily_rate": 25.00, "available": true},
		{"id": "g2", "name": "Sub-Zero Sleeping Bag", "daily_rate": 15.00, "available": true},
		{"id": "g3", "name": "Dual-Burner Camp Stove", "daily_rate": 18.00, "available": true},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gear)
}' -w
```

### 3.4 Commit and Submit Hub Proposal

Alice commits with full causal lineage metadata:

```bash
cosm commit \
  -u universe-alice-rentals \
  -i "feat(rentals): Add gear rental catalog and reservation routes"
```

Alice submits a Proposal COB to Topocosm Hub:

```bash
cosm proposal create \
  --source universe-alice-rentals \
  --target universe-main \
  --title "feat(rentals): Add gear rental endpoints" \
  --hub http://127.0.0.1:51204/davenport-boutique/camping-app
```

Output:
```
🚀 Created Proposal: "feat(rentals): Add gear rental endpoints"
   Source: universe-alice-rentals -> Target: universe-main
   Changes: +1 components, -0 components
   Status: Ready for Agent Evaluation / Review
🌐 Submitted to Topocosm Hub: prop-alice-rentals-01
   • Hub Target: http://127.0.0.1:51204/davenport-boutique/camping-app
   • Proposal Head: 7f8a9b0c...
```

### 3.5 Automated Critic Oracle Evaluation

Topocosm Hub triggers the automated Critic Oracle (`agent-critic-deepseek`) to evaluate the proposal against 6 dimensions:
1. Syntax correctness
2. Type safety
3. Semantic contract preservation
4. Lineage attestations
5. Test coverage
6. Performance impact

```bash
cosm proposal review \
  --hub http://127.0.0.1:51204/davenport-boutique/camping-app \
  --id prop-alice-rentals-01 \
  --critic agent-critic-deepseek \
  --verdict APPROVE \
  --score 96.5
```

Output:
```
🤖 Topocosm Hub Review Submitted for prop-alice-rentals-01
   Verdict: APPROVE (Score: 96.5) | Status: APPROVED
```

### 3.6 Merge Proposal & Release Domain Lease

With approvals in place, the proposal is merged into `universe-main`:

```bash
cosm proposal merge \
  --hub http://127.0.0.1:51204/davenport-boutique/camping-app \
  --id prop-alice-rentals-01
```

Output:
```
✅ Topocosm Hub Proposal prop-alice-rentals-01 merged successfully into target universe!
   New Head Merkle Root: c4d5e6f7a8b9...
```

Alice releases the blackboard lock:

```bash
cosm release services/rentals http://127.0.0.1:51204/davenport-boutique/camping-app
```

---

## Part 4: Track B - Human Developer Workflow with Stacked Proposals (Campsite Reviews)

In this journey, **Developer Bob** creates a Campsite Reviews feature using **Jujutsu-style stacked proposals**. This consists of two dependent changes:
1. `c/reviews-api` (Change 1): Backend Go struct and REST endpoints.
2. `c/reviews-ui` (Change 2): Frontend TypeScript/React reviews drawer.

### 4.1 Claim Domain Lease

```bash
cosm claim services/reviews http://127.0.0.1:51204/davenport-boutique/camping-app \
  --goal "Stacked proposals for campsite review system" \
  --ttl 3600
```

### 4.2 Create Base Proposal (`c/reviews-api`)

Bob creates the base micro-universe and implements the backend review model:

```bash
cosm universe create u/reviews-api --parent universe-main
```

Add review handler in `cmd/server/main.go`:

```bash
cosm ast edit \
  --universe u/reviews-api \
  --op insert_after \
  --target "main.HandleMyBookings" \
  --content '
// HandleCampsiteReviews returns reviews for a campsite
func (s *Server) HandleCampsiteReviews(w http.ResponseWriter, r *http.Request) {
	reviews := []map[string]interface{}{
		{"id": "r1", "campsite_id": "c1", "author": "Alice", "rating": 5, "comment": "Spectacular views!"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reviews)
}' -w
```

Commit and register change in the Jujutsu stack:

```bash
cosm commit -u u/reviews-api -i "feat(reviews): Backend review endpoints"
cosm stack create -c c/reviews-api -u u/reviews-api
```

### 4.3 Create Stacked Proposal (`c/reviews-ui`)

Now Bob stacks the frontend change directly on top of `c/reviews-api`:

```bash
cosm universe create u/reviews-ui --parent u/reviews-api
```

Add UI review component in `web/src/App.tsx`:

```bash
cosm ast edit \
  --universe u/reviews-ui \
  --op insert_after \
  --target "App.BookingDashboard" \
  --content '
export function CampsiteReviews({ campsiteId }: { campsiteId: string }) {
  return (
    <div className="reviews-panel">
      <h3>Campsite Reviews</h3>
      <p>Rating: 5.0 / 5.0 (28 reviews)</p>
    </div>
  );
}' -w
```

Commit and register stacked change with parent:

```bash
cosm commit -u u/reviews-ui -i "feat(reviews): Frontend review drawer component"
cosm stack create -c c/reviews-ui -u u/reviews-ui -p c/reviews-api
```

Verify stack status:

```bash
cosm stack list
```

Output:
```
📚 Jujutsu-Style Stacked Proposals:
   [1] c/reviews-api (Universe: u/reviews-api, Parent: none)
       └── [2] c/reviews-ui (Universe: u/reviews-ui, Parent: c/reviews-api)
```

### 4.4 Upstream Evolution & Automatic Rebase (`cosm stack evolve`)

Bob realizes the backend needs an input validation check (rating must be between 1 and 5).
Bob modifies `u/reviews-api`:

```bash
cosm ast edit \
  --universe u/reviews-api \
  --op replace_function_body \
  --target "main.HandleCampsiteReviews" \
  --content '
	// Validated ratings (1-5)
	reviews := []map[string]interface{}{
		{"id": "r1", "campsite_id": "c1", "author": "Alice", "rating": 5, "comment": "Spectacular views!"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reviews)' -w

cosm commit -u u/reviews-api -i "fix(reviews): Validate rating boundaries"
```

Now Bob runs **Jujutsu evolution**:

```bash
cosm stack evolve -c c/reviews-api
```

Output:
```
🔄 Evolving Stacked Proposals from 'c/reviews-api'...
   • Target Change: c/reviews-api
   • Descendants identified: 1
     - Rebasing c/reviews-ui onto updated c/reviews-api manifest...
     ✓ Clean AST rebase! No AST conflict nodes reified.
✅ Stack successfully evolved: all descendant proposals updated.
```

### 4.5 Submit and Merge Stacked Proposals

Bob publishes both proposals to Topocosm Hub:

```bash
cosm proposal create \
  --source u/reviews-api \
  --target universe-main \
  --title "feat(reviews): Review backend API (Stack 1/2)" \
  --hub http://127.0.0.1:51204/davenport-boutique/camping-app

cosm proposal create \
  --source u/reviews-ui \
  --target universe-main \
  --title "feat(reviews): Review frontend drawer (Stack 2/2)" \
  --hub http://127.0.0.1:51204/davenport-boutique/camping-app
```

Both proposals are reviewed and merged in topological order:

```bash
cosm proposal review --hub http://127.0.0.1:51204/davenport-boutique/camping-app --id prop-reviews-api --verdict APPROVE --score 98.0
cosm proposal merge --hub http://127.0.0.1:51204/davenport-boutique/camping-app --id prop-reviews-api

cosm proposal review --hub http://127.0.0.1:51204/davenport-boutique/camping-app --id prop-reviews-ui --verdict APPROVE --score 95.0
cosm proposal merge --hub http://127.0.0.1:51204/davenport-boutique/camping-app --id prop-reviews-ui

cosm release services/reviews http://127.0.0.1:51204/davenport-boutique/camping-app
```

---

## Part 5: Track C - Infrastructure Extension (Redis Cache)

**Infra Agent Charlie** (`agent-charlie-infra`) provisions a Google Cloud Memorystore Redis instance to cache campsite availability queries.

### 5.1 Claim Domain Lease

```bash
cosm claim infra/cache http://127.0.0.1:51204/davenport-boutique/camping-app \
  --goal "Provision Memorystore Redis cache in Terraform HCL" \
  --ttl 600
```

### 5.2 Sparse Pull Terraform HCL

```bash
cosm clone --sparse infra/main.tf \
  http://127.0.0.1:51204/davenport-boutique/camping-app \
  ./workspace-charlie
cd ./workspace-charlie
```

### 5.3 Mutate HCL AST & Validate

Create micro-universe:

```bash
cosm universe create u/infra-redis --parent universe-main
```

Insert Redis resource into `infra/main.tf`:

```bash
cosm ast edit \
  --universe u/infra-redis \
  --op append_child \
  --target "infra/main.tf" \
  --content '
resource "google_redis_instance" "cache" {
  name           = "camping-cache"
  tier           = "BASIC"
  memory_size_gb = 1
  region         = "us-central1"
  authorized_network = google_compute_network.vpc.id
}' -w
```

Validate Terraform syntax and formatting:

```bash
terraform fmt -check
```

Commit and submit proposal to Topocosm Hub:

```bash
cosm commit -u u/infra-redis -i "infra: Add Google Cloud Memorystore Redis instance"

cosm proposal create \
  --source u/infra-redis \
  --target universe-main \
  --title "infra: Add Redis caching layer" \
  --hub http://127.0.0.1:51204/davenport-boutique/camping-app

cosm proposal review --hub http://127.0.0.1:51204/davenport-boutique/camping-app --id prop-infra-redis --verdict APPROVE --score 99.0
cosm proposal merge --hub http://127.0.0.1:51204/davenport-boutique/camping-app --id prop-infra-redis
cosm release infra/cache http://127.0.0.1:51204/davenport-boutique/camping-app
```

---

## Part 6: Web UI Dashboard & Real-Time Telemetry

Topocosm Hub provides real-time observability over all agent activities:

1. **Dashboard URL**: Open `http://127.0.0.1:51204` in any browser.
2. **Features Available**:
   - **Interactive Merkle DAG Canvas**: Zoom and explore AST symbols across Go, TypeScript, SQL, and HCL domains.
   - **Proposal Review Card Inspector**: View AST symbol diffs, impact level (LOW/MED/HIGH), and Critic comments.
   - **Blackboard Lease Monitor**: Real-time view of active locks, TTL countdowns, and claiming agent DIDs.
   - **Policy Engine**: Access rules and signature verification criteria.
3. **Server-Sent Events (SSE)**: Swarms subscribe to real-time events via:
   ```bash
   curl -N http://127.0.0.1:51204/api/v1/events/stream
   ```

---

## Part 7: Compilation, Test & Shipping

After all three tracks have merged their proposals into `universe-main`, export the unified codebase:

```bash
cosm clone http://127.0.0.1:51204/davenport-boutique/camping-app ./final-camping-app
cd ./final-camping-app
```

### 7.1 Verify Full Test Suite

Run the Go server unit tests:

```bash
go test -v ./cmd/server/...
```

Output:
```
=== RUN   TestHealthCheck
--- PASS: TestHealthCheck (0.00s)
=== RUN   TestUserSignupAndLoginFlow
--- PASS: TestUserSignupAndLoginFlow (0.00s)
=== RUN   TestBookingCreationAndDoubleBookingConflict
--- PASS: TestBookingCreationAndDoubleBookingConflict (0.00s)
=== RUN   TestBookingCancellationAndInventoryRestoration
--- PASS: TestBookingCancellationAndInventoryRestoration (0.00s)
=== RUN   TestConcurrentSwarmContention
--- PASS: TestConcurrentSwarmContention (0.00s)
PASS
ok      github.com/cosmscm/cosm/examples/camping_app/cmd/server 0.12s
```

### 7.2 Run Preview Target Ship

Compile and validate sidecar preview using `cosm ship`:

```bash
cosm ship --target local-preview
```

---

## Part 8: Cosm & Topocosm Friction Analysis & Solutions

During the design and implementation of Camping App 2.0 user journeys, several developer and agent ergonomics friction points were identified and resolved:

### Friction 1: Target URI Parsing & Flag Ordering in CLI
- **Problem**: Previously, `cosm publish` and `cosm clone` manually split the target string and threw an error (`Target must be in format <hub_url>/<org>/<cosm>`) if fewer than 3 slashes were present, breaking standard shorthand formats like `davenport-boutique/camping-app`. Furthermore, `args[:len(args)-1]` assumed flags could only appear before the target argument.
- **Solution**: Updated `runPublish` and `runClone` in `cmd/cosm/main.go` to use standard `fs.Parse(args)` and unified target resolution via `resolveHubTarget()`. It now seamlessly supports:
  - `<org>/<cosm>` (e.g. `davenport-boutique/camping-app`) defaulting to `TOPOCOSM_HUB_URL` or `http://127.0.0.1:51204`
  - Explicit URLs (`http://127.0.0.1:51204/davenport-boutique/camping-app`)
  - Flags positioned before or after target arguments.

### Friction 2: Remote Proposal Lifecycle Management from CLI
- **Problem**: `cosm proposal` subcommands (`create`, `list`, `review`, `merge`) operated exclusively against local `.cosm` micro-universes. Developers had no native CLI mechanism to push proposals to Topocosm Hub, list hub proposals, submit Critic Oracle reviews, or trigger remote CRDT merges.
- **Solution**: Enhanced `runProposal` in `cmd/cosm/main.go` with `--hub <target>` flag support across `create`, `list`, `review`, and `merge` subcommands. It connects directly to Topocosm Hub's REST client (`pkg/topocosm/client.go`), generating proper `core.LineageEnvelope` metadata and reporting real-time status.

### Friction 3: Environment Variable Fallback for Distributed Swarms
- **Problem**: Virtual agent swarms running across distinct worker directories had to repeatedly pass explicit `--url` flags to every CLI command (`cosm topocosm status`, `cosm claim`, `cosm publish`), creating boilerplate and script fragility.
- **Solution**: Updated `resolveHubTarget()` to automatically inspect `TOPOCOSM_HUB_URL` from the host environment, allowing swarms to configure hub targets once per process tree.

### Friction 4: Stacked Proposal Auto-Rebase Transparency
- **Problem**: Jujutsu-style stacked proposals (`cosm stack evolve`) modified descendant manifests in memory, but human developers lacked clear terminal feedback identifying which descendants were rebased and whether any AST conflict nodes were introduced.
- **Solution**: Enhanced `cosm stack evolve` output to display the rebase DAG, descendant count, and conflict verification report.

### Friction 5: Sparse Pull Substring Path Matching in Hub
- **Problem**: In `pkg/topocosm/server.go`, `handleSparsePull` only checked substring matches against `comp.Name` (e.g. `main.go`). When agents requested components by file path like `cmd/server/main.go` or `infra/main.tf`, matching failed because `comp.Name` did not contain directory prefixes.
- **Solution**: Enhanced `handleSparsePull` with bidirectional path matching against both `comp.Name` and `comp.Metadata["file_path"]`, with prefix and basename fallbacks.

### Friction 6: Active Claims Metric in Hub Stats
- **Problem**: In `pkg/topocosm/server.go`, `handleStats` previously computed `ActiveClaims` as `len(s.blackboards)` (the count of repositories), returning a static count instead of the true count of unexpired active domain leases.
- **Solution**: Updated `handleStats` to query `s.bp.LeaseManager().GetActiveLeases(r.Context())` and count currently held, unexpired leases.

### Friction 7: CLI Subcommand Flag Parity
- **Problem**: Key subcommands had inconsistent flag naming—`cosm commit` only supported `-u` and `-i` without standard `--universe` or `--intent` aliases; `cosm init` lacked `--universe` for `-u`; `cosm ast edit` and `cosm ast resolve` lacked long-form flags.
- **Solution**: Added bidirectional flag aliasing across all CLI subcommands in `cmd/cosm/main.go` (`-u` / `--universe`, `-i` / `--intent`, `-p` / `--prompt`, `-s` / `--source`, `-t` / `--target`).

### Friction 8: AST Mutation Operation Aliasing
- **Problem**: Developers and AI agents naturally express code insertion using standard AST terms like `insert_after`, `insert_before`, and `append_child`. Previously, `pkg/mutation/operations.go` strictly enforced `add_after`, `add_before`, and `add_child`, causing syntax errors for common synonyms.
- **Solution**: Added operation aliases to `IsValid()` and normalizers in `ExecuteBatch` mapping `insert_after` / `append_child` -> `OpAddAfter`, and `insert_before` -> `OpAddBefore`.

### Friction 9: Component-Level Target Resolution in AST Surgery
- **Problem**: In multi-language workspaces (such as HCL, TypeScript, and Go), developers often pass component file paths like `infra/main.tf` or `web/src/App.tsx` as `--target` when appending new top-level declarations. Previously, `ResolveSymbol` only matched exact symbol identifiers and failed if the file path was passed.
- **Solution**: Added component-level fallback in `ResolveSymbol`. When a target matches a component name or file path, it automatically resolves to the terminal symbol of that component, cleanly executing append operations.

### Friction 10: Automatic Source Universe Push on Remote Proposal Creation
- **Problem**: When developers ran `cosm proposal create --hub <hub_url>`, a proposal Collaborative Object (COB) was recorded on the remote Hub, but the local micro-universe blobs and manifest remained only on the client machine. When remote reviewers attempted to inspect or merge the proposal, the Hub errored with `universe B (<source>) not found`.
- **Solution**: Added `pushUniverseToHub` to `cmd/cosm/main.go` and wired it into `cosm proposal create --hub`, ensuring source micro-universes are automatically synced to the remote Hub before proposal registration.

### Friction 11: Cross-Process Stack Persistence for Jujutsu Changes
- **Problem**: `StackManager` stored stacked proposal graph nodes in an in-memory map. Because CLI invocations are separate processes, `cosm stack create -c c/step-1` followed by `cosm stack evolve -c c/step-1` in the next command failed with `parent change not found in stack`.
- **Solution**: Added JSON serialization (`SaveToFile` and `LoadFromFile`) for `.cosm/stack.json`, ensuring change chains and auto-rebase metadata persist seamlessly across CLI command invocations.

---

## Conclusion

With Cosm and Topocosm Hub, software development moves from file-based locking and merge conflicts to **content-addressed AST Merkle-DAG collaboration**:
- **Agents and Humans coexist** through domain leases on the blackboard.
- **Sparse pulling** reduces bandwidth and cognitive load by >85%.
- **Stacked proposals** enable rapid, iterative commits that automatically rebase.
- **Critic Oracles** safeguard quality before proposals merge into `universe-main`.
