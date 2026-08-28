# Getting Started with Cosm

This guide walks you through installing Cosm, initializing your first repository, staging polyglot code into AST symbol nodes, and running your first target preview in under 5 minutes.

---

## 1. Installation

### Option A: Build from Source (Go 1.22+)
```bash
git clone https://github.com/cosmscm/cosm.git
cd cosm
go build -o cosm ./cmd/cosm
sudo mv cosm /usr/local/bin/
```

### Option B: Verify Installation
```bash
cosm --help
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

## 4. Interactive Terminal Dashboard

Launch the full-screen Bubble Tea TUI dashboard to inspect micro-universes, review AST graphs, and browse prompt lineage interactively:

```bash
cosm dashboard
```
