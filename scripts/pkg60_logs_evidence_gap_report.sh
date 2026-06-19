#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

usage() {
  cat >&2 <<'USAGE'
Usage:
  scripts/pkg60_logs_evidence_gap_report.sh [evidence-root]

Reads PKG-60 evidence bundles and writes an actionable Markdown gap report.
Controlled evidence is reported, but it does not close real validation gaps.

Environment:
  PKG60_GAP_REPORT_FILE              Output markdown path.
  PKG60_GAP_REPORT_REQUIRE_COMPLETE=true
                                     Exit non-zero while any real scenario is pending.
  PKG60_GAP_REPORT_REQUIRE_ACCEPTED=true
                                     Exit non-zero when complete evidence was not accepted.
  PKG60_ACCEPT_CLOSURE=true          Explicit closure acceptance after review.
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

evidence_root="${1:-docs/evidence/pkg60}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
REPORT_FILE="${PKG60_GAP_REPORT_FILE:-/tmp/aiceberg_pkg60_gap_report_${timestamp}.md}"
REQUIRE_COMPLETE="${PKG60_GAP_REPORT_REQUIRE_COMPLETE:-false}"
REQUIRE_ACCEPTED="${PKG60_GAP_REPORT_REQUIRE_ACCEPTED:-false}"
ACCEPT_CLOSURE="${PKG60_ACCEPT_CLOSURE:-false}"

tmp_dir="$(mktemp -d /tmp/aiceberg_pkg60_gap.XXXXXX)"
trap 'rm -rf "$tmp_dir"' EXIT
manifest_list="$tmp_dir/manifests.txt"
classified="$tmp_dir/classified.tsv"

required_scenarios="
pkg60-real-os-files
pkg60-real-source-formats
pkg60-real-journald-windows-channels
"
controlled_scenario="pkg60-controlled-logs"

scenario_label() {
  case "$1" in
    pkg60-real-os-files) printf 'Windows EventLog, Linux syslog e arquivo comum em ambiente real\n' ;;
    pkg60-real-source-formats) printf 'Graylog, Windows Security, Linux auth, app JSON e log texto em ambiente real\n' ;;
    pkg60-real-journald-windows-channels) printf 'journald, Security/System/Application e Sysmon em ambiente real\n' ;;
    pkg60-controlled-logs) printf 'Teste controlado de parsing, cursor e build Windows\n' ;;
    *) printf '%s\n' "$1" ;;
  esac
}

scenario_action() {
  case "$1" in
    pkg60-real-os-files) printf 'Coletar bundle real com EventLog Windows, syslog Linux e arquivo comum, contendo MANIFEST.tsv, PROVENANCE.tsv, evidence.md e artefato bruto sanitizado.\n' ;;
    pkg60-real-source-formats) printf 'Coletar bundle real com Graylog/GELF, Windows Security, Linux auth, app JSON e log texto, comprovando level/severity e campos essenciais.\n' ;;
    pkg60-real-journald-windows-channels) printf 'Coletar bundle real com journald e canais Security/System/Application/Sysmon, comprovando cursor e filtro de severidade.\n' ;;
    pkg60-controlled-logs) printf 'Manter como evidencia funcional controlada; nao usar como substituta de homologacao real.\n' ;;
    *) printf 'Revisar o bundle e classificar o cenario conforme o contrato do gate.\n' ;;
  esac
}

md_cell() {
  printf '%s' "$1" | tr '|' '/'
}

scenario_has_status() {
  local scenario="$1"
  local status="$2"
  awk -F '\t' -v scenario="$scenario" -v status="$status" '$1 == scenario && $3 == status { found=1 } END { exit found ? 0 : 1 }' "$classified"
}

scenario_row() {
  local scenario="$1"
  local status="$2"
  awk -F '\t' -v scenario="$scenario" -v status="$status" '$1 == scenario && $3 == status { print; exit }' "$classified"
}

scenario_exists() {
  local scenario="$1"
  awk -F '\t' -v scenario="$scenario" '$1 == scenario { found=1 } END { exit found ? 0 : 1 }' "$classified"
}

is_required() {
  local scenario="$1"
  printf '%s\n' "$required_scenarios" | grep -Fxq "$scenario"
}

resolve_path() {
  local manifest_dir="$1"
  local path="$2"
  if [[ "$path" == /* ]]; then
    printf '%s\n' "$path"
  elif [[ -e "$path" ]]; then
    printf '%s\n' "$path"
  else
    printf '%s/%s\n' "$manifest_dir" "$path"
  fi
}

if [[ -d "$evidence_root" ]]; then
  find "$evidence_root" -name MANIFEST.tsv -type f | sort >"$manifest_list"
elif [[ -f "$evidence_root" ]]; then
  printf '%s\n' "$evidence_root" >"$manifest_list"
else
  : >"$manifest_list"
fi

printf 'scenario\ttype\tstatus\tevidence\treason\n' >"$classified"

while IFS= read -r manifest; do
  [[ -n "$manifest" ]] || continue
  manifest_dir="$(dirname "$manifest")"
  while IFS=$'\t' read -r scenario evidence_file _evidence_sha _evidence_size raw_file _raw_sha _raw_size _created_at; do
    [[ "$scenario" == "scenario" || -z "$scenario" ]] && continue

    evidence_path="$(resolve_path "$manifest_dir" "${evidence_file:-}")"
    raw_path="$(resolve_path "$manifest_dir" "${raw_file:-}")"
    evidence_status="OK"
    reason="path=$evidence_path"

    if [[ ! -s "$evidence_path" ]]; then
      evidence_status="INVALIDO"
      reason="evidence.md ausente ou vazio: $evidence_path"
    elif [[ -n "${raw_file:-}" && ! -s "$raw_path" ]]; then
      evidence_status="INVALIDO"
      reason="artefato bruto ausente ou vazio: $raw_path"
    fi

    if [[ "$scenario" == "$controlled_scenario" ]]; then
      printf '%s\tCONTROLADO\t%s\t%s\t%s\n' "$scenario" "$evidence_status" "$evidence_path" "$reason" >>"$classified"
    elif is_required "$scenario"; then
      printf '%s\tREAL\t%s\t%s\t%s\n' "$scenario" "$evidence_status" "$evidence_path" "$reason" >>"$classified"
    else
      printf '%s\tEXTRA\t%s\t%s\t%s\n' "$scenario" "$evidence_status" "$evidence_path" "$reason" >>"$classified"
    fi
  done <"$manifest"
done <"$manifest_list"

total_required=0
ok_required=0
pending=0
invalid=0
controlled_found=0

for scenario in $required_scenarios; do
  total_required=$((total_required + 1))
  if scenario_has_status "$scenario" "INVALIDO"; then
    invalid=$((invalid + 1))
  elif scenario_has_status "$scenario" "OK"; then
    ok_required=$((ok_required + 1))
  else
    pending=$((pending + 1))
  fi
done

if scenario_has_status "$controlled_scenario" "OK"; then
  controlled_found=1
fi

closure_status="BLOQUEADO"
closure_reason="faltam evidencias reais obrigatorias"
closure_acceptance="missing"
if [[ "$invalid" -gt 0 ]]; then
  closure_reason="existem evidencias invalidas"
elif [[ "$pending" -eq 0 ]]; then
  closure_status="PRONTO_PARA_REVISAO"
  closure_reason="todas as evidencias reais estao presentes; ainda exige revisao e aceite explicito"
  if [[ "$ACCEPT_CLOSURE" == "true" ]]; then
    closure_status="ACEITO_PARA_FECHAMENTO"
    closure_reason="todas as evidencias reais estao presentes e PKG60_ACCEPT_CLOSURE=true"
    closure_acceptance="accepted"
  fi
fi

{
  printf '# PKG-60 - Relatorio de lacunas de evidencia\n\n'
  printf -- '- Gerado UTC: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf -- '- Evidence root: `%s`\n' "$evidence_root"
  printf -- '- Fechamento: %s - %s.\n' "$closure_status" "$closure_reason"
  printf -- '- Resumo real: %s/%s evidencias reais OK; %s pendentes; %s invalidas.\n' "$ok_required" "$total_required" "$pending" "$invalid"
  if [[ "$controlled_found" -eq 1 ]]; then
    printf -- '- Evidencia controlada: presente, usada apenas como suporte funcional.\n\n'
  else
    printf -- '- Evidencia controlada: ausente.\n\n'
  fi
  printf '| Cenario | Tipo | Status | Evidencia | Proxima acao |\n'
  printf '|---|---|---|---|---|\n'
  for scenario in $required_scenarios; do
    if scenario_has_status "$scenario" "OK"; then
      row="$(scenario_row "$scenario" "OK")"
      evidence="$(printf '%s' "$row" | cut -f4)"
      printf '| `%s` | REAL | OK | %s | Revisar evidencia e manter bundle anexado para fechamento. |\n' "$scenario" "$(md_cell "$evidence")"
    elif scenario_has_status "$scenario" "INVALIDO"; then
      row="$(scenario_row "$scenario" "INVALIDO")"
      reason="$(printf '%s' "$row" | cut -f5)"
      printf '| `%s` | REAL | INVALIDO | %s | %s |\n' "$scenario" "$(md_cell "$reason")" "$(md_cell "$(scenario_action "$scenario")")"
    else
      printf '| `%s` | REAL | PENDENTE | %s | %s |\n' "$scenario" "$(md_cell "$(scenario_label "$scenario")")" "$(md_cell "$(scenario_action "$scenario")")"
    fi
  done
  if scenario_exists "$controlled_scenario"; then
    row="$(awk -F '\t' -v scenario="$controlled_scenario" '$1 == scenario { print; exit }' "$classified")"
    status="$(printf '%s' "$row" | cut -f3)"
    evidence="$(printf '%s' "$row" | cut -f4)"
    printf '| `%s` | CONTROLADO | %s | %s | %s |\n' "$controlled_scenario" "$status" "$(md_cell "$evidence")" "$(md_cell "$(scenario_action "$controlled_scenario")")"
  else
    printf '| `%s` | CONTROLADO | AUSENTE | %s | %s |\n' "$controlled_scenario" "$(md_cell "$(scenario_label "$controlled_scenario")")" "$(md_cell "$(scenario_action "$controlled_scenario")")"
  fi
} >"$REPORT_FILE"

printf 'report=%s\n' "$REPORT_FILE"
printf 'evidence_root=%s\n' "$evidence_root"
printf 'closure_status=%s\n' "$closure_status"
printf 'closure_reason=%s\n' "$closure_reason"
printf 'closure_acceptance=%s\n' "$closure_acceptance"
printf 'real_evidence_ok=%s/%s\n' "$ok_required" "$total_required"

if [[ "$invalid" -gt 0 ]]; then
  exit 2
fi
if [[ "$REQUIRE_COMPLETE" == "true" && "$pending" -gt 0 ]]; then
  exit 3
fi
if [[ "$REQUIRE_ACCEPTED" == "true" && "$closure_acceptance" != "accepted" ]]; then
  exit 4
fi
