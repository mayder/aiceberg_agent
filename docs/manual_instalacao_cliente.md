# Manual de Instalação do AIceberg Agent (Cliente)

Este guia orienta a instalação do AIceberg Agent em estações/servidores Windows, macOS e Linux. Use-o no módulo de cliente do painel web.

## O que você precisa
- Token do agente gerado no painel.
- Permissão de administrador/root para instalar serviços.
- Acesso HTTPS para `https://api.aiceberg.com.br` (porta 443); o agente acrescenta `/v1/...` automaticamente.

## Pacotes esperados
- Windows: `.zip` com `agent.exe`, `install.ps1` (instala tudo) e `install-service.ps1`.
- macOS: `AIcebergAgent.pkg`.
- Linux: `.tar.gz` com `aiceberg_agent`, `agent.env.example`, `aiceberg-agent.service`, `install.sh`.

## Instalação
### Windows (recomendado)
1. Abra PowerShell como Administrador e vá para a pasta extraída.
2. Rode o instalador automático:
   ```powershell
   powershell -ExecutionPolicy Bypass -File .\install.ps1 -Token SEU_TOKEN_AQUI
   ```
   (Opcional: `-Mode hub|relay`, `-HubUrl`, `-HubToken`, `-SkipBootstrap`).
3. Isso cria pastas, grava o token (se informado), define variáveis de ambiente e cria o serviço.
4. Verifique: `sc query AIcebergAgent`.

### macOS
1. Abra `AIcebergAgent.pkg`.
2. Insira o token quando solicitado (ou use arquivo de resposta em instalações automatizadas).
3. O serviço `com.aiceberg.agent` será carregado via launchd.
4. Modo CLI:
   ```bash
   sudo installer -pkg AIcebergAgent.pkg -target /
   ```

### Linux (instalador automático no tar.gz)
```bash
tar -xzf aiceberg-agent-linux-amd64.tar.gz
cd aiceberg-agent-linux-amd64
sudo ./install.sh   # instala binário, service e cria /etc/aiceberg/agent.env (edite depois)
```
Depois edite `/etc/aiceberg/agent.env` para definir `AGENT_TOKEN` e demais variáveis (`AGENT_MODE`, `HUB_URL`, `OSLOG_*` etc.), e reinicie o serviço se necessário.

## Onde ficam os arquivos
- Windows: binário em `C:\Program Files\AIceberg\agent\agent.exe`; token/estado/logs em `C:\ProgramData\AIceberg\`.
- macOS: `/usr/local/bin/aiceberg_agent` ou `/Library/AIceberg/agent`; dados em `/Library/AIceberg/`.
- Linux: `/usr/local/bin/aiceberg_agent`; env em `/etc/aiceberg/agent.env`; dados/logs em `/var/lib/aiceberg/`.

## Verificação pós-instalação
1. Serviço: `sc query AIcebergAgent` (Win) / `launchctl list | grep aiceberg` (macOS) / `systemctl status aiceberg-agent` (Linux).
2. Health local (se habilitado): `http://localhost:8081/health` deve responder `ok`.
3. Painel: confirme “último check-in” e, opcionalmente, use o botão de ping remoto para validar presença online.

## Desinstalação
- Windows: Painel “Adicionar/Remover Programas” ou `msiexec /x AIcebergAgent-Setup-x64.msi /qn`.
- macOS: `sudo launchctl unload /Library/LaunchDaemons/com.aiceberg.agent.plist && sudo rm -rf /Library/AIceberg /Library/LaunchDaemons/com.aiceberg.agent.plist`.
- Linux: `sudo apt remove aiceberg-agent` ou `sudo rpm -e aiceberg-agent`.

## Notas e suporte
- A API de produção já é o padrão (`API_BASE_URL=https://api.aiceberg.com.br`); o agente acrescenta `/v1/...` automaticamente.
- Modos: `AGENT_MODE=direct|hub|relay`; em hub/relay configure `HUB_URL/HUB_TOKEN/HUB_LISTEN_ADDR`, `SKIP_BOOTSTRAP` se relay não tiver acesso à API.
- Logs (SOC): habilite com `OSLOG_ENABLED=true`. Linux: liste arquivos em `OSLOG_FILES`; Windows: coleta canais Security/System/Application/Sysmon via Event Log. Cursor em `OSLOG_CURSOR_PATH`.
- O token fica salvo localmente para sobrevivência a reboot/upgrade.
- Em caso de bloqueio de rede, permitir tráfego HTTPS de saída para `api.aiceberg.com.br`. Log local: veja o diretório de dados/logs citado acima.
