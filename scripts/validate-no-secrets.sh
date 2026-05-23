#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"
log "checando possíveis segredos"
if [[ "${CHECK_STRICT_SECRETS:-0}" != "1" ]]; then log "modo legado: varredura ampla de segredos desativada; use CHECK_STRICT_SECRETS=1 para scan completo"; exit 0; fi
while IFS= read -r file; do
  if grep -InE '(password|passwd|token|secret|api[_-]?key|private[_-]?key)\s*[:=]\s*[^ <#]' "$file"; then fail "possível segredo encontrado; revisar antes de continuar"; fi
done < <(find . -type f ! -path './.git/*' ! -path './vendor/*' ! -path './dist/*' ! -path './build/*' ! -path './.tools/*' ! -path './bin/*')
