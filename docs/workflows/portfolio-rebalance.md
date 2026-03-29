# Workflow E: Portfolio Rebalance

## Purpose

`PortfolioRebalanceWorkflow` is the second capability-backed follow-up workflow. It closes the first proactive execution loop for allocation drift and liquidity follow-up without introducing a new execution backbone.

## Authoritative Chain

1. runtime activates a generated `portfolio_rebalance` task when capability is available and auto-run policy allows it
2. child workflow receives:
   - `TaskSpec`
   - `FollowUpActivationContext`
   - latest committed state from the parent task graph
3. follow-up observation queries event/deadline/portfolio/transaction evidence using activation-seed scope
4. reducers update state and produce a child state diff
5. `MemorySteward`
6. `PlannerAgent` -> `portfolio_rebalance_block`
7. `PortfolioAgent`
8. `ReportAgent` draft -> `PortfolioRebalanceReport`
9. `VerificationAgent`
10. `GovernanceAgent`
11. `ReportAgent` finalize
12. runtime records execution outcome and commits child state only on `completed`

## Output Contract

`PortfolioRebalanceReport` is a formal report contract with:

- `Summary`
- `DeterministicMetrics`
- `RecommendedActions`
- `RiskFlags`
- provenance fields for block / memory / evidence
- `ApprovalRequired`

## Runtime Semantics

- liquidity or rebalance caveats must stay structured in `RiskFlags`
- child state is only committed when the workflow reaches `completed`
- approval-gated or failed runs still remain replayable through execution records

## Current Boundary

- this workflow is capability-backed and auto-executable only as a first-level follow-up from Workflow C
- it does not recurse into automatic execution of newly generated tasks
