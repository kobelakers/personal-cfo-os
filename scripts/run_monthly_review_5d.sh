#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-mock}"
OUT_DIR="${2:-$ROOT/docs/eval/samples}"
MEMORY_DB="${3:-${MEMORY_DB_PATH:-$ROOT/var/memory-5d.db}}"

TRACE_OUT="$OUT_DIR/monthly_review_5d_trace.json"
ARTIFACT_OUT="$OUT_DIR/monthly_review_5d_report.json"
NOW="${FIXED_NOW:-2026-03-30T08:00:00Z}"

mkdir -p "$OUT_DIR" "$(dirname "$MEMORY_DB")"

if [[ "$MODE" == "live" ]]; then
  if [[ -z "${OPENAI_API_KEY:-}" ]]; then
    echo "OPENAI_API_KEY is required for live mode" >&2
    exit 1
  fi
  if [[ -z "${OPENAI_REASONING_MODEL:-}" ]]; then
    echo "OPENAI_REASONING_MODEL is required for live mode" >&2
    exit 1
  fi
  if [[ -z "${OPENAI_FAST_MODEL:-}" ]]; then
    echo "OPENAI_FAST_MODEL is required for live mode" >&2
    exit 1
  fi
  if [[ -z "${OPENAI_EMBEDDING_MODEL:-}" ]]; then
    echo "OPENAI_EMBEDDING_MODEL is required for live mode" >&2
    exit 1
  fi
fi

cd "$ROOT"

go run ./cmd/eval \
  --phase 5d \
  --workflow monthly_review \
  --provider-mode "$MODE" \
  --memory-db "$MEMORY_DB" \
  --fixed-now "$NOW" \
  --artifact-out "$ARTIFACT_OUT" \
  --trace-out "$TRACE_OUT"

echo "memory_db: $MEMORY_DB"
echo "artifact: $ARTIFACT_OUT"
echo "trace: $TRACE_OUT"
echo "note: this run proves finance-engine-backed recommendations, trust validators, and governance/approval fields on the Monthly Review path"
