#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

usage() {
  cat >&2 <<'USAGE'
Usage:
  scripts/pkg69_evidence_gap_report.sh [bundle-dir-or-parent ...]

Runs the PKG-69 evidence gate, reads its manifest TSV and writes an actionable
Markdown gap report. When bundle directories are provided, they are mapped
through scripts/pkg69_run_evidence_gate_from_bundles.sh before the report.

Environment:
  PKG69_GAP_REPORT_FILE      Output markdown path.
  PKG69_EVIDENCE_FILE        Gate evidence markdown path.
  PKG69_EVIDENCE_MANIFEST_TSV Gate manifest TSV path.
  PKG69_GAP_REPORT_REQUIRE_COMPLETE=true
                             Exit non-zero when any scenario is pending.
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
EVIDENCE_FILE="${PKG69_EVIDENCE_FILE:-/tmp/aiceberg_pkg69_gap_gate_${timestamp}.md}"
MANIFEST_TSV="${PKG69_EVIDENCE_MANIFEST_TSV:-/tmp/aiceberg_pkg69_gap_manifest_${timestamp}.tsv}"
REPORT_FILE="${PKG69_GAP_REPORT_FILE:-/tmp/aiceberg_pkg69_gap_report_${timestamp}.md}"
REQUIRE_COMPLETE="${PKG69_GAP_REPORT_REQUIRE_COMPLETE:-false}"

run_gate() {
  if [[ "$#" -gt 0 ]]; then
    PKG69_EVIDENCE_FILE="$EVIDENCE_FILE" \
    PKG69_EVIDENCE_MANIFEST_TSV="$MANIFEST_TSV" \
      scripts/pkg69_run_evidence_gate_from_bundles.sh "$@" >/dev/null
  else
    PKG69_EVIDENCE_FILE="$EVIDENCE_FILE" \
    PKG69_EVIDENCE_MANIFEST_TSV="$MANIFEST_TSV" \
      scripts/pkg69_operational_evidence_gate.sh >/dev/null
  fi
}

scenario_action() {
  case "$1" in
    windows-server) printf 'Executar smoke.ps1 e update/rollback controlado em Windows Server, depois empacotar o template preenchido.\n' ;;
    windows-desktop) printf 'Executar smoke.ps1, EventLog real e instalador em Windows desktop, depois empacotar o template preenchido.\n' ;;
    linux-debian) printf 'Executar smoke.sh, systemd e coleta real em Debian/Ubuntu, depois empacotar o template preenchido.\n' ;;
    linux-rhel) printf 'Executar smoke.sh, systemd e instalador em RHEL/Alma/Rocky, depois empacotar o template preenchido.\n' ;;
    docker-runtime) printf 'Validar Docker daemon real com CONTAINER_ENABLED e carga controlada, depois empacotar o template preenchido.\n' ;;
    kubernetes-rbac) printf 'Validar DaemonSet/Helm/RBAC minimo em cluster controlado, depois empacotar o template preenchido.\n' ;;
    proxy-tls) printf 'Validar proxy autenticado real, TLS valido/invalido e rollback de config, depois empacotar o template preenchido.\n' ;;
    clock-skew) printf 'Validar clock/NTP controlado em host real, depois empacotar o template preenchido.\n' ;;
    permission-ebpf) printf 'Validar eBPF/pcap com permissao restrita em Linux real, depois empacotar o template preenchido.\n' ;;
    reboot-during-collection) printf 'Validar reboot real durante coleta e replay sem duplicidade, depois empacotar o template preenchido.\n' ;;
    disk-full) printf 'Validar disco cheio controlado sem corrupcao de outbox e recuperacao, depois empacotar o template preenchido.\n' ;;
    high-volume-overhead) printf 'Validar DogStatsD/OTLP/logs em alto volume com CPU/memoria, depois empacotar o template preenchido.\n' ;;
    relay-hub-direct-hosts) printf 'Validar direct, hub e relay em hosts separados; relay deve enviar somente ao Hub e relay_direct_api_attempts=0.\n' ;;
    remote-update-rollback) printf 'Validar download remoto com artefato assinado, update, falha induzida e rollback de servico.\n' ;;
    *) printf 'Revisar evidencia e ajustar template/bundle conforme gate.\n' ;;
  esac
}

status_label() {
  case "$1" in
    evidence) printf 'OK' ;;
    pending) printf 'PENDENTE' ;;
    invalid-template) printf 'INVALIDO' ;;
    *) printf '%s' "$1" ;;
  esac
}

md_cell() {
  printf '%s' "$1" | tr '|' '/'
}

run_gate "$@"

if [[ ! -s "$MANIFEST_TSV" ]]; then
  echo "gate manifest was not generated: $MANIFEST_TSV" >&2
  exit 65
fi

total=0
ok=0
pending=0
invalid=0
while IFS=$'\t' read -r name status _rest; do
  if [[ "$name" == "name" ]]; then
    continue
  fi
  total=$((total + 1))
  case "$status" in
    evidence) ok=$((ok + 1)) ;;
    pending) pending=$((pending + 1)) ;;
    invalid-template) invalid=$((invalid + 1)) ;;
  esac
done <"$MANIFEST_TSV"

closure_status="BLOQUEADO"
closure_reason="faltam evidencias reais obrigatorias"
if [[ "$invalid" -gt 0 ]]; then
  closure_reason="existem evidencias invalidas"
elif [[ "$pending" -eq 0 ]]; then
  closure_status="PRONTO_PARA_REVISAO"
  closure_reason="todas as evidencias reais estao presentes; ainda exige revisao e aceite explicito"
fi

{
  printf '# PKG-69 - Relatorio de lacunas de evidencia\n\n'
  printf -- '- Gerado UTC: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf -- '- Manifest TSV: `%s`\n' "$MANIFEST_TSV"
  printf -- '- Gate evidence: `%s`\n' "$EVIDENCE_FILE"
  printf -- '- Fechamento: %s - %s.\n' "$closure_status" "$closure_reason"
  printf -- '- Resumo: %s/%s evidencias OK; %s pendentes; %s invalidas.\n\n' "$ok" "$total" "$pending" "$invalid"
  printf '| Cenario | Status | Motivo | Proxima acao |\n'
  printf '|---|---|---|---|\n'
  while IFS=$'\t' read -r name status path _sha _bytes _artifact _artifact_sha _artifact_bytes reason; do
    if [[ "$name" == "name" ]]; then
      continue
    fi
    action="$(scenario_action "$name")"
    if [[ "$status" == "evidence" ]]; then
      reason="path=$path"
      action="Revisar evidencia e manter bundle anexado para fechamento."
    fi
    printf '| `%s` | %s | %s | %s |\n' "$name" "$(status_label "$status")" "$(md_cell "${reason:-"-"}")" "$(md_cell "$action")"
  done <"$MANIFEST_TSV"
} >"$REPORT_FILE"

printf 'report=%s\n' "$REPORT_FILE"
printf 'manifest=%s\n' "$MANIFEST_TSV"
printf 'evidence=%s\n' "$EVIDENCE_FILE"
printf 'closure_status=%s\n' "$closure_status"
printf 'closure_reason=%s\n' "$closure_reason"

if [[ "$invalid" -gt 0 ]]; then
  exit 2
fi
if [[ "$REQUIRE_COMPLETE" == "true" && "$pending" -gt 0 ]]; then
  exit 3
fi
