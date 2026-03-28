# Phase 2 Evaluation Plan

Phase 2 validates the first real system path and the structural remediation that moved logic out of workflow-heavy files.

## Covered Checks

- raw ledger/document inputs can be normalized into typed evidence
- evidence can deterministically update `FinancialWorldState`
- memory writes enforce provenance and confidence rules
- hybrid retrieval returns ranked memories with audit logs
- context views differ by planning / execution / verification and obey budgets
- compaction remains state-aware rather than string truncation
- Monthly Review happy path completes
- Monthly Review missing evidence path enters `replanning`
- Monthly Review high-risk path enters `waiting_approval`
- governance can deny invalid memory writes
- approval service evaluates both action approval and report disclosure
- artifact service produces structured workflow artifacts
- structured observability dump and replay bundle can be built from timeline, checkpoint, memory, and policy records
- Debt vs Invest MVP produces an evidence-backed conclusion and passes through business validator, risk classifier, and approval decision
- local runtime can checkpoint and resume

## Important Negative Paths

- missing mandatory evidence
- low-confidence memory write
- omitted tax signal in report
- invalid resume token
- approval-required outcome after verification succeeds
- verification diagnostics must carry failed rules, missing evidence, and recommended replan action

## Known Stubs And Why They Are Acceptable

- semantic retrieval backend is fake but only reachable through embedding/vector/search interfaces
- agentic parsing is still deterministic, but already isolated behind its adapter boundary
- local runtime is Temporal-aligned but not backed by a live Temporal service
- the system is still workflow-engine-first rather than strong actor-execution-first, but the current substrate is intentionally ready for stronger system-agent boundaries in Phase 3

These are acceptable in Phase 2 because the execution chain, state semantics, governance boundaries, verification surfaces, and subsystem boundaries are already real.
