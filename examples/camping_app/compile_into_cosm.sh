#!/usr/bin/env bash
# ==============================================================================
# Compile Camping App into Cosm AST Merkle-DAG
#
# When downloading Cosm from Git, the repository includes the source files of the
# Camping App on disk, but the .cosm/ AST database is gitignored.
# This script compiles the source code into Cosm's AST Merkle-DAG and verifies
# the materialized working tree lens.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." 2>/dev/null && pwd || true)"

cd "${SCRIPT_DIR}"

echo "====================================================================="
echo "  🏕️  COMPILING CAMPING APP INTO COSM AST MERKLE-DAG"
echo "====================================================================="

# 1. Locate or build the 'cosm' binary
COSM_BIN=""
if command -v cosm >/dev/null 2>&1; then
    COSM_BIN="$(command -v cosm)"
elif [[ -n "${REPO_ROOT}" ]] && [[ -x "${REPO_ROOT}/bin/cosm" ]]; then
    COSM_BIN="${REPO_ROOT}/bin/cosm"
elif [[ -n "${REPO_ROOT}" ]] && [[ -f "${REPO_ROOT}/cmd/cosm/main.go" ]]; then
    echo "🔨 Building cosm CLI binary from source..."
    (cd "${REPO_ROOT}" && go build -o bin/cosm ./cmd/cosm)
    COSM_BIN="${REPO_ROOT}/bin/cosm"
fi

if [[ -z "${COSM_BIN}" ]] || [[ ! -x "${COSM_BIN}" ]]; then
    echo "❌ Error: 'cosm' binary not found."
    echo "   Please install it via 'go install github.com/cosmscm/cosm/cmd/cosm@latest'"
    echo "   or build it from the repository root: 'go build -o bin/cosm ./cmd/cosm'"
    exit 1
fi

echo "• Using Cosm CLI: ${COSM_BIN}"
echo "• Workspace Dir:  ${SCRIPT_DIR}"
echo ""

# 2. Check if already initialized
FORCE="${1:-}"
if [[ -d ".cosm" ]]; then
    if [[ "${FORCE}" == "--force" || "${FORCE}" == "-f" ]]; then
        echo "⚠️  Existing .cosm/ found; resetting workspace (--force requested)..."
        rm -rf .cosm
    else
        echo "ℹ️  .cosm/ already exists in this directory."
        echo "   Current status:"
        "${COSM_BIN}" status
        echo ""
        echo "💡 To re-compile from scratch, run: ./compile_into_cosm.sh --force"
        exit 0
    fi
fi

# 3. Initialize the Cosm repository
echo "1️⃣  Initializing Cosm repository on universe 'universe-main'..."
"${COSM_BIN}" init --universe universe-main

# 4. Stage all polyglot files into typed AST symbol nodes
echo ""
echo "2️⃣  Parsing polyglot files and staging AST symbol DAG..."
"${COSM_BIN}" add .

# 5. Commit the Merkle root with causal pedigree lineage
echo ""
echo "3️⃣  Committing AST Merkle root with causal lineage..."
"${COSM_BIN}" commit -u universe-main -i "Compile camping app into cosm"

# 6. Verify status
echo ""
echo "4️⃣  Verifying universe and working tree lens status..."
"${COSM_BIN}" status

echo ""
echo "====================================================================="
echo "  ✅ CAMPING APP SUCCESSFULLY COMPILED INTO COSM!"
echo "====================================================================="
echo "Next steps to explore Cosm:"
echo "  • View topology map:      cosm topology"
echo "  • Inspect AST hierarchy:  cosm ast tree"
echo "  • Ship preview sandbox:   cosm ship"
echo "  • Run Go tests:           go test -v ./..."
echo "  • Launch server:          go run ./cmd/server"
echo "====================================================================="
