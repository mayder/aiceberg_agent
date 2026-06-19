# PKG-69 - Alto Volume e Overhead

## Ambiente

- Data UTC: 2026-06-19T03:34:36Z
- Responsavel: Codex AIceberg
- Cliente/lab: lab local controlado com receptores reais do agente
- Ambiente: Darwin 25.5.0 arm64, Go test probe com portas efemeras loopback
- Host/agente/HUB/relay: receptores locais DogStatsD/HTTP custom metrics e OTLP HTTP/JSON sem servico persistente
- Versao agente: 0.8.8 / coletores custommetrics e otlp do commit HEAD
- Artefato instalado: probe temporario removido apos execucao; nenhum agente instalado alterado
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- DogStatsD alto volume: yes, 1500 series UDP loopback com limite de 1000 series
- OTLP alto volume: yes, 180 itens por tipo para metrics, logs e traces com limite de 100 por receiver
- logs alto volume: yes, 180 OTLP logRecords com 100 aceitos e 80 descartados
- CPU/memoria dentro do limite: yes, CPU reportada 0.0% no probe e RSS proxy 14567688 bytes
- rollback de carga: yes, portas efemeras encerradas e arquivo temporario removido

## Metricas

- proc_cpu_percent: 0.0
- proc_rss_bytes: 14567688
- accepted_count: 1300
- dropped_count: 740

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_high_volume_raw_20260619T033436Z.tgz
- Observacoes: Prova controlada usa receptores reais dos coletores, sem backend remoto e sem alterar agentes instalados. Breakdown: DogStatsD 1000/500, OTLP metrics 100/80, OTLP logs 100/80, OTLP traces 100/80.
- Rollback validado: yes
- Revisor: Codex AIceberg
- Aprovacao fechamento: yes
