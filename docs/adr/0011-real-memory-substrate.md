# ADR 0011: Real Memory Substrate

## Status

Accepted

## Context

By the end of Phase 5B, Personal CFO OS already had a real-intelligence-backed Monthly Review golden path, but memory was still structurally ahead of its implementation. The repository had a memory schema, memory-aware context wiring, and workflow-facing memory services, yet semantic retrieval still depended on fake backends and memory was not durable in the way a 2026-style agent system needs it to be.

Phase 5C upgrades memory from a shaped interface to a load-bearing substrate:

- durable across restart
- semantically retrievable through real embeddings
- lexically retrievable through a real index
- fused and policy-rejected through typed retrieval logic
- traceable and governance-aware
- able to change downstream planning and reasoning in a later session

## Decision

We add a local-first real memory substrate with the following boundaries:

1. Memory durable plane is separate from runtime durable plane.
   - Runtime durable store continues to own task graphs, execution records, checkpoints, approvals, replay events, and artifact refs.
   - Memory durable store owns `MemoryRecord`, relations, embeddings, lexical postings, access audit, and write events.
   - Both may use SQLite today, but they are separate stores, schemas, and lifecycles.

2. `MemoryStore` stays small.
   - `MemoryStore` remains the core truth-source seam for `Put/Get/List`.
   - specialized responsibilities are split into typed seams:
     - `MemoryQueryStore`
     - `MemoryRelationStore`
     - `MemoryAuditStore`
     - `MemoryWriteEventStore`
     - `MemoryEmbeddingStore`

3. Retrieval query formation is formalized.
   - Monthly Review planning and cashflow reasoning now build typed retrieval queries through dedicated builders rather than workflow-local string logic.

4. Hybrid retrieval is the minimum real stack.
   - lexical retrieval from durable term postings
   - semantic retrieval from provider-backed embeddings
   - reciprocal-rank fusion
   - policy-driven rejection with explicit rule ids and reasons

5. Memory influence must be observable and testable.
   - memory query / hit / reject / selection enter workflow trace
   - selected durable memories are injected into planning / execution context
   - Monthly Review is run twice against the same `memory.db` to prove cross-session influence

## Consequences

### Positive

- Personal CFO OS now has a first real memory substrate instead of a fake semantic-memory story.
- durable memory survives restart and can influence later workflow runs.
- memory can now be explained through query, score, fusion, rejection, selection, audit, and write-event evidence.
- the design stays upgrade-friendly for stronger stores in later phases without forcing workflow rewrites.

### Negative / Deferred

- SQLite + brute-force cosine is not the final retrieval stack.
- lexical retrieval is BM25-like token-posting retrieval, not a mature search engine.
- memory traces are now visible in workflow trace, but not yet elevated to a full operator-grade queryable replay/eval plane.
- finance reasoning hardening remains a separate concern for Phase 5D.
