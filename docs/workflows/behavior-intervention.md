# Workflow F: Behavior Intervention

## Purpose

`BehaviorInterventionWorkflow` is the first canonical Phase 6B workflow. It promotes behavior from a deferred idea into a real domain path while keeping workflow orchestration thin and reusing the existing planner / verification / governance / replay backbone.

## Authoritative Chain

1. deterministic intake parses a behavior-oriented request into `TaskSpec`
2. observation queries typed behavior-relevant evidence:
   - `transaction_batch`
   - `recurring_subscription_signal`
   - `late_night_spending_signal`
3. reducers update `FinancialWorldState`
4. `MemorySteward` syncs and retrieves relevant memory, including procedural memory
5. `PlannerAgent` emits one `behavior_intervention_block`
6. orchestrator-side `SkillSelector` resolves:
   - skill family
   - version
   - recipe
   - typed selection reasons
7. `BehaviorAgent` executes deterministic behavior analysis with the selected skill metadata
8. `ReportAgent` drafts `BehaviorInterventionReport`
9. `VerificationAgent` validates behavior-specific evidence / anomaly / recommendation consistency
10. `GovernanceAgent` evaluates intervention intensity and approval policy
11. runtime finalizes `completed`, `failed`, or `waiting_approval`
12. runtime persists:
   - skill execution record
   - behavior report artifact
   - replay/debug artifacts
   - procedural skill-outcome memory

## Behavior Output Contract

`BehaviorBlockResult` is a load-bearing block result, not a report sidecar. It enters the same main chain as other domain blocks and includes:

- summary
- deterministic behavior metrics
- anomaly/trend summary
- evidence refs
- metric refs
- grounding refs
- recommendations
- risk/caveat/approval fields
- selected skill metadata
- skill selection reasons

## Skill Selection Boundary

The behavior workflow does not let `BehaviorAgent` privately choose a recipe. Concrete skill selection is made before dispatch and written into the typed execution contract.

Current behavior skill families:

- `subscription_cleanup`
- `late_night_spend_nudge`
- `discretionary_guardrail`

`discretionary_guardrail` is the canonical escalation family, with:

- `soft_nudge.v1`
- `budget_guardrail.v1`
- `hard_cap.v1`

## Runtime Semantics

- validator failure -> `failed`
- high-intensity guardrail recommendation -> `waiting_approval`
- operator approval resumes continuation without redefining the workflow model
- behavior workflow writes procedural memory for later skill-selection influence

## Current Boundary

- this workflow is the canonical 6B golden path
- it does not auto-wire itself into Monthly Review or Workflow C as a generated follow-up yet
- it does not execute real external payment/account actions
