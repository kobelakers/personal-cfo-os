# Phase 2 Evaluation Plan

Phase 2 validates the first real system path, not just contracts in isolation.

## Covered Checks

- raw ledger/document inputs can be normalized into typed evidence
- evidence can deterministically update `FinancialWorldState`
- memory writes enforce provenance and confidence rules
- hybrid retrieval returns ranked memories with audit logs
- Monthly Review happy path completes
- Monthly Review missing evidence path enters `replanning`
- Monthly Review high-risk path enters `waiting_approval`
- governance can deny invalid memory writes
- Debt vs Invest MVP produces an evidence-backed conclusion and passes through business validator, risk classifier, and approval decision
- local runtime can checkpoint and resume

## Important Negative Paths

- missing mandatory evidence
- low-confidence memory write
- omitted tax signal in report
- invalid resume token
- approval-required outcome after verification succeeds

## Known Stubs And Why They Are Acceptable

- semantic retrieval backend is fake but only reachable through embedding/vector/search interfaces
- agentic parsing is still deterministic, but already isolated behind its adapter boundary
- local runtime is Temporal-aligned but not backed by a live Temporal service

These are acceptable in Phase 2 because the execution chain, state semantics, governance boundaries, and verification surfaces are already real.
