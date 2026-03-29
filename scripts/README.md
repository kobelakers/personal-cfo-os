# Scripts

The repository now ships reproducible phase runners instead of keeping scripts as placeholders.

- `run_monthly_review_5b.sh`: reproduce the Phase 5B Monthly Review golden path.
- `run_monthly_review_5c.sh`: run Monthly Review twice against the same injected `memory.db` so the second run demonstrates cross-session durable memory influence.
- `rebuild_memory_index.sh`: rebuild `memory_embeddings` and `memory_terms` for an injected memory database without touching runtime durable tables.

All scripts are local-first and env-driven. They do not hardcode database locations inside the memory subsystem; the path is injected through arguments or `MEMORY_DB_PATH`.
