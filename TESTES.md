# TESTES.md

Fonte de verdade para testes automatizados, validação manual e reteste de bugs.

## Ordem de leitura

1. `PATHS.toml`
2. `QUALITY_ROADMAP.md`
3. `GOVERNANCA.md`
4. `DEMANDAS.md`
5. `TELAS.md`
6. `BUGS.md`

## Pirâmide de testes

### Unitário

- Testa regra pura, mapper, policy, validator, parser e cálculo.
- Deve ser rápido e determinístico.
- Não acessa rede, banco real, filesystem real ou UI real.

### Service/use case

- Testa fluxo de negócio.
- Usa mocks/fakes de repositories, gateways e adapters.
- Cobre caso feliz, validação, permissão e falha de dependência.

### Repository/adapter

- Testa transformação, query, serialização ou contrato com dependência externa.
- Pode usar banco local, fixture ou fake controlado.
- Não deve depender de produção.

### Contrato/API

- Testa payload público, status, erro, paginação, filtros e autorização.
- Deve impedir vazamento de entidade interna.

### Componente/widget

- Testa renderização, estados e ações principais.
- Cobre loading, empty, error, success e interação principal.

### E2E

- Usar para fluxo crítico e regressão de alto risco.
- Deve ser menor que o fluxo real completo quando possível.
- Precisa ter dados controlados e evidência clara.

## Cobertura mínima por criticidade

Cobertura aqui significa evidência obrigatória, não apenas percentual.

Percentual de cobertura pode ser ativado por projeto em `PATHS.toml`, mas não é obrigatório no modelo universal.

### P0 / crítico

Aplicável a login, permissão, pagamento, dados sensíveis, operação principal, banco, integrações críticas, publicação e rollback.

Exige:

- teste unitário para regra de negócio;
- teste de service/use case para fluxo principal;
- teste de contrato quando houver API;
- teste de permissão/autorização quando aplicável;
- teste de erro/falha de dependência;
- validação de rollback quando houver risco operacional ou banco;
- validação manual ampla ou E2E do fluxo principal no fechamento do pacote.

### P1 / importante

Aplicável a CRUDs principais, telas operacionais, relatórios importantes e fluxos usados com frequência.

Exige:

- teste unitário ou service quando houver regra;
- teste de contrato quando houver API;
- teste de componente/widget quando UI crítica;
- teste de erro principal;
- validação manual focada no fechamento.

### P2 / baixo risco

Aplicável a ajustes visuais, copy, documentação, melhorias pequenas e telas secundárias.

Exige:

- teste local/focado quando houver código;
- revisão de diff quando for documentação;
- validação visual quando a mudança for visível e houver ambiente disponível;
- sem obrigação de cobertura automatizada, salvo regressão recorrente.

## Cobertura percentual opcional

Use percentual quando o projeto já tiver ferramenta estável de cobertura.

Regras:

- não usar percentual como única métrica de qualidade;
- não bloquear projeto legado só por cobertura histórica baixa sem plano incremental;
- exigir cobertura maior apenas para código novo ou alterado quando fizer sentido;
- configurar comando e metas em `PATHS.toml`.

Configuração sugerida:

```toml
[quality.coverage]
enabled = true
min_lines = 70
critical_min_lines = 80
command = "comando de cobertura do projeto"
```

## Dados de teste e fixtures

Regra principal: teste não pode depender de dado real instável, estado manual ou ordem implícita de execução.

### Unitário

- Usar objetos em memória.
- Não acessar banco, rede, filesystem real ou UI real.
- Usar factories pequenas quando precisar montar entidades.

### Service/use case

- Usar fakes/mocks de repositories e gateways.
- Declarar dados explicitamente no teste.
- Evitar fixture global grande.

### Repository/adapter

- Usar banco local, fixture SQL ou arquivo controlado.
- Limpar estado antes/depois quando houver persistência.
- Não depender de produção.

### Contrato/API

- Usar payloads fixture versionados.
- Testar resposta esperada.
- Testar erro de validação.
- Testar autorização quando aplicável.

### E2E

- Usar seed controlado.
- Criar dados descartáveis.
- Limpar dados quando seguro.
- Não usar conta ou dado real sem autorização explícita.
- Evitar depender de horário real, ordenação solta ou dados que mudam.

## Regras de fixture

- Fixture deve ser pequena, legível e específica.
- Fixture compartilhada só quando representar contrato estável.
- Não colocar segredo real em fixture.
- Não usar dump grande de produção.
- Dado temporal deve usar data fixa ou clock controlado.
- IDs devem ser determinísticos quando o teste depende deles.
- Teste deve poder rodar isolado.
- Configurar diretórios de fixture em `PATHS.toml` quando o projeto usar fixtures.

## Roteiro para `PKG-XX`

1. Ler pacote em `DEMANDAS.md`.
2. Identificar módulos, contratos e telas afetadas.
3. Separar validação por lote e validação de fechamento do pacote.
4. Definir testes mínimos antes de implementar cada lote.
5. Criar ou ajustar testes junto da entrega quando a mudança introduz regra ou contrato.
6. Rodar validação rasa por lote.
7. Rodar validação completa somente quando o pacote estiver 100%.
8. Registrar validação e bloqueios.

## PKG-58 - Inventario e matriz de paridade do agente

- Matriz unica: `/Users/brenomayder/projects/web/public/aiceberg_web/docs/agente_datadog_paridade.md`.
- Inventario deste repo: `docs/pkg58_inventario_agente_atual.md`.
- Validacao focada: `go test ./internal/bootstrap ./internal/common/config ./internal/domain/usecase ./internal/platform/collectors/sysmetrics ./internal/platform/collectors/oslogs ./internal/platform/collectors/networkcapture`.
- Fechamento do pacote coordenado com web: rodar `./check.sh` neste repo e no `aiceberg_web`.
- PKG-58 nao valida runtime novo nem paridade operacional; Windows, Linux, container e Kubernetes ficam pendentes ate PKG-69.

## PKG-59 - Runtime Collector/Forwarder compativel

- Unitario focado: `go test ./internal/domain/runtime ./internal/domain/usecase ./internal/interfaces/health ./internal/bootstrap`.
- Contrato: validar que `CollectAndBuffer` preserva o corpo original e adiciona apenas `schema_version`, `agent_pipeline_version`, `collector_name` e `ingest_endpoint`.
- Diagnostico: validar que `/health` e `inspect_runtime_config` expõem `agent_pipeline_version` sem segredo.
- Fechamento: rodar `./check.sh`.
- Validacao real fechada pelo PKG-69 aceito em 2026-06-19: Windows, Linux, container, perda de rede, API indisponivel, disco cheio/outbox preservada, proxy/TLS e agente instalado anteriormente.

## PKG-60 - Pipeline seguro de logs

- Unitario focado: `go test ./internal/platform/collectors/oslogs ./internal/common/config ./internal/domain/usecase`.
- Build Windows do coletor em host nao Windows: `GOOS=windows GOARCH=amd64 go test -c -o /tmp/aiceberg_oslogs_windows.test.exe ./internal/platform/collectors/oslogs`.
- Evidencia controlada auditavel: `scripts/pkg60_logs_controlled_evidence.sh`, gerando `docs/evidence/pkg60/controlled-*/evidence.md`, `MANIFEST.tsv`, `PROVENANCE.tsv` e artefato bruto sem reter binario Windows no repo.
- Gate de lacunas: `scripts/pkg60_logs_evidence_gap_report.sh docs/evidence/pkg60`, com self-test em `scripts/pkg60_logs_evidence_gap_report_selftest.sh`. `PKG60_GAP_REPORT_REQUIRE_COMPLETE=true` deve falhar enquanto faltar evidencia real; `PKG60_GAP_REPORT_REQUIRE_ACCEPTED=true` exige tambem `PKG60_ACCEPT_CLOSURE=true`.
- Cobrir redaction de token/senha/Authorization, atributos JSON sanitizados, Graylog/GELF, Linux auth (`auth.log` e `/var/log/secure`), app JSON, log texto, multiline, cursor por arquivo/canal/journald, filtros include/exclude/min severity/unit/prioridade/provider/event_id, processors parse/remap/drop/mask/route/sample/enrich, dual-shipping restrito a `/v1/logs/*` e `dropped_count` sem conteudo descartado.
- Validacao real de fechamento exige bundles reais separados de teste controlado: `pkg60-real-os-files`, `pkg60-real-source-formats` e `pkg60-real-journald-windows-channels`. Estado aceito em 2026-06-19 com `PKG60_GAP_REPORT_REQUIRE_ACCEPTED=true PKG60_ACCEPT_CLOSURE=true scripts/pkg60_logs_evidence_gap_report.sh docs/evidence/pkg60`.

## PKG-61 - Metricas custom locais

- Unitario focado: `go test ./internal/platform/collectors/custommetrics ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`.
- Cobrir parser DogStatsD-like, HTTP local, tipos count/gauge/histogram/distribution/set/service_check, tags canonicas, limite de cardinalidade, `accepted_count` e `dropped_count`.
- Validacao real fechada por `docs/evidence/pkg61/local-app-high-volume-20260619T181900Z` e pelo bundle PKG-69 `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z`.

## PKG-62 - OTLP HTTP/JSON

- Unitario focado: `go test ./internal/platform/collectors/otlp ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`.
- Cobrir HTTP local loopback, metrics/logs/traces JSON, resource attributes essenciais, limite de atributos/cardinalidade, redaction de atributos sensiveis, `trace_id`/`span_id`, limite de itens e `dropped_count`.
- PKG-63: revisar `docs/pkg63_apm_sampling.md` para instrumentacao por linguagem com SDKs oficiais OpenTelemetry, sem SDK proprio AIceberg.
- Validacao real/controlada fechada por `docs/evidence/pkg62/example-service-otlp-20260619T182600Z` e alto volume OTLP com CPU/memoria coberto por `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z`. gRPC/protobuf permanece fora do escopo do PKG-62.

## PKG-63 - APM/traces sampling

- Unitario focado: `go test ./internal/platform/collectors/otlp ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`.
- Evidencia focada: `PKG63_EVIDENCE_DIR=/tmp/aiceberg_pkg63_apm_high_volume_error_20260619T183604Z go test ./internal/platform/collectors/otlp -run TestPKG63APMHighVolumeErrorJourneyEvidence -count=1 -v`.
- Bundle versionado: `docs/evidence/pkg63/apm-high-volume-error-20260619T183604Z`.
- Cobertura: alto volume controlado com 80 spans, sampling configurado, erro de aplicacao preservado, span lento preservado, log ERROR com `trace_id`/`span_id` e jornada log -> trace -> service -> host.
- Overhead antes de ativacao ampla: referenciado pelo bundle PKG-69 `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z`, com OTLP traces em alto volume e CPU/memoria registrados.
- Profiler: fora do escopo inicial por decisao; nao ha teste de profiler nem declaracao de cobertura.

## PKG-64 - Containers Docker inicial

- Unitario focado: `go test ./internal/platform/collectors/containers ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`.
- Cobrir normalizacao Docker, labels sensiveis mascaradas, compose service, CPU, memoria, rede e IO.
- Evidencia focada: `PKG64_EVIDENCE_DIR=/tmp/aiceberg_pkg64_container_lifecycle_20260619T184258Z go test ./internal/platform/collectors/containers -run TestPKG64ContainerLifecycleAutodiscoverySecretEvidence -count=1 -v`.
- Bundle versionado: `docs/evidence/pkg64/container-lifecycle-autodiscovery-secret-20260619T184258Z`.
- Cobertura: container parado, reiniciado, alta carga, autodiscovery por label em container novo e ausencia de env/volume sensivel no payload.
- Docker real/logs JSON/cursor/cleanup cobertos pelo bundle PKG-69 `docs/evidence/pkg69/docker-runtime-20260619T031843Z`.
- Containerd real continua como hardening operacional futuro; PKG-64 cobre parser/configuracao e fallback.

## PKG-70 - Rede avancada, USM e workload security

- Unitario focado: `go test ./internal/platform/collectors/networkcapture`.
- Cobrir servico inferido sem OpenTelemetry, `service/env/version` por metadado explicito ou cmdline, dependencia `service -> database`, fallback eBPF, NPM/top talkers, redaction de IP publico e sinais SOC sem acao destrutiva.
- Evidencia focada: `PKG70_EVIDENCE_DIR=/Users/brenomayder/projects/desktop/aiceberg_agent/docs/evidence/pkg70/network-usm-workload-20260619T192000Z go test ./internal/platform/collectors/networkcapture -run TestPKG70NetworkUSMWorkloadEvidence -count=1 -v`.
- Bundle versionado: `docs/evidence/pkg70/network-usm-workload-20260619T192000Z`.
- Cobertura: fluxo controlado `web -> api -> postgres`, dependencia `service -> service` e `service -> database`, fallback sem eBPF, estado eBPF ativo somente quando `applied_mode` traz `ebpf_probe`, porta administrativa publica degradada, sinais SOC evidence-only, `source_score` e redaction de IP publico em NPM/workload.
- Complemento real vem do PKG-69 para permissao eBPF restrita, Docker, Kubernetes e overhead. eBPF kernel ativo produtivo nao e reivindicado nesta versao.

## PKG-71 - Integracoes avancadas

- Unitario focado: `go test ./internal/platform/collectors/localchecks`.
- Cobrir OpenMetrics com allowlist/cardinalidade, labels permitidas, JMX via Jolokia, metadados de manifest oficial e bloqueio de tipo arbitrario.
- Evidencia focada: `PKG71_EVIDENCE_DIR=/Users/brenomayder/projects/desktop/aiceberg_agent/docs/evidence/pkg71/advanced-integrations-20260619T200500Z go test ./internal/platform/collectors/localchecks -run TestPKG71AdvancedIntegrationsEvidence -count=1 -v`.
- Bundle versionado: `docs/evidence/pkg71/advanced-integrations-20260619T200500Z`.
- Cobertura: `/metrics` controlado, JMX/Jolokia, WMI/IIS guard fora de Windows, PostgreSQL/RabbitMQ reachability, MySQL falha controlada, Nginx HTTP, beta sem homologacao bloqueado, sem vazamento de `credentials_ref` ou metrica negada. Windows Server real e credenciais produtivas continuam exigindo homologacao por cliente antes de ativacao ampla.

## PKG-72 - Diferenciais AIceberg

- Unitario focado: `go test ./internal/bootstrap ./internal/data/local/outbox`.
- Benchmark local bloqueante: `TestBuildSuperiorityBenchmarkEvidenceBlocksWeakClaims` valida claim bloqueado, politica comparavel, quatro cenarios, metricas e referencia Datadog obrigatoria; nao substitui benchmark real.
- Reducao de ruido local assistiva: `TestBuildLocalAINoiseReductionIsAssistiveOnly` valida pre-classificacao deterministica sem supressao automatica, sem descarte bruto, com benchmark e revisao humana obrigatorios; nao substitui comparacao real antes/depois.
- Correlacao local Agent+Agentless: `TestBuildContextualEvidenceAgentAgentlessCorrelationGaps` valida estrategias de divergencia e lacunas quando Agentless esta desligado ou worker indisponivel; nao substitui falha controlada real.
- Privacidade minima local: `TestBuildContextualEvidenceMinimalProfileAvoidsRawSecrets` valida `PRIVACY_PROFILE=minimal`, coletores minimizados, politica sem payload sensivel bruto e ausencia de segredo cru na evidencia contextual; nao substitui validacao real de cliente regulado.
- Replay offline 24h local controlado: `TestBoltStoreSimulates24hOfflineReplayWithoutDuplicatesAfterAck` simula 24 envelopes horarios, replay repetido antes do ACK e ACK idempotente com IDs duplicados; nao substitui evidencia real 24h.
- Homologacao reproduzivel: `scripts/pkg72_contextual_evidence_homologation.sh`.
- Autoteste do gate de evidencias: `scripts/pkg72_contextual_evidence_gate_selftest.sh` roda no `./check.sh`.
- Templates de evidencia real: `PKG72_TEMPLATE_DIR=/tmp/aiceberg_pkg72_templates scripts/pkg72_contextual_evidence_homologation.sh`.
- Manifesto auditavel de evidencia real: `PKG72_EVIDENCE_MANIFEST_TSV=/tmp/aiceberg_pkg72_manifest.tsv scripts/pkg72_contextual_evidence_homologation.sh`.
- Gate bloqueante de fechamento: `PKG72_REQUIRE_REAL_EVIDENCE=true scripts/pkg72_contextual_evidence_homologation.sh`.
- Gate de aceite final: `PKG72_REQUIRE_CLOSURE_ACCEPTED=true PKG72_ACCEPT_CLOSURE=true scripts/pkg72_contextual_evidence_homologation.sh`; sem `PKG72_ACCEPT_CLOSURE=true`, o roteiro nao autoriza fechamento mesmo com arquivos anexados.
- O gate nao conta template sem preenchimento, com campos de evidencia/metrica vazios, com `Status` diferente de `pass`, sem aprovacao de fechamento `yes` ou anexado na variavel errada como evidencia real.
- Cobrir `contextual_evidence`, privacidade sensivel, IA deterministica sem LLM, bloqueio de acao destrutiva, replay local ate ACK, topologia relay -> HUB -> AIceberg e claim de superioridade bloqueado sem benchmark.
- Fechamento controlado: `docs/evidence/pkg72/contextual-ai-offline-20260619T193000Z` roda o gate final com cinco anexos auditaveis e `PKG72_ACCEPT_CLOSURE=true`; a evidencia valida os diferenciais funcionais e a trava de nao declarar superioridade sem benchmark Datadog comparavel.

## Validação por lote

Objetivo: dar feedback rápido, sem gastar tempo com bateria completa a cada subentrega.

Rodar somente o menor conjunto que prova que o lote não quebrou o ponto alterado:

- lint/formatação do arquivo ou módulo tocado quando a stack permitir;
- teste unitário do service, função, controller, widget ou componente alterado;
- teste de contrato focado quando endpoint/payload mudou;
- build/type-check focado quando disponível e rápido;
- validação manual curta da tela/fluxo alterado quando houver UI.

Não é obrigatório rodar E2E completo, suíte inteira, teste externo lento ou validação manual ampla a cada lote.

Ao fechar lote:

- atualizar `DEMANDAS.md`;
- registrar teste raso executado;
- registrar pendência real se algo ficar para o fechamento do pacote;
- não fazer commit obrigatório ainda, salvo pedido explícito.

## Validação de fechamento do pacote

Quando todos os lotes do pacote estiverem implementados:

1. Rodar `./check.sh` completo.
2. Rodar suítes automatizadas relevantes para todos os módulos tocados.
3. Rodar E2E ou validação manual ampla do fluxo principal quando aplicável.
4. Retestar bugs relacionados.
5. Validar regressões dos lotes anteriores.
6. Atualizar `DEMANDAS.md`, `TELAS.md`, `BUGS.md`, `TESTES.md` e `RUNBOOK.md` quando aplicável.
7. Corrigir tudo que apareceu no fechamento, mesmo que venha de lote anterior do mesmo pacote.
8. Executar review de fechamento do pacote conforme `QUALITY_ROADMAP.md`.
9. Fazer commit do pacote fechado.

## Commit por pacote

- Todo pacote 100% implementado e validado deve terminar em commit.
- Não é obrigatório fazer push.
- O commit deve conter somente o pacote ou uma mudança lógica claramente relacionada.
- A mensagem deve citar o pacote quando existir: `Implementa PKG-XX ...`.
- O corpo do commit deve citar o check executado: `Check: ./check.sh`.
- Se houver SQL, citar scripts e ordem de execução.
- Se o pacote não puder fechar, não fazer commit de fechamento; registrar bloqueio real em `DEMANDAS.md`.

## Roteiro para `TELA-XX`

1. Ler tela em `TELAS.md`.
2. Validar campos, ações, estados e regras de exibição.
3. Validar responsividade quando houver UI real.
4. Capturar evidência visual quando possível.
5. Registrar bug se houver divergência.

## Roteiro para `BUG-XX`

1. Ler bug em `BUGS.md`.
2. Reproduzir passos.
3. Classificar risco da correção.
4. Aplicar menor correção possível.
5. Retestar conforme o risco.
6. Atualizar status, evidência e critério de fechamento.

## Validação para bug

### Bug simples

Use quando a causa é clara, o impacto é local e a correção toca pouco código.

Exemplos:

- label incorreto;
- texto, acento, capitalização ou copy;
- alinhamento local;
- validação simples;
- fallback visual isolado;
- erro em uma função pequena.

Validação esperada:

- reproduzir antes quando possível;
- aplicar correção;
- rodar teste local/focado;
- validar a tela, função ou fluxo específico;
- atualizar `BUGS.md` se o bug estava registrado.

Não exige suíte completa nem E2E amplo, salvo se o bug estiver em fluxo crítico.

### Bug complexo

Use quando a correção toca regra de negócio, contrato, persistência, permissão, segurança, sincronização, concorrência, integração externa ou muitos arquivos.

Validação esperada:

- reproduzir ou comprovar a falha;
- criar ou ajustar teste automatizado;
- rodar testes focados durante a correção;
- rodar `./check.sh`;
- retestar fluxo completo afetado;
- validar regressões próximas;
- atualizar `BUGS.md`, `TELAS.md`, `TESTES.md` ou `RUNBOOK.md` quando aplicável.

## Validação para melhoria simples fora de pacote

Use quando a mudança não faz parte de pacote e tem baixo risco.

### Sem código runtime

Exemplos:

- atualizar `DEMANDAS.md`;
- ajustar documentação;
- corrigir mapa mental;
- ajustar README;
- reorganizar texto de governança.

Validação esperada:

- revisar o diff;
- rodar validação documental ou `./check.sh` quando o check for rápido;
- não exigir teste automatizado, E2E ou validação manual de produto.

### Com código runtime local

Exemplos:

- trocar label;
- ajustar copy;
- corrigir ícone;
- pequeno espaçamento;
- mensagem de erro local;
- ajuste visual isolado.

Validação esperada:

- rodar teste local/focado quando existir;
- abrir a tela afetada quando viável;
- capturar evidência visual se a mudança for visível e houver ambiente disponível;
- não exigir suíte completa, salvo se a mudança afetar fluxo crítico.

### Com risco médio ou alto

Se a melhoria simples crescer e tocar regra, contrato, permissão, persistência, integração ou múltiplos módulos, ela deixa de ser simples.

Nesse caso:

- criar pacote em `DEMANDAS.md` ou vincular a pacote existente;
- aplicar validação por lote e fechamento de pacote;
- executar check completo no fechamento.

## Critério mínimo por entrega

- Lote: validação rasa e focada no que mudou.
- Pacote 100%: validação completa e commit.
- Mudança só documental: revisão de diff e check documental quando aplicável.
- Melhoria simples com código: teste local/focado.
- Bug simples: reprodução/reteste focado.
- Bug complexo: teste focado durante a correção e validação completa do fluxo afetado.
- Regra de negócio: teste unitário ou service.
- API: teste de contrato no lote e suíte relevante no fechamento.
- UI: teste de componente/widget ou validação manual documentada.
- Bug: reteste pelo passo de reprodução.
- Banco: validação do SQL e rollback documentado.
- Fluxo crítico: validar log/auditoria/métrica mínima quando aplicável.

## Testes por stack

Cada projeto deve adaptar o comando ao ecossistema real.

- Go: unidade/service/API com `go test`; build quando fechar pacote.
- Python: unidade/service/API com test runner do projeto; type-check/lint quando configurado.
- PHP: lint/static analysis/testes conforme framework.
- Node/TypeScript: unit/component/API, type-check e build no fechamento.
- Flutter/Dart: controller/widget tests, `flutter analyze` e `flutter test`.
- Mobile nativo: unit/UI tests e build/smoke no fechamento.

## Sinal de teste insuficiente

- Mudança de regra sem teste unitário ou service.
- Mudança de contrato sem teste de contrato.
- Mudança de UI crítica sem teste de componente/widget ou validação manual.
- Bug complexo sem teste novo ou justificativa.
- Pacote fechado sem `./check.sh`.
- P0 sem teste de permissão, erro ou falha de dependência quando aplicável.
- Pacote fechado sem review de escopo, regressão, arquitetura, docs e diff final.
- E2E ou contrato dependente de dado real instável.
- Fixture grande, opaca ou derivada de produção.
- Fluxo crítico sem forma de diagnóstico em produção.
- Log/auditoria persistido sem retenção ou cleanup.

## Adaptacao deste projeto

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
- Pacote/lote: lotes usam testes rasos e direcionados; fechamento de pacote exige `./check.sh` completo ou justificativa documentada.

### PKG-65 - Kubernetes

- Unitario focado: `go test ./internal/platform/collectors/kubernetes ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`.
- Cobertura local: normalizacao de pods/containers, sanitizacao de labels/annotations, autodiscovery por annotations, logs de pod e Metrics API opcional.
- Validacao documental: `deploy/kubernetes/aiceberg-agent.yaml`, Helm chart, RBAC minimo e rollback documentados.
- Evidencia focada: `PKG65_EVIDENCE_DIR=/tmp/aiceberg_pkg65_kubernetes_payload_20260619T185052Z go test ./internal/platform/collectors/kubernetes -run TestPKG65KubernetesPayloadAutodiscoveryMetricsEvidence -count=1 -v`.
- Bundle versionado: `docs/evidence/pkg65/kubernetes-payload-autodiscovery-metrics-20260619T185052Z`.
- Cobertura da evidencia: node, pod, container, event, log com redaction, annotations de autodiscovery, Metrics API normalizada e ausencia de volume sensivel no payload.
- Validacao real PKG-69 executada em cluster kind controlado: DaemonSet rodando, events/logs coletados, Helm install/upgrade/rollback e RBAC minimo sem `secrets`, `pods/exec` ou `delete pods`.

### PKG-66 - Checks locais

- Unitario focado: `go test ./internal/platform/collectors/localchecks ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`.
- Cobertura local: HTTP, TCP, OpenMetrics, tipo nao permitido, timeout por contexto e redaction de credenciais.
- Evidencia focada: `PKG66_EVIDENCE_DIR=/tmp/aiceberg_pkg66_localchecks_lifecycle_20260619T185846Z go test ./internal/platform/collectors/localchecks -run TestPKG66LocalChecksLifecycleRollbackUpgradeEvidence -count=1 -v`.
- Bundle versionado: `docs/evidence/pkg66/localchecks-lifecycle-rollback-upgrade-20260619T185846Z`.
- Cobertura da evidencia: criacao, execucao, falha, bloqueio de tipo, rollback por config, upgrade de manifest e preservacao de config sem vazamento de credencial.
- Apps reais, bancos reais, JMX real e IIS/WMI real ficam para PKG-71.

### PKG-67 - Fleet, rollout e flare

- Unitario focado: `go test ./internal/domain/channel ./internal/domain/usecase ./internal/bootstrap`.
- Cobertura local: comando `collect_support_flare` na allowlist, redaction recursiva e `fleet_runtime` no snapshot.
- Validacao real pendente para PKG-69: rollout canario com agentes em versoes diferentes, rollback por artefato anterior e bundle de suporte em host real.

### PKG-68 - Seguranca e assinatura

- Unitario focado: `go test ./internal/common/config ./internal/common/httpx ./internal/domain/usecase ./internal/bootstrap`.
- Cobertura local: HMAC de config, bloqueio de payload sensivel sem assinatura, expiracao, downgrade sem force, cadeia Ed25519 de artefato de update, token rotation e TLS inseguro contra producao.
- Validacao real pendente para PKG-69: proxy autenticado, TLS invalido, publicacao assinada, FIPS/pinning e rotacao com agentes reais.

### PKG-69 - Matriz operacional

- Local reproduzivel: `scripts/pkg69_operational_homologation.sh`.
- Cobertura local: readiness de ferramentas, testes focados dos pacotes PKG-59 a PKG-68, cadeia Ed25519 de artefato de update, reconexao pos-update com `version_confirmed` e mismatch apos rollback, `HTTP_PROXY` autenticado, rejeicao padrao de TLS invalido, timeout de download sem artefato finalizado, classificacao de clock skew, degradacao PCAP/tcpdump por permissao, replay de outbox apos restart local, burst de cardinalidade custom metrics com drops contabilizados, testes dedicados para API indisponivel, backoff/rede intermitente, payload grande e outbox cheia, topologia relay -> hub -> AIceberg para canal/ping/self-heal/update, e2e multi-processo direct/hub/relay, smoke POSIX com RSS/CPU/goroutines locais e `./check.sh`.
- Gate de fechamento: `scripts/pkg69_operational_evidence_gate.sh` gera/valida manifest TSV de evidencias reais com SHA256/tamanho do template e do anexo bruto; `scripts/pkg69_operational_evidence_gate_selftest.sh` roda em `./check.sh`, incluindo rejeicao de `Data UTC` fora de `YYYY-MM-DDTHH:MM:SSZ`, topologia placeholder, evidencia sem anexo bruto local existente ou com anexo vazio, rollback nao validado, `relay_direct_api_attempts > 0`, Relay com campo textual de conexao direta marcado como nao seguro, Agentless sem comprovacao via Hub, `direct_host_id`/`hub_host_id`/`relay_host_id` repetidos, `relay_upstream_host_id` diferente do Hub, CPU/RSS acima dos limites iniciais, status invalido de clock/update, metricas numericas obrigatorias, RBAC Kubernetes com `secrets/exec/delete` permitido e marcadores self-test/sinteticos/fake/mock/placeholder em modo `PKG69_REQUIRE_REAL_EVIDENCE=true`, `PKG69_REQUIRE_CLOSURE_ACCEPTED=true` ou `PKG69_REJECT_SYNTHETIC_EVIDENCE=true`. `scripts/pkg69_bundle_evidence_selftest.sh` tambem roda em `./check.sh` e valida o helper `scripts/pkg69_bundle_evidence.sh`, incluindo bloqueio de cenario desconhecido e `PROVENANCE.tsv`. `scripts/pkg69_collect_host_evidence_selftest.sh` roda em `./check.sh` e valida o coletor read-only `scripts/pkg69_collect_host_evidence.sh` com proveniencia, ambiente redigido e `COLLECTION_SUMMARY.tsv`. `scripts/pkg69_run_evidence_gate_from_bundles_selftest.sh` roda em `./check.sh` e valida integridade SHA256/tamanho dos bundles, manifest com uma unica evidencia, `PROVENANCE.tsv` obrigatorio e coerente, `COLLECTION_SUMMARY.tsv` e `COMMANDS.tsv` obrigatorios para bundle `raw-host`, cenario do resumo coerente com o manifest, contadores numericos, coerentes e iguais aos comandos registrados, `template` coerente com `evidence.md`, `created_at_utc` em `YYYYMMDDTHHMMSSZ`, artefato do manifest coerente com `Evidencia bruta anexada`, cenario desconhecido e mapeamento de bundles reais para o gate oficial sem aceitar fechamento incompleto. `scripts/pkg69_evidence_gap_report_selftest.sh` roda em `./check.sh` e valida o relatorio Markdown e stdout estruturado de pendencias, invalidos, rejeicao de sintetico em modo aceite, caminho `PRONTO_PARA_REVISAO` com 14/14 cenarios de lab controlado, `closure_acceptance` e modo bloqueante `PKG69_GAP_REPORT_REQUIRE_ACCEPTED=true` sem `PKG69_ACCEPT_CLOSURE=true`, alem de `PKG69_GAP_REPORT_REQUIRE_COMPLETE=true`.
- Fechamento real exige executar matriz em Windows Server, Windows desktop, Linux Debian/RHEL, Docker e Kubernetes controlado.
- Validacao real fechada em 2026-06-19: o gate `scripts/pkg69_evidence_gap_report.sh docs/evidence/pkg69` mapeia 14/14 cenarios reais, incluindo Windows Server, Windows desktop, Linux Debian/RHEL, Docker, Kubernetes, API indisponivel/recuperacao, rede/backoff, proxy/TLS, disco cheio/outbox preservada, payload/alto volume, clock skew, permissao eBPF/PCAP restrita, update remoto/rollback, Relay/Hub/Direct e reboot real durante coleta. Com `PKG69_GAP_REPORT_REQUIRE_ACCEPTED=true PKG69_ACCEPT_CLOSURE=true`, o gate retorna `closure_status=ACEITO_PARA_FECHAMENTO` e `closure_acceptance=accepted`.

### PKG-73 - Taxonomia SOC de logs

- Unitario focado: `go test ./internal/common/soclog ./internal/platform/collectors/oslogs ./internal/platform/collectors/containers ./internal/platform/collectors/kubernetes ./internal/platform/collectors/otlp`.
- Compilacao Windows do parser EventLog: `GOOS=windows GOARCH=amd64 go test -c ./internal/platform/collectors/oslogs -o /tmp/aiceberg-oslogs-windows.test.exe`.
- Cobrir Windows Security, Sysmon, DistributedCOM 10028, Linux auth, app JSON, Graylog/GELF, journald, OTLP log, Docker log e Kubernetes pod log.
- Validar `aiceberg_transport`, `aiceberg_tool_origin`, `aiceberg_source_category`, `aiceberg_soc_source_type`, `aiceberg_soc_eligible`, `aiceberg_origin_confidence`, `aiceberg_route_reason`, redaction e campos SOC promovidos.
- Fechamento coordenado com web: rodar `./check.sh` neste repo e no `aiceberg_web`.

### PKG-74 - Descoberta automática de fontes locais

- Unitario focado previsto: `go test ./internal/platform/collectors/logdiscovery ./internal/platform/collectors/oslogs ./internal/platform/collectors/containers ./internal/platform/collectors/kubernetes ./internal/platform/collectors/otlp ./internal/domain/usecase ./internal/bootstrap`.
- Contrato: validar `log_source_discovery_v1` com payload completo, payload parcial, agente legado, fingerprint estável, deduplicação e serialização em snapshot/bootstrap.
- Linux: cobrir Nginx, Apache/httpd, Plesk, journald, auth.log/secure, syslog, app log, PostgreSQL, MySQL/MariaDB, Redis, RabbitMQ/MongoDB quando fixture existir, processo/listener e permissão negada.
- Windows: cobrir EventLog Security/System/Application, Sysmon quando fixture existir, IIS/W3SVC, SQL Server, serviços Windows, app log, permissão negada e compilação cruzada.
- Containers/Kubernetes: cobrir Docker/containerd, labels/annotations, path de log, pod/container/namespace, socket/API indisponível e RBAC insuficiente.
- Segurança: validar redaction de cmdline, env, URL, token, cookie, segredo, Authorization e payload sensível em candidato, snapshot, log local, flare e evidência.
- Volume: validar `min_severity=error`, descarte local de evento sem nível quando severidade mínima exigir nível conhecido, limites de bytes/eventos, backpressure, sampling e retenção/limpeza local quando aplicável.
- Remoto: validar aplicação de configuração aprovada assinada/escopada, rejeição/ignorar fonte, rollback, API indisponível, perda de rede, outbox cheia/disco cheio, payload grande e compatibilidade com coletas atuais.
- Fechamento coordenado com web: rodar `./check.sh` neste repo e no `aiceberg_web`, com evidência controlada em Linux, Windows, container e Kubernetes antes de marcar 100%.

Evidência 2026-06-20:

- `go test ./internal/platform/collectors/logdiscovery` validou contrato, disable remoto, dedupe e redaction.
- `go test ./internal/bootstrap ./internal/common/config ./internal/data/local/prefs` validou scheduler, config e `collect_now=log_source_discovery`.
- `go test ./internal/platform/collectors/logdiscovery ./internal/bootstrap` passou após inclusão de systemd/journald, paths ampliados, Kubernetes básico e `useful_for`.
- `go test ./internal/platform/collectors/logdiscovery ./internal/bootstrap ./internal/common/config` validou também sinais controlados de Docker, containerd, Kubernetes token e OTLP sem depender de socket real externo.
- `./check.sh` passou no repo do agente.
- Artefatos `0.8.13` foram gerados por `./scripts/build_installers.sh`, copiados para `aiceberg_web/cliente/web/downloads/agent/0.8.13/` e publicados em produção; SHA256 HTTP validado para Linux amd64 e Windows amd64.
- Pendência de fechamento: validação real de update/aplicação em Linux/Windows.

## Politica minima de testes

- Unitário: validar regra de negocio, casos de erro e limites sem depender de rede, banco real ou UI.
- Integracao: validar contratos entre camadas, banco local/controlado, API e adapters.
- E2E: validar fluxos criticos do usuario quando houver interface ou jornada operacional relevante.
- Cobertura mínima por criticidade: modulo critico deve ter meta documentada no projeto; modulo simples pode exigir apenas evidencia direcionada.
- Dados de teste e fixtures: usar dados pequenos, deterministas, sem dump real de producao e com IDs previsiveis.
- Pacote/lote: lotes usam testes rasos e direcionados; fechamento do pacote exige check completo.
- Bug simples ou melhoria sem impacto de codigo pode usar teste local/direcionado. Bug complexo ou mudanca ampla exige check completo.
