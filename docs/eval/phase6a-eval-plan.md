# Phase 6A Evaluation Plan

Phase 6A upgrades the current governed finance agent backbone with a **queryable replay plane, deterministic regression harness, and operator-grade debug surface**. It does not widen scope into new workflows, UI expansion, infra promotion, or remote actorization.

## Definition of Done

- `cmd/replay` is no longer a placeholder and can query workflow / task graph / task / execution / approval replay views.
- replay answers why / how / what changed from:
  - runtime durable truth
  - replay/debug projection rows
  - artifact refs
- replay/debug projections are versioned and rebuildable/backfillable from authoritative runtime truth.
- `ReplayQueryService` returns:
  - hard failure when authoritative runtime truth is missing
  - partial replay view plus degradation reasons when projections are missing, stale, or incomplete
- `cmd/eval` runs a canonical deterministic/mock-only corpus rather than only phase runners.
- the P0 corpus covers these 10 canonical cases:
  1. Monthly Review happy path
  2. Monthly Review cross-session memory influence
  3. Monthly Review trust/validator failure
  4. Debt vs Invest waiting_approval
  5. Debt vs Invest deny/fail
  6. Life Event -> generated follow-up tasks
  7. Tax child workflow happy path
  8. Portfolio child workflow happy path
  9. retry / reevaluate
  10. parent workflow -> child workflow provenance reconstruction
- golden replay/debug outputs are checked in for representative 6A cases.
- 5B / 5C / 5D regression coverage still passes.

## Truth Source and Degrade Semantics

Replay truth source is deliberately local and durable:

- runtime durable truth
- normalized replay/debug projection rows
- artifact refs for rich bundles and golden/debug payloads

Authoritative truth is never projection-owned. Query behavior is fixed:

- authoritative runtime truth missing -> hard failure
- projection missing / stale / incomplete -> partial replay view + degradation reasons
- rebuild/backfill is available, but query must still return best-effort structured replay instead of an opaque error

## Canonical Corpus Policy

The default 6A regression corpus is **deterministic/mock only**:

- deterministic fixtures
- mock intelligence paths
- local runtime/memory/verification/governance behavior

Live provider paths are explicitly excluded from canonical regression so provider drift does not destabilize:

- golden traces
- eval scores
- replay diffs

Provider-sensitive fields such as token/cost may still be recorded when available, but they are not a prerequisite for corpus stability.

## Replay / Provenance Checks

- workflow replay query
- task graph replay query
- task replay query
- execution replay query
- approval replay query
- parent -> generated task -> child workflow -> artifact -> state commit -> operator action reconstruction
- why failed attribution
- why waiting_approval attribution
- why generated task attribution
- why child workflow executed attribution
- why memory was selected / rejected attribution
- replay comparison diff for changed plan/memory/validator/governance outcomes

## Eval Checks

- full corpus run
- single scenario run
- eval diff
- regression failure reporting
- backward-compatible phase runner adapter
- deterministic/mock-only corpus stability

## Run Evidence

Canonical 6A samples:

- `docs/eval/samples/phase6a_eval_default_corpus.json`
- `docs/eval/samples/phase6a_replay_compare_monthly_review_memory.json`
- `docs/eval/samples/phase6a_replay_debt_vs_invest_waiting_approval.json`
- `docs/eval/samples/phase6a_replay_life_event_task_graph.json`

Typical commands:

```bash
go run ./cmd/eval --mode corpus --corpus phase6a-default --format summary
go run ./cmd/replay --runtime-db ./var/runtime.db --workflow-id <workflow-id> --format summary
go run ./cmd/replay --runtime-db ./var/runtime.db --rebuild-projections --all
```

## Explicitly Deferred Beyond Phase 6A

- blocked/deferred/capability variants beyond the P0 canonical corpus
- richer provider/prompt A/B evaluation
- full UI/debug panel work
- external observability infra promotion
- remote runtime / Postgres / Temporal / object storage promotion
