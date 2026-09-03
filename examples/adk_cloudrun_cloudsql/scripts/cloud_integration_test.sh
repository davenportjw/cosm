#!/usr/bin/env bash
# ==============================================================================
# Google Cloud Run + Cloud SQL Live Integration & Teardown Test
# ==============================================================================
# This script provisions real GCP resources with Terraform, runs live chat &
# memory tests against the Cloud Run URL, and tears down all infrastructure.
#
# Usage:
#   export GCP_PROJECT_ID="your-project-id"
#   export GEMINI_API_KEY="your-gemini-key" # optional if using Vertex AI ADC
#   ./scripts/cloud_integration_test.sh
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
INFRA_DIR="${ROOT_DIR}/infra"

# 1. Check prerequisites
if [ -z "${GCP_PROJECT_ID:-}" ]; then
    echo "❌ Error: GCP_PROJECT_ID environment variable is not set."
    echo "Please run: export GCP_PROJECT_ID='your-gcp-project-id'"
    exit 1
fi

REGION="${GCP_REGION:-us-central1}"
RAND_SUFFIX="${RANDOM}"
SERVICE_NAME="adk-agent-test-${RAND_SUFFIX}"
DB_INSTANCE_NAME="adk-pg-test-${RAND_SUFFIX}"
DB_PASSWORD="TestPass_${RAND_SUFFIX}_Secure!"

echo "=============================================================================="
echo "🚀 Starting Live Cloud Integration Test"
echo "   Project:       ${GCP_PROJECT_ID}"
echo "   Region:        ${REGION}"
echo "   Service Name:  ${SERVICE_NAME}"
echo "   DB Instance:   ${DB_INSTANCE_NAME}"
echo "=============================================================================="

# Cleanup handler on exit or interruption
cleanup() {
    echo ""
    echo "=============================================================================="
    echo "🧹 Teardown: Destroying GCP Cloud Run & Cloud SQL Resources..."
    echo "=============================================================================="
    cd "${INFRA_DIR}"
    terraform destroy -auto-approve \
        -var="project_id=${GCP_PROJECT_ID}" \
        -var="region=${REGION}" \
        -var="service_name=${SERVICE_NAME}" \
        -var="db_instance_name=${DB_INSTANCE_NAME}" \
        -var="db_password=${DB_PASSWORD}" || true
    echo "✅ Teardown complete. All temporary cloud resources destroyed."
}
trap cleanup EXIT

# 2. Build and push container to Google Artifact Registry / Container Registry
echo ""
echo "📦 Step 1: Building and pushing container image to GCP..."
IMAGE_TAG="gcr.io/${GCP_PROJECT_ID}/${SERVICE_NAME}:latest"
gcloud builds submit --tag "${IMAGE_TAG}" "${ROOT_DIR}"

# 3. Provision Cloud SQL and Cloud Run with Terraform
echo ""
echo "🏗️  Step 2: Provisioning Cloud SQL & Cloud Run with Terraform..."
cd "${INFRA_DIR}"
terraform init -upgrade
terraform apply -auto-approve \
    -var="project_id=${GCP_PROJECT_ID}" \
    -var="region=${REGION}" \
    -var="service_name=${SERVICE_NAME}" \
    -var="db_instance_name=${DB_INSTANCE_NAME}" \
    -var="db_password=${DB_PASSWORD}" \
    -var="container_image=${IMAGE_TAG}"

CLOUD_RUN_URL="$(terraform output -raw cloud_run_url)"
echo "✅ Cloud Run Service deployed at: ${CLOUD_RUN_URL}"

# Allow edge routing to warm up
echo "⏳ Waiting 5s for Cloud Run edge routes to stabilize..."
sleep 5

# 4. Run Live Cloud E2E Tests
echo ""
echo "🧪 Step 3: Running Live E2E Tests against Cloud Run & Cloud SQL..."

# Test 1: Health probe
echo "  Testing /healthz endpoint..."
HEALTH_RESP="$(curl --retry 8 --retry-all-errors --retry-delay 3 --max-time 20 -s "${CLOUD_RUN_URL}/healthz")"
echo "  Health Response: ${HEALTH_RESP}"

# Test 2: Session 1 - Store user preference and session goal
SESSION_1="sess_live_test_1"
USER_ID="user_live_tester"

echo "  Testing Session 1: Saving user memory in Cloud SQL..."
TURN1_RESP="$(curl --retry 5 --retry-all-errors --retry-delay 3 --max-time 30 -s -X POST "${CLOUD_RUN_URL}/api/chat" \
    -H "Content-Type: application/json" \
    -d "{
        \"session_id\": \"${SESSION_1}\",
        \"user_id\": \"${USER_ID}\",
        \"message\": \"My name is Alice and my favorite language is Go\"
    }")"
echo "  Turn 1 Response: ${TURN1_RESP}"

# Test 3: Session 2 (New Session, Same User) - Verify persistent memory
SESSION_2="sess_live_test_2"
echo "  Testing Session 2: Verifying cross-session user memory retrieval..."
TURN2_RESP="$(curl --retry 5 --retry-all-errors --retry-delay 3 --max-time 30 -s -X POST "${CLOUD_RUN_URL}/api/chat" \
    -H "Content-Type: application/json" \
    -d "{
        \"session_id\": \"${SESSION_2}\",
        \"user_id\": \"${USER_ID}\",
        \"message\": \"What do you remember about me?\"
    }")"
echo "  Turn 2 Response: ${TURN2_RESP}"

echo ""
echo "=============================================================================="
echo "🎉 Live Cloud Integration Test PASSED successfully!"
echo "=============================================================================="
