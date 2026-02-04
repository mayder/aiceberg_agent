#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

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

ensure_golangci_lint
log "golangci-lint"
"$GOLANGCI_LINT_BIN" run

log "go vet"
go vet ./...

log "go test"
go test ./...

log "OK"
