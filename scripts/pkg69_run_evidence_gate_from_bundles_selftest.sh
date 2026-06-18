#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TMP_DIR="$(mktemp -d /tmp/aiceberg_pkg69_bundle_gate_selftest.XXXXXX)"
trap 'rm -rf "$TMP_DIR"' EXIT

assert_contains() {
  local path="$1"
  local pattern="$2"
  grep -Fq -- "$pattern" "$path" || {
    echo "expected pattern not found: $pattern" >&2
    echo "file: $path" >&2
    exit 1
  }
}

write_template() {
  local path="$1"
  local title="$2"
  local topology="$3"
  local required="$4"
  local metrics="$5"
  cat >"$path" <<EOF
# $title

## Ambiente

- Data UTC: 2026-06-18T00:00:00Z
- Responsavel: bundle-gate-selftest
- Cliente/lab: local
- Ambiente: selftest
- Host/agente/HUB/relay: host-1
- Versao agente: selftest
- Artefato instalado: selftest.tar.gz
- Topologia: $topology

## Evidencia obrigatoria

$required

## Metricas

$metrics

## Resultado

- Status: pass
- Evidencia bruta anexada: old.log
- Observacoes: selftest
- Rollback validado: sim
- Revisor: bundle-gate-selftest
- Aprovacao fechamento: yes
EOF
}

raw="$TMP_DIR/raw.log"
printf 'raw bundle gate evidence\n' >"$raw"

relay_template="$TMP_DIR/relay.md"
write_template "$relay_template" \
  "PKG-69 - Relay Hub Direct Hosts" \
  "direct/hub/relay hosts separados" \
  "- direct -> AIceberg confirmado: yes
- hub -> AIceberg confirmado: yes
- relay -> hub -> AIceberg confirmado: yes
- relay sem conexao direta com API AIceberg: yes
- agentless via Hub quando aplicavel: yes" \
  "- direct_ingested: yes
- hub_ingested: yes
- relay_ingested_via_hub: yes
- relay_direct_api_attempts: 0"

proxy_template="$TMP_DIR/proxy.md"
write_template "$proxy_template" \
  "PKG-69 - Proxy TLS" \
  "direct -> AIceberg" \
  "- proxy autenticado real: yes
- TLS invalido rejeitado: yes
- TLS valido aceito: yes
- sem token em log: yes
- rollback de config validado: yes" \
  "- requests_ok: 1
- requests_failed_expected: 1
- retry_count: 0"

scripts/pkg69_bundle_evidence.sh relay-hub-direct-hosts "$relay_template" "$raw" "$TMP_DIR/bundles/relay" >/dev/null
scripts/pkg69_bundle_evidence.sh proxy-tls "$proxy_template" "$raw" "$TMP_DIR/bundles/proxy" >/dev/null

PKG69_EVIDENCE_FILE="$TMP_DIR/gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/gate.tsv" \
scripts/pkg69_run_evidence_gate_from_bundles.sh "$TMP_DIR/bundles" >"$TMP_DIR/runner.out" 2>"$TMP_DIR/runner.err"

assert_contains "$TMP_DIR/runner.err" "mapped"
assert_contains "$TMP_DIR/gate.md" "relay-hub-direct-hosts: evidence"
assert_contains "$TMP_DIR/gate.md" "proxy-tls: evidence"
assert_contains "$TMP_DIR/gate.md" "real-evidence-manifest: incomplete"
assert_contains "$TMP_DIR/gate.tsv" "$TMP_DIR/bundles/relay/evidence.md"
assert_contains "$TMP_DIR/gate.tsv" "$TMP_DIR/bundles/proxy/evidence.md"

mkdir -p "$TMP_DIR/bundles-tampered"
cp -R "$TMP_DIR/bundles/relay" "$TMP_DIR/bundles-tampered/relay"
printf '\n# tampered\n' >>"$TMP_DIR/bundles-tampered/relay/evidence.md"
set +e
scripts/pkg69_run_evidence_gate_from_bundles.sh "$TMP_DIR/bundles-tampered/relay" >/dev/null 2>"$TMP_DIR/tampered.err"
tampered_exit=$?
set -e
if [[ "$tampered_exit" -eq 0 ]]; then
  echo "expected tampered bundle to fail" >&2
  exit 1
fi
assert_contains "$TMP_DIR/tampered.err" "bundle evidence sha256 mismatch"

set +e
PKG69_EVIDENCE_FILE="$TMP_DIR/required.md" \
PKG69_REQUIRE_REAL_EVIDENCE=true \
scripts/pkg69_run_evidence_gate_from_bundles.sh "$TMP_DIR/bundles" >/dev/null 2>&1
required_exit=$?
set -e
if [[ "$required_exit" -eq 0 ]]; then
  echo "expected incomplete real-evidence run to fail" >&2
  exit 1
fi

unknown_manifest_dir="$TMP_DIR/bundles/unknown"
mkdir -p "$unknown_manifest_dir"
cp "$TMP_DIR/bundles/relay/evidence.md" "$unknown_manifest_dir/evidence.md"
{
  printf 'scenario\ttemplate\tsha256\tbytes\tartifact\tartifact_sha256\tartifact_bytes\tcreated_at_utc\n'
  printf 'unknown-scenario\t%s\t-\t-\t-\t-\t-\t20260618T000000Z\n' "$unknown_manifest_dir/evidence.md"
} >"$unknown_manifest_dir/MANIFEST.tsv"

set +e
scripts/pkg69_run_evidence_gate_from_bundles.sh "$unknown_manifest_dir" >/dev/null 2>&1
unknown_exit=$?
set -e
if [[ "$unknown_exit" -eq 0 ]]; then
  echo "expected unknown scenario bundle to fail" >&2
  exit 1
fi

echo "PKG-69 evidence bundle gate self-test OK"
