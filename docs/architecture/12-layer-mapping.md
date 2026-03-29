# 12-Layer Mapping

| Layer | Package(s) | Current Phase 4A Status |
| --- | --- | --- |
| Goal Layer | `internal/taskspec` | `TaskSpec` remains the only executable goal contract. Life events now enter through `life_event_trigger`, and generated downstream tasks are represented as `TaskSpec + generation metadata` rather than a second incompatible goal type. |
| Observation Layer | `internal/observation`, `internal/tools` | Ledger/document adapters remain live, and Phase 4A adds concrete `event source` and `calendar/deadline source` adapters. Workflow C no longer consumes raw event text; it consumes typed event/deadline evidence with provenance, confidence, source, and time range. |
| State Layer | `internal/state`, `internal/reducers` | Reducers still build deterministic `EvidencePatch` and apply snapshot/diff/versioned updates. Workflow C now uses the same reducer path to turn event evidence into a state diff instead of inventing a side channel for life events. |
| Context Engineering Layer | `internal/context` | Planning / execution / verification context is already live for block execution, and Workflow C extends the same pattern to event-driven block dispatch, task-generation inputs, and verification pass 2. |
| Memory Layer | `internal/memory` | Memory remains structured and governed. Workflow C reuses the current schema with minimal event-oriented patterns, and retrieved memories now influence life-event planning, task generation rationale, and verification diagnostics. |
| Planning / Policy Layer | `internal/planning`, `internal/agents`, `internal/taskspec` | `PlannerAgent` now plans both passive analysis workflows and proactive life-event workflows. `plan.Blocks` remains the only execution truth source, and `TaskGenerationAgent` adds a second planning surface for downstream follow-up tasks without becoming a coordinator. |
| Skills + Tools Layer | `internal/tools`, `internal/analysis`, `internal/agents` | Deterministic tools remain the execution substrate. Phase 4A adds `TaxAgent` and `PortfolioAgent` for Workflow C, plus event/deadline query tools and typed block result schemas for tax/portfolio event impact. |
| Runtime / Harness Layer | `internal/runtime` | Local Temporal-aligned runtime now manages generated follow-up task graphs, spawned/deferred task records, approval-gated follow-up tasks, and capability-gated queued tasks. Generated tasks are registered and replayable, but not recursively executed in Phase 4A. |
| Protocol Layer | `internal/protocol`, `internal/agents` | Protocol remains execution-first and is only extended, not replaced. New typed request/result kinds cover `tax_analysis`, `portfolio_analysis`, `task_generation`, and the Workflow C assessment artifact path while reusing the existing bus/backbone. |
| Verification Layer | `internal/verification`, `internal/agents` | Verification still covers block + final validation, and Workflow C adds a second verification pass for generated tasks and final life-event assessment. Task generation is now grounded, deduplicated, and verified instead of treated as an unvalidated side effect. |
| Safety / Governance / HITL Layer | `internal/governance`, `internal/agents` | Governance continues to control approval/disclosure, and now also evaluates spawned-task policy, approval propagation, and unsafe generated tasks. Runtime registration only happens after this governed pass. |
| Observability / Evaluation / Feedback Layer | `internal/observability`, `internal/runtime`, `internal/workflows` | Replay/trace already exposed block plan and agent lifecycle; Phase 4A extends the same plane to event ingestion, state diff logging, generated task graph registration, follow-up task capability gaps, and secondary life-event assessment artifacts. |

## Trade-Offs

- The repository should now be described as **system-agent backbone + first real domain-agent execution path + first proactive life-event loop**.
- `CashflowAgent` and `DebtAgent` remain the load-bearing domain agents for Monthly Review and Debt vs Invest.
- `TaxAgent` and `PortfolioAgent` are now real execution boundaries, but only inside Workflow C for controlled scope.
- `TaskGenerationAgent` generates typed downstream tasks, but those tasks are intentionally registered first and not fully executed in Phase 4A.
- `LifeEventAssessmentReport` exists as a secondary artifact contract so Workflow C stays protocol-complete without turning the workflow back into a report-first system.
- Behavior-domain execution, real provider integration, real Temporal/Postgres/pgvector/MinIO, and full tracing infrastructure remain deferred behind stable interfaces.
