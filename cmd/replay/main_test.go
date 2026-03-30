package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestRunPrintsReplaySummary(t *testing.T) {
	seed := seedReplayRuntimeDB(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"--runtime-db", seed.dbPath, "--task-graph-id", seed.graphID, "--format", "summary"}, &stdout, &stderr); err != nil {
		t.Fatalf("run replay summary: %v stderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "scope=task_graph:"+seed.graphID) {
		t.Fatalf("expected replay summary to include task graph scope, got %s", output)
	}
	if !strings.Contains(output, "final_state=completed") {
		t.Fatalf("expected replay summary to include completed state, got %s", output)
	}
}

func TestRunPrintsApprovalReplaySummary(t *testing.T) {
	seed := seedReplayRuntimeDB(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"--runtime-db", seed.dbPath, "--approval-id", seed.approvalID, "--format", "summary"}, &stdout, &stderr); err != nil {
		t.Fatalf("run replay approval summary: %v stderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "scope=approval:"+seed.approvalID) {
		t.Fatalf("expected approval replay summary to include approval scope, got %s", output)
	}
	if !strings.Contains(output, "final_state=waiting_approval") {
		t.Fatalf("expected approval replay summary to include waiting_approval state, got %s", output)
	}
}

func TestRunPrintsComparisonDetails(t *testing.T) {
	seed := seedReplayRuntimeDB(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{
		"--runtime-db", seed.dbPath,
		"--compare-left", "workflow:" + seed.compareLeftWorkflowID,
		"--compare-right", "workflow:" + seed.compareRightWorkflowID,
		"--format", "summary",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("run replay comparison: %v stderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "[memory] memory selection/rejection changed") {
		t.Fatalf("expected replay comparison to include memory diff summary, got %s", output)
	}
	if !strings.Contains(output, "detail=added:") || !strings.Contains(output, "detail=left_count=") || !strings.Contains(output, "detail=right_count=") {
		t.Fatalf("expected replay comparison to include content-level detail lines, got %s", output)
	}
}

func TestRunRebuildsProjections(t *testing.T) {
	seed := seedReplayRuntimeDB(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"--runtime-db", seed.dbPath, "--rebuild-projections", "--task-graph-id", seed.graphID, "--format", "json"}, &stdout, &stderr); err != nil {
		t.Fatalf("run replay rebuild: %v stderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "\"schema_version\": 1") {
		t.Fatalf("expected rebuild output to include replay projection schema version, got %s", output)
	}
}

type replayCLITestSeed struct {
	dbPath                 string
	graphID                string
	approvalID             string
	compareLeftWorkflowID  string
	compareRightWorkflowID string
}

func seedReplayRuntimeDB(t *testing.T) replayCLITestSeed {
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
	rebuilder := runtime.NewReplayProjectionRebuilder(service, stores.WorkflowRuns, stores.ReplayProjection, stores.Artifacts, stores.Replay, func() time.Time { return now })
	service.SetReplayProjectionWriter(rebuilder)
	service.SetCapabilities(runtime.StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization:    "tax_optimization_workflow",
			taskspec.UserIntentPortfolioRebalance: "portfolio_rebalance_workflow",
		},
		Workflows: map[taskspec.UserIntentType]runtime.FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization:    replayCLITestCompletedCapability{},
			taskspec.UserIntentPortfolioRebalance: replayCLITestApprovalCapability{},
		},
	})

	completedParentWorkflowID := "workflow-life-event-cli"
	completedParentTaskID := "task-life-event-cli"
	completedGraph := taskspec.TaskGraph{
		GraphID:          "graph-replay-cli-test",
		ParentWorkflowID: completedParentWorkflowID,
		ParentTaskID:     completedParentTaskID,
		TriggerSource:    taskspec.TaskTriggerSourceLifeEvent,
		GeneratedAt:      now,
		GeneratedTasks: []taskspec.GeneratedTaskSpec{
			replayCLISeedGeneratedTask(now, completedParentWorkflowID, completedParentTaskID, "task-tax-cli", taskspec.UserIntentTaxOptimization),
		},
	}
	completedExecCtx := runtime.ExecutionContext{WorkflowID: completedGraph.ParentWorkflowID, TaskID: completedGraph.ParentTaskID, CorrelationID: completedGraph.ParentWorkflowID, Attempt: 1}
	if _, err := service.Runtime().RegisterFollowUpTasks(completedExecCtx, completedGraph, replayCLITestState(now, 1)); err != nil {
		t.Fatalf("register replay cli graph: %v", err)
	}
	if _, err := service.ExecuteAutoReadyFollowUps(context.Background(), completedGraph.GraphID, runtime.DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("execute replay cli graph: %v", err)
	}
	if _, err := rebuilder.RebuildTaskGraph(context.Background(), completedGraph.GraphID); err != nil {
		t.Fatalf("rebuild replay cli graph: %v", err)
	}

	waitingApprovalParentWorkflowID := "workflow-life-event-cli-approval"
	waitingApprovalParentTaskID := "task-life-event-cli-approval"
	waitingApprovalGraph := taskspec.TaskGraph{
		GraphID:          "graph-replay-cli-approval",
		ParentWorkflowID: waitingApprovalParentWorkflowID,
		ParentTaskID:     waitingApprovalParentTaskID,
		TriggerSource:    taskspec.TaskTriggerSourceLifeEvent,
		GeneratedAt:      now.Add(time.Minute),
		GeneratedTasks: []taskspec.GeneratedTaskSpec{
			replayCLISeedGeneratedTask(now.Add(time.Minute), waitingApprovalParentWorkflowID, waitingApprovalParentTaskID, "task-portfolio-cli", taskspec.UserIntentPortfolioRebalance),
		},
	}
	approvalExecCtx := runtime.ExecutionContext{WorkflowID: waitingApprovalGraph.ParentWorkflowID, TaskID: waitingApprovalGraph.ParentTaskID, CorrelationID: waitingApprovalGraph.ParentWorkflowID, Attempt: 1}
	if _, err := service.Runtime().RegisterFollowUpTasks(approvalExecCtx, waitingApprovalGraph, replayCLITestState(now.Add(time.Minute), 2)); err != nil {
		t.Fatalf("register replay cli approval graph: %v", err)
	}
	if _, err := service.ExecuteAutoReadyFollowUps(context.Background(), waitingApprovalGraph.GraphID, runtime.DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("execute replay cli approval graph: %v", err)
	}
	if _, err := rebuilder.RebuildTaskGraph(context.Background(), waitingApprovalGraph.GraphID); err != nil {
		t.Fatalf("rebuild replay cli approval graph: %v", err)
	}
	approval, ok, err := service.Runtime().Approvals.LoadByTask(waitingApprovalGraph.GraphID, "task-portfolio-cli")
	if err != nil || !ok {
		t.Fatalf("load replay cli approval state: ok=%t err=%v", ok, err)
	}

	saveWorkflowProjectionForCompare(t, stores, "workflow-compare-left", now, runtime.WorkflowStateCompleted, []string{"cashflow-review", "debt-review"}, []string{"selected=memory-stable"}, []string{"validation_pass"}, []string{"governance_allow"}, []string{"child:tax:completed"})
	saveWorkflowProjectionForCompare(t, stores, "workflow-compare-right", now.Add(2*time.Minute), runtime.WorkflowStateCompleted, []string{"cashflow-review", "debt-review", "tax-review"}, []string{"selected=memory-stable", "rejection_rule=memory-rejected:stale_episodic"}, []string{"validation_pass", "business_warning"}, []string{"governance_allow"}, []string{"child:tax:completed", "child:portfolio:completed"})

	if err := stores.DB.Close(); err != nil {
		t.Fatalf("close replay cli db: %v", err)
	}
	return replayCLITestSeed{
		dbPath:                 dbPath,
		graphID:                completedGraph.GraphID,
		approvalID:             approval.ApprovalID,
		compareLeftWorkflowID:  "workflow-compare-left",
		compareRightWorkflowID: "workflow-compare-right",
	}
}

func replayCLISeedGeneratedTask(now time.Time, parentWorkflowID string, parentTaskID string, taskID string, intent taskspec.UserIntentType) taskspec.GeneratedTaskSpec {
	return taskspec.GeneratedTaskSpec{
		Task: taskspec.TaskSpec{
			ID:    taskID,
			Goal:  "deterministic replay cli test",
			Scope: taskspec.TaskScope{Areas: []string{"finance"}},
			Constraints: taskspec.ConstraintSet{
				Hard: []string{"must remain deterministic"},
			},
			RiskLevel:           taskspec.RiskLevelMedium,
			SuccessCriteria:     []taskspec.SuccessCriteria{{ID: "done", Description: "complete task"}},
			RequiredEvidence:    []taskspec.RequiredEvidenceRef{{Type: "event_signal", Reason: "cli test", Mandatory: true}},
			ApprovalRequirement: taskspec.ApprovalRequirementRecommended,
			UserIntentType:      intent,
			CreatedAt:           now,
		},
		Metadata: taskspec.GeneratedTaskMetadata{
			GeneratedAt:       now,
			ParentWorkflowID:  parentWorkflowID,
			ParentTaskID:      parentTaskID,
			RootCorrelationID: parentWorkflowID,
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
	}
}

func replayCLITestState(now time.Time, version uint64) state.FinancialWorldState {
	return state.FinancialWorldState{
		UserID: "user-1",
		Version: state.StateVersion{
			Sequence:   version,
			SnapshotID: "cli-state-v" + time.Unix(int64(version), 0).UTC().Format("150405"),
			UpdatedAt:  now,
		},
	}
}

func saveWorkflowProjectionForCompare(t *testing.T, stores *runtime.SQLiteRuntimeStores, workflowID string, now time.Time, finalState runtime.WorkflowExecutionState, plan []string, memorySummary []string, validator []string, governance []string, child []string) {
	t.Helper()
	if err := stores.WorkflowRuns.Save(runtime.WorkflowRunRecord{
		WorkflowID:   workflowID,
		TaskID:       "task-" + workflowID,
		Intent:       string(taskspec.UserIntentMonthlyReview),
		RuntimeState: finalState,
		Summary:      "workflow compare seed",
		StartedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("save workflow run compare seed: %v", err)
	}
	summaryJSON, err := json.Marshal(map[string]any{
		"goal_summary":           "monthly review compare seed",
		"plan_summary":           plan,
		"memory_summary":         memorySummary,
		"validator_summary":      validator,
		"governance_summary":     governance,
		"child_workflow_summary": child,
		"final_state":            string(finalState),
	})
	if err != nil {
		t.Fatalf("marshal workflow compare seed summary: %v", err)
	}
	explanationJSON, err := json.Marshal(map[string]any{
		"why_memory_decision":   memorySummary,
		"why_validation_failed": validator,
	})
	if err != nil {
		t.Fatalf("marshal workflow compare seed explanation: %v", err)
	}
	if err := stores.ReplayProjection.SaveWorkflowProjection(runtime.WorkflowReplayProjection{
		WorkflowID:          workflowID,
		TaskID:              "task-" + workflowID,
		Intent:              string(taskspec.UserIntentMonthlyReview),
		RuntimeState:        finalState,
		ProjectionStatus:    runtime.ReplayProjectionStatusComplete,
		SchemaVersion:       runtime.ReplayProjectionSchemaVersion,
		SummaryJSON:         string(summaryJSON),
		ExplanationJSON:     string(explanationJSON),
		CompareInputJSON:    string(summaryJSON),
		UpdatedAt:           now,
		ProjectionFreshness: now,
	}); err != nil {
		t.Fatalf("save workflow compare projection: %v", err)
	}
	if err := stores.ReplayProjection.SaveBuild(runtime.ReplayProjectionBuildRecord{
		ScopeKind:           runtime.ReplayScopeWorkflow,
		ScopeID:             workflowID,
		SchemaVersion:       runtime.ReplayProjectionSchemaVersion,
		Status:              runtime.ReplayProjectionStatusComplete,
		BuiltAt:             now,
		SourceEventCount:    1,
		SourceArtifactCount: 0,
	}); err != nil {
		t.Fatalf("save workflow compare build: %v", err)
	}
}

type replayCLITestCompletedCapability struct{}

func (replayCLITestCompletedCapability) CapabilityName() string { return "tax_optimization_workflow" }

func (replayCLITestCompletedCapability) Execute(_ context.Context, spec taskspec.TaskSpec, _ runtime.FollowUpActivationContext, current state.FinancialWorldState) (runtime.FollowUpWorkflowRunResult, error) {
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

func (replayCLITestCompletedCapability) Resume(_ context.Context, spec taskspec.TaskSpec, activation runtime.FollowUpActivationContext, current state.FinancialWorldState, checkpoint runtime.CheckpointRecord, token runtime.ResumeToken, payload runtime.CheckpointPayloadEnvelope) (runtime.FollowUpWorkflowRunResult, error) {
	return replayCLITestCompletedCapability{}.Execute(context.Background(), spec, activation, current)
}

type replayCLITestApprovalCapability struct{}

func (replayCLITestApprovalCapability) CapabilityName() string { return "portfolio_rebalance_workflow" }

func (replayCLITestApprovalCapability) Execute(_ context.Context, spec taskspec.TaskSpec, _ runtime.FollowUpActivationContext, current state.FinancialWorldState) (runtime.FollowUpWorkflowRunResult, error) {
	now := current.Version.UpdatedAt
	checkpoint := runtime.CheckpointRecord{
		ID:           "workflow-child-" + spec.ID + "-checkpoint",
		WorkflowID:   "workflow-child-" + spec.ID,
		State:        runtime.WorkflowStateVerifying,
		ResumeState:  runtime.WorkflowStateVerifying,
		StateVersion: current.Version.Sequence,
		Summary:      "waiting approval",
		CapturedAt:   now,
	}
	token := runtime.ResumeToken{
		Token:        "workflow-child-" + spec.ID + "-resume",
		WorkflowID:   "workflow-child-" + spec.ID,
		CheckpointID: checkpoint.ID,
		IssuedAt:     now,
		ExpiresAt:    now.Add(24 * time.Hour),
	}
	pending := runtime.HumanApprovalPending{
		ApprovalID:      "workflow-child-" + spec.ID + "-approval",
		WorkflowID:      "workflow-child-" + spec.ID,
		RequestedAction: "follow_up_execution",
		RequestedAt:     now,
	}
	return runtime.FollowUpWorkflowRunResult{
		WorkflowID:      "workflow-child-" + spec.ID,
		RuntimeState:    runtime.WorkflowStateWaitingApproval,
		UpdatedState:    current,
		Checkpoint:      &checkpoint,
		ResumeToken:     &token,
		PendingApproval: &pending,
	}, nil
}

func (replayCLITestApprovalCapability) Resume(_ context.Context, spec taskspec.TaskSpec, _ runtime.FollowUpActivationContext, current state.FinancialWorldState, _ runtime.CheckpointRecord, _ runtime.ResumeToken, _ runtime.CheckpointPayloadEnvelope) (runtime.FollowUpWorkflowRunResult, error) {
	next := current
	next.Version.Sequence++
	next.Version.SnapshotID = "state-v3"
	next.Version.UpdatedAt = current.Version.UpdatedAt
	return runtime.FollowUpWorkflowRunResult{
		WorkflowID:   "workflow-child-" + spec.ID,
		RuntimeState: runtime.WorkflowStateCompleted,
		UpdatedState: next,
	}, nil
}
