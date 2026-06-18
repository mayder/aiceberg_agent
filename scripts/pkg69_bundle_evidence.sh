#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  scripts/pkg69_bundle_evidence.sh <scenario-name> <filled-template.md> <raw-artifact-file-or-dir> [output-dir]

Creates a reviewable PKG-69 evidence bundle:
- copies the filled template to evidence.md;
- copies a raw file or archives a raw directory under raw/;
- updates "Evidencia bruta anexada" in evidence.md to the bundled artifact;
- writes MANIFEST.tsv with SHA256 and byte sizes.

The output evidence.md can be passed to scripts/pkg69_operational_evidence_gate.sh
through the matching PKG69_*_EVIDENCE environment variable.
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ "$#" -lt 3 || "$#" -gt 4 ]]; then
  usage
  exit 64
fi

SCENARIO="$1"
TEMPLATE="$2"
RAW_ARTIFACT="$3"
OUT_DIR="${4:-}"

if [[ ! "$SCENARIO" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
  echo "scenario-name must contain only lowercase letters, numbers, '_' or '-'" >&2
  exit 65
fi
if [[ ! -f "$TEMPLATE" ]]; then
  echo "template not found: $TEMPLATE" >&2
  exit 66
fi
if [[ ! -e "$RAW_ARTIFACT" ]]; then
  echo "raw artifact not found: $RAW_ARTIFACT" >&2
  exit 67
fi

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="/tmp/aiceberg_pkg69_${SCENARIO}_${timestamp}"
fi
mkdir -p "$OUT_DIR/raw"

file_size_bytes() {
  wc -c <"$1" | tr -d ' '
}

file_sha256() {
  shasum -a 256 "$1" | awk '{print $1}'
}

artifact_size_bytes() {
  local path="$1"
  if [[ -f "$path" ]]; then
    file_size_bytes "$path"
    return
  fi
  find "$path" -type f -exec wc -c {} + | awk '{sum += $1} END {print sum + 0}'
}

if [[ -f "$RAW_ARTIFACT" && ! -s "$RAW_ARTIFACT" ]]; then
  echo "raw artifact is empty: $RAW_ARTIFACT" >&2
  exit 68
fi
if [[ -d "$RAW_ARTIFACT" && "$(artifact_size_bytes "$RAW_ARTIFACT")" == "0" ]]; then
  echo "raw artifact directory has no non-empty files: $RAW_ARTIFACT" >&2
  exit 69
fi

cp "$TEMPLATE" "$OUT_DIR/evidence.md"

if [[ -d "$RAW_ARTIFACT" ]]; then
  raw_name="$(basename "$RAW_ARTIFACT").tgz"
  tar -C "$(dirname "$RAW_ARTIFACT")" -czf "$OUT_DIR/raw/$raw_name" "$(basename "$RAW_ARTIFACT")"
else
  raw_name="$(basename "$RAW_ARTIFACT")"
  cp "$RAW_ARTIFACT" "$OUT_DIR/raw/$raw_name"
fi

if ! grep -Fq -- "- Evidencia bruta anexada:" "$OUT_DIR/evidence.md"; then
  echo "template missing required field: Evidencia bruta anexada" >&2
  exit 70
fi
BUNDLED_RAW="raw/$raw_name" \
  perl -0pi -e 's#^- Evidencia bruta anexada:.*$#- Evidencia bruta anexada: $ENV{BUNDLED_RAW}#m' "$OUT_DIR/evidence.md"

template_sha="$(file_sha256 "$OUT_DIR/evidence.md")"
template_bytes="$(file_size_bytes "$OUT_DIR/evidence.md")"
artifact_sha="$(file_sha256 "$OUT_DIR/raw/$raw_name")"
artifact_bytes="$(file_size_bytes "$OUT_DIR/raw/$raw_name")"

{
  printf 'scenario\ttemplate\tsha256\tbytes\tartifact\tartifact_sha256\tartifact_bytes\tcreated_at_utc\n'
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$SCENARIO" \
    "$OUT_DIR/evidence.md" \
    "$template_sha" \
    "$template_bytes" \
    "$OUT_DIR/raw/$raw_name" \
    "$artifact_sha" \
    "$artifact_bytes" \
    "$timestamp"
} >"$OUT_DIR/MANIFEST.tsv"

printf 'bundle=%s\n' "$OUT_DIR"
printf 'evidence=%s\n' "$OUT_DIR/evidence.md"
printf 'artifact=%s\n' "$OUT_DIR/raw/$raw_name"
printf 'manifest=%s\n' "$OUT_DIR/MANIFEST.tsv"
