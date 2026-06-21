# Demandas AIceberg Agent

Backlog do agente desktop/serviço. Este arquivo complementa o backlog do `aiceberg_web` quando a entrega exigir mudanças coordenadas nos dois repositórios.

---

## Organização por pacotes entregáveis

- `PKG-32` Canal operacional bidirecional com agentes
- `PKG-33` Flush resiliente e backpressure para HUB/Relay
- `PKG-73` Agente/SOC — taxonomia canônica de origem, campos SOC e roteamento seguro para PKG-54
- `PKG-74` Agente/Web — descoberta automática de fontes de logs, aplicações e dependências locais
- `PKG-75` Logs/IA — sinais do agente para triagem recorrente e deduplicação segura

---

## [PKG-32] Canal operacional bidirecional com agentes

**Problema a resolver** — o agente depende demais de polling HTTP para comandos, atualização, coleta sob demanda e confirmação de estado. Isso aumenta latência, gera filas grandes e dificulta diagnóstico de falhas como update, sudoers, restart e reconexão.

**Escopo no `aiceberg_agent`** — implementar o lado do agente para manter um canal operacional persistente ou quase persistente com o servidor, preservando o fluxo atual como fallback.

**Dependências** — `aiceberg_web` PKG-32, PKG-19 (self-update, self-healing, modo remoto), PKG-20 (envio incremental), configuração segura do serviço por SO.

### Lotes propostos

1) **Contrato local do canal**
   - [x] definir envelope canônico para mensagens de sessão, comando, ACK, progresso, resultado, erro e heartbeat;
   - [x] incluir `agent_id`, hostname, versão, modo, capacidades, `command_id`, `correlation_id`, timestamps e tentativas;
   - [x] garantir topologia por modo: `direct -> AIceberg`, `hub -> AIceberg`, `relay -> hub -> AIceberg`;
   - [x] preservar os contratos HTTP atuais (`ping`, `bootstrap`, `config`, `selfheal`, `update-report`, `ingest`) como compatibilidade e fallback;
   - [x] separar domínio/contrato de transporte para permitir WebSocket no MVP e polling como fallback.

2) **Cliente persistente outbound**
   - [x] criar cliente outbound para o endpoint operacional do modo `direct` (`/v1/agent/channel`, HTTP equivalente ao canal persistente);
   - [x] em modo `relay`, conectar somente ao `hub` configurado, sem exigir conexão direta com AIceberg;
   - [x] em modo `hub`, aceitar conexões dos relays e manter conexão do hub com AIceberg;
   - [x] autenticar com token atual do agente e validar rejeições de credencial;
   - [x] implementar reconnect com backoff, jitter e limite de logs;
   - [x] manter heartbeat e medição de latência.

3) **Fallback para polling atual**
   - [x] manter `/v1/agent/ping`, `/v1/agent/config` e self-healing atuais funcionando;
   - [x] ativar fallback automático quando o canal persistente cair;
   - [x] manter compatibilidade com relays que ainda operem apenas pelo fluxo atual durante rollout;
   - [x] evitar execução duplicada do mesmo comando entre canal novo e polling por idempotência local.

4) **Executor de comandos com progresso**
   - [x] padronizar ACK imediato ao receber comando;
   - [x] enviar eventos de progresso para comandos longos;
   - [x] retornar resultado final com evidência sanitizada;
   - [x] aplicar timeout, cancelamento cooperativo e retry controlado.

5) **Coleta sob demanda e streaming**
   - [x] permitir coletas sob demanda sem bloquear o loop principal;
   - [x] enviar payloads grandes em chunks com checksum/ordem;
   - [x] reportar progresso de inventário, health, network capture e agentless;
   - [x] respeitar quotas de CPU, memória, disco e rede.

6) **Update remoto com handshake**
   - [x] expor etapas de update: precheck, download, validação, apply, restart agendado, reconexão e confirmação;
   - [x] reportar falhas específicas: permissão/sudoers, pacote inválido, binário ausente, restart falho, versão não confirmada;
   - [x] confirmar versão nova após restart sem depender apenas de bootstrap manual;
   - [x] manter rollback local e fallback para o launcher atual.

7) **Segurança local**
   - [x] manter allowlist de comandos suportados;
   - [x] bloquear shell arbitrário;
   - [x] sanitizar logs, paths, tokens e payloads;
   - [x] limitar comandos destrutivos a fluxos com aprovação explícita na web.

8) **Operação e suporte**
   - [x] criar diagnóstico local do canal (`doctor` ou comando remoto equivalente);
   - [x] expor status do canal no health local;
   - [x] documentar instalação, variáveis e troubleshooting;
   - [x] adicionar testes unitários e integração local simulando servidor.

### Critérios de aceite

- [x] agente conecta outbound ao servidor e mantém presença/heartbeat;
- [x] comando simples executa via canal persistente com ACK e resultado;
- [x] queda do canal volta para polling sem perder comando pendente;
- [x] update remoto reporta etapa detalhada e confirma versão após reconexão;
- [x] payload grande é transmitido em chunks sem travar coleta principal;
- [x] `./check.sh` passa antes de qualquer release do agente.

### Fora de escopo inicial

- abrir porta inbound obrigatória no cliente;
- SSH reverso genérico;
- shell remoto arbitrário;
- conexão direta obrigatória de `relay` com AIceberg;
- remoção dos contratos HTTP atuais antes de homologação;
- remover polling atual antes de homologação e rollback.

---

## [PKG-33] Flush resiliente e backpressure para HUB/Relay

**Problema a resolver** — em topologia `relay -> hub -> AIceberg`, o Hub pode acumular backlog quando a API demora para responder a `/v1/ingest/metrics`. No incidente observado em 2026-04-29, o Relay `RHL-CTG-HML-01` coletava métricas e fazia flush para o Hub, mas o Hub registrava `context deadline exceeded`, `queue_items > 1000` e `flush_err` crescente. Como o flush atual trabalha com lote FIFO e só confirma ACK depois de enviar todos os grupos, falhas temporárias de um endpoint podem atrasar métricas recentes e degradar a visibilidade do relay.

**Escopo no `aiceberg_agent`** — tornar o flush do Hub resiliente sob carga, com timeout/batch configuráveis, ACK granular, retry controlado, priorização de telemetria operacional e diagnóstico local suficiente para operação sem desligar coletas essenciais.

**Dependências** — `aiceberg_web` PKG-33, PKG-20 (envio incremental), PKG-32 (canal operacional), publicação oficial de artefatos conforme `GOVERNANCA.md`.

### Evidências do incidente

- Relay em `AGENT_MODE=relay` com `channel.connected=true`, `relay_uses_hub_url=true` e sem conexão direta com AIceberg.
- Relay gerando `collect buffered route=/v1/ingest/metrics` e `flushed` sem erro.
- Hub em `AGENT_MODE=hub` com `queue_items` crescendo e `flush_err` aumentando.
- Erro do Hub: `Post "https://api.aiceberg.com.br/v1/ingest/metrics": context deadline exceeded`.
- Banco sem `metrics` recente do relay afetado, apesar de `bootstrap`, `health`, `network_capture` e `inventory` recentes.

### Lotes propostos

1) **Configuração operacional do flush**
   - [x] adicionar variáveis/env e prefs remotas para timeout HTTP de ingestão (`INGEST_TIMEOUT_SEC` ou equivalente);
   - [x] tornar tamanho de batch da outbox configurável sem recompilar (`OUTBOX_FLUSH_BATCH` ou equivalente);
   - [x] permitir intervalo de flush configurável quando o Hub estiver sob backlog;
   - [x] expor a configuração efetiva no `/health`, no comando `inspect_runtime_config` e no payload de métricas do agente;
   - [x] manter defaults compatíveis com instalações existentes.

2) **ACK granular e isolamento por rota**
   - [x] alterar `FlushOutbox` para confirmar ACK dos grupos enviados com sucesso mesmo se outro grupo falhar;
   - [x] isolar falhas por `authHeader + endpoint`, evitando que timeout em `/v1/ingest/metrics` prenda `health`, `bootstrap`, `inventory` e outros endpoints;
   - [x] preservar envelopes não enviados para retry sem duplicar os já confirmados;
   - [x] registrar logs com `route`, `batch_size`, `acked`, `retained`, `duration_ms` e `err`;
   - [x] cobrir com testes unitários de sucesso parcial e falha por endpoint.

3) **Retry, backoff e descarte seguro**
   - [x] implementar retry com backoff por rota/autorização, evitando loop agressivo quando a API estiver lenta;
   - [x] respeitar respostas de backpressure da API quando disponíveis (`retry-after`, batch sugerido, rota degradada);
   - [x] classificar erro temporário, erro HTTP definitivo e envelope inválido;
   - [x] permitir descarte auditável de envelope irrecuperável sem bloquear a fila inteira;
   - [x] manter segurança: nunca descartar payload por timeout temporário.

4) **Priorização operacional em Hub**
   - [x] priorizar telemetria curta de saúde/fila/canal para manter diagnóstico em tempo real;
   - [x] evitar starvation de métricas recentes quando houver backlog antigo grande;
   - [x] avaliar filas lógicas por endpoint ou leitura balanceada da outbox preservando idempotência;
   - [x] garantir que Relay sem saída direta continue sem conectar ao AIceberg.

5) **Observabilidade local do backlog**
   - [x] expor no `/health` e `/metrics`: último flush com duração, batch confirmado, batch retido, rota com última falha, idade do item mais antigo e contadores por endpoint;
   - [x] incluir diagnóstico do backlog no comando remoto `inspect_runtime_config`;
   - [x] melhorar logs de `transport failed` com `status`, timeout, rota, tamanho do lote e próxima tentativa;
   - [x] documentar comandos de suporte para Hub/Relay.

6) **Homologação HUB/Relay sob carga**
   - [x] criar teste local/simulação com Hub recebendo relays e API lenta em `/v1/ingest/metrics`;
   - [x] validar que `health/bootstrap/inventory` continuam fluindo mesmo com timeout de `metrics`;
   - [x] validar que métricas recentes voltam a drenar após recuperação da API;
   - [ ] [bloqueado] medir crescimento/drenagem da fila em cenário com Agentless ativo;
   - [x] rodar `./check.sh` antes de gerar qualquer artefato.

7) **Publicação e rollout**
   - [x] publicar artefatos oficiais da nova versão antes de acionar update remoto;
   - [ ] [validacao] validar update em um Hub e um Relay de homologação;
   - [x] documentar rollback para versão anterior e parâmetros temporários de mitigação;
   - [x] registrar no web qual versão mínima do agente possui flush resiliente.

### Critérios de aceite

- [x] timeout de `/v1/ingest/metrics` não impede ACK de outros endpoints enviados com sucesso;
- [x] Hub não mantém crescimento indefinido da fila quando a API volta a responder;
- [x] `/health` mostra dados úteis de backlog e última falha sem depender de `journalctl`;
- [x] batch e timeout podem ser ajustados por configuração;
- [x] Relay continua enviando somente ao Hub em modo `relay`;
- [x] testes cobrem falha parcial, retry e ACK granular;
- [x] `./check.sh` passa antes de release.

### Fora de escopo inicial

- desligar coletas como solução definitiva;
- conexão direta obrigatória de `relay` com AIceberg;
- remover outbox persistente;
- trocar o protocolo HTTP de ingestão antes da correção de resiliência;
- alterar banco da Web a partir do repositório do agente.

---

## [PKG-73] Agente/SOC — taxonomia canônica de origem, campos SOC e roteamento seguro para PKG-54

**Status** — implementado e validado em 19/06/2026 como pacote coordenado com `aiceberg_web`.

**Objetivo** — emitir contrato SOC canônico em todos os logs relevantes do agente sem quebrar payload legado, reduzindo inferência frágil no PKG-54.

### Lotes concluídos

1) **Contrato canônico de origem SOC no agente**
   - [x] adicionar `aiceberg_transport`, `aiceberg_tool_origin`, `aiceberg_source_category`, `aiceberg_soc_source_type`, `aiceberg_soc_eligible`, `aiceberg_origin_confidence` e `aiceberg_route_reason`;
   - [x] manter compatibilidade com `transport`, `source_tool`, `source_category`, `level`, `severity`, `attributes` e `/v1/logs/raw`;
   - [x] cobrir `windows_eventlog`, `ad_security`, `linux_syslog`, `linux_auth`, `journald`, `graylog_gelf`, `application`, `container_log`, `kubernetes_log`, `otlp_log` e `unknown`;
   - [x] documentar contrato em `docs/pkg73_soc_log_origin.md`;
   - [x] validar payload legado pelo normalizador web.

2) **Campos SOC mínimos normalizados**
   - [x] promover campos seguros quando existirem: `event_code`, `vendor`, `product`, `src_ip`, `dst_ip`, `src_host`, `dst_host`, `username`, `process_name`, `command_line`, `file_hash`, `domain`, `url`, `action`, `rule_name`, `technique_id` e `alert_id`;
   - [x] manter `attributes` sanitizado como evidência reduzida;
   - [x] impedir promoção de senha, token, cookie, segredo e Authorization;
   - [x] validar app JSON, Graylog/GELF, Windows Security, Linux auth, Docker, Kubernetes e OTLP.

3) **Classificação segura por origem**
   - [x] classificar Windows `Security`/Security-Auditing como `ad_security`/`windows_security`;
   - [x] classificar Sysmon como SOC elegível;
   - [x] manter Windows `System`/`Application`, incluindo DistributedCOM `10028`, como observability/no;
   - [x] classificar Linux auth como `linux_security`;
   - [x] classificar app JSON, OTLP, container e Kubernetes como `conditional` por padrão;
   - [x] preservar Graylog/GELF e ferramenta real quando configurada/inferida;
   - [x] validar DCOM/RPC operacional sem promoção SOC.

4) **Configuração e override controlado**
   - [x] aceitar campos `aiceberg_*` em logs estruturados;
   - [x] aceitar Docker labels `aiceberg.ai/*` e `aiceberg.com/*`;
   - [x] aceitar Kubernetes annotations `aiceberg.ai/*` e `aiceberg.com/*`;
   - [x] manter `logs.processors`/`OSLOG_PROCESSORS_JSON` para remap/route/enrich sob as regras de assinatura/escopo do PKG-68;
   - [x] separar `configured`, `inferred` e `unknown`;
   - [x] divergência/ausência vira lacuna ou `conditional`, não SOC crítico automático.

5) **Consumo web e roteamento para PKG-54**
   - [x] entregue no `aiceberg_web` por `SocAgentLogOriginNormalizerService`;
   - [x] backend adiciona snapshot `AICEBERG_SOC_ORIGIN=` no conteúdo do `log_bruto` sem SQL novo;
   - [x] payload novo e legado são aceitos;
   - [x] PKG-54 pode extrair transporte, origem, categoria, tipo SOC, elegibilidade, confiança, motivo, campos SOC e lacunas.

6) **Evidência, homologação e pacote de release**
   - [x] evidência controlada em `docs/evidence/pkg73/soc-log-origin-20260619T210500Z/evidence.md`;
   - [x] testes focados do agente executados;
   - [x] teste focado web executado;
   - [x] documentação atualizada;
   - [x] fechamento depende de `./check.sh` nos dois repositórios.

### Critérios de aceite

- [x] todo log coberto pelo agente possui transporte, origem, categoria, elegibilidade SOC e confiança;
- [x] Graylog/GELF não mascara ferramenta real quando configurada/inferida;
- [x] Windows EventLog preserva campos essenciais e classifica canais corretamente;
- [x] logs de aplicação/observabilidade não entram como SOC crítico por padrão;
- [x] fontes security conhecidas carregam campos SOC mínimos quando disponíveis;
- [x] campos sensíveis não vazam em top-level SOC;
- [x] PKG-54 consegue consumir o contrato sem inferência frágil;
- [x] agentes/payloads antigos continuam compatíveis.

### Rollback

Desligar logs por `OSLOG_ENABLED=false`, remover overrides `aiceberg.*`, ignorar campos `aiceberg_*` no backend ou publicar versão anterior.

---

## [PKG-74] Agente/Web — descoberta automática de fontes de logs, aplicações e dependências locais

**Status** — implementado em código em 20/06/2026 como entrega coordenada com `aiceberg_web`. Testes focados e `./check.sh` executados; artefatos `0.8.13` gerados e publicados no web. Validação real parcial em produção atualizou seis agentes online do cliente InspectApp para `0.8.13`, consumiu `collect_now=log_source_discovery` e persistiu candidatos reais no web. Bundle controlado `docs/evidence/pkg74/discovery-controlled-20260620T180000Z` cobre troubleshooting de aplicação lenta com web/app/banco/rede, runtime, segurança, Kubernetes/OTLP controlados, redaction e lacuna de permissão. Desktop `72` está desligado e foi removido do bloqueio desta meta por decisão operacional do usuário; IIS/Kubernetes reais ficam como homologação por cliente quando existirem no ambiente alvo.

**Problema a resolver** — o agente já coleta fontes configuradas e emite taxonomia SOC, mas ainda não inventaria automaticamente tudo que pode ajudar Log/NOC/SOC/APM/troubleshooting no host: IIS, Nginx, Apache, Plesk, aplicações, bancos, filas, containers, Kubernetes, EventLog Security, Linux auth, serviços, portas e dependências. Sem isso, a web não consegue listar fontes candidatas para aprovação e a IA recebe contexto incompleto.

**Objetivo** — implementar descoberta local segura, bounded e read-only de fontes de log, aplicações e dependências, reportando candidatos `log_source_discovery_v1` para o web. O agente deve propor candidatos com evidência, confiança, severidade mínima, risco, volume e permissões, mas a coleta ampla depende de configuração aprovada/remota e assinada pela web.

**Reuso obrigatório** — coletores `oslogs`, `journald`, `containers`, `kubernetes`, `otlp`, `localchecks`, `networkcapture`, snapshot/bootstrap, configuração remota assinada do PKG-68, runtime de plugins/checks do PKG-66, integrações do PKG-71, contexto do PKG-72 e contrato SOC do PKG-73. Não criar runtime paralelo nem shell arbitrário.

**Fora do escopo** — varrer disco inteiro, coletar `debug`/`info` por padrão, ler segredos, executar comandos remotos arbitrários, ativar EDR/NDR/SOAR, alterar banco da web, burlar permissões do sistema operacional ou declarar causa raiz sem evidência.

### Lotes propostos

1) **Contrato e domínio de discovery**
   - [x] [exec] criar domínio/contrato `log_source_discovery_v1` com `schema_version`, `agent_id`, `asset_id`, `host`, `os`, `collected_at`, `scan_policy`, `capabilities`, `candidates[]`, `gaps[]` e `redaction_summary`;
   - [x] [exec] cada candidato deve expor `fingerprint`, `kind`, `product`, `service_name`, `process_name`, `port`, `listener`, `path`, `channel`, `unit`, `container`, `pod`, `namespace`, `runtime`, `version`, `confidence`, `evidence`, `recommended_category`, `useful_for`, `soc_source_type`, `soc_eligible`, `origin_confidence`, `min_severity`, `estimated_volume`, `usefulness_score`, `risk_score`, `permissions_required`, `redaction_policy`, `retention_hint`, `freshness`, `status` e `rollback_ref`;
   - [x] [exec] deduplicar candidatos por fingerprint estável;
   - [x] [validacao] validar serialização, compatibilidade de snapshot/bootstrap e payload parcial.

2) **Descoberta Linux**
   - [x] [exec] descobrir systemd units, processos, listeners e paths ligados a `nginx`, `apache/httpd`, `php-fpm`, `tomcat`, `java`, `node`, `python`, `dotnet`, `plesk`, `ssh`, `auth.log/secure`, `journald`, `syslog`, `postgres`, `mysql/mariadb`, `redis`, `rabbitmq`, `mongodb`, filas e aplicações comuns;
   - [x] [exec] mapear paths/canais conhecidos sem scan recursivo amplo, com allowlist de diretórios, limite de candidatos e permissões;
   - [x] [exec] redigir secrets de cmdline antes de evidência ou flare;
   - [x] [validacao] fixtures/testes e hosts reais cobriram Nginx/Apache/Plesk/app/banco/rede; permissão negada coberta por teste controlado `path_permission_denied`.

3) **Descoberta Windows**
   - [x] [exec] descobrir EventLog `System`, `Application`, `Security`, Sysmon quando existir, IIS/W3SVC, Windows Defender, processos .NET/Java/Node/Python e listeners;
   - [x] [exec] preferir EventLog/canal estruturado a arquivo bruto quando possível;
   - [x] [exec] detectar permissão ausente sem falhar o agente;
   - [x] [validacao] cobrir Windows Server real com EventLog Security/Defender/SQL Server e IIS/app log controlados no bundle PKG-74; Windows desktop `72` permanece pendente por estar offline.

4) **Containers, Kubernetes, OTLP e APM**
   - [x] [exec] descobrir Docker/containerd por socket/permissão, log JSON path e portas sem expor env sensível;
   - [x] [exec] descobrir Kubernetes por namespace/pod quando rodando em cluster;
   - [x] [exec] correlacionar OTLP configurado e runtime local quando existirem;
   - [x] [validacao] cobrir Docker/containerd real, Kubernetes/OTLP controlados, permissão negada controlada e rollback transacional de config na web.

5) **Scoring, severidade e proteção de volume**
   - [x] [exec] classificar candidatos como `log`, `noc`, `observability`, `soc` ou `conditional`, com `useful_for` para Log/NOC/SOC/troubleshooting;
   - [x] [exec] aplicar `min_severity=error` por padrão para logs;
   - [x] [exec] descartar localmente evento sem nível quando severidade mínima configurada exigir nível conhecido, salvo override explícito;
   - [x] [exec] estimar volume e impor limite de candidatos/bytes de evidência;
   - [x] [validacao] provar que segredo não vaza em evidência por teste unitário.

6) **Aplicação de configuração aprovada**
   - [x] [exec] receber configuração remota assinada/escopada com fontes aprovadas via payload `logs`;
   - [x] [exec] ativar coleta somente para candidatos aprovados, mantendo configs atuais compatíveis;
   - [x] [exec] registrar versão de config, origem, rollback e status de aplicação pelo fluxo existente;
   - [x] [validacao] aprovar fonte/rejeitar/ignorar e rollback transacional já validados na web; perda de rede, API indisponível, disco cheio/outbox, proxy, payload grande, rollback de update e CPU/mem permanecem cobertos pelas evidências operacionais PKG-69 reutilizadas como base de homologação do agente.

7) **Evidência e fechamento**
   - [x] [validacao] rodar testes focados por coletor e contrato;
   - [x] [validacao] rodar `./check.sh`;
   - [x] [validacao] gerar bundle controlado com Linux, Windows/IIS controlado, Docker/containerd, Kubernetes, OTLP/APM, redaction, permissão negada, app lenta, banco e rede; compatibilidade com agente legado é validada no web por payload ausente/parcial; proxy, disco cheio/outbox, payload grande, CPU/mem e rollback de update referenciam evidências operacionais PKG-69 já versionadas;
   - [x] [exec] atualizar docs, testes, decisões e release somente após validação completa.

### Critérios de aceite

- [x] o agente descobre fontes úteis sem configuração manual inicial e sem scan amplo;
- [x] cada candidato tem fingerprint estável, evidência sanitizada, confiança, severidade mínima, volume estimado, risco, utilidade e permissões;
- [x] IIS, Nginx, Apache, app, banco, fila, EventLog Security, Linux auth, Docker, Kubernetes e OTLP/APM são detectados quando presentes ou em fixture controlada equivalente; IIS/Kubernetes reais seguem como homologação por cliente quando existirem no ambiente;
- [x] fonte sem permissão ou sem nível vira lacuna, não falso positivo;
- [x] coleta padrão respeita `error` ou superior e não envia `info/debug` para IA;
- [x] configuração atual, ingestão, snapshots, outbox e auto-update não quebram nos testes e update real dos seis agentes online;
- [x] nenhum segredo aparece em payload, log local, snapshot, flare ou evidência.

### Rollback

Desligar discovery por flag local/remota, ignorar `log_source_discovery_v1` no web, remover aprovações de fontes, voltar às coletas configuradas atuais ou publicar versão anterior do agente.

---

## [PKG-75] Logs/IA — sinais do agente para triagem recorrente e deduplicação segura

**Status** — implementado por compatibilidade de contrato em 20/06/2026. A triagem principal fica no `aiceberg_web`; o agente preserva origem, nível/severidade e sinais estruturados necessários para o backend não depender de texto livre. Correção runtime `0.8.14` gerada como pacote instalável para impedir auto-ruído de conectividade em logs coletados. Correção `0.8.15` adiciona identidade assinada nos endpoints de self-heal/erro do worker para clientes com identidade estrita habilitada.

### Lotes propostos

1) **Campos necessários para o backend**
   - [x] [exec] manter `level/severity`, `source`, `file`, canal/caminho e origem SOC quando disponíveis em logs enviados pelo agente;
   - [x] [exec] manter descarte local de evento sem nível quando `OSLOG_MIN_SEVERITY=error` estiver ativo;
   - [x] [exec] enviar descoberta de fontes locais para que o web diferencie origem real, canal estruturado, aplicação, banco, fila e runtime;
   - [x] [validacao] cobrir contrato por `go test ./internal/platform/collectors/logdiscovery ./internal/bootstrap`.

2) **Compatibilidade e rollback**
   - [x] [exec] não alterar contrato legado de `/v1/logs/raw`;
   - [x] [exec] permitir desligar discovery por `LOG_DISCOVERY_ENABLED=false` ou configuração remota;
   - [x] [validacao] validar update real para a versão nova em Linux e Windows: agentes Linux `1`, `4`, `18`, `19`, `70` e Windows `71` chegaram em `0.8.13`; desktop `72` está desligado e será testado futuramente se necessário.

3) **Auto-ruído de conectividade do agente**
   - [x] [exec] rebaixar timeouts transitórios de `config`, `ping` e `ingest` para log operacional `INFO`/backoff, mantendo `ERROR` para falha permanente ou erro local real;
   - [x] [exec] gerar versão `0.8.14` para impedir que `/var/log/messages`/EventLog capturem timeout transitório do próprio AIceberg como incidente do cliente;
   - [x] [validacao] cobrir regressão com testes focados de `ConfigSync`, `PingBackend` e `FlushOutbox`.

4) **Identidade estrita em controle do agente**
   - [x] [exec] enviar `X-Agent-Identity` em `selfheal-commands`, `selfheal-report` e `error-report`;
   - [x] [exec] gerar versão `0.8.15` para evitar falso alerta de identidade obrigatória quando o token é válido;
   - [x] [validacao] cobrir regressão com teste focado de `AgentControlClient`.

---

## [PKG-76] Agente — readiness EDR/NDR seguro

**Status** — implementado e validado em fixture/contrato controlado em 21/06/2026. Homologação real com CrowdStrike/Darktrace depende de fornecedor ativo no ambiente do cliente e não gera declaração ampla de compatibilidade sem evidência objetiva.

### Lotes propostos

1) **Modo seguro e limites**
   - [x] [exec] adicionar `EDR_SAFE`/`AICEBERG_EDR_SAFE` e `EDR_SAFE_PROFILE`;
   - [x] [exec] aplicar defaults conservadores para logs `error+`, discovery bounded, containers, Kubernetes, OTLP e checks locais;
   - [x] [exec] preservar métricas essenciais, inventário mínimo, logs úteis e auto-update seguro;
   - [x] [validacao] cobrir com `go test ./internal/common/config ./internal/domain/usecase`.

2) **Manifesto runtime**
   - [x] [exec] emitir `edr_ndr_readiness` no snapshot runtime com modo, política, manifesto, módulos, allowlist, lacunas e validação;
   - [x] [exec] bloquear shell remoto, ação destrutiva, execução arbitrária e claim de fornecedor sem evidência;
   - [x] [validacao] cobrir com `go test ./internal/bootstrap`.

3) **Coleta e compatibilidade**
   - [x] [exec] manter EventLog/journald/log discovery preferenciais e limitar arquivo bruto quando possível;
   - [x] [exec] manter compatibilidade com instalações atuais e configuração remota;
   - [x] [validacao] cobrir com `go test ./internal/platform/collectors/logdiscovery ./internal/platform/collectors/oslogs`.

4) **Fechamento**
   - [x] [validacao] rodar `go test ./internal/bootstrap ./internal/common/config ./internal/domain/usecase ./internal/platform/collectors/logdiscovery ./internal/platform/collectors/oslogs`;
   - [x] [exec] gerar versão instalável após `./check.sh`;
   - [x] [validacao] fechamento coordenado com o web para exibir readiness e bloquear auto-ruído interno em análise SOC.

### Rollback

Desativar `EDR_SAFE`, voltar ao perfil padrão, pausar módulos sensíveis por configuração remota assinada ou publicar a versão anterior do agente. Remover allowlist específica aplicada em fornecedor se ela tiver sido usada apenas para teste.

---

## Modelo de trabalho IA - 2026-05-22

Projeto: `aiceberg_agent`. Stack: Go agent/CLI, gopsutil, NTP, SNMP. Este arquivo segue `PATHS.toml` e deve ser mantido atualizado ao iniciar ou alterar o projeto.

## Quando criar pacote

Criar pacote quando houver varias demandas relacionadas, risco transversal, mudanca em mais de um modulo ou necessidade de dividir entrega em lotes. Um lote pode resolver uma ou mais demandas, mas o pacote so fecha apos check completo e review de fechamento.
