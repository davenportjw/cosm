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

### Single Operation Edit
```bash
# Replace function body
cosm ast edit --op replace_function_body --target LRUCache.get --content "return self.cache.get(key)"

# Add new endpoint after existing handler
cosm ast edit --op add_after --target read_root --content "@app.get('/health')\ndef health():\n    return {'status':'ok'}"

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
    "llm_version": "gemini-3.7-flash",
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
  -m "gemini-3.7-flash" \
  --prompt-tokens 1420 \
  --completion-tokens 312 \
  --reasoning-tokens 850 \
  --cost-usd 0.00184 \
  --latency-ms 640 \
  --trace-id "4bf92f3577b34da6a3ce929d0e0e4736" \
  --span-id "00f067aa0ba902b7"
```

For complete specification details, see [Agent Commit Contract Reference](file:///Users/jasondavenport/GitHub/cosm/docs/reference/agent-commit-contract.md).

