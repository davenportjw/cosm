# Cosm (`cosm`): Pull Requests, Universe Proposals & Distributed Collaboration Guide

This document defines how **Pull Requests (PRs)** and their AI-native equivalent—**Universe Proposals**—work in `cosm`, including **human-understandable code review cards**, **Jujutsu-style stacked proposals**, **Radicle-inspired P2P Collaborative Objects (COBs)**, and **Git interoperability**.

---

## 1. Concept: Human-Understandable PRs on an AST SCM

Cosm does **not** abandon the human-understandable concept of a Pull Request (PR). Instead, Cosm **elevates** the PR model to be more expressive, verifiable, and resilient.

| PR Capability | Traditional Git / GitHub PR | `cosm` Universe Proposal (Elevated PR) |
| :--- | :--- | :--- |
| **Discrete Proposed Unit** | Branch comparison with Title & Description | First-class Proposal COB with stable `ChangeID` |
| **Code Representations** | Line-by-line unified text diff (`+` / `-`) | **Dual View**: Unified text diff + syntax-highlighted AST Symbol Cards |
| **Contract & Architecture** | None (hidden inside diff lines) | Visual Cross-Domain Topology Graph (`CONSUMES_API`, `DEPLOYS_TO`, `BINDS_ENV`) |
| **Provenance Pedigree** | Unverified author string & commit message | Cryptographic Ed25519 in-toto envelope ($\text{Prompt} \rightarrow \text{Session} \rightarrow \text{Agent} \rightarrow \text{Model}$) |
| **Review & Critique** | Human line comments | **Dual Review**: Human comments + Autonomous AI Critic multi-vector scoring |
| **Live Target Verification** | External CI build (minutes later) | Sub-second local Preview Sandbox URL (`cosm ship`) with compiler diagnostics |
| **Stacked PR Evolution** | Manual rebase hell when parent PR changes | Automatic AST Evolution (`cosm stack evolve`) with zero rebase collisions |

---

## 2. Distributed Architecture: Radicle-Inspired P2P Swarms & COBs

Traditional centralized hosts (like GitHub) force 50+ concurrent agents through a single bottleneck, leading to massive download egress, central ref locks, and rebase storms. Cosm adopts a **Radicle-inspired distributed P2P architecture**:

```mermaid
sequenceDiagram
    autonumber
    actor Dev as Human Engineer / Orchestrator
    participant Swarm as AI Worker Swarm
    participant Mesh as Cosm P2P Mesh (COBs & CRDTs)
    participant Preview as Target Shipping Sidecar
    participant GitBridge as Local Git Shim / Remote Bridge

    Dev->>Swarm: "Build Stripe Webhook & Terraform PubSub Topic"
    Swarm->>Mesh: Claim Domain (BlackboardCOB: services/billing)
    Swarm->>Mesh: Sparse AST Sync (Download only billing subtree, 45 KB)
    Swarm->>Swarm: Mutate AST & Commit with Ed25519 Lineage
    Swarm->>Preview: Compile & Launch Local Target Sandbox (cosm ship)
    Preview-->>Swarm: Preview URL (http://127.0.0.1:51204) & Health=true
    Swarm->>Mesh: Submit ProposalCOB (c/billing-v1 stacked on universe-main)
    
    rect rgb(240, 248, 255)
        note over Mesh, Dev: Dual Human & Autonomous Critic Review
        Mesh->>Mesh: Run CriticEngine (Contract Drift, IAM Security, Compiler Checks)
        Mesh->>Dev: Render Human PR Presentation Card (AST Diffs + Topology + Sandbox URL)
    end

    alt Human Approves via Local CLI / TUI
        Dev->>Mesh: SetApproval(did:key:reviewer-jason, APPROVED)
        Mesh->>Mesh: Auto-Collapse Proposal into universe-main (cosm merge)
    else Export to External GitHub Repo
        Dev->>GitBridge: cosm git push origin universe-main:main
        GitBridge->>Dev: Pushes synthetic Git commit tree to remote Git repository
    end
```

---

## 3. Developer Workflow Guide

### Guide 3.1: Creating and Proposing a Change (PR)

1. **Fork a Micro-Universe**:
   ```bash
   ./cosm init -u universe-main
   ./cosm universe create u/feature-billing -p universe-main
   ```

2. **Stage and Parse Files into AST**:
   ```bash
   ./cosm add -u u/feature-billing \
       -a "dev-jason" \
       -p "Implement Stripe billing webhook and cloud pubsub subscription" \
       -i "Add billing webhook handler and terraform pubsub topic" \
       services/billing/main.go \
       infra/pubsub.tf \
       web/src/BillingSettings.tsx
   ```

3. **Verify Target Compilation & Live Preview Sandbox**:
   ```bash
   ./cosm ship -u u/feature-billing
   # Output:
   # 🚀 Shipping Sidecar Execution Succeeded!
   #    Package Size: 2410 bytes
   #    Preview URL:  http://127.0.0.1:51204
   #    Health:       true
   ```

4. **Open a Proposal (PR)**:
   ```bash
   ./cosm proposal create \
       --source u/feature-billing \
       --target universe-main \
       --title "Feature: Stripe Billing Webhook & PubSub"
   ```

---

### Guide 3.2: Human Review & Code Inspection

1. **View Rich Human-Understandable PR Presentation**:
   ```bash
   ./cosm proposal view --source u/feature-billing --format terminal
   # Or export full Markdown for Web/IDE preview:
   ./cosm proposal view --source u/feature-billing --format markdown
   ```

2. **Run Autonomous AI Critic Review**:
   ```bash
   ./cosm proposal review --source u/feature-billing --critic critic-agent-core
   ```

3. **Inspect Architecture Topology**:
   ```bash
   ./cosm topology -u u/feature-billing
   ```

4. **Merge the Proposal**:
   ```bash
   ./cosm proposal merge --source u/feature-billing --target universe-main
   ```

---

## 4. Jujutsu-Style Stacked Proposals (`cosm stack`)

Large features are easiest to review when broken into small, discrete, stacked changes (e.g., Data Model $\rightarrow$ API Route $\rightarrow$ Frontend UI $\rightarrow$ Cloud Infra).

1. **Register a Stack of Dependent Changes**:
   ```bash
   ./cosm stack create -c c/auth-model -u u/auth-model -p universe-main --title "Step 1: Auth Model"
   ./cosm stack create -c c/auth-api -u u/auth-api -p c/auth-model --title "Step 2: Auth Route API"
   ./cosm stack create -c c/auth-ui -u u/auth-ui -p c/auth-api --title "Step 3: Auth React UI"
   ```

2. **List the Active Stack**:
   ```bash
   ./cosm stack list
   # Output:
   # 🥞 Jujutsu-Style Stacked Proposals & Change Chains:
   #    [1] 🔹 c/auth-model         (Universe: u/auth-model, Head: a93b1e84)
   #        Parent: universe-main | Auto-Rebase: Active
   #    [2] 🔹 c/auth-api           (Universe: u/auth-api, Head: c4109fa2)
   #        Parent: c/auth-model | Auto-Rebase: Active
   #    [3] 🔹 c/auth-ui            (Universe: u/auth-ui, Head: e82b7190)
   #        Parent: c/auth-api | Auto-Rebase: Active
   ```

3. **Auto-Evolve Descendants When Parent Changes**:
   When you modify `c/auth-model`, all descendant changes automatically rebase their AST subtrees without conflict:
   ```bash
   ./cosm stack evolve -c c/auth-model
   # Output:
   # ⚡ Auto-evolved 2 descendant changes in stack:
   #    ✓ Rebased c/auth-api onto new parent AST root without conflict
   #    ✓ Rebased c/auth-ui onto new parent AST root without conflict
   ```

---

## 5. Decentralized P2P & Sparse Replication (`cosm peer`)

When running parallel agent swarms across multiple processes or distributed machines:

1. **Check Swarm & Seed Node Mesh Status**:
   ```bash
   ./cosm peer status
   ```

2. **Sparse AST Replication (Bandwidth-Optimized Sync)**:
   Instead of cloning a 250 MB repository, an agent synchronizes only the specific component and its contract edges:
   ```bash
   ./cosm peer sync -u universe-main --sparse "services/billing"
   # Output:
   # 📡 Sparse AST Replication for Universe 'universe-main':
   #    • Filter:           services/billing
   #    • Synchronized:     12 AST blobs (1 components, 11 symbols)
   #    • Bandwidth Saved:  94.2% reduction vs full repository clone (12 blobs synced vs 207 total blobs)
   # ✨ Local state synchronized with peer swarm.
   ```

---

## 6. Git Interoperability & Remote Git Compatibility

When collaborating with external teams or CI systems that use standard Git:
- **`cosm git status/diff/log`**: Queries virtual AST trees locally without disk overhead.
- **`cosm git push <remote> <universe>:<branch>`**: Synthesizes standard Git commit trees on-the-fly and pushes to any Git server (GitHub, GitLab, self-hosted Git).
