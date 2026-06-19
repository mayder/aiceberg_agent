# PKG-60 - Evidencia real de journald e canais Windows

## Ambiente

- Data UTC: 2026-06-19T18:07:23Z
- Responsavel: Codex
- Cliente/lab: VMAIPROD2 / S&S Solucoes em TI LTDA e Windows desktop GOVI
- Linux host: VMAIPROD2, Oracle Linux/RHEL-like
- Windows host: GOVI, 192.168.15.24
- Binario Linux temporario: `/tmp/aiceberg_agent_pkg60_linux_amd64`
- SHA256 do binario Linux temporario: `0020f7e8a0f49738dfb2eaad2dee173e486db28ee0350ab1a3ba4faf9eceb9f6`
- Topologia: direct temporario para backend fake local; servico instalado do agente nao foi alterado.

## Escopo validado

- Journald: evento real gerado no host Linux e coletado via `journalctl --output=json`, com `source_tool=journald`, `transport=agent_journald`, `level=error` e `severity=error`.
- Windows Security: canal real lido em GOVI com eventos presentes e provider do sistema operacional.
- Windows System: canal real lido em GOVI com eventos presentes.
- Windows Application: canal real lido em GOVI com eventos presentes.
- Sysmon: canal `Microsoft-Windows-Sysmon/Operational` validado em GOVI com Sysmon instalado temporariamente apenas para a prova e removido ao final.

## Resultado da coleta Linux

- `logs_raw_count=3`.
- `journald_seen=yes`.
- `journald_source_tool=yes`.
- `journald_transport=yes`.
- `real_token_used=no_dummy_token_only`.
- Mensagem journald: `AIceberg PKG60 real journald error 20260619T180723Z`.

## Resultado da coleta Windows

- `Security`: status `ok`, 5 eventos coletados.
- `System`: status `ok`, 5 eventos coletados.
- `Application`: status `ok`, 5 eventos coletados.
- `Microsoft-Windows-Sysmon/Operational`: status `ok`, 5 eventos coletados.
- Estado antes da remocao: `Sysmon64=Running`, `Sysmon=absent`.
- Estado apos remocao: `Sysmon64=absent`, `Sysmon=absent`.

## Controles de seguranca

- O agente Linux foi executado com backend fake local e token dummy sanitizado.
- Sysmon foi instalado temporariamente a partir do pacote oficial Sysinternals somente para gerar canal/eventos reais de teste.
- O Sysmon foi removido ao final com checagem posterior de servico ausente.
- Nenhum servico AIceberg instalado foi substituido ou reiniciado para esta prova.
- O artefato bruto foi escaneado antes de versionar e nao contem token real, senha conhecida ou chave privada.

## Artefato bruto

- Arquivo principal: `raw/pkg60-real-journald-windows-channels-raw.tgz`.
- Conteudo: bundle Linux sanitizado de journald, resumo da coleta, captura do backend fake, cursor, saidas de agente/backend, eventos Windows por canal, saidas de instalacao/remocao temporaria do Sysmon e checagem posterior de servico ausente.

## Resultado

Validado em ambiente real que o pipeline de logs processa journald com severidade e origem preservadas, e que os canais Windows Security/System/Application/Sysmon existem e retornam eventos reais para a matriz de coleta. Este bundle cobre o cenario `pkg60-real-journald-windows-channels`.

## Limites

Esta evidencia fecha o cenario operacional faltante do PKG-60. Ela nao declara superioridade sobre Datadog; apenas comprova cobertura real de fontes de log exigidas pelo gate do pacote.
