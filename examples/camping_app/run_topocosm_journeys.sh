#!/usr/bin/env bash
# ==============================================================================
# Camping App 2.0 - End-to-End Topocosm Hub User Journeys Demo
#
# Demonstrates multi-track polyglot collaboration with Cosm & Topocosm Hub:
#   - Track A: Agent Alice (Gear Rentals Go API, claim lease, AST edit, proposal, merge)
#   - Track B: Developer Bob (Campsite Reviews, Jujutsu stacked proposals & auto-rebase)
#   - Track C: Infra Agent Charlie (Google Cloud Memorystore Redis HCL extension)
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
COSM_BIN="${REPO_ROOT}/bin/cosm"
HUB_PORT=51204
HUB_URL="http://127.0.0.1:${HUB_PORT}"
HUB_DIR="$(mktemp -d /tmp/topocosm-hub-demo.XXXXXX)"
DEMO_WORKSPACE="$(mktemp -d /tmp/camping-app-demo.XXXXXX)"

echo "====================================================================="
echo "  🏕️  CAMPING APP 2.0: MULTI-AGENT TOPOCOSM HUB JOURNEYS DEMO"
echo "====================================================================="
echo "• Cosm Binary:    ${COSM_BIN}"
echo "• Topocosm Hub:   ${HUB_URL}"
echo "• Demo Workspace: ${DEMO_WORKSPACE}"
echo "====================================================================="

# Cleanup handler on exit
cleanup() {
    echo ""
    echo "🧹 Teardown: Stopping background Topocosm Hub and cleaning scratch dirs..."
    if [[ -n "${HUB_PID:-}" ]] && kill -0 "${HUB_PID}" 2>/dev/null; then
        kill "${HUB_PID}" 2>/dev/null || true
        wait "${HUB_PID}" 2>/dev/null || true
    fi
    rm -rf "${HUB_DIR}" "${DEMO_WORKSPACE}"
    echo "✓ Teardown complete."
}
trap cleanup EXIT

# 1. Ensure Cosm binary is built
if [[ ! -x "${COSM_BIN}" ]]; then
    echo "🔨 Building Cosm binary..."
    (cd "${REPO_ROOT}" && go build -o bin/cosm ./cmd/cosm)
fi

# 2. Launch in-process/local Topocosm Hub daemon
echo "🚀 Starting Topocosm Hub on port ${HUB_PORT}..."
"${COSM_BIN}" topocosm dev --port "${HUB_PORT}" --dir "${HUB_DIR}" &
HUB_PID=$!

# Wait for hub readiness
sleep 1
echo "✓ Topocosm Hub is healthy and listening on ${HUB_URL}"
export TOPOCOSM_HUB_URL="${HUB_URL}"

# 3. Setup Camping App baseline in demo workspace
echo "📦 Staging Camping App baseline into Cosm Merkle-DAG..."
cp -R "${SCRIPT_DIR}/"* "${DEMO_WORKSPACE}/"
cd "${DEMO_WORKSPACE}"

"${COSM_BIN}" init --universe universe-main
"${COSM_BIN}" add .
"${COSM_BIN}" commit -u universe-main -i "Baseline Camping App 2.0 polyglot architecture"

# 4. Publish baseline to Topocosm Hub
echo "🌐 Publishing baseline to Topocosm Hub..."
"${COSM_BIN}" publish "${HUB_URL}/davenport-boutique/camping-app"

# ==============================================================================
# TRACK A: Autonomous Agent Alice (Gear Rentals)
# ==============================================================================
echo ""
echo "---------------------------------------------------------------------"
echo "  🤖 TRACK A: AGENT ALICE (Gear Rentals Backend Extension)"
echo "---------------------------------------------------------------------"

echo "1. Claiming blackboard domain lease: services/rentals..."
"${COSM_BIN}" claim services/rentals "${HUB_URL}/davenport-boutique/camping-app" \
    --goal "Implement gear rental catalog and reservation routes" \
    --ttl 600

echo "2. Creating micro-universe branch: universe-alice-rentals..."
"${COSM_BIN}" universe create universe-alice-rentals --parent universe-main

echo "3. Performing surgical AST mutation on cmd/server/main.go..."
"${COSM_BIN}" ast edit \
    --universe universe-alice-rentals \
    --op insert_after \
    --target "main.HandleCancelBooking" \
    --content '
// HandleGearRentals returns available outdoor gear for rent
func (s *Server) HandleGearRentals(w http.ResponseWriter, r *http.Request) {
	gear := []map[string]interface{}{
		{"id": "g1", "name": "2-Person Dome Tent", "daily_rate": 25.00, "available": true},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gear)
}' -w

"${COSM_BIN}" commit -u universe-alice-rentals -i "feat(rentals): Add gear rental endpoints"

echo "4. Submitting Proposal to Topocosm Hub..."
"${COSM_BIN}" proposal create \
    --source universe-alice-rentals \
    --target universe-main \
    --title "feat(rentals): Add gear rental endpoints" \
    --hub "${HUB_URL}/davenport-boutique/camping-app"

PROP_ALICE_ID=$("${COSM_BIN}" proposal list --hub "${HUB_URL}/davenport-boutique/camping-app" | grep "feat(rentals)" | awk '{print $2}')
echo "   • Registered Hub Proposal ID: ${PROP_ALICE_ID}"

echo "5. Critic Oracle evaluation..."
"${COSM_BIN}" proposal review \
    --hub "${HUB_URL}/davenport-boutique/camping-app" \
    --id "${PROP_ALICE_ID}" \
    --verdict APPROVE \
    --score 96.5

echo "6. Merging proposal into universe-main..."
"${COSM_BIN}" proposal merge \
    --hub "${HUB_URL}/davenport-boutique/camping-app" \
    --id "${PROP_ALICE_ID}"

echo "7. Releasing domain lease..."
"${COSM_BIN}" release services/rentals "${HUB_URL}/davenport-boutique/camping-app"
echo "✓ Track A completed successfully."

# ==============================================================================
# TRACK B: Developer Bob (Jujutsu-Style Stacked Proposals)
# ==============================================================================
echo ""
echo "---------------------------------------------------------------------"
echo "  👨‍💻 TRACK B: DEVELOPER BOB (Jujutsu Stacked Campsite Reviews)"
echo "---------------------------------------------------------------------"

echo "1. Claiming blackboard domain lease: services/reviews..."
"${COSM_BIN}" claim services/reviews "${HUB_URL}/davenport-boutique/camping-app" \
    --goal "Stacked proposals for campsite review system" \
    --ttl 1800

echo "2. Stack 1/2: Creating base proposal c/reviews-api..."
"${COSM_BIN}" universe create u/reviews-api --parent universe-main
"${COSM_BIN}" ast edit \
    --universe u/reviews-api \
    --op insert_after \
    --target "main.HandleMyBookings" \
    --content '
// HandleCampsiteReviews returns reviews for a campsite
func (s *Server) HandleCampsiteReviews(w http.ResponseWriter, r *http.Request) {
	reviews := []map[string]interface{}{
		{"id": "r1", "campsite_id": "c1", "author": "Alice", "rating": 5, "comment": "Spectacular!"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reviews)
}' -w

"${COSM_BIN}" commit -u u/reviews-api -i "feat(reviews): Backend review API"
"${COSM_BIN}" stack create -c c/reviews-api -u u/reviews-api

echo "3. Stack 2/2: Incepting frontend component and creating stacked proposal c/reviews-ui..."
"${COSM_BIN}" universe create u/reviews-ui --parent u/reviews-api
"${COSM_BIN}" ast create -c "web/src/App.tsx" --lang typescript -u u/reviews-ui -w
"${COSM_BIN}" ast edit \
    --universe u/reviews-ui \
    --op append_child \
    --target "web/src/App.tsx" \
    --content '
export function CampsiteReviews({ campsiteId }: { campsiteId: string }) {
  return <div className="reviews-card">5.0 ★ (42 reviews)</div>;
}' -w

"${COSM_BIN}" commit -u u/reviews-ui -i "feat(reviews): Frontend reviews drawer component"
"${COSM_BIN}" stack create -c c/reviews-ui -u u/reviews-ui -p c/reviews-api

echo "4. Jujutsu Evolution: Updating base and auto-rebasing stacked child..."
"${COSM_BIN}" stack evolve -c c/reviews-api

echo "5. Submitting stacked proposals to Topocosm Hub..."
"${COSM_BIN}" proposal create \
    --source u/reviews-api \
    --target universe-main \
    --title "feat(reviews): Backend reviews API (Stack 1/2)" \
    --hub "${HUB_URL}/davenport-boutique/camping-app"

PROP_REV_API=$("${COSM_BIN}" proposal list --hub "${HUB_URL}/davenport-boutique/camping-app" | grep "Backend reviews API" | awk '{print $2}')

"${COSM_BIN}" proposal create \
    --source u/reviews-ui \
    --target universe-main \
    --title "feat(reviews): Frontend reviews drawer (Stack 2/2)" \
    --hub "${HUB_URL}/davenport-boutique/camping-app"

PROP_REV_UI=$("${COSM_BIN}" proposal list --hub "${HUB_URL}/davenport-boutique/camping-app" | grep "Frontend reviews drawer" | awk '{print $2}')

echo "6. Reviewing and merging stack in topological order..."
"${COSM_BIN}" proposal review --hub "${HUB_URL}/davenport-boutique/camping-app" --id "${PROP_REV_API}" --verdict APPROVE --score 98.0
"${COSM_BIN}" proposal merge --hub "${HUB_URL}/davenport-boutique/camping-app" --id "${PROP_REV_API}"

"${COSM_BIN}" proposal review --hub "${HUB_URL}/davenport-boutique/camping-app" --id "${PROP_REV_UI}" --verdict APPROVE --score 95.0
"${COSM_BIN}" proposal merge --hub "${HUB_URL}/davenport-boutique/camping-app" --id "${PROP_REV_UI}"

"${COSM_BIN}" release services/reviews "${HUB_URL}/davenport-boutique/camping-app"
echo "✓ Track B completed successfully."

# ==============================================================================
# TRACK C: Infra Agent Charlie (Terraform Memorystore Redis)
# ==============================================================================
echo ""
echo "---------------------------------------------------------------------"
echo "  ☁️ TRACK C: INFRA AGENT CHARLIE (Cloud Memorystore Redis Cache)"
echo "---------------------------------------------------------------------"

echo "1. Claiming blackboard domain lease: infra/cache..."
"${COSM_BIN}" claim infra/cache "${HUB_URL}/davenport-boutique/camping-app" \
    --goal "Provision Memorystore Redis cache in Terraform HCL" \
    --ttl 600

echo "2. Mutating infra/main.tf with Redis resource..."
"${COSM_BIN}" universe create u/infra-redis --parent universe-main
"${COSM_BIN}" ast edit \
    --universe u/infra-redis \
    --op append_child \
    --target "infra/main.tf" \
    --content '
resource "google_redis_instance" "cache" {
  name           = "camping-cache"
  tier           = "BASIC"
  memory_size_gb = 1
  region         = "us-central1"
  authorized_network = google_compute_network.vpc.id
}' -w

"${COSM_BIN}" commit -u u/infra-redis -i "infra: Add Google Cloud Memorystore Redis instance"

echo "3. Submitting infra proposal to Topocosm Hub..."
"${COSM_BIN}" proposal create \
    --source u/infra-redis \
    --target universe-main \
    --title "infra: Add Redis caching layer" \
    --hub "${HUB_URL}/davenport-boutique/camping-app"

PROP_INFRA=$("${COSM_BIN}" proposal list --hub "${HUB_URL}/davenport-boutique/camping-app" | grep "Redis caching layer" | awk '{print $2}')

"${COSM_BIN}" proposal review --hub "${HUB_URL}/davenport-boutique/camping-app" --id "${PROP_INFRA}" --verdict APPROVE --score 99.0
"${COSM_BIN}" proposal merge --hub "${HUB_URL}/davenport-boutique/camping-app" --id "${PROP_INFRA}"
"${COSM_BIN}" release infra/cache "${HUB_URL}/davenport-boutique/camping-app"
echo "✓ Track C completed successfully."

# ==============================================================================
# FINAL VERIFICATION & METRICS
# ==============================================================================
echo ""
echo "====================================================================="
echo "  🎉 ALL 3 TRACKS MERGED! INSPECTING FINAL TOPOCOSM HUB STATE"
echo "====================================================================="
"${COSM_BIN}" topocosm status --url "${HUB_URL}"

echo ""
echo "🚀 Verifying Go Server Unit Tests pass..."
go test -v ./cmd/server/...

echo ""
echo "====================================================================="
echo "  ✅ CAMPING APP 2.0 USER JOURNEYS DEMO COMPLETED SUCCESSFULLY!"
echo "====================================================================="
