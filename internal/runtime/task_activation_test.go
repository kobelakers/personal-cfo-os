package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestExecuteReadyFollowUpsUsesCommittedStateAcrossIndependentSiblings(t *testing.T) {
	now := time.Date(2026, 3, 29, 11, 0, 0, 0, time.UTC)
	base := runtimeTestState(now, 1)
	seenInputVersions := make([]uint64, 0, 2)

	runtime := LocalWorkflowRuntime{
		TaskGraphs: NewInMemoryTaskGraphStore(),
		Capabilities: StaticTaskCapabilityResolver{
			Capabilities: map[taskspec.UserIntentType]string{
				taskspec.UserIntentTaxOptimization:    "tax_optimization_workflow",
				taskspec.UserIntentPortfolioRebalance: "portfolio_rebalance_workflow",
			},
			Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
				taskspec.UserIntentTaxOptimization: runtimeTestCapability{
					name: "tax_optimization_workflow",
					execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
						seenInputVersions = append(seenInputVersions, current.Version.Sequence)
						return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
					},
				},
				taskspec.UserIntentPortfolioRebalance: runtimeTestCapability{
					name: "portfolio_rebalance_workflow",
					execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
						seenInputVersions = append(seenInputVersions, current.Version.Sequence)
						return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
					},
				},
			},
		},
		Now: func() time.Time { return now },
	}

	graph := runtimeTestGraph(now,
		runtimeGeneratedTask(now, "task-tax-1", taskspec.UserIntentTaxOptimization, 1),
		runtimeGeneratedTask(now, "task-portfolio-1", taskspec.UserIntentPortfolioRebalance, 1),
	)
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := runtime.RegisterFollowUpTasks(execCtx, graph, base); err != nil {
		t.Fatalf("register tasks: %v", err)
	}

	batch, err := runtime.ExecuteReadyFollowUps(context.Background(), execCtx, graph.GraphID, DefaultAutoExecutionPolicy())
	if err != nil {
		t.Fatalf("execute ready follow-ups: %v", err)
	}
	if len(batch.ExecutedTasks) != 2 {
		t.Fatalf("expected two executed tasks, got %+v", batch.ExecutedTasks)
	}
	if !slices.Equal(seenInputVersions, []uint64{1, 2}) {
		t.Fatalf("expected committed state handoff across siblings, got %+v", seenInputVersions)
	}
	if batch.LatestCommittedStateSnapshot.State.Version.Sequence != 3 {
		t.Fatalf("expected latest committed state version 3, got %+v", batch.LatestCommittedStateSnapshot.State.Version)
	}
}

func TestExecuteReadyFollowUpsContinuesAfterWaitingApprovalForIndependentSibling(t *testing.T) {
	now := time.Date(2026, 3, 29, 11, 5, 0, 0, time.UTC)
	base := runtimeTestState(now, 1)
	seenInputVersions := make([]uint64, 0, 1)
	runtime := LocalWorkflowRuntime{
		TaskGraphs: NewInMemoryTaskGraphStore(),
		Capabilities: StaticTaskCapabilityResolver{
			Capabilities: map[taskspec.UserIntentType]string{
				taskspec.UserIntentTaxOptimization:    "tax_optimization_workflow",
				taskspec.UserIntentPortfolioRebalance: "portfolio_rebalance_workflow",
			},
			Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
				taskspec.UserIntentTaxOptimization: runtimeTestCapability{
					name: "tax_optimization_workflow",
					execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
						return waitingApprovalFollowUpResult("workflow-child-"+spec.ID, current), nil
					},
				},
				taskspec.UserIntentPortfolioRebalance: runtimeTestCapability{
					name: "portfolio_rebalance_workflow",
					execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
						seenInputVersions = append(seenInputVersions, current.Version.Sequence)
						return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
					},
				},
			},
		},
		Now: func() time.Time { return now },
	}

	graph := runtimeTestGraph(now,
		runtimeGeneratedTask(now, "task-tax-approval", taskspec.UserIntentTaxOptimization, 1),
		runtimeGeneratedTask(now, "task-portfolio-ready", taskspec.UserIntentPortfolioRebalance, 1),
	)
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := runtime.RegisterFollowUpTasks(execCtx, graph, base); err != nil {
		t.Fatalf("register tasks: %v", err)
	}

	batch, err := runtime.ExecuteReadyFollowUps(context.Background(), execCtx, graph.GraphID, DefaultAutoExecutionPolicy())
	if err != nil {
		t.Fatalf("execute ready follow-ups: %v", err)
	}
	if len(batch.ExecutedTasks) != 2 {
		t.Fatalf("expected two execution records, got %+v", batch.ExecutedTasks)
	}
	waiting := runtimeTestRecordByTaskID(t, batch.ExecutedTasks, "task-tax-approval")
	if waiting.Status != TaskQueueStatusWaitingApproval || waiting.CheckpointID == "" || waiting.ResumeToken == "" || !waiting.PendingApproval {
		t.Fatalf("expected resumable waiting approval record, got %+v", waiting)
	}
	portfolio := runtimeTestRecordByTaskID(t, batch.ExecutedTasks, "task-portfolio-ready")
	if portfolio.Status != TaskQueueStatusCompleted {
		t.Fatalf("expected independent sibling to complete, got %+v", portfolio)
	}
	if !slices.Equal(seenInputVersions, []uint64{1}) {
		t.Fatalf("expected sibling after waiting approval to read latest committed state only, got %+v", seenInputVersions)
	}
	if batch.LatestCommittedStateSnapshot.State.Version.Sequence != 2 {
		t.Fatalf("expected committed state to advance only once, got %+v", batch.LatestCommittedStateSnapshot.State.Version)
	}
}

func TestExecuteReadyFollowUpsBlocksDependentsButContinuesIndependentSiblingAfterFailure(t *testing.T) {
	now := time.Date(2026, 3, 29, 11, 10, 0, 0, time.UTC)
	base := runtimeTestState(now, 1)
	runtime := LocalWorkflowRuntime{
		TaskGraphs: NewInMemoryTaskGraphStore(),
		Capabilities: StaticTaskCapabilityResolver{
			Capabilities: map[taskspec.UserIntentType]string{
				taskspec.UserIntentTaxOptimization:    "tax_optimization_workflow",
				taskspec.UserIntentPortfolioRebalance: "portfolio_rebalance_workflow",
			},
			Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
				taskspec.UserIntentTaxOptimization: runtimeTestCapability{
					name: "tax_optimization_workflow",
					execute: func(_ context.Context, _ taskspec.TaskSpec, _ FollowUpActivationContext, _ state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
						return FollowUpWorkflowRunResult{}, &FollowUpExecutionError{
							Category: FailureCategoryUnrecoverable,
							Summary:  "simulated unrecoverable follow-up failure",
							Err:      errors.New("boom"),
						}
					},
				},
				taskspec.UserIntentPortfolioRebalance: runtimeTestCapability{
					name: "portfolio_rebalance_workflow",
					execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
						return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
					},
				},
			},
		},
		Now: func() time.Time { return now },
	}

	graph := runtimeTestGraph(now,
		runtimeGeneratedTask(now, "task-tax-fail", taskspec.UserIntentTaxOptimization, 1),
		runtimeGeneratedTask(now, "task-portfolio-blocked", taskspec.UserIntentPortfolioRebalance, 1),
		runtimeGeneratedTask(now, "task-portfolio-independent", taskspec.UserIntentPortfolioRebalance, 1),
	)
	graph.Dependencies = []taskspec.TaskDependency{
		{
			UpstreamTaskID:   "task-tax-fail",
			DownstreamTaskID: "task-portfolio-blocked",
			Reason:           "portfolio follow-up depends on tax optimization outcome",
			Mandatory:        true,
		},
	}
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := runtime.RegisterFollowUpTasks(execCtx, graph, base); err != nil {
		t.Fatalf("register tasks: %v", err)
	}

	batch, err := runtime.ExecuteReadyFollowUps(context.Background(), execCtx, graph.GraphID, DefaultAutoExecutionPolicy())
	if err != nil {
		t.Fatalf("execute ready follow-ups: %v", err)
	}
	if len(batch.ExecutedTasks) != 2 {
		t.Fatalf("expected failed task and independent sibling execution records, got %+v", batch.ExecutedTasks)
	}
	failed := runtimeTestRecordByTaskID(t, batch.ExecutedTasks, "task-tax-fail")
	if failed.Status != TaskQueueStatusFailed {
		t.Fatalf("expected failed task record, got %+v", failed)
	}
	independent := runtimeTestRecordByTaskID(t, batch.ExecutedTasks, "task-portfolio-independent")
	if independent.Status != TaskQueueStatusCompleted {
		t.Fatalf("expected independent sibling to complete, got %+v", independent)
	}
	blocked := runtimeTestTaskByID(t, batch.RegisteredTasks, "task-portfolio-blocked")
	if blocked.Status != TaskQueueStatusDependencyBlocked {
		t.Fatalf("expected dependent task to be blocked, got %+v", blocked)
	}
	if len(blocked.BlockingReasons) == 0 || !strings.Contains(blocked.BlockingReasons[0], "task-tax-fail") {
		t.Fatalf("expected blocking reason to reference failed upstream task, got %+v", blocked.BlockingReasons)
	}
}

func TestExecuteReadyFollowUpsPreservesSuppressedReasons(t *testing.T) {
	now := time.Date(2026, 3, 29, 11, 15, 0, 0, time.UTC)
	base := runtimeTestState(now, 1)
	runtime := LocalWorkflowRuntime{
		TaskGraphs: NewInMemoryTaskGraphStore(),
		Capabilities: StaticTaskCapabilityResolver{
			Capabilities: map[taskspec.UserIntentType]string{
				taskspec.UserIntentMonthlyReview:      "monthly_review_workflow",
				taskspec.UserIntentTaxOptimization:    "tax_optimization_workflow",
				taskspec.UserIntentPortfolioRebalance: "portfolio_rebalance_workflow",
			},
		},
		Now: func() time.Time { return now },
	}

	graph := runtimeTestGraph(now,
		runtimeGeneratedTask(now, "task-monthly-ready", taskspec.UserIntentMonthlyReview, 1),
		runtimeGeneratedTask(now, "task-tax-depth-2", taskspec.UserIntentTaxOptimization, 2),
	)
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := runtime.RegisterFollowUpTasks(execCtx, graph, base); err != nil {
		t.Fatalf("register tasks: %v", err)
	}

	batch, err := runtime.ExecuteReadyFollowUps(context.Background(), execCtx, graph.GraphID, DefaultAutoExecutionPolicy())
	if err != nil {
		t.Fatalf("execute ready follow-ups: %v", err)
	}
	if len(batch.ExecutedTasks) != 0 {
		t.Fatalf("expected suppressed tasks not to execute, got %+v", batch.ExecutedTasks)
	}
	monthly := runtimeTestTaskByID(t, batch.RegisteredTasks, "task-monthly-ready")
	if monthly.Status != TaskQueueStatusReady {
		t.Fatalf("expected non-allowlisted task to remain ready, got %+v", monthly)
	}
	if len(monthly.SuppressedReasons) == 0 || !strings.Contains(monthly.SuppressedReasons[0], "allowlist") {
		t.Fatalf("expected allowlist suppression reason, got %+v", monthly.SuppressedReasons)
	}
	depth := runtimeTestTaskByID(t, batch.RegisteredTasks, "task-tax-depth-2")
	if depth.Status != TaskQueueStatusReady {
		t.Fatalf("expected depth-suppressed task to remain ready, got %+v", depth)
	}
	if len(depth.SuppressedReasons) == 0 || !strings.Contains(depth.SuppressedReasons[0], "execution_depth") {
		t.Fatalf("expected execution depth suppression reason, got %+v", depth.SuppressedReasons)
	}
}

func TestExecuteReadyFollowUpsRecordsRetryHistory(t *testing.T) {
	now := time.Date(2026, 3, 29, 11, 20, 0, 0, time.UTC)
	base := runtimeTestState(now, 1)
	attempts := 0
	runtime := LocalWorkflowRuntime{
		TaskGraphs: NewInMemoryTaskGraphStore(),
		Capabilities: StaticTaskCapabilityResolver{
			Capabilities: map[taskspec.UserIntentType]string{
				taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
			},
			Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
				taskspec.UserIntentTaxOptimization: runtimeTestCapability{
					name: "tax_optimization_workflow",
					execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
						attempts++
						if attempts == 1 {
							return FollowUpWorkflowRunResult{}, &FollowUpExecutionError{
								Category: FailureCategoryTransient,
								Summary:  "temporary transport failure",
								Err:      errors.New("temporary transport failure"),
							}
						}
						return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
					},
				},
			},
		},
		Now: func() time.Time { return now },
	}

	graph := runtimeTestGraph(now, runtimeGeneratedTask(now, "task-tax-retry", taskspec.UserIntentTaxOptimization, 1))
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := runtime.RegisterFollowUpTasks(execCtx, graph, base); err != nil {
		t.Fatalf("register tasks: %v", err)
	}

	batch, err := runtime.ExecuteReadyFollowUps(context.Background(), execCtx, graph.GraphID, DefaultAutoExecutionPolicy())
	if err != nil {
		t.Fatalf("execute ready follow-ups: %v", err)
	}
	if attempts != 2 || len(batch.ExecutedTasks) != 1 {
		t.Fatalf("expected a retry path followed by success, got attempts=%d batch=%+v", attempts, batch)
	}
	record := batch.ExecutedTasks[0]
	if record.Status != TaskQueueStatusCompleted || record.Attempt != 2 || record.RetryCount != 1 || record.LastRecoveryStrategy != RecoveryStrategyRetry {
		t.Fatalf("expected retry metadata on execution record, got %+v", record)
	}
	if record.LastTransitionAt.IsZero() {
		t.Fatalf("expected retry path to update last_transition_at")
	}
}

type runtimeTestCapability struct {
	name    string
	execute func(ctx context.Context, spec taskspec.TaskSpec, activation FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error)
}

func (c runtimeTestCapability) CapabilityName() string { return c.name }

func (c runtimeTestCapability) Execute(
	ctx context.Context,
	spec taskspec.TaskSpec,
	activation FollowUpActivationContext,
	current state.FinancialWorldState,
) (FollowUpWorkflowRunResult, error) {
	return c.execute(ctx, spec, activation, current)
}

func runtimeGeneratedTask(now time.Time, taskID string, intent taskspec.UserIntentType, depth int) taskspec.GeneratedTaskSpec {
	spec := sampleGeneratedTaskSpec(now, intent)
	spec.ID = taskID
	return taskspec.GeneratedTaskSpec{
		Task: spec,
		Metadata: taskspec.GeneratedTaskMetadata{
			GeneratedAt:       now,
			ParentWorkflowID:  "workflow-life-event-rt",
			ParentTaskID:      "task-life-event-rt",
			RootCorrelationID: "workflow-life-event-rt",
			TriggerSource:     taskspec.TaskTriggerSourceLifeEvent,
			Priority:          taskspec.TaskPriorityHigh,
			ExecutionDepth:    depth,
			GenerationReasons: []taskspec.TaskGenerationReason{
				{
					Code:          taskspec.TaskGenerationReasonLifeEventImpact,
					Description:   "generated from runtime test",
					LifeEventID:   "event-runtime-1",
					LifeEventKind: "salary_change",
					EvidenceIDs:   []string{"evidence-life-event-event-runtime-1"},
				},
			},
		},
	}
}

func runtimeTestGraph(now time.Time, tasks ...taskspec.GeneratedTaskSpec) taskspec.TaskGraph {
	return taskspec.TaskGraph{
		GraphID:          "graph-runtime-test",
		ParentWorkflowID: "workflow-life-event-rt",
		ParentTaskID:     "task-life-event-rt",
		TriggerSource:    taskspec.TaskTriggerSourceLifeEvent,
		GeneratedAt:      now,
		GeneratedTasks:   tasks,
	}
}

func runtimeTestState(now time.Time, version uint64) state.FinancialWorldState {
	return state.FinancialWorldState{
		UserID: "user-1",
		Version: state.StateVersion{
			Sequence:   version,
			SnapshotID: fmt.Sprintf("state-v%d", version),
			UpdatedAt:  now,
		},
	}
}

func completedFollowUpResult(workflowID string, updated state.FinancialWorldState) FollowUpWorkflowRunResult {
	return FollowUpWorkflowRunResult{
		WorkflowID:   workflowID,
		RuntimeState: WorkflowStateCompleted,
		UpdatedState: updated,
	}
}

func waitingApprovalFollowUpResult(workflowID string, current state.FinancialWorldState) FollowUpWorkflowRunResult {
	now := current.Version.UpdatedAt
	checkpoint := CheckpointRecord{
		ID:           workflowID + "-checkpoint",
		WorkflowID:   workflowID,
		State:        WorkflowStateVerifying,
		ResumeState:  WorkflowStateVerifying,
		StateVersion: current.Version.Sequence,
		Summary:      "waiting approval",
		CapturedAt:   now,
	}
	token := ResumeToken{
		Token:        workflowID + "-resume",
		WorkflowID:   workflowID,
		CheckpointID: checkpoint.ID,
		IssuedAt:     now,
		ExpiresAt:    now.Add(24 * time.Hour),
	}
	pending := HumanApprovalPending{
		ApprovalID:      workflowID + "-approval",
		WorkflowID:      workflowID,
		RequestedAction: "follow_up_execution",
		RequestedAt:     now,
	}
	return FollowUpWorkflowRunResult{
		WorkflowID:      workflowID,
		RuntimeState:    WorkflowStateWaitingApproval,
		UpdatedState:    current,
		Checkpoint:      &checkpoint,
		ResumeToken:     &token,
		PendingApproval: &pending,
	}
}

func runtimeTestTaskByID(t *testing.T, tasks []FollowUpTaskRecord, taskID string) FollowUpTaskRecord {
	t.Helper()
	for _, item := range tasks {
		if item.Task.ID == taskID {
			return item
		}
	}
	t.Fatalf("expected task %q in %+v", taskID, tasks)
	return FollowUpTaskRecord{}
}

func runtimeTestRecordByTaskID(t *testing.T, records []TaskExecutionRecord, taskID string) TaskExecutionRecord {
	t.Helper()
	for _, item := range records {
		if item.TaskID == taskID {
			return item
		}
	}
	t.Fatalf("expected execution record %q in %+v", taskID, records)
	return TaskExecutionRecord{}
}
