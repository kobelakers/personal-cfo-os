# System Overview

Personal CFO OS is a long-running personal finance agent system designed around typed evidence, state-first reasoning, structured memory, explicit runtime semantics, protocol contracts, governance, and verification.

## Core Loop

1. Natural language first enters deterministic task intake and becomes a `TaskSpec`.
2. Ledger and document adapters ingest raw inputs and emit typed `EvidenceRecord` values.
3. Deterministic reducers convert evidence into state patches and update `FinancialWorldState`.
4. Workflow services keep observation/reducer orchestration thin and hand system steps to a workflow-facing `SystemStepBus`.
5. `SystemStepBus` constructs typed envelopes and dispatches them to `PlannerAgent`, `MemorySteward`, `ReportAgent`, `VerificationAgent`, and `GovernanceAgent`.
6. Structured memory writes store episodic / semantic / procedural / policy memories with provenance, confidence, conflict, and audit semantics.
7. Hybrid retrieval and context assembly feed the planning stage through the `PlannerAgent`.
8. `ReportAgent` follows `draft -> verification -> governance -> finalize`, so final artifacts are gated behind verification and disclosure/approval decisions.
9. Runtime semantics manage checkpoints, pause/resume, approval gates, retries, protocol failures, and recovery.
10. Observability and replay record workflow timeline plus agent dispatch lifecycle.

## Real Data Path With System Agents

The current chain now looks like:

- raw ledger transactions / debt rows / holdings / payslip / tax text
- typed evidence generation and normalization
- evidence-driven state update
- planner dispatch through typed `plan_request`
- memory sync dispatch through typed `memory_sync_request`
- report draft dispatch through typed `report_draft_request`
- verification dispatch through typed `verification_request`
- governance dispatch through typed `governance_evaluation_request`
- report finalize dispatch through typed `report_finalize_request`
- runtime state transition driven by structured verification/governance outcomes and typed agent failure categories

## Current Narrative Boundary

The repository is now best described as a **partial system-agent execution architecture**.

- It is stronger than a workflow engine that merely has “agent interfaces on paper”.
- It is weaker than a fully actorized, durable, remote-executable strong multi-agent system.
- This is intentional: system-agent boundaries for planning, memory, reporting, verification, and governance are now real, while domain-agent execution remains deferred.

## Current Stubs

- agentic document parsing is still a deterministic stub behind a formal observation adapter
- semantic retrieval still uses a fake backend behind embedding/vector interfaces
- runtime is local Temporal-aligned rather than connected to a live Temporal cluster
- observability is structured dump / replay ready, but not yet backed by full tracing infrastructure
- system-agent execution is local synchronous dispatch, not yet async/durable inbox-outbox execution
- domain agents are still capability placeholders, not main-path executors

The system is still intentionally local-first. Real Postgres, pgvector, MinIO, Temporal, and model providers are deferred, but only behind already-fixed interfaces. That keeps the direction aligned with a 2026 agent system instead of collapsing into a Phase 2 demo.
