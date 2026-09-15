# IDE Quickstart: Developing Applications with Cosm

This guide specifies the developer workflow for engineers and AI agent swarms using **Cosm** as their source control management (SCM), micro-universe branching, and target compilation system inside modern IDEs (VS Code, Antigravity IDE, Cursor, Windsurf).

---

## 1. Environment & Architecture Model

Cosm is an AST Merkle-DAG source control system that stores code as typed symbol nodes rather than flat line-based text files.

```
┌────────────────────────────────────────────────────────┐
│           VS Code / Antigravity IDE Workspace          │
│   • Source Files (.go, .py, .ts, .tf)                  │
│   • Cosm SCM Provider (Staging & Commit)               │
│   • Status Bar Controller ([🌌 universe-main])         │
│   • Cosm MCP Server (cosm mcp)                         │
└───────────────────────────┬────────────────────────────┘
                            │ (Local IPC / stdio)
                            ▼
┌────────────────────────────────────────────────────────┐
│                   Local Cosm Storage                   │
│   ├── .cosm/graph.db  (Pure-Go SQLite WAL Database)    │
│   └── .cosm/objects/  (Content-Addressed AST Blobs)    │
└────────────────────────────────────────────────────────┘
```

### Key Invariant: IDE Disk Synchronization
When mutating code via `cosm ast edit` or MCP tools, Cosm automatically synchronizes changes to disk files (`--write-disk=true` / `-w`). Open editor buffers, Language Server Protocols (`gopls`, `pyright`, `tsserver`), and test runners observe AST mutations instantaneously.

---

## 2. Zero-Configuration IDE Setup

### A. Automatic MCP Tool Registration (Antigravity & Cursor)
Cosm provides `.agents/mcp_config.json` at the repository root:
```json
{
  "mcpServers": {
    "cosm": {
      "command": "cosm",
      "args": ["mcp"],
      "env": {
        "COSM_UNIVERSE": "universe-main"
      }
    }
  }
}
```
When opening the project in Antigravity or an MCP-compliant editor, AI agents immediately obtain native structured tools:
- `cosm_status`: Inspect working tree and universe head.
- `cosm_ast_resolve`: Resolve scoped symbol paths (`path/file.py::ClassName.method`).
- `cosm_ast_edit`: Execute 9 Geometric AST verbs without full-file rewrites.
- `cosm_blast_radius`: Query contract dependencies and impact scores.
- `cosm_topology`: Output 3-tier dependency graph (Frontend $\to$ Backend $\to$ Infra).
- `cosm_universe_create`: Zero-copy micro-universe branching.
- `cosm_commit`: Commit staged AST symbols with causal lineage.
- `cosm_ship`: Ephemeral sandbox preview compilation.

### B. Cosm VS Code / Antigravity Extension (`cosm-vscode`)

#### 1. Compiling & Packaging from the Repository
To compile the extension and core engine from source:

```bash
# Step 1: Compile the core Cosm binary (pure Go, zero CGO)
cd /path/to/cosm
go build -o cosm ./cmd/cosm
sudo cp cosm /usr/local/bin/   # or ensure it is in your PATH

# Step 2: Compile the extension TypeScript sources
cd editors/vscode
npm install
npm run compile

# Step 3: Package into a .vsix distribution bundle
npx @vscode/vsce package
# Produces cosm-vscode-0.1.0.vsix
```

#### 2. Installing into your IDE
Install the compiled `.vsix` package:

```bash
# Visual Studio Code
code --install-extension cosm-vscode-0.1.0.vsix

# Google Antigravity IDE
antigravity --install-extension cosm-vscode-0.1.0.vsix

# Cursor
cursor --install-extension cosm-vscode-0.1.0.vsix

# Windsurf
windsurf --install-extension cosm-vscode-0.1.0.vsix
```
*Alternatively, in your editor: Open Extensions (`Cmd+Shift+X`) $\rightarrow$ Click `...` menu $\rightarrow$ Select **"Install from VSIX..."** $\rightarrow$ choose `cosm-vscode-0.1.0.vsix`.*

#### 3. Live Development / Symlink Mode
If actively contributing or testing changes without packaging `.vsix` bundles:
```bash
# Symlink editors/vscode directly into your IDE's extensions directory:
ln -s "/path/to/cosm/editors/vscode" ~/.vscode/extensions/cosm-vscode
# (Or ~/.antigravity/extensions/cosm-vscode for Google Antigravity)
```
Run `npm run watch` in `editors/vscode`, then press `Cmd+Shift+P` $\rightarrow$ `Developer: Reload Window`.

#### 4. Post-Install Activation
Once installed:
1. Open any workspace containing `.cosm/` (or run `Cmd+Shift+P` $\rightarrow$ `Cosm: Initialize Repository`).
2. The bottom-left status bar displays `[🌌 universe-main]`.
3. The Source Control (SCM) tab lists staged and modified AST symbols.
4. If `cosm` is not in your system `$PATH`, configure `"cosm.binaryPath": "/path/to/cosm"` in your IDE settings (`settings.json`).

---

## 3. Core Development Workflows

### Workflow 1: Inspecting AST State & Workspace Status
```bash
# Check working tree vs universe head
cosm status

# Resolve AST symbol metadata and node ID
cosm ast resolve "app/database.py::DatabaseManager.get_user_state"

# View reconstituted source code with causal provenance
cosm view sym-py-89b41a2e7c00
```

### Workflow 2: Zero-Copy Micro-Universe Branching
Create an isolated micro-universe for an experimental feature:
```bash
cosm universe create u/auth-jwt-refresh --parent universe-main
```
*In VS Code, the status bar switches to `[🌌 u/auth-jwt-refresh]`. Files on disk remain intact until symbols are hydrated or mutated.*

### Workflow 3: Surgical AST Mutation (AI Agent or CLI)
Execute targeted mutations using the 9 Geometric AST verbs:
```bash
cosm ast edit \
  --universe u/auth-jwt-refresh \
  --op add_method \
  --target "services/auth::AuthService" \
  --content "def verify_refresh_token(self, token: str) -> bool:
    return self.jwt_signer.decode(token).get('type') == 'refresh'"
```
**Under the Hood:**
1. Only the new method node is created and hashed into `.cosm/objects/`.
2. Sibling methods retain their exact SHA-256 Merkle hashes.
3. Cosm hydrates the updated component to disk so `pyright` and VS Code re-evaluate types immediately.

### Workflow 4: Cross-Domain Blast Radius & Contract Verification
Before committing or shipping, verify cross-domain contracts:
```bash
# Query blast radius of modified symbol
cosm blast-radius sym-auth-verify

# Render multi-tier topology graph
cosm topology
```

### Workflow 5: Committing with Causal Lineage
Commit the staged symbols to the active micro-universe:
```bash
cosm commit \
  -u u/auth-jwt-refresh \
  -i "feat(auth): add JWT refresh token verification" \
  -p "Implement secure refresh token rotation"
```

### Workflow 6: Target Preview Compilation
Ship directly from the AST Merkle root into an isolated local preview:
```bash
cosm ship -u u/auth-jwt-refresh -t target:local-preview
```

---

## 4. AST Mutation Verbs Reference

| AST Verb | Target Syntax | Scope & Semantics |
| :--- | :--- | :--- |
| `replace_function_body` | `Function` or `Class.method` | Replaces body statements while preserving parameters, return types, decorators, and docstrings. |
| `replace_function` | `Function` or `Class.method` | Replaces entire signature and body statements. |
| `add_method` | `ClassName` | Appends a new method directly into class body. |
| `add_before` | `SymbolIdentifier` | Inserts symbol or statement before target. |
| `add_after` | `SymbolIdentifier` | Inserts symbol or statement after target. |
| `delete` | `SymbolIdentifier` | Removes symbol from parent component Merkle tree. |
| `add_import` | `ComponentPath` | Ingests a new package import or header. |
| `replace_imports` | `ComponentPath` | Replaces all package import blocks. |
| `replace_global` | `Const` / `Var` | Updates global variable or constant declaration. |
