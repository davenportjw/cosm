---
name: polyglot-codecs-guide
description: >-
  Implement, extend, and test language AST codecs, hydrators, and cross-boundary contract
  inferencers for Go, Python, TypeScript, HCL/Terraform, Rust, Java, SQL, Protobuf, and C++.
---

# Polyglot AST Codecs & Hydrators Guide

Use this skill when adding support for a new programming language or modifying existing AST codecs under `pkg/codecs/` and `pkg/materialize/`.

---

## Supported Codec Packages (`pkg/codecs/`)

Language       | Codec Path               | AST Library / Strategy                | Key Extracted Entities
:------------- | :----------------------- | :------------------------------------ | :---------------------
**Go**         | `pkg/codecs/golang`      | `go/parser`, `go/ast`, `go/token`     | Structs, Functions, Methods, HTTP Routes
**HCL / TF**   | `pkg/codecs/hcl`         | `github.com/hashicorp/hcl/v2`         | Resource Blocks, Modules, Variables, Outputs
**TypeScript** | `pkg/codecs/typescript`  | TS/TSX & ESTree AST Parser            | React Components, Functions, API Fetch Calls
**Python**     | `pkg/codecs/python`      | Python AST Parser                     | Classes, Functions, Decorators, FastAPI Routes
**Rust**       | `pkg/codecs/rust`        | Rust Syntax Codec                     | Structs, Enums, Functions, Axum Handlers
**Java**       | `pkg/codecs/java`        | Java Syntax Codec                     | Classes, Interfaces, Spring Endpoints
**SQL**        | `pkg/codecs/sql`         | SQL DDL/DML Codec                     | Tables, Columns, Indices, Migrations
**Protobuf**   | `pkg/codecs/protobuf`    | Proto3 IDL Codec                      | Messages, Services, RPC Methods
**C++**        | `pkg/codecs/cpp`         | C/C++ Header/Source Codec             | Classes, Structs, Functions, Namespaces

---

## Implementation Requirements for a Codec

Every language codec must provide:

1. **Parser (`parser.go`)**:
   - `ParseSource(path string, content []byte, env core.LineageEnvelope)`: Returns structured AST symbols.
   - Extracts symbol identifiers, type signatures, and local dependency tokens.
   - Handles syntax errors gracefully without crashing the engine.

2. **Hydrator (`hydrator.go` or in `pkg/materialize/hydrator.go`)**:
   - Converts serialized `ASTSymbolNode` binary payloads back into clean, canonical source text.
   - Must satisfy structural AST roundtrip isomorphism:
     $$\text{Source} \xrightarrow{\text{Parse}} \text{AST} \xrightarrow{\text{Hydrate}} \text{Source'} \xrightarrow{\text{Parse}} \text{AST' == AST}$$

3. **Cross-Boundary Edge Discovery (`pkg/core/crossboundary.go`)**:
   - `CONSUMES_API`: Links client fetch/HTTP calls to backend route declarations.
   - `DEPLOYS_TO`: Links backend service definitions to Terraform container/compute resources.
   - `BINDS_ENV`: Links application environment variable lookups to Terraform `env` declarations.

4. **Lossless Raw Fallback**:
   - Non-AST assets (markdown, YAML configs, images, binaries) are wrapped in `core.CompService` as `RawBlobNode`s to guarantee zero data loss.

---

## Testing Codecs

Run codec-specific unit tests:
```bash
go test -v ./pkg/codecs/...
go test -v ./pkg/materialize/...
```

---

## Documentation Invariants
- **Always Update Docs**: Whenever a new language codec or contract extractor is added or changed, update `docs/reference/codecs.md` immediately.
- **Direct Style**: Keep documentation strictly technical, direct, and concise (table of entities, parsing rules, and hydration AST mappings without filler).
