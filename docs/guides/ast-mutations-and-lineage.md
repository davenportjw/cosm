# Surgical AST Mutations & Lineage Provenance

Cosm treats code as discrete, addressable symbol nodes rather than monolithic text files. This unlocks two capabilities impossible in traditional version control: **Surgical AST Mutation** and **Cryptographic Causal Lineage**.

---

## 1. Surgical AST Mutation

When an AI agent or developer edits a single function, struct, or cloud resource:
* **Traditional Git**: Re-hashes and stores the entire modified file blob.
* **Cosm**: Surgically re-serializes and updates **only the target AST symbol node**, deduplicating untouched symbols and preserving uncorrupted historical Merkle paths.

### Edit an AST Symbol In-Place
```bash
cosm symbol edit \
  --symbol-id "func_handle_billing" \
  --code "func HandleBilling(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }" \
  -u universe-main
```

### Audit Downstream Contract Impact
Before applying a surgical edit, evaluate the blast radius of modifying a symbol:
```bash
cosm symbol impact --symbol-id "func_handle_billing" -u universe-main
```
**Output:**
```
🎯 Blast-Radius & Contract Impact for 'func_handle_billing':
   • Risk Score:          0.25 / 1.00
   • Incoming Contracts:  2 (frontend/src/BillingView.tsx, services/gateway/router.go)
   • Outgoing Contracts:  1 (infra/pubsub.tf)
   • Status:              SAFE (No breaking signature changes)
```

---

## 2. Cryptographic Causal Lineage

Every AST node committed to Cosm contains a `LineageEnvelope` that establishes an unbroken cryptographic chain of custody:

$$\text{User Prompt} \longrightarrow \text{Session ID} \longrightarrow \text{Agent ID} \longrightarrow \text{LLM Model} \longrightarrow \text{Ed25519 Signature} \longrightarrow \text{Code Node}$$

### Trace Lineage from Production Code to Originating Prompt
```bash
cosm lineage node-4f2b90d81a9e
```
**Output:**
```
================================================================================
                        COSM: CAUSAL LINEAGE TRACE                              
================================================================================

📍 Target Node: node-4f2b90d81a9e (Language: Go, Type: FunctionDecl)
   Symbol:      services/auth.ValidateToken

📜 Lineage Pedigree:
   • User Prompt:     "Add JWT authentication middleware with RS256 token verification"
   • Session ID:      sess_993821aa-8371-4209
   • Orchestrator:    agent-orchestrator-alpha
   • Executing Agent: cosm-worker-agent-4
   • LLM Version:     gemini-3.7-flash
   • Timestamp:       2026-08-27T19:42:11Z
   • Ed25519 Signed:  VALID (Signature verified against agent public key)
```

### Audit Blast Radius by Agent or Model
Audit all code produced across all domains by a specific agent or model version:
```bash
cosm blast-radius cosm-worker-agent-4
```
**Output:**
```
🎯 Blast-Radius Audit for Agent 'cosm-worker-agent-4':
   • Total Nodes Created:    14
   • Cross-Domain Contracts: 8
   • Affected Services:      3 (auth, billing, gateway)
   • Risk Profile:           LOW (All automated tests and security audits passed)
```

---

## 3. How-To: Fixing Mistakes via Surgical AST Mutation (Zero History Rewriting)

When a developer or agent introduces a logic defect, traditional SCM requires amending commits (`git commit --amend`), running interactive rebases (`git rebase -i`), or creating messy "fixup" commits that pollute history logs.

In Cosm, surgical AST mutation amends the code structure directly at the AST node level without altering prior commit nodes or invalidating cryptographic lineage:

### Step 1: Locate the Target Symbol Node
```bash
# Resolve symbol by qualified path or name
cosm ast resolve "services/auth::ValidateToken"
```

### Step 2: Surgically Edit the AST Symbol
```bash
# Replace only the flawed function body while keeping signature and annotations intact
cosm ast edit \
  --op replace_function_body \
  --target "services/auth::ValidateToken" \
  --content "return token.Valid && !token.Expired()" \
  -u universe-main -w
```

### Step 3: Verify Downstream Blast Radius & Contracts
```bash
# Audit contract breakages across consuming services and infrastructure
cosm blast-radius func_validate_token -u universe-main
```

### Step 4: Commit the Surgical Revision
```bash
# Stamps new Merkle root and attaches signed LineageEnvelope
cosm commit -u universe-main \
  -i "Fix token expiration boundary validation" \
  -p "Patch ValidateToken expiration check"
```
