# PKG-69 - Linux RHEL Alma Rocky

## Ambiente

- Data UTC: 2026-06-19T02:51:13Z
- Responsavel: Codex AIceberg
- Cliente/lab: S&S Solucoes em TI LTDA - VMAIPROD2
- Ambiente: Oracle Linux/RHEL-like 6.12.0-104.43.4.2.el9uek.x86_64
- Host/agente/HUB/relay: VMAIPROD2 agent_id=4 service=aiceberg-agent.service
- Versao agente: 0.8.8
- Artefato instalado: /usr/local/bin/aiceberg_agent sha256=fd32a7d00e18eaeb4f26b55a39676783d5a9959c8e20c67d1a6b6c5234787341
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- smoke.sh executado: yes, smoke oficial anterior no mesmo host em /tmp/aiceberg_pkg69_official_smoke_20260619T010125Z
- systemd service validado: yes, aiceberg-agent.service active, MainPID restaurado, NRestarts=0
- dnf/yum install/update validado: not applicable; instalacao operacional atual e rollback foram por binario systemd controlado
- syslog/journald real coletado: yes, journalctl do servico anexado
- logs sem segredo: yes, artifact raw redigido e scan local sem segredo conhecido

## Metricas

- proc_cpu_percent: 3.4
- proc_rss_bytes: 16748544
- queue_items: 0
- ingest_confirmed: yes

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_linux_rhel_rollback_20260619T024805Z.tgz
- Observacoes: Rollback controlado aplicou backup anterior sha256=0b9e6a71a564fb08a8e85865200648ee7383c5e1e9561673ae92dfc0583b4e3f, restaurou o binario atual, manteve systemd active, NRestarts=0 e flush ACK com retained=0. API/web do AIceberg nao foi reiniciada.
- Rollback validado: yes
- Revisor: Codex AIceberg
- Aprovacao fechamento: yes
