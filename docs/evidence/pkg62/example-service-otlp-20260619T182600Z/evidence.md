# PKG-62 - Evidencia de servico exemplo OTLP

## Ambiente

- Data UTC: 2026-06-19T18:26:00Z
- Responsavel: Codex
- Cliente/lab: lab local controlado no host de desenvolvimento
- Sistema: Darwin arm64
- Comando: `PKG62_EVIDENCE_DIR=/tmp/aiceberg_pkg62_example_service_20260619T182600Z go test ./internal/platform/collectors/otlp -run TestPKG62ExampleServiceOTLPEvidence -count=1 -v`
- Topologia: servico de exemplo instrumentado enviando OTLP HTTP/JSON em loopback para o receiver local do agente.

## Escopo validado

- App de exemplo enviou metricas OTLP HTTP/JSON para `/v1/metrics`.
- Servico simples `checkout-service` enviou log OTLP para `/v1/logs` com `trace_id` e `span_id`.
- Servico simples enviou span OTLP para `/v1/traces` com o mesmo `trace_id`/`span_id`.
- `service.name`, `deployment.environment`, `service.version` e `host.name` foram preservados.
- Atributo sensivel `payment.token` foi redigido antes de sair no payload de logs.
- Nenhuma credencial de API foi usada pela aplicacao local.

## Resultado

- `metrics_items=2`.
- `logs_events=1`.
- `traces_items=1`.
- `service=checkout-service`.
- `env=controlled`.
- `trace_correlation=yes`.
- `redaction=yes`.
- `api_credential=not_used`.
- `transport=otlp_http_json`.
- `simple_service_span=yes`.

## Evidencia relacionada

O bundle real do PKG-69 `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z/evidence.md` validou OTLP metrics/logs/traces em alto volume com limite de 100 itens por receiver, `accepted_count=1300`, `dropped_count=740`, CPU `0.0%` e RSS `14567688` no probe conjunto DogStatsD/OTLP/logs.

## Artefato bruto

- Arquivo: `raw/pkg62-example-service-otlp-raw.tgz`.
- Conteudo: payloads gerados para metrics, logs e traces, `SUMMARY.tsv` e log do `go test`.

## Limites

O escopo do PKG-62 continua restrito a OTLP HTTP/JSON. OTLP gRPC/protobuf permanece fora deste pacote por decisao registrada em `DECISOES.md`; usar OpenTelemetry Collector como conversor quando a aplicacao emitir apenas gRPC/protobuf.

## Resultado final do pacote

Esta evidencia fecha os itens pendentes do PKG-62: app de exemplo real/controlado e servico simples instrumentado. Ela nao declara superioridade sobre Datadog e nao altera persistencia fora dos contratos aditivos existentes.
