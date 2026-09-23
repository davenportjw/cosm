#!/usr/bin/env bash
# ==============================================================================
# Deploy Alpine Escapes Camping App to Google Cloud Run
#
# Deploys the containerized application to Google Cloud Run (project: davenport-boutique,
# region: us-central1) and verifies live service availability via an HTTP probe.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${APP_DIR}/../.." 2>/dev/null && pwd || true)"

PROJECT_ID="${GCP_PROJECT:-davenport-boutique}"
REGION="${GCP_REGION:-us-central1}"
SERVICE_NAME="cosm-camping-app"

echo "====================================================================="
echo "  🚀 DEPLOYING CAMPING APP TO GOOGLE CLOUD RUN"
echo "====================================================================="
echo "• Project:  ${PROJECT_ID}"
echo "• Region:   ${REGION}"
echo "• Service:  ${SERVICE_NAME}"
echo "• App Root: ${APP_DIR}"
echo "====================================================================="

# 1. Check gcloud CLI
if ! command -v gcloud >/dev/null 2>&1; then
    echo "❌ Error: 'gcloud' CLI is required to deploy to Cloud Run."
    exit 1
fi

# 2. Check Cosm status if binary is available
COSM_BIN=""
if command -v cosm >/dev/null 2>&1; then
    COSM_BIN="$(command -v cosm)"
elif [[ -n "${REPO_ROOT}" ]] && [[ -x "${REPO_ROOT}/bin/cosm" ]]; then
    COSM_BIN="${REPO_ROOT}/bin/cosm"
fi

if [[ -n "${COSM_BIN}" && -d "${APP_DIR}/.cosm" ]]; then
    echo "🔍 Verifying Cosm AST Merkle-DAG status..."
    (cd "${APP_DIR}" && "${COSM_BIN}" status)
fi

# 3. Deploy to Cloud Run from source
echo ""
echo "📦 Building and deploying service to Cloud Run..."
cd "${APP_DIR}"
gcloud run deploy "${SERVICE_NAME}" \
    --source . \
    --project "${PROJECT_ID}" \
    --region "${REGION}" \
    --allow-unauthenticated \
    --quiet

# 4. Retrieve live service URL
SERVICE_URL="$(gcloud run services describe "${SERVICE_NAME}" --project "${PROJECT_ID}" --region "${REGION}" --format='value(status.url)')"

echo ""
echo "====================================================================="
echo "  ✅ DEPLOYMENT SUCCEEDED!"
echo "====================================================================="
echo "• Service URL: ${SERVICE_URL}"

# 5. Live HTTP Health Probe
echo "🔬 Probing live health endpoint..."
TOKEN="$(gcloud auth print-identity-token 2>/dev/null || true)"
if [[ -n "${TOKEN}" ]]; then
    HTTP_STATUS="$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer ${TOKEN}" "${SERVICE_URL}/health" || echo "000")"
else
    HTTP_STATUS="$(curl -s -o /dev/null -w "%{http_code}" "${SERVICE_URL}/health" || echo "000")"
fi

if [[ "${HTTP_STATUS}" == "200" ]]; then
    echo "✓ Live health probe passed: HTTP 200 OK (${SERVICE_URL}/health)"
else
    echo "⚠️  Health probe returned HTTP ${HTTP_STATUS}; check Cloud Run logs for details."
fi
echo "====================================================================="
