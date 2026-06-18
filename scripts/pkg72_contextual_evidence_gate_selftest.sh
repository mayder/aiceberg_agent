#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TMP_DIR="$(mktemp -d /tmp/aiceberg_pkg72_gate_selftest.XXXXXX)"
trap 'rm -rf "$TMP_DIR"' EXIT

fill_template() {
  local path="$1"
  local status="$2"
  perl -0pi -e \
    's/- Data UTC:/- Data UTC: 2026-06-18T00:00:00Z/;
     s/- Responsavel:/- Responsavel: gate-selftest/;
     s/- Cliente\/lab:/- Cliente\/lab: local/;
     s/- Host\/agente\/HUB\/relay:/- Host\/agente\/HUB\/relay: host-1/;
     s/- Versao agente:/- Versao agente: selftest/;
     s/- Evidencia bruta anexada:/- Evidencia bruta anexada: raw.log/;
     s/- Observacoes:/- Observacoes: selftest/;
     s/- Rollback validado:/- Rollback validado: sim/;
     s/- Revisor:/- Revisor: gate-selftest/' "$path"
  perl -0pi -e "s/- Status: pending\\|pass\\|fail/- Status: $status/" "$path"
  perl -0pi -e "s/- Aprovacao fechamento: pending\\|yes\\|no/- Aprovacao fechamento: yes/" "$path"
  perl -0pi -e 's/^- ([^:\n]+):[[:space:]]*$/- $1: selftest/mg' "$path"
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
  PKG72_EVIDENCE_FILE="$evidence_file" \
  PKG72_INCIDENT_EVIDENCE="$TMP_DIR/all/noc_soc_incident_host_agentless.md" \
  PKG72_REPLAY_24H_EVIDENCE="$TMP_DIR/all/offline_replay_24h.md" \
  PKG72_REGULATED_CLIENT_EVIDENCE="$TMP_DIR/all/regulated_client_minimal_collection.md" \
  PKG72_NOISE_COST_EVIDENCE="$TMP_DIR/all/noise_cost_before_after.md" \
  PKG72_DATADOG_BENCHMARK_EVIDENCE="$TMP_DIR/all/datadog_scenario_benchmark.md" \
  "$@" scripts/pkg72_contextual_evidence_homologation.sh >/dev/null
}

PKG72_TEMPLATE_DIR="$TMP_DIR/templates" \
PKG72_EVIDENCE_FILE="$TMP_DIR/template-generation.md" \
scripts/pkg72_contextual_evidence_homologation.sh >/dev/null

cp "$TMP_DIR/templates/noc_soc_incident_host_agentless.md" "$TMP_DIR/noc-pass.md"
fill_template "$TMP_DIR/noc-pass.md" "pass"

cp "$TMP_DIR/templates/noc_soc_incident_host_agentless.md" "$TMP_DIR/noc-fail.md"
fill_template "$TMP_DIR/noc-fail.md" "fail"

cp "$TMP_DIR/templates/noc_soc_incident_host_agentless.md" "$TMP_DIR/noc-empty-specific-field.md"
fill_template "$TMP_DIR/noc-empty-specific-field.md" "pass"
perl -0pi -e 's/- Evidencia host local: selftest/- Evidencia host local:/' "$TMP_DIR/noc-empty-specific-field.md"

cp "$TMP_DIR/templates/noc_soc_incident_host_agentless.md" "$TMP_DIR/noc-no-approval.md"
fill_template "$TMP_DIR/noc-no-approval.md" "pass"
perl -0pi -e 's/- Aprovacao fechamento: yes/- Aprovacao fechamento: no/' "$TMP_DIR/noc-no-approval.md"

PKG72_EVIDENCE_FILE="$TMP_DIR/correct-slot.md" \
PKG72_EVIDENCE_MANIFEST_TSV="$TMP_DIR/correct-slot.tsv" \
PKG72_INCIDENT_EVIDENCE="$TMP_DIR/noc-pass.md" \
scripts/pkg72_contextual_evidence_homologation.sh >/dev/null
assert_contains "$TMP_DIR/correct-slot.md" "noc-soc-incident-host-agentless: evidence"
assert_contains "$TMP_DIR/correct-slot.tsv" $'name\tstatus\tpath\tsha256\tbytes\treason'
assert_contains "$TMP_DIR/correct-slot.tsv" $'noc-soc-incident-host-agentless\tevidence\t'

PKG72_EVIDENCE_FILE="$TMP_DIR/wrong-slot.md" \
PKG72_DATADOG_BENCHMARK_EVIDENCE="$TMP_DIR/noc-pass.md" \
scripts/pkg72_contextual_evidence_homologation.sh >/dev/null
assert_contains "$TMP_DIR/wrong-slot.md" "datadog-scenario-benchmark: invalid-template"
assert_contains "$TMP_DIR/wrong-slot.md" "reason=template title mismatch"

PKG72_EVIDENCE_FILE="$TMP_DIR/fail-status.md" \
PKG72_INCIDENT_EVIDENCE="$TMP_DIR/noc-fail.md" \
scripts/pkg72_contextual_evidence_homologation.sh >/dev/null
assert_contains "$TMP_DIR/fail-status.md" "noc-soc-incident-host-agentless: invalid-template"
assert_contains "$TMP_DIR/fail-status.md" "reason=template status is not pass"

PKG72_EVIDENCE_FILE="$TMP_DIR/empty-specific-field.md" \
PKG72_INCIDENT_EVIDENCE="$TMP_DIR/noc-empty-specific-field.md" \
scripts/pkg72_contextual_evidence_homologation.sh >/dev/null
assert_contains "$TMP_DIR/empty-specific-field.md" "noc-soc-incident-host-agentless: invalid-template"
assert_contains "$TMP_DIR/empty-specific-field.md" "reason=template required field blank"

PKG72_EVIDENCE_FILE="$TMP_DIR/no-approval.md" \
PKG72_EVIDENCE_MANIFEST_TSV="$TMP_DIR/no-approval.tsv" \
PKG72_INCIDENT_EVIDENCE="$TMP_DIR/noc-no-approval.md" \
scripts/pkg72_contextual_evidence_homologation.sh >/dev/null
assert_contains "$TMP_DIR/no-approval.md" "noc-soc-incident-host-agentless: invalid-template"
assert_contains "$TMP_DIR/no-approval.md" "reason=template closure approval is not yes"
assert_contains "$TMP_DIR/no-approval.tsv" $'noc-soc-incident-host-agentless\tinvalid-template\t'
assert_contains "$TMP_DIR/no-approval.tsv" "template closure approval is not yes"

mkdir -p "$TMP_DIR/all"
cp "$TMP_DIR/templates/"*.md "$TMP_DIR/all/"
for template in "$TMP_DIR/all/"*.md; do
  fill_template "$template" "pass"
done

set +e
PKG72_EVIDENCE_FILE="$TMP_DIR/closure-missing-evidence.md" \
PKG72_REQUIRE_CLOSURE_ACCEPTED=true \
scripts/pkg72_contextual_evidence_homologation.sh >/dev/null 2>&1
closure_missing_evidence_exit=$?
set -e

if [[ "$closure_missing_evidence_exit" -ne 3 ]]; then
  echo "expected closure gate exit 3 without evidence, got $closure_missing_evidence_exit" >&2
  exit 1
fi

set +e
run_with_all_evidence "$TMP_DIR/closure-no-accept.md" env PKG72_REQUIRE_CLOSURE_ACCEPTED=true
closure_no_accept_exit=$?
set -e

if [[ "$closure_no_accept_exit" -ne 3 ]]; then
  echo "expected closure gate exit 3 without explicit accept, got $closure_no_accept_exit" >&2
  exit 1
fi

run_with_all_evidence "$TMP_DIR/closure-accepted.md" env PKG72_REQUIRE_CLOSURE_ACCEPTED=true PKG72_ACCEPT_CLOSURE=true
assert_contains "$TMP_DIR/closure-accepted.md" "pkg72-status: accepted-for-closure"

set +e
PKG72_EVIDENCE_FILE="$TMP_DIR/blocking.md" \
PKG72_REQUIRE_REAL_EVIDENCE=true \
scripts/pkg72_contextual_evidence_homologation.sh >/dev/null 2>&1
blocking_exit=$?
set -e

if [[ "$blocking_exit" -ne 2 ]]; then
  echo "expected blocking gate exit 2, got $blocking_exit" >&2
  exit 1
fi

echo "PKG-72 evidence gate self-test OK"
