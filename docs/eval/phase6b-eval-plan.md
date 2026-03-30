# Phase 6B Evaluation Plan

Phase 6B upgrades the current governed finance agent backbone with a **versioned skill runtime, a formal behavior domain, and procedural-memory-influenced skill selection**. It does not widen scope into UI, infra promotion, runtime promotion, or live-provider regression.

## Definition of Done

- `behavior_intervention` enters through deterministic intake and produces a valid `TaskSpec`.
- planner emits a `behavior_intervention_block` with allowed skill families and skill-selection hint metadata.
- orchestrator-side skill selection resolves a typed `SkillSelection` with family / version / recipe / reasons.
- `BehaviorBlockResult` flows through `analysis.BlockResultEnvelope` as a load-bearing block result rather than a report sidecar.
- at least one skill family demonstrates recipe/version differentiation.
- procedural memory is written into the existing durable memory substrate and changes the next similar skill selection.
- runtime persists skill execution records.
- verification and governance can both affect the behavior workflow outcome.
- replay/eval/debug explains why this skill / why this recipe / which memory influenced the change.
- deterministic 6B regression corpus runs with four stable scenarios.
- 5B / 5C / 5D / 6A regression coverage still passes.

## Canonical Workflow

Phase 6B intentionally promotes one narrow workflow rather than spreading behavior logic across every path:

- canonical intent: `behavior_intervention`
- canonical high-risk proof:
  - skill family: `discretionary_guardrail`
  - recipe: `hard_cap.v1`
  - governance outcome: `waiting_approval`

The authoritative chain is:

1. deterministic intake -> `TaskSpec`
2. observation / reducer / state update
3. memory sync + retrieval
4. planner emits `behavior_intervention_block`
5. orchestrator-side `SkillSelector` resolves family/version/recipe
6. `BehaviorAgent` executes deterministic domain analysis
7. `VerificationAgent`
8. `GovernanceAgent`
9. finalize / `waiting_approval`
10. runtime persists skill execution + procedural memory outcome

## Canonical Deterministic Corpus

`phase6b-default` is deliberately separate from `phase6a-default` and remains deterministic/mock-only.

It covers these four scenarios:

1. `behavior_intervention_happy_path`
2. `behavior_skill_selection_influenced_by_procedural_memory`
3. `behavior_intervention_waiting_approval`
4. `behavior_intervention_validator_failure`

Live provider paths are explicitly excluded from the canonical 6B regression corpus so skill-selection and replay evidence stay stable.

## Replay / Debug / Compare Expectations

Replay must now explain at least:

- why this skill family was selected
- why this recipe/version was chosen
- which evidence/state/procedural memory influenced selection
- what skill execution produced
- what behavior validators concluded
- why governance allowed, denied, or required approval
- which skill-outcome procedural memory was written

Compare must make changed skill selection visible in a deterministic, human-readable way.

## Run Evidence

Canonical commands:

```bash
go run ./cmd/eval --mode corpus --corpus phase6b-default --format summary
./scripts/run_behavior_intervention_6b.sh mock
```

Stable 6B sample outputs:

- `docs/eval/samples/phase6b_eval_default_corpus.json`
- `docs/eval/samples/phase6b_replay_behavior_intervention.json`
- `docs/eval/samples/phase6b_replay_behavior_intervention_waiting_approval.json`
- `docs/eval/samples/phase6b_compare_procedural_memory_skill_selection.json`

## Explicitly Deferred Beyond Phase 6B

- behavior follow-up auto-generation from Monthly Review or Workflow C
- live-provider-backed behavior reasoning as a regression baseline
- real payment / card / bank account external actions
- async runtime promotion
- UI/debug panel expansion
- external protocol or infra promotion
