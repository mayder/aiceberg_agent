AIceberg Agent - pacote standalone

Conteúdo:
- aiceberg_agent (binário)
- agent.env.example (variáveis de ambiente)
- service (systemd ou launchd) para instalar como serviço

Passos resumidos:
1) Crie diretórios: sudo mkdir -p /usr/local/bin /var/lib/aiceberg && sudo mkdir -p /var/lib/aiceberg/data
2) Copie o binário para /usr/local/bin ou /Library/AIceberg/agent (macOS).
3) Copie agent.env.example para /etc/aiceberg/agent.env e defina AGENT_TOKEN (e outras variáveis, se precisar). Garanta permissão 600.
4) Instale o serviço:
   - Linux (systemd): sudo cp service/aiceberg-agent.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now aiceberg-agent
   - macOS (launchd): sudo cp service/com.aiceberg.agent.plist /Library/LaunchDaemons/ && sudo launchctl load /Library/LaunchDaemons/com.aiceberg.agent.plist
5) Verifique: curl http://localhost:8081/health (se HEALTH_PORT habilitado).

API de produção já é padrão: https://api.aiceberg.com.br/v1
