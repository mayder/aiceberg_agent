#!/bin/bash
set -euo pipefail
export PATH=/usr/bin:/bin:/usr/sbin:/sbin
[[ $EUID -eq 0 && "$(uname -s)" == Darwin ]] || { echo 'Updater macOS exige root.' >&2; exit 1; }
APPLY=/Library/AIceberg/sbin/aiceberg-agent-apply-update.sh
[[ -f "$APPLY" && -x "$APPLY" && ! -L "$APPLY" && "$(stat -f %u "$APPLY")" == 0 ]] || exit 1
[[ "${AICEBERG_UPDATE_BIN_DST:-}" == /Library/AIceberg/bin/aiceberg_agent ]] || { echo 'Instalação legada: destino incompatível.' >&2; exit 1; }
[[ -f "${AICEBERG_UPDATE_FILE:-}" && ! -L "$AICEBERG_UPDATE_FILE" ]] || exit 1
# Job separado do daemon monitorado: sobrevive ao bootout/restart do agente.
# Somente valores necessários; comandos de restart e destinos arbitrários não são herdados.
launchctl submit -l "com.aiceberg.agent.update.$$" \
    -o /var/log/aiceberg-agent-update.log -e /var/log/aiceberg-agent-update.log -- \
    /usr/bin/env -i PATH="$PATH" \
    AICEBERG_UPDATE_JOB_LABEL="com.aiceberg.agent.update.$$" \
    AICEBERG_UPDATE_FILE="$AICEBERG_UPDATE_FILE" \
    AICEBERG_UPDATE_VERSION="${AICEBERG_UPDATE_VERSION:-}" \
    AICEBERG_AGENT_VERSION_CURRENT="${AICEBERG_AGENT_VERSION_CURRENT:-}" \
    AICEBERG_UPDATE_SHA256="${AICEBERG_UPDATE_SHA256:-}" \
    AICEBERG_AGENT_HEALTH_URL="${AICEBERG_AGENT_HEALTH_URL:-}" "$APPLY"
