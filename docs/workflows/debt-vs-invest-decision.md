# Workflow B: Debt vs Invest Decision

## Phase 2 MVP Scope

Phase 2 only implements the first evidence-backed MVP, not the full future strategy workflow.

Current path:

1. request enters deterministic intake and becomes a `TaskSpec`
2. debt, cashflow, and portfolio evidence are collected
3. reducers update state
4. planner creates a minimal comparison plan
5. `DebtOptimizationSkill` produces an evidence-backed conclusion
6. business validator checks the output
7. risk classifier assigns action risk
8. approval decider returns `allow` or `require_approval`

## What This MVP Already Proves

- the system does not answer debt-vs-invest from chat history
- the conclusion is rooted in typed evidence and deterministic metrics
- business validation, risk classification, and approval decision are already separate steps

## What Is Deferred

- multi-scenario simulation tree
- richer liquidity stress testing
- explicit tax-adjusted investment return modeling
- multi-step replan loops for alternative decision paths
