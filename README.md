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

The repository now runs a real governed finance workflow backbone with system-agent execution, a first real domain-agent path, and a first proactive life-event loop:

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
12. Workflow C now ingests structured life events and deadlines, updates state/memory, executes event-specific domain blocks, generates typed follow-up tasks, verifies and governs them, and registers them into runtime as follow-up task graph records

## Current Positioning

The codebase is no longer just an **agent-ready substrate**. It is now best described as a **system-agent backbone + first real domain-agent execution path + first proactive life-event loop**.

- The current strength is still system-layer-first: observation, state, memory, context, runtime, verification, governance, and observability remain the center of gravity.
- `PlannerAgent / MemorySteward / ReportAgent / VerificationAgent / GovernanceAgent` now enter the Monthly Review and Debt vs Invest main paths through real typed envelope dispatch.
- `CashflowAgent` and `DebtAgent` are now the first load-bearing domain agents in the main execution path.
- `TaxAgent` and `PortfolioAgent` now enter the new Life Event Trigger path as the next narrow domain expansion, but only inside Workflow C.
- `ReportAgent` is no longer the primary cashflow/debt analyst; it is an aggregator and finalize boundary.
- Workflow C now produces state diff, memory updates, a generated task graph, and runtime-registered follow-up tasks as its primary outputs; `LifeEventAssessmentReport` is only a secondary artifact.
- This is still not a fully realized strong multi-agent finance operating system.
- generated downstream tasks are now formal `TaskSpec`-backed queue objects, but capability-gated intents such as `tax_optimization` and `portfolio_rebalance` remain `queued_pending_capability` in Phase 4A instead of auto-executing recursively.
- behavior-domain execution is still intentionally deferred so the implementation does not collapse into a fake “many agents chatting” story.

## Phase 3A / 3B / 4A Highlights

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

## Repository Layout

```text
cmd/                 Entrypoints for api, worker, eval, replay
internal/            Core application layers and domain contracts
web/                 Minimal operator UI placeholder
deployments/         Docker Compose skeleton and deployment notes
schemas/             JSON schemas for core contracts
docs/                Architecture, ADRs, workflows, threat model, eval notes
tests/               Cross-package test notes
scripts/             Developer workflow placeholders
```

## Quick Start

```bash
go test ./...
```

The `web/` directory is intentionally minimal in Phase 1. Install dependencies with `npm install` inside `web/` when you are ready to iterate on the UI skeleton.

## What Is Still Stubbed

- agentic document parsing is still a deterministic stub behind a formal adapter boundary
- semantic retrieval uses a fake embedding/vector backend, but only through `EmbeddingProvider`, `VectorIndex`, `RetrievalScorer`, and `SemanticSearchBackend`
- runtime is still a local Temporal-aligned implementation, not a live Temporal cluster
- system agents are currently local synchronous handlers behind a local bus, not remote or durable inbox/outbox actors yet
- `TaxAgent` and `PortfolioAgent` are only live inside Workflow C; Monthly Review and Debt vs Invest still keep tax/portfolio as deferred or residual sections
- generated downstream tasks are registered and replayable, but capability-gated intents do not fully execute in Phase 4A
- no real Postgres / pgvector / MinIO / provider service is required yet

These are deliberate trade-offs. The important part is that business logic now talks to stable protocol contracts, typed agent boundaries, and deterministic subsystem services, so replacing the stubbed pieces in later phases does not require rewriting workflow logic or collapsing the 12-layer structure.

## Architecture Priorities

1. Preserve the 12-layer structure so later workflows can plug into stable contracts.
2. Keep financial calculations deterministic and outside of the LLM path.
3. Model runtime, protocol, governance, verification, and memory as first-class surfaces from day one.
4. Use stubs only behind durable interfaces so the system stays interview-defensible and production-shaped.
5. Keep workflows thin; when logic grows, it must move down into reusable subsystems or system-agent handlers rather than staying in orchestration files.
