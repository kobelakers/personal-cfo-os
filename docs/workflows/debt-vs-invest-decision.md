# Workflow B: Debt vs Invest Decision

## Phase 2 MVP Scope

Phase 2 only implements the first evidence-backed MVP, not the full future strategy workflow.

Current path:

1. request enters deterministic intake and becomes a `TaskSpec`
2. `DebtVsInvestService` orchestrates debt, cashflow, and portfolio evidence collection plus reducer application
3. reducers update state
4. memory service performs debt-decision memory write/read
5. planner creates a minimal comparison plan
6. execution and verification contexts are both assembled
7. `DebtOptimizationSkill` produces an evidence-backed conclusion
8. verification pipeline checks coverage, business rules, success criteria, and oracle outcome
9. approval service evaluates risk classification, action approval, and report disclosure
10. runtime decides whether the workflow completes or pauses for approval

## What This MVP Already Proves

- the system does not answer debt-vs-invest from chat history
- the conclusion is rooted in typed evidence and deterministic metrics
- memory, verification, risk classification, approval decision, and report disclosure are already separate subsystem steps
- the workflow file is now orchestration only rather than a second monolith

## What Is Deferred

- multi-scenario simulation tree
- richer liquidity stress testing
- explicit tax-adjusted investment return modeling
- multi-step replan loops for alternative decision paths
- strong actor-style system agent execution boundaries
