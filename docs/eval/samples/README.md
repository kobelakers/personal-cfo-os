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

## Phase 5D

Stable mock-generated artifacts for trustworthy finance reasoning:

- `monthly_review_5d_report.json`
- `monthly_review_5d_trace.json`
- `debt_vs_invest_5d_waiting_approval.json`
- `debt_vs_invest_5d_waiting_approval_trace.json`

Regenerate Monthly Review:

```bash
./scripts/run_monthly_review_5d.sh mock
```

Regenerate the deterministic Debt vs Invest approval sample:

```bash
go run ./cmd/eval --phase 5d --workflow debt_vs_invest --provider-mode mock --memory-db ./var/memory-5d.db --artifact-out ./docs/eval/samples/debt_vs_invest_5d_waiting_approval.json
```
