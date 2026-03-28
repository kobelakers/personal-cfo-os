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

The repository now runs a real governed finance workflow backbone with partial system-agent execution:

1. raw ledger and document fixtures are ingested by observation adapters
2. adapters emit typed `EvidenceRecord` values
3. workflow services orchestrate evidence collection and deterministic reducers build `EvidencePatch`
4. `FinancialWorldState` is updated with versioning, snapshot, and diff semantics
5. `SystemStepBus` dispatches typed protocol envelopes to `PlannerAgent`, `MemorySteward`, `ReportAgent`, `VerificationAgent`, and `GovernanceAgent`
6. `ReportAgent` is split into draft and finalize stages so final artifacts are produced only after verification and governance
7. runtime consumes structured verification diagnostics and typed agent failure categories to decide `completed / replanning / waiting_approval / failed`
8. observability and replay outputs now include agent dispatch lifecycle, checkpoint timeline, memory access, and policy decisions

## Current Positioning

The codebase is no longer just an **agent-ready substrate**. It is now best described as a **partial system-agent execution architecture on top of a governed workflow backbone**.

- The current strength is still system-layer-first: observation, state, memory, context, runtime, verification, governance, and observability remain the center of gravity.
- `PlannerAgent / MemorySteward / ReportAgent / VerificationAgent / GovernanceAgent` now enter the Monthly Review and Debt vs Invest main paths through real typed envelope dispatch.
- This is still not a fully realized strong multi-agent finance operating system.
- Domain agents remain future-facing. Their execution boundary is intentionally deferred so the current implementation does not collapse into a fake “many agents chatting” story.

## Phase 3A Highlights

- `internal/protocol` is now execution-first: typed request/result message kinds, oneof-style request/result bodies, and response envelopes participate in real dispatch.
- `internal/agents` now contains a concrete execution plane: registry, dispatcher, executor, system-step bus, typed execution errors, and registered system-agent handlers.
- `MonthlyReviewWorkflow` and `DebtVsInvestWorkflow` no longer directly call planner, memory, verification, governance, or report generation services.
- `ReportAgent` now follows `draft -> verification -> governance -> finalize`; final report artifacts and `report_ready` are not emitted before governance.
- runtime now has an explicit bridge from typed agent failure categories to workflow recovery semantics.
- observability and replay now expose agent dispatch / handler started / handler completed / handler failed lifecycle records.

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
- domain agents are registration-ready only; they are not yet in the execution backbone
- no real Postgres / pgvector / MinIO / provider service is required yet

These are deliberate trade-offs. The important part is that business logic now talks to stable protocol contracts, typed agent boundaries, and deterministic subsystem services, so replacing the stubbed pieces in later phases does not require rewriting workflow logic or collapsing the 12-layer structure.

## Architecture Priorities

1. Preserve the 12-layer structure so later workflows can plug into stable contracts.
2. Keep financial calculations deterministic and outside of the LLM path.
3. Model runtime, protocol, governance, verification, and memory as first-class surfaces from day one.
4. Use stubs only behind durable interfaces so the system stays interview-defensible and production-shaped.
5. Keep workflows thin; when logic grows, it must move down into reusable subsystems or system-agent handlers rather than staying in orchestration files.
