# ADR 0001: State-First Design with Typed Evidence

## Status

Accepted

## Decision

Represent all external observations as typed `EvidenceRecord` values and treat `FinancialWorldState` as the authoritative working model of the user's financial world.

## Rationale

- Loose tool output and message history are too unstable for long-running finance workflows.
- Typed evidence gives the system provenance, confidence, artifacts, and normalization metadata.
- State snapshots, diffs, and versioned reducers provide a clean path to replay and verification.

## Consequences

- New adapters must normalize into evidence before they can affect state.
- Workflow implementations gain determinism at the cost of more up-front schema work.
