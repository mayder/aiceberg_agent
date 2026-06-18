# PKG-64 - Containers Docker inicial

## Escopo entregue

O agente possui coletor `containers`, desligado por padrao, que usa Docker API via socket Unix local.

Dados coletados:

- container id, nome, imagem, estado e status;
- labels sanitizadas;
- rede, portas e network mode;
- compose/swarm service quando houver label;
- namespace local quando houver label de compose, Kubernetes ou `aiceberg.ai/namespace`;
- user, restart_count e log_path quando `/containers/<id>/json` responder;
- CPU, memoria, rede e IO quando `/stats?stream=false` responder.
- checks de autodiscovery derivados de labels.
- logs Docker JSON por `log_path`, com cursor, tags de container e redaction.

## Configuracao

```env
CONTAINER_ENABLED=true
CONTAINER_DOCKER_SOCKET=/var/run/docker.sock
CONTAINER_INTERVAL=30
CONTAINER_MAX_ITEMS=200
CONTAINER_INCLUDE_REGEX=prod|backend
CONTAINER_EXCLUDE_REGEX=root|secret
CONTAINER_LOGS_ENABLED=true
CONTAINER_LOGS_CURSOR_PATH=./data/container_logs.cursor
CONTAINER_LOGS_MAX_LINES=200
CONTAINER_LOGS_MAX_BYTES=262144
```

Config remota equivalente:

```json
{
  "containers": {
    "enabled": true,
    "docker_socket": "/var/run/docker.sock",
    "interval": 30,
    "max_items": 200,
    "include_regex": "prod|backend",
    "exclude_regex": "root|secret",
    "logs_enabled": true,
    "logs_max_lines": 200,
    "logs_max_bytes": 262144
  }
}
```

## Contrato enviado

O coletor envia `body.containers` para `/v1/ingest/metrics`:

- `schema_version`;
- `source=docker_socket`;
- `items[]`;
- `logs.events[]`;
- `autodiscovery_checks`;
- `dropped_count`.

`logs.events[]` contem `container_id`, `container_name`, `image`, `service`, `namespace`, `stream`, `timestamp_utc`, `message`, `redaction_status`, `transport=docker_json_file` e metadados de origem.

## Autodiscovery

Labels suportadas:

```yaml
labels:
  aiceberg.ai/checks: '[{"type":"http","url":"http://%%host%%:8080/health"}]'
  aiceberg.ai/check.tcp: "8080"
  aiceberg.ai/check.openmetrics: "http://%%host%%:9100/metrics"
```

Cada check recebe `container_id`, `container_name`, `image` e `service` quando existir label de compose/swarm. A execucao efetiva fica ligada ao runtime de checks/plugins.

## Segurança

- Desligado por padrao.
- Labels com `secret`, `token` ou `password` sao mascaradas.
- Nao coleta env vars nem conteudo de volumes.
- Acesso ao Docker socket deve ser concedido explicitamente pelo operador.
- `include_regex` e `exclude_regex` atuam sobre id, nome, imagem, labels, namespace local, service e user.

## Limites

- Containerd nativo, envio de logs por rota dedicada e validacao real de carga ficam pendentes.
- Sem leitura de secrets montados.

## Rollback

Definir `CONTAINER_ENABLED=false` ou config remota `containers.enabled=false`. Nao ha SQL do PKG-64.

## Validacao local

```bash
go test ./internal/platform/collectors/containers ./internal/common/config ./internal/domain/usecase ./internal/bootstrap
```
