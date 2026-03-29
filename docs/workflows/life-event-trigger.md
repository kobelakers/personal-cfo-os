# Workflow C: Life Event Trigger

## Current Executable Path

Workflow C is now a real proactive workflow rather than a Phase 1 contract placeholder.

Supported Phase 4A event kinds:

- `salary_change`
- `new_child`
- `job_change`
- `housing_change`

Authoritative execution chain:

1. structured life-event input enters deterministic intake and becomes a standard `TaskSpec` with intent `life_event_trigger`
2. `EventObservationAdapter` normalizes the event into typed `event_signal` evidence
3. `CalendarDeadlineObservationAdapter` emits typed `calendar_deadline` evidence when the event implies follow-up windows
4. supporting ledger/document evidence is gathered according to task scope and required evidence
5. reducers apply the evidence patch to `FinancialWorldState` and produce a state diff
6. `MemorySteward` writes event-driven memories and retrieves relevant prior decisions, tax signals, debt pressure, and procedural memories
7. `PlannerAgent` returns a block-level `ExecutionPlan`, and `plan.Blocks` remains the only execution truth source
8. workflow dispatches domain analysis blocks in plan order through:
   - `CashflowAgent`
   - `DebtAgent`
   - `TaxAgent`
   - `PortfolioAgent`
9. `VerificationAgent` pass 1 validates block grounding, evidence coverage, schema shape, and event-impact plausibility
10. `TaskGenerationAgent` generates typed downstream follow-up tasks from validated block results, event evidence, state diff, and retrieved memories
11. `VerificationAgent` pass 2 validates generated tasks and final life-event assessment consistency
12. `GovernanceAgent` evaluates disclosure, approval propagation, and spawned-task policy
13. runtime registers the generated follow-up task graph and assigns queue status such as `ready`, `deferred`, `waiting_approval`, `dependency_blocked`, or `queued_pending_capability`
14. `ReportAgent` finalizes `LifeEventAssessmentReport` only as a secondary artifact, and only after governance and runtime registration

## Primary Outputs

Workflow C is not report-first. Its primary outputs are:

- state diff caused by the event
- memory updates caused by the event
- typed generated task graph
- runtime-registered follow-up task records

`LifeEventAssessmentReport` is an important secondary artifact, but it is not the primary reason Workflow C exists.

## Follow-Up Task Contract

- `TaskSpec` remains the only executable goal contract in the repository.
- generated downstream tasks are represented as `TaskSpec + generation metadata`
- runtime never consumes a second incompatible goal type
- all follow-up tasks can round-trip back into a standard `TaskSpec`

Phase 4A execution contract:

- generated tasks are formally registered into runtime
- capability-gated intents such as `tax_optimization` and `portfolio_rebalance` are not fully executed yet
- instead, they are registered as `queued_pending_capability` with explicit:
  - `required_capability`
  - `missing_capability_reason`

This keeps the proactive loop real without pretending that deferred capabilities already have consumers.

## Domain Expansion Boundary

Workflow C is the only path that currently adds new domain execution beyond cashflow/debt.

- `TaxAgent` is used for event-triggered tax impact blocks
- `PortfolioAgent` is used for event-triggered portfolio impact blocks
- `CashflowAgent` and `DebtAgent` are reused where the event affects liquidity or housing debt pressure

This is deliberate sequencing:

1. system-agent backbone
2. first load-bearing domain agents
3. first proactive loop with targeted domain expansion
4. broader domain coverage later

## Verification and Governance

Workflow C uses a unique two-pass verification chain:

1. analysis blocks
2. verification pass 1
3. `TaskGenerationAgent`
4. verification pass 2 for generated tasks + final assessment
5. `GovernanceAgent`
6. runtime follow-up registration
7. `ReportAgent` finalize as secondary artifact only

This means generated tasks are not treated as a side effect outside the governed system.

## Observability and Replay

Replay and trace must be able to explain:

- which event was ingested
- which state fields changed
- which memories were written or retrieved
- which blocks were planned and in what order
- which domain agents executed those blocks
- why each follow-up task was generated
- why any task was suppressed, deferred, approval-gated, or capability-gated

## Still Deferred

- behavior-domain execution
- automatic execution of generated follow-up tasks
- stronger external calendar/policy signal integrations
- real Temporal/Postgres/pgvector/MinIO/provider infrastructure
- full operator UI for event timeline and task queue inspection
