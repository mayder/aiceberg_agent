#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

WORKDIR="${E2E_WORKDIR:-$(mktemp -d "${TMPDIR:-/tmp}/aiceberg-e2e.XXXXXX")}"
KEEP="${E2E_KEEP:-}"
PYTHON="${PYTHON:-python3}"

log() {
  echo "[e2e] $*"
}

backend_pid=""
hub_pid=""
relay_pid=""
direct_pid=""

cleanup() {
  for pid in "$direct_pid" "$relay_pid" "$hub_pid" "$backend_pid"; do
    if [[ -n "${pid}" ]] && kill -0 "${pid}" >/dev/null 2>&1; then
      kill "${pid}" >/dev/null 2>&1 || true
      wait "${pid}" >/dev/null 2>&1 || true
    fi
  done
  if [[ -z "${KEEP}" ]]; then
    rm -rf "${WORKDIR}"
  else
    log "preserving workdir: ${WORKDIR}"
  fi
}
trap cleanup EXIT

wait_for_backend() {
  local retries=30
  local sleep_s=1
  for _ in $(seq 1 "${retries}"); do
    if curl -sf "http://127.0.0.1:${BACKEND_PORT}/__stats" >/dev/null; then
      return 0
    fi
    sleep "${sleep_s}"
  done
  return 1
}

wait_for_hub() {
  local retries=30
  local sleep_s=1
  for _ in $(seq 1 "${retries}"); do
    local code
    code="$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${HUB_PORT}/v1/ingest")" || true
    if [[ "${code}" != "000" ]]; then
      return 0
    fi
    sleep "${sleep_s}"
  done
  return 1
}

wait_for_stats() {
  local retries=40
  local sleep_s=2
  for _ in $(seq 1 "${retries}"); do
    local stats
    if stats="$(curl -sf "http://127.0.0.1:${BACKEND_PORT}/__stats")"; then
      if "${PYTHON}" - "${stats}" <<'PY'
import json,sys
s=json.loads(sys.argv[1])
by_auth=s.get("by_auth",{})
def has(auth):
    return by_auth.get(auth,0) >= 1
ok = True
ok = ok and has("Token token-direct")
ok = ok and has("Token token-hub")
ok = ok and has("Token token-relay")
ok = ok and s.get("ping_get",0) >= 1
ok = ok and s.get("bootstraps",0) >= 1
sys.exit(0 if ok else 1)
PY
      then
        return 0
      fi
    fi
    sleep "${sleep_s}"
  done
  return 1
}

log "workdir=${WORKDIR}"
if ! command -v "${PYTHON}" >/dev/null 2>&1; then
  if command -v python >/dev/null 2>&1; then
    PYTHON=python
  else
    log "python not found (set PYTHON=python3)"
    exit 1
  fi
fi

if [[ "${CGO_CFLAGS:-}" != *-Wno-gnu-folding-constant* ]]; then
  export CGO_CFLAGS="${CGO_CFLAGS:-} -Wno-gnu-folding-constant"
fi

pick_port() {
  "${PYTHON}" - <<'PY'
import socket
s=socket.socket()
s.bind(("127.0.0.1",0))
print(s.getsockname()[1])
s.close()
PY
}

if [[ -n "${E2E_BACKEND_PORT:-}" ]]; then
  BACKEND_PORT="${E2E_BACKEND_PORT}"
else
  BACKEND_PORT="$(pick_port)"
fi
if [[ -n "${E2E_HUB_PORT:-}" ]]; then
  HUB_PORT="${E2E_HUB_PORT}"
else
  HUB_PORT="$(pick_port)"
fi
if [[ "${HUB_PORT}" == "${BACKEND_PORT}" ]]; then
  HUB_PORT="$(pick_port)"
fi
log "build agent binary"
AGENT_BIN="${WORKDIR}/agent"
go build -o "${AGENT_BIN}" ./cmd/agent

log "build backend binary"
BACKEND_BIN="${WORKDIR}/e2e-backend"
go build -o "${BACKEND_BIN}" ./scripts/e2e_backend.go

log "start backend"
E2E_BACKEND_PORT="${BACKEND_PORT}" \
E2E_CONFIG_MODE=payload \
"${BACKEND_BIN}" >"${WORKDIR}/backend.log" 2>&1 &
backend_pid=$!

if ! wait_for_backend; then
  log "backend not ready"
  if [[ -f "${WORKDIR}/backend.log" ]]; then
    log "backend log:"
    tail -n 200 "${WORKDIR}/backend.log"
  fi
  exit 1
fi

start_agent() {
  local name="${1}"
  local mode="${2}"
  local health_port="${3}"
  local hub_listen="${4}"
  local hub_url="${5}"
  local dir="${WORKDIR}/${name}"
  mkdir -p "${dir}"
  : > "${dir}/empty.log"
  AGENT_TOKEN="token-${name}" \
  AGENT_MODE="${mode}" \
  API_BASE_URL="http://127.0.0.1:${BACKEND_PORT}" \
  HUB_LISTEN_ADDR="${hub_listen}" \
  HUB_URL="${hub_url}" \
  HEALTH_PORT="${health_port}" \
  PING_INTERVAL=2 \
  CONFIG_SYNC_INTERVAL=5 \
  OUTBOX_PATH="${dir}/outbox.db" \
  OUTBOX_MAX_MB=5 \
  AGENTLESS_OUTBOX_PATH="${dir}/agentless_outbox.db" \
  AGENTLESS_OUTBOX_MAX_MB=5 \
  AGENTLESS_ENABLED=false \
  PREFS_PATH="${dir}/prefs.json" \
  AGENT_TOKEN_PATH="${dir}/agent.token" \
  AGENT_STATE_PATH="${dir}/bootstrap.ok" \
  OSLOG_CURSOR_PATH="${dir}/oslogs.cursor" \
  OSLOG_FILES="${dir}/empty.log" \
  LOG_LEVEL=info \
  "${AGENT_BIN}" >"${dir}/agent.log" 2>&1 &
  echo $!
}

log "start hub agent"
hub_pid="$(start_agent "hub" "hub" "18083" "127.0.0.1:${HUB_PORT}" "")"

if ! wait_for_hub; then
  log "hub not ready"
  exit 1
fi

log "start relay agent"
relay_pid="$(start_agent "relay" "relay" "18082" "" "http://127.0.0.1:${HUB_PORT}")"

log "start direct agent"
direct_pid="$(start_agent "direct" "direct" "18081" "" "")"

log "wait for ingestion and ping/bootstraps"
if ! wait_for_stats; then
  log "stats did not reach expected values"
  curl -sf "http://127.0.0.1:${BACKEND_PORT}/__stats" || true
  exit 1
fi

log "check metrics endpoint"
metrics="$(curl -sf "http://127.0.0.1:18081/metrics")"
echo "${metrics}" | grep -q "agent_invalid_envelopes_total"
echo "${metrics}" | grep -q "agent_agentless_jobs_total"

log "ok"
