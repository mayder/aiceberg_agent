# DECISOES.md

Registro de decisões arquiteturais, técnicas e operacionais relevantes.

Este arquivo existe para evitar que decisões importantes fiquem espalhadas em backlog, escopo, comentários ou histórico de chat.

## Quando registrar

- Escolha de stack, biblioteca, provider ou framework.
- Criação de abstração.
- Mudança de boundary.
- Exceção a padrão.
- Decisão de não refatorar agora.
- Trade-off de prazo, risco, performance ou compatibilidade.
- Mudança de estratégia de teste, deploy ou rollback.
- Decisão que a IA provavelmente rediscutiria no futuro se não estivesse documentada.

## Regras

- Decisão registrada deve ser curta e objetiva.
- Não registrar decisão trivial.
- Se uma decisão substituir outra, marcar a anterior como `substituída`.
- Se uma decisão for revertida, registrar motivo e forma de reversão.
- Decisões aceitas devem ser consideradas fonte de verdade até serem substituídas ou revertidas.

## Modelo

```txt
### DEC-YYYYMMDD-01 - Título

Status: proposta | aceita | substituída | revertida
Data:
Contexto:
Decisão:
Alternativas consideradas:
Consequências:
Impacto em testes:
Impacto em rollback:
Como reverter:
Referências:
```

## Decisões

### DEC-20260618-01 - Matriz evidencial para evolucao Datadog-like do agente

- Status: aceita
- Contexto: a evolucao do agente para capacidades Datadog-like envolve mudancas coordenadas no `aiceberg_agent` e no `aiceberg_web`, com risco de quebrar snapshots, ingestao, canal, update remoto e Agentless HUB.
- Decisao: usar a matriz unica do web em `docs/agente_datadog_paridade.md` e manter neste repo o inventario tecnico `docs/pkg58_inventario_agente_atual.md`. Cada pacote de runtime deve preservar contratos atuais, introduzir flags/configs de rollback e preencher evidencia por ambiente antes de declarar paridade.
- Alternativas consideradas: manter backlog local solto ou implementar capacidades por demanda sem matriz.
- Consequencias: PKG-59 a PKG-72 precisam apontar dependencias, validacao e rollback antes de fechar.
- Impacto em testes: PKG-58 exige validacao documental e checks dos dois repos; runtime novo exige testes por pacote.
- Impacto em rollback: nenhum runtime novo e ativado por esta decisao.
- Como reverter: corrigir ou remover os documentos do PKG-58 antes de iniciar pacote dependente.

### DEC-20260618-02 - Runtime v2 compativel por metadados aditivos

- Status: aceita
- Contexto: PKG-59 precisa criar base para Collector, Forwarder, Scheduler, Supervisor e ExtensionRuntime sem quebrar os coletores legados nem os snapshots do web.
- Decisao: criar `internal/domain/runtime` com contratos internos e marcar o pipeline atual como `2-compatible`. `CollectAndBuffer` adiciona metadados opcionais ao corpo JSON (`schema_version`, `agent_pipeline_version`, `collector_name`, `ingest_endpoint`) preservando as secoes antigas. O health local e o snapshot remoto expõem a versao do pipeline e o scheduler snapshot.
- Alternativas consideradas: refatorar todo `app.Run` de uma vez ou criar um runtime paralelo. Ambas aumentariam risco de regressao.
- Consequencias: novos coletores podem migrar para o contrato v2 sem alterar endpoints atuais.
- Impacto em testes: testes unitarios cobrem metadados, health e snapshot de runtime.
- Impacto em rollback: publicar a versao anterior do agente; o backend ignora campos aditivos.
- Como reverter: remover injecao de metadados e voltar `CollectAndBuffer` ao corpo original.

### DEC-20260618-03 - Logs v2 usam campos aditivos no contrato legado

- Status: aceita
- Contexto: PKG-60 precisa evoluir logs com parsing, redaction, cursor e filtros sem quebrar `/v1/logs/raw`, SOC/NOC e agentes ja instalados.
- Decisao: manter o coletor `oslogs` e adicionar campos opcionais por evento, com `schema_version=1`, `redaction_status`, atributos JSON sanitizados, origem real e contagem agregada `dropped_count`. Filtros locais aceitos por env/config remota nao persistem conteudo descartado.
- Alternativas consideradas: criar endpoint novo de logs ou persistencia nova no agente.
- Consequencias: o backend continua aceitando eventos antigos e novos; recursos avancados como OTLP, dual-shipping e listeners locais ficam em pacotes proprios.
- Impacto em testes: unitarios cobrem redaction, JSON attributes, multiline e filtros; build Windows do coletor e validado por `GOOS=windows`.
- Impacto em rollback: desativar `OSLOG_ENABLED`/flags remotas de logs ou publicar versao anterior.
- Como reverter: remover campos aditivos e filtros do coletor `oslogs`.

### DEC-20260618-04 - Metricas custom usam receptor local desligado por padrao

- Status: aceita
- Contexto: PKG-61 precisa permitir metricas de aplicacoes locais sem compartilhar credencial da API e sem misturar com snapshots base do host.
- Decisao: criar coletor `custommetrics`, desligado por padrao, com UDP DogStatsD-like e HTTP local loopback. O coletor agrega por janela e envia contrato aditivo em `body.custom_metrics` para `/v1/ingest/metrics`.
- Alternativas consideradas: exigir integracao direta da app com a API ou criar persistencia nova no agente.
- Consequencias: o backend preserva o payload em `raw_json`; materializacao/telas podem evoluir sem quebrar o agente.
- Impacto em testes: unitarios cobrem parser, agregacao, limite de cardinalidade e HTTP local.
- Impacto em rollback: desligar `CUSTOM_METRICS_ENABLED` ou config remota `custom_metrics.enabled=false`.
- Como reverter: remover o coletor do scheduler e ignorar `body.custom_metrics`.

### DEC-20260618-05 - OTLP inicial usa HTTP/JSON local

- Status: aceita
- Contexto: PKG-62 precisa receber OpenTelemetry sem criar dependencias pesadas ou quebrar o agente atual.
- Decisao: suportar primeiro OTLP HTTP/JSON em loopback, desligado por padrao. Metrics e traces seguem para `/v1/ingest/metrics`; logs seguem para `/v1/logs/raw`. OTLP gRPC/protobuf fica para evolucao posterior.
- Alternativas consideradas: embutir collector OpenTelemetry completo ou exigir sidecar externo.
- Consequencias: apps com exporter HTTP/JSON podem enviar sinais basicos; persistencia APM dedicada e redaction completa de logs OTLP ficam para PKG-63/PKG-69.
- Impacto em testes: unitarios cobrem ingestao HTTP/JSON de metrics, logs e traces.
- Impacto em rollback: desligar `OTLP_ENABLED` ou config remota `otlp.enabled=false`.
- Como reverter: remover coletores `otlp_*` do scheduler.

### DEC-20260618-06 - Containers iniciam por Docker socket opt-in

- Status: aceita
- Contexto: PKG-64 precisa descobrir workloads sem exigir agente Kubernetes nem dependencias externas.
- Decisao: criar coletor `containers`, desligado por padrao, usando Docker API via socket Unix local para discovery e metricas basicas. Containerd nativo, logs e autodiscovery de checks ficam para evolucao posterior.
- Alternativas consideradas: depender do Docker CLI ou embutir SDK externo.
- Consequencias: hosts Docker podem enviar inventario/metricas de containers sem quebrar host metrics.
- Impacto em testes: unitarios cobrem normalizacao, labels sensiveis e stats.
- Impacto em rollback: desligar `CONTAINER_ENABLED` ou config remota `containers.enabled=false`.
- Como reverter: remover coletor `containers` do scheduler.

### DEC-20260618-07 - Kubernetes usa API do cluster com RBAC minimo

- Status: aceita
- Contexto: PKG-65 precisa instalar o agente como DaemonSet e coletar metadados Kubernetes sem exigir cluster agent nem permissao ampla por padrao.
- Decisao: criar manifest direto e Helm chart com ServiceAccount, ClusterRole somente leitura para `nodes`, `pods` e `events`, alem de coletor opt-in via API Kubernetes usando o token da ServiceAccount.
- Alternativas consideradas: usar SDK Kubernetes, coletar direto do kubelet ou introduzir Cluster Agent agora.
- Consequencias: entrega discovery de node/pod/container, events e annotations com baixa dependencia externa; uso real de Metrics API/kubelet, logs de pod e execucao de checks ficam para evolucao controlada.
- Impacto em testes: unitarios cobrem normalizacao, sanitizacao e autodiscovery por annotations; cluster real fica para homologacao PKG-69.
- Impacto em rollback: remover DaemonSet/Helm ou desligar `KUBERNETES_ENABLED`.
- Como reverter: remover coletor `kubernetes` do scheduler e apagar os manifests.

### DEC-20260618-08 - Checks locais usam allowlist sem shell generico

- Status: aceita
- Contexto: PKG-66 precisa executar checks locais extensiveis sem transformar config remota em execucao arbitraria.
- Decisao: criar coletor `localchecks`, desligado por padrao, com contrato aditivo em `body.local_checks`, timeout por check e allowlist de tipos (`http`, `tcp`, `openmetrics` e checks basicos de integracoes). Nao ha shell generico, comando livre, PowerShell remoto ou script arbitrario.
- Alternativas consideradas: plugins executaveis externos ja no primeiro pacote ou reusar Agentless remoto para tudo.
- Consequencias: checks locais simples rodam no agente sem duplicar Agentless remoto; integracoes profundas ficam para manifestos oficiais versionados.
- Impacto em testes: unitarios cobrem HTTP, TCP, OpenMetrics, bloqueio de tipo nao permitido e redaction.
- Impacto em rollback: desligar `LOCAL_CHECKS_ENABLED` ou config remota `local_checks.enabled=false`.
- Como reverter: remover coletor `localchecks` do scheduler.

### DEC-20260618-09 - Fleet usa snapshot sanitizado e flare sem shell

- Status: aceita
- Contexto: PKG-67 precisa apoiar operacao de frota, rollout e diagnostico sem coletar segredo nem abrir execucao remota generica.
- Decisao: expor `fleet_runtime` no snapshot sanitizado com versao, modo, config hash, drift basico e estado de rollback; adicionar comando permitido `collect_support_flare` com redaction recursiva.
- Alternativas consideradas: coletar arquivos arbitrarios por comando remoto ou criar um agente de frota separado.
- Consequencias: o backend pode montar visao de frota e canario usando evidencias existentes; coleta profunda de bundle fica restrita a allowlist e pode evoluir incrementalmente.
- Impacto em testes: unitarios cobrem redaction do flare e allowlist do comando.
- Impacto em rollback: remover comando da allowlist ou desabilitar no backend.
- Como reverter: remover `collect_support_flare` e `fleet_runtime` do snapshot.

### DEC-20260618-10 - Config sensivel exige assinatura HMAC opt-in

- Status: aceita
- Contexto: PKG-68 precisa endurecer update, config remota e credenciais sem quebrar agentes legados.
- Decisao: validar HMAC-SHA256 em payload sensivel quando `REMOTE_CONFIG_SIGNATURE_SECRET` estiver configurado, com modo obrigatorio por `REMOTE_CONFIG_SIGNATURE_REQUIRED`. Downgrade de update sem `force` e bloqueado, token rotation persiste novo token sem logar segredo, e `security_runtime` expõe estado sem valores sensiveis.
- Alternativas consideradas: exigir assinatura obrigatoria imediatamente ou esperar backend completo de PKI.
- Consequencias: rollout pode ativar assinatura por cliente/frota; assinatura assimetrica, revogacao backend e FIPS ficam para evolucao/homologacao.
- Impacto em testes: unitarios cobrem assinatura, payload sensivel sem assinatura, downgrade e rotacao.
- Impacto em rollback: desativar `REMOTE_CONFIG_SIGNATURE_REQUIRED` ou permitir unsigned sensitive temporariamente.
- Como reverter: remover validação de assinatura e token rotation do config payload.

### DEC-20260618-11 - Homologacao operacional separa local de ambiente real

- Status: aceita
- Contexto: PKG-69 precisa provar maturidade por ambiente e falha, sem tratar `./check.sh` local como homologacao multiplataforma.
- Decisao: criar `scripts/pkg69_operational_homologation.sh` e `docs/pkg69_operational_matrix.md` para registrar evidencia local, prontidao de ferramentas e pendencias por Windows, Linux, Docker, Kubernetes, proxy, disco, payload e update rollback.
- Alternativas consideradas: declarar pacotes anteriores como fechados apenas por testes unitarios e check local.
- Consequencias: a matriz diferencia validacao local de validacao real; pendencias reais seguem explicitas ate execucao em ambiente controlado.
- Impacto em testes: script roda testes focados e `./check.sh`.
- Impacto em rollback: nao altera runtime.
- Como reverter: remover script/matriz sem impacto no binario.

### DEC-20260618-12 - USM e workload security evoluem sobre networkcapture

- Status: aceita
- Contexto: PKG-70 precisa descobrir servicos sem instrumentacao, dependencias e sinais de workload security sem criar grafo paralelo nem exigir eBPF em todos os hosts.
- Decisao: evoluir `networkcapture` com payloads aditivos `service_map`, `network_performance` e `workload_security`, ativados por `network_advanced_enabled`, `usm_enabled` e `workload_security_enabled`. O modo `network_passive_mode=ebpf` apenas declara tentativa de system probe e degrada para socket/netlink quando nao suportado.
- Alternativas consideradas: criar novo coletor USM separado ou exigir eBPF como dependencia obrigatoria.
- Consequencias: backend antigo continua aceitando payloads atuais; USM fica opt-in, auditavel e com fallback claro. Workload security gera evidencia SOC, sem acao destrutiva automatica.
- Impacto em testes: unitarios cobrem servico inferido, dependencia por trafego, fallback eBPF, NPM e sinais de seguranca.
- Impacto em rollback: desligar `network_advanced_enabled`, `usm_enabled`, `workload_security_enabled`, `network_pcap_enabled` e voltar `network_passive_mode=socket`.
- Como reverter: ignorar/remover os blocos aditivos e manter `networkcapture` legado.

### DEC-20260618-13 - Integracoes avancadas exigem manifest e estado de homologacao

- Status: aceita
- Contexto: PKG-71 precisa ampliar checks locais sem virar catalogo sem qualidade nem duplicar Agentless.
- Decisao: evoluir `localchecks` com manifests versionados, status `official|beta|experimental`, OpenMetrics com allowlist/cardinalidade e JMX apenas via Jolokia HTTP. Integracoes profundas de WMI e bancos ficam beta/experimental ate validacao real.
- Alternativas consideradas: embutir plugins executaveis externos ou coletar JMX/WMI por comando local.
- Consequencias: o agente continua sem shell arbitrario; integracoes oficiais tem manifest, teste e rollback; ativacao ampla de beta/experimental fica bloqueada por governanca operacional.
- Impacto em testes: unitarios cobrem OpenMetrics, cardinalidade, Jolokia e metadados oficiais.
- Impacto em rollback: desativar a integracao especifica em `local_checks` ou `local_checks_enabled=false`.
- Como reverter: remover manifests novos e voltar ao runtime de checks basicos do PKG-66.

### DEC-20260618-14 - Diferenciais AIceberg começam por evidencia contextual deterministica

- Status: aceita
- Contexto: PKG-72 precisa mostrar diferenciais de NOC/SOC, IA local, offline e agent+agentless sem declarar superioridade sem benchmark.
- Decisao: adicionar `contextual_evidence` ao snapshot seguro com evidencias, lacunas, regras deterministicas, privacidade, offline-first e estrategia agent+agentless. `superiority_benchmark.claim_allowed=false` permanece ate haver comparacao objetiva.
- Alternativas consideradas: embutir LLM local no agente ou declarar superioridade por implementacao de pacotes.
- Consequencias: o backend pode exibir evidencias e lacunas sem mudar ingestao; IA local fica limitada a regras auditaveis e sem acao destrutiva.
- Impacto em testes: unitario cobre snapshot, privacidade sensivel, bloqueio de acao destrutiva e bloqueio de claim.
- Impacto em rollback: ignorar `contextual_evidence` no backend ou remover o bloco do snapshot.
- Como reverter: remover `buildContextualEvidenceSnapshot` sem afetar coletores existentes.

## Adaptacao deste projeto

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.

## Modelo de decisao

### DEC-YYYYMMDD-01 - Titulo da decisao

- Contexto:
- Decisao:
- Alternativas consideradas:
- Impacto:
- Rollback:
