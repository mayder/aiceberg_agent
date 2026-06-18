#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

EVIDENCE_FILE="${PKG69_EVIDENCE_FILE:-/tmp/aiceberg_pkg69_operational_evidence.md}"
EVIDENCE_MANIFEST_TSV="${PKG69_EVIDENCE_MANIFEST_TSV:-}"
TEMPLATE_DIR="${PKG69_TEMPLATE_DIR:-}"
REQUIRE_REAL_EVIDENCE="${PKG69_REQUIRE_REAL_EVIDENCE:-false}"
REQUIRE_CLOSURE_ACCEPTED="${PKG69_REQUIRE_CLOSURE_ACCEPTED:-false}"
REJECT_SYNTHETIC_EVIDENCE="${PKG69_REJECT_SYNTHETIC_EVIDENCE:-false}"
ACCEPT_CLOSURE="${PKG69_ACCEPT_CLOSURE:-false}"
REAL_EVIDENCE_PRESENT=0
REAL_EVIDENCE_TOTAL=0

section() {
  printf '\n## %s\n\n' "$1" | tee -a "$EVIDENCE_FILE" >/dev/null
}

result() {
  local name="$1"
  local status="$2"
  local detail="$3"
  printf -- '- %s: %s — %s\n' "$name" "$status" "$detail" | tee -a "$EVIDENCE_FILE" >/dev/null
}

manifest_result() {
  if [[ -z "$EVIDENCE_MANIFEST_TSV" ]]; then
    return
  fi
  local name="$1"
  local status="$2"
  local path="$3"
  local sha256="$4"
  local bytes="$5"
  local artifact_path="$6"
  local artifact_sha256="$7"
  local artifact_bytes="$8"
  local reason="$9"
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$name" "$status" "$path" "$sha256" "$bytes" "$artifact_path" "$artifact_sha256" "$artifact_bytes" "$reason" >>"$EVIDENCE_MANIFEST_TSV"
}

file_size_bytes() {
  wc -c <"$1" | tr -d ' '
}

file_sha256() {
  shasum -a 256 "$1" | awk '{print $1}'
}

resolve_artifact_path() {
  local template_path="$1"
  local value="$2"
  if [[ "$value" = /* ]]; then
    printf '%s\n' "$value"
  else
    printf '%s/%s\n' "$(dirname "$template_path")" "$value"
  fi
}

artifact_size_bytes() {
  local path="$1"
  if [[ -f "$path" ]]; then
    file_size_bytes "$path"
    return
  fi
  find "$path" -type f -exec wc -c {} + | awk '{sum += $1} END {print sum + 0}'
}

artifact_sha256() {
  local path="$1"
  if [[ -f "$path" ]]; then
    file_sha256 "$path"
    return
  fi
  (
    cd "$path"
    find . -type f -print | sort | while IFS= read -r file; do
      shasum -a 256 "$file"
    done
  ) | shasum -a 256 | awk '{print $1}'
}

field_value() {
  local path="$1"
  local field="$2"
  awk -v field="$field" '
    BEGIN { prefix = "- " field ":" }
    index($0, prefix) == 1 {
      value = substr($0, length(prefix) + 1)
      sub(/^[[:space:]]*/, "", value)
      print value
      exit
    }
  ' "$path"
}

require_exact_field() {
  local path="$1"
  local field="$2"
  local expected="$3"
  local value
  value="$(field_value "$path" "$field")"
  if [[ "$value" != "$expected" ]]; then
    echo "field $field must be $expected"
    return 0
  fi
  return 1
}

require_bool_field() {
  local path="$1"
  local field="$2"
  local value
  value="$(field_value "$path" "$field")"
  if [[ "$value" != "yes" && "$value" != "true" && "$value" != "sim" ]]; then
    echo "field $field must be yes, true or sim"
    return 0
  fi
  return 1
}

require_utc_timestamp_field() {
  local path="$1"
  local field="$2"
  local value
  value="$(field_value "$path" "$field")"
  if ! [[ "$value" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
    echo "field $field must use UTC format YYYY-MM-DDTHH:MM:SSZ"
    return 0
  fi
  return 1
}

require_existing_artifact_field() {
  local path="$1"
  local field="$2"
  local value
  value="$(field_value "$path" "$field")"
  if [[ -z "$value" ]]; then
    echo "field $field must point to an existing evidence artifact"
    return 0
  fi
  if [[ "$value" =~ ^https?:// ]]; then
    echo "field $field must use a local evidence artifact path, not URL"
    return 0
  fi
  local artifact_path
  artifact_path="$(resolve_artifact_path "$path" "$value")"
  if [[ ! -e "$artifact_path" ]]; then
    echo "field $field artifact does not exist: $value"
    return 0
  fi
  if [[ -f "$artifact_path" && ! -s "$artifact_path" ]]; then
    echo "field $field artifact is empty: $value"
    return 0
  fi
  if [[ -d "$artifact_path" && "$(artifact_size_bytes "$artifact_path")" == "0" ]]; then
    echo "field $field artifact directory has no non-empty files: $value"
    return 0
  fi
  return 1
}

require_topology_field() {
  local path="$1"
  local value
  value="$(field_value "$path" "Topologia")"
  if [[ "$value" == "direct|hub|relay -> hub -> AIceberg" ]]; then
    echo "field Topologia placeholder not filled"
    return 0
  fi
  case "$value" in
    "direct -> AIceberg"|"hub -> AIceberg"|"relay -> hub -> AIceberg"|"direct/hub/relay hosts separados")
      return 1
      ;;
  esac
  echo "field Topologia must be direct -> AIceberg, hub -> AIceberg, relay -> hub -> AIceberg or direct/hub/relay hosts separados"
  return 0
}

require_number_field() {
  local path="$1"
  local field="$2"
  local value
  value="$(field_value "$path" "$field")"
  if ! [[ "$value" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    echo "field $field must be numeric"
    return 0
  fi
  return 1
}

require_number_max_field() {
  local path="$1"
  local field="$2"
  local max="$3"
  local value
  value="$(field_value "$path" "$field")"
  if ! [[ "$value" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    echo "field $field must be numeric"
    return 0
  fi
  if ! awk -v value="$value" -v max="$max" 'BEGIN { exit(value <= max ? 0 : 1) }'; then
    echo "field $field must be <= $max"
    return 0
  fi
  return 1
}

require_one_of_field() {
  local path="$1"
  local field="$2"
  shift 2
  local value
  value="$(field_value "$path" "$field")"
  local allowed
  for allowed in "$@"; do
    if [[ "$value" == "$allowed" ]]; then
      return 1
    fi
  done
  echo "field $field must be one of: $*"
  return 0
}

require_no_synthetic_marker() {
  local path="$1"
  local field
  local value
  for field in "Responsavel" "Cliente/lab" "Versao agente" "Artefato instalado" "Observacoes" "Revisor"; do
    value="$(field_value "$path" "$field")"
    if [[ "$value" =~ [Ss]elftest|[Ss]ynthetic|[Ss]intetico|[Ff]ake|[Mm]ock|[Pp]laceholder ]]; then
      echo "field $field must not contain self-test or synthetic markers when real evidence is required"
      return 0
    fi
  done
  return 1
}

scenario_incomplete_reason() {
  local path="$1"
  local expected_title="$2"
  local reason
  case "$expected_title" in
    "PKG-69 - Windows Server"|"PKG-69 - Linux Debian Ubuntu"|"PKG-69 - Linux RHEL Alma Rocky")
      if reason="$(require_bool_field "$path" "ingest_confirmed")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_max_field "$path" "proc_cpu_percent" "5")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_max_field "$path" "proc_rss_bytes" "262144000")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_field "$path" "queue_items")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Windows Desktop")
      if reason="$(require_bool_field "$path" "ingest_confirmed")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_max_field "$path" "proc_cpu_percent" "5")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_max_field "$path" "proc_rss_bytes" "262144000")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Docker Runtime")
      if reason="$(require_number_field "$path" "containers_seen")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_field "$path" "container_logs_seen")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_max_field "$path" "proc_cpu_percent" "10")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_max_field "$path" "proc_rss_bytes" "419430400")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Kubernetes RBAC")
      if reason="$(require_number_field "$path" "pods_seen")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_field "$path" "events_seen")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_max_field "$path" "proc_cpu_percent" "10")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_max_field "$path" "proc_rss_bytes" "419430400")"; then echo "$reason"; return 0; fi
      if reason="$(require_exact_field "$path" "secrets_allowed" "no")"; then echo "$reason"; return 0; fi
      if reason="$(require_exact_field "$path" "exec_allowed" "no")"; then echo "$reason"; return 0; fi
      if reason="$(require_exact_field "$path" "delete_allowed" "no")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Proxy TLS")
      if reason="$(require_number_field "$path" "requests_ok")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_field "$path" "requests_failed_expected")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_field "$path" "retry_count")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Clock Skew Real")
      if reason="$(require_number_field "$path" "offset_ms")"; then echo "$reason"; return 0; fi
      if reason="$(require_one_of_field "$path" "status_before" "ok" "warning" "critical")"; then echo "$reason"; return 0; fi
      if reason="$(require_one_of_field "$path" "status_after" "ok" "warning" "critical")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Permissao eBPF Restrita")
      if reason="$(require_bool_field "$path" "ingest_confirmed")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_max_field "$path" "proc_cpu_percent" "5")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Reboot Durante Coleta")
      if reason="$(require_number_field "$path" "queued_before")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_field "$path" "replayed_after")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_field "$path" "duplicate_count")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Disco Cheio Real")
      if reason="$(require_number_field "$path" "free_bytes_before")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_field "$path" "queue_items")"; then echo "$reason"; return 0; fi
      if reason="$(require_bool_field "$path" "recovered")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Alto Volume e Overhead")
      if reason="$(require_number_max_field "$path" "proc_cpu_percent" "15")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_max_field "$path" "proc_rss_bytes" "524288000")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_field "$path" "accepted_count")"; then echo "$reason"; return 0; fi
      if reason="$(require_number_field "$path" "dropped_count")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Relay Hub Direct Hosts")
      if reason="$(require_bool_field "$path" "direct -> AIceberg confirmado")"; then echo "$reason"; return 0; fi
      if reason="$(require_bool_field "$path" "hub -> AIceberg confirmado")"; then echo "$reason"; return 0; fi
      if reason="$(require_bool_field "$path" "relay -> hub -> AIceberg confirmado")"; then echo "$reason"; return 0; fi
      if reason="$(require_bool_field "$path" "relay sem conexao direta com API AIceberg")"; then echo "$reason"; return 0; fi
      if reason="$(require_bool_field "$path" "agentless via Hub quando aplicavel")"; then echo "$reason"; return 0; fi
      if reason="$(require_bool_field "$path" "direct_ingested")"; then echo "$reason"; return 0; fi
      if reason="$(require_bool_field "$path" "hub_ingested")"; then echo "$reason"; return 0; fi
      if reason="$(require_bool_field "$path" "relay_ingested_via_hub")"; then echo "$reason"; return 0; fi
      if reason="$(require_exact_field "$path" "relay_direct_api_attempts" "0")"; then echo "$reason"; return 0; fi
      ;;
    "PKG-69 - Update Remoto e Rollback")
      if reason="$(require_bool_field "$path" "version_confirmed reportado")"; then echo "$reason"; return 0; fi
      if reason="$(require_one_of_field "$path" "update_report_status" "success" "rolled_back" "apply_failed")"; then echo "$reason"; return 0; fi
      ;;
  esac
  return 1
}

template_incomplete_reason() {
  local path="$1"
  local expected_title="$2"
  if ! grep -Fxq "# $expected_title" "$path"; then
    echo "template title mismatch"
    return 0
  fi
  if grep -Eq '^- Status: pending\|pass\|fail$' "$path"; then
    echo "template status placeholder not filled"
    return 0
  fi
  if ! grep -Eq '^- Status: pass$' "$path"; then
    echo "template status is not pass"
    return 0
  fi
  if grep -Eq '^- Aprovacao fechamento: pending\|yes\|no$' "$path"; then
    echo "template closure approval placeholder not filled"
    return 0
  fi
  if ! grep -Eq '^- Aprovacao fechamento: yes$' "$path"; then
    echo "template closure approval is not yes"
    return 0
  fi
  if grep -Eq '^- [^:]+:[[:space:]]*$' "$path"; then
    echo "template required field blank"
    return 0
  fi
  local data_utc_reason
  if data_utc_reason="$(require_utc_timestamp_field "$path" "Data UTC")"; then
    echo "$data_utc_reason"
    return 0
  fi
  local evidence_reason
  if evidence_reason="$(require_existing_artifact_field "$path" "Evidencia bruta anexada")"; then
    echo "$evidence_reason"
    return 0
  fi
  local rollback_reason
  if rollback_reason="$(require_bool_field "$path" "Rollback validado")"; then
    echo "$rollback_reason"
    return 0
  fi
  local topology_reason
  if topology_reason="$(require_topology_field "$path")"; then
    echo "$topology_reason"
    return 0
  fi
  local scenario_reason
  if scenario_reason="$(scenario_incomplete_reason "$path" "$expected_title")"; then
    echo "$scenario_reason"
    return 0
  fi
  if [[ "$REQUIRE_REAL_EVIDENCE" == "true" || "$REQUIRE_CLOSURE_ACCEPTED" == "true" || "$REJECT_SYNTHETIC_EVIDENCE" == "true" ]]; then
    local synthetic_reason
    if synthetic_reason="$(require_no_synthetic_marker "$path")"; then
      echo "$synthetic_reason"
      return 0
    fi
  fi
  return 1
}

require_file_or_pending() {
  local name="$1"
  local path="$2"
  local detail="$3"
  local expected_title="$4"
  REAL_EVIDENCE_TOTAL=$((REAL_EVIDENCE_TOTAL + 1))
  if [[ -n "$path" && -f "$path" ]]; then
    local incomplete_reason
    if incomplete_reason="$(template_incomplete_reason "$path" "$expected_title")"; then
      result "$name" "invalid-template" "path=$path reason=$incomplete_reason"
      manifest_result "$name" "invalid-template" "$path" "-" "-" "-" "-" "-" "$incomplete_reason"
      return
    fi
    REAL_EVIDENCE_PRESENT=$((REAL_EVIDENCE_PRESENT + 1))
    local sha256
    local bytes
    local raw_value
    local raw_path
    local raw_sha256
    local raw_bytes
    sha256="$(file_sha256 "$path")"
    bytes="$(file_size_bytes "$path")"
    raw_value="$(field_value "$path" "Evidencia bruta anexada")"
    raw_path="$(resolve_artifact_path "$path" "$raw_value")"
    raw_sha256="$(artifact_sha256 "$raw_path")"
    raw_bytes="$(artifact_size_bytes "$raw_path")"
    result "$name" "evidence" "path=$path sha256=$sha256 bytes=$bytes artifact=$raw_path artifact_sha256=$raw_sha256 artifact_bytes=$raw_bytes"
    manifest_result "$name" "evidence" "$path" "$sha256" "$bytes" "$raw_path" "$raw_sha256" "$raw_bytes" "-"
  else
    result "$name" "pending" "$detail"
    manifest_result "$name" "pending" "-" "-" "-" "-" "-" "-" "$detail"
  fi
}

write_template() {
  local file="$1"
  local title="$2"
  local environment="$3"
  local required="$4"
  local metrics="$5"
  if [[ -f "$file" ]]; then
    return
  fi
  cat >"$file" <<EOF
# $title

## Ambiente

- Data UTC:
- Responsavel:
- Cliente/lab:
- Ambiente: $environment
- Host/agente/HUB/relay:
- Versao agente:
- Artefato instalado:
- Topologia: direct|hub|relay -> hub -> AIceberg

## Evidencia obrigatoria

$required

## Metricas

$metrics

## Resultado

- Status: pending|pass|fail
- Evidencia bruta anexada:
- Observacoes:
- Rollback validado:
- Revisor:
- Aprovacao fechamento: pending|yes|no
EOF
}

generate_templates() {
  if [[ -z "$TEMPLATE_DIR" ]]; then
    return
  fi
  mkdir -p "$TEMPLATE_DIR"
  write_template "$TEMPLATE_DIR/windows_server.md" \
    "PKG-69 - Windows Server" \
    "Windows Server" \
    "- smoke.ps1 executado:
- EventLog Security/System/Application coletado:
- servico Windows instalado/reiniciado:
- update controlado com rollback:
- logs sem segredo:" \
    "- proc_cpu_percent:
- proc_rss_bytes:
- queue_items:
- ingest_confirmed:"
  write_template "$TEMPLATE_DIR/windows_desktop.md" \
    "PKG-69 - Windows Desktop" \
    "Windows desktop" \
    "- smoke.ps1 executado:
- EventLog real coletado:
- instalador validado:
- proxy autenticado quando aplicavel:
- logs sem segredo:" \
    "- proc_cpu_percent:
- proc_rss_bytes:
- ingest_confirmed:"
  write_template "$TEMPLATE_DIR/linux_debian.md" \
    "PKG-69 - Linux Debian Ubuntu" \
    "Debian/Ubuntu" \
    "- smoke.sh executado:
- systemd service validado:
- syslog/journald real coletado:
- update controlado:
- logs sem segredo:" \
    "- proc_cpu_percent:
- proc_rss_bytes:
- queue_items:
- ingest_confirmed:"
  write_template "$TEMPLATE_DIR/linux_rhel.md" \
    "PKG-69 - Linux RHEL Alma Rocky" \
    "RHEL/Alma/Rocky" \
    "- smoke.sh executado:
- systemd service validado:
- dnf/yum install/update validado:
- syslog/journald real coletado:
- logs sem segredo:" \
    "- proc_cpu_percent:
- proc_rss_bytes:
- queue_items:
- ingest_confirmed:"
  write_template "$TEMPLATE_DIR/docker_runtime.md" \
    "PKG-69 - Docker Runtime" \
    "Docker host" \
    "- CONTAINER_ENABLED=true:
- Docker socket real acessado:
- labels sensiveis mascaradas:
- logs JSON com cursor:
- carga controlada executada:" \
    "- containers_seen:
- container_logs_seen:
- proc_cpu_percent:
- proc_rss_bytes:"
  write_template "$TEMPLATE_DIR/kubernetes_rbac.md" \
    "PKG-69 - Kubernetes RBAC" \
    "Kubernetes" \
    "- DaemonSet aplicado:
- Helm install/upgrade/rollback:
- ServiceAccount com RBAC minimo:
- pods/events/logs coletados:
- sem permissao a secrets/exec/delete:" \
    "- pods_seen:
- events_seen:
- proc_cpu_percent:
- proc_rss_bytes:
- secrets_allowed:
- exec_allowed:
- delete_allowed:"
  write_template "$TEMPLATE_DIR/proxy_tls.md" \
    "PKG-69 - Proxy TLS" \
    "Proxy/TLS controlado" \
    "- proxy autenticado real:
- TLS invalido rejeitado:
- TLS valido aceito:
- sem token em log:
- rollback de config validado:" \
    "- requests_ok:
- requests_failed_expected:
- retry_count:"
  write_template "$TEMPLATE_DIR/clock_skew.md" \
    "PKG-69 - Clock Skew Real" \
    "Host com clock/NTP controlado" \
    "- clock skew aplicado:
- time_sync.status reportado:
- coleta continua sem travar:
- retorno ao clock correto:
- evidencias sem segredo:" \
    "- offset_ms:
- status_before:
- status_after:"
  write_template "$TEMPLATE_DIR/permission_ebpf.md" \
    "PKG-69 - Permissao eBPF Restrita" \
    "Linux restrito" \
    "- eBPF/pcap sem permissao:
- degradacao clara reportada:
- agente segue coletando sinais basicos:
- sem execucao remota insegura:
- logs sem segredo:" \
    "- degraded_collectors:
- ingest_confirmed:
- proc_cpu_percent:"
  write_template "$TEMPLATE_DIR/reboot_during_collection.md" \
    "PKG-69 - Reboot Durante Coleta" \
    "Host real" \
    "- coleta ativa antes do reboot:
- outbox preservada:
- servico voltou automaticamente:
- replay sem duplicidade indevida:
- rollback validado:" \
    "- queued_before:
- replayed_after:
- duplicate_count:"
  write_template "$TEMPLATE_DIR/disk_full.md" \
    "PKG-69 - Disco Cheio Real" \
    "Host com disco controlado" \
    "- disco cheio induzido:
- outbox nao corrompeu:
- status claro reportado:
- recuperacao apos liberar disco:
- logs sem segredo:" \
    "- free_bytes_before:
- queue_items:
- recovered:"
  write_template "$TEMPLATE_DIR/high_volume_overhead.md" \
    "PKG-69 - Alto Volume e Overhead" \
    "Carga DogStatsD/OTLP/logs" \
    "- DogStatsD alto volume:
- OTLP alto volume:
- logs altos:
- limites/drop contabilizados:
- sem travamento:" \
    "- proc_cpu_percent:
- proc_rss_bytes:
- accepted_count:
- dropped_count:"
  write_template "$TEMPLATE_DIR/relay_hub_direct_hosts.md" \
    "PKG-69 - Relay Hub Direct Hosts" \
    "Hosts separados" \
    "- direct -> AIceberg confirmado:
- hub -> AIceberg confirmado:
- relay -> hub -> AIceberg confirmado:
- relay sem conexao direta com API AIceberg:
- agentless via Hub quando aplicavel:" \
    "- direct_ingested:
- hub_ingested:
- relay_ingested_via_hub:
- relay_direct_api_attempts:"
  write_template "$TEMPLATE_DIR/remote_update_rollback.md" \
    "PKG-69 - Update Remoto e Rollback" \
    "Update remoto controlado" \
    "- artefato assinado baixado:
- SHA256 validado:
- update aplicado:
- falha induzida reverteu:
- version_confirmed reportado:" \
    "- version_before:
- version_after:
- rollback_version:
- update_report_status:"
}

: >"$EVIDENCE_FILE"
if [[ -n "$EVIDENCE_MANIFEST_TSV" ]]; then
  printf 'name\tstatus\tpath\tsha256\tbytes\tartifact_path\tartifact_sha256\tartifact_bytes\treason\n' >"$EVIDENCE_MANIFEST_TSV"
fi
generate_templates

section "PKG-69 operational evidence gate"
result "repo" "info" "$ROOT"
result "os" "info" "$(uname -srm)"

section "local baseline"
result "local-homologation" "reference" "run scripts/pkg69_operational_homologation.sh before closure review"
result "relay-rule" "required" "relay evidence must prove relay -> hub -> AIceberg and relay_direct_api_attempts=0"

section "required real evidence"
if [[ -n "$TEMPLATE_DIR" ]]; then
  result "evidence-templates" "written" "$TEMPLATE_DIR"
fi
require_file_or_pending "windows-server" "${PKG69_WINDOWS_SERVER_EVIDENCE:-}" "set PKG69_WINDOWS_SERVER_EVIDENCE to Windows Server evidence" "PKG-69 - Windows Server"
require_file_or_pending "windows-desktop" "${PKG69_WINDOWS_DESKTOP_EVIDENCE:-}" "set PKG69_WINDOWS_DESKTOP_EVIDENCE to Windows desktop evidence" "PKG-69 - Windows Desktop"
require_file_or_pending "linux-debian" "${PKG69_LINUX_DEBIAN_EVIDENCE:-}" "set PKG69_LINUX_DEBIAN_EVIDENCE to Debian/Ubuntu evidence" "PKG-69 - Linux Debian Ubuntu"
require_file_or_pending "linux-rhel" "${PKG69_LINUX_RHEL_EVIDENCE:-}" "set PKG69_LINUX_RHEL_EVIDENCE to RHEL/Alma/Rocky evidence" "PKG-69 - Linux RHEL Alma Rocky"
require_file_or_pending "docker-runtime" "${PKG69_DOCKER_RUNTIME_EVIDENCE:-}" "set PKG69_DOCKER_RUNTIME_EVIDENCE to Docker runtime evidence" "PKG-69 - Docker Runtime"
require_file_or_pending "kubernetes-rbac" "${PKG69_KUBERNETES_RBAC_EVIDENCE:-}" "set PKG69_KUBERNETES_RBAC_EVIDENCE to Kubernetes RBAC evidence" "PKG-69 - Kubernetes RBAC"
require_file_or_pending "proxy-tls" "${PKG69_PROXY_TLS_EVIDENCE:-}" "set PKG69_PROXY_TLS_EVIDENCE to proxy/TLS evidence" "PKG-69 - Proxy TLS"
require_file_or_pending "clock-skew" "${PKG69_CLOCK_SKEW_EVIDENCE:-}" "set PKG69_CLOCK_SKEW_EVIDENCE to real clock skew evidence" "PKG-69 - Clock Skew Real"
require_file_or_pending "permission-ebpf" "${PKG69_PERMISSION_EBPF_EVIDENCE:-}" "set PKG69_PERMISSION_EBPF_EVIDENCE to restricted permission/eBPF evidence" "PKG-69 - Permissao eBPF Restrita"
require_file_or_pending "reboot-during-collection" "${PKG69_REBOOT_EVIDENCE:-}" "set PKG69_REBOOT_EVIDENCE to real reboot/replay evidence" "PKG-69 - Reboot Durante Coleta"
require_file_or_pending "disk-full" "${PKG69_DISK_FULL_EVIDENCE:-}" "set PKG69_DISK_FULL_EVIDENCE to real disk-full evidence" "PKG-69 - Disco Cheio Real"
require_file_or_pending "high-volume-overhead" "${PKG69_HIGH_VOLUME_EVIDENCE:-}" "set PKG69_HIGH_VOLUME_EVIDENCE to high-volume CPU/memory evidence" "PKG-69 - Alto Volume e Overhead"
require_file_or_pending "relay-hub-direct-hosts" "${PKG69_RELAY_HUB_DIRECT_EVIDENCE:-}" "set PKG69_RELAY_HUB_DIRECT_EVIDENCE to separated-host relay/hub/direct evidence" "PKG-69 - Relay Hub Direct Hosts"
require_file_or_pending "remote-update-rollback" "${PKG69_REMOTE_UPDATE_ROLLBACK_EVIDENCE:-}" "set PKG69_REMOTE_UPDATE_ROLLBACK_EVIDENCE to signed remote update/rollback evidence" "PKG-69 - Update Remoto e Rollback"

section "closure rule"
closure_accepted=0
if [[ "$REAL_EVIDENCE_PRESENT" -eq "$REAL_EVIDENCE_TOTAL" ]]; then
  result "real-evidence-manifest" "ready-for-review" "$REAL_EVIDENCE_PRESENT/$REAL_EVIDENCE_TOTAL files present with SHA256; explicit closure acceptance still required"
  if [[ "$ACCEPT_CLOSURE" == "true" ]]; then
    closure_accepted=1
  fi
else
  result "real-evidence-manifest" "incomplete" "$REAL_EVIDENCE_PRESENT/$REAL_EVIDENCE_TOTAL files present"
fi
if [[ "$closure_accepted" -eq 1 ]]; then
  result "pkg69-status" "accepted-for-closure" "all required real evidence is present and PKG69_ACCEPT_CLOSURE=true"
else
  result "pkg69-status" "not-closed" "do not mark PKG-69 100% until every required real evidence item above is present and PKG69_ACCEPT_CLOSURE=true"
fi
result "evidence-file" "written" "$EVIDENCE_FILE"
if [[ -n "$EVIDENCE_MANIFEST_TSV" ]]; then
  result "evidence-manifest-tsv" "written" "$EVIDENCE_MANIFEST_TSV"
fi

if [[ "$REQUIRE_REAL_EVIDENCE" == "true" && "$REAL_EVIDENCE_PRESENT" -ne "$REAL_EVIDENCE_TOTAL" ]]; then
  result "gate" "failed" "PKG69_REQUIRE_REAL_EVIDENCE=true and real evidence manifest is incomplete"
  exit 2
fi

if [[ "$REQUIRE_CLOSURE_ACCEPTED" == "true" && "$closure_accepted" -ne 1 ]]; then
  result "gate" "failed" "PKG69_REQUIRE_CLOSURE_ACCEPTED=true and closure acceptance is missing"
  exit 3
fi
