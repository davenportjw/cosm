# Git Interoperability & Local VCS Shim

Cosm is an AI-native AST Source Control Management (SCM) system. To ensure seamless compatibility with existing developer environments, it includes a built-in **Local Git Compatibility Shim Layer** while maintaining a distinct, decentralized collaboration engine.

---

## 1. Differentiating Git vs. GitHub.com

It is vital to distinguish between **Git** and **GitHub.com**:

| Entity | Role & Scope | Cosm Integration |
| :--- | :--- | :--- |
| **Git (Local VCS & Merkle Protocol)** | Content-addressed DAG of commits, trees, and blobs; local plumbing & porcelain tools (`git status`, `git diff`, `git commit`). | **Supported via Local Shim (`pkg/gitshim`)**: Synthesizes in-memory Git objects on-the-fly so local IDEs, VS Code, and terminal Git scripts can query Cosm as a standard Git repository. |
| **GitHub.com (Centralized SaaS Forge)** | Centralized cloud hosting, web PR review UI, issues, webhooks, centralized ref locking (`main`). | **Optional Export Adapter**: Can push synthetic Git commit branches or export Markdown PR cards to GitHub when integrating with centralized teams. |
| **Cosm Native P2P Mesh (Radicle-Inspired)** | Decentralized P2P swarm gossip, CRDT Collaborative Objects (COBs), sparse AST replication, stacked change auto-evolution. | **Cosm Native Core (`pkg/distributed`)**: Default high-velocity collaboration for AI agent swarms and human engineers without centralized bottlenecks. |

---

## 2. Using the `cosm git` Local Proxy

You can use standard Git commands directly through `cosm git`:

### Check Working Tree Status
```bash
cosm git status
```
**Output:**
```
On universe: universe-main (mapped to branch 'main')
Changes tracked in AST Merkle-DAG:
  (use "cosm add <file>..." to stage changes)
	modified:   server.go
	modified:   infra/main.tf
```

### View AST-Reconstituted Diffs
```bash
cosm git diff
```

### Export Synthetic Commit Trees to a Git Remote
When pushing to an external Git repository (e.g. self-hosted Git, GitHub, GitLab), Cosm synthesizes standard Git commit objects stamped with the unbroken prompt lineage envelope:
```bash
cosm git push origin universe-main:main
```
**Output:**
```
🚀 Synthesizing standard Git commit tree from Merkle Root 'a83b92f7e02c'...
   • Synthetic Commits: 1
   • Tree Objects:      3
   • Blob Objects:      6
   • Pushing to:        git@github.com:my-org/my-cloud-app.git
✨ Push completed successfully!
```

---

## 3. How Git Rebase & History Rewriting Map to Cosm

Traditional Git models history as an immutable linear chain of full-file textual snapshots. Correcting code or integrating upstream commits requires destructive history rewrites:

```
Git:  [Commit A] ──> [Commit B] ──> [Commit C] 
         │ (git rebase onto A')
         ▼
      [Commit A'] ──> [Commit B'] ──> [Commit C']  <-- Rewritten hashes & conflict cascades
```

Cosm treats code as a typed, content-addressed AST Merkle-DAG where nodes represent functions, structs, and cloud resources. This eliminates the need for destructive history manipulation:

| Git Operation | Cosm Equivalent | Architectural Difference |
| :--- | :--- | :--- |
| `git rebase <upstream>` | `cosm stack evolve -c <parent_id>` | Descendant micro-universes rebase their AST subtrees via semilattice union joins ($H_C' = \text{MerkleRoot}(H_P' \sqcup \Delta_{\text{AST}}(C))$) without textual rebase collisions. |
| `git commit --amend` | `cosm ast edit` | Surgically edits the targeted AST symbol in-place; untouched symbols retain exact hashes, preserving unbroken causal lineage envelopes. |
| `git rebase -i` (squash/edit) | `cosm ast edit --batch <batch.json>` | Batched declarative AST transformations applied directly against the workspace manifest. |
| `git reset --hard` | `cosm universe switch <parent>` | Resets the active frontier pointer to an existing universe head with zero disk worktree churn. |
| `git cherry-pick <commit>` | `cosm universe merge <src> -s union` | Reconciles discrete AST symbol deltas into the active micro-universe. |
