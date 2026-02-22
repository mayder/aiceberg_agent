#!/usr/bin/env bash
set -euo pipefail

timestamp() { date -u +"%Y-%m-%dT%H:%M:%SZ"; }
log() { printf '[%s] [launcher] %s\n' "$(timestamp)" "$*" >&2; }
fail() { log "ERROR: $*"; exit 1; }

UPDATE_FILE="${AICEBERG_UPDATE_FILE:-}"
if [[ -z "$UPDATE_FILE" ]]; then
  fail "AICEBERG_UPDATE_FILE não informado."
fi

if [[ ! -f "$UPDATE_FILE" ]]; then
  fail "Arquivo de update não encontrado: $UPDATE_FILE"
fi

APPLY_SCRIPT="${AICEBERG_UPDATE_APPLY_SCRIPT:-/usr/local/sbin/aiceberg-agent-apply-update.sh}"
if [[ ! -x "$APPLY_SCRIPT" ]]; then
  fail "Script de apply não encontrado/executável: $APPLY_SCRIPT"
fi

run_as_root() {
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    "$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo -n "$@"
    return
  fi
  return 1
}

run_systemd_unit() {
  local unit
  unit="aiceberg-agent-update-$(date +%s)"
  run_as_root \
    systemd-run \
      --unit="$unit" \
      --collect \
      --property=Type=oneshot \
      --setenv=AICEBERG_UPDATE_FILE="${AICEBERG_UPDATE_FILE:-}" \
      --setenv=AICEBERG_UPDATE_VERSION="${AICEBERG_UPDATE_VERSION:-}" \
      --setenv=AICEBERG_UPDATE_URL="${AICEBERG_UPDATE_URL:-}" \
      --setenv=AICEBERG_UPDATE_SHA256="${AICEBERG_UPDATE_SHA256:-}" \
      --setenv=AICEBERG_UPDATE_DIR="${AICEBERG_UPDATE_DIR:-}" \
      --setenv=AICEBERG_AGENT_VERSION_CURRENT="${AICEBERG_AGENT_VERSION_CURRENT:-}" \
      --setenv=AICEBERG_AGENT_BIN="${AICEBERG_AGENT_BIN:-}" \
      --setenv=AICEBERG_AGENT_ENV_FILE="${AICEBERG_AGENT_ENV_FILE:-}" \
      --setenv=AICEBERG_AGENT_PID_FILE="${AICEBERG_AGENT_PID_FILE:-}" \
      --setenv=AICEBERG_AGENT_STDOUT_LOG="${AICEBERG_AGENT_STDOUT_LOG:-}" \
      --setenv=AICEBERG_UPDATE_RESTART_COMMAND="${AICEBERG_UPDATE_RESTART_COMMAND:-}" \
      --setenv=AICEBERG_UPDATE_SERVICE="${AICEBERG_UPDATE_SERVICE:-aiceberg-agent}" \
      --setenv=AICEBERG_UPDATE_BIN_DST="${AICEBERG_UPDATE_BIN_DST:-/usr/local/bin/aiceberg_agent}" \
      "$APPLY_SCRIPT" >/dev/null
  log "update enfileirado em unidade transitória: $unit"
}

run_fallback_bg() {
  local log_file
  log_file="${AICEBERG_UPDATE_LOG_FILE:-/var/log/aiceberg-agent-update.log}"
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    nohup env \
      AICEBERG_UPDATE_FILE="${AICEBERG_UPDATE_FILE:-}" \
      AICEBERG_UPDATE_VERSION="${AICEBERG_UPDATE_VERSION:-}" \
      AICEBERG_UPDATE_URL="${AICEBERG_UPDATE_URL:-}" \
      AICEBERG_UPDATE_SHA256="${AICEBERG_UPDATE_SHA256:-}" \
      AICEBERG_UPDATE_DIR="${AICEBERG_UPDATE_DIR:-}" \
      AICEBERG_AGENT_VERSION_CURRENT="${AICEBERG_AGENT_VERSION_CURRENT:-}" \
      AICEBERG_AGENT_BIN="${AICEBERG_AGENT_BIN:-}" \
      AICEBERG_AGENT_ENV_FILE="${AICEBERG_AGENT_ENV_FILE:-}" \
      AICEBERG_AGENT_PID_FILE="${AICEBERG_AGENT_PID_FILE:-}" \
      AICEBERG_AGENT_STDOUT_LOG="${AICEBERG_AGENT_STDOUT_LOG:-}" \
      AICEBERG_UPDATE_RESTART_COMMAND="${AICEBERG_UPDATE_RESTART_COMMAND:-}" \
      AICEBERG_UPDATE_SERVICE="${AICEBERG_UPDATE_SERVICE:-aiceberg-agent}" \
      AICEBERG_UPDATE_BIN_DST="${AICEBERG_UPDATE_BIN_DST:-/usr/local/bin/aiceberg_agent}" \
      "$APPLY_SCRIPT" >>"$log_file" 2>&1 < /dev/null &
  elif command -v sudo >/dev/null 2>&1; then
    nohup sudo -n env \
      AICEBERG_UPDATE_FILE="${AICEBERG_UPDATE_FILE:-}" \
      AICEBERG_UPDATE_VERSION="${AICEBERG_UPDATE_VERSION:-}" \
      AICEBERG_UPDATE_URL="${AICEBERG_UPDATE_URL:-}" \
      AICEBERG_UPDATE_SHA256="${AICEBERG_UPDATE_SHA256:-}" \
      AICEBERG_UPDATE_DIR="${AICEBERG_UPDATE_DIR:-}" \
      AICEBERG_AGENT_VERSION_CURRENT="${AICEBERG_AGENT_VERSION_CURRENT:-}" \
      AICEBERG_AGENT_BIN="${AICEBERG_AGENT_BIN:-}" \
      AICEBERG_AGENT_ENV_FILE="${AICEBERG_AGENT_ENV_FILE:-}" \
      AICEBERG_AGENT_PID_FILE="${AICEBERG_AGENT_PID_FILE:-}" \
      AICEBERG_AGENT_STDOUT_LOG="${AICEBERG_AGENT_STDOUT_LOG:-}" \
      AICEBERG_UPDATE_RESTART_COMMAND="${AICEBERG_UPDATE_RESTART_COMMAND:-}" \
      AICEBERG_UPDATE_SERVICE="${AICEBERG_UPDATE_SERVICE:-aiceberg-agent}" \
      AICEBERG_UPDATE_BIN_DST="${AICEBERG_UPDATE_BIN_DST:-/usr/local/bin/aiceberg_agent}" \
      "$APPLY_SCRIPT" >>"$log_file" 2>&1 < /dev/null &
  else
    fail "não foi possível elevar privilégio para iniciar apply em background."
  fi
  log "update enfileirado em background (fallback)."
}

if command -v systemd-run >/dev/null 2>&1; then
  run_systemd_unit || run_fallback_bg
else
  run_fallback_bg
fi

exit 0
