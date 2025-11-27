#!/usr/bin/env bash
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "Este instalador precisa rodar como root (use sudo)." >&2
  exit 1
fi

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BIN_SRC="$DIR/aiceberg_agent"
ENV_SRC="$DIR/agent.env.example"
SERVICE_SRC="$DIR/service/aiceberg-agent.service"

BIN_DST="/usr/local/bin/aiceberg_agent"
ENV_DST="/etc/aiceberg/agent.env"
SERVICE_DST="/etc/systemd/system/aiceberg-agent.service"

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

echo "Recarregando systemd e iniciando serviço..."
systemctl daemon-reload
systemctl enable --now aiceberg-agent

echo "Pronto. Verifique status com: systemctl status aiceberg-agent"
echo "Edite $ENV_DST para configurar AGENT_TOKEN e demais variáveis."
