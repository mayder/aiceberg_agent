#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

EVIDENCE_FILE="${PKG72_EVIDENCE_FILE:-/tmp/aiceberg_pkg72_contextual_evidence.md}"
GO_TEST_LOG="${PKG72_GO_TEST_LOG:-/tmp/aiceberg_pkg72_go_test.log}"
REQUIRE_REAL_EVIDENCE="${PKG72_REQUIRE_REAL_EVIDENCE:-false}"
TEMPLATE_DIR="${PKG72_TEMPLATE_DIR:-}"
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

write_template() {
  local file="$1"
  local title="$2"
  local required="$3"
  local metrics="$4"
  if [[ -f "$file" ]]; then
    return
  fi
  cat >"$file" <<EOF
# $title

## Ambiente

- Data UTC:
- Responsavel:
- Cliente/lab:
- Host/agente/HUB/relay:
- Versao agente:
- Topologia: direct|hub|relay -> hub -> AIceberg

## Evidencia obrigatoria

$required

## Metricas

$metrics

## Resultado

- Status: pending|pass|fail
- Evidencia bruta anexada:
- Observacoes:
- Rollback validado:
EOF
}

generate_templates() {
  if [[ -z "$TEMPLATE_DIR" ]]; then
    return
  fi
  mkdir -p "$TEMPLATE_DIR"
  write_template "$TEMPLATE_DIR/noc_soc_incident_host_agentless.md" \
    "PKG-72 - Incidente NOC/SOC com host + Agentless" \
    "- Evidencia host local
- Observacao Agentless correlata
- Timestamp comum
- Link ou ID do incidente NOC/SOC
- Lacunas reportadas pelo agente" \
    "- time_to_diagnosis
- evidence_completeness
- operator_steps"
  write_template "$TEMPLATE_DIR/offline_replay_24h.md" \
    "PKG-72 - Replay offline 24h" \
    "- Inicio/fim da janela offline
- Quantidade de envelopes acumulados
- Replay apos retorno da API
- Duplicatas observadas
- Topologia relay -> HUB -> AIceberg preservada quando aplicavel" \
    "- offline_replay_success
- duplicate_rate
- support_export_integrity"
  write_template "$TEMPLATE_DIR/regulated_client_minimal_collection.md" \
    "PKG-72 - Cliente regulado com coleta reduzida" \
    "- Perfil de privacidade aplicado
- Coletores minimizados
- Campos mascarados/hash
- Confirmacao de ausencia de segredo bruto
- Evidencia de rollback da configuracao" \
    "- minimized_collectors
- sensitive_mode
- raw_secret_logging"
  write_template "$TEMPLATE_DIR/noise_cost_before_after.md" \
    "PKG-72 - Ruido/custo antes e depois" \
    "- Janela baseline antes
- Janela depois
- Regra deterministica aplicada
- Evidencia preservada
- Confirmacao de ausencia de supressao automatica" \
    "- noise_before
- noise_after
- manual_review_required
- cost_before
- cost_after"
  write_template "$TEMPLATE_DIR/datadog_scenario_benchmark.md" \
    "PKG-72 - Benchmark comparavel Datadog" \
    "- Cenario AIceberg
- Referencia Datadog usada
- Mesma janela/carga/ambiente ou justificativa
- Dados brutos rastreaveis
- Revisao operacional" \
    "- time_to_diagnosis
- deployment_effort
- agent_plus_agentless
- executive_evidence"
}

: >"$EVIDENCE_FILE"
generate_templates

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
if [[ -n "$TEMPLATE_DIR" ]]; then
  result "evidence-templates" "written" "$TEMPLATE_DIR"
fi
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
