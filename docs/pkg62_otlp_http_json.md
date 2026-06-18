# PKG-62 - OTLP HTTP/JSON inicial

## Decisao tecnica

O suporte inicial e OTLP HTTP/JSON local, desligado por padrao. OTLP gRPC/protobuf fica fora deste lote para reduzir dependencias, consumo e risco de regressao no agente atual.

## Endpoints locais

Quando `OTLP_ENABLED=true`, o agente escuta `OTLP_HTTP_ADDR` e aceita:

- `POST /v1/metrics`;
- `POST /v1/logs`;
- `POST /v1/traces`.

A origem aceita e apenas loopback.

## Configuracao

```env
OTLP_ENABLED=true
OTLP_HTTP_ADDR=127.0.0.1:4318
OTLP_INTERVAL=10
OTLP_MAX_ITEMS=1000
OTLP_MAX_BYTES=1048576
```

Config remota equivalente:

```json
{
  "otlp": {
    "enabled": true,
    "http_addr": "127.0.0.1:4318",
    "interval": 10,
    "max_items": 1000,
    "max_bytes": 1048576
  }
}
```

## Mapeamento

- Metrics: `resourceMetrics` vira `body.otlp.kind=metrics` em `/v1/ingest/metrics`.
- Logs: `resourceLogs` vira `events[]` em `/v1/logs/raw`, com `trace_id`, `span_id`, `service`, `severity` e `transport=otlp_http_json`; redaction, include/exclude e severidade minima reutilizam as preferencias do PKG-60.
- Traces: `resourceSpans` vira `body.otlp.kind=traces` em `/v1/ingest/metrics` como contrato transitorio ate PKG-63.

Resource attributes mapeados:

- `host.name` -> `host`;
- `service.name` -> `service`;
- `deployment.environment` -> `env`;
- `service.version` -> `version`.

## Limites

- `OTLP_MAX_BYTES` limita o corpo HTTP.
- `OTLP_MAX_ITEMS` limita cardinalidade/volume da janela.
- `dropped_count` registra descartes sem persistir payload rejeitado.
- Persistencia APM dedicada fica para PKG-63/PKG-69.

## Rollback

Definir `OTLP_ENABLED=false` ou enviar config remota `otlp.enabled=false`. Nao ha SQL do PKG-62.

## Validacao local

```bash
go test ./internal/platform/collectors/otlp ./internal/common/config ./internal/domain/usecase ./internal/bootstrap
```
