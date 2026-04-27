# PKG-32 Lote 1 - Contrato tecnico do canal operacional

Fonte de verdade compartilhada: `aiceberg.agent.channel.v1`, `schema_version=1`.

## Topologia travada

- `direct -> AIceberg`: agente abre conexao outbound direto com o AIceberg.
- `hub -> AIceberg`: hub abre conexao outbound com o AIceberg e pode representar relays.
- `relay -> hub -> AIceberg`: relay conecta somente ao hub. Relay nao abre canal direto com o AIceberg.

## Contratos HTTP preservados

O canal novo nao remove nem altera estes fluxos durante o rollout:

- `ping`: `/v1/agent/ping`
- `bootstrap`: `/v1/agent/bootstrap`, `/v1/ingest/bootstrap`
- `config`: `/v1/agent/config`
- `selfheal`: `/v1/agent/selfheal-commands`, `/v1/agent/selfheal-report`
- `update-report`: `/v1/agent/update-report`
- `ingest`: `/v1/ingest`, `/v1/ingest/metrics`, `/v1/ingest/health`, `/v1/ingest/inventory`, `/v1/ingest/network_capture`

## Envelope canonico

Campos base obrigatorios:

- `contract_id`: sempre `aiceberg.agent.channel.v1`
- `schema_version`: sempre `1`
- `message_id`: UUID/ULID/id unico da mensagem
- `type`: `session.open`, `heartbeat`, `command`, `ack`, `progress`, `result`, `error`, `timeout`, `retry`
- `timestamp_utc`: timestamp UTC ISO-8601
- `agent_id`: identificador do agente emissor ou alvo
- `mode`: `direct`, `hub` ou `relay`

Campos condicionais:

- `hub_agent_id`: obrigatorio quando a mensagem envolver relay.
- `command_id`: obrigatorio em `command`, `ack`, `progress`, `result`, `error`, `timeout`, `retry`.
- `correlation_id`: recomendado para agrupar toda a execucao ponta a ponta.
- `attempt`: numero da tentativa, iniciando em `1`.
- `timeout_ms`: limite negociado para execucao ou ACK.
- `retry_after_ms`: espera sugerida em `retry`.
- `capabilities`: lista de capacidades reportadas em `session.open` e `heartbeat`.
- `payload`: entrada de comando ou evento.
- `progress`: etapa atual, percentual opcional e mensagem sanitizada.
- `result`: status final e evidencias sanitizadas.
- `error`: codigo estavel, mensagem sanitizada, etapa e retryable.

## Estados padrao

- ACK: `accepted`, `rejected`, `duplicate`.
- Progresso: `running`.
- Resultado: `success`, `failed`, `skipped`.
- Erro/timeout/retry: `failed`, `timeout`, `retrying`.

## Idempotencia e compatibilidade

- `command_id` e a chave idempotente entre canal novo e polling atual.
- ACK duplicado deve responder `duplicate`, sem reexecutar comando.
- Se o canal cair, o comando pode voltar ao fluxo antigo somente quando o `command_id` ainda nao tiver resultado terminal.
- O polling atual continua sendo fallback ate homologacao e rollback validado.

## Seguranca e auditoria

- O agente executa apenas comandos da allowlist: `collect_now`, `apply_agent_mode`, `restart_agentless_worker`, `reload_configuration`, `clear_local_lock`, `requeue_pending_collect`, `validate_api_connectivity`, `resync_clock`, `inspect_runtime_config`.
- Comandos de shell arbitrario (`shell`, `exec`, `bash`, `sh`, `cmd`, `powershell`, `script` e aliases equivalentes) sao rejeitados antes da execucao.
- Comando duplicado pelo mesmo `command_id` nao reexecuta entre canal e polling.
- Evidencias de resultado usam snapshots sanitizados; tokens, secrets, passwords e authorization nao podem sair em payload de auditoria.
