# Personal CFO OS

Personal CFO OS is a 2026-style personal finance agent system. It is intentionally designed as a goal-driven, stateful, memory-aware, protocol-oriented, verifiable, governed, observable, replayable/evaluable, and skill-aware system rather than a toy "LLM routes to an agent and calls a few tools" demo.

## Why This Is Not a Toy Multi-Agent Demo

- User requests do not flow directly into execution. They must be normalized into a typed `TaskSpec` with explicit goal, constraints, risk, approval, required evidence, and success criteria.
- Observations are not kept as loose chat text. The system defines typed `EvidenceRecord` contracts and uses evidence-driven state updates.
- State is a first-class object. `FinancialWorldState` supports snapshots, diffs, versioning, and reducer-based updates.
- Memory is structured and governed. The schema captures provenance, confidence, supersedes, conflicts, and access audit instead of a raw JSON save/recall blob.
- Runtime semantics are explicit. Failure categories, checkpoint records, resume tokens, approval waiting, and recovery strategies are part of the design surface.
- Governance and verification are front-loaded. The system models approval policy, tool policy, memory write policy, disclosure policy, audit events, evidence coverage, and oracle verdicts.
- Protocols are explicit. Internal agent envelopes and workflow UI events include correlation and causation identifiers so the system can be replayed and traced.

## What Now Runs End-to-End

The repository now runs a real governed finance workflow backbone with system-agent execution, a first real domain-agent path, a first proactive life-event loop, a first capability-backed follow-up execution path, a promoted async-capable durable runtime backbone, a first real-intelligence-backed Monthly Review golden path, a first real memory substrate, a first trustworthy finance reasoning substrate, a first operator-grade replay/eval/debug plane, a first versioned skill runtime, and a first formal behavior domain:

1. raw ledger and document fixtures are ingested by observation adapters
2. adapters emit typed `EvidenceRecord` values
3. workflow services orchestrate evidence collection and deterministic reducers build `EvidencePatch`
4. `FinancialWorldState` is updated with versioning, snapshot, and diff semantics
5. `SystemStepBus` dispatches typed protocol envelopes to `MemorySteward`, `PlannerAgent`, `CashflowAgent`, `DebtAgent`, `ReportAgent`, `VerificationAgent`, and `GovernanceAgent`
6. `PlannerAgent` returns a block-level execution plan, and `plan.Blocks` becomes the only execution truth source for downstream dispatch
7. `CashflowAgent` and `DebtAgent` execute real deterministic analysis blocks using block-specific execution context and retrieved memories
8. `ReportAgent` aggregates domain block results into a draft and only finalizes artifacts after verification and governance
9. `VerificationAgent` now runs block-level validation before final report validation and can short-circuit into structured replan diagnostics
10. runtime consumes structured verification diagnostics and typed agent failure categories to decide `completed / replanning / waiting_approval / failed`
11. observability and replay outputs now include block plan, domain block execution order, selected memory/evidence/state slices, checkpoint timeline, and policy decisions
12. Workflow C now ingests structured life events and deadlines, updates state/memory, executes event-specific domain blocks, generates typed follow-up tasks, verifies and governs them, registers them into runtime as follow-up task graph records, then lets runtime activate and execute allowlisted first-level follow-up capabilities for `tax_optimization` and `portfolio_rebalance`
13. runtime state is now backed by a local durable SQLite seam for task graphs, execution records, checkpoints, resume tokens, approvals, operator actions, replay events, committed state snapshots, and artifact metadata refs
14. `cmd/api` and `cmd/worker` now provide a minimal runnable operator surface for approvals, resume, retry, reevaluate, and durable worker passes instead of placeholder binaries
15. `PlannerAgent` now has a real provider-backed structured planning path for Monthly Review, but still compiles back into the existing typed `planning.ExecutionPlan`
16. `CashflowAgent` now has a real provider-backed structured analysis path for Monthly Review, but deterministic finance metrics remain the source of truth
17. prompts are now versioned system objects under `internal/prompt`, structured output is validated/repaired/fallbacked under `internal/structured`, and token-aware context budgeting now materially changes model inputs
18. `cmd/eval` plus `scripts/run_monthly_review_5b.sh` can now produce a trace dump and report artifact for the Phase 5B Monthly Review golden path in either mock or env-gated live mode
19. `cmd/eval --phase 5c` plus `scripts/run_monthly_review_5c.sh` now reopen the same injected `memory.db`, retrieve durable memories through lexical + semantic + fusion + rejection, and show that prior monthly-review memories can change planner/cashflow output in a later session
20. `internal/finance` now acts as the current live path's numeric truth source, shared typed recommendations carry risk/grounding/approval semantics, deterministic validators harden recommendation trust, and governance can move high-risk finance actions into `waiting_approval`
21. runtime durable truth is now augmented by replay/debug projection rows plus artifact refs, `cmd/replay` can answer workflow/task/execution/approval why/how questions from the same durable plane, and `cmd/eval --mode corpus` can run a deterministic regression corpus instead of only phase runners
22. `behavior_intervention` now enters through deterministic intake, planner emits a skill-aware behavior block, orchestrator-side selection resolves concrete skill family/version/recipe, `BehaviorAgent` executes a formal behavior-domain result, and procedural memory can change the selected recipe on a later similar run

## Phase 7A Runtime Promotion

Phase 7A is now in closeout-hardened state: the runtime backbone is promoted without turning the repo into a UI phase, a broker-first distributed system, or a remote-agent platform:

- `cmd/worker` now runs an async-capable claim/lease/heartbeat/reclaim model instead of a plain worker pass loop
- runtime uses durable typed work items for:
  - `reevaluate_task_graph`
  - `execute_ready_task`
  - `resume_approved_checkpoint`
  - `retry_failed_execution`
  - `scheduler_wakeup`
- scheduler and reevaluator are now explicit runtime subsystems that enqueue work rather than executing workflow business logic inline
- Postgres work-queue `heartbeat / complete / fail / requeue` mutations now use atomic fenced CAS, dedupe is safe under concurrent writers, reclaim has a single winner, and long-running claims renew leases through periodic heartbeat
- completion, checkpoint, and transition commits remain guarded by lease ownership plus fencing token checks, so reclaimed stale workers cannot commit successfully
- SQLite remains the `local-lite` profile, while Postgres is now a real `runtime-promotion` backend for runtime-authoritative stores
- `SkillExecutionStore` now has Postgres parity, so the promoted runtime profile does not drop the 6B skill-runtime truth surface
- checkpoint payloads, final report payloads, and replay bundles now support ref-backed blob storage through LocalFS or MinIO-compatible backends
- replay/debug stays on the same canonical `internal/runtime.ReplayQueryService` plane and can now explain worker claim, lease heartbeat, reclaim, retry scheduling, and approval resume across workers

## Phase 7B Productization / Externalization

Phase 7B does not change the kernel truth surface. It productizes the existing typed runtime/operator/replay/eval surfaces:

- `web/` is now a real operator console instead of a placeholder and is served by `cmd/api` in `interview-demo` and `runtime-promotion`
- `/api/v1` is now the canonical operator/read surface; existing unversioned endpoints remain compatibility aliases only
- deployment profiles are now formalized as:
  - `local-lite`
  - `runtime-promotion`
  - `interview-demo`
  - `dev-stack`
- versioned external legibility now exists through `schemas/public/v1/*`
- `AgentEnvelope` is documented only as reference/internal execution protocol exposure, not as a public write contract
- benchmark/reporting is now a formal read/compare/export surface on top of deterministic corpus truth and checked-in samples

Phase 7B closeout hardens that product surface rather than widening it:

- benchmark registry now reads both checked-in deterministic samples and artifact-plane `eval_run_result` artifacts behind one typed catalog
- benchmark summaries now carry source, artifact identity, and a cost-ish summary with explicit precision/source semantics
- the operator UI has been refactored into a panelized structure plus minimal hooks, without changing its API-only boundary
- interview-demo, dev-stack, and runtime-promotion now have clearer runbooks/sample indexes, so the product surface is easier to demo and defend externally

## Phase 5C Real Memory Substrate

Phase 5C upgrades memory from a shaped interface to a load-bearing substrate for Monthly Review:

- `internal/memory` now has a real SQLite-backed durable memory seam with separate tables for memory records, relations, embeddings, lexical terms, access audit, and write events
- memory writes now follow `prepare -> single durable commit`: provenance/confidence validation, conflict/supersedes detection, lexical term generation, and embedding generation happen before one SQLite transaction atomically commits records, relations, embeddings, terms, audit, and write events
- memory durable plane is intentionally separate from the runtime durable plane introduced in Phase 5A; they may both use SQLite locally, but they are not the same semantic layer
- semantic retrieval now uses a real embedding provider seam with one OpenAI-compatible live implementation and one deterministic static provider for tests and mock runs
- retrieval is now a real hybrid stack:
  - lexical retrieval from durable term postings
  - semantic retrieval from persisted embeddings
  - reciprocal-rank fusion
  - policy-driven rejection with explicit reasons, applied after fusion/rerank and before final accepted `topK` selection
- retrieval query formation is no longer workflow-local string glue; planner and cashflow use dedicated typed query builders
- conflict / supersedes auto-detection is now explicit and intentionally narrow:
  - `conflict`: same memory kind, different record id, same fact key, different value
  - `supersedes`: same memory kind, different record id, same summary semantics, newer update time
- cross-session influence is now part of the Monthly Review proof: the second run against the same `memory.db` can alter planner rationale, recommendation framing, or report provenance because prior durable memories were selected into context

## Phase 5D Trustworthy Finance Reasoning

Phase 5D adds a trust layer on top of the existing intelligence + memory backbone:

- `internal/finance` is now the formal numeric truth source for the current live path; key finance numbers are emitted as typed metric bundles plus `metric_records`
- shared typed recommendations now carry:
  - `recommendation_type`
  - `risk_level`
  - `metric/evidence/memory/grounding refs`
  - `caveats`
  - `approval_required`
  - `approval_reason`
  - `policy_rule_refs`
- verification is no longer only schema-oriented; the live path now runs:
  - grounding validation
  - numeric consistency validation
  - business-rule validation
- GovernanceAgent now consumes typed recommendation/risk/disclosure state and uses fixed runtime transitions:
  - grounding/numeric/business validator fail -> `failed`
  - governance `RequireApproval` -> `waiting_approval`
  - governance `Deny` -> `failed(governance_denied)`
  - operator approve after `waiting_approval` -> resume continuation
- the canonical 5D approval proof is deterministic and fixture-driven:
  - `Debt vs Invest`
  - low emergency fund or high debt pressure
  - aggressive `invest_more`
  - GovernanceAgent -> `waiting_approval`
- Monthly Review final reports now carry grounded recommendations, risk fields, caveats/disclosures, approval fields, and provenance refs instead of only freeform summary text
- trust trace now includes:
  - finance metric records
  - grounding verdicts
  - numeric verdicts
  - business-rule verdicts
  - policy rule hits
  - approval triggers

## Phase 6A Replay / Eval / Debug Plane

Phase 6A upgrades replay/eval/debug from "export a trace and read JSON by hand" into a local operator-grade plane:

- replay truth source is now:
  - runtime durable truth
  - normalized replay/debug projection rows
  - artifact refs for rich bundles and golden outputs
- replay/debug projections are versioned and rebuildable/backfillable from authoritative runtime truth; query does not assume projections are always complete
- `ReplayQueryService` now supports workflow / task-graph / task / execution / approval replay queries with explicit degrade semantics:
  - authoritative runtime truth missing -> hard failure
  - projection missing, stale, or incomplete -> partial replay view plus degradation reasons
- provenance is now a directed graph rather than an ID bag, so replay can explain:
  - why a workflow failed
  - why a task entered `waiting_approval`
  - why a generated task exists
  - why a child workflow executed
  - why memory was selected or rejected
  - what changed between two runs
- the canonical replay plane is now `internal/runtime.ReplayQueryService`; the earlier observability-local replay stack has been retired so the repo only tells one replay story
- `cmd/replay` is no longer a placeholder; it can query a single scope, compare two scopes, rebuild projections, and print JSON or human-readable summaries
- workflow-level replay projection freshness is now explicitly hardened for `completed`, `failed`, `waiting_approval`, and approval-resume transitions; stale or incomplete projections degrade to partial replay views instead of becoming opaque errors
- `cmd/eval` is no longer mainly a phase runner; it now has a deterministic regression harness with a canonical 11-scenario corpus
- the canonical corpus now includes an explicit `monthly_review_memory_rejection_visibility` case so rejected-memory reasons are a first-class replay/debug regression surface
- the canonical 6A corpus only runs deterministic fixtures / mock intelligence paths; live provider paths remain smoke/manual evidence and are intentionally excluded from stable regression

## Phase 6B Skills System + Behavior Domain

Phase 6B adds a narrow but load-bearing Skills + Behavior layer without widening the repo into UI, infra, or runtime-promotion work:

- `behavior_intervention` is now a real deterministic intake path, not just an eval-only wiring
- `internal/skills` now holds canonical skill manifests, family/version/recipe metadata, policy, typed selection reasons, and runtime execution records
- `internal/behavior` is now a formal domain with deterministic metrics, anomaly detection, grounded recommendations, validators, and governance mapping
- `BehaviorBlockResult` now flows through `analysis.BlockResultEnvelope`, so behavior is a load-bearing block result rather than a report sidecar
- procedural memory is now written back into the existing durable memory substrate and can deterministically change the next similar skill/recipe selection
- the canonical high-risk proof is `discretionary_guardrail / hard_cap.v1`, which enters `waiting_approval` without performing any external payment or account action
- replay/eval/debug now explains why a skill family/version/recipe was chosen and which procedural memory influenced the change

## Current Positioning

The codebase is no longer just an **agent-ready substrate**. It is now best described as a **system-agent backbone + first real domain-agent execution path + first proactive life-event loop + first capability-backed follow-up execution + promoted async-capable durable runtime backbone + real-intelligence-backed Monthly Review golden path + first real memory substrate + trustworthy finance reasoning substrate + first operator-grade replay/eval/debug plane + first versioned skill runtime + first formal behavior domain + procedural-memory-influenced skill selection**.

- The current strength is still system-layer-first: observation, state, memory, context, runtime, verification, governance, and observability remain the center of gravity.
- `PlannerAgent / MemorySteward / ReportAgent / VerificationAgent / GovernanceAgent` now enter the Monthly Review and Debt vs Invest main paths through real typed envelope dispatch.
- `CashflowAgent` and `DebtAgent` are now the first load-bearing domain agents in the main execution path.
- `TaxAgent` and `PortfolioAgent` now enter the new Life Event Trigger path as the next narrow domain expansion, but only inside Workflow C.
- `ReportAgent` is no longer the primary cashflow/debt analyst; it is an aggregator and finalize boundary.
- Workflow C now produces state diff, memory updates, a generated task graph, and runtime-registered follow-up tasks as its primary outputs; `LifeEventAssessmentReport` is only a secondary artifact.
- This is still not a fully realized strong multi-agent finance operating system.
- generated downstream tasks are now formal `TaskSpec`-backed queue objects, and Phase 4B lights up real workflow capability for `tax_optimization` and `portfolio_rebalance`
- runtime now advances capability-backed follow-up tasks through `queued_pending_capability -> ready -> executing -> completed / waiting_approval / failed`
- only allowlisted first-level follow-up tasks auto-execute; deeper or non-allowlisted follow-ups remain registered but not recursively auto-run
- runtime persistence no longer lives only inside a single process: task graphs, execution records, approval state, checkpoint payloads, replay events, and artifact refs now survive process restart through a local SQLite seam
- operator-facing actions are now formal typed commands with idempotent request ids and optimistic-concurrency transitions instead of ad hoc workflow-local mutation
- replay queries now read durable `ReplayStore` records rather than in-memory helper timelines
- only `PlannerAgent` and `CashflowAgent` enter the real provider-backed intelligence path in this phase, and only inside Monthly Review
- only Monthly Review currently proves durable-memory influence; this is not yet a claim about every workflow or every agent
- Monthly Review is now the strongest trustworthy-finance path, while Debt vs Invest carries the canonical deterministic approval proof for 5D
- replay/eval/debug is now operator-grade on the same local durable plane, but it is still local-first rather than a full external observability stack
- runtime promotion is now real at the backbone layer: multi-worker-safe claim/lease execution, scheduler wakeups, retry backoff, approval resume enqueue, fencing-token commit protection, and Postgres-backed runtime-authoritative stores all exist behind the same typed runtime contracts
- `runtime-promotion` is now a formal profile: Postgres carries the stronger runtime truth, MinIO carries ref-backed checkpoint/report/replay payloads, and LocalFS/SQLite remain the lighter local profile rather than the only runtime shape
- provider-backed intelligence is now a load-bearing substrate layer rather than workflow-local string prompts: prompts are versioned, render policy is real code rather than dead metadata, context is token-aware at MVP scope, outputs are schema-validated/repaired/fallbacked, and traces include provider/prompt/token/cost/fallback evidence
- deterministic finance truth now lives in Finance Engine metric bundles rather than in scattered helper logic or model text
- behavior-domain execution is now live only through the narrow `behavior_intervention` workflow; it still does not rewrite Monthly Review or Workflow C into behavior-first orchestration.

## Phase 3A / 3B / 4A / 4B / 5A / 5B Highlights

- `internal/protocol` is now execution-first: typed request/result message kinds, oneof-style request/result bodies, and response envelopes participate in real dispatch.
- `internal/agents` now contains a concrete execution plane: registry, dispatcher, executor, system-step bus, typed execution errors, and registered system-agent handlers.
- `MonthlyReviewWorkflow` and `DebtVsInvestWorkflow` no longer directly call planner, memory, domain analysis, verification, governance, or report generation services.
- `ReportAgent` now follows `draft -> verification -> governance -> finalize`; final report artifacts and `report_ready` are not emitted before governance.
- `PlannerAgent` now returns block-level execution plans, and `plan.Blocks` is the only truth source for block order, recipient, requirements, success criteria, and verification hints.
- `MemorySteward` is now load-bearing: retrieved memories influence block ordering and recommendation emphasis instead of being a sidecar retrieval step.
- `CashflowAgent` and `DebtAgent` now consume block-specific execution context and return typed `CashflowBlockResult` / `DebtBlockResult`.
- `VerificationAgent` now validates domain blocks before final report validation and can short-circuit on severe block failures.
- runtime now has an explicit bridge from typed agent failure categories to workflow recovery semantics.
- observability and replay now expose planner block plan, domain block execution order, selected memory/evidence/state slices, and agent dispatch lifecycle records.
- Workflow C now exists as a real `life_event_trigger` path instead of a contract-only placeholder.
- structured `event source` and `calendar/deadline source` adapters now turn life events into typed evidence before workflow execution.
- `TaskGenerationAgent` now generates `TaskSpec`-backed follow-up tasks from validated life-event analysis, state diff, evidence, and retrieved memories without redoing domain analysis.
- runtime now registers generated follow-up tasks into a task graph with explicit statuses such as `dependency_blocked`, `deferred`, `waiting_approval`, and `queued_pending_capability`.
- `LifeEventAssessmentReport` now gives Workflow C a secondary artifact contract, but the primary product of Workflow C remains state/memory/task-graph mutation rather than a narrative report.
- `TaxOptimizationWorkflow` and `PortfolioRebalanceWorkflow` now exist as real follow-up workflow entrypoints behind runtime capability activation.
- runtime now owns follow-up task reevaluation, execution ordering, execution records, committed state handoff, retry metadata, approval resumability metadata, and task-level suppression reasons.
- replay can now explain parent life-event workflow -> generated task -> child workflow -> child artifact -> child state commit as one proactive chain.
- runtime store seams are now explicit (`TaskGraphStore`, `TaskExecutionStore`, `ApprovalStateStore`, `CheckpointStore`, `ReplayStore`) and backed by a local SQLite implementation for restart-resilient operator flows.
- `cmd/api` now exposes minimal operator endpoints for listing task graphs, listing pending approvals, approve/deny, resume/retry, and task-graph reevaluation with explicit `400 / 404 / 409` semantics.
- `cmd/worker` now runs reevaluate / auto-execute / resume / operator-retry passes against the durable runtime instead of remaining a placeholder.
- approval-aware resume is now a real runtime continuation path: approved waiting child workflows resume from persisted checkpoint payloads rather than restarting the whole child workflow from the top.
- `internal/model` now provides a provider-agnostic chat/structured seam with an OpenAI-compatible live adapter and stub adapters for future providers.
- `internal/prompt` now owns versioned prompt registry, rendering, and prompt render trace for `planner.monthly_review.v1` and `cashflow.monthly_review.v1`.
- `internal/structured` now owns schema validation, parse retry, repair retry, deterministic fallback, and trace recording for planner/cashflow structured output.
- `PlannerAgent` no longer needs workflow-local prompt/provider wiring; it can use a provider-backed planner while still emitting the same typed `ExecutionPlan`.
- `CashflowAgent` can now use a provider-backed reasoner while still grounding on deterministic metrics and typed evidence refs.
- context budgeting is now token-aware for planning and cashflow execution, and trace captures what was included, excluded, compacted, and estimated.
- `cmd/eval` is now a real Monthly Review 5B runner rather than a placeholder, and `scripts/run_monthly_review_5b.sh` is the reproducible run-evidence entrypoint.

## Repository Layout

```text
cmd/                 Entrypoints for api, worker, eval, replay
internal/            Core application layers and domain contracts
var/                 Local durable runtime state and artifact refs (generated at runtime)
web/                 React + TypeScript operator console
deployments/         Docker Compose skeleton and deployment notes
schemas/             JSON schemas for core contracts
docs/                Architecture, ADRs, workflows, threat model, eval notes
tests/               Cross-package test notes and fixture inputs
scripts/             Reproducible run helpers, including runtime and product profile launchers
```

## Quick Start

```bash
go test ./...
go vet ./...
go run ./cmd/api --db ./var/runtime.db
go run ./cmd/worker --db ./var/runtime.db --once
go run ./cmd/replay --runtime-db ./var/runtime.db --workflow-id <workflow-id> --format summary
go run ./cmd/replay --runtime-db ./var/runtime.db --rebuild-projections --all
go run ./cmd/eval --mode corpus --corpus phase6a-default --format summary
go run ./cmd/eval --mode corpus --corpus phase6b-default --format summary
./scripts/run_monthly_review_5b.sh mock
./scripts/run_monthly_review_5c.sh mock
./scripts/run_monthly_review_5d.sh mock
./scripts/run_behavior_intervention_6b.sh mock
./scripts/run_interview_demo_7b.sh all
./scripts/run_dev_stack_7b.sh all
./scripts/run_runtime_promotion_7b.sh smoke
```

### Phase 5B Monthly Review Golden Path

Mock mode writes stable run evidence under `docs/eval/samples/`:

```bash
./scripts/run_monthly_review_5b.sh mock
```

Live mode is env-gated and intentionally non-CI:

```bash
export OPENAI_API_KEY=...
export OPENAI_REASONING_MODEL=...
export OPENAI_FAST_MODEL=...
./scripts/run_monthly_review_5b.sh live /tmp/monthly-review-5b
```

Optional transport override env vars:

- `OPENAI_BASE_URL`
- `OPENAI_CHAT_ENDPOINT_PATH`

`OPENAI_REASONING_MODEL` and `OPENAI_FAST_MODEL` are required in live mode. The live provider path no longer bakes in default model names; mock mode remains env-free.

The live smoke test is also env-gated:

```bash
RUN_MONTHLY_REVIEW_5B_LIVE_SMOKE=1 OPENAI_API_KEY=... go test ./internal/app -run TestMonthlyReview5BLiveSmoke -v
```

Mock sample outputs checked into the repo:

- `docs/eval/samples/monthly_review_5b_report.json`
- `docs/eval/samples/monthly_review_5b_trace.json`

### Phase 5C Monthly Review With Durable Memory

Mock mode runs Monthly Review twice against the same injected `memory.db`. The second run is the canonical output and should show memory selection / provenance from the first run:

```bash
./scripts/run_monthly_review_5c.sh mock
```

Live mode is env-gated and additionally requires an embedding model:

```bash
export OPENAI_API_KEY=...
export OPENAI_REASONING_MODEL=...
export OPENAI_FAST_MODEL=...
export OPENAI_EMBEDDING_MODEL=...
./scripts/run_monthly_review_5c.sh live /tmp/monthly-review-5c /tmp/monthly-review-5c/memory.db
```

You can rebuild embeddings and lexical postings for an existing memory database with:

```bash
MEMORY_DB_PATH=./var/memory.db ./scripts/rebuild_memory_index.sh mock
```

Or directly through `cmd/eval`:

```bash
go run ./cmd/eval --phase 5c --provider-mode mock --memory-db ./var/memory.db --reindex-memory --index-only
```

Mock sample outputs checked into the repo:

- `docs/eval/samples/monthly_review_5c_report.json`
- `docs/eval/samples/monthly_review_5c_trace.json`
- `docs/eval/samples/monthly_review_5c_cross_session.json`

The checked-in 5C trace sample now includes both accepted and rejected memory evidence:

- stale episodic rejection
- low-confidence rejection
- selected memories that still influence planner/cashflow output after rejection is applied

### Phase 5D Trustworthy Finance Reasoning

Monthly Review positive path:

```bash
./scripts/run_monthly_review_5d.sh mock
```

Debt vs Invest deterministic approval proof:

```bash
go run ./cmd/eval --phase 5d --workflow debt_vs_invest --provider-mode mock --memory-db ./var/memory-5d.db --artifact-out ./docs/eval/samples/debt_vs_invest_5d_waiting_approval.json
```

Mock sample outputs checked into the repo:

- `docs/eval/samples/monthly_review_5d_report.json`
- `docs/eval/samples/monthly_review_5d_trace.json`
- `docs/eval/samples/debt_vs_invest_5d_waiting_approval.json`
- `docs/eval/samples/debt_vs_invest_5d_waiting_approval_trace.json`

Phase 5D now proves:

- Finance Engine is the current live path's numeric truth source
- grounded recommendations / risk / caveat / approval fields are visible in Monthly Review artifacts
- Debt vs Invest can enter `waiting_approval` deterministically
- trust validators and governance decisions are visible in workflow trace

### Phase 6A Replay / Eval / Debug Evidence

Run the canonical deterministic corpus:

```bash
go run ./cmd/eval --mode corpus --corpus phase6a-default --format summary
```

Replay a waiting-approval workflow:

```bash
go run ./cmd/replay --runtime-db /tmp/personal-cfo-6a-evidence/corpus/debt_vs_invest_waiting_approval/runtime.db --workflow-id workflow-debt-vs-invest-20260329080000 --format summary
```

Rebuild replay/debug projections from authoritative runtime truth:

```bash
go run ./cmd/replay --runtime-db ./var/runtime.db --rebuild-projections --all
```

Stable 6A sample outputs checked into the repo:

- `docs/eval/samples/phase6a_eval_default_corpus.json`
- `docs/eval/samples/phase6a_replay_compare_monthly_review_memory.json`
- `docs/eval/samples/phase6a_replay_debt_vs_invest_waiting_approval.json`
- `docs/eval/samples/phase6a_replay_life_event_task_graph.json`

### Phase 6B Skills + Behavior Evidence

Run the canonical deterministic behavior corpus:

```bash
go run ./cmd/eval --mode corpus --corpus phase6b-default --format summary
```

Generate the checked-in 6B corpus and replay samples:

```bash
./scripts/run_behavior_intervention_6b.sh mock
```

Stable 6B sample outputs checked into the repo:

- `docs/eval/samples/phase6b_eval_default_corpus.json`
- `docs/eval/samples/phase6b_replay_behavior_intervention.json`
- `docs/eval/samples/phase6b_replay_behavior_intervention_waiting_approval.json`
- `docs/eval/samples/phase6b_compare_procedural_memory_skill_selection.json`

### Phase 7A Runtime Promotion Evidence

Bring up the local runtime-promotion profile and regenerate the checked-in proof samples:

```bash
./scripts/run_runtime_promotion_7a.sh proof
```

This profile uses:

- Postgres as the promoted runtime backend
- MinIO as the blob/object store seam for checkpoint/report/replay payload refs
- API + 2 workers against the same durable store

Stable 7A sample outputs checked into the repo:

- `docs/eval/samples/phase7a_runtime_promotion_profile.json`
- `docs/eval/samples/phase7a_async_runtime_proofs.json`

These samples now reflect closeout-hardened promoted-backend proofs instead of only in-memory/runtime-contract evidence.

### Phase 7B Productization / Externalization

7B turns the same kernel into a usable operator surface instead of starting a new logic layer:

```bash
./scripts/run_interview_demo_7b.sh all
./scripts/run_dev_stack_7b.sh all
./scripts/run_runtime_promotion_7b.sh smoke
```

Versioned external legibility now lives at:

- `/api/v1`
- `schemas/public/v1/*`
- `docs/product/operator-ui.md`
- `docs/product/deployment-profiles.md`
- `docs/product/protocol-exposure.md`
- `docs/product/benchmark-reporting.md`

Stable 7B operator-facing sample outputs checked into the repo:

- `docs/eval/samples/phase7b_benchmark_summary.json`
- `docs/eval/samples/phase7b_benchmark_compare.json`
- `docs/eval/samples/phase7b_operator_surface.json`

## What Is Still Stubbed

- agentic document parsing is still a deterministic stub behind a formal adapter boundary
- durable memory now exists for Monthly Review through a local SQLite memory seam, but it is still local-first rather than a stronger remote memory substrate
- semantic retrieval is now real on the Monthly Review path through a true embedding provider seam, but it still uses local brute-force vector scoring rather than ANN / pgvector
- finance reasoning is now hardened on the current live path, but this is still not a full finance engine, market simulator, or tax-law system
- runtime is still Temporal-aligned rather than backed by a live Temporal cluster
- runtime promotion is now real, but only through a local-first `runtime-promotion` profile rather than a fully remote distributed execution fabric
- SQLite + LocalFS still exist as the `local-lite` profile; Postgres + MinIO now exist as stronger store seams, but the system is not yet a fully externalized production deployment
- replay/eval/debug is now queryable and regression-capable on the local durable plane, but it is not yet a full OTel / Prometheus / Grafana / Tempo observability stack
- system agents are currently local synchronous handlers behind a local bus, not remote or durable inbox/outbox actors yet
- repair traces now preserve distinct initial vs repair prompt identity, but that intelligence evidence still only exists on the Monthly Review golden path
- `TaxAgent` and `PortfolioAgent` are only live inside Workflow C; Monthly Review and Debt vs Invest still keep tax/portfolio as deferred or residual sections
- behavior is now formalized, but only through the narrow `behavior_intervention` workflow rather than every existing workflow
- capability-backed follow-up execution is still intentionally narrow: only `tax_optimization` and `portfolio_rebalance` are live child workflow capabilities, and only for first-level auto-execution
- no live Temporal cluster, pgvector-backed memory plane, or remote actor mailbox runtime is required yet
- richer provider/prompt A/B regression, broader replay coverage for blocked/deferred capability cases, fuller observability infra promotion, broader behavior-follow-up rollout, and stronger external protocol / remote-agent seams are still deferred after 7B

These are deliberate trade-offs. The important part is that business logic now talks to stable protocol contracts, typed agent boundaries, and deterministic subsystem services, so replacing the stubbed pieces in later phases does not require rewriting workflow logic or collapsing the 12-layer structure.

## Architecture Priorities

1. Preserve the 12-layer structure so later workflows can plug into stable contracts.
2. Keep financial calculations deterministic and outside of the LLM path.
3. Model runtime, protocol, governance, verification, and memory as first-class surfaces from day one.
4. Use stubs only behind durable interfaces so the system stays interview-defensible and production-shaped.
5. Keep workflows thin; when logic grows, it must move down into reusable subsystems or system-agent handlers rather than staying in orchestration files.
