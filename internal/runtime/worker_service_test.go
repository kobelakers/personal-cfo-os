package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestWorkerPassResumesApprovedWaitingTask(t *testing.T) {
	now := time.Date(2026, 3, 29, 16, 30, 0, 0, time.UTC)
	service, taskID := seedWaitingApprovalService(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			return waitingApprovalWithPayloadResult("workflow-child-"+spec.ID, spec.ID, current), nil
		},
		resume: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState, _ CheckpointRecord, _ ResumeToken, _ CheckpointPayloadEnvelope) (FollowUpWorkflowRunResult, error) {
			return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
		},
	})
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
	result, err := service.RunWorkerPass(context.Background(), DefaultAutoExecutionPolicy(), false)
	if err != nil {
		t.Fatalf("run worker pass: %v", err)
	}
	if len(result.ResumedTasks) != 1 || result.ResumedTasks[0] != taskID {
		t.Fatalf("expected worker to resume approved waiting task, got %+v", result)
	}
	latest, ok, err := service.runtime.Executions.LoadLatestByTask("graph-runtime-test", taskID)
	if err != nil || !ok {
		t.Fatalf("load latest execution: %v %+v", err, latest)
	}
	if latest.Status != TaskQueueStatusCompleted {
		t.Fatalf("expected resumed task to complete, got %+v", latest)
	}
}

func TestWorkerPassDryRunDoesNotMutateTaskGraph(t *testing.T) {
	now := time.Date(2026, 3, 29, 16, 40, 0, 0, time.UTC)
	service, taskID := seedWaitingApprovalService(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			return waitingApprovalWithPayloadResult("workflow-child-"+spec.ID, spec.ID, current), nil
		},
	})
	result, err := service.RunWorkerPass(context.Background(), DefaultAutoExecutionPolicy(), true)
	if err != nil {
		t.Fatalf("run dry-run worker pass: %v", err)
	}
	if !result.DryRun || len(result.ResumedTasks) != 0 || len(result.Executed) != 0 {
		t.Fatalf("expected dry run not to mutate runtime, got %+v", result)
	}
	graph, err := service.loadTaskGraph("graph-runtime-test")
	if err != nil {
		t.Fatalf("load task graph: %v", err)
	}
	task := runtimeTestTaskByID(t, graph.RegisteredTasks, taskID)
	if task.Status != TaskQueueStatusWaitingApproval {
		t.Fatalf("expected dry run to keep task waiting approval, got %+v", task)
	}
}
