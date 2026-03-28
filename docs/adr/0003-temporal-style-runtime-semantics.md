# ADR 0003: Temporal-Style Runtime Semantics

## Status

Accepted

## Decision

Model runtime behavior around durable workflow semantics: checkpointing, retries, pause/resume, approval waits, and recovery strategies.

## Rationale

- Personal finance workflows are long-running and must survive restarts, human approval latency, and evidence gaps.
- Temporal-style semantics map naturally to agent execution checkpoints and recovery planning.
- Explicit failure categories prevent the system from collapsing into a generic "something went wrong" loop.

## Consequences

- Workflow execution state is part of the domain model, not an implementation detail.
- Future Temporal workflows will bind to the current state model instead of redefining behavior later.
