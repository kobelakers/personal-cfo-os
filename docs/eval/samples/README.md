# Phase 5B Sample Evidence

These files are stable mock-generated run artifacts for the Phase 5B Monthly Review golden path:

- `monthly_review_5b_report.json`
- `monthly_review_5b_trace.json`

Regenerate them from the repository root with:

```bash
./scripts/run_monthly_review_5b.sh mock
```

For a local live smoke run, provide OpenAI-compatible env vars and choose `live` mode:

```bash
OPENAI_API_KEY=... OPENAI_REASONING_MODEL=... OPENAI_FAST_MODEL=... ./scripts/run_monthly_review_5b.sh live /tmp/monthly-review-5b
```
