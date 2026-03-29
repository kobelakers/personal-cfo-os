# ADR 0008: Capability-Backed Follow-Up Execution

## Status

Accepted

## Context

After Phase 4A, Personal CFO OS could already:

- ingest structured life events
- update state and memory
- generate typed follow-up tasks
- register those tasks into runtime

What it still could not do was execute those generated follow-up tasks through real workflow capability. `tax_optimization` and `portfolio_rebalance` existed as registered runtime objects, but they remained mostly `queued_pending_capability`.

## Decision

Phase 4B introduces the first capability-backed follow-up execution loop:

- runtime reevaluates generated task graphs after registration
- runtime activates capability-backed tasks when workflow capability is available
- runtime dispatches real child workflows:
  - `TaxOptimizationWorkflow`
  - `PortfolioRebalanceWorkflow`
- child workflows still consume standard `TaskSpec`
- child workflows still run through the existing system-agent backbone
- runtime records execution, retry, approval, and committed-state handoff in `TaskExecutionRecord`

## Guardrails

### Parent workflow stays thin

Workflow C may only call runtime-facing methods:

- register
- reevaluate
- execute ready follow-ups

It may not instantiate child workflows directly.

### Runtime owns committed-state handoff

Task graphs now carry:

- `BaseStateSnapshot`
- `LatestCommittedStateSnapshot`

Only completed child workflows advance committed state. Approval-gated or failed child workflows do not.

### Resumability is first-class

`TaskExecutionRecord` must preserve:

- checkpoint id
- resume token/ref
- approval id
- pending approval
- resume state

This prevents follow-up execution from collapsing into non-resumable status strings.

### Auto-run remains narrow

Only allowlisted depth-1 follow-up tasks auto-execute. Deeper or non-allowlisted tasks may still be registered, explained, and replayed, but not recursively auto-run.

## Consequences

### Positive

- the system now closes its first proactive execution loop instead of stopping at task generation
- replay can explain parent workflow -> generated task -> child workflow -> child artifact -> child state commit
- runtime semantics are now more durable, stateful, and interview-defensible

### Trade-offs

- only `tax_optimization` and `portfolio_rebalance` are capability-backed in this phase
- automatic execution depth remains intentionally capped
- real Temporal / Postgres / provider infrastructure is still deferred behind stable seams
