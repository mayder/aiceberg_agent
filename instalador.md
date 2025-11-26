# Roteiro de Instaladores (Windows / macOS / Linux)

Este documento lista os passos necessários para criar instaladores reais do AIceberg Agent, por sistema operacional. Use como checklist de implementação e QA.

## Comum a todos
- [ ] Pipeline de build cross-compilado (GOOS/GOARCH) com versão embutida.
- [ ] Layout de paths: binário, config, estado (token/bootstrap), logs, dados (fila).
- [ ] Aceitar/preencher token na instalação (via UI ou arquivo/token fornecido).
- [ ] Registrar serviço (daemon) e garantir start automático.
- [ ] Scripts de start/stop/restart e uninstall limpo (remover serviço, opcionalmente binário).
- [ ] Assinatura/validação (codesign/notarization em macOS, assinatura MSI/EXE, GPG para pacotes Linux).
- [ ] Teste pós-instalação: subir serviço, health OK, enviar um batch de telemetria.

## Windows (MSI/EXE)
- [ ] Gerar binário `aiceberg_agent.exe` (amd64/arm64).
- [ ] Empacotar com MSI/EXE (WiX ou ferramenta similar):
  - Path sugerido: `C:\Program Files\AIceberg\agent\agent.exe`.
  - Diretório de dados/config: `C:\ProgramData\AIceberg\`.
  - Criar serviço Windows (`sc.exe create` ou via WiX):
    - Nome: `AIcebergAgent`.
    - Comando: `"agent.exe -config C:\ProgramData\AIceberg\config.yml"`.
    - Start automático.
  - Incluir script PowerShell para instalação silenciosa (passando token).
- [ ] Captura do token na instalação:
  - UI do instalador aceita token e grava em `C:\ProgramData\AIceberg\agent.token` e `bootstrap.ok`.
  - Para silent install: parâmetro `TOKEN=<...>`.
- [ ] Permissões: diretórios com ACL restrita (SYSTEM/Administrators/serviço).
- [ ] Assinatura do binário e do instalador (Authenticode).
- [ ] Uninstall: parar serviço, remover serviço, remover binário; preservar config/token opcionalmente.

## macOS (pkg + launchd)
- [ ] Gerar binário `aiceberg_agent` (universal ou amd64/arm64 separado).
- [ ] Empacotar em `.pkg`:
  - Path sugerido: `/usr/local/bin/aiceberg_agent` ou `/Library/AIceberg/agent`.
  - Config/estado: `/Library/AIceberg/config.yml`, `/Library/AIceberg/data/agent.token`, `/Library/AIceberg/data/bootstrap.ok`, `/Library/AIceberg/logs`.
  - launchd plist em `/Library/LaunchDaemons/com.aiceberg.agent.plist`:
    - ProgramArguments com `-config /Library/AIceberg/config.yml`.
    - RunAtLoad, KeepAlive.
- [ ] Captura do token na instalação:
  - UI do pkg solicita token; script pós-instalação grava `agent.token`/`bootstrap.ok`.
  - Alternativa: arquivo de resposta para instalação silenciosa.
- [ ] Permissões: diretórios de dados com 700/600; propriedade root ou usuário dedicado.
- [ ] Assinatura + notarization do pkg/binário.
- [ ] Uninstall: unload do launchd, remover plist/binário; preservar config/token opcionalmente.

## Linux (.deb / .rpm / script)
- [ ] Gerar binário `aiceberg_agent` (amd64/arm64).
- [ ] Pacotes:
  - `.deb`: instalar em `/usr/local/bin/aiceberg_agent`, config em `/etc/aiceberg/config.yml`, dados em `/var/lib/aiceberg/` (token/bootstrap/queue/logs).
  - `.rpm`: equivalente para RHEL/Alma.
  - Script de instalação direta (curl/bash) para ambientes sem gerenciador.
- [ ] systemd unit (já temos base em `scripts/linux/aiceberg-agent.service`):
  - Ajustar paths para config/data/logs.
  - Usuário/grupo dedicados `aiceberg_agent`.
  - Diretiva `StateDirectory`/`LogsDirectory` conforme paths.
- [ ] Captura do token:
  - Instalador pede token e grava em `/var/lib/aiceberg/agent.token` e `bootstrap.ok`.
  - Suporte a instalação não interativa: `TOKEN=<...> apt install ...`.
- [ ] Dependências opcionais: `smartmontools` para SMART, `nvidia-smi` para GPU; avisar se ausentes.
- [ ] Pós-install script: cria usuário, diretórios, permissions (700/600), habilita e inicia serviço.
- [ ] Uninstall: `systemctl stop/disable`, remover pacote; preservar dados opcional.

## Pós-instalação / Verificação
- [ ] Serviço ativo (`systemctl status` / `launchctl list` / `sc query`).
- [ ] Health endpoint acessível na porta configurada.
- [ ] Telemetria chegando no backend (ver último seen / ingest log).
- [ ] Logs locais rotacionando se necessário.
- [ ] Atualização: validar upgrade de versão preservando token/estado/config.
