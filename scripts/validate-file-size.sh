#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"
log "validando tamanho de arquivos"
max_lines="$(toml_int_value quality max_file_lines)"
[[ "$max_lines" =~ ^[0-9]+$ ]] || fail "quality.max_file_lines inválido em PATHS.toml"
runtime_dirs=(); while IFS= read -r dir; do [[ -n "$dir" ]] && runtime_dirs+=("$dir"); done < <(toml_array_values quality runtime_dirs)
if [[ ${#runtime_dirs[@]} -eq 0 ]]; then log "runtime_dirs vazio; pulando validação de tamanho runtime no modelo"; exit 0; fi
for dir in "${runtime_dirs[@]}"; do [[ -d "$dir" ]] || fail "runtime_dir inexistente: $dir"; done
while IFS= read -r file; do lines="$(wc -l < "$file" | tr -d ' ')"; if (( lines > max_lines )); then fail "arquivo acima de ${max_lines} linhas: $file ($lines)"; fi; done < <(find "${runtime_dirs[@]}" -type f -name '*.go' ! -path '*/vendor/*' ! -path '*/dist/*' ! -path '*/build/*' ! -path '*/.tools/*' ! -path '*/bin/*')
