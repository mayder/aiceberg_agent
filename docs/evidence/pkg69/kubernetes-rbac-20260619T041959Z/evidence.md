# PKG-69 - Kubernetes RBAC

## Ambiente

- Data UTC: 2026-06-19T04:18:42Z
- Responsavel: Codex
- Cliente/lab: kind local controlado aiceberg-pkg69-rbac
- Ambiente: Kubernetes kind v1.34.0 com Helm v3.16.4
- Host/agente/HUB/relay: DaemonSet aiceberg-agent em namespace aiceberg
- Versao agente: 0.8.8-pkg69-k8s
- Artefato instalado: imagem local aiceberg-agent:pkg69-k8s carregada no kind, chart deploy/helm/aiceberg-agent
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- DaemonSet aplicado: yes, daemonset/aiceberg-agent com 1/1 pod Running e rollout OK
- Helm install/upgrade/rollback: yes, revisoes Helm 1 install, 2 upgrade e 3 rollback para 1 concluídas
- ServiceAccount com RBAC minimo: yes, ClusterRole permite somente leitura para nodes/pods/events e pods/log
- pods/events/logs coletados: yes, kubectl coletou pods, events e logs redigidos do pod do agente
- sem permissao a secrets/exec/delete: yes, auth can-i retornou no para secrets, pods/exec e delete pods

## Metricas

- pods_seen: 1
- events_seen: 26
- proc_cpu_percent: 0.2
- proc_rss_bytes: 15760
- secrets_allowed: no
- exec_allowed: no
- delete_allowed: no

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/aiceberg_pkg69_kubernetes_rbac_raw_20260619T041625Z.tgz
- Observacoes: Cluster kind efemero criado via Docker local; secret real nao foi exportado, apenas manifesto redigido. API do agente apontou para 127.0.0.1:9 para evitar envio externo; a prova valida DaemonSet, Helm, RBAC minimo, pod Running, events e logs redigidos.
- Rollback validado: yes
- Revisor: Codex
- Aprovacao fechamento: yes
