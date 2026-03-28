# System Overview

Personal CFO OS is a long-running personal finance agent system designed around typed evidence, state-first reasoning, structured memory, explicit runtime semantics, protocol contracts, governance, and verification.

## Core Loop

1. User input is transformed into a `TaskSpec`.
2. Observation adapters collect and normalize external inputs into typed `EvidenceRecord` values.
3. Evidence-driven reducers update `FinancialWorldState`.
4. Context assembly selects the right state, memory, evidence, and skill slices for the current phase.
5. Planning drives `plan -> act -> verify -> replan/escalate/abort`.
6. Runtime semantics manage checkpoints, pause/resume, approval gates, retries, and recovery.
7. Governance policies evaluate sensitive actions, memory writes, tool execution, and report disclosure.
8. Verification checks evidence coverage, business rules, success criteria, and end-to-end oracle outcomes.

## Phase 1 Focus

Phase 1 locks down the contracts that define the system boundary:

- `TaskSpec`
- `EvidenceRecord`
- `FinancialWorldState`
- `MemoryRecord`
- `AgentEnvelope`
- `WorkflowEvent`
- runtime failure and recovery types
- governance policy models
- verification result and coverage models

This keeps later phases free to build adapters, workflows, and UI on top of stable contracts instead of rewriting the foundation.
