# PKG-69 - Permissao eBPF Restrita

## Ambiente

- Data UTC: 2026-06-19T03:27:53Z
- Responsavel: Codex AIceberg
- Cliente/lab: servidor Linux novo InspectApp agente 70
- Ambiente: Ubuntu 24.04, usuario deploy sem sudo e sem CAP_NET_RAW/CAP_NET_ADMIN para captura
- Host/agente/HUB/relay: srv-ainalise-prod agent_id=70 health local 127.0.0.1:18081
- Versao agente: 0.8.8
- Artefato instalado: /home/deploy/aiceberg-agent/bin/aiceberg_agent sem alteracao de binario/config durante esta prova; probe isolado removido apos execucao
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- eBPF/pcap sem permissao: yes, tcpdump -i any retornou Operation not permitted para usuario deploy
- degradacao clara reportada: yes, payload reportou warning pcap indisponivel por permissao insuficiente
- agente segue coletando sinais basicos: yes, health do agente 70 status ok, flush_ok=566 e channel direct conectado
- sem execucao remota insegura: yes, probe local isolado sem shell remoto via agente e sem sudo
- logs sem segredo: yes, raw scan local sem segredo conhecido

## Metricas

- degraded_collectors: pcap
- ingest_confirmed: yes
- proc_cpu_percent: 1.4

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_permission_ebpf_clean_raw_20260619T032753Z.tgz
- Observacoes: Prova executada no host Linux real 187.45.180.181 como usuario deploy. O coletor de rede caiu para socket_snapshot+pcap com warning de PCAP sem permissao, sem crash, enquanto o agente instalado manteve ingest direct ativo. Nao houve mudanca persistente no agente instalado.
- Rollback validado: yes
- Revisor: Codex AIceberg
- Aprovacao fechamento: yes
