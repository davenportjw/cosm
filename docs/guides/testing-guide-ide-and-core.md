# Guide 1: IDE Setup & Polyglot AST Core Workflow

This guide covers setting up **any IDE** (VS Code, Cursor, GoLand, PyCharm, IntelliJ, Zed, Neovim) with Cosm, initializing a repository, staging polyglot source code into AST symbol nodes, inspecting cross-domain topology, and running local preview compilation.

---

## 1. Prerequisites & Compilation

Ensure **Go 1.22+** is installed on your system:

```bash
# Clone and build the cosm CLI binary
cd /Users/jasondavenport/GitHub/future-of-git
go build -o cosm ./cmd/cosm

# Optional: Add to PATH for the current session
export PATH="$(pwd):$PATH"
```

---

## 2. IDE Integration & Connection Model

Cosm is built to be transparent to your favorite editor or IDE:

* **Direct Filesystem Storage**: Cosm operates on standard source files (`.go`, `.py`, `.ts`, `.tf`, etc.) in your project root. Your IDE opens and edits these files natively with full syntax highlighting, autocomplete, and language servers (LSP).
* **Metadata Store**: Cosm tracks immutable AST nodes in `.cosm/objects/` and index graphs in pure-Go SQLite WAL mode (`.cosm/graph.db`).
* **Integrated Terminal**: Execute all `cosm` CLI commands directly within your IDE's built-in terminal.
* **Git Shim Compatibility**: For IDE Git extensions or source control tabs, Cosm provides a local compatibility shim (`cosm git status`, `cosm git diff`, `cosm git log`) that synthesizes Git trees on-the-fly.

```
┌────────────────────────────────────────────────────────┐
│               Any IDE (VS Code / Cursor / Zed)         │
│  • Normal file editing (.go, .py, .ts, .tf)            │
│  • Integrated Terminal (`cosm` CLI commands)           │
│  • Git Source Control Tab (`cosm git` compatibility)   │
└───────────────────────────┬────────────────────────────┘
                            │
                            ▼
 ┌──────────────────────────────────────────────────────┐
 │               Local Workspace Directory              │
 │  ├── server.go        (Hydrated working files)       │
 │  ├── infra/main.tf                                   │
 │  └── .cosm/                                          │
 │      ├── graph.db     (Pure-Go SQLite WAL Database)  │
 │      └── objects/     (Content-Addressed AST Blobs)  │
 └──────────────────────────────────────────────────────┘
```

---

## 3. Step-by-Step Validation Walkthrough

### Step 1: Initialize a New Cosm Workspace
Create an empty workspace directory and initialize Cosm:
```bash
mkdir -p ~/cosm-test-workspace && cd ~/cosm-test-workspace
cosm init -u universe-main
```
**Expected Output:**
```
✨ Initialized empty Cosm repository in .cosm/
   Active Universe: universe-main
```
* **Validation Check**: Verify that `.cosm/objects/` and `.cosm/graph.db` exist.

### Step 2: Create Polyglot Application Files in Your IDE
Open `~/cosm-test-workspace` in your IDE and create the following two files:

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
  name     = "cloud-api-service"
  location = "us-central1"
}
```

### Step 3: Stage and Parse Files into AST Symbols
Cosm parses code into language AST symbol nodes rather than raw text lines. Note that CLI flags must precede file arguments:
```bash
cosm add -p "Implement health check endpoint and Cloud Run infra" -i "Initial backend and infrastructure" server.go infra/main.tf
```
**Expected Output:**
```
✓ Staged AST Component: server.go (go, 1 symbols, hash: ...)
✓ Staged AST Component: infra/main.tf (hcl, 1 symbols, hash: ...)
```

### Step 4: Commit with Cryptographic Lineage
```bash
cosm commit -i "Initial backend and infrastructure commit"
```
**Expected Output:**
```
🌟 Committed to universe 'universe-main' (Merkle Root: ...)
   Components: 2 | Cross-Domain Edges: 0
```

### Step 5: Inspect Workspace Status & Cross-Domain Topology
```bash
# Check working tree status
cosm status

# Render multi-tier cross-domain dependency topology
cosm topology
```
**Expected Output:**
```
================================================================================
                            COSM: TOPOLOGY MAP                                  
================================================================================

┌── [1] FRONTEND TIER (TypeScript / React)
│   (No frontend components detected)
│
├── [2] BACKEND / API TIER (Go Microservices)
│   └── main.HandleHealth (FunctionDecl)
│
└── [3] CLOUD INFRASTRUCTURE TIER (Terraform HCL)
    └── resource.google_cloud_run_service.api (ResourceBlock)

Summary: 2 Nodes, 0 Cross-Domain Edges
```

### Step 6: Validate Shipping Sidecar & Ephemeral Preview
```bash
cosm ship
```
**Expected Output:**
```
🚀 Shipping Sidecar Execution Succeeded!
   • Package Size: ... bytes (Artifact: target:local-preview)
   • Preview URL:  http://127.0.0.1:...
   • Health:       true
```

---

## 4. Validation Summary Checklist

- [ ] Repository initializes cleanly with SQLite WAL engine.
- [ ] Language codecs extract AST symbols for Go and HCL.
- [ ] Merkle root hash is generated upon commit.
- [ ] Topology visualizer classifies components across tiers.
- [ ] Shipping sidecar generates preview package and validates execution.
