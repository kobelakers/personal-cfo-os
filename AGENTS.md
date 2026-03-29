# Codex Persistent Memory

## Stable Preferences
- Use Chinese for collaboration with the user.
- Persist only stable, reusable project and interview information in this file.

## Stable Project Context
- This repository is for the Personal CFO OS project.
- Personal CFO OS should be positioned as a 2026-style personal finance agent system, not a toy multi-agent demo.
- The system narrative should prioritize goal-driven execution, typed evidence, state-first design, structured memory, protocol-oriented coordination, runtime durability, governance, verification, and observability.
- For this project, preserving architectural layers is more important than reducing complexity for convenience.
- Current repo positioning has advanced to: `system-agent backbone + first real domain-agent execution path + first proactive life-event loop + first capability-backed follow-up execution`.
- `tax_optimization` and `portfolio_rebalance` are now the first capability-backed follow-up intents; runtime can activate and execute them as first-level child workflows while keeping recursive auto-execution disabled.
- Phase 2 has already crossed from contract scaffold into a real executable path: raw ledger/document inputs can now flow through typed evidence, deterministic state update, structured memory write/read, Monthly Review workflow execution, verification, governance, and local durable runtime semantics.
- Current acceptable stubs for project narrative: agentic document parsing is still a deterministic stub behind a formal adapter boundary, semantic retrieval still uses a fake backend behind embedding/vector interfaces, and runtime is local Temporal-aligned rather than backed by a live Temporal cluster.
- Project narrative boundary update: after Phase 3A, the repo should be described as a partial system-agent execution architecture on top of a governed workflow backbone, not merely a systemized workflow engine with agent-ready substrate.
- Current executed system-agent backbone: `PlannerAgent`, `MemorySteward`, `ReportAgent`, `VerificationAgent`, and `GovernanceAgent` now participate in the Monthly Review and Debt vs Invest main path through typed protocol envelopes and a workflow-facing `SystemStepBus`.
- Report generation boundary update: report production now follows `draft -> verification -> governance -> finalize`; final artifacts and `report_ready` are only emitted after governance allows or redacts the result.
- Current narrative guardrail: the project is still not a fully realized strong multi-agent finance OS; domain agents remain intentionally outside the main execution backbone until a later phase.
- Structural remediation priority update: workflow files must stay orchestration-only, and growing logic should sink into context, memory, verification, governance, runtime, and observability subsystems instead of remaining in workflow code.
- Phase 3B narrative update: the repo should now be described as `system-agent backbone + first real domain-agent execution path`, not only `partial system-agent execution architecture`.
- First load-bearing domain agents: `CashflowAgent` and `DebtAgent` now execute real analysis blocks in the main path, and their typed block results are aggregated by `ReportAgent`.
- Planning boundary update: `PlannerAgent` now returns a block-level `ExecutionPlan`, and `plan.Blocks` is the only execution truth source for downstream dispatch, reporting, and verification.
- Context/memory load-bearing update: retrieved memories, execution context, and verification context now materially affect block ordering, block emphasis, verification diagnostics, and replay explainability.
- Report boundary update: `ReportAgent` is now an aggregator/finalizer and must not regenerate missing cashflow/debt core analysis from raw state/evidence.
- Phase 4A narrative update: the repo has now entered `system-agent backbone + first real domain-agent execution path + first proactive life-event loop`.
- Workflow C boundary update: `life_event_trigger` is now a real workflow whose primary outputs are state diff, memory updates, generated task graph, and runtime-registered follow-up tasks; `LifeEventAssessmentReport` is only a secondary artifact.
- Goal/runtime contract update: `TaskSpec` remains the only executable goal contract; generated follow-up tasks are `TaskSpec` plus metadata, and capability-gated intents are explicitly registered as `queued_pending_capability` with `required_capability` and `missing_capability_reason`.
- Phase 5A narrative update: the repo has now entered `system-agent backbone + first real domain-agent execution path + first proactive life-event loop + first capability-backed follow-up execution + first operator-runnable durable runtime plane`.
- Runtime durability boundary update: task graphs, execution records, checkpoints, resume tokens, approvals, replay events, committed state snapshot refs, and artifact metadata refs now have a local SQLite durable seam; operator-facing replay must read durable `ReplayStore`, not in-memory helper timelines.
- Phase 5A scope guard: this phase is about durable runtime and operator control, not semantic retrieval hardening, deterministic finance engine hardening, or deeper business-rule validator expansion.
- Phase 5B narrative update: the repo now has a `real-intelligence-backed Monthly Review golden path`; `PlannerAgent` and `CashflowAgent` are the only agents on the real provider-backed path in this phase.
- Phase 5B architecture boundary update: prompt system, provider/model layer, structured output pipeline, and token-aware context budget are now load-bearing sublayers; workflows still stay thin and deterministic finance truth remains in code.
- Phase 5B scope guard: durable memory/embedding, finance hardening, deeper validator expansion, and broader domain-model rollout remain deferred; this phase only upgrades Monthly Review intelligence substrate.
- Phase 5B closure boundary update: repair prompt identity must stay visible in trace, live provider model selection must be config-only, and cashflow narrative grounding only adds narrow numeric-consistency guards for key deterministic metrics; full finance hardening still belongs to 5D.
