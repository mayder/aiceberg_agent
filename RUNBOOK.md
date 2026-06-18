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

### PKG-62 - OTLP HTTP/JSON

Contrato tecnico: `docs/pkg62_otlp_http_json.md`.

Configuracoes locais:

- `OTLP_ENABLED=true`;
- `OTLP_HTTP_ADDR=127.0.0.1:4318`;
- `OTLP_INTERVAL=10`;
- `OTLP_MAX_ITEMS=1000`;
- `OTLP_MAX_BYTES=1048576`.

Rollback: definir `OTLP_ENABLED=false` ou enviar config remota `otlp.enabled=false`. Nao ha SQL do PKG-62.

### PKG-63 - APM/traces sampling

Contrato tecnico: `docs/pkg63_apm_sampling.md`.

Configuracoes locais:

- `APM_TRACE_SAMPLE_RATE=1`;
- `APM_TRACE_SLOW_THRESHOLD_MS=1000`;
- `APM_TRACE_PRESERVE_ERRORS=true`.

Rollback: definir `APM_TRACE_SAMPLE_RATE=1` para enviar todos os spans ou desligar OTLP com `OTLP_ENABLED=false`.

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

Para empacotar uma evidencia real ja coletada, use `scripts/pkg69_bundle_evidence.sh <cenario> <template-preenchido.md> <arquivo-ou-diretorio-bruto> [saida]`. O helper aceita apenas os cenarios oficiais do gate PKG-69; cenario desconhecido falha antes de gerar bundle.

Para coletar evidencia read-only no proprio host e gerar o bundle, use `scripts/pkg69_collect_host_evidence.sh <cenario> <template-preenchido.md> [saida]`. Com `PKG69_RUN_SMOKE=true`, o script tambem executa `scripts/smoke.sh` e anexa log/JSON ao artefato bruto. O gate continua obrigatorio depois da coleta.

Para rodar o gate a partir de uma pasta com bundles ja coletados, use `scripts/pkg69_run_evidence_gate_from_bundles.sh <pasta-bundles>`. O script valida que cada `MANIFEST.tsv` tem uma unica linha de evidencia, `template` coerente com `evidence.md`, `created_at_utc` em formato compacto `YYYYMMDDTHHMMSSZ`, artefato do manifest coerente com `Evidencia bruta anexada`, confere SHA256/tamanho do `evidence.md` e do anexo bruto, mapeia cada cenario para a variavel `PKG69_*_EVIDENCE` correta e chama o gate oficial; com `PKG69_REQUIRE_REAL_EVIDENCE=true`, continua falhando enquanto qualquer cenario obrigatorio estiver ausente.

Para gerar um relatorio acionavel de lacunas, use `scripts/pkg69_evidence_gap_report.sh <pasta-bundles>`. O relatorio Markdown lista cenario, status, motivo, proxima acao e veredito explicito de fechamento (`BLOQUEADO`, `PRONTO_PARA_REVISAO` ou `ACEITO_PARA_FECHAMENTO`) a partir do manifest do gate; o stdout tambem publica `closure_status`, `closure_reason` e `closure_acceptance` para automacao. Para usar como bloqueio de fechamento/CI, execute com `PKG69_GAP_REPORT_REQUIRE_COMPLETE=true`; o comando falha enquanto qualquer cenario estiver pendente. Para bloquear aceite sem revisao explicita, execute tambem com `PKG69_GAP_REPORT_REQUIRE_ACCEPTED=true`; esse modo ativa rejeicao de marcadores sinteticos no gate e so passa quando `PKG69_ACCEPT_CLOSURE=true`.

Para fechamento, preencher os templates dos ambientes reais e rodar com `PKG69_REQUIRE_REAL_EVIDENCE=true` e `PKG69_REQUIRE_CLOSURE_ACCEPTED=true`. O pacote nao deve ser marcado como 100% enquanto o gate reportar `pkg69-status: not-closed`. Cada template precisa preencher `Data UTC` em formato `YYYY-MM-DDTHH:MM:SSZ`, `Topologia` com `direct -> AIceberg`, `hub -> AIceberg`, `relay -> hub -> AIceberg` ou `direct/hub/relay hosts separados`, apontar `Evidencia bruta anexada` para arquivo/diretorio local existente e nao vazio, e registrar rollback validado. O manifest TSV inclui SHA256/tamanho do template e do anexo bruto e `created_at_utc` em formato `YYYYMMDDTHHMMSSZ`. Na evidencia de Relay/Hub/Direct, os campos textuais de direct, hub, relay via Hub, relay sem conexao direta e agentless via Hub devem ser `yes|true|sim`, e `relay_direct_api_attempts` precisa ser `0`. Evidencia com CPU/RSS acima dos limites iniciais, Kubernetes permitindo `secrets`, `exec` ou `delete`, ou campos de responsavel/lab/versao/artefato/observacao/revisor contendo marcador de self-test/sintetico/fake/mock/placeholder nao passa no gate real.

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

Configuracoes opcionais:

- `PRIVACY_PROFILE=standard|sensitive|minimal`;
- `SENSITIVE_MODE=true`.

Operacao segura:

- IA local e deterministica, sem LLM obrigatorio;
- nenhuma acao destrutiva automatica;
- usar evidencia e lacunas como apoio NOC/SOC;
- validar replay offline e correlacao agentless em ambiente real antes de declarar diferencial.
- para anexar evidencias reais ao roteiro, use `PKG72_INCIDENT_EVIDENCE`, `PKG72_REPLAY_24H_EVIDENCE`, `PKG72_REGULATED_CLIENT_EVIDENCE`, `PKG72_NOISE_COST_EVIDENCE` e `PKG72_DATADOG_BENCHMARK_EVIDENCE`.

Rollback: ignorar `contextual_evidence` no backend. Nao ha SQL do PKG-72.

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

## Adaptacao deste projeto

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
- Banco de dados: nao usar migrations. Mudancas devem ser scripts `.sql` idempotentes quando possivel, com ordem e rollback.
