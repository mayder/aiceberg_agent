#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

log "validando stack"

run_optional_command() {
  local key="$1"
  local command_value
  command_value="$(toml_string_value quality.stack "$key")"
  [[ -z "$command_value" ]] && return 0

  log "rodando quality.stack.$key: $command_value"
  bash -lc "$command_value"
}

run_optional_command lint
run_optional_command typecheck
run_optional_command test
run_optional_command build
if [[ "$(toml_string_value quality.coverage enabled)" == "true" ]]; then
  coverage_command="$(toml_string_value quality.coverage command)"
  [[ -n "$coverage_command" ]] || fail "quality.coverage.enabled=true exige quality.coverage.command"
  log "rodando quality.coverage.command: $coverage_command"
  bash -lc "$coverage_command"
fi

if [[ -z "$(toml_string_value quality.stack lint)" && \
      -z "$(toml_string_value quality.stack typecheck)" && \
      -z "$(toml_string_value quality.stack test)" && \
      -z "$(toml_string_value quality.stack build)" ]]; then
  log "nenhum comando de stack configurado; esperado no modelo, ajustar no projeto real"
fi
