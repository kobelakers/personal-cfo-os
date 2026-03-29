package runtime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestSQLiteRuntimeStoresPersistAndRecoverDurableRuntimeState(t *testing.T) {
	now := time.Date(2026, 3, 29, 13, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(t.TempDir(), "runtime.db")
	stores, err := NewSQLiteRuntimeStores(dbPath)
	if err != nil {
		t.Fatalf("create sqlite runtime stores: %v", err)
	}
	runtime := LocalWorkflowRuntime{
		TaskGraphs:      stores.TaskGraphs,
		Executions:      stores.Executions,
		Approvals:       stores.Approvals,
		OperatorActions: stores.OperatorActions,
		CheckpointStore: stores.Checkpoints,
		Replay:          stores.Replay,
		Artifacts:       stores.Artifacts,
		Now:             func() time.Time { return now },
	}
	runtime.Capabilities = StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
		},
		Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: runtimeTestCapability{
				name: "tax_optimization_workflow",
				execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
					return waitingApprovalWithPayloadResult("workflow-child-"+spec.ID, spec.ID, current), nil
				},
			},
		},
	}
	base := runtimeTestState(now, 1)
	graph := runtimeTestGraph(now, runtimeGeneratedTask(now, "task-tax-durable", taskspec.UserIntentTaxOptimization, 1))
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := runtime.RegisterFollowUpTasks(execCtx, graph, base); err != nil {
		t.Fatalf("register follow-up tasks: %v", err)
	}
	batch, err := runtime.ExecuteReadyFollowUps(context.Background(), execCtx, graph.GraphID, DefaultAutoExecutionPolicy())
	if err != nil {
		t.Fatalf("execute follow-up tasks: %v", err)
	}
	waiting := runtimeTestRecordByTaskID(t, batch.ExecutedTasks, "task-tax-durable")
	if waiting.Status != TaskQueueStatusWaitingApproval {
		t.Fatalf("expected waiting approval execution record, got %+v", waiting)
	}
	artifact := reporting.WorkflowArtifact{
		ID:         "artifact-tax-durable",
		WorkflowID: waiting.WorkflowID,
		TaskID:     waiting.TaskID,
		Kind:       reporting.ArtifactKindTaxOptimizationReport,
		ProducedBy: "test",
		Ref: reporting.ArtifactRef{
			Kind:    reporting.ArtifactKindTaxOptimizationReport,
			ID:      "artifact-tax-durable",
			Summary: "durable artifact metadata",
		},
		ContentJSON: `{"kind":"tax_optimization_report"}`,
		CreatedAt:   now,
	}
	if err := stores.Artifacts.SaveArtifact(waiting.WorkflowID, waiting.TaskID, artifact); err != nil {
		t.Fatalf("save artifact metadata: %v", err)
	}
	if err := stores.DB.Close(); err != nil {
		t.Fatalf("close sqlite runtime db: %v", err)
	}

	reopened, err := NewSQLiteRuntimeStores(dbPath)
	if err != nil {
		t.Fatalf("reopen sqlite runtime stores: %v", err)
	}
	defer func() { _ = reopened.DB.Close() }()

	loadedGraph, ok, err := reopened.TaskGraphs.Load(graph.GraphID)
	if err != nil {
		t.Fatalf("load persisted task graph: %v", err)
	}
	if !ok {
		t.Fatalf("expected persisted task graph")
	}
	if loadedGraph.LatestCommittedStateSnapshot.State.Version.Sequence != 1 {
		t.Fatalf("waiting approval task should not advance committed state, got %+v", loadedGraph.LatestCommittedStateSnapshot.State.Version)
	}
	loadedExecution, ok, err := reopened.Executions.LoadLatestByTask(graph.GraphID, "task-tax-durable")
	if err != nil {
		t.Fatalf("load persisted execution: %v", err)
	}
	if !ok || loadedExecution.CheckpointID == "" || loadedExecution.ResumeToken == "" {
		t.Fatalf("expected persisted execution with resume anchors, got %+v", loadedExecution)
	}
	approval, ok, err := reopened.Approvals.LoadByTask(graph.GraphID, "task-tax-durable")
	if err != nil {
		t.Fatalf("load persisted approval: %v", err)
	}
	if !ok || approval.Status != ApprovalStatusPending {
		t.Fatalf("expected pending approval record, got %+v", approval)
	}
	if _, err := reopened.Checkpoints.Load(loadedExecution.WorkflowID, loadedExecution.CheckpointID); err != nil {
		t.Fatalf("load persisted checkpoint: %v", err)
	}
	token, err := reopened.Checkpoints.LoadResumeToken(loadedExecution.ResumeToken)
	if err != nil {
		t.Fatalf("load persisted resume token: %v", err)
	}
	if token.CheckpointID != loadedExecution.CheckpointID {
		t.Fatalf("expected token to point to checkpoint %q, got %+v", loadedExecution.CheckpointID, token)
	}
	payload, err := reopened.Checkpoints.LoadPayload(loadedExecution.CheckpointID)
	if err != nil {
		t.Fatalf("load persisted checkpoint payload: %v", err)
	}
	if payload.Kind != CheckpointPayloadKindFollowUpFinalizeResume || payload.FollowUpFinalizeResume == nil {
		t.Fatalf("expected follow-up finalize payload, got %+v", payload)
	}
	if payload.FollowUpFinalizeResume.PendingStateSnapshotRef != loadedExecution.UpdatedStateSnapshotRef {
		t.Fatalf("expected pending state snapshot ref %q, got %+v", loadedExecution.UpdatedStateSnapshotRef, payload.FollowUpFinalizeResume)
	}
	artifacts, err := reopened.Artifacts.ListArtifactsByTask("task-tax-durable")
	if err != nil {
		t.Fatalf("load persisted artifact metadata: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Ref.Location == "" || artifacts[0].ContentJSON != "" {
		t.Fatalf("expected metadata-only artifact persistence, got %+v", artifacts)
	}
	events, err := reopened.Replay.ListByTask("task-tax-durable")
	if err != nil {
		t.Fatalf("load persisted replay events: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected persisted replay events")
	}
}

func TestSQLiteRuntimeDBEnsureSchemaIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runtime.db")
	db, err := NewSQLiteRuntimeDB(dbPath)
	if err != nil {
		t.Fatalf("create sqlite runtime db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.EnsureSchema(); err != nil {
		t.Fatalf("ensure schema first pass: %v", err)
	}
	if err := db.EnsureSchema(); err != nil {
		t.Fatalf("ensure schema second pass: %v", err)
	}
}

func TestSQLiteOperatorActionStoreEnforcesUniqueRequestID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runtime.db")
	stores, err := NewSQLiteRuntimeStores(dbPath)
	if err != nil {
		t.Fatalf("create sqlite runtime stores: %v", err)
	}
	defer func() { _ = stores.DB.Close() }()
	record := OperatorActionRecord{
		ActionID:    "action-1",
		RequestID:   "request-1",
		ActionType:  OperatorActionApprove,
		Actor:       "operator",
		Status:      OperatorActionStatusApplied,
		RequestedAt: time.Date(2026, 3, 29, 13, 5, 0, 0, time.UTC),
	}
	if err := stores.OperatorActions.Save(record); err != nil {
		t.Fatalf("save operator action: %v", err)
	}
	record.ActionID = "action-2"
	if err := stores.OperatorActions.Save(record); err == nil || !IsConflict(err) {
		t.Fatalf("expected duplicate request id conflict, got %v", err)
	}
}

func waitingApprovalWithPayloadResult(workflowID string, taskID string, current state.FinancialWorldState) FollowUpWorkflowRunResult {
	result := waitingApprovalFollowUpResult(workflowID, current)
	result.CheckpointPayload = &CheckpointPayloadEnvelope{
		Kind: CheckpointPayloadKindFollowUpFinalizeResume,
		FollowUpFinalizeResume: &FollowUpFinalizeResumePayload{
			GraphID:      "graph-runtime-test",
			TaskID:       taskID,
			WorkflowID:   workflowID,
			ArtifactKind: reporting.ArtifactKindTaxOptimizationReport,
			DraftReport: reporting.ReportPayload{
				TaxOptimization: &reporting.TaxOptimizationReport{
					TaskID:               taskID,
					WorkflowID:           workflowID,
					Summary:              "draft tax optimization report",
					DeterministicMetrics: map[string]any{"effective_tax_rate": 0.22},
					RecommendedActions:   nil,
					SourceEvidenceIDs:    nil,
					RiskFlags: []analysis.RiskFlag{
						{Code: "deadline", Severity: "medium", Detail: "review before filing deadline"},
					},
					ApprovalRequired: true,
					Confidence:       0.81,
					GeneratedAt:      current.Version.UpdatedAt,
				},
			},
			PendingStateSnapshotRef: current.Version.SnapshotID,
		},
	}
	return result
}
