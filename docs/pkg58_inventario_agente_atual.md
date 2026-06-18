# Inventario tecnico atual do agente AIceberg

Documento de suporte ao PKG-58 do `aiceberg_web`. A matriz unica fica em `/Users/brenomayder/projects/web/public/aiceberg_web/docs/agente_datadog_paridade.md`.

Este inventario descreve o estado real inspecionado no agente Go. Ele nao declara paridade completa com Datadog Agent e nao fecha validacao multiplataforma.

## Arquitetura atual

| Camada | Arquivos principais | Responsabilidade atual |
|---|---|---|
| Entrypoint | `cmd/agent/main.go`, `cmd/agent/main_windows.go` | Carregar config e iniciar `app.Run`. |
| Bootstrap/runtime | `internal/bootstrap/app.go`, `internal/bootstrap/runtime_snapshot.go` | Orquestrar coletores, outbox, flush, canal, self-heal, self-update, health e Agentless. |
| Config | `internal/common/config/config.go` | Ler env/env-file/JSON/YAML simples, prefs remotas e variaveis operacionais. |
| Transporte | `internal/data/remote/transport/*` | Enviar HTTP JSON para API ou HUB, com outbox e rotas separadas. |
| Dominio/usecases | `internal/domain/usecase/*` | Coleta, flush, config sync, canal, self-heal, self-update, ping e Agentless. |
| Coletores locais | `internal/platform/collectors/sysmetrics`, `oslogs`, `networkcapture` | Coletar metricas host, logs de SO e contexto de rede. |
| HUB/Agentless | `internal/interfaces/hub`, `internal/platform/agentless` | Receber relay, encaminhar API, buscar jobs Agentless e enviar observacoes. |
| Persistencia local | `internal/data/local/outbox`, `agentless`, `prefs` | Buffer, outbox resiliente, prefs e estado local. |

## Coletores e payloads atuais

| Coletor | Endpoint | Payload/chaves principais | Estado para matriz |
|---|---|---|---|
| `sysmetrics` filtrado como metricas | `/v1/ingest/metrics` | `capabilities`, `cpu`, `memory`, `disk`, `network`, `net_active`, `sanity`, `services`, `processes`, `agent`, `gpu`, `power`, `sensors`, `time_sync` | feito/parcial |
| `sysmetrics` filtrado como health | `/v1/ingest/health` | `capabilities`, `disk`, `updates`, `time_sync`, `sanity`, `vulns`, `logs` | parcial |
| `sysmetrics` filtrado como inventory | `/v1/ingest/inventory` | `capabilities`, `inventory`, `host`, `agent` | parcial |
| `sysmetrics` filtrado como bootstrap | `/v1/ingest/bootstrap` | `capabilities`, `inventory`, `host`, `network` | feito |
| `networkcapture` | `/v1/ingest/network_capture` | sockets, peers, listeners, DNS, riscos, processo/servico quando disponivel | parcial |
| `oslogs` Unix | `/v1/logs/raw` | eventos de arquivo, syslog parseado, cursor local | parcial |
| `oslogs` Windows | `/v1/logs/raw` | canais Event Log via `wevtutil`, record id, provider/event id quando disponivel | parcial |
| Agentless SNMP HUB | `/v1/hub-agentless/jobs`, `/v1/hub-agentless/observations` | jobs remotos, observacoes SNMP, segmentos e perfis vendor | feito/parcial |

## Endpoints consumidos pelo agente

| Endpoint | Uso atual | Compatibilidade |
|---|---|---|
| `/v1/agent/bootstrap` | Registro/bootstrap com token e versao | Manter fallback e token atual. |
| `/v1/agent/ping` | Long-poll/ack de desafio legado | Manter ate canal substituir com homologacao real. |
| `/v1/agent/config` | Puxar prefs remotas e update payload | Novas configs devem ser opcionais e aditivas. |
| `/v1/agent/channel` | Canal operacional HTTP equivalente a canal persistente | Comandos novos exigem allowlist, escopo e idempotencia. |
| `/v1/agent/selfheal-commands` | Buscar comandos de auto-remediacao permitidos | Nao aceitar shell/script arbitrario. |
| `/v1/agent/selfheal-report` | Reportar execucao de comando permitido | Manter trilha de status. |
| `/v1/agent/error-report` | Reportar erro operacional do worker | Sem segredo em logs/payload. |
| `/v1/agent/update-report` | Reportar etapas de update | Manter precheck, falha, retry e confirmacao de versao. |
| `/v1/agent/update/download` | Proxy de download via HUB quando necessario | Validar tamanho, sha256 e auth. |
| `/v1/ingest/*` | Envio de snapshots segmentados | Nao quebrar `agente_snapshot*`. |

## Configuracoes remotas atuais

`config.CollectPrefs` ja cobre:

- `cpu`, `memory`, `disk`, `network`, `net_active`, `host`;
- `sensors`, `power`, `sanity`, `gpu`, `services`, `time_sync`;
- `logs`, `updates`, `agent`, `processes`, `vulns`, `inventory`;
- flags avancadas de logs: `oslog_enrich`, `oslog_detections`, `oslog_diag`, `oslog_win_channels`, `oslog_files`;
- Agentless: enable, poll, flush, lock, batch;
- network capture: modo, janela, sample, limites, pcap e fontes externas;
- `collect_now`;
- `cve_signatures_url`.

Lacunas para PKG-59 a PKG-72:

- nao ha runtime separado formal de Collector/Forwarder/Scheduler/Supervisor;
- nao ha listener DogStatsD/HTTP para metricas custom;
- nao ha receptor OTLP;
- nao ha APM/traces;
- nao ha autodiscovery Docker/containerd;
- nao ha Helm/DaemonSet Kubernetes;
- nao ha runtime de plugins/checks locais assinados;
- nao ha flare seguro completo;
- assinatura obrigatoria de config remota sensivel ainda nao esta fechada;
- nao ha eBPF/USM/workload security;
- nao ha catalogo avancado OpenMetrics/JMX/WMI;
- nao ha IA local embarcada no agente.

## Evidencia de compatibilidade atual

- `internal/domain/channel/contract.go` preserva contratos HTTP legados: ping, bootstrap, config, selfheal, update-report e ingest.
- `internal/interfaces/hub/http_hub.go` encaminha rotas de ingest, config, bootstrap, ping, selfheal, error-report, update-report, channel e update/download.
- `internal/domain/usecase/flush_outbox.go` possui isolamento por rota e backoff de falha.
- `internal/bootstrap/runtime_snapshot.go` mascara tokens e segredos em snapshot de runtime.
- `internal/domain/usecase/self_update.go` valida download, sha256, status e reporta etapas.

## Validacao do PKG-58

Validacao focada recomendada:

```bash
go test ./internal/bootstrap ./internal/common/config ./internal/domain/usecase ./internal/platform/collectors/sysmetrics ./internal/platform/collectors/oslogs ./internal/platform/collectors/networkcapture
```

Fechamento:

```bash
./check.sh
```

PKG-58 nao prova Windows, Linux, container ou Kubernetes. Essas evidencias devem ser preenchidas nos pacotes de runtime e consolidadas no PKG-69.
