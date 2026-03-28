# Workflow A: Monthly Financial Review

## Phase 2 Executable Path

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
6. `memory.WorkflowMemoryService` derives monthly-review memories, passes them through memory write gating, and retrieves relevant memories back through hybrid retrieval.
7. `DefaultContextAssembler` creates planning / execution / verification contexts with state blocks, evidence blocks, memory blocks, and skill injection.
8. `DeterministicPlanner` creates the explicit monthly review plan.
9. `MonthlyReviewSkill` generates a structured report with risk items, optimization suggestions, todo items, and approval flag.
10. `verification.Pipeline` runs:
   - evidence coverage checker
   - deterministic validator
   - business validator
   - success criteria checker
   - trajectory oracle
11. `governance.ApprovalService` evaluates approval and report disclosure.
12. runtime then decides whether the workflow completes, replans, or pauses for approval.

## Structural Boundary After Remediation

- workflow file: orchestration only
- workflow service: evidence collection + reducer orchestration
- memory subsystem: derived memory generation + gating + retrieval
- verification subsystem: reusable validation pipeline
- governance subsystem: reusable approval / disclosure evaluation
- runtime subsystem: checkpoint / replan / approval pause semantics
- artifact service: report artifact generation

## Current Artifacts

- monthly review report artifact
- workflow checkpoint journal
- workflow timeline entries
- memory access audit entries
- policy decision audit entries
- replay-ready trace dump inputs

## Still Stubbed

- agentic tax parsing is still a deterministic stub behind a formal adapter
- timeline is currently local structured dump state, not a remote tracing backend
- report generation is deterministic skill logic, not yet a reasoning-model synthesis layer

These stubs do not change the workflow shape: the system is already evidence-first, stateful, verifiable, and governed.
