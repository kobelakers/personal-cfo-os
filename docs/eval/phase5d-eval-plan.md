# Phase 5D Evaluation Plan

Phase 5D upgrades the current backbone from "real intelligence + real memory" to **trustworthy finance reasoning**. It does not widen scope into 6A replay/eval-plane maturation, 6B skills runtime, UI work, or async runtime promotion.

## Definition of Done

- Monthly Review final report includes grounded recommendations, risk fields, caveat/disclosure fields, and approval fields.
- Debt vs Invest has at least one fixture-driven, deterministic path that enters `waiting_approval`.
- grounding, numeric, and business validators all execute on the live path and emit typed diagnostics.
- Finance Engine is the only numeric truth source for the current live path.
- trace can explain why a recommendation was grounded, validated, blocked, or sent to approval.
- 5B / 5C regression coverage still passes.

## Covered Checks

- Finance Engine emits typed metric bundles and `metric_records` for:
  - Monthly Review cashflow/debt metrics
  - Debt vs Invest tradeoff metrics
  - minimal Tax / Portfolio deterministic bundles for validator hooks
- recommendation semantics are unified through shared typed recommendation/risk fields rather than scattered natural-language-only payloads
- grounding validator rejects:
  - missing grounding refs
  - unsupported metric/evidence/memory refs
  - high-risk recommendations without caveats/disclosure
- numeric validator rejects:
  - unsupported metric refs
  - unsupported numeric claims in recommendation/risk narrative
  - metric/text inconsistency against deterministic metric records
- business validator rejects:
  - aggressive liquidity-insensitive Monthly Review advice
  - debt-vs-invest `invest_more` recommendations that do not escalate under low buffer/high debt pressure
  - missing tax / portfolio caveat requirements on the active follow-up domains
- GovernanceAgent consumes recommendation type + risk + disclosure state and can:
  - allow
  - require approval
  - deny
- canonical deterministic approval proof path is:
  - Debt vs Invest
  - low emergency fund or high debt pressure
  - aggressive `invest_more`
  - GovernanceAgent -> `waiting_approval`

## Runtime Transition Mapping

| Trigger | Runtime result |
| --- | --- |
| grounding validator fail | `failed` |
| numeric validator fail | `failed` |
| business rule validator fail | `failed` |
| governance `RequireApproval` | `waiting_approval` |
| governance `Deny` | `failed(governance_denied)` |
| operator approve after `waiting_approval` | resume continuation through existing runtime resume semantics |
| operator deny after `waiting_approval` | `failed` |

## Run Evidence

- Monthly Review positive path:
  - `scripts/run_monthly_review_5d.sh`
  - `docs/eval/samples/monthly_review_5d_report.json`
  - `docs/eval/samples/monthly_review_5d_trace.json`
- Debt vs Invest deterministic approval path:
  - `docs/eval/samples/debt_vs_invest_5d_waiting_approval.json`
  - `docs/eval/samples/debt_vs_invest_5d_waiting_approval_trace.json`

## Explicitly Deferred Beyond Phase 5D

- broader workflow hardening beyond the current live path
- full replay/eval-plane maturation
- skill runtime / behavior domain expansion
- async runtime promotion
- remote finance APIs, market simulation, tax-law engines, or full optimization solvers
