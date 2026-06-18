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
- Validacao real pendente para PKG-69: Windows, Linux, container, perda de rede, API indisponivel, disco cheio, proxy e agente instalado anteriormente.

## PKG-60 - Pipeline seguro de logs

- Unitario focado: `go test ./internal/platform/collectors/oslogs ./internal/common/config ./internal/domain/usecase`.
- Build Windows do coletor: `GOOS=windows GOARCH=amd64 go test ./internal/platform/collectors/oslogs`.
- Cobrir redaction de token/senha/Authorization, atributos JSON sanitizados, multiline, cursor por arquivo/canal/journald, filtros include/exclude/min severity/unit/prioridade/provider/event_id, processors parse/remap/drop/mask/route/sample/enrich, dual-shipping restrito a `/v1/logs/*` e `dropped_count` sem conteudo descartado.
- Validacao real Windows/Linux/container/proxy/disco cheio fica pendente para PKG-69.

## PKG-61 - Metricas custom locais

- Unitario focado: `go test ./internal/platform/collectors/custommetrics ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`.
- Cobrir parser DogStatsD-like, HTTP local, tipos count/gauge/histogram/distribution/set/service_check, tags canonicas, limite de cardinalidade, `accepted_count` e `dropped_count`.
- Validacao real de alto volume, UDS, container e app externa fica pendente para PKG-69.

## PKG-62 - OTLP HTTP/JSON

- Unitario focado: `go test ./internal/platform/collectors/otlp ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`.
- Cobrir HTTP local loopback, metrics/logs/traces JSON, resource attributes essenciais, limite de atributos/cardinalidade, redaction de atributos sensiveis, `trace_id`/`span_id`, limite de itens e `dropped_count`.
- PKG-63: revisar `docs/pkg63_apm_sampling.md` para instrumentacao por linguagem com SDKs oficiais OpenTelemetry, sem SDK proprio AIceberg.
- Validacao real com exporter OpenTelemetry, gRPC/protobuf e consumo de CPU/memoria fica pendente para PKG-69.

## PKG-64 - Containers Docker inicial

- Unitario focado: `go test ./internal/platform/collectors/containers ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`.
- Cobrir normalizacao Docker, labels sensiveis mascaradas, compose service, CPU, memoria, rede e IO.
- Validacao real Docker/containerd/logs/autodiscovery fica pendente para PKG-69.

## PKG-70 - Rede avancada, USM e workload security

- Unitario focado: `go test ./internal/platform/collectors/networkcapture`.
- Cobrir servico inferido sem OpenTelemetry, `service/env/version` por metadado explicito ou cmdline, dependencia `service -> database`, fallback eBPF, NPM/top talkers, redaction de IP publico e sinais SOC sem acao destrutiva.
- Validacao real de kernel eBPF, permissao sem eBPF, fluxo `web -> api -> db`, container, Kubernetes e overhead fica pendente ate ambiente controlado.

## PKG-71 - Integracoes avancadas

- Unitario focado: `go test ./internal/platform/collectors/localchecks`.
- Cobrir OpenMetrics com allowlist/cardinalidade, labels permitidas, JMX via Jolokia, metadados de manifest oficial e bloqueio de tipo arbitrario.
- Validacao real de `/metrics`, app Java, Windows Server, bancos/fila/web servers e autodiscovery container/Kubernetes fica pendente ate ambiente controlado.

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
- Validacao real de incidente NOC/SOC, replay 24h offline, agent+agentless, cliente regulado e benchmark contra Datadog fica pendente ate ambiente controlado.

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
- Cobertura local: normalizacao de pods/containers, sanitizacao de labels/annotations e autodiscovery por annotations.
- Validacao documental: `deploy/kubernetes/aiceberg-agent.yaml`, Helm chart, RBAC minimo e rollback documentados.
- Validacao real pendente para PKG-69: cluster controlado, DaemonSet rodando em nodes, eventos reais, upgrade/rollback Helm, logs de pod e uso real via Metrics API/kubelet.

### PKG-66 - Checks locais

- Unitario focado: `go test ./internal/platform/collectors/localchecks ./internal/common/config ./internal/domain/usecase ./internal/bootstrap`.
- Cobertura local: HTTP, TCP, OpenMetrics, tipo nao permitido, timeout por contexto e redaction de credenciais.
- Validacao real pendente para PKG-69/PKG-71: apps reais, bancos reais, JMX real, IIS/WMI real e ponte automatica com autodiscovery Kubernetes.

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
- Gate de fechamento: `scripts/pkg69_operational_evidence_gate.sh` gera/valida manifest TSV de evidencias reais com SHA256/tamanho do template e do anexo bruto; `scripts/pkg69_operational_evidence_gate_selftest.sh` roda em `./check.sh`, incluindo rejeicao de `Data UTC` fora de `YYYY-MM-DDTHH:MM:SSZ`, topologia placeholder, evidencia sem anexo bruto local existente ou com anexo vazio, rollback nao validado, `relay_direct_api_attempts > 0`, CPU/RSS acima dos limites iniciais e RBAC Kubernetes com `secrets/exec/delete` permitido. `scripts/pkg69_bundle_evidence_selftest.sh` tambem roda em `./check.sh` e valida o helper `scripts/pkg69_bundle_evidence.sh`. `scripts/pkg69_collect_host_evidence_selftest.sh` roda em `./check.sh` e valida o coletor read-only `scripts/pkg69_collect_host_evidence.sh`. `scripts/pkg69_run_evidence_gate_from_bundles_selftest.sh` roda em `./check.sh` e valida integridade SHA256/tamanho dos bundles, cenario desconhecido e mapeamento de bundles reais para o gate oficial sem aceitar fechamento incompleto. `scripts/pkg69_evidence_gap_report_selftest.sh` roda em `./check.sh` e valida o relatorio Markdown de pendencias/invalidos a partir do manifest.
- Fechamento real exige executar matriz em Windows Server, Windows desktop, Linux Debian/RHEL, Docker e Kubernetes controlado.

## Politica minima de testes

- Unitário: validar regra de negocio, casos de erro e limites sem depender de rede, banco real ou UI.
- Integracao: validar contratos entre camadas, banco local/controlado, API e adapters.
- E2E: validar fluxos criticos do usuario quando houver interface ou jornada operacional relevante.
- Cobertura mínima por criticidade: modulo critico deve ter meta documentada no projeto; modulo simples pode exigir apenas evidencia direcionada.
- Dados de teste e fixtures: usar dados pequenos, deterministas, sem dump real de producao e com IDs previsiveis.
- Pacote/lote: lotes usam testes rasos e direcionados; fechamento do pacote exige check completo.
- Bug simples ou melhoria sem impacto de codigo pode usar teste local/direcionado. Bug complexo ou mudanca ampla exige check completo.
