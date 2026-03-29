# ADR 0006: Load-Bearing Domain Execution

## Status

Accepted

## Context

After Phase 3A, Personal CFO OS already had a real system-agent execution backbone:

- `PlannerAgent`
- `MemorySteward`
- `ReportAgent`
- `VerificationAgent`
- `GovernanceAgent`

That was enough to move the project beyond schema-first protocol design, but it still left an important weakness: cashflow and debt analysis were not yet executed through real domain-agent boundaries. The system could claim domain-agent readiness, but the report path still risked feeling too workflow-centric or report-centric.

At the same time, we did not want to jump straight into full domain coverage. That would have expanded scope too early, mixed unfinished domains into the main path, and weakened the architecture story.

## Decision

We introduce **load-bearing domain execution** in a deliberately narrow form:

- `CashflowAgent` and `DebtAgent` are now the first real domain agents in the execution backbone
- `PlannerAgent` returns a block-level `ExecutionPlan`
- `plan.Blocks` is the only execution truth source for downstream dispatch
- workflow iterates `plan.Blocks` and dispatches domain agents accordingly
- `MemorySteward` retrieved memories must be allowed to change block order or block emphasis
- `ExecutionContextAssembler` and `VerificationContextAssembler` now provide block-specific execution material
- `ReportAgent` is explicitly downgraded to an aggregator/finalizer; it may not recreate missing cashflow/debt core analysis from raw state/evidence
- `VerificationAgent` now validates domain blocks before final report validation and may short-circuit final validation on severe block failure

## Consequences

### Positive

- The project now has a first real domain-agent execution path, not just system-agent boundaries.
- Planning, memory, and context become load-bearing instead of sidecar steps.
- Replay can explain not only which system agents executed, but also which domain blocks executed, in what order, and with which context slices.
- The architecture is more defensible as a 2026-style agent system because execution is increasingly shaped by protocol, plan, context, and verification rather than a workflow file directly stitching services together.

### Trade-offs

- Only cashflow and debt are agentized; portfolio, tax, and behavior remain residual deterministic sections for now.
- Dispatch is still local synchronous execution rather than durable remote mailboxes.
- Semantic retrieval and agentic document parsing are still stubbed behind stable interfaces.

## Why Not Full Domain Coverage Yet

Full domain coverage would add breadth faster than the architecture can currently absorb. The deliberate sequence is:

1. system-agent backbone
2. first load-bearing domain agents
3. broader domain coverage
4. stronger async/durable execution

This keeps the project honest: it is now stronger than an agent-ready substrate, but it is still not a fully realized strong multi-agent finance operating system.
