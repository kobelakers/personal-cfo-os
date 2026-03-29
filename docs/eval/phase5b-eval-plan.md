# Phase 5B Evaluation Plan

Phase 5B validates the first real-intelligence-backed Monthly Review golden path without widening scope into durable memory, finance hardening, or broader domain rollout.

## Covered Checks

- OpenAI-compatible provider adapter happy path, timeout, rate-limit retry, 5xx handling, and malformed-body failure
- prompt registry lookup, render, version trace, and prompt render metadata
- planner structured output parse / validate / repair / fallback
- cashflow structured output parse / validate / grounding pre-check / fallback
- token-aware planning and cashflow execution context budgets
- Monthly Review golden path completes with provider-backed planner + cashflow
- trace dump exposes prompt id/version, provider call, token usage, estimated cost, and structured-output repair/fallback metadata
- mock run evidence can be reproduced through `scripts/run_monthly_review_5b.sh`
- env-gated live smoke can prove that real provider calls enter the Monthly Review golden path when local credentials are present

## Important Negative Paths

- provider timeout
- rate limit retry before success
- malformed JSON from provider
- schema-invalid planner output
- grounding-invalid cashflow output
- deterministic fallback success
- fallback failure surfacing through workflow/runtime failure semantics

## Explicitly Deferred Beyond Phase 5B

- durable memory store and real embeddings
- semantic retrieval hardening
- deterministic finance engine hardening
- deeper business-rule validator expansion
- broader provider comparison or multi-provider benchmarks
- full replay/eval plane maturation
