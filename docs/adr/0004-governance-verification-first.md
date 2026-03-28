# ADR 0004: Governance and Verification Are First-Class

## Status

Accepted

## Decision

Front-load governance and verification contracts in Phase 1 instead of adding them after the workflows exist.

## Rationale

- Finance agents need approval policy, tool policy, disclosure policy, memory write policy, and audit events from the start.
- Verification must cover deterministic checks, evidence coverage, business validation, and end-to-end oracle outcomes.
- Interview defensibility is stronger when governance and verification are built into the core architecture rather than attached later.

## Consequences

- Workflow implementations in later phases inherit explicit policy and verification touchpoints.
- The Phase 1 surface area is larger, but the project avoids architectural backtracking.
