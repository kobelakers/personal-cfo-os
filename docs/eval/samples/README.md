# Sample Evidence

## Phase 5B

Stable mock-generated artifacts for the real-intelligence-backed Monthly Review golden path:

- `monthly_review_5b_report.json`
- `monthly_review_5b_trace.json`

Regenerate:

```bash
./scripts/run_monthly_review_5b.sh mock
```

## Phase 5C

Stable mock-generated artifacts for the first real memory substrate on Monthly Review:

- `monthly_review_5c_report.json`
- `monthly_review_5c_trace.json`
- `monthly_review_5c_cross_session.json`

Regenerate:

```bash
./scripts/run_monthly_review_5c.sh mock
```

Live mode remains local-only and env-gated:

```bash
OPENAI_API_KEY=... \
OPENAI_REASONING_MODEL=... \
OPENAI_FAST_MODEL=... \
OPENAI_EMBEDDING_MODEL=... \
./scripts/run_monthly_review_5c.sh live /tmp/monthly-review-5c /tmp/monthly-review-5c/memory.db
```

## Phase 5D

Stable mock-generated artifacts for trustworthy finance reasoning:

- `monthly_review_5d_report.json`
- `monthly_review_5d_trace.json`
- `debt_vs_invest_5d_waiting_approval.json`
- `debt_vs_invest_5d_waiting_approval_trace.json`

Regenerate Monthly Review:

```bash
./scripts/run_monthly_review_5d.sh mock
```

Regenerate the deterministic Debt vs Invest approval sample:

```bash
go run ./cmd/eval --phase 5d --workflow debt_vs_invest --provider-mode mock --memory-db ./var/memory-5d.db --artifact-out ./docs/eval/samples/debt_vs_invest_5d_waiting_approval.json
```

## Phase 6A

Stable deterministic/mock-only replay/eval/debug evidence:

- `phase6a_eval_default_corpus.json`
- `phase6a_replay_compare_monthly_review_memory.json`
- `phase6a_replay_monthly_review_memory_rejection.json`
- `phase6a_replay_debt_vs_invest_waiting_approval.json`
- `phase6a_replay_life_event_task_graph.json`

Regenerate the canonical corpus:

```bash
go run ./cmd/eval --mode corpus --corpus phase6a-default --format json --output ./docs/eval/samples/phase6a_eval_default_corpus.json
```

Regenerate replay samples:

```bash
go run ./cmd/replay --runtime-db /tmp/personal-cfo-6a-evidence/corpus/monthly_review_memory_rejection_visibility/runtime.db --compare-left workflow:workflow-monthly-review-20260329080000 --compare-right workflow:workflow-monthly-review-20260330080000 --format json > ./docs/eval/samples/phase6a_replay_compare_monthly_review_memory.json
go run ./cmd/replay --runtime-db /tmp/personal-cfo-6a-evidence/corpus/monthly_review_memory_rejection_visibility/runtime.db --workflow-id workflow-monthly-review-20260330080000 --format json > ./docs/eval/samples/phase6a_replay_monthly_review_memory_rejection.json
go run ./cmd/replay --runtime-db /tmp/personal-cfo-6a-evidence/corpus/debt_vs_invest_waiting_approval/runtime.db --workflow-id workflow-debt-vs-invest-20260329080000 --format json > ./docs/eval/samples/phase6a_replay_debt_vs_invest_waiting_approval.json
go run ./cmd/replay --runtime-db /tmp/personal-cfo-6a-evidence/corpus/life_event_generated_follow_up_tasks/life_event_follow_up_runtime.db --task-graph-id graph-life-event-eval-20260329080000 --format json > ./docs/eval/samples/phase6a_replay_life_event_task_graph.json
```

Projection rebuild/backfill remains local-first and query-safe:

```bash
go run ./cmd/replay --runtime-db ./var/runtime.db --rebuild-projections --all
```

The canonical 6A corpus intentionally excludes live provider paths so golden traces and eval diffs stay stable. It now covers 11 deterministic scenarios, including an explicit memory-rejection visibility case.

## Phase 6B

Stable deterministic/mock-only skills + behavior evidence:

- `phase6b_eval_default_corpus.json`
- `phase6b_replay_behavior_intervention.json`
- `phase6b_replay_behavior_intervention_waiting_approval.json`
- `phase6b_compare_procedural_memory_skill_selection.json`

Regenerate the canonical 6B corpus and replay samples:

```bash
./scripts/run_behavior_intervention_6b.sh mock
```

Equivalent manual corpus command:

```bash
go run ./cmd/eval --mode corpus --corpus phase6b-default --fixed-now 2026-03-30T08:00:00Z --workdir ./var/phase6b-evidence --format json --output ./docs/eval/samples/phase6b_eval_default_corpus.json
```

The 6B corpus intentionally remains deterministic/mock-only so:

- skill family / version / recipe selection stays stable
- procedural-memory-driven compare output stays regression-friendly
- checked-in replay samples do not depend on provider drift

## Phase 7A

Stable runtime-promotion evidence for the closeout-hardened async runtime backbone:

- `phase7a_runtime_promotion_profile.json`
- `phase7a_async_runtime_proofs.json`

Bring up the promoted runtime profile and regenerate the checked-in 7A evidence:

```bash
./scripts/run_runtime_promotion_7a.sh proof
```

The 7A proof surface is intentionally different from 6A/6B:

- it is driven by deterministic async/runtime tests plus a local runtime-promotion deployment profile
- it uses Postgres as the promoted runtime backend
- it proves queue correctness on the promoted backend rather than only through in-memory semantics
- it uses MinIO-compatible blob refs for checkpoint/report/replay payload storage
- it keeps canonical replay/debug on the same durable runtime truth plane instead of introducing a second async debugger
