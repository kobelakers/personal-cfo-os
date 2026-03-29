#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-mock}"
MEMORY_DB="${2:-${MEMORY_DB_PATH:-$ROOT/var/memory.db}}"

mkdir -p "$(dirname "$MEMORY_DB")"

if [[ "$MODE" == "live" ]]; then
  if [[ -z "${OPENAI_API_KEY:-}" ]]; then
    echo "OPENAI_API_KEY is required for live mode" >&2
    exit 1
  fi
  if [[ -z "${OPENAI_EMBEDDING_MODEL:-}" ]]; then
    echo "OPENAI_EMBEDDING_MODEL is required for live mode" >&2
    exit 1
  fi
fi

cd "$ROOT"
go run ./cmd/eval \
  --phase 5c \
  --provider-mode "$MODE" \
  --memory-db "$MEMORY_DB" \
  --reindex-memory \
  --index-only
