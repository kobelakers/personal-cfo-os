# ADR 0012: Trustworthy Finance Reasoning

## Status

Accepted

## Context

By the end of Phase 5C, Personal CFO OS already had:

- a real-intelligence-backed Monthly Review golden path
- a first real memory substrate
- a governed workflow/runtime backbone

What it still lacked was a **trustworthy finance reasoning substrate**. Recommendations could be well-structured and memory-aware, but the repository still needed a stronger guarantee that key financial advice was grounded in deterministic numbers, checked by explicit finance/business rules, and gated by governance when risk crossed a threshold.

## Decision

Phase 5D adds a trust layer on top of the current backbone with these boundaries:

1. `internal/finance` becomes the formal numeric truth source for the current live path.
2. Shared typed recommendation objects carry risk, grounding refs, metric refs, caveats, policy refs, and approval semantics.
3. verification is split into three deterministic layers:
   - grounding validator
   - numeric consistency validator
   - business rule validator
4. GovernanceAgent consumes typed recommendation/risk/disclosure state and can return:
   - allow
   - require approval
   - deny
5. high-risk finance actions must be explainable in trace through:
   - metric refs
   - validator verdicts
   - policy rule hits
   - approval triggers

The canonical proof path for 5D is intentionally deterministic:

- Debt vs Invest
- low emergency fund or high debt pressure
- aggressive `invest_more`
- GovernanceAgent -> `waiting_approval`

This proof path is fixture-driven rather than model-random.

## Consequences

### Positive

- the system now separates numeric truth from model reasoning more explicitly
- recommendation trust no longer depends on "model sounded reasonable"
- governance approval is now tied to typed recommendation semantics rather than keyword matching in prose
- final reports and trace dumps can explain why a recommendation passed, failed, or required approval

### Negative / Deferred

- this is not a full finance engine or market simulator
- Tax / Portfolio only receive minimal deterministic metric bundles plus caveat/disclosure hooks in this phase
- validator failures currently map to `failed` rather than auto-replanning
- this ADR does not introduce 6A replay/eval-plane maturation, async runtime promotion, or additional provider-backed agents
