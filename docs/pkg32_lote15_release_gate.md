# PKG-32 Lote 15 - Release gate final

Data: 2026-04-26

## Escopo

Gate final do canal operacional bidirecional no `aiceberg_agent`, incluindo qualidade, runbook, rollback e geracao dos artefatos oficiais do agente.

## Runbook operacional do agente

### Pre-check

1. Confirmar versao em `internal/common/version/version.go`.
2. Rodar `./check.sh`.
3. Confirmar que `scripts/build_installers.sh` e o unico caminho oficial para gerar artefatos.
4. Confirmar que o canal foi homologado no lab: `docs/pkg32_lote14_homologacao.md`.

### Operacao por modo

- `direct`: agente abre conexao outbound com AIceberg e mantem heartbeat.
- `hub`: hub abre conexao outbound com AIceberg, registra presenca propria e recebe relays.
- `relay`: relay conecta somente ao hub via `HUB_URL`; nao abre canal direto com AIceberg.

### Diagnostico

1. Verificar logs estruturados: `agent started`, `channel connected`, `channel heartbeat`, `collect buffered`, `flushed`.
2. Verificar `/health` quando `HEALTH_PORT` estiver habilitado.
3. Em `relay`, validar `HUB_URL` e token do hub.
4. Em update remoto, acompanhar etapas reportadas: precheck, download, validation, apply, restart, reconnected e version_confirmed.

## Rollback

Rollback operacional:

1. Desligar `agent_channel_enabled` no `aiceberg_web` para o cliente.
2. Manter polling atual ativo; nao remover `ping`, `bootstrap`, `config`, `selfheal`, `update-report` nem `ingest`.
3. Nao reexecutar comando que ja tenha resultado terminal para o mesmo `command_id`.

Rollback local do agente:

1. Voltar `AGENT_MODE`/override para o modo anterior, se a troca de modo falhar.
2. Reiniciar o servico pelo gerenciador local (`systemd`, `launchd` ou Windows Service).
3. Usar a versao anterior do pacote se o update falhar antes da confirmacao de `version_confirmed`.

Rollback de artefato:

1. Nao distribuir a versao nova se `./check.sh` falhar.
2. Se a versao nova ja foi publicada, manter a pasta historica e apontar a configuracao remota para a versao anterior validada.
3. Publicar somente compactados oficiais gerados por `scripts/build_installers.sh`.

## Publicacao

1. Rodar `./check.sh`.
2. Rodar `./scripts/build_installers.sh`.
3. Publicar os compactados em `aiceberg_web/cliente/web/downloads/agent/<versao>/`.
4. Registrar SHA-256 dos artefatos.
5. Atualizar `update.version/url/sha256` somente depois da publicacao dos compactados.

## Artefatos 0.8.1

Gerados por `./scripts/build_installers.sh` apos `./check.sh` verde nos dois repos.

| Arquivo | SHA-256 |
| --- | --- |
| `aiceberg-agent-darwin-amd64.tar.gz` | `fbee2c7e4811530be90af35162cbff0eccf94b24dc16dd484c0612cc8030825b` |
| `aiceberg-agent-darwin-arm64.tar.gz` | `e4e45001e37054ac89be24cbdfd5a1ac73f1025a04f8f48b511a61021be64783` |
| `aiceberg-agent-linux-amd64.tar.gz` | `2284468ed2ce5353c1ce4420dd44fe3919a8b281786567c3ed5c7b55f5cd7163` |
| `aiceberg-agent-linux-arm64.tar.gz` | `ba24d3763bfaf6d27fac14392c205e546ec1e78eb9882989dea0b55597d17a61` |
| `aiceberg-agent-windows-amd64.zip` | `41a8f6d13ab486675533c4de98994f004591dd2f0b8fd97c72beedb2d2859325` |

## Artefatos 0.8.2

Gerados por `./scripts/build_installers.sh` apos `./check.sh` verde nos dois repos.

| Arquivo | SHA-256 |
| --- | --- |
| `aiceberg-agent-darwin-amd64.tar.gz` | `d446f704714b37ff00f30d85d4c56edcb9ee4be55ef39ab3757bd4df013110cc` |
| `aiceberg-agent-darwin-arm64.tar.gz` | `609e3edc8f519b79dc25f5cacdc6446619dba12a16c8dbf0ad60ab3dad6c87e7` |
| `aiceberg-agent-linux-amd64.tar.gz` | `fab8b2187888c638b66bbcbd2e9b3420a0113d41d1b1655843b758c73d0a2ac4` |
| `aiceberg-agent-linux-arm64.tar.gz` | `4dd3f9830279405ffbd1827a6186e8bae8784039dcdb7ab867ed1bd9980decc3` |
| `aiceberg-agent-windows-amd64.zip` | `33c40122ea3b2c089a188367d2845e7ff806acddebced1410f269311616bc7b9` |

## Artefatos 0.8.8

Gerados em 2026-06-18 por `./scripts/build_installers.sh` apos `./check.sh` verde no `aiceberg_agent`. Publicados em `aiceberg_web/cliente/web/downloads/agent/0.8.8/` com `SHA256SUMS` validado localmente.

| Arquivo | SHA-256 |
| --- | --- |
| `aiceberg-agent-darwin-amd64.tar.gz` | `4f93259351a1c7c80a57b5a94d62cc25f80af7d6dce3e21b5d3f7e7fd5f872d3` |
| `aiceberg-agent-darwin-arm64.tar.gz` | `b1ba2d6322e4a9b34208d565632fa14b6dbae99716f25f0949f87c5cec5be0a6` |
| `aiceberg-agent-linux-amd64.tar.gz` | `7ec788d694956850f8d8e9a1d864d46b764cef0b3473bd5020586cdf4858a9d8` |
| `aiceberg-agent-linux-arm64.tar.gz` | `c9846007c6a7ff5a62ae77b831e17ff4ed8598ffde948c2869d22e143956dabf` |
| `aiceberg-agent-windows-amd64.zip` | `482ede3f6a7d8758dd4c00b341d2cad92f4be23014a9a6209827dd3f644cceb5` |

## Criterio de fechamento

- `./check.sh` verde no `aiceberg_agent`.
- `./check.sh` verde no `aiceberg_web`.
- Versao nova do agente gerada somente depois dos checks.
- Artefatos compactados oficiais publicados no web.
- Rollback por flag e por artefato documentado.
