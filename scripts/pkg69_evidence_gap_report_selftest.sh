#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TMP_DIR="$(mktemp -d /tmp/aiceberg_pkg69_gap_report_selftest.XXXXXX)"
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

template="$TMP_DIR/relay.md"
raw="$TMP_DIR/raw.log"
cat >"$template" <<'EOF'
# PKG-69 - Relay Hub Direct Hosts

## Ambiente

- Data UTC: 2026-06-18T00:00:00Z
- Responsavel: gap-report-selftest
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
- Revisor: gap-report-selftest
- Aprovacao fechamento: yes
EOF
printf 'raw gap evidence\n' >"$raw"
scripts/pkg69_bundle_evidence.sh relay-hub-direct-hosts "$template" "$raw" "$TMP_DIR/bundles/relay" >/dev/null

PKG69_GAP_REPORT_FILE="$TMP_DIR/report.md" \
PKG69_EVIDENCE_FILE="$TMP_DIR/gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/manifest.tsv" \
scripts/pkg69_evidence_gap_report.sh "$TMP_DIR/bundles" >"$TMP_DIR/report.out" 2>"$TMP_DIR/report.err"

assert_contains "$TMP_DIR/report.md" "# PKG-69 - Relatorio de lacunas de evidencia"
assert_contains "$TMP_DIR/report.md" "- Fechamento: BLOQUEADO - faltam evidencias reais obrigatorias."
assert_contains "$TMP_DIR/report.out" "closure_status=BLOQUEADO"
assert_contains "$TMP_DIR/report.out" "closure_reason=faltam evidencias reais obrigatorias"
assert_contains "$TMP_DIR/report.md" '| `relay-hub-direct-hosts` | OK |'
assert_contains "$TMP_DIR/report.md" '| `windows-server` | PENDENTE |'
assert_contains "$TMP_DIR/report.md" 'Executar smoke.ps1 e update/rollback controlado em Windows Server'
assert_contains "$TMP_DIR/report.md" '13 pendentes'

set +e
PKG69_GAP_REPORT_FILE="$TMP_DIR/required-report.md" \
PKG69_EVIDENCE_FILE="$TMP_DIR/required-gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/required-manifest.tsv" \
PKG69_GAP_REPORT_REQUIRE_COMPLETE=true \
scripts/pkg69_evidence_gap_report.sh "$TMP_DIR/bundles" >/dev/null 2>"$TMP_DIR/required-report.err"
required_exit=$?
set -e
if [[ "$required_exit" -ne 3 ]]; then
  echo "expected incomplete required report exit 3, got $required_exit" >&2
  exit 1
fi
assert_contains "$TMP_DIR/required-report.md" '13 pendentes'

invalid_template="$TMP_DIR/proxy-invalid.md"
cat >"$invalid_template" <<'EOF'
# PKG-69 - Proxy TLS

## Ambiente

- Data UTC: 2026-06-18T00:00:00Z
- Responsavel: gap-report-selftest
- Cliente/lab: local
- Ambiente: Proxy/TLS controlado
- Host/agente/HUB/relay: host-1
- Versao agente: selftest
- Artefato instalado: selftest.tar.gz
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- proxy autenticado real: yes
- TLS invalido rejeitado: yes
- TLS valido aceito: yes
- sem token em log: yes
- rollback de config validado: yes

## Metricas

- requests_ok: nao-numerico
- requests_failed_expected: 1
- retry_count: 0

## Resultado

- Status: pass
- Evidencia bruta anexada: old.log
- Observacoes: selftest
- Rollback validado: sim
- Revisor: gap-report-selftest
- Aprovacao fechamento: yes
EOF
scripts/pkg69_bundle_evidence.sh proxy-tls "$invalid_template" "$raw" "$TMP_DIR/invalid/proxy" >/dev/null

set +e
PKG69_GAP_REPORT_FILE="$TMP_DIR/invalid-report.md" \
PKG69_EVIDENCE_FILE="$TMP_DIR/invalid-gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/invalid-manifest.tsv" \
scripts/pkg69_evidence_gap_report.sh "$TMP_DIR/invalid" >"$TMP_DIR/invalid-report.out" 2>"$TMP_DIR/invalid-report.err"
invalid_exit=$?
set -e
if [[ "$invalid_exit" -ne 2 ]]; then
  echo "expected invalid gap report exit 2, got $invalid_exit" >&2
  exit 1
fi
assert_contains "$TMP_DIR/invalid-report.md" '| `proxy-tls` | INVALIDO | field requests_ok must be numeric |'
assert_contains "$TMP_DIR/invalid-report.md" "- Fechamento: BLOQUEADO - existem evidencias invalidas."
assert_contains "$TMP_DIR/invalid-report.out" "closure_reason=existem evidencias invalidas"

echo "PKG-69 evidence gap report self-test OK"
