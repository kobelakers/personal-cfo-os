# Personal CFO OS

Personal CFO OS is a 2026-style personal finance agent system. It is intentionally designed as a goal-driven, stateful, memory-aware, protocol-oriented, verifiable, governed, and observable system rather than a toy "LLM routes to an agent and calls a few tools" demo.

## Why This Is Not a Toy Multi-Agent Demo

- User requests do not flow directly into execution. They must be normalized into a typed `TaskSpec` with explicit goal, constraints, risk, approval, required evidence, and success criteria.
- Observations are not kept as loose chat text. The system defines typed `EvidenceRecord` contracts and uses evidence-driven state updates.
- State is a first-class object. `FinancialWorldState` supports snapshots, diffs, versioning, and reducer-based updates.
- Memory is structured and governed. The schema captures provenance, confidence, supersedes, conflicts, and access audit instead of a raw JSON save/recall blob.
- Runtime semantics are explicit. Failure categories, checkpoint records, resume tokens, approval waiting, and recovery strategies are part of the design surface.
- Governance and verification are front-loaded. The system models approval policy, tool policy, memory write policy, disclosure policy, audit events, evidence coverage, and oracle verdicts.
- Protocols are explicit. Internal agent envelopes and workflow UI events include correlation and causation identifiers so the system can be replayed and traced.

## What Now Runs End-to-End

The repository now runs a real governed finance workflow backbone with system-agent execution, a first real domain-agent path, a first proactive life-event loop, a first capability-backed follow-up execution path, a first operator-runnable durable runtime plane, and a first real-intelligence-backed Monthly Review golden path:

1. raw ledger and document fixtures are ingested by observation adapters
2. adapters emit typed `EvidenceRecord` values
3. workflow services orchestrate evidence collection and deterministic reducers build `EvidencePatch`
4. `FinancialWorldState` is updated with versioning, snapshot, and diff semantics
5. `SystemStepBus` dispatches typed protocol envelopes to `MemorySteward`, `PlannerAgent`, `CashflowAgent`, `DebtAgent`, `ReportAgent`, `VerificationAgent`, and `GovernanceAgent`
6. `PlannerAgent` returns a block-level execution plan, and `plan.Blocks` becomes the only execution truth source for downstream dispatch
7. `CashflowAgent` and `DebtAgent` execute real deterministic analysis blocks using block-specific execution context and retrieved memories
8. `ReportAgent` aggregates domain block results into a draft and only finalizes artifacts after verification and governance
9. `VerificationAgent` now runs block-level validation before final report validation and can short-circuit into structured replan diagnostics
10. runtime consumes structured verification diagnostics and typed agent failure categories to decide `completed / replanning / waiting_approval / failed`
11. observability and replay outputs now include block plan, domain block execution order, selected memory/evidence/state slices, checkpoint timeline, and policy decisions
12. Workflow C now ingests structured life events and deadlines, updates state/memory, executes event-specific domain blocks, generates typed follow-up tasks, verifies and governs them, registers them into runtime as follow-up task graph records, then lets runtime activate and execute allowlisted first-level follow-up capabilities for `tax_optimization` and `portfolio_rebalance`
13. runtime state is now backed by a local durable SQLite seam for task graphs, execution records, checkpoints, resume tokens, approvals, operator actions, replay events, committed state snapshots, and artifact metadata refs
14. `cmd/api` and `cmd/worker` now provide a minimal runnable operator surface for approvals, resume, retry, reevaluate, and durable worker passes instead of placeholder binaries
15. `PlannerAgent` now has a real provider-backed structured planning path for Monthly Review, but still compiles back into the existing typed `planning.ExecutionPlan`
16. `CashflowAgent` now has a real provider-backed structured analysis path for Monthly Review, but deterministic finance metrics remain the source of truth
17. prompts are now versioned system objects under `internal/prompt`, structured output is validated/repaired/fallbacked under `internal/structured`, and token-aware context budgeting now materially changes model inputs
18. `cmd/eval` plus `scripts/run_monthly_review_5b.sh` can now produce a trace dump and report artifact for the Phase 5B Monthly Review golden path in either mock or env-gated live mode

## Current Positioning

The codebase is no longer just an **agent-ready substrate**. It is now best described as a **system-agent backbone + first real domain-agent execution path + first proactive life-event loop + first capability-backed follow-up execution + first operator-runnable durable runtime plane + real-intelligence-backed Monthly Review golden path**.

- The current strength is still system-layer-first: observation, state, memory, context, runtime, verification, governance, and observability remain the center of gravity.
- `PlannerAgent / MemorySteward / ReportAgent / VerificationAgent / GovernanceAgent` now enter the Monthly Review and Debt vs Invest main paths through real typed envelope dispatch.
- `CashflowAgent` and `DebtAgent` are now the first load-bearing domain agents in the main execution path.
- `TaxAgent` and `PortfolioAgent` now enter the new Life Event Trigger path as the next narrow domain expansion, but only inside Workflow C.
- `ReportAgent` is no longer the primary cashflow/debt analyst; it is an aggregator and finalize boundary.
- Workflow C now produces state diff, memory updates, a generated task graph, and runtime-registered follow-up tasks as its primary outputs; `LifeEventAssessmentReport` is only a secondary artifact.
- This is still not a fully realized strong multi-agent finance operating system.
- generated downstream tasks are now formal `TaskSpec`-backed queue objects, and Phase 4B lights up real workflow capability for `tax_optimization` and `portfolio_rebalance`
- runtime now advances capability-backed follow-up tasks through `queued_pending_capability -> ready -> executing -> completed / waiting_approval / failed`
- only allowlisted first-level follow-up tasks auto-execute; deeper or non-allowlisted follow-ups remain registered but not recursively auto-run
- runtime persistence no longer lives only inside a single process: task graphs, execution records, approval state, checkpoint payloads, replay events, and artifact refs now survive process restart through a local SQLite seam
- operator-facing actions are now formal typed commands with idempotent request ids and optimistic-concurrency transitions instead of ad hoc workflow-local mutation
- replay queries now read durable `ReplayStore` records rather than in-memory helper timelines
- only `PlannerAgent` and `CashflowAgent` enter the real provider-backed intelligence path in this phase, and only inside Monthly Review
- provider-backed intelligence is now a load-bearing substrate layer rather than workflow-local string prompts: prompts are versioned, context is token-aware, outputs are schema-validated, and traces include provider/prompt/token/cost/fallback evidence
- behavior-domain execution is still intentionally deferred so the implementation does not collapse into a fake “many agents chatting” story.

## Phase 3A / 3B / 4A / 4B / 5A / 5B Highlights

- `internal/protocol` is now execution-first: typed request/result message kinds, oneof-style request/result bodies, and response envelopes participate in real dispatch.
- `internal/agents` now contains a concrete execution plane: registry, dispatcher, executor, system-step bus, typed execution errors, and registered system-agent handlers.
- `MonthlyReviewWorkflow` and `DebtVsInvestWorkflow` no longer directly call planner, memory, domain analysis, verification, governance, or report generation services.
- `ReportAgent` now follows `draft -> verification -> governance -> finalize`; final report artifacts and `report_ready` are not emitted before governance.
- `PlannerAgent` now returns block-level execution plans, and `plan.Blocks` is the only truth source for block order, recipient, requirements, success criteria, and verification hints.
- `MemorySteward` is now load-bearing: retrieved memories influence block ordering and recommendation emphasis instead of being a sidecar retrieval step.
- `CashflowAgent` and `DebtAgent` now consume block-specific execution context and return typed `CashflowBlockResult` / `DebtBlockResult`.
- `VerificationAgent` now validates domain blocks before final report validation and can short-circuit on severe block failures.
- runtime now has an explicit bridge from typed agent failure categories to workflow recovery semantics.
- observability and replay now expose planner block plan, domain block execution order, selected memory/evidence/state slices, and agent dispatch lifecycle records.
- Workflow C now exists as a real `life_event_trigger` path instead of a contract-only placeholder.
- structured `event source` and `calendar/deadline source` adapters now turn life events into typed evidence before workflow execution.
- `TaskGenerationAgent` now generates `TaskSpec`-backed follow-up tasks from validated life-event analysis, state diff, evidence, and retrieved memories without redoing domain analysis.
- runtime now registers generated follow-up tasks into a task graph with explicit statuses such as `dependency_blocked`, `deferred`, `waiting_approval`, and `queued_pending_capability`.
- `LifeEventAssessmentReport` now gives Workflow C a secondary artifact contract, but the primary product of Workflow C remains state/memory/task-graph mutation rather than a narrative report.
- `TaxOptimizationWorkflow` and `PortfolioRebalanceWorkflow` now exist as real follow-up workflow entrypoints behind runtime capability activation.
- runtime now owns follow-up task reevaluation, execution ordering, execution records, committed state handoff, retry metadata, approval resumability metadata, and task-level suppression reasons.
- replay can now explain parent life-event workflow -> generated task -> child workflow -> child artifact -> child state commit as one proactive chain.
- runtime store seams are now explicit (`TaskGraphStore`, `TaskExecutionStore`, `ApprovalStateStore`, `CheckpointStore`, `ReplayStore`) and backed by a local SQLite implementation for restart-resilient operator flows.
- `cmd/api` now exposes minimal operator endpoints for listing task graphs, listing pending approvals, approve/deny, resume/retry, and task-graph reevaluation with explicit `400 / 404 / 409` semantics.
- `cmd/worker` now runs reevaluate / auto-execute / resume / operator-retry passes against the durable runtime instead of remaining a placeholder.
- approval-aware resume is now a real runtime continuation path: approved waiting child workflows resume from persisted checkpoint payloads rather than restarting the whole child workflow from the top.
- `internal/model` now provides a provider-agnostic chat/structured seam with an OpenAI-compatible live adapter and stub adapters for future providers.
- `internal/prompt` now owns versioned prompt registry, rendering, and prompt render trace for `planner.monthly_review.v1` and `cashflow.monthly_review.v1`.
- `internal/structured` now owns schema validation, parse retry, repair retry, deterministic fallback, and trace recording for planner/cashflow structured output.
- `PlannerAgent` no longer needs workflow-local prompt/provider wiring; it can use a provider-backed planner while still emitting the same typed `ExecutionPlan`.
- `CashflowAgent` can now use a provider-backed reasoner while still grounding on deterministic metrics and typed evidence refs.
- context budgeting is now token-aware for planning and cashflow execution, and trace captures what was included, excluded, compacted, and estimated.
- `cmd/eval` is now a real Monthly Review 5B runner rather than a placeholder, and `scripts/run_monthly_review_5b.sh` is the reproducible run-evidence entrypoint.

## Repository Layout

```text
cmd/                 Entrypoints for api, worker, eval, replay
internal/            Core application layers and domain contracts
var/                 Local durable runtime state and artifact refs (generated at runtime)
web/                 Minimal operator UI placeholder
deployments/         Docker Compose skeleton and deployment notes
schemas/             JSON schemas for core contracts
docs/                Architecture, ADRs, workflows, threat model, eval notes
tests/               Cross-package test notes and fixture inputs
scripts/             Reproducible run helpers, including the Phase 5B Monthly Review golden path runner
```

## Quick Start

```bash
go test ./...
go vet ./...
go run ./cmd/api --db ./var/runtime.db
go run ./cmd/worker --db ./var/runtime.db --once
./scripts/run_monthly_review_5b.sh mock
```

### Phase 5B Monthly Review Golden Path

Mock mode writes stable run evidence under `docs/eval/samples/`:

```bash
./scripts/run_monthly_review_5b.sh mock
```

Live mode is env-gated and intentionally non-CI:

```bash
export OPENAI_API_KEY=...
export OPENAI_REASONING_MODEL=...
export OPENAI_FAST_MODEL=...
./scripts/run_monthly_review_5b.sh live /tmp/monthly-review-5b
```

Optional transport override env vars:

- `OPENAI_BASE_URL`
- `OPENAI_CHAT_ENDPOINT_PATH`

The live smoke test is also env-gated:

```bash
RUN_MONTHLY_REVIEW_5B_LIVE_SMOKE=1 OPENAI_API_KEY=... go test ./internal/app -run TestMonthlyReview5BLiveSmoke -v
```

Mock sample outputs checked into the repo:

- `docs/eval/samples/monthly_review_5b_report.json`
- `docs/eval/samples/monthly_review_5b_trace.json`

The `web/` directory is intentionally minimal in this phase. Install dependencies with `npm install` inside `web/` when you are ready to iterate on the UI skeleton.

## What Is Still Stubbed

- agentic document parsing is still a deterministic stub behind a formal adapter boundary
- semantic retrieval uses a fake embedding/vector backend, but only through `EmbeddingProvider`, `VectorIndex`, `RetrievalScorer`, and `SemanticSearchBackend`
- runtime is still a local Temporal-aligned implementation, not a live Temporal cluster
- durable runtime uses a local SQLite seam and metadata/file refs, not Postgres + object storage yet
- system agents are currently local synchronous handlers behind a local bus, not remote or durable inbox/outbox actors yet
- `TaxAgent` and `PortfolioAgent` are only live inside Workflow C; Monthly Review and Debt vs Invest still keep tax/portfolio as deferred or residual sections
- capability-backed follow-up execution is still intentionally narrow: only `tax_optimization` and `portfolio_rebalance` are live child workflow capabilities, and only for first-level auto-execution
- no real Postgres / pgvector / MinIO / provider service is required yet
- semantic retrieval hardening, deterministic finance engine hardening, deeper business-rule validator expansion, and durable memory/embedding promotion are intentionally deferred to later phases instead of being mixed into the 5B intelligence substrate work

These are deliberate trade-offs. The important part is that business logic now talks to stable protocol contracts, typed agent boundaries, and deterministic subsystem services, so replacing the stubbed pieces in later phases does not require rewriting workflow logic or collapsing the 12-layer structure.

## Architecture Priorities

1. Preserve the 12-layer structure so later workflows can plug into stable contracts.
2. Keep financial calculations deterministic and outside of the LLM path.
3. Model runtime, protocol, governance, verification, and memory as first-class surfaces from day one.
4. Use stubs only behind durable interfaces so the system stays interview-defensible and production-shaped.
5. Keep workflows thin; when logic grows, it must move down into reusable subsystems or system-agent handlers rather than staying in orchestration files.
