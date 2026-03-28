# System Overview

Personal CFO OS is a long-running personal finance agent system designed around typed evidence, state-first reasoning, structured memory, explicit runtime semantics, protocol contracts, governance, and verification.

## Core Loop

1. Natural language first enters deterministic task intake and becomes a `TaskSpec`.
2. Ledger and document adapters ingest raw inputs and emit typed `EvidenceRecord` values.
3. Deterministic reducers convert evidence into state patches and update `FinancialWorldState`.
4. Structured memory writes store episodic / semantic / procedural / policy memories with provenance, confidence, conflict, and audit semantics.
5. Hybrid retrieval feeds context assembly for planning, execution, and verification views.
6. Planning drives `plan -> act -> verify -> replan/escalate/abort`.
7. Runtime semantics manage checkpoints, pause/resume, approval gates, retries, and recovery.
8. Governance evaluates memory writes, report disclosure, and high-risk actions before the workflow can complete.
9. Verification checks evidence coverage, structure, business rules, success criteria, and oracle outcomes.

## Phase 2 Real Data Path

Phase 2 already runs one real chain:

- raw ledger transactions / debt rows / holdings / payslip / tax text
- typed evidence generation and normalization
- evidence-driven state update
- derived memory write and hybrid read
- Monthly Review workflow execution
- verification, approval decision, checkpointing, and timeline output

## What Structural Remediation Changed

The original Phase 2 code proved the end-to-end path, but too much system logic still lived inside workflow files. Structural remediation changed that:

- workflow files now keep orchestration only
- evidence collection and state reduction are handled by dedicated workflow services
- derived memory generation and write gating moved into the memory/governance boundary
- verification moved into a reusable verification pipeline
- approval and disclosure logic moved into a reusable governance approval service
- runtime concrete implementations now live behind runtime constructors and interfaces instead of workflow-local object assembly
- context engineering now has clearer package-level boundaries for assembler, selection, budget, and compaction

## Current Narrative Boundary

The repository is currently closer to a **systemized workflow engine with agent-ready substrate** than a strong multi-agent execution system.

- This is intentional for Phase 2.
- The current code is already protocol-oriented and system-agent-ready.
- Strong actor-style execution boundaries for `VerificationAgent`, `GovernanceAgent`, `MemorySteward`, and `ReportAgent` are deferred to Phase 3 instead of being faked early.

## Current Stubs

- agentic document parsing is still a deterministic stub behind a formal observation adapter
- semantic retrieval still uses a fake backend behind embedding/vector interfaces
- runtime is local Temporal-aligned rather than connected to a live Temporal cluster
- observability is structured dump / replay ready, but not yet backed by full tracing infrastructure

The system is still intentionally local-first. Real Postgres, pgvector, MinIO, Temporal, and model providers are deferred, but only behind already-fixed interfaces. That keeps the direction aligned with a 2026 agent system instead of collapsing into a Phase 2 demo.
