# Evidencia PKG-66 - Local checks lifecycle, rollback e upgrade

Data UTC: 20260619T185846Z

## Resultado

- Runtime localchecks executou checks HTTP e OpenMetrics em loopback.
- Cenario cobre check OK, check com falha, tipo bloqueado por allowlist e rollback por config remota desligada.
- Upgrade de manifest instalavel adicionou nova versao sem perder a configuracao dos checks.
- Target com valor sensivel e credentials_ref nao vazaram no payload.

## Sumario

- checks_created	4
- checks_executed	4
- ok_seen	yes
- failure_seen	yes
- blocked_kind_seen	yes
- rollback_disabled	yes
- manifest_versions	2
- config_preserved	yes
- credential_ref_leaked	no
- target_secret_leaked	no

## Validacao executada

PKG66_EVIDENCE_DIR=/tmp/aiceberg_pkg66_localchecks_lifecycle_20260619T185846Z go test ./internal/platform/collectors/localchecks -run TestPKG66LocalChecksLifecycleRollbackUpgradeEvidence -count=1 -v

## Evidencia bruta

- Arquivo: raw/pkg66-localchecks-lifecycle-rollback-upgrade-raw.tgz
