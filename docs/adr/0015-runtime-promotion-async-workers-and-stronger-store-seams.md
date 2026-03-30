# ADR 0015: Runtime Promotion With Async Workers And Stronger Store Seams

## Status

Accepted, with Phase 7A closeout hardening applied

## Context

By the end of Phase 6B, Personal CFO OS already had:

- a system-agent backbone
- load-bearing domain-agent execution
- a durable local runtime plane
- canonical replay/eval/debug on the same durable plane
- a versioned skill runtime and formal behavior domain

The remaining runtime gap was not protocol design or new business logic. It was the gap between:

- a local durable worker pass
- and an async-capable, multi-worker-safe runtime backbone

The system therefore needed stronger runtime semantics without breaking:

- `TaskSpec` as the goal contract
- `plan.Blocks` as the execution truth source
- typed protocol envelopes
- verification and governance boundaries
- canonical replay truth alignment

## Decision

Phase 7A promotes the runtime by introducing:

1. a durable typed work queue
2. lease / heartbeat / reclaim semantics
3. fencing-token-protected commits
4. scheduler / reevaluator services
5. Postgres as the promoted authoritative runtime backend
6. a narrow object-storage seam for checkpoint/report/replay payload refs
7. async replay fields on the existing canonical replay plane

Phase 7A closeout then hardens the promoted backend by requiring:

1. atomic fenced CAS for Postgres `heartbeat / complete / fail / requeue`
2. concurrent-writer-safe active dedupe for enqueue
3. single-winner reclaim
4. real periodic heartbeat renewal during long-running claims
5. Postgres parity for `SkillExecutionStore`
6. promoted-backend proof tests instead of only in-memory/runtime-contract evidence

## Why This Design

### DB-backed queue first

We intentionally promote the runtime with a DB-backed queue/lease model instead of introducing Kafka, NATS, or Redis as a new truth source.

That keeps:

- authoritative runtime truth
- operator actions
- approvals
- checkpoints
- replay events
- async claim/lease/reclaim state

on the same durable plane.

### Fencing instead of exactly-once claims

Phase 7A does not attempt exactly-once execution. It instead formalizes:

- at-least-once delivery
- idempotent commands
- CAS transitions
- durable attempts
- lease exclusivity
- fencing token validation on final commits

Closeout hardening makes that lease model backend-correct instead of only architecturally present: Postgres queue mutations are now committed through one fenced CAS SQL statement, reclaim has a single winner, and duplicate active work is suppressed by an atomic dedupe constraint rather than a preflight read.

This is strong enough for a promoted runtime backbone while remaining compatible with the current local-first architecture.

### Postgres promotion without deleting SQLite

SQLite remains the `local-lite` profile for development and deterministic tests.
Postgres becomes the promoted authoritative backend for multi-worker runtime execution.

### Blob seam without full storage rewrite

This phase does not migrate every payload into object storage. It only formalizes ref-backed storage for:

- checkpoint payloads
- final report payloads
- replay bundles

That is enough to prevent the promoted runtime from staying implicitly local-file-only.

## Consequences

After Phase 7A, the repository should be described as:

**system-agent backbone + first real domain-agent execution path + first proactive life-event loop + first capability-backed follow-up execution + promoted async-capable durable runtime backbone**

But it is still not:

- a fully remote distributed actor system
- a Temporal-backed production runtime
- a productized UI/externalization phase

## Deferred

This ADR does not introduce:

- remote agent mailboxes / inbox-outbox actors
- live Temporal cluster execution
- broker-first queue architecture
- full artifact lifecycle management
- full observability infrastructure promotion
- a first-class migration runner; runtime schema is still managed through `EnsureSchema()` and should be promoted later without changing runtime semantics
