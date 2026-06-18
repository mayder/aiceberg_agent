# PKG-63 - APM sampling inicial

## Escopo entregue no agente

O receiver OTLP HTTP/JSON aplica sampling deterministico no flush de traces antes de enviar `body.otlp.kind=traces`.

Configuracoes locais:

```env
APM_TRACE_SAMPLE_RATE=1
APM_TRACE_SLOW_THRESHOLD_MS=1000
APM_TRACE_PRESERVE_ERRORS=true
```

Config remota equivalente:

```json
{
  "apm": {
    "trace_sample_rate": 0.25,
    "trace_slow_threshold_ms": 1000,
    "trace_preserve_errors": true
  }
}
```

## Regras

- `trace_sample_rate` varia de `0` a `1`.
- Erros sao preservados quando `trace_preserve_errors=true`.
- Spans com `duration_ms >= trace_slow_threshold_ms` sao preservados.
- Spans descartados incrementam `dropped_count`; payload descartado nao e persistido.
- Cada span enviado inclui `duration_ms`, `status`, `error` e `sampling_reason`.

## Limites

Este lote nao entrega tail-based sampling completo no backend nem UI APM. Essas validacoes ficam no `aiceberg_web` e no PKG-69.

## Instrumentacao por linguagem

O AIceberg Agent nao fornece SDK proprio neste ciclo. Aplicacoes devem usar os SDKs oficiais OpenTelemetry da propria linguagem e enviar OTLP para o receiver local do agente quando `OTLP_ENABLED=true`.

Configuracao base para apps que emitem OTLP HTTP/JSON compativel:

```env
OTEL_SERVICE_NAME=minha-api
OTEL_RESOURCE_ATTRIBUTES=deployment.environment=prod,service.version=1.0.0
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
OTEL_TRACES_EXPORTER=otlp
OTEL_METRICS_EXPORTER=otlp
OTEL_LOGS_EXPORTER=otlp
```

Rotas locais esperadas pelo agente:

- traces: `POST /v1/traces`;
- metrics: `POST /v1/metrics`;
- logs: `POST /v1/logs`.

Diretrizes por stack:

- Java: usar OpenTelemetry Java Agent oficial e apontar o exporter OTLP para o loopback do host ou para um OpenTelemetry Collector sidecar quando a aplicacao emitir protocolo ainda nao suportado diretamente.
- Node.js: usar `@opentelemetry/sdk-node` e exporter OTLP oficial; manter `service.name`, `deployment.environment` e `service.version`.
- Python: usar `opentelemetry-sdk` e instrumentacoes oficiais; evitar incluir payload sensivel em atributos customizados.
- .NET: usar `OpenTelemetry.Extensions.Hosting` e exporters oficiais; manter traces e logs com `trace_id`/`span_id` correlacionaveis.
- Go: usar `go.opentelemetry.io/otel` e exporter OTLP oficial; configurar sampling da aplicacao separado do sampling do agente.

Se o SDK ou collector emitir apenas gRPC/protobuf, usar OpenTelemetry Collector como sidecar/daemon para converter ou aguardar o pacote futuro de gRPC/protobuf. O PKG-63 nao cria wrapper, auto-instrumentacao proprietaria nem profiler.

## Persistencia no web

Traces enviados como `body.otlp.kind=traces` sao materializados pelo `aiceberg_web` em `agent_apm_span` quando o SQL `sql/2026_06_18_pkg63_agent_apm_span.sql` ja foi aplicado. Se a tabela ainda nao existir, a ingestao ignora a materializacao APM e preserva compatibilidade com snapshots e agentes existentes.
