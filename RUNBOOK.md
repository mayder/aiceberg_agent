## CPU/HUB e envio do relay — candidato0.8.65

Flags: HUB_OUTBOX_RAW_BODY_ENABLED=true somenteHUB e RELAY_OUTBOX_ASYNC_ENABLED=true somenteRelay. False restaura materialização ou loop/timer anterior após reinício autorizado. Worker único aceita sinal só após persistência; aplicação de configuração própria continua no loop principal. Não antecipar ACK, pruning ou retry401. Pacote0.8.64/HUB e versão anterior doRelay são rollback, com outbox preservada; validar pacote assinado/plataforma anterior antes do piloto.

Piloto autorizado deve atualizar primeiroHUB84, mantendoRelay124 antigo para medir a chegada; depois somenteRelay124. Conferir hash/versão e flags por inspector temporário, restaurar observabilidadeOFF e confirmar aplicação. Comparar CPU por delta cpu_time_sec/tempo do mesmoPID, escrita/ACK, fila e latência nos dois saltos. O carimbo meta.hub_received_at_unix_ms é gerado após parse HTTP/antes de Append; representa a recepção desta tentativa, não o primeiro byte nem o instante exato do fsync. Relay→HUB inclui espera de envio, transporte e parse; HUB→API inclui persistência/fila/transporte/ingestão. Comparar received_at UTC explicitamente com epoch, sem UNIX_TIMESTAMP sujeito ao fuso da sessão; manter precisão/clock como ressalva. Sem carimbo anterior, não atribuir os49s históricos a um salto por suposição.

Metadados usam raw_json/retencão existentes; não persistir payload/segredo em logs novos. Nenhuma limpeza, replay, SQL, mudança de credencial, desligamento de coleta ou rollout fora do piloto. Evidência: docs/evidence/hub-cpu-relay-arrival-local-20261104.md.

## Redução de I/O do HUB — candidato0.8.64

HUB_OUTBOX_FREELIST_REBUILD_ENABLED=true somente no HUB omite a gravação da freelist reconstruível do bbolt; dados/metadados continuam fsync e o sucesso local continua posterior ao commit. A abertura/recovery percorre páginas alcançáveis: medir tempo de retomada no piloto, sem esvaziar/compactar a fila. Direct/relay conservam a configuração anterior. Inspector expõe ingest_runtime.hub_outbox_freelist_rebuild_enabled. Opt-out false exige reinício autorizado; pacote0.8.63 lê o mesmo arquivo e reconstrói a freelist automaticamente. Não mudar NoSync. Não declarar CPU resolvida a partir do microbenchmark; medir deltas do mesmo processo e atraso/recibos reais no HUB84 antes de ampliar. Evidência e limites: docs/evidence/hub-freelist-and-normalization-local-20261104.md.

# RUNBOOK.md

## Candidato 0.8.63 — agrupamento de escrita da outbox

HUB_OUTBOX_WRITE_BATCH_ENABLED=true habilita somente no HUB agrupamento durável de Push/ACK (64 callbacks, até 5 ms para formar lote). Atraso de agrupamento não é timeout de commit; sucesso exige commit/fsync. NoSync/NoFreelistSync permanecem desligados. Direct/relay conservam escrita anterior; não há nova retenção/limpeza. Conferir ingest_runtime.hub_outbox_write_batch_enabled, versão, ACK e idade real.

Piloto proposto somente HUB84 após autorização específica de push/publicação/update. Medir janelas equivalentes de I/O por delta do mesmo processo, CPU com denominador explícito, fila/atraso e recibos. Não atribuir redução a CPU média desde startup. Opt-out local pela flag false/restart autorizado ou rollback assinado para 0.8.62, preservando arquivos. Não limpar fila. Evidência: docs/evidence/hub-durable-write-batching-local-20261104.md.


Runbook operacional do projeto.

## Confirmação de configuração — candidato0.8.62

Após aplicar configuração própria via ACK de ingest no HUB assíncrono, a próxima
consulta periódica solicita resposta completa para registrar config-report.
Não há chamada HTTP extra no caminho de recepção, restart ou novo timer. Em
falha do report, nova tentativa fica para a próxima cadência normal; sucesso
retoma GET condicional. Confirmar config_version_applied e a versão publicada,
não apenas send_configuration=0 ou o botão OFF. Rollback para0.8.61 preserva
fila/configuração, mas devolve a lacuna de confirmação. Não ampliar o piloto
além do HUB84 sem nova autorização. Nenhuma publicação efetuada deste candidato.


## Worker do HUB acordado por chegada — candidata local — 04/11/2026

HUB_OUTBOX_ASYNC_ENABLED=true habilita um worker dedicado somente em modo HUB.
Após Store.Push concluir, Append emite sinal não bloqueante em canal de 1 posição.
O worker acorda sem aguardar OUTBOX_FLUSH_INTERVAL; recebimentos durante envio
deixam um sinal pendente. Não se cria worker por envelope nem outro flush inline.
Coletores continuam no loop principal e os próprios intervalos não mudam.

O worker executa main/oslogs sequencialmente, mantendo até 8 grupos independentes
em paralelo dentro do flush,janela de 8s / 8MiB,ACK e cooldown existentes. O timer
normal recupera pendentes/retries e a retomada após progresso continua disponível.
Falha sem ACK limita novos wakes a uma passagem por segundo, respeitando também
o backoff HTTP por grupo; chegadas saudáveis em worker ocioso acordam imediatamente.
Pruning não passa a executar a cada chegada; sua cadência/política são preservadas.

Configuração do próprio HUB recebida no envio vai para o loop principal, junto
do config-sync; só relay config usa o PendingConfigStore concorrente. Essa fila
de configuração tem 1 posição e cancela com `Run`, sem persistência/log adicional.
Ao sair de `Run`, cancelar contexto do listener/worker e aguardar a requisição em
curso até o timeout HTTP configurado; não confirmar ou remover itens não enviados.

Conferir ingest_runtime.hub_async_enabled no inspector, LastAck/LastDeferred,
fila/idade real e coletasAgentless/host. A rota pode ter latência/fila/backoff:
acordar imediatamente não garante persistência imediata na API. Validar Bolt
ativo no piloto; seleção/fallback de storage anterior não foi alterada.

Rollback: HUB_OUTBOX_ASYNC_ENABLED=false e reinício controlado autorizado
voltam ao envio inline anterior; HUB_OUTBOX_CATCHUP_ENABLED é independente.
Não mudar credencial, limites, pruning, registros ou outbox para reverter.
Nenhum push, pacote oficial ou update remoto nesta etapa.

## HUB com backlog após recuperar a ingestão — 03/11/2026

Na candidata local, HUB_OUTBOX_CATCHUP_ENABLED=true permite uma próxima vez em1s
após ACK positivo quando last_read_batch atinge batch_limit ou last_deferred>0.
O intervalo normal OUTBOX_FLUSH_INTERVAL continua15s por padrão. Falha total,
fila vazia, parent context cancelado e modosdirect/relay não antecipam o timer.
O limite8s de despacho/concorrência8/8MiB e cooldown por auth/identidade/rota
permanecem. Pruning conserva a cadência normal, sem nova política de descarte.

Conferir no inspector outbox_runtime.last_read_batch,last_deferred,last_retained,
último ACK e backlog; ingest_runtime.hub_catchup_enabled informa a configuração.
LastError é histórico e retenção não significa sempre timeout/deferral. A queda
de queue_items não prova entrega integral: existe pruning automático anterior.
Comparar ts_unix_ms com received_at emUTC, além de ACK, coletaAgentless/relays,
CPU/I/O do agente e carga/latência da API. Não usar só idade do recebimento da UI.

Rollback local da candidata: HUB_OUTBOX_CATCHUP_ENABLED=false e reinício
controlado pelo procedimento autorizado, ou versão anterior assinada. Não apagar
outbox, alterar token, ownership, retenção ou baseline. Sem novo log/tabela;
campos inteiros em evidência existente, rotação/retenção de logs preservadas.
Esta entrega não gera artefatos nem autoriza push/update remoto.

## Objetivo

Padronizar como rodar, diagnosticar, publicar, validar e reverter o projeto.

## Atualização deste modelo em projeto novo

1. Copiar arquivos para a raiz do projeto.
2. Ajustar `PATHS.toml`.
3. Inspecionar linguagem, framework, estrutura de pastas e convenções existentes.
4. Preencher `ESCOPO.md` com produto, módulos, stack e arquitetura real.
5. Preencher a nomenclatura oficial do projeto em `ESCOPO.md`.
6. Ajustar `QUALITY_ROADMAP.md` para stack real, mantendo SOLID.
7. Ajustar `GOVERNANCA.md` para riscos reais.
8. Trocar exemplos de `DEMANDAS.md` por pacotes reais.
9. Mapear telas reais em `TELAS.md`.
10. Definir testes reais em `TESTES.md`.
11. Registrar decisões iniciais relevantes em `DECISOES.md`.
12. Rodar `./check.sh`.

## Checklist de aplicação em projeto real

- [ ] Confirmar branch e status.
- [ ] Ler `PATHS.toml` antigo se existir.
- [ ] Inspecionar stack, framework e comandos disponíveis.
- [ ] Mapear diretórios runtime reais.
- [ ] Mapear testes existentes.
- [ ] Mapear fixtures/seeds existentes.
- [ ] Mapear estrutura de camadas e imports proibidos.
- [ ] Preencher arquitetura real em `ESCOPO.md`.
- [ ] Preencher nomenclatura oficial.
- [ ] Ajustar `quality.runtime_dirs`.
- [ ] Ajustar `quality.stack`.
- [ ] Ajustar `quality.fixtures`.
- [ ] Ativar `quality.layering` se houver regra verificável.
- [ ] Registrar decisões iniciais em `DECISOES.md`.
- [ ] Rodar `./check.sh`.
- [ ] Corrigir scripts até o check representar a realidade do projeto.

## Adaptação do check por projeto

1. Preencher `quality.runtime_dirs` em `PATHS.toml` com diretórios de código real.
2. Preencher `quality.stack` com comandos da linguagem.
3. Preencher `quality.fixtures` quando houver fixtures, seeds, contrato ou E2E.
4. Ajustar `scripts/validate-file-size.sh` se a stack exigir exceções.
5. Ajustar `scripts/validate-no-runtime-pkg-names.sh` para ignorar fixtures ou docs embutidas.
6. Configurar `scripts/validate-layering.sh` quando o projeto tiver estrutura definida.
7. Manter `check.sh` como orquestrador; evitar colocar todas as regras diretamente nele.
8. Não inventar estrutura nova antes de mapear a arquitetura atual.

## Ambientes

| Ambiente | URL/host | Observações |
|---|---|---|
| Local |  |  |
| Homologação |  |  |
| Produção |  |  |

## Comandos locais

```bash
./check.sh
```

## Variáveis e segredos

- Segredos ficam fora do Git.
- `.env.example` documenta chaves sem valores reais quando existir.
- Nunca registrar token, senha ou chave em log, docs, print ou commit.

## Matriz de paridade do agente

A matriz operacional fica no repo web em:

```txt
/Users/brenomayder/projects/web/public/aiceberg_web/docs/agente_datadog_paridade.md
```

## Descoberta automática de fontes locais

Variáveis locais:

```bash
LOG_DISCOVERY_ENABLED=true
LOG_DISCOVERY_INTERVAL=300
LOG_DISCOVERY_MAX_CANDIDATES=200
LOG_DISCOVERY_MAX_EVIDENCE_BYTES=2048
```

Operação:

1. O coletor `log_source_discovery` envia `log_source_discovery_v1` em `/v1/ingest/metrics`.
2. O comando remoto `collect_now=log_source_discovery` força uma nova varredura bounded/read-only.
3. O agente descobre EventLog/Windows Defender/IIS, journald/systemd, Nginx, Apache, Plesk, Linux auth, bancos, filas, Docker/containerd, Kubernetes básico, OTLP e processos/listeners conhecidos.
4. A descoberta não ativa coleta ampla. A web precisa aprovar a fonte e devolver configuração em `logs.win_channels` ou `logs.files`.
5. Rollback: `LOG_DISCOVERY_ENABLED=false`, remover a fonte aprovada da configuração remota ou publicar versão anterior.

O inventario tecnico local fica em:

```txt
docs/pkg58_inventario_agente_atual.md
```

Antes de implementar PKG-59 a PKG-72:

1. Confirmar dependencia na matriz.
2. Preservar contratos HTTP legados e `/v1/agent/channel`.
3. Adicionar payload novo de forma opcional ou versionada.
4. Criar flag/config de rollback antes de ativar coletor novo.
5. Validar agente instalado anteriormente.
6. Garantir que logs e flare nao exponham token, segredo, payload sensivel ou comando inseguro.

Nao declarar superioridade sobre Datadog sem benchmark, evidencia funcional e comparacao objetiva registrada na matriz.

## Perfil local de performance

O coletor `sysmetrics` envia `performance_profile` em `/v1/ingest/metrics` quando houver evidência mínima ou lacuna útil.

Campos principais:

- `resources`: CPU, memória, maior uso de disco e bytes de rede acumulados por interfaces ativas;
- `processes`: top processos já coletados, com papel provável, CPU, memória, IO acumulado e command line sanitizado;
- `checks`: DNS/TCP de sanity e flush do agente;
- `gaps`: coletores desativados ou indisponíveis.

Segurança:

- command line do perfil mascara `token`, `password`, `passwd`, `secret`, `authorization`, `api_key` e `Bearer`;
- o contrato é aditivo e não cria endpoint novo;
- rollback: remover `performance_profile` de `metricsKeys` ou publicar a versão anterior do agente.

Validação focada:

```bash
go test ./internal/platform/collectors/sysmetrics
```

### PKG-59 - Runtime Collector/Forwarder

Contrato tecnico: `docs/pkg59_runtime_architecture.md`.

Diagnostico local:

```bash
curl -s http://127.0.0.1:<HEALTH_PORT>/health
```

Campos esperados:

- `agent_pipeline_version`;
- `queue_items`;
- `queue_bytes`;
- `flush_detail`;
- `channel`, quando o canal estiver ativo.

Diagnostico remoto seguro:

- usar comando permitido `inspect_runtime_config`;
- conferir `scheduler_snapshot` e `agent_pipeline_version`;
- nao executar shell remoto ou script arbitrario.

Rollback: voltar o artefato do agente para a versao anterior e manter os campos aditivos ignorados pelo backend. O PKG-59 nao exige SQL.

### PKG-60 - Pipeline seguro de logs

Contrato tecnico: `docs/pkg60_logs_pipeline.md`.

Configuracoes locais:

- `OSLOG_ENABLED=true`;
- `OSLOG_FILES=/var/log/auth.log,/var/log/syslog`;
- `OSLOG_WIN_CHANNELS=Security,System,Application,Microsoft-Windows-Sysmon/Operational`;
- `OSLOG_WIN_PROVIDERS=Microsoft-Windows-Security-Auditing`;
- `OSLOG_WIN_EVENT_IDS=4624,4625,4688`;
- `OSLOG_INCLUDE_REGEX`;
- `OSLOG_EXCLUDE_REGEX`;
- `OSLOG_MIN_SEVERITY`.
- `OSLOG_UDP_ADDR` e `OSLOG_TCP_ADDR` para listeners locais opcionais em POSIX.
- `OSLOG_JOURNALD_ENABLED=true`;
- `OSLOG_JOURNALD_UNITS=nginx.service,sshd.service`;
- `OSLOG_JOURNALD_PRIORITIES=warning,error,critical`.
- `OSLOG_PROCESSORS_JSON='[{"type":"drop","pattern":"health check"},{"type":"route","value":"security"}]'`.
- `OSLOG_DUAL_SHIP_ENDPOINTS=/v1/logs/archive`.

Config remota equivalente em `logs.win_channels`, `logs.win_providers`, `logs.win_event_ids`, `logs.include_regex`, `logs.exclude_regex`, `logs.min_severity`, `logs.processors` e `logs.dual_ship_endpoints`.
Listeners locais tambem podem ser enviados como `logs.udp_addr` e `logs.tcp_addr`.
Journald tambem pode ser enviado por `logs.journald_enabled`, `logs.journald_units` e `logs.journald_priorities`.
Dual-shipping aceita apenas endpoints internos `/v1/logs/*`; URL externa e endpoint fora de logs sao descartados pelo agente.

Rollback: desligar logs por `OSLOG_ENABLED=false` ou remover flags remotas `OSLogFiles`/`OSLogWinChannels`. Se a regressao for no binario, publicar a versao anterior.

Nao registrar conteudo descartado por filtro; usar apenas `dropped_count`.

Evidencia controlada:

```bash
scripts/pkg60_logs_controlled_evidence.sh
```

O comando gera `docs/evidence/pkg60/controlled-*/evidence.md`, `MANIFEST.tsv`, `PROVENANCE.tsv` e artefato bruto com logs de teste. A compilacao Windows e registrada por SHA256/tamanho, mas o binario Windows de teste nao fica retido no repo. Essa evidencia nao substitui homologacao real de Graylog, Windows Security, Linux auth e Sysmon.

Gate de lacunas:

```bash
scripts/pkg60_logs_evidence_gap_report.sh docs/evidence/pkg60
PKG60_GAP_REPORT_REQUIRE_COMPLETE=true scripts/pkg60_logs_evidence_gap_report.sh docs/evidence/pkg60
```

O gate aceita a evidencia controlada apenas como suporte funcional. Para fechamento, devem existir bundles reais com `MANIFEST.tsv`, `PROVENANCE.tsv`, `evidence.md` e artefato bruto sanitizado para `pkg60-real-os-files`, `pkg60-real-source-formats` e `pkg60-real-journald-windows-channels`. O aceite final exige `PKG60_ACCEPT_CLOSURE=true` junto de `PKG60_GAP_REPORT_REQUIRE_ACCEPTED=true`.

Estado aceito em 2026-06-19: 3/3 bundles reais presentes e gate retornando `closure_status=ACEITO_PARA_FECHAMENTO` com `PKG60_GAP_REPORT_REQUIRE_ACCEPTED=true PKG60_ACCEPT_CLOSURE=true scripts/pkg60_logs_evidence_gap_report.sh docs/evidence/pkg60`.

### PKG-61 - Metricas custom locais

Contrato tecnico: `docs/pkg61_custom_metrics.md`.

Configuracoes locais:

- `CUSTOM_METRICS_ENABLED=true`;
- `CUSTOM_METRICS_UDP_ADDR=127.0.0.1:8125`;
- `CUSTOM_METRICS_UDS_PATH=/var/run/aiceberg/custommetrics.sock`;
- `CUSTOM_METRICS_HTTP_ADDR=127.0.0.1:8126`;
- `CUSTOM_METRICS_INTERVAL=10`;
- `CUSTOM_METRICS_MAX_SERIES=1000`;
- `CUSTOM_METRICS_MAX_BYTES=65536`.

Rollback: definir `CUSTOM_METRICS_ENABLED=false` ou enviar config remota `custom_metrics.enabled=false`. Nao ha SQL do PKG-61.

Estado aceito em 2026-06-19: `docs/evidence/pkg61/local-app-high-volume-20260619T181900Z` valida app local controlada, HTTP, UDP, UDS `0600`, alta cardinalidade e `dropped_count`; o bundle PKG-69 `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z` valida alto volume com CPU/memoria.

### PKG-62 - OTLP HTTP/JSON

Contrato tecnico: `docs/pkg62_otlp_http_json.md`.

Configuracoes locais:

- `OTLP_ENABLED=true`;
- `OTLP_HTTP_ADDR=127.0.0.1:4318`;
- `OTLP_INTERVAL=10`;
- `OTLP_MAX_ITEMS=1000`;
- `OTLP_MAX_BYTES=1048576`.

Rollback: definir `OTLP_ENABLED=false` ou enviar config remota `otlp.enabled=false`. Nao ha SQL do PKG-62.

Estado aceito em 2026-06-19: `docs/evidence/pkg62/example-service-otlp-20260619T182600Z` valida app de exemplo, servico simples instrumentado, metrics/logs/traces, trace correlation e redaction; `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z` cobre alto volume OTLP com CPU/memoria.

### PKG-63 - APM/traces sampling

Contrato tecnico: `docs/pkg63_apm_sampling.md`.

Configuracoes locais:

- `APM_TRACE_SAMPLE_RATE=1`;
- `APM_TRACE_SLOW_THRESHOLD_MS=1000`;
- `APM_TRACE_PRESERVE_ERRORS=true`.

Rollback: definir `APM_TRACE_SAMPLE_RATE=1` para enviar todos os spans ou desligar OTLP com `OTLP_ENABLED=false`.

Estado aceito em 2026-06-19: `docs/evidence/pkg63/apm-high-volume-error-20260619T183604Z` valida alto volume controlado, erro de aplicacao, preservacao de span lento/erro por sampling e jornada log -> trace -> service -> host. Overhead antes de ativacao ampla fica coberto pelo bundle PKG-69 `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z`. Profiler permanece fora do escopo inicial por decisao registrada.

### PKG-64 - Containers Docker inicial

Contrato tecnico: `docs/pkg64_containers.md`.

Configuracoes locais:

- `CONTAINER_ENABLED=true`;
- `CONTAINER_RUNTIME=auto|docker|containerd`;
- `CONTAINER_DOCKER_SOCKET=/var/run/docker.sock`;
- `CONTAINER_CONTAINERD_SOCKET=/run/containerd/containerd.sock`;
- `CONTAINER_CONTAINERD_NAMESPACE=k8s.io`;
- `CONTAINER_CTR_PATH=ctr`;
- `CONTAINER_INTERVAL=30`;
- `CONTAINER_MAX_ITEMS=200`.
- `CONTAINER_INCLUDE_REGEX` e `CONTAINER_EXCLUDE_REGEX` para restringir imagem, nome, labels, namespace, service e user.
- `CONTAINER_LOGS_ENABLED=true` para coletar logs Docker JSON com cursor e redaction.

Rollback: definir `CONTAINER_ENABLED=false` ou config remota `containers.enabled=false`. Nao ha SQL do PKG-64.

Estado aceito em 2026-06-19: `docs/evidence/pkg64/container-lifecycle-autodiscovery-secret-20260619T184258Z` valida container parado, reiniciado, alta carga, autodiscovery por label e ausencia de env/volume sensivel no payload. Docker real/logs/cursor/cleanup ficam cobertos por `docs/evidence/pkg69/docker-runtime-20260619T031843Z`.

### PKG-65 - Kubernetes DaemonSet e Helm inicial

Contrato tecnico: `docs/pkg65_kubernetes.md`.

Instalacao:

- manifest direto: `deploy/kubernetes/aiceberg-agent.yaml`;
- Helm chart: `deploy/helm/aiceberg-agent`;
- secret obrigatorio: `aiceberg-agent/token`.

Configuracoes principais:

- `KUBERNETES_ENABLED=true`;
- `KUBERNETES_NODE_NAME` via `spec.nodeName`;
- `KUBERNETES_NAMESPACE` opcional;
- `KUBERNETES_INTERVAL=30`;
- `KUBERNETES_MAX_ITEMS=500`;
- `KUBERNETES_MAX_EVENTS=100`.
- `KUBERNETES_LOGS_ENABLED=false`;
- `KUBERNETES_LOGS_MAX_LINES=200`;
- `KUBERNETES_LOGS_MAX_BYTES=262144`;
- `KUBERNETES_LOGS_INCLUDE_REGEX` e `KUBERNETES_LOGS_EXCLUDE_REGEX` para limitar logs por namespace, pod, container, imagem, node e labels.

RBAC minimo: leitura de `nodes`, `pods`, `events` e `get` em `pods/log` quando logs forem habilitados. Nao conceder `secrets`, `exec`, `update`, `patch` ou `delete` sem nova decisao.

Rollback: `helm uninstall aiceberg-agent -n aiceberg`, `kubectl delete -f deploy/kubernetes/aiceberg-agent.yaml` ou config remota `kubernetes.enabled=false`. Nao ha SQL do PKG-65.

Estado aceito em 2026-06-19: `docs/evidence/pkg65/kubernetes-payload-autodiscovery-metrics-20260619T185052Z` valida node, pod, container, event, log com redaction, annotations de autodiscovery, Metrics API opcional e ausencia de volume sensivel no payload. DaemonSet/Helm/RBAC/rollback reais ficam cobertos por `docs/evidence/pkg69/kubernetes-rbac-20260619T041959Z`.

### PKG-66 - Runtime de checks locais

Contrato tecnico: `docs/pkg66_local_checks.md`.

Configuracoes locais:

- `LOCAL_CHECKS_ENABLED=true`;
- `LOCAL_CHECKS_INTERVAL=30`;
- `LOCAL_CHECKS_MAX_CHECKS=100`;
- `LOCAL_CHECKS_MAX_BYTES=1048576`;
- `LOCAL_CHECKS_MANIFEST_DIRS=./integrations/localchecks/manifests,/etc/aiceberg/localchecks.d`;
- `LOCAL_CHECKS_JSON=[...]`.

Tipos permitidos: `http`, `tcp`, `openmetrics`, `jmx`, `postgresql`, `mysql`, `redis`, `nginx`, `apache`, `iis_wmi`, `windows_service`.

Instalar/remover integracao sem rebuild: adicionar/remover manifest JSON em diretorio de `LOCAL_CHECKS_MANIFEST_DIRS`. O agente carrega apenas metadados validados e ignora manifests com permissao de shell/exec/comando.

Rollback: definir `LOCAL_CHECKS_ENABLED=false` ou config remota `local_checks.enabled=false`. Nao ha SQL do PKG-66.

Estado aceito em 2026-06-19: `docs/evidence/pkg66/localchecks-lifecycle-rollback-upgrade-20260619T185846Z` valida criacao, execucao, falha, bloqueio de tipo fora da allowlist, rollback por config, upgrade de manifest e preservacao de config sem vazamento de credencial.

### PKG-67 - Fleet, rollout e flare seguro

Contrato tecnico: `docs/pkg67_fleet_rollout_flare.md`.

Diagnostico:

- `inspect_runtime_config` mostra `fleet_runtime`;
- `collect_support_flare` coleta evidencias sanitizadas;
- acompanhar update remoto por `/v1/agent/update-report`.

Politica operacional:

- rollout global exige canario;
- update precisa `version`, `url` e `sha256`;
- rollback usa artefato anterior conhecido e acompanha `version_confirmed`;
- flare nao pode conter token, secret, password, authorization ou cookie.

Rollback: desabilitar auto-update remoto e o comando `collect_support_flare` no backend. Nao ha SQL do PKG-67.

### PKG-84 - Update resiliente e diagnosticavel

Diagnostico:

- `inspect_runtime_config` mostra `auto_update.pending_state` quando ha update despachado aguardando reconexao;
- `pending_state.rollback_available=true` e `rollback_version` indicam que a versao anterior conhecida foi preservada no contrato local;
- `update-report` inclui `rollback_available`/`rollback_version` nos reports de reconexao e `rolled_back`;
- diretórios antigos de staging em `AUTO_UPDATE_DIR` sao limpos apos validacao de artefato, preservando a versao atual, diretorios recentes e arquivos `.pending_update.json`/`.update_cooldown.json`.

Operacao segura:

- download valida SHA256 e assinatura antes do apply;
- arquivo `.part` pode ser retomado com `Range` quando o servidor suporta `206 Partial Content`;
- scripts oficiais Linux fazem backup do binario atual antes da troca e tentam rollback se o restart falhar;
- em Linux, o destino do update deve ser o mesmo binario em execucao (`AICEBERG_UPDATE_BIN_DST`, fallback `AICEBERG_AGENT_BIN`, fallback legado `/usr/local/bin/aiceberg_agent`);
- falha repetida deve respeitar `.update_cooldown.json`, evitando loop quente apos restart.

Rollback:

- desativar auto-update remoto no backend;
- usar comando manual oficial gerado pela web para reinstalar a versao anterior quando o launcher nao conseguir restaurar;
- apagar `.update_cooldown.json` somente apos diagnostico aprovado e quando for necessario liberar nova tentativa controlada.

### PKG-68 - Seguranca e assinatura

Contrato tecnico: `docs/pkg68_security_hardening.md`.

Variaveis:

- `REMOTE_CONFIG_SIGNATURE_SECRET`;
- `REMOTE_CONFIG_SIGNATURE_REQUIRED=true`;
- `REMOTE_CONFIG_ALLOW_UNSIGNED_SENSITIVE=false`;
- `AUTO_UPDATE_TRUST_REQUIRED=true`;
- `AUTO_UPDATE_TRUST_PUBLIC_KEY=<ed25519-public-key-hex-ou-base64>`;
- `TLS_INSECURE_SKIP_VERIFY=false`;
- `TLS_INSECURE_ALLOW_PROD=false`.

Regras:

- payload sensivel sem assinatura deve ser rejeitado quando segredo de assinatura existir;
- downgrade de update sem `force=true` e bloqueado;
- update com assinatura/cadeia de artefato invalida deve falhar antes do `apply`;
- `token_rotation.new_token` nunca deve aparecer em log;
- revogacao/transicao backend do token antigo e operada pelo web via `sql/2026_06_18_pkg68_agent_token_rotation.sql`;
- `security_runtime` deve indicar politica sem expor segredo;
- proxy corporativo segue `HTTP_PROXY`, `HTTPS_PROXY` e `NO_PROXY`.

Rollback: desativar obrigatoriedade de assinatura ou permitir unsigned sensitive apenas durante janela controlada. Nao ha SQL no repo do agente; o SQL de transicao de token fica no `aiceberg_web`.

### PKG-69 - Matriz operacional real

Contrato tecnico: `docs/pkg69_operational_matrix.md`.

Comando local:

```bash
scripts/pkg69_operational_homologation.sh
```

O comando valida ambiente local, testes focados, burst local de cardinalidade custom metrics, testes dedicados de API indisponivel, backoff/rede intermitente, payload grande e outbox cheia, topologia relay -> hub -> AIceberg para canal/ping/self-heal/update, update local com `version_confirmed` e mismatch apos rollback, proxy autenticado e rejeicao padrao de TLS invalido, e2e multi-processo direct/hub/relay, smoke POSIX com RSS/CPU/goroutines locais, `./check.sh`, e lista pendencias reais por Windows, Linux, Docker, Kubernetes, proxy, disco, payload, alto volume com CPU/mem e update rollback.

Gate de evidencias reais:

```bash
PKG69_TEMPLATE_DIR=/tmp/aiceberg_pkg69_templates \
PKG69_EVIDENCE_MANIFEST_TSV=/tmp/aiceberg_pkg69_manifest.tsv \
scripts/pkg69_operational_evidence_gate.sh
```

Para empacotar uma evidencia real ja coletada, use `scripts/pkg69_bundle_evidence.sh <cenario> <template-preenchido.md> <arquivo-ou-diretorio-bruto> [saida]`. O helper aceita apenas os cenarios oficiais do gate PKG-69, grava `MANIFEST.tsv` e `PROVENANCE.tsv`; cenario desconhecido falha antes de gerar bundle.

Para coletar evidencia read-only no proprio host e gerar o bundle, use `scripts/pkg69_collect_host_evidence.sh <cenario> <template-preenchido.md> [saida]`. O artefato bruto inclui `README.tsv`, `COMMANDS.tsv`, `COLLECTION_SUMMARY.tsv` e ambiente redigido para auditoria da coleta. Com `PKG69_RUN_SMOKE=true`, o script tambem executa `scripts/smoke.sh` e anexa log/JSON ao artefato bruto. Em host sem Go, `scripts/smoke.sh` aceita `SMOKE_AGENT_BIN` e `SMOKE_BACKEND_BIN` para usar binarios pre-compilados sem instalar toolchain no ambiente validado. O gate continua obrigatorio depois da coleta.

Para rodar o gate a partir de uma pasta com bundles ja coletados, use `scripts/pkg69_run_evidence_gate_from_bundles.sh <pasta-bundles>`. O script valida que cada `MANIFEST.tsv` tem uma unica linha de evidencia, `PROVENANCE.tsv` com ferramenta/cenario/data/artefato coerentes, `template` coerente com `evidence.md`, `created_at_utc` em formato compacto `YYYYMMDDTHHMMSSZ`, artefato do manifest coerente com `Evidencia bruta anexada`, confere SHA256/tamanho do `evidence.md` e do anexo bruto, exige `COLLECTION_SUMMARY.tsv` e `COMMANDS.tsv` quando o bundle veio do coletor read-only `raw-host`, valida o cenario do resumo contra o manifest e exige contadores numericos, coerentes e iguais aos comandos registrados, mapeia cada cenario para a variavel `PKG69_*_EVIDENCE` correta e chama o gate oficial; com `PKG69_REQUIRE_REAL_EVIDENCE=true`, continua falhando enquanto qualquer cenario obrigatorio estiver ausente.

Para gerar um relatorio acionavel de lacunas, use `scripts/pkg69_evidence_gap_report.sh <pasta-bundles>`. Para o estado versionado no repo, use `scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69`. O relatorio Markdown lista cenario, status, motivo, proxima acao e veredito explicito de fechamento (`BLOQUEADO`, `PRONTO_PARA_REVISAO` ou `ACEITO_PARA_FECHAMENTO`) a partir do manifest do gate; o stdout tambem publica `closure_status`, `closure_reason` e `closure_acceptance` para automacao. Para usar como bloqueio de fechamento/CI, execute com `PKG69_GAP_REPORT_REQUIRE_COMPLETE=true`; o comando falha enquanto qualquer cenario estiver pendente. Para bloquear aceite sem revisao explicita, execute tambem com `PKG69_GAP_REPORT_REQUIRE_ACCEPTED=true`; esse modo ativa rejeicao de marcadores sinteticos no gate e so passa quando `PKG69_ACCEPT_CLOSURE=true`.

Para fechamento, preencher os templates dos ambientes reais e rodar com `PKG69_REQUIRE_REAL_EVIDENCE=true` e `PKG69_REQUIRE_CLOSURE_ACCEPTED=true`. O pacote nao deve ser marcado como 100% enquanto o gate reportar `pkg69-status: not-closed`. Cada template precisa preencher `Data UTC` em formato `YYYY-MM-DDTHH:MM:SSZ`, `Topologia` com `direct -> AIceberg`, `hub -> AIceberg`, `relay -> hub -> AIceberg` ou `direct/hub/relay hosts separados`, apontar `Evidencia bruta anexada` para arquivo/diretorio local existente e nao vazio, e registrar rollback validado. O manifest TSV inclui SHA256/tamanho do template e do anexo bruto e `created_at_utc` em formato `YYYYMMDDTHHMMSSZ`. Na evidencia de Relay/Hub/Direct, `direct_host_id`, `hub_host_id` e `relay_host_id` precisam ser distintos, `relay_upstream_host_id` precisa ser igual a `hub_host_id`, os campos textuais de direct, hub, relay via Hub, relay sem conexao direta e agentless via Hub devem ser `yes|true|sim`, e `relay_direct_api_attempts` precisa ser `0`. Evidencia com CPU/RSS acima dos limites iniciais, Kubernetes permitindo `secrets`, `exec` ou `delete`, ou campos de responsavel/lab/versao/artefato/observacao/revisor contendo marcador de self-test/sintetico/fake/mock/placeholder nao passa no gate real.

Validacao real fechada em 2026-06-19: `scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69` mapeia 14/14 cenarios reais. O fechamento aceito exige `PKG69_GAP_REPORT_REQUIRE_ACCEPTED=true PKG69_ACCEPT_CLOSURE=true scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69`, que retorna `closure_status=ACEITO_PARA_FECHAMENTO` e `closure_acceptance=accepted`. As evidencias versionadas cobrem Windows Server, Windows desktop, Linux Debian/RHEL, Docker, Kubernetes, API indisponivel/recuperacao, rede/backoff, proxy/TLS, disco cheio/outbox preservada, payload/alto volume, clock skew, permissao eBPF/PCAP restrita, update remoto/rollback, Relay/Hub/Direct e reboot real durante coleta.

Rollback: nao altera runtime; se a validacao real falhar, reabrir pacote tecnico correspondente e manter artefato anterior.

### PKG-70 - Rede avancada, USM e workload security

Contrato tecnico: `docs/pkg70_network_usm_workload.md`.

Configuracoes principais:

- `network_advanced_enabled=true`;
- `usm_enabled=true`;
- `workload_security_enabled=true`;
- `network_passive_mode=auto|socket|netlink|pcap|ebpf`;
- `network_pcap_enabled=true` quando PCAP for permitido.

Operacao segura:

- manter desligado por padrao;
- ativar primeiro em canario;
- validar permissao de host antes de `pcap` ou `ebpf`;
- tratar `workload_security.signals` como evidencia SOC/NOC, sem bloqueio automatico;
- revisar overhead de CPU/memoria antes de ativacao ampla.

Estado aceito em 2026-06-19: `docs/evidence/pkg70/network-usm-workload-20260619T192000Z` valida em ambiente controlado o fluxo `web -> api -> postgres`, dependencias, fallback sem eBPF, contrato de `ebpf_probe`, porta administrativa publica degradada, sinais SOC evidence-only, `source_score` e redaction dos blocos NPM/workload. Os bundles reais do PKG-69 complementam permissao eBPF restrita, Docker, Kubernetes e overhead operacional. Nao declarar eBPF kernel ativo produtivo como obrigatorio ou superior.

Rollback: definir `network_advanced_enabled=false`, `usm_enabled=false`, `workload_security_enabled=false`, `network_pcap_enabled=false` e `network_passive_mode=socket`. Nao ha SQL do PKG-70.

### PKG-71 - Integracoes avancadas

Contrato tecnico: `docs/pkg71_advanced_integrations.md`.

Catalogo: `integrations/localchecks/catalog.json`.

Manifestos oficiais iniciais:

- `integrations/localchecks/manifests/openmetrics.json`;
- `integrations/localchecks/manifests/redis.json`.

Operacao segura:

- ativar `official` primeiro;
- usar `credentials_ref`, nunca senha inline;
- usar `metric_allowlist` e `label_allowlist` em OpenMetrics;
- manter `jmx` em `mode=jolokia`;
- tratar `iis_wmi` e `windows_service` como experimentais ate Windows Server real.

Estado aceito em 2026-06-19: `docs/evidence/pkg71/advanced-integrations-20260619T200500Z` valida em ambiente controlado OpenMetrics `/metrics`, JMX/Jolokia, guard WMI/IIS fora de Windows, PostgreSQL/RabbitMQ reachability, MySQL com falha controlada, Nginx HTTP, bloqueio beta sem homologacao e ausencia de vazamento de `credentials_ref`/metrica negada. Windows Server real e credenciais produtivas continuam exigindo homologacao por cliente antes de ativacao ampla.

Rollback: remover a entrada da integracao em `local_checks` ou definir `local_checks_enabled=false`. Nao ha SQL do PKG-71.

### PKG-72 - Evidencia contextual, IA local e offline

Contrato tecnico: `docs/pkg72_contextual_ai_offline.md`.

Diagnostico:

- `inspect_runtime_config` inclui `contextual_evidence`;
- `collect_support_flare` inclui o mesmo snapshot sanitizado;
- `superiority_benchmark.claim_allowed=false` ate benchmark real.
- `scripts/pkg72_contextual_evidence_homologation.sh` gera evidencia local, lista explicitamente pendencias reais e registra SHA256/tamanho das evidencias anexadas.
- `PKG72_TEMPLATE_DIR=/tmp/aiceberg_pkg72_templates scripts/pkg72_contextual_evidence_homologation.sh` gera modelos editaveis das cinco evidencias reais.
- `PKG72_REQUIRE_REAL_EVIDENCE=true scripts/pkg72_contextual_evidence_homologation.sh` deve ser usado como gate bloqueante antes de tentar fechar o pacote.
- Estado aceito em 2026-06-19: `docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z` passou com os cinco anexos exigidos e `PKG72_ACCEPT_CLOSURE=true`. O fechamento valida diferencial funcional e governanca; superioridade sobre Datadog segue bloqueada por `claim_allowed=false` ate haver benchmark produtivo comparavel por cenario.

Configuracoes opcionais:

- `PRIVACY_PROFILE=standard|sensitive|minimal`;
- `SENSITIVE_MODE=true`.

Operacao segura:

- IA local e deterministica, sem LLM obrigatorio;
- nenhuma acao destrutiva automatica;
- usar evidencia e lacunas como apoio NOC/SOC;
- validar replay offline e correlacao agentless em ambiente do cliente antes de declarar claim comercial por cenario.
- para anexar evidencias reais ao roteiro, use `PKG72_INCIDENT_EVIDENCE`, `PKG72_REPLAY_24H_EVIDENCE`, `PKG72_REGULATED_CLIENT_EVIDENCE`, `PKG72_NOISE_COST_EVIDENCE` e `PKG72_DATADOG_BENCHMARK_EVIDENCE`.

Rollback: ignorar `contextual_evidence` no backend. Nao ha SQL do PKG-72.

### PKG-73 - Taxonomia SOC de logs

Contrato tecnico: `docs/pkg73_soc_log_origin.md`.

O agente adiciona contrato SOC canonico aos eventos de log coletados por `oslogs`, journald, Windows EventLog, OTLP logs, Docker logs e Kubernetes pod logs. Campos novos sao aditivos: `aiceberg_transport`, `aiceberg_tool_origin`, `aiceberg_source_category`, `aiceberg_soc_source_type`, `aiceberg_soc_eligible`, `aiceberg_origin_confidence` e `aiceberg_route_reason`.

Overrides seguros:
- logs estruturados podem informar `aiceberg_*` no evento;
- Docker usa labels `aiceberg.ai/*` ou `aiceberg.com/*`;
- Kubernetes usa annotations `aiceberg.ai/*` ou `aiceberg.com/*`;
- config remota pode usar `logs.processors`, sujeita a assinatura/escopo quando sensivel.

Rollback: desligar logs por `OSLOG_ENABLED=false`, remover overrides `aiceberg.*` ou publicar versao anterior. O backend deve continuar aceitando payload legado.

## Deploy/publicação

1. Confirmar branch e diff.
2. Rodar `./check.sh`.
3. Confirmar scripts SQL necessários.
4. Executar deploy conforme stack do projeto.
5. Rodar smoke test.
6. Monitorar logs.

### Publicação assinada do agente

A chave privada oficial fica fora do repositório e nunca deve ser copiada para pacote, agente, servidor web ou log. O build recebe apenas o caminho local por variável de ambiente:

```bash
AICEBERG_UPDATE_SIGNING_REQUIRED=true \
AICEBERG_INSTALLERS_OUTPUT_DIR=/caminho/novo/da/release \
AICEBERG_UPDATE_SIGNING_PRIVATE_KEY=/caminho/privado/private.pem \
AICEBERG_UPDATE_SIGNING_KEY_ID=aiceberg-agent-prod-v1 \
./scripts/build_installers.sh

go run ./cmd/update-signer verify \
  -manifest dist/UPDATE_SIGNATURES.json \
  -public-key /caminho/privado/public.pem \
  -artifact-dir dist

(cd dist && shasum -a 256 -c SHA256SUMS)
```

Publicar juntos os cinco compactados, `SHA256SUMS` e `UPDATE_SIGNATURES.json`. O backend só deve agendar pacotes cujo manifesto, SHA256 e assinatura estejam válidos. Novas instalações recebem a chave pública pelo instalador. Desde `0.8.48`, instalações existentes sem `agent.env` usam a chave pública oficial e a exigência de assinatura compiladas no binário; a chave privada nunca é distribuída.

O build recusa saída já existente e não apaga pacotes anteriores. Usar o mesmo novo
diretório nos comandos de verificação de manifesto e SHA256 acima. Candidato0.8.58
inclui readiness e scripts macOS; versão no código não implica publicação.

Rotação sem indisponibilidade:

1. gerar novo par fora do Git e preservar a chave anterior para rollback;
2. publicar versão intermediária que confie nas chaves atual e nova;
3. confirmar a versão intermediária na frota;
4. assinar a versão seguinte com a chave nova;
5. remover a chave anterior somente após a janela de rollback.

A rotação da chave compilada exige uma versão intermediária assinada pela chave ainda confiável. Não substituir a constante pública e assinar o mesmo release apenas com a chave nova.

Rollback: restaurar configuração e binário anteriores. Não apagar a chave antiga enquanto houver agentes ou pacotes válidos dependentes dela.

Para ingestão, o agente `0.8.44+` divide grupos da outbox em requisições JSON de até 8 MiB. A partir de `0.8.46`, envelope individual excessivo de `/v1/logs/raw` também é dividido por `body.events`: a outbox substitui o original pelas partes atomicamente e preserva metadados, identidade e autorização. Evento individual que exceda o limite ou corpo sem `events` permanece retido e diagnosticado. Após rollout, conferir `last_flush_batch`, fila por endpoint, log `oversized log envelope split` e ausência de HTTP 400/413. Rollback é retornar à versão anterior; a outbox já fracionada continua válida e não deve ser apagada.

## Smoke test mínimo

- Health/readiness ou página principal responde.
- Login ou fluxo principal abre.
- Ação principal do pacote funciona.
- Logs não exibem erro crítico.

## Diagnóstico e observabilidade

Para fluxos críticos, o runbook deve explicar:

- onde ver logs;
- como filtrar por `request_id`, `correlation_id`, `job_id` ou equivalente;
- onde ver auditoria quando existir;
- quais métricas indicam falha;
- como diferenciar erro de usuário, erro de integração e erro interno;
- política de retenção;
- como funciona a limpeza automática.

Regra operacional:

- não criar tabela nova de log/auditoria se estrutura existente resolver;
- se criar tabela, documentar retenção e cleanup;
- não guardar payload sensível completo.

## Rollback

1. Identificar versão anterior estável.
2. Reverter deploy/código.
3. Executar rollback SQL somente quando documentado e necessário.
4. Rodar smoke test.
5. Registrar causa e ação tomada.

## Incidentes

Modelo:

```txt
Data/hora:
Ambiente:
Sintoma:
Impacto:
Causa provável:
Ação imediata:
Rollback:
Validação:
Próximo ajuste:
```

## Readiness EDR/NDR

Ativação:

- Caminho principal: receber da web/API o bloco `edr_ndr` no `GET /v1/agent/config`.
- O agente persiste em prefs: modo seguro, perfil, exigência de assinatura da config remota, exigência de assinatura do auto-update e exigência de prova de identidade.
- Fallback local: `EDR_SAFE=true` ou `AICEBERG_EDR_SAFE=true`.
- Perfil fallback: `EDR_SAFE_PROFILE=conservative|standard|crowdstrike|darktrace|defender`.

Comportamento esperado:

- Preserva métricas essenciais, saúde, inventário mínimo, logs `error+`, update seguro e snapshots.
- Limita discovery/coletas volumosas e reduz ruído local.
- Não executa shell remoto arbitrário, ação destrutiva, varredura ampla ou coleta de segredo.
- Emite `edr_ndr_readiness` no snapshot runtime com manifesto, política, allowlist mínima, lacunas e validação.

Validação:

```bash
go test ./internal/bootstrap ./internal/common/config ./internal/domain/usecase ./internal/platform/collectors/logdiscovery ./internal/platform/collectors/oslogs
./check.sh
```

Rollback:

- Remover `EDR_SAFE`/`AICEBERG_EDR_SAFE`.
- Reverter configuração remota para perfil padrão.
- Remover o bloco `edr_ndr` ou desligar as flags na web.
- Publicar versão anterior do agente se o modo seguro afetar coleta essencial.

## Observabilidade controlada

Ativação:

- Receber da web/API `custom_metrics.validation_samples_enabled=true` e `otlp.validation_samples_enabled=true`.
- `custom_metrics.enabled` e `otlp.enabled` também precisam estar ativos.

Comportamento esperado:

- `custommetrics` emite uma métrica por ciclo com nome `aiceberg.validation.custom_metrics`, serviço `aiceberg-agent-validation` e origem `agent_controlled_sample`.
- `otlp_traces` emite um span por ciclo com atributo `aiceberg.validation_sample=true`.
- Nenhum shell remoto é executado e nenhum arquivo local do cliente é lido para gerar essa amostra.

Validação:

```bash
go test ./internal/domain/usecase ./internal/platform/collectors/custommetrics ./internal/platform/collectors/otlp
```

Rollback:

- Desligar as flags `validation_samples_enabled` na configuração remota.
- Aguardar novo ciclo de configuração e snapshot.

## Coleta de segurança WordPress por access log

- Requer arquivo Nginx no formato combined e permissão somente leitura para o usuário do agente.
- Adicione o usuário ao grupo proprietário do log; não relaxe o modo do arquivo e não execute o agente como root.
- Em fonte nova com histórico já reconstruído, pare somente o agente, faça backup do cursor, grave identidade e EOF atuais, instale a versão nova e reinicie somente o agente.
- Configure o path pela configuração remota. Confirme versão aplicada, cursor avançando e ausência de valores de query no payload.
- O agente coleta REST batch, criação REST de usuário, login inferido por redirect, upload/ativação de plugin e requisição executável com chave de comando. Ele não cria caso; a correlação pertence ao backend.

Rollback: retirar o path remoto, restaurar cursor/binário anteriores e remover o grupo adicional do usuário do agente.

## Adaptacao deste projeto

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
- Banco de dados: nao usar migrations. Mudancas devem ser scripts `.sql` idempotentes quando possivel, com ordem e rollback.


## Agentless — rollout de capacidade

Publicar a versão assinada `0.8.51` após check/commits. A `0.8.50` confirmou no HUB 10 o bloqueio de preflight do lançador via `env` e não foi instalada. Atualizar primeiro apenas HUB 10, preservando binário/configuração para rollback. Confirmar versão reportada, serviço e fila; só depois aumentar `agentless_jobs_limit` e a política Web do mesmo cliente/HUB. Começar com lote 20 e medir tempo total frente ao lock de 600s, consumo e atraso. Reduzir/reverter se fila ou atraso crescerem. Nunca habilitar todo o inventário com base no benchmark local.

A `0.8.52` preserva o código da `0.8.51` e usa um novo alvo assinado para contornar somente o cooldown local, indexado por versão, deixado pelas tentativas de bootstrap do HUB 10. Ela não amplia o rollout: continuar restrito ao HUB 10 até confirmação real.

O coletor usa oito workers por destino distinto e drena até oito lotes por flush. A fila pode pausar fetch de rotina para priorizar envio; falha HTTP não deve remover observações. Arquivo Bolt maior que o limite configurado não prova fila cheia: páginas livres podem ser reutilizadas, e o limite considera bytes dos registros. Não apagar outbox como correção de capacidade.

Dependências externas (UDP/161, credenciais, gerência dos dispositivos) devem permanecer identificadas por ativo. A mitigação Web com WALK limitado continua válida até retestar GET na versão corrigida.

## Canário do polling condicional e recuperação

1. Registrar por host uma janela anterior equivalente: CPU, requisições por rota, 5xx e versão reportada.
2. Atualizar um único agente Linux acessível, preservando binário/configuração anteriores para rollback; confirmar serviço, health, versão e drenagem da outbox.
3. Após pelo menos dois ciclos de configuração, medir a proporção de `GET /v1/agent/config` com `204`; sem comando/config pendente, ela deve predominar.
4. Confirmar redução proporcional de `POST /v1/agent/config-report` e ausência de `recovered` periódico em `POST /v1/agent/error-report`.
5. Disparar de forma controlada `collect_now`, update e self-heal e confirmar que continuam passando pelo retorno `200` e concluindo normalmente.
6. Comparar CPU, taxa total de requisições e erros 5xx. Só ampliar se não houver perda de coleta, comando, update, Agentless ou self-heal.

Rollback: restaurar o binário assinado anterior e reiniciar somente o serviço do agente. Não apagar prefs, token, outbox ou evidências; a versão condicional e o jitter não exigem mudança no Web/Runtime.

### Diagnóstico de fila HUB sem acesso local

A partir de 0.8.55, `inspect_runtime_config` inclui `outbox_runtime`: quantidade/bytes, último ACK, rota e classe de erro, retenção do último lote e backoff. O snapshot é somente leitura e não contém erro bruto nem conteúdo de envelope. O comando continua no canal autenticado existente. Em fila cheia, o HUB responde503/Retry-After e o relay mantém seu lote. Não limpar ou reproduzir filas manualmente; conferir escoamento e novas métricas antes de declarar recuperação. Rollout inicial restrito ao HUB84/SADA; rollback para pacote assinado anterior preservando estado.

### Capacidade de envio HUB —0.8.56

`HUB_OUTBOX_FLUSH_BATCH` default400 (máximo400), `HUB_OUTBOX_FLUSH_CONCURRENCY` default4 (máximo8). Aplicam-se somente ao modo HUB; relay/direct mantêm lote anterior e envio serial. Grupos são separados por autorização, identidade e rota; chunks do grupo permanecem sequenciais, callback de configuração serializado, falhas retidas. `outbox_runtime` informa capacidade efetiva. Rollback operacional: valores50/1 no ambiente do HUB ou pacote0.8.55 assinado. Não alterar armazenamento, retenção, filas centrais ou ownership.

O envio paralelo limita o despacho a oito segundos por ciclo, respeita cancelamento e mantém grupos ainda não enviados. Requisições já em andamento permanecem limitadas pelo timeout HTTP configurado.

### Capacidade HUB0.8.57

Piloto84/SADA mensurado com206 relays ativos: defaults HUB_OUTBOX_FLUSH_BATCH=800 (teto800) e HUB_OUTBOX_FLUSH_CONCURRENCY=8 (teto8). Preserva despacho8s, timeout HTTP e estado. Valores50/1 restauram capacidade anterior; pacote0.8.56 permite rollback assinado. Não ampliar rollout além do HUB84 nem limpar outbox.

## Pré-verificação para atualização em lote (candidato local)

Solicitar `inspect_runtime_config` com `payload.update_readiness=true` pelo canal
existente, mantendo as mesmas permissões e allowlist do inspector. Resultado deve
conter `update_readiness.schema=aiceberg-agent-update-readiness.v1` e todos os checks
booleanos aprovados. A ausência do resultado em versão antiga exige preparação.
O probe não baixa/aplica pacote nem reinicia serviço e preserva pendências existentes.

Launchers aceitos: Linux `/usr/local/sbin/aiceberg-agent-update-launcher.sh`, direto
para root ou com `sudo -n`; Windows comando PowerShell `& '<caminho-absoluto-do-launcher>'`
para `aiceberg-agent-update-launcher.ps1`. Não receber shell arbitrário da tela.
No candidato macOS, o launcher é `/Library/AIceberg/sbin/aiceberg-agent-update-launcher.sh`.
Mesmo com preflight aprovado, acompanhar reconnect e versão efetivamente confirmada.

### Instalador e updater macOS (candidato, sem piloto publicado)

Os dois pacotes Darwin passam a conter `install.sh`, plist e scripts launcher/apply.
Usar `sudo /bin/bash ./install.sh` no pacote oficial assinado, previamente validado.
O instalador usa somente `/Library/AIceberg` (bin, sbin, data e agent.env), com serviço
`system/com.aiceberg.agent` em `/Library/LaunchDaemons/com.aiceberg.agent.plist`.
Configuração e dados existentes são preservados; destinos simbólicos são recusados.
Não alterar diretórios do Homebrew nem conceder sudo irrestrito. Serviço executa como root.

Para instalações novas, o env configura assinatura obrigatória, staging em
`/Library/AIceberg/data/updates`, launcher oficial e HEALTH_PORT=8081. Configuração
preexistente deve ser revisada manualmente com esses mesmos parâmetros; token nunca
é exibido. A pré-verificação exige binário canônico, health habilitado e daemon registrado.
Instalação legada em outro caminho exige preparação explícita, sem troca automática.

O launcher agenda job launchd independente, com ambiente mínimo, sem herdar comandos
de restart ou destinos arbitrários. Apply exige SHA256, versão, pacote da arquitetura
local e health HTTP em 127.0.0.1; extrai somente o conteúdo do binário oficial do tar.
Reserva por mkdir impede concorrência. Faz backup, bootout somente do daemon oficial,
troca binário e bootstrap. Health local precisa retornar a versão desejada; falha
restaura backup e exige saúde da versão anterior, retornando falha mesmo após rollback.
A confirmação final continua sendo o report autenticado e a versão no Web.

Logs: `/var/log/aiceberg-agent-update.log` e `/var/log/aiceberg-agent.log`, retenção
operacional de30 dias. Backups `.bak.*` e candidatos `.aiceberg-update.*` ficam em
`/Library/AIceberg/bin` para diagnóstico; revisar após30 dias e remover somente mediante
autorização, preservando binário atual e backup de rollback. Lock remanescente após
interrupção abrupta requer inspeção de processo/job antes de liberação manual.
Não apagar outbox nem prefs. Rollback operacional: desabilitar política de update,
restaurar pacote assinado anterior e configuração preservada; não assumir layout legado.

Validação: `python3 scripts/test_macos_updater.py`, `plutil -lint
scripts/macos/com.aiceberg.agent.plist`, testes Go de readiness e `./check.sh`.
O ensaio substitui apenas launchd/health em processos de teste e arquivos temporários;
não prova instalação, privilégio, rede nem restart de serviço real em host piloto.

## Piloto da central — versão0.8.59

A versão0.8.59 permite validar atualização real a partir da0.8.58, primeira versão
com inspector estruturado. Mantém os procedimentos e assinaturas oficiais dos cinco
pacotes. Preparar agentes legados pelo fluxo individual antes de iniciar lote;
compatibilidade local não comprova atualização física. Registrar confirmação no servidor
após reinício. Sem atualização ampla da frota.

## Reinício com política remota —0.8.60

Na inicialização, requisitar uma configuração completa autenticada e validada,
mesmo com versão de preferências persistida. Assim a política de atualização em
memória é reaplicada antes dos polls condicionais seguintes. Resposta204 ou falha
não consome a recuperação pendente. Agentes anteriores podem precisar de reenvio
da configuração pelo servidor antes do preflight; não remover o check de habilitação.

### Preparação individual Windows legado com serviço padrão

`scripts/windows/prepare-legacy-update-launcher.ps1` é uma operação administrativa
restrita ao piloto, executada pelo fluxo individual já existente. Exige serviço
AIcebergAgent/binário no caminho padrão e pacote0.8.59 previamente validado por
assinatura no agente; revalida SHA256 e lê apenas dois scripts oficiais do ZIP.
Instala esses scripts em diretório restrito a SYSTEM/Administradores, preservando
serviço, binário, token e coletores. Guarda cópia anterior e variáveis de atualização
em subpasta protegida; não remove dados. Ao confirmar versão, configurar no backend
o comando canônico do launcher e reenviar somente a configuração autorizada.
Em falha, manter bloqueio do lote e restaurar comando/arquivos pelo backup somente
após diagnóstico; não apagar a evidência. Backup operacional retido até aprovação
explícita de limpeza; não criar rotina automática para removê-lo.

Validação portátil dos bloqueios antes de qualquer alteração no host:
`pwsh -NoProfile -NonInteractive -File scripts/test_windows_legacy_updater.ps1`.
Cobre serviço/binário divergente, versão não aprovada, checksum divergente e pacote
sem os scripts oficiais. AST PowerShell validado; prova de serviço/ACL/apply depende
do piloto Windows real, não é substituída por esses testes negativos locais.


## Entrega Agentless por segmento — PKG-49

Implementação local; sem ativação/publicação. `AGENTLESS_DELIVERY_PRIORITY=false`
é padrão. Habilitar true somente em HUB autorizado depois de publicar Agent/Web
compatíveis e confirmar versão instalada; requer configuração local e reinício
pelos procedimentos existentes. False restaura seleção antiga no mesmo Bolt,
sem exclusão, transformação ou reconstrução de backlog. Não alterar frequência,
limites/backpressure ou apagar pendências para obter SLA artificial.

Fila opt-in: uma cabeça por check/segmento/coleta, três slots fast recente/um head antigo
incluindo lentos. Até oito requisições por ciclo, incluindo splits. Erro de payload
pode ser isolado só com dedupe; stream recusado bloqueia seus finais e permanece
pendente. Erros globais, auth,429,rede,timeout e confirmação ambígua param o ciclo;
retomada usa a cadência existente. Verificar logs seguros
`agentless delivery deferred/acknowledged` e diagnóstico Web por segmento.

Contrato `delivery_timing.version=agentless_delivery_v1`: UTC RFC3339 por origem.
Fim da coleta é imutável; início de envio é da tentativa atual. Espera desde fim
inclui tentativas anteriores. Web mede entrada da action e save antes de commit,
sem fingir gravação completa/tempo da tela. Estimativas entre relógios ficam
separadas de etapas locais até aferir offset; conferir janela/carga antes/depois.

Reteste: `go test -race ./internal/data/local/agentless ./internal/data/remote ./internal/domain/usecase ./internal/common/config -run 'TestAgentless|TestDelivery|TestLoadAgentlessDelivery' -count=1`.
Gate de fechamento: `./check.sh`. Benchmark opcional:
`go test ./internal/data/local/agentless -run '^$' -bench BenchmarkDeliverySelection -benchtime=3x`.
Não é capacidade/SLA do HUB real. Evidência local em docs/evidence/pkg49-local-20261106.md.
Publicação, ativação, relógios, homologação de erro/retry/rollback e comprovação de
metrics_fast em1–2min são gates separados. Preservar backlog/observações em todos.

Compatibilidade de ACK: em parcial, failure_class=payload permite isolamento
com dedupe; service ou classe ausente interrompe o ciclo. Atualizar Web antes
de ativar a prioridade nova. Consumidor antigo ignora o campo aditivo.

Coletas recentes podem passar backlog antigo, preservando partes/final de cada
coleta. Web revalida projeção sob lock e não regride estado/threshold/binding;
observações seguem retenção atual. Timestamp futuro acima de1800s da recepção
permanece na observação, mas não atualiza a projeção. Corrigir relógios só após
diagnóstico. Rollback por flag é preferencial; não reintroduzir ACK total de
resposta200parcial ao planejar rollback de binário/código.

## Diagnóstico de aplicação de configuração — PKG-134

ConfigSync e ingest próprio compartilham admissão/aplicação, sem rede no callback
de ingest. Falha local mantém prefs válidas, agenda nova aplicação da mesma
configuração (transitória: jitter/teto existentes; persistente/desconhecida:60s)
e continua consultando configuração nova na cadência normal. Assinaturas são
verificadas antes de efeitos; uma assinatura corrigida não herda a espera da
inválida, e renovar timestamps/assinatura válida não cria outra falha de filesystem.
Não mudar a política de assinatura para fazer um teste passar.

CONFIG_FAILURE_STATE_PATH define o arquivo de estado, que pode ficar em volume
independente do caminho de prefs. Caminhos/aliases ou inodes que coincidam com
prefs/outbox são recusados, sem sobrescrevê-los. Por padrão config.Load usa o cache do usuário,
subdiretório aiceberg/config-failures e hash do caminho absoluto de prefs; nenhum
caminho/token integra o relato. Construtores sem esse campo usam sidecar de prefs
para compatibilidade. Estado ausente é inicializado; inválido/oversized/symlink é
preservado e a operação fica em memória limitada, com durabilidade degradada
explícita. Persistência indisponível é reavaliada a cada minuto, sem impedir a
aplicação de configuração nova. Não apagar um arquivo inválido sem autorização.

Limites:32condições/64recibos pendentes/128KiB, resolvidos30dias; checkpoints de
contagem até1min e checkpoints próprios para relatos/ACK. Retry conserva recibo;
primeira falha/mudança/recuperação são imediatas, summary no máximo1por30min por
condição. Capacidade extrema descarta primeiro summary pendente ou o mais antigo,
contabilizando a perda. Não há promessa de histórico ilimitado durante outage.
Fila é entregue antes/depois da consulta: até2recibos por passagem com orçamento4s,
sem reexecutar aplicação para reenviar relatório. Não confundir esse limite com SLA.

Prefs e diagnóstico usam temporário sincronizado e substituição. Linux/macOS
resolvem diretórios por descritores com O_NOFOLLOW e sincronizam o diretório após
rename; Windows mantém handles de diretórios sem compartilhamento de exclusão,
recusando reparse points. Somente pastas ausentes são criadas; conflitos/symlinks
não são corrigidos, permissões existentes são preservadas, sem elevação. Memória
de prefs muda somente depois de sucesso. Falha física de energia/armazenamento
ou ACL nativa em host real é homologação distinta das fixtures locais.

Saúde/metrics existentes expõem attempts/suppressed/report_bytes/report_duration_ms/
dropped_reports em config_failures e agent_config_*_total, sem labels de identidade,
caminho ou fingerprint. Logs seguros registram causa, espera, duração e fila.
As consultas GET normais continuam para detectar configuração nova; economia real
de CPU/disco/tráfego e tempo de diagnóstico deve ser medida no piloto.

Publicar Web e guard API-006 Runtime compatíveis antes do produtor, com autorização,
artefatos oficiais assinados e smoke. Rollback do agente anterior preserva JSON de
prefs compatível, estado/outbox e história; manter o consumidor PHP para metadados
novos enquanto houver agentes novos. Não apagar caches/filas para aparentar recuperação.
Homologação/piloto controlados com versões/SO registrados, sem falha destrutiva em
host real, precedem aceite operacional; código100% não comprova esses gates.

## Diagnóstico limitado do serviço — 17/11/2026

collect_support_flare usa journalctl restrito a aiceberg-agent.service no Linux
(última hora/100linhas) ou wevtutil Application/AIcebergAgent no Windows (100eventos).
Sem shell remoto, sem leitura arbitrária de arquivos. Até16KiB/fonte em5s, com
redação por linha e truncamento explícito. Ferramenta/fonte ausente, permissão,
fonte vazia, timeout/cancelamento e SO não suportado não retornam diagnóstico OK.
Snapshot de runtime permanece complementar; não substitui logs reais.

Publicar pacote oficial assinado e backend compatível antes da ativação. Pilotar
Linux/Windows/SADA com conta/host autorizados e confirmar versão/retorno. Falha de
permissão não autoriza elevar privilégio ou mudar a política do host. macOS não
recebeu coletor de suporte neste escopo. Rollback usa pacote assinado anterior,
preservando token/env/outbox. Conteúdo centralizado:30dias, metadados180dias;
procedimento de descarte governado no RUNBOOK Web, sem novo cron no agente.

Suporte0.8.70: em journal/EventLog indisponível, a fonte agent_service_process_log
pode fornecer logs reais do próprio processo. É volátil, limitada16KiB/100entradas/
1h, redigida antes de reter; reinício esvazia naturalmente o buffer. Evidência
mantém falha da fonte SO e degraded=true. Não conceder systemd-journal nem acesso
amplo ao host apenas para o piloto. Sem entrada real ou após cancelamento, falhar.

## Piloto SNMPv1 —19/11/2026

Candidata0.8.71-rc.1 com artefatos oficiais assinados, oculta das listas gerais Web; instalar apenas no HUB SADA selecionado após gates, confirmar versão/readiness/outbox e manter rollback0.8.70. Probe manual temporário cliente/HUB/check no Web, sem alterar perfil ou rotina; oito colunas de sensor fixas, zero retry,5s/16linhas. Não interpretar SNMP OK como temperatura: exigir temperature_sensors qualificado, idade e três ciclos reais. Procedimento: Web docs/operations/snmp-v1-sensor-pilot.md.

## Candidata0.8.71-rc.2 e prioridade manual —19/11/2026

HUB84 confirmou rc.1 assinada, mas diagnósticos166/167/168 terminaram skipped_busy, sem resultado SNMP. Poll1s/flush2s para1873checks torna a janela manual estreita; ajuste temporário30/30s foi restaurado1/2s e piloto desativado, sem reativar os15checks. Nova candidata permite uma espera manual com prazo, cedendo apenas o próximo slot da rotina; nunca cancela a coleta em curso ou libera seu lock. Outra solicitação simultânea mantém skipped_busy. Prazo inclui espera e execução; expiração continua diagnóstico não concluído, sem leitura inventada. Construir/assinar nova versão imutável, atualizar apenas o HUB84 e repetir v2c/v1 pelo serviço Web. Reusar backup privado e rollback0.8.70/rc.1; promoção geral ainda bloqueada por temperatura física, frescor e três ciclos/referência local.

Resultado piloto restrito: rc.2 instalada/confirmada somente no HUB84; execuções170(v1) e171(v2c) comprovaram protocolo efetivo, sem dados válidos no budget5s para Catalão. Não houve temperatura; ainda exige qualificação de alcance/ACL/credencial/origem e aceite físico. Piloto desligado, configuração privada original restaurada, poll/flush1/2s, zero observações operacionais do check. Restante14sensores não extrapolado. Recibo no Web: docs/evidence/snmp-v1-sada-pilot-20261119.md.

## Diagnóstico SNMP básico —20/11/2026

Candidata0.8.71-rc.3: preset system_get com GET único de uptime/identidade,
1500ms/retries0/teto5s, sem WALK. Testes: go test ./internal/platform/agentless
-run TestBasic -count=1; fixtures UDP reais em loopback v1/v2c com dados sintéticos,
valor zero válido, timeout sem resposta, counters, limites, redaction e erros PDU.
Receber UDP não comprova resposta SNMP válida. Piloto apenas HUB84 após gates
e assinatura; controles positivos antes dos sensores, sem alterar perfil/rotina.
PKG-116 parcial, sem leitura física nova ou delegação relay declarada. Procedimento
e rollback: Web docs/operations/snmp-v1-sensor-pilot.md; preservar config/outbox.

## Diagnóstico limitado pelo relay —20/11/2026

Candidata assinada mínima0.8.72-rc.1 em HUB/relay; primeiro HUB84, depois somente
relay424 elegível após preflight, versão, heartbeat e fila. Default Web desligado,
listas explícitas e origem HUB válida. Roteiro: Web docs/operations/relay-snmp-probe-pilot.md.
Não habilitar rotina ou trocar modo do relay. A fila Bolt usa AGENTLESS_OUTBOX_PATH
com sufixo .relay-probe,0600/teto configurado50MiB, sem fallback em memória.
Retry15–240s somente na entrega pelo HUB; API accepted=true antes do ACK local.
Sem pruning por idade; quando cheia, bloquear, preservar e reconciliar. Não apagar
registro para contornar falha. Claim/resultado atrasado não ganham autoridade.
Rollback assinado HUBrc.4/relay0.8.70 após drenar resultados; preservar filas e prefs.
PKG-120 permanece parcial; sem eleição/ownership/HA ou promoção geral declarados.

Piloto físico20/11 em0.8.72-rc.1: controle SNMPv2c e três consultas manuais v1
válidas por relay424→HUB84→API. Sensor Catalão sem resposta em ambas as origens
com perfis autorizados30/29. Ambos os recortes privados Web restaurados/desligados,
15ICMP ok/60s e nenhuma observação/rotina alterada. Conferir recibos e limites no
Web docs/evidence/sada-relay-snmp-pilot-20261120.md; não habilitar assignment
recorrente ou promover a frota por esse ensaio.

## Candidata canônica SNMP0.8.72-rc.2

Consumo servidor deve preceder update restrito. Campos novos são opcionais;
servidor antigo pode ignorá-los. Não apagar outbox para atualizar/retroceder.
Conferir IDs/dedupe estáveis, extensão com2séries e parciais unknown, ACK explícito
e observação única. Sem métricas, status online/offline existente é preservado.
Assinatura e checksums oficiais são obrigatórios antes de distribuir. Homologação
de sensores reais exige três ciclos; fixture UDP não comprova resposta SADA.


TCP canônico: instalar consumidor PHP compatível antes de promover candidata
que emita agentless_tcp. Não exige mudança de jobs/cadência/perfil ou probe novo.
Rollback Agent omite metadado opcional, preservando outbox e observação legada;
servidor reconstrói pela latência dedicada. Zero é válido, falha não produz valor.
Contrato Web monitoring-tcp-execution-v1.md. Nenhum artefato novo gerado no item1.


monitoring.records.v1 é contrato inerte. Não emitir nem converter snapshots SNMP/TCP
existentes por inferência: integração/negociação de transporte pertence ao item3.
Revalidar decode→ValidateRecords com definição/autoridade externa, revisão e relógio;
RecordFreshness separa idade atual da qualidade no evaluated_at_ms original.
Rollback retira biblioteca sem apagar/configurar/coletar/replay. Não há novo binário
ou instalação decorrente somente desta definição de contrato.


Monitoramento universal/transporte22nov: novos codecs/perfis são internos e
inertes; nenhuma capability/rota instalada foi habilitada. Não inferir suporte
por versão/HTTP200: exigir controle autorizado, perfis compatíveis de2..8hops,
JSON opaco e ACK qualificado após aceitação durável. Negociação limita contrato/
fontes/tipos/bytes/registros; incompatible_monitoring_transport e
monitoring_transport_limit bloqueiam envio, sem truncar/coagir/recoletar.
HUB existente exige entrada plain; gzip na entrada é incompatível e deve manter
fila pendente, sem mudar opção global. Read recebe stream já descomprimido e com
deadline; limita bytes antes do parser. ACK intermediário não prova consumo final.
Ativação/rollout ficam nos gates do item14; rollback local remove componentes
inertes, sem SQL, limpeza/replay/alteração de dados/configuração/serviços.
Contrato canônico Web:docs/contracts/monitoring-records-transport-v1.md.


Configuração composta universal22nov: monitoring.configuration.v1 é um contrato
compilado/validado inerte. Fornecer autoridade independente de tenant/checks/root/
pins/revisão e referências; verificar assinatura/autorização no controle, sem
aceitar autoridade do corpo. Campo faltante não ganha default. Diff/edição não
permitem aplicar pipeline/host/custom/derived pelo runner legado; este recusa
universais inteiramente. Publicação/instalação/ativação ficam no item14.
Depois de revisões universais gravadas em ambiente instalado, manter o leitor/
normalizador novo no rollback de UI/compilador, pois o leitor antigo recusa campos
novos. Retirar escrita antes de rollback, restaurar conteúdo conhecido em nova
revisão e preservar histórico/chave/filas/checks. Sem SQL/DELETE/expurgo implícito,
restauração global de params ou alteração de ACL. Contrato canônico Web:
docs/contracts/monitoring-configuration-v1.md.


### Pipeline básico — item5/14,22/11/2026

Núcleo Go executável versionado/tipado com identity/scale/offset, plano imutável
fixado por hash exato/revisão/escopo e configuração compilada autorizada. Estágios
locais explícitos de entrada/resultado evitam reaplicação acidental; somente batch
final validado segue contrato existente. Orçamentos de etapas/operações/bytes/tempo,
inteiros exatos e falhas unknown/null sem zero fictício; erros de configuração,
limites ou cancelamento retornam sem saída parcial.76vetores comuns PHP/Agent/
Runtime, schema, race, concorrência, replay/recompilação, compositor real e TCP
loopback→pipeline→PHP. Sem SQL, UI, collector/scheduler produtivo, release/update,
configuração privada ou teste físico SADA. Contrato canônico Web:
docs/contracts/monitoring-pipeline-v1.md. Item5 de desenvolvimento qualificado;
itens6–14 abertos, pacote115/contraparte19 não encerrados.


### Pipeline de monitoramento — item 6, 23/11/2026

O executor Go do Agent e Runtime passa a validar faixa ou comprimento, extrair
uma captura por regex e selecionar valores tipados por JSONPath singular.
PHP verifica a estrutura e valida o batch de saída. A compilação Go autoriza
a sintaxe e o custo do regex antes da execução. Os vetores compartilhados
cobrem a ordem das etapas, limites, tipos, valores ausentes e falhas explícitas.
A entrega local mantém o contrato de entrada e saída do item 5. A adoção requer
confirmar que todos os executores da configuração suportam as novas operações.
Em rollback, restaurar primeiro a revisão compatível ou desativar a atribuição.
Ver contrato Web em docs/contracts/monitoring-pipeline-v1.md. Itens 7–14 abertos.


### Pipeline de monitoramento — item 7

XMLPath singular no Agente e Runtime, com seleção por filho direto, posição
e atributo final. Valida documento completo, limites, cardinalidade e tipo
antes de emitir resultado. XML rejeitado, ambiguidade, ausência e estouro de
limite são distinguíveis sem expor conteúdo. PHP valida o caminho e consome o
batch final. Ver contrato Web em docs/contracts/monitoring-pipeline-v1.md.
A adoção produtiva depende de executores compatíveis no item 14.


### Pipeline de monitoramento — item 8

Delta para gauge e contador, com base inicial explícita, reset e proteção
de faixa. Throttle por igualdade tipada e janela desde a última emissão.
Checkpoint bbolt por vínculo com recibos atômicos, recuperação após
reinício e recusa de observações conflitantes. Lote suprimido por inteiro
não gera batch. Ver testes de transação, reentrega e PHP no contrato Web
docs/contracts/monitoring-pipeline-v1.md. Ativação e gestão do ciclo de vida
dos checkpoints ficam no item 14.


### Pipeline de monitoramento — item 9

O Agente e Runtime executam os itens dependentes do modelo validado, com
ordem topológica e uma única passagem por nó a partir de um snapshot de
coleta. Tipos, unidades, frescor e indisponibilidade são propagados por ramo.
Delta e throttle usam checkpoint atômico do grafo. Qualificação local cobre
fan-out TCP real com uma conexão, resultado consumível por PHP e reentrega
do grafo após reabertura bbolt. Adoção em produção no item 14. Ver
docs/contracts/monitoring-pipeline-v1.md no Web.


### Pipeline de monitoramento — item 10

Itens calculados executam fórmulas numéricas tipadas no DAG, com orçamento e
validação estática de dependências, operadores e unidades. Lacunas, erros
numéricos e frescor permanecem explícitos. Identidade do resultado calculado
usa o ciclo, e o DAG preserva a idade da origem mais antiga para a cadeia.
O compositor Go preserva a declaração da fórmula, e PHP valida a configuração
e consome o lote. Os testes locais cobrem fórmulas encadeadas, múltiplas
origens, falhas isoladas e combinação com checkpoint. Contrato Web:
docs/contracts/monitoring-pipeline-v1.md. Adoção operacional no item 14.


### Pipeline de monitoramento — item 11

Etapa script WebAssembly com ABI para valores JSON tipados, no Agente e
Runtime. O perfil sem imports restringe memória, instruções, concorrência
e prazo. Tempo/capacidade abortam sem commit parcial. Falhas do valor são
explícitas e isoladas no ramo. PHP valida o envelope e o resultado. Os
testes locais cobrem rejeição de recursos proibidos, memória nova por
chamada, tipo, trap, orçamento, fan-out e reentrega. Ver o contrato Web
docs/contracts/monitoring-pipeline-v1.md. Adoção operacional no item 14.


### Séries de monitoramento — item 12

Persistência MySQL única para registros canônicos, com catálogo por ativo,
revisão protegida, reentrega idempotente, histórico indexado e último registro.
Cotas de séries e amostras são transacionais por cliente. A retenção limita
leitura e escrita e oferece plano revisável de remoção exata, sem ativação
automática. MySQL 8.4 descartável validou o SQL 286 e o roundtrip DAG → SQL →
PHP, além de escopo, atomicidade, conflitos, cursor temporal e concorrência.
Contrato Web em docs/contracts/monitoring-series-v1.md. Agregações ficam no
item 13. Ativação, grants e conciliação operacional no item 14.

## Recepção de traps SNMP no HUB

O listener é opt-in e usa um IP dedicado da rede de gerenciamento.
Acompanhar `snmp_trap_configuration_invalid`, `snmp_trap_profiles_unavailable`,
`snmp_trap_listen_failed` e `snmp_trap_outbox_unavailable` no serviço do HUB.
Para v3, conferir authPriv, SHA configurado, AES de 128 bits, usuário, Engine
ID, contexto e relógio com um emissor autorizado. A versão do perfil é
explícita por IP. O estado de tempo e replay fica no arquivo Bolt
`.snmp-traps` e precisa ser preservado em reinício e rollback. Para
desativar o listener, remover suas duas configurações de ambiente e
reiniciar somente o HUB selecionado. Ver `docs/contracts/monitoring-snmp-trap-v1.md`
no Web para o contrato e a validação do histórico.


## OpenMetrics e Jolokia autenticados no HUB

Configurar o arquivo privado referenciado por `AICEBERG_HTTP_PROBE_PROFILES_FILE`
no HUB Linux/macOS e confirmar o pull com `http-probe-auth-v1`. Manter a
URL HTTPS completa, escopo e referência iguais aos do job. Diagnosticar
`http_credential_unavailable` por ausência ou escopo do perfil e
`http_access_denied` por recusa do destino. Inspecionar o certificado e a
CA quando houver `http_fail`. O material de autenticação fica no HUB.
O procedimento de rotação e rollback está no contrato
`docs/contracts/monitoring-probe-policy-v1.md` do Web.


## Reteste de armazenamento InspectApp —23/11/2026

Host187.45.180.181,cliente1,agente70. Preservar DenyKey smart.*, smartd inativo e contas/cursors/outbox. Não executar SMART/perccli/carga/reset/firmware para smoke. No Agente garantir AICEBERG_ALLOW_SMART não habilitado; sampler usa procfs/sysfs somente. Preservar fontes e cursores existentes: syslog já contém kernel; não adicionar kern.log em paralelo nem recontar eventos históricos.
Publicar somente código deste incidente pelo fluxo oficial, sem incorporar alterações SNMP concorrentes. Confirmar configuração publicada/aplicada e versão/hash do binário, amostra passiva, alerta persistido no sink controlado e tela instalada. SQLs de preparação/ativação/rollback no Web. Fixtures no-send não são notificação entregue. A publicação permanece pendente neste incremento.
## Execução SNMP delegada no relay

O worker e as rotas do HUB ficam desligados por padrão. Para laboratório,
`RELAY_SNMP_ASSIGNMENT_ENABLED=true` libera as rotas de encaminhamento no HUB
e o worker no relay. O relay requer `RELAY_SNMP_ASSIGNMENT_HUB_ID`,
`RELAY_SNMP_ASSIGNMENT_PUBLIC_KEY` (Ed25519 em hexadecimal) e os IDs do
cliente e do agente. As duas conexões usam HTTPS verificado. O worker
usa arquivo Bolt dedicado com sufixo `.relay-assignment`. O primeiro
envio usa a observação gravada no Bolt logo após a coleta. Se a entrega
falhar, o worker reenvia a observação antes de solicitar outra coleta.

O contrato usa `/v1/relay-snmp-assignment/job` para oferta, claim e credencial
referenciada, e `/v1/relay-snmp-assignment/result` para entrega. A execução
exige um job assinado compatível com o check e a epoch da oferta. O recibo
de entrega precisa devolver o mesmo `dedupe_key`. O Runtime recebe as rotas
upstream no recorte piloto configurado. Um resultado recusado por perda de
autoridade só sai da fila de envio após ser preservado no bucket
`relay_assignment_fenced_v1`, com o mesmo identificador. A fila preserva os
dados se esse armazenamento atingir 8 MiB. Manter o recurso desligado fora do
laboratório até validar a integração operacional e o piloto físico.

## Retirada de interface no Discovery

Depois de autorizar cliente e ativo na ingestão do Discovery, configure
`DISCOVERY_INTERFACE_CLIENT_ID` e `DISCOVERY_INTERFACE_ASSET_ID` no ambiente
privado do serviço Agente ou HUB Linux ou Windows que monitora esse ativo diretamente.
O código exige os dois IDs positivos e não habilita a emissão em relays.
Em Linux, o coletor usa a notificação de exclusão de link do kernel.
Em Windows, exige notificação de exclusão da interface IP e confirma que
a identidade e o MAC não continuam na tabela de interfaces do sistema.
Uma família IPv4/IPv6 removida não basta se o adaptador ainda estiver presente.
A retirada confirmada gera um fingerprint da interface,
enviado pela fila existente para `/v1/ingest/metrics`. O limite é 16 eventos
por minuto e o frescor é de 300 segundos. A fila mantém o mesmo comportamento de
retry. A retirada efetiva da série exige a janela e o plano revisado do servidor.
Para desativar o coletor, remova os dois parâmetros e reinicie o serviço.

### Discos e partições Linux

Configure `DISCOVERY_DISK_CLIENT_ID` e `DISCOVERY_DISK_ASSET_ID` no
ambiente privado do Agente ou HUB Linux que monitora diretamente o ativo.
A autorização na ingestão e a revisão de retirada são as mesmas das
interfaces. O coletor usa o inventário local padrão do kernel e recusa
redirecionamentos de `HOST_PROC` ou `HOST_SYS`. Somente uma exclusão
explícita de dispositivo de bloco com ausência confirmada em
`/sys/class/block` gera evento. Desmontagem, alteração e falha de leitura
de I/O não geram retirada. O limite é de 16 notificações aceitas por
minuto e o frescor é de 300 segundos. Remova os dois parâmetros e reinicie
o serviço para desativar a emissão.

### Temperatura local Linux

Configure `DISCOVERY_SENSOR_CLIENT_ID` e `DISCOVERY_SENSOR_ASSET_ID` no
ambiente privado do Agente ou HUB Linux vinculado diretamente ao ativo. A
emissão requer exclusão explícita do dispositivo hwmon pelo kernel, uma
chave de temperatura previamente observada e única, e ausência confirmada
do dispositivo e da chave após a remoção. Aplica-se às temperaturas
expostas diretamente por hwmon no sysfs local. Leituras térmicas legadas
podem impedir a retirada de uma chave ainda visível. O limite é de 16
eventos por minuto e o frescor é de 300 segundos. Se o outbox ficar
indisponível, o coletor retém até 16 lotes em memória. Tenta enviar um
lote a cada cinco segundos, com novo inventário. Se o sensor reaparecer
ou o evento vencer, o reenvio é descartado. A janela de retirada,
a aprovação vigente e a revisão continuam exigidas no servidor. Remova
os dois parâmetros e reinicie o serviço para desativar a emissão.

### Serviços Windows e Linux

Configure `DISCOVERY_SERVICE_CLIENT_ID` e `DISCOVERY_SERVICE_ASSET_ID` no
ambiente privado do Agente ou HUB vinculado ao ativo. No Windows, o coletor
enumera serviços no SCM a cada 30 segundos com permissões de consulta.
Confirma individualmente a exclusão de nomes previamente observados e
envia no máximo 16 retiradas por captura, com frescor de 300 segundos.
Se o outbox falhar, o evento mantém seu horário enquanto estiver fresco e
a exclusão continuar confirmada. A aprovação e a janela de retirada são
validadas no servidor. Remova os dois parâmetros e reinicie o serviço
para desativar a emissão.

No Linux com systemd, os mesmos parâmetros autorizam uma consulta a cada
30 segundos. O coletor acompanha unidades `.service` concretas e instaladas,
com diferenciação entre maiúsculas e minúsculas. Uma unidade previamente
vista precisa estar ausente da listagem e responder `LoadState=not-found`
e `ActiveState=inactive`.
Os limites são 1.024 unidades, 64 consultas individuais e 16 retiradas por
captura, com até 10 segundos para consulta e envio. A desativação usa os
mesmos parâmetros do Windows.
Em um host Linux isolado com systemd, execute
`TestNativeSystemdServiceRemovalAndReturn` com
`AICEBERG_DISCOVERY_SYSTEMD_FIXTURE=1` e permissão de escrita em
`/run/systemd/system`. O teste usa uma unit temporária inativa e confirma
remoção, reenvio após falha do outbox e retorno com a mesma identidade.

### Pacotes Debian e RPM

Configure `DISCOVERY_APPLICATION_CLIENT_ID`, `DISCOVERY_APPLICATION_ASSET_ID`
e `AICEBERG_AGENT_ENABLE_PACKAGE_INVENTORY=true` no ambiente privado do
Agente ou HUB Linux vinculado ao ativo. A consulta somente leitura usa a
base local do dpkg ou RPM a cada 30 segundos. O coletor acompanha até
4.096 registros e publica até 16 eventos por consulta. Preserva o nome
binário, incluindo o sufixo de arquitetura quando fornecido pelo dpkg, e
a origem `dpkg`. A ausência total precisa ser confirmada em duas capturas
completas. Estados intermediários de atualização mantêm o pacote em
observação. Remova os dois parâmetros `DISCOVERY_APPLICATION_*` e reinicie
o serviço para desativar a emissão.
Para RPM, a identidade usa nome, arquitetura e origem `rpm`. Versões
paralelas da mesma arquitetura entram na mesma identidade. O Agente envia
a retirada depois de duas capturas completas sem nenhuma versão instalada.

### Aplicações Windows

Use os mesmos parâmetros `DISCOVERY_APPLICATION_*` e habilite
`AICEBERG_AGENT_ENABLE_PACKAGE_INVENTORY=true` no Agente Windows. A consulta
somente leitura verifica as chaves de desinstalação em `HKLM` nas visões
de Registro disponíveis. A retirada usa o nome observado e a origem
`registry`. Confirme o ciclo em um host Windows com um aplicativo de
teste antes de habilitar em produção. Aplicações registradas somente no
perfil do usuário e fontes fora dessas chaves ficam fora da cobertura.

No Windows de teste autorizado, execute
`TestNativeRegistryApplicationRemovalAndReturn` com
`AICEBERG_DISCOVERY_WINDOWS_REGISTRY_FIXTURE=1` e permissão de
escrita em HKLM. O teste cria uma chave de desinstalação exclusiva,
confirma a ausência e o retorno da identidade e remove somente a
própria chave. Execute em um host de laboratório com inventário
acessível e sem instalação de software concorrente.

Para qualificar o SCM, execute
`TestNativeWindowsServiceRemovalAndReturn` no mesmo host isolado com
`AICEBERG_DISCOVERY_WINDOWS_SERVICE_FIXTURE=1`. O ensaio requer
permissão de criação de serviço. Usa um nome exclusivo e um serviço
desabilitado. A fixture nunca inicia o serviço e remove somente
o registro temporário criado pelo teste.

### Arquivos de log

No Agente ou HUB vinculado ao ativo, habilite o Discovery de fontes de
logs e configure `DISCOVERY_LOG_SOURCE_CLIENT_ID` e
`DISCOVERY_LOG_SOURCE_ASSET_ID`. A cada ciclo do Discovery, o Agente
acompanha arquivos regulares já enviados ao outbox. A retirada exige
ausência em duas capturas, separadas por pelo menos dois minutos, com
o mesmo diretório acessível. A consulta verifica apenas metadados.
O retorno do arquivo cancela a retirada pendente na ingestão.
Para desativar, remova os dois parâmetros de escopo e reinicie o Agente.

### Fontes de logs do systemd e journald

Use o mesmo escopo de retirada de arquivos de log em agentes Linux.
Quando uma fonte conhecida deixa de aparecer, o Agente exige a ausência
do arquivo da unit ou do socket conhecido, sob o mesmo diretório
acessível. A consulta somente leitura a `systemctl show` precisa
confirmar `LoadState=not-found` e `ActiveState=inactive`.
Duas verificações separadas por pelo menos dois minutos confirmam a
retirada. Uma unit ativa, parada ou mascarada ainda pode estar instalada.
O socket pode desaparecer enquanto o serviço continua instalado.
Tempo excedido, permissão negada ou erro ao consultar o gerenciador
interrompem a confirmação para a fonte. A análise tem orçamento de
quatro segundos por ciclo e retoma as fontes restantes no ciclo seguinte.

Para validar em um host Linux isolado com systemd, execute
`TestNativeSystemdLogSourceRemovalAndReturn` com a variável
`AICEBERG_DISCOVERY_SYSTEMD_FIXTURE=1` e permissão de escrita em
`/run/systemd/system`. O teste cria só a unit temporária, recarrega
o gerenciador nas transições e remove a fixture ao terminar. O relógio
da confirmação é controlado pelo teste.

### Canais do Windows Event Log

A descoberta consulta a configuração local de Security, System,
Application, Sysmon e canais adicionais da configuração efetiva do
Agente, com limite total de 64 canais. O mesmo escopo de arquivos de log
habilita a retirada dos canais já enviados. A ausência exige duas
respostas de canal inexistente separadas por pelo menos dois minutos.
Canal desabilitado ou erro de permissão preserva o item. Antes de ativar
em produção, execute o ensaio de descoberta, remoção e retorno de um
canal de teste em host Windows autorizado.

## Contabilidade transacional da fila —25/11/2026

A candidata mantém totals em bucket interno separado; abertura reconstrói a partir de todos os envelopes antes de aceitar gravações. Backup/rollback seguem o pacote assinado anterior, sem apagar/compactar fila nem editar bucket manualmente. Versão antiga ignora os metadados; reupgrade reconstrói novamente. Comparar deltas do mesmo PID, CPU/RSS, idade medida/recebida, ACK/deferred/retained e falhas por rota. Não usar queda de ocupação isolada como prova de entrega, porque a retenção preexistente continua existindo. Relay585 sem canal/heartbeat não se recupera com mudança no HUB: verificar serviço Windows e comprovar nova amostra do mesmo host/volume.

## Piloto0.8.72-rc.4 — vazão por agrupamento autenticado

Preservar backup/cfg75 da0.8.72-rc.3,pré-verificação fresca,assinatura/cincoSHA e instalação somenteHUB84. Conferir last_input_groups versus last_dispatch_groups,ACK/idade real/recusas eCPU/RSS do novoPID por deltas; não confundir pacote antigo recém-recebido com dado atual. Rollback por pacote assinado0.8.72-rc.3/force controlado ou backup do lançador se serviço não iniciar;sem limpeza/compactação/replay,sem ampliar concorrência,capacidade ou frequência. Relay585 será verificado por operador Windows amanhã por decisão do titular,sem tentativa remota nesta entrega.

### Piloto0.8.72-rc.5

Comparar last_input_groups/last_dispatch_groups e last_identity_reasons (somente contagens),ACK,idade da fila e CPU/RSS do mesmoPID. Claims legados semIDs só agrupam após HMAC válido e sujeitohost/instalação estável. Atualizar exclusivamenteHUB84 com readiness fresco,backup/configCAS e pacotes assinados;rollbackrc.4,sem limpeza de filas ou alteraçãoAPI/Runtime/DLQ.
