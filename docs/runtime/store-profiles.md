# Store Profiles

Phase 7A introduces explicit runtime profiles instead of treating SQLite as the only durable shape.

## `local-lite`

`local-lite` keeps the existing local development experience.

- runtime backend: SQLite
- blob backend: LocalFS
- intended use:
  - local dev
  - deterministic tests
  - small single-machine runs

This profile still uses the same typed runtime stores and replay plane, but it is not the canonical strong multi-worker profile.

## `runtime-promotion`

`runtime-promotion` is the formal 7A proof profile.

- runtime backend: Postgres
- blob backend: MinIO-compatible object storage
- expected topology:
  - API
  - 2 workers
  - Postgres
  - MinIO

This profile proves:

- multi-worker-safe claim/lease execution
- scheduler-backed wakeups and retries
- stronger authoritative runtime persistence
- ref-backed checkpoint/report/replay payload storage

## Why Postgres First

Phase 7A promotes runtime durability through a DB-backed queue/lease model rather than a broker-first design.

This keeps:

- typed protocol boundaries
- CAS/version semantics
- approval/governance continuity
- replay truth alignment

on the same authoritative runtime plane.

## What Is Not Yet Promoted

Phase 7A still does not claim:

- live Temporal cluster execution
- remote agent inbox/outbox runtime
- external protocol exposure
- full artifact lifecycle management
- a full observability stack such as dedicated tracing/metrics backends
