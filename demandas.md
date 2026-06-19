# Demandas AIceberg Agent

Backlog do agente desktop/serviço. Este arquivo complementa o backlog do `aiceberg_web` quando a entrega exigir mudanças coordenadas nos dois repositórios.

---

## Organização por pacotes entregáveis

- `PKG-32` Canal operacional bidirecional com agentes
- `PKG-33` Flush resiliente e backpressure para HUB/Relay
- `PKG-73` Agente/SOC — taxonomia canônica de origem, campos SOC e roteamento seguro para PKG-54

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

## Modelo de trabalho IA - 2026-05-22

Projeto: `aiceberg_agent`. Stack: Go agent/CLI, gopsutil, NTP, SNMP. Este arquivo segue `PATHS.toml` e deve ser mantido atualizado ao iniciar ou alterar o projeto.

## Quando criar pacote

Criar pacote quando houver varias demandas relacionadas, risco transversal, mudanca em mais de um modulo ou necessidade de dividir entrega em lotes. Um lote pode resolver uma ou mais demandas, mas o pacote so fecha apos check completo e review de fechamento.
