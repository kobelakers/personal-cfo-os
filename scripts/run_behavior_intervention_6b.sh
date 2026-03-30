#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-mock}"
OUT_DIR="${2:-$ROOT/docs/eval/samples}"
WORK_DIR="${3:-$ROOT/var/phase6b-evidence}"
NOW="${FIXED_NOW:-2026-03-30T08:00:00Z}"

if [[ "$MODE" != "mock" ]]; then
  echo "Phase 6B canonical evidence is deterministic/mock-only; live mode is intentionally unsupported for this script" >&2
  exit 1
fi

mkdir -p "$OUT_DIR" "$WORK_DIR"

cd "$ROOT"

go run ./cmd/eval \
  --mode corpus \
  --corpus phase6b-default \
  --fixed-now "$NOW" \
  --workdir "$WORK_DIR" \
  --format json \
  --output "$OUT_DIR/phase6b_eval_default_corpus.json" >/dev/null

# Normalize volatile artifact timestamps so checked-in evidence stays stable.
tmp="$(mktemp)"
jq 'del(.results[].replay.workflow.artifacts[]?.created_at)' \
  "$OUT_DIR/phase6b_eval_default_corpus.json" > "$tmp"
mv "$tmp" "$OUT_DIR/phase6b_eval_default_corpus.json"

HAPPY_ID="$(jq -r '.results[] | select(.scenario_id=="behavior_intervention_happy_path") | .workflow_id' "$OUT_DIR/phase6b_eval_default_corpus.json")"
APPROVAL_ID="$(jq -r '.results[] | select(.scenario_id=="behavior_intervention_waiting_approval") | .workflow_id' "$OUT_DIR/phase6b_eval_default_corpus.json")"
LEFT_ID="$(jq -r '.results[] | select(.scenario_id=="behavior_skill_selection_influenced_by_procedural_memory") | .comparison.left.id' "$OUT_DIR/phase6b_eval_default_corpus.json")"
RIGHT_ID="$(jq -r '.results[] | select(.scenario_id=="behavior_skill_selection_influenced_by_procedural_memory") | .comparison.right.id' "$OUT_DIR/phase6b_eval_default_corpus.json")"

go run ./cmd/replay \
  --runtime-db "$WORK_DIR/behavior_intervention_happy_path/runtime.db" \
  --workflow-id "$HAPPY_ID" \
  --format json > "$OUT_DIR/phase6b_replay_behavior_intervention.json"

go run ./cmd/replay \
  --runtime-db "$WORK_DIR/behavior_intervention_waiting_approval/runtime.db" \
  --workflow-id "$APPROVAL_ID" \
  --format json > "$OUT_DIR/phase6b_replay_behavior_intervention_waiting_approval.json"

go run ./cmd/replay \
  --runtime-db "$WORK_DIR/behavior_skill_selection_influenced_by_procedural_memory/runtime.db" \
  --compare-left "workflow:$LEFT_ID" \
  --compare-right "workflow:$RIGHT_ID" \
  --format json > "$OUT_DIR/phase6b_compare_procedural_memory_skill_selection.json"

tmp="$(mktemp)"
jq 'del(.workflow.artifacts[]?.created_at)' \
  "$OUT_DIR/phase6b_replay_behavior_intervention.json" > "$tmp"
mv "$tmp" "$OUT_DIR/phase6b_replay_behavior_intervention.json"

tmp="$(mktemp)"
jq 'del(.workflow.artifacts[]?.created_at)' \
  "$OUT_DIR/phase6b_replay_behavior_intervention_waiting_approval.json" > "$tmp"
mv "$tmp" "$OUT_DIR/phase6b_replay_behavior_intervention_waiting_approval.json"

echo "corpus: $OUT_DIR/phase6b_eval_default_corpus.json"
echo "replay: $OUT_DIR/phase6b_replay_behavior_intervention.json"
echo "approval replay: $OUT_DIR/phase6b_replay_behavior_intervention_waiting_approval.json"
echo "compare: $OUT_DIR/phase6b_compare_procedural_memory_skill_selection.json"
echo "workdir: $WORK_DIR"
echo "note: this run proves versioned skill runtime, formal behavior domain, and procedural-memory-influenced skill selection on a deterministic/mock-only path"
