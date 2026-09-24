# Cosm AST SCM & Polyglot Architecture (`cosm-vscode`)

`cosm-vscode` is the official native Visual Studio Code and Google Antigravity IDE extension for **Cosm (`cosm`)**, providing an AI-native polyglot AST source control management (SCM) and 3-tier architectural development experience.

---

## 1. Architectural Overview & Philosophy

Traditional source control tools (Git, Mercurial, Jujutsu) treat code as linear byte-streams and flat lines of text. Cosm models software as a **content-addressed AST Merkle-DAG** where every function, class, struct, route, and cloud resource is an immutable semantic node with multi-tier causal lineage.

`cosm-vscode` bridges the gap between text editors and AST-native SCM:
1. **Zero-Copy Micro-Universes**: Switch branches and evaluate speculative agent mutations instantly without branch checkout locks or disk clobbering.
2. **3-Tier Cross-Domain Architecture**: Live visual canvas spanning **Frontend** (React/TS), **Backend** (Go/Python), and **Cloud Infrastructure** (Terraform HCL) linked by cross-boundary contract edges (`CALLS /api/v1/users`, `BINDS PORT`, `DEPLOYS_TO`).
3. **AST Merkle Tree Explorer**: Native collapsible tree view (`cosm.astTreeView`) visualizing the entire codebase Merkle DAG: Universe $\rightarrow$ Directories $\rightarrow$ Components $\rightarrow$ AST Symbols $\rightarrow$ Details (Signatures, Contracts/Edges, Lineage, Node ID).
4. **5-Tier Causal Lineage**: Real-time inspection of the originating human prompt, session orchestrator, executing agent, model token telemetry, and Ed25519 cryptographic attestations.
5. **Autonomous Shipping Sidecar**: One-click target compilation (`cosm ship`) with ephemeral preview staging directly inside the IDE.
6. **Real-Time Disk Synchronization**: Integrates with Cosm's auto-sync engine (`--write-disk` / `-w`), ensuring VS Code language servers (`gopls`, `tsserver`, `pyright`), linters, and open editor buffers update automatically on AST surgery.

```
                    ┌────────────────────────────────────────────────────────┐
                    │               Antigravity / VS Code IDE                │
                    └────────────────────────────────────────────────────────┘
                                    │                           │
                   Source Control Panel (SCM)          Status Bar & QuickPick
             [Staged Components / Working AST]       [🌌 universe-main] (Active Head)
                                    │                           │
                                    ▼                           ▼
                     ┌────────────────────────────────────────────────────────┐
                     │          CosmClient (Go CLI / stdio RPC)               │
                     └────────────────────────────────────────────────────────┘
                                    │                           │
                                    ▼                           ▼
            ┌────────────────────────────────────────┐   Polyglot CodeLens & Hover
            │       Cosm Activity Bar Container      │   [5-Tier Causal Pedigree]
            ├───────────────────┬────────────────────┤
            │ Architecture      │ AST Merkle Tree    │
            │ Topology (Webview)│ Explorer (Tree)    │
            │ [FE ➔ BE ➔ Infra] │ [Universe➔Symbols] │
            └───────────────────┴────────────────────┘
```

---

## 2. Compiling & Installing from Source

If you are building `cosm-vscode` from the Cosm repository or compiling alongside the `cosm` core engine, follow these steps.

### Prerequisites
- **Node.js**: `v18.0.0` or higher and `npm`
- **Go**: `1.22+` (required to build the `cosm` backend engine)
- **Target IDE**: Visual Studio Code (`>= 1.85`), Google Antigravity IDE, Cursor, or Windsurf

---

### Step 1: Compile the `cosm` Core Engine Binary
The extension communicates with the `cosm` CLI engine via stdio RPC and JSON protocols. First, build the `cosm` binary from the repository root:

```bash
# 1. Navigate to the root of the cosm repository
cd /path/to/cosm

# 2. Compile the pure-Go binary (zero CGO required)
go build -o cosm ./cmd/cosm

# 3. Verify the executable is functional
./cosm version

# 4. Make cosm available in your PATH (choose one):
# Option A: Copy to a system PATH directory
sudo cp cosm /usr/local/bin/

# Option B: Add repository root to your current shell PATH
export PATH="$PWD:$PATH"

# Option C: If keeping cosm in a custom path, configure it in VS Code settings:
# "cosm.binaryPath": "/absolute/path/to/cosm/cosm"
```

---

### Step 2: Compile the TypeScript Extension
Navigate to `editors/vscode`, install dependencies, and compile the TypeScript sources:

```bash
cd editors/vscode

# Install development dependencies (@types/vscode, typescript)
npm install

# Compile TypeScript into JavaScript (out/)
npm run compile

# Run the extension test suite to verify invariants
node ./out/test/runTest.js
```

---

### Step 3: Package into a `.vsix` Distribution Bundle
Package the extension into a standalone `.vsix` installer using `@vscode/vsce`:

```bash
# Generate cosm-vscode-0.1.0.vsix
npx @vscode/vsce package
```
*This generates `cosm-vscode-0.1.0.vsix` in the `editors/vscode` directory.*

---

### Step 4: Install the `.vsix` into your IDE

#### Option A: Command Line Installation
Run the command corresponding to your editor:

```bash
# Standard Visual Studio Code
code --install-extension cosm-vscode-0.1.0.vsix

# Google Antigravity IDE
antigravity --install-extension cosm-vscode-0.1.0.vsix

# Cursor
cursor --install-extension cosm-vscode-0.1.0.vsix

# Windsurf
windsurf --install-extension cosm-vscode-0.1.0.vsix
```

#### Option B: GUI Installation (Inside IDE)
1. Open your editor (VS Code, Antigravity, or Cursor).
2. Open the **Extensions View** (`Cmd+Shift+X` on macOS, `Ctrl+Shift+X` on Linux/Windows).
3. Click the **`...` (Views and More Actions)** menu icon in the top right corner of the Extensions panel.
4. Select **"Install from VSIX..."**.
5. Browse to `/path/to/cosm/editors/vscode/cosm-vscode-0.1.0.vsix` and click **Install**.
6. When prompted, reload the editor window.

---

### Step 5: Fast Developer Symlink Mode (Live Local Development)
For contributors actively developing the extension who want live reloads without rebuilding `.vsix` packages every time:

```bash
# Symlink directly into your IDE's extensions folder:

# For VS Code:
ln -s "$(pwd)" ~/.vscode/extensions/cosm-vscode

# For Google Antigravity IDE:
ln -s "$(pwd)" ~/.antigravity/extensions/cosm-vscode

# For Cursor:
ln -s "$(pwd)" ~/.cursor/extensions/cosm-vscode
```

Then in `editors/vscode`, run:
```bash
npm run watch
```
Any changes you save in TypeScript will automatically recompile into `out/`. Press `Cmd+Shift+P` $\rightarrow$ `Developer: Reload Window` in your editor to immediately see updates.

---

## 3. Key Features & Walkthrough

### 3.1. Source Control Management (SCM) Panel & Human Commit Flow
- **Active Universe Header**: Displays current micro-universe and head Merkle root.
- **Title Bar Actions**:
  - `$(check)` **Commit (`cosm.commit`)**: Direct one-click commit of staged AST components using the commit message box (`navigation@1`).
  - `$(layers)` **Create Stack (`cosm.createStack`)**: Launch interactive Jujutsu-style stacked proposal inception wizard (`navigation@2`).
  - `$(refresh)` **Refresh (`cosm.refreshSCM`)**: Re-scans working tree AST components and head state (`navigation@3`).
  - `$(globe)` **Switch Universe (`cosm.switchUniverse`)**: Quick-switch active micro-universe directly from the SCM title bar (`navigation@4`).
- **Resource Group & State Inline Controls**:
  - **Stage All (`+`) / Unstage All (`-`)**: Inline group actions on `Changes` and `Staged Changes` groups (`cosm.stageAll`, `cosm.unstageAll`).
  - **Stage File (`+`) / Unstage File (`-`)**: Inline resource state actions for individual polyglot files (`cosm.stageFile`, `cosm.unstageFile`).
  - **Diff Symbol (`↔`)**: Inline side-by-side AST comparison against universe head reconstitution (`cosm.diffSymbol`).
- **Human Intent & Prompt Commit Flow**: The commit input box captures high-level human semantic intent (`feat(auth): add JWT cache`). The secondary commit action (`cosm.commitWithPrompt`) additionally prompts for originating causal prompt telemetry for 5-tier lineage tracking.
- **Side-by-Side Symbol Diff**: Compare active working tree code directly against the universe head AST reconstitution via the virtual `cosm://` document scheme.

### 3.2. Jujutsu-Style Stacked Proposals View (`cosm.scmView`)
Integrated directly into the VS Code Source Control View Container as **Cosm Stacks (Jujutsu-style)** (`cosm.scmView`):
- **Ordered Proposal Stack**: Visualizes chained micro-universe proposals (`🥞 [order_index] change_id`) ordered from base to frontier tip.
- **Active Universe Badge**: The currently checked-out micro-universe is highlighted with a green `$(pass-filled)` icon and `(Active)` indicator badge.
- **One-Click Universe Switching**: Clicking any stacked proposal automatically switches the workspace micro-universe to that change's universe (`cosm.switchUniverse`).
- **Head & Lineage Tooltips**: Hovering over any proposal reveals its parent change lineage, universe ID, head Merkle hash, author DID, and review status.
- **Autonomous Stack Evolution (`cosm.evolveStack`)**:
  - Rebase descendants automatically whenever upstream contract symbols change (`$(zap)` icon inline on each item and in the view title bar).
  - Maintains topological consistency across stacked PRs without manual conflict-heavy rebases.
- **Interactive Stack Wizard (`cosm.createStack`)**:
  - Step-by-step QuickPick wizard prompting for Change ID (e.g. `c/auth-jwt`), Micro-Universe ID, Parent Change, and Proposal Title.
- **View Header Actions**:
  - `$(layers)` **Create Stack**: Incept new stacked change atop the current universe.
  - `$(zap)` **Evolve Stack**: Auto-rebase descendant changes.
  - `$(refresh)` **Refresh Stack**: Refresh stack proposals from the WAL storage engine.

### 3.3. Status Bar & Micro-Universe QuickPick
Located in the bottom-left status bar:
```
$(globe) [🌌 universe-main]
```
Clicking the status bar item opens the QuickPick action controller:
- **Switch Micro-Universe**: Instant non-linear switching across micro-universes.
- **Create New Micro-Universe**: Zero-copy branching from the active Merkle head.
- **Inspect Stacked Proposals**: Jujutsu-style chained changes (`cosm stack`).
- **Open Architecture Canvas**: Launch the interactive 3-tier visual canvas.
- **Ship Ephemeral Preview**: Build and launch the autonomous shipping sidecar.
- **Audit Blast Radius**: Audit downstream contract dependencies and risk score.

### 3.4. Cross-Domain Architecture Canvas (Webview: `cosm.topologyView`)
Visualizes your polyglot application across 3 swimlanes:
1. **Frontend Tier (React / TypeScript)**: Component declarations, API client calls, routes.
2. **Backend / API Tier (Go Microservices / Python FastAPI)**: Handlers, endpoints, database models.
3. **Cloud Infrastructure Tier (Terraform HCL)**: Cloud Run services, Cloud SQL, IAM policies.

**Interactivity**:
- **Click to Navigate**: Clicking any symbol node instantly opens the enclosing source file and highlights the symbol declaration.
- **Blast-Radius Cascade**: Hovering over a symbol highlights its entire dependency path (callers and dependencies) and dims unrelated architecture.
- **Search & Filter**: Filter symbols in real time across all three tiers.
- **Inline Ship Trigger**: Run `cosm ship` directly from the canvas header.

### 3.5. AST Merkle Tree Explorer (Tree View: `cosm.astTreeView`)
Accessible via the **Cosm AST** container in the Activity Bar alongside the **Architecture Topology** canvas, the **AST Merkle Tree Explorer** (`cosm.astTreeView`) provides fine-grained visual inspection and interaction with the codebase's content-addressed AST Merkle-DAG.

#### Tree Hierarchy:
1. **Universe (`UniverseItem`)**:
   - Displays the active micro-universe ID (e.g. `🌌 universe-main`) and truncated Merkle root hash (e.g. `[Merkle: 8c94fa52...]`).
   - Summary description reflects total component, symbol, and cross-boundary contract edge counts (`(N components, N symbols, N edges)`).
   - Tooltip details full universe metadata, Merkle root, and totals.
2. **Compacted Directories (`DirectoryItem`)**:
   - Collapsible folder hierarchies (e.g. `pkg/core`, `cmd/cosm`, `services/auth`).
   - Single-child directory chains are automatically compacted (e.g. `pkg` containing only `core` becomes `pkg/core`) for streamlined navigation.
3. **Components (`ComponentItem`)**:
   - Source files representing polyglot AST compilation units.
   - Displays filename, polyglot language badge (e.g. `[go]`, `[typescript]`), and symbol count `(N symbols)`.
   - Clicking opens the source file in the active editor.
4. **AST Symbols (`SymbolItem`)**:
   - Fine-grained semantic declarations (functions, methods, structs, classes, interfaces, cloud resources, route endpoints, variables).
   - Displayed with contextual semantic ThemeIcons:
     - Method / Function: `symbol-method`
     - Struct / Class / Interface / Type: `symbol-structure`
     - Cloud / Terraform Resource: `cloud`
     - Endpoint / Route / API: `globe`
     - Variable / Field / Const: `symbol-variable`
   - Single-click activates `cosm.navigateToSymbol`, jumping directly to the declaration line in the editor using intelligent multi-pass signature and keyword matching.
5. **Details (`DetailItem`)**:
   - Expandable inspection sub-items underneath each symbol:
     - **Signature** (`symbol-key`): Displays the full function or type signature, with nested **Docstring** (`note`) if present.
     - **Contracts/Edges** (`references`): Outgoing semantic contract links (`CALLS`, `BINDS_ENV`, `DEPLOYS_TO`, `CONSUMES_API`) showing edge types and target symbol IDs.
     - **Lineage** (`history`): 5-tier causal pedigree showing originating **Prompt** (`comment`), semantic **Intent** (`git-commit`), executing **Agent** (`hubot`), and **Timestamp** (`calendar`).
     - **Node ID** (`key`): Content-addressed SHA-256 AST node hash with one-click clipboard copying.

#### AST Tree Commands:
- `cosm.refreshASTTree`: Refreshes the AST Merkle Tree Explorer from the CLI backend for the active micro-universe. Available in the view title bar and Command Palette.
- `cosm.navigateToSymbol`: Navigates to the enclosing source file and selects/scrolls to the AST symbol declaration. Triggered automatically by clicking any symbol item.
- `cosm.viewASTPayload`: Opens a side-by-side virtual editor displaying the symbol's reconstituted AST Intermediate Representation (IR), JSON payload, and resolved metadata. Available inline on symbol items.
- `cosm.copyNodeId`: Copies the selected symbol or detail node's content-addressed SHA-256 Node ID to the system clipboard. Available inline on symbol items and Node ID detail items.

### 3.6. 5-Tier Causal Lineage CodeLens & Hover
Every function, method, class, and Terraform resource displays an unobtrusive CodeLens:
```
🌌 Pedigree: "Implement OAuth2 PKCE..." • Agent: gemini-3.8-flash • [Verified Ed25519 ✓]
```
Hovering over the symbol header reveals the complete 5-tier causal pedigree:

| Tier | Dimension | Description |
|---|---|---|
| **Tier 1** | **User Prompt & Intent** | Originating human prompt, commit intent, and author User ID (`developer`). |
| **Tier 2** | **Session & Orchestration** | Session UUID and orchestrator agent ID (`cosm-orchestrator`). |
| **Tier 3** | **Executing Agent** | Specialized executing agent (`cosm-ast-surgeon`, `gemini-3.8-flash`). |
| **Tier 4** | **Model Telemetry & Cost** | LLM version, sampling params, prompt/completion/reasoning token counts, latency, and estimated USD cost. |
| **Tier 5** | **AST Node & Cryptography** | Content-addressed SHA-256 Merkle hash and Ed25519 cryptographic signature verification badge (`[Verified Valid Signature ✓]`). |

### 3.7. Command Palette Reference

| Command | Title | Category | Interface Context | Description |
|---|---|---|---|---|
| `cosm.commit` | Commit Staged AST Symbols | Cosm | SCM Title Bar, Palette | Direct one-click commit of staged AST components |
| `cosm.commitWithPrompt` | Commit AST Symbols (with Prompt) | Cosm | SCM Title Bar, Palette | Commits staged AST symbols with causal prompt capture |
| `cosm.createStack` | Create Stacked Change (Jujutsu-style) | Cosm | SCM Title, Stack Title, Palette | Interactive wizard to incept a stacked proposal |
| `cosm.evolveStack` | Evolve Stack (Auto-Rebase Descendants) | Cosm | Stack View Item, Title, Palette | Automatically rebase dependent stacked proposals |
| `cosm.refreshStack` | Refresh Stacked Proposals | Cosm | Stack View Title, Palette | Reloads active proposal stacks from WAL engine |
| `cosm.stageAll` | Stage All AST Components | Cosm | SCM Group Inline, Palette | Stages all modified AST components in working tree |
| `cosm.unstageAll` | Unstage All AST Components | Cosm | SCM Group Inline, Palette | Unstages all staged components back to working tree |
| `cosm.stageFile` | Stage AST Component | Cosm | SCM Resource Inline, Palette | Stages modified component into commit set |
| `cosm.unstageFile` | Unstage AST Component | Cosm | SCM Resource Inline, Palette | Removes component from staged commit set |
| `cosm.diffSymbol` | Compare Symbol AST with Head | Cosm | SCM / Symbol Context, Palette | Diff working AST code against universe head |
| `cosm.switchUniverse` | Switch Micro-Universe | Cosm | Status Bar, SCM Title, Palette | Non-linear switching across micro-universes |
| `cosm.createUniverse` | Create Micro-Universe | Cosm | QuickPick, Palette | Creates a zero-copy branch from active Merkle head |
| `cosm.refreshSCM` | Refresh Workspace State | Cosm | SCM Title Bar, Palette | Refreshes workspace and SCM status |
| `cosm.refreshASTTree` | Refresh AST Merkle Tree | Cosm | AST View Title, Palette | Re-queries the AST Merkle DAG and refreshes tree nodes |
| `cosm.navigateToSymbol` | Navigate to AST Symbol | Cosm | Tree Item Click, Palette | Focuses source file and selects symbol declaration |
| `cosm.viewASTPayload` | View AST Payload & IR | Cosm | Symbol Item Context, Palette | Opens side-by-side virtual editor with JSON/YAML AST IR |
| `cosm.copyNodeId` | Copy AST Node ID | Cosm | Symbol/NodeID Context, Palette | Copies SHA-256 AST node ID to system clipboard |
| `cosm.openTopology` | Open Cross-Domain Topology Canvas | Cosm | Activity Bar, Editor, Palette | Launches 3-tier architecture visualization canvas |
| `cosm.shipPreview` | Ship Target & Ephemeral Preview | Cosm | QuickPick, Canvas, Palette | Compiles target package and launches local preview |
| `cosm.blastRadius` | Audit Blast Radius | Cosm | Symbol Context, Palette | Audits downstream contract dependencies and risk |
| `cosm.resolveSymbol` | Resolve Symbol AST Details | Cosm | Palette | Resolves symbol metadata and IR representation |

---

## 4. Configuration Settings

Configure extension settings via VS Code Settings (`Cmd+,` or `Ctrl+,`):

| Setting | Type | Default | Description |
|---|---|---|---|
| `cosm.binaryPath` | `string` | `"cosm"` | Path to the `cosm` CLI executable (or `cosm` if in PATH, or relative path to workspace binary). |
| `cosm.autoSync` | `boolean` | `true` | Automatically synchronize workspace disk files on AST mutation. |
| `cosm.enableCodeLens` | `boolean` | `true` | Show causal lineage and agent pedigree CodeLens above AST symbols. |
| `cosm.topocosmUrl` | `string` | `"https://topocosm.dev"` | Topocosm cloud distribution hub URL. |
| `cosm.previewTarget` | `string` | `"local-preview"` | Default target profile for `cosm ship` preview builds. |

---

## 5. Developer Workflows

### 5.1. For Developers USING Cosm (Day-to-Day Workflow)

1. **Initialize or Open Workspace**:
   Open a workspace containing `.cosm/`. The extension activates automatically (`onStartupFinished`).
2. **Switching Micro-Universes**:
   Click `[🌌 universe-main]` in the status bar to branch (`u/feature-auth`) or switch active universes.
3. **Staging & Committing**:
   - In the **Cosm SCM Panel**, click `+` on any modified AST symbol to stage it.
   - Enter your semantic intent and user prompt in the commit dialog.
4. **Inspecting Blast Radius & Architecture**:
   - Run `Cmd+Shift+P` $\rightarrow$ `Cosm: Open Cross-Domain Topology Canvas`.
   - Hover over endpoints to inspect which frontend components call them and which Terraform resources deploy them.
5. **Shipping Previews**:
   - Click `🚀 Ship Preview` in the status bar or topology canvas to compile the target and launch local preview.

### 5.2. For Developers BUILDING `cosm-vscode` (Contributing)

The extension is written in TypeScript and designed for minimal dependencies:

```bash
# 1. Navigate to extension directory
cd editors/vscode

# 2. Build TypeScript sources
npm run compile

# 3. Run unit tests
npm test

# 4. Watch mode during development
npm run watch
```

**Debugging in VS Code**:
1. Open `editors/vscode` in VS Code.
2. Press `F5` to launch an **Extension Development Host** window with `cosm-vscode` loaded.
3. Open any Cosm repository (such as the Cosm workspace root) in the host window.

---

## 6. File Structure Reference

```
editors/vscode/
├── package.json                   # Extension manifest, contributes, commands, configuration
├── tsconfig.json                  # TypeScript compiler settings (ES2022, CommonJS, strict)
├── .vscodeignore                  # Packaging exclusion rules
├── README.md                      # Architecture and user documentation
└── src/
    ├── extension.ts               # Extension lifecycle and command registration
    ├── types/
    │   └── index.ts               # TypeScript interfaces matching Cosm Go core domain
    ├── client/
    │   └── cosmClient.ts          # Strongly-typed CLI communication client
    ├── scm/
    │   └── provider.ts            # vscode.SourceControl and virtual document provider
    ├── views/
    │   ├── astTreeProvider.ts     # AST Merkle Tree Explorer (Universe -> Dir -> Comp -> Symbol -> Details)
    │   ├── stackedChangesProvider.ts # Jujutsu-style Stacked Changes Provider (cosm.scmView)
    │   ├── statusBar.ts           # Status bar item & interactive QuickPick controller
    │   ├── topologyTreeProvider.ts # 3-Tier Architecture Tree View Provider
    │   └── topologyWebview.ts     # 3-Swimlane interactive architecture canvas (Webview)
    ├── providers/
    │   └── lineageCodeLens.ts     # CodeLens and 5-Tier Hover lineage providers
    └── test/
        ├── extension.test.ts      # Unit tests verifying client, SCM, lineage, and topology
        └── runTest.ts             # Standalone test runner
```
