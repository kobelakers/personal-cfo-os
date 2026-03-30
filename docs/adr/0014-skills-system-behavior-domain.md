# ADR 0014: Versioned Skill Runtime and Formal Behavior Domain

## Status

Accepted

## Context

By the end of Phase 6A, Personal CFO OS already had:

- a governed workflow/runtime backbone
- real intelligence, memory, and finance-trust layers
- a canonical replay/eval/debug plane

What it still lacked was a load-bearing **Skills + Behavior** layer. Skills were still too close to interface-level contracts, behavior remained deferred, and procedural memory did not yet influence real capability selection on the main path.

## Decision

Phase 6B adds one narrow but formal capability/domain path with these boundaries:

1. `behavior_intervention` becomes a real deterministic intake-to-runtime workflow
2. `internal/skills` becomes the canonical skill system with:
   - manifests
   - family / version / recipe
   - trigger / policy / expected output
   - typed selection reasons
   - runtime execution records
3. behavior becomes an independent domain under `internal/behavior`; it does not collapse back into cashflow logic
4. `BehaviorBlockResult` enters `analysis.BlockResultEnvelope` and therefore becomes:
   - verification input
   - governance input
   - report aggregation input
   - replay/debug input
5. procedural memory extends the existing durable memory substrate rather than creating a second memory system
6. skill selection happens before domain dispatch and is written into the execution contract; `BehaviorAgent` does not privately choose recipes
7. canonical high-risk proof is deterministic:
   - family = `discretionary_guardrail`
   - recipe = `hard_cap.v1`
   - runtime outcome = `waiting_approval`
8. canonical 6B regression remains deterministic/mock-only and lives in a dedicated `phase6b-default` corpus

## Consequences

### Positive

- skills are now explicit system capabilities rather than prompt fragments or report hints
- behavior is now a real domain with typed metrics, anomalies, validators, and governed recommendations
- procedural memory can now change future skill/recipe selection in a replayable way
- replay/eval/debug can explain why a skill was selected and why a later run chose a stronger recipe

### Negative / Deferred

- behavior is still intentionally scoped to a single canonical workflow
- no real payment/bank/account external actions are introduced
- live-provider-backed behavior reasoning is still out of scope for stable regression
- this ADR does not introduce UI expansion, runtime promotion, or external protocol exposure
