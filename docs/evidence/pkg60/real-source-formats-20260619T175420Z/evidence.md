# PKG-60 - Evidencia real de formatos e origem de logs

## Ambiente

- Data UTC: 2026-06-19T17:54:20Z
- Responsavel: Codex
- Cliente/lab: VMAIPROD2 / S&S Solucoes em TI LTDA e Windows desktop GOVI
- Linux host: VMAIPROD2, Oracle Linux/RHEL-like
- Windows host: GOVI, 192.168.15.24
- Binario Linux temporario: `/tmp/aiceberg_agent_pkg60_linux_amd64`
- SHA256 do binario Linux temporario: `0020f7e8a0f49738dfb2eaad2dee173e486db28ee0350ab1a3ba4faf9eceb9f6`
- Topologia: direct temporario para backend fake local; servico instalado nao foi alterado.

## Escopo validado

- Graylog/GELF: linha GELF real em arquivo no host Linux, processada pelo agente com `source_tool=graylog_gelf`, `level=error`, `severity=error` e `short_message` como mensagem canonica.
- Linux auth: evento real em `/var/log/secure` gerado via `logger -p authpriv.err`, processado pelo agente com `source_tool=linux_auth`, `source_category=security` e redaction de comando sudo.
- App JSON: linha JSON de aplicacao com `level=error`, `service=orders-api`, atributos parseados e `password` redigido.
- Log texto: linha texto comum com marcador `ERROR`, coletada pelo mesmo pipeline `/v1/logs/raw`.
- Windows Security: eventos reais do canal `Security` coletados via WinRM read-only em GOVI, incluindo IDs `4624`, `4634`, `4672` e `4799`, provider `Microsoft-Windows-Security-Auditing`.

## Resultado da coleta Linux

- `logs_raw_count=1`.
- `graylog_seen=yes`.
- `linux_auth_seen=yes`.
- `app_json_seen=yes`.
- `log_text_seen=yes`.
- `graylog_source_tool=yes`.
- `linux_auth_source_tool=yes`.
- `password_redacted=yes`.
- `real_token_used=no_dummy_token_only`.

## Correcao gerada pela validacao

A prova em Oracle Linux revelou que `/var/log/secure` nao era classificado como `linux_auth`, porque o classificador reconhecia apenas nomes com `auth`. O coletor foi corrigido para tratar basename `secure` como log de autenticacao Linux, preservando compatibilidade com `auth.log`.

## Artefato bruto

- Arquivo principal: `raw/pkg60-real-source-formats-raw.tgz`.
- Conteudo: bundle Linux sanitizado, captura do backend fake, stdout/stderr do agente, cursor, entradas de teste sanitizadas e `windows-security-20260619T175420Z.json`.
- Segredos: token real nao usado; token dummy redigido; nomes/valores sensiveis no comando sudo e senha de teste foram redigidos antes do commit.

## Resultado

Validado em ambiente real que o pipeline de logs processa Graylog/GELF, Linux auth em RHEL/Oracle Linux, app JSON, log texto e Windows Security com campos de origem/severidade suficientes para o fluxo AIceberg. Este bundle cobre o cenario `pkg60-real-source-formats`.

## Limites

Este bundle nao fecha journald nem os canais Windows System/Application/Sysmon. Esses itens permanecem no cenario `pkg60-real-journald-windows-channels`.
