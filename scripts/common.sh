#!/usr/bin/env bash
set -euo pipefail

log() {
  echo "[check:modelo] $*"
}

fail() {
  echo "[check:modelo] ERROR: $*" >&2
  exit 1
}

require_file() {
  [[ -s "$1" ]] || fail "arquivo obrigatório ausente ou vazio: $1"
}

toml_string_value() {
  local section="$1"
  local key="$2"
  awk -v target_section="[$section]" -v target_key="$key" '
    $0 == target_section { in_section = 1; next }
    /^\[/ { in_section = 0 }
    in_section && $1 == target_key {
      value = $0
      sub(/^[^=]*=[[:space:]]*/, "", value)
      gsub(/^"/, "", value)
      gsub(/"$/, "", value)
      print value
      exit
    }
  ' PATHS.toml
}

toml_array_values() {
  local section="$1"
  local key="$2"
  awk -v target_section="[$section]" -v target_key="$key" '
    $0 == target_section { in_section = 1; next }
    /^\[/ { in_section = 0 }
    in_section && $1 == target_key && $0 ~ /\[[[:space:]]*\]/ { next }
    in_section && $1 == target_key && $0 ~ /\[/ { in_array = 1; next }
    in_array && /\]/ { in_array = 0; next }
    in_array {
      value = $0
      gsub(/[",]/, "", value)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      if (value != "") print value
    }
  ' PATHS.toml
}

toml_int_value() {
  local section="$1"
  local key="$2"
  awk -v target_section="[$section]" -v target_key="$key" '
    $0 == target_section { in_section = 1; next }
    /^\[/ { in_section = 0 }
    in_section && $1 == target_key {
      value = $0
      sub(/^[^=]*=[[:space:]]*/, "", value)
      print value
      exit
    }
  ' PATHS.toml
}
