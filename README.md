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

## What Phase 2 Now Runs End-to-End

Phase 2 now runs one real executable path and has already been structurally remediated so the logic is no longer trapped inside workflow files:

1. raw ledger and document fixtures are ingested by observation adapters
2. adapters emit typed `EvidenceRecord` values
3. workflow services orchestrate evidence collection and deterministic reducers build `EvidencePatch`
4. `FinancialWorldState` is updated with versioning, snapshot, and diff semantics
5. `memory.WorkflowMemoryService` derives memories, enforces write gates, and performs hybrid retrieval
6. `context.DefaultContextAssembler` builds planning / execution / verification views with real budgets and compaction
7. `MonthlyReviewWorkflow` now acts as a thin orchestrator over workflow service, memory service, verification pipeline, approval service, artifact service, and runtime
8. verification, governance, checkpoints, approval waiting, timeline dumps, and replay-ready outputs all produce structured artifacts

## Current Phase 2 Positioning

The codebase is currently best described as a **systemized workflow engine with agent-ready substrate**, not a strong multi-agent execution system yet.

- The current strength is system-layer-first: observation, state, memory, context, runtime, verification, governance, and observability now have real executable boundaries.
- Domain agents and system agents are still part of the long-term design, but strong actor/envelope/handler execution boundaries are intentionally deferred to Phase 3.
- This is a deliberate narrative choice: the project is more defensible in interviews as a finance agent system with solid runtime and governance foundations than as a premature “many agents talking to each other” demo.

## Structural Remediation Highlights

- `internal/workflows/monthly_review.go` and `internal/workflows/debt_vs_invest.go` are now thin orchestrators.
- Evidence collection and state reduction live in dedicated workflow services.
- Derived memory generation and memory write gating live in the memory/governance boundary instead of inside workflows.
- Verification is assembled by `internal/verification/pipeline.go`, not by workflow-local validator chains.
- Approval and disclosure decisions are assembled by `internal/governance/approval_service.go`.
- Runtime concrete implementations now live in dedicated runtime files and are obtained through runtime constructors instead of workflow-local object graphs.
- Context engineering is now its own concrete subsystem with assembler, selection, budget, and compactor files.

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
- runtime is a local Temporal-aligned implementation, not a live Temporal cluster
- no real Postgres / pgvector / MinIO / provider service is required yet

These are deliberate trade-offs. The important part is that business logic already talks to stable interfaces and typed contracts, so replacing the stubbed pieces in Phase 3 does not require rewriting workflow logic or collapsing the 12-layer structure.

## Architecture Priorities

1. Preserve the 12-layer structure so later workflows can plug into stable contracts.
2. Keep financial calculations deterministic and outside of the LLM path.
3. Model runtime, protocol, governance, verification, and memory as first-class surfaces from day one.
4. Use stubs only behind durable interfaces so the system stays interview-defensible and production-shaped.
5. Keep workflows thin; when logic grows, it must move down into reusable subsystems rather than staying in orchestration files.
