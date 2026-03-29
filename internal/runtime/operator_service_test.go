package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestApproveTaskAutoResumeCompletesWaitingTask(t *testing.T) {
	now := time.Date(2026, 3, 29, 14, 0, 0, 0, time.UTC)
	executeCalls := 0
	resumeCalls := 0
	service, taskID := seedWaitingApprovalService(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			executeCalls++
			return waitingApprovalWithPayloadResult("workflow-child-"+spec.ID, spec.ID, current), nil
		},
		resume: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState, _ CheckpointRecord, _ ResumeToken, _ CheckpointPayloadEnvelope) (FollowUpWorkflowRunResult, error) {
			resumeCalls++
			return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
		},
	})
	operator := NewOperatorService(service)

	approval, ok, err := service.runtime.Approvals.LoadByTask("graph-runtime-test", taskID)
	if err != nil || !ok {
		t.Fatalf("load seeded approval: %v %+v", err, approval)
	}
	result, err := operator.ApproveTask(context.Background(), ApproveTaskCommand{
		RequestID:  "approve-1",
		ApprovalID: approval.ApprovalID,
		Actor:      "operator",
		Roles:      []string{"operator"},
		Note:       "approve and auto resume",
	})
	if err != nil {
		t.Fatalf("approve task: %v", err)
	}
	if !result.AutoResumeTried || !result.AutoResumeApplied || result.Status != TaskQueueStatusCompleted {
		t.Fatalf("expected auto resume completion, got %+v", result)
	}
	if executeCalls != 1 || resumeCalls != 1 {
		t.Fatalf("expected execute once and resume once, got execute=%d resume=%d", executeCalls, resumeCalls)
	}
	latest, ok, err := service.runtime.Executions.LoadLatestByTask("graph-runtime-test", taskID)
	if err != nil || !ok {
		t.Fatalf("load latest execution: %v %+v", err, latest)
	}
	if latest.Status != TaskQueueStatusCompleted || !latest.Committed {
		t.Fatalf("expected committed completed execution, got %+v", latest)
	}
	graph, err := service.loadTaskGraph("graph-runtime-test")
	if err != nil {
		t.Fatalf("load task graph: %v", err)
	}
	task := runtimeTestTaskByID(t, graph.RegisteredTasks, taskID)
	if task.Status != TaskQueueStatusCompleted {
		t.Fatalf("expected task completed after auto resume, got %+v", task)
	}
}

func TestApproveTaskKeepsApprovalResolvedWhenAutoResumeFails(t *testing.T) {
	now := time.Date(2026, 3, 29, 14, 10, 0, 0, time.UTC)
	resumeCalls := 0
	service, taskID := seedWaitingApprovalService(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			return waitingApprovalWithPayloadResult("workflow-child-"+spec.ID, spec.ID, current), nil
		},
		resume: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState, _ CheckpointRecord, _ ResumeToken, _ CheckpointPayloadEnvelope) (FollowUpWorkflowRunResult, error) {
			resumeCalls++
			if resumeCalls == 1 {
				return FollowUpWorkflowRunResult{}, errors.New("temporary finalize transport failure")
			}
			return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
		},
	})
	operator := NewOperatorService(service)

	approval, ok, err := service.runtime.Approvals.LoadByTask("graph-runtime-test", taskID)
	if err != nil || !ok {
		t.Fatalf("load seeded approval: %v %+v", err, approval)
	}
	approved, err := operator.ApproveTask(context.Background(), ApproveTaskCommand{
		RequestID:  "approve-2",
		ApprovalID: approval.ApprovalID,
		Actor:      "operator",
		Roles:      []string{"operator"},
		Note:       "approve first",
	})
	if err != nil {
		t.Fatalf("approve task: %v", err)
	}
	if !approved.AutoResumeTried || approved.AutoResumeApplied || approved.FailureSummary == "" {
		t.Fatalf("expected auto resume failure summary with resolved approval, got %+v", approved)
	}
	approval, ok, err = service.runtime.Approvals.Load(approval.ApprovalID)
	if err != nil || !ok {
		t.Fatalf("reload approval: %v %+v", err, approval)
	}
	if approval.Status != ApprovalStatusApproved {
		t.Fatalf("expected approval to remain approved, got %+v", approval)
	}
	graph, err := service.loadTaskGraph("graph-runtime-test")
	if err != nil {
		t.Fatalf("load task graph: %v", err)
	}
	task := runtimeTestTaskByID(t, graph.RegisteredTasks, taskID)
	if task.Status != TaskQueueStatusWaitingApproval {
		t.Fatalf("expected task to remain waiting approval for explicit resume, got %+v", task)
	}
	resumed, err := operator.ResumeFollowUpTask(context.Background(), ResumeFollowUpTaskCommand{
		RequestID: "resume-2",
		GraphID:   "graph-runtime-test",
		TaskID:    taskID,
		Actor:     "operator",
		Roles:     []string{"operator"},
		Note:      "explicit resume after auto-resume failure",
	})
	if err != nil {
		t.Fatalf("explicit resume: %v", err)
	}
	if resumed.Status != TaskQueueStatusCompleted || resumeCalls != 2 {
		t.Fatalf("expected explicit resume to complete on second attempt, got %+v resumeCalls=%d", resumed, resumeCalls)
	}
}

func TestDenyTaskMapsToFailedDeniedByOperator(t *testing.T) {
	now := time.Date(2026, 3, 29, 14, 20, 0, 0, time.UTC)
	service, taskID := seedWaitingApprovalService(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			return waitingApprovalWithPayloadResult("workflow-child-"+spec.ID, spec.ID, current), nil
		},
	})
	operator := NewOperatorService(service)

	approval, ok, err := service.runtime.Approvals.LoadByTask("graph-runtime-test", taskID)
	if err != nil || !ok {
		t.Fatalf("load seeded approval: %v %+v", err, approval)
	}
	result, err := operator.DenyTask(context.Background(), DenyTaskCommand{
		RequestID:  "deny-1",
		ApprovalID: approval.ApprovalID,
		Actor:      "operator",
		Roles:      []string{"operator"},
		Note:       "operator denied the follow-up",
	})
	if err != nil {
		t.Fatalf("deny task: %v", err)
	}
	if result.Status != TaskQueueStatusFailed {
		t.Fatalf("expected failed task command result, got %+v", result)
	}
	latest, ok, err := service.runtime.Executions.LoadLatestByTask("graph-runtime-test", taskID)
	if err != nil || !ok {
		t.Fatalf("load latest execution: %v %+v", err, latest)
	}
	if latest.FailureCategory != FailureCategoryDeniedByOp || latest.Status != TaskQueueStatusFailed {
		t.Fatalf("expected denied_by_operator failed execution, got %+v", latest)
	}
	graph, err := service.loadTaskGraph("graph-runtime-test")
	if err != nil {
		t.Fatalf("load task graph: %v", err)
	}
	if graph.LatestCommittedStateSnapshot.State.Version.Sequence != 1 {
		t.Fatalf("deny must not advance committed state, got %+v", graph.LatestCommittedStateSnapshot.State.Version)
	}
}

func TestResumeFollowUpTaskIsIdempotentByRequestID(t *testing.T) {
	now := time.Date(2026, 3, 29, 14, 30, 0, 0, time.UTC)
	service, taskID := seedWaitingApprovalService(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			return waitingApprovalWithPayloadResult("workflow-child-"+spec.ID, spec.ID, current), nil
		},
		resume: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState, _ CheckpointRecord, _ ResumeToken, _ CheckpointPayloadEnvelope) (FollowUpWorkflowRunResult, error) {
			return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
		},
	})
	operator := NewOperatorService(service)
	approval, ok, err := service.runtime.Approvals.LoadByTask("graph-runtime-test", taskID)
	if err != nil || !ok {
		t.Fatalf("load approval: %v %+v", err, approval)
	}
	approval.Status = ApprovalStatusApproved
	resolvedAt := now.Add(time.Minute)
	approval.ResolvedAt = &resolvedAt
	approval.ResolvedBy = "operator"
	if err := service.runtime.Approvals.Update(approval, approval.Version); err != nil {
		t.Fatalf("approve task in store: %v", err)
	}
	first, err := operator.ResumeFollowUpTask(context.Background(), ResumeFollowUpTaskCommand{
		RequestID: "resume-idempotent",
		GraphID:   "graph-runtime-test",
		TaskID:    taskID,
		Actor:     "operator",
		Roles:     []string{"operator"},
	})
	if err != nil {
		t.Fatalf("first resume: %v", err)
	}
	second, err := operator.ResumeFollowUpTask(context.Background(), ResumeFollowUpTaskCommand{
		RequestID: "resume-idempotent",
		GraphID:   "graph-runtime-test",
		TaskID:    taskID,
		Actor:     "operator",
		Roles:     []string{"operator"},
	})
	if err != nil {
		t.Fatalf("second resume with same request id: %v", err)
	}
	if first.ExecutionID == "" || first.ExecutionID != second.ExecutionID || second.Action.RequestID != "resume-idempotent" {
		t.Fatalf("expected idempotent resume result, got first=%+v second=%+v", first, second)
	}
}

func TestRetryFailedFollowUpTaskExecutesOperatorRequestedRetry(t *testing.T) {
	now := time.Date(2026, 3, 29, 14, 40, 0, 0, time.UTC)
	executeCalls := 0
	service := NewService(ServiceOptions{
		CheckpointStore: NewInMemoryCheckpointStore(),
		TaskGraphs:      NewInMemoryTaskGraphStore(),
		Executions:      NewInMemoryTaskExecutionStore(),
		Approvals:       NewInMemoryApprovalStateStore(),
		OperatorActions: NewInMemoryOperatorActionStore(),
		Replay:          NewInMemoryReplayStore(),
		Artifacts:       NewInMemoryArtifactMetadataStore(),
		Controller:      DefaultWorkflowController{},
		Now:             func() time.Time { return now },
	})
	service.SetCapabilities(StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
		},
		Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: runtimeTestCapability{
				name: "tax_optimization_workflow",
				execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
					executeCalls++
					if executeCalls == 1 {
						return FollowUpWorkflowRunResult{}, &FollowUpExecutionError{
							Category: FailureCategoryUnrecoverable,
							Summary:  "initial failure",
							Err:      errors.New("boom"),
						}
					}
					return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
				},
			},
		},
	})
	taskID := "task-tax-retry"
	base := runtimeTestState(now, 1)
	graph := runtimeTestGraph(now, runtimeGeneratedTask(now, taskID, taskspec.UserIntentTaxOptimization, 1))
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := service.runtime.RegisterFollowUpTasks(execCtx, graph, base); err != nil {
		t.Fatalf("register follow-up tasks: %v", err)
	}
	if _, err := service.runtime.ExecuteReadyFollowUps(context.Background(), execCtx, graph.GraphID, DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("execute seeded failure: %v", err)
	}
	operator := NewOperatorService(service)
	result, err := operator.RetryFailedFollowUpTask(context.Background(), RetryFailedFollowUpTaskCommand{
		RequestID: "retry-1",
		GraphID:   graph.GraphID,
		TaskID:    taskID,
		Actor:     "operator",
		Roles:     []string{"operator"},
		Note:      "retry failed follow-up",
	})
	if err != nil {
		t.Fatalf("retry failed follow-up: %v", err)
	}
	if result.Status != TaskQueueStatusCompleted || executeCalls != 2 {
		t.Fatalf("expected retry to complete on second attempt, got %+v executeCalls=%d", result, executeCalls)
	}
}

func seedWaitingApprovalService(t *testing.T, now time.Time, capability runtimeTestCapability) (*Service, string) {
	t.Helper()
	service := NewService(ServiceOptions{
		CheckpointStore: NewInMemoryCheckpointStore(),
		TaskGraphs:      NewInMemoryTaskGraphStore(),
		Executions:      NewInMemoryTaskExecutionStore(),
		Approvals:       NewInMemoryApprovalStateStore(),
		OperatorActions: NewInMemoryOperatorActionStore(),
		Replay:          NewInMemoryReplayStore(),
		Artifacts:       NewInMemoryArtifactMetadataStore(),
		Controller:      DefaultWorkflowController{},
		Now:             func() time.Time { return now },
	})
	service.SetCapabilities(StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
		},
		Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: capability,
		},
	})
	taskID := "task-tax-operator"
	base := runtimeTestState(now, 1)
	graph := runtimeTestGraph(now, runtimeGeneratedTask(now, taskID, taskspec.UserIntentTaxOptimization, 1))
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := service.runtime.RegisterFollowUpTasks(execCtx, graph, base); err != nil {
		t.Fatalf("register follow-up tasks: %v", err)
	}
	if _, err := service.runtime.ExecuteReadyFollowUps(context.Background(), execCtx, graph.GraphID, DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("execute seeded follow-up: %v", err)
	}
	return service, taskID
}
