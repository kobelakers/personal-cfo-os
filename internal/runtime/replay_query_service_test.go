package runtime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestReplayQueryServiceByWorkflowSuccessAndStaleProjectionDegrades(t *testing.T) {
	t.Parallel()

	harness := newWorkflowReplayHarness(t)
	workflowID := "workflow-monthly-review-test"
	saveWorkflowReplayForTest(t, harness, workflowID, WorkflowStateCompleted, replayQueryTestNow, observability.WorkflowTraceDump{
		WorkflowID:  workflowID,
		TraceID:     workflowID + "-trace",
		GeneratedAt: replayQueryTestNow,
		MemoryRetrievals: []observability.MemoryRetrievalTraceRecord{{
			QueryID:          "query-memory-1",
			SelectedMemoryID: []string{"memory-selected-1"},
		}},
	})

	view, err := harness.query.Query(context.Background(), observability.ReplayQuery{WorkflowID: workflowID})
	if err != nil {
		t.Fatalf("query workflow replay: %v", err)
	}
	if view.Scope.Kind != string(ReplayScopeWorkflow) || view.Scope.ID != workflowID {
		t.Fatalf("expected workflow replay scope, got %+v", view.Scope)
	}
	if view.Summary.FinalState != string(WorkflowStateCompleted) {
		t.Fatalf("expected completed workflow replay summary, got %+v", view.Summary)
	}
	if view.Degraded {
		t.Fatalf("expected fresh workflow projection to be non-degraded, got %+v", view.DegradationReasons)
	}

	updated := WorkflowRunRecord{
		WorkflowID:      workflowID,
		TaskID:          "task-monthly-review-test",
		Intent:          string(taskspec.UserIntentMonthlyReview),
		RuntimeState:    WorkflowStateFailed,
		FailureCategory: FailureCategoryTrustValidation,
		FailureSummary:  "workflow became stale relative to projection",
		Summary:         "updated workflow state after projection rebuild",
		StartedAt:       replayQueryTestNow,
		UpdatedAt:       replayQueryTestNow.Add(2 * time.Hour),
	}
	if err := harness.workflowRuns.Save(updated); err != nil {
		t.Fatalf("save newer workflow runtime truth: %v", err)
	}

	view, err = harness.query.Query(context.Background(), observability.ReplayQuery{WorkflowID: workflowID})
	if err != nil {
		t.Fatalf("query stale workflow replay: %v", err)
	}
	if !view.Degraded {
		t.Fatalf("expected stale workflow projection to degrade replay view")
	}
	if !hasReplayDegradationReason(view.DegradationReasons, observability.ReplayDegradationProjectionStale) {
		t.Fatalf("expected projection_stale degradation, got %+v", view.DegradationReasons)
	}
	if view.Summary.FinalState != string(WorkflowStateFailed) {
		t.Fatalf("expected authoritative runtime state to win after staleness, got %+v", view.Summary)
	}
	if view.Workflow == nil || view.Workflow.RuntimeState != string(WorkflowStateFailed) {
		t.Fatalf("expected workflow replay view to reflect authoritative failed state, got %+v", view.Workflow)
	}
}

func TestReplayQueryServiceByWorkflowMissingProjectionReturnsPartialView(t *testing.T) {
	t.Parallel()

	harness := newWorkflowReplayHarness(t)
	workflowID := "workflow-monthly-review-missing-projection"
	saveWorkflowReplayForTest(t, harness, workflowID, WorkflowStateCompleted, replayQueryTestNow, observability.WorkflowTraceDump{
		WorkflowID:  workflowID,
		TraceID:     workflowID + "-trace",
		GeneratedAt: replayQueryTestNow,
	})

	delete(harness.projections.workflow, workflowID)
	delete(harness.projections.builds, replayScopeKey(ReplayProjectionScope{ScopeKind: ReplayScopeWorkflow, ScopeID: workflowID}))
	harness.projections.ReplaceProvenance(ReplayProjectionScope{ScopeKind: ReplayScopeWorkflow, ScopeID: workflowID}, nil, nil)
	harness.projections.ReplaceExecutionAttributions(ReplayProjectionScope{ScopeKind: ReplayScopeWorkflow, ScopeID: workflowID}, nil)
	harness.projections.ReplaceFailureAttributions(ReplayProjectionScope{ScopeKind: ReplayScopeWorkflow, ScopeID: workflowID}, nil)

	view, err := harness.query.Query(context.Background(), observability.ReplayQuery{WorkflowID: workflowID})
	if err != nil {
		t.Fatalf("query workflow replay without projection rows: %v", err)
	}
	if !view.Degraded {
		t.Fatalf("expected workflow replay without projection rows to degrade")
	}
	if !hasReplayDegradationReason(view.DegradationReasons, observability.ReplayDegradationProjectionMissing) {
		t.Fatalf("expected projection_missing degradation, got %+v", view.DegradationReasons)
	}
	if len(view.Provenance.Nodes) == 0 {
		t.Fatalf("expected best-effort authoritative provenance fallback, got %+v", view.Provenance)
	}
}

func TestReplayQueryServiceByTaskGraphReturnsPartialViewWhenProjectionIsIncomplete(t *testing.T) {
	t.Parallel()

	harness := newCompletedTaskGraphReplayHarness(t)

	view, err := harness.query.Query(context.Background(), observability.ReplayQuery{TaskGraphID: harness.graph.GraphID})
	if err != nil {
		t.Fatalf("query replay without projection: %v", err)
	}
	if !view.Degraded {
		t.Fatalf("expected missing projection to degrade replay view")
	}
	if !hasReplayDegradationReason(view.DegradationReasons, observability.ReplayDegradationProjectionIncomplete) {
		t.Fatalf("expected projection_incomplete degradation reason, got %+v", view.DegradationReasons)
	}
	if !hasReplayEdgeType(view.Provenance, "triggered_child_workflow") {
		t.Fatalf("expected best-effort provenance to retain child workflow edge, got %+v", view.Provenance)
	}
}

func TestReplayProjectionRebuilderBackfillsTaskGraphFromRuntimeTruth(t *testing.T) {
	t.Parallel()

	harness := newCompletedTaskGraphReplayHarness(t)
	build, err := harness.rebuilder.RebuildTaskGraph(context.Background(), harness.graph.GraphID)
	if err != nil {
		t.Fatalf("rebuild task-graph projection: %v", err)
	}
	if build.SchemaVersion != ReplayProjectionSchemaVersion {
		t.Fatalf("expected schema version %d, got %+v", ReplayProjectionSchemaVersion, build)
	}

	view, err := harness.query.Query(context.Background(), observability.ReplayQuery{TaskGraphID: harness.graph.GraphID})
	if err != nil {
		t.Fatalf("query replay after rebuild: %v", err)
	}
	if view.ProjectionVersion != ReplayProjectionSchemaVersion {
		t.Fatalf("expected replay view to carry projection version %d, got %+v", ReplayProjectionSchemaVersion, view)
	}
	for _, edgeType := range []string{"generated_task", "triggered_child_workflow", "produced_artifact", "committed_state"} {
		if !hasReplayEdgeType(view.Provenance, edgeType) {
			t.Fatalf("expected provenance edge %q, got %+v", edgeType, view.Provenance)
		}
	}
	if len(view.ExecutionAttributions) == 0 {
		t.Fatalf("expected execution attributions after replay rebuild")
	}
}

func TestReplayQueryServiceByTaskFiltersScopeToSingleTask(t *testing.T) {
	t.Parallel()

	harness := newCompletedTaskGraphReplayHarness(t)
	if _, err := harness.rebuilder.RebuildTaskGraph(context.Background(), harness.graph.GraphID); err != nil {
		t.Fatalf("rebuild task-graph projection: %v", err)
	}

	view, err := harness.query.Query(context.Background(), observability.ReplayQuery{TaskID: harness.taskID})
	if err != nil {
		t.Fatalf("query replay by task: %v", err)
	}
	if view.Scope.Kind != string(ReplayScopeTask) || view.Scope.ID != harness.taskID {
		t.Fatalf("expected task replay scope, got %+v", view.Scope)
	}
	if view.TaskGraph == nil || len(view.TaskGraph.TaskIDs) != 1 || view.TaskGraph.TaskIDs[0] != harness.taskID {
		t.Fatalf("expected task replay view to be filtered to a single task, got %+v", view.TaskGraph)
	}
}

func TestReplayQueryServiceByExecutionFiltersScopeToSingleExecution(t *testing.T) {
	t.Parallel()

	harness := newCompletedTaskGraphReplayHarness(t)
	if _, err := harness.rebuilder.RebuildTaskGraph(context.Background(), harness.graph.GraphID); err != nil {
		t.Fatalf("rebuild task-graph projection: %v", err)
	}

	view, err := harness.query.Query(context.Background(), observability.ReplayQuery{ExecutionID: harness.executionID})
	if err != nil {
		t.Fatalf("query replay by execution: %v", err)
	}
	if view.Scope.Kind != string(ReplayScopeExecution) || view.Scope.ID != harness.executionID {
		t.Fatalf("expected execution replay scope, got %+v", view.Scope)
	}
	if len(view.ExecutionAttributions) != 1 || view.ExecutionAttributions[0].ExecutionID != harness.executionID {
		t.Fatalf("expected execution replay to retain only the requested execution attribution, got %+v", view.ExecutionAttributions)
	}
}

func TestReplayQueryServiceByApprovalReturnsWaitingApprovalView(t *testing.T) {
	t.Parallel()

	harness := newWaitingApprovalReplayHarness(t)
	if _, err := harness.rebuilder.RebuildTaskGraph(context.Background(), harness.graph.GraphID); err != nil {
		t.Fatalf("rebuild waiting-approval task-graph projection: %v", err)
	}

	view, err := harness.query.Query(context.Background(), observability.ReplayQuery{ApprovalID: harness.approvalID})
	if err != nil {
		t.Fatalf("query replay by approval: %v", err)
	}
	if view.Scope.Kind != string(ReplayScopeApproval) || view.Scope.ID != harness.approvalID {
		t.Fatalf("expected approval replay scope, got %+v", view.Scope)
	}
	if view.Approval == nil || view.Approval.ApprovalID != harness.approvalID {
		t.Fatalf("expected approval replay view, got %+v", view.Approval)
	}
	if view.Summary.FinalState != string(WorkflowStateWaitingApproval) {
		t.Fatalf("expected waiting_approval summary, got %+v", view.Summary)
	}
}

func TestReplayQueryServiceHardFailsWhenAuthoritativeTruthMissing(t *testing.T) {
	t.Parallel()

	workflowHarness := newWorkflowReplayHarness(t)
	completedHarness := newCompletedTaskGraphReplayHarness(t)
	waitingHarness := newWaitingApprovalReplayHarness(t)

	testCases := []struct {
		name  string
		query observability.ReplayQuery
	}{
		{name: "workflow", query: observability.ReplayQuery{WorkflowID: "workflow-missing"}},
		{name: "task_graph", query: observability.ReplayQuery{TaskGraphID: "graph-missing"}},
		{name: "task", query: observability.ReplayQuery{TaskID: "task-missing"}},
		{name: "execution", query: observability.ReplayQuery{ExecutionID: "execution-missing"}},
		{name: "approval", query: observability.ReplayQuery{ApprovalID: "approval-missing"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var queryService *ReplayQueryService
			switch tc.name {
			case "workflow":
				queryService = workflowHarness.query
			case "approval":
				queryService = waitingHarness.query
			default:
				queryService = completedHarness.query
			}
			if _, err := queryService.Query(context.Background(), tc.query); err == nil {
				t.Fatalf("expected authoritative runtime truth miss to hard-fail replay query for %s", tc.name)
			}
		})
	}
}

func TestReplayQueryServiceCompareAcrossSupportedScopes(t *testing.T) {
	t.Parallel()

	workflowHarness := newWorkflowReplayHarness(t)
	saveWorkflowReplayForTest(t, workflowHarness, "workflow-compare-left", WorkflowStateCompleted, replayQueryTestNow, observability.WorkflowTraceDump{
		WorkflowID:  "workflow-compare-left",
		TraceID:     "trace-workflow-compare-left",
		GeneratedAt: replayQueryTestNow,
		MemoryRetrievals: []observability.MemoryRetrievalTraceRecord{{
			QueryID:          "memory-left",
			SelectedMemoryID: []string{"memory-a"},
		}},
	})
	saveWorkflowReplayForTest(t, workflowHarness, "workflow-compare-right", WorkflowStateFailed, replayQueryTestNow.Add(time.Minute), observability.WorkflowTraceDump{
		WorkflowID:  "workflow-compare-right",
		TraceID:     "trace-workflow-compare-right",
		GeneratedAt: replayQueryTestNow.Add(time.Minute),
		MemoryRetrievals: []observability.MemoryRetrievalTraceRecord{{
			QueryID:          "memory-right",
			SelectedMemoryID: nil,
			RejectedMemoryID: []string{"memory-b"},
			Results: []memory.RetrievalResult{{
				MemoryID:        "memory-b",
				Rejected:        true,
				RejectionRule:   "low_confidence",
				RejectionReason: "below threshold",
			}},
		}},
		Events: []observability.LogEntry{{
			Category:   "runtime",
			Message:    "workflow compare right failed",
			OccurredAt: replayQueryTestNow.Add(time.Minute),
		}},
	})

	comparison, err := workflowHarness.query.Compare(context.Background(),
		observability.ReplayQuery{WorkflowID: "workflow-compare-left"},
		observability.ReplayQuery{WorkflowID: "workflow-compare-right"},
	)
	if err != nil {
		t.Fatalf("compare workflow replay views: %v", err)
	}
	if len(comparison.Diffs) == 0 {
		t.Fatalf("expected workflow replay comparison to produce diffs")
	}
	if !comparisonContainsCategory(comparison, "memory") || !comparisonContainsCategory(comparison, "runtime") {
		t.Fatalf("expected workflow comparison to include memory and runtime diff categories, got %+v", comparison)
	}

	taskGraphHarness := newCompletedTaskGraphReplayHarness(t)
	if _, err := taskGraphHarness.rebuilder.RebuildTaskGraph(context.Background(), taskGraphHarness.graph.GraphID); err != nil {
		t.Fatalf("rebuild left task-graph projection: %v", err)
	}
	rightGraph := runtimeTestGraph(replayQueryTestNow.Add(time.Minute), runtimeGeneratedTask(replayQueryTestNow.Add(time.Minute), "task-replay-tax-right", taskspec.UserIntentTaxOptimization, 1))
	rightGraph.GraphID = "graph-runtime-test-right"
	execCtx := ExecutionContext{WorkflowID: rightGraph.ParentWorkflowID, TaskID: rightGraph.ParentTaskID, CorrelationID: rightGraph.ParentWorkflowID, Attempt: 1}
	if _, err := taskGraphHarness.service.Runtime().RegisterFollowUpTasks(execCtx, rightGraph, runtimeTestState(replayQueryTestNow.Add(time.Minute), 2)); err != nil {
		t.Fatalf("register right task-graph compare seed: %v", err)
	}
	if _, err := taskGraphHarness.service.ExecuteAutoReadyFollowUps(context.Background(), rightGraph.GraphID, DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("execute right task-graph compare seed: %v", err)
	}
	if _, err := taskGraphHarness.rebuilder.RebuildTaskGraph(context.Background(), rightGraph.GraphID); err != nil {
		t.Fatalf("rebuild right task-graph projection: %v", err)
	}
	comparison, err = taskGraphHarness.query.Compare(context.Background(),
		observability.ReplayQuery{TaskGraphID: taskGraphHarness.graph.GraphID},
		observability.ReplayQuery{TaskGraphID: rightGraph.GraphID},
	)
	if err != nil {
		t.Fatalf("compare task-graph replay views: %v", err)
	}
	if len(comparison.Diffs) == 0 {
		t.Fatalf("expected task-graph replay comparison to produce diffs")
	}
}

func TestSQLiteReplayProjectionStorePersistsSchemaVersion(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "runtime.db")
	stores, err := NewSQLiteRuntimeStores(dbPath)
	if err != nil {
		t.Fatalf("open sqlite runtime stores: %v", err)
	}
	scope := ReplayProjectionScope{ScopeKind: ReplayScopeTaskGraph, ScopeID: "graph-projection"}
	if err := stores.ReplayProjection.SaveTaskGraphProjection(TaskGraphReplayProjection{
		GraphID:             scope.ScopeID,
		RuntimeState:        WorkflowStateCompleted,
		ProjectionStatus:    ReplayProjectionStatusComplete,
		SchemaVersion:       ReplayProjectionSchemaVersion,
		SummaryJSON:         `{"final_state":"completed"}`,
		ExplanationJSON:     `{"why_failed":""}`,
		CompareInputJSON:    `{"final_state":"completed"}`,
		UpdatedAt:           replayQueryTestNow,
		ProjectionFreshness: replayQueryTestNow,
	}); err != nil {
		t.Fatalf("save task-graph projection: %v", err)
	}
	if err := stores.ReplayProjection.SaveBuild(ReplayProjectionBuildRecord{
		ScopeKind:           scope.ScopeKind,
		ScopeID:             scope.ScopeID,
		SchemaVersion:       ReplayProjectionSchemaVersion,
		Status:              ReplayProjectionStatusComplete,
		BuiltAt:             replayQueryTestNow,
		SourceEventCount:    1,
		SourceArtifactCount: 1,
	}); err != nil {
		t.Fatalf("save projection build: %v", err)
	}
	if err := stores.DB.Close(); err != nil {
		t.Fatalf("close sqlite runtime stores: %v", err)
	}

	reopened, err := NewSQLiteRuntimeStores(dbPath)
	if err != nil {
		t.Fatalf("reopen sqlite runtime stores: %v", err)
	}
	defer func() { _ = reopened.DB.Close() }()

	projection, ok, err := reopened.ReplayQuery.LoadTaskGraphProjection(scope.ScopeID)
	if err != nil || !ok {
		t.Fatalf("load persisted task-graph projection: ok=%t err=%v", ok, err)
	}
	build, ok, err := reopened.ReplayQuery.LoadBuild(scope)
	if err != nil || !ok {
		t.Fatalf("load persisted projection build: ok=%t err=%v", ok, err)
	}
	if projection.SchemaVersion != ReplayProjectionSchemaVersion || build.SchemaVersion != ReplayProjectionSchemaVersion {
		t.Fatalf("expected persisted projection schema version %d, got projection=%+v build=%+v", ReplayProjectionSchemaVersion, projection, build)
	}
}

var replayQueryTestNow = time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC)

type workflowReplayHarness struct {
	workflowRuns *InMemoryWorkflowRunStore
	projections  *InMemoryReplayProjectionStore
	artifacts    *InMemoryArtifactMetadataStore
	replayStore  *InMemoryReplayStore
	rebuilder    *ReplayProjectionRebuilder
	query        *ReplayQueryService
}

type taskGraphReplayHarness struct {
	service     *Service
	query       *ReplayQueryService
	rebuilder   *ReplayProjectionRebuilder
	graph       taskspec.TaskGraph
	taskID      string
	executionID string
	approvalID  string
}

func newWorkflowReplayHarness(t *testing.T) *workflowReplayHarness {
	t.Helper()

	workflowRuns := NewInMemoryWorkflowRunStore()
	projections := NewInMemoryReplayProjectionStore()
	artifacts := NewInMemoryArtifactMetadataStore()
	replayStore := NewInMemoryReplayStore()
	rebuilder := NewReplayProjectionRebuilder(nil, workflowRuns, projections, artifacts, replayStore, func() time.Time { return replayQueryTestNow })
	query := NewReplayQueryService(nil, workflowRuns, projections, artifacts, replayStore)
	return &workflowReplayHarness{
		workflowRuns: workflowRuns,
		projections:  projections,
		artifacts:    artifacts,
		replayStore:  replayStore,
		rebuilder:    rebuilder,
		query:        query,
	}
}

func saveWorkflowReplayForTest(t *testing.T, harness *workflowReplayHarness, workflowID string, finalState WorkflowExecutionState, updatedAt time.Time, trace observability.WorkflowTraceDump) {
	t.Helper()

	taskID := "task-" + workflowID
	record := WorkflowRunRecord{
		WorkflowID:   workflowID,
		TaskID:       taskID,
		Intent:       string(taskspec.UserIntentMonthlyReview),
		RuntimeState: finalState,
		Summary:      "workflow replay test summary",
		StartedAt:    updatedAt,
		UpdatedAt:    updatedAt,
	}
	if finalState == WorkflowStateFailed {
		record.FailureCategory = FailureCategoryTrustValidation
		record.FailureSummary = "workflow replay test failure"
	}
	if err := harness.workflowRuns.Save(record); err != nil {
		t.Fatalf("save workflow run: %v", err)
	}

	bundle := observability.NewReplayBundle("workflow_replay_test", trace, map[string]string{
		"workflow_id": workflowID,
	})
	bundle.GeneratedAt = updatedAt
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal workflow replay bundle: %v", err)
	}
	if err := harness.artifacts.SaveArtifact(workflowID, taskID, reporting.WorkflowArtifact{
		ID:         workflowID + "-replay_bundle",
		WorkflowID: workflowID,
		TaskID:     taskID,
		Kind:       reporting.ArtifactKindReplayBundle,
		ProducedBy: "replay_query_test",
		Ref: reporting.ArtifactRef{
			Kind:     reporting.ArtifactKindReplayBundle,
			ID:       workflowID + "-replay_bundle",
			Summary:  "workflow replay bundle",
			Location: workflowID + "-replay_bundle.json",
		},
		ContentJSON: string(bundleJSON),
		CreatedAt:   updatedAt,
	}); err != nil {
		t.Fatalf("save workflow replay bundle artifact: %v", err)
	}
	if _, err := harness.rebuilder.RebuildWorkflow(context.Background(), workflowID); err != nil {
		t.Fatalf("rebuild workflow projection: %v", err)
	}
}

func newCompletedTaskGraphReplayHarness(t *testing.T) *taskGraphReplayHarness {
	t.Helper()
	return newCompletedTaskGraphReplayHarnessWithID(t, "graph-runtime-test", "task-replay-tax")
}

func newCompletedTaskGraphReplayHarnessWithID(t *testing.T, graphID string, taskID string) *taskGraphReplayHarness {
	t.Helper()

	workflowRuns := NewInMemoryWorkflowRunStore()
	projections := NewInMemoryReplayProjectionStore()
	artifacts := NewInMemoryArtifactMetadataStore()
	replayStore := NewInMemoryReplayStore()
	service := NewService(ServiceOptions{
		CheckpointStore: NewInMemoryCheckpointStore(),
		TaskGraphs:      NewInMemoryTaskGraphStore(),
		Executions:      NewInMemoryTaskExecutionStore(),
		Approvals:       NewInMemoryApprovalStateStore(),
		OperatorActions: NewInMemoryOperatorActionStore(),
		Replay:          replayStore,
		Artifacts:       artifacts,
		Controller:      DefaultWorkflowController{},
		Now:             func() time.Time { return replayQueryTestNow },
	})
	service.SetCapabilities(StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
		},
		Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: runtimeTestCapability{
				name: "tax_optimization_workflow",
				execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
					return FollowUpWorkflowRunResult{
						WorkflowID:   "workflow-child-" + spec.ID,
						RuntimeState: WorkflowStateCompleted,
						UpdatedState: runtimeTestState(replayQueryTestNow, current.Version.Sequence+1),
						Artifacts: []reporting.WorkflowArtifact{{
							ID:         "artifact-" + spec.ID,
							WorkflowID: "workflow-child-" + spec.ID,
							TaskID:     spec.ID,
							Kind:       reporting.ArtifactKindTaxOptimizationReport,
							ProducedBy: "replay_query_test",
							Ref: reporting.ArtifactRef{
								Kind:    reporting.ArtifactKindTaxOptimizationReport,
								ID:      "artifact-" + spec.ID,
								Summary: "child workflow artifact",
							},
							ContentJSON: `{"kind":"tax_optimization_report"}`,
							CreatedAt:   replayQueryTestNow,
						}},
					}, nil
				},
			},
		},
	})
	rebuilder := NewReplayProjectionRebuilder(service, workflowRuns, projections, artifacts, replayStore, func() time.Time { return replayQueryTestNow })
	service.SetReplayProjectionWriter(rebuilder)
	queryService := NewReplayQueryService(service, workflowRuns, projections, artifacts, replayStore)
	graph := runtimeTestGraph(replayQueryTestNow, runtimeGeneratedTask(replayQueryTestNow, taskID, taskspec.UserIntentTaxOptimization, 1))
	graph.GraphID = graphID
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := service.Runtime().RegisterFollowUpTasks(execCtx, graph, runtimeTestState(replayQueryTestNow, 1)); err != nil {
		t.Fatalf("register test graph: %v", err)
	}
	batch, err := service.ExecuteAutoReadyFollowUps(context.Background(), graph.GraphID, DefaultAutoExecutionPolicy())
	if err != nil {
		t.Fatalf("execute test graph: %v", err)
	}
	return &taskGraphReplayHarness{
		service:     service,
		query:       queryService,
		rebuilder:   rebuilder,
		graph:       graph,
		taskID:      taskID,
		executionID: batch.ExecutedTasks[0].ExecutionID,
	}
}

func newWaitingApprovalReplayHarness(t *testing.T) *taskGraphReplayHarness {
	t.Helper()

	workflowRuns := NewInMemoryWorkflowRunStore()
	projections := NewInMemoryReplayProjectionStore()
	artifacts := NewInMemoryArtifactMetadataStore()
	replayStore := NewInMemoryReplayStore()
	service := NewService(ServiceOptions{
		CheckpointStore: NewInMemoryCheckpointStore(),
		TaskGraphs:      NewInMemoryTaskGraphStore(),
		Executions:      NewInMemoryTaskExecutionStore(),
		Approvals:       NewInMemoryApprovalStateStore(),
		OperatorActions: NewInMemoryOperatorActionStore(),
		Replay:          replayStore,
		Artifacts:       artifacts,
		Controller:      DefaultWorkflowController{},
		Now:             func() time.Time { return replayQueryTestNow },
	})
	service.SetCapabilities(StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
		},
		Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: runtimeTestCapability{
				name: "tax_optimization_workflow",
				execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
					return waitingApprovalFollowUpResult("workflow-child-"+spec.ID, current), nil
				},
				resume: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState, _ CheckpointRecord, _ ResumeToken, _ CheckpointPayloadEnvelope) (FollowUpWorkflowRunResult, error) {
					return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(replayQueryTestNow, current.Version.Sequence+1)), nil
				},
			},
		},
	})
	rebuilder := NewReplayProjectionRebuilder(service, workflowRuns, projections, artifacts, replayStore, func() time.Time { return replayQueryTestNow })
	service.SetReplayProjectionWriter(rebuilder)
	queryService := NewReplayQueryService(service, workflowRuns, projections, artifacts, replayStore)
	graph := runtimeTestGraph(replayQueryTestNow, runtimeGeneratedTask(replayQueryTestNow, "task-replay-tax-approval", taskspec.UserIntentTaxOptimization, 1))
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := service.Runtime().RegisterFollowUpTasks(execCtx, graph, runtimeTestState(replayQueryTestNow, 1)); err != nil {
		t.Fatalf("register waiting-approval graph: %v", err)
	}
	batch, err := service.ExecuteAutoReadyFollowUps(context.Background(), graph.GraphID, DefaultAutoExecutionPolicy())
	if err != nil {
		t.Fatalf("execute waiting-approval graph: %v", err)
	}
	approval, ok, err := service.Runtime().Approvals.LoadByTask(graph.GraphID, graph.GeneratedTasks[0].Task.ID)
	if err != nil || !ok {
		t.Fatalf("load waiting-approval record: ok=%t err=%v", ok, err)
	}
	return &taskGraphReplayHarness{
		service:     service,
		query:       queryService,
		rebuilder:   rebuilder,
		graph:       graph,
		taskID:      graph.GeneratedTasks[0].Task.ID,
		executionID: batch.ExecutedTasks[0].ExecutionID,
		approvalID:  approval.ApprovalID,
	}
}

func hasReplayEdgeType(graph observability.ProvenanceGraph, edgeType string) bool {
	for _, edge := range graph.Edges {
		if edge.Type == edgeType {
			return true
		}
	}
	return false
}

func hasReplayDegradationReason(items []observability.ReplayDegradation, reason observability.ReplayDegradationReason) bool {
	for _, item := range items {
		if item.Reason == reason {
			return true
		}
	}
	return false
}

func comparisonContainsCategory(comparison observability.ReplayComparison, category string) bool {
	for _, diff := range comparison.Diffs {
		if diff.Category == category {
			return true
		}
	}
	return false
}
