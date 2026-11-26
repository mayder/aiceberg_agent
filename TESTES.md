## Diagnóstico de configuração — PKG-134 (08/11/2026)

`go test -race ./internal/domain/configfailure ./internal/data/local/configstate ./internal/data/local/safefile ./internal/data/local/prefs ./internal/domain/usecase ./internal/interfaces/hub ./internal/interfaces/health ./internal/common/config ./internal/bootstrap` passou.
Casos novos: causas tipadas sem inferência por texto, fingerprint estável/escopado,
1000ciclos iguais, summary30min, recuperação, ACK/reboot, clock skew, limites/
contadores saturados, armazenamento inválido/oversized/symlink preservado, conflito
real de caminho mantendo prefs boas, concorrência, versão nova e ingest próprio.
HUB mantém corpo e identidade. Contrato compila para Windows/Linux; isso não é
execução nativa nesses SO. Regressões de assinatura/política do pacote usecase e
HUB/bootstrap passaram no gate amplo. Persistência usa diretórios ancorados e leitura limitada sem seguir links; teste
de troca concorrente de ancestral e preservação de arquivo/memória passaram.
Consolidação e limites em docs/evidence/pkg134-local-20261108.md. Não converter
esse gate em homologação/publicação.

## JSON opaco e worker Relay — 04/11/2026

Reteste: go test -race ./internal/bootstrap ./internal/interfaces/hub ./internal/common/envelopejson ./internal/data/local/outbox ./internal/common/config ./internal/domain/usecase -run 'TestAsyncRelay|TestRelayCatchup|TestDecodeBatch|TestHubRaw|TestBolt|TestAsyncOutbox|TestLoadRelay|TestFlushOutbox|TestHubOutbox|TestAsyncHub|TestOwnIngest|TestHubWorker|TestRawBoltBodies' -count=1. Casos: Relay→HUB→API HTTP/Bolt reais antes de liberar coletor bloqueado, timer1h,503/nenhumACK/reabertura, configuração própria serializada, captura de ingresso que sobrescreve valor forjado, RawMessage sem alias de memória, rollback/construtor legado, split atômico com requests≤400B e identidade/rota. Filhos persistidos por ID: validar conjunto de eventos sem inventar ordenação global. Benchmarks Mac/Linux em docs/evidence/hub-cpu-relay-arrival-local-20261104.md; check.sh obrigatório no hook de consolidação. Sem dados/replay produtivos.

## Freelist reconstruível e normalização por rota — 04/11/2026

Reteste com race: go test -race ./internal/data/local/outbox ./internal/domain/usecase ./internal/bootstrap ./internal/common/config -run 'TestBolt|TestMetricsBypass|TestRawLogsStill|TestFlushOutbox|TestHub|TestLoadHub|TestAsyncHub' -count=1. Cobre commit bloqueado, falha/capacidade concorrente, kill/reabertura por construtor anterior, upgrade/rollback, fsync ativo, opt-outs por modo e independentes, tamanho/codificação de request e logs oversized. Benchmarks macOS/Linux e limites em docs/evidence/hub-freelist-and-normalization-local-20261104.md. Não apaga dados produtivos; não equivale a falha física de energia/disco. ./check.sh no fechamento do lote e assinatura dos cinco pacotes são gates locais; publicação/piloto exigem autorização própria.

# TESTES.md

## Agrupamento durável da outbox do HUB — 04/11/2026

Reteste: go test -race ./internal/data/local/outbox ./internal/bootstrap ./internal/common/config -run 'TestBoltStore|TestBoltBatch|TestHubOutbox|TestLoadHubOutbox|TestAsyncHub|TestBuildSelfHealRuntimeSnapshot' -count=1. Casos: commit bloqueado sem ACK antecipado, capacidade/duplicata, rollback interno de callback vizinho, reabertura legada após kill e HTTP/64 relays com API bloqueada. Benchmark controlado e limites em docs/evidence/hub-durable-write-batching-local-20261104.md. Sem dados produtivos; piloto e consumo real são gates separados.


Fonte de verdade para testes automatizados, validação manual e reteste de bugs.

## Confirmação após configuração recebida no ingest — 04/11/2026

`go test -race ./internal/bootstrap ./internal/domain/usecase -run 'TestOwnIngestConfigIsConfirmed|TestIngestConfirmation|TestConfigSync|TestHubWorkerQueues|TestAsyncHub' -count=1`.

Reprodução: v40→v41 pelo callback real; GET condicional204 não gera report.
Correção: consulta completa/ACK autenticado v41 e retorno ao204; duplicatas,
relay, versão vazia/falha de persistência preservam o limite. Report503 solicita
nova tentativa na cadência normal. Fixtures HTTP/prefs em arquivo, sem dados produtivos.
`./check.sh` é o gate do commit/publicação. Instalação real é gate separado.


## Envio por chegada do HUB — 04/11/2026

`go test -race ./internal/bootstrap ./internal/interfaces/hub ./internal/data/repositories ./internal/common/config ./internal/domain/usecase -run 'TestAsyncHub|TestHubOutboxWorker|TestHubWorkerQueues|TestOutboxNotifies|TestLoadHubOutbox|TestServeHubStops|TestHubDoesNotAcknowledge|TestRealRelayTransport|TestFlushOutbox|TestBuildSelfHealRuntimeSnapshotSanitizes' -count=1`.

- Append só notifica após gravação; falha não notifica. Mil sinais ficam em 1 slot,
  sem sobrepor ciclos; fallback periódico e cancelamento durante guard funcionam.
- Timer de1hora no teste: Relay realHTTP é aceito após Bolt enquanto API bloqueada;
  chegada acorda worker,ACK remoto remove só o recebido e shutdown preserva outros
  dois envelopes após reabrir arquivo. Não depende de dados/credenciais reais.
- Rajada de 64 Relays HTTP simultâneos: todos aceitos com API bloqueada; depois todos
  entregues uma vez,concorrência≤8,pruning 1 vez e fronteiras de auth/rota preservadas.
- Own config não é aplicada no worker; relay config preserva rota e fila própria
  cancela sem deadlock. Opt-out local e inspector têm configuração explícita.
- Mutante que suprime a notificação faz o teste Relay→HUB→API falhar por ausência
  do envio; fonte restaurada e retestada. HTTP503 na persistência conserva relay.
- `./check.sh`,cross-build e piloto produtivo continuam gates separados. Não
  considerar esse teste de loopback como SLA/capacidade da frota ou publicação.

## Retomada do flush do HUB — BUG-20261103-01

- `go test ./internal/domain/usecase ./internal/bootstrap ./internal/common/schedule ./internal/common/config -run 'TestFlushOutbox|TestOutboxCatchup|TestOutboxRuntimeEvidence|TestPeriodicTimer|TestLoadHubOutboxCatchup|TestHubOutboxCapacity' -count=1`
  cobre ACK seguro/parcial, despacho cancelado, retomada sem duplicação,
  autenticação/identidade/rota, HTTP401/cooldown, timer e flag de rollback.
- Repetir os pacotes/casos com `-race` para o dispatcher e snapshots concorrentes.
- `go test -race ./internal/bootstrap -run TestOutboxCatchupDrainsPersistentBacklogWithRealHTTPAndRetains401 -count=1 -v`:
  Bolt e HTTP reais em loopback,900frames/615identidades sintéticas. Entrega
  uma vez cada, preserva401/cooldown e envelope pendente após reabrir arquivo.
  Esse teste tem filesystem/rede local; não é teste de capacidade produtiva.
- Regressões causais controladas: desativar a retomada na policy ou zerar
  LastDeferred faz os testes correspondentes falharem; fontes restauradas e
  retestadas. Timer/pruning usam datas controladas; timers reais só na API de timer.
- Gate final `./check.sh`; publicação e piloto comAgentless são gates posteriores.

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

## Reteste de lote de ingestão por bytes

- BUG-20260807-01: `FlushOutbox` deve limitar o JSON serializado de cada requisição a 8 MiB, abaixo dos 10 MB da API.
- Validar múltiplos sublotes, ACK granular e preservação de endpoint, autorização e identidade.
- BUG-20260807-03: envelope individual excessivo de `/v1/logs/raw` deve ser substituído atomicamente por partes com `body.events`, preservando ordem e metadados.
- Corpo sem `events` ou evento individual acima do limite deve permanecer na outbox sem chegar ao transporte e sem ser confirmado.
- A outbox Bolt deve preservar o original quando a substituição falhar e persistir apenas as partes após sucesso/restart.
- Comando focado: `go test ./internal/domain/usecase ./internal/data/local/outbox -run 'TestFlushOutbox_(SplitsRequestBySerializedSize|RetainsSingleEnvelopeAboveRequestLimit|SplitsAndFlushesOversizedRawLogEnvelope|RetainsBatchWhenIngestDidNotPersist|AcksDuplicateEnvelopeSkip)|TestBoltStoreReplaceIsAtomicAndDurable' -count=1 -v`.
- Piloto: medir fila antes/depois, tamanho das requisições, último ACK, erros HTTP e ausência de perda.
- Evidência de publicação em 08/08/2026: `./check.sh` verde; artefatos `0.8.46` com hashes `linux-amd64=1b6fdc13ff400061b223bac6171d022725491587e2f014329227be4c1ccb01ef` e `windows-amd64=c24b9fce65858e08987a338232fc723763df05027e593479ce5b6ee3d6a78964`; prontidão oficial e Range passaram nos dois domínios de download.

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

## PKG-82 - Perfil local de performance

- Unitario focado: `go test ./internal/platform/collectors/sysmetrics`.
- Contrato: validar `performance_profile` aditivo em `sysmetrics`, com `resources`, `processes`, `checks` e `gaps`.
- Segurança: validar redaction de command line para token, senha, secret, authorization, api key e bearer.
- Validacao produtiva complementar em 24/06/2026: agentes InspectApp `1`, `4`, `18`, `19`, `70` e `71` enviaram `performance_profile` recente em `/v1/ingest/metrics`, com `resources=yes`, 10 processos e 5 checks por host.
- Hotfix Windows CPU `0.8.38`: `go test ./internal/platform/collectors/sysmetrics` e `./check.sh` passaram em 01/07/2026; a coleta deixa de usar amostra `0s` e descarta padrão Windows binário `0/100`.
- Fechamento coordenado com web: rodar `./check.sh` neste repo e no `aiceberg_web`.
- Fechamento: rodar `./check.sh`.
- Validacao real fechada pelo PKG-69 aceito em 2026-06-19: Windows, Linux, container, perda de rede, API indisponivel, disco cheio/outbox preservada, proxy/TLS e agente instalado anteriormente.

## PKG-83 - Cobertura e integridade de logs locais

- Unitario focado: `go test ./internal/platform/collectors/oslogs`.
- Build Windows do coletor em host nao Windows: `GOOS=windows GOARCH=amd64 go test -c -o /tmp/aiceberg_oslogs_windows.test.exe ./internal/platform/collectors/oslogs`.
- Cobrir health por fonte, `no_new_events`, `channel_missing`, `dropped_by_severity`, sinais SSH, mail/SASL, sudo/PAM, nginx/apache, Windows Security 4625 e Sysmon.
- Cobrir POSIX com tail de arquivo, truncate/rotação, permissão negada, arquivo inexistente e cursor desalinhado sem reter conteúdo descartado.
- Validacao produtiva complementar em 24/06/2026: agentes InspectApp `1`, `4`, `18`, `19`, `70` e `71` enviaram `log_source_health` recente em `/v1/logs/raw`, cobrindo os estados `delivering`, `dropped_by_severity`, `file_missing`, `permission_denied`, `channel_missing` e `no_new_events`.
- Fechamento coordenado com web: rodar `./check.sh` neste repo e no `aiceberg_web`.

## PKG-84 - Update resiliente e diagnosticavel

- Unitario focado: `go test ./internal/domain/usecase -run 'TestSelfUpdate_ReportIncludesDownloadMetadata|TestSelfUpdate_PreflightFailsWhenStagingIsNotWritable|TestSelfUpdate_PersistsCooldownAcrossInstances|TestSelfUpdate_ReportPendingResult' -count=1 -v`.
- Download resiliente: `go test ./internal/domain/usecase -run 'TestSelfUpdate_DownloadResumesPartialFileWithRange|TestSelfUpdate_DownloadRestartsWhenPartialRangeIsNotSatisfiable|TestSelfUpdate_DownloadTimeoutDoesNotFinalizePartialFile|TestSelfUpdate_ReportIncludesDownloadMetadata' -count=1 -v`.
- Preflight: validar executavel atual, staging gravavel, espaco livre do staging, gerenciador de servico, shell/PowerShell, workdir e lacunas de lock/EDR/SELinux-AppArmor sem executar acao destrutiva.
- Rollback/staging: `go test ./internal/domain/usecase -run 'TestSelfUpdate_(SnapshotIncludesPendingStateMetadata|ReportPendingResultMarksVersionMismatchAfterRollback|CleanupOldUpdateStagingKeepsCurrentAndMetadata|ReportIncludesDownloadMetadata|PreflightFailsWhenStagingIsNotWritable|PreflightFailsWhenStagingHasLowSpace|PreflightFailsWhenInstallDirIsNotWritable|ApplyFailureReportsExitCodeCooldownAndClearsPending|PersistsCooldownAcrossInstances|DownloadResumesPartialFileWithRange|DownloadRestartsWhenPartialRangeIsNotSatisfiable|DownloadTimeoutDoesNotFinalizePartialFile|ChecksumMismatch|RejectsInvalidTrustedArtifactSignature)' -count=1 -v` passou em 24/06/2026 e cobre hash inválido, assinatura inválida, timeout sem finalização, retomada por `Range`, limpeza de `.part` inválido após `416`, metadados de download, preflight sem staging gravável, staging sem espaço em simulação controlada, diretório de instalação sem escrita, falha de apply com exit code/cooldown/limpeza de pendência, cooldown persistido, `rollback_available`/`rollback_version` em estado/report e limpeza de staging antigo preservando estado.
- Validacao produtiva complementar em 24/06/2026: agentes InspectApp `1`, `4`, `18`, `19`, `70` e `71` confirmaram `0.8.31`, `version_confirmed`, sem `pending_update_payload` e com `send_configuration=0`.
- Falha produtiva/controlada em 24/06/2026: agente Linux `1` revelou `.part` inválido seguido de `416 Requested Range Not Satisfiable`; a versão `0.8.31` removeu o parcial e baixou limpo. Agente Windows `71` recebeu probe `0.8.32-probe` com SHA inválido, reportou `self_update_apply_failed`/`sha256 mismatch`, limpou a pendência no backend e permaneceu executando `0.8.31`.
- Hotfix ACK de ingestão `0.8.39`: validar `go test ./internal/domain/usecase -run 'TestFlushOutbox_(RetainsBatchWhenIngestDidNotPersist|AcksDuplicateEnvelopeSkip)' -count=1 -v`; o flush não deve remover da outbox quando o backend responder HTTP 200 com `received=0`, `skipped=1` e motivo não idempotente, como `persist_failed`.
- Hotfix destino Linux `0.8.40`: validar `bash -n scripts/linux/aiceberg-agent-update-launcher.sh scripts/linux/aiceberg-agent-apply-update.sh` e `go test ./internal/domain/usecase -run 'TestSelfUpdate_(RunCommand|ReportPendingResultMarksVersionMismatchAfterRollback|ReportPendingResultConfirmsVersionAfterReconnect|ApplyFailureReportsExitCodeCooldownAndClearsPending|SnapshotIncludesPendingStateMetadata)' -count=1 -v`; o update deve instalar no mesmo binário que o serviço está executando.
- Hotfix identidade/outbox/update-report `0.8.42`: validar `go test ./internal/domain/usecase -run 'Test(CollectAndBuffer|CachedIdentityHeaderProvider|FlushOutbox|SelfUpdate_ReportIncludesDownloadMetadata)' -count=1 -v`; coletores devem renovar `X-Agent-Identity`, replay local da outbox deve trocar credencial expirada antes do envio, envelopes `meta.via=hub` devem preservar a identidade original, e `update-report` deve autenticar com identidade atual.
- Healthcheck Windows `0.8.32`: script `scripts/windows/aiceberg-agent-apply-update.ps1` passou a aceitar `-HealthUrl`/`AICEBERG_AGENT_HEALTH_URL`, exigir HTTP 2xx quando informado e restaurar backup se o serviço subir sem ficar saudável. Validação local de sintaxe PowerShell depende de `pwsh`/Windows; neste host macOS sem `pwsh`, a cobertura automatizada fica em `./check.sh` e validação real Windows no rollout.
- Hardening Linux `0.8.33`: `go test ./internal/domain/usecase ./internal/platform/collectors/sysmetrics -run 'TestSelfUpdate_ClearsCurrentVersionPendingStateBeforeNewUpdate|TestSelfUpdate_(ReportPendingResultMarksVersionMismatchAfterRollback|ReportPendingResultConfirmsVersionAfterReconnect|ApplyFailureReportsExitCodeCooldownAndClearsPending|DownloadRestartsWhenPartialRangeIsNotSatisfiable)|TestBoundedContext|TestCollect_RespectsPrefs' -count=1 -v` passou em 24/06/2026 cobrindo limpeza de estado pendente obsoleto, timeout local dos coletores caros e regressões principais de self-update.
- Hot path Linux `0.8.34`: `go test ./internal/platform/collectors/sysmetrics -run 'TestLinuxSecurityUpdatesSkipsDnfByDefault|TestBoundedContext|TestCollect_RespectsPrefs' -count=1 -v` passou em 24/06/2026 cobrindo que `dnf updateinfo` fica desabilitado por padrão no ciclo de métricas.
- Snapshot operacional `0.8.35`: `go test ./internal/domain/usecase ./internal/platform/collectors/sysmetrics -run 'TestSelfUpdate_SnapshotClearsCurrentVersionPendingState|TestLinuxSecurityUpdatesSkipsDnfByDefault|TestBoundedContext|TestCollect_RespectsPrefs' -count=1 -v` passou em 24/06/2026 cobrindo limpeza de pendência local da versão atual no snapshot e limites dos coletores do hot path.
- Inventário Linux `0.8.36`: `go test ./internal/platform/collectors/sysmetrics -run 'TestListLinuxPackagesSkipsFullInventoryByDefault|TestLinuxSecurityUpdatesSkipsDnfByDefault|TestBoundedContext|TestCollect_RespectsPrefs' -count=1 -v` passou em 24/06/2026 cobrindo que `rpm/dpkg` ficam fora do hot path por padrão.
- Fechamento PKG-84 em 24/06/2026: `go test ./internal/domain/usecase -run 'TestSelfUpdate_(ReportIncludesDownloadMetadata|PreflightFailsWhenStagingIsNotWritable|PreflightFailsWhenStagingHasLowSpace|PreflightFailsWhenInstallDirIsNotWritable|ApplyFailureReportsExitCodeCooldownAndClearsPending|PersistsCooldownAcrossInstances|DownloadResumesPartialFileWithRange|DownloadRestartsWhenPartialRangeIsNotSatisfiable|DownloadTimeoutDoesNotFinalizePartialFile|ChecksumMismatch|RejectsInvalidTrustedArtifactSignature|ReportPendingResultMarksVersionMismatchAfterRollback|ReportPendingResultConfirmsVersionAfterReconnect|SnapshotIncludesPendingStateMetadata|CleanupOldUpdateStagingKeepsCurrentAndMetadata)' -count=1 -v` passou cobrindo preflight, staging, espaço, permissão, hash, assinatura, download lento/parcial, `416`, falha de apply, cooldown, confirmação de versão, rollback por mismatch/reconexão e retenção segura de staging. Produção confirmou agentes InspectApp `1`, `4`, `18`, `19`, `70`, `71` em `0.8.36`, `version_confirmed`, sem pendência.
- Fechamento coordenado com web: rodar `./check.sh`, gerar instaladores, publicar artefatos versionados e validar update real nos agentes InspectApp.
- Artefatos agente `0.8.38`: gerados no `aiceberg_agent` por `./scripts/build_installers.sh`, copiados para `aiceberg_web/cliente/web/downloads/agent/0.8.38/` e acompanhados de `SHA256SUMS` (`linux-amd64=b7960ed8acbd2be261e93df47049199c205ae9d72c7b395653cadec97bc2a94d`, `windows-amd64=6dd988870184f0c8c6224b7104b32cbb1fc754b6cbedfb16ca317f1c20ceb4f5`). O pacote corrige amostragem de CPU Windows para evitar alertas falsos de 100%.
- Artefatos agente `0.8.39`: gerados no `aiceberg_agent` por `./scripts/build_installers.sh`, copiados para `aiceberg_web/cliente/web/downloads/agent/0.8.39/` e acompanhados de `SHA256SUMS` (`linux-amd64=7eec664b26e24044c3942f4e70f5ca085211bdfd94e6c5102b2aa9c26eb4aba9`, `windows-amd64=80a89b01083202cd0b9ce6de5c9acaaf7f601b540cf9d3820fc7bc593b846711`). O pacote impede ACK local quando a ingestão HTTP 200 informa falha de persistência não idempotente.
- Artefatos agente `0.8.40`: gerados no `aiceberg_agent` por `./scripts/build_installers.sh`, copiados para `aiceberg_web/cliente/web/downloads/agent/0.8.40/` e acompanhados de `SHA256SUMS` (`linux-amd64=b3bdcaaae9b5c8ae0587af94ca9463a32c5fef354eef80418b43b04c22897d58`, `windows-amd64=da4090ed2552f58b721ea4541d316561b542ad68fe81b7b67226f74c49f80d03`). O pacote corrige o destino do self-update Linux para aplicar no mesmo binário executado pelo serviço.

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
- `go test ./internal/platform/collectors/logdiscovery ./internal/bootstrap ./internal/common/config` validou também sinais controlados de Docker, containerd, Kubernetes token, OTLP e lacuna `path_permission_denied` sem depender de socket real externo.
- Bundle controlado versionado: `docs/evidence/pkg74/discovery-controlled-20260620T180000Z`, gerado com `PKG74_EVIDENCE_DIR=/Users/brenomayder/projects/desktop/aiceberg_agent/docs/evidence/pkg74/discovery-controlled-20260620T180000Z go test ./internal/platform/collectors/logdiscovery -run TestPKG74ControlledDiscoveryEvidence -count=1 -v`. A evidência cobre aplicação lenta com web/app/banco/rede, Nginx, Apache, IIS controlado, Linux auth, Docker/containerd, Kubernetes token, OTLP, redaction, lacuna de permissão e política `bounded/read_only/error+`.
- `./check.sh` passou no repo do agente.
- Artefatos `0.8.13` foram gerados por `./scripts/build_installers.sh`, copiados para `aiceberg_web/cliente/web/downloads/agent/0.8.13/` e publicados em produção; SHA256 HTTP validado para Linux amd64 e Windows amd64.
- Produção: agentes online do cliente InspectApp `1`, `4`, `18`, `19`, `70` e `71` atualizaram para `0.8.13`; `collect_now=log_source_discovery` foi consumido; o web persistiu candidatos reais de Linux e Windows, incluindo Nginx, Apache, Plesk, Linux auth/syslog, journald, Docker/containerd, SQL Server, PostgreSQL, MySQL, Redis, OpenTelemetry, Windows EventLog e Defender.
- Pendência operacional externa: Windows desktop `72` estava offline. IIS e Kubernetes reais ficam como homologação por cliente quando esses componentes existirem no ambiente alvo; o pacote não declara causa raiz nem substitui validação produtiva específica.
- Correção 2026-06-20: `go test ./internal/domain/usecase -run 'Test(ConfigSyncBackoffSkipsTransientFailure|PingBackendBackoffSkipsTransientFailure|FlushOutbox_TransientTransportFailureDoesNotLogError|FlushOutbox_BackoffSkipsFailedRouteTemporarily)' -count=1 -v` validou que timeouts transitórios de `/v1/agent/config`, `/v1/agent/ping` e `/v1/ingest/*` entram em backoff sem log local `ERROR`; versão runtime atualizada para `0.8.14`.
- Artefatos `0.8.14`: `./scripts/build_installers.sh` gerou os compactados oficiais; `SHA256SUMS` validou `linux-amd64=9c73d73cba4e3a60bcdfa4e0da53cf4b42decaf3bf3548d44fedc1ff83c9ae78` e `windows-amd64=b490efc744c661b9f93cad700ffb014a9cf61b4695ebd619cc767aed74fc5713`.
- Correção 2026-06-20: `go test ./internal/data/remote -run 'TestAgentControlClient(FetchSelfHealCommandsDirect|ReportsSendIdentityHeader|ReportSelfHealRelayDoesNotFallbackToAPI)' -count=1 -v` validou que `selfheal-commands`, `selfheal-report` e `error-report` enviam `X-Agent-Identity`, evitando falso alerta de identidade obrigatória em clientes com `agent_identity_strict_enabled=1`; versão runtime atualizada para `0.8.15`.
- Artefatos `0.8.15`: `./scripts/build_installers.sh` gerou os compactados oficiais; `SHA256SUMS` validou `linux-amd64=9ddfc55a08178747cebc6028347bd407aabb9a67b405c979188958ddf66d75bf` e `windows-amd64=6b014fa9b9fc8ac661e0bdbac652cfc0ed89d5c039e83e473c9a7ac001a9ebf0`.
- Correção 2026-06-20: `go test ./internal/domain/usecase -run 'TestConfigSync'` validou que o agente confirma `/v1/agent/config-report` com `status=applied`, versão e hash após receber payload de configuração. `go test ./internal/interfaces/hub -run 'TestHubProxyForwardsAgentControlRoutes|TestHubProxyPreservesIdentityForBootstrapConfigAndPing'` validou o forward de `config-report` via HUB preservando token e identidade; versão runtime atualizada para `0.8.16`.
- Artefatos `0.8.16`: `./scripts/build_installers.sh` gerou os compactados oficiais; `SHA256SUMS` validou `linux-amd64=feb83617fa836a141c6a6c64cd6b174fc2996751bff3527a93461ba7504e6b07` e `windows-amd64=9507a565c71b046d9a6829f545bea94d21c01916739d15956e28007f2b9875d2`.

## Politica minima de testes

### Cadeia oficial de assinatura de update

- `go test ./cmd/update-signer ./internal/common/updatetrust ./internal/domain/usecase -run 'Test(Generate|Verify|Official|SelfUpdate_(VerifiesTrustedArtifactSignature|RequiresSignatureFromRemotePolicyPrefs|RejectsInvalidTrustedArtifactSignature))'` cobre geração sem sobrescrita, assinatura válida, artefato adulterado, chave oficial válida e rejeição no limite real do self-update.
- `go test ./internal/common/config -run TestLoadUsesOfficialUpdateTrustByDefault` cobre o bootstrap de instalações sem variáveis locais: assinatura obrigatória e chave pública oficial permanecem ativas.
- Release assinada: `AICEBERG_UPDATE_SIGNING_REQUIRED=true AICEBERG_UPDATE_SIGNING_PRIVATE_KEY=<private.pem> ./scripts/build_installers.sh` deve gerar `SHA256SUMS` e `UPDATE_SIGNATURES.json` para cinco artefatos.
- Verificação independente: `go run ./cmd/update-signer verify -manifest dist/UPDATE_SIGNATURES.json -public-key <public.pem> -artifact-dir dist` deve confirmar versão, chave e cinco assinaturas.
- Piloto produtivo deve manter `AUTO_UPDATE_TRUST_REQUIRED=true`, confirmar a chave pública presente, rejeitar uma assinatura inválida sem substituir binário e concluir o pacote oficial com `version_confirmed`.
- Evidência do piloto agente 2: `0.8.48` reportou chave pública configurada e confiança obrigatória; alvo de teste com assinatura válida para outra versão foi rejeitado como `validation_failed/trust_chain`; o pacote oficial `0.8.49` foi aceito e confirmado sem remover a proteção.
- Evidência da expansão: 11 agentes Windows ativos do InspectApp reportaram `0.8.49`, `version_confirmed`, nenhuma pendência e assinatura obrigatória. Hosts legados que falharam por PowerShell ausente, sudo ou launcher sem troca de versão conservaram o binário anterior e a política obrigatória.

- Unitário: validar regra de negocio, casos de erro e limites sem depender de rede, banco real ou UI.
- Integracao: validar contratos entre camadas, banco local/controlado, API e adapters.
- E2E: validar fluxos criticos do usuario quando houver interface ou jornada operacional relevante.
- Cobertura mínima por criticidade: modulo critico deve ter meta documentada no projeto; modulo simples pode exigir apenas evidencia direcionada.
- Dados de teste e fixtures: usar dados pequenos, deterministas, sem dump real de producao e com IDs previsiveis.
- Pacote/lote: lotes usam testes rasos e direcionados; fechamento do pacote exige check completo.
- Bug simples ou melhoria sem impacto de codigo pode usar teste local/direcionado. Bug complexo ou mudanca ampla exige check completo.

### PKG-76 - Readiness EDR/NDR seguro

- Unitario focado: `go test ./internal/bootstrap ./internal/common/config ./internal/domain/usecase ./internal/platform/collectors/logdiscovery ./internal/platform/collectors/oslogs`.
- Cobertura: bloco remoto `edr_ndr`, `EDR_SAFE` fallback, perfil seguro, caps remotos, logs `error+`, manifesto `edr_ndr_readiness`, EventLog/journald, discovery bounded, assinatura de config, assinatura de update e compatibilidade com configuração remota.
- Fechamento: rodar `./check.sh`, gerar artefatos oficiais com `./scripts/build_installers.sh`, copiar para o diretório versionado do web e validar SHA256.
- Artefatos `0.8.18`: `./scripts/build_installers.sh` gerou os compactados oficiais; `SHA256SUMS` validou `linux-amd64=29bd698d9694906f5d48309ba23e0eb1ba590f7445ed20083a6e1935dfa63ffe` e `windows-amd64=f3b9ec0e16b9b23cdecbfe1643a71c13cf0a2ea570d69d5c4c24e6702db7c908`.
- Artefatos `0.8.20`: `./scripts/build_installers.sh` gerou os compactados oficiais; `SHA256SUMS` validou `linux-amd64=50c45fe3c1e2b01e3173df78ee5a29a0967043fb2999d7e17a5f3f888b7c0dd2` e `windows-amd64=1c89c27a360f941097571c7c0b3a8ebb168f3d87191da3a3f952cfa737185f7d`.
- Homologação de fornecedor real: CrowdStrike/Darktrace exigem ambiente real, janela aprovada, evidência do console do fornecedor e ação operacional registrada no web.

### Observabilidade controlada

- Unitário focado: `go test ./internal/domain/usecase ./internal/platform/collectors/custommetrics ./internal/platform/collectors/otlp`.
- Cobertura: `validation_samples_enabled` remoto persiste em prefs, `custommetrics` emite série `aiceberg.validation.custom_metrics` e `otlp_traces` emite span `aiceberg-agent-validation` com `aiceberg.validation_sample=true`.
- Limite: isso prova o pipeline técnico. Telemetria real de aplicação exige DogStatsD/HTTP local ou OpenTelemetry emitido pela aplicação do cliente.

### WordPress SOC em Nginx access

- Unitário focado: `go test ./internal/platform/collectors/oslogs`.
- Positivo: três REST batch, login POST com redirect, upload de plugin e tentativa executável produzem eventos estruturados com timestamp UTC, IP e ação.
- Privacidade: payload não contém valores de query; somente nomes limitados das chaves.
- Negativo: rota WordPress comum é descartada e batch isolada é coletada sem o agente declarar comprometimento.
- Fechamento coordenado: `./check.sh` no agente e no web, artefato 0.8.43 com SHA-256 e validação da configuração aplicada no agente 70.
- Artefatos 0.8.43: `linux-amd64=a0efdad5ce9d59d95918ad4d2f6368ef2fbb0f188c7dda68845d68542c34a388` e `windows-amd64=38c119fc98ccd0427cf11579922a83313061924fe57071f2afcb74ab7ad7911b`.

### Auto-update com launcher Linux privilegiado

- Unitário focado: `go test ./internal/domain/usecase -run 'TestSelfUpdate_Preflight(AcceptsAuthorizedPrivilegedLauncher|FailsWhenInstallDirIsNotWritable)' -count=1 -v`.
- O preflight só pode delegar a escrita do diretório de instalação quando o comando usa `sudo -n`, o launcher é absoluto/executável e `sudo -n -l` confirma a autorização; sem qualquer desses requisitos, deve retornar `install_dir_not_writable`.
- Piloto `0.8.45`: confirmar `precheck_ok`, download/hash, `apply_dispatched`, restart, reconexão, `version_confirmed`, serviço ativo e outbox drenando.

### Identidade assinada no Agentless HUB

- `go test ./internal/data/remote -run 'TestAgentlessHubClient(FetchJobsWithCommandOptions|SendObservationsIncludesSegmentMeta)' -count=1 -v` valida `Authorization` e `X-Agent-Identity` em jobs e observações.
- `./check.sh` fecha a validação do agente antes da geração do artefato assinado `0.8.54`.
- Artefatos `0.8.54` gerados e verificados com a raiz Ed25519 oficial: `linux-amd64=d6664ba2fb8101dfe393baff9aa7d6d04c6de4f8127f1c5c14646d2e5d41d568` e `windows-amd64=27e5ad28459ab245d8ca31d35d13881a507f1bb9d412381b79fb9d9183c22e7b`; manifesto confirmou cinco assinaturas com `aiceberg-agent-prod-v1`.
- Canário: Agente 4 deve consumir jobs via HUB sem `identity_missing`, enviar observação e manter serviço/health ativos antes da expansão.
- Expansão: confirmar `version_confirmed` e serviço saudável em todos os agentes ativos do cliente; preservar `0.8.53` para rollback.
- Produção: artefatos `0.8.54` publicados e validados em `downloads.aiceberg.com.br` e `update.aiceberg.com.br`; Agentes InspectApp `4`, `70`, `71` e `73` chegaram a `version_confirmed`, sem payload pendente. No canário `4`, o check TCP `14453` do MySQL OCI voltou ao HUB, persistiu observação `source=hub`/`hub_agente_id=4`/`status=ok` e não registrou novo `identity_missing` após o restart.


## Agentless — capacidade e OIDs (16/09/2026)

- `go test -race ./internal/domain/usecase ./internal/platform/agentless`: testes focados de paralelismo limitado, não sobreposição por endereço, cancelamento, propagação de falha, persistência de 100 resultados, drenagem limitada, falha HTTP sem ACK e fila preservada. Teste de GET cobre resposta com e sem ponto inicial e OID configurado com ponto.
- `./check.sh`: lint, vet, testes e self-tests oficiais; executar novamente no repositório de integração antes do commit. Primeira rodada na cópia isolada falhou por expectativa int64 num payload normalizado para float64; expectativa corrigida e reteste aprovado.
- Benchmark sintético: `go test ./internal/domain/usecase -run '^$' -bench BenchmarkAgentlessBatch -benchtime=3x -count=1`; 64 destinos com espera simulada de 5ms: 356,5ms por lote sequencial e 44,4ms com oito workers na amostra local. Não representa vazão de produção nem gate de capacidade do HUB.
- Aceite operacional pendente: confirmar versão assinada no HUB, fila sem crescimento sustentado, tempo de lote menor que lock, disponibilidade preservada, resultados úteis e recorrência.

## Polling condicional, recuperação e jitter (01/10/2026)

- `go test ./internal/common/schedule ./internal/domain/usecase ./internal/bootstrap`: cobre ausência/presença da versão no GET, `204` sem aplicação nem `config-report`, `200` com configuração, `collect_now` e update, recuperação somente após `open` confirmado, cooldown e falha de entrega de `open`.
- `go test -race ./internal/common/schedule ./internal/domain/usecase ./internal/bootstrap`: valida a proteção concorrente da máquina de estados.
- O jitter usa relógio/hash controlados, sem espera real: mesma identidade/rotina mantém a fase, identidades/rotinas diferentes distribuem fases, o valor fica em `[0, intervalo)` e o intervalo permanente não muda.
- Reteste canário: confirmar versão, serviço e health; comparar janela equivalente antes/depois para `GET /v1/agent/config` (`200`/`204`), `POST /v1/agent/config-report`, `POST /v1/agent/error-report`, CPU, taxa total e 5xx; executar `collect_now`, update e self-heal controlados antes de ampliar.

## Reteste de ingestão relay/HUB com fila cheia — 16/10/2026

`TestHubDoesNotAcknowledgeFailedOrPartialPersistence` exige HTTP503/Retry-After nas rotas métricas/logs quando Append falha no início ou no meio do lote, preservando os envelopes já gravados. Os testes existentes mantêm ACK202 para lotes íntegros e identidade relay. `TestOutboxRuntimeEvidenceIsReadOnlyAndDoesNotExposeTransportPayload` confere contadores e ausência de credenciais/erro bruto. Gate completo obrigatório. Validação produtiva: confirmar versão no HUB84, outbox_runtime, métricas novas do SBC674 e regularidade da janela; update sozinho não encerra o incidente.

Os testes de contrato de arquivos/runtimes de logdiscovery injetam processos/listeners controlados, em vez de varrer processos reais do Mac com lsof. Os adaptadores reais continuam como defaults em produção; a validação não desliga nem enfraquece a descoberta produtiva.

### Capacidade limitada do HUB —0.8.56

Teste coordenado por canais comprova dois destinos simultâneos, limite de workers, ACK apenas dos três destinos bons e retenção do destino falho. Teste de bootstrap cobre defaults HUB400/4, tetos400/8, rollback50/1 e ausência de mudança em relay/direct. Executar race detector nos novos testes e gate completo. Regularidade SBC em produção permanece gate separado.

Publicação produtiva: agente468cb44 e Web3a74351e enviados, workflow https://github.com/mayder/aiceberg_web/actions/runs/38944283175 SUCCESS; cinco hashes/assinaturas Ed25519 e hash Linux público conferidos. HUB84/SADA confirmou0.8.56/config26 às18:04:04UTC. Inspect96: capacidade400/4, 201572082bytes, ACKs válidos. Inspect97: idade mais antiga do lote400s (antes550s), fila207613953bytes/9208itens; um ACK recusado é mantido para retry, sem alteração de retenção. Regularidade SBC permanece em validação. Race detector, gate completo e contrato relay/HUB passaram.

0.8.57: mesmos testes de limite/concurrency/cancelamento/race, com default/teto800/8 no HUB. Relay/direct inalterados. Motivação produtiva: fila novamente209713514bytes com400/4; sustentar escoamento/atualidade do SBC é gate separado.

Rollout57/HUB84 confirmado às18:26:37UTC, config27 aplicada. Inspect100 confirmou800/8, fila199948968bytes/8813itens e ACKs sem perda; dados do SBC voltaram (memória75%, entrega18:30:48UTC, fonte18:26:41UTC, atraso246,6s). A fila ainda está em escoamento; não declarar fim do atraso nem avaliação natural da média corrigida. Carga central medida4,32→4,14 em8CPUs e memória disponível80,7→80,6%; não é ensaio de p95/capacidade máxima. GOS/runtime sem alteração de ownership ou versão.

Central de atualização: testes focados de readiness cobrem launchers canônicos por SO,
rejeição de comandos adicionais/paths alternativos, todos os booleanos obrigatórios,
updater desabilitado/contexto cancelado sem criar staging, pendência preservada e
configuração alterada. Inspector explícito não invoca snapshot amplo e recusa flag
malformada ou callback ausente. Build/teste local não comprova apply em Windows/macOS.

Validação deste candidato: gate raiz PASS; testes focados/race PASS para readiness e
inspector; compilação Windows AMD64 e Linux ARM64 PASS. Nenhum pacote publicado,
serviço instalado ou agente remoto modificado. macOS launchd e piloto continuam pendentes.

macOS updater candidato: `python3 scripts/test_macos_updater.py` cobre8 cenários:
sucesso com backup, health falho seguido de rollback, bootout recusado sem troca,
SHA256 inválido, health externo recusado, caminhos maliciosos/plataforma errada no
tar, symlink de entrada e health com versão divergente. Shell real executa sobre
arquivos temporários; launchd e resposta health são substituídos, sem instalar serviço.
`bash -n` e testes entram no gate raiz; plist é validado com plutil em Darwin.
Piloto real, instalação em root, assinatura dos novos artefatos e confirmação remota
permanecem gates separados. Nenhuma assinatura oficial é usada nos fixtures.


## Entrega rápida Agentless — PKG-49 (06/11/2026)

`go test -race ./internal/data/local/agentless ./internal/data/remote ./internal/domain/usecase ./internal/common/config -run 'TestAgentless|TestDelivery|TestLoadAgentlessDelivery' -count=1`:
20testes/13subcasos PASS. HTTP/Bolt reais locais: paridade/reinício, uma cabeça por
coleta, sequência/final, fast recente sem bloquear todos os antigos, justiça para
lentos, isolamento limitado de payload com dedupe, parcial/legado sem classe
interrompe,401/429/5xx/ACK vazio/malformado, cancelamento ativo e flag opt-in/rollback.
Web protege projeção sob lock contra backlog velho e clock futuro sem reescrever
observações, com concorrência real verificada em MySQL. Não é homologação do HUB.

Benchmark Bolt sintético:1000streams mistos/1KiB lógico, seleção100,3iterações,
Mac ARM64:11,54ms/seleção. Não comprova p95/capacidade/SLA instalado. Gate final
`./check.sh`; cross-build/artefatos assinados, publicação, ativação e prova dos120s
permanecem nos gates posteriores. Evidência em docs/evidence/pkg49-local-20261106.md.

## Interop e builds do diagnóstico de configuração

`AICEBERG_CONFIG_FAILURE_WEB_ROOT=/Users/brenomayder/projects/web/public/aiceberg_web go test -race ./internal/domain/usecase -run TestConfigDiagnosticWireAcceptedByActualPHPConsumer -count=1`:
PASS com four phases first/summary/changed/recovered: serializador HTTP real Go e
normalizador PHP real, sem DB/env local/provedor. Fixtures de assinatura inválida
sem efeito, correção válida imediata, renovação sem novo incidente, estado não
gravável, proteção de prefs/outbox, métricas e corpo excessivo/trailing passaram.
Builds CGO0 cmd/agent:linux/amd64,linux/arm64,windows/amd64,darwin/amd64,darwin/arm64
PASS. Testes nativos Windows de códigos e safefile compilados; não executados neste
Mac. Binários de validação temporários, sem gerar/assinar/publicar release oficial.
Execução instalada/ACL nativa/falha física por SO pertencem à homologação separada.

## Artefatos Go — gate de publicação PKG-134

Usar o Go fixado no go.mod/toolchain; conferir go version -m nos cinco binários
com versão esperada, vcs.revision exata e vcs.modified=false. Executar Trivy
rootfs nos diretórios extraídos, com targets gobinary reconhecidos para todos
os cinco OS/arch e zero HIGH/CRITICAL antes da assinatura/distribuição. Scan fs
sem alvo gobinary não conta como validação. Não adicionar VEX/ignore para liberar.
Verificar Ed25519/SHA256SUMS, catálogo e hashes dos sete arquivos públicos.
Versão publicada é imutável; correção gera novo número e preserva anterior.

check.sh usa golangci-lint2.10.1 compilado com GOVERSION do projeto e cache por
versão/toolchain. Configuração v2 preserva defaults v1 pela migração oficial;
nenhuma regra QF foi desabilitada. Depois de mudança de compiler/dependências,
rodar race dos fluxos de configuração/estado/updater/HUB e dos consumidores de
rede afetados, além do check completo e cross-builds. Homologação nativa segue
separada; não injetar falhas destrutivas em produção.

## Suporte do próprio serviço — 17/11/2026

`go test -race ./internal/bootstrap ./internal/platform/support ./internal/domain/usecase -run 'Test(BootstrapSupport|Collect|ServiceSources|SupportFlare|SelfHealExecutorCollectSupport)' -count=1`
valida fábrica usada no bootstrap, ferramenta fixa por plataforma, execução real
de subprocesso controlado, timeout/cancelamento, tamanho, fonte ausente/vazia,
permissão negada e redação de credenciais repetidas/URL/header. Callback ausente
ou com erro conserva evidência segura e falha. Fixture de shell de seleção de
ferramentas é POSIX; não é prova de Event Log/permissão em Windows nativo.
Cross-build/artefato assinado e piloto por SO são gates separados. Recibo no Web:
docs/evidence/pkg80-local-20261117.md. Nenhum host real comandado por estes testes.

Suporte18/11: go test -race em logger/support/bootstrap/usecase PASS. Cobertos
limites/ordem/expiração1h, escrita/leitura concorrente, segredo após limite de bytes,
fallback com log real redigido, fonte SO ausente preservada, buffer vazio,
cancelamento e plataforma não suportada. Permissão produtiva falhou explicitamente
no HUB84/0.8.69; novo candidato0.8.70 exige recibo real, não fixture.

## Amostras canônicas iniciais —19/11/2026

`AICEBERG_MONITORING_WEB_ROOT=<Web> go test -race ./internal/domain/monitoring -count=1`:18vetores e PHP real. `go test -race ./internal/platform/agentless -run TestCanonicalProjection -count=1`: observação legada preservada, duas séries sem nova coleta, chave estável ao mudar executor e rejeição de outro check. Não comprova sensor físico, emissão canônica ou capacidade da frota.

## SNMPv1 e candidata de sensores —19/11/2026

`go test -race ./internal/platform/agentless -count=1`: seleção de versão, GET/GETNEXT UDP em loopback, OID ausente, sem GETBULK v1, row/request/rate/budget/cancel, ordenação/loops e regressões v2c/v3. Fixture ENTITY-SENSOR-MIB produz25.3°C com tipo/escala/precisão/estado/TimeTicks; casos incompletos, indisponíveis, antigos, overflow, precisão negativa e wrap são unknown. `go test -race ./internal/domain/usecase -run TestUpdateCandidateVersionPrecedence -count=1` e testes de update: candidata→final, build metadata e proteção de downgrade. Não é teste do sensor físico. Gate geral continua bloqueado.

Complemento116: probe transporta diagnostic_probe em resultado final/segmentos; servidor exige execução autorizada e armazena somente no resultado diagnóstico, sem projeção/check/threshold. Teste UDP verifica o marcador. Nenhuma observação de rotina é marcada.

## Espera limitada do comando Agentless —19/11/2026

`go test -race ./internal/bootstrap`: rotina ativa não se sobrepõe a comando; reserva única rejeita outro comando e64tentativas concorrentes de rotina; aquisição aguarda liberação, cancelamento/deadline não removem lock alheio e rotina retoma. Prazo inclui espera, default5min e teto1h. Extração de parsing mantém testes de canal existentes. Não comprova coleta física do sensor; piloto anterior recebeu skipped_busy166/167/168.

## Diagnóstico SNMP básico —20/11/2026

Candidata0.8.71-rc.3: preset system_get com GET único de uptime/identidade,
1500ms/retries0/teto5s, sem WALK. Testes: go test ./internal/platform/agentless
-run TestBasic -count=1; fixtures UDP reais em loopback v1/v2c com dados sintéticos,
valor zero válido, timeout sem resposta, counters, limites, redaction e erros PDU.
Receber UDP não comprova resposta SNMP válida. Piloto apenas HUB84 após gates
e assinatura; controles positivos antes dos sensores, sem alterar perfil/rotina.
PKG-116 parcial, sem leitura física nova ou delegação relay declarada. Procedimento
e rollback: Web docs/operations/snmp-v1-sensor-pilot.md; preservar config/outbox.

Revisão rc.4: TestBasicProbeRealUDPRejectsMismatchedCommunityAndZeroRequestID
verifica datagrama recebido sem qualificação de resposta quando community ou
requestID divergem. Não registra community de requisição/resposta no relatório.

## Diagnóstico relay —20/11/2026

Testes Go dos pacotes bootstrap/domain/usecase/data/remote/interfaces/hub/common/relayprobe:
jornada UDPv1 sintética real através do proxy, identidade/prova do HUB,503 e ACK
explícito, persistência/reabertura Bolt e retry sem outro GET. Rejeição de modo,
TTL, comando/payload/OID/versão/executor inválidos; sem API direta ou memória.
Vetor HMAC idêntico no PHP. Web python3 -B scripts/snmp-sensor-probe-lab.py --relay
prova request/claim/resultado concorrentes em MySQL nativo exclusivo, com cleanup.
Fixtures não são sensores físicos; piloto, três ciclos e aceite permanecem separados.

Instalações legadas sem IDs numéricos em env reaproveitam HostGUID medido e
persistido no bootstrap (ou lido do host), mantendo a mesma identidade assinada.
Sem identidade, não fazer fetch. Teste verifica que não é inventado installation_id.

## Definições de itens — incremento local20/11/2026

`go test -race -count=1 ./internal/domain/monitoring ./internal/platform/agentless`
cobre52vetores, limites, DAG, roundtrip e adapter sem alterar job, identidade,
política ou copiar segredo/endereço. O adapter rejeita diagnóstico como rotina.
`AICEBERG_MONITORING_WEB_ROOT=<Web> go test -race -count=1 ./internal/domain/monitoring`
comparou os52vetores com PHP real antes de isolar o patch Web. Enquanto o Web
estiver só em patch, esse interop exige integrar o patch; sem env, SKIP explícito.
`go test ./internal/domain/monitoring -run '^$' -fuzz '^FuzzDefinitionWire$' -fuzztime=5s -parallel=2`
executou597860casos sem falha. Cobertura das regras semânticas novas100%; codec
acima80%. Isso não é cobertura global do agente nem teste produtivo/aceite.
Sem fixture de banco, Docker, simulador, nova build distribuível ou deploy.

## Execução canônica SNMP21/11

AICEBERG_MONITORING_WEB_ROOT=<Web> go test -race -count=1
./internal/domain/monitoring ./internal/platform/agentless ./internal/data/remote
./internal/data/local/agentless ./internal/domain/usecase. Novos casos executam
definições,18vetores, parciais/diagnóstico, UDPv1 sintético com2OIDs/uma coleta,
Bolt reaberto, HTTP503/retry e comparação dos consumidores PHP/MySQL/Go.
Native opt-in pelo scripts/location-schema-lab.py do Web, sem produção/config local.
O banco e a outbox são descartados no cleanup; nenhum dispositivo físico testado.


TCP canônico22nov: `AICEBERG_MONITORING_WEB_ROOT=<checkout Web> go test -race
./internal/domain/monitoring ./internal/platform/agentless -run 'TestTCP|TestCanonicalTCP'
-count=1`.21vetores compartilhados, definição/tenant/clock, zero, cancelamento,
parcial/diagnóstico e coleta TCP única. Bolt reaberto/HTTP503/retry/PHP/MySQL no
laboratório oficial Web, com regressão SNMP/consumidor SQL Go. Não é piloto real.


PKG-115/item2: AICEBERG_MONITORING_WEB_ROOT=<Web> go test -race -count=1
./internal/domain/monitoring.110vetores semânticos,12wire,8evolução, schema PHP,
paridade com PHP real, imutabilidade/roundtrip,128registros/256KiB, Unicode/duplicatas,
underflow/overflow e histórico atrasado vs frescor atual. FuzzRecordsDecodeNeverPanics
com3s/2workers executou292908casos sem panic, mais seeds. Sem produção/banco/rede.
Fixtures idênticas às cópias Web/Runtime; novos contratos continuam inertes.


Monitoramento universal/item3 —22nov:86vetores idênticos PHP/Agent/Runtime,
45combinações fonte/tipo, negociação/perfis/limites/read antes do parser,
no downgrade e JSON opaco. PHP MonitoringRecordsTransportTest:3testes/
382asserções/47statements cobertos; Go novas funções100%statements. Fuzz3s/
2workers:207868execuções sem panic. Reteste: AICEBERG_MONITORING_WEB_ROOT=<Web>
go test -race -count=1 ./internal/monitoring (Runtime) ou
./internal/domain/monitoring ./internal/interfaces/hub (Agent). Agent cobre
Relay→HUB→Bolt reaberta→HTTP→Go/PHP, request externo4096B,503/ACK unsupported,
resposta perdida/reentrega sem mudar IDs/valores/estados/clocks. Gzip no HUB
recusado mantendo fila; plain até HUB, gzip direto qualificado. Modo legado é
regressão de valores válidos, não perfil autorizado universal. Dedupe final usa
fixture por ID, sem SQL universal. Check completo no hook; sem aceite produtivo.
Contrato Web:docs/contracts/monitoring-records-transport-v1.md.


Configuração composta/item4 —22nov:82vetores compartilhados e45combinações fonte/
tipo; schema, autoridade/pins/vínculos/revisão, completude/limites, referências,
DAG e evolução. PHP MonitoringConfigurationServiceTest6testes/345asserções,
94/94statements dos serviços novos; novas funções Go83,3–100% e race PASS.
Runtime: AICEBERG_MONITORING_WEB_ROOT=<Web> go test -race -count=1
./internal/monitoring ./internal/controlapi ./cmd/monitoring-preview; Agent:
./internal/domain/monitoring.9catálogos PHP reais cifrados/CAS/restaurados com
IDs/config_json preservados, herança/precedência/cancelamento e regressões.
Fuzz3s/2workers253execuções sem panic. UI real no laboratório oficial:10campos,
prévia/save/recarga2→3,1280/390px sem overflow/console, CSRF/perfis/tenant,422 na
aplicação legada sem criação de checks. Laboratório próprio removido; sem aceite
produtivo ou físico. Contrato Web:docs/contracts/monitoring-configuration-v1.md.


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


### Projeção TCP para o grafo canônico

`TestTCPMonitoringUsesOneExistingConnection` usa socket TCP em loopback
e projeta as amostras da execução pelo adaptador compartilhado com o Runtime.
Uma conexão alimenta cinco resultados. Os testes focados de SNMPv1 em loopback
confirmam GET, GETNEXT, retorno de um sensor simulado e entrega ao consumidor
PHP via fila local e HTTP. O teste instalado certificará as respostas dos
sensores da SADA com agentes do recorte autorizado.


### Recibo dos adapters de coleta

A suíte `go test -race ./internal/platform/agentless ./internal/data/remote`
cobre protocolo desconhecido sem eco da entrada, cancelamento de handshake
TLS e o contrato TCP em loopback. Com `AICEBERG_MONITORING_WEB_ROOT`, o teste
integrado confirma uma conexão TCP, persistência na fila após reabertura,
HTTP 503 e reentrega ao consumidor PHP com recibo vinculado ao check.


A política de execução tem regressão HTTP real em loopback para timeout total e rejeição antes da rede, além de cobertura para perfil SNMP e frescor do check base.

`go test -race ./internal/domain/usecase ./internal/platform/agentless -run 'AgentlessHTTPKeepsRaw|ICMPExtractsLatency'`
verifica que credenciais e parâmetros de URL, texto de status recebido
e saída de ping ficam fora do debug e do outbox. O teste HTTP usa uma
resposta de loopback, uma falha de conexão e a fila Bolt. Os códigos
de resultado, o status HTTP normalizado e a latência continuam disponíveis.

`TestSNMPErrorDetailsStayOutOfPartialAndFinalObservations` simula erros
de tempo, autenticação, conexão recusada e transporte no GET e WALK.
Confere a categoria, os OIDs com falha e a ausência de dados brutos nas
observações parciais e finais. O teste Net-SNMP continua cobrindo
credenciais aceitas e recusadas nas três versões.
No macOS, usar o Net-SNMP com controle de acesso. O binário nativo
5.6.2.1 ignorou `rocommunity` no laboratório. A fixture confere a
recusa de uma comunidade incorreta antes de executar o teste do Agente.

HTTP em loopback validou uma coleta para cinco itens derivados via contrato canônico, usando a mesma implementação das séries de latência.

`TestManualSNMPBackpressureReservesEveryObservation` cobre fila Bolt
cheia, folga insuficiente para as partes SNMP e recuperação. Depois de
liberar três vagas na fixture, o HUB envia duas parciais e um final
por HTTP a partir de respostas UDP de loopback.

`TestAgentlessByteBackpressureStopsLeasesAndRecoversAfterFlush` deixa
um único registro na fila Bolt com 1 byte livre. Confere que o Agente
adia rotina e comando manual, recusa uma nova gravação e volta a
executar após o servidor confirmar a entrega do registro pendente.

`TestAgentlessBoltUsageMatchesCommittedRecords` compara os arquivos
Bolt com os totais usados para admissão após inclusão, substituição,
recusa por tamanho, ACK repetido, expiração e reabertura. A suíte de
traps verifica os mesmos totais após descartar uma repetição.
Para medir a consulta e a gravação com 400 e 10.000 observações,
rode `go test -run '^$' -bench '^BenchmarkAgentlessBoltCapacity$' -benchtime=200ms -count=3 ./internal/data/local/agentless`.

`TestAgentlessRateCapacityKeepsOtherClientsRunnable` preenche a cota
de um cliente e confirma o despacho de outro no mesmo hostname,
a liberação de vaga em cancelamento e a retomada após a janela.
`TestAgentlessGlobalRateCapacityResumesAtFirstVacancy` preenche o teto
do HUB, confirma o primeiro vencimento e reaproveita exatamente uma vaga.

`TestServerCancelledPostgresStatementProvesEndpointReachable` valida
que o SQLSTATE 57014 encerra a sequência de falhas de conexão, enquanto
timeouts de conexão PostgreSQL continuam abrindo o circuito.
`TestSNMPErrorDetailsStayOutOfPartialAndFinalObservations` também
confere os motivos do recibo em recusa de conexão e rejeição de
autenticação.

`TestAgentlessHubPausesOnlyTheCheckExhaustingBudget` executa dois
checks HTTP no mesmo endpoint. O cenário valida a pausa após três
timeouts de orçamento, continuidade do check saudável, diagnóstico
manual e teste de recuperação após a pausa. O teste SNMP cobre
orçamento excedido com leitura aproveitável. A jornada de Chromium
também verifica o motivo `budget` no recibo de timeout do runner.

`TestAgentlessHubMixedLoadSurvivesDeliveryOutageAndRestart` usa 62
checks, nove clientes, até oito workers, HTTP e SNMP UDP em loopback,
com fila Bolt. O receptor responde HTTP 503 após o primeiro ACK.
O teste reabre a fila, confere a ordem dos segmentos SNMP, a entrega
das 56 observações pendentes e a ausência de nova coleta.

`AICEBERG_MONITORING_WEB_ROOT=/caminho/do/web go test -race -count=1 ./internal/data/remote -run '^TestSNMPPilotReceiptsFromUDPQualifyThroughTheWebExport$'`
exercita três GETs SNMPv1 por referência em loopback, o export de
recibos do Web e a qualificação pelo validador. Um timeout posterior
reinicia a sequência e o estado do check operacional é preservado.

O SSH restrito usa servidor SSH em loopback e chaves Ed25519 efêmeras.
`go test -race ./internal/platform/agentless -run 'SSHProbe'` cobre
autenticação por chave, escopo, chave de host divergente, comando fixo,
saída excedida, cancelamento e leitura de valor zero.

`TestSSHProbeAgainstRestrictedOpenSSH` executa o mesmo coletor contra
OpenSSH em `127.0.0.1`. Fornecer
`AICEBERG_SSH_OPENSSH_FIXTURE_PORT` e o arquivo `0600`
`AICEBERG_SSH_OPENSSH_FIXTURE_PROFILE`. O perfil de laboratório usa
`ssh_probe_profile:51`, cliente 3, ativo 52, check 72 e endpoint 17,
com chaves temporárias. Usar conta de destino separada e forçar o
comando fixo, com sessões interativas e forwarding desabilitados.
O ensaio Ubuntu arm64 verificou três medições de `/proc`, recusa por
escopo e chave de host divergente e recuperação com a chave correta.
Em Debian arm64 com OpenSSH 9.2, o reteste também confirmou que a chave
cliente recusada retorna `ssh_access_denied`, sem medição ou credencial
no recibo. A regressão de admissão confirma que os outros checks do
serviço permanecem elegíveis após uma recusa explícita.
Reexecutar a regressão de chave recusada ao atualizar `x/crypto/ssh`,
que sinaliza a rejeição do lado cliente por uma mensagem de erro.

`TestIPMIProbeAgainstLoopbackBMC` exercita o coletor com ipmitool 1.8.19
e o BMC em `internal/platform/agentless/testdata/ipmi_rmcpplus_bmc.py`.
Use um ambiente Python isolado com Pyghmi na
[revisão testada](https://opendev.org/x/pyghmi/commit/7f826446c14427db3516d48bd5cc36f2b81450ec).
Inicie a fixture Python e use a porta impressa em
`AICEBERG_IPMI_LOOPBACK_FIXTURE_PORT`. Informe o caminho absoluto do
ipmitool em `AICEBERG_IPMI_LOOPBACK_FIXTURE_TOOL` e rode
`go test -count=1 -v ./internal/platform/agentless -run '^TestIPMIProbeAgainstLoopbackBMC$'`.
Encerre a fixture após o teste. O cenário comprova cifra 17, unidades,
zero, lacuna, credencial incorreta e recuperação em loopback.
Linux amd64 em Ubuntu 22.04.5 passou com o ipmitool 1.8.19 do Debian
extraído em ambiente descartável. Com o ipmitool 1.8.18 da distribuição,
o mesmo ensaio retornou sem linhas e o coletor marcou
`invalid_ipmi_response`. Registrar o binário efetivo na matriz de campo.

`go test -race -count=1 ./internal/domain/usecase -run '^TestCancelledSNMPDiagnosticSurvivesOutboxReopen$'`
cancela o diagnóstico após o primeiro datagrama em loopback. O teste
reabre o Bolt e confirma a entrega do recibo com comando e correlação
originais e sem amostras operacionais.

`python3 scripts/check-mysql-collector.py` executa MySQL local descartável
com TLS e conta sem grants de tabelas. Cobre as quatro variáveis, CA
desconhecida, credencial incorreta, escopo de cliente e SQL livre recusado.
Defina `AICEBERG_MYSQL_PROBE_FIXTURE_BIN` quando os binários não estiverem no PATH.

`python3 scripts/check-postgresql-collector.py` executa PostgreSQL
descartável com TLS/SCRAM. Os dois scripts alteram a senha da conta da
fixture pelo socket Unix privado e mantêm o mesmo processo de teste.
O probe com o perfil antigo falha sem publicar valores. A substituição
atômica do arquivo privado recupera todas as séries com a mesma
referência. O teste também verifica a ausência de segredos nas
observações de falha e recuperação.

`go test -race ./cmd/agent ./internal/platform/telnetprobe` cobre a
entrada do processo filho com socket real em loopback, opções recusadas,
banner sem conteúdo persistido, escopo local, cancelamento e resposta
excedida. TCP com texto comum mantém falha de identificação.

Em um ambiente Python descartável com `telnetlib3==5.0.3`, executar
`python3 scripts/check-telnet-collector.py`. O servidor independente
abre duas portas efêmeras em loopback e confirma fim de sessão sem
dados de aplicação. Os recibos indicam 1 com banner e 0 após negociar
sem banner, sem incluir o texto recebido. Em Linux arm64, os testes
do processo filho, escopo, cancelamento e orçamento também passaram
sob UID 65534 em contêiner descartável, sem capacidades e sem rede externa.

A jornada de navegador usa um Chrome gerenciado já instalado com perfil
temporário. Execute `AICEBERG_BROWSER_FIXTURE_CHROMIUM=<binário-absoluto>
go test -race ./cmd/browser-probe ./internal/platform/browserprobe`.
Com `AICEBERG_BROWSER_FIXTURE_CHROMIUM`, a jornada HTTPS local valida
certificado confiável, recusa de certificado sem confiança e recusa de
nome incorreto. As raízes sintéticas ficam no processo de teste. O
runner instalado usa a cadeia de confiança do sistema.
A fixture navega, clica, lê estado via GET e confirma texto visível. Cobre
bloqueio de POST, subrecurso e redirect externos, corpo excessivo e
cancelamento de uma requisição ativa.
O teste de sandbox executa um wrapper com `--no-sandbox` e confirma
`runner_unavailable` e zero requisições à origem. A regra de argumentos
também roda nativamente na fixture Ubuntu arm64 sem depender de Chromium.

Validar a unidade Linux do pacote com `systemd-analyze verify`. A fixture
Ubuntu arm64 compila e inicia o runner sob um UID dedicado, confirma
diretório do socket `0750` e socket `0660`, aceita conexão de outro UID
no grupo, recusa UID fora do grupo, confere a montagem somente leitura
das receitas e confirma a remoção do socket na parada. A prova da
jornada com Chromium e sandbox é um ensaio separado.

Num laboratório Linux com systemd, executar
`sudo python3 scripts/check-browser-linux-network.py --fixture-ip <ip-local-da-vm>`.
O roteiro abre duas fixtures HTTP locais e aplica as mesmas regras da
unidade a processos transitórios. Verifica loopback liberado, IP fora da
lista bloqueado e acesso restaurado somente após incluir o IP exato.
Usar um endereço IPv4 privado ou IPv6 ULA atribuído à VM, fora do
loopback. Executar para as duas famílias atendidas. As units de teste são
coletadas ao terminar.


## Armazenamento passivo —23/11/2026

Fixtures novas de storage_pressure e kernel wait; no Agente testar sysmetrics/oslogs, baseline, boot/reset, fila sem conclusão, arquivoISO/fuso e preservação deError para fontes alheias. No Runtime executar TestStoragePressurePHPParityAndNoSend e TestStorageControllerWaitDeterministicWithoutProvider; PHP precisa ser local e AICEBERG_WEB_ROOT deve apontar ao Web oficial. Não usa provedor real nem dispatch externo. Publicação/ingestão/ACK instalado e entrega permanecem gates separados.

`TestBrowserServiceRunsTheApprovedRecipeFromTheHub` conecta o HUB ao
serviço local em socket Unix e executa uma jornada de três etapas em Chrome
real. Verifica as duas amostras e recusa alteração de escopo ou hash antes
de repetir a coleta. A suite verifica limite de concorrência no runner e
remoção do socket ao finalizar.

`python3 scripts/check-openmetrics-collector.py <binário node_exporter>`
inicia o Node Exporter (qualificado com a versão 1.12.1) e um exporter
descartável com `promhttp` v1.23.2, ambos apenas em HTTPS loopback. O
roteiro usa Go 1.25, OpenSSL e `htpasswd` com bcrypt. Confirma Text 0.0.4
e OpenMetrics 1.0.0, duas métricas reais de runtime e recusa de senha,
CA e cliente incorretos. A recusa de senha produz lacunas sem valores.
Repor o perfil recupera a coleta. Os dois ensaios usam `-race` e os
processos e arquivos temporários são removidos no final.

`python3 scripts/check-jolokia-collector.py <JAR JVM javaagent>` inicia
Jolokia em uma JVM descartável no loopback, com Basic e TLS. O Agente
lê heap e threads por referência, recusa senha incorreta e CA
desconhecida, bloqueia outro cliente e recupera a coleta com o perfil
autorizado. A fixture verifica também a recusa de um comando fora da
política de leitura e encerra a JVM no final.

`python3 scripts/check-jmx-collector.py` compila o executor JMX direto com
`javac --release 17` e usa o agente nativo de gerenciamento do OpenJDK com
JMX RMI/TLS e credencial somente leitura. Cobre as seis métricas, porta
anunciada fora do perfil, CA e identidade TLS, credencial e prazo total.
O teste encerra as JVMs e remove os arquivos da fixture.

A fixture JMX empacota o JAR localmente e chama o coletor pelo fluxo
normal `RunJob`. Comprova dados de heap, uptime e threads, escopo e
recusa de JAR com hash divergente. O jar, o perfil e os diretórios
de execução são removidos ao final.

Para executar em imagem Linux com JDK sem instalar Go nela, compilar
`GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o "$BINARIO_TESTE" ./internal/platform/agentless`
e passar o caminho do binário no ambiente de destino em
`AICEBERG_JMX_GO_TEST_BINARY` ao chamar o mesmo script. Essa modalidade
executa o binário de testes nativo da plataforma sem detecção de race.

### VMware vCenter e ESXi

`go test -race -count=1 ./internal/platform/agentless ./internal/domain/usecase -run 'VCenter|ServiceAuthenticationRejection'`
roda os modelos TLS de vCenter e ESXi. Injeta HTTP 503 no login, perda de
conexão durante a leitura, HTTP 401, recusa SOAP de permissão e resposta
inválida. Confirma os motivos no recibo, a recuperação da coleta e a retomada
do circuito por resposta de autenticação sem pausar o check vizinho.

### Polling contra Net-SNMP

`AICEBERG_SNMPD_BIN=/caminho/para/snmpd go test -race -count=1 ./internal/platform/agentless -run '^TestSNMPPollingAgainstNetSNMP$'`
inicia um daemon independente, somente em UDP loopback e com configuração
privada de fixture. Para v1, v2c e v3 com SHA/AES authPriv, verifica GET de
`sysName`, OID ausente, WALK truncado no limite de três linhas, recusa de
credencial inválida e recuperação com a referência original. O ensaio
verifica o recibo e a ausência de segredos na observação serializada. O
daemon e os arquivos temporários são removidos pelo teste.

## PKG-116/L09 — Admissão e isolamento do HUB

`go test -race -count=1 ./internal/domain/usecase -run 'TestAgentless(HubCircuit|Circuit)'`
verifica as portas que identificam o serviço de TCP, TLS e HTTP. O ensaio
TCP usa duas portas no mesmo IP de loopback: a indisponível abre o
circuito após três falhas e a saudável mantém quatro coletas. URLs HTTP
equivalentes compartilham a cadência por host, com circuitos separados
por porta.

O teste `TestCollectCommandDoesNotEnterRoutineTargetsOrDisk` usa uma API
de loopback com jobs de rotina, comando anterior, correlação divergente,
check fora do escopo e job duplicado. Somente o job ligado ao comando
é coletado e entregue. Sem job elegível, o resultado do comando indica falha.

`AICEBERG_MONITORING_WEB_ROOT=<checkout Web> go test -race -count=1 ./internal/data/remote -run '^TestAdminSNMPPollingFromWebToUDPAndBack$'`
exercita o fluxo completo em uma fixture de loopback. O Admin gera cada
comando, o cliente do HUB usa TLS e resolve a credencial por referência,
o coletor troca datagramas SNMPv1 reais com um dispositivo sintético e o
Web grava o resultado encaminhado pelo outbox Bolt. Três ciclos com
sete GETs cada confirmam sysUpTime, versão e recibo por comando. Um
ciclo sem resposta comprova o estado de falha. A reentrega mantém o
mesmo recibo e a credencial fica fora do outbox e da observação.
O piloto é revogado depois da entrega de um job para reproduzir a
recusa da credencial. O Admin recebe `snmp_credential_unavailable`
como falha de configuração, sem novo datagrama e com o estado de
disponibilidade do check preservado.

O ensaio força falha na gravação do recibo Web e confirma o rollback.
`TestCollectCommandRetriesQueuedObservationWithoutCollectingAgain`
passa pelo comando manual e pela descarga normal do outbox. Confere
erro transitório identificado, uma única coleta e duas tentativas de
entrega da mesma observação.

`go test -race ./internal/domain/usecase -run 'Agentless|CollectCommand'`
cobre concorrência, serialização entre aliases do destino, alternância de
clientes e retomada quando o destino é liberado. Em loopback, valida o
limite enviado com pedido de lease, a folga do outbox com cache antigo e a
espera cancelada de comando manual sem pedir lease. A coleta HTTP
cancelada ou interrompida pelo deadline do lote preserva a ausência de
observação terminal de falha. Os testes do adapter cobrem execução já
cancelada ou vencida antes do início, retorno tardio após o orçamento e
descarte apenas das parciais emitidas depois do cancelamento.

`TestAgentlessBatchReservesSlotsForAllPossibleSNMPParts` cobre a reserva
do final e duas parciais, o avanço de coletas que cabem na fila e a
execução isolada de uma coleta maior quando o outbox está vazio.
O ensaio Net-SNMP compara o custo reservado com as entregas efetivas.

A mesma suíte cobre cadência entre lotes, cancelamento na fila antes de
iniciar o probe, abertura após falhas de rede, uma tentativa após a pausa,
recuperação, diagnóstico manual separado e descarte de estado.
O ensaio HTTP em loopback confirma que uma porta indisponível abre seu
circuito e as coletas válidas em outra porta do mesmo IP continuam.


### Discovery de discos Linux

O pacote `internal/platform/collectors/diskretirement` cobre sinal de
remoção de bloco, identidade de I/O para disco ou partição e ausência
confirmada no sysfs. `TestDiskWatcherReportsKernelRemovalOfOwnedLoopDevice`
aguarda a inscrição no netlink, remove um dispositivo de bloco virtual
criado para o teste e confere a identidade, o instante e a ausência no sysfs.
A primeira publicação falha de propósito e o Agente tenta novamente com
o mesmo tombstone. O ensaio usa um contêiner Linux isolado sem rede, acesso
ao dispositivo de controle loop e `AICEBERG_TEST_DISPOSABLE_LOOP_DEVICE=1`.
Discos físicos e partições permanecem pendentes de validação em campo.

Os testes Linux de hwmon verificam que o coletor aceita a próxima retirada
após falha do outbox e reenvia o mesmo evento que ficou pendente. Um
inventário indisponível adia o envio. Reaparecimento da identidade em
outro dispositivo ou vencimento cancela o reenvio.

`TestDiscoveryRetirementsSurviveRestartAndLostAcknowledgment`
enfileira retiradas de interface, disco, sensor e serviço pelo bootstrap, reabre
o Bolt e usa o transporte HTTP com credenciais atualizadas. Uma resposta
de erro simula a perda do ACK após o recebimento. A tentativa seguinte
leva os mesmos envelopes, e o ACK de duplicidade esvazia a fila.

Os testes de `serviceretirement` cobrem enumeração indisponível,
serviço ainda instalado, acesso negado, exclusão confirmada e retorno
antes do reenvio. O teste de outbox preserva o fingerprint e o instante
originais. A compilação Windows amd64 cobre a ligação ao SCM. O ensaio de
exclusão usando o SCM de um host Windows continua pendente.
O pacote local `discovery-windows-amd64-20261124T232612Z.zip` reúne os
testes nativos compilados de aplicativo no Registro, serviço no SCM e
consulta ao Event Log. O runner confere SHA-256, exige PowerShell elevado
e exige a linha `PASS` de cada teste. Foram validados formato PE AMD64,
integridade do ZIP e sintaxe do runner no Mac. A execução em um Windows
de laboratório autorizado segue pendente.
As fixtures do systemd cobrem nomes que diferem por caixa, serviços
parados ou mascarados, unidades geradas e respostas incompletas. Apenas
a ausência confirmada de uma unidade instalada produz a retirada.
`TestNativeSystemdServiceRemovalAndReturn` passou em Linux arm64 com
systemd 255 na VM Colima existente. O coletor real observou uma unit
inativa instalada e emitiu a retirada após o gerenciador confirmar a
remoção. O reenvio após falha simulada do outbox manteve o evento original.
A restauração recuperou a mesma identidade. A fixture e o binário foram
removidos e a VM retornou ao estado desligado.

### Retirada de aplicações Debian

`TestDebianRetirementUsesDPKGInDisposableRoot` instala, remove, reinstala e
expurga um pacote de fixture, usando uma raiz dpkg em `/tmp` dentro do
contêiner já disponível `php:8.3-cli-bookworm`. O container é descartável,
sem rede e com o sistema raiz somente leitura. O teste confirma o mesmo
fingerprint no remove e no purge e exige duas capturas depois do purge.
As fixtures cobrem atualização intermediária, arquitetura qualificada,
saída incompleta e falha de outbox. A integração de bootstrap inclui o
evento de aplicação no replay HTTP após reabrir o Bolt.

`TestRPMRetirementUsesNativeDatabaseInDisposableRoot` usa o RPM real do
contêiner local `mysql:8.4.11`. Remove só a entrada de um pacote na cópia
temporária do banco RPM e comprova que duas capturas completas geram o
evento com a arquitetura exata. As fixtures com duas versões e arquiteturas
confirmam que a versão ou arquitetura restante continua no inventário.

### Retirada de aplicações Windows

As fixtures `TestRegistryApplication*` cobrem chave movida para outra
visão, nomes ambíguos, renomeação na mesma chave, título indisponível e
chave duplicada na captura. A integração de bootstrap inclui o replay do
evento `registry` via outbox. O fluxo de reinstalação é exercitado com
`inventory.windows_apps` na fixture MySQL do Runtime. A validação nativa
de desinstalação e reinstalação em um host Windows continua pendente.
O ensaio opt-in `TestNativeRegistryApplicationRemovalAndReturn` está
compilado para Windows x64. Ele usa uma chave nova e exclusiva no
Registro local, lê as visões pelo coletor real e verifica retirada e
retorno do mesmo fingerprint. Falta executar o ensaio em um Windows
de teste autorizado.
A fixture nativa `TestNativeWindowsServiceRemovalAndReturn` cria um
serviço temporário desabilitado, verifica a captura pelo SCM, sua
ausência após exclusão e a mesma identidade após recriação. Também
foi compilada para Windows x64 e aguarda execução em host autorizado.

### Retirada de arquivos de log

`TestFileRetirement*` usa arquivos e diretórios reais em raiz temporária
para conferir rotação, retorno, troca do diretório e desativação do
Discovery. A fixture de outbox confirma reenvio com o mesmo evento quando
a primeira gravação falha. O replay HTTP do bootstrap inclui a origem
`log_sources`. Web e Runtime exercitam reconciliação por fingerprint
único; o mesmo evento reenviado após retorno não reabre a retirada.

### Fontes de logs do systemd e journald

`TestSystemdLogSourceWaitsForUnitAndPathToBeAbsent` remove o caminho
temporário de uma unit de fixture e comprova que a resposta do
gerenciador precisa confirmar a ausência. A fixture cobre serviço
presente, permissão negada e duas confirmações separadas. O teste do
journald usa um socket Unix real e comprova que o socket ausente sozinho
preserva a fonte. No VMAIPROD2, a consulta somente leitura verificou
`loaded/active` para systemd-journald e `not-found/inactive` para um
nome de fixture inexistente. Em 24/11/2026,
`TestNativeSystemdLogSourceRemovalAndReturn` passou na VM Colima
existente, Linux arm64 com systemd 255. O coletor real descobriu uma
unit instalada e inativa em `/run/systemd/system`, emitiu um evento
após remoção e duas confirmações e recuperou o fingerprint original
após a restauração. A fixture avançou o relógio do tracker para
exercitar a carência de dois minutos. A unit e o binário de teste
foram removidos, a VM voltou a ficar desligada e o contexto Docker
Desktop continuou ativo. O ensaio exercita o coletor e a emissão
local da retirada sem qualificar a operação em produção.

### Canais do Windows Event Log

`TestEventChannel*` cobre descoberta de canais padrão e configurados,
identidade estável por nome, acesso negado, canal registrado fora da
captura e ausência sustentada. `TestNativeEventLogChannelProbe` consulta
System e um canal de fixture inexistente em Windows. Sua compilação foi
verificada para Windows x64. A execução em host Windows e a remoção e
volta de um canal real seguem pendentes.

### Consulta SNMPv1 delegada

`TestRelayPilotSNMPv1AgainstNetSNMP` executa três consultas recorrentes
independentes contra um processo Net-SNMP local. Confere `sysName`, a
versão aplicada e o relatório de execução e exporta as capturas para
a fixture descartável do Runtime testar a cadência pela ingestão.

`TestRelayAssignmentVerifiesJobAndRetriesDeliveryWithoutRepeatingSNMP`
executa o worker e o coletor oficiais do relay contra um sensor sintético
em UDP loopback. A fixture confere GET SNMPv1, OID, credencial restrita ao
job, valor recebido e a confirmação após reabrir o Bolt. O primeiro
envio ocorre no mesmo ciclo e lê o resultado já gravado. Uma falha de
entrega não repete a consulta nem divulga a comunidade na observação.
O transporte do HUB é representado por um cliente de teste nesse cenário.

`TestSNMPSwitchportWirePreservesPrintableBitmapsAndCollectionCompleteness`
passa pelo coletor oficial e verifica que bytes aparentemente textuais
mantêm o bitmap original. Uma tabela truncada fica marcada como
incompleta no resultado.
`TestSNMPSwitchportWireOverUDP` executa o coletor com GETNEXT SNMPv1
em loopback e valida os três payloads usados no ensaio do Runtime.
O modo `relay-switchport-wire` encaminha esses payloads à fixture da
API de atribuição para conferir a projeção e a avaliação.

`TestHubAssignmentProxyKeepsResultAndReceiptAcrossTLS` percorre relay →
HUB → API de teste com TLS validado nos dois trechos. Confere a identidade,
o corpo assinado e a preservação de indisponibilidade, rejeição por
autoridade e ACK. O HUB recusa requisições HTTP e parâmetros na query
antes de encaminhar o resultado. As rotas ficam ausentes sem o opt-in.

## Contabilidade da outbox —25/11/2026

`go test -race ./internal/data/local/outbox -count=1` PASS; totais comparados à varredura real após mutações/capacidade recusada/ACK duplicado/retenção,64escritas Batch concorrentes e reabertura após escritor legado. `go test ./internal/data/local/outbox -run '^$' -bench BenchmarkBoltAccountingFullQueue -benchtime=300ms -count=1`:12mil itens/12MiB,consulta anterior103644ns/35832B/4416alocações versus239ns/520B/10alocações. Benchmark macOS local, sem promessa de ganho produtivo equivalente. Gate completo, pacote assinado, piloto HUB e confirmação do relay585 separados.

## Agrupamento autenticado de relay —25/11/2026

`AICEBERG_WEB_ROOT=<checkoutWeb> go test -race ./internal/common/config ./internal/domain/usecase ./internal/bootstrap -run 'TestIdentityBatch|TestVerifiedRelay|TestFlushOutbox|TestHubOutbox' -count=1` PASS. Validador PHP real aceita assinatura do signerGo comUnicode e recusa sujeito alterado; nenhum banco/rede/escrita no oráculo. Casos:nonce/horário variável do mesmo sujeito;cliente/agente/instalação/host/rota separados;assinatura/credencial inválidas,claim vencido/futuro/schema desconhecido,limites,503 semACK/retry,ordem e headers duráveis preservados. Nonce novo não contorna cooldown401. Fixture100mensagens/100claims faz1request por tentativa e100ACKs somente após persistência confirmada. Benchmark800claims/200agentes:800→200grupos;agrupamento~0,125→3,92ms/~0,34→3,47MB alocados por ciclo, com concorrência e lote inalterados. Ganho instalado deve ser medido separadamente.

### Contrato legado de identidade —26/11/2026

Gerador oficial semIDs comhost/instalação e somentehost aceito por PHP real e agrupadorGo;host diferente permanece separado. Ausência difere de zero,sem sujeito é conservador;alteração do sujeito rejeitada pelo PHP. Snapshot copia contagens para impedir mutação externa. Testes race focados incluídos no gate normal.
