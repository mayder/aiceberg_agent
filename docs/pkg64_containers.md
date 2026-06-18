# PKG-64 - Containers Docker inicial

## Escopo entregue

O agente possui coletor `containers`, desligado por padrao, que usa Docker API via socket Unix local.

Dados coletados:

- container id, nome, imagem, estado e status;
- labels sanitizadas;
- rede, portas e network mode;
- compose/swarm service quando houver label;
- CPU, memoria, rede e IO quando `/stats?stream=false` responder.

## Configuracao

```env
CONTAINER_ENABLED=true
CONTAINER_DOCKER_SOCKET=/var/run/docker.sock
CONTAINER_INTERVAL=30
CONTAINER_MAX_ITEMS=200
```

Config remota equivalente:

```json
{
  "containers": {
    "enabled": true,
    "docker_socket": "/var/run/docker.sock",
    "interval": 30,
    "max_items": 200
  }
}
```

## Contrato enviado

O coletor envia `body.containers` para `/v1/ingest/metrics`:

- `schema_version`;
- `source=docker_socket`;
- `items[]`;
- `dropped_count`.

## Segurança

- Desligado por padrao.
- Labels com `secret`, `token` ou `password` sao mascaradas.
- Nao coleta env vars nem conteudo de volumes.
- Acesso ao Docker socket deve ser concedido explicitamente pelo operador.

## Limites

- Containerd nativo, logs de container, autodiscovery de checks e validacao real de carga ficam pendentes.
- Sem leitura de secrets montados.

## Rollback

Definir `CONTAINER_ENABLED=false` ou config remota `containers.enabled=false`. Nao ha SQL do PKG-64.

## Validacao local

```bash
go test ./internal/platform/collectors/containers ./internal/common/config ./internal/domain/usecase ./internal/bootstrap
```
