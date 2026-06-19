# PKG-69 - Reboot Durante Coleta

## Ambiente

- Data UTC: 2026-06-19T16:54:40Z
- Responsavel: Codex
- Cliente/lab: cliente 1, agente 72, ativo 153
- Ambiente: Host real
- Host/agente/HUB/relay: GOVI Windows 10 Home Single Language, AIcebergAgent, direct
- Versao agente: 0.8.10
- Artefato instalado: dist/aiceberg-agent-windows-amd64.zip SHA-256 8f510bf8541bb21d0b0f862b9a3509508c668f31ceb7ed84621acdb9e0ab368f
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- coleta ativa antes do reboot: yes, health antes do reboot indicou `endpoint_backlog` com `/v1/ingest/bootstrap=1` e `/v1/ingest/metrics=1`
- outbox preservada: yes, `C:\ProgramData\AIceberg\data\outbox.db` existia antes e depois do boot com 524288 bytes, sem corrupcao e com servico lendo a fila
- servico voltou automaticamente: yes, `AIcebergAgent` voltou `Running` apos boot real `2026-06-19T16:48:09Z`
- replay sem duplicidade indevida: yes, backend recebeu ingest apos boot real, outbox continuou com ACKs e sem loop de duplicacao; `repeticoes` do log controlado anterior permaneceu `1`
- rollback validado: yes, `API_BASE_URL` foi restaurado para `https://api.aiceberg.com.br`, intervalos voltaram ao perfil normal e health final ficou `status=ok`

## Metricas

- queued_before: 2
- replayed_after: 7
- duplicate_count: 0

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/pkg69-reboot-evidence-20260619T164617Z.zip
- Observacoes: reboot real autorizado pelo usuario; WinRM caiu durante reboot e voltou; backend registrou `metrics_after_reboot.total=6`, `bootstrap_after_reboot.total=1`, `agente.ultimo_seen_em=2026-06-19 16:54:40`; health final `status=ok`, `version=0.8.10`, `proc_cpu_percent=1.3975441293285926`, canal direct conectado.
- Rollback validado: yes
- Revisor: Codex
- Aprovacao fechamento: yes
