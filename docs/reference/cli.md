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
Initializes a new `.cosm/` repository structure in the current directory.
```bash
cosm init [-u <universe_id>]
```
* **Exit Code 0**: Successfully initialized repository.
* **Exit Code 1**: Directory permissions error or already initialized.

---

### 2. `cosm add`
Parses specified polyglot source files into AST symbol nodes or stages non-AST raw files (such as `LICENSE`, `README.md`, YAML/TOML configs, and assets) as content-addressed `RawBlobNode`s.
```bash
cosm add <files...> [-u <universe>] [-p <prompt>] [-i <intent>] [-a <agent>]
```
* **AST Codec Extensions**: `.go`, `.tf`, `.hcl`, `.ts`, `.tsx`, `.js`, `.jsx`, `.py`, `.rs`, `.java`, `.cpp`, `.cc`, `.sql`, `.proto`.
* **Raw Non-AST Files**: Any non-code file (e.g. `LICENSE`, `README.md`, `.gitignore`, `Makefile`, configs) is automatically wrapped into a `RawBlobNode` (`language: "raw"`) with verbatim byte preservation, content hashing, and cryptographic lineage tracking.

#### Examples:
```bash
# Stage code symbols and repository metadata (README, LICENSE)
cosm add main.go infra/main.tf LICENSE README.md \
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
Displays the active micro-universe head Merkle hash, component counts, and working tree modification status.
```bash
cosm status [-u <universe>]
```

---

### 5. `cosm topology`
Renders the multi-domain cross-boundary dependency map across Frontend, Backend, and Infrastructure tiers in ASCII or Mermaid format.
```bash
cosm topology [-u <universe>] [--mermaid]
```

---

### 6. `cosm lineage`
Traverses the unbroken causal provenance graph for a given AST symbol or component node back to the originating prompt and agent session.
```bash
cosm lineage <node_id> [-u <universe>]
```

---

### 7. `cosm blast-radius`
Audits the downstream blast radius, affected services, and contract risks for code produced by a specific agent or LLM model version.
```bash
cosm blast-radius <agent_id> [-u <universe>]
```

---

### 8. `cosm view`
Reconstitutes an AST symbol or component node from raw binary AST payload into syntax-highlighted source code with full lineage provenance metadata.
```bash
cosm view <node_id> [-u <universe>]
```

---

### 9. `cosm ship`
Executes the Go shipping sidecar: runs `terraform fmt` and `terraform validate`, compiles Go services, packages composite artifacts, and launches an ephemeral local preview sandbox.
```bash
cosm ship [-u <universe>] [-t <target_profile>]
```
- `-u <universe>`: Target universe to hydrate and compile (defaults to `universe-main`).
- `-t <target_profile>`: Packaging target profile (`target:cosm`, `target:local-preview`, `target:cloud-run`, etc.). Defaults to `target:cosm`.

---

### 10. `cosm universe` (alias: `cosm branch`)
Manages zero-copy micro-universes.
```bash
cosm universe list
cosm universe create <new_universe_id> -p <parent_universe_id>
cosm universe diff --source <u1> --target <u2>
cosm universe merge --source <source_u> --target <target_u>
```

---

### 11. `cosm proposal` (alias: `cosm pr`)
Manages AI-native universe proposals (semantic PRs).
```bash
cosm proposal create --source <u_src> --target <u_tgt> -i <intent>
cosm proposal list
cosm proposal view <proposal_id>
cosm proposal review <proposal_id> --verdict <APPROVE|REJECT> --comment <msg>
cosm proposal merge <proposal_id>
```

---

### 12. `cosm symbol`
Inspects, edits, and audits individual AST symbol nodes in the content-addressed DAG.
```bash
cosm symbol view <symbol_id> [--format terminal|ast|code|contract]
cosm symbol edit --id <symbol_id> --code "<code>" [-u <universe>]
cosm symbol impact --id <symbol_id> [-u <universe>]
```

---

### 13. `cosm ast`
Executes high-precision declarative AST mutation operations (9 Geometric AST verbs) and scoped symbol resolution without whole-file serialization roundtrips.
```bash
# Resolve symbol by scoped dotted identifier or name
cosm ast resolve <scoped_target> [-u <universe>] [--format terminal|json]
# Example:
cosm ast resolve "services/auth::ValidateToken"

# Declaratively edit an AST symbol
cosm ast edit --op <op> --target <target> --content "<content>" [-u <universe>]
# Example:
cosm ast edit --op replace_function_body --target "LRUCache.get" --content "return self.cache.get(key)"

# Execute a multi-operation AST batch from file
cosm ast edit --batch <batch.json> [-u <universe>]
```

**Supported Operations (`--op`)**:
- `replace_function_body`: Spliced replacement of function body retaining signature, docstring, and annotations.
- `replace_function`: Full signature and body replacement.
- `add_method`: Appends method to class/struct symbol.
- `add_before` / `add_after`: Inserts new symbol node relative to target.
- `delete`: Removes symbol node from component Merkle list.
- `add_import` / `replace_imports`: Package header manipulation.
- `replace_global`: Variable / constant declaration replacement.

---

### 13. `cosm import-repo` (alias: `cosm onboard`)
Recursively scans an existing polyglot codebase, extracts AST symbols across all supported languages, infers cross-domain contracts, and commits the initial Merkle DAG.
```bash
cosm import-repo -d <directory_path> [-u <universe>]
```

---

### 14. `cosm import`
Recursively ingests files from a directory into the active micro-universe.
```bash
cosm import -d <directory_path> [-u <universe>]
```

---

### 15. `cosm export`
Reconstitutes the entire active universe AST Merkle-DAG back into standard source code files on disk.
```bash
cosm export -d <target_directory> [-u <universe>]
```

---

### 16. `cosm dashboard` (alias: `cosm tui`)
Launches the full-screen interactive terminal user interface (Bubble Tea TUI) to explore universes, topology graphs, and prompt lineage.
```bash
cosm dashboard
```

---

### 17. `cosm git`
Git compatibility proxy translating Git CLI commands (`status`, `log`, `diff`, `push`, `pull`) into AST Merkle queries.
```bash
cosm git <status|log|diff|push|pull> [args...]
```

---

### 18. `cosm topocosm`
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

### 19. `cosm publish`
Streams and commits the local universe AST Merkle-DAG and content-addressed blobs to a Topocosm Hub (`topocosm.dev` or local daemon).
```bash
cosm publish [flags] <hub_url>/<org>/<cosm>
# Example:
cosm publish http://127.0.0.1:51204/demo-org/cloud-platform -u universe-main -i "Implement Stripe webhook"
```

---

### 20. `cosm clone`
Clones or sparse-pulls a cosm repository from Topocosm Hub, initializing local storage and materializing source code onto disk.
```bash
cosm clone [flags] <hub_url>/<org>/<cosm> [dest_dir]
# Full clone:
cosm clone http://127.0.0.1:51204/demo-org/cloud-platform ./cloud-platform

# Agent-filtered sparse subtree clone:
cosm clone http://127.0.0.1:51204/demo-org/cloud-platform ./billing --sparse "services/billing"
```

