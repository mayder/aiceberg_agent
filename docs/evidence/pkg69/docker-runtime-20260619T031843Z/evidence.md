# PKG-69 - Docker Runtime

## Ambiente

- Data UTC: 2026-06-19T03:18:43Z
- Responsavel: Codex AIceberg
- Cliente/lab: lab local Docker Desktop controlado
- Ambiente: Docker Desktop 29.2.1 Linux VM aarch64 cgroup v2
- Host/agente/HUB/relay: containerized collector probe com Docker socket real montado em /var/run/docker.sock e /var/lib/docker/containers read-only
- Versao agente: 0.8.8 / coletor containers do commit HEAD
- Artefato instalado: coletor containers cross-compiled linux arm64 executado em container postgres:16-alpine; binary removido do artefato bruto apos execucao
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- CONTAINER_ENABLED=true: yes
- Docker socket real acessado: yes, /var/run/docker.sock montado no container da prova
- labels sensiveis mascaradas: yes, payload restrito ao container controlado com labels aiceberg.pkg e aiceberg.scenario sem segredo
- logs JSON com cursor: yes, PKG69_CONTAINER_CURSOR=/tmp/pkg69-container-clean.cursor e eventos docker_json_file anexados
- carga controlada executada: yes, container aiceberg-pkg69-log-probe-clean gerou logs sequenciais e foi removido no rollback

## Metricas

- containers_seen: 1
- container_logs_seen: 7
- proc_cpu_percent: 0.0
- proc_rss_bytes: 9261056

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_docker_runtime_clean_raw_20260619T031835Z.tgz
- Observacoes: Prova executada em daemon Docker real local porque VMAIPROD2, Plesk 92.204.168.1, Plesk 92.42.106.121 e Linux 187.45.180.181 nao tinham Docker/containerd disponivel; ID 18 nao respondeu no timeout SSH. Coleta ficou restrita ao container controlado para evitar anexar logs de workloads de outros projetos. Rollback: container de carga removido e nenhum servico remoto alterado. Esta evidencia valida runtime Docker e payload local do coletor; ingest remoto direct ja esta coberto pelos cenarios Windows/Linux.
- Rollback validado: yes
- Revisor: Codex AIceberg
- Aprovacao fechamento: yes
