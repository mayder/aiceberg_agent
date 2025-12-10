# Guia de instalação do AIceberg Agent (usuário final)

Este guia é para quem vai instalar e configurar o agente nos hosts (Windows, macOS, Linux). Assume que você já tem o **token** fornecido pelo painel.

## Configuração básica (variáveis)
- `AGENT_TOKEN` (obrigatório) ou `AGENT_TOKEN_PATH` apontando para um arquivo com o token.
- `API_BASE_URL` (opcional, default produção `https://api.aiceberg.com.br`).
- `AGENT_MODE`: `direct` (padrão, envia direto à API), `relay` (envia para um hub), `hub` (age como hub e também envia à API).
- `HUB_URL`: URL do hub de destino (necessário em `relay` e para logs em hub).
- `HUB_TOKEN`: token que o relay envia no header Authorization para o hub.
- `HUB_LISTEN_ADDR`: endereço/porta para ouvir como hub (ex.: `:9090`) — usado apenas em `hub`.
- `SKIP_BOOTSTRAP`: `true` somente se não houver API disponível (ex.: relay isolado).
- Transporte HTTP: `HTTP_GZIP` (compressão) e `HTTP_IDEMPOTENCY` (header Idempotency-Key).
- TLS: `TLS_INSECURE_SKIP_VERIFY=true` apenas para homolog/teste com certificados inválidos; respeita `HTTPS_PROXY/NO_PROXY` do ambiente.
- Saúde opcional: `HEALTH_PORT=8081`.
- Outbox persistente (fila local): `OUTBOX_PATH` para o arquivo da fila (ex.: Linux `/var/lib/aiceberg/outbox.db`, Windows `C:\ProgramData\AIceberg\outbox.db`, macOS `/Library/AIceberg/outbox.db`) e `OUTBOX_MAX_MB` (default 200). Garanta permissão de escrita do usuário/serviço nesse caminho; se não conseguir abrir o arquivo, o agente cai para memória.
- Coleta de logs (SOC):
  - Unix: `OSLOG_ENABLED=true`, `OSLOG_FILES=/var/log/auth.log,/var/log/syslog`, `OSLOG_CURSOR_PATH=/var/lib/aiceberg/oslogs.cursor`, `OSLOG_INTERVAL=15`, `OSLOG_MAX_BYTES=262144`.
  - Windows: `OSLOG_ENABLED=true`, `OSLOG_WIN_CHANNELS=System,Application,Security`.
  - Diagnóstico: `OSLOG_DIAG=true` para receber erros de permissão/arquivo no log do agente.
- Paths opcionais: `PREFS_PATH`, `OSLOG_CURSOR_PATH`, `AGENT_TOKEN_PATH`.

## Modos de operação (escolha um)
- **Direct** (padrão): `AGENT_MODE=direct`. Requer `AGENT_TOKEN`. Envia direto para a API (`API_BASE_URL`).
- **Relay**: `AGENT_MODE=relay`, `HUB_URL` e `HUB_TOKEN` (ou Authorization custom via `HUB_TOKEN`). Envia tudo para o hub. Use `SKIP_BOOTSTRAP=true` apenas se o host não puder falar com a API.
- **Hub**: `AGENT_MODE=hub`, `HUB_LISTEN_ADDR` (ex.: `:9090`). O hub aceita ingest em `/v1/ingest` com header Authorization vindo dos relays/agents e repassa para a API. Ainda precisa de `AGENT_TOKEN` para o próprio hub se registrar e enviar métricas.

## Windows (zip)
1) Baixe e extraia `aiceberg-agent-windows-amd64.zip` em uma pasta (contém `agent.exe`, `install.ps1`, `install-service.ps1`, `README_INSTALL.txt`).
2) Abra PowerShell **como Administrador** e vá até a pasta extraída.
3) Instale com o token e modo desejado (exemplo direct):
   ```powershell
   powershell -ExecutionPolicy Bypass -File .\install.ps1 -Token SEU_TOKEN -Mode direct
   ```
   Relay:
   ```powershell
   powershell -ExecutionPolicy Bypass -File .\install.ps1 -Token SEU_TOKEN -Mode relay -HubUrl https://meu-hub:9090 -HubToken TOKEN_DO_HUB
   ```
   Hub (listener local):
   ```powershell
   powershell -ExecutionPolicy Bypass -File .\install.ps1 -Token SEU_TOKEN -Mode hub -HubListen :9090
   ```
   O script grava variáveis de ambiente no escopo Máquina e cria o serviço `AIcebergAgent`.
4) (Opcional) Habilitar coleta de logs do Windows:
   ```powershell
   [Environment]::SetEnvironmentVariable("OSLOG_ENABLED","true","Machine")
   [Environment]::SetEnvironmentVariable("OSLOG_WIN_CHANNELS","System,Application,Security","Machine")
   sc stop AIcebergAgent; sc start AIcebergAgent
   ```
5) Verifique: `sc query AIcebergAgent`. Logs: Event Viewer (Application) ou `C:\ProgramData\AIceberg\` conforme configurar.

## Linux (tar.gz, systemd)
1) Baixe e extraia `aiceberg-agent-linux-<arch>.tar.gz`.
2) (Recomendado) criar usuário de serviço:
   ```bash
   sudo useradd --system --no-create-home --shell /usr/sbin/nologin aiceberg_agent
   ```
3) Dentro da pasta extraída, rode como root: `sudo ./install.sh`. Isso instala o binário em `/usr/local/bin/aiceberg_agent`, copia o service file e cria diretórios em `/etc/aiceberg` e `/var/lib/aiceberg`.
4) Edite `/etc/aiceberg/agent.env` e configure:
   - Direct: `AGENT_TOKEN=...` (padrão).
   - Relay: `AGENT_TOKEN=...`, `AGENT_MODE=relay`, `HUB_URL=...`, `HUB_TOKEN=...`.
   - Hub: `AGENT_TOKEN=...`, `AGENT_MODE=hub`, `HUB_LISTEN_ADDR=:9090`.
   - Logs: `OSLOG_ENABLED=true`, `OSLOG_FILES=/var/log/auth.log,/var/log/syslog`.
5) Aplique as mudanças: `sudo systemctl daemon-reload && sudo systemctl restart aiceberg-agent`.
6) Verifique: `systemctl status aiceberg-agent` e `curl http://localhost:8081/health` se `HEALTH_PORT` estiver definido.

## macOS (tar.gz, launchd)
1) Baixe e extraia `aiceberg-agent-darwin-<arch>.tar.gz`.
2) Copie o binário: `sudo install -m 0755 aiceberg_agent /usr/local/bin/aiceberg_agent`.
3) Crie diretório de trabalho: `sudo mkdir -p /Library/AIceberg`.
4) Crie `/Library/LaunchDaemons/com.aiceberg.agent.plist` com conteúdo (preencha os valores):
   ```xml
   <?xml version="1.0" encoding="UTF-8"?>
   <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
   <plist version="1.0">
   <dict>
     <key>Label</key><string>com.aiceberg.agent</string>
     <key>ProgramArguments</key>
       <array><string>/usr/local/bin/aiceberg_agent</string></array>
     <key>EnvironmentVariables</key>
       <dict>
         <key>AGENT_TOKEN</key><string>SEU_TOKEN</string>
         <key>AGENT_MODE</key><string>direct</string>
         <!-- Relay: troque para relay e adicione HUB_URL/HUB_TOKEN -->
         <!-- Hub: troque para hub e adicione HUB_LISTEN_ADDR -->
         <key>HUB_URL</key><string></string>
         <key>HUB_TOKEN</key><string></string>
         <key>HUB_LISTEN_ADDR</key><string></string>
         <key>OSLOG_ENABLED</key><string>true</string>
         <key>OSLOG_FILES</key><string>/var/log/system.log</string>
         <key>HEALTH_PORT</key><string>8081</string>
       </dict>
     <key>RunAtLoad</key><true/>
     <key>KeepAlive</key><true/>
     <key>WorkingDirectory</key><string>/Library/AIceberg</string>
   </dict>
   </plist>
   ```
   Ajuste variáveis conforme direct/relay/hub e logs.
5) Garanta permissões: `sudo chmod 644 /Library/LaunchDaemons/com.aiceberg.agent.plist`.
6) Carregue o serviço: `sudo launchctl load -w /Library/LaunchDaemons/com.aiceberg.agent.plist`.
7) Verifique: `sudo launchctl list | grep com.aiceberg.agent` e `curl http://localhost:8081/health` se configurado.

## Dicas finais
- Reaplique/reinicie o serviço sempre que ajustar variáveis.
- Se estiver em modo relay, garanta que `HUB_URL` está acessível e que o hub aceita o token enviado (header Authorization).
- Em modo hub, exponha `HUB_LISTEN_ADDR` e repasse a porta no firewall; relays enviarão para `/v1/ingest`.
