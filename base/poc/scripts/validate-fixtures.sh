#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

log "validando fixtures"

fixture_dirs=()
while IFS= read -r dir; do
  [[ -n "$dir" ]] && fixture_dirs+=("$dir")
done < <(toml_array_values quality.fixtures dirs)

if [[ ${#fixture_dirs[@]} -eq 0 ]]; then
  log "fixtures.dirs vazio; pulando validação de fixtures no modelo"
  exit 0
fi

max_kb="$(toml_int_value quality.fixtures max_fixture_file_kb)"
[[ "$max_kb" =~ ^[0-9]+$ ]] || fail "quality.fixtures.max_fixture_file_kb inválido"
max_bytes=$((max_kb * 1024))

patterns=()
while IFS= read -r pattern; do
  [[ -n "$pattern" ]] && patterns+=("$pattern")
done < <(toml_array_values quality.fixtures forbid_fixture_name_patterns)

for dir in "${fixture_dirs[@]}"; do
  [[ -d "$dir" ]] || fail "fixture dir inexistente: $dir"
done

for pattern in "${patterns[@]}"; do
  if find "${fixture_dirs[@]}" -iname "*${pattern}*" -print | grep -q .; then
    find "${fixture_dirs[@]}" -iname "*${pattern}*" -print
    fail "fixture com nome proibido encontrado: $pattern"
  fi
done

while IFS= read -r file; do
  size="$(wc -c < "$file" | tr -d ' ')"
  if (( size > max_bytes )); then
    fail "fixture acima de ${max_kb}KB: $file"
  fi
done < <(find "${fixture_dirs[@]}" -type f)

if grep -RInE '(password|passwd|token|secret|api[_-]?key|private[_-]?key)\s*[:=]\s*[^ <#]' "${fixture_dirs[@]}"; then
  fail "possível segredo encontrado em fixture"
fi
