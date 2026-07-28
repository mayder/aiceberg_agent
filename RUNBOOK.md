# RUNBOOK.md

Runbook operacional do projeto.

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
