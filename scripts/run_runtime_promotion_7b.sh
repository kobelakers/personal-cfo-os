#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-up}"
ENV_FILE="${2:-$ROOT/.env.runtime-promotion.example}"
COMPOSE_FILE="$ROOT/deployments/docker-compose.runtime-promotion.yml"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing env file: $ENV_FILE" >&2
  exit 1
fi

compose() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

build_ui() {
  npm --prefix "$ROOT/web" run build
}

wait_for_surface() {
  local api_url
  api_url="$(grep '^API_PORT=' "$ENV_FILE" | cut -d= -f2)"
  for _ in $(seq 1 60); do
    if curl -fsS "http://127.0.0.1:${api_url}/api/v1/meta/profile" >/dev/null 2>&1 && curl -fsS "http://127.0.0.1:${api_url}/" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "runtime-promotion operator surface did not become ready" >&2
  return 1
}

case "$MODE" in
  up)
    build_ui
    compose up -d postgres minio minio-init api worker-a worker-b
    wait_for_surface
    echo "runtime-promotion operator surface is ready"
    echo "open /api/v1/meta/profile and the UI root to verify the 7B surface"
    ;;
  smoke)
    build_ui
    compose up -d postgres minio minio-init api worker-a worker-b
    wait_for_surface
    echo "runtime-promotion operator surface is ready"
    echo "see docs/product/interview-demo-runbook.md for the panel checklist"
    ;;
  down)
    compose down -v
    ;;
  *)
    echo "usage: $0 [up|smoke|down] [env-file]" >&2
    exit 1
    ;;
esac
