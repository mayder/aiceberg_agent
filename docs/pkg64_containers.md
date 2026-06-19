# PKG-64 - Containers Docker inicial

## Escopo entregue

O agente possui coletor `containers`, desligado por padrao, que usa Docker API via socket Unix local e pode detectar containerd via `ctr` local quando configurado.

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
- containerd: id, nome, imagem, labels e namespace quando `ctr containers info` retornar metadados.

## Configuracao

```env
CONTAINER_ENABLED=true
CONTAINER_RUNTIME=auto
CONTAINER_DOCKER_SOCKET=/var/run/docker.sock
CONTAINER_CONTAINERD_SOCKET=/run/containerd/containerd.sock
CONTAINER_CONTAINERD_NAMESPACE=k8s.io
CONTAINER_CTR_PATH=ctr
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
    "runtime": "auto",
    "docker_socket": "/var/run/docker.sock",
    "containerd_socket": "/run/containerd/containerd.sock",
    "containerd_namespace": "k8s.io",
    "ctr_path": "ctr",
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
- `source=containerd_ctr` quando o runtime efetivo for containerd;
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

Cada check recebe `container_id`, `container_name`, `image` e `service` quando existir label de compose/swarm. O payload tambem sai normalizado como check local canonico (`kind`, `target`, `enabled`, `tags`) para execucao segura pelo runtime de checks/plugins, sem shell remoto.

## Segurança

- Desligado por padrao.
- Labels com `secret`, `token` ou `password` sao mascaradas.
- Nao coleta env vars nem conteudo de volumes.
- Acesso ao Docker socket deve ser concedido explicitamente pelo operador.
- Acesso ao socket containerd e ao binario `ctr` deve ser concedido explicitamente pelo operador.
- Config remota de runtime, socket containerd ou caminho `ctr` e tratada como sensivel e deve seguir a politica de assinatura do agente.
- `include_regex` e `exclude_regex` atuam sobre id, nome, imagem, labels, namespace local, service e user.

## Limites

- Metricas/logs containerd nativos e envio de logs por rota dedicada ficam para hardening futuro se o pipeline exigir.
- Sem leitura de secrets montados.

## Rollback

Definir `CONTAINER_ENABLED=false` ou config remota `containers.enabled=false`. Nao ha SQL do PKG-64.

## Validacao local

```bash
go test ./internal/platform/collectors/containers ./internal/common/config ./internal/domain/usecase ./internal/bootstrap
```

## Evidencia de fechamento

Bundle aceito: `docs/evidence/pkg64/container-lifecycle-autodiscovery-secret-20260619T184258Z`.

Cobertura:

- containers `running`, `restarted` e `exited`;
- alta carga normalizada em CPU, memoria e rede;
- `restart_count` preservado;
- autodiscovery de check por labels em container novo;
- env sensivel e volume de segredo presentes no inspect controlado nao aparecem no payload;
- Docker real referenciado pelo bundle PKG-69 `docs/evidence/pkg69/docker-runtime-20260619T031843Z/evidence.md`.
