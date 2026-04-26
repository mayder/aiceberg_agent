#!/usr/bin/env bash
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "Este instalador precisa rodar como root (use sudo)." >&2
  exit 1
fi

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BIN_SRC="$DIR/aiceberg_agent"
ENV_SRC="$DIR/agent.env.example"
SERVICE_SRC="$DIR/service/aiceberg-agent.service"
UPDATE_LAUNCHER_SRC="$DIR/service/aiceberg-agent-update-launcher.sh"
UPDATE_APPLY_SRC="$DIR/service/aiceberg-agent-apply-update.sh"

BIN_DST="/usr/local/bin/aiceberg_agent"
ENV_DST="/etc/aiceberg/agent.env"
SERVICE_DST="/etc/systemd/system/aiceberg-agent.service"
UPDATE_LAUNCHER_DST="/usr/local/sbin/aiceberg-agent-update-launcher.sh"
UPDATE_APPLY_DST="/usr/local/sbin/aiceberg-agent-apply-update.sh"
SUDOERS_DST="/etc/sudoers.d/aiceberg-agent-update"

echo "Instalando binário em $BIN_DST"
install -m 0755 "$BIN_SRC" "$BIN_DST"

echo "Criando diretórios de dados/config..."
mkdir -p /etc/aiceberg /var/lib/aiceberg /var/lib/aiceberg/data

if [[ ! -f "$ENV_DST" ]]; then
  echo "Criando $ENV_DST a partir do template (edite AGENT_TOKEN após a instalação)."
  cp "$ENV_SRC" "$ENV_DST"
  chmod 600 "$ENV_DST"
else
  echo "$ENV_DST já existe, preservado."
fi

echo "Instalando service em $SERVICE_DST"
cp "$SERVICE_SRC" "$SERVICE_DST"

if [[ -f "$UPDATE_LAUNCHER_SRC" && -f "$UPDATE_APPLY_SRC" ]]; then
  echo "Instalando scripts de update em /usr/local/sbin..."
  install -m 0755 "$UPDATE_LAUNCHER_SRC" "$UPDATE_LAUNCHER_DST"
  install -m 0755 "$UPDATE_APPLY_SRC" "$UPDATE_APPLY_DST"
  if command -v visudo >/dev/null 2>&1; then
    echo "Configurando sudo restrito para auto-update remoto..."
    cat >"$SUDOERS_DST.tmp" <<EOF
Defaults:aiceberg_agent env_keep += "AICEBERG_UPDATE_FILE AICEBERG_UPDATE_VERSION AICEBERG_UPDATE_URL AICEBERG_UPDATE_SHA256 AICEBERG_UPDATE_DIR AICEBERG_UPDATE_APPLY_SCRIPT AICEBERG_UPDATE_LOG_FILE AICEBERG_AGENT_VERSION_CURRENT AICEBERG_AGENT_BIN AICEBERG_AGENT_ENV_FILE AICEBERG_AGENT_PID_FILE AICEBERG_AGENT_STDOUT_LOG AICEBERG_UPDATE_RESTART_COMMAND AICEBERG_UPDATE_SERVICE AICEBERG_UPDATE_BIN_DST"
aiceberg_agent ALL=(root) NOPASSWD: $UPDATE_LAUNCHER_DST
EOF
    chmod 0440 "$SUDOERS_DST.tmp"
    visudo -cf "$SUDOERS_DST.tmp"
    mv -f "$SUDOERS_DST.tmp" "$SUDOERS_DST"
  else
    echo "visudo não encontrado; sudo restrito do auto-update não foi configurado."
  fi
else
  echo "Scripts de update não encontrados no pacote (seguindo sem instalar update helper)."
fi

echo "Recarregando systemd e iniciando serviço..."
systemctl daemon-reload
systemctl enable --now aiceberg-agent

echo "Pronto. Verifique status com: systemctl status aiceberg-agent"
echo "Edite $ENV_DST para configurar AGENT_TOKEN e demais variáveis."
echo "Para update remoto robusto, use no env:"
echo "  AUTO_UPDATE_ENABLED=true"
echo "  AUTO_UPDATE_COMMAND=sudo -n /usr/local/sbin/aiceberg-agent-update-launcher.sh"
