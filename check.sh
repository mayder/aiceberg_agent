#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

run_model_validations() {
  local script
  for script in     scripts/validate-required-files.sh     scripts/validate-paths.sh     scripts/validate-docs.sh     scripts/validate-rules.sh     scripts/validate-no-secrets.sh     scripts/validate-file-size.sh     scripts/validate-no-runtime-pkg-names.sh     scripts/validate-fixtures.sh     scripts/validate-layering.sh     scripts/validate-stack.sh; do
    [[ -x "$script" ]] || { echo "[check:modelo] ERROR: script obrigatório ausente ou sem execução: $script" >&2; exit 1; }
    "$script"
  done
}

run_model_validations

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi
if [[ -f configs/agent.env ]]; then
  set -a
  # shellcheck disable=SC1091
  source configs/agent.env
  set +a
fi

log() {
  echo "[check] $*"
}

CGO_CFLAGS="${CGO_CFLAGS:-}"
if [[ "$CGO_CFLAGS" != *-Wno-gnu-folding-constant* ]]; then
  CGO_CFLAGS="${CGO_CFLAGS} -Wno-gnu-folding-constant"
fi
export CGO_CFLAGS

CGO_ENABLED="${CGO_ENABLED:-0}"
export CGO_ENABLED

GOFLAGS="${GOFLAGS:-}"
if [[ "$GOFLAGS" != *-buildvcs=false* ]]; then
  GOFLAGS="${GOFLAGS} -buildvcs=false"
fi
export GOFLAGS

GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v1.62.2}"
TOOLS_DIR="$ROOT_DIR/.tools"
TOOLS_BIN="$TOOLS_DIR/bin"
GOLANGCI_LINT_BIN="$TOOLS_BIN/golangci-lint"

ensure_golangci_lint() {
  if [[ -x "$GOLANGCI_LINT_BIN" ]]; then
    return
  fi
  if ! command -v go >/dev/null 2>&1; then
    echo "[check] go not found; cannot install golangci-lint" >&2
    exit 1
  fi
  mkdir -p "$TOOLS_BIN"
  log "install golangci-lint $GOLANGCI_LINT_VERSION"
  GOBIN="$TOOLS_BIN" go install "github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
}

run_golangci_lint() {
  local lint_output
  lint_output="$(mktemp)"

  if "$GOLANGCI_LINT_BIN" run 2>&1 | tee "$lint_output"; then
    rm -f "$lint_output"
    return 0
  fi

  if grep -Eq 'failed to load package cgo|no export data for "runtime/cgo"' "$lint_output"; then
    log "golangci-lint retry after go clean -cache (runtime/cgo export data)"
    go clean -cache
    "$GOLANGCI_LINT_BIN" run
    rm -f "$lint_output"
    return 0
  fi

  rm -f "$lint_output"
  return 1
}

ensure_golangci_lint
log "golangci-lint"
run_golangci_lint

log "go vet"
go vet ./...

log "go test"
go test ./...

log "pkg72 evidence gate self-test"
scripts/pkg72_contextual_evidence_gate_selftest.sh

log "OK"
