# 12-Layer Mapping

| Layer | Package(s) | Current Phase 4B Status |
| --- | --- | --- |
| Goal Layer | `internal/taskspec` | `TaskSpec` remains the only executable goal contract. Life events enter through `life_event_trigger`, generated downstream tasks stay `TaskSpec + generation metadata`, and follow-up child workflows still consume standard `TaskSpec` instead of a second goal type. |
| Observation Layer | `internal/observation`, `internal/tools` | Ledger/document adapters remain live, and Phase 4A adds concrete `event source` and `calendar/deadline source` adapters. Workflow C no longer consumes raw event text; it consumes typed event/deadline evidence with provenance, confidence, source, and time range. |
| State Layer | `internal/state`, `internal/reducers` | Reducers still build deterministic `EvidencePatch` and apply snapshot/diff/versioned updates. Workflow C now uses the same reducer path to turn event evidence into a state diff instead of inventing a side channel for life events. |
| Context Engineering Layer | `internal/context` | Planning / execution / verification context is already live for block execution, and Workflow C extends the same pattern to event-driven block dispatch, task-generation inputs, and verification pass 2. |
| Memory Layer | `internal/memory` | Memory remains structured and governed. Workflow C reuses the current schema with minimal event-oriented patterns, and retrieved memories now influence life-event planning, task generation rationale, and verification diagnostics. |
| Planning / Policy Layer | `internal/planning`, `internal/agents`, `internal/taskspec` | `PlannerAgent` now plans passive workflows, Workflow C, and the new `tax_optimization` / `portfolio_rebalance` follow-up workflows. `plan.Blocks` remains the only execution truth source, and `TaskGenerationAgent` still generates downstream tasks without becoming a coordinator or executor. |
| Skills + Tools Layer | `internal/tools`, `internal/analysis`, `internal/agents` | Deterministic tools remain the execution substrate. `TaxAgent` and `PortfolioAgent` now serve both Workflow C event-impact blocks and the new follow-up optimization/rebalance blocks, while `ReportAgent` stays an aggregator/finalizer. |
| Runtime / Harness Layer | `internal/runtime` | Local Temporal-aligned runtime now manages follow-up task graphs, capability activation, task state advancement, child workflow dispatch, resumable execution records, committed state handoff, retry metadata, and suppression/blocking reasons. Auto-run stays limited to allowlisted depth-1 follow-ups. |
| Protocol Layer | `internal/protocol`, `internal/agents` | Protocol remains execution-first and is only extended, not replaced. New typed request/result kinds cover `tax_analysis`, `portfolio_analysis`, `task_generation`, and the Workflow C assessment artifact path while reusing the existing bus/backbone. |
| Verification Layer | `internal/verification`, `internal/agents` | Verification still covers block + final validation, and now extends into follow-up child workflows with typed tax/portfolio validators, task success criteria, and replayable retry/failure paths. |
| Safety / Governance / HITL Layer | `internal/governance`, `internal/agents` | Governance continues to control approval/disclosure, and now also governs child workflow outputs. Follow-up execution records carry checkpoint/resume/approval anchors instead of collapsing waiting approval into a plain status string. |
| Observability / Evaluation / Feedback Layer | `internal/observability`, `internal/runtime`, `internal/workflows` | Replay/trace now spans parent Workflow C -> generated task -> child workflow -> child artifact -> child state commit, including capability activation, blocking/suppression reasons, retry attempts, and committed-state handoff. |

## Trade-Offs

- The repository should now be described as **system-agent backbone + first real domain-agent execution path + first proactive life-event loop + first capability-backed follow-up execution**.
- `CashflowAgent` and `DebtAgent` remain the load-bearing domain agents for Monthly Review and Debt vs Invest.
- `TaxAgent` and `PortfolioAgent` are now real execution boundaries, but only inside Workflow C for controlled scope.
- `TaskGenerationAgent` still only generates typed downstream tasks; runtime now executes a narrow allowlisted subset instead of turning task generation into recursive orchestration.
- `LifeEventAssessmentReport` remains a secondary artifact contract so Workflow C stays protocol-complete without turning the workflow back into a report-first system.
- `TaxOptimizationWorkflow` and `PortfolioRebalanceWorkflow` are the first two capability-backed child workflows, and the only auto-executable follow-up intents in the current phase.
- Behavior-domain execution, real provider integration, real Temporal/Postgres/pgvector/MinIO, and full tracing infrastructure remain deferred behind stable interfaces.
