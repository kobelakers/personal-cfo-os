# 12-Layer Mapping

| Layer | Package(s) | Phase 1 Contract |
| --- | --- | --- |
| Goal Layer | `internal/taskspec` | `TaskSpec`, `TaskScope`, `SuccessCriteria`, `RequiredEvidenceRef` |
| Observation Layer | `internal/observation` | `EvidenceRecord`, `ObservationAdapter`, `EvidenceExtractor`, `EvidenceNormalizer` |
| State Layer | `internal/state` | `FinancialWorldState`, `StateSnapshot`, `StateDiff`, `EvidencePatch`, `StateReducer` |
| Context Engineering Layer | `internal/context` | `ContextView`, `ContextSlice`, `InjectedStateBlock`, `CompactionStrategy` |
| Memory Layer | `internal/memory` | `MemoryRecord`, retrieval interfaces, provenance and conflict schema |
| Planning / Policy Layer | `internal/planning` | explicit plan state machine with `plan -> act -> verify -> replan/escalate/abort` |
| Skills + Tools Layer | `internal/skills`, `internal/tools` | skill triggers and tool family interfaces |
| Runtime / Harness Layer | `internal/runtime` | `WorkflowExecutionState`, `CheckpointRecord`, `ResumeToken`, `RecoveryStrategy` |
| Protocol Layer | `internal/protocol` | `AgentEnvelope`, `WorkflowEvent`, correlation and causation chain |
| Verification Layer | `internal/verification` | `VerificationResult`, `EvidenceCoverageReport`, `OracleVerdict` |
| Safety / Governance / HITL Layer | `internal/governance` | policies, decisions, audit events, policy engine |
| Observability / Evaluation / Feedback Layer | `internal/observability`, `cmd/eval`, `cmd/replay` | placeholder refs for tracing, metrics, eval, replay |

## Trade-Offs

- Phase 1 optimizes for contract stability rather than operational completeness.
- Temporal, Postgres, pgvector, MinIO, and observability services are represented as deployment skeletons, not fully integrated services yet.
- The UI is intentionally minimal so the design effort stays concentrated on the agent system's architectural core.
