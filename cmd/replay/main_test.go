package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestRunPrintsReplaySummary(t *testing.T) {
	dbPath, graphID := seedReplayRuntimeDB(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"--runtime-db", dbPath, "--task-graph-id", graphID, "--format", "summary"}, &stdout, &stderr); err != nil {
		t.Fatalf("run replay summary: %v stderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "scope=task_graph:"+graphID) {
		t.Fatalf("expected replay summary to include task graph scope, got %s", output)
	}
	if !strings.Contains(output, "final_state=completed") {
		t.Fatalf("expected replay summary to include completed state, got %s", output)
	}
}

func TestRunRebuildsProjections(t *testing.T) {
	dbPath, graphID := seedReplayRuntimeDB(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"--runtime-db", dbPath, "--rebuild-projections", "--task-graph-id", graphID, "--format", "json"}, &stdout, &stderr); err != nil {
		t.Fatalf("run replay rebuild: %v stderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "\"schema_version\": 1") {
		t.Fatalf("expected rebuild output to include replay projection schema version, got %s", output)
	}
}

func seedReplayRuntimeDB(t *testing.T) (string, string) {
	t.Helper()

	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	dbPath := t.TempDir() + "/runtime.db"
	stores, err := runtime.NewSQLiteRuntimeStores(dbPath)
	if err != nil {
		t.Fatalf("open sqlite runtime stores: %v", err)
	}
	service := runtime.NewService(runtime.ServiceOptions{
		CheckpointStore: stores.Checkpoints,
		TaskGraphs:      stores.TaskGraphs,
		Executions:      stores.Executions,
		Approvals:       stores.Approvals,
		OperatorActions: stores.OperatorActions,
		Replay:          stores.Replay,
		Artifacts:       stores.Artifacts,
		Controller:      runtime.DefaultWorkflowController{},
		Now:             func() time.Time { return now },
	})
	workflowRuns := runtime.NewInMemoryWorkflowRunStore()
	rebuilder := runtime.NewReplayProjectionRebuilder(service, workflowRuns, stores.ReplayProjection, stores.Artifacts, stores.Replay, func() time.Time { return now })
	service.SetReplayProjectionWriter(rebuilder)
	service.SetCapabilities(runtime.StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
		},
		Workflows: map[taskspec.UserIntentType]runtime.FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: replayCLITestCapability{},
		},
	})

	graph := taskspec.TaskGraph{
		GraphID:          "graph-replay-cli-test",
		ParentWorkflowID: "workflow-life-event-cli",
		ParentTaskID:     "task-life-event-cli",
		TriggerSource:    taskspec.TaskTriggerSourceLifeEvent,
		GeneratedAt:      now,
		GeneratedTasks: []taskspec.GeneratedTaskSpec{
			{
				Task: taskspec.TaskSpec{
					ID:    "task-tax-cli",
					Goal:  "deterministic replay cli test",
					Scope: taskspec.TaskScope{Areas: []string{"tax"}},
					Constraints: taskspec.ConstraintSet{
						Hard: []string{"must remain deterministic"},
					},
					RiskLevel:           taskspec.RiskLevelMedium,
					SuccessCriteria:     []taskspec.SuccessCriteria{{ID: "done", Description: "complete task"}},
					RequiredEvidence:    []taskspec.RequiredEvidenceRef{{Type: "event_signal", Reason: "cli test", Mandatory: true}},
					ApprovalRequirement: taskspec.ApprovalRequirementRecommended,
					UserIntentType:      taskspec.UserIntentTaxOptimization,
					CreatedAt:           now,
				},
				Metadata: taskspec.GeneratedTaskMetadata{
					GeneratedAt:       now,
					ParentWorkflowID:  "workflow-life-event-cli",
					ParentTaskID:      "task-life-event-cli",
					RootCorrelationID: "workflow-life-event-cli",
					TriggerSource:     taskspec.TaskTriggerSourceLifeEvent,
					Priority:          taskspec.TaskPriorityHigh,
					ExecutionDepth:    1,
					GenerationReasons: []taskspec.TaskGenerationReason{{
						Code:          taskspec.TaskGenerationReasonLifeEventImpact,
						Description:   "replay cli test",
						LifeEventID:   "event-cli",
						LifeEventKind: "salary_change",
						EvidenceIDs:   []string{"evidence-life-event-event-cli"},
					}},
				},
			},
		},
	}
	execCtx := runtime.ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := service.Runtime().RegisterFollowUpTasks(execCtx, graph, state.FinancialWorldState{
		UserID: "user-1",
		Version: state.StateVersion{
			Sequence:   1,
			SnapshotID: "state-v1",
			UpdatedAt:  now,
		},
	}); err != nil {
		t.Fatalf("register replay cli graph: %v", err)
	}
	if _, err := service.ExecuteAutoReadyFollowUps(context.Background(), graph.GraphID, runtime.DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("execute replay cli graph: %v", err)
	}
	if _, err := rebuilder.RebuildTaskGraph(context.Background(), graph.GraphID); err != nil {
		t.Fatalf("rebuild replay cli graph: %v", err)
	}
	if err := stores.DB.Close(); err != nil {
		t.Fatalf("close replay cli db: %v", err)
	}
	return dbPath, graph.GraphID
}

type replayCLITestCapability struct{}

func (replayCLITestCapability) CapabilityName() string { return "tax_optimization_workflow" }

func (replayCLITestCapability) Execute(_ context.Context, spec taskspec.TaskSpec, _ runtime.FollowUpActivationContext, current state.FinancialWorldState) (runtime.FollowUpWorkflowRunResult, error) {
	next := current
	next.Version.Sequence++
	next.Version.SnapshotID = "state-v2"
	next.Version.UpdatedAt = current.Version.UpdatedAt
	return runtime.FollowUpWorkflowRunResult{
		WorkflowID:   "workflow-child-" + spec.ID,
		RuntimeState: runtime.WorkflowStateCompleted,
		UpdatedState: next,
		Artifacts: []reporting.WorkflowArtifact{{
			ID:         "artifact-" + spec.ID,
			WorkflowID: "workflow-child-" + spec.ID,
			TaskID:     spec.ID,
			Kind:       reporting.ArtifactKindTaxOptimizationReport,
			ProducedBy: "cmd_replay_test",
			Ref: reporting.ArtifactRef{
				Kind:    reporting.ArtifactKindTaxOptimizationReport,
				ID:      "artifact-" + spec.ID,
				Summary: "replay cli artifact",
			},
			ContentJSON: `{"kind":"tax_optimization_report"}`,
			CreatedAt:   current.Version.UpdatedAt,
		}},
	}, nil
}

func (replayCLITestCapability) Resume(_ context.Context, spec taskspec.TaskSpec, activation runtime.FollowUpActivationContext, current state.FinancialWorldState, checkpoint runtime.CheckpointRecord, token runtime.ResumeToken, payload runtime.CheckpointPayloadEnvelope) (runtime.FollowUpWorkflowRunResult, error) {
	return replayCLITestCapability{}.Execute(context.Background(), spec, activation, current)
}
