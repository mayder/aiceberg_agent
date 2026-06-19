# PKG-61 - Metricas custom locais

## Escopo entregue

O agente possui um coletor `custommetrics`, desligado por padrao, que recebe metricas locais sem expor token/API key para aplicacoes.

Entradas suportadas:

- UDP local DogStatsD-like em `CUSTOM_METRICS_UDP_ADDR`;
- Unix Domain Socket DogStatsD-like em `CUSTOM_METRICS_UDS_PATH`, quando suportado pelo SO;
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

No web, o detalhe do agente mostra **Métricas custom locais** a partir desse payload preservado em `raw_json`, com aceitas/descartadas, janela, tipos, service/env, tags e séries recentes. Não há SQL neste pacote.

## Configuracao

```env
CUSTOM_METRICS_ENABLED=true
CUSTOM_METRICS_UDP_ADDR=127.0.0.1:8125
CUSTOM_METRICS_UDS_PATH=/var/run/aiceberg/custommetrics.sock
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
    "uds_path": "/var/run/aiceberg/custommetrics.sock",
    "http_addr": "127.0.0.1:8126",
    "interval": 10,
    "max_series": 1000,
    "max_bytes": 65536
  }
}
```

## Limites

- O receptor aceita apenas origem loopback.
- O UDS cria socket com permissao `0600` e remove o arquivo ao encerrar.
- Cardinalidade acima de `CUSTOM_METRICS_MAX_SERIES` e contada em `dropped_count`.
- HTTP tem limite por corpo em `CUSTOM_METRICS_MAX_BYTES`.
- UDS depende de suporte do sistema operacional; evidencia local controlada valida socket `0600` e remocao no shutdown, e a validacao operacional de alto volume fica registrada nos bundles PKG-61/PKG-69.

## Evidencia operacional

- `docs/evidence/pkg61/local-app-high-volume-20260619T181900Z/evidence.md`: app local controlada, HTTP, UDP DogStatsD-like, UDS `0600`, tags `service/env`, limite `CUSTOM_METRICS_MAX_SERIES`, `accepted_count` e `dropped_count`.
- `docs/evidence/pkg69/high-volume-overhead-20260619T033436Z/evidence.md`: DogStatsD alto volume com CPU/memoria dentro do limite no probe conjunto DogStatsD/OTLP/logs.

## Rollback

Desativar `CUSTOM_METRICS_ENABLED` ou enviar config remota `custom_metrics.enabled=false`. O coletor nao cria SQL nem altera snapshots base.

## Validacao local

```bash
go test ./internal/platform/collectors/custommetrics ./internal/common/config ./internal/domain/usecase ./internal/bootstrap
PKG61_EVIDENCE_DIR=/tmp/aiceberg_pkg61_local_app_high_volume go test ./internal/platform/collectors/custommetrics -run TestPKG61LocalAppHighVolumeEvidence -count=1 -v
```
