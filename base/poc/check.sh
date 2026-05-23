#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"
log() { echo "[check:aiceberg_poc] $*"; }
fail() { echo "[check:aiceberg_poc] ERROR: $*" >&2; exit 1; }

run_model_validations() {
  local script
  for script in     scripts/validate-required-files.sh     scripts/validate-paths.sh     scripts/validate-docs.sh     scripts/validate-rules.sh     scripts/validate-no-secrets.sh     scripts/validate-file-size.sh     scripts/validate-no-runtime-pkg-names.sh     scripts/validate-fixtures.sh     scripts/validate-layering.sh     scripts/validate-stack.sh; do
    [[ -x "$script" ]] || { echo "[check:modelo] ERROR: script obrigatório ausente ou sem execução: $script" >&2; exit 1; }
    "$script"
  done
}

run_model_validations

if [[ "${RUN_GO_CHECKS:-0}" != "1" ]]; then
  log "checks Go do POC pulados; use RUN_GO_CHECKS=1 ./check.sh"
  exit 0
fi

command -v go >/dev/null 2>&1 || fail "go não encontrado"
GOFILES="$(find . -type f -name '*.go' -not -path './vendor/*')"
if [[ -n "$GOFILES" && -n "$(gofmt -l $GOFILES)" ]]; then fail "gofmt encontrou arquivos fora do padrão"; fi
log "go test"; go test ./...
log "go vet"; go vet ./...
log "OK"
