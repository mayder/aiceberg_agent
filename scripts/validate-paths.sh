#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

log "validando PATHS.toml"

[[ "$(toml_string_value files roadmap)" == "QUALITY_ROADMAP.md" ]] || fail "PATHS.toml sem files.roadmap correto"
[[ "$(toml_string_value files governance)" == "GOVERNANCA.md" ]] || fail "PATHS.toml sem files.governance correto"
[[ "$(toml_string_value files backlog)" == "DEMANDAS.md" ]] || fail "PATHS.toml sem files.backlog correto"
[[ "$(toml_string_value files decisions)" == "DECISOES.md" ]] || fail "PATHS.toml sem files.decisions correto"
[[ "$(toml_string_value files check)" == "./check.sh" ]] || fail "PATHS.toml sem files.check correto"
[[ "$(toml_string_value files mindmap_complete)" == "MAPA_MENTAL_MARKMAP.md" ]] || fail "PATHS.toml sem files.mindmap_complete correto"
[[ "$(toml_string_value files mindmap_executive)" == "MAPA_EXECUTIVO_MARKMAP.md" ]] || fail "PATHS.toml sem files.mindmap_executive correto"

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  path="${file#./}"
  [[ -e "$path" ]] || fail "PATHS.toml referencia caminho inexistente: $file"
done < <(toml_array_values checks required)
