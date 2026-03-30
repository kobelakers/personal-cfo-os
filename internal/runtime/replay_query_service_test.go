package runtime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestReplayQueryServiceReturnsPartialViewWhenProjectionIsIncomplete(t *testing.T) {
	t.Parallel()

	service, queryService, _, graph := newReplayQueryTestHarness(t)
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := service.Runtime().RegisterFollowUpTasks(execCtx, graph, runtimeTestState(replayQueryTestNow, 1)); err != nil {
		t.Fatalf("register test graph: %v", err)
	}
	if _, err := service.ExecuteAutoReadyFollowUps(context.Background(), graph.GraphID, DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("execute test graph: %v", err)
	}

	view, err := queryService.Query(context.Background(), observability.ReplayQuery{TaskGraphID: graph.GraphID})
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

	service, queryService, rebuilder, graph := newReplayQueryTestHarness(t)
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := service.Runtime().RegisterFollowUpTasks(execCtx, graph, runtimeTestState(replayQueryTestNow, 1)); err != nil {
		t.Fatalf("register test graph: %v", err)
	}
	if _, err := service.ExecuteAutoReadyFollowUps(context.Background(), graph.GraphID, DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("execute test graph: %v", err)
	}
	build, err := rebuilder.RebuildTaskGraph(context.Background(), graph.GraphID)
	if err != nil {
		t.Fatalf("rebuild task-graph projection: %v", err)
	}
	if build.SchemaVersion != ReplayProjectionSchemaVersion {
		t.Fatalf("expected schema version %d, got %+v", ReplayProjectionSchemaVersion, build)
	}

	view, err := queryService.Query(context.Background(), observability.ReplayQuery{TaskGraphID: graph.GraphID})
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

func TestReplayQueryServiceHardFailsWhenAuthoritativeTruthMissing(t *testing.T) {
	t.Parallel()

	_, queryService, _, graph := newReplayQueryTestHarness(t)
	_, err := queryService.Query(context.Background(), observability.ReplayQuery{TaskGraphID: graph.GraphID})
	if err == nil {
		t.Fatalf("expected authoritative runtime truth miss to hard-fail replay query")
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

func newReplayQueryTestHarness(t *testing.T) (*Service, *ReplayQueryService, *ReplayProjectionRebuilder, taskspec.TaskGraph) {
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
	graph := runtimeTestGraph(replayQueryTestNow, runtimeGeneratedTask(replayQueryTestNow, "task-replay-tax", taskspec.UserIntentTaxOptimization, 1))
	return service, queryService, rebuilder, graph
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
