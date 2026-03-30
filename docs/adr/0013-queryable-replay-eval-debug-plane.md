# ADR 0013: Queryable Replay / Eval / Debug Plane

## Status

Accepted

## Context

By the end of Phase 5D, Personal CFO OS already had:

- a governed runtime backbone with durable SQLite persistence
- a real Monthly Review intelligence path
- a real memory substrate
- trustworthy finance reasoning with deterministic validators and governance gates

What it still lacked was a **first operator-grade replay/eval/debug plane**. The repository could emit traces and phase-specific run artifacts, but it could not yet answer structured why/how questions, compare runs as a regression harness, or expose provenance as a directed graph on top of the same durable runtime truth.

## Decision

Phase 6A adds replay/eval/debug as a load-bearing layer with these boundaries:

1. replay truth source is:
   - runtime durable truth
   - normalized replay/debug projection rows
   - artifact refs
2. artifacts are not the only durable source; queryable replay must not depend on deserializing one large JSON blob
3. replay/debug projections are versioned and rebuildable/backfillable from authoritative runtime truth
4. query semantics are explicit:
   - authoritative runtime truth missing -> hard failure
   - projection missing/stale/incomplete -> partial replay view with degradation reasons
5. provenance is upgraded from an ID bag to a directed graph
6. `cmd/replay` becomes the first formal operator/developer replay CLI
7. `cmd/eval` becomes a deterministic regression harness; phase runners remain as adapters rather than the main story
8. canonical 6A regression corpus is deterministic/mock only; live provider paths remain smoke/manual evidence

## Consequences

### Positive

- replay can now answer why failed, why waiting_approval, why child workflow executed, and what changed between runs
- provenance is now machine-queryable rather than a human-only trace-reading exercise
- eval becomes a regression safety net instead of a collection of phase-specific scripts
- replay/debug stays aligned with the same runtime durable plane rather than inventing a second explanation store

### Negative / Deferred

- this is still a local-first plane, not a full external observability stack
- provider/prompt A/B comparison is still intentionally narrow
- blocked/deferred/capability corpus expansion remains a later follow-up
- this ADR does not introduce remote runtime promotion, UI-heavy debugging, or infra upgrades such as Postgres/Temporal/MinIO
