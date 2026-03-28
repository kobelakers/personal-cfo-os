# ADR 0005: System-Agent Execution Boundaries

## Status

Accepted

## Context

After Phase 2 structural remediation, the repository had clear subsystem boundaries but still executed core system steps through direct workflow-to-service calls. That made the protocol layer largely schema-first and left the system-agent narrative under-realized in code.

## Decision

We introduced a concrete system-agent execution plane for system steps only:

- `PlannerAgent`
- `MemorySteward`
- `ReportAgent`
- `VerificationAgent`
- `GovernanceAgent`

The workflow-facing surface is a narrow `SystemStepBus`, not the raw registry/dispatcher/executor internals.

The protocol layer now carries execution-first typed contracts:

- request/result message kinds
- oneof-style typed request/result bodies
- typed response envelopes
- correlation and causation propagation
- typed failure categories for runtime mapping

`ReportAgent` is explicitly split into draft and finalize phases so that final artifacts and `report_ready` are only emitted after governance has evaluated approval and disclosure.

## Consequences

### Positive

- workflows are thinner and no longer directly assemble planner/memory/report/verification/governance services
- protocol participates in real execution, observability, and replay rather than only schema definition
- runtime can map system-agent failures through typed categories instead of brittle string matching
- the project narrative becomes more defensible as a partial system-agent execution architecture

### Negative

- the current agent bus is still local synchronous dispatch, not remote or durable inbox/outbox execution
- domain agents are still outside the main execution backbone
- protocol DTOs and domain report types still require explicit mapping and do not yet represent a full cross-process compatibility layer

## Why This Is Deliberate

We intentionally protocolized system steps before domain-agent expansion. This keeps the execution backbone honest and observable without inflating the project into a fake “many agents talking” demo before runtime, governance, and verification boundaries are mature.
