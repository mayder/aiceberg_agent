# PKG-61 - Evidencia de app local e alta cardinalidade

## Ambiente

- Data UTC: 2026-06-19T18:19:00Z
- Responsavel: Codex
- Cliente/lab: lab local controlado no host de desenvolvimento
- Sistema: Darwin arm64
- Comando: `PKG61_EVIDENCE_DIR=/tmp/aiceberg_pkg61_local_app_high_volume_20260619T181900Z go test ./internal/platform/collectors/custommetrics -run TestPKG61LocalAppHighVolumeEvidence -count=1 -v`
- Topologia: receptor local do agente em loopback, sem API remota e sem credencial de API.

## Escopo validado

- App local controlada enviou metrica HTTP em `POST /v1/custom-metrics`.
- Metrica DogStatsD-like foi enviada por UDP loopback.
- Metrica DogStatsD-like foi enviada por Unix Domain Socket com permissao `0600`.
- Burst de alta cardinalidade foi limitado por `CUSTOM_METRICS_MAX_SERIES`.
- Excesso de series foi contabilizado em `dropped_count`, sem payload descartado persistido.
- Tags `service`, `env` e tags operacionais foram preservadas no payload.

## Resultado

- `accepted_count=32`.
- `dropped_count=51`.
- `series_count=32`.
- `http_app_metric=true`.
- `udp_dogstatsd=true`.
- `uds_dogstatsd=true`.
- `uds_socket_mode=0600`.
- `max_series=32`.
- `high_cardinality=80`.
- `api_credential=not_used`.
- `loopback_only=yes`.
- `service_env_tags=yes`.

## Evidencia relacionada

O bundle real do PKG-69 `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z/evidence.md` validou DogStatsD em alto volume com 1500 series UDP loopback, limite de 1000 series, `accepted_count=1300`, `dropped_count=740`, CPU `0.0%` e RSS `14567688` no probe conjunto DogStatsD/OTLP/logs.

## Artefato bruto

- Arquivo: `raw/pkg61-local-app-high-volume-raw.tgz`.
- Conteudo: payload `custom_metrics`, `SUMMARY.tsv` e log do `go test`.

## Resultado final do pacote

Esta evidencia fecha os itens pendentes do PKG-61: alto volume/tags explosivas e metrica enviada por app local controlada. Ela nao cria tabela, nao altera snapshot base e nao declara superioridade sobre Datadog.
