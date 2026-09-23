---
name: cosm-ast-surgeon
description: >-
  Execute high-precision declarative AST code mutations and surgical edits in Cosm repositories
  without full-file overwrites or transcription errors. Covers the 9 Geometric AST verbs, scoped
  symbol resolution, batch mutations, and Merkle root commits.
---

# Cosm AST Surgeon - Declarative AST Mutation Skill

Use this skill whenever an AI agent needs to modify code inside a Cosm repository. Rather than reading an entire file, modifying lines in memory, and writing back the full file (which causes high failure rates and destroys fine-grained Merkle deduplication), use Cosm's **Declarative AST Surgery Engine**.

---

## 1. Why Declarative AST Surgery?

1. **Zero Transcription / Hallucination Errors**: Target only the function body, method, or statement block to modify.
2. **Sub-Symbol Precision**: Preserves function signatures, docstrings, type annotations, and decorators automatically.
3. **Automatic Merkle Deduplication**: Unmodified sibling functions in the same file retain their exact SHA-256 hashes and are deduplicated with zero copy overhead.
4. **Instant Blast-Radius Auditing**: Every AST mutation automatically verifies incoming and outgoing cross-boundary contracts (API endpoints, DB models, Terraform infrastructure).

---

## 2. Supported AST Operations

| Operation | Target Syntax | Description | Example Payload |
|---|---|---|---|
| `create_component` | Component / Path | Incepts a new component and auto-parses symbols into DAG (zero-disk) | `package main...` or auto-scaffolded |
| `replace_function_body` | `Func` or `Class.method` | Replaces body while strictly preserving signature, decorators, and types | `return self.cache.get(key, None)` |
| `replace_function` | `Func` or `Class.method` | Completely replaces function signature and body | `def get(self, key: str) -> Optional[V]: ...` |
| `add_method` | `ClassName` | Appends a new method to a class/struct | `def evict(self) -> None:\n    self.cache.clear()` |
| `add_before` | Symbol / Target | Inserts a new symbol or statement before target | `func ValidateHeader(h http.Header) error { ... }` |
| `add_after` | Symbol / Target | Inserts a new symbol or statement after target | `func PostProcess() {}` |
| `delete` | Symbol / Target | Removes target symbol from parent component | `""` |
| `add_import` | Component / File | Ingests new package import or header | `import "sync/atomic"` |
| `replace_imports` | Component / File | Replaces all import declarations | `import (\n    "os"\n    "time"\n)` |
| `replace_global` | Global Var / Const | Replaces global variable/constant declaration | `const MaxRetries = 5` |

---

## 3. Targeting & Scoped Resolution Rules

Target symbols can be resolved using any of the following formats:
- **Bare Identifier**: `"ProcessPayment"` or `"read_root"`
- **Dotted Class Member**: `"BillingService.ProcessPayment"` or `"LRUCache.get"`
- **Scoped Component Path**: `"services/billing::BillingService.ProcessPayment"`
- **Exact Node ID**: 64-character SHA-256 hash or 8-character prefix

---

## 4. CLI Usage Examples

### Blank-Slate Component Inception (`cosm ast create`)
```bash
# Incept a new component with auto-scaffolded starter AST nodes directly into the DAG
cosm ast create -c services/billing --lang go -w

# Or incept a component with explicit source code
cosm ast create -c services/campsite -f cmd/server/main.go --code "package main..." -w
```

### Single Operation Edit (`cosm ast edit`)
```bash
# Replace function body
cosm ast edit --op replace_function_body --target LRUCache.get --content "return self.cache.get(key)" -w

# Add new endpoint after existing handler
cosm ast edit --op add_after --target read_root --content "@app.get('/health')\ndef health():\n    return {'status':'ok'}" -w

# Resolve symbol details
cosm ast resolve "services/auth::ValidateToken"
```

### Batch Mutation File (`batch.json`)
```json
{
  "universe_id": "universe-main",
  "operations": [
    {
      "operation": "add_import",
      "target": "backend/main.py",
      "content": "from fastapi import HTTPException"
    },
    {
      "operation": "replace_function_body",
      "target": "read_item",
      "content": "if item_id not in items:\n    raise HTTPException(status_code=404)\nreturn items[item_id]"
    }
  ]
}
```

Apply batch:
```bash
cosm ast edit --batch batch.json
```

---

## 5. Agent Tool Calling & REST API

### Tool: `cosm_ast_edit`
```json
{
  "universe_id": "universe-main",
  "operation": "replace_function_body",
  "target": "read_root",
  "content": "return {\"status\": \"healthy\", \"version\": \"2.0.0\"}",
  "intent": "Update root endpoint health response"
}
```

### HTTP REST: `POST /api/v1/ast/edit`
```http
POST /api/v1/ast/edit HTTP/1.1
Content-Type: application/json

{
  "universe_id": "universe-main",
  "operations": [
    {
      "operation": "replace_function_body",
      "target": "BillingService.ProcessPayment",
      "content": "w.WriteHeader(http.StatusOK)\nreturn nil"
    }
  ],
  "lineage": {
    "user_prompt": "Fix payment return status",
    "executing_agent_id": "cosm-ast-surgeon",
    "llm_version": "gemini-3.8-flash",
    "tokens": {
      "prompt_tokens": 820,
      "completion_tokens": 140,
      "reasoning_tokens": 260,
      "cost_usd": 0.00062,
      "latency_ms": 320
    },
    "trace": {
      "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
      "span_id": "00f067aa0ba902b7"
    }
  }
}
```

---

## 6. Commit Contract & Token Telemetry Guidelines

When finalizing AST mutations and committing the micro-universe workspace, autonomous agents MUST supply complete causal lineage and token telemetry:

```bash
cosm commit \
  -u universe-feat-auth \
  -i "feat: update token verification" \
  -p "Implement RS256 token verification" \
  --session-id "sess-4491-a8b2" \
  -a cosm-ast-surgeon \
  -m "gemini-3.8-flash" \
  --prompt-tokens 1420 \
  --completion-tokens 312 \
  --reasoning-tokens 850 \
  --cost-usd 0.00184 \
  --latency-ms 640 \
  --trace-id "4bf92f3577b34da6a3ce929d0e0e4736" \
  --span-id "00f067aa0ba902b7"
```

For complete specification details, see [Agent Commit Contract Reference](../../../docs/reference/agent-commit-contract.md).

---

## 7. Operating Inside VS Code & IDE Workspaces

When operating in an interactive developer environment (such as VS Code, Cursor, or JetBrains):

1. **Automatic Disk Synchronization**:
   - `cosm ast edit` automatically updates the enclosing source file on disk using `--write-disk` (which is enabled by default, `-w`).
   - This ensures open VS Code editor buffers, Language Server Protocols (LSP - `gopls`, `tsserver`, `pyright`), linters, and test runners instantly observe mutations without requiring manual export.
   - If mutating in headless/DAG-only mode (e.g. background batch processing), pass `--write-disk=false`.

2. **Resolving Targets Before Editing**:
   - Always run `cosm ast resolve <target>` to verify the symbol ID and component name before executing an edit.
   - You can specify targets using symbol names (`ValidateToken`), scoped identifiers (`services/auth::ValidateToken`), or hex symbol IDs.

3. **Git Shim Visibility**:
   - `cosm git diff` and `cosm git status` project the synthetic Git repository, allowing VS Code's Source Control view (Git pane) to display live AST changes as normal file diffs.
   - To manually re-hydrate the workspace from the active universe head at any time:
     ```bash
     cosm export -u universe-main -d .
     ```

---

## 8. Multi-Agent Swarm Discipline: Pre-Mutation Leases & Contract Blast Radius

When operating as an autonomous agent within a multi-agent swarm:

1. **Acquire Blackboard Leases First (Hierarchical URIs)**:
   - Before mutating shared components or interface contracts, claim the target domain, component, or specific symbol:
     ```bash
     # Symbol-level claim (allows other agents to edit sibling functions simultaneously)
     cosm claim services/billing::ProcessPayment --granularity symbol --ttl 600 --agent "$AGENT_DID" --goal "Refactoring Stripe webhook"

     # Or component-level claim
     cosm claim services/billing --granularity component --ttl 600 --agent "$AGENT_DID"
     ```
   - If the claim returns conflict (`409`), yield and re-queue rather than attempting concurrent edits on overlapping paths.
   - Release the lease immediately upon proposal submission or failure (`cosm release services/billing::ProcessPayment`).

2. **Audit Cross-Boundary Blast Radius**:
   - Before modifying any function signature or interface, inspect connected edges:
     ```bash
     cosm blast-radius <symbol_id>
     ```
   - Check for incoming `CONSUMES_API` or `CALLS` dependencies. Modifying a signature without updating consumer contracts will trigger a `ROUTE_CONTRACT_BROKEN` or `API_CONTRACT_MISMATCH` semantic conflict.

3. **Always Mutate in an Isolated Micro-Universe**:
   - Never apply AST edits directly to `universe-main`. Fork a micro-universe:
     ```bash
     cosm universe create u/agent-task -p universe-main
     ```
   - All AST mutations, disk synchronization, and target preview tests occur within this isolated frontier.

4. **Resolving AST Conflicts**:
   - If an edge or symbol collision occurs during proposal merge, Cosm writes an `ASTConflictNode` to the DAG. Resolve it surgically:
     ```bash
     cosm ast resolve --conflict <conflict_id> --choose-version 0
     ```

---

## 9. Post-Mutation Verification & Lifecycle Protocol

To avoid diagnostic loops or false alerts when verifying mutations:

1. **Immediate Test Execution (Compiler/Test Parity)**:
   - With `--write-disk` (default `true` / `-w`), `cosm ast edit` immediately updates enclosing workspace files on disk.
   - Run unit tests or build commands immediately (e.g. `go test -v ./...`, `pytest`, `npm test`) without manual export steps.

2. **AST Verification via Explicit Symbol Node ID**:
   - `cosm ast edit` outputs the exact new Node ID of every modified symbol:
     ```text
     • Modified Symbols: 1
       └─ 5f9faf5d034b main.(Server).HandleHealth (FunctionDecl) in cmd/server/main.go
     ```
   - To inspect the reconstituted symbol in the AST DAG, always query using the **exact Node ID**:
     ```bash
     cosm view 5f9faf5d034b
     ```
   - Avoid querying by ambiguous human symbol strings immediately post-mutation unless specifying the micro-universe explicitly (`cosm view <symbol> -u <universe_id>`).

3. **Solidifying Branch Frontier with Cosm Commit**:
   - `cosm ast edit` applies and persists the mutation into the micro-universe manifest.
   - To seal the frontier and clear "File drift detected" in `cosm status`, execute a commit:
     ```bash
     cosm commit -u <universe_id> -i "<intent>"
     ```



