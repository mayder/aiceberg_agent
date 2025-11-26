#!/usr/bin/env bash
# Dev helper to run the agent with sane defaults.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export LOG_LEVEL="${LOG_LEVEL:-info}"
export API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8082}"
export HEALTH_PORT="${HEALTH_PORT:-8081}"

exec go run ./cmd/agent -config ./configs/config.example.yml
