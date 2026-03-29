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
6. workflow dispatches `memory_sync_request` to `MemorySteward`, which derives memories, applies write gating, and retrieves relevant memories.
7. workflow dispatches `plan_request` to `PlannerAgent`, which assembles planning context and returns a block-level `ExecutionPlan`.
8. `plan.Blocks` becomes the only execution truth source; workflow iterates it in order instead of rebuilding structure from intent.
9. for each block, workflow assembles block-specific execution context and dispatches:
   - `cashflow_review_block` -> `CashflowAgent`
   - `debt_review_block` -> `DebtAgent`
10. workflow dispatches `report_draft_request` to `ReportAgent`, which aggregates typed domain block results and residual deterministic sections into a draft, but does not emit a final artifact yet.
11. workflow dispatches `verification_request` to `VerificationAgent`, which runs block-level validation first, then only runs final report validation if no severe block failure is found.
12. workflow dispatches `governance_evaluation_request` to `GovernanceAgent`, which evaluates risk, approval, and report disclosure.
13. only after governance allows or redacts does workflow dispatch `report_finalize_request` back to `ReportAgent` to produce the final artifact and `report_ready`.
14. runtime then decides whether the workflow completes, replans, or pauses for approval.

## Structural Boundary After Remediation

- workflow file: orchestration only
- workflow service: evidence collection + reducer orchestration
- planner agent: planning context assembly + deterministic block planning
- memory steward: derived memory generation + gating + retrieval
- cashflow agent: typed cashflow block analysis using deterministic metrics and selected evidence
- debt agent: typed debt block analysis using deterministic metrics and selected evidence
- report agent: aggregator + finalize split with governance-aware finalization
- verification agent: block + final validation pipeline with short-circuit on severe block failures
- governance agent: reusable approval / disclosure evaluation
- runtime subsystem: checkpoint / replan / approval pause semantics
- protocol layer: typed request/result envelopes with correlation/causation chain

## Current Artifacts

- monthly review draft payload
- monthly review final report artifact
- planner block plan snapshot
- cashflow block result
- debt block result
- workflow checkpoint journal
- workflow timeline entries
- agent dispatch lifecycle records
- memory access audit entries
- policy decision audit entries
- replay-ready trace dump inputs

## Still Stubbed

- agentic tax parsing is still a deterministic stub behind a formal adapter
- timeline is currently local structured dump state, not a remote tracing backend
- portfolio / tax / behavior areas are still residual deterministic sections, not yet domain-agent-executed
- agent execution is local synchronous dispatch, not yet durable remote actor execution

These stubs do not change the workflow shape: the system is already evidence-first, stateful, context-engineered, verifiable, governed, and now includes first real load-bearing domain execution.
