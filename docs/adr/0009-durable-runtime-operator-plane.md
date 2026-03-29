# ADR 0009: Durable Runtime and Operator Plane

## Status

Accepted

## Context

After Phase 4B, Personal CFO OS could already:

- generate and register follow-up task graphs
- execute first-level capability-backed child workflows
- checkpoint child workflows for approval waits
- carry resume tokens, approval ids, retry metadata, and committed state handoff in runtime records

What it still lacked was a runtime plane that survived process boundaries and could be operated outside the parent workflow lifetime. Follow-up execution was real, but still too tied to in-memory runtime state and test-oriented control surfaces.

## Decision

Phase 5A introduces a narrow but real durable runtime/operator plane:

- runtime state is split behind explicit persistence seams:
  - `TaskGraphStore`
  - `TaskExecutionStore`
  - `ApprovalStateStore`
  - `CheckpointStore`
  - `ReplayStore`
- the first durable implementation uses local SQLite plus file-backed artifact refs
- operator actions become typed runtime commands with:
  - request id based idempotency
  - optimistic-concurrency / compare-and-swap transitions
  - explicit approve / deny / resume / retry / reevaluate boundaries
- `approve` resolves policy and may attempt one auto-resume
- `resume` remains a separate runtime continuation action
- `deny` maps to:
  - `Status = failed`
  - `FailureCategory = denied_by_operator`
  - no committed-state advance
- `ReplayStore` becomes the only durable source of truth for operator-facing replay queries
- `cmd/api` and `cmd/worker` become the first runnable operator surfaces

## Why This Is Deliberate

We intentionally do **not** use Phase 5A to broaden business scope. The goal is to turn the existing proactive loop into a durable, operator-runnable loop before adding:

- more domains
- stronger finance-engine hardening
- semantic retrieval hardening
- real distributed runtime infrastructure

That keeps the architecture honest: runtime durability, operator control, replay, and resume semantics become real before the project claims production-shaped long-running agent execution.

## Consequences

### Positive

- follow-up execution is no longer only a code-path demo; it survives restart and can be advanced by an operator or worker loop
- approval-aware child workflows can resume from persisted checkpoint payloads instead of being restarted from the top
- replay and provenance now become durable query planes rather than process-local helper dumps
- workflow files remain thin because runtime persistence and operator control sit below them

### Trade-offs

- durability is local-first (`SQLite + file refs`), not yet Postgres + object storage + Temporal
- operator API is minimal and intentionally not a full UI
- memory hardening, retrieval hardening, and deeper finance/business validation remain deferred to the next phase
