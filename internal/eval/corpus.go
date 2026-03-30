package eval

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/app"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

func DefaultScenarioCorpus() ScenarioCorpus {
	return ScenarioCorpus{
		ID:                "phase6a-default",
		DeterministicOnly: true,
		Cases: []ScenarioCase{
			{
				ID:            "monthly_review_happy_path",
				Category:      "monthly_review",
				Description:   "Monthly Review happy path with completed report and replay query by workflow",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState: "completed",
					ExpectedScopeKind:  "workflow",
				},
				run: runMonthlyReviewHappyPath,
			},
			{
				ID:            "monthly_review_cross_session_memory_influence",
				Category:      "monthly_review",
				Description:   "Monthly Review cross-session memory influence with replay comparison between two workflow runs",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState:           "completed",
					ExpectedScopeKind:            "workflow",
					RequireComparison:            true,
					RequiredComparisonCategories: []string{"memory"},
				},
				run: runMonthlyReviewCrossSessionMemoryInfluence,
			},
			{
				ID:            "monthly_review_memory_rejection_visibility",
				Category:      "monthly_review",
				Description:   "Monthly Review deterministic memory rejection scenario exposes selected and rejected memory reasons through replay/debug surfaces",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState:           "completed",
					ExpectedScopeKind:            "workflow",
					RequireComparison:            true,
					RequiredComparisonCategories: []string{"memory"},
				},
				run: runMonthlyReviewMemoryRejectionVisibility,
			},
			{
				ID:            "monthly_review_trust_validator_failure",
				Category:      "monthly_review",
				Description:   "Monthly Review trust validation failure path ending in failed runtime state",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState:        "failed",
					ExpectedScopeKind:         "workflow",
					RequireFailureExplanation: true,
					RequireValidatorSummary:   true,
				},
				run: runMonthlyReviewTrustFailure,
			},
			{
				ID:            "debt_vs_invest_waiting_approval",
				Category:      "debt_vs_invest",
				Description:   "Debt vs Invest deterministic waiting_approval path with approval metadata and policy provenance",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState:      "waiting_approval",
					ExpectedScopeKind:       "workflow",
					RequireApproval:         true,
					RequiredProvenanceEdges: []string{"requested_approval"},
				},
				run: runDebtVsInvestWaitingApproval,
			},
			{
				ID:            "debt_vs_invest_fail",
				Category:      "debt_vs_invest",
				Description:   "Debt vs Invest trust failure path ending in failed runtime state",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState:        "failed",
					ExpectedScopeKind:         "workflow",
					RequireFailureExplanation: true,
					RequireValidatorSummary:   true,
				},
				run: runDebtVsInvestFailure,
			},
			{
				ID:            "life_event_generated_follow_up_tasks",
				Category:      "life_event",
				Description:   "Life event workflow generates and executes capability-backed follow-up task graph",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState:      "completed",
					ExpectedScopeKind:       "task_graph",
					MinimumChildWorkflows:   2,
					RequiredProvenanceEdges: []string{"generated_task", "triggered_child_workflow"},
				},
				run: runLifeEventGeneratedFollowUps,
			},
			{
				ID:            "tax_child_workflow_happy_path",
				Category:      "tax_follow_up",
				Description:   "Tax child workflow completes and can be replayed by task id",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState: "completed",
					ExpectedScopeKind:  "task",
				},
				run: runTaxChildWorkflowHappyPath,
			},
			{
				ID:            "portfolio_child_workflow_happy_path",
				Category:      "portfolio_follow_up",
				Description:   "Portfolio child workflow completes and can be replayed by execution id",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState: "completed",
					ExpectedScopeKind:  "execution",
				},
				run: runPortfolioChildWorkflowHappyPath,
			},
			{
				ID:            "retry_and_reevaluate",
				Category:      "runtime_recovery",
				Description:   "Reevaluate a queued task graph and complete execution through retry recovery",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState: "completed",
					ExpectedScopeKind:  "task_graph",
				},
				run: runRetryAndReevaluate,
			},
			{
				ID:            "parent_child_provenance_reconstruction",
				Category:      "provenance",
				Description:   "Replay reconstructs parent workflow to child workflow, artifact, and state chain",
				Deterministic: true,
				Expectation: ScenarioExpectation{
					ExpectedFinalState:      "completed",
					ExpectedScopeKind:       "task_graph",
					MinimumChildWorkflows:   2,
					RequiredProvenanceEdges: []string{"generated_task", "triggered_child_workflow", "produced_artifact", "committed_state"},
				},
				run: runParentChildProvenanceReconstruction,
			},
		},
	}
}

func runMonthlyReviewHappyPath(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	env, err := openMockPhase5DEnvironment(runCtx, "holdings_2026-03-safe.csv", nil)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = env.Close() }()
	result, err := env.RunMonthlyReview(ctx, "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	view, err := env.ReplayQuery.Query(ctx, observability.ReplayQuery{WorkflowID: result.Result.WorkflowID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	return scenarioRunOutput{
		RuntimeState:       result.Result.RuntimeState,
		WorkflowID:         result.Result.WorkflowID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		TokenUsage:         traceTokenUsage(result.Trace),
		EvidenceComplete:   result.Result.CoverageReport.CoverageRatio >= 1,
		ChildWorkflowCount: len(view.Summary.ChildWorkflowSummary),
	}, nil
}

func runMonthlyReviewCrossSessionMemoryInfluence(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	env1, err := openMockPhase5DEnvironment(runCtx, "holdings_2026-03-safe.csv", nil)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	first, err := env1.RunMonthlyReview(ctx, "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		_ = env1.Close()
		return scenarioRunOutput{}, err
	}
	if err := env1.Close(); err != nil {
		return scenarioRunOutput{}, err
	}

	secondRunCtx := runCtx
	secondRunCtx.Now = runCtx.Now.Add(time.Second)
	env2, err := openMockPhase5DEnvironment(secondRunCtx, "holdings_2026-03-safe.csv", nil)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = env2.Close() }()
	second, err := env2.RunMonthlyReview(ctx, "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	view, err := env2.ReplayQuery.Query(ctx, observability.ReplayQuery{WorkflowID: second.Result.WorkflowID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	comparison, err := env2.ReplayQuery.Compare(ctx,
		observability.ReplayQuery{WorkflowID: first.Result.WorkflowID},
		observability.ReplayQuery{WorkflowID: second.Result.WorkflowID},
	)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	return scenarioRunOutput{
		RuntimeState:       second.Result.RuntimeState,
		WorkflowID:         second.Result.WorkflowID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		Comparison:         &comparison,
		TokenUsage:         traceTokenUsage(second.Trace),
		EvidenceComplete:   second.Result.CoverageReport.CoverageRatio >= 1,
		ChildWorkflowCount: len(view.Summary.ChildWorkflowSummary),
	}, nil
}

func runMonthlyReviewMemoryRejectionVisibility(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	env1, err := openMockPhase5DEnvironment(runCtx, "holdings_2026-03-safe.csv", nil)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	first, err := env1.RunMonthlyReview(ctx, "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		_ = env1.Close()
		return scenarioRunOutput{}, err
	}
	stale := memory.MemoryRecord{
		ID:      "memory-eval-stale-episodic",
		Kind:    memory.MemoryKindEpisodic,
		Summary: "subscription subscription subscription cleanup reminder from a much older review",
		Facts: []memory.MemoryFact{
			{Key: "duplicate_subscription_count", Value: "2", EvidenceID: observation.EvidenceID("evidence-subscription")},
		},
		Source: memory.MemorySource{
			EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-subscription")},
			TaskID:      "task-eval-stale",
			WorkflowID:  "workflow-eval-stale",
			TraceID:     "trace-eval-stale",
			Actor:       "memory_steward",
		},
		Confidence: memory.MemoryConfidence{Score: 0.92, Rationale: "old deterministic eval memory"},
		CreatedAt:  time.Date(2025, 11, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2025, 11, 1, 8, 0, 0, 0, time.UTC),
	}
	lowConfidence := memory.MemoryRecord{
		ID:      "memory-eval-low-confidence",
		Kind:    memory.MemoryKindSemantic,
		Summary: "subscription subscription recurring cashflow cleanup should maybe stay a recommendation priority",
		Facts: []memory.MemoryFact{
			{Key: "duplicate_subscription_count", Value: "2", EvidenceID: observation.EvidenceID("evidence-subscription")},
		},
		Source: memory.MemorySource{
			EvidenceIDs: []observation.EvidenceID{observation.EvidenceID("evidence-subscription")},
			TaskID:      "task-eval-low-confidence",
			WorkflowID:  "workflow-eval-low-confidence",
			TraceID:     "trace-eval-low-confidence",
			Actor:       "memory_steward",
		},
		Confidence: memory.MemoryConfidence{Score: 0.42, Rationale: "intentionally weak deterministic eval memory"},
		CreatedAt:  time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC),
	}
	for _, record := range []memory.MemoryRecord{stale, lowConfidence} {
		if err := env1.MemoryStores.Store.Put(ctx, record); err != nil {
			_ = env1.Close()
			return scenarioRunOutput{}, err
		}
	}
	if _, err := env1.RebuildMemoryIndexes(ctx); err != nil {
		_ = env1.Close()
		return scenarioRunOutput{}, err
	}
	if err := env1.Close(); err != nil {
		return scenarioRunOutput{}, err
	}

	secondRunCtx := runCtx
	secondRunCtx.Now = runCtx.Now.Add(24 * time.Hour)
	env2, err := openMockPhase5DEnvironment(secondRunCtx, "holdings_2026-03-safe.csv", nil)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = env2.Close() }()
	second, err := env2.RunMonthlyReview(ctx, "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	view, err := env2.ReplayQuery.Query(ctx, observability.ReplayQuery{WorkflowID: second.Result.WorkflowID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	comparison, err := env2.ReplayQuery.Compare(ctx,
		observability.ReplayQuery{WorkflowID: first.Result.WorkflowID},
		observability.ReplayQuery{WorkflowID: second.Result.WorkflowID},
	)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	if !containsReplayLine(view.Summary.MemorySummary, "rejection_rule=") && !containsReplayLine(view.Explanation.WhyMemoryDecision, "memory rejected by") {
		return scenarioRunOutput{}, fmt.Errorf("expected replay surface to expose memory rejection visibility, got summary=%v explanation=%v", view.Summary.MemorySummary, view.Explanation.WhyMemoryDecision)
	}
	return scenarioRunOutput{
		RuntimeState:       second.Result.RuntimeState,
		WorkflowID:         second.Result.WorkflowID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		Comparison:         &comparison,
		TokenUsage:         traceTokenUsage(second.Trace),
		EvidenceComplete:   second.Result.CoverageReport.CoverageRatio >= 1,
		ChildWorkflowCount: len(view.Summary.ChildWorkflowSummary),
	}, nil
}

func runMonthlyReviewTrustFailure(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	env, err := openMockPhase5DEnvironment(runCtx, "holdings_2026-03-safe.csv", func(base verification.Pipeline) verification.Pipeline {
		base.GroundingValidator = forcedTrustFailureValidator{
			validator: "forced_monthly_review_grounding_failure",
			code:      "forced_monthly_review_trust_failure",
			message:   "deterministic eval grounding failure for monthly review",
		}
		return base
	})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = env.Close() }()
	result, err := env.RunMonthlyReview(ctx, "user-1", "请帮我做一份月度财务复盘", state.FinancialWorldState{})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	view, err := env.ReplayQuery.Query(ctx, observability.ReplayQuery{WorkflowID: result.Result.WorkflowID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	return scenarioRunOutput{
		RuntimeState:       result.Result.RuntimeState,
		WorkflowID:         result.Result.WorkflowID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		TokenUsage:         traceTokenUsage(result.Trace),
		EvidenceComplete:   result.Result.CoverageReport.CoverageRatio >= 1,
		ChildWorkflowCount: len(view.Summary.ChildWorkflowSummary),
	}, nil
}

func runDebtVsInvestWaitingApproval(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	env, err := openMockPhase5DEnvironment(runCtx, "holdings_2026-03-safe.csv", nil)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = env.Close() }()
	result, err := env.RunDebtVsInvest(ctx, "user-1", "提前还贷还是继续投资更合适", state.FinancialWorldState{})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	if result.Result.PendingApproval == nil {
		return scenarioRunOutput{}, fmt.Errorf("expected deterministic approval path to populate pending approval")
	}
	view, err := env.ReplayQuery.Query(ctx, observability.ReplayQuery{WorkflowID: result.Result.WorkflowID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	return scenarioRunOutput{
		RuntimeState:       result.Result.RuntimeState,
		WorkflowID:         result.Result.WorkflowID,
		ApprovalID:         result.Result.PendingApproval.ApprovalID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		TokenUsage:         traceTokenUsage(result.Trace),
		EvidenceComplete:   result.Result.CoverageReport.CoverageRatio >= 1,
		ChildWorkflowCount: len(view.Summary.ChildWorkflowSummary),
	}, nil
}

func runDebtVsInvestFailure(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	env, err := openMockPhase5DEnvironment(runCtx, "holdings_2026-03-safe.csv", func(base verification.Pipeline) verification.Pipeline {
		base.GroundingValidator = forcedTrustFailureValidator{
			validator: "forced_debt_decision_grounding_failure",
			code:      "forced_debt_decision_trust_failure",
			message:   "deterministic eval grounding failure for debt-vs-invest",
		}
		return base
	})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = env.Close() }()
	result, err := env.RunDebtVsInvest(ctx, "user-1", "提前还贷还是继续投资更合适", state.FinancialWorldState{})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	view, err := env.ReplayQuery.Query(ctx, observability.ReplayQuery{WorkflowID: result.Result.WorkflowID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	return scenarioRunOutput{
		RuntimeState:       result.Result.RuntimeState,
		WorkflowID:         result.Result.WorkflowID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		TokenUsage:         traceTokenUsage(result.Trace),
		EvidenceComplete:   result.Result.CoverageReport.CoverageRatio >= 1,
		ChildWorkflowCount: len(view.Summary.ChildWorkflowSummary),
	}, nil
}

func runLifeEventGeneratedFollowUps(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	plane, result, err := runSalaryFollowUpGraphScenario(ctx, runCtx)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = plane.Close() }()
	view, err := plane.ReplayQuery.Query(ctx, observability.ReplayQuery{TaskGraphID: result.TaskGraphID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	return scenarioRunOutput{
		RuntimeState:       runtime.WorkflowExecutionState(view.Summary.FinalState),
		WorkflowID:         result.WorkflowID,
		TaskGraphID:        result.TaskGraphID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		EvidenceComplete:   true,
		ChildWorkflowCount: len(view.Summary.ChildWorkflowSummary),
	}, nil
}

func runTaxChildWorkflowHappyPath(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	plane, result, err := runSalaryFollowUpGraphScenario(ctx, runCtx)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = plane.Close() }()
	task := findFollowUpTaskByIntent(result.RegisteredTasks, taskspec.UserIntentTaxOptimization)
	if task == nil {
		return scenarioRunOutput{}, fmt.Errorf("expected generated tax follow-up task")
	}
	view, err := plane.ReplayQuery.Query(ctx, observability.ReplayQuery{TaskID: task.Task.ID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	return scenarioRunOutput{
		RuntimeState:       runtime.WorkflowExecutionState(view.Summary.FinalState),
		WorkflowID:         result.WorkflowID,
		TaskGraphID:        result.TaskGraphID,
		TaskID:             task.Task.ID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		EvidenceComplete:   true,
		ChildWorkflowCount: 1,
	}, nil
}

func runPortfolioChildWorkflowHappyPath(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	plane, result, err := runSalaryFollowUpGraphScenario(ctx, runCtx)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = plane.Close() }()
	task := findFollowUpTaskByIntent(result.RegisteredTasks, taskspec.UserIntentPortfolioRebalance)
	if task == nil {
		return scenarioRunOutput{}, fmt.Errorf("expected generated portfolio follow-up task")
	}
	execution := findExecutionByTaskID(result.ExecutedTasks, task.Task.ID)
	if execution == nil {
		return scenarioRunOutput{}, fmt.Errorf("expected executed portfolio follow-up task")
	}
	view, err := plane.ReplayQuery.Query(ctx, observability.ReplayQuery{ExecutionID: execution.ExecutionID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	return scenarioRunOutput{
		RuntimeState:       runtime.WorkflowExecutionState(view.Summary.FinalState),
		WorkflowID:         result.WorkflowID,
		TaskGraphID:        result.TaskGraphID,
		TaskID:             task.Task.ID,
		ExecutionID:        execution.ExecutionID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		EvidenceComplete:   true,
		ChildWorkflowCount: 1,
	}, nil
}

func runRetryAndReevaluate(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	stores, err := runtime.NewSQLiteRuntimeStores(filepath.Join(runCtx.WorkDir, "retry_reevaluate_runtime.db"))
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = stores.DB.Close() }()
	service := runtime.NewService(runtime.ServiceOptions{
		CheckpointStore: stores.Checkpoints,
		TaskGraphs:      stores.TaskGraphs,
		Executions:      stores.Executions,
		Approvals:       stores.Approvals,
		OperatorActions: stores.OperatorActions,
		Replay:          stores.Replay,
		Artifacts:       stores.Artifacts,
		Controller:      runtime.DefaultWorkflowController{},
		Now:             func() time.Time { return runCtx.Now },
	})
	rebuilder := runtime.NewReplayProjectionRebuilder(service, stores.WorkflowRuns, stores.ReplayProjection, stores.Artifacts, stores.Replay, func() time.Time { return runCtx.Now })
	service.SetReplayProjectionWriter(rebuilder)
	queryService := runtime.NewReplayQueryService(service, stores.WorkflowRuns, stores.ReplayQuery, stores.Artifacts, stores.Replay)

	graph := scenarioTaskGraph(runCtx.Now, scenarioGeneratedTask(runCtx.Now, "task-tax-retry-eval", taskspec.UserIntentTaxOptimization, 1))
	base := scenarioBaseState(runCtx.Now)
	execCtx := runtime.ExecutionContext{
		WorkflowID:    graph.ParentWorkflowID,
		TaskID:        graph.ParentTaskID,
		CorrelationID: graph.ParentWorkflowID,
		Attempt:       1,
	}
	service.SetCapabilities(runtime.StaticTaskCapabilityResolver{})
	if _, err := service.Runtime().RegisterFollowUpTasks(execCtx, graph, base); err != nil {
		return scenarioRunOutput{}, err
	}
	if _, err := rebuilder.RebuildTaskGraph(ctx, graph.GraphID); err != nil {
		return scenarioRunOutput{}, err
	}

	attempts := 0
	service.SetCapabilities(runtime.StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
		},
		Workflows: map[taskspec.UserIntentType]runtime.FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: evalFollowUpCapability{
				name: "tax_optimization_workflow",
				execute: func(_ context.Context, spec taskspec.TaskSpec, _ runtime.FollowUpActivationContext, current state.FinancialWorldState) (runtime.FollowUpWorkflowRunResult, error) {
					attempts++
					if attempts == 1 {
						return runtime.FollowUpWorkflowRunResult{}, &runtime.FollowUpExecutionError{
							Category: runtime.FailureCategoryTransient,
							Summary:  "deterministic transient retry path",
							Err:      fmt.Errorf("retry once"),
						}
					}
					return completedScenarioFollowUpResult("workflow-child-"+spec.ID, current, runCtx.Now), nil
				},
			},
		},
	})
	if _, _, err := service.ReevaluateTaskGraph(ctx, runtime.ReevaluateTaskGraphCommand{
		RequestID: "eval-reevaluate-1",
		GraphID:   graph.GraphID,
		Actor:     "eval",
		Roles:     []string{"system"},
		Note:      "deterministic reevaluate before retry execution",
	}); err != nil {
		return scenarioRunOutput{}, err
	}
	if _, err := service.ExecuteAutoReadyFollowUps(ctx, graph.GraphID, runtime.DefaultAutoExecutionPolicy()); err != nil {
		return scenarioRunOutput{}, err
	}
	view, err := queryService.Query(ctx, observability.ReplayQuery{TaskGraphID: graph.GraphID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	return scenarioRunOutput{
		RuntimeState:       runtime.WorkflowStateCompleted,
		TaskGraphID:        graph.GraphID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		EvidenceComplete:   true,
		ChildWorkflowCount: len(view.Summary.ChildWorkflowSummary),
	}, nil
}

func runParentChildProvenanceReconstruction(ctx context.Context, runCtx ScenarioRunContext) (scenarioRunOutput, error) {
	plane, result, err := runSalaryFollowUpGraphScenario(ctx, runCtx)
	if err != nil {
		return scenarioRunOutput{}, err
	}
	defer func() { _ = plane.Close() }()
	view, err := plane.ReplayQuery.Query(ctx, observability.ReplayQuery{TaskGraphID: result.TaskGraphID})
	if err != nil {
		return scenarioRunOutput{}, err
	}
	return scenarioRunOutput{
		RuntimeState:       runtime.WorkflowExecutionState(view.Summary.FinalState),
		WorkflowID:         result.WorkflowID,
		TaskGraphID:        result.TaskGraphID,
		Replay:             view,
		DebugSummary:       observability.BuildDebugSummaryFromReplay(view),
		EvidenceComplete:   true,
		ChildWorkflowCount: len(view.Summary.ChildWorkflowSummary),
	}, nil
}

func openMockPhase5DEnvironment(
	runCtx ScenarioRunContext,
	holdingsFixture string,
	override func(base verification.Pipeline) verification.Pipeline,
) (*app.Phase5DEnvironment, error) {
	return app.OpenPhase5DEnvironment(app.Phase5DOptions{
		FixtureDir:      defaultFixtureDir(runCtx),
		HoldingsFixture: holdingsFixture,
		MemoryDBPath:    filepath.Join(runCtx.WorkDir, "memory.db"),
		RuntimeDBPath:   filepath.Join(runCtx.WorkDir, "runtime.db"),
		EmbeddingModel:  "mock-embedding-model",
		Now:             func() time.Time { return runCtx.Now.UTC() },
		ChatModelFactory: func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel {
			return app.NewMockMonthlyReviewChatModelWithTrace(callRecorder, usageRecorder)
		},
		EmbeddingProviderFactory: func(callRecorder memory.EmbeddingCallRecorder, usageRecorder memory.EmbeddingUsageRecorder) memory.EmbeddingProvider {
			return app.NewMockMonthlyReviewEmbeddingProvider(callRecorder, usageRecorder)
		},
		VerificationPipelineOverride: override,
	})
}

type followUpGraphScenarioResult struct {
	WorkflowID      string
	TaskGraphID     string
	RegisteredTasks []runtime.FollowUpTaskRecord
	ExecutedTasks   []runtime.TaskExecutionRecord
}

func runSalaryFollowUpGraphScenario(ctx context.Context, runCtx ScenarioRunContext) (*app.RuntimePlane, followUpGraphScenarioResult, error) {
	plane, err := app.OpenRuntimePlane(app.RuntimePlaneOptions{
		DBPath:     filepath.Join(runCtx.WorkDir, "life_event_follow_up_runtime.db"),
		FixtureDir: defaultFixtureDir(runCtx),
		Now:        func() time.Time { return runCtx.Now.UTC() },
	})
	if err != nil {
		return nil, followUpGraphScenarioResult{}, err
	}

	plane.Service.SetCapabilities(runtime.StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization:    "tax_optimization_workflow",
			taskspec.UserIntentPortfolioRebalance: "portfolio_rebalance_workflow",
		},
		Workflows: map[taskspec.UserIntentType]runtime.FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: evalFollowUpCapability{
				name: "tax_optimization_workflow",
				execute: func(_ context.Context, spec taskspec.TaskSpec, _ runtime.FollowUpActivationContext, current state.FinancialWorldState) (runtime.FollowUpWorkflowRunResult, error) {
					return completedScenarioFollowUpResultWithArtifact(
						"workflow-child-tax-"+spec.ID,
						spec.ID,
						reporting.ArtifactKindTaxOptimizationReport,
						"deterministic tax follow-up artifact",
						current,
						runCtx.Now,
					), nil
				},
			},
			taskspec.UserIntentPortfolioRebalance: evalFollowUpCapability{
				name: "portfolio_rebalance_workflow",
				execute: func(_ context.Context, spec taskspec.TaskSpec, _ runtime.FollowUpActivationContext, current state.FinancialWorldState) (runtime.FollowUpWorkflowRunResult, error) {
					return completedScenarioFollowUpResultWithArtifact(
						"workflow-child-portfolio-"+spec.ID,
						spec.ID,
						reporting.ArtifactKindPortfolioRebalanceReport,
						"deterministic portfolio follow-up artifact",
						current,
						runCtx.Now,
					), nil
				},
			},
		},
	})

	event := scenarioSalaryChangeEvent(runCtx.Now)
	deadline := scenarioSalaryDeadline(runCtx.Now, event)
	workflowID := "workflow-life-event-eval-" + runCtx.Now.UTC().Format("20060102150405")
	parentTaskID := "task-life-event-eval"
	graph := scenarioLifeEventTaskGraph(runCtx.Now, workflowID, parentTaskID,
		scenarioGeneratedTaskFromLifeEvent(runCtx.Now, workflowID, parentTaskID, "task-tax-follow-up", taskspec.UserIntentTaxOptimization, 1, event, deadline),
		scenarioGeneratedTaskFromLifeEvent(runCtx.Now, workflowID, parentTaskID, "task-portfolio-follow-up", taskspec.UserIntentPortfolioRebalance, 1, event, deadline),
	)
	execCtx := runtime.ExecutionContext{
		WorkflowID:    workflowID,
		TaskID:        parentTaskID,
		CorrelationID: workflowID,
		Attempt:       1,
	}
	activation, err := plane.Service.Runtime().RegisterFollowUpTasks(execCtx, graph, scenarioBaseState(runCtx.Now))
	if err != nil {
		_ = plane.Close()
		return nil, followUpGraphScenarioResult{}, err
	}
	batch, err := plane.Service.ExecuteAutoReadyFollowUps(ctx, graph.GraphID, runtime.DefaultAutoExecutionPolicy())
	if err != nil {
		_ = plane.Close()
		return nil, followUpGraphScenarioResult{}, err
	}
	if plane.ReplayRebuilder != nil {
		if _, err := plane.ReplayRebuilder.RebuildTaskGraph(ctx, graph.GraphID); err != nil {
			_ = plane.Close()
			return nil, followUpGraphScenarioResult{}, err
		}
	}
	return plane, followUpGraphScenarioResult{
		WorkflowID:      workflowID,
		TaskGraphID:     graph.GraphID,
		RegisteredTasks: activation.RegisteredTasks,
		ExecutedTasks:   batch.ExecutedTasks,
	}, nil
}

type evalFollowUpCapability struct {
	name    string
	execute func(ctx context.Context, spec taskspec.TaskSpec, activation runtime.FollowUpActivationContext, current state.FinancialWorldState) (runtime.FollowUpWorkflowRunResult, error)
	resume  func(ctx context.Context, spec taskspec.TaskSpec, activation runtime.FollowUpActivationContext, current state.FinancialWorldState, checkpoint runtime.CheckpointRecord, token runtime.ResumeToken, payload runtime.CheckpointPayloadEnvelope) (runtime.FollowUpWorkflowRunResult, error)
}

func (c evalFollowUpCapability) CapabilityName() string { return c.name }

func (c evalFollowUpCapability) Execute(
	ctx context.Context,
	spec taskspec.TaskSpec,
	activation runtime.FollowUpActivationContext,
	current state.FinancialWorldState,
) (runtime.FollowUpWorkflowRunResult, error) {
	return c.execute(ctx, spec, activation, current)
}

func (c evalFollowUpCapability) Resume(
	ctx context.Context,
	spec taskspec.TaskSpec,
	activation runtime.FollowUpActivationContext,
	current state.FinancialWorldState,
	checkpoint runtime.CheckpointRecord,
	token runtime.ResumeToken,
	payload runtime.CheckpointPayloadEnvelope,
) (runtime.FollowUpWorkflowRunResult, error) {
	if c.resume != nil {
		return c.resume(ctx, spec, activation, current, checkpoint, token, payload)
	}
	if c.execute != nil {
		return c.execute(ctx, spec, activation, current)
	}
	return runtime.FollowUpWorkflowRunResult{}, nil
}

type forcedTrustFailureValidator struct {
	validator string
	code      string
	message   string
}

func (v forcedTrustFailureValidator) Validate(
	_ context.Context,
	spec taskspec.TaskSpec,
	_ state.FinancialWorldState,
	_ []observation.EvidenceRecord,
	_ []memory.MemoryRecord,
	_ contextview.BlockVerificationContext,
	_ any,
) ([]verification.VerificationResult, error) {
	now := time.Now().UTC()
	return []verification.VerificationResult{
		{
			Status:    verification.VerificationStatusFail,
			Scope:     verification.VerificationScopeFinal,
			Validator: v.validator,
			Message:   v.message,
			Category:  verification.ValidationCategoryGrounding,
			Severity:  string(verification.ValidationSeverityCritical),
			Diagnostics: []verification.ValidationDiagnostic{
				{
					Code:     v.code,
					Category: verification.ValidationCategoryGrounding,
					Severity: verification.ValidationSeverityCritical,
					Message:  v.message,
				},
			},
			EvidenceCoverage: verification.EvidenceCoverageReport{TaskID: spec.ID},
			CheckedAt:        now,
		},
	}, nil
}

func defaultFixtureDir(runCtx ScenarioRunContext) string {
	if runCtx.FixtureDir != "" {
		return runCtx.FixtureDir
	}
	return filepath.Join("tests", "fixtures")
}

func traceTokenUsage(trace observability.WorkflowTraceDump) int {
	total := 0
	for _, item := range trace.Usage {
		total += item.TotalTokens
	}
	return total
}

func containsReplayLine(lines []string, needle string) bool {
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}

func scenarioBaseState(now time.Time) state.FinancialWorldState {
	return state.FinancialWorldState{
		UserID: "user-1",
		Version: state.StateVersion{
			Sequence:   1,
			SnapshotID: "state-v1",
			UpdatedAt:  now.UTC(),
		},
	}
}

func scenarioTaskGraph(now time.Time, tasks ...taskspec.GeneratedTaskSpec) taskspec.TaskGraph {
	return taskspec.TaskGraph{
		GraphID:           "graph-eval-" + now.UTC().Format("20060102150405"),
		ParentWorkflowID:  "workflow-life-event-eval",
		ParentTaskID:      "task-life-event-eval",
		TriggerSource:     taskspec.TaskTriggerSourceLifeEvent,
		GeneratedAt:       now.UTC(),
		GeneratedTasks:    tasks,
		GenerationSummary: "deterministic eval task graph",
	}
}

func scenarioLifeEventTaskGraph(now time.Time, workflowID string, parentTaskID string, tasks ...taskspec.GeneratedTaskSpec) taskspec.TaskGraph {
	return taskspec.TaskGraph{
		GraphID:           "graph-life-event-eval-" + now.UTC().Format("20060102150405"),
		ParentWorkflowID:  workflowID,
		ParentTaskID:      parentTaskID,
		TriggerSource:     taskspec.TaskTriggerSourceLifeEvent,
		GeneratedAt:       now.UTC(),
		GeneratedTasks:    tasks,
		GenerationSummary: "deterministic eval life-event follow-up task graph",
	}
}

func scenarioGeneratedTask(now time.Time, taskID string, intent taskspec.UserIntentType, depth int) taskspec.GeneratedTaskSpec {
	return taskspec.GeneratedTaskSpec{
		Task: taskspec.TaskSpec{
			ID:    taskID,
			Goal:  "deterministic eval follow-up task",
			Scope: taskspec.TaskScope{Areas: []string{"finance"}},
			Constraints: taskspec.ConstraintSet{
				Hard: []string{"must remain grounded"},
			},
			RiskLevel: taskspec.RiskLevelMedium,
			SuccessCriteria: []taskspec.SuccessCriteria{
				{ID: "done", Description: "complete deterministic follow-up"},
			},
			RequiredEvidence: []taskspec.RequiredEvidenceRef{
				{Type: "event_signal", Reason: "generated from deterministic eval scenario", Mandatory: true},
			},
			ApprovalRequirement: taskspec.ApprovalRequirementRecommended,
			UserIntentType:      intent,
			CreatedAt:           now.UTC(),
		},
		Metadata: taskspec.GeneratedTaskMetadata{
			GeneratedAt:       now.UTC(),
			ParentWorkflowID:  "workflow-life-event-eval",
			ParentTaskID:      "task-life-event-eval",
			RootCorrelationID: "workflow-life-event-eval",
			TriggerSource:     taskspec.TaskTriggerSourceLifeEvent,
			Priority:          taskspec.TaskPriorityHigh,
			ExecutionDepth:    depth,
			GenerationReasons: []taskspec.TaskGenerationReason{
				{
					Code:          taskspec.TaskGenerationReasonLifeEventImpact,
					Description:   "deterministic eval follow-up generation",
					LifeEventID:   "event-eval-1",
					LifeEventKind: "salary_change",
				},
			},
		},
	}
}

func scenarioGeneratedTaskFromLifeEvent(
	now time.Time,
	workflowID string,
	parentTaskID string,
	taskID string,
	intent taskspec.UserIntentType,
	depth int,
	event observation.LifeEventRecord,
	deadline observation.CalendarDeadlineRecord,
) taskspec.GeneratedTaskSpec {
	task := scenarioGeneratedTask(now, taskID, intent, depth)
	task.Metadata.ParentWorkflowID = workflowID
	task.Metadata.ParentTaskID = parentTaskID
	task.Metadata.RootCorrelationID = workflowID
	task.Metadata.GenerationReasons = []taskspec.TaskGenerationReason{
		{
			Code:             taskspec.TaskGenerationReasonLifeEventImpact,
			Description:      "deterministic eval follow-up generated from salary change event",
			LifeEventID:      event.ID,
			LifeEventKind:    string(event.Kind),
			EvidenceIDs:      []string{"evidence-life-event-" + event.ID},
			DeadlineEvidence: []string{"evidence-deadline-" + deadline.ID},
		},
	}
	task.Task.RequiredEvidence = append(task.Task.RequiredEvidence,
		taskspec.RequiredEvidenceRef{Type: "calendar_deadline", Reason: "follow-up should retain deadline provenance", Mandatory: false},
	)
	return task
}

func completedScenarioFollowUpResult(workflowID string, current state.FinancialWorldState, now time.Time) runtime.FollowUpWorkflowRunResult {
	return completedScenarioFollowUpResultWithArtifact(workflowID, "task-"+workflowID, reporting.ArtifactKindReplaySummary, "deterministic eval artifact", current, now)
}

func completedScenarioFollowUpResultWithArtifact(
	workflowID string,
	taskID string,
	kind reporting.ArtifactKind,
	summary string,
	current state.FinancialWorldState,
	now time.Time,
) runtime.FollowUpWorkflowRunResult {
	next := current
	next.Version.Sequence = current.Version.Sequence + 1
	next.Version.SnapshotID = fmt.Sprintf("state-v%d", next.Version.Sequence)
	next.Version.UpdatedAt = now.UTC()
	artifact := reporting.WorkflowArtifact{
		ID:         "artifact-" + workflowID,
		WorkflowID: workflowID,
		TaskID:     taskID,
		Kind:       kind,
		ProducedBy: "phase_6a_eval",
		Ref: reporting.ArtifactRef{
			Kind:    kind,
			ID:      "artifact-" + workflowID,
			Summary: summary,
		},
		ContentJSON: fmt.Sprintf(`{"kind":%q}`, kind),
		CreatedAt:   now.UTC(),
	}
	return runtime.FollowUpWorkflowRunResult{
		WorkflowID:   workflowID,
		RuntimeState: runtime.WorkflowStateCompleted,
		UpdatedState: next,
		Artifacts:    []reporting.WorkflowArtifact{artifact},
	}
}

func findFollowUpTaskByIntent(tasks []runtime.FollowUpTaskRecord, intent taskspec.UserIntentType) *runtime.FollowUpTaskRecord {
	for i := range tasks {
		if tasks[i].Task.UserIntentType == intent {
			return &tasks[i]
		}
	}
	return nil
}

func findExecutionByTaskID(records []runtime.TaskExecutionRecord, taskID string) *runtime.TaskExecutionRecord {
	for i := range records {
		if records[i].TaskID == taskID {
			return &records[i]
		}
	}
	return nil
}

func approvePendingFollowUps(ctx context.Context, plane *app.RuntimePlane, graphID string) error {
	if plane == nil || plane.Query == nil || plane.Operator == nil || plane.Stores == nil || plane.Stores.Approvals == nil {
		return nil
	}
	view, err := plane.Query.GetTaskGraph(ctx, graphID)
	if err != nil {
		return err
	}
	for _, task := range view.Snapshot.RegisteredTasks {
		approval, ok, err := plane.Stores.Approvals.LoadByTask(graphID, task.Task.ID)
		if err != nil {
			return err
		}
		if !ok || approval.Status != runtime.ApprovalStatusPending {
			continue
		}
		if _, err := plane.Operator.ApproveTask(ctx, runtime.ApproveTaskCommand{
			RequestID:  "eval-approve-" + task.Task.ID,
			ApprovalID: approval.ApprovalID,
			Actor:      "eval",
			Roles:      []string{"system"},
			Note:       "deterministic eval approval to complete child workflow replay path",
		}); err != nil {
			return err
		}
	}
	return nil
}

func scenarioSalaryChangeEvent(now time.Time) observation.LifeEventRecord {
	return observation.LifeEventRecord{
		ID:         "event-salary-follow-up",
		UserID:     "user-1",
		Kind:       observation.LifeEventSalaryChange,
		Source:     "fixture-hris",
		Provenance: "local runtime fixture salary change",
		ObservedAt: now.UTC(),
		Confidence: 0.95,
		SalaryChange: &observation.SalaryChangeEventPayload{
			PreviousMonthlyIncomeCents: 1000000,
			NewMonthlyIncomeCents:      1250000,
			EffectiveAt:                now.AddDate(0, 0, -1).UTC(),
		},
	}
}

func scenarioSalaryDeadline(now time.Time, event observation.LifeEventRecord) observation.CalendarDeadlineRecord {
	return observation.CalendarDeadlineRecord{
		ID:               "deadline-salary-follow-up",
		UserID:           "user-1",
		Kind:             "withholding_review",
		RelatedEventID:   event.ID,
		RelatedEventKind: event.Kind,
		Source:           "fixture-calendar",
		Provenance:       "local runtime fixture salary deadline",
		ObservedAt:       now.UTC(),
		DeadlineAt:       now.Add(14 * 24 * time.Hour).UTC(),
		Description:      "review payroll withholding after salary change",
		Confidence:       0.9,
	}
}

func scenarioJobChangeEvent(now time.Time) observation.LifeEventRecord {
	return observation.LifeEventRecord{
		ID:         "event-job-follow-up",
		UserID:     "user-1",
		Kind:       observation.LifeEventJobChange,
		Source:     "fixture-hris",
		Provenance: "local runtime fixture job change",
		ObservedAt: now.UTC(),
		Confidence: 0.94,
		JobChange: &observation.JobChangeEventPayload{
			PreviousEmployer:             "OldCo",
			NewEmployer:                  "NextCo",
			PreviousMonthlyIncomeCents:   1000000,
			NewMonthlyIncomeCents:        1400000,
			BenefitsEnrollmentDeadlineAt: now.Add(7 * 24 * time.Hour).UTC(),
		},
	}
}

func scenarioJobDeadline(now time.Time, event observation.LifeEventRecord) observation.CalendarDeadlineRecord {
	return observation.CalendarDeadlineRecord{
		ID:               "deadline-job-follow-up",
		UserID:           "user-1",
		Kind:             "benefits_enrollment",
		RelatedEventID:   event.ID,
		RelatedEventKind: event.Kind,
		Source:           "fixture-calendar",
		Provenance:       "local runtime fixture job deadline",
		ObservedAt:       now.UTC(),
		DeadlineAt:       now.Add(7 * 24 * time.Hour).UTC(),
		Description:      "complete benefits enrollment after job change",
		Confidence:       0.92,
	}
}
