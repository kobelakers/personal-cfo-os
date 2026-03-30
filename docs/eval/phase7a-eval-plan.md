# Phase 7A Eval Plan

Phase 7A is not another domain corpus phase. Its proof surface is a deterministic async-runtime test suite plus a runtime-promotion deployment profile. The current emphasis is closeout hardening: proving that the promoted Postgres backend is concurrency-correct, not just architecturally present.

## Scope

7A only hardens the runtime backbone:

- durable work queue
- lease / heartbeat / reclaim
- fencing-token commit protection
- scheduler / reevaluator
- Postgres runtime core stores
- ref-backed checkpoint/report/replay payload semantics
- async replay / observability

It does not widen scope into new business workflows, UI, remote agentization, or broker-based architecture.

## Canonical Proofs

The canonical 7A proofs are:

1. deferred follow-up task -> due window reached -> scheduler wake-up -> worker executes -> completed
2. waiting approval -> operator approve -> different worker resumes -> completed
3. transient failure -> retry backoff -> later worker retry -> completed
4. worker crash / stale worker -> lease timeout -> reclaim -> stale fenced commit rejected
5. concurrent enqueue against Postgres suppresses duplicate active work atomically
6. reclaim against Postgres has a single effective winner
7. long-running claims renew leases through periodic heartbeat
8. `SkillExecutionStore` preserves runtime-authoritative parity under the Postgres profile
9. runtime-promotion profile with Postgres + MinIO passes the same runtime core contract

## Determinism Rules

All async tests must use:

- injected `Clock`
- explicit `WorkerID`
- deterministic lease timing
- manual tick rather than wall-clock sleeps

This keeps replay/debug evidence stable and prevents async proof drift.

## Runtime Promotion Profile

The local proof profile is:

- Postgres
- MinIO
- API
- 2 workers

Run script:

```bash
scripts/run_runtime_promotion_7a.sh proof
```

This produces checked-in runtime-promotion evidence samples under `docs/eval/samples/phase7a_*`.
