#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

created_at_utc="$(date -u +%Y%m%dT%H%M%SZ)"
created_human_utc="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
out_dir="${PKG60_EVIDENCE_DIR:-docs/evidence/pkg60/controlled-${created_at_utc}}"
raw_dir="$out_dir/raw"
mkdir -p "$raw_dir"

posix_log="$raw_dir/go-test-oslogs.log"
focused_log="$raw_dir/go-test-focused.log"
windows_log="$raw_dir/windows-compile.log"

echo "[pkg60] go test oslogs" | tee "$posix_log"
go test ./internal/platform/collectors/oslogs 2>&1 | tee -a "$posix_log"

echo "[pkg60] go test focused" | tee "$focused_log"
go test ./internal/platform/collectors/oslogs ./internal/common/config ./internal/domain/usecase ./internal/bootstrap 2>&1 | tee -a "$focused_log"

echo "[pkg60] windows compile" | tee "$windows_log"
GOOS=windows GOARCH=amd64 go test -c -o "$raw_dir/aiceberg_oslogs_windows.test.exe" ./internal/platform/collectors/oslogs 2>&1 | tee -a "$windows_log"

sha256() {
  shasum -a 256 "$1" | awk '{print $1}'
}

size_bytes() {
  wc -c <"$1" | tr -d ' '
}

windows_bin_sha="$(sha256 "$raw_dir/aiceberg_oslogs_windows.test.exe")"
windows_bin_size="$(size_bytes "$raw_dir/aiceberg_oslogs_windows.test.exe")"
cat >"$raw_dir/windows-build.tsv" <<EOF
key	value
windows_test_binary_sha256	$windows_bin_sha
windows_test_binary_size_bytes	$windows_bin_size
windows_test_binary_retained	no
EOF
rm -f "$raw_dir/aiceberg_oslogs_windows.test.exe"

raw_archive="$raw_dir/pkg60-controlled-raw.tgz"
tar -czf "$raw_archive" -C "$raw_dir" \
  "$(basename "$posix_log")" \
  "$(basename "$focused_log")" \
  "$(basename "$windows_log")" \
  "windows-build.tsv"

raw_sha="$(sha256 "$raw_archive")"
raw_size="$(size_bytes "$raw_archive")"

cat >"$out_dir/evidence.md" <<EOF
# PKG-60 - Evidencia Controlada de Pipeline de Logs

## Ambiente

- Data UTC: ${created_human_utc}
- Responsavel: Codex
- Cliente/lab: lab local controlado
- Ambiente: $(uname -s) $(uname -r) $(uname -m)
- Versao agente: workspace atual
- Topologia: direct -> AIceberg para contrato de logs; Relay/Hub nao envolvidos neste teste controlado.

## Evidencia coberta

- Graylog/GELF: coberto por teste controlado de parser/classificacao.
- Linux auth: coberto por teste controlado de arquivo auth.log.
- App JSON: coberto por teste controlado com severity, service e redaction.
- Log texto comum: coberto por teste controlado de arquivo simples.
- Cursor/restart/truncamento/rotacao: coberto por teste controlado do coletor POSIX.
- Windows Security: coberto por parser no build Windows.
- Sysmon: coberto por parser no build Windows.
- Windows build: compilacao cruzada do pacote oslogs concluida.

## Comandos

- \`go test ./internal/platform/collectors/oslogs\`
- \`go test ./internal/platform/collectors/oslogs ./internal/common/config ./internal/domain/usecase ./internal/bootstrap\`
- \`GOOS=windows GOARCH=amd64 go test -c -o raw/aiceberg_oslogs_windows.test.exe ./internal/platform/collectors/oslogs\`

## Resultado

- Status: pass
- Evidencia bruta anexada: raw/$(basename "$raw_archive")
- SHA256 bruto: ${raw_sha}
- Tamanho bruto bytes: ${raw_size}
- SHA256 binario Windows de teste: ${windows_bin_sha}
- Tamanho binario Windows de teste bytes: ${windows_bin_size}
- Binario Windows retido no repo: no

## Limites

Esta evidencia e controlada. Ela reduz risco funcional, mas nao fecha as validacoes reais ainda abertas de PKG-60: Graylog real, Windows Security real, Linux auth real e Sysmon real em ambiente operacional.
EOF

evidence_sha="$(sha256 "$out_dir/evidence.md")"
evidence_size="$(size_bytes "$out_dir/evidence.md")"

cat >"$out_dir/MANIFEST.tsv" <<EOF
scenario	evidence_file	evidence_sha256	evidence_size_bytes	raw_file	raw_sha256	raw_size_bytes	created_at_utc
pkg60-controlled-logs	$out_dir/evidence.md	$evidence_sha	$evidence_size	$out_dir/raw/$(basename "$raw_archive")	$raw_sha	$raw_size	$created_at_utc
EOF

cat >"$out_dir/PROVENANCE.tsv" <<EOF
key	value
bundle_tool	scripts/pkg60_logs_controlled_evidence.sh
scenario	pkg60-controlled-logs
created_at_utc	$created_at_utc
evidence_file	evidence.md
raw_archive	raw/$(basename "$raw_archive")
EOF

echo "evidence=$out_dir/evidence.md"
echo "manifest=$out_dir/MANIFEST.tsv"
