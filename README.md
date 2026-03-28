# Personal CFO OS

Personal CFO OS is a 2026-style personal finance agent system scaffold. It is intentionally designed as a goal-driven, stateful, memory-aware, protocol-oriented, verifiable, governed, and observable system rather than a toy "LLM routes to an agent and calls a few tools" demo.

## Why This Is Not a Toy Multi-Agent Demo

- User requests do not flow directly into execution. They must be normalized into a typed `TaskSpec` with explicit goal, constraints, risk, approval, required evidence, and success criteria.
- Observations are not kept as loose chat text. The system defines typed `EvidenceRecord` contracts and uses evidence-driven state updates.
- State is a first-class object. `FinancialWorldState` supports snapshots, diffs, versioning, and reducer-based updates.
- Memory is structured and governed. The schema captures provenance, confidence, supersedes, conflicts, and access audit instead of a raw JSON save/recall blob.
- Runtime semantics are explicit. Failure categories, checkpoint records, resume tokens, approval waiting, and recovery strategies are part of the design surface.
- Governance and verification are front-loaded. The system models approval policy, tool policy, memory write policy, disclosure policy, audit events, evidence coverage, and oracle verdicts.
- Protocols are explicit. Internal agent envelopes and workflow UI events include correlation and causation identifiers so the system can be replayed and traced.

## Phase 1 Deliverables

- Monorepo scaffold with Go as the primary backend language
- Core packages for task specification, observation, state, context, memory, planning, runtime, protocol, verification, governance, agents, skills, and tools
- JSON schemas for the main contracts
- Architecture documentation and ADRs
- Deterministic unit tests for typed evidence, policies, runtime transitions, state integration, memory invariants, and verification artifacts
- Minimal frontend and infrastructure placeholders without spending Phase 1 time on full infrastructure bring-up

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

## Architecture Priorities

1. Preserve the 12-layer structure so later workflows can plug into stable contracts.
2. Keep financial calculations deterministic and outside of the LLM path.
3. Model runtime, protocol, governance, verification, and memory as first-class surfaces from day one.
4. Optimize for interview defensibility and real engineering extensibility over short-term demo speed.
