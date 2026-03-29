# ADR 0007: First Proactive Life-Event Loop

## Status

Accepted

## Context

After Phase 3B, Personal CFO OS already had:

- a real system-agent execution backbone
- load-bearing `CashflowAgent` and `DebtAgent`
- block-level planning through `plan.Blocks`
- block + final verification
- replayable observability for passive analysis workflows

What the system still lacked was a proactive loop. It could analyze a user request well, but it still behaved like a reactive analysis system rather than a finance OS that notices events, updates internal state, and creates follow-up work.

At the same time, we did not want Phase 4A to explode into:

- real provider integration
- real distributed runtime
- recursive task execution
- full domain expansion

## Decision

We introduce a narrow but real **proactive life-event loop**:

- Workflow C (`life_event_trigger`) becomes executable
- life events are first normalized into typed evidence
- reducers update state and emit a state diff
- `MemorySteward` writes/retrieves event-relevant memories
- `PlannerAgent` returns a block-level event-impact plan
- workflow dispatches event blocks through `CashflowAgent`, `DebtAgent`, `TaxAgent`, and `PortfolioAgent`
- `VerificationAgent` runs pass 1 on block analysis
- `TaskGenerationAgent` creates typed follow-up tasks from validated inputs only
- `VerificationAgent` runs pass 2 on generated tasks and final assessment
- `GovernanceAgent` decides disclosure, approval propagation, and spawned-task policy
- runtime registers follow-up tasks into a task graph
- `ReportAgent` finalizes `LifeEventAssessmentReport` only as a secondary artifact

## Key Boundary Decisions

### `TaskSpec` remains the only executable goal contract

Generated downstream tasks may carry generation metadata, but they must still round-trip to a standard `TaskSpec`. Runtime does not consume a second incompatible goal type.

### `TaskGenerationAgent` is not a coordinator

`TaskGenerationAgent` may:

- consume validated block results
- consume event evidence, state diff, and retrieved memories
- generate typed downstream tasks and dependency edges

It may not:

- redo domain analysis
- rebuild `plan.Blocks`
- replace `GovernanceAgent`
- become a new workflow brain

### Follow-up tasks are registered before they are fully executable

In Phase 4A, generated tasks are formal runtime objects, but capability-gated intents such as `tax_optimization` and `portfolio_rebalance` are intentionally registered as `queued_pending_capability` instead of being auto-executed recursively.

This gives the system a real proactive queue without pretending that all downstream capabilities already exist.

### Workflow C is not report-first

The primary outputs of Workflow C are:

- state diff
- memory updates
- generated task graph
- runtime-registered follow-up tasks

`LifeEventAssessmentReport` exists so the workflow has a complete artifact/report contract, but it is a secondary artifact rather than the primary reason for the workflow.

## Consequences

### Positive

- The project now has its first proactive loop, not just reactive analysis workflows.
- Runtime, verification, governance, and observability now all participate in task generation, not only report production.
- `TaxAgent` and `PortfolioAgent` enter the system through a constrained, high-value path instead of being bolted onto every workflow at once.
- The system becomes much easier to defend as a 2026-style OS-shaped agent system because it can explain why it generated follow-up work.

### Trade-offs

- Generated follow-up tasks are registered but not fully executed in the same phase.
- Behavior-domain execution remains deferred.
- Real infrastructure and real external policy/market sources remain stubbed behind stable seams.

## Why Not More Than This Yet

Doing more in Phase 4A would likely collapse the architecture back into either:

- workflow-heavy orchestration
- fake “many agents chatting” demos
- infrastructure-first work that does not strengthen the proactive system story

The deliberate sequence is now:

1. system-agent backbone
2. first load-bearing domain agents
3. first proactive life-event loop
4. broader domain execution + stronger runtime semantics later
