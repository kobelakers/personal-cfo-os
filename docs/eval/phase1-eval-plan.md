# Phase 1 Evaluation Plan

Phase 1 focuses on deterministic contract validation.

## Covered Checks

- `TaskSpec` validation and normalization
- `EvidenceRecord` round-trip and normalization status handling
- evidence-driven state updates, snapshots, and diffs
- memory schema invariants
- policy decision round-trip and approval gating
- runtime failure/recovery transitions and resume token validation
- protocol round-trip with correlation and causation chain
- verification result, oracle verdict, and evidence coverage serialization

## Deferred to Later Phases

- scenario benchmarks with real imported data
- replay against full workflow histories
- online metrics dashboards
- provider-specific evaluation comparisons
