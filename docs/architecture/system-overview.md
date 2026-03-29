# System Overview

Personal CFO OS is a long-running personal finance agent system designed around typed evidence, state-first reasoning, structured memory, explicit runtime semantics, protocol contracts, governance, verification, and replayable observability.

## Core Loop

1. Natural language first enters deterministic task intake and becomes a `TaskSpec`.
2. Ledger and document adapters ingest raw inputs and emit typed `EvidenceRecord` values.
3. Deterministic reducers convert evidence into state patches and update `FinancialWorldState`.
4. Workflow services keep observation/reducer orchestration thin and hand execution to a workflow-facing `SystemStepBus`.
5. `MemorySteward` derives and retrieves memories before planning; retrieved memories now influence downstream block ordering and recommendation emphasis.
6. `PlannerAgent` assembles planning context and returns a block-level `ExecutionPlan`; `plan.Blocks` becomes the only execution truth source.
7. Workflow iterates `plan.Blocks`, assembles block-specific execution context, and dispatches `CashflowAgent` or `DebtAgent` for deterministic domain analysis.
8. `ReportAgent` aggregates typed domain block results into a draft, then later finalizes only after verification and governance allow or redact.
9. `VerificationAgent` runs block-level validation first and may short-circuit final report validation with structured replan diagnostics.
10. `GovernanceAgent` evaluates approval and disclosure policy before finalize.
11. Runtime semantics manage checkpoints, pause/resume, approval gates, retries, protocol failures, and recovery.
12. Observability and replay record workflow timeline, block plan, domain block execution order, selected context slices, and agent dispatch lifecycle.

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

The repository is now best described as **system-agent backbone + first real domain-agent execution path**.

- It is stronger than a workflow engine that merely has “agent interfaces on paper”.
- It is weaker than a fully actorized, durable, remote-executable strong multi-agent system.
- This is intentional: system-agent boundaries are real, and only the first two load-bearing domain agents are in the main path.

## Current Stubs

- agentic document parsing is still a deterministic stub behind a formal observation adapter
- semantic retrieval still uses a fake backend behind embedding/vector interfaces
- runtime is local Temporal-aligned rather than connected to a live Temporal cluster
- observability is structured dump / replay ready, but not yet backed by full tracing infrastructure
- system-agent execution is local synchronous dispatch, not yet async/durable inbox-outbox execution
- only `CashflowAgent` and `DebtAgent` are live; portfolio / tax / behavior domain execution is deferred

The system is still intentionally local-first. Real Postgres, pgvector, MinIO, Temporal, and model providers are deferred, but only behind already-fixed interfaces. That keeps the direction aligned with a 2026 agent system instead of collapsing into a Phase 2 demo.
