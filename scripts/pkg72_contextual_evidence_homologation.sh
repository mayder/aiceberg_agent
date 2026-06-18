#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

EVIDENCE_FILE="${PKG72_EVIDENCE_FILE:-/tmp/aiceberg_pkg72_contextual_evidence.md}"
GO_TEST_LOG="${PKG72_GO_TEST_LOG:-/tmp/aiceberg_pkg72_go_test.log}"
REQUIRE_REAL_EVIDENCE="${PKG72_REQUIRE_REAL_EVIDENCE:-false}"
REAL_EVIDENCE_PRESENT=0
REAL_EVIDENCE_TOTAL=0

section() {
  printf '\n## %s\n\n' "$1" | tee -a "$EVIDENCE_FILE" >/dev/null
}

result() {
  local name="$1"
  local status="$2"
  local detail="$3"
  printf -- '- %s: %s — %s\n' "$name" "$status" "$detail" | tee -a "$EVIDENCE_FILE" >/dev/null
}

file_size_bytes() {
  wc -c <"$1" | tr -d ' '
}

file_sha256() {
  shasum -a 256 "$1" | awk '{print $1}'
}

require_file_or_pending() {
  local name="$1"
  local path="$2"
  local detail="$3"
  REAL_EVIDENCE_TOTAL=$((REAL_EVIDENCE_TOTAL + 1))
  if [[ -n "$path" && -f "$path" ]]; then
    REAL_EVIDENCE_PRESENT=$((REAL_EVIDENCE_PRESENT + 1))
    result "$name" "evidence" "path=$path sha256=$(file_sha256 "$path") bytes=$(file_size_bytes "$path")"
  else
    result "$name" "pending" "$detail"
  fi
}

: >"$EVIDENCE_FILE"

section "PKG-72 contextual evidence homologation"
result "repo" "info" "$ROOT"
result "os" "info" "$(uname -srm)"
result "go" "info" "$(go version 2>/dev/null || echo 'go indisponivel')"

section "focused local validation"
go test ./internal/bootstrap ./internal/data/local/outbox >"$GO_TEST_LOG"
result "go-test-focused" "pass" "log=$GO_TEST_LOG"
result "contextual-evidence" "pass" "snapshot exposes contextual_evidence, local AI guardrails, privacy, offline-first and benchmark gate"
result "relay-topology" "pass" "tests keep relay_to_hub_only=true and direct_api_from_relay=false in relay mode"
result "offline-replay-local" "pass" "BoltStore keeps envelopes until ACK and ACK is idempotent"
result "superiority-claim" "pass" "claim_allowed=false and benchmark status remains pending_evidence"

section "required real evidence"
require_file_or_pending "noc-soc-incident-host-agentless" "${PKG72_INCIDENT_EVIDENCE:-}" "set PKG72_INCIDENT_EVIDENCE to a controlled incident evidence file with host + Agentless correlation"
require_file_or_pending "offline-replay-24h" "${PKG72_REPLAY_24H_EVIDENCE:-}" "set PKG72_REPLAY_24H_EVIDENCE to a 24h offline/replay evidence file with duplicate-rate analysis"
require_file_or_pending "regulated-client-minimal-collection" "${PKG72_REGULATED_CLIENT_EVIDENCE:-}" "set PKG72_REGULATED_CLIENT_EVIDENCE to a regulated-client reduced-collection validation file"
require_file_or_pending "noise-cost-before-after" "${PKG72_NOISE_COST_EVIDENCE:-}" "set PKG72_NOISE_COST_EVIDENCE to before/after noise and cost comparison"
require_file_or_pending "datadog-scenario-benchmark" "${PKG72_DATADOG_BENCHMARK_EVIDENCE:-}" "set PKG72_DATADOG_BENCHMARK_EVIDENCE to scenario-matched Datadog comparison evidence"

section "benchmark scenarios"
result "noc_soc_context" "pending-real" "measure time_to_diagnosis, evidence_completeness and operator_steps against comparable Datadog scenario"
result "sovereign_offline" "pending-real" "measure offline_replay_success, duplicate_rate and support_export_integrity"
result "agent_plus_agentless" "pending-real" "measure correlation_detected, false_positive_rate and agentless_observation_link"
result "noise_reduction" "pending-real" "measure noise_before, noise_after and manual_review_required"

section "closure rule"
if [[ "$REAL_EVIDENCE_PRESENT" -eq "$REAL_EVIDENCE_TOTAL" ]]; then
  result "real-evidence-manifest" "ready-for-review" "$REAL_EVIDENCE_PRESENT/$REAL_EVIDENCE_TOTAL files present with SHA256; manual review and benchmark acceptance still required"
else
  result "real-evidence-manifest" "incomplete" "$REAL_EVIDENCE_PRESENT/$REAL_EVIDENCE_TOTAL files present"
fi
result "pkg72-status" "not-closed" "do not mark PKG-72 100% until every required real evidence item above is present, reviewed and accepted"
result "evidence-file" "written" "$EVIDENCE_FILE"

if [[ "$REQUIRE_REAL_EVIDENCE" == "true" && "$REAL_EVIDENCE_PRESENT" -ne "$REAL_EVIDENCE_TOTAL" ]]; then
  result "gate" "failed" "PKG72_REQUIRE_REAL_EVIDENCE=true and real evidence manifest is incomplete"
  exit 2
fi
