# Manual de Instalação do AIceberg Agent (Cliente)

Este guia orienta a instalação do AIceberg Agent em estações/servidores Windows, macOS e Linux. Use-o no módulo de cliente do painel web.

## O que você precisa
- Token do agente gerado no painel.
- Permissão de administrador/root para instalar serviços.
- Acesso HTTPS para `https://api.aiceberg.com.br/v1` (porta 443).

## Pacotes esperados
- Windows: `AIcebergAgent-Setup-x64.msi` (ou `.exe`).
- macOS: `AIcebergAgent.pkg`.
- Linux: `aiceberg-agent_<versao>_amd64.deb` (Debian/Ubuntu) ou `aiceberg-agent-<versao>.x86_64.rpm` (RHEL/Alma/Rocky/Fedora).

## Instalação
### Windows
1. Baixe o instalador MSI/EXE.
2. Execute e insira o token quando solicitado.
3. O serviço `AIcebergAgent` será criado e iniciado automaticamente.
4. Modo silencioso (PowerShell/admin):
   ```powershell
   msiexec /i .\AIcebergAgent-Setup-x64.msi TOKEN=SEU_TOKEN_HERE /qn
   ```

### macOS
1. Abra `AIcebergAgent.pkg`.
2. Insira o token quando solicitado (ou use arquivo de resposta em instalações automatizadas).
3. O serviço `com.aiceberg.agent` será carregado via launchd.
4. Modo CLI:
   ```bash
   sudo installer -pkg AIcebergAgent.pkg -target /
   ```

### Linux (Debian/Ubuntu)
```bash
sudo TOKEN=SEU_TOKEN_HERE dpkg -i aiceberg-agent_<versao>_amd64.deb
sudo systemctl status aiceberg-agent
```

### Linux (RHEL/Alma/Rocky/Fedora)
```bash
sudo TOKEN=SEU_TOKEN_HERE rpm -i aiceberg-agent-<versao>.x86_64.rpm
sudo systemctl status aiceberg-agent
```

## Onde ficam os arquivos
- Windows: binário em `C:\Program Files\AIceberg\agent\agent.exe`; token/estado/logs em `C:\ProgramData\AIceberg\`.
- macOS: `/usr/local/bin/aiceberg_agent` ou `/Library/AIceberg/agent`; dados em `/Library/AIceberg/`.
- Linux: `/usr/local/bin/aiceberg_agent`; config em `/etc/aiceberg/config.yml`; dados/logs em `/var/lib/aiceberg/`.

## Verificação pós-instalação
1. Serviço: `sc query AIcebergAgent` (Win) / `launchctl list | grep aiceberg` (macOS) / `systemctl status aiceberg-agent` (Linux).
2. Health local (se habilitado): `http://localhost:8081/health` deve responder `ok`.
3. Painel: confirme “último check-in” e, opcionalmente, use o botão de ping remoto para validar presença online.

## Desinstalação
- Windows: Painel “Adicionar/Remover Programas” ou `msiexec /x AIcebergAgent-Setup-x64.msi /qn`.
- macOS: `sudo launchctl unload /Library/LaunchDaemons/com.aiceberg.agent.plist && sudo rm -rf /Library/AIceberg /Library/LaunchDaemons/com.aiceberg.agent.plist`.
- Linux: `sudo apt remove aiceberg-agent` ou `sudo rpm -e aiceberg-agent`.

## Notas e suporte
- A API de produção já é o padrão (`API_BASE_URL`); só altere se indicado pelo suporte.
- O token fica salvo localmente para sobrevivência a reboot/upgrade.
- Em caso de bloqueio de rede, permitir tráfego HTTPS de saída para `api.aiceberg.com.br`. Log local: veja o diretório de dados/logs citado acima.
