#!/bin/bash
set -euo pipefail
export PATH=/usr/bin:/bin:/usr/sbin:/sbin

log() { printf '[%s] [apply] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
fail() { log "$*" >&2; return 1; }
require_root() { [[ $EUID -eq 0 && "$(uname -s)" == Darwin ]]; }
service_command() { launchctl "$@"; }

validate_input() {
    [[ -f "$UPDATE_FILE" && ! -L "$UPDATE_FILE" && -f "$BIN_DST" && ! -L "$BIN_DST" ]] || return 1
    [[ "$UPDATE_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ && "$UPDATE_SHA" =~ ^[a-f0-9]{64}$ ]] || return 1
    [[ "$FROM_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || return 1
    [[ "$HEALTH_URL" =~ ^http://127\.0\.0\.1:[0-9]+/health$ ]] || return 1
    [[ "$(shasum -a 256 "$UPDATE_FILE" | awk '{print $1}')" == "$UPDATE_SHA" ]] || return 1
}

stage_binary() {
    local entry arch
    case "$(uname -m)" in arm64) arch=arm64 ;; x86_64) arch=amd64 ;; *) return 1 ;; esac
    entry="$(tar -tzf "$UPDATE_FILE" | awk -v expected="aiceberg-agent-darwin-$arch/aiceberg_agent" '$0 == expected {print}')"
    [[ -n "$entry" && "$entry" != *$'\n'* ]] || return 1
    # Extrair só o conteúdo do binário, sem materializar caminhos/links do tar.
    tar -xOzf "$UPDATE_FILE" "$entry" >"$CANDIDATE" || return 1
    [[ -s "$CANDIDATE" ]] || return 1
    chmod 0755 "$CANDIDATE"
}

wait_healthy() {
    local attempt
    for ((attempt=0; attempt<30; attempt++)); do
        if service_command print system/com.aiceberg.agent >/dev/null 2>&1 \
            && [[ "$(health_version)" == "$EXPECTED_VERSION" ]]; then
            return 0
        fi
        sleep 1
    done
    return 1
}

health_version() {
    curl --noproxy '*' -fsS --max-time 2 "$HEALTH_URL" 2>/dev/null \
        | /usr/bin/plutil -extract version raw -o - - 2>/dev/null
}

restart_service() {
    service_command bootstrap system "$PLIST" && wait_healthy
}

release_job() {
    rmdir "$LOCK" 2>/dev/null || true
    # Remover apenas o registro transitório do próprio updater, após liberar o lock.
    if [[ "${AICEBERG_UPDATE_JOB_LABEL:-}" =~ ^com\.aiceberg\.agent\.update\.[0-9]+$ ]]; then
        service_command remove "$AICEBERG_UPDATE_JOB_LABEL" >/dev/null 2>&1 || true
    fi
}

apply_binary() {
    cp -p "$BIN_DST" "$BACKUP" || return 1
    service_command bootout system/com.aiceberg.agent || return 1
    if mv -f "$CANDIDATE" "$BIN_DST" && restart_service; then
        log "aplicação concluída; aguardando confirmação remota da versão=$UPDATE_VERSION"
        return 0
    fi
    log 'Falha após parada; restaurando binário anterior.'
    service_command bootout system/com.aiceberg.agent >/dev/null 2>&1 || true
    cp -p "$BACKUP" "$BIN_DST" || return 1
    EXPECTED_VERSION="$FROM_VERSION"
    if restart_service; then
        log 'Rollback concluído; atualização não confirmada.'
    else
        log 'Rollback sem confirmação de saúde; intervenção necessária.'
    fi
    return 1
}

main() {
    require_root || { fail 'Updater macOS exige root.'; return 1; }
    BIN_DST=/Library/AIceberg/bin/aiceberg_agent
    PLIST=/Library/LaunchDaemons/com.aiceberg.agent.plist
    UPDATE_FILE="${AICEBERG_UPDATE_FILE:-}"
    UPDATE_VERSION="${AICEBERG_UPDATE_VERSION:-}"
    FROM_VERSION="${AICEBERG_AGENT_VERSION_CURRENT:-}"
    EXPECTED_VERSION="$UPDATE_VERSION"
    UPDATE_SHA="${AICEBERG_UPDATE_SHA256:-}"
    HEALTH_URL="${AICEBERG_AGENT_HEALTH_URL:-}"
    validate_input || { fail 'Pacote, SHA256, versão ou health local inválido.'; return 1; }
    # Diretório criado atomicamente; não expirar lock automaticamente durante execução incerta.
    LOCK=/Library/AIceberg/update.lock.d
    mkdir "$LOCK" 2>/dev/null || { fail 'Atualização já reservada; verificar execução anterior.'; return 1; }
    trap release_job EXIT
    CANDIDATE="$(mktemp /Library/AIceberg/bin/.aiceberg-update.XXXXXX)"
    BACKUP="${BIN_DST}.bak.$(date +%Y%m%d%H%M%S).$$"
    stage_binary || { fail 'Binário macOS ausente ou pacote inválido.'; return 1; }
    apply_binary
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
