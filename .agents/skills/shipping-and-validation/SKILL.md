---
name: shipping-and-validation
description: >-
  Build, validate, and ship multi-tier polyglot projects to local preview sandboxes
  using `cosm ship` (`pkg/shipping/`, `pkg/target/`). Covers Terraform HCL formatting & validation,
  Go compilation, TypeScript bundling, Python testing with `uv`, and Apple container / local service infrastructure.
---

# Target Shipping & Toolchain Validation Guide

Use this skill when running pre-commit toolchain checks, validating multi-language projects, or executing target builds via `cosm ship`.

---

## Toolchain & Infrastructure Constraints

1. **Terraform / HCL**:
   - Always run `terraform fmt` and `terraform validate` on infrastructure directories.
   - Handled programmatically via `pkg/shipping/terraform_runner.go`.

2. **Python Runtimes**:
   - Always use `uv` for managing virtual environments, running tests, or installing dependencies:
     ```bash
     uv run pytest
     ```

3. **Local Container / Infrastructure Engine**:
   - Docker is NOT installed on the host machine.
   - Use Apple containers or local in-process services for running background databases and staging runners.

---

## Shipping Engine Workflows (`pkg/shipping/`)

1. **Target Specification (`targetspec.go`)**:
   - Declares build targets: `target:local-preview`, `target:cloud-run`, `target:static-web`.

2. **Ephemeral Staging Runner (`staging.go`)**:
   - Materializes the AST universe into an isolated in-memory or temporary directory (`/tmp/cosm-staging-*`).
   - Runs compilers against the staging directory without polluting the primary workspace tree.

3. **Incremental Compilers (`compiler.go`)**:
   - Compiles Go binaries (`go build`).
   - Bundles TypeScript/React frontends.
   - Validates Terraform plans (`terraform validate`).

4. **Running `cosm ship` from the CLI**:
   ```bash
   # Ship the active universe to a local preview sandbox
   cosm ship --target local-preview

   # Ship a specific micro-universe
   cosm ship --target local-preview --universe universe-feature-x
   ```

---

## Verification Commands

To verify target projection and shipping packages:
```bash
go test -v ./pkg/shipping/...
go test -v ./pkg/target/...
```

---

## Documentation Invariants
- **Always Update Docs**: Whenever shipping targets, compilers, or projection drift algorithms change, update `docs/guides/target-shipping.md` and `docs/reference/schema-and-storage.md`.
- **Direct Style**: Maintain a concise, direct, and authoritative style (concrete shell commands, drift tables, and target specs).
