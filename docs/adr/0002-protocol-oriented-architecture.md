# ADR 0002: Protocol-Oriented Internal Contracts

## Status

Accepted

## Decision

Define internal agent communication and UI eventing through explicit protocol types with metadata, correlation identifiers, and causation identifiers.

## Rationale

- Protocols make replay, audit, and cross-agent coordination possible without ad hoc JSON blobs.
- Correlation and causation identifiers allow downstream observability and verification tooling to reconstruct trajectories.
- Stable schemas decouple backend evolution from the future operator UI.

## Consequences

- All agent-to-agent messages must pass through `AgentEnvelope`.
- Workflow UI streaming must use the event contract instead of bespoke payloads.
