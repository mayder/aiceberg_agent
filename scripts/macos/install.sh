#!/bin/bash
set -euo pipefail
export PATH=/usr/bin:/bin:/usr/sbin:/sbin

[[ "$(uname -s)" == Darwin && $EUID -eq 0 ]] || { echo 'Execute no macOS como root.' >&2; exit 1; }
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_DST=/Library/AIceberg/agent.env
PLIST=/Library/LaunchDaemons/com.aiceberg.agent.plist
for file in aiceberg_agent agent.env.example service/com.aiceberg.agent.plist service/aiceberg-agent-update-launcher.sh service/aiceberg-agent-apply-update.sh; do
    [[ -f "$DIR/$file" && ! -L "$DIR/$file" ]] || { echo 'Pacote incompleto.' >&2; exit 1; }
done
# Configuração existente e dados são preservados. Não instalar com destinos simbólicos.
for path in /Library/AIceberg /Library/AIceberg/data /Library/AIceberg/bin /Library/AIceberg/sbin "$ENV_DST" "$PLIST" /Library/AIceberg/bin/aiceberg_agent /Library/AIceberg/sbin/aiceberg-agent-update-launcher.sh /Library/AIceberg/sbin/aiceberg-agent-apply-update.sh; do
    [[ ! -L "$path" ]] || { echo 'Destino simbólico recusado.' >&2; exit 1; }
done
plutil -lint "$DIR/service/com.aiceberg.agent.plist" >/dev/null
install -d -m 0755 /Library/AIceberg /Library/AIceberg/bin /Library/AIceberg/sbin /Library/AIceberg/data
chown root:wheel /Library/AIceberg /Library/AIceberg/bin /Library/AIceberg/sbin /Library/AIceberg/data
if [[ ! -f "$ENV_DST" ]]; then
    install -m 0600 "$DIR/agent.env.example" "$ENV_DST"
    printf '\nAUTO_UPDATE_ENABLED=true\nAUTO_UPDATE_TRUST_REQUIRED=true\nAUTO_UPDATE_COMMAND=/Library/AIceberg/sbin/aiceberg-agent-update-launcher.sh\nAUTO_UPDATE_DIR=/Library/AIceberg/data/updates\nHEALTH_PORT=8081\n' >>"$ENV_DST"
fi
chown root:wheel "$ENV_DST"
chmod 0600 "$ENV_DST"
install -m 0755 "$DIR/service/aiceberg-agent-update-launcher.sh" /Library/AIceberg/sbin/aiceberg-agent-update-launcher.sh
install -m 0755 "$DIR/service/aiceberg-agent-apply-update.sh" /Library/AIceberg/sbin/aiceberg-agent-apply-update.sh
# Bootout somente do serviço oficial: nunca matar processos por nome.
if launchctl print system/com.aiceberg.agent >/dev/null 2>&1; then
    launchctl bootout system/com.aiceberg.agent
fi
install -m 0755 "$DIR/aiceberg_agent" /Library/AIceberg/bin/aiceberg_agent
install -m 0644 "$DIR/service/com.aiceberg.agent.plist" "$PLIST"
chown root:wheel /Library/AIceberg/bin/aiceberg_agent /Library/AIceberg/sbin/aiceberg-agent-update-launcher.sh /Library/AIceberg/sbin/aiceberg-agent-apply-update.sh "$PLIST"
plutil -lint "$PLIST" >/dev/null
launchctl bootstrap system "$PLIST"
echo 'Instalação concluída. Configure o token em /Library/AIceberg/agent.env; confirme serviço e comunicação no AIceberg.'
echo 'Configuração anterior foi preservada; instalações legadas precisam dos parâmetros de auto-update e health local descritos no manual.'
