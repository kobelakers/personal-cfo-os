#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-mock}"
OUT_DIR="${2:-$ROOT/docs/eval/samples}"
MEMORY_DB="${3:-${MEMORY_DB_PATH:-$ROOT/var/memory.db}}"

FIRST_NOW="${FIRST_NOW:-2026-03-30T08:00:00Z}"
SECOND_NOW="${SECOND_NOW:-2026-03-31T08:00:00Z}"

TRACE_OUT="$OUT_DIR/monthly_review_5c_trace.json"
ARTIFACT_OUT="$OUT_DIR/monthly_review_5c_report.json"

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

rm -f "$MEMORY_DB"

cd "$ROOT"

# Seed run: writes durable memory into the injected memory plane.
go run ./cmd/eval \
  --phase 5c \
  --provider-mode "$MODE" \
  --memory-db "$MEMORY_DB" \
  --fixed-now "$FIRST_NOW" >/dev/null

# Influenced run: reopens the same memory.db so durable memory can affect planning/reasoning.
go run ./cmd/eval \
  --phase 5c \
  --provider-mode "$MODE" \
  --memory-db "$MEMORY_DB" \
  --fixed-now "$SECOND_NOW" \
  --artifact-out "$ARTIFACT_OUT" \
  --trace-out "$TRACE_OUT"

echo "memory_db: $MEMORY_DB"
echo "artifact: $ARTIFACT_OUT"
echo "trace: $TRACE_OUT"
echo "note: this script runs twice against the same memory db to prove cross-session memory influence"
