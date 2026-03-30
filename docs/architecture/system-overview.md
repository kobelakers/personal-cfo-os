# System Overview

Personal CFO OS is a long-running personal finance agent system designed around typed evidence, state-first reasoning, structured memory, explicit runtime semantics, protocol contracts, governance, verification, replayable observability, and now a first proactive life-event loop with first capability-backed follow-up execution, a promoted async-capable durable runtime backbone, a first real-intelligence-backed Monthly Review golden path, a first real memory substrate, a trustworthy finance reasoning substrate on the current live path, a first operator-grade replay/eval/debug plane, a first versioned skill runtime, and a first formal behavior domain.

## Core Loop

1. Natural language first enters deterministic task intake and becomes a `TaskSpec`.
2. Ledger and document adapters ingest raw inputs and emit typed `EvidenceRecord` values.
3. Deterministic reducers convert evidence into state patches and update `FinancialWorldState`.
4. Workflow services keep observation/reducer orchestration thin and hand execution to a workflow-facing `SystemStepBus`.
5. `MemorySteward` derives and retrieves memories before planning; retrieved memories now come from a durable memory plane and influence downstream block ordering, planner rationale, and recommendation emphasis.
6. `PlannerAgent` assembles planning context, renders a versioned prompt with an applied render policy, performs provider-backed structured generation, validates/repairs/fallbacks the output, and still returns the same block-level `ExecutionPlan`; `plan.Blocks` remains the only execution truth source.
7. Workflow iterates `plan.Blocks`, assembles block-specific execution context, and dispatches `CashflowAgent`, `DebtAgent`, or `BehaviorAgent`; behavior dispatch now includes orchestrator-selected skill family/version/recipe metadata instead of letting the domain agent improvise a recipe.
8. `ReportAgent` aggregates typed domain block results into a draft, then later finalizes only after verification and governance allow or redact.
9. `VerificationAgent` runs block-level validation first and now also executes grounding, numeric, and business-rule validators against the final report surface on the live path.
10. `GovernanceAgent` evaluates typed recommendation/risk/disclosure state before finalize and can require approval or deny publication.
11. Runtime semantics manage checkpoints, pause/resume, approval gates, retries, protocol failures, recovery, follow-up task graphs, capability activation, child workflow execution records, and committed state handoff.
12. Durable runtime stores persist task graphs, execution records, checkpoints, approvals, replay events, and artifact refs across process restarts through a local SQLite seam.
13. Operator-facing service / API / worker layers query and control the runtime without pushing orchestration back into workflow files.
14. Runtime durable truth is now augmented by versioned replay/debug projection rows plus artifact refs, so replay/debug can query the same durable plane without introducing a second explanation database.
15. `internal/runtime.ReplayQueryService` is now the canonical replay plane; it answers workflow/task-graph/task/execution/approval why/how questions from authoritative runtime truth, normalized projections, and artifact refs, while degrading gracefully when projections are missing or stale instead of falling back to a second replay system.
16. `cmd/eval` now runs a deterministic canonical 11-scenario regression corpus over the current backbone instead of only phase-specific runners, and golden replay/debug samples are produced from the same mock/runtime fixtures, including explicit memory-rejection visibility coverage.
17. `behavior_intervention` now enters through deterministic intake, planner emits a skill-aware behavior block, orchestrator-side selection resolves concrete family/version/recipe, `BehaviorAgent` executes deterministic behavior analysis, and procedural memory can alter the next similar skill choice.
18. runtime promotion now adds durable work items, lease/heartbeat/reclaim, fencing-token-protected commits, scheduler-generated wakeups, Postgres-backed runtime-authoritative stores, and ref-backed checkpoint/report/replay payload storage without changing the canonical replay plane or pushing scheduling logic back into workflows.

## Replay / Eval / Debug Plane (Phase 6A)

Phase 6A upgrades replay/eval/debug from durable trace export into a local operator-grade plane with these boundaries:

1. durable truth source is now:
   - runtime durable truth
   - replay/debug projection rows
   - artifact refs for rich bundles and golden outputs
2. projection rows are normalized and versioned rather than artifacts-only; query does not require deserializing a single large JSON blob to answer replay questions
3. projection rows are rebuildable/backfillable from authoritative runtime truth through a dedicated projection rebuilder
4. query semantics are explicit:
   - authoritative runtime truth missing -> hard failure
   - projection missing/stale/incomplete -> partial replay view with degradation reasons
5. provenance is now treated as a directed graph instead of an ID bag, so replay can explain parent workflow -> generated task -> child workflow -> artifact -> state commit -> operator action chains
6. the canonical 6A regression corpus is deterministic/mock only; live provider paths are retained for smoke/manual evidence, not stable regression

## Skills System + Behavior Domain (Phase 6B)

Phase 6B promotes one narrow but load-bearing capability/domain path rather than spreading behavior logic everywhere:

1. `behavior_intervention` is now a real deterministic intake path and workflow intent
2. `internal/skills` now provides canonical manifests, family/version/recipe metadata, policy, typed selection reasons, and runtime execution records
3. `internal/behavior` is now a formal domain with deterministic metrics, anomaly detection, grounded recommendations, and behavior-specific validation
4. `BehaviorBlockResult` now enters the same `analysis.BlockResultEnvelope` main chain as other domains, so behavior becomes verification/governance/reporting/replay input rather than a report appendix
5. procedural memory extends the existing durable memory substrate and can deterministically influence later skill/recipe selection
6. the canonical high-risk proof is `discretionary_guardrail / hard_cap.v1`, which escalates into `waiting_approval` without performing any external account action

## Runtime Promotion (Phase 7A)

Phase 7A upgrades the runtime backbone rather than widening the system into UI or externalized protocol work:

1. durable work items now drive async execution instead of a pure in-process worker pass
2. workers claim work through leases, heartbeat lease ownership, and can lose the lease through expiry/reclaim
3. stale workers are fenced: completion/checkpoint/transition commits now validate lease ownership plus fencing token, so reclaimed workers cannot successfully apply final state
4. scheduler and reevaluator now enqueue typed work for deferred wakeups, approval resumes, dependency/capability reevaluations, and retry backoff
5. runtime-authoritative stores now have two formal profiles:
   - `local-lite`: SQLite + LocalFS
   - `runtime-promotion`: Postgres + MinIO-compatible blob storage
6. replay/debug still uses the same canonical `internal/runtime.ReplayQueryService`, but now it can explain claim/lease/heartbeat/reclaim/retry/scheduler chains across workers

## Trustworthy Finance Reasoning (Phase 5D)

Phase 5D hardens the current live path without widening the system into a larger workflow or infra expansion:

1. `internal/finance` is now the formal numeric truth source for Monthly Review and Debt vs Invest, with minimal deterministic bundles for Tax / Portfolio validator hooks.
2. recommendations are carried as shared typed objects with risk, grounding refs, caveats, approval fields, and policy rule refs instead of only narrative prose.
3. verification now includes three deterministic validator layers:
   - grounding validator
   - numeric consistency validator
   - business-rule validator
4. governance consumes the same typed recommendation contract and maps outcomes to runtime transitions:
   - grounding/numeric/business failure -> `failed`
   - `RequireApproval` -> `waiting_approval`
   - `Deny` -> `failed(governance_denied)`
5. the canonical approval proof is deterministic and fixture-driven on `Debt vs Invest`, where low emergency fund or high debt pressure plus aggressive `invest_more` escalates into `waiting_approval`.

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

The repository is now best described as **system-agent backbone + first real domain-agent execution path + first proactive life-event loop + first capability-backed follow-up execution + promoted async-capable durable runtime backbone + real-intelligence-backed Monthly Review golden path + first real memory substrate + trustworthy finance reasoning substrate + first operator-grade replay/eval/debug plane + first versioned skill runtime + first formal behavior domain + procedural-memory-influenced skill selection**.

- It is stronger than a workflow engine that merely has “agent interfaces on paper”.
- It is weaker than a fully actorized, durable, remote-executable strong multi-agent system.
- This is intentional: system-agent boundaries are real, the first two load-bearing finance domain agents are live in A/B workflows, `BehaviorAgent` is now live only in the narrow `behavior_intervention` workflow, and Tax/Portfolio expansion remains limited to Workflow C so scope stays controlled.

## Current Stubs

- agentic document parsing is still a deterministic stub behind a formal observation adapter
- durable memory now exists for Monthly Review through a local SQLite memory seam, but it is not yet a stronger remote memory substrate
- semantic retrieval is now real for the Monthly Review path through provider-backed embeddings, but still uses local brute-force vector scoring instead of ANN/pgvector
- runtime is still Temporal-aligned rather than connected to a live Temporal cluster
- stronger persistence now exists through `runtime-promotion` with Postgres + MinIO-compatible blob refs, but the system is still local-first rather than a fully externalized production deployment
- observability is now durable and queryable for runtime replay/debug, but not yet backed by full tracing infrastructure
- system-agent execution is still local synchronous dispatch; 7A promotes the runtime around those handlers rather than turning agents into remote inbox/outbox actors
- only `PlannerAgent` and `CashflowAgent` currently use real provider-backed reasoning, and only inside Monthly Review
- prompt/provider/token/cost traces are now visible in workflow dumps, but they are not yet promoted to a separate operator-facing durable intelligence store
- `TaxAgent` and `PortfolioAgent` are only live in Workflow C; behavior-domain execution is now live only through `behavior_intervention`
- follow-up execution is now capability-backed for `tax_optimization` and `portfolio_rebalance`, but only at execution depth 1 and only through runtime allowlist policy
- other generated intents can still remain `ready`, `dependency_blocked`, `deferred`, or `queued_pending_capability` without being auto-run
- broader finance-engine expansion, deeper rule coverage, stronger memory infra promotion, and richer blocked/deferred/capability regression coverage remain explicitly out of scope for this phase
- richer provider/prompt A/B evaluation and full external observability infra promotion remain explicitly out of scope for this phase

The system is still intentionally local-first. Postgres and MinIO are now real runtime-promotion backends, but live Temporal, pgvector-backed memory infra, remote agentization, and full observability infrastructure are still deferred behind already-fixed interfaces. That keeps the direction aligned with a 2026 agent system instead of collapsing into a Phase 2 demo.
