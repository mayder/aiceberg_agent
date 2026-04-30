# PKG-33 - Flush resiliente HUB/Relay

## Configuracao

- `INGEST_TIMEOUT_SEC`: timeout do POST para API ou Hub. Default: `10`.
- `OUTBOX_FLUSH_BATCH`: tamanho maximo lido da outbox por ciclo. Default: `50`.
- `OUTBOX_FLUSH_INTERVAL`: intervalo do flush principal. Default: `15`.

Os defaults preservam o comportamento anterior. Em Hub com backlog alto, reduza primeiro `OUTBOX_FLUSH_BATCH` e aumente `OUTBOX_FLUSH_INTERVAL`; nao desligue coletas.

## Diagnostico

No Hub ou Relay:

```bash
curl -s http://127.0.0.1:8081/health | jq .
curl -s http://127.0.0.1:8081/metrics
journalctl -u aiceberg-agent --since "30 min ago" | rg "transport failed|transport backoff active|flushed"
```

Campos principais:

- `queue_items` / `queue_bytes`: backlog local.
- `flush_err` / `flush_ok`: tendencia de falhas e sucesso.
- `flush_detail.last_error_route`: endpoint que falhou por ultimo.
- `flush_detail.last_ack_route`: endpoint confirmado por ultimo.
- `flush_detail.last_retained`: itens mantidos para retry no ultimo ciclo.
- `flush_detail.oldest_pending_age_sec`: idade aproximada do item mais antigo lido no lote.
- `ingest_timeout_sec`, `flush_interval_sec`, `flush_batch_limit`: configuracao efetiva.

## Comportamento esperado

- Falha temporaria em `/v1/ingest/metrics` aplica backoff apenas para o grupo `authHeader + endpoint`.
- Grupos saudaveis, como `/v1/ingest/health`, `/v1/ingest/bootstrap` e `/v1/ingest/inventory`, continuam sendo enviados e confirmados.
- Timeout temporario nunca descarta envelope.
- Respostas da API com `retry_after`, `suggested_batch_size` e `degraded_endpoint` podem orientar backoff do Hub sem quebrar agentes antigos.

## Rollback

Rollback operacional sem trocar binario:

1. Restaurar `INGEST_TIMEOUT_SEC=10`, `OUTBOX_FLUSH_BATCH=50`, `OUTBOX_FLUSH_INTERVAL=15`.
2. Reiniciar o servico do agente.
3. Confirmar em `/health` que os valores efetivos voltaram ao padrao.

Rollback de versao segue o gate de `GOVERNANCA.md`: publicar artefato anterior em `cliente/web/downloads/agent/<versao>/` e acionar update remoto somente depois de validar checksum e disponibilidade.
