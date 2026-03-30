# Async Execution

Phase 7A promotes the runtime from a local worker pass into an async-capable backbone while keeping the existing typed contracts intact.

## Core Model

The promoted runtime now revolves around durable typed work items instead of an in-process "look around and do whatever is ready" loop.

- `WorkItem` is the durable queue object.
- `WorkItemKind` stays typed and narrow:
  - `reevaluate_task_graph`
  - `execute_ready_task`
  - `resume_approved_checkpoint`
  - `retry_failed_execution`
  - `scheduler_wakeup`
- workers claim work through a lease, keep the lease alive with heartbeats, and either complete, fail, or lose the lease.

## Lease And Fencing

7A intentionally does not claim exactly-once execution. The runtime instead uses:

- at-least-once delivery
- exclusive lease ownership
- durable attempts
- idempotent operator commands
- compare-and-swap state transitions
- fencing token checks on completion/checkpoint/transition commits

This means a stale worker that was reclaimed can still continue running user code, but it cannot successfully commit the final state because the lease epoch no longer matches.

## Scheduler / Reevaluator

The scheduler is now a first-class runtime subsystem. It does not execute business logic directly. It only decides when to enqueue the next typed work item.

It currently covers:

- `deferred` due-window wakeups
- dependency unblock reevaluation
- approval resume wakeups
- capability reevaluation
- transient retry backoff
- operator-triggered reevaluation

## Replay Alignment

`internal/runtime.ReplayQueryService` remains the canonical replay plane. Async runtime promotion extends the same replay truth source rather than introducing a second queue debugger.

Replay/debug can now explain:

- which worker claimed a work item
- when the lease was created, heartbeated, expired, and reclaimed
- why a deferred task became ready
- why a retry was scheduled
- why approval resume continued on another worker
- why a claim or transition failed due to fencing/CAS

## Boundary

Phase 7A promotes the runtime backbone, not the agent topology.

- system agents remain local synchronous handlers
- workflows remain thin orchestrators
- no remote actor mailbox / inbox-outbox system is introduced here
- no external broker becomes the new truth source
