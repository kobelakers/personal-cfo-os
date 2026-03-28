# Workflow A: Monthly Financial Review

## Current Executable Path

1. Natural language request goes through deterministic intake and becomes a `TaskSpec` with success criteria and required evidence.
2. `MonthlyReviewWorkflow` delegates evidence collection and reducer application to `MonthlyReviewService`, which orchestrates the observation tools.
3. `LedgerObservationAdapter` emits:
   - `transaction_batch`
   - `recurring_subscription_signal`
   - `late_night_spending_signal`
   - `debt_obligation_snapshot`
   - `portfolio_allocation_snapshot`
4. document adapters emit:
   - `payslip_statement`
   - `credit_card_statement`
   - `tax_document`
5. reducers build a deterministic `EvidencePatch` and update `FinancialWorldState`.
6. workflow dispatches `plan_request` to `PlannerAgent`, which assembles planning context and returns a deterministic plan.
7. workflow dispatches `memory_sync_request` to `MemorySteward`, which derives memories, applies write gating, and retrieves relevant memories.
8. workflow dispatches `report_draft_request` to `ReportAgent`, which generates a draft monthly review payload but does not emit a final artifact yet.
9. workflow dispatches `verification_request` to `VerificationAgent`, which runs:
   - evidence coverage checker
   - deterministic validator
   - business validator
   - success criteria checker
   - trajectory oracle
10. workflow dispatches `governance_evaluation_request` to `GovernanceAgent`, which evaluates risk, approval, and report disclosure.
11. only after governance allows or redacts does workflow dispatch `report_finalize_request` back to `ReportAgent` to produce the final artifact and `report_ready`.
12. runtime then decides whether the workflow completes, replans, or pauses for approval.

## Structural Boundary After Remediation

- workflow file: orchestration only
- workflow service: evidence collection + reducer orchestration
- planner agent: planning context assembly + deterministic planning
- memory steward: derived memory generation + gating + retrieval
- report agent: draft / finalize split with governance-aware finalization
- verification agent: reusable validation pipeline
- governance agent: reusable approval / disclosure evaluation
- runtime subsystem: checkpoint / replan / approval pause semantics
- protocol layer: typed request/result envelopes with correlation/causation chain

## Current Artifacts

- monthly review draft payload
- monthly review final report artifact
- workflow checkpoint journal
- workflow timeline entries
- agent dispatch lifecycle records
- memory access audit entries
- policy decision audit entries
- replay-ready trace dump inputs

## Still Stubbed

- agentic tax parsing is still a deterministic stub behind a formal adapter
- timeline is currently local structured dump state, not a remote tracing backend
- report generation is deterministic skill logic, not yet a reasoning-model synthesis layer
- agent execution is local synchronous dispatch, not yet durable remote actor execution

These stubs do not change the workflow shape: the system is already evidence-first, stateful, verifiable, and governed.
