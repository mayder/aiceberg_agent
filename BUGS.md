# BUGS.md

Registro oficial de bugs conhecidos, reprodução, impacto, correção e reteste.

## Regras

- Todo bug precisa ter severidade, área, passos de reprodução, esperado, observado e critério de fechamento.
- Bug corrigido só fecha após reteste seguindo `TESTES.md`.
- Bugs críticos ou altos bloqueiam release da frente afetada.
- Bugs de UI precisam de evidência visual quando tecnicamente possível.
- Não apagar bugs antigos; atualizar status e histórico.
- Causa provável deve separar hipótese principal de alternativas.

## Severidades

- Crítica: impede login, operação principal, pagamento, entrega, publicação ou expõe dados.
- Alta: quebra fluxo importante, causa interpretação operacional errada ou fere segurança.
- Média: degrada experiência sem bloquear fluxo principal.
- Baixa: ajuste visual, texto ou comportamento secundário.

## Diagnóstico obrigatório

1. Causa mais provável.
2. Uma ou duas hipóteses alternativas.
3. Menor correção possível.
4. Como validar.

## Validação por risco

### Bug simples

- Correção local e pequena.
- Reteste focado no passo de reprodução.
- Teste automatizado só é obrigatório se já existir cobertura próxima ou se a correção alterar regra.
- Não exige suíte completa.

### Bug complexo

- Toca regra, contrato, banco, permissão, segurança, integração, concorrência, sincronização ou vários módulos.
- Exige teste automatizado novo ou ajustado quando tecnicamente viável.
- Exige `./check.sh`.
- Exige validação completa do fluxo afetado e regressões próximas.

## Organização

## [BUG-20260807-03] Envelope individual de logs permanecia acima do limite da API

Status: corrigido e publicado; rollout SADA pendente de autorização
Severidade: Alta
Área: Forwarder/outbox do agente
Módulo: `internal/domain/usecase`, `internal/data/local/outbox`
Data: 2026-08-07

Reprodução:
- Coletar um lote de `oslogs` com muitos eventos próximos ao limite individual de 256 KiB.
- Persistir o lote como um único envelope em `/v1/logs/raw`.
- Executar o flush com limite de requisição de 8 MiB.

Esperado:
- O agente divide o envelope em partes menores, preserva todos os eventos e metadados e envia cada requisição abaixo de 8 MiB.
- A substituição do envelope original é atômica e recuperável após restart.

Observado:
- A versão `0.8.45` evitava enviar o envelope excessivo, mas o mantinha indefinidamente na outbox.
- Produção continuava recebendo HTTP 400/413 de agentes antigos com payloads de aproximadamente 28 a 43 MB.

Causa provável:
- O limite por bytes atuava somente entre envelopes; `oslogs` empacotava até 100 eventos em um único envelope.

Correção:
- `FlushOutbox` divide `body.events` de `/v1/logs/raw` em envelopes determinísticos de até 8 MiB.
- A outbox Bolt substitui o original pelas partes em uma única transação, sem ACK ou descarte intermediário.
- Metadados do corpo, endpoint, identidade e autorização são preservados.
- Evento individual que não cabe no limite ou corpo sem contrato `events` continua retido e diagnosticado.
- Versão do agente elevada para `0.8.46`.

Critério de fechamento:
- Testes cobrem divisão, ordem, metadados, limite serializado, IDs determinísticos e substituição Bolt atômica/durável.
- `./check.sh` passa.
- Artefatos `0.8.46` são publicados e um piloto SADA autorizado drena a fila sem novos HTTP 400/413 ou perda de eventos.

Publicação:
- Commit do agente `b603454`, integrado e enviado para `main`.
- Cinco artefatos `0.8.46` publicados em `downloads.aiceberg.com.br` e `update.aiceberg.com.br`, com SHA-256 e HTTP Range validados.
- Rollout permanece bloqueado até autorização explícita para o cliente SADA.

## [BUG-20260807-02] Preflight bloqueava launcher Linux autorizado

Status: corrigido em código; publicação e rollout pendentes
Severidade: Alta
Área: Auto-update do agente
Módulo: `internal/domain/usecase`
Data: 2026-08-07

Reprodução:
- Executar o agente como usuário dedicado sem escrita em `/usr/local/bin`.
- Configurar o launcher oficial `sudo -n /usr/local/sbin/aiceberg-agent-update-launcher.sh`, autorizado explicitamente no sudoers.
- Agendar atualização remota.

Esperado:
- O usuário do serviço grava apenas no staging; a troca do binário é delegada ao launcher autorizado e executada com privilégio mínimo.

Observado:
- O preflight exigia escrita direta do usuário do agente no diretório do binário e encerrava com `install_dir_not_writable` antes de chamar o launcher.

Causa provável:
- A validação não distinguia instalação direta de instalação delegada a um comando não interativo já autorizado no sudoers.

Correção:
- Quando a escrita direta falha, o preflight aceita somente launcher absoluto e executável iniciado por `sudo -n` cuja autorização é confirmada por `sudo -n -l`.
- Comando comum, caminho relativo, launcher inexistente ou sudo sem autorização continuam bloqueados.
- Versão do agente elevada para `0.8.45`.

Critério de fechamento:
- Testes cobrem bloqueio sem privilégio e aceite estrito do launcher autorizado.
- `./check.sh` passa.
- Artefatos `0.8.45` são publicados e o agente 73 confirma download, apply, restart e `version_confirmed` sem ampliar permissões do diretório de instalação.

## [BUG-20260807-01] Lote da outbox podia ultrapassar o limite da API

Status: corrigido em código; publicação e rollout pendentes
Severidade: Alta
Área: Forwarder/outbox do agente
Módulo: `internal/domain/usecase`
Pacote relacionado: PKG-33
Data: 2026-08-07

Reprodução:
- Acumular envelopes grandes de métricas ou logs na outbox.
- O flush agrupava até 50 envelopes apenas por identidade, autorização e endpoint.
- O JSON agregado podia ultrapassar os 10 MB aceitos pela API e entrar em retry HTTP 400.

Esperado:
- Cada requisição deve permanecer abaixo do limite da API sem perder envelopes.
- ACK deve ocorrer por sublote efetivamente persistido.

Observado:
- Produção registrou dois payloads de aproximadamente 24 MB e milhares de retries de lotes incompletos durante a saturação da API.

Causa provável:
- `FlushOutbox` limitava somente quantidade de envelopes, sem considerar os bytes do JSON serializado.

Correção:
- O flush divide cada grupo em sublotes de no máximo 8 MiB antes do transporte.
- ACK, retry, backoff, configuração recebida e validação da resposta continuam independentes por sublote.
- Envelope individual acima do limite é retido e diagnosticado sem ser enviado nem descartado.
- Versão do agente elevada para `0.8.44`.

Critério de fechamento:
- Testes cobrem divisão por bytes, ACK dos sublotes e retenção de envelope individual excessivo.
- `./check.sh` passa.
- Artefatos `0.8.44` são publicados e um piloto real confirma drenagem sem novos HTTP 400/413.

## [BUG-20260715-01] Identidade do ingest expirava em agente long-running

Status: corrigido
Severidade: Crítica
Área: Ingestão/outbox do agente
Módulo: `internal/bootstrap`, `internal/domain/usecase`
Pacote relacionado: mapa de rede e ingestão resiliente
Tela relacionada: Mapa de rede
Data: 2026-07-15

Reprodução:
- Agente fica rodando por mais de 7 dias sem restart.
- O backend exige identidade confiável e valida `issued_at` da claim `X-Agent-Identity`.
- Auto-coleta de mapa de rede dispara `network_capture` e o agente registra coleta concluída.
- No flush, o backend responde HTTP 200 com `received=0`, `skipped=1` e `errors_by_reason.identity_rejected`.

Esperado:
- Identidade operacional usada em ingest deve ser renovada pelo próprio agente antes de expirar.
- Envelopes antigos retidos na outbox devem ser reenviados com credencial/identidade atual quando pertencem ao agente local.
- Envelopes encaminhados por HUB devem preservar a credencial e identidade do agente de origem.

Observado:
- `CollectAndBuffer` criava envelopes com `identityHeader` calculado uma vez no start do processo.
- A outbox persistia o header antigo; após expiração da janela de 7 dias, replay e novas coletas podiam continuar sendo rejeitados até restart/update.

Causa provável:
- O bootstrap calculava `cfg.AgentIdentityHeader("")` uma vez e injetava a string fixa nos coletores.
- O flush reaproveitava a identidade persistida no envelope, sem renovar credenciais locais antes do envio.

Hipóteses alternativas:
- Relógio local com desvio maior que a janela aceita ainda pode gerar claim inválida; isso deve aparecer como problema de clock no runtime.
- Envelopes de outro agente via HUB não podem ter identidade recriada pelo HUB sem quebrar a cadeia de confiança.

Correção:
- Coletores passam a receber um provider de identidade cacheado e renovável, em vez de uma string fixa.
- O flush da outbox renova `Authorization` e `X-Agent-Identity` para envelopes locais antes de agrupar/enviar.
- Envelopes com `meta.via=hub` preservam credenciais originais.
- `update-report` passa a enviar `X-Agent-Identity` atual, evitando 401 quando a política exige identidade também nos reports do auto-update.
- Versão do agente elevada para `0.8.42`.

Critério de fechamento:
- `go test ./internal/domain/usecase -run 'Test(CollectAndBuffer|CachedIdentityHeaderProvider|FlushOutbox)' -count=1 -v` passa.
- `./check.sh` passa.
- Artefatos `0.8.42` são gerados e publicados antes do rollout.
- Validação produtiva: agente com outbox retida por `identity_claim_expired_or_invalid_issued_at` deve voltar a persistir `/v1/ingest/network_capture` sem restart manual.

## [BUG-20260703-01] Update Linux podia instalar em binário diferente do serviço

Status: corrigido
Severidade: Alta
Área: Auto-update do agente
Módulo: `internal/domain/usecase`, `scripts/linux`
Pacote relacionado: PKG-84
Tela relacionada: Detalhe do agente / update remoto
Data: 2026-07-03

Reprodução:
- Host Linux executava o serviço a partir de `/opt/aiceberg/bin/aiceberg_agent`.
- O launcher recebia `AICEBERG_AGENT_BIN=/opt/aiceberg/bin/aiceberg_agent`, mas não recebia `AICEBERG_UPDATE_BIN_DST`.
- O apply Linux usava fallback rígido para `/usr/local/bin/aiceberg_agent`.
- Após restart, o update podia confirmar ou retornar `version_mismatch_after_restart` contra o binário errado.

Esperado:
- O update deve substituir o mesmo binário que o serviço está executando.
- `version_confirmed` só deve ocorrer quando o processo ativo corresponde ao alvo instalado.

Observado:
- O update podia concluir com `bin=/usr/local/bin/aiceberg_agent` mesmo quando o agente ativo era `/opt/aiceberg/bin/aiceberg_agent`.

Causa provável:
- O agente exportava `AICEBERG_AGENT_BIN`, mas não exportava `AICEBERG_UPDATE_BIN_DST`.
- O launcher/apply Linux priorizava o destino default em vez do binário em execução.

Hipóteses alternativas:
- Instalações antigas em `/usr/local/bin/aiceberg_agent` devem continuar funcionando pelo fallback legado.

Correção:
- O agente passa `AICEBERG_UPDATE_BIN_DST` igual ao binário em execução.
- O launcher Linux resolve destino por prioridade: `AICEBERG_UPDATE_BIN_DST`, `AICEBERG_AGENT_BIN`, `/usr/local/bin/aiceberg_agent`.
- O apply Linux usa o mesmo fallback.
- Versão do agente elevada para `0.8.40`.

Critério de fechamento:
- `go test ./internal/domain/usecase -run 'TestSelfUpdate_(RunCommand|ReportPendingResultMarksVersionMismatchAfterRollback|ReportPendingResultConfirmsVersionAfterReconnect|ApplyFailureReportsExitCodeCooldownAndClearsPending|SnapshotIncludesPendingStateMetadata)' -count=1 -v` passa.
- `bash -n scripts/linux/aiceberg-agent-update-launcher.sh scripts/linux/aiceberg-agent-apply-update.sh` passa.
- `./check.sh` passa.
- Artefatos `0.8.40` são gerados por `./scripts/build_installers.sh` e publicados no web.

## [BUG-20260702-01] Flush podia ACKar ingestão não persistida

Status: corrigido
Severidade: Alta
Área: Forwarder/outbox do agente
Módulo: `internal/domain/usecase`
Pacote relacionado: mapa de rede e ingestão resiliente
Tela relacionada: Mapa de rede
Data: 2026-07-02

Reprodução:
- Agente Linux executava coleta sob demanda `network_capture`.
- Backend respondia HTTP 200 com `received=0`, `skipped=1` e motivo não idempotente, como `persist_failed`.
- O agente registrava `flushed ... acked=...` e removia o envelope da outbox.

Esperado:
- HTTP 200 só deve remover o lote da outbox quando o corpo confirmar persistência ou skip seguro/idempotente.
- Falha de persistência deve manter o envelope para retry e diagnóstico.

Observado:
- O flush considerava qualquer HTTP 2xx como ACK definitivo, mesmo sem snapshot persistido no backend.

Causa provável:
- O contrato do `FlushOutbox` validava apenas status HTTP, sem interpretar `received`, `skipped` e `errors_by_reason`.

Hipóteses alternativas:
- Duplicidade real por `envelope_id` deve continuar sendo ACK seguro para não reter lote já aceito.

Correção:
- O flush passa a interpretar o corpo de resposta da ingestão.
- `duplicate_envelope_id` e `invalid_envelope` continuam ACK seguros.
- `persist_failed`, falhas de autenticação/identidade e demais skips não idempotentes retêm o lote e registram erro operacional.
- Versão do agente elevada para `0.8.39`.

Critério de fechamento:
- `go test ./internal/domain/usecase -run 'TestFlushOutbox_(RetainsBatchWhenIngestDidNotPersist|AcksDuplicateEnvelopeSkip)' -count=1 -v` passa.
- `./check.sh` passa.
- Artefatos `0.8.39` são gerados por `./scripts/build_installers.sh` e publicados no web.

## [BUG-20260701-01] Amostra de CPU Windows podia reportar 100% falso

Status: corrigido
Severidade: Alta
Área: Métricas do agente
Módulo: `internal/platform/collectors/sysmetrics`
Pacote relacionado: observabilidade operacional do agente
Tela relacionada: Detalhe do agente / alertas de threshold
Data: 2026-07-01

Reprodução:
- Agente Windows `0.8.37` coletando `sysmetrics`.
- Backend recebia `cpu.percent_total=100` enquanto `percent_per_cpu` vinha baixo ou em padrão binário `0/100`.
- Windows Task Manager/Resource Monitor mostravam uso real próximo de 1%.

Esperado:
- A CPU total deve representar a média real da janela de coleta.
- Amostra incoerente deve ser omitida para não abrir alerta falso.

Observado:
- `cpu.PercentWithContext(ctx, 0, false)` seguido de `cpu.PercentWithContext(ctx, 0, true)` gerava amostra instantânea incoerente no Windows.

Causa provável:
- Uso de intervalo `0` no gopsutil em duas leituras sequenciais de CPU no Windows.

Hipóteses alternativas:
- Saturação real de poucos núcleos deve continuar visível, mas não pode virar CPU total 100% sem média coerente.

Correção:
- Coleta de CPU passa a usar janela curta de amostragem.
- CPU total passa a ser derivada da média por núcleo quando a amostra por núcleo é coerente.
- Padrão binário `0/100` no Windows é descartado na amostra atual.
- Versão do agente elevada para `0.8.38`.

Critério de fechamento:
- `go test ./internal/platform/collectors/sysmetrics` passa.
- `./check.sh` passa.
- Artefatos `0.8.38` são gerados por `./scripts/build_installers.sh` e publicados no web.

## [BUG-20260624-02] Falhas recorrentes do worker eram reabertas indefinidamente

Status: corrigido
Severidade: Alta
Área: Observabilidade do agente
Módulo: `internal/bootstrap/app.go`
Pacote relacionado: observabilidade operacional
Tela relacionada: Detalhe do agente
Data: 2026-06-24

Reprodução:
- Agente com falha recorrente em oslogs, containers, kubernetes, flush, ping, config ou self-healing.
- A cada janela de coleta, o agente reportava `recovery_status=open` novamente.
- A web acumulava milhares de ocorrências abertas.

Esperado:
- Fingerprint estável por tipo/modo/contexto operacional.
- Falha repetida deve ter throttle maior.
- Sucesso posterior deve reportar `recovered` para fechar o estado aberto.

Observado:
- Fingerprint incluía texto do erro, que pode variar.
- Throttle era curto.
- Ciclo com sucesso não emitia recuperação para os tipos recorrentes.

Causa provável:
- O reportador local registrava falhas, mas não modelava estado de recuperação.

Hipóteses alternativas:
- Falhas reais de backend ou permissão devem continuar aparecendo, mas sem multiplicar indefinidamente o estado aberto.

Correção:
- Fingerprint usa `error_type`, modo e contexto estável (`route`, `source`, `name`, `command_code`).
- Repetição do mesmo estado fica limitada a 30 minutos para `open` e 5 minutos para `recovered`.
- Ciclos bem-sucedidos de containers, Kubernetes, flush, ping, config, oslogs e self-healing reportam `recovered`.

Critério de fechamento:
- `go test ./internal/bootstrap ./internal/data/remote` passa.
- O agente não infla indefinidamente o contador de ocorrências abertas para a mesma falha recorrente.

## [BUG-20260620-01] Coleta vazia de oslogs era tratada como erro e podia virar log bruto

Status: corrigido
Severidade: Alta
Área: Coleta de logs
Módulo: `internal/platform/collectors/oslogs`
Pacote relacionado: PKG-60, PKG-73, PKG-74
Tela relacionada: Detalhe de log no web
Data: 2026-06-20

Reprodução:
- Habilitar `OSLOG_DIAG=true` em agente Windows com `OSLOG_WIN_CHANNELS=System,Application,Security`.
- Rodar coleta quando não há evento novo após o cursor.
- O agente registrava `collect failed collector=oslogs err=oslogs: nenhum evento lido...`; em alguns ambientes essa mensagem podia ser capturada e enviada como log bruto.

Esperado:
- Ausência de evento novo deve ser coleta vazia, sem erro e sem persistir log para IA.

Observado:
- O coletor retornava erro diagnóstico mesmo sem falha real de canal/permissão.

Causa provável:
- `oslogs` adicionava `nenhum evento lido` em `c.errors` com diagnóstico ativo e retornava `formatDiagError` quando não havia eventos.

Hipóteses alternativas:
- Falha real de `wevtutil`, permissão ou arquivo inexistente deve continuar sendo erro diagnóstico.
- Backend pode receber o padrão de agentes antigos até todos atualizarem.

Correção:
- Windows: remover `nenhum evento lido do canal` da lista de erros e retornar `nil` quando não houver evento novo nem falha real.
- Linux/POSIX: retornar `nil` quando não houver evento novo mesmo com diagnóstico ativo; manter erro para arquivo ausente/permissão/falha real.
- Web: filtrar defensivamente `collect failed|collect empty collector=oslogs` em `/v1/logs/raw`.

Critério de fechamento:
- `go test ./internal/platform/collectors/oslogs` passa.
- `php -l api/modules/v1/controllers/LogsController.php` passa no web.
- `./check.sh` passa nos dois repositórios.
- Agente atualizado não gera novo log bruto apenas por ausência de eventos.

## [BUG-20260808-01] Política de update exigia assinatura sem chave pública provisionada

Status: corrigido em código; validação positiva final do piloto em `0.8.49`
Severidade: Alta
Área: auto-update e cadeia de confiança
Módulos: `cmd/update-signer`, `internal/common/updatetrust`, instaladores Linux/Windows
Data: 2026-08-08

Resultado observado:

- o agente rejeitava o pacote com `validation_failed/trust_chain` e `artifact trust public key missing`;
- a publicação gerava SHA256, mas não produzia manifesto Ed25519 consumível pelo backend;
- instaladores existentes não provisionavam a chave pública oficial.

Correção aplicada:

- chave Ed25519 oficial criada fora do Git, com chave privada em modo `0600`;
- signer reproduzível gera e verifica `UPDATE_SIGNATURES.json` para todos os artefatos;
- build de release falha quando assinatura é obrigatória e a chave privada não foi informada;
- instaladores provisionam `AUTO_UPDATE_TRUST_REQUIRED=true` e a chave pública oficial;
- versão atualizada para `0.8.48`;
- chave pública oficial e exigência de assinatura definidas também como defaults compilados, cobrindo instalações legadas sem `agent.env`.

Validação/reteste:

- assinatura válida é aceita;
- alteração do artefato após assinatura é rejeitada;
- chave privada existente não é sobrescrita pelo comando de geração;
- SHA256 e assinaturas dos cinco pacotes oficiais são verificados antes da publicação.
- configuração sem variáveis locais continua exigindo assinatura e usa a chave pública oficial compilada.

Resultado do primeiro piloto:

- a ponte `0.8.47` foi encerrada sem alterar o binário do agente 2;
- a política obrigatória foi restaurada imediatamente;
- a evidência de runtime confirmou que o Windows legado não possui `agent.env`, motivando o default compilado de `0.8.48`.
- em `0.8.48`, o agente 2 rejeitou pacote cuja assinatura pertencia a outra versão com `validation_failed/trust_chain`, sem trocar o binário;
- `0.8.49` foi reservado como próximo alvo assinado para validar o caminho positivo entre versões distintas.

Rollback: publicar a versão anterior e restaurar o arquivo de ambiente do serviço. A chave pública pode permanecer instalada; ela não é secreta e não concede capacidade de assinatura.

## Modelo para novos registros

```txt
## [BUG-001] Título curto

Status:
Severidade:
Área:
Módulo:
Pacote relacionado:
Tela relacionada:
Data:

Objetivo:

Passos para reproduzir:

1.
2.
3.

Resultado esperado:

Resultado observado:

Impacto:

Causa mais provável:

Hipóteses alternativas:

Menor correção possível:

Correção aplicada:

Validação/reteste:

Evidência:

Critério de fechamento:

Validação executada:
```

## Adaptacao deste projeto

- Projeto: `aiceberg_agent`
- Tipo: Agente Go
- Stack detectada: Go agent/CLI, gopsutil, NTP, SNMP
- Regra: adaptar comandos, camadas, testes, fixtures e limites conforme a realidade deste modulo Go.
## [BUG-20260728-01] Access log WordPress era descartado antes da correlação SOC

Status: corrigido em código; implantação pendente
Severidade: P1
Área: coleta de logs Linux
Data: 2026-07-28

Resultado observado:

- o agente 0.8.42 estava ativo durante o incidente do InspectApp;
- o access log Nginx não estava configurado e o usuário do agente não possuía leitura via `adm`;
- mesmo configurada, uma linha de access log sem nível reconhecido era descartada pela política `error+`;
- linhas IP-prefixed também não eram reconhecidas como novo evento multiline.

Correção aplicada:

- parser específico para sinais de segurança WordPress em Nginx combined access;
- timestamp da fonte convertido para UTC e campos HTTP/IP/ação estruturados;
- valores de query removidos antes do envio;
- eventos relevantes recebem severidade suficiente para a política `error+`;
- requisições WordPress comuns continuam descartadas localmente.

Validação/reteste:

- `go test ./internal/platform/collectors/oslogs`;
- fixture positiva com batch, login inferido, upload e tentativa de webshell;
- fixture negativa com requisição WordPress comum e batch isolada sem veredito local.

Rollback: instalar o binário 0.8.42 e retirar o access log da configuração remota.
