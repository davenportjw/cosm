# Cosm CLI Reference Manual

Exhaustive command-line specification for `cosm`.

---

## Global Usage

```bash
cosm <command> [arguments] [flags]
```

### Global Flags
| Flag | Shorthand | Type | Default | Description |
| :--- | :---: | :---: | :--- | :--- |
| `--universe` | `-u` | `string` | `universe-main` | Target micro-universe ID to query or mutate |
| `--intent` | `-i` | `string` | `""` | Intent or commit message description |
| `--agent` | `-a` | `string` | `cosm-user-agent` | Identity of the executing agent or developer |
| `--prompt` | `-p` | `string` | `""` | Originating user prompt text for lineage tracking |
| `--json` | | `bool` | `false` | Emit machine-readable JSON output |
| `--help` | `-h` | `bool` | `false` | Display command help and usage |

---

## Commands

### 1. `cosm init`
Initializes a new `.cosm/` repository structure in the current directory and prepares the initial universe head.
```bash
cosm init [-u <universe_id>] [--ledger]
```
* **Exit Code 0**: Successfully initialized repository.
* **Exit Code 1**: Directory permissions error or already initialized.
* `--ledger`: Initializes the workspace in strict append-only Ledger Mode, writing `"ledger_mode": true` to `.cosm/config.json`. Enforces strict linear Merkle-DAG progression ($C_{N+1} = \text{Commit}(\text{Parent} = C_N, \dots)$); destructive history rewinds, head resets, and `cosm reset` are strictly prohibited.
* **Workspace Configuration (`.cosm/config.json`)**:
  ```json
  {
    "ledger_mode": true,
    "default_universe": "universe-main",
    "created_at": "2026-09-23T20:00:00Z"
  }
  ```
  Persists repository-level settings, default micro-universe targets, and strict audit invariants across all agent sessions.
* **Onboarding Guidance**: Displays explicit next steps for both the **AST-First Paradigm** (`cosm ast create`) and the **Filesystem Staging Lens** (`cosm add .`).

---

### 2. `cosm add`
Stages specified files or recursively walks directory trees into AST symbol nodes. Non-AST raw files (such as `LICENSE`, `README.md`, YAML/TOML configs, and assets) are staged as content-addressed `RawBlobNode`s.
```bash
cosm add <files_or_dirs...> [-u <universe>] [-p <prompt>] [-i <intent>] [-a <agent>]
```
* **Recursive Directory Expansion**: Passing a directory (such as `.` or `services/`) automatically traverses and stages all non-ignored source files, skipping `.cosm/`, `.git/`, `node_modules/`, `vendor/`, and `.cosmignore` entries.
* **AST Codec Extensions**: `.go`, `.tf`, `.hcl`, `.ts`, `.tsx`, `.js`, `.jsx`, `.py`, `.rs`, `.java`, `.cpp`, `.cc`, `.sql`, `.proto`.
* **Raw Non-AST Files**: Any non-code file (e.g. `LICENSE`, `README.md`, `.gitignore`, `Makefile`, configs) is automatically wrapped into a `RawBlobNode` (`language: "raw"`) with verbatim byte preservation, content hashing, and cryptographic lineage tracking.

#### Examples:
```bash
# Stage entire workspace recursively into AST symbol DAG
cosm add .

# Stage specific code symbols, subtrees, and repository metadata
cosm add services/ infra/main.tf LICENSE README.md \
  -p "Add core service, terraform infra, and Apache-2.0 license" \
  -i "Initial project bootstrap"
```

---

### 3. `cosm commit`
Computes the deterministic SHA-256 Merkle root hash of staged components, links cross-domain edges, stamps the `LineageEnvelope`, and advances the universe head.
```bash
cosm commit -i <intent> [-u <universe>] [-a <agent>]
```

---

### 4. `cosm status`
Displays the active micro-universe head Merkle hash, component counts, and the synchronization status of the **Materialized Working Tree Lens** (reporting clean sync or highlighting drifted files on disk).
```bash
cosm status [-u <universe>] [-format json|-f json]
```
Returns human-readable status or structured JSON containing:
```json
{
  "status": "SUCCESS",
  "universe_id": "universe-main",
  "merkle_root": "1c170a7199e6ca461bacbffaa78e33df68d51256fe41739b0d404930a3b1f3c8",
  "components_count": 9,
  "cross_edges_count": 0,
  "components": ["..."],
  "working_tree_lens": {
    "status": "clean",
    "projected_files_count": 5,
    "drifted_files": []
  }
}
```

---

### 5. `cosm topology`
Renders the multi-domain cross-boundary dependency map across Frontend, Backend, and Infrastructure tiers in ASCII, Mermaid, or machine JSON.
```bash
cosm topology [-u <universe>] [--mermaid] [-format json|-f json]
```
Returns structured JSON with arrays of `frontend_nodes`, `backend_nodes`, `infra_nodes`, and aggregate counts.

---

### 6. `cosm lineage`
Traverses the unbroken causal provenance graph for a given AST symbol or component node back to the originating prompt and agent session. Accepts a 64-character Merkle node ID, a short hex prefix, or a human scoped symbol name (e.g. `services/campsite::main.HandleHealth` or `HandleHealth`).
```bash
cosm lineage <node_id_or_symbol> [-u <universe>]
```

---

### 7. `cosm blast-radius`
Audits the downstream blast radius, affected services, and contract risks for a given symbol, component, or agent ID. Accepts a 64-character Merkle node ID, short hex prefix, human scoped symbol name, or agent ID.
```bash
cosm blast-radius <node_id_or_symbol> [-u <universe>] [-format json|-f json]
```
Returns structured JSON with `agent_id`, `total_nodes`, `affected_components`, `risk_score`, and `contracts_at_risk`.

---

### 8. `cosm view`
Reconstitutes an AST symbol or component node from raw binary AST payload into syntax-highlighted source code with full lineage provenance metadata. Supports human symbol identifiers (e.g. `services/campsite::main.HandleHealth` or `HandleHealth`), short hex prefixes, or 64-character Merkle hashes.
```bash
cosm view <node_id_or_symbol> [-u <universe>] [--format terminal|source|ast|markdown|raw]
```
* `--format terminal`: Colorized inspection card with syntax-aligned line numbers and causal provenance.
* `--format source`: Plain hydrated source code.
* `--format ast`: Formatted JSON AST payload.
* `--format markdown`: GitHub Flavored Markdown snippet.
* `--format raw`: Raw unparsed bytes.

---

### 9. `cosm log`
Traverses universe commit ancestry in the WAL graph engine, diffing consecutive component symbol manifests to display author, executing agent, prompt, timestamp, and AST symbol deltas.
```bash
cosm log [-u <universe>] [--format terminal|json] [-n <limit>]
```
* `-u, --universe <id>`: Universe branch to traverse (defaults to active universe or `universe-main`).
* `--format terminal`: Colorized terminal log with commit hashes, author, agent, intent, originating prompt, and symbol diffs (`+ Added`, `~ Modified`, `- Removed`).
* `--format json`: Machine-readable JSON array of `CommitLogEntry` objects.
* `-n <limit>`: Maximum number of commits to display (default: all commits).

---

### 10. `cosm ship`
Executes the autonomous target compilation sidecar. Ephemerally hydrates source code directly from the universe AST Merkle-DAG into an isolated build sandbox (completely decoupled from local workspace disk drift), automatically stages repository build manifests (`go.mod`, `go.sum`, `package.json`, `tsconfig.json`), executes real compilers without synthetic mock fallbacks, captures genuine toolchain diagnostics, and launches an ephemeral local preview sandbox.
```bash
cosm ship [-u|--universe <universe>] [-t|--target <target_profile>]
```
- `-u, --universe <universe>`: Target universe to hydrate and compile (defaults to `universe-main`).
- `-t, --target <target_profile>`: Packaging target profile (`target:cosm`, `target:local-preview`, `target:cloud-run`, etc.). Defaults to `target:cosm`.
- `--format terminal|json`: Output format.

---

### 11. `cosm universe` (alias: `cosm branch`)
Manages zero-copy micro-universes.
```bash
cosm universe list
cosm universe switch <universe_id>
cosm switch <universe_id>
cosm universe create <new_universe_id> -p <parent_universe_id>
cosm universe diff --source <u1> --target <u2>
cosm universe merge --source <source_u> --target <target_u>
```
* `cosm universe switch <universe_id>` (alias: `cosm switch <universe_id>`): Sets the active default micro-universe in `.cosm/config.json`.
* `cosm universe list`: Lists known micro-universes with their head Merkle roots and status, marking the active default universe with `*` and `[ACTIVE]`.

---

### 12. `cosm proposal` (alias: `cosm pr`)
Manages AI-native universe proposals (semantic PRs).
```bash
cosm proposal create --source <u_src> --target <u_tgt> -i <intent>
cosm proposal list
cosm proposal view <proposal_id>
cosm proposal review <proposal_id> --verdict <APPROVE|REJECT> --comment <msg>
cosm proposal merge <proposal_id>
```

---

### 13. `cosm stack`
Manages Jujutsu-style stacked proposals and executes automatic AST-level rebasing across dependent proposal chains.
```bash
# List all active stacked changes and auto-rebase statuses (supports text or -format json / -f json)
cosm stack list [-format json|-f json]

# Register a stacked proposal change
cosm stack create -c <change_id> -u <universe_id> [-p <parent_change_id>] [--title "<title>"]

# Auto-evolve and rebase descendant changes onto updated parent AST root
cosm stack evolve -c <parent_change_id>
```
* `list`: Display active proposals in stack order. Flags: `-format <text|json>`, `-f <text|json>`.
* `-c <change_id>`: Unique change identifier (e.g. `c/auth-model`).
* `-u <universe_id>`: Underlying micro-universe ID backing the proposal.
* `-p <parent_change_id>`: Parent change ID or base branch (default: `universe-main`).
* `--title "<title>"`: Descriptive title for proposal presentation cards.
* **Auto-Rebase Mechanism**: When an upstream parent change mutates, `cosm stack evolve` visits descendant micro-universes in topological order, applies the child's AST delta onto the updated parent manifest root using semilattice union joins ($H_C' = \text{MerkleRoot}(H_P' \sqcup \Delta_{\text{AST}}(C))$), and updates child manifest heads without textual merge collisions.

---

### 14. `cosm ast`
Executes high-precision declarative AST mutation operations (the 10 Geometric AST verbs), scoped symbol resolution, and blank-slate component inception without whole-file serialization roundtrips.
```bash
# Incept a new empty or scaffolded AST component directly in the DAG (blank-slate inception)
cosm ast create -c <component_name> --lang <go|python|typescript|sql> [-w]
# Or incept a component with explicit source code or projection file
cosm ast create -c <component_name> -f <file_path> --code "<code>" [-w]
# Examples:
cosm ast create -c "services/billing" --lang go --type service
cosm ast create -c "services/campsite" -f "services/campsite/main.go" --code "package main..." -w

# Resolve symbol by scoped dotted identifier or name
cosm ast resolve <scoped_target> [-u <universe>] [--format terminal|json]
# Example:
cosm ast resolve "services/auth::ValidateToken"

# Inspect complete AST Merkle Tree hierarchy (ASCII tree or machine JSON)
cosm ast tree [-u <universe>] [--format text|json]
# Example:
cosm ast tree -u universe-main --format json

# Declaratively edit an AST symbol (auto-synchronizes to workspace disk files by default)
cosm ast edit --op <op> --target <target> --content "<content>" [-u <universe>] [--write-disk=true|-w]
# Example:
cosm ast edit --op replace_function_body --target "LRUCache.get" --content "return self.cache.get(key)" -w

# Execute a multi-operation AST batch from file
cosm ast edit --batch <batch.json> [-u <universe>] [--write-disk=true|-w]
```

**Flags**:
- `--write-disk`, `-w` (default: `true`): Automatically synchronizes modified components to workspace disk files so VS Code, Cursor, and Language Server Protocols (LSP) immediately reload updated symbols. Set to `false` for headless DAG-only batch workflows.

**Supported Operations (`--op`)**:
- `replace_function_body`: Spliced replacement of function body retaining signature, docstring, annotations, and parameters.
- `replace_function`: Full signature and body replacement.
- `add_method`: Appends method to class/struct symbol.
- `add_before` / `add_after`: Inserts new symbol node relative to target.
- `delete`: Removes symbol node from component Merkle list.
- `add_import` / `replace_imports`: Package header manipulation.
- `replace_global`: Variable / constant declaration replacement.
- `create_component`: Incepts new component directly in the AST Merkle-DAG.

---

### 15. `cosm import-repo` (alias: `cosm onboard`)
Recursively scans an existing polyglot codebase, extracts AST symbols across all supported languages, infers cross-domain contracts, and commits the initial Merkle DAG.
```bash
cosm import-repo -d <directory_path> [-u <universe>]
```

---

### 16. `cosm import`
Recursively ingests files from a directory into the active micro-universe.
```bash
cosm import -d <directory_path> [-u <universe>]
```

---

### 17. `cosm export`
Reconstitutes the entire active universe AST Merkle-DAG back into standard source code files on disk.
```bash
cosm export -d <target_directory> [-u <universe>]
```

---

### 18. `cosm dashboard` (alias: `cosm tui`)
Launches the full-screen interactive terminal user interface (Bubble Tea TUI) to explore universes, topology graphs, and prompt lineage.
```bash
cosm dashboard
```

---

### 19. `cosm git`
Git compatibility proxy translating Git CLI commands (`status`, `log`, `diff`, `push`, `pull`, `revert`, `reset`, `init-bridge`) into AST Merkle queries.
```bash
cosm git <status|log|diff|push|pull|revert|reset|init-bridge> [args...]
```
* `cosm git status`: Reports clean working tree or unstaged modified files against active universe manifest.
* `cosm git log`: Displays commit ancestry, author, date, and commit messages.
* `cosm git revert [-m <message>] [-u <universe>] <commit>`: Reverts specified commit by appending a forward compensating commit through the Git shim interceptor.
* `cosm git reset [--hard] [-u <universe>] <commit>`: Resets universe head to target commit. When `--hard` is specified, also synchronizes workspace files on disk.
* `cosm git init-bridge [-d <dir>] [-u <universe>]`: Initializes synthetic `.git` bridge for external IDE compatibility.

---

### 20. `cosm topocosm`
Manages the local zero-Docker Topocosm Hub daemon, fixture seeding, multi-agent simulation benchmarks, and hub status.
```bash
# Start local hub daemon on port 51204
cosm topocosm dev [--port 51204] [--dir .topocosm]

# Seed demo polyglot cosms, proposals, and blackboard claims
cosm topocosm seed [--dir .topocosm]

# Run concurrent multi-agent swarm simulation benchmark
cosm topocosm test-swarm [--agents 25] [--duration 5s] [--dir .topocosm]

# Inspect hub status, active proposals, and agent leases
cosm topocosm status [--url http://127.0.0.1:51204]
```

---

### 21. `cosm publish`
Streams and commits the local universe AST Merkle-DAG and content-addressed blobs to a Topocosm Hub (`topocosm.dev` or local daemon).
```bash
cosm publish [flags] <hub_url>/<org>/<cosm>
# Example:
cosm publish http://127.0.0.1:51204/demo-org/cloud-platform -u universe-main -i "Implement Stripe webhook"
```

---

### 22. `cosm clone`
Clones or sparse-pulls a cosm repository from Topocosm Hub, initializing local storage and materializing source code onto disk.
```bash
cosm clone [flags] <hub_url>/<org>/<cosm> [dest_dir]
# Full clone:
cosm clone http://127.0.0.1:51204/demo-org/cloud-platform ./cloud-platform

# Agent-filtered sparse subtree clone:
cosm clone http://127.0.0.1:51204/demo-org/cloud-platform ./billing --sparse "services/billing"
```

---

### 23. `cosm claim`
Acquires an exclusive mutation lease on a blackboard domain on Topocosm Hub to prevent concurrent agent conflicts.
```bash
cosm claim <domain> [flags] [hub_url/org/cosm]

# Examples:
cosm claim services/billing --goal "Refactoring Stripe webhook HMAC validation" --ttl 600
cosm claim infra/pubsub http://127.0.0.1:51204/demo-org/cloud-platform --ttl 300
```
* `--goal <text>`: Intent or description of task holding the domain lease (default: `"AST mutation lease"`).
* `--ttl <seconds>`: Lease time-to-live duration in seconds before automatic expiration (default: `600`).
* `--url <hub_url>`: Explicit Topocosm Hub URL (overrides default or `COSM_HUB_URL`).
* `--agent <did>`: Explicit agent DID identifier holding the lease (overrides auth credentials).

---

### 24. `cosm release`
Releases an active blackboard domain mutation lease on Topocosm Hub, unlocking it for other agents.
```bash
cosm release <domain> [flags] [hub_url/org/cosm]

# Example:
cosm release services/billing
```

---

### 25. `cosm blackboard`
Inspects all active agent blackboard domain leases, holder DIDs, intent goals, and countdown timers.
```bash
cosm blackboard [flags] [hub_url/org/cosm]

# Example:
cosm blackboard
cosm blackboard http://127.0.0.1:51204/demo-org/cloud-platform
```

---

### 26. `cosm peer`
Inspects P2P swarm mesh status and performs sparse AST subtree replication across micro-universes.
```bash
# Query local node DID, CAS cache statistics, and hub connectivity
cosm peer status

# Perform sparse AST subtree synchronization
cosm peer sync -u <universe_id> [--sparse <component_name>]
# Example:
cosm peer sync -u universe-main --sparse "services/billing"
```

---

### 27. `cosm auth`
Manages developer and agent authentication, Personal Access Tokens (PATs), and Topocosm Hub identities.
```bash
# Authenticate against Topocosm Hub
cosm auth login [--token <pat>] [--hub <url>]

# Query active user and DID identity
cosm auth whoami

# Manage Personal Access Tokens
cosm auth token list
cosm auth token create <name> [--days <ttl>]
cosm auth token revoke <token_hash>
cosm auth token set <pat>

# Log out and erase cached credentials
cosm auth logout
```

---

### 28. `cosm credential-helper`
Git-compatible credential helper implementation allowing Git tooling and IDEs to authenticate seamlessly with Cosm and Topocosm Hub.
```bash
cosm credential-helper get
cosm credential-helper store
cosm credential-helper erase
```

---

### 29. `cosm share`
Generates cryptographically sealed, recipient-addressed share bundles for secure peer-to-peer collaboration without exposing raw tokens.
```bash
cosm share [-u <universe>] [--recipient <did>]
```

---

### 30. `cosm mcp`
Launches the Model Context Protocol (MCP) JSON-RPC 2.0 stdio server for AI agents and IDE extensions (Antigravity IDE, Cursor, Windsurf, Claude Desktop).
```bash
cosm mcp [-d <dir>]
```
* **Protocol**: MCP JSON-RPC 2.0 over standard I/O (stdin/stdout).
* **Tools Exposed**:
  * `cosm_status`: Query active universe, staged components, untracked files, and cross-boundary edges.
  * `cosm_ast_resolve`: Scoped symbol resolution (`path::Class.Method`).
  * `cosm_ast_edit`: Surgical AST operations (the 10 Geometric verbs) with workspace disk synchronization.
  * `cosm_blast_radius`: Multi-tier downstream dependency impact analysis.
  * `cosm_topology`: Cross-boundary topology visualization (ASCII/Mermaid).
  * `cosm_universe_create`: Zero-copy micro-universe branching.
  * `cosm_commit`: Causal lineage-stamped manifest commit.
  * `cosm_ship`: Autonomous isolated target build & preview sidecar.

---

### 31. `cosm lsp`
Launches the Language Server Protocol (LSP) stdio server (`Content-Length` framed JSON-RPC 2.0) providing real-time cross-boundary contract diagnostics, cross-language definition jumps, and causal lineage CodeLens annotations.
```bash
cosm lsp [-d <dir>]
```
* **Supported Methods**: `initialize`, `initialized`, `shutdown`, `exit`, `textDocument/didOpen`, `textDocument/didSave`, `textDocument/definition`, `textDocument/codeLens`.
* **Diagnostics**: Live contract validation on save via `core.DetectContractBreakages` across polyglot AST edges.
* **Definition**: Cross-language jumps (Frontend API endpoint $\rightarrow$ Backend route handler $\rightarrow$ Cloud infrastructure resource).
* **CodeLens**: Displays agent DID, intent description, and commit timestamp directly above AST symbol declarations.

---

### 32. `cosm watch`
Runs the background filesystem monitoring daemon that auto-detects source code mutations, parses AST symbol nodes via `codecs.ParseSourceFile`, re-links cross-boundary edges, and commits updated working manifests to the active micro-universe.
```bash
cosm watch [-d <dir>] [-u <universe>] [--interval <ms>]
```
* `--interval <ms>`: File polling interval in milliseconds (default: `250`).
* `--universe <id>`: Target micro-universe to synchronize (default: `universe-main`).
* **Ignore Rules**: Automatically skips `.cosm/`, `.git/`, `node_modules/`, `dist/`, `build/`, `.venv/`, `vendor/`, and hidden directories.

---

### 33. `cosm git init-bridge`
Initializes a synthetic `.git` directory structure (`.git/HEAD`, `.git/config`, `.git/objects/`, `refs/`) synchronized with the active micro-universe so standard Git-aware IDEs and toolchains identify the workspace as a valid repository.
```bash
cosm git init-bridge [-d <dir>] [-u <universe>]
```
* **Synthetic Objects**: Converts active AST manifest components and symbols into virtual loose Git commit and tree objects with SHA-1 addressing.
* **Workspace Isolation**: Configures `.git/info/exclude` to ignore `.cosm/` storage engine files.

---

### 34. `cosm undo`
Unrolls the most recent commit(s), restores the micro-universe head pointer to the previous commit manifest, reverses applied Oplog events, and synchronizes the Plane 2 disk working tree.
```bash
cosm undo [count] [-u <universe>] [-w|--write-disk=true|false]
```
* `count` (optional, default: `1`): Number of commits to unroll sequentially.
* `-u, --universe <universe>`: Target micro-universe (default: `universe-main`).
* `-w, --write-disk` (default: `true`): Automatically synchronizes workspace files on disk to match the restored commit manifest. Deleted files introduced in the undone commits are cleanly unlinked from disk; modified files are restored to their previous content. Set `--write-disk=false` for headless DAG-only operations.
* **Standard Mode vs. Ledger Mode Behavior**:
  * **Standard Mode**: Directly unrolls the universe head pointer to the parent manifest hash and decrements the active commit sequence.
  * **Ledger Mode (`--ledger`)**: Rather than unrolling history or deleting commits, `cosm undo` appends a forward compensating commit ($C_{N+1}$) whose tree matches the parent state ($C_{N-1}$), recording an `ActionRevertManifest` (`"REVERT_MANIFEST"`) event in the Oplog to preserve an immutable cryptographic audit trail.

---

### 35. `cosm revert` (alias: `cosm rollback`)
Appends a forward compensating commit that reverses the AST symbol mutations of a specified target commit while preserving linear history, full causal lineage, and subsequent commits.
```bash
cosm revert <target_hash> [-u <universe>] [-i|--intent <msg>] [-w|--write-disk=true|false]
cosm rollback <target_hash> [-u <universe>] [-i|--intent <msg>] [-w|--write-disk=true|false]
```
* `<target_hash>`: 64-character SHA-256 Merkle root hash or prefix of the commit to revert.
* `-u, --universe <universe>`: Target micro-universe (default: `universe-main`).
* `-i, --intent <msg>`: Causal intent description for the compensating commit (e.g. `"Revert broken auth middleware"`).
* `-w, --write-disk` (default: `true`): Automatically synchronizes modified and removed components to the workspace disk.
* **Non-Destructive Linearity**: Reversal never deletes or rewrites historical commits. It computes the inverse AST delta of the target commit, applies it against the current universe head ($H_{\text{current}}$), logs an `ActionRevertManifest` (`"REVERT_MANIFEST"`) event in `.cosm/graph.db`, and advances the head pointer to the new compensating commit ($C_{\text{new}}$).

---

### 36. `cosm reset`
Repositions the active micro-universe head pointer directly to a specified target commit hash.
```bash
cosm reset [--hard|--soft] <target_hash> [-u <universe>] [-w|--write-disk=true|false]
```
* `<target_hash>`: Target commit Merkle root hash or prefix to reposition the head pointer to.
* `--hard`: Moves the universe head pointer and synchronizes workspace files on disk to match the target commit manifest (equivalent to `-w`).
* `--soft`: Moves the universe head pointer only, leaving the Plane 2 disk working tree files untouched.
* `-u, --universe <universe>`: Target micro-universe (default: `universe-main`).
* `-w, --write-disk` (default: `false` unless `--hard` is passed): Updates workspace files on disk.
* **Strict Ledger Mode Invariant**: `cosm reset` is **strictly prohibited** in repositories initialized with `--ledger` (or with `"ledger_mode": true` in `.cosm/config.json`). Any attempt to execute `cosm reset` in ledger mode fails immediately with exit code 1 (`ErrLedgerLinearityViolation`), preventing destructive history modification or timeline tampering. In ledger mode, use `cosm revert` to reverse changes via forward compensating commits.




