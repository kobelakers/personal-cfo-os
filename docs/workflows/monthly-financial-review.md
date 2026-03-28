# Workflow A: Monthly Financial Review

## Phase 2 Executable Path

1. Natural language request goes through deterministic intake and becomes a `TaskSpec` with success criteria and required evidence.
2. `LedgerObservationAdapter` emits:
   - `transaction_batch`
   - `recurring_subscription_signal`
   - `late_night_spending_signal`
   - `debt_obligation_snapshot`
   - `portfolio_allocation_snapshot`
3. document adapters emit:
   - `payslip_statement`
   - `credit_card_statement`
   - `tax_document`
4. reducers build a deterministic `EvidencePatch` and update `FinancialWorldState`.
5. derived memories are written through memory write policy checks and retrieved back through hybrid retrieval.
6. `DefaultContextAssembler` creates planning and execution contexts with state blocks, evidence blocks, memory blocks, and skill injection.
7. `DeterministicPlanner` creates the explicit monthly review plan.
8. `MonthlyReviewSkill` generates a structured report with risk items, optimization suggestions, todo items, and approval flag.
9. verification runs:
   - evidence coverage checker
   - deterministic validator
   - business validator
   - success criteria checker
   - trajectory oracle
10. governance and runtime then decide whether the workflow completes, replans, or pauses for approval.

## Current Artifacts

- monthly review report artifact
- workflow checkpoints
- workflow timeline entries
- memory access audit entries
- policy decision audit entries

## Still Stubbed

- agentic tax parsing is still a deterministic stub behind a formal adapter
- timeline is currently local JSON-serializable state, not a remote tracing backend
- report generation is deterministic skill logic, not yet a reasoning-model synthesis layer

These stubs do not change the workflow shape: the system is already evidence-first, stateful, verifiable, and governed.
