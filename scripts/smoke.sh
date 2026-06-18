#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

WORKDIR="${SMOKE_WORKDIR:-$(mktemp -d "${TMPDIR:-/tmp}/aiceberg-smoke.XXXXXX")}"
KEEP="${SMOKE_KEEP:-}"
PYTHON="${PYTHON:-python3}"

log() {
  echo "[smoke] $*"
}

backend_pid=""
agent_pid=""

cleanup() {
  for pid in "$agent_pid" "$backend_pid"; do
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

BACKEND_PORT="${SMOKE_BACKEND_PORT:-$(pick_port)}"
HEALTH_PORT="${SMOKE_HEALTH_PORT:-$(pick_port)}"

log "workdir=${WORKDIR}"

AGENT_BIN="${WORKDIR}/agent"
BACKEND_BIN="${WORKDIR}/smoke-backend"
LOG_FILE="${WORKDIR}/agent.debug.log"
OSLOG_FILE="${WORKDIR}/oslog.log"
EVIDENCE_FILE="${SMOKE_EVIDENCE_FILE:-${WORKDIR}/smoke-evidence.json}"

log "build agent binary"
go build -o "${AGENT_BIN}" ./cmd/agent

log "build backend binary"
go build -o "${BACKEND_BIN}" ./scripts/e2e_backend.go

log "prepare oslog file"
printf "Jan  1 00:00:01 host app[123]: hello world\n" > "${OSLOG_FILE}"

log "start backend"
E2E_BACKEND_PORT="${BACKEND_PORT}" \
E2E_CONFIG_MODE=payload \
"${BACKEND_BIN}" >"${WORKDIR}/backend.log" 2>&1 &
backend_pid=$!

wait_for_backend() {
  local retries=30
  for _ in $(seq 1 "${retries}"); do
    if curl -sf "http://127.0.0.1:${BACKEND_PORT}/__stats" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  return 1
}

if ! wait_for_backend; then
  log "backend not ready"
  exit 1
fi

log "start agent"
AGENT_TOKEN="token-smoke" \
AGENT_MODE="direct" \
API_BASE_URL="http://127.0.0.1:${BACKEND_PORT}" \
HEALTH_PORT="${HEALTH_PORT}" \
PING_INTERVAL=2 \
CONFIG_SYNC_INTERVAL=5 \
OUTBOX_PATH="${WORKDIR}/outbox.db" \
OUTBOX_MAX_MB=5 \
AGENTLESS_OUTBOX_PATH="${WORKDIR}/agentless_outbox.db" \
AGENTLESS_OUTBOX_MAX_MB=5 \
PREFS_PATH="${WORKDIR}/prefs.json" \
AGENT_TOKEN_PATH="${WORKDIR}/agent.token" \
AGENT_STATE_PATH="${WORKDIR}/bootstrap.ok" \
OSLOG_FILES="${OSLOG_FILE}" \
OSLOG_INTERVAL=1 \
OSLOG_BATCH_LINES=10 \
OSLOG_MAX_BYTES=256 \
LOG_LEVEL=debug \
LOG_FILE_PATH="${LOG_FILE}" \
LOG_FILE_MAX_MB=1 \
LOG_FILE_MAX_BACKUPS=2 \
"${AGENT_BIN}" >"${WORKDIR}/agent.log" 2>&1 &
agent_pid=$!

wait_for_agent() {
  local retries=30
  for _ in $(seq 1 "${retries}"); do
    if curl -sf "http://127.0.0.1:${HEALTH_PORT}/health" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  return 1
}

if ! wait_for_agent; then
  log "agent not ready"
  exit 1
fi

log "check metrics"
HEALTH_JSON="$(curl -sf "http://127.0.0.1:${HEALTH_PORT}/health")"
METRICS_BODY="$(curl -sf "http://127.0.0.1:${HEALTH_PORT}/metrics")"
if [[ -z "${METRICS_BODY}" ]]; then
  log "metrics endpoint returned empty body"
  exit 1
fi

log "check backend stats for oslogs"
wait_for_oslogs() {
  local retries=20
  for _ in $(seq 1 "${retries}"); do
    local stats
    if stats="$(curl -sf "http://127.0.0.1:${BACKEND_PORT}/__stats")"; then
      if "${PYTHON}" - "${stats}" <<'PY'
import json,sys
s=json.loads(sys.argv[1])
ingested=s.get("ingested",{})
sys.exit(0 if ingested.get("/v1/logs/raw",0) >= 1 else 1)
PY
      then
        return 0
      fi
    fi
    sleep 1
  done
  return 1
}

if ! wait_for_oslogs; then
  log "oslogs not ingested"
  if [[ -f "${WORKDIR}/backend.log" ]]; then
    log "backend log:"
    tail -n 200 "${WORKDIR}/backend.log"
  fi
  if [[ -f "${WORKDIR}/agent.log" ]]; then
    log "agent log:"
    tail -n 200 "${WORKDIR}/agent.log"
  fi
  exit 1
fi

STATS_JSON="$(curl -sf "http://127.0.0.1:${BACKEND_PORT}/__stats")"

if [[ ! -f "${LOG_FILE}" ]]; then
  log "log file not found"
  exit 1
fi

log "write smoke evidence"
"${PYTHON}" - "${EVIDENCE_FILE}" "${HEALTH_JSON}" "${STATS_JSON}" "${LOG_FILE}" "${OSLOG_FILE}" <<'PY'
import hashlib
import json
import os
import sys
from datetime import datetime, timezone

target, health_raw, stats_raw, log_file, oslog_file = sys.argv[1:6]

def digest(path):
    h = hashlib.sha256()
    with open(path, "rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()

health = json.loads(health_raw)
stats = json.loads(stats_raw)
evidence = {
    "schema": "aiceberg.agent.smoke.v1",
    "generated_at": datetime.now(timezone.utc).isoformat(),
    "platform": "posix",
    "checks": {
        "health_endpoint": bool(health),
        "metrics_endpoint": True,
        "logs_ingested": stats.get("ingested", {}).get("/v1/logs/raw", 0) >= 1,
        "debug_log_created": os.path.exists(log_file),
    },
    "health": {
        "status": health.get("status"),
        "agent_pipeline_version": health.get("agent_pipeline_version"),
        "uptime_sec": health.get("uptime_sec"),
        "proc_rss_bytes": health.get("proc_rss_bytes"),
        "proc_cpu_percent": health.get("proc_cpu_percent"),
        "goroutines": health.get("goroutines"),
        "queue_items": health.get("queue_items"),
        "queue_bytes": health.get("queue_bytes"),
        "last_collect_ms": health.get("last_collect_ms"),
        "last_flush_ms": health.get("last_flush_ms"),
        "flush_detail": health.get("flush_detail"),
    },
    "backend": {
        "ingested": stats.get("ingested", {}),
        "ping_get": stats.get("ping_get"),
        "ping_post": stats.get("ping_post"),
        "bootstraps": stats.get("bootstraps"),
        "config_gets": stats.get("config_gets"),
    },
    "artifacts": {
        "agent_log_sha256": digest(log_file),
        "oslog_fixture_sha256": digest(oslog_file),
    },
}
os.makedirs(os.path.dirname(target), exist_ok=True)
with open(target, "w", encoding="utf-8") as fh:
    json.dump(evidence, fh, indent=2, sort_keys=True)
    fh.write("\n")
PY

log "evidence=${EVIDENCE_FILE}"
log "ok"
