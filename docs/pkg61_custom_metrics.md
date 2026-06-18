# PKG-61 - Metricas custom locais

## Escopo entregue

O agente possui um coletor `custommetrics`, desligado por padrao, que recebe metricas locais sem expor token/API key para aplicacoes.

Entradas suportadas:

- UDP local DogStatsD-like em `CUSTOM_METRICS_UDP_ADDR`;
- HTTP local `POST /v1/custom-metrics` em `CUSTOM_METRICS_HTTP_ADDR`.

Tipos suportados:

- `count`;
- `gauge`;
- `histogram`;
- `distribution`;
- `set`;
- `service_check`.

## Contrato enviado

O coletor envia para `/v1/ingest/metrics` dentro de `body.custom_metrics`:

- `schema_version`;
- `source`;
- `flush_window_sec`;
- `series[]`;
- `accepted_count`;
- `dropped_count`.

Cada serie contem nome canonico, tipo, valor/agregados, tags, host, service, env e source quando enviados.

## Configuracao

```env
CUSTOM_METRICS_ENABLED=true
CUSTOM_METRICS_UDP_ADDR=127.0.0.1:8125
CUSTOM_METRICS_HTTP_ADDR=127.0.0.1:8126
CUSTOM_METRICS_INTERVAL=10
CUSTOM_METRICS_MAX_SERIES=1000
CUSTOM_METRICS_MAX_BYTES=65536
```

Config remota equivalente:

```json
{
  "custom_metrics": {
    "enabled": true,
    "udp_addr": "127.0.0.1:8125",
    "http_addr": "127.0.0.1:8126",
    "interval": 10,
    "max_series": 1000,
    "max_bytes": 65536
  }
}
```

## Limites

- O receptor aceita apenas origem loopback.
- Cardinalidade acima de `CUSTOM_METRICS_MAX_SERIES` e contada em `dropped_count`.
- HTTP tem limite por corpo em `CUSTOM_METRICS_MAX_BYTES`.
- UDS fica pendente para pacote posterior porque exige ciclo de vida e permissao de socket por SO.

## Rollback

Desativar `CUSTOM_METRICS_ENABLED` ou enviar config remota `custom_metrics.enabled=false`. O coletor nao cria SQL nem altera snapshots base.

## Validacao local

```bash
go test ./internal/platform/collectors/custommetrics ./internal/common/config ./internal/domain/usecase ./internal/bootstrap
```
