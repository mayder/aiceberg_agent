# Evidencia PKG-64 - Containers lifecycle, autodiscovery e segredo

Data UTC: 20260619T184258Z

## Resultado

- Coletor de containers validado com payload controlado do contrato Docker.
- Cenario cobre containers running, restarted e exited.
- Metricas de alta carga foram normalizadas com CPU, memoria e rede.
- Labels de autodiscovery geraram checks canonicos sem reiniciar agente.
- Env sensivel e volume de segredo presentes no inspect controlado nao foram coletados no payload.
- Docker real fica referenciado pela evidencia PKG-69 docs/evidence/pkg69/docker-runtime-20260619T031843Z/evidence.md.

## Sumario

- containers_seen	3
- running_seen	2
- stopped_seen	1
- restarted_seen	1
- high_load_seen	yes
- autodiscovery_checks	2
- new_container_check	yes
- sensitive_env_collected	no
- sensitive_volume_collected	no
- redaction_or_omission	yes
- docker_real_reference	docs/evidence/pkg69/docker-runtime-20260619T031843Z/evidence.md

## Validacao executada

PKG64_EVIDENCE_DIR=/tmp/aiceberg_pkg64_container_lifecycle_20260619T184258Z go test ./internal/platform/collectors/containers -run TestPKG64ContainerLifecycleAutodiscoverySecretEvidence -count=1 -v

## Evidencia bruta

- Arquivo: raw/pkg64-container-lifecycle-autodiscovery-secret-raw.tgz
