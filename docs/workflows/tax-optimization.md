# Workflow D: Tax Optimization

## Purpose

`TaxOptimizationWorkflow` is the first capability-backed follow-up workflow for a generated proactive task. It consumes a standard `TaskSpec` plus a narrow `FollowUpActivationContext`, then executes a real child workflow through the existing system-agent backbone.

## Authoritative Chain

1. runtime activates a generated `tax_optimization` task when capability is available and auto-run policy allows it
2. child workflow receives:
   - `TaskSpec`
   - `FollowUpActivationContext`
   - latest committed state from the parent task graph
3. follow-up observation queries event/deadline/document/transaction evidence using activation-seed scope
4. reducers update state and produce a child state diff
5. `MemorySteward`
6. `PlannerAgent` -> `tax_optimization_block`
7. `TaxAgent`
8. `ReportAgent` draft -> `TaxOptimizationReport`
9. `VerificationAgent`
10. `GovernanceAgent`
11. `ReportAgent` finalize
12. runtime records execution outcome and commits child state only on `completed`

## Output Contract

`TaxOptimizationReport` is a formal report contract with:

- `Summary`
- `DeterministicMetrics`
- `RecommendedActions`
- `RiskFlags`
- provenance fields for block / memory / evidence
- `ApprovalRequired`

## Runtime Semantics

- `waiting_approval` must persist checkpoint / resume / approval anchors in `TaskExecutionRecord`
- `failed` must preserve failure category, summary, retry metadata, and attempt count
- `completed` updates the parent task graph's `LatestCommittedStateSnapshot`

## Current Boundary

- this workflow is capability-backed and auto-executable only as a first-level follow-up from Workflow C
- it does not recursively auto-execute newly spawned tasks
