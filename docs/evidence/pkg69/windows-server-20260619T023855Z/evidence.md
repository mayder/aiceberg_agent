# PKG-69 - Windows Server

## Ambiente

- Data UTC: 2026-06-19T02:38:55Z
- Responsavel: Codex AIceberg
- Cliente/lab: S&S Solucoes em TI LTDA - Windows Server InspectApp
- Ambiente: Windows Server 2022
- Host/agente/HUB/relay: agent_id=71 host=10.100.35.3 service=AIcebergAgent
- Versao agente: 0.8.8
- Artefato instalado: agent.exe sha256=950d73174467ec3bbca4fac9ba255c9a9bc458d68fbee165fbedd370ae3b4c4e
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- smoke.ps1 executado: health local validado via WinRM em http://127.0.0.1:8081/health
- EventLog Security/System/Application coletado: System coletado com Service Control Manager para AIcebergAgent
- servico Windows instalado/reiniciado: yes, AIcebergAgent Running apos rollback e restore
- update controlado com rollback: yes, rollback para backup agent.exe.backup_pkg69_metrics_interval_20260619T022451Z e restore do binario atual
- logs sem segredo: yes, artifact raw sem token/senha e sem payload sensivel

## Metricas

- proc_cpu_percent: 1.2083213528861931
- proc_rss_bytes: 31109120
- queue_items: 1
- ingest_confirmed: yes

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_windows_server_rollback_retry_20260619T023541Z.tgz
- Observacoes: Rollback controlado validou backup anterior, servico Running, canal direct conectado, flush ACK e relay_connects_to_aiceberg=false; binario atual restaurado ao final.
- Rollback validado: yes
- Revisor: Codex AIceberg
- Aprovacao fechamento: yes
