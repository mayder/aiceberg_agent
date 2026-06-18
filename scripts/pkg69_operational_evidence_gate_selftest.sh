#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TMP_DIR="$(mktemp -d /tmp/aiceberg_pkg69_gate_selftest.XXXXXX)"
trap 'rm -rf "$TMP_DIR"' EXIT

fill_template() {
  local path="$1"
  local status="$2"
  printf 'raw evidence for %s\n' "$(basename "$path")" >"$(dirname "$path")/raw.log"
  perl -0pi -e \
    's/- Data UTC:/- Data UTC: 2026-06-18T00:00:00Z/;
     s/- Responsavel:/- Responsavel: gate-selftest/;
     s/- Cliente\/lab:/- Cliente\/lab: local/;
     s/- Host\/agente\/HUB\/relay:/- Host\/agente\/HUB\/relay: host-1/;
     s/- Versao agente:/- Versao agente: selftest/;
     s/- Artefato instalado:/- Artefato instalado: selftest.tar.gz/;
     s/- Topologia: direct\|hub\|relay -> hub -> AIceberg/- Topologia: direct\/hub\/relay hosts separados/;
     s/- Evidencia bruta anexada:/- Evidencia bruta anexada: raw.log/;
     s/- Observacoes:/- Observacoes: selftest/;
     s/- Rollback validado:/- Rollback validado: sim/;
     s/- Revisor:/- Revisor: gate-selftest/' "$path"
  perl -0pi -e "s/- Status: pending\\|pass\\|fail/- Status: $status/" "$path"
  perl -0pi -e "s/- Aprovacao fechamento: pending\\|yes\\|no/- Aprovacao fechamento: yes/" "$path"
  perl -0pi -e 's/^- ([^:\n]+):[[:space:]]*$/- $1: selftest/mg' "$path"
  perl -0pi -e \
    's/- ingest_confirmed: selftest/- ingest_confirmed: yes/g;
     s/- proc_cpu_percent: selftest/- proc_cpu_percent: 1/g;
     s/- proc_rss_bytes: selftest/- proc_rss_bytes: 1048576/g;
     s/- queue_items: selftest/- queue_items: 0/g;
     s/- containers_seen: selftest/- containers_seen: 1/g;
     s/- container_logs_seen: selftest/- container_logs_seen: 1/g;
     s/- pods_seen: selftest/- pods_seen: 1/g;
     s/- events_seen: selftest/- events_seen: 1/g;
     s/- secrets_allowed: selftest/- secrets_allowed: no/g;
     s/- exec_allowed: selftest/- exec_allowed: no/g;
     s/- delete_allowed: selftest/- delete_allowed: no/g;
     s/- requests_ok: selftest/- requests_ok: 1/g;
     s/- requests_failed_expected: selftest/- requests_failed_expected: 1/g;
     s/- retry_count: selftest/- retry_count: 1/g;
     s/- offset_ms: selftest/- offset_ms: 5000/g;
     s/- status_before: selftest/- status_before: critical/g;
     s/- status_after: selftest/- status_after: ok/g;
     s/- queued_before: selftest/- queued_before: 1/g;
     s/- replayed_after: selftest/- replayed_after: 1/g;
     s/- duplicate_count: selftest/- duplicate_count: 0/g;
     s/- free_bytes_before: selftest/- free_bytes_before: 1048576/g;
     s/- recovered: selftest/- recovered: yes/g;
     s/- accepted_count: selftest/- accepted_count: 100/g;
     s/- dropped_count: selftest/- dropped_count: 0/g;
     s/- direct -> AIceberg confirmado: selftest/- direct -> AIceberg confirmado: yes/g;
     s/- hub -> AIceberg confirmado: selftest/- hub -> AIceberg confirmado: yes/g;
     s/- relay -> hub -> AIceberg confirmado: selftest/- relay -> hub -> AIceberg confirmado: yes/g;
     s/- relay sem conexao direta com API AIceberg: selftest/- relay sem conexao direta com API AIceberg: yes/g;
     s/- agentless via Hub quando aplicavel: selftest/- agentless via Hub quando aplicavel: yes/g;
     s/- direct_ingested: selftest/- direct_ingested: yes/g;
     s/- hub_ingested: selftest/- hub_ingested: yes/g;
     s/- relay_ingested_via_hub: selftest/- relay_ingested_via_hub: yes/g;
     s/- relay_direct_api_attempts: selftest/- relay_direct_api_attempts: 0/g;
     s/- version_confirmed reportado: selftest/- version_confirmed reportado: yes/g;
     s/- update_report_status: selftest/- update_report_status: success/g' "$path"
}

mark_real_template() {
  local path="$1"
  perl -0pi -e \
    's/gate-selftest/pkg69-reviewer/g;
     s/selftest\.tar\.gz/aiceberg-agent-linux-amd64.tar.gz/g;
     s/selftest/controlled-lab/g;
     s/- Cliente\/lab: local/- Cliente\/lab: controlled-lab-69/g' "$path"
}

assert_contains() {
  local path="$1"
  local pattern="$2"
  grep -Fq "$pattern" "$path" || {
    echo "expected pattern not found: $pattern" >&2
    echo "file: $path" >&2
    exit 1
  }
}

run_with_all_evidence() {
  local evidence_file="$1"
  shift
  PKG69_EVIDENCE_FILE="$evidence_file" \
  PKG69_WINDOWS_SERVER_EVIDENCE="$TMP_DIR/all/windows_server.md" \
  PKG69_WINDOWS_DESKTOP_EVIDENCE="$TMP_DIR/all/windows_desktop.md" \
  PKG69_LINUX_DEBIAN_EVIDENCE="$TMP_DIR/all/linux_debian.md" \
  PKG69_LINUX_RHEL_EVIDENCE="$TMP_DIR/all/linux_rhel.md" \
  PKG69_DOCKER_RUNTIME_EVIDENCE="$TMP_DIR/all/docker_runtime.md" \
  PKG69_KUBERNETES_RBAC_EVIDENCE="$TMP_DIR/all/kubernetes_rbac.md" \
  PKG69_PROXY_TLS_EVIDENCE="$TMP_DIR/all/proxy_tls.md" \
  PKG69_CLOCK_SKEW_EVIDENCE="$TMP_DIR/all/clock_skew.md" \
  PKG69_PERMISSION_EBPF_EVIDENCE="$TMP_DIR/all/permission_ebpf.md" \
  PKG69_REBOOT_EVIDENCE="$TMP_DIR/all/reboot_during_collection.md" \
  PKG69_DISK_FULL_EVIDENCE="$TMP_DIR/all/disk_full.md" \
  PKG69_HIGH_VOLUME_EVIDENCE="$TMP_DIR/all/high_volume_overhead.md" \
  PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/all/relay_hub_direct_hosts.md" \
  PKG69_REMOTE_UPDATE_ROLLBACK_EVIDENCE="$TMP_DIR/all/remote_update_rollback.md" \
  "$@" scripts/pkg69_operational_evidence_gate.sh >/dev/null
}

PKG69_TEMPLATE_DIR="$TMP_DIR/templates" \
PKG69_EVIDENCE_FILE="$TMP_DIR/template-generation.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-pass.md"
fill_template "$TMP_DIR/relay-pass.md" "pass"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-fail.md"
fill_template "$TMP_DIR/relay-fail.md" "fail"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-empty-specific-field.md"
fill_template "$TMP_DIR/relay-empty-specific-field.md" "pass"
perl -0pi -e 's/- relay_direct_api_attempts: 0/- relay_direct_api_attempts:/' "$TMP_DIR/relay-empty-specific-field.md"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-direct-attempts.md"
fill_template "$TMP_DIR/relay-direct-attempts.md" "pass"
perl -0pi -e 's/- relay_direct_api_attempts: 0/- relay_direct_api_attempts: 1/' "$TMP_DIR/relay-direct-attempts.md"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-direct-api-text.md"
fill_template "$TMP_DIR/relay-direct-api-text.md" "pass"
perl -0pi -e 's/- relay sem conexao direta com API AIceberg: yes/- relay sem conexao direta com API AIceberg: no/' "$TMP_DIR/relay-direct-api-text.md"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-agentless-not-via-hub.md"
fill_template "$TMP_DIR/relay-agentless-not-via-hub.md" "pass"
perl -0pi -e 's/- agentless via Hub quando aplicavel: yes/- agentless via Hub quando aplicavel: no/' "$TMP_DIR/relay-agentless-not-via-hub.md"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-no-approval.md"
fill_template "$TMP_DIR/relay-no-approval.md" "pass"
perl -0pi -e 's/- Aprovacao fechamento: yes/- Aprovacao fechamento: no/' "$TMP_DIR/relay-no-approval.md"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-invalid-date.md"
fill_template "$TMP_DIR/relay-invalid-date.md" "pass"
perl -0pi -e 's/- Data UTC: 2026-06-18T00:00:00Z/- Data UTC: 18\/06\/2026 00:00:00/' "$TMP_DIR/relay-invalid-date.md"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-missing-artifact.md"
fill_template "$TMP_DIR/relay-missing-artifact.md" "pass"
perl -0pi -e 's/- Evidencia bruta anexada: raw.log/- Evidencia bruta anexada: missing.log/' "$TMP_DIR/relay-missing-artifact.md"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-empty-artifact.md"
fill_template "$TMP_DIR/relay-empty-artifact.md" "pass"
: >"$TMP_DIR/empty.log"
perl -0pi -e 's/- Evidencia bruta anexada: raw.log/- Evidencia bruta anexada: empty.log/' "$TMP_DIR/relay-empty-artifact.md"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-empty-dir-artifact.md"
fill_template "$TMP_DIR/relay-empty-dir-artifact.md" "pass"
mkdir -p "$TMP_DIR/emptydir"
perl -0pi -e 's/- Evidencia bruta anexada: raw.log/- Evidencia bruta anexada: emptydir/' "$TMP_DIR/relay-empty-dir-artifact.md"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-rollback-not-validated.md"
fill_template "$TMP_DIR/relay-rollback-not-validated.md" "pass"
perl -0pi -e 's/- Rollback validado: sim/- Rollback validado: no/' "$TMP_DIR/relay-rollback-not-validated.md"

cp "$TMP_DIR/templates/relay_hub_direct_hosts.md" "$TMP_DIR/relay-topology-placeholder.md"
fill_template "$TMP_DIR/relay-topology-placeholder.md" "pass"
perl -0pi -e 's/- Topologia: direct\/hub\/relay hosts separados/- Topologia: direct|hub|relay -> hub -> AIceberg/' "$TMP_DIR/relay-topology-placeholder.md"

cp "$TMP_DIR/templates/high_volume_overhead.md" "$TMP_DIR/high-volume-over-limit.md"
fill_template "$TMP_DIR/high-volume-over-limit.md" "pass"
perl -0pi -e 's/- proc_cpu_percent: 1/- proc_cpu_percent: 16/' "$TMP_DIR/high-volume-over-limit.md"

cp "$TMP_DIR/templates/kubernetes_rbac.md" "$TMP_DIR/kubernetes-secrets-allowed.md"
fill_template "$TMP_DIR/kubernetes-secrets-allowed.md" "pass"
perl -0pi -e 's/- secrets_allowed: no/- secrets_allowed: yes/' "$TMP_DIR/kubernetes-secrets-allowed.md"

cp "$TMP_DIR/templates/clock_skew.md" "$TMP_DIR/clock-invalid-status.md"
fill_template "$TMP_DIR/clock-invalid-status.md" "pass"
perl -0pi -e 's/- status_after: ok/- status_after: unknown/' "$TMP_DIR/clock-invalid-status.md"

cp "$TMP_DIR/templates/remote_update_rollback.md" "$TMP_DIR/update-invalid-status.md"
fill_template "$TMP_DIR/update-invalid-status.md" "pass"
perl -0pi -e 's/- update_report_status: success/- update_report_status: pending/' "$TMP_DIR/update-invalid-status.md"

PKG69_EVIDENCE_FILE="$TMP_DIR/correct-slot.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/correct-slot.tsv" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-pass.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/correct-slot.md" "relay-hub-direct-hosts: evidence"
assert_contains "$TMP_DIR/correct-slot.tsv" $'name\tstatus\tpath\tsha256\tbytes\tartifact_path\tartifact_sha256\tartifact_bytes\treason'
assert_contains "$TMP_DIR/correct-slot.tsv" $'relay-hub-direct-hosts\tevidence\t'
assert_contains "$TMP_DIR/correct-slot.tsv" "raw.log"

PKG69_EVIDENCE_FILE="$TMP_DIR/wrong-slot.md" \
PKG69_WINDOWS_SERVER_EVIDENCE="$TMP_DIR/relay-pass.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/wrong-slot.md" "windows-server: invalid-template"
assert_contains "$TMP_DIR/wrong-slot.md" "reason=template title mismatch"

PKG69_EVIDENCE_FILE="$TMP_DIR/fail-status.md" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-fail.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/fail-status.md" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/fail-status.md" "reason=template status is not pass"

PKG69_EVIDENCE_FILE="$TMP_DIR/empty-specific-field.md" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-empty-specific-field.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/empty-specific-field.md" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/empty-specific-field.md" "reason=template required field blank"

PKG69_EVIDENCE_FILE="$TMP_DIR/no-approval.md" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-no-approval.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/no-approval.md" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/no-approval.md" "reason=template closure approval is not yes"

PKG69_EVIDENCE_FILE="$TMP_DIR/invalid-date.md.out" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-invalid-date.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/invalid-date.md.out" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/invalid-date.md.out" "reason=field Data UTC must use UTC format YYYY-MM-DDTHH:MM:SSZ"

PKG69_EVIDENCE_FILE="$TMP_DIR/relay-direct-attempts.md.out" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-direct-attempts.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/relay-direct-attempts.md.out" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/relay-direct-attempts.md.out" "reason=field relay_direct_api_attempts must be 0"

PKG69_EVIDENCE_FILE="$TMP_DIR/relay-direct-api-text.md.out" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-direct-api-text.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/relay-direct-api-text.md.out" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/relay-direct-api-text.md.out" "reason=field relay sem conexao direta com API AIceberg must be yes, true or sim"

PKG69_EVIDENCE_FILE="$TMP_DIR/relay-agentless-not-via-hub.md.out" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-agentless-not-via-hub.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/relay-agentless-not-via-hub.md.out" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/relay-agentless-not-via-hub.md.out" "reason=field agentless via Hub quando aplicavel must be yes, true or sim"

PKG69_EVIDENCE_FILE="$TMP_DIR/relay-missing-artifact.md.out" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-missing-artifact.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/relay-missing-artifact.md.out" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/relay-missing-artifact.md.out" "reason=field Evidencia bruta anexada artifact does not exist: missing.log"

PKG69_EVIDENCE_FILE="$TMP_DIR/relay-empty-artifact.md.out" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-empty-artifact.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/relay-empty-artifact.md.out" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/relay-empty-artifact.md.out" "reason=field Evidencia bruta anexada artifact is empty: empty.log"

PKG69_EVIDENCE_FILE="$TMP_DIR/relay-empty-dir-artifact.md.out" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-empty-dir-artifact.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/relay-empty-dir-artifact.md.out" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/relay-empty-dir-artifact.md.out" "reason=field Evidencia bruta anexada artifact directory has no non-empty files: emptydir"

PKG69_EVIDENCE_FILE="$TMP_DIR/relay-rollback-not-validated.md.out" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-rollback-not-validated.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/relay-rollback-not-validated.md.out" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/relay-rollback-not-validated.md.out" "reason=field Rollback validado must be yes, true or sim"

PKG69_EVIDENCE_FILE="$TMP_DIR/relay-topology-placeholder.md.out" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/relay-topology-placeholder.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/relay-topology-placeholder.md.out" "relay-hub-direct-hosts: invalid-template"
assert_contains "$TMP_DIR/relay-topology-placeholder.md.out" "reason=field Topologia placeholder not filled"

PKG69_EVIDENCE_FILE="$TMP_DIR/high-volume-over-limit.md.out" \
PKG69_HIGH_VOLUME_EVIDENCE="$TMP_DIR/high-volume-over-limit.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/high-volume-over-limit.md.out" "high-volume-overhead: invalid-template"
assert_contains "$TMP_DIR/high-volume-over-limit.md.out" "reason=field proc_cpu_percent must be <= 15"

PKG69_EVIDENCE_FILE="$TMP_DIR/kubernetes-secrets-allowed.md.out" \
PKG69_KUBERNETES_RBAC_EVIDENCE="$TMP_DIR/kubernetes-secrets-allowed.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/kubernetes-secrets-allowed.md.out" "kubernetes-rbac: invalid-template"
assert_contains "$TMP_DIR/kubernetes-secrets-allowed.md.out" "reason=field secrets_allowed must be no"

PKG69_EVIDENCE_FILE="$TMP_DIR/clock-invalid-status.md.out" \
PKG69_CLOCK_SKEW_EVIDENCE="$TMP_DIR/clock-invalid-status.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/clock-invalid-status.md.out" "clock-skew: invalid-template"
assert_contains "$TMP_DIR/clock-invalid-status.md.out" "reason=field status_after must be one of: ok warning critical"

PKG69_EVIDENCE_FILE="$TMP_DIR/update-invalid-status.md.out" \
PKG69_REMOTE_UPDATE_ROLLBACK_EVIDENCE="$TMP_DIR/update-invalid-status.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/update-invalid-status.md.out" "remote-update-rollback: invalid-template"
assert_contains "$TMP_DIR/update-invalid-status.md.out" "reason=field update_report_status must be one of: success rolled_back apply_failed"

mkdir -p "$TMP_DIR/all"
cp "$TMP_DIR/templates/"*.md "$TMP_DIR/all/"
for template in "$TMP_DIR/all/"*.md; do
  fill_template "$template" "pass"
done

set +e
run_with_all_evidence "$TMP_DIR/real-mode-synthetic.md" env PKG69_REQUIRE_REAL_EVIDENCE=true
real_mode_synthetic_exit=$?
set -e

if [[ "$real_mode_synthetic_exit" -ne 2 ]]; then
  echo "expected real-evidence gate exit 2 with synthetic markers, got $real_mode_synthetic_exit" >&2
  exit 1
fi
assert_contains "$TMP_DIR/real-mode-synthetic.md" "must not contain self-test or synthetic markers when real evidence is required"

for template in "$TMP_DIR/all/"*.md; do
  mark_real_template "$template"
done

set +e
PKG69_EVIDENCE_FILE="$TMP_DIR/closure-missing-evidence.md" \
PKG69_REQUIRE_CLOSURE_ACCEPTED=true \
scripts/pkg69_operational_evidence_gate.sh >/dev/null 2>&1
closure_missing_evidence_exit=$?
set -e

if [[ "$closure_missing_evidence_exit" -ne 3 ]]; then
  echo "expected closure gate exit 3 without evidence, got $closure_missing_evidence_exit" >&2
  exit 1
fi

set +e
run_with_all_evidence "$TMP_DIR/closure-no-accept.md" env PKG69_REQUIRE_CLOSURE_ACCEPTED=true
closure_no_accept_exit=$?
set -e

if [[ "$closure_no_accept_exit" -ne 3 ]]; then
  echo "expected closure gate exit 3 without explicit accept, got $closure_no_accept_exit" >&2
  exit 1
fi

run_with_all_evidence "$TMP_DIR/closure-accepted.md" env PKG69_REQUIRE_CLOSURE_ACCEPTED=true PKG69_ACCEPT_CLOSURE=true
assert_contains "$TMP_DIR/closure-accepted.md" "pkg69-status: accepted-for-closure"

set +e
PKG69_EVIDENCE_FILE="$TMP_DIR/blocking.md" \
PKG69_REQUIRE_REAL_EVIDENCE=true \
scripts/pkg69_operational_evidence_gate.sh >/dev/null 2>&1
blocking_exit=$?
set -e

if [[ "$blocking_exit" -ne 2 ]]; then
  echo "expected blocking gate exit 2, got $blocking_exit" >&2
  exit 1
fi

echo "PKG-69 evidence gate self-test OK"
