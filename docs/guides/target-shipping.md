# Target Shipping & Ephemeral Previews

Cosm bridges the gap between source control and deployment through its embedded **Target Shipping Sidecar**.

Instead of treating CI/CD as an external, slow batch pipeline, `cosm ship` performs instant local compilation, runs infrastructure validation hooks, packages composite multi-tier bundles, and boots live preview sandboxes directly from the AST Merkle DAG.

---

## 1. How Shipping Works

```
                        cosm ship -u <universe_id>
                                    │
    ┌───────────────────────────────┼──────────────────────────────┐
    │                               │                              │
    ▼                               ▼                              ▼
[1. Terraform Hooks]     [2. Compiler Engine]            [3. Ephemeral Sandbox]
• Runs terraform fmt     • Incremental Go build          • Allocates random port
• Runs terraform validate• Bundles frontend assets       • Boots composite server
• Enforces IAM policies  • Validates route bindings      • Runs HTTP health check
                                    │
                                    ▼
                         [Preview URL Delivered]
                        http://127.0.0.1:51204
```

---

## 2. Configuring Target Environments

Target environments are defined in `.cosm/config.json`:

```json
{
  "default_universe": "universe-main",
  "targets": {
    "local-preview": {
      "type": "ephemeral-sandbox",
      "staging_dir": ".tmp/staging",
      "runners": [
        {
          "language": "hcl",
          "command": "terraform validate"
        },
        {
          "language": "go",
          "command": "go build -o /tmp/backend main.go"
        }
      ],
      "health_endpoint": "/health",
      "timeout_seconds": 10
    },
    "cloud-staging": {
      "type": "gcp-cloud-run",
      "project_id": "my-prod-project",
      "region": "us-central1"
    }
  }
}
```

---

## 3. Shipping Commands

### Execute Full Shipping Workflow
```bash
# Ship default target profile (target:cosm for self-hosting or target:local-preview)
cosm ship -u universe-main

# Ship specific compilation target (e.g. target:cosm CLI binary)
cosm ship -u universe-main -t target:cosm

# Ship Cloud Run serverless container bundle
cosm ship -u universe-main -t target:cloud-run
```
**Output:**
```
🚀 Shipping Sidecar Execution Succeeded!
   • Package Size: 6999866 bytes (Artifact: 8a82a5d6c38da76b)
   • Artifact Path: dist/cosm.tar.gz
   • Preview URL:  http://127.0.0.1:51204
   • Health:       true
```
   • Launching ephemeral preview sandbox...  [PASS]

✨ Live Preview Ready!
   • Local URL:      http://127.0.0.1:54912
   • Health Check:   OK (200 Status)
   • Elapsed Time:   48ms
```

### Automated Terraform Hooks
Every time `cosm ship` or `cosm add` runs on HCL files:
1. Cosm formats all HCL blocks canonically (equivalent to `terraform fmt`).
2. Cosm invokes native schema validation (equivalent to `terraform validate`).
3. If an invalid cloud resource attribute or syntax error is detected, the AST node is flagged before reaching the durable Merkle store.
