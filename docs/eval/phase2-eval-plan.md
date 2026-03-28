# Phase 2 Evaluation Plan

This evaluation plan now covers the Phase 2 executable path plus the Phase 3A protocolization step that turned system-agent boundaries into real execution boundaries.

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
- report draft and finalize are separated; final artifact is not emitted before governance
- structured observability dump and replay bundle can be built from timeline, checkpoint, agent dispatch, memory, and policy records
- Debt vs Invest MVP produces an evidence-backed conclusion and passes through business validator, risk classifier, and approval decision
- local runtime can checkpoint and resume
- protocol oneof payload validation prevents ad hoc JSON-style agent messages
- typed agent failure categories map into runtime recovery semantics

## Important Negative Paths

- missing mandatory evidence
- low-confidence memory write
- omitted tax signal in report
- invalid resume token
- approval-required outcome after verification succeeds
- verification diagnostics must carry failed rules, missing evidence, and recommended replan action
- report disclosure redaction path
- unknown recipient / unsupported message kind
- bad payload / protocol failure path
- replay bundle must show agent dispatch lifecycle

## Known Stubs And Why They Are Acceptable

- semantic retrieval backend is fake but only reachable through embedding/vector/search interfaces
- agentic parsing is still deterministic, but already isolated behind its adapter boundary
- local runtime is Temporal-aligned but not backed by a live Temporal service
- system agents currently execute through a local synchronous dispatcher rather than async durable mailboxes
- domain agents are still not part of the execution backbone
- the system is still not a fully realized strong multi-agent finance OS, but it is no longer only an agent-ready substrate

These are acceptable because the execution chain, state semantics, protocol dispatch, governance boundaries, verification surfaces, and subsystem boundaries are already real.
