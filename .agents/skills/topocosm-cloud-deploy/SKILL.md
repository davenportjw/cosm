---
name: topocosm-cloud-deploy
description: >-
  Deploy Topocosm Hub to Google Cloud (Cloud Run, GCS CAS, Memorystore for Redis, Cloud Pub/Sub, Secret Manager)
  and perform zero-downtime updates, canary traffic routing, rollback, and Day-2 cloud operations.
---

# Topocosm Hub: Google Cloud Deployment & Continuous Update Runbook

Use this skill when provisioning, deploying, updating, scaling, or troubleshooting **Topocosm Hub (`topocosm`)** on **Google Cloud Platform (GCP)**.

---

## 1. Cloud Architecture & Storage Topology

Topocosm Hub is the decentralized cloud distribution hub, CAS object store, and agent CRDT backplane for the Cosm AST Source Control System.

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                              Google Cloud Platform (GCP)                               │
│                                                                                        │
│   ┌────────────────────────────────────────────────────────────────────────────────┐   │
│   │ Google Cloud Run (`topocosm-hub`)                                               │   │
│   │ • Pure-Go zero-CGO binary (`PORT=8080`, `HOST=0.0.0.0`)                        │   │
│   │ • Stateless container instances (`min-instances=1`, `concurrency=80`)          │   │
│   │ • Endpoints: `/.well-known/cosm-agent.json`, `/api/v1/`, `/git/`, `/cosms`     │   │
│   └──────────────────────┬─────────────────────────────┬───────────────────────────┘   │
│                          │                             │                               │
│            (A) CAS Blobs │                             │ (B) Private RFC 1918          │
│            HTTPS / REST  │                             │ Serverless VPC Access         │
│                          ▼                             ▼                               │
│   ┌─────────────────────────────┐           ┌──────────────────────────────────────┐   │
│   │ Google Cloud Storage (GCS)  │           │ Google Cloud Memorystore for Redis   │   │
│   │ • Bucket: `<proj>-cas`      │           │ • Domain leases & blackboard CRDT    │   │
│   │ • Keys: `objects/xx/yy/hash`│           │ • Distributed lock engine            │   │
│   └─────────────────────────────┘           └──────────────────────────────────────┘   │
│                          │                                                             │
│                          ▼                                                             │
│   ┌────────────────────────────────────────────────────────────────────────────────┐   │
│   │ Google Cloud Pub/Sub (`topocosm-events`)                                        │   │
│   │ • Proposal gossip, cross-region mesh notifications, and agent swarm sync       │   │
│   └────────────────────────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

### Invariant Configuration Settings
- **Zero-CGO & Pure-Go**: Runs statically compiled Linux AMD64/ARM64 binary without external C library dependencies.
- **CAS Path Convention**: GCS blobs are stored content-addressed with two-level sharding:
  $$\text{Key} = \text{objects/} + \text{hash}[0:2] + \text{"/"} + \text{hash}[2:4] + \text{"/"} + \text{hash}$$
- **Zero-Docker Requirement**: Host environment does not have Docker installed; all container images must be built remotely via **Google Cloud Build** (`gcloud builds submit`).
- **Autoscaling Invariant**: `min-instances` must be $\ge 1$ in production to maintain warm CRDT memory state and prevent cold-start latency for real-time agent coordination.

---

## 2. Prerequisites & Environment Setup

```bash
# 1. Authenticate with Google Cloud
gcloud auth login
gcloud auth application-default login

# 2. Configure project and default compute region
export GCP_PROJECT_ID="${GCP_PROJECT_ID:-davenport-boutique}"
export GCP_REGION="us-central1"
export TOPOCOSM_SERVICE_NAME="topocosm-hub"

gcloud config set project "$GCP_PROJECT_ID"
gcloud config set compute/region "$GCP_REGION"

# 3. Enable required Google Cloud APIs
gcloud services enable \
  run.googleapis.com \
  storage.googleapis.com \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com \
  pubsub.googleapis.com \
  redis.googleapis.com \
  vpcaccess.googleapis.com \
  secretmanager.googleapis.com
```

---

## 3. Initial Deployment Workflows

You can deploy Topocosm to Google Cloud using either the automated script or the declarative Terraform recipe.

### Option A: Automated CLI Script (`deploy/scripts/deploy.sh`)

The script idempotently creates Artifact Registry, submits container compilation to Cloud Build, creates the GCS CAS bucket, configures IAM, and deploys the Cloud Run service:

```bash
# Execute deployment script
./deploy/scripts/deploy.sh
```

### Option B: Declarative Terraform Workflow (`deploy/terraform/`)

```bash
cd deploy/terraform

# 1. Format and validate
terraform fmt -check
terraform init -backend=false
terraform validate

# 2. Plan and apply
terraform init
terraform apply -var="project_id=$GCP_PROJECT_ID" -var="region=$GCP_REGION"
```

### Option C: Manual `gcloud` Execution

```bash
# 1. Create Artifact Registry repository
gcloud artifacts repositories create topocosm-repo \
  --repository-format docker \
  --location "$GCP_REGION" \
  --description "Topocosm Docker repository"

# 2. Build container via Cloud Build (Zero local Docker required)
IMAGE_URI="${GCP_REGION}-docker.pkg.dev/${GCP_PROJECT_ID}/topocosm-repo/topocosm-hub:latest"
gcloud builds submit . --tag "$IMAGE_URI"

# 3. Create CAS bucket
CAS_BUCKET="${GCP_PROJECT_ID}-topocosm-cas"
gcloud storage buckets create "gs://$CAS_BUCKET" \
  --location "$GCP_REGION" \
  --uniform-bucket-level-access

# 4. Create Pub/Sub topic
gcloud pubsub topics create topocosm-events

# 5. Deploy to Cloud Run
gcloud run deploy "$TOPOCOSM_SERVICE_NAME" \
  --image "$IMAGE_URI" \
  --region "$GCP_REGION" \
  --port 8080 \
  --allow-unauthenticated \
  --min-instances 1 \
  --max-instances 10 \
  --cpu 2 \
  --memory 2Gi \
  --set-env-vars "PORT=8080,HOST=0.0.0.0,DATA_DIR=/tmp/topocosm,GCP_PROJECT=$GCP_PROJECT_ID,GCS_BUCKET=$CAS_BUCKET,GCP_PUBSUB_TOPIC=topocosm-events,TOPOCOSM_REGION=$GCP_REGION"
```

---

## 4. Making Updates to Topocosm in the Cloud

All code, configuration, and infrastructure updates must follow zero-downtime rolling update and canary validation patterns.

### 4.1 Automated Zero-Downtime Rolling Update (`deploy/scripts/update.sh`)

The update script builds a new image tag, deploys it to Cloud Run tagged with `candidate` at 0% traffic, verifies the candidate health endpoints, gradually shifts canary traffic, and promotes to 100%:

```bash
./deploy/scripts/update.sh
```

### 4.2 Manual Canary Traffic Routing Workflow

```bash
# Step 1: Build updated image tag via Cloud Build
NEW_TAG="v-$(git rev-parse --short HEAD)-$(date +%s)"
NEW_IMAGE="${GCP_REGION}-docker.pkg.dev/${GCP_PROJECT_ID}/topocosm-repo/topocosm-hub:${NEW_TAG}"
gcloud builds submit . --tag "$NEW_IMAGE"

# Step 2: Deploy candidate revision with zero production traffic
gcloud run deploy "$TOPOCOSM_SERVICE_NAME" \
  --image "$NEW_IMAGE" \
  --region "$GCP_REGION" \
  --no-traffic \
  --tag candidate

# Step 3: Extract and test the candidate URL
CANDIDATE_URL=$(gcloud run services describe "$TOPOCOSM_SERVICE_NAME" \
  --region "$GCP_REGION" \
  --format "value(status.traffic[?tag=='candidate'].url)")

./deploy/scripts/health_check.sh "$CANDIDATE_URL"

# Step 4: Canary traffic shift (10% to candidate, 90% to current)
CURRENT_REV=$(gcloud run services describe "$TOPOCOSM_SERVICE_NAME" --region "$GCP_REGION" --format "value(status.traffic[0].revisionName)")
CANDIDATE_REV=$(gcloud run revisions list --service "$TOPOCOSM_SERVICE_NAME" --region "$GCP_REGION" --limit 1 --format "value(name)")

gcloud run services update-traffic "$TOPOCOSM_SERVICE_NAME" \
  --region "$GCP_REGION" \
  --to-revisions "${CANDIDATE_REV}=10,${CURRENT_REV}=90"

# Step 5: Promote candidate to 100% production traffic
gcloud run services update-traffic "$TOPOCOSM_SERVICE_NAME" \
  --region "$GCP_REGION" \
  --to-latest
```

### 4.3 Updating Environment Variables and Configuration

To update environment variables without rebuilding the container:

```bash
# Update GCS bucket or PubSub topic
gcloud run services update "$TOPOCOSM_SERVICE_NAME" \
  --region "$GCP_REGION" \
  --update-env-vars "GCS_BUCKET=new-cas-bucket,GCP_PUBSUB_TOPIC=new-events-topic"

# Update Memory or CPU allocations
gcloud run services update "$TOPOCOSM_SERVICE_NAME" \
  --region "$GCP_REGION" \
  --memory 4Gi \
  --cpu 4
```

### 4.4 Rotating Secrets (Redis Password / Ed25519 Signing Keys)

```bash
# Add new secret version in Secret Manager
echo -n "new-super-secret-key" | gcloud secrets versions add topocosm-secret-key --data-file=-

# Update Cloud Run to mount the latest secret version
gcloud run services update "$TOPOCOSM_SERVICE_NAME" \
  --region "$GCP_REGION" \
  --update-secrets "SECRET_KEY=topocosm-secret-key:latest"
```

---

## 5. Instant Rollback & Disaster Recovery

If an update exhibits regressions or errors in production:

### Option A: Automated Rollback Script
```bash
# Automatically detects previous revision and restores 100% traffic
./deploy/scripts/rollback.sh
```

### Option B: Targeted Revision Rollback
```bash
# 1. List available revisions
gcloud run revisions list \
  --service "$TOPOCOSM_SERVICE_NAME" \
  --region "$GCP_REGION" \
  --format "table(name,active,deployed,traffic_percent)"

# 2. Shift 100% traffic to specific revision
gcloud run services update-traffic "$TOPOCOSM_SERVICE_NAME" \
  --region "$GCP_REGION" \
  --to-revisions "topocosm-hub-00042-abc=100"

# 3. Verify health
./deploy/scripts/health_check.sh "$(gcloud run services describe "$TOPOCOSM_SERVICE_NAME" --region "$GCP_REGION" --format "value(status.url)")"
```

---

## 6. Day-2 Operations, Observability & Diagnostics

### Health Verification Probes
Run the health probe script against any Topocosm URL:
```bash
./deploy/scripts/health_check.sh https://<your-service-url>.a.run.app
```
Endpoints inspected:
1. `GET /.well-known/cosm-agent.json` (Agent discovery manifest)
2. `GET /api/v1/health` (Uptime, service status)
3. `GET /api/v1/stats` (Active proposals, CAS blob count, lease metrics)
4. `GET /cosms` (HTMX Web UI dashboard)

### Cloud Logging Queries

```bash
# 1. Stream all Topocosm Hub logs in real time
gcloud logging tail "resource.type=cloud_run_revision AND resource.labels.service_name=$TOPOCOSM_SERVICE_NAME"

# 2. Query application panics or HTTP 5xx errors in the last hour
gcloud logging read \
  "resource.type=cloud_run_revision AND resource.labels.service_name=$TOPOCOSM_SERVICE_NAME AND (severity>=ERROR OR httpRequest.status>=500)" \
  --limit 50 \
  --freshness 1h

# 3. Trace CRDT blackboard lease acquisition events
gcloud logging read \
  "resource.type=cloud_run_revision AND resource.labels.service_name=$TOPOCOSM_SERVICE_NAME AND textPayload=~\"lease\"" \
  --limit 20
```

### Cloud Monitoring & Alerting
Monitor these critical metrics in Cloud Monitoring:
- `run.googleapis.com/request_latencies` (p95 < 250ms for AST blob lookups)
- `run.googleapis.com/container/instance_count` (Track scaling bounds)
- `run.googleapis.com/container/cpu/utilizations` (< 70% threshold)
- `run.googleapis.com/container/memory/utilizations` (< 80% threshold)

---

## 7. Cosm Client Federation Verification

Once Topocosm is deployed and updated, configure the local Cosm CLI to publish and synchronize AST micro-universes:

```bash
# 1. Query remote discovery manifest
curl -s https://<service-url>/.well-known/cosm-agent.json | jq .

# 2. Publish local micro-universe to Cloud Hub
cosm publish https://<service-url>/my-org/my-cosm -u universe-main

# 3. Inspect repository in Web UI
open https://<service-url>/cosms
```
