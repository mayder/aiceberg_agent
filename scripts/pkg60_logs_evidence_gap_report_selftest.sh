#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TMP_DIR="$(mktemp -d /tmp/aiceberg_pkg60_gap_selftest.XXXXXX)"
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

make_bundle() {
  local scenario="$1"
  local dir="$2"
  mkdir -p "$dir/raw"
  cat >"$dir/evidence.md" <<EOF
# PKG-60 - Evidence $scenario

- Status: pass
- Scenario: $scenario
EOF
  printf 'raw evidence for %s\n' "$scenario" >"$dir/raw/evidence.log"
  cat >"$dir/MANIFEST.tsv" <<EOF
scenario	evidence_file	evidence_sha256	evidence_size_bytes	raw_file	raw_sha256	raw_size_bytes	created_at_utc
$scenario	$dir/evidence.md	sha	1	$dir/raw/evidence.log	sha	1	20260619T000000Z
EOF
  cat >"$dir/PROVENANCE.tsv" <<EOF
key	value
scenario	$scenario
created_at_utc	20260619T000000Z
EOF
}

make_bundle "pkg60-controlled-logs" "$TMP_DIR/controlled"

PKG60_GAP_REPORT_FILE="$TMP_DIR/controlled-report.md" \
scripts/pkg60_logs_evidence_gap_report.sh "$TMP_DIR" >"$TMP_DIR/controlled.out"

assert_contains "$TMP_DIR/controlled-report.md" '# PKG-60 - Relatorio de lacunas de evidencia'
assert_contains "$TMP_DIR/controlled-report.md" '- Fechamento: BLOQUEADO - faltam evidencias reais obrigatorias.'
assert_contains "$TMP_DIR/controlled-report.md" '- Resumo real: 0/3 evidencias reais OK; 3 pendentes; 0 invalidas.'
assert_contains "$TMP_DIR/controlled-report.md" '| `pkg60-controlled-logs` | CONTROLADO | OK |'
assert_contains "$TMP_DIR/controlled.out" 'closure_status=BLOQUEADO'
assert_contains "$TMP_DIR/controlled.out" 'real_evidence_ok=0/3'

set +e
PKG60_GAP_REPORT_FILE="$TMP_DIR/required-report.md" \
PKG60_GAP_REPORT_REQUIRE_COMPLETE=true \
scripts/pkg60_logs_evidence_gap_report.sh "$TMP_DIR" >/dev/null
required_exit=$?
set -e
if [[ "$required_exit" -ne 3 ]]; then
  echo "expected require complete exit 3, got $required_exit" >&2
  exit 1
fi

make_bundle "pkg60-real-os-files" "$TMP_DIR/real-os"
make_bundle "pkg60-real-source-formats" "$TMP_DIR/real-source"
make_bundle "pkg60-real-journald-windows-channels" "$TMP_DIR/real-journald"

PKG60_GAP_REPORT_FILE="$TMP_DIR/complete-report.md" \
scripts/pkg60_logs_evidence_gap_report.sh "$TMP_DIR" >"$TMP_DIR/complete.out"

assert_contains "$TMP_DIR/complete-report.md" '- Fechamento: PRONTO_PARA_REVISAO - todas as evidencias reais estao presentes; ainda exige revisao e aceite explicito.'
assert_contains "$TMP_DIR/complete-report.md" '- Resumo real: 3/3 evidencias reais OK; 0 pendentes; 0 invalidas.'
assert_contains "$TMP_DIR/complete.out" 'closure_status=PRONTO_PARA_REVISAO'
assert_contains "$TMP_DIR/complete.out" 'closure_acceptance=missing'

set +e
PKG60_GAP_REPORT_FILE="$TMP_DIR/accepted-required-report.md" \
PKG60_GAP_REPORT_REQUIRE_ACCEPTED=true \
scripts/pkg60_logs_evidence_gap_report.sh "$TMP_DIR" >/dev/null
accepted_required_exit=$?
set -e
if [[ "$accepted_required_exit" -ne 4 ]]; then
  echo "expected require accepted exit 4, got $accepted_required_exit" >&2
  exit 1
fi

PKG60_GAP_REPORT_FILE="$TMP_DIR/accepted-report.md" \
PKG60_GAP_REPORT_REQUIRE_ACCEPTED=true \
PKG60_ACCEPT_CLOSURE=true \
scripts/pkg60_logs_evidence_gap_report.sh "$TMP_DIR" >"$TMP_DIR/accepted.out"

assert_contains "$TMP_DIR/accepted-report.md" '- Fechamento: ACEITO_PARA_FECHAMENTO - todas as evidencias reais estao presentes e PKG60_ACCEPT_CLOSURE=true.'
assert_contains "$TMP_DIR/accepted.out" 'closure_acceptance=accepted'

make_bundle "pkg60-real-os-files" "$TMP_DIR/invalid-real-os"
rm -f "$TMP_DIR/invalid-real-os/evidence.md"

set +e
PKG60_GAP_REPORT_FILE="$TMP_DIR/invalid-report.md" \
scripts/pkg60_logs_evidence_gap_report.sh "$TMP_DIR/invalid-real-os" >/dev/null
invalid_exit=$?
set -e
if [[ "$invalid_exit" -ne 2 ]]; then
  echo "expected invalid evidence exit 2, got $invalid_exit" >&2
  exit 1
fi
assert_contains "$TMP_DIR/invalid-report.md" '- Fechamento: BLOQUEADO - existem evidencias invalidas.'

echo "PKG-60 evidence gap report self-test OK"
