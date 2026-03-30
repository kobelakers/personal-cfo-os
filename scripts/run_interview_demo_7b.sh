#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-all}"
ENV_FILE="${2:-$ROOT/deployments/.env.interview-demo.example}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing env file: $ENV_FILE" >&2
  exit 1
fi

set -a
source "$ENV_FILE"
set +a

build_ui() {
  npm --prefix "$ROOT/web" run build
}

seed_demo() {
  (
    cd "$ROOT"
    go run ./cmd/demo-seed \
      --runtime-db "$ROOT/${INTERVIEW_DEMO_RUNTIME_DB#./}" \
      --memory-db "$ROOT/${INTERVIEW_DEMO_MEMORY_DB#./}" \
      --blob-root "$ROOT/${INTERVIEW_DEMO_BLOB_ROOT#./}" \
      --fixture-dir "$ROOT/${INTERVIEW_DEMO_FIXTURE_DIR#./}" \
      --out "$ROOT/${INTERVIEW_DEMO_MANIFEST#./}" \
      --reset
  )
}

run_api() {
  (
    cd "$ROOT"
    go run ./cmd/api \
      --addr "$INTERVIEW_DEMO_ADDR" \
      --db "$ROOT/${INTERVIEW_DEMO_RUNTIME_DB#./}" \
      --runtime-profile local-lite \
      --runtime-backend sqlite \
      --blob-backend localfs \
      --blob-root "$ROOT/${INTERVIEW_DEMO_BLOB_ROOT#./}" \
      --fixture-dir "$ROOT/${INTERVIEW_DEMO_FIXTURE_DIR#./}" \
      --benchmark-dir "$ROOT/${INTERVIEW_DEMO_BENCHMARK_DIR#./}" \
      --ui-dist "$ROOT/${INTERVIEW_DEMO_UI_DIST#./}"
  )
}

case "$MODE" in
  build-ui)
    build_ui
    ;;
  seed)
    build_ui
    seed_demo
    ;;
  api)
    run_api
    ;;
  all)
    build_ui
    seed_demo
    echo "interview-demo available at http://127.0.0.1${INTERVIEW_DEMO_ADDR}"
    run_api
    ;;
  *)
    echo "usage: $0 [build-ui|seed|api|all] [env-file]" >&2
    exit 1
    ;;
esac
