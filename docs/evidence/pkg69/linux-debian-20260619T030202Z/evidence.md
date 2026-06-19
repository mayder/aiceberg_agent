# PKG-69 - Linux Debian Ubuntu

## Ambiente

- Data UTC: 2026-06-19T03:02:02Z
- Responsavel: Codex AIceberg
- Cliente/lab: S&S Solucoes em TI LTDA - Plesk antigo Inspect agente 19
- Ambiente: Debian GNU/Linux 11 bullseye
- Host/agente/HUB/relay: zen-davinci.92-204-168-1.plesk.page agent_id=19 service=aiceberg-agent.service
- Versao agente: 0.8.8
- Artefato instalado: /usr/local/bin/aiceberg_agent sha256=a3e03f930857f1bfc3c8a8e033f9adf53bb8e5e727f1072bc9720ec1fe451023
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- smoke.sh executado: yes, smoke POSIX anterior e journal/systemd real no host Debian
- systemd service validado: yes, aiceberg-agent.service active, MainPID restaurado, NRestarts=0
- syslog/journald real coletado: yes, journalctl do servico anexado
- update controlado: rollback local controlado por binario systemd; update remoto assinado fica no cenario dedicado
- logs sem segredo: yes, artifact raw redigido e scan local sem segredo conhecido

## Metricas

- proc_cpu_percent: 3.0
- proc_rss_bytes: 20197376
- queue_items: 0
- ingest_confirmed: yes

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_linux_debian_rollback_20260619T025854Z.tgz
- Observacoes: Rollback controlado aplicou backup anterior sha256=794426c0ecddd17e8ef5f50798504fee455b75e59ebcf0a2a5e2b42a66ba25fe, restaurou o binario atual, manteve systemd active, NRestarts=0 e flush ACK com retained=0.
- Rollback validado: yes
- Revisor: Codex AIceberg
- Aprovacao fechamento: yes
