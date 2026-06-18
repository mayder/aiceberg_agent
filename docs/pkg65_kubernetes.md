# PKG-65 - Kubernetes DaemonSet, Helm e autodiscovery

## Escopo entregue

O agente possui coletor `kubernetes`, desligado por padrao, que consulta a API do cluster usando a ServiceAccount do pod.

Config local:

```env
KUBERNETES_ENABLED=true
KUBERNETES_API_URL=https://kubernetes.default.svc
KUBERNETES_TOKEN_PATH=/var/run/secrets/kubernetes.io/serviceaccount/token
KUBERNETES_CA_PATH=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
KUBERNETES_NODE_NAME=<node>
KUBERNETES_NAMESPACE=<namespace opcional>
KUBERNETES_INTERVAL=30
KUBERNETES_MAX_ITEMS=500
KUBERNETES_MAX_EVENTS=100
```

Config remota:

```json
{
  "kubernetes": {
    "enabled": true,
    "api_url": "https://kubernetes.default.svc",
    "node_name": "node-a",
    "namespace": "aiceberg",
    "interval": 30,
    "max_items": 500,
    "max_events": 100
  }
}
```

## Payload

O coletor envia `body.kubernetes` para `/v1/ingest/metrics`:

- `nodes`: nome, labels/annotations sanitizadas, addresses, capacity, allocatable, conditions e nodeInfo;
- `pods`: namespace, nome, UID, node, fase, IPs, labels/annotations sanitizadas, owner e containers;
- `containers`: imagem, requests, limits, ports, ready, restart_count, state, image_id e container_id curto;
- `events`: eventos Kubernetes limitados por lote, com mensagem truncada;
- `autodiscovery_checks`: checks derivados de annotations.

Nao sao enviados secrets, env vars, volumes nem `kubectl.kubernetes.io/last-applied-configuration`.

## Autodiscovery

Annotations suportadas:

```yaml
metadata:
  annotations:
    aiceberg.ai/checks: '[{"type":"http","url":"http://%%host%%:8080/health"}]'
    aiceberg.ai/check.tcp: "8080"
    aiceberg.ai/check.openmetrics: "http://%%host%%:9100/metrics"
```

O PKG-65 materializa os templates no payload. A execucao efetiva de checks plugaveis fica ligada ao runtime do PKG-66.

## Instalacao Kubernetes

Manifest direto:

```bash
kubectl create namespace aiceberg
kubectl -n aiceberg create secret generic aiceberg-agent --from-literal=token='<TOKEN>'
kubectl apply -f deploy/kubernetes/aiceberg-agent.yaml
```

Helm:

```bash
kubectl create namespace aiceberg
kubectl -n aiceberg create secret generic aiceberg-agent --from-literal=token='<TOKEN>'
helm upgrade --install aiceberg-agent deploy/helm/aiceberg-agent -n aiceberg
```

RBAC minimo:

- `nodes`: `get`, `list`, `watch`;
- `pods`: `get`, `list`, `watch`;
- `events`: `get`, `list`, `watch`.

## Limites atuais

- Coleta uso real CPU/memoria via Metrics API ou kubelet fica pendente.
- Logs de pod/container nativos ficam pendentes para integrar com o pipeline de logs sem duplicar cursor.
- Autodiscovery executavel fica pendente do runtime de checks/plugins do PKG-66.
- Validacao real em cluster, upgrade/rollback do chart e remocao limpa ficam pendentes para PKG-69.

## Rollback

```bash
helm uninstall aiceberg-agent -n aiceberg
kubectl delete -f deploy/kubernetes/aiceberg-agent.yaml
```

Tambem e possivel desligar sem remover:

```bash
KUBERNETES_ENABLED=false
```

ou config remota `kubernetes.enabled=false`.

## Validacao local

```bash
go test ./internal/platform/collectors/kubernetes ./internal/common/config ./internal/domain/usecase ./internal/bootstrap
```
