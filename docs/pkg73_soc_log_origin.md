# PKG-73 - Taxonomia SOC de logs

## Contrato aditivo

Todo evento de log emitido pelo agente deve preservar os campos legados e adicionar, quando coletado por `oslogs`, journald, EventLog, OTLP logs, Docker logs ou Kubernetes pod logs:

- `aiceberg_transport`
- `aiceberg_tool_origin`
- `aiceberg_source_category`
- `aiceberg_soc_source_type`
- `aiceberg_soc_eligible`
- `aiceberg_origin_confidence`
- `aiceberg_route_reason`

Campos SOC seguros sao promovidos quando existirem: `event_code`, `vendor`, `product`, `src_ip`, `dst_ip`, `src_host`, `dst_host`, `username`, `process_name`, `command_line`, `file_hash`, `domain`, `url`, `action`, `rule_name`, `technique_id` e `alert_id`.

## Regras de classificacao

- Windows `Security` e `Microsoft-Windows-Security-Auditing`: `tool_origin=ad_security`, `source_category=soc`.
- Windows Sysmon: `source_category=soc`, `soc_eligible=yes`.
- Windows `System`/`Application`, incluindo DistributedCOM `10028`: `source_category=observability`, `soc_eligible=no`.
- Linux `auth.log` e `/var/log/secure`: `soc_source_type=linux_security`; falha de autenticacao ou severidade `error+` vira `soc_eligible=yes`.
- Graylog/GELF: `aiceberg_transport=graylog` quando configurado; ferramenta real vem de campos `aiceberg_tool_origin`/vendor/product quando disponiveis.
- App JSON, OTLP, container e Kubernetes ficam `conditional` por padrao, salvo override configurado.

## Overrides controlados

- Logs estruturados podem trazer campos `aiceberg_*` no proprio evento/attributes.
- Docker logs aceitam labels `aiceberg.ai/*` ou `aiceberg.com/*`.
- Kubernetes pod logs aceitam annotations `aiceberg.ai/*` ou `aiceberg.com/*`.
- Config remota pode usar `logs.processors`/`OSLOG_PROCESSORS_JSON` para `route`, `remap` e `enrich`; esse canal permanece sujeito as regras de assinatura/escopo do PKG-68 quando sensivel.

## Redaction

Chaves ou valores com senha, token, cookie, segredo ou Authorization nao sao promovidos para top-level SOC. O payload original sanitizado permanece em `attributes` quando aplicavel.

## Rollback

Desativar coleta de logs (`OSLOG_ENABLED=false`), remover overrides `aiceberg.*` ou publicar a versao anterior do agente. O backend continua aceitando payload legado sem os campos novos.
