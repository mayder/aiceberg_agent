#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

log "validando arquivos obrigatórios"

require_file PATHS.toml
require_file ESCOPO.md
require_file GOVERNANCA.md
require_file QUALITY_ROADMAP.md
require_file DEMANDAS.md
require_file BUGS.md
require_file TELAS.md
require_file TESTES.md
require_file DECISOES.md
require_file RUNBOOK.md
require_file README.md
require_file MAPA_MENTAL_MARKMAP.md
require_file MAPA_EXECUTIVO_MARKMAP.md
require_file scripts/validate-layering.sh
require_file check.sh

for script in scripts/*.sh; do
  [[ -x "$script" ]] || fail "script sem permissão de execução: $script"
done
