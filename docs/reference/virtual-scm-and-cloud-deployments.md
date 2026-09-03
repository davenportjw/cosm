# Virtual SCM & Cloud Deployments: Bridging AST Merkle-DAGs to External CLI Toolchains

This document provides an in-depth architectural and operational analysis of how **Cosm (`cosm`)** bridges its **content-addressed Abstract Syntax Tree (AST) Merkle-DAG** to external cloud deployment tools (such as `gcloud run deploy --source .`, `gcloud builds submit`, `docker build`, `pack build`, `terraform apply`, and `sam deploy`) that expect physical POSIX directory trees on disk.

---

## 1. The Core Architectural Challenge

Traditional build and deployment tools operate under a 50-year-old computing invariant:
$$\text{Source Code} \equiv \text{Physical Text Files on a POSIX Filesystem} \quad (/\text{path}/\text{to}/\text{file.ext})$$

When you execute:
```bash
gcloud run deploy my-agent --source .
```
The `gcloud` CLI executes a multi-step pipeline:
1. Scans the current working directory (`.`) and filters files via `.gcloudignore`.
2. Packages the files into an in-memory/temporary compressed archive (`.tar.gz`).
3. Uploads the tarball to a staging Google Cloud Storage (GCS) bucket (`gs://staging.<PROJECT_ID>.appspot.com`).
4. Dispatches a **Cloud Build** job pointing to the GCS tarball URI.
5. Cloud Build compiles the container image via a `Dockerfile` or Google Cloud Buildpacks (`gcr.io/buildpacks/builder`) and deploys the new revision to Cloud Run.

### The Cosm Reality
In Cosm, code is **not** stored as flat files in a local folder. Software is stored as an **immutable AST Merkle-DAG** inside:
- `.cosm/objects/` (SHA-256 content-addressed AST symbol payloads)
- `.cosm/graph.db` (Pure-Go SQLite WAL with typed components, universe heads, and cross-domain contract edges).

```
┌──────────────────────────────────────┐       ┌──────────────────────────────────────┐
│        Cosm Storage Reality          │       │      External Tool Requirement       │
│  (AST Merkle-DAG & SQLite Graph)     │  vs.  │    (POSIX Directory / File Tree)     │
│                                      │       │                                      │
│  • Symbol: sym-py-89b41a (AST Node)  │       │  • app/database.py (Physical File)   │
│  • Edge:   Contract(FastAPI -> SQL)  │       │  • app/main.py     (Physical File)   │
│  • Head:   cf9c2a53947f96c9 (Merkle) │       │  • infra/main.tf   (Physical File)   │
└──────────────────────────────────────┘       └──────────────────────────────────────┘
```

---

## 2. Industry Precedents & Comparative Architectures

How do other world-class systems that decouple source code from local physical disk files solve this problem?

| System | Storage Backend | Local Interface | How External Compilers & Cloud Tools Deploy |
| :--- | :--- | :--- | :--- |
| **Google Piper & CitC** *(Clients in the Cloud)* | Planet-scale Spanner / Bigtable database of object hashes | **FUSE Virtual Filesystem** mounted at `/google/src/cloud/<user>/<ws>/` | Blaze / Bazel read virtual paths; FUSE intercepts read syscalls and streams AST/blobs on-demand from data centers. For remote builds, snapshot IDs are sent directly to Remote Build Execution (RBE). |
| **Meta EdenFS & Sapling (`sl`)** | Mononoke distributed CAS object store | **EdenFS VFS (FUSE / ProjFS)** with local overlay directory | Buck2 and CI daemons read from virtual EdenFS mount points. Unmodified files never hit local disk; Watchman pushes live change notifications to build daemons. |
| **Unison Language (`ucm`)** | Pure-AST SQLite database of hash-addressed definitions | **Ephemeral Scratchpad Views** (`scratch.u`) + Codebase Manager | Unison Cloud deployments serialize AST nodes directly over the wire (`Cloud.main`), bypassing Docker, Buildpacks, and file tarballs entirely. |
| **Cosm (`cosm`)** | Pure-Go SQLite WAL (`.cosm/graph.db`) + CAS (`.cosm/objects/`) | **Multi-Tier: VFS + Staging Sidecar + Direct Stream + Ephemeral Exporter** | Supports native AST shipping (`cosm ship`), POSIX VFS projection, in-memory tarball streaming to Cloud Build (`--tar-source -`), and temporary scratchpad materialization. |

---

## 3. The 4 Cosm Bridging Strategies

Cosm provides 4 distinct strategies depending on the deployment environment, performance requirements, and tool capabilities:

```
                                  ┌─────────────────────────────┐
                                  │   Cosm AST Merkle-DAG Head  │
                                  │   (.cosm/objects/ + SQLite) │
                                  └──────────────┬──────────────┘
                                                 │
            ┌────────────────────────────────────┼────────────────────────────────────┐
            ▼                                    ▼                                    ▼
   ┌──────────────────┐                ┌──────────────────┐                ┌──────────────────┐
   │ Strategy A:      │                │ Strategy B:      │                │ Strategy C:      │
   │ `cosm ship`      │                │ Tarball Stream   │                │ Virtual VFS      │
   │ Autonomous Sidecar│               │ Direct to Cloud  │                │ POSIX Mount      │
   └────────┬─────────┘                └────────┬─────────┘                └────────┬─────────┘
            │                                    │                                    │
            ▼                                    ▼                                    ▼
   ┌──────────────────┐                ┌──────────────────┐                ┌──────────────────┐
   │ Hydrates RAM/Tmp │                │ Generates in-RAM │                │ FUSE / VFS Mount │
   │ Executes gcloud  │                │ .tar.gz stream   │                │ at `.cosm/vfs/`  │
   │ Auto-destroys dir│                │ Piped to gcloud  │                │ On-demand bytes  │
   └──────────────────┘                └──────────────────┘                └──────────────────┘
```

---

### Strategy A: The Native Shipping Sidecar (`cosm ship` - Recommended)

The canonical Cosm-native approach uses `cosm ship` (`pkg/shipping/staging.go`). It treats staging directories as disposable, hermetic environments:

```bash
# Build and deploy universe-main directly to Cloud Run
cosm ship -u universe-main -t target:cloud-run
```

#### Under the Hood (`pkg/shipping/staging.go`):
```go
// 1. Load universe manifest and AST symbols from SQLite WAL
manifest, _ := universeMgr.GetUniverseManifest("universe-main")
compMap, symMap := loadManifestState(blobStore, graphEngine, manifest)

// 2. Hydrate AST symbols into an in-memory virtual file map
hydrator := materialize.NewHydrator()
virtualFiles, _ := hydrator.HydrateWorkspace(manifest, compMap, symMap)

// 3. Create isolated ephemeral staging directory (e.g. /tmp/cosm-stage-91283)
workspace, _ := stagingMgr.CreateStagingWorkspace(targetSpec)
_ = workspace.WriteFiles(virtualFiles)

// 4. Execute the deployment command inside the staging directory
result, _ := workspace.ExecuteCommand("gcloud", "run", "deploy", "my-agent", "--source", ".", "--region", "us-central1")

// 5. Hermetic teardown (deletes /tmp/cosm-stage-91283)
_ = workspace.Cleanup()
```

**Benefits:**
- 100% clean developer workspace (no build artifacts, caches, or temporary files littering source trees).
- Exact target validation (e.g. automated `terraform validate` and `go test` executed before cloud submission).

---

### Strategy B: Zero-Disk In-Memory Tarball Streaming

When deploying to Google Cloud Build, Docker registries, or remote CI without touching local storage, Cosm can stream a synthetic `.tar.gz` archive directly from its AST blob store into standard input:

```bash
# Stream hydrated AST archive directly into Cloud Build without touching disk
cosm export --format tar -u universe-main | gcloud builds submit --tar-source -
```

#### How it Works:
1. `materialize.NewHydrator()` walks the Merkle DAG in memory.
2. An `archive/tar` + `compress/gzip` stream is constructed on-the-fly in standard output (`os.Stdout`).
3. The `gcloud builds submit --tar-source -` command consumes the stdin pipe, uploads directly to the GCS staging bucket, and triggers Cloud Build.
4. **Disk I/O written locally: Exactly 0 bytes.**

---

### Strategy C: Virtual File System (VFS) / POSIX Mount Projection

Cosm provides a VFS projection layer (`pkg/materialize/vfs`) that mounts the AST DAG as a virtual filesystem:

```bash
# Point gcloud directly to the virtual universe mount
gcloud run deploy my-agent \
    --source .cosm/vfs/universe-main/ \
    --region us-central1 \
    --allow-unauthenticated
```

#### How it Works:
- The path `.cosm/vfs/universe-main/` does not store files on disk.
- When `gcloud` invokes POSIX system calls (`opendir`, `readdir`, `stat`, `open`, `read`), Cosm's VFS intercepts the kernel calls.
- Unmodified symbols are reconstituted into byte streams on-demand in $< 1\text{ms}$.
- `gcloud` creates its deployment tarball normally, completely unaware that the underlying filesystem is backed by a Merkle-DAG.

---

### Strategy D: Ephemeral Export Scripting

For custom shell scripts, Terraform `local-exec` provisioners, or legacy CI/CD runners where you want full manual control over the directory:

```bash
#!/usr/bin/env bash
set -euo pipefail

# 1. Allocate isolated temp folder
TMP_SRC=$(mktemp -d)

# 2. Reconstitute active AST micro-universe into temp folder
cosm export -u universe-main -d "${TMP_SRC}"

# 3. Deploy from the ephemeral source directory
gcloud run deploy my-service \
    --source "${TMP_SRC}" \
    --project davenport-boutique \
    --region us-central1 \
    --allow-unauthenticated

# 4. Clean up temporary directory
rm -rf "${TMP_SRC}"
```

**Compact One-Liner:**
```bash
(TMP_SRC=$(mktemp -d) && cosm export -u universe-main -d "$TMP_SRC" && gcloud run deploy my-service --source "$TMP_SRC" --region us-central1 && rm -rf "$TMP_SRC")
```

---

## 4. Architectural Comparison Matrix

| Dimension | Strategy A: `cosm ship` | Strategy B: Tarball Stream | Strategy C: VFS Mount | Strategy D: Ephemeral Export |
| :--- | :--- | :--- | :--- | :--- |
| **Local Disk Writes** | Temporary (auto-deleted) | **0 bytes** | **0 bytes** | Temporary (manual / script cleanup) |
| **Execution Latency** | Very Fast ($< 50\text{ms}$ hydration) | Instantaneous (streaming) | Sub-millisecond lazy fetch | Fast ($< 100\text{ms}$ export) |
| **Command Compatibility** | Any toolchain via TargetSpec | Tools accepting tar stdin (`gcloud builds`, `docker`) | Any POSIX tool (`gcloud`, `docker`, `pack`, `sam`) | 100% universal compatibility |
| **Ideal Use Case** | Daily developer CLI, agent sidecars | Remote cloud builds, container registries, headless CI | Live IDE editing, volume mounts, rapid local iteration | Custom shell scripts, Terraform `local-exec` |

---

## 5. Summary & Best Practice Recommendation

1. **For Cosm-Native Development & CI/CD**: Use **`cosm ship -t target:cloud-run`**. It isolates staging builds, runs pre-flight contract checks, and prevents workspace pollution.
2. **For Headless Cloud Build Submission**: Use **`cosm export --format tar | gcloud builds submit --tar-source -`** for zero-disk, zero-footprint pipeline executions.
3. **For Direct Interactive Scripting**: Use **`cosm export -u <universe> -d $(mktemp -d)`** to bridge any legacy CLI command without changing the mental model.
