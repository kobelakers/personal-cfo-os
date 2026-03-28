# 12-Layer Mapping

| Layer | Package(s) | Current Phase 2 Status |
| --- | --- | --- |
| Goal Layer | `internal/taskspec` | Concrete `TaskSpec` intake path is live via deterministic intake; `TaskIntakeResult` is already the formal entry gate for Monthly Review and Debt vs Invest. |
| Observation Layer | `internal/observation` | Concrete ledger/document adapters are live and emit typed `EvidenceRecord`; structured parsing is real, agentic parsing is still a stub behind the same adapter interface. |
| State Layer | `internal/state`, `internal/reducers` | Concrete deterministic reducers build `EvidencePatch` and apply snapshot/diff/versioned state updates into `FinancialWorldState`. |
| Context Engineering Layer | `internal/context` | Concrete assembler/selection/budget/compactor implementation is live; planning / execution / verification views now have real differences and budget enforcement. |
| Memory Layer | `internal/memory` | Concrete writer, hybrid retrieval, semantic fake backend, derived memory writer, conflict/supersedes handling, and access audit are live. |
| Planning / Policy Layer | `internal/planning`, `internal/agents` | Deterministic planning is live and is now executed through `PlannerAgent` rather than direct workflow-local planner calls. |
| Skills + Tools Layer | `internal/skills`, `internal/tools` | Core tools and skills are live for Monthly Review and Debt vs Invest MVP; deterministic calculations stay outside LLM paths. |
| Runtime / Harness Layer | `internal/runtime` | Concrete local Temporal-aligned runtime is live with `LocalWorkflowRuntime`, `DefaultWorkflowController`, checkpoint store, journal, timeline, pause/resume, retry, replan transitions, and typed agent-failure bridging. |
| Protocol Layer | `internal/protocol`, `internal/agents` | Protocol is now execution-first: typed request/result kinds, oneof bodies, response envelopes, correlation/causation propagation, and real dispatch through `SystemStepBus` and the local agent dispatcher. |
| Verification Layer | `internal/verification`, `internal/agents` | Concrete verification pipeline is live and now executes through `VerificationAgent`, returning structured diagnostics consumed by workflow/runtime replan logic. |
| Safety / Governance / HITL Layer | `internal/governance`, `internal/agents` | Concrete approval service, risk classifier, disclosure evaluation, memory write gate, policy engine, and audit event generation are live; `GovernanceAgent` now executes approval/disclosure decisions in the main path. |
| Observability / Evaluation / Feedback Layer | `internal/observability`, `internal/runtime`, `internal/agents`, `cmd/eval`, `cmd/replay` | Structured event log, checkpoint log, timeline dump, replay bundle, trace dump, and agent dispatch lifecycle records are live; infra-backed tracing/metrics remains deferred. |

## Trade-Offs

- Current code is still intentionally stronger on system layers than on domain-agent actor execution.
- The repository should now be described as a **partial system-agent execution architecture on top of a governed workflow backbone**, not as a fully realized strong multi-agent execution system.
- `PlannerAgent / MemorySteward / ReportAgent / VerificationAgent / GovernanceAgent` are now real execution boundaries, but domain agents are still deferred.
- Temporal, Postgres, pgvector, MinIO, provider adapters, and full tracing infrastructure are still deferred behind already-fixed interfaces.
- The UI remains intentionally minimal so engineering effort stays concentrated on evidence, state, memory, verification, governance, and runtime.
