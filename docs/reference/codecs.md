# Language Codecs & Contract Inference Reference

Cosm features native, high-performance AST codecs across polyglot languages and infrastructure frameworks.

---

## 1. Supported Languages & AST Codecs

| Language | File Extensions | Codec Implementation | Extracted AST Symbols |
| :--- | :--- | :--- | :--- |
| **Go** | `.go` | `go/parser`, `go/ast`, `go/types` | Functions, Structs, Interfaces, HTTP Route Bindings |
| **Terraform HCL** | `.tf`, `.hcl` | `hashicorp/hcl/v2` | Resource Blocks, Data Blocks, Variables, Outputs |
| **TypeScript / React** | `.ts`, `.tsx`, `.js`, `.jsx` | Tree-sitter / ESTree AST | React Components, Functions, API fetch/axios calls |
| **Python** | `.py` | Python AST Parser | FastAPI/Flask Routes, Class Definitions, Functions |
| **Rust** | `.rs` | Rust AST Codec | Functions, Structs, Enums, Axum/Actix Route Handlers |
| **Java** | `.java` | Java AST Parser | Spring Boot `@RestController`, JPA Entities, Classes |
| **Swift** | `.swift` | Pure-Go Swift AST Codec | Structs, Classes, Protocols, Extensions, Enums, SwiftUI Views (`View`), URLSession API Calls |
| **Kotlin** | `.kt`, `.kts` | Pure-Go Kotlin AST Codec | Classes, Data Classes, Interfaces, Objects, Jetpack Compose (`@Composable`), Spring/Ktor Routes & Client Calls |
| **C++** | `.cpp`, `.cc`, `.h`, `.hpp` | C++ AST Codec | Functions, Classes, Structs, Methods |
| **WebAssembly / WASM** | `.wat`, `.wasm`, `.wit` | Pure-Go WASM/WAT/WIT Codec | Functions, Types, Imports, Exports, Memories, Tables, Globals, WIT Interfaces & Worlds |
| **Zig** | `.zig` | Pure-Go Zig AST Codec | Structs (packed/extern), Enums, Unions, Functions (`pub`, `export`, `inline`, `callconv`), Error Sets, Comptime, Tests |
| **SQL** | `.sql` | SQL DDL Parser | Tables, Columns, Foreign Keys, Indexes |
| **C#** | `.cs` | Pure-Go C# AST Codec (`pkg/codecs/csharp/`) | Classes, Structs, Interfaces, Records, Enums, ASP.NET Core (`[HttpGet]`, `[HttpPost]`, Minimal API `.MapGet()`), EF Core (`DbContext`, `[Table]`) |
| **GraphQL** | `.graphql`, `.gql` | Pure-Go GraphQL SDL Codec (`pkg/codecs/graphql/`) | Types, Interfaces, Unions, Enums, Inputs, Scalars, Directives, Operations (`Query`, `Mutation`, `Subscription`), Apollo Federation (`@key`, `@extends`, `@external`) |
| **Ruby** | `.rb` | Pure-Go Ruby AST Codec (`pkg/codecs/ruby/`) | Classes, Modules, Methods (`def`, `self.`), Rails/Sinatra Routes (`get/post`), ActiveRecord Associations (`has_many`, `belongs_to`) |
| **PHP** | `.php` | Pure-Go PHP AST Codec (`pkg/codecs/php/`) | Classes, Interfaces, Enums, Traits, Functions, Methods, Symfony/Laravel Routes (`#[Route(...)]`, `Route::get`) |
| **Elixir** | `.ex`, `.exs` | Pure-Go Elixir AST Codec (`pkg/codecs/elixir/`) | Modules (`defmodule`), Functions & Macros (`def`, `defp`, `defmacro` with guards `when`), Ecto Schemas, Phoenix Routes (`scope`, `resources`, `get`) |
| **Dockerfile** | `Dockerfile`, `Containerfile` | Pure-Go Dockerfile AST Codec (`pkg/codecs/dockerfile/`) | `BaseImage` (`FROM`), `PortExpose` (`EXPOSE`), `EnvBinding` (`ENV`), `EntryPoint` (`ENTRYPOINT`/`CMD`), `CopyInstruction` (`COPY`/`ADD`), `WorkdirInstruction` (`WORKDIR`) |
| **Raw / Non-AST** | `LICENSE`, `.md`, `.yaml`, `.yml`, `.json`, `.toml`, `Makefile`, configs, binaries | Zero-loss raw blob pipeline | `RawBlobNode` (verbatim payload preservation) |

---

## 2. Cross-Boundary Edge Types

Cosm automatically infers semantic edges connecting symbols across different languages:

| Edge Type | Source Node | Target Node | Inference Trigger |
| :--- | :--- | :--- | :--- |
| **`CONSUMES_API`** | Frontend TSX / Swift URLSession / Kotlin Client | Backend Go/Python/Rust/Kotlin/C#/Dockerfile HTTP Route/Port | Matching URL path & HTTP method (e.g. `POST /api/v1/charge`) or Dockerfile `EXPOSE` port |
| **`QUERIES_TABLE`** | Backend JPA / EF Core / SQLx / GORM Entities | SQL DDL `CreateTableStatement` | Matching table name or explicit mapping annotation |
| **`DEPLOYS_TO`** | Backend Service / Dockerfile Base Image | Terraform Cloud Resource (`google_cloud_run_service`) | Service image/name reference in HCL resource block or Dockerfile `FROM` stage |
| **`BINDS_ENV`** | Backend Configuration Struct / Dockerfile `ENV` | Terraform Output / Cloud Secret | Environment variable binding name match |
| **`CALLS`** | Service A RPC Client / GraphQL Resolver | Service B Protobuf gRPC Definition / GraphQL Schema | gRPC package & RPC method invocation / schema type reference |
| **`DEPENDS_ON`** | Any AST Node | Downstream AST Node | Explicit symbol dependency declaration |

---

## 3. Hydration & Reconstitution

Cosm reconstitutes immutable AST nodes back into perfectly formatted source files on demand:
* **Go**: Formatted with standard `gofmt` rules.
* **HCL**: Formatted with standard `terraform fmt` rules.
* **C#**: Reconstituted into idiomatic C# source preserving classes, structs, positional records, interfaces, enums, ASP.NET Core attributes, Minimal APIs, and EF Core mappings.
* **GraphQL**: Reconstituted into canonical GraphQL SDL schema text preserving descriptions (`"""..."""`), types, fields, arguments, directives, and Apollo Federation annotations.
* **Swift**: Reconstituted into canonical Swift source preserving visibility, docstrings, property wrappers, SwiftUI bodies, and protocol conformance.
* **Kotlin**: Reconstituted into idiomatic Kotlin source preserving constructors, `@Composable` functions, Spring annotations, and Ktor route definitions.
* **WebAssembly**: Reconstituted into standard `.wat` S-expression format, WIT interface definitions, or compiled to binary `.wasm` format.
* **Zig**: Reconstituted into idiomatic Zig source preserving structs, enums, unions, comptime declarations, error sets, and C-interop.
* **Ruby**: Reconstituted into idiomatic Ruby source preserving classes, modules, method definitions, Rails/Sinatra routes, and ActiveRecord associations.
* **PHP**: Reconstituted into clean PHP 8+ source preserving classes, interfaces, backed enums, traits, methods, Symfony `#[Route]` attributes, and Laravel Route facade calls.
* **Elixir**: Reconstituted into canonical Elixir source preserving module definitions, `@moduledoc`, `@doc`, `@spec`, pattern-matched functions with default arguments and guards (`when`), Ecto schemas, and Phoenix routes.
* **Dockerfile**: Reconstituted into canonical multi-stage Dockerfile / Containerfile format preserving build stages, base images, port exposures, env variables, copy commands, and entrypoints.
* **TypeScript / Python / Rust / Java / C++ / SQL / Proto**: Reconstituted into syntactically valid source files matching language conventions.
* **Raw / Non-AST Artifacts (`README.md`, `LICENSE`, configs)**: Materialized verbatim byte-for-byte from `ASTPayload` preserving formatting, comments, and checksums.

---

## 4. Codec & Binary Requirements

For full implementation requirements, storage contracts, presentation formats, and compiler/shipping pipelines, see the [Language Codec & Binary Asset Specification](file:///Users/jasondavenport/GitHub/cosm/docs/reference/language-and-binary-requirements.md).


