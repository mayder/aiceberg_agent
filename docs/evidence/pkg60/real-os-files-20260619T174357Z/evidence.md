# PKG-60 - Evidencia real de OS logs e arquivo comum

## Ambiente

- Data UTC: 2026-06-19T17:43:57Z
- Responsavel: Codex
- Cliente/lab: VMAIPROD2 / S&S Solucoes em TI LTDA
- Host: VMAIPROD2
- Sistema: Oracle Linux/RHEL-like
- Agente validado: `/usr/local/bin/aiceberg_agent`
- Versao reportada no bootstrap: 0.8.9
- SHA256 do binario: `a25d50df4aec1cbb798d5afcac963b7848a84fcb7c8dcf8a6e7e124f42bfa78d`
- Topologia: direct temporario para backend fake local; servico instalado nao foi alterado.

## Escopo validado

- Windows EventLog: reaproveitado de evidencia real `docs/evidence/pkg69/windows-desktop-real-client1-20260619T164239Z/evidence.md`, que comprovou EventLog Application real com `OSLOG_MIN_SEVERITY=error`, normalizacao de `Erro` para `error` e persistencia no backend.
- Linux syslog: validado neste bundle com `/var/log/messages` real em VMAIPROD2, cursor inicializado no tamanho atual antes de gerar evento por `logger -p user.err`.
- Arquivo comum: validado neste bundle com arquivo temporario real `common-app.log`, lido pelo mesmo coletor de arquivos do agente.

## Execucao isolada

- Backend fake local: `127.0.0.1:18084`.
- Health temporario: `127.0.0.1:18085`.
- Token: dummy, sanitizado como `token-redacted` no artefato.
- Estado/cursor/outbox: diretorio temporario em `/tmp/aiceberg_pkg60_real_os_files_20260619T174357Z`.
- Config de logs: `OSLOG_ENABLED=true`, `OSLOG_FILES=/var/log/messages,<common-app.log>`, `OSLOG_INCLUDE_REGEX=AIceberg PKG60 real`, `OSLOG_ENRICH=true`.
- O servico real instalado continuou separado; a prova executou um processo temporario com `timeout`.

## Evidencia capturada

- `logs_raw_count=1`.
- `common_seen=yes`.
- `syslog_seen=yes`.
- Mensagem syslog real: `AIceberg PKG60 real linux syslog error 20260619T174357Z`.
- Mensagem arquivo comum: `AIceberg PKG60 real common file error 20260619T174357Z`.
- Evento `/var/log/messages` saiu com `source_tool=linux_syslog`, `transport=agent_file`, `source_category=observability`.
- Evento do arquivo comum saiu pelo mesmo `/v1/logs/raw`, com `transport=agent_file`.

## Artefato bruto

- Arquivo: `raw/aiceberg_pkg60_real_os_files_20260619T174357Z.tgz`.
- Conteudo: `SUMMARY.tsv`, captura sanitizada do backend fake, stdout/stderr do agente, stats do backend, arquivo comum e copia do cursor.
- Segredos: nenhum token real usado; token dummy sanitizado; scan local nao encontrou chave privada, senha conhecida ou token bruto da prova.

## Resultado

Validado em ambiente real que o pipeline de logs coleta Linux syslog e arquivo comum pelo contrato `/v1/logs/raw`, preservando cursor e campos de origem. Com a evidencia Windows EventLog real ja versionada no PKG-69, este bundle cobre o cenario `pkg60-real-os-files`.

## Limites

Este bundle nao fecha Graylog real, Windows Security, Linux auth, app JSON/texto amplo, journald nem Sysmon. Esses cenarios permanecem nos bundles obrigatorios `pkg60-real-source-formats` e `pkg60-real-journald-windows-channels`.
