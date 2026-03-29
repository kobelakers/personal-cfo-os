# Phase 5C Evaluation Plan

Phase 5C validates the first real memory substrate without widening scope into 5D finance hardening, 6A replay/eval-plane maturation, more workflows, or UI/runtime expansion.

## Covered Checks

- durable SQLite-backed memory store persists records, relations, write events, access audit, lexical terms, and embeddings across restart
- live embedding path is OpenAI-compatible and env-driven; deterministic static embedding remains the CI-safe stub
- hybrid retrieval is real and typed:
  - lexical retrieval via durable token postings
  - semantic retrieval via provider-backed embeddings
  - reciprocal-rank fusion
  - policy-driven rejection with rule ids and reasons
- retrieval query formation is formalized through planner vs cashflow query builders instead of workflow-local string assembly
- Monthly Review golden path runs twice against the same `memory.db`, and the second run shows durable memory influence on planner/cashflow output
- trace dump now includes memory query, retrieval hit/reject, selection, embedding call, embedding usage, and final memory ids consumed by planner/cashflow
- reindex/backfill can rebuild embeddings and lexical postings for existing durable memory records

## Important Negative Paths

- missing `OPENAI_EMBEDDING_MODEL` in live embedding mode
- stale episodic memory rejected by freshness policy
- low-confidence memory rejected by retrieval policy
- empty or low-score retrieval rejected with explicit reasons
- old records require backfill before semantic/lexical retrieval is complete

## Explicitly Deferred Beyond Phase 5C

- finance engine hardening and deeper business-rule validators
- operator-grade replay/eval plane for memory traces
- async runtime promotion / worker model changes
- broader workflow rollout beyond Monthly Review
- ANN / pgvector / Postgres / MinIO / Temporal
