---
name: cosm-agent-harness
description: >-
  Execute, develop, and benchmark autonomous test agents and rater oracles
  using `cmd/cosm-agent-harness` and `test/agents/`. Covers scenario execution,
  LLM providers (Gemini 3.7 Flash and deterministic mock), weighted scorecards (0-100),
  and 6-dimension verification oracles.
---

# Cosm Autonomous Agent Harness & Rater Guide

Use this skill when developing, testing, or running the autonomous test agent harness and grading SCM interactions.

---

## Architecture of `test/agents/`

1. **LLM Provider Layer (`test/agents/llm/`)**:
   - `provider.go`: `LLMProvider` interface (`Generate(ctx, messages, tools, opts) (*Response, error)`).
   - `gemini.go`: Official Google Gemini API client configured for `gemini-3.7-flash`, structured tool calling, system prompts, and temperature controls. Requires `GEMINI_API_KEY`.
   - `mock.go`: Deterministic mock provider for hermetic CI tests that replays canned tool execution sequences without external network calls.

2. **Autonomous Execution Framework (`test/agents/framework/`)**:
   - `loop.go`: Autonomous multi-turn ReAct execution loop collecting structured traces, tool calls, and reflection steps.
   - `types.go`: Definitions for messages, tool definitions, tool results, and execution traces.

3. **Test Agent Orchestration (`test/agents/testagent/`)**:
   - `agent.go`: Test agent runner executing polyglot web app scenarios.
   - `scenarios.go`: Multi-tier application templates (FastAPI + React + TF, Go Gin + Vue + TF, Rust Axum + React + Postgres + TF).
   - `tools.go`: Strongly-typed tool execution handlers for `cosm_init`, `cosm_add`, `cosm_commit`, `cosm_universe_create`, `cosm_symbol_edit`, `cosm_ship`, `cosm_proposal_create`.

4. **Deep Verification Oracle & Rater (`test/agents/rater/`)**:
   - `oracle.go`: 6-dimension verification:
     1. AST Syntactic Validity across all languages.
     2. Toolchain & Compiler Checks (`go build`, `terraform validate`, `terraform fmt`).
     3. Cross-Boundary Contract Verification.
     4. Merkle Root Recalculation & SHA-256 Bit-Rot Detection.
     5. Causal Lineage & Ed25519 Cryptographic Attestation Verification.
   - `scorecard.go`: Weighted score model (0–100) and letter grades (A+, A, B, C, F).
   - `critic.go`: Multi-turn LLM code architecture critic using `gemini-3.7-flash`.
   - `reporter.go`: Formatted Markdown scorecard generator.

---

## How to Run the Agent Harness

### 1. Hermetic Unit & Integration Tests (Mock Provider)
```bash
go test -v ./test/agents/...
```

### 2. Building the Harness Binary
```bash
go build -o cosm-agent-harness ./cmd/cosm-agent-harness
```

### 3. Running Scenarios with Live Gemini API
Set `GEMINI_API_KEY` in your environment:
```bash
export GEMINI_API_KEY="your-api-key"

# Run a specific scenario
./cosm-agent-harness run --scenario polyglot-fastapi-react --model gemini-3.7-flash

# Run and rate a completed session
./cosm-agent-harness rate --session <session_id>

# Run full test suite
./cosm-agent-harness suite --all
```

---

## Best Practices
- **Isolation**: Keep all agent testing code inside `test/agents/` and `cmd/cosm-agent-harness/`.
- **Model Version**: Always default to `gemini-3.7-flash` for agent reasoning.
- **Hermetic Tests**: Ensure all CI unit tests in `test/agents/agents_test.go` use `mock.go` to guarantee zero flakiness.
- **Always Update Docs**: Keep all rater, harness, and scoring guides aligned with the latest runner flags and scorecard metrics.
- **Direct Style**: Write documentation directly without meta-commentary, emphasizing concrete evaluation metrics and commands.
