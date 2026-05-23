#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"
log "validando ausência de nomes internos em runtime"
if [[ "${CHECK_STRICT_RUNTIME_NAMES:-0}" != "1" ]]; then log "modo legado: varredura bloqueante de nomes internos desativada; use CHECK_STRICT_RUNTIME_NAMES=1"; exit 0; fi
runtime_dirs=(); while IFS= read -r dir; do [[ -n "$dir" ]] && runtime_dirs+=("$dir"); done < <(toml_array_values quality runtime_dirs)
patterns=(); while IFS= read -r pattern; do [[ -n "$pattern" ]] && patterns+=("$pattern"); done < <(toml_array_values quality forbid_runtime_patterns)
if [[ ${#runtime_dirs[@]} -eq 0 || ${#patterns[@]} -eq 0 ]]; then log "runtime_dirs ou forbid_runtime_patterns vazio; pulando validação runtime no modelo"; exit 0; fi
for pattern in "${patterns[@]}"; do if grep -RInE "$pattern" "${runtime_dirs[@]}" --include='*.go' --exclude='*_test.go' --exclude-dir=vendor --exclude-dir=dist --exclude-dir=build --exclude-dir=.tools --exclude-dir=bin; then fail "padrão interno encontrado em runtime: $pattern"; fi; done
