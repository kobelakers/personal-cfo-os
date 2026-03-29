# Workflow B: Debt vs Invest Decision

## Current MVP Scope

The workflow is still intentionally scoped as an evidence-backed MVP, but it is no longer a direct service chain. It now runs through the same system-agent execution backbone as Monthly Review.

Current path:

1. request enters deterministic intake and becomes a `TaskSpec`
2. `DebtVsInvestService` orchestrates debt, cashflow, and portfolio evidence collection plus reducer application
3. reducers update state
4. workflow dispatches `memory_sync_request` to `MemorySteward`
5. workflow dispatches `plan_request` to `PlannerAgent`
6. `PlannerAgent` returns a block-level `ExecutionPlan`, and `plan.Blocks` becomes the only execution truth source
7. workflow dispatches:
   - `cashflow_liquidity_block` -> `CashflowAgent`
   - `debt_tradeoff_block` -> `DebtAgent`
8. workflow dispatches `report_draft_request` to `ReportAgent`, which aggregates domain block outputs into a debt decision draft
9. workflow dispatches `verification_request` to `VerificationAgent`, which validates blocks before final report validation
10. workflow dispatches `governance_evaluation_request` to `GovernanceAgent`
11. if governance allows or redacts, workflow dispatches `report_finalize_request` to `ReportAgent`
12. runtime decides whether the workflow completes, replans, or pauses for approval

## What This MVP Already Proves

- the system does not answer debt-vs-invest from chat history
- the conclusion is rooted in typed evidence and deterministic metrics
- planning, memory, domain analysis, reporting, verification, and governance are now separate typed agent steps
- retrieved memories can change block ordering or recommendation framing
- `CashflowAgent` and `DebtAgent` now supply the core tradeoff analysis; `ReportAgent` only aggregates and finalizes
- the workflow file is now orchestration only rather than a second monolith

## What Is Deferred

- multi-scenario simulation tree
- richer liquidity stress testing
- explicit tax-adjusted investment return modeling
- multi-step replan loops for alternative decision paths
- portfolio / tax / behavior domain execution boundaries
- remote/durable agent dispatch
