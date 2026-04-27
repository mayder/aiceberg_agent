# PKG-32 Lote 20 - Diagnostico local e health do canal

## Health local

Quando `HEALTH_PORT` estiver ativo, `GET /health` inclui o bloco `channel`:

- `enabled`: canal aplicavel ao modo atual.
- `mode`: `direct`, `hub` ou `relay`.
- `endpoint`: destino usado pelo canal.
- `fallback_active`: `true` quando o agente esta operando por polling/fallback.
- `connected`: sessao do canal aberta.
- `session_id`: ultima sessao conhecida.
- `last_heartbeat_utc`: ultimo heartbeat aceito.
- `last_error`: ultimo erro de abertura/heartbeat.
- `reconnect_retries`: quantidade de falhas/reconexoes registradas.
- `last_latency_ms`: latencia do ultimo open/heartbeat.
- `hub_url_configured`: `HUB_URL` presente.
- `relay_uses_hub_url`: relay usando hub em vez de AIceberg direto.
- `connects_to_aiceberg`: modo conecta diretamente ao AIceberg.
- `relay_connects_to_aiceberg`: deve permanecer `false`.

## Doctor local

Executar:

```bash
./bin/aiceberg-agent -doctor -config configs/agent.env
```

O comando imprime JSON e encerra sem iniciar o servico. Exit code:

- `0`: diagnostico `ok` ou `warn`.
- `1`: diagnostico `fail`.

## Checks cobertos

- `direct`: valida topologia `direct -> AIceberg`.
- `hub`: valida topologia `hub -> AIceberg` e avisa quando `HUB_LISTEN_ADDR` usa padrao.
- `relay`: valida topologia `relay -> hub -> AIceberg` e exige `HUB_URL`.
- `agent_token`: falha quando `AGENT_TOKEN` esta ausente.
- `api_reachable`: testa alcance basico do `API_BASE_URL`.
- `hub_reachable`: em relay, testa alcance basico do `HUB_URL`.

Status HTTP `2xx/3xx` e considerado `ok`; `4xx` e `warn` porque a rede respondeu, mas pode haver auth/rota; `5xx` ou erro de rede e `fail`.

## Troubleshooting

- `agent_token=fail`: configurar `AGENT_TOKEN` ou arquivo de token antes de iniciar o servico.
- `hub_url=fail` em relay: configurar `HUB_URL`; relay nao deve abrir canal direto com AIceberg.
- `api_reachable=fail`: revisar DNS, proxy, firewall, certificado TLS e `API_BASE_URL`.
- `fallback_active=true` no `/health`: o canal caiu ou ainda nao abriu; conferir `last_error`, `reconnect_retries` e conectividade.
- `relay_connects_to_aiceberg=true`: estado invalido; rollback/revisar configuracao, pois relay deve falar somente com hub.

## Rollback

Remover o uso de `Snapshot()` no `/health` e desabilitar a flag `-doctor`. Nao ha SQL nem estado persistente novo.
