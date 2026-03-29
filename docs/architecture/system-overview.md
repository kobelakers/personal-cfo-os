# System Overview

Personal CFO OS is a long-running personal finance agent system designed around typed evidence, state-first reasoning, structured memory, explicit runtime semantics, protocol contracts, governance, verification, replayable observability, and now a first proactive life-event loop with first capability-backed follow-up execution, a first operator-runnable durable runtime plane, a first real-intelligence-backed Monthly Review golden path, and a first real memory substrate.

## Core Loop

1. Natural language first enters deterministic task intake and becomes a `TaskSpec`.
2. Ledger and document adapters ingest raw inputs and emit typed `EvidenceRecord` values.
3. Deterministic reducers convert evidence into state patches and update `FinancialWorldState`.
4. Workflow services keep observation/reducer orchestration thin and hand execution to a workflow-facing `SystemStepBus`.
5. `MemorySteward` derives and retrieves memories before planning; retrieved memories now come from a durable memory plane and influence downstream block ordering, planner rationale, and recommendation emphasis.
6. `PlannerAgent` assembles planning context, renders a versioned prompt with an applied render policy, performs provider-backed structured generation, validates/repairs/fallbacks the output, and still returns the same block-level `ExecutionPlan`; `plan.Blocks` remains the only execution truth source.
7. Workflow iterates `plan.Blocks`, assembles block-specific execution context, and dispatches `CashflowAgent` or `DebtAgent`; `CashflowAgent` now has a real provider-backed structured reasoning path, while `DebtAgent` stays deterministic.
8. `ReportAgent` aggregates typed domain block results into a draft, then later finalizes only after verification and governance allow or redact.
9. `VerificationAgent` runs block-level validation first, including structured-output/grounding checks for the new intelligence path, and may short-circuit final report validation with structured replan diagnostics.
10. `GovernanceAgent` evaluates approval and disclosure policy before finalize.
11. Runtime semantics manage checkpoints, pause/resume, approval gates, retries, protocol failures, recovery, follow-up task graphs, capability activation, child workflow execution records, and committed state handoff.
12. Durable runtime stores persist task graphs, execution records, checkpoints, approvals, replay events, and artifact refs across process restarts through a local SQLite seam.
13. Operator-facing service / API / worker layers query and control the runtime without pushing orchestration back into workflow files.
14. Observability and replay record workflow timeline, block plan, domain block execution order, selected context slices, prompt id/version, repair prompt identity, provider calls, token usage, estimated cost, structured-output repair/fallback, memory query/hit/reject/select traces, embedding calls, and operator/runtime provenance chains.

## Real Memory Substrate (Phase 5C)

Phase 5C promotes memory from a shaped interface to a true system layer:

1. `internal/memory` now has a durable SQLite seam for `MemoryRecord`, relations, embeddings, lexical terms, access audit, and write events.
2. memory write now follows `prepare -> atomic commit`, so records, relations, embeddings, lexical terms, access audit, and write events either persist together or roll back together.
3. memory durable plane is explicitly separate from runtime durable plane even though both are local SQLite today.
4. semantic retrieval is no longer fake on the Monthly Review path; it now uses provider-backed embeddings through a dedicated embedding seam.
5. retrieval is hybrid and typed:
   - lexical retrieval from durable token postings
   - semantic retrieval from persisted embeddings
   - reciprocal-rank fusion
   - configurable rejection policy with structured reasons
   - rejection happens after fusion/rerank, and final selected `topK` is chosen only from accepted candidates
6. retrieval query formation is now formalized with distinct planner and cashflow builders instead of workflow-local string assembly.
7. conflict and supersedence remain explicit and intentionally narrow:
   - same fact key + different value => conflict
   - same summary semantics + newer update => supersedes
8. Monthly Review can now be run across sessions against the same injected `memory.db`, and selected durable memories can change planning and reasoning output on the second run.

## Real Intelligence Substrate (Phase 5B)

Phase 5B does not merely sprinkle model calls into workflows. It adds a load-bearing cognition chain beneath the existing backbone:

1. `internal/context` now makes token-aware budget decisions for planning and cashflow execution instead of only block-count/character-count compaction.
2. `internal/prompt` owns versioned prompt templates, render policies, and render traces for `planner.monthly_review.v1` and `cashflow.monthly_review.v1`.
3. `internal/model` owns the provider-agnostic chat/structured seam, with one real OpenAI-compatible adapter plus stub seams for future providers.
4. `internal/structured` owns schema validation, parse retry, repair retry, deterministic fallback, and trace recording with distinct initial/repair generation identity.
5. `PlannerAgent` and `CashflowAgent` are the only two agents on the real provider-backed path in this phase.
6. Deterministic finance truth remains in code: state, reducers, and finance metrics still come from deterministic tools rather than model-invented numbers, and 5B closure now adds a narrow numeric-consistency guard so cashflow narrative text cannot freely invent key metrics.

This means Monthly Review can now show a full evidence chain from context selection -> prompt render -> provider call -> structured output -> verification -> report artifact without breaking workflow thinness or typed protocol boundaries. It does not mean that every workflow or every domain agent is already intelligence-promoted.

## Proactive Life Event Loop

Phase 4A adds the first proactive workflow rather than another passive analysis path, and Phase 4B closes the first follow-up execution loop.

1. Structured life-event input enters through `life_event_trigger` intake and becomes a standard `TaskSpec`.
2. `EventObservationAdapter` and `CalendarDeadlineObservationAdapter` normalize event/deadline inputs into typed evidence with provenance, confidence, source, and time range.
3. Supporting ledger/document evidence is collected, reducers update `FinancialWorldState`, and a state diff is produced.
4. `MemorySteward` writes event-driven memory and retrieves relevant prior decisions, tax signals, debt pressure, and procedural memories.
5. `PlannerAgent` returns a block-level life-event plan; `plan.Blocks` remains the only execution truth source.
6. Workflow dispatches event-specific domain blocks through:
   - `CashflowAgent`
   - `DebtAgent`
   - `TaxAgent`
   - `PortfolioAgent`
7. `VerificationAgent` pass 1 validates analysis blocks before task generation.
8. `TaskGenerationAgent` generates downstream `TaskSpec`-backed follow-up tasks from validated block results, event evidence, state diff, and retrieved memories.
9. `VerificationAgent` pass 2 validates generated tasks and the final life-event assessment.
10. `GovernanceAgent` evaluates disclosure and spawned-task policy / approval requirements.
11. Runtime registers follow-up tasks into a task graph with explicit statuses such as `waiting_approval`, `deferred`, or `queued_pending_capability`.
12. Runtime reevaluates the graph, activates capability-backed tasks, and executes allowlisted depth-1 child workflows through `TaxOptimizationWorkflow` and `PortfolioRebalanceWorkflow`.
13. The same task graph can later be reevaluated, resumed, retried, or operator-controlled outside the parent workflow through the runtime service and worker plane.
14. `ReportAgent` finalizes `LifeEventAssessmentReport` only as a secondary artifact after runtime registration/execution and governance.

## Real Data Path With System Agents

The current chain now looks like:

- raw ledger transactions / debt rows / holdings / payslip / tax text
- typed evidence generation and normalization
- evidence-driven state update
- memory sync dispatch through typed `memory_sync_request`
- planner dispatch through typed `plan_request`
- domain block dispatch through typed `cashflow_analysis_request` / `debt_analysis_request`
- report draft dispatch through typed `report_draft_request`
- verification dispatch through typed `verification_request`
- governance dispatch through typed `governance_evaluation_request`
- report finalize dispatch through typed `report_finalize_request`
- runtime state transition driven by structured verification/governance outcomes and typed agent failure categories

## Current Narrative Boundary

The repository is now best described as **system-agent backbone + first real domain-agent execution path + first proactive life-event loop + first capability-backed follow-up execution + first operator-runnable durable runtime plane + real-intelligence-backed Monthly Review golden path + first real memory substrate**.

- It is stronger than a workflow engine that merely has “agent interfaces on paper”.
- It is weaker than a fully actorized, durable, remote-executable strong multi-agent system.
- This is intentional: system-agent boundaries are real, the first two load-bearing domain agents are live in A/B workflows, and Tax/Portfolio expansion is currently limited to Workflow C so scope remains controlled.

## Current Stubs

- agentic document parsing is still a deterministic stub behind a formal observation adapter
- durable memory now exists for Monthly Review through a local SQLite memory seam, but it is not yet a stronger remote memory substrate
- semantic retrieval is now real for the Monthly Review path through provider-backed embeddings, but still uses local brute-force vector scoring instead of ANN/pgvector
- runtime is local Temporal-aligned rather than connected to a live Temporal cluster
- durable persistence is local SQLite + artifact refs rather than Postgres + object storage
- observability is now durable and queryable for runtime replay, but not yet backed by full tracing infrastructure
- system-agent execution is local synchronous dispatch, not yet async/durable inbox-outbox execution
- only `PlannerAgent` and `CashflowAgent` currently use real provider-backed reasoning, and only inside Monthly Review
- prompt/provider/token/cost traces are now visible in workflow dumps, but they are not yet promoted to a separate operator-facing durable intelligence store
- `TaxAgent` and `PortfolioAgent` are only live in Workflow C; behavior-domain execution is still deferred
- follow-up execution is now capability-backed for `tax_optimization` and `portfolio_rebalance`, but only at execution depth 1 and only through runtime allowlist policy
- other generated intents can still remain `ready`, `dependency_blocked`, `deferred`, or `queued_pending_capability` without being auto-run
- finance engine hardening, broader memory-native workflow rollout, stronger memory infra promotion, and full replay/eval-plane maturation remain explicitly out of scope for this phase

The system is still intentionally local-first. Real Postgres, pgvector, MinIO, Temporal, and model providers are deferred, but only behind already-fixed interfaces. That keeps the direction aligned with a 2026 agent system instead of collapsing into a Phase 2 demo.
