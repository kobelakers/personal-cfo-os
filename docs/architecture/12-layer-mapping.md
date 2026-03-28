# 12-Layer Mapping

| Layer | Package(s) | Current Phase 2 Status |
| --- | --- | --- |
| Goal Layer | `internal/taskspec` | Concrete `TaskSpec` intake path is live via deterministic intake; `TaskIntakeResult` is already the formal entry gate for Monthly Review and Debt vs Invest. |
| Observation Layer | `internal/observation` | Concrete ledger/document adapters are live and emit typed `EvidenceRecord`; structured parsing is real, agentic parsing is still a stub behind the same adapter interface. |
| State Layer | `internal/state`, `internal/reducers` | Concrete deterministic reducers build `EvidencePatch` and apply snapshot/diff/versioned state updates into `FinancialWorldState`. |
| Context Engineering Layer | `internal/context` | Concrete assembler/selection/budget/compactor implementation is live; planning / execution / verification views now have real differences and budget enforcement. |
| Memory Layer | `internal/memory` | Concrete writer, hybrid retrieval, semantic fake backend, derived memory writer, conflict/supersedes handling, and access audit are live. |
| Planning / Policy Layer | `internal/planning` | Deterministic planning is live for Monthly Review and Debt vs Invest; workflows use explicit `plan -> act -> verify` semantics instead of prompt-only loops. |
| Skills + Tools Layer | `internal/skills`, `internal/tools` | Core tools and skills are live for Monthly Review and Debt vs Invest MVP; deterministic calculations stay outside LLM paths. |
| Runtime / Harness Layer | `internal/runtime` | Concrete local Temporal-aligned runtime is live with `LocalWorkflowRuntime`, `DefaultWorkflowController`, checkpoint store, journal, timeline, pause/resume, retry, and replan transitions. |
| Protocol Layer | `internal/protocol` | Contracts are still primarily schema-first in Phase 2; envelope/event types are stable, but strong actor execution boundaries are deferred to Phase 3. |
| Verification Layer | `internal/verification` | Concrete verification pipeline is live with coverage, deterministic, business, success-criteria, and oracle stages plus structured replan diagnostics. |
| Safety / Governance / HITL Layer | `internal/governance` | Concrete approval service, risk classifier, disclosure evaluation, memory write gate, policy engine, and audit event generation are live. |
| Observability / Evaluation / Feedback Layer | `internal/observability`, `cmd/eval`, `cmd/replay` | Structured event log, checkpoint log, timeline dump, replay bundle, and trace dump are live; full infra-backed tracing/metrics remains deferred. |

## Trade-Offs

- Current code is intentionally stronger on system layers than on explicit multi-agent actor execution.
- The repository should be described as a **systemized workflow engine with agent-ready substrate** in Phase 2, not as a fully realized strong multi-agent execution system.
- Temporal, Postgres, pgvector, MinIO, provider adapters, and full tracing infrastructure are still deferred behind already-fixed interfaces.
- The UI remains intentionally minimal so engineering effort stays concentrated on evidence, state, memory, verification, governance, and runtime.
