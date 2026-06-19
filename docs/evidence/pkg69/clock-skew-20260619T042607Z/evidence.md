# PKG-69 - Clock Skew Real

## Ambiente

- Data UTC: 2026-06-19T04:25:18Z
- Responsavel: Codex
- Cliente/lab: Docker local com NTP controlado e backend local
- Ambiente: Host com clock/NTP controlado
- Host/agente/HUB/relay: container isolado aiceberg-pkg69-clock com agente real e backend local
- Versao agente: 0.8.8-pkg69-clock
- Artefato instalado: binario linux arm64 do agente em container isolado com servidor NTP controlado para time.google.com
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- clock skew aplicado: yes, offset NTP controlado de 180000 ms sem alterar relogio do host
- time_sync.status reportado: yes, ingest metrics capturou time_sync.status=critical
- coleta continua sem travar: yes, backend local recebeu bootstrap e metrics durante clock critical
- retorno ao clock correto: yes, segunda execucao com offset zero reportou time_sync.status=ok
- evidencias sem segredo: yes, token dummy redigido e nenhum envio externo executado

## Metricas

- offset_ms: 180000
- status_before: critical
- status_after: ok

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_clock_skew_raw_20260619T042518Z.tgz
- Observacoes: Prova executada em container isolado com DNS local apontando time.google.com para NTP controlado no proprio container; backend HTTP local capturou os batches e confirmou time_sync critical, depois ok. O relogio do macOS/host nao foi alterado.
- Rollback validado: yes
- Revisor: Codex
- Aprovacao fechamento: yes
