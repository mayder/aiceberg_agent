# PKG-69 - Matriz operacional real

## Objetivo

Fechar a homologacao operacional dos pacotes PKG-59 a PKG-68 com evidencia por ambiente, falha e capacidade. Build/check local nao basta para declarar maturidade.

## Comando reproduzivel

```bash
scripts/pkg69_operational_homologation.sh
```

O script de homologacao executa:

- deteccao de Docker, kubectl, Helm e PowerShell;
- testes focados dos coletores e contratos adicionados em PKG-59 a PKG-68;
- validacao local da cadeia Ed25519 de artefato de update, aceitando assinatura valida e rejeitando assinatura invalida antes do `apply`;
- cenarios locais automatizados de API indisponivel, backoff, payload grande e outbox cheia;
- `./check.sh`;
- listagem explicita de cenarios pendentes de ambiente real.

Execucao local em 2026-06-18, Darwin 25.5.0 arm64:

- `scripts/pkg69_operational_homologation.sh`: passou com `go test` focado, cadeia Ed25519 local de artefato de update, cenarios locais de API indisponivel/rede intermitente/payload grande/outbox cheia e `./check.sh`;
- Docker daemon indisponivel no host local;
- `kubectl` disponivel, mas sem validacao de cluster/DaemonSet/Helm;
- Helm e PowerShell indisponiveis neste host;
- Windows, Linux real, Docker, Kubernetes, proxy autenticado, disco cheio real, alto volume real e rollback de update seguem pendentes de ambiente controlado.

Smokes por sistema operacional:

```bash
scripts/smoke.sh
powershell -ExecutionPolicy Bypass -File scripts/smoke.ps1
```

Os smokes geram `smoke-evidence.json` no diretório temporário ou no caminho definido por
`SMOKE_EVIDENCE_FILE`/`-SmokeEvidenceFile`, com health, métricas, ingestão de logs e SHA256
dos artefatos locais usados na validação.

Smoke POSIX local executado em 2026-06-18 com `SMOKE_EVIDENCE_FILE=/tmp/aiceberg_pkg69_smoke_evidence.json`:

- `health.status=ok`;
- `agent_pipeline_version=2-compatible`;
- ingestao confirmada em `/v1/ingest/bootstrap`, `/v1/ingest/health`, `/v1/ingest/metrics` e `/v1/logs/raw`;
- `agent_log_sha256=55fa32917610298090284f24906b144030424ccb64212da613aa0af7743beae5`;
- `oslog_fixture_sha256=e14a2773b6da308c2776891300d37c55cd9750137af86071d40b7655c0a35525`.

Artefatos oficiais 0.8.8 gerados em 2026-06-18:

- `./check.sh` passou antes da geracao;
- `./scripts/build_installers.sh` gerou Linux amd64/arm64, Darwin amd64/arm64 e Windows amd64;
- compactados publicados localmente em `aiceberg_web/cliente/web/downloads/agent/0.8.8/`;
- `shasum -a 256 -c SHA256SUMS` passou na pasta publicada;
- download/update remoto controlado ainda nao foi acionado.

Hashes 0.8.8:

| Arquivo | SHA-256 |
| --- | --- |
| `aiceberg-agent-darwin-amd64.tar.gz` | `4f93259351a1c7c80a57b5a94d62cc25f80af7d6dce3e21b5d3f7e7fd5f872d3` |
| `aiceberg-agent-darwin-arm64.tar.gz` | `b1ba2d6322e4a9b34208d565632fa14b6dbae99716f25f0949f87c5cec5be0a6` |
| `aiceberg-agent-linux-amd64.tar.gz` | `7ec788d694956850f8d8e9a1d864d46b764cef0b3473bd5020586cdf4858a9d8` |
| `aiceberg-agent-linux-arm64.tar.gz` | `c9846007c6a7ff5a62ae77b831e17ff4ed8598ffde948c2869d22e143956dabf` |
| `aiceberg-agent-windows-amd64.zip` | `482ede3f6a7d8758dd4c00b341d2cad92f4be23014a9a6209827dd3f644cceb5` |

## Ambientes obrigatorios

| Ambiente | Evidencia exigida | Estado atual |
| --- | --- | --- |
| Windows Server | `smoke.ps1`, EventLog, servico Windows, update/rollback | pendente real |
| Windows desktop | `smoke.ps1`, EventLog, instalador, proxy | pendente real |
| Ubuntu/Debian | `smoke.sh`, systemd, logs, outbox, update | pendente real |
| RHEL/Alma/Rocky | `smoke.sh`, systemd, dnf/yum, update | pendente real |
| Docker | `CONTAINER_ENABLED=true`, Docker socket, labels, recursos | pendente |
| Kubernetes | DaemonSet/Helm, RBAC minimo, pods/events, rollback chart | pendente |
| macOS/dev local | testes focados, `./check.sh` e smoke POSIX | validado local em 2026-06-18 |

## Cenarios obrigatorios

| Cenario | Evidencia exigida | Estado atual |
| --- | --- | --- |
| API indisponivel | backoff, outbox preservada, sem crash | parcial local |
| Rede intermitente | retry/backoff e flush posterior | parcial local |
| Proxy/TLS | proxy autenticado e TLS invalido controlado | pendente |
| Disco cheio/outbox cheia | erro claro, sem corrupcao | parcial local |
| Payload grande | limite/queda controlada em DogStatsD, OTLP e logs | parcial local |
| Clock errado | health/time sync com diagnostico | pendente |
| Reboot durante coleta | servico retorna e outbox preserva dados | pendente |
| Update quebrado | `update-report` com falha e rollback seguro | pendente; cadeia Ed25519 validada localmente antes do apply |
| Permissao insuficiente | degradacao com status claro | pendente |
| Kubernetes RBAC minimo | sem permissao a secrets/exec/delete | pendente |
| Alto volume simultaneo | CPU/memoria dentro do limite definido | pendente |

## Limites aceitaveis iniciais

| Perfil | CPU media | Memoria RSS | Observacao |
| --- | ---: | ---: | --- |
| idle | <= 2% | <= 150 MB | host sem coletores avancados |
| coleta normal | <= 5% | <= 250 MB | sysmetrics, logs e health |
| alto volume local | <= 15% | <= 500 MB | DogStatsD/OTLP/logs simultaneos |
| containers/Kubernetes | <= 10% | <= 400 MB | intervalos >= 30s |

Limites devem ser revisados com evidencia real antes de declarar superioridade.

## Fechamento

PKG-69 so pode ser fechado quando:

- cada ambiente aplicavel tiver evidencia anexada ou bloqueio real documentado;
- cenarios marcados como `parcial local` forem reexecutados em ambiente real quando dependerem de SO/rede/disco/carga;
- `./check.sh` passar no `aiceberg_agent` e no `aiceberg_web`;
- artefatos oficiais forem gerados com SHA256;
- download/update remoto controlado for testado com artefato assinado;
- matriz web `docs/agente_datadog_paridade.md` refletir validado vs pendente.
