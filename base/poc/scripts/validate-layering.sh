#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"
log "validando arquitetura por camadas"
enabled="$(toml_string_value quality.layering enabled)"
if [[ "$enabled" != "true" ]]; then log "quality.layering.enabled=false; pulando validação de camadas no modelo"; exit 0; fi
if [[ "${STRICT_GO_LAYERING:-0}" != "1" ]]; then log "modo legado: layering Go estrito desativado; use STRICT_GO_LAYERING=1"; exit 0; fi
runtime_dirs=(); while IFS= read -r dir; do [[ -n "$dir" ]] && runtime_dirs+=("$dir"); done < <(toml_array_values quality runtime_dirs)
for dir in "${runtime_dirs[@]}"; do [[ -d "$dir" ]] || fail "runtime_dir inexistente: $dir"; done
if grep -RInE 'net/http|github.com/jackc/pgx|database/sql|redis/go-redis' "${runtime_dirs[@]}" --include='*.go' --exclude-dir=vendor | grep -E '/(domain|entity|entities|usecase|usecases)/'; then fail "dominio/usecase Go não deve depender diretamente de transport/infra"; fi
