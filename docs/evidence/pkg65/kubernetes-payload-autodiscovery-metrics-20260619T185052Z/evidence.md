# Evidencia PKG-65 - Kubernetes payload, autodiscovery e metricas

Data UTC: 20260619T185052Z

## Resultado

- Payload Kubernetes controlado valida node, pod, container, event, log de pod e autodiscovery por annotation.
- Metrics API normalizada com metricas de node e pod/container.
- Log de pod com valor sensivel foi redigido antes do payload.
- Volume sensivel presente no pod controlado nao foi coletado.
- DaemonSet, Helm, rollback e RBAC minimo reais ficam referenciados pela evidencia PKG-69 docs/evidence/pkg69/kubernetes-rbac-20260619T041959Z/evidence.md.

## Sumario

- pods_seen	1
- nodes_seen	1
- events_seen	1
- pod_logs_seen	1
- autodiscovery_checks	2
- node_metrics_seen	1
- pod_metrics_seen	1
- secret_volume_present	yes
- secret_volume_leaked	no
- log_redaction	yes
- kind_real_reference	docs/evidence/pkg69/kubernetes-rbac-20260619T041959Z/evidence.md

## Validacao executada

PKG65_EVIDENCE_DIR=/tmp/aiceberg_pkg65_kubernetes_payload_20260619T185052Z go test ./internal/platform/collectors/kubernetes -run TestPKG65KubernetesPayloadAutodiscoveryMetricsEvidence -count=1 -v

## Evidencia bruta

- Arquivo: raw/pkg65-kubernetes-payload-autodiscovery-metrics-raw.tgz
