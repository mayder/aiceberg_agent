#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TMP_DIR="$(mktemp -d /tmp/aiceberg_pkg69_gap_report_selftest.XXXXXX)"
trap 'rm -rf "$TMP_DIR"' EXIT

assert_contains() {
  local path="$1"
  local pattern="$2"
  grep -Fq -- "$pattern" "$path" || {
    echo "expected pattern not found: $pattern" >&2
    echo "file: $path" >&2
    exit 1
  }
}

set_field() {
  local path="$1"
  local field="$2"
  local value="$3"
  FIELD="$field" VALUE="$value" perl -0pi -e 's#^- \Q$ENV{FIELD}\E:.*$#- $ENV{FIELD}: $ENV{VALUE}#m' "$path"
}

fill_synthetic_template() {
  local path="$1"
  local topology="$2"

  set_field "$path" "Data UTC" "2026-06-18T00:00:00Z"
  set_field "$path" "Responsavel" "gap-report-selftest"
  set_field "$path" "Cliente/lab" "local"
  set_field "$path" "Host/agente/HUB/relay" "selftest-host"
  set_field "$path" "Versao agente" "selftest"
  set_field "$path" "Artefato instalado" "selftest.tar.gz"
  set_field "$path" "Topologia" "$topology"
  set_field "$path" "Status" "pass"
  set_field "$path" "Evidencia bruta anexada" "old.log"
  set_field "$path" "Observacoes" "selftest synthetic complete path"
  set_field "$path" "Rollback validado" "sim"
  set_field "$path" "Revisor" "gap-report-selftest"
  set_field "$path" "Aprovacao fechamento" "yes"

  perl -0pi -e 's#^- ([^:\n]+):[[:space:]]*$#- $1: yes#mg' "$path"
  for field in proc_cpu_percent proc_rss_bytes queue_items containers_seen container_logs_seen pods_seen events_seen requests_ok requests_failed_expected retry_count offset_ms degraded_collectors queued_before replayed_after duplicate_count free_bytes_before accepted_count dropped_count; do
    set_field "$path" "$field" "1"
  done
  set_field "$path" "direct_host_id" "direct-host"
  set_field "$path" "hub_host_id" "hub-host"
  set_field "$path" "relay_host_id" "relay-host"
  set_field "$path" "relay_upstream_host_id" "hub-host"
  set_field "$path" "secrets_allowed" "no"
  set_field "$path" "exec_allowed" "no"
  set_field "$path" "delete_allowed" "no"
  set_field "$path" "status_before" "ok"
  set_field "$path" "status_after" "ok"
  set_field "$path" "relay_direct_api_attempts" "0"
  set_field "$path" "update_report_status" "success"
}

mark_real_template() {
  local path="$1"

  set_field "$path" "Responsavel" "pkg69-reviewer"
  set_field "$path" "Cliente/lab" "controlled-lab-69"
  set_field "$path" "Host/agente/HUB/relay" "controlled-host-69"
  set_field "$path" "Versao agente" "0.8.8"
  set_field "$path" "Artefato instalado" "aiceberg-agent-linux-amd64.tar.gz"
  set_field "$path" "Observacoes" "controlled lab evidence path"
  set_field "$path" "Revisor" "pkg69-reviewer"
}

scenario_for_template() {
  case "$(basename "$1")" in
    windows_server.md) printf 'windows-server\n' ;;
    windows_desktop.md) printf 'windows-desktop\n' ;;
    linux_debian.md) printf 'linux-debian\n' ;;
    linux_rhel.md) printf 'linux-rhel\n' ;;
    docker_runtime.md) printf 'docker-runtime\n' ;;
    kubernetes_rbac.md) printf 'kubernetes-rbac\n' ;;
    proxy_tls.md) printf 'proxy-tls\n' ;;
    clock_skew.md) printf 'clock-skew\n' ;;
    permission_ebpf.md) printf 'permission-ebpf\n' ;;
    reboot_during_collection.md) printf 'reboot-during-collection\n' ;;
    disk_full.md) printf 'disk-full\n' ;;
    high_volume_overhead.md) printf 'high-volume-overhead\n' ;;
    relay_hub_direct_hosts.md) printf 'relay-hub-direct-hosts\n' ;;
    remote_update_rollback.md) printf 'remote-update-rollback\n' ;;
    *) return 1 ;;
  esac
}

template="$TMP_DIR/relay.md"
raw="$TMP_DIR/raw.log"
cat >"$template" <<'EOF'
# PKG-69 - Relay Hub Direct Hosts

## Ambiente

- Data UTC: 2026-06-18T00:00:00Z
- Responsavel: gap-report-selftest
- Cliente/lab: local
- Ambiente: Hosts separados
- Host/agente/HUB/relay: host-1
- Versao agente: selftest
- Artefato instalado: selftest.tar.gz
- Topologia: direct/hub/relay hosts separados

## Evidencia obrigatoria

- direct_host_id: direct-host
- hub_host_id: hub-host
- relay_host_id: relay-host
- relay_upstream_host_id: hub-host
- direct -> AIceberg confirmado: yes
- hub -> AIceberg confirmado: yes
- relay -> hub -> AIceberg confirmado: yes
- relay sem conexao direta com API AIceberg: yes
- agentless via Hub quando aplicavel: yes

## Metricas

- direct_ingested: yes
- hub_ingested: yes
- relay_ingested_via_hub: yes
- relay_direct_api_attempts: 0

## Resultado

- Status: pass
- Evidencia bruta anexada: old.log
- Observacoes: selftest
- Rollback validado: sim
- Revisor: gap-report-selftest
- Aprovacao fechamento: yes
EOF
printf 'raw gap evidence\n' >"$raw"
scripts/pkg69_bundle_evidence.sh relay-hub-direct-hosts "$template" "$raw" "$TMP_DIR/bundles/relay" >/dev/null

PKG69_GAP_REPORT_FILE="$TMP_DIR/report.md" \
PKG69_EVIDENCE_FILE="$TMP_DIR/gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/manifest.tsv" \
scripts/pkg69_evidence_gap_report.sh "$TMP_DIR/bundles" >"$TMP_DIR/report.out" 2>"$TMP_DIR/report.err"

assert_contains "$TMP_DIR/report.md" "# PKG-69 - Relatorio de lacunas de evidencia"
assert_contains "$TMP_DIR/report.md" "- Fechamento: BLOQUEADO - faltam evidencias reais obrigatorias."
assert_contains "$TMP_DIR/report.out" "closure_status=BLOQUEADO"
assert_contains "$TMP_DIR/report.out" "closure_reason=faltam evidencias reais obrigatorias"
assert_contains "$TMP_DIR/report.md" '| `relay-hub-direct-hosts` | OK |'
assert_contains "$TMP_DIR/report.md" '| `windows-server` | PENDENTE |'
assert_contains "$TMP_DIR/report.md" 'Executar smoke.ps1 e update/rollback controlado em Windows Server'
assert_contains "$TMP_DIR/report.md" '13 pendentes'

set +e
PKG69_GAP_REPORT_FILE="$TMP_DIR/required-report.md" \
PKG69_EVIDENCE_FILE="$TMP_DIR/required-gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/required-manifest.tsv" \
PKG69_GAP_REPORT_REQUIRE_COMPLETE=true \
scripts/pkg69_evidence_gap_report.sh "$TMP_DIR/bundles" >/dev/null 2>"$TMP_DIR/required-report.err"
required_exit=$?
set -e
if [[ "$required_exit" -ne 3 ]]; then
  echo "expected incomplete required report exit 3, got $required_exit" >&2
  exit 1
fi
assert_contains "$TMP_DIR/required-report.md" '13 pendentes'

invalid_template="$TMP_DIR/proxy-invalid.md"
cat >"$invalid_template" <<'EOF'
# PKG-69 - Proxy TLS

## Ambiente

- Data UTC: 2026-06-18T00:00:00Z
- Responsavel: gap-report-selftest
- Cliente/lab: local
- Ambiente: Proxy/TLS controlado
- Host/agente/HUB/relay: host-1
- Versao agente: selftest
- Artefato instalado: selftest.tar.gz
- Topologia: direct -> AIceberg

## Evidencia obrigatoria

- proxy autenticado real: yes
- TLS invalido rejeitado: yes
- TLS valido aceito: yes
- sem token em log: yes
- rollback de config validado: yes

## Metricas

- requests_ok: nao-numerico
- requests_failed_expected: 1
- retry_count: 0

## Resultado

- Status: pass
- Evidencia bruta anexada: old.log
- Observacoes: selftest
- Rollback validado: sim
- Revisor: gap-report-selftest
- Aprovacao fechamento: yes
EOF
scripts/pkg69_bundle_evidence.sh proxy-tls "$invalid_template" "$raw" "$TMP_DIR/invalid/proxy" >/dev/null

set +e
PKG69_GAP_REPORT_FILE="$TMP_DIR/invalid-report.md" \
PKG69_EVIDENCE_FILE="$TMP_DIR/invalid-gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/invalid-manifest.tsv" \
scripts/pkg69_evidence_gap_report.sh "$TMP_DIR/invalid" >"$TMP_DIR/invalid-report.out" 2>"$TMP_DIR/invalid-report.err"
invalid_exit=$?
set -e
if [[ "$invalid_exit" -ne 2 ]]; then
  echo "expected invalid gap report exit 2, got $invalid_exit" >&2
  exit 1
fi
assert_contains "$TMP_DIR/invalid-report.md" '| `proxy-tls` | INVALIDO | field requests_ok must be numeric |'
assert_contains "$TMP_DIR/invalid-report.md" "- Fechamento: BLOQUEADO - existem evidencias invalidas."
assert_contains "$TMP_DIR/invalid-report.out" "closure_reason=existem evidencias invalidas"

templates_dir="$TMP_DIR/templates"
PKG69_TEMPLATE_DIR="$templates_dir" \
PKG69_EVIDENCE_FILE="$TMP_DIR/template-generation.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/template-generation.tsv" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null

for generated_template in "$templates_dir"/*.md; do
  scenario="$(scenario_for_template "$generated_template")"
  topology="direct -> AIceberg"
  if [[ "$scenario" == "relay-hub-direct-hosts" ]]; then
    topology="direct/hub/relay hosts separados"
  fi
  fill_synthetic_template "$generated_template" "$topology"
  scripts/pkg69_bundle_evidence.sh "$scenario" "$generated_template" "$raw" "$TMP_DIR/all-bundles/$scenario" >/dev/null
done

PKG69_GAP_REPORT_FILE="$TMP_DIR/complete-report.md" \
PKG69_EVIDENCE_FILE="$TMP_DIR/complete-gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/complete-manifest.tsv" \
scripts/pkg69_evidence_gap_report.sh "$TMP_DIR/all-bundles" >"$TMP_DIR/complete-report.out" 2>"$TMP_DIR/complete-report.err"

assert_contains "$TMP_DIR/complete-report.md" "- Fechamento: PRONTO_PARA_REVISAO - todas as evidencias reais estao presentes; ainda exige revisao e aceite explicito."
assert_contains "$TMP_DIR/complete-report.md" "14/14 evidencias OK; 0 pendentes; 0 invalidas."
assert_contains "$TMP_DIR/complete-report.out" "closure_status=PRONTO_PARA_REVISAO"
assert_contains "$TMP_DIR/complete-report.out" "closure_acceptance=missing"

set +e
PKG69_GAP_REPORT_FILE="$TMP_DIR/complete-required-accepted-report.md" \
PKG69_EVIDENCE_FILE="$TMP_DIR/complete-required-accepted-gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/complete-required-accepted-manifest.tsv" \
PKG69_GAP_REPORT_REQUIRE_ACCEPTED=true \
scripts/pkg69_evidence_gap_report.sh "$TMP_DIR/all-bundles" >"$TMP_DIR/complete-required-accepted.out" 2>"$TMP_DIR/complete-required-accepted.err"
required_accept_exit=$?
set -e
if [[ "$required_accept_exit" -ne 2 ]]; then
  echo "expected accepted required report exit 2 with synthetic markers, got $required_accept_exit" >&2
  exit 1
fi
assert_contains "$TMP_DIR/complete-required-accepted-report.md" "must not contain self-test or synthetic markers when real evidence is required"

real_templates_dir="$TMP_DIR/real-templates"
PKG69_TEMPLATE_DIR="$real_templates_dir" \
PKG69_EVIDENCE_FILE="$TMP_DIR/real-template-generation.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/real-template-generation.tsv" \
scripts/pkg69_operational_evidence_gate.sh >/dev/null

for generated_template in "$real_templates_dir"/*.md; do
  scenario="$(scenario_for_template "$generated_template")"
  topology="direct -> AIceberg"
  if [[ "$scenario" == "relay-hub-direct-hosts" ]]; then
    topology="direct/hub/relay hosts separados"
  fi
  fill_synthetic_template "$generated_template" "$topology"
  mark_real_template "$generated_template"
  scripts/pkg69_bundle_evidence.sh "$scenario" "$generated_template" "$raw" "$TMP_DIR/real-bundles/$scenario" >/dev/null
done

set +e
PKG69_GAP_REPORT_FILE="$TMP_DIR/real-complete-required-accepted-report.md" \
PKG69_EVIDENCE_FILE="$TMP_DIR/real-complete-required-accepted-gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/real-complete-required-accepted-manifest.tsv" \
PKG69_GAP_REPORT_REQUIRE_ACCEPTED=true \
scripts/pkg69_evidence_gap_report.sh "$TMP_DIR/real-bundles" >"$TMP_DIR/real-complete-required-accepted.out" 2>"$TMP_DIR/real-complete-required-accepted.err"
real_required_accept_exit=$?
set -e
if [[ "$real_required_accept_exit" -ne 4 ]]; then
  echo "expected accepted required report exit 4 without acceptance, got $real_required_accept_exit" >&2
  exit 1
fi
assert_contains "$TMP_DIR/real-complete-required-accepted.out" "closure_status=PRONTO_PARA_REVISAO"

PKG69_GAP_REPORT_FILE="$TMP_DIR/accepted-report.md" \
PKG69_EVIDENCE_FILE="$TMP_DIR/accepted-gate.md" \
PKG69_EVIDENCE_MANIFEST_TSV="$TMP_DIR/accepted-manifest.tsv" \
PKG69_GAP_REPORT_REQUIRE_ACCEPTED=true \
PKG69_ACCEPT_CLOSURE=true \
scripts/pkg69_evidence_gap_report.sh "$TMP_DIR/real-bundles" >"$TMP_DIR/accepted-report.out" 2>"$TMP_DIR/accepted-report.err"

assert_contains "$TMP_DIR/accepted-report.md" "- Fechamento: ACEITO_PARA_FECHAMENTO - todas as evidencias reais estao presentes e PKG69_ACCEPT_CLOSURE=true."
assert_contains "$TMP_DIR/accepted-report.out" "closure_status=ACEITO_PARA_FECHAMENTO"
assert_contains "$TMP_DIR/accepted-report.out" "closure_acceptance=accepted"

echo "PKG-69 evidence gap report self-test OK"
