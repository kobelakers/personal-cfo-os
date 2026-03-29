# ADR 0010: Real Intelligence Substrate For Monthly Review

## Status

Accepted

## Context

By the end of Phase 5A, Personal CFO OS already had a governed workflow backbone, a real domain-agent execution path, proactive follow-up execution, and an operator-runnable durable runtime plane. What it still lacked was a real cognition chain beneath the workflow:

- prompts were not yet formal system objects
- provider-backed reasoning was not load-bearing in the main execution path
- structured output did not yet exist as a first-class subsystem
- context budgets were not token-aware
- prompt/provider/token/fallback evidence was not visible in Monthly Review traces

The project needed to move forward without breaking the backbone contracts already established in earlier phases.

## Decision

Phase 5B introduces a real intelligence substrate, but only for the Monthly Review golden path and only for two agents:

- `PlannerAgent`
- `CashflowAgent`

The implementation adds four load-bearing subsystems:

1. `internal/model`
   - provider-agnostic chat / structured interfaces
   - OpenAI-compatible live adapter
   - stub adapters for future providers
2. `internal/prompt`
   - versioned prompt registry
   - file-backed templates
   - render trace metadata
3. `internal/structured`
   - schema validation
   - parser
   - repair retry
   - deterministic fallback
   - structured trace metadata
4. token-aware upgrades in `internal/context`
   - planning and cashflow execution budgets now reflect model-window constraints

This is additive-only to the existing backbone:

- `TaskSpec` remains the only goal contract
- `planning.ExecutionPlan` remains the only execution-plan truth source
- `protocol.AgentEnvelope` remains the dispatch envelope
- `SystemStepBus` remains the workflow-facing execution boundary

## Consequences

### Positive

- Monthly Review now has a real provider-backed intelligence path that is still typed, verifiable, governed, and observable.
- `PlannerAgent` and `CashflowAgent` can use real model output without pushing provider/prompt/parser logic into workflow files.
- prompt version, provider call, token usage, estimated cost, repair, and fallback now become first-class trace evidence.
- deterministic finance truth remains in code, so model output does not become the source of truth for amounts or ratios.

### Negative / Deferred

- only one workflow is upgraded in this phase
- only two agents use real provider-backed reasoning
- durable memory, embeddings, retrieval hardening, finance hardening, and deeper validator expansion remain out of scope
- intelligence trace is visible in workflow dumps, but not yet promoted to a separate durable operator-facing truth plane

## Why Not More

This ADR intentionally avoids:

- spreading LLM calls across every domain agent
- adding real provider logic directly to workflows
- inventing a second plan or protocol contract
- mixing durable memory, finance hardening, or replay-plane expansion into the same phase

That keeps Phase 5B aligned with the 12-layer architecture and prevents the system from regressing into a 2025-style “workflow with a few model calls” implementation.
