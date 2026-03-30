#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-all}"
ENV_FILE="${2:-$ROOT/deployments/.env.dev-stack.example}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing env file: $ENV_FILE" >&2
  exit 1
fi

set -a
source "$ENV_FILE"
set +a

API_PID=""
WORKER_PID=""
WEB_PID=""

cleanup() {
  for pid in "$WEB_PID" "$WORKER_PID" "$API_PID"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
      kill "$pid" >/dev/null 2>&1 || true
      wait "$pid" >/dev/null 2>&1 || true
    fi
  done
}
trap cleanup EXIT INT TERM

mkdir -p "$(dirname "$ROOT/${DEV_STACK_RUNTIME_DB#./}")" "$ROOT/${DEV_STACK_BLOB_ROOT#./}"

run_api() {
  (
    cd "$ROOT"
    go run ./cmd/api \
      --addr "$DEV_STACK_API_ADDR" \
      --db "$ROOT/${DEV_STACK_RUNTIME_DB#./}" \
      --runtime-profile local-lite \
      --runtime-backend sqlite \
      --blob-backend localfs \
      --blob-root "$ROOT/${DEV_STACK_BLOB_ROOT#./}" \
      --fixture-dir "$ROOT/${DEV_STACK_FIXTURE_DIR#./}" \
      --benchmark-dir "$ROOT/${DEV_STACK_BENCHMARK_DIR#./}" \
      --ui-dist ""
  )
}

run_worker() {
  (
    cd "$ROOT"
    go run ./cmd/worker \
      --db "$ROOT/${DEV_STACK_RUNTIME_DB#./}" \
      --runtime-profile local-lite \
      --runtime-backend sqlite \
      --blob-backend localfs \
      --blob-root "$ROOT/${DEV_STACK_BLOB_ROOT#./}" \
      --fixture-dir "$ROOT/${DEV_STACK_FIXTURE_DIR#./}" \
      --worker-id dev-stack-worker \
      --role all \
      --interval 5s
  )
}

run_web() {
  VITE_API_BASE="$DEV_STACK_VITE_API_BASE" npm --prefix "$ROOT/web" run dev -- --host 127.0.0.1 --port "$DEV_STACK_UI_PORT"
}

case "$MODE" in
  api)
    run_api
    ;;
  worker)
    run_worker
    ;;
  web)
    run_web
    ;;
  all)
    run_api &
    API_PID="$!"
    run_worker &
    WORKER_PID="$!"
    run_web &
    WEB_PID="$!"
    echo "dev-stack api=http://127.0.0.1${DEV_STACK_API_ADDR} ui=http://127.0.0.1:${DEV_STACK_UI_PORT}"
    echo "panels: task-graphs approvals replay intelligence artifacts benchmarks"
    wait
    ;;
  *)
    echo "usage: $0 [api|worker|web|all] [env-file]" >&2
    exit 1
    ;;
esac
