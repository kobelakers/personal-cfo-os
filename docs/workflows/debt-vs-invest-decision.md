# Workflow B: Debt vs Invest Decision

## Current Trust-Hardened Scope

The workflow is still intentionally scoped, but it is no longer just an evidence-backed MVP. In Phase 5D it becomes the canonical deterministic approval path for trustworthy finance reasoning.

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
8. workflow dispatches `report_draft_request` to `ReportAgent`, which aggregates domain block outputs into a debt decision draft and carries shared typed recommendation/risk/approval fields
9. workflow dispatches `verification_request` to `VerificationAgent`, which now runs grounding, numeric, and finance business validators on the final report surface
10. workflow dispatches `governance_evaluation_request` to `GovernanceAgent`, which consumes recommendation type + risk level + disclosure readiness
11. if governance allows or redacts, workflow dispatches `report_finalize_request` to `ReportAgent`
12. if governance requires approval, runtime persists checkpoint/resume anchors and moves the workflow to `waiting_approval`
13. operator approval resumes continuation from the existing runtime resume path instead of rerunning the whole workflow from the top

## Canonical Waiting Approval Proof

Phase 5D uses Debt vs Invest as the canonical deterministic approval sample:

1. fixtures produce low emergency-fund coverage or high debt pressure
2. deterministic `DebtAgent` still emits an aggressive `invest_more` recommendation
3. that recommendation is typed as high-risk and approval-required
4. `GovernanceAgent` consumes the typed contract and returns `RequireApproval`
5. runtime transitions the workflow into `waiting_approval`
6. operator approval can later resume finalization through the existing continuation path

This path is fixture-driven and deterministic; it does not depend on model randomness.

## What This Workflow Already Proves

- the system does not answer debt-vs-invest from chat history
- the conclusion is rooted in typed evidence and deterministic metrics from the Finance Engine
- planning, memory, domain analysis, reporting, verification, and governance are now separate typed agent steps
- retrieved memories can change block ordering or recommendation framing
- `CashflowAgent` and `DebtAgent` now supply the core tradeoff analysis; `ReportAgent` only aggregates and finalizes
- recommendation type / risk / caveat / approval semantics are now shared typed fields, not just prose
- governance can now genuinely gate a high-risk debt-vs-invest recommendation into `waiting_approval`
- the workflow file is now orchestration only rather than a second monolith

## What Is Deferred

- multi-scenario simulation tree
- richer liquidity stress testing
- explicit tax-adjusted investment return modeling
- multi-step replan loops for alternative decision paths
- portfolio / tax / behavior domain execution boundaries
- remote/durable agent dispatch
