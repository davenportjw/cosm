# Cosm-Native Developer Experience: Working with Code as an AST DAG in VS Code & Modern IDEs

This guide provides a deep technical walkthrough of the **Cosm-Native Developer Experience**. It explains how software engineers and AI agents interact with software as an **Abstract Syntax Tree (AST) Merkle-DAG**, rather than as flat text files arranged in disk folders.

---

## 1. The Core Paradigm Shift: Files vs. Semantic AST Nodes

For 50+ years, software development has been constrained by the physical abstraction of **files on a disk**—a historical artifact of 1970s punched cards, magnetic tapes, and teletype terminals (`\n`).

### Why the File Abstraction Fails in the AI Era
1. **Merge Conflicts**: Git merges lines, blind to syntax. Renaming a variable or reformatting whitespace causes catastrophic merge conflicts across parallel branches.
2. **AI Transcription Errors & Hallucinations**: When an AI edits a 2,000-line file, asking it to output the entire modified file wastes context tokens, slows down execution, and frequently introduces accidental regressions in unrelated functions.
3. **Cross-Domain Blindness**: A flat file cannot express that `backend/models.py:User` is the exact schema contract backing `src/types/user.ts` and `infra/main.tf:google_sql_table`.
4. **Disk Bloat**: Running 50 parallel agent experiments requires 50 full disk clones of the repository.

### The Cosm-Native Solution: The AST Merkle-DAG
In Cosm, code is stored as a **content-addressed Directed Acyclic Graph (DAG)**:
- **Atom**: Individual functions, methods, structs, types, imports, and cloud resources (`ASTSymbolNode`).
- **Component**: A logical grouping of symbols (`ComponentNode`).
- **Workspace**: A Merkle root pointing to the complete graph of components and cross-boundary semantic edges (`WorkspaceManifestNode`).

$$\text{Merkle Root} = \mathcal{H}\left(\sum_{i} \mathcal{H}(\text{Component}_i) \parallel \sum_{j} \mathcal{H}(\text{ContractEdge}_j)\right)$$

```
                               ┌─────────────────────────────┐
                               │ Workspace Merkle Root Head  │
                               │   cf9c2a53947f96c9          │
                               └──────────────┬──────────────┘
                                              │
                     ┌────────────────────────┴────────────────────────┐
                     ▼                                                 ▼
        ┌─────────────────────────┐                       ┌─────────────────────────┐
        │ Backend Component Node  │                       │ Infra Component Node    │
        │   (app/database.py)     │                       │   (infra/main.tf)       │
        └────────────┬────────────┘                       └────────────┬────────────┘
                     │                                                 │
          ┌──────────┴──────────┐                           ┌──────────┴──────────┐
          ▼                     ▼                           ▼                     ▼
┌──────────────────┐  ┌──────────────────┐        ┌──────────────────┐  ┌──────────────────┐
│ Symbol:          │  │ Symbol:          │        │ Symbol:          │  │ Symbol:          │
│ DatabaseManager  │  │ get_user_state   │───────>│ google_sql_db    │  │ google_cloud_run │
│ (ClassDecl)      │  │ (FunctionDecl)   │Contract│ (ResourceBlock)  │  │ (ResourceBlock)  │
└──────────────────┘  └──────────────────┘  Edge  └──────────────────┘  └──────────────────┘
```

---

## 2. How VS Code & Modern IDEs Work with Cosm in AST Mode

Developers do not need to memorize raw Merkle hashes. Cosm provides three native integration mechanisms for editors like **VS Code**, **Cursor**, **Zed**, and **JetBrains**:

### A. The Virtual File System (VFS) Projection
Cosm projects the AST DAG into the editor via a virtual filesystem scheme (`cosm://universe-main/...`) implemented in `pkg/materialize/vfs`.
- **Read**: When VS Code opens a buffer, Cosm reconstitutes the AST symbols on-the-fly into syntax-highlighted source with exact formatting.
- **Save**: When you press `Ctrl+S` / `Cmd+S`, Cosm parses only the modified symbols, computes their new SHA-256 hashes, links cross-domain contracts, and commits the new Merkle root in $< 5\text{ms}$.
- **Zero Disk Mutation**: You can switch between 100 micro-universes in the VS Code status bar without triggering filesystem file watcher storms or rebuilding node_modules.

### B. Language Server Protocol (LSP) AST Bridge (`cosm lsp`)
The Cosm LSP server communicates directly with VS Code's editor features:
- **Symbol Outline**: The editor outline pane displays the live AST hierarchy directly from `.cosm/graph.db`.
- **Go to Definition / Find References**: Traces semantic edges across languages (e.g. clicking a FastAPI route jumps directly to the React `fetch()` call or Terraform IAM binding).
- **Diagnostics**: Flags broken cross-boundary contracts in real-time before you compile or test via `core.DetectContractBreakages`.
- **Causal CodeLens**: Renders prompt, session, agent model, and cryptographic verification status directly above symbol definitions.

### C. Model Context Protocol (MCP) Server for AI Agents (`cosm mcp`)
For AI agents operating inside Antigravity IDE, Cursor, Claude Code, and Windsurf, Cosm provides a stdio JSON-RPC MCP server (`cosm mcp`):
- **Structured Tool Calling**: Replaces fragile shell quoting with type-safe JSON tools (`cosm_status`, `cosm_ast_resolve`, `cosm_ast_edit`, `cosm_blast_radius`, `cosm_topology`, `cosm_universe_create`, `cosm_commit`, `cosm_ship`).
- **Zero-Escape AST Surgery**: Agents mutate code using structured parameters, automatically keeping workspace disk files and AST Merkle-DAG in sync.
- **Configured via `.agents/mcp_config.json`**: Auto-discovered by Antigravity IDE and modern agent frameworks.

### D. Native VS Code / Antigravity Extension (`cosm-vscode`)
The official extension (`cosm/editors/vscode`) brings the full AST experience into the IDE UI:
- **Custom Source Control (`vscode.SourceControl`)**: View staged and modified AST symbols, review fine-grained syntax diffs, and commit with intent and prompt provenance.
- **Status Bar Micro-Universe Controller**: Zero-copy micro-universe switching and branching (`[🌌 universe-main]`).
- **Interactive Cross-Domain Topology Canvas**: Webview panel rendering Frontend $\to$ Backend $\to$ Cloud Infra dependency swimlanes with real-time blast-radius highlighting.

---

## 3. Surgical AST Mutations: The 9 Geometric AST Verbs

When an AI agent or developer modifies code in Cosm, they execute **declarative surgical mutations** rather than overwriting full files:

| AST Verb | Target Syntax | What Happens Under the Hood |
| :--- | :--- | :--- |
| `replace_function_body` | `Function` or `Class.method` | Replaces the implementation statements while preserving signature, docstrings, decorators, and type annotations. |
| `replace_function` | `Function` or `Class.method` | Replaces both signature and body. |
| `add_method` | `ClassName` | Appends a new method directly into the class AST node. |
| `add_before` | `SymbolIdentifier` | Inserts a new symbol or statement immediately before the target in the component sequence. |
| `add_after` | `SymbolIdentifier` | Inserts a new symbol or statement immediately after the target. |
| `delete` | `SymbolIdentifier` | Removes the target symbol from the parent component Merkle tree. |
| `add_import` | `ComponentPath` | Ingests a new package import or header declaration. |
| `replace_imports` | `ComponentPath` | Replaces the entire import block of a component. |
| `replace_global` | `Const` / `Var` | Updates a global constant or variable definition. |

---

## 4. Day-in-the-Life Walkthrough: Building in Pure AST Mode

Let's walk through building and modifying a polyglot application in pure Cosm AST mode.

### Step 1: Inspecting the Active AST Universe

List all components and symbols tracked in the active universe:

```bash
# View summary status
cosm status

# Resolve symbol details and signature
cosm ast resolve "app/database.py::DatabaseManager.get_user_state"
```

**Output:**
```
🎯 Resolved Symbol: DatabaseManager.get_user_state
   • Node ID:    sym-py-89b41a2e7c00
   • Language:   python
   • Type:       MethodDecl
   • Component:  app/database.py
   • Signature:  def get_user_state(self, user_id: str) -> Dict[str, Any]:
```

To view the exact reconstituted syntax and cryptographic lineage of any AST node:
```bash
cosm view sym-py-89b41a2e7c00
```

---

### Step 2: Instant Zero-Copy Micro-Universe Branching

Create an isolated micro-universe for an experimental feature:

```bash
cosm universe create u/fast-redis-cache --parent universe-main
```

> In VS Code, the bottom-left status bar instantly switches from `[🌌 universe-main]` to `[🌌 u/fast-redis-cache]`. No files are rewritten on disk.

---

### Step 3: Performing Surgical AST Surgery (No File Rewrites)

An AI agent or developer can add a Redis caching method to `DatabaseManager` with a single command:

```bash
cosm ast edit \
  --u u/fast-redis-cache \
  --op add_method \
  --target "app/database.py::DatabaseManager" \
  --content "def get_cached_state(self, user_id: str) -> Optional[dict]:
    return self.redis_client.get(f'user:{user_id}')"
```

**What Happens in the Object Store & Workspace:**
1. Only the new method node `sym-py-new-cache` is created and hashed.
2. Sibling methods (`load_combined_state`, `save_combined_state`, `append_event`) retain their exact existing SHA-256 hashes.
3. A new component Merkle hash and workspace root are generated in pure memory.
4. **Automatic IDE Disk Synchronization (`--write-disk` / `-w`)**: Enabled by default, Cosm automatically synchronizes the enclosing file (`app/database.py`) to disk. Open VS Code buffers, Language Server Protocols (`pyright`, `gopls`), and linters instantly reload the updated symbol without manual export steps.

```bash
cosm commit \
  -u u/fast-redis-cache \
  -i "feat(cache): add Redis caching method to DatabaseManager" \
  -p "Add high-speed Redis user state cache"
```

---

### Step 4: Real-Time Cross-Boundary Blast-Radius Auditing

Before compiling or shipping, query Cosm's Contract Cascade Engine to verify downstream safety:

```bash
# Query blast radius of the modified symbol
cosm symbol impact sym-py-new-cache
```

**Output:**
```
🎯 Blast-Radius & Contract Impact for 'sym-py-new-cache':
   • Risk Score:          0.05 / 1.00 (VERY LOW)
   • Incoming Contracts:  1 (app/main.py::chat_turn)
   • Outgoing Contracts:  1 (Redis Connection Pool)
   • Status:              SAFE - No breaking signature changes
```

Inspect the entire multi-tier topology:
```bash
cosm topology
```

---

### Step 5: Direct Target Compilation & Shipping (`cosm ship`)

In Cosm-native mode, you don't run local build scripts or Docker commands manually. You ship directly from the AST Merkle root:

```bash
cosm ship -u u/fast-redis-cache -t target:cloud-run
```

**Output:**
```
🚀 Shipping Sidecar Execution Succeeded!
   • Target:       target:cloud-run
   • Manifest:     Root '7a9c1e042b88'
   • Package Size: 19,840,112 bytes
   • Staging:      Ephemeral sandbox booted at http://localhost:8080
   • Health Check: true (HTTP 200)
```

---

### Step 6: Causal Lineage & Audit Provenance

Every AST node in Cosm has an unbroken causal chain back to its human prompt and AI session:

```bash
cosm lineage sym-py-new-cache
```

**Lineage Output:**
```
=== Causal Pedigree Chain for Node 'sym-py-new-cache' ===
[Tier 1] User Prompt:      "Add high-speed Redis user state cache"
[Tier 2] Session ID:       sess-8839-a912-44df
[Tier 3] Executing Agent:  agent-claude-or-human (Model: gemini-3.7-flash, Temp: 0.2)
[Tier 4] AST Node:         DatabaseManager.get_cached_state (SHA-256: 7a9c1e042b88)
[Tier 5] Ed25519 Sig:      3045022100e4b8a9123f... (Verified Valid)
```

---

## 5. Architectural Invariants for Tool & Agent Builders

When building agent tools or IDE plugins for Cosm:

1. **Never write full files to disk** when performing modifications—use `cosm ast edit` or `mutation.SurgeryEngine`.
2. **Never poll the file system**—subscribe to SQLite WAL change notifications on `.cosm/graph.db`.
3. **Respect content addressing**—all blobs written to `.cosm/objects/` MUST match their SHA-256 hex payload hash.
4. **Always preserve causal lineage**—every mutation MUST supply `intent`, `prompt`, and `agent_id` for Ed25519 signing.
5. **IDE & Workspace Auto-Synchronization**—`cosm ast edit` synchronizes modified components to workspace disk files by default (`--write-disk=true` / `-w`), ensuring VS Code, Cursor, and Language Server Protocols (LSPs) always remain consistent with the AST Merkle DAG. Pass `--write-disk=false` only for headless/DAG-only transformations.

---

## 6. Summary Comparison

| Dimension | File-Based Development (Legacy) | Cosm AST-Native Development |
| :--- | :--- | :--- |
| **Unit of Code** | File lines & characters | Typed AST Symbols (functions, types, resources) |
| **Branch Switching** | Heavy disk I/O, file locks | Instant zero-copy pointer swap |
| **Refactoring Safety** | Hope tests catch breaks | Real-time cross-boundary contract topology |
| **AI Generation** | Full-file generation & hallucinations | Surgical AST payload insertion |
| **Deployment** | Files $\rightarrow$ Git $\rightarrow$ CI/CD $\rightarrow$ Container | AST Root $\rightarrow$ `cosm ship` $\rightarrow$ Target Container |
| **Audit Trail** | Git commit message | Cryptographically signed prompt-to-AST pedigree |
