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

if ! systemctl restart "$SERVICE_NAME"; then
  log "falha ao reiniciar serviço $SERVICE_NAME; tentando rollback."
  if [[ -n "$backup" && -f "$backup" ]]; then
    cp -f "$backup" "$BIN_DST"
    chmod 0755 "$BIN_DST" || true
    systemctl restart "$SERVICE_NAME" || true
  fi
  fail "reinício falhou."
fi

log "update concluído com sucesso version=${UPDATE_VERSION} bin=${BIN_DST}"
exit 0
