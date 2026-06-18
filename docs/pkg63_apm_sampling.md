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
