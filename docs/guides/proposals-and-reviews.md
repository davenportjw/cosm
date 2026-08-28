# AI-Native Proposals & Code Review

Traditional Git Pull Requests are limited to raw text line diffs (`+` and `-` lines). Reviewers are forced to manually parse syntax, infer cross-file impacts, and wait for external CI jobs.

In Cosm, PRs are elevated to **Universe Proposals**—rich, semantic packages that bundle:
1. **AST Symbol Deltas**: Surgical changes to functions, structs, and Terraform resources.
2. **Cross-Boundary Edge Diffs**: New or altered API routes and cloud bindings.
3. **Blast Radius & Downstream Risk**: Impact scores across services and infrastructure.
4. **Live Target Preview URLs**: Sub-second ephemeral sandboxes created by the shipping sidecar.
5. **Cryptographic Lineage**: Prompt-to-code provenance signed with Ed25519.

---

## 1. The Proposal Lifecycle

```
[Agent / Developer] 
       │ 
       ▼
1. cosm proposal create --source u/feature-billing --target universe-main
       │
       ▼
2. Cosm builds Automated Review Card:
   • AST Symbol Diffs
   • Cross-Domain Topology Changes
   • Blast-Radius Risk Score (0.00 to 1.00)
   • Ephemeral Target Preview URL (http://127.0.0.1:54912)
       │
       ▼
3. cosm proposal review <proposal_id> (Human or Autonomous AI Critic)
       │
       ▼
4. cosm proposal merge <proposal_id> (Collapsed into universe-main)
```

---

## 2. Proposal Commands

### Create a Proposal
```bash
cosm proposal create \
  --source u/feature-billing \
  --target universe-main \
  -i "Add Stripe billing webhook and Pub/Sub subscription"
```
**Output:**
```
🚀 Created Proposal: "prop-u/feature-billing-universe-main"
   Source: u/feature-billing -> Target: universe-main
   Changes: +3 components, -0 components
   Status: Ready for Agent Evaluation / Review
```

### List Active Proposals
```bash
cosm proposal list
```

### View Proposal Details & Topology Card
```bash
cosm proposal view prop-u/feature-billing-universe-main
```
**Output:**
```
══════════════════════════════════════════════════════════════════════
 🌌 PROPOSAL: u/feature-billing -> universe-main
 Intent:  Add Stripe billing webhook and Pub/Sub subscription
 Agent:   cosm-worker-agent | Signed: true | Build: PASS
══════════════════════════════════════════════════════════════════════

 📦 AST SYMBOL MODIFICATIONS:
   [+] services/billing/main.go.HandleStripeWebhook (RouteBinding: POST /webhook/stripe)
   [+] infra/pubsub.tf.google_pubsub_topic.billing (ResourceBlock)
   [+] frontend/src/BillingView.tsx.StripePaymentForm (ComponentDecl)

 🔗 CROSS-DOMAIN TOPOLOGY IMPACT:
   • services/billing ───[BINDS_ENV]───> infra/pubsub.tf
   • frontend/src ───[CONSUMES_API]───> services/billing (POST /webhook/stripe)

 🎯 BLAST RADIUS AUDIT:
   • Risk Score:          0.18 / 1.00 (LOW)
   • Affected Services:   1 (billing)
   • Breaking Contracts:  0

 🚀 EPHEMERAL PREVIEW:
   • Sandbox URL: http://127.0.0.1:51204
   • Health Status: PASS
```

### Perform Code Review
Review proposals manually or invoke the autonomous AI critic engine:
```bash
cosm proposal review prop-u/feature-billing-universe-main \
  --verdict APPROVE \
  --comment "Verified Stripe webhook signature verification and PubSub topic binding"
```

### Merge Proposal into Target Universe
```bash
cosm proposal merge prop-u/feature-billing-universe-main
```
**Output:**
```
🌟 Successfully merged proposal into 'universe-main'!
   New Head Merkle Root: 8c3a9f10d24e
```
