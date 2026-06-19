#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TMP_DIR="$(mktemp -d /tmp/aiceberg_pkg69_bundle_selftest.XXXXXX)"
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

template="$TMP_DIR/template.md"
raw="$TMP_DIR/raw.log"
cat >"$template" <<'EOF'
# PKG-69 - Relay Hub Direct Hosts

## Ambiente

- Data UTC: 2026-06-18T00:00:00Z
- Responsavel: bundle-selftest
- Cliente/lab: local
- Ambiente: Hosts separados
- Host/agente/HUB/relay: host-1
- Versao agente: selftest
- Artefato instalado: selftest.tar.gz
- Topologia: direct/hub/relay hosts separados

## Evidencia obrigatoria

- direct -> AIceberg confirmado: yes
- hub -> AIceberg confirmado: yes
- relay -> hub -> AIceberg confirmado: yes
- relay sem conexao direta com API AIceberg: yes
- agentless via Hub quando aplicavel: yes

## Metricas

- direct_ingested: yes
- hub_ingested: yes
- relay_ingested_via_hub: yes
- relay_direct_api_attempts: 0

## Resultado

- Status: pass
- Evidencia bruta anexada: old.log
- Observacoes: selftest
- Rollback validado: sim
- Revisor: bundle-selftest
- Aprovacao fechamento: yes
EOF
printf 'raw bundle evidence\n' >"$raw"

output="$(scripts/pkg69_bundle_evidence.sh relay-hub-direct-hosts "$template" "$raw" "$TMP_DIR/out")"
assert_contains "$TMP_DIR/out/evidence.md" "- Evidencia bruta anexada: raw/raw.log"
assert_contains "$TMP_DIR/out/MANIFEST.tsv" "artifact_sha256"
assert_contains "$TMP_DIR/out/MANIFEST.tsv" "relay-hub-direct-hosts"
assert_contains "$TMP_DIR/out/MANIFEST.tsv" "$TMP_DIR/out/raw/raw.log"
assert_contains "$TMP_DIR/out/PROVENANCE.tsv" $'bundle_tool\tscripts/pkg69_bundle_evidence.sh'
assert_contains "$TMP_DIR/out/PROVENANCE.tsv" $'bundle_tool_version\t1'
assert_contains "$TMP_DIR/out/PROVENANCE.tsv" $'scenario\trelay-hub-direct-hosts'
assert_contains "$TMP_DIR/out/PROVENANCE.tsv" $'raw_source_type\tfile'
assert_contains "$TMP_DIR/out/PROVENANCE.tsv" $'artifact_file\traw/raw.log'
printf '%s\n' "$output" | grep -Fq "evidence=$TMP_DIR/out/evidence.md"
printf '%s\n' "$output" | grep -Fq "provenance=$TMP_DIR/out/PROVENANCE.tsv"

PKG69_EVIDENCE_FILE="$TMP_DIR/gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/gate.tsv" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/out/evidence.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/gate.md" "relay-hub-direct-hosts: evidence"
assert_contains "$TMP_DIR/gate.tsv" "$TMP_DIR/out/evidence.md"
assert_contains "$TMP_DIR/gate.tsv" "$TMP_DIR/out/raw/raw.log"

empty="$TMP_DIR/empty.log"
: >"$empty"
set +e
scripts/pkg69_bundle_evidence.sh relay-hub-direct-hosts "$template" "$empty" "$TMP_DIR/empty-out" >/dev/null 2>&1
empty_exit=$?
set -e
if [[ "$empty_exit" -eq 0 ]]; then
  echo "expected empty raw artifact to fail" >&2
  exit 1
fi

set +e
scripts/pkg69_bundle_evidence.sh unknown-scenario "$template" "$raw" "$TMP_DIR/unknown-out" >/dev/null 2>"$TMP_DIR/unknown.err"
unknown_exit=$?
set -e
if [[ "$unknown_exit" -eq 0 ]]; then
  echo "expected unknown scenario to fail" >&2
  exit 1
fi
assert_contains "$TMP_DIR/unknown.err" "unknown PKG-69 scenario"

raw_dir="$TMP_DIR/rawdir"
mkdir -p "$raw_dir"
printf 'dir evidence\n' >"$raw_dir/log.txt"
scripts/pkg69_bundle_evidence.sh relay-hub-direct-hosts "$template" "$raw_dir" "$TMP_DIR/dir-out" >/dev/null
test -s "$TMP_DIR/dir-out/raw/rawdir.tgz"
assert_contains "$TMP_DIR/dir-out/evidence.md" "- Evidencia bruta anexada: raw/rawdir.tgz"
assert_contains "$TMP_DIR/dir-out/PROVENANCE.tsv" $'raw_source_type\tdirectory'
assert_contains "$TMP_DIR/dir-out/PROVENANCE.tsv" $'artifact_file\traw/rawdir.tgz'

echo "PKG-69 evidence bundle self-test OK"
