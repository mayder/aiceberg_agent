# Evidencia PKG-63 - APM alto volume, erro e jornada correlata

Data UTC: 20260619T183604Z

## Resultado

- Receiver OTLP HTTP/JSON real do agente executado em loopback.
- Entrada controlada com 80 spans no mesmo trace e um log ERROR correlato.
- Sampling configurado com `APM_TRACE_SAMPLE_RATE=0`, preservando erro de aplicacao e span lento.
- Jornada validada: log -> trace -> span -> service -> host.
- Profiler permanece fora do escopo inicial por decisao registrada; nao foi marcado como coberto.
- Overhead operacional referenciado pelo bundle PKG-69 `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z/evidence.md`.

## Sumario

- `input_spans`: `80`
- `accepted_count`: `2`
- `dropped_count`: `78`
- `trace_items`: `2`
- `logs_events`: `1`
- `application_error`: `yes`
- `sampling_error`: `yes`
- `sampling_slow`: `yes`
- `journey_log_trace`: `yes`
- `service`: `checkout-api`
- `host`: `pkg63-host`
- `api_credential`: `not_used`
- `transport`: `otlp_http_json`
- `profiler_scope`: `out_of_scope_by_decision`
- `overhead_reference`: `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z/evidence.md`

## Validacao executada

`PKG63_EVIDENCE_DIR=/tmp/aiceberg_pkg63_apm_high_volume_error_20260619T183604Z go test ./internal/platform/collectors/otlp -run TestPKG63APMHighVolumeErrorJourneyEvidence -count=1 -v`

## Evidencia bruta

- Arquivo: `raw/pkg63-apm-high-volume-error-raw.tgz`
