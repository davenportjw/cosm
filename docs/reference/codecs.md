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
| **C++** | `.cpp`, `.cc`, `.h`, `.hpp` | C++ AST Codec | Functions, Classes, Structs, Methods |
| **SQL** | `.sql` | SQL DDL Parser | Tables, Columns, Foreign Keys, Indexes |
| **Protobuf** | `.proto` | Protocol Buffer Parser | gRPC Services, RPC Methods, Messages, Fields |
| **Raw / Non-AST** | `LICENSE`, `.md`, `.yaml`, `.yml`, `.json`, `.toml`, `Makefile`, configs, binaries | Zero-loss raw blob pipeline | `RawBlobNode` (verbatim payload preservation) |

---

## 2. Cross-Boundary Edge Types

Cosm automatically infers semantic edges connecting symbols across different languages:

| Edge Type | Source Node | Target Node | Inference Trigger |
| :--- | :--- | :--- | :--- |
| **`CONSUMES_API`** | Frontend TSX Component / Client SDK | Backend Go/Python/Rust HTTP Route | Matching URL path & HTTP method (e.g. `POST /api/v1/charge`) |
| **`DEPLOYS_TO`** | Backend Go / Python Service | Terraform Cloud Resource (`google_cloud_run_service`) | Service image/name reference in HCL resource block |
| **`BINDS_ENV`** | Backend Configuration Struct | Terraform Output / Cloud Secret | Environment variable binding name match |
| **`CALLS`** | Service A RPC Client | Service B Protobuf gRPC Definition | gRPC package & RPC method invocation |
| **`DEPENDS_ON`** | Any AST Node | Downstream AST Node | Explicit symbol dependency declaration |

---

## 3. Hydration & Reconstitution

Cosm reconstitutes immutable AST nodes back into perfectly formatted source files on demand:
* **Go**: Formatted with standard `gofmt` rules.
* **HCL**: Formatted with standard `terraform fmt` rules.
* **TypeScript / Python / Rust / SQL / Proto**: Reconstituted into syntactically valid source files matching language conventions.
* **Raw / Non-AST Artifacts (`README.md`, `LICENSE`, configs)**: Materialized verbatim byte-for-byte from `ASTPayload` preserving formatting, comments, and checksums.
