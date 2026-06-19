#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

usage() {
  cat >&2 <<'USAGE'
Usage:
  scripts/pkg69_collect_host_evidence.sh <scenario-name> <filled-template.md> [output-dir]

Collects read-only host evidence for PKG-69 and creates a bundle through
scripts/pkg69_bundle_evidence.sh.

Set PKG69_RUN_SMOKE=true to also run scripts/smoke.sh and include its logs and
JSON evidence in the raw artifact directory.
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi
if [[ "$#" -lt 2 || "$#" -gt 3 ]]; then
  usage
  exit 64
fi

SCENARIO="$1"
TEMPLATE="$2"
OUT_DIR="${3:-}"
RUN_SMOKE="${PKG69_RUN_SMOKE:-false}"
PYTHON="${PYTHON:-python3}"
SMOKE_STATUS="not-requested"

if [[ ! -f "$TEMPLATE" ]]; then
  echo "template not found: $TEMPLATE" >&2
  exit 65
fi

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="/tmp/aiceberg_pkg69_host_${SCENARIO}_${timestamp}"
fi
RAW_DIR="$OUT_DIR/raw-host"
mkdir -p "$RAW_DIR"

record_cmd() {
  local name="$1"
  shift
  local output="$RAW_DIR/${name}.txt"
  local status="pass"
  {
    printf '$'
    printf ' %q' "$@"
    printf '\n\n'
    "$@"
  } >"$output" 2>&1 || status="fail"
  printf '%s\t%s\t%s\n' "$name" "$status" "$output" >>"$RAW_DIR/COMMANDS.tsv"
}

redacted_env() {
  env | sort | while IFS= read -r line; do
    local key="${line%%=*}"
    local value="${line#*=}"
    local upper_key
    upper_key="$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')"
    case "$line" in
      *=*)
        case "$upper_key" in
          *TOKEN*|*SECRET*|*PASSWORD*|*PASS*|*KEY*|*COOKIE*|*AUTH*|*CREDENTIAL*)
            printf '%s=<redacted>\n' "$key"
            ;;
          HTTP_PROXY|HTTPS_PROXY|NO_PROXY)
            printf '%s=%s\n' "$key" "$(printf '%s' "$value" | sed -E 's#([a-zA-Z][a-zA-Z0-9+.-]*://)[^/@]+@#\1<redacted>@#')"
            ;;
        esac
        ;;
    esac
  done
}

write_collection_summary() {
  local command_count
  local command_pass
  local command_fail
  command_count="$(awk 'END { print NR + 0 }' "$RAW_DIR/COMMANDS.tsv")"
  command_pass="$(awk -F '\t' '$2 == "pass" { count++ } END { print count + 0 }' "$RAW_DIR/COMMANDS.tsv")"
  command_fail="$(awk -F '\t' '$2 == "fail" { count++ } END { print count + 0 }' "$RAW_DIR/COMMANDS.tsv")"
  {
    printf 'key\tvalue\n'
    printf 'scenario\t%s\n' "$SCENARIO"
    printf 'created_at_utc\t%s\n' "$timestamp"
    printf 'host_uname\t%s\n' "$(uname -srm)"
    printf 'command_count\t%s\n' "$command_count"
    printf 'command_pass\t%s\n' "$command_pass"
    printf 'command_fail\t%s\n' "$command_fail"
    printf 'smoke_requested\t%s\n' "$RUN_SMOKE"
    printf 'smoke_status\t%s\n' "$SMOKE_STATUS"
    printf 'redacted_env_file\t%s\n' "proxy_env_redacted.txt"
  } >"$RAW_DIR/COLLECTION_SUMMARY.tsv"
}

{
  printf 'scenario\t%s\n' "$SCENARIO"
  printf 'created_at_utc\t%s\n' "$timestamp"
  printf 'repo\t%s\n' "$ROOT"
  printf 'template\t%s\n' "$TEMPLATE"
} >"$RAW_DIR/README.tsv"
: >"$RAW_DIR/COMMANDS.tsv"

record_cmd uname uname -a
record_cmd hostname hostname
record_cmd whoami whoami
record_cmd date_utc date -u
record_cmd disk df -k .
record_cmd go_version go version
command -v docker >/dev/null 2>&1 && record_cmd docker_info docker info
command -v kubectl >/dev/null 2>&1 && record_cmd kubectl_version kubectl version --client=true
command -v helm >/dev/null 2>&1 && record_cmd helm_version helm version --short
command -v systemctl >/dev/null 2>&1 && record_cmd systemctl_status systemctl status aiceberg-agent

redacted_env >"$RAW_DIR/proxy_env_redacted.txt"

if [[ "$RUN_SMOKE" == "true" ]]; then
  if SMOKE_EVIDENCE_FILE="$RAW_DIR/smoke-evidence.json" \
    PYTHON="$PYTHON" \
    scripts/smoke.sh >"$RAW_DIR/smoke.log" 2>&1; then
    SMOKE_STATUS="pass"
  else
    SMOKE_STATUS="fail"
    echo "smoke failed; see $RAW_DIR/smoke.log" >&2
    exit 66
  fi
fi

write_collection_summary

scripts/pkg69_bundle_evidence.sh "$SCENARIO" "$TEMPLATE" "$RAW_DIR" "$OUT_DIR/bundle"
