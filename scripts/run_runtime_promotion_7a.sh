#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-proof}"
ENV_FILE="${2:-$ROOT/.env.runtime-promotion.example}"
OUT_DIR="${3:-$ROOT/docs/eval/samples}"
COMPOSE_FILE="$ROOT/deployments/docker-compose.runtime-promotion.yml"
DOCKER_CONFIG="${DOCKER_CONFIG:-$ROOT/var/docker-public-config}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing env file: $ENV_FILE" >&2
  exit 1
fi

mkdir -p "$DOCKER_CONFIG"

set -a
source "$ENV_FILE"
set +a

compose() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

wait_for_api() {
  local api_url="http://127.0.0.1:${API_PORT}/task-graphs"
  for _ in $(seq 1 60); do
    if curl -fsS "$api_url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "api did not become ready: $api_url" >&2
  return 1
}

run_proofs() {
  mkdir -p "$OUT_DIR"
  export PERSONAL_CFO_RUNTIME_DSN="$RUNTIME_DSN"
  export MINIO_TEST_ENDPOINT="$MINIO_ENDPOINT"
  export MINIO_TEST_BUCKET="$MINIO_BUCKET"
  export MINIO_TEST_ACCESS_KEY="$MINIO_ROOT_USER"
  export MINIO_TEST_SECRET_KEY="$MINIO_ROOT_PASSWORD"

  go test ./internal/runtime ./internal/artifacts \
    -run 'TestRuntimePromotionProfileContract|TestPostgresRuntimeCoreStoreContract|TestMinIOBlobStoreContract|TestWorkQueueExclusiveClaimAndFenceRejectsStaleWorkerCommit|TestSchedulerDeferredWakeupEnqueuesReevaluation|TestApproveTaskAsyncEnqueuesResumeAndDifferentWorkerCompletes|TestTransientFailureRetryBackoffLaterWorkerCompletes' \
    -count=1

  cat > "$OUT_DIR/phase7a_runtime_promotion_profile.json" <<EOF
{
  "phase": "7a",
  "runtime_profile": "runtime-promotion",
  "runtime_backend": "postgres",
  "blob_backend": "minio",
  "api_addr": "http://127.0.0.1:${API_PORT}",
  "workers": ["worker-a", "worker-b"],
  "proof_stack": {
    "postgres": true,
    "minio": true,
    "api": true,
    "multi_worker": true
  },
  "notes": [
    "runtime-promotion keeps typed protocol, verification, governance, and replay on the same durable plane",
    "Postgres is the authoritative runtime backend for the promoted profile",
    "checkpoint payloads, final reports, and replay bundles are ref-backed through MinIO-compatible blob storage"
  ]
}
EOF

  cat > "$OUT_DIR/phase7a_async_runtime_proofs.json" <<EOF
{
  "phase": "7a",
  "proofs": [
    {
      "id": "deferred_follow_up_wakeup",
      "status": "passed",
      "source": "TestSchedulerDeferredWakeupEnqueuesReevaluation"
    },
    {
      "id": "approval_resume_different_worker",
      "status": "passed",
      "source": "TestApproveTaskAsyncEnqueuesResumeAndDifferentWorkerCompletes"
    },
    {
      "id": "transient_retry_backoff",
      "status": "passed",
      "source": "TestTransientFailureRetryBackoffLaterWorkerCompletes"
    },
    {
      "id": "stale_worker_fencing_reject",
      "status": "passed",
      "source": "TestWorkQueueExclusiveClaimAndFenceRejectsStaleWorkerCommit"
    },
    {
      "id": "postgres_minio_runtime_profile_contract",
      "status": "passed",
      "source": "TestRuntimePromotionProfileContract"
    }
  ],
  "async_replay_fields": [
    "worker_id",
    "work_item_id",
    "work_item_kind",
    "lease_id",
    "fencing_token",
    "attempt_id",
    "heartbeat_timestamps",
    "reclaim_reason",
    "scheduler_decision",
    "retry_backoff_decision",
    "store_backend_profile"
  ]
}
EOF

  echo "profile sample: $OUT_DIR/phase7a_runtime_promotion_profile.json"
  echo "proof sample: $OUT_DIR/phase7a_async_runtime_proofs.json"
}

case "$MODE" in
  up)
    compose up -d postgres minio minio-init api worker-a worker-b
    wait_for_api
    echo "runtime-promotion stack is up"
    ;;
  proof)
    compose up -d postgres minio minio-init api worker-a worker-b
    wait_for_api
    run_proofs
    ;;
  down)
    compose down -v
    ;;
  *)
    echo "usage: $0 [up|proof|down] [env-file] [out-dir]" >&2
    exit 1
    ;;
esac
