# Workflow A: Monthly Financial Review

## Current Executable Path

1. Natural language request goes through deterministic intake and becomes a `TaskSpec` with success criteria and required evidence.
2. `MonthlyReviewWorkflow` delegates evidence collection and reducer application to `MonthlyReviewService`, which orchestrates the observation tools.
3. `LedgerObservationAdapter` emits:
   - `transaction_batch`
   - `recurring_subscription_signal`
   - `late_night_spending_signal`
   - `debt_obligation_snapshot`
   - `portfolio_allocation_snapshot`
4. document adapters emit:
   - `payslip_statement`
   - `credit_card_statement`
   - `tax_document`
5. reducers build a deterministic `EvidencePatch` and update `FinancialWorldState`.
6. workflow dispatches `memory_sync_request` to `MemorySteward`, which derives memories, applies write gating, writes durable memory, builds/reads lexical + semantic retrieval state, and retrieves relevant memories through typed planner/cashflow query builders.
7. workflow dispatches `plan_request` to `PlannerAgent`, which now uses token-aware planning context, `planner.monthly_review.v1`, an applied prompt render policy, provider-backed structured generation, schema validation, repair/fallback, and then returns a block-level `ExecutionPlan`.
8. `plan.Blocks` remains the only execution truth source; workflow iterates it in order instead of rebuilding structure from intent.
9. for each block, workflow assembles block-specific execution context and dispatches:
   - `cashflow_review_block` -> `CashflowAgent`
   - `debt_review_block` -> `DebtAgent`
10. `CashflowAgent` now uses token-aware execution context, `cashflow.monthly_review.v1`, an applied prompt render policy, provider-backed structured generation, schema validation, grounding pre-check, narrow numeric-consistency validation, repair/fallback, and then merges the typed result back onto deterministic cashflow metrics.
11. `DebtAgent` remains deterministic in this phase.
12. workflow dispatches `report_draft_request` to `ReportAgent`, which aggregates typed domain block results and residual deterministic sections into a draft, but does not emit a final artifact yet.
13. workflow dispatches `verification_request` to `VerificationAgent`, which runs block-level validation first, including structured-output and grounding checks for planner/cashflow, then only runs final report validation if no severe block failure is found.
14. workflow dispatches `governance_evaluation_request` to `GovernanceAgent`, which evaluates risk, approval, and report disclosure.
15. only after governance allows or redacts does workflow dispatch `report_finalize_request` back to `ReportAgent` to produce the final artifact and `report_ready`.
16. runtime then decides whether the workflow completes, replans, or pauses for approval.
17. trace dump now includes prompt version, repair prompt identity, provider call, token usage, estimated cost, repair/fallback, structured-output trace, memory query, hit/reject/select, and embedding usage alongside the existing workflow/runtime observability surface.

## Structural Boundary After Remediation

- workflow file: orchestration only
- workflow service: evidence collection + reducer orchestration
- planner agent: planning context assembly + prompt render + provider-backed structured planning + typed plan compile
- memory steward: derived memory generation + gating + durable write + hybrid retrieval
- cashflow agent: typed cashflow block analysis using deterministic metrics, selected evidence, provider-backed structured reasoning, and deterministic fallback
- debt agent: typed debt block analysis using deterministic metrics and selected evidence
- report agent: aggregator + finalize split with governance-aware finalization
- verification agent: block + final validation pipeline with structured-output/grounding checks and short-circuit on severe block failures
- governance agent: reusable approval / disclosure evaluation
- runtime subsystem: checkpoint / replan / approval pause semantics
- memory subsystem: durable store + embedding provider + lexical retriever + semantic retriever + fusion + rejection + trace
- protocol layer: typed request/result envelopes with correlation/causation chain
- prompt system: versioned prompt registry, rendering, and prompt render trace
- model layer: provider-agnostic chat/structured seam with OpenAI-compatible live adapter
- structured output layer: parser / validator / repair / fallback / trace for planner and cashflow, with repair attempts preserved as distinct traceable generations

## Current Artifacts

- monthly review draft payload
- monthly review final report artifact
- planner block plan snapshot
- cashflow block result
- debt block result
- workflow checkpoint journal
- workflow timeline entries
- agent dispatch lifecycle records
- prompt render traces
- provider call traces
- usage / cost traces
- structured output traces
- memory access audit entries
- memory query traces
- memory retrieval traces
- memory selection traces
- embedding call / usage traces
- policy decision audit entries
- replay-ready trace dump inputs

## Still Stubbed

- agentic tax parsing is still a deterministic stub behind a formal adapter
- timeline is currently local structured dump state, not a remote tracing backend
- only `PlannerAgent` and `CashflowAgent` are on the real provider-backed path in this phase; `DebtAgent` stays deterministic, and debt/tax/portfolio/behavior are not being “half-upgraded”
- portfolio / tax / behavior areas are still residual deterministic sections, not yet domain-agent-executed inside Monthly Review
- agent execution is local synchronous dispatch, not yet durable remote actor execution
- full finance-engine hardening and full replay/eval-plane maturation are intentionally deferred to later phases

These stubs do not change the workflow shape: the system is already evidence-first, stateful, context-engineered, verifiable, governed, and now includes first real load-bearing domain execution.
