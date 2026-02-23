#!/usr/bin/env bash
set -euo pipefail

timestamp() { date -u +"%Y-%m-%dT%H:%M:%SZ"; }
log() { printf '[%s] [apply] %s\n' "$(timestamp)" "$*"; }
fail() { log "ERROR: $*"; exit 1; }

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  fail "este script precisa executar como root."
fi

UPDATE_FILE="${AICEBERG_UPDATE_FILE:-}"
UPDATE_VERSION="${AICEBERG_UPDATE_VERSION:-unknown}"
UPDATE_SHA="${AICEBERG_UPDATE_SHA256:-}"
SERVICE_NAME="${AICEBERG_UPDATE_SERVICE:-aiceberg-agent}"
BIN_DST="${AICEBERG_UPDATE_BIN_DST:-/usr/local/bin/aiceberg_agent}"
AGENT_BIN="${AICEBERG_AGENT_BIN:-$BIN_DST}"
AGENT_ENV_FILE="${AICEBERG_AGENT_ENV_FILE:-/etc/aiceberg/agent.env}"
AGENT_PID_FILE="${AICEBERG_AGENT_PID_FILE:-/var/run/${SERVICE_NAME}.pid}"
AGENT_STDOUT_LOG="${AICEBERG_AGENT_STDOUT_LOG:-/var/log/aiceberg-agent.log}"
RESTART_COMMAND="${AICEBERG_UPDATE_RESTART_COMMAND:-}"
STATE_DIR="${AICEBERG_UPDATE_STATE_DIR:-/var/lib/aiceberg}"
LOG_FILE="${AICEBERG_UPDATE_LOG_FILE:-/var/log/aiceberg-agent-update.log}"
LOCK_FILE="${STATE_DIR}/update.lock"

mkdir -p "$STATE_DIR" "$(dirname "$LOG_FILE")"
exec >>"$LOG_FILE" 2>&1

log "start update version=${UPDATE_VERSION} file=${UPDATE_FILE}"

if [[ -z "$UPDATE_FILE" ]]; then
  fail "AICEBERG_UPDATE_FILE não informado."
fi
if [[ ! -f "$UPDATE_FILE" ]]; then
  fail "arquivo de update não encontrado: $UPDATE_FILE"
fi

exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  log "outro update já está em execução; ignorando requisição."
  exit 0
fi

if [[ -n "$UPDATE_SHA" ]]; then
  normalized_sha="$(printf '%s' "$UPDATE_SHA" | tr '[:upper:]' '[:lower:]' | sed -E 's/^sha256://')"
  if [[ "$normalized_sha" =~ ^[a-f0-9]{64}$ ]]; then
    got_sha="$(sha256sum "$UPDATE_FILE" | awk '{print $1}')"
    if [[ "$got_sha" != "$normalized_sha" ]]; then
      fail "sha256 inválido: esperado=$normalized_sha recebido=$got_sha"
    fi
  else
    log "sha256 informado inválido, ignorando validação: $UPDATE_SHA"
  fi
fi

restart_manual() {
  local target_bin="$AGENT_BIN"
  if [[ ! -x "$target_bin" && -x "$BIN_DST" ]]; then
    target_bin="$BIN_DST"
  fi
  if [[ ! -x "$target_bin" ]]; then
    log "restart manual indisponível: binário não executável ($target_bin)."
    return 1
  fi

  mkdir -p "$(dirname "$AGENT_PID_FILE")" "$(dirname "$AGENT_STDOUT_LOG")"

  if [[ -f "$AGENT_PID_FILE" ]]; then
    local old_pid=""
    old_pid="$(cat "$AGENT_PID_FILE" 2>/dev/null || true)"
    if [[ "$old_pid" =~ ^[0-9]+$ ]] && kill -0 "$old_pid" 2>/dev/null; then
      kill "$old_pid" 2>/dev/null || true
      for _ in $(seq 1 20); do
        if ! kill -0 "$old_pid" 2>/dev/null; then
          break
        fi
        sleep 0.25
      done
      if kill -0 "$old_pid" 2>/dev/null; then
        kill -9 "$old_pid" 2>/dev/null || true
      fi
    fi
    rm -f "$AGENT_PID_FILE" || true
  fi

  if [[ -f "$AGENT_ENV_FILE" ]]; then
    nohup env AGENT_ENV_FILE="$AGENT_ENV_FILE" "$target_bin" >>"$AGENT_STDOUT_LOG" 2>&1 < /dev/null 9>&- &
  else
    log "AGENT_ENV_FILE não encontrado em $AGENT_ENV_FILE; iniciando sem env file explícito."
    nohup "$target_bin" >>"$AGENT_STDOUT_LOG" 2>&1 < /dev/null 9>&- &
  fi
  local new_pid="$!"
  echo "$new_pid" > "$AGENT_PID_FILE"
  sleep 1
  if ! kill -0 "$new_pid" 2>/dev/null; then
    log "processo não permaneceu ativo após restart manual (pid=$new_pid)."
    return 1
  fi
  log "restart manual concluído pid=$new_pid bin=$target_bin"
  return 0
}

restart_agent() {
  local attempts=()

  if [[ -n "$RESTART_COMMAND" ]]; then
    attempts+=("custom")
    if /bin/sh -c "$RESTART_COMMAND"; then
      log "restart via comando customizado concluído."
      return 0
    fi
    log "falha no restart via comando customizado."
  fi

  if command -v systemctl >/dev/null 2>&1; then
    attempts+=("systemctl")
    if systemctl restart "$SERVICE_NAME"; then
      log "restart via systemctl concluído."
      return 0
    fi
    log "falha no restart via systemctl para $SERVICE_NAME."
  fi

  if command -v service >/dev/null 2>&1; then
    attempts+=("service")
    if service "$SERVICE_NAME" restart; then
      log "restart via service concluído."
      return 0
    fi
    log "falha no restart via service para $SERVICE_NAME."
  fi

  attempts+=("manual")
  if restart_manual; then
    return 0
  fi

  log "todas as tentativas de restart falharam (${attempts[*]})."
  return 1
}

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

case "$UPDATE_FILE" in
  *.tar.gz|*.tgz)
    tar -xzf "$UPDATE_FILE" -C "$tmpdir"
    ;;
  *.zip)
    if ! command -v unzip >/dev/null 2>&1; then
      fail "arquivo zip recebido, mas unzip não está instalado."
    fi
    unzip -q "$UPDATE_FILE" -d "$tmpdir"
    ;;
  *)
    fail "formato de pacote não suportado: $UPDATE_FILE"
    ;;
esac

bin_src="$(find "$tmpdir" -type f -name aiceberg_agent | head -n1 || true)"
if [[ -z "$bin_src" ]]; then
  fail "binário aiceberg_agent não encontrado no pacote."
fi

backup=""
if [[ -f "$BIN_DST" ]]; then
  backup="${BIN_DST}.bak.$(date +%Y%m%d%H%M%S)"
  cp -f "$BIN_DST" "$backup"
  chmod 0755 "$backup" || true
fi

install -m 0755 "$bin_src" "${BIN_DST}.new"
mv -f "${BIN_DST}.new" "$BIN_DST"

if ! restart_agent; then
  log "falha ao reiniciar serviço $SERVICE_NAME; tentando rollback."
  if [[ -n "$backup" && -f "$backup" ]]; then
    cp -f "$backup" "$BIN_DST"
    chmod 0755 "$BIN_DST" || true
    restart_agent || true
  fi
  fail "reinício falhou."
fi

log "update concluído com sucesso version=${UPDATE_VERSION} bin=${BIN_DST}"
exit 0
