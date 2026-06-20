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

### DEC-20260619-01 - Cursor de arquivo considera identidade e fronteira de linha

- Status: aceita
- Contexto: PKG-60 ja resetava cursor quando o arquivo ficava menor, mas podia reutilizar offset antigo se um log fosse truncado/recriado com tamanho maior que o offset anterior ou rotacionado por troca de inode.
- Decisao: persistir chave auxiliar de identidade do arquivo no cursor POSIX e validar que o offset salvo continua em fronteira de linha antes de reutiliza-lo.
- Consequencias: reduz perda parcial de linhas e duplicacao indevida em restart, truncamento e rotacao comum sem alterar o contrato `/v1/logs/raw`.
- Impacto em testes: `TestCollectorPersistsCursorAcrossRestartAndHandlesRotation` cobre restart sem duplicacao, truncamento e rotacao.
- Impacto em rollback: publicar binario anterior ou remover a chave auxiliar; cursores antigos continuam compativeis porque a chave adicional e opcional.
- Como reverter: remover `fileIdentityCursorKey`, `fileIdentity` e `cursorAtLineBoundary` do coletor POSIX.

### DEC-20260618-04 - Metricas custom usam receptor local desligado por padrao

- Status: aceita
- Contexto: PKG-61 precisa permitir metricas de aplicacoes locais sem compartilhar credencial da API e sem misturar com snapshots base do host.
- Decisao: criar coletor `custommetrics`, desligado por padrao, com UDP DogStatsD-like, UDS POSIX e HTTP local loopback. O coletor agrega por janela e envia contrato aditivo em `body.custom_metrics` para `/v1/ingest/metrics`.
- Alternativas consideradas: exigir integracao direta da app com a API ou criar persistencia nova no agente.
- Consequencias: o backend preserva o payload em `raw_json`; materializacao/telas podem evoluir sem quebrar o agente.
- Impacto em testes: unitarios cobrem parser, agregacao, limite de cardinalidade, HTTP local, UDP, UDS e evidencia opt-in do PKG-61.
- Impacto em rollback: desligar `CUSTOM_METRICS_ENABLED` ou config remota `custom_metrics.enabled=false`.
- Como reverter: remover o coletor do scheduler e ignorar `body.custom_metrics`.

### DEC-20260618-05 - OTLP inicial usa HTTP/JSON local

- Status: aceita
- Contexto: PKG-62 precisa receber OpenTelemetry sem criar dependencias pesadas ou quebrar o agente atual.
- Decisao: suportar primeiro OTLP HTTP/JSON em loopback, desligado por padrao. Metrics e traces seguem para `/v1/ingest/metrics`; logs seguem para `/v1/logs/raw`. OTLP gRPC/protobuf fica para evolucao posterior.
- Alternativas consideradas: embutir collector OpenTelemetry completo ou exigir sidecar externo.
- Consequencias: apps com exporter HTTP/JSON podem enviar sinais basicos; persistencia APM dedicada fica para PKG-63. O fechamento operacional valida app de exemplo, servico simples instrumentado e redaction; gRPC/protobuf permanece fora deste pacote.
- Impacto em testes: unitarios cobrem ingestao HTTP/JSON de metrics, logs e traces.
- Impacto em rollback: desligar `OTLP_ENABLED` ou config remota `otlp.enabled=false`.
- Como reverter: remover coletores `otlp_*` do scheduler.

### DEC-20260618-06 - Containers iniciam por Docker socket opt-in e autodiscovery seguro

- Status: aceita
- Contexto: PKG-64 precisa descobrir workloads sem exigir agente Kubernetes nem dependencias externas.
- Decisao: criar coletor `containers`, desligado por padrao, usando Docker API via socket Unix local para discovery, metricas basicas, logs JSON com cursor/redaction e autodiscovery por labels. Containerd entra por `ctr` local para metadados e checks; metricas/logs nativos de containerd ficam para hardening futuro.
- Alternativas consideradas: depender do Docker CLI ou embutir SDK externo.
- Consequencias: hosts Docker podem enviar inventario, metricas, logs e checks descobertos sem quebrar host metrics; env vars e volumes nao sao coletados.
- Impacto em testes: unitarios cobrem normalizacao, labels sensiveis, stats, lifecycle, autodiscovery e ausencia de env/volume sensivel no payload.
- Impacto em rollback: desligar `CONTAINER_ENABLED` ou config remota `containers.enabled=false`.
- Como reverter: remover coletor `containers` do scheduler.

### DEC-20260618-07 - Kubernetes usa API do cluster, RBAC minimo e Metrics API opcional

- Status: aceita
- Contexto: PKG-65 precisa instalar o agente como DaemonSet e coletar metadados Kubernetes sem exigir cluster agent nem permissao ampla por padrao.
- Decisao: criar manifest direto e Helm chart com ServiceAccount, ClusterRole somente leitura para `nodes`, `pods`, `pods/log` e `events`, alem de coletor opt-in via API Kubernetes usando o token da ServiceAccount. Metrics API `metrics.k8s.io` e consultada de forma opcional; ausencia de Metrics Server nao falha o coletor.
- Alternativas consideradas: usar SDK Kubernetes, coletar direto do kubelet ou introduzir Cluster Agent agora.
- Consequencias: entrega discovery de node/pod/container, events, logs de pod, annotations, checks descobertos e metricas quando disponiveis, com baixa dependencia externa; kubelet direto e Cluster Agent ficam fora do gate inicial.
- Impacto em testes: unitarios cobrem normalizacao, sanitizacao, logs, autodiscovery e Metrics API; cluster real fica para homologacao PKG-69.
- Impacto em rollback: remover DaemonSet/Helm ou desligar `KUBERNETES_ENABLED`.
- Como reverter: remover coletor `kubernetes` do scheduler e apagar os manifests.

### DEC-20260618-08 - Checks locais usam allowlist sem shell generico

- Status: aceita
- Contexto: PKG-66 precisa executar checks locais extensiveis sem transformar config remota em execucao arbitraria.
- Decisao: criar coletor `localchecks`, desligado por padrao, com contrato aditivo em `body.local_checks`, timeout por check, manifests locais instalaveis e allowlist de tipos (`http`, `tcp`, `openmetrics` e checks basicos de integracoes). Nao ha shell generico, comando livre, PowerShell remoto ou script arbitrario.
- Alternativas consideradas: plugins executaveis externos ja no primeiro pacote ou reusar Agentless remoto para tudo.
- Consequencias: checks locais simples rodam no agente sem duplicar Agentless remoto; upgrade de manifest nao altera config do check e rollback operacional desliga o coletor.
- Impacto em testes: unitarios cobrem HTTP, TCP, OpenMetrics, bloqueio de tipo nao permitido, redaction, falha, rollback e upgrade de manifest.
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
- Consequencias: rollout pode ativar assinatura por cliente/frota; cadeia Ed25519 e revogacao backend foram evoluidas no mesmo pacote; FIPS fica para homologacao real.
- Impacto em testes: unitarios cobrem assinatura, payload sensivel sem assinatura, downgrade e rotacao.
- Impacto em rollback: desativar `REMOTE_CONFIG_SIGNATURE_REQUIRED` ou permitir unsigned sensitive temporariamente.
- Como reverter: remover validação de assinatura e token rotation do config payload.

### DEC-20260618-16 - Update valida cadeia Ed25519 de artefato

- Status: aceita
- Contexto: SHA256 valida integridade do download, mas nao prova a origem do artefato publicado.
- Decisao: adicionar validacao opt-in/obrigatoria de assinatura Ed25519 do update sobre versao, SHA256 e `signing_key_id`, ativada por `AUTO_UPDATE_TRUST_PUBLIC_KEY` ou `AUTO_UPDATE_TRUST_REQUIRED=true`.
- Alternativas consideradas: manter apenas SHA256 ou exigir PKI completa imediatamente.
- Consequencias: agentes legados continuam aceitando SHA256; clientes regulados podem exigir assinatura antes do `apply`; publicacao oficial precisa gerar assinatura correspondente.
- Impacto em testes: unitarios cobrem assinatura valida e assinatura invalida antes do `apply`.
- Impacto em rollback: remover `AUTO_UPDATE_TRUST_REQUIRED` ou limpar `AUTO_UPDATE_TRUST_PUBLIC_KEY` durante janela controlada.
- Como reverter: publicar agente anterior ou remover os campos de assinatura do payload remoto.

### DEC-20260618-11 - Homologacao operacional separa local de ambiente real

- Status: aceita
- Contexto: PKG-69 precisa provar maturidade por ambiente e falha, sem tratar `./check.sh` local como homologacao multiplataforma.
- Decisao: criar `scripts/pkg69_operational_homologation.sh` e `docs/pkg69_operational_matrix.md` para registrar evidencia local, prontidao de ferramentas e pendencias por Windows, Linux, Docker, Kubernetes, proxy, disco, payload e update rollback.
- Alternativas consideradas: declarar pacotes anteriores como fechados apenas por testes unitarios e check local.
- Consequencias: a matriz diferencia validacao local de validacao real; pendencias reais seguem explicitas ate execucao em ambiente controlado.
- Impacto em testes: script roda testes focados e `./check.sh`.
- Impacto em rollback: nao altera runtime.
- Como reverter: remover script/matriz sem impacto no binario.

### DEC-20260619-01 - Intervalo de sysmetrics configuravel para reduzir overhead

- Status: aceita
- Contexto: validacao real do agente `70` em Ubuntu user-mode mostrou CPU ~7.7% com sysmetrics a cada 10s, acima do limite inicial de fechamento do PKG-69.
- Decisao: adicionar `METRICS_INTERVAL` para controlar o intervalo da coleta principal de metricas do host, mantendo default `10` segundos para compatibilidade retroativa.
- Alternativas consideradas: desligar coletores via prefs, aumentar todos os intervalos de forma global ou aceitar CPU acima do limite.
- Consequencias: ambientes reais podem reduzir overhead sem recompilar nem desativar capacidades; fechamento do gate ainda exige evidencia real por host.
- Impacto em testes: unitarios cobrem env customizado e fallback para default quando o valor nao e positivo.
- Impacto em rollback: remover `METRICS_INTERVAL` do ambiente volta ao default anterior de 10s.
- Como reverter: restaurar binario anterior ou remover o campo/configuracao e voltar ao ticker hardcoded.

### DEC-20260619-02 - Fechamento de logs exige evidencia real separada de teste controlado

- Status: aceita
- Contexto: PKG-60 tem testes controlados cobrindo parsing, cursor, Graylog/GELF, Linux auth, app JSON, texto comum, Windows Security e Sysmon, mas esses testes nao provam comportamento em fonte real operacional.
- Decisao: criar gate `scripts/pkg60_logs_evidence_gap_report.sh` para distinguir evidencia controlada de homologacao real. O pacote so pode ser aceito quando os cenarios `pkg60-real-os-files`, `pkg60-real-source-formats` e `pkg60-real-journald-windows-channels` tiverem bundles reais e `PKG60_ACCEPT_CLOSURE=true`.
- Alternativas consideradas: fechar PKG-60 com evidencias controladas ou reaproveitar genericamente a matriz PKG-69.
- Consequencias: o pacote permaneceu bloqueado de forma objetiva ate haver prova real de Windows EventLog, Linux syslog/auth, Graylog, app JSON/texto, journald, Security/System/Application e Sysmon; em 2026-06-19 o gate passou com 3/3 evidencias reais e aceite explicito.
- Impacto em testes: `./check.sh` passa a rodar o self-test do gate; `PKG60_GAP_REPORT_REQUIRE_COMPLETE=true` falha enquanto faltar qualquer bundle real.
- Impacto em rollback: nao altera runtime; remover o gate volta ao controle manual por checklist.
- Como reverter: remover scripts do gate, self-test no `check.sh` e referencias em docs.

### DEC-20260619-03 - APM inicial fecha por traces, correlacao e overhead de OTLP, sem profiler

- Status: aceita
- Contexto: PKG-63 precisa validar alto volume, erro de aplicacao, jornada log -> trace -> servico -> host e overhead antes de habilitar APM para cliente, mas profiler continuo foi explicitamente colocado fora do escopo inicial.
- Decisao: aceitar o APM inicial quando o receiver OTLP HTTP/JSON provar sampling com erro/span lento preservados, log correlato e host/service preservados, usando `docs/evidence/pkg63/apm-high-volume-error-20260619T183604Z`; overhead de ativacao ampla fica coberto pela evidencia PKG-69 `high-volume-overhead`.
- Alternativas consideradas: bloquear todo o APM ate implementar profiler ou declarar profiler coberto por traces.
- Consequencias: APM/traces inicial pode ser fechado sem falsa paridade de profiler; a matriz continua marcando profiler como fora do escopo inicial ate pacote/decisao futura.
- Impacto em testes: `TestPKG63APMHighVolumeErrorJourneyEvidence` cobre 80 spans, `dropped_count`, erro, span lento e jornada correlata.
- Impacto em rollback: desligar OTLP/APM com `OTLP_ENABLED=false` ou ajustar `APM_TRACE_SAMPLE_RATE=1` para remover descarte por sampling.
- Como reverter: remover a evidencia/teste dedicado e voltar o PKG-63 para pendente de validacao.

### DEC-20260618-12 - USM e workload security evoluem sobre networkcapture

- Status: aceita
- Contexto: PKG-70 precisa descobrir servicos sem instrumentacao, dependencias e sinais de workload security sem criar grafo paralelo nem exigir eBPF em todos os hosts.
- Decisao: evoluir `networkcapture` com payloads aditivos `service_map`, `network_performance` e `workload_security`, ativados por `network_advanced_enabled`, `usm_enabled` e `workload_security_enabled`. O modo `network_passive_mode=ebpf` apenas declara tentativa de system probe e degrada para socket/netlink quando nao suportado.
- Alternativas consideradas: criar novo coletor USM separado ou exigir eBPF como dependencia obrigatoria.
- Consequencias: backend antigo continua aceitando payloads atuais; USM fica opt-in, auditavel e com fallback claro. Workload security gera evidencia SOC, sem acao destrutiva automatica.
- Impacto em testes: unitarios cobrem servico inferido, dependencia por trafego, fallback eBPF, NPM e sinais de seguranca.
- Impacto em rollback: desligar `network_advanced_enabled`, `usm_enabled`, `workload_security_enabled`, `network_pcap_enabled` e voltar `network_passive_mode=socket`.
- Como reverter: ignorar/remover os blocos aditivos e manter `networkcapture` legado.

### DEC-20260619-18 - PKG-70 fecha com eBPF opt-in e evidencia controlada

- Status: aceita
- Contexto: a validacao real do PKG-69 ja cobre permissao eBPF restrita, Docker, Kubernetes e overhead, mas a versao atual do PKG-70 nao entrega um system probe eBPF produtivo obrigatorio.
- Decisao: fechar o PKG-70 com contrato aditivo, fallback seguro e evidencia controlada `docs/evidence/pkg70/network-usm-workload-20260619T192000Z`, sem declarar eBPF kernel ativo como superioridade ou requisito de ativacao ampla.
- Alternativas consideradas: bloquear o pacote ate um probe eBPF produtivo ou declarar a presenca de `/sys/fs/bpf` como prova suficiente.
- Consequencias: o agente entrega USM/workload security inicial, NPM e service map com privacidade; hosts sem permissao continuam em socket/netlink/pcap; eBPF produtivo fica para hardening futuro.
- Impacto em testes: teste dedicado gera bundle com fluxo `web -> api -> postgres`, contrato `ebpf_probe`, fallback, porta exposta, sinais SOC e redaction.
- Impacto em rollback: desligar `network_advanced_enabled`, `usm_enabled`, `workload_security_enabled`, `network_pcap_enabled` e voltar `network_passive_mode=socket`.
- Como reverter: remover o teste/evidencia e voltar os checkboxes de validacao do PKG-70 para pendente.

### DEC-20260618-13 - Integracoes avancadas exigem manifest e estado de homologacao

- Status: aceita
- Contexto: PKG-71 precisa ampliar checks locais sem virar catalogo sem qualidade nem duplicar Agentless.
- Decisao: evoluir `localchecks` com manifests versionados, status `official|beta|experimental`, OpenMetrics com allowlist/cardinalidade e JMX apenas via Jolokia HTTP. Integracoes profundas de WMI e bancos ficam beta/experimental ate validacao real.
- Alternativas consideradas: embutir plugins executaveis externos ou coletar JMX/WMI por comando local.
- Consequencias: o agente continua sem shell arbitrario; integracoes oficiais tem manifest, teste e rollback; ativacao ampla de beta/experimental fica bloqueada por governanca operacional.
- Impacto em testes: unitarios cobrem OpenMetrics, cardinalidade, Jolokia e metadados oficiais.
- Impacto em rollback: desativar a integracao especifica em `local_checks` ou `local_checks_enabled=false`.
- Como reverter: remover manifests novos e voltar ao runtime de checks basicos do PKG-66.

### DEC-20260619-19 - PKG-71 fecha com homologacao minima por evidencia controlada

- Status: aceita
- Contexto: as integracoes avancadas precisam de evidencia objetiva sem liberar beta/experimental em producao por acidente.
- Decisao: fechar o PKG-71 com `docs/evidence/pkg71/advanced-integrations-20260619T200500Z`, cobrindo `/metrics`, Jolokia, WMI/IIS guard, reachability de banco/fila/web server, falha controlada e bloqueio beta sem homologacao.
- Alternativas consideradas: exigir bancos reais e Windows Server real para fechar o pacote ou tratar unitarios antigos como suficientes.
- Consequencias: o catalogo fica pronto para canario/homologacao por cliente; Windows Server real, bancos produtivos e marketplace amplo continuam exigindo `homologation_ref` e rollback por integracao antes de ativacao ampla.
- Impacto em testes: teste dedicado gera bundle com status por integracao e checa ausencia de vazamento de `credentials_ref` e metricas negadas.
- Impacto em rollback: remover a integracao especifica de `local_checks` ou desligar `local_checks_enabled=false`.
- Como reverter: remover a evidencia/teste dedicado e voltar checkboxes de validacao do PKG-71 para pendente.

### DEC-20260618-14 - Diferenciais AIceberg começam por evidencia contextual deterministica

- Status: aceita
- Contexto: PKG-72 precisa mostrar diferenciais de NOC/SOC, IA local, offline e agent+agentless sem declarar superioridade sem benchmark.
- Decisao: adicionar `contextual_evidence` ao snapshot seguro com evidencias, lacunas, regras deterministicas, privacidade, offline-first e estrategia agent+agentless. `superiority_benchmark.claim_allowed=false` permanece ate haver comparacao objetiva.
- Alternativas consideradas: embutir LLM local no agente ou declarar superioridade por implementacao de pacotes.
- Consequencias: o backend pode exibir evidencias e lacunas sem mudar ingestao; IA local fica limitada a regras auditaveis e sem acao destrutiva.
- Impacto em testes: unitario cobre snapshot, privacidade sensivel, bloqueio de acao destrutiva e bloqueio de claim.
- Impacto em rollback: ignorar `contextual_evidence` no backend ou remover o bloco do snapshot.
- Como reverter: remover `buildContextualEvidenceSnapshot` sem afetar coletores existentes.

### DEC-20260619-20 - PKG-72 fecha com diferenciais funcionais e superioridade bloqueada

- Status: aceita
- Contexto: PKG-72 exige evidencias de NOC/SOC contextual, IA local, offline-first, privacidade, Agent+Agentless e benchmark sem permitir claim fraco contra Datadog.
- Decisao: fechar o pacote com `docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z`, que anexa cinco evidencias auditaveis ao gate final e mantem `superiority_benchmark.claim_allowed=false`. A evidencia de benchmark valida a trava de governanca, nao uma declaracao comercial de superioridade.
- Alternativas consideradas: declarar superioridade por implementacao dos diferenciais ou bloquear o pacote ate um lab Datadog produtivo. A primeira violaria a governanca; a segunda impediria fechar os diferenciais funcionais ja validados.
- Consequencias: o agente entrega diferenciais operacionais prontos para homologacao por cliente; qualquer claim de superioridade segue dependente de benchmark Datadog comparavel, raw evidence e revisao operacional.
- Impacto em testes: gate final roda `go test ./internal/bootstrap ./internal/data/local/outbox`, valida as cinco evidencias anexadas e exige `PKG72_ACCEPT_CLOSURE=true`.
- Impacto em rollback: ignorar `contextual_evidence` no backend ou desligar o diferencial por configuracao sem alterar ingestao, scheduler, outbox ou Agentless.
- Como reverter: remover o bundle de evidencia e voltar o PKG-72 para pendente de validacao.

### DEC-20260619-21 - PKG-73 adiciona contrato SOC aditivo aos logs do agente

- Status: aceita
- Contexto: PKG-54 no web precisa de origem real, elegibilidade e tipo SOC por log sem depender apenas do Graylog nem inferir tudo por texto.
- Decisao: criar normalizador comum `internal/common/soclog` e enriquecer eventos de `oslogs`, journald, Windows EventLog, OTLP logs, Docker logs e Kubernetes pod logs com campos `aiceberg_*`, motivo de roteamento e campos SOC seguros. O agente sugere origem/elegibilidade; a decisao final LOG/NOC/SOC permanece no web.
- Alternativas consideradas: deixar todo roteamento no backend ou exigir Graylog como unica fonte de enriquecimento. Ambas foram rejeitadas por fragilidade e dependencia operacional.
- Consequencias: payloads antigos seguem validos; payloads novos carregam evidencia auditavel para PKG-54; DistributedCOM 10028 e logs operacionais nao viram SOC critico por padrao.
- Impacto em testes: unitarios cobrem Windows Security, Sysmon, DCOM operacional, Linux auth, Graylog/GELF, app JSON, journald, Docker, Kubernetes e OTLP; compilacao cruzada valida build Windows.
- Impacto em rollback: desligar logs, remover overrides `aiceberg.*` ou publicar versao anterior do agente.
- Como reverter: ignorar campos `aiceberg_*` no backend e remover o enriquecimento dos coletores.

### DEC-20260620-22 - PKG-74 descobre candidatos locais sem coletar tudo automaticamente

- Status: aceita
- Contexto: para POCs e investigação real, o agente precisa encontrar automaticamente IIS, Nginx, Apache, aplicações, bancos, filas, EventLog Security, Linux auth, Docker, Kubernetes, OTLP/APM e dependências locais. Coletar tudo que for encontrado por padrão criaria ruído, risco de segredo e custo de IA.
- Decisao: criar discovery local read-only, bounded e sanitizado que emite candidatos `log_source_discovery_v1`. O agente calcula evidência, confiança, severidade mínima, volume estimado, risco, utilidade, lacunas e permissões; a coleta ampla só ativa com configuração aprovada, assinada e escopada pelo web.
- Alternativas consideradas: exigir configuração manual por cliente ou coletar todos os arquivos/canais descobertos automaticamente. A primeira mantém lacuna operacional; a segunda viola governança de custo, segurança e privacidade.
- Consequencias: o agente passa a alimentar inventário e contexto de IA/NOC/SOC sem depender de Graylog, mas preserva controle humano/operacional para ativação de fonte, severidade `error+`, limites de volume, redaction e rollback.
- Impacto em testes: fixtures e evidências devem cobrir Linux, Windows, container, Kubernetes, OTLP/APM, permissão negada, segredo, severidade, volume, API indisponível, perda de rede, disco cheio/outbox e rollback.
- Impacto em rollback: desligar discovery por flag, ignorar `log_source_discovery_v1`, remover fontes aprovadas e manter coletas atuais.
- Como reverter: remover o coletor de discovery e publicar versão anterior sem afetar `oslogs`, snapshots, outbox ou auto-update.

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
