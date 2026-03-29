# Sample Evidence

## Phase 5B

Stable mock-generated artifacts for the real-intelligence-backed Monthly Review golden path:

- `monthly_review_5b_report.json`
- `monthly_review_5b_trace.json`

Regenerate:

```bash
./scripts/run_monthly_review_5b.sh mock
```

## Phase 5C

Stable mock-generated artifacts for the first real memory substrate on Monthly Review:

- `monthly_review_5c_report.json`
- `monthly_review_5c_trace.json`
- `monthly_review_5c_cross_session.json`

Regenerate:

```bash
./scripts/run_monthly_review_5c.sh mock
```

Live mode remains local-only and env-gated:

```bash
OPENAI_API_KEY=... \
OPENAI_REASONING_MODEL=... \
OPENAI_FAST_MODEL=... \
OPENAI_EMBEDDING_MODEL=... \
./scripts/run_monthly_review_5c.sh live /tmp/monthly-review-5c /tmp/monthly-review-5c/memory.db
```
