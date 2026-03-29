# 12-Layer Mapping

| Layer | Package(s) | Current Phase 3B Status |
| --- | --- | --- |
| Goal Layer | `internal/taskspec` | Concrete `TaskSpec` intake path is live via deterministic intake; `TaskIntakeResult` is already the formal entry gate for Monthly Review and Debt vs Invest. |
| Observation Layer | `internal/observation` | Concrete ledger/document adapters are live and emit typed `EvidenceRecord`; structured parsing is real, agentic parsing is still a stub behind the same adapter interface. |
| State Layer | `internal/state`, `internal/reducers` | Concrete deterministic reducers build `EvidencePatch` and apply snapshot/diff/versioned state updates into `FinancialWorldState`. |
| Context Engineering Layer | `internal/context` | Concrete assembler/selection/budget/compactor implementation is live; planning / execution / verification views now differ at block level, and execution/verification contexts are part of main-path dispatch and validation inputs. |
| Memory Layer | `internal/memory` | Concrete writer, hybrid retrieval, semantic fake backend, derived memory writer, conflict/supersedes handling, and access audit are live; retrieved memories now influence block ordering and block output emphasis. |
| Planning / Policy Layer | `internal/planning`, `internal/agents` | Deterministic planning is live through `PlannerAgent`, and its output is now a block-level `ExecutionPlan`; `plan.Blocks` is the only execution truth source for downstream dispatch, reporting, and verification. |
| Skills + Tools Layer | `internal/skills`, `internal/tools`, `internal/analysis` | Core deterministic tools are live, and `CashflowAgent` / `DebtAgent` now use them to produce typed `CashflowBlockResult` / `DebtBlockResult` instead of pushing cashflow/debt analysis back into workflow or report generation. |
| Runtime / Harness Layer | `internal/runtime` | Concrete local Temporal-aligned runtime is live with `LocalWorkflowRuntime`, `DefaultWorkflowController`, checkpoint store, journal, timeline, pause/resume, retry, replan transitions, and typed agent-failure bridging. |
| Protocol Layer | `internal/protocol`, `internal/agents` | Protocol is now execution-first: typed request/result kinds, oneof bodies, response envelopes, correlation/causation propagation, block analysis request/result payloads, and real dispatch through `SystemStepBus` and the local agent dispatcher. |
| Verification Layer | `internal/verification`, `internal/agents` | Concrete verification pipeline is live and now executes through `VerificationAgent`; it validates domain blocks before final report validation and short-circuits on severe block failures with structured replan diagnostics. |
| Safety / Governance / HITL Layer | `internal/governance`, `internal/agents` | Concrete approval service, risk classifier, disclosure evaluation, memory write gate, policy engine, and audit event generation are live; `GovernanceAgent` now executes approval/disclosure decisions in the main path. |
| Observability / Evaluation / Feedback Layer | `internal/observability`, `internal/runtime`, `internal/agents`, `cmd/eval`, `cmd/replay` | Structured event log, checkpoint log, timeline dump, replay bundle, trace dump, and agent dispatch lifecycle records are live; agent traces now expose `plan_id`, block order, selected memory/evidence/state slices, and block result summaries. |

## Trade-Offs

- Current code is still intentionally stronger on system layers than on full domain coverage.
- The repository should now be described as **system-agent backbone + first real domain-agent execution path**, not as a fully realized strong multi-agent execution system.
- `PlannerAgent / MemorySteward / ReportAgent / VerificationAgent / GovernanceAgent` are real execution boundaries, and `CashflowAgent / DebtAgent` are now the first load-bearing domain-agent boundaries.
- `ReportAgent` is now an aggregator/finalizer rather than the primary cashflow/debt analyst.
- Only cashflow and debt domains are agentized so far; portfolio / tax / behavior remain residual deterministic sections.
- Temporal, Postgres, pgvector, MinIO, provider adapters, and full tracing infrastructure are still deferred behind already-fixed interfaces.
- The UI remains intentionally minimal so engineering effort stays concentrated on evidence, state, memory, verification, governance, and runtime.
