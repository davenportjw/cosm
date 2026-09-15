# Cosm for GitHub & Git Users: The Comprehensive Bridge Guide

This guide establishes the mental model bridge, technical architecture, and practical workflows for developers and teams transitioning from **Git / GitHub.com** to **Cosm (`cosm`)**.

It explains how Cosm bridges the traditional file-based version control paradigm (files, lines, branches, pull requests) to an **AI-native Abstract Syntax Tree (AST) Merkle-DAG**, while providing full backward compatibility with local Git tooling, IDEs, and GitHub remotes.

---

## 1. Mental Model Comparison Matrix

| Workflow Dimension | Traditional Git / GitHub | Cosm AST System (`cosm`) | The Paradigm Shift |
| :--- | :--- | :--- | :--- |
| **Fundamental Atom** | Flat text file lines (`\n`) | **AST Symbol Nodes** (functions, structs, classes, types, routes) | Code is parsed into semantic nodes with typed signatures, AST payloads, and cross-domain edges. |
| **Storage Model** | Git Tree/Blob Merkle DAG (`.git/objects/`) | **Content-Addressed AST Store + SQLite WAL Engine** (`.cosm/objects/` + `.cosm/graph.db`) | Granular symbol deduplication. Modifying 1 function in a 1,000-line file only stores the modified symbol; sibling symbols retain identical hashes. |
| **Branching** | Ref pointers to commit chains (`refs/heads/feature`) requiring working tree checkout | **Zero-Copy Micro-Universes** (`cosm universe create`) | Ephemeral micro-universes branch the AST root instantaneously without disk I/O, file locks, or stash conflicts. |
| **Line Diffs vs Semantic Diffs** | Line-based hunk diffs (`git diff` `-` / `+`) | **AST Topology & Symbol Diffs** (`cosm status`, `cosm view`) | Shows exact structural modifications, signature mutations, and cross-boundary contract breaks. |
| **Commit Model** | Commit author, committer, message | **Multi-Tier Causal Pedigree** (`cosm commit -i <intent> -p <prompt>`) | Cryptographically binds every AST node: $\text{Prompt} \rightarrow \text{Session} \rightarrow \text{Agent ID} \rightarrow \text{Model Params} \rightarrow \text{Ed25519 Signature}$. |
| **Pull Requests (PRs)** | GitHub Web PRs with line comments & file reviews | **Universe Proposals & Jujutsu-Style Stacked Proposals** (`cosm proposal`, `cosm stack`) | Semantic proposal cards with blast-radius risk scoring ($[0.00, 1.00]$), automated AI critic reviews, and CRDT auto-evolution. |
| **CI / CD Builds** | GitHub Actions YAML pipelines running in VMs | **Autonomous Target Compilers & Shipping Sidecars** (`cosm ship`) | Compiles, validates Terraform, builds container images, and boots isolated preview sandboxes directly from the AST Merkle root. |
| **Collaboration Hub** | Centralized GitHub.com SaaS forge | **Decentralized Topocosm Hub (`topocosm.dev`) & P2P Swarm Mesh** | High-velocity agent-swarm coordination, sparse AST subtree synchronization, and CRDT semilattice merges ($\sqcup$). |

---

## 2. The Bridging Architecture: How Cosm Interoperates with Git

Cosm does **not** force you to abandon your existing Git ecosystem or IDEs. Instead, it provides a bidirectional bridging layer:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       Developer IDE / VS Code / Terminal                     │
├──────────────────────────────────────┬──────────────────────────────────────┤
│  A. Traditional Git Porcelain Path   │   B. Cosm-Native AST Commands        │
│  (git status, git diff, git push)    │   (cosm add, cosm commit, cosm ship) │
└──────────────────┬───────────────────┴──────────────────┬───────────────────┘
                   │                                      │
                   ▼                                      ▼
┌──────────────────────────────────────┐ ┌────────────────────────────────────┐
│      Cosm Git Shim (`pkg/gitshim`)   │ │   Cosm AST Engine (`pkg/core`)     │
│  • Synthesizes in-memory Git objects │ │   • Polyglot Parsers & Codecs      │
│  • Emulates .git refs & commit log   │ │   • Multi-Domain Cross-Linker      │
│  • Intercepts IDE Git requests       │ │   • Lineage & Ed25519 Attestor     │
└──────────────────┬───────────────────┘ └────────────────┬───────────────────┘
                   │                                      │
                   ▼                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│            Content-Addressed Storage & SQLite WAL (`.cosm/`)                 │
│  • AST Object Store:  .cosm/objects/ (SHA-256 Blobs)                         │
│  • Graph Engine:      .cosm/graph.db (WAL SQLite: Nodes, Edges, Pedigree)    │
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │
                     ┌─────────────────┴─────────────────┐
                     ▼                                   ▼
┌──────────────────────────────────────┐ ┌────────────────────────────────────┐
│         GitHub.com Remote            │ │     Topocosm Cloud Hub & Mesh      │
│   (Synthetic Commit Tree Push)       │ │     (Direct AST DAG Federation)    │
└──────────────────────────────────────┘ └────────────────────────────────────┘
```

### In-Memory Git Tree Synthesis
When an IDE (e.g. VS Code Source Control panel) or terminal script queries Git, Cosm's Git Shim (`pkg/gitshim`) projects the active micro-universe AST into standard Git trees, commits, and blobs on the fly. This enables:
1. **Zero IDE plugin requirement**: VS Code's built-in Git pane displays changes automatically.
2. **Standard Git remotes**: Push synthetic commit trees to GitHub, GitLab, or Bitbucket via `cosm git push`.
3. **Lossless conversion**: Reconstitutes formatting and docstrings identically.

---

## 3. Step-by-Step GitHub User Walkthrough

### Step 1: Initialize or Onboard an Existing Git Repository

To migrate an existing Git repository into Cosm without losing files:

```bash
# Ingest full repository files, extract AST symbols, and link contracts
cosm import-repo -d . -u universe-main -p "Initial import from GitHub"
```

**Output:**
```
🚀 Onboarding repository at '.' into universe 'universe-main'...
✅ Repository Successfully Onboarded!
   • Total Files Scanned:   42 (Code: 38, Raw: 4)
   • AST Symbols Extracted: 412
   • Components Created:    38
   • Cross-Edges Linked:    14
   • Merkle Root Hash:      cf9c2a53947f
   • Duration:              38ms
```

To create a brand new blank Cosm repository from scratch:
```bash
cosm init --universe universe-main
```

---

### Step 2: Branching with Zero-Copy Micro-Universes

In Git, switching branches requires updating the physical filesystem (`git checkout -b feature/auth`). In Cosm, micro-universes are lightweight non-linear frontier heads:

```bash
# Create an isolated micro-universe branched from universe-main
cosm universe create u/auth-service --parent universe-main

# List all active micro-universes
cosm universe list
```

**Output:**
```
🌌 Active Micro-Universes:
  * u/auth-service (Head: cf9c2a53947f, Status: active)
  * universe-main  (Head: cf9c2a53947f, Status: active)
```

> [!NOTE]
> Creating 1,000 micro-universes consumes **0 MB** of additional disk space because all unchanged AST nodes are content-addressed and shared.

---

### Step 3: Staging & Making Semantic Changes

#### A. Traditional File Staging (`cosm add`)
You can edit files normally in your editor and stage them:
```bash
cosm add backend/auth.py infra/main.tf
```

#### B. Direct AST Surgical Mutation (`cosm ast edit`)
Instead of editing flat files, AI agents or developers can modify exact AST symbols directly:
```bash
cosm ast edit \
  --u u/auth-service \
  --op replace_function_body \
  --target "backend/auth.py::validate_jwt" \
  --content "return jwt.decode(token, secret, algorithms=['HS256'])"
```

---

### Step 4: Committing with Causal Lineage

Git commits only record committer email and timestamp. Cosm commits record the entire causal chain:

```bash
cosm commit \
  -u u/auth-service \
  -i "Add JWT validation with token expiry check" \
  -p "Implement secure JWT validation in auth service" \
  -a "agent-claude-or-human"
```

**Output:**
```
🌟 Committed to universe 'u/auth-service' (Merkle Root: e3b0c44298fc)
   Components: 38 | Cross-Domain Edges: 16 | Causal Lineage: Attested
```

---

### Step 5: Checking Status & Cross-Domain Topology

Instead of simple line counts, Cosm visualizes how changes impact your entire system across frontend, backend, and cloud infrastructure:

```bash
# View working tree AST status
cosm status

# Render full cross-boundary topology graph
cosm topology
```

**Topology Output:**
```
=== Cosm Multi-Domain Contract Topology Graph ===
[Universe: u/auth-service | Root: e3b0c44298fc]

├── [1] FRONTEND UI TIER (TypeScript / React)
│   ├── src/components/Login.tsx:LoginForm (Component)
│       └───[CALLS API: /api/auth/login]───> backend/auth.py:login_handler
│
├── [2] BACKEND SERVICE TIER (Python / FastAPI)
│   ├── backend/auth.py:validate_jwt (Function)
│   ├── backend/auth.py:login_handler (Function)
│       └───[PERSISTS TO]───> CloudSQL:users_table
│
└── [3] CLOUD INFRASTRUCTURE TIER (Terraform HCL)
    ├── infra/main.tf:google_cloud_run_v2_service.auth_service (ResourceBlock)
        └───[BINDS PORT 8080]───> backend/auth.py
```

---

### Step 6: Proposals & Jujutsu-Style Stacking (PR & Rebase Replacement)

In GitHub, stacked PRs require messy rebasing. In Cosm, proposal stacks auto-evolve using CRDTs:

```bash
# Create stacked proposal change
cosm stack create -c ch-auth-jwt -u u/auth-service -p ch-initial-scaffold

# Create a human/agent reviewable Proposal
cosm proposal create \
  --source u/auth-service \
  --target universe-main \
  --title "feat(auth): Add JWT token verification and Cloud Run secrets"

# List and inspect proposals
cosm proposal list
cosm proposal view prop-101
```

#### How-To Rebase: Auto-Evolving Stacked Changes (`git rebase` Replacement)
When upstream parent `ch-initial-scaffold` mutates, rebase all downstream changes automatically:
```bash
cosm stack evolve -c ch-initial-scaffold
# Output:
# ⚡ Auto-evolved 1 descendant change in stack:
#    ✓ Rebased ch-auth-jwt onto new parent AST root without conflict
```

#### How-To Fix History & Mistakes (`git commit --amend` Replacement)
Instead of destructive history rewriting or interactive git rebases (`git rebase -i`), surgically edit the exact AST symbol in-place:
```bash
# Surgically fix symbol without rewriting commit trees
cosm ast edit --op replace_function_body --target "backend/auth.py::validate_jwt" \
  --content "    return jwt.decode(token, secret, algorithms=['RS256'])" -u u/auth-service -w

# Re-commit with updated causal lineage
cosm commit -u u/auth-service -i "Fix RS256 algorithm enforcement" -p "Enforce RS256 algorithm in validate_jwt"
```

**Proposal Card Output:**
```
==============================================================================
📋 Proposal: prop-101 | feat(auth): Add JWT token verification
   Source: u/auth-service ──> Target: universe-main
   Author: agent-claude-or-human | Status: OPEN
==============================================================================

🔍 Semantic AST Deltas:
  • backend/auth.py::validate_jwt: FUNCTION_MODIFIED (+12 lines, -4 lines)
  • infra/main.tf::google_secret_manager_secret.jwt_key: RESOURCE_ADDED

🎯 Blast-Radius Assessment:
  • Risk Score:          0.18 / 1.00 (LOW)
  • Incoming Contracts:  2 (Login.tsx, AppRouter.tsx)
  • Outgoing Contracts:  1 (Cloud Run Environment Secret)
  • Critic Consensus:    ✅ APPROVED (Gemini 3.7 Flash Architecture Oracle)
```

---

### Step 7: Shipping & Deploying (`cosm ship`)

In traditional GitHub workflows, you push to a branch and wait for GitHub Actions CI/CD to run inside a remote VM.

With Cosm, the **Shipping Sidecar** autonomously performs target compilation and staging builds directly from the AST Merkle root:

```bash
# Build, validate Terraform, and run preview sandbox
cosm ship -u u/auth-service -t target:cloud-run
```

**Output:**
```
🚀 Shipping Sidecar Execution Succeeded!
   • Target:       target:cloud-run
   • Terraform:    Validated (0 errors, 0 warnings)
   • Package Size: 18,421,902 bytes (Artifact: art-8392a)
   • Preview URL:  http://localhost:8080
   • Health Probe: OK (HTTP 200)
```

---

### Step 8: Bridging Back to GitHub Remotes

If your organization hosts its canonical upstream on GitHub, push your Cosm changes directly as standard Git commits:

```bash
# Push synthetic commit tree to GitHub remote
cosm git push origin universe-main:main

# Or inspect synthetic Git log
cosm git log
```

**Output:**
```
🚀 Synthesizing standard Git commit tree from Merkle Root 'e3b0c44298fc'...
   • Synthetic Commits: 2
   • Tree Objects:      8
   • Blob Objects:      42
   • Pushing to:        git@github.com:my-org/cloud-agent.git
✨ Push completed successfully!
```

---

## 4. GitHub vs Cosm CLI Command Cheat Sheet

| GitHub / Git Command | Cosm Equivalent | What Cosm Does Differently |
| :--- | :--- | :--- |
| `git init` | `cosm init` | Initializes `.cosm/` object store and SQLite WAL database. |
| `git clone <url>` | `cosm clone <url>` | Sparse-replicates AST nodes with multi-peer mesh acceleration. |
| `git status` | `cosm status` or `cosm git status` | Displays AST symbol modifications and cross-domain contract changes. |
| `git add <files>` | `cosm add <files>` | Parses source into typed AST nodes and stores content-addressed blobs. |
| `git commit -m "<msg>"` | `cosm commit -i "<msg>" -p "<prompt>"` | Records unbroken causal lineage with Ed25519 digital signature. |
| `git commit --amend` | `cosm ast edit` / `cosm symbol edit` | Surgically edits AST symbol in-place; preserves lineage without rewriting ancestor Merkle hashes. |
| `git branch <name>` | `cosm universe create <name>` | Zero-copy micro-universe branching with zero disk overhead. |
| `git checkout <name>` | `cosm universe switch <name>` | Switches active universe pointer without touching disk files. |
| `git reset --hard` | `cosm universe switch <parent>` | Discards uncommitted or branched pointer changes with zero disk churn. |
| `git diff` | `cosm view <node_id>` or `cosm git diff` | Reconstitutes exact AST symbol syntax with semantic highlighting. |
| `gh pr create` | `cosm proposal create` | Generates semantic AST delta cards with blast-radius scoring. |
| `git rebase` / `jj evolve` | `cosm stack evolve -c <change_id>` | Auto-evolves stacked proposals over new AST parents using CRDTs. |
| `git rebase -i` (squash/fixup) | `cosm ast edit --batch <batch.json>` | Executes batched declarative AST transformations with atomic Merkle commit. |
| GitHub Actions CI | `cosm ship -t <target>` | Compiles and validates targets directly from AST Merkle root. |
| `git push origin main` | `cosm git push origin universe-main:main` | Synthesizes Git trees on-the-fly and pushes to GitHub. |
| `git export` | `cosm export -d dist/` | Reconstitutes full AST DAG back into standard directory files on disk. |

---

## 5. Summary: The Best of Both Worlds

With Cosm:
1. **GitHub Developers** retain familiar tools (VS Code, terminal Git commands, GitHub remotes).
2. **AI Agents** gain high-precision AST mutations, micro-universe branch isolation, zero hallucination transcription, and instant blast-radius contract safety.
3. **Teams** get transparent bidirectional bridging between flat files and semantic AST graphs.
