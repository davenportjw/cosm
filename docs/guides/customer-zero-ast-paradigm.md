# Customer Zero: The Cosm AST-Native Architecture & Developer Experience Guide

This document establishes the developer experience, mental model, and operational workflows for building software **AST-first** using the **Cosm AST Source Control Management (SCM)** system (`cosm`).

---

## 1. Paradigm Shift: File-Centric (Git) vs. AST-Centric (Cosm)

Traditional source control systems treat source code as arbitrary bags of ASCII/UTF-8 byte streams arranged in arbitrary filesystem directories. In Git, a commit is a tree of line-delimited text blobs, and changes are expressed as imprecise text diffs (hunks).

**Cosm (`cosm`)** fundamentally inverts this model. Software is not a file tree; software is an **AST Merkle-DAG** of semantic symbols, architectural components, and cross-boundary contract edges.

| Dimension | File-Centric Paradigm (Git) | AST-Centric Paradigm (Cosm) |
| :--- | :--- | :--- |
| **Atomic Unit of Code** | File / Line of text (`blob`) | **AST Symbol Node** (`ASTSymbolNode`: `FuncDecl`, `TypeStruct`, `RouteBinding`, `SQLTable`) |
| **Structural Unit** | Filesystem Directory (`tree`) | **Architectural Component** (`ComponentNode`: Service, Database, Infra, Frontend) |
| **Relationships** | Implicit (discovered by external language servers) | **Explicit Semantic Edges** (`CONSUMES_API`, `QUERIES_TABLE`, `DEPLOYS_TO`, `BINDS_ENV`) |
| **Mutation Mechanism** | Text replacement / string overwrite | **Declarative AST Surgery** (`cosm ast edit`: 9 Geometric AST Verbs) |
| **Disk Files** | The absolute source of truth | **Materialized Projection / Working Lens** (hydrated on demand for LSP / VS Code) |
| **Shipping / Building** | Points compiler directly at dirty local files | **Ephemeral AST Hydration** (`cosm ship` reconstitutes DAG into sandbox before build) |
| **Lineage & Pedigree** | Git commit message + author email | **Causal Pedigree Envelope** (`User Prompt` $\rightarrow$ `Session ID` $\rightarrow$ `Agent ID` $\rightarrow$ `LLM Params` $\rightarrow$ `AST Node`) |

```mermaid
flowchart TD
    subgraph "The Cosm AST-Native Source of Truth (.cosm/)"
        A[WorkspaceManifestNode\nMerkle Root] --> C1[ComponentNode:\nservices/campsite]
        A --> C2[ComponentNode:\ndatabase/schema]
        A --> C3[ComponentNode:\ninfra/cloudrun]
        
        C1 --> S1[ASTSymbolNode:\nfunc HandleBooking]
        C1 --> S2[ASTSymbolNode:\ntype Campsite]
        C1 --> S3[ASTSymbolNode:\ntype Reservation]
        C2 --> S4[ASTSymbolNode:\nCREATE TABLE campsites]
        C2 --> S5[ASTSymbolNode:\nEXCLUSION CONSTRAINT]
        
        S1 -- "QUERIES_TABLE" --> S4
        S1 -- "BINDS_ENV" --> C3
    end

    subgraph "Ephemeral Projections & Shipping"
        A -->|Hydrate Workspace| H[materialize.Hydrator]
        H -->|In-Memory Files| S[StagingWorkspace Sandbox]
        S -->|Compile & Package| B[cosm ship\nGo Binary & Container]
        H -.->|Optional Disk Lens (-w)| D[Workspace Disk Files\nfor VS Code / gopls]
    end
```

---

## 2. What Files Are *Really* Needed on Disk?

In a pure Cosm AST repository, source code files on disk are **projections**, not the canonical storage. When designing Cosm-native repositories, we distinguish between **strictly required host files** and **hydrated AST components**:

### Strictly Required Host Files (Minimal Disk Footprint)
1. **`go.mod` & `go.sum`**: Required by the Go toolchain (`go 1.23+`) to identify the module path, language version, and external checksummed dependencies (e.g., `lib/pq`, `golang.org/x/crypto/bcrypt`).
2. **`.cosmignore`**: Mandatory local hygiene file preventing secret leakage, binaries (`dist/`, `.env`), and OS metadata from entering the AST store.
3. **`.cosm/` (Database & Objects)**:
   - `.cosm/objects/`: Content-addressed immutable blobs (SHA-256) storing `ASTSymbolNode`, `ComponentNode`, and `WorkspaceManifestNode`.
   - `.cosm/graph.db`: Pure-Go WAL database storing semantic edges, micro-universe heads, and event Oplogs.

### What Does NOT Need to Live on Disk as Static Files?
- Application structs, HTTP route handlers, SQL migration schemas, and templates do NOT originate as arbitrary loose files.
- They are defined, versioned, and manipulated as **AST symbols inside Cosm**.
- When an engineer or editor (VS Code, Cursor, `gopls`) inspects the workspace, Cosm's hydrator materializes the source view on disk (via `--write-disk` / `-w`).
- When `cosm ship` builds the container or preview, it hydrates **directly from the Merkle-DAG into an ephemeral staging sandbox**, compiles the target, and ships it—completely decoupled from local file drift!

---

## 3. Customer Zero Experience: From `cosm init` to Building Code

When a developer or autonomous agent runs `cosm init`, how do they start building code? As Customer Zero, we identified the exact workflow and friction points:

### The Desired AST-First Developer Journey

```bash
# 1. Initialize empty Cosm repository with auto-committed initial root manifest
cosm init --universe universe-main

# 2. Check active universe status
cosm status

# 3. Create initial AST component and symbol tree directly in Cosm AST store
cosm ast create --component "services/campsite" --file "cmd/server/main.go" --code "package main\n\nfunc main() {}\n"

# 4. Surgically edit AST symbol payload with full causal lineage tracing
cosm ast edit --target "services/campsite::main" --op replace_function_body --content "println(\"Camping App Active\")"

# 5. Inspect cross-domain topology graph
cosm topology

# 6. Reconstitute AST Merkle-DAG into staging sandbox, compile, and package target
cosm ship --target local-preview
```

---

## 4. Customer Zero Gaps & Invariants Discovered

During empirical testing of `cosm init` and `cmd/cosm`, three friction points were identified and resolved:

### Gap 1: Empty Initial Manifest After `cosm init`
- **Symptom**: `cosm init` created `.cosm/objects` and registered `universe-main` with `HeadManifestHash: ""`. Running `cosm status`, `cosm ship`, or `cosm ast resolve` immediately failed with: `Getting universe manifest: universe universe-main has no committed manifest`.
- **Root Cause**: `UniverseManager.CreateUniverse()` leaves `headManifestHash` empty when `parentUniverseID` is empty, rather than committing an empty initial `WorkspaceManifestNode`.
- **Fix**: In `runInit` (`cmd/cosm/main.go`), immediately commit an empty `WorkspaceManifestNode` with `Components: []string{}` and `CrossEdges: []CrossBoundaryEdge{}`. This ensures the universe starts in a valid, committed state with Merkle root `0000000000000000`.

### Gap 2: Blank-Slate AST Creation (`OpCreateComponent`)
- **Symptom**: All 9 geometric AST verbs in `pkg/mutation/operations.go` required the target symbol or component to ALREADY exist in the universe manifest. There was no declarative way to create the initial component or symbol without first writing a physical file on disk and running `cosm add <file>`.
- **Root Cause**: Inception required the file-centric `cosm add` route.
- **Fix**: Introduce `OpCreateComponent` (`"create_component"`) in `pkg/mutation/operations.go` and `cosm ast create` in `cmd/cosm/main.go`. This allows agents and developers to introduce new components and symbol trees directly into Cosm's AST Merkle-DAG without pre-existing disk files.

### Gap 3: `cosm status` Handling of Zero-Component Universes
- **Symptom**: `cosm status` exited with an error when no commits or components existed.
- **Fix**: `cosm status` displays a clean status:
  ```text
  On universe universe-main (Merkle Root: 06033bdf04e6)
  0 AST components staged. Working tree clean.
  ```

### Gap 4: Go Hydrator Aliased & Anonymous Import Formatting
- **Symptom**: When rehydrating Go components that use aliased or anonymous driver imports (`_ "github.com/jackc/pgx/v5/stdlib"`), the hydrator previously output quotes around the entire string (`"_ github.com/jackc/pgx/v5/stdlib"`), breaking Go compilation (`expected 'STRING', found '_'`).
- **Root Cause**: `pkg/materialize/hydrator.go` treated all items in `comp.Metadata["imports"]` as literal import paths without checking for an alias prefix.
- **Fix**: Updated `pkg/materialize/hydrator.go` to split on whitespace (`strings.Fields(imp)`), extracting the alias/blank identifier and wrapping only the actual import path in quotes (`fmt.Sprintf("%s \"%s\"", alias, path)`).

### Gap 5: Working Tree Lens Transparency
- **Symptom**: Developers noticed standard files in the folder structure and questioned whether Cosm was genuinely AST-native or still relying on disk files as source of truth.
- **Root Cause**: Cosm purposely materializes files to disk for host toolchains (`go test`, `gopls`, `terraform validate`), but `cosm status` and `cosm ship` did not explicitly communicate that disk files are merely an ephemeral **Materialized Working Tree Lens**.
- **Fix**: Upgraded `cosm status` to explicitly report lens synchronization and detect disk drift (`modified on disk` / `missing on disk`), providing explicit commands (`cosm add` to stage or `cosm export -d .` to reset). Updated `cosm ship` to explicitly announce isolated sandbox staging hydrated directly from the Merkle DAG.

---

## 5. Empirical Case Study: The Cosm Camping App

To validate Cosm as a viable replacement for Git, we built the **Cosm Camping App** (`examples/camping_app`) without using Git:
- **Zero `.git` Directory**: SCM is managed 100% via `.cosm/` (WAL database and content-addressed CAS).
- **Dual-Plane Architecture**: The canonical source of truth lives in `.cosm/objects/` and `.cosm/graph.db`. The 5 disk files (`cmd/server/main.go`, `cmd/server/main_test.go`, `deploy/main.tf`, `migrations/001_initial_schema.sql`, `Dockerfile`) are an ephemeral projection forming the **Materialized Working Tree Lens**.
- **5 AST Components**:
  - `database/schema` (PostgreSQL 15 DDL with `btree_gist` exclusion constraint)
  - `services/campsite` (Go HTTP service with HTMX + Tailwind sepia 3-panel layout)
  - `services/campsite_test` (Hermetic Go unit and concurrency tests)
  - `infra/cloudrun` (Terraform HCL infrastructure declaration)
  - `infra/dockerfile` (Multi-stage container specification)
- **5 Verified User Journeys**:
  1. *Campsite Discovery & Availability Inspection* (3-panel UI, faceted filters, real-time availability calendar)
  2. *User Signup & Multi-Persona Authentication* (Bcrypt passwords, session cookies, quick persona switcher)
  3. *Atomic Reservation Booking & Concurrency Enforcement* (High-precision date overlap validation, exclusion lock)
  4. *Reservation Management & Self-Service Cancellation* (Camper dashboard, state transition to `cancelled`, instant inventory replenishment)
  5. *Live Swarm Contention Demonstrator* (Simultaneous goroutines competing for the exact same campsite and dates, demonstrating microsecond-level conflict detection and 409 Conflict return)
- **Zero-Copy Micro-Universes & AST Surgery**:
  - Branched `universe-feat-vip-discount` from `universe-main` in 0ms.
  - Surgically mutated `services/campsite::main.(Server).HandleHealth` using `cosm ast edit` (deduplicating 42 sibling symbols).
  - Formed a Universe Proposal (`cosm proposal create`) and merged it back into `universe-main` (`cosm proposal merge`).
- **Target Compilation, Shipping & Deployment**:
  - `cosm ship -t target:local-preview` verified pure DAG hydration and local container preview.
  - **Zero-Cloud Local Deployment**: Executed `./deploy/deploy_local.sh` for non-blocking daemon execution, automated PID tracking, built-in in-memory concurrency storage, and automated health checks (`http://localhost:8080/health`) with zero GCP credentials.
  - **Production Cloud Deployment**: Deployed containerized target to Google Cloud Run (`davenport-boutique` in `us-central1`) via `./deploy/deploy_cloudrun.sh`.

### Bootstrapping from Git: Compiling the Camping App into Cosm

When cloning the Cosm repository, the Camping App files exist in `examples/camping_app/` as materialized source files, while `.cosm/` is gitignored. To compile the application into Cosm's AST Merkle-DAG and activate the Customer Zero environment:

```bash
cd examples/camping_app

# 1. Initialize Cosm repository on universe-main
cosm init --universe universe-main

# 2. Stage all polyglot files into typed AST symbol nodes
cosm add .

# 3. Commit the Merkle root with causal lineage
cosm commit -u universe-main -i "Compile camping app into cosm"

# 4. Verify AST Merkle root and working tree lens
cosm status

# 5. Compile and deploy locally (Zero-Cloud daemon with health probing)
cosm ship
./deploy/deploy_local.sh

# Or run interactively via standard Go toolchain
go run ./cmd/server
```

Alternatively, run `./compile_into_cosm.sh` to execute the automated compilation sequence, followed by `./deploy/deploy_local.sh` for immediate zero-cloud local deployment.

