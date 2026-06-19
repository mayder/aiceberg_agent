#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TMP_DIR="$(mktemp -d /tmp/aiceberg_pkg69_collect_selftest.XXXXXX)"
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
cat >"$template" <<'EOF'
# PKG-69 - Relay Hub Direct Hosts

## Ambiente

- Data UTC: 2026-06-18T00:00:00Z
- Responsavel: collect-selftest
- Cliente/lab: local
- Ambiente: Hosts separados
- Host/agente/HUB/relay: host-1
- Versao agente: selftest
- Artefato instalado: selftest.tar.gz
- Topologia: direct/hub/relay hosts separados

## Evidencia obrigatoria

- direct_host_id: direct-host
- hub_host_id: hub-host
- relay_host_id: relay-host
- relay_upstream_host_id: hub-host
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
- Revisor: collect-selftest
- Aprovacao fechamento: yes
EOF

output="$(HTTP_PROXY="http://user:secret@example.test:8080" scripts/pkg69_collect_host_evidence.sh relay-hub-direct-hosts "$template" "$TMP_DIR/out")"
assert_contains "$TMP_DIR/out/bundle/evidence.md" "- Evidencia bruta anexada: raw/raw-host.tgz"
assert_contains "$TMP_DIR/out/bundle/MANIFEST.tsv" "artifact_sha256"
assert_contains "$TMP_DIR/out/bundle/PROVENANCE.tsv" $'scenario\trelay-hub-direct-hosts'
assert_contains "$TMP_DIR/out/bundle/PROVENANCE.tsv" $'raw_source_type\tdirectory'
assert_contains "$TMP_DIR/out/bundle/PROVENANCE.tsv" $'artifact_file\traw/raw-host.tgz'
test -s "$TMP_DIR/out/bundle/raw/raw-host.tgz"
printf '%s\n' "$output" | grep -Fq "manifest=$TMP_DIR/out/bundle/MANIFEST.tsv"
printf '%s\n' "$output" | grep -Fq "provenance=$TMP_DIR/out/bundle/PROVENANCE.tsv"

PKG69_EVIDENCE_FILE="$TMP_DIR/gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/gate.tsv" \
PKG69_RELAY_HUB_DIRECT_EVIDENCE="$TMP_DIR/out/bundle/evidence.md" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null
assert_contains "$TMP_DIR/gate.md" "relay-hub-direct-hosts: evidence"
assert_contains "$TMP_DIR/gate.tsv" "$TMP_DIR/out/bundle/raw/raw-host.tgz"

tar -tzf "$TMP_DIR/out/bundle/raw/raw-host.tgz" | grep -Fq "raw-host/README.tsv"
tar -tzf "$TMP_DIR/out/bundle/raw/raw-host.tgz" | grep -Fq "raw-host/COMMANDS.tsv"
tar -tzf "$TMP_DIR/out/bundle/raw/raw-host.tgz" | grep -Fq "raw-host/proxy_env_redacted.txt"
mkdir -p "$TMP_DIR/extract"
tar -xzf "$TMP_DIR/out/bundle/raw/raw-host.tgz" -C "$TMP_DIR/extract"
assert_contains "$TMP_DIR/extract/raw-host/proxy_env_redacted.txt" "HTTP_PROXY=http://<redacted>@example.test:8080"
if grep -Fq -- "secret" "$TMP_DIR/extract/raw-host/proxy_env_redacted.txt"; then
  echo "proxy credentials leaked in redacted env evidence" >&2
  exit 1
fi

echo "PKG-69 host evidence collector self-test OK"
