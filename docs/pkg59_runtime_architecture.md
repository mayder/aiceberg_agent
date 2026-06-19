# PKG-59 - Arquitetura Collector/Forwarder compativel

## Objetivo

Criar uma base de runtime para evoluir o agente como plataforma local sem quebrar os snapshots, endpoints e fluxos existentes.

## Contratos internos

Arquivo: `internal/domain/runtime/contracts.go`.

| Contrato | Responsabilidade | Estado |
|---|---|---|
| `Collector` | Nome, versao, capacidades, intervalo, timeout e coleta versionada | definido para novos coletores |
| `Forwarder` | Flush com resultado e snapshot operacional | definido para evoluir `FlushOutbox` |
| `Scheduler` | Registro e snapshot dos coletores ativos | definido com `SchedulerSnapshot` |
| `Supervisor` | Estado de saude do agente e ultimos tempos/erros | definido com `SupervisorSnapshot` |
| `ExtensionRuntime` | Limite futuro para checks/plugins opcionais | definido para PKG-66 |

O runtime atual continua usando os coletores legados por compatibilidade. A migracao completa para as interfaces novas deve ser incremental por coletor.

## Compatibilidade de payload

`CollectAndBuffer` injeta metadados aditivos no `body` antes de colocar o envelope na outbox:

- `schema_version`;
- `agent_pipeline_version`;
- `collector_name`;
- `ingest_endpoint`.

Esses campos sao opcionais para o web. O payload antigo continua presente no mesmo objeto JSON, portanto `agente_snapshot*` continua lendo `cpu`, `memory`, `disk`, `network`, `services`, `processes`, `agent` e demais secoes atuais.

## Diagnostico local

O health local passa a expor `agent_pipeline_version`.

O snapshot remoto de self-heal passa a expor:

- `agent_pipeline_version`;
- `scheduler_snapshot`;
- lista de coletores legados registrados como `legacy-compatible`.

Isso permite diagnosticar runtime, config, outbox e coletores sem acessar banco ou logs brutos.

## Coletores atuais mapeados

| Coletor atual | Endpoint | Intervalo | Prioridade |
|---|---|---:|---:|
| `sysmetrics` | `/v1/ingest/metrics` | 10s | 10 |
| `networkcapture` | `/v1/ingest/network_capture` | 10s | 20 |
| `oslogs` | `/v1/logs/raw` | `OSLOG_INTERVAL` | 20 |
| `sysmetrics_health` | `/v1/ingest/health` | 10min | 30 |
| `sysmetrics_inventory` | `/v1/ingest/inventory` | 8h | 40 |
| `sysmetrics_bootstrap` | `/v1/ingest/bootstrap` | 24h | 50 |

## Rollback

Rollback primario: publicar a versao anterior do agente.

Rollback por config: manter novos coletores desligados ate os pacotes dependentes. O PKG-59 nao ativa listener novo, nao muda endpoint e nao cria persistencia nova.

## Validacao

Focada:

```bash
go test ./internal/domain/runtime ./internal/domain/usecase ./internal/interfaces/health ./internal/bootstrap
```

Fechamento:

```bash
./check.sh
```

Evidencia real:

- PKG-69 aceito em 2026-06-19 cobre API indisponivel, recuperacao apos retorno da API, rede intermitente, disco cheio/outbox preservada, proxy autenticado, TLS estrito, reboot com replay, Windows, Linux, container e Kubernetes.
- Bundles principais: `docs/evidence/pkg69/linux-rhel-20260619T025113Z`, `docs/evidence/pkg69/disk-full-20260619T034504Z`, `docs/evidence/pkg69/proxy-tls-20260619T033945Z`, `docs/evidence/pkg69/reboot-during-collection-20260619T165440Z`.
