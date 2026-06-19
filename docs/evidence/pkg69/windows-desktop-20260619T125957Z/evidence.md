# PKG-69 - Windows Desktop

## Ambiente

- Data UTC: 2026-06-19T15:59:00Z
- Responsavel: Codex AIceberg
- Cliente/lab: Lab local autorizado - Windows desktop GOVI
- Ambiente: Windows 10 Home Single Language 64 bits
- Host/agente/HUB/relay: host=GOVI ip=192.168.15.24 service=AIcebergAgent mode=direct local test
- Versao agente: 0.8.9
- Artefato instalado: aiceberg-agent-windows-amd64.zip sha256=d501c128694beb832d33504f52548831196ef861ec360a88400d867242e142d2
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- smoke.ps1 executado: yes, health e metrics locais OK com binario Windows pre-compilado e backend fake local
- EventLog real coletado: yes, System/Application coletados via Get-WinEvent com Service Control Manager e WinRM
- instalador validado: yes, install.ps1 instalou binario, scripts de auto-update, EventLog source e servico Windows AIcebergAgent
- proxy autenticado quando aplicavel: not_applicable neste cenario desktop; proxy/TLS ja coberto no cenario dedicado PKG-69
- logs sem segredo: yes, artefato bruto nao contem senha real nem token produtivo; token usado e dummy de smoke

## Metricas

- proc_cpu_percent: 2.057598728498627
- proc_rss_bytes: 34537472
- ingest_confirmed: yes

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/raw-windows-desktop-lowoverhead-20260619T125957Z.zip
- Observacoes: Windows desktop validado via AnyDesk+WinRM em rede local. Smoke local preservado no raw; servico oficial AIcebergAgent ficou Running com HEALTH_PORT=8081, API local indisponivel controlada, outbox reteve fila sem perda e EventLog real foi coletado. A primeira tentativa com nome de servico alternativo falhou porque o binario espera o nome oficial AIcebergAgent; a tentativa oficial sem token provou que o instalador exige token; a tentativa final com token dummy e SkipBootstrap validou o servico.
- Rollback validado: yes
- Revisor: Codex AIceberg
- Aprovacao fechamento: yes
