#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

log "validando documentação"

while IFS= read -r file; do
  count="$(awk 'BEGIN{n=0} /^```/{n++} END{print n}' "$file")"
  if (( count % 2 != 0 )); then
    fail "bloco markdown quebrado em $file"
  fi
done < <(find . -maxdepth 1 -type f -name '*.md' -print)

grep -F "PATHS.toml" README.md >/dev/null || fail "README.md deve explicar PATHS.toml"
grep -F "MAPA_MENTAL_MARKMAP.md" README.md >/dev/null || fail "README.md deve citar mapa mental"
