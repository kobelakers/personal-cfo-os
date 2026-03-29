#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-mock}"
OUT_DIR="${2:-$ROOT/docs/eval/samples}"
TRACE_OUT="$OUT_DIR/monthly_review_5b_trace.json"
ARTIFACT_OUT="$OUT_DIR/monthly_review_5b_report.json"

mkdir -p "$OUT_DIR"

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
fi

cd "$ROOT"
go run ./cmd/eval \
  --provider-mode "$MODE" \
  --artifact-out "$ARTIFACT_OUT" \
  --trace-out "$TRACE_OUT"

echo "artifact: $ARTIFACT_OUT"
echo "trace: $TRACE_OUT"
