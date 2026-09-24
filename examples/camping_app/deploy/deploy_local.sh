#!/usr/bin/env bash
# ==============================================================================
# Deploy Alpine Escapes Camping App Locally (Zero-Cloud / Pure-Go)
#
# Launches the Customer Zero Camping App locally on a designated port using the
# pure-Go in-memory reservation engine (or PostgreSQL if DATABASE_URL is set).
# Features daemon/foreground execution, PID tracking, automated HTTP health probing,
# and AST Merkle-DAG verification.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${APP_DIR}/../.." 2>/dev/null && pwd || true)"

PID_FILE="${SCRIPT_DIR}/camping_app.pid"
LOG_FILE="${SCRIPT_DIR}/camping_app.log"
BIN_FILE="${APP_DIR}/bin/server"

PORT="${PORT:-8080}"
DATABASE_URL="${DATABASE_URL:-}"
FOREGROUND=false
COSM_VERIFY=false
VERB=""

show_help() {
    cat <<EOF
Usage: $0 [VERB] [OPTIONS]

Lifecycle Verbs:
  start          Compile binary (if needed), launch daemon, and probe health (default)
  stop           Gracefully terminate running daemon via SIGTERM (SIGKILL fallback)
  restart        Execute stop followed by start
  status         Display process status, PID, port, and health check output

Options:
  -p, --port <port>       Port to bind (default: \$PORT or 8080)
  -f, --foreground        Run attached in foreground instead of background daemon
  --db <DATABASE_URL>     PostgreSQL database URL (default: in-memory store)
  --cosm-verify           Verify Cosm AST Merkle-DAG status before starting
  -h, --help              Show this help message and exit

Examples:
  $0 start --port 8080
  $0 stop
  $0 restart --port 8080
  $0 status --port 8080
  $0 --foreground --cosm-verify
EOF
}

# Parse CLI arguments and flags
while [[ $# -gt 0 ]]; do
    case "$1" in
        -p|--port)
            if [[ $# -lt 2 ]]; then
                echo "❌ Error: --port requires a port argument." >&2
                exit 1
            fi
            PORT="$2"
            shift 2
            ;;
        --port=*)
            PORT="${1#*=}"
            shift
            ;;
        -f|--foreground)
            FOREGROUND=true
            shift
            ;;
        --db)
            if [[ $# -lt 2 ]]; then
                echo "❌ Error: --db requires a DATABASE_URL argument." >&2
                exit 1
            fi
            DATABASE_URL="$2"
            shift 2
            ;;
        --db=*)
            DATABASE_URL="${1#*=}"
            shift
            ;;
        --cosm-verify)
            COSM_VERIFY=true
            shift
            ;;
        start|stop|restart|status)
            if [[ -n "${VERB}" ]]; then
                echo "❌ Error: Multiple lifecycle verbs specified ('${VERB}' and '$1')." >&2
                exit 1
            fi
            VERB="$1"
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo "❌ Error: Unknown argument '$1'." >&2
            echo "   Run '$0 --help' for usage." >&2
            exit 1
            ;;
    esac
done

VERB="${VERB:-start}"

print_banner() {
    local title="$1"
    echo "====================================================================="
    echo "  ${title}"
    echo "====================================================================="
    echo "• App Root:     ${APP_DIR}"
    echo "• Port:         ${PORT}"
    if [[ -n "${DATABASE_URL}" ]]; then
        echo "• Storage:      PostgreSQL (${DATABASE_URL})"
    else
        echo "• Storage:      Pure-Go In-Memory Store (Zero-Cloud)"
    fi
    echo "• Verb:         ${VERB}"
    echo "====================================================================="
}

verify_cosm() {
    if [[ "${COSM_VERIFY}" == "true" ]]; then
        if [[ -d "${APP_DIR}/.cosm" ]]; then
            local cosm_bin=""
            if command -v cosm >/dev/null 2>&1; then
                cosm_bin="$(command -v cosm)"
            elif [[ -n "${REPO_ROOT}" ]] && [[ -x "${REPO_ROOT}/bin/cosm" ]]; then
                cosm_bin="${REPO_ROOT}/bin/cosm"
            elif [[ -x "${APP_DIR}/bin/cosm" ]]; then
                cosm_bin="${APP_DIR}/bin/cosm"
            fi

            if [[ -n "${cosm_bin}" ]]; then
                echo "🔍 Verifying Cosm AST Merkle-DAG status..."
                (cd "${APP_DIR}" && "${cosm_bin}" status)
                echo ""
            else
                echo "⚠️  '--cosm-verify' requested, but 'cosm' CLI not found on PATH or in bin/."
            fi
        else
            echo "ℹ️  '--cosm-verify' requested, but no .cosm/ directory found in ${APP_DIR}."
        fi
    fi
}

compile_server() {
    local needs_build=false
    if [[ ! -f "${BIN_FILE}" ]]; then
        needs_build=true
    else
        local newer_file
        newer_file="$(find "${APP_DIR}/cmd" "${APP_DIR}/internal" "${APP_DIR}/go.mod" "${APP_DIR}/go.sum" -type f -newer "${BIN_FILE}" 2>/dev/null | head -n 1 || true)"
        if [[ -n "${newer_file}" ]]; then
            needs_build=true
        fi
    fi

    if [[ "${needs_build}" == "true" ]]; then
        echo "📦 Compiling server binary into ${BIN_FILE}..."
        mkdir -p "$(dirname "${BIN_FILE}")"
        (cd "${APP_DIR}" && go build -o "${BIN_FILE}" ./cmd/server)
        echo "✓ Server binary compiled successfully."
    else
        echo "✓ Server binary is up to date: ${BIN_FILE}"
    fi
}

start_server() {
    print_banner "🏕️  LAUNCHING ALPINE ESCAPES LOCAL SERVER"

    verify_cosm
    compile_server

    # Check if a server is already running using PID file and kill -0
    if [[ -f "${PID_FILE}" ]]; then
        local existing_pid
        existing_pid="$(cat "${PID_FILE}" 2>/dev/null || true)"
        if [[ -n "${existing_pid}" ]] && kill -0 "${existing_pid}" 2>/dev/null; then
            echo "⚠️  Server is already running with PID ${existing_pid}."
            echo "• Live URL:        http://localhost:${PORT}"
            echo "• Health Endpoint: http://localhost:${PORT}/health"
            echo "• Stop Command:    $0 stop"
            return 0
        else
            echo "⚠️  Found stale PID file (${PID_FILE}); clearing."
            rm -f "${PID_FILE}"
        fi
    fi

    # Check if port is already occupied
    local port_pids
    port_pids="$(lsof -ti tcp:"${PORT}" 2>/dev/null || true)"
    if [[ -n "${port_pids}" ]]; then
        echo "❌ Error: Port ${PORT} is already in use by process(es): ${port_pids}." >&2
        echo "   Please stop the process or specify a different port with --port <port>." >&2
        return 1
    fi

    # Foreground execution
    if [[ "${FOREGROUND}" == "true" ]]; then
        echo ""
        echo "🚀 Starting server in foreground on http://localhost:${PORT} (Press Ctrl+C to stop)..."
        echo "====================================================================="
        cd "${APP_DIR}"
        if [[ -n "${DATABASE_URL}" ]]; then
            export DATABASE_URL="${DATABASE_URL}"
        fi
        export PORT="${PORT}"
        exec "${BIN_FILE}"
    fi

    # Daemon mode: redirect stdout/stderr to deploy/camping_app.log
    mkdir -p "$(dirname "${LOG_FILE}")"
    echo ""
    echo "🚀 Starting server as background daemon on port ${PORT}..."

    (
        cd "${APP_DIR}"
        if [[ -n "${DATABASE_URL}" ]]; then
            export DATABASE_URL="${DATABASE_URL}"
        fi
        export PORT="${PORT}"
        exec "${BIN_FILE}" >> "${LOG_FILE}" 2>&1
    ) &
    local server_pid=$!
    disown "${server_pid}" 2>/dev/null || true
    echo "${server_pid}" > "${PID_FILE}"

    echo "• Process PID:     ${server_pid} (saved to ${PID_FILE})"
    echo "• Daemon Log:      ${LOG_FILE}"

    # Actively poll http://127.0.0.1:${PORT}/health for up to 10s (sleep 0.5s)
    echo "⏳ Waiting for health check at http://127.0.0.1:${PORT}/health..."
    local healthy=false
    for ((i=1; i<=20; i++)); do
        if ! kill -0 "${server_pid}" 2>/dev/null; then
            echo "❌ Error: Server process (PID ${server_pid}) terminated unexpectedly." >&2
            echo "--- Recent Log Output (${LOG_FILE}) ---" >&2
            tail -n 25 "${LOG_FILE}" 2>/dev/null || true
            rm -f "${PID_FILE}"
            return 1
        fi

        local http_code
        http_code="$(curl --noproxy "*" -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${PORT}/health" 2>/dev/null || echo "000")"
        if [[ "${http_code}" == "200" ]]; then
            healthy=true
            break
        fi
        sleep 0.5
    done

    if [[ "${healthy}" != "true" ]]; then
        echo "❌ Error: Timed out waiting for server health check (10s)." >&2
        echo "--- Recent Log Output (${LOG_FILE}) ---" >&2
        tail -n 25 "${LOG_FILE}" 2>/dev/null || true
        kill -KILL "${server_pid}" 2>/dev/null || true
        rm -f "${PID_FILE}"
        return 1
    fi

    echo ""
    echo "====================================================================="
    echo "  ✅ LOCAL SERVER STARTED SUCCESSFULLY!"
    echo "====================================================================="
    echo "• PID:             ${server_pid}"
    echo "• Live URL:        http://localhost:${PORT}"
    echo "• Health Endpoint: http://localhost:${PORT}/health"
    echo "• Log Path:        ${LOG_FILE}"
    echo "• Stop Command:    $0 stop"
    echo "====================================================================="
}

stop_server() {
    echo "====================================================================="
    echo "  🛑 STOPPING ALPINE ESCAPES LOCAL SERVER"
    echo "====================================================================="

    if [[ ! -f "${PID_FILE}" ]]; then
        echo "ℹ️  No running server found (PID file does not exist: ${PID_FILE})."
        echo "====================================================================="
        return 0
    fi

    local pid
    pid="$(cat "${PID_FILE}" 2>/dev/null || true)"
    if [[ -z "${pid}" ]]; then
        echo "⚠️  PID file was empty. Removing ${PID_FILE}."
        rm -f "${PID_FILE}"
        echo "====================================================================="
        return 0
    fi

    if ! kill -0 "${pid}" 2>/dev/null; then
        echo "ℹ️  Process ${pid} is not running (cleaning up stale ${PID_FILE})."
        rm -f "${PID_FILE}"
        echo "====================================================================="
        return 0
    fi

    echo "• Sending SIGTERM to PID ${pid}..."
    kill -TERM "${pid}" 2>/dev/null || true

    local stopped=false
    for ((i=1; i<=6; i++)); do
        if ! kill -0 "${pid}" 2>/dev/null; then
            stopped=true
            break
        fi
        sleep 0.5
    done

    if [[ "${stopped}" != "true" ]]; then
        echo "⚠️  Process ${pid} did not terminate within 3s; sending SIGKILL..."
        kill -KILL "${pid}" 2>/dev/null || true
        sleep 0.5
    fi

    rm -f "${PID_FILE}"
    echo "✓ Process terminated and PID file removed."
    echo "====================================================================="
    echo "  ✅ LOCAL SERVER STOPPED"
    echo "====================================================================="
}

restart_server() {
    stop_server
    echo ""
    start_server
}

status_server() {
    echo "====================================================================="
    echo "  📊 ALPINE ESCAPES: LOCAL SERVER STATUS"
    echo "====================================================================="

    local is_running=false
    local pid=""

    if [[ -f "${PID_FILE}" ]]; then
        pid="$(cat "${PID_FILE}" 2>/dev/null || true)"
        if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
            is_running=true
        fi
    fi

    if [[ "${is_running}" == "true" ]]; then
        echo "• Status:          🟢 RUNNING"
        echo "• PID:             ${pid}"
        echo "• Port:            ${PORT}"
        echo "• Live URL:        http://localhost:${PORT}"
        echo "• Log Path:        ${LOG_FILE}"

        local resp
        resp="$(curl --noproxy "*" -s -w "\n%{http_code}" "http://127.0.0.1:${PORT}/health" 2>/dev/null || printf "\n000")"
        local http_code
        http_code="$(echo "${resp}" | tail -n 1)"
        local http_body
        http_body="$(echo "${resp}" | sed '$d')"

        if [[ "${http_code}" == "200" ]]; then
            echo "• Health Probe:    ✓ HTTP 200 OK"
            echo "• Health Response: ${http_body}"
        else
            echo "• Health Probe:    ⚠️ HTTP ${http_code} (Unhealthy or unreachable)"
            if [[ -n "${http_body}" ]]; then
                echo "• Health Response: ${http_body}"
            fi
        fi
    else
        echo "• Status:          🔴 STOPPED"
        if [[ -f "${PID_FILE}" ]]; then
            echo "• PID File:        ${PID_FILE} (stale PID: ${pid:-empty})"
        else
            echo "• PID File:        Not found (${PID_FILE})"
        fi
        echo "• Port:            ${PORT}"
    fi
    echo "====================================================================="
}

case "${VERB}" in
    start)
        start_server
        ;;
    stop)
        stop_server
        ;;
    restart)
        restart_server
        ;;
    status)
        status_server
        ;;
    *)
        echo "❌ Unknown verb: ${VERB}" >&2
        show_help
        exit 1
        ;;
esac
