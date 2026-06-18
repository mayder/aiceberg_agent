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
- cenarios locais automatizados de API indisponivel, backoff, payload grande e outbox cheia;
- `./check.sh`;
- listagem explicita de cenarios pendentes de ambiente real.

Execucao local em 2026-06-18, Darwin 25.5.0 arm64:

- `scripts/pkg69_operational_homologation.sh`: passou com `go test` focado, cenarios locais de API indisponivel/rede intermitente/payload grande/outbox cheia e `./check.sh`;
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
| Update quebrado | `update-report` com falha e rollback seguro | pendente |
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
- download/update remoto controlado for testado;
- matriz web `docs/agente_datadog_paridade.md` refletir validado vs pendente.
