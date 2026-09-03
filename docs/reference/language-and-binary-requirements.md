# Cosm Language Codec & Binary Asset Specification

This document defines the formal architectural requirements, data contracts, and implementation standards for integrating programming languages, Domain-Specific Languages (DSLs), and raw binary assets into the Cosm AST Source Control and Target Compilation System.

---

## 1. Overview & Architecture Pillars

Every language, schema, or binary format in Cosm must implement three fundamental pillars:

```
┌────────────────────────────────────────────────────────────────────────┐
│                          COSM ASSET LIFECYCLE                          │
└────────────────────────────────────────────────────────────────────────┘
        │                                 │                              │
        ▼                                 ▼                              ▼
┌───────────────────┐           ┌───────────────────┐          ┌───────────────────┐
│ 1. STORAGE        │           │ 2. PRESENTATION   │          │ 3. COMPILATION    │
│    SUBSYSTEM      │           │    SUBSYSTEM      │          │    & SHIPPING     │
├───────────────────┤           ├───────────────────┤          ├───────────────────┤
│ • AST Parser      │           │ • ANSI Terminal   │          │ • Lossless AST    │
│ • Symbol Slicing  │           │ • Contract Cards  │          │   Hydration       │
│ • Merkle Hashing  │           │ • AST JSON Export │          │ • Staging VFS     │
│ • Content-Address │           │ • Topology Graph  │          │ • Toolchain Build │
│   Object Store    │           │ • Blast Radius    │          │ • Hermetic Syntax │
│ • SQLite WAL      │           │ • PR Review Card  │          │   Fallback        │
│   Graph Engine    │           │ • TUI Dashboard   │          │ • Target Preview  │
│ • Causal Lineage  │           │ • Diff Generation │          │ • Health Checks   │
└───────────────────┘           └───────────────────┘          └───────────────────┘
```

---

## 2. Supported Languages & Expansion Matrix

### 2.1 Currently Implemented Languages & Codecs

| Domain | Language Constant | File Extensions | Codec Implementation | Key Extracted AST Entities | Cross-Boundary Edges |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Go** | `LangGo` (`"go"`) | `.go` | `pkg/codecs/golang` (`go/parser`, `go/ast`) | `FunctionDecl`, `StructDecl`, `InterfaceDecl`, `RouteBinding` | `CONSUMES_API`, `DEPLOYS_TO`, `BINDS_ENV` |
| **Terraform HCL** | `LangHCL` (`"hcl"`) | `.tf`, `.tfvars`, `.hcl` | `pkg/codecs/hcl` (`hashicorp/hcl/v2`) | `ResourceBlock`, `DataBlock`, `VariableBlock`, `OutputBlock`, `ModuleBlock` | `DEPLOYS_TO`, `BINDS_ENV` |
| **TypeScript / JS** | `LangTypeScript` (`"typescript"`) | `.ts`, `.tsx`, `.js`, `.jsx` | `pkg/codecs/typescript` (TS/TSX ESTree parser) | `ReactComponent`, `InterfaceDeclaration`, `TypeAliasDeclaration`, `ApiClientCall` | `CONSUMES_API` |
| **Python** | `LangPython` (`"python"`) | `.py` | `pkg/codecs/python` (Python AST parser) | `ClassDef`, `FunctionDef`, `RouteBinding` (FastAPI/Flask), `ImportDecl` | `CONSUMES_API`, `DEPLOYS_TO`, `BINDS_ENV` |
| **Rust** | `LangRust` (`"rust"`) | `.rs` | `pkg/codecs/rust` (Rust AST syntax parser) | `StructDecl`, `EnumDecl`, `FunctionDecl`, `AxumRouteBinding` | `CONSUMES_API`, `BINDS_ENV`, `DEPLOYS_TO` |
| **Java** | `LangJava` (`"java"`) | `.java` | `pkg/codecs/java` (Java AST parser) | `ClassDecl`, `InterfaceDecl`, `RecordDecl`, `SpringEndpoint` | `CONSUMES_API`, `QUERIES_TABLE` |
| **C++ / C** | `LangCpp` (`"cpp"`), `LangC` (`"c"`) | `.cpp`, `.cc`, `.cxx`, `.c`, `.h`, `.hpp` | `pkg/codecs/cpp` (C/C++ parser & hydrator) | `ClassDecl`, `StructDecl`, `FunctionDecl`, `NamespaceDecl` | `CALLS`, `DEPENDS_ON` |
| **SQL** | `LangSQL` (`"sql"`) | `.sql` | `pkg/codecs/sql` (SQL DDL parser) | `TableDecl`, `ColumnDef`, `PrimaryKeyDef`, `ForeignKeyDef`, `IndexDef` | `QUERIES_TABLE` |
| **Protobuf** | `LangProtobuf` (`"protobuf"`) | `.proto` | `pkg/codecs/protobuf` (Proto3 IDL parser) | `MessageDecl`, `EnumDecl`, `ServiceDecl`, `RPCMethodDecl` | `CALLS`, `IMPLEMENTS_RPC` |
| **OpenAPI** | `LangOpenAPI` (`"openapi"`) | `.json`, `.yaml` | Contract schema validator | Paths, Operations, Parameters, Response Schemas | `CONSUMES_API` |
| **Raw / Binary** | `LangRaw` (`"raw"`) | All unparsed files (e.g. `.md`, `.wasm`, `.png`, `.so`, configs) | Lossless raw blob pipeline | `RawBlobNode` (verbatim binary byte fidelity) | `DEPENDS_ON` |

---

### 2.2 Expansion Roadmap

```
Phase 1: Modern Cloud & Mobile Ecosystems
├── Swift (.swift)        ──► SwiftUI, Protocols, URLSession API callers
├── Kotlin (.kt, .kts)    ──► Data Classes, Ktor/Spring routes, Jetpack Compose
└── C# (.cs)              ──► ASP.NET Core controllers, Minimal APIs, EF Entities

Phase 2: Systems & Binary Targets
├── WebAssembly (.wasm)   ──► WASI Preview 2 components, WIT interfaces, exported functions
├── Zig (.zig)            ──► Comptime functions, structs, C-interop declarations
└── GraphQL (.graphql)    ──► Types, Queries, Mutations, Subscriptions, Resolver bindings

Phase 3: Additional Backend & Scripting DSLs
├── Ruby (.rb)            ──► Rails Controllers/Routes, ActiveRecord models
├── PHP (.php)            ──► Laravel/Symfony route attributes, classes
├── Elixir (.ex, .exs)    ──► Phoenix routes, GenServers, Modules
└── Dockerfile/Container  ──► FROM, EXPOSE, ENV directives linked to cloud contracts
```

---

## 3. Pillar 1: Storage Subsystem Requirements

The storage subsystem governs how source code and binaries enter the content-addressed AST Merkle-DAG and the SQLite WAL graph engine.

### 3.1 Lexical & AST Parser Contract (`pkg/codecs/<language>/parser.go`)
Every language parser must implement the standard parsing interface:

```go
type LanguageParser interface {
    ParseSource(filePath string, content []byte, envelope core.LineageEnvelope) (*ParsedFileResult, error)
}
```

#### Requirements:
1. **Atomicity**: The parser must slice continuous text into discrete, semantically cohesive `ASTSymbolNode` instances (e.g. individual structs, functions, routes, tables).
2. **Deterministic Content Hashing**:
   - Each symbol node receives a deterministic SHA-256 `node_id`:
     $$\text{NodeID} = \text{SHA-256}(\text{CanonicalJSON}(\text{Language} \parallel \text{NodeType} \parallel \text{Identifier} \parallel \text{ASTPayload}))$$
3. **Graceful Error Handling**: Syntax errors in a single symbol must not crash the entire ingestion engine; the parser must report errors as structured diagnostics while preserving surrounding valid symbols.
4. **Zero-Loss Raw Fallback for Non-AST / Binary Files**:
   - For unstructured assets, media, pre-compiled binaries (`.wasm`, `.so`, `.png`, `.md`), the codec creates a `core.ASTSymbolNode` with:
     - `Language: core.LangRaw`
     - `NodeType: "RawBlobNode"`
     - `Identifier: <relative_file_path>`
     - `ASTPayload: <verbatim_raw_bytes>`

### 3.2 Relational Graph Engine Persistence (`.cosm/graph.db`)
Parsed symbols are recorded into pure-Go SQLite WAL mode with the following indices:

```sql
CREATE TABLE IF NOT EXISTS ast_nodes (
    node_id TEXT PRIMARY KEY,
    language TEXT NOT NULL,
    node_type TEXT NOT NULL,
    identifier TEXT NOT NULL,
    signature TEXT,
    docstring TEXT,
    visibility TEXT,
    ast_payload BLOB NOT NULL,
    ast_metadata TEXT, -- JSON key-value map
    local_dependencies TEXT, -- JSON string array
    lineage_envelope TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cross_boundary_edges (
    source_node_id TEXT NOT NULL,
    target_node_id TEXT NOT NULL,
    edge_type TEXT NOT NULL, -- CONSUMES_API, DEPLOYS_TO, BINDS_ENV, etc.
    contract_schema_id TEXT,
    metadata TEXT,
    PRIMARY KEY (source_node_id, target_node_id, edge_type)
);
```

### 3.3 Causal Lineage & Provenance Envelope
Every AST symbol mutation must carry an unbroken `LineageEnvelope` recording:
- `user_id` and `user_prompt`
- `session_id`, `orchestrator_agent_id`, `executing_agent_id`
- `llm_version` (e.g. `gemini-3.7-flash`) and `generation_params`
- `intent` description
- `signature_ed25519` cryptographic attestation

---

## 4. Pillar 2: Presentation & UX Requirements

The presentation subsystem governs how code, symbols, contracts, and proposals are presented to human developers, terminal tools, and AI agents.

### 4.1 Multi-Format Rendering Engine (`pkg/materialize/viewer.go`)
Every language codec must support rendering symbols into 6 standard presentation formats:

| Format Mode | Target Medium | Content Rendered |
| :--- | :--- | :--- |
| **`FormatTerminal`** | CLI (`cosm view <id>`) | ANSI color-coded reconstituted source code, syntax highlighting, metadata badge, and Ed25519 signature verification status. |
| **`FormatCode`** | Editors, Hydrators | Pure, unadorned source code ready for compilation or pasting. |
| **`FormatAST`** | Machine Tools, LLMs | Pretty-printed canonical JSON representation of the AST symbol and its metadata attributes. |
| **`FormatContract`** | Agent Reasoning | Concise API contract card displaying exported signature, parameters, return types, and active cross-boundary edges. |
| **`FormatMarkdown`** | Proposals, GitHub PRs | GitHub-flavored markdown with syntax blocks, diff badges, and causal lineage tables. |
| **`FormatPlain`** | Logs, Headless CI | Unformatted text output without terminal ANSI escape sequences. |

### 4.2 Cross-Domain Contract Topology (`cosm topology`)
When a language symbol interacts across runtime boundaries, the presentation layer must visualize the topological DAG:

```
[Frontend (TSX)] --CONSUMES_API--> [Backend Service (Go/Python/Rust)]
                                             │
                       ┌─────────────────────┴─────────────────────┐
                       ▼                                           ▼
          --QUERIES_TABLE--> [SQL Schema]           --DEPLOYS_TO--> [Terraform HCL]
```

### 4.3 Blast Radius Computation (`cosm blast-radius <node_id>`)
When an AST symbol is modified or deleted, Cosm traverses directional dependency edges to compute the impacted blast radius:
1. **Direct Dependencies**: Symbols in the same component calling this node.
2. **Cross-Boundary Consumers**: Remote client components consuming this API endpoint.
3. **Infrastructure Resources**: Cloud resources deploying or configuring this service.
4. **Severity Classification**:
   - `CRITICAL`: Breaking signature change or deleted endpoint with active callers.
   - `WARNING`: Deprecated parameters or non-breaking field additions.
   - `INFO`: Internal implementation change with unchanged public signature.

### 4.4 Stacked Proposal Review Cards (`pkg/review/`, `cosm proposal view`)
Universe proposals must present changes at the semantic AST symbol level rather than raw unstructured line diffs:
- **`SYMBOL_ADDED`**: Green addition card with signature and docstring.
- **`SYMBOL_MODIFIED`**: Side-by-side structural diff of changed fields/statements.
- **`SYMBOL_DELETED`**: Red deletion card with impact warning if dependent edges exist.

---

## 5. Pillar 3: Compilation, Hydration & Shipping Requirements

The shipping subsystem governs how AST Merkle DAGs are hydrated back into runnable source files, compiled by language toolchains, packaged, and executed in preview environments.

### 5.1 Structural AST Hydration Isomorphism
The codec hydrator (`pkg/materialize/hydrator.go` and `pkg/codecs/<lang>/hydrator.go`) must satisfy **roundtrip isomorphism**:

$$\text{Source Code} \xrightarrow{\text{Parse}} \text{AST DAG} \xrightarrow{\text{Hydrate}} \text{Reconstituted Source} \xrightarrow{\text{Parse}} \text{AST DAG'} \equiv \text{AST DAG}$$

#### Hydration Rules by Language:
- **Go**: Canonical formatting compliant with `gofmt` and standard package/import blocks.
- **HCL / Terraform**: Standard 2-space indentation compliant with `terraform fmt`.
- **TypeScript / React**: Clean module headers, imports, React functional components, and type definitions.
- **Python**: PEP 8 compliance, formatted imports, class definitions, and route decorators.
- **Rust**: Standard `rustfmt` block structure and module visibility.
- **Java**: Standard class structure, annotations, and package declarations.
- **SQL**: Upper-case keywords (`CREATE TABLE`, `PRIMARY KEY`), indented column definitions.
- **Protobuf**: Standard `syntax = "proto3";` syntax with ordered field indices.
- **Raw / Binary**: Exact byte-for-byte serialization from `ASTPayload` with preserved file permissions.

### 5.2 Ephemeral Staging & Sandbox Orchestration (`pkg/shipping/staging.go`)
Before execution, Cosm constructs an isolated staging sandbox in `.cosm/staging/<target_id>/`:
1. **Hydrate Virtual File System (VFS)**: Reconstitute all component symbols into concrete files on disk.
2. **Materialize Build Manifests**: Write `go.mod`, `package.json`, `Cargo.toml`, `requirements.txt`, `CMakeLists.txt`, or `main.tf`.
3. **Inject Environment Configuration**: Populate `.env` files derived from `BINDS_ENV` contract edges.

### 5.3 Polyglot Compiler & Bundler Pipeline (`pkg/shipping/compiler.go`)

```go
type TargetCompiler interface {
    Compile(workspace *StagingWorkspace, targetSpec *TargetSpec) (*BuildResult, error)
}
```

#### Dual-Execution Strategy:
1. **Native Toolchain Execution (When Installed)**:
   - **Go**: Executes `go build -o <out_path> ./...` with `go test` validation.
   - **TypeScript / React**: Bundles modules into browser-ready or Node-ready assets (`esbuild`, `tsc`, `bun`).
   - **Python**: Validates syntax and tests with **`uv` runner** (`uv run python -m py_compile`, `uv run pytest`), falling back to `python3 -m py_compile`.
   - **Terraform / HCL**: Executes `terraform fmt -check` and `terraform validate` using `TerraformRunner`.
   - **Rust**: Executes `cargo build --release` / `rustc`.
   - **Java**: Executes `javac -d bin` / `mvn compile` / `gradle build`.
   - **C++ / C**: Executes `clang++ -O3 -o bin/app` / `clang -O3 -o bin/app`.
   - **SQL**: Verifies DDL syntax and migration scripts via SQLite engine.
   - **Protobuf**: Executes `protoc --descriptor_set_out` for Proto3 schema definitions.
   - **GraphQL**: Validates GraphQL type definitions, queries, mutations, and resolvers.
   - **OpenAPI**: Validates OpenAPI 3.x schema specifications.
   - **Swift**: Executes `swift build -c release` / `swiftc`.
   - **Kotlin**: Executes `kotlinc -include-runtime -d bin/app.jar` / `gradle`.
   - **C#**: Executes `dotnet build -c Release`.
   - **Wasm**: Executes `wat2wasm`.
   - **Zig**: Executes `zig build-exe`.
   - **Ruby**: Executes `ruby -c` / `bundle exec rake`.
   - **PHP**: Executes `php -l`.
   - **Elixir**: Executes `mix compile`.
   - **Dockerfile**: Validates container directives for Apple container / local staging.
   - **Raw / Binary**: Packages binary assets with SHA-256 validation.

2. **Hermetic Pure-Go Fallback (Zero-Dependency Guarantee)**:
   - If the host machine lacks the native compiler (or in restricted CI sandbox environments), Cosm executes a pure-Go syntax validator and generates a synthetic executable payload.
   - Guarantees that agent harnesses and offline tests run reliably without crashing.

| Language | Primary Compiler / Toolchain | Validation Hook | Notes & Invariants |
| :--- | :--- | :--- | :--- |
| `go` | `go build` | `go test -v ./...` | Pure Go 1.22+, zero CGO SQLite WAL (`modernc.org/sqlite`) |
| `typescript` | `tsc`, `esbuild`, `bun` | `tsc --noEmit` | ESTree JSX/TSX component parsing & module bundling |
| `python` | **`uv run`** | **`uv run pytest`** | `uv` strictly preferred over raw system Python |
| `hcl` | `terraform` | `terraform fmt`, `terraform validate` | HCL2 block parsing & contract infrastructure validation |
| `rust` | `cargo` | `cargo test` | Zero-copy memory model, Axum/Actix route binding |
| `java` | `javac`, `mvn` | `mvn test` | Spring Boot routes, Java Records & classes |
| `cpp` / `c` | `clang++` / `clang` | CTest / make test | Clang-first native compilation, C++17/20 |
| `sql` | `sqlite3` / pure-Go SQL | DDL check | Table schema, foreign keys, index verification |
| `protobuf` | `protoc` | `buf lint` | Proto3 IDL message and RPC service contracts |
| `graphql` | `graphql` | schema linter | Types, queries, mutations, subscriptions |
| `openapi` | `spectral` | schema validate | OpenAPI v3 paths, parameters, schemas |
| `swift` | `swift build` | `swift test` | SwiftUI, Swift protocols, URLSession |
| `kotlin` | `kotlinc`, `gradle` | `gradle test` | Kotlin data classes, Ktor/Spring routes |
| `csharp` | `dotnet build` | `dotnet test` | ASP.NET Core controllers, EF entities |
| `wasm` | `wat2wasm` | wasm-validate | WebAssembly bytecode & WASI Preview 2 |
| `zig` | `zig build-exe` | `zig test` | Comptime structs, C interop |
| `ruby` | `ruby -c` | `bundle exec rspec`| Rails routes, models, classes |
| `php` | `php -l` | `composer test` | Laravel/Symfony route attributes |
| `elixir` | `mix compile` | `mix test` | Phoenix routes, GenServers |
| `dockerfile` | Apple container / local | container lint | Docker daemon not required on host |
| `raw` | Byte validator | SHA-256 digest | Lossless binary preservation |

### 5.4 Target Shipping & Local Preview (`pkg/target/`)
Once compiled, Cosm ships the runnable artifacts:
- **Local Service Sandbox**: Spawns supervised background processes with port allocation and log capturing.
- **Health Verification**: Polls `/healthz` or configured readiness endpoints.
- **Apple Container / Local Sandboxing**: Runs isolated staging without external Docker daemon dependencies.

---

## 6. Check-List for Adding a New Language Codec to Cosm

To add support for a new language (e.g. `Swift`, `Kotlin`, `C#`, `Zig`, or `WASM`):

1. [ ] **Define Schema Constant**: Add `Lang<Name> Language = "<name>"` to `pkg/core/schema.go`.
2. [ ] **Implement Parser**: Create `pkg/codecs/<name>/parser.go` implementing `ParseSource(path, content, env)`.
3. [ ] **Implement Symbol Structures**: Define typed symbol structs (e.g. `SwiftStructSymbol`, `KotlinClassSymbol`) with JSON marshaling.
4. [ ] **Implement Hydrator**: Create `pkg/codecs/<name>/hydrator.go` and register it in `pkg/materialize/hydrator.go`.
5. [ ] **Implement Cross-Boundary Extractor**: Detect route declarations, API calls, environment variable lookups in `pkg/core/crossboundary.go`.
6. [ ] **Add Compiler & Shipping Integration**: Add language build logic in `pkg/shipping/compiler.go`.
7. [ ] **Write Codec Tests**: Add comprehensive unit tests in `pkg/codecs/<name>/<name>_test.go` verifying parse $\rightarrow$ hydrate $\rightarrow$ re-parse roundtrip isomorphism.
8. [ ] **Update Reference Documentation**: Update `docs/reference/codecs.md` and this requirements document.
