# Guide 2: Micro-Universes, AST Surgery & Universe Proposals

This guide covers zero-copy micro-universe branching, AST-level symbol diffing, prompt lineage tracing, Universe Proposals (AI-native PR equivalent), and CRDT-based universe merging.

---

## 1. Conceptual Invariants

* **Micro-Universes**: Micro-universes are lightweight pointers to content-addressed `WorkspaceManifestNode` structures. Creating, diffing, and merging micro-universes requires **zero disk space duplication** and zero worktree checkouts.
* **Universe Proposals**: Human-understandable review cards displaying AST symbol deltas, blast-radius contract impact scores, and cryptographic attestations.

---

## 2. Step-by-Step Validation Walkthrough

### Step 1: Fork a Zero-Copy Micro-Universe Branch
From your initialized repository (`~/cosm-test-workspace`), create a new micro-universe branched from `universe-main`:
```bash
cosm universe create u/auth-feature -p universe-main
cosm universe list
```
**Expected Output:**
```
✨ Created micro-universe 'u/auth-feature' branched from 'universe-main' (Head: ...)
🌌 Active Micro-Universes:
  * u/auth-feature (Head: ..., Status: active)
  * universe-main (Head: ..., Status: active)
```

### Step 2: Make Code Modifications in Your IDE
Open `server.go` in your IDE and add a new HTTP handler function:

```go
package main

import "net/http"

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy"}`))
}

func HandleAuth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"auth":"authorized"}`))
}
```

### Step 3: Stage and Commit to the Micro-Universe
Target the new micro-universe using the `-u` flag:
```bash
cosm add -u u/auth-feature -p "Add auth handler" -i "Implement HandleAuth endpoint" server.go
cosm commit -u u/auth-feature -i "Add HandleAuth endpoint"
```
**Expected Output:**
```
✓ Staged AST Component: server.go (go, 2 symbols, hash: ...)
🌟 Committed to universe 'u/auth-feature' (Merkle Root: ...)
```

### Step 4: Diff Micro-Universes at the AST Level
Compare the canonical branch with your micro-universe:
```bash
cosm universe diff universe-main u/auth-feature
```
**Expected Output:**
```
=== Universe Diff: universe-main <-> u/auth-feature ===
Added Components:    []
Removed Components:  []
Cross-Edge Delta:    0 edges
```

### Step 5: Create and View a Universe Proposal (PR Card)
```bash
# Create the proposal
cosm proposal create --source u/auth-feature --target universe-main --title "Add HandleAuth endpoint"

# View the proposal review card with AST symbol deltas
cosm proposal view --source u/auth-feature --target universe-main
```
**Expected Output:**
```
══════════════════════════════════════════════════════════════════════
 🌌 PROPOSAL: u/auth-feature -> universe-main
 Intent:  Autonomous branch proposal
 Agent:   cosm-agent-worker | Signed: true | Build: PASS
══════════════════════════════════════════════════════════════════════

 📦 AST SYMBOL MODIFICATIONS:
```

### Step 6: Merge Universe Heads
Merge the micro-universe changes back into `universe-main`:
```bash
cosm universe merge u/auth-feature -t universe-main -s union
```
**Expected Output:**
```
🔀 Successfully merged 'u/auth-feature' into 'universe-main' (New Head: ...)
```

### Step 7: Launch the Full-Screen TUI Dashboard
Launch the interactive terminal user interface to browse universes, components, and topology:
```bash
cosm dashboard
```
* **Validation Check**: Verify that navigation across the multi-tier graph and micro-universe list renders in the terminal. Press `Ctrl+C` or `q` to exit.

---

## 3. Validation Summary Checklist

- [ ] Micro-universes fork instantly with zero disk copies.
- [ ] Staging and committing to a scoped universe does not alter `universe-main`.
- [ ] `cosm universe diff` computes exact symbol and component deltas.
- [ ] `cosm proposal view` renders structured PR cards with cryptographic signature status.
- [ ] `cosm universe merge` cleanly combines AST manifests.
- [ ] Bubble Tea dashboard renders universe state and topology interactively.
