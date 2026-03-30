package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type asyncRuntimeHarness struct {
	service      *Service
	operator     *OperatorService
	clock        *ManualClock
	workflowRuns *InMemoryWorkflowRunStore
	projections  *InMemoryReplayProjectionStore
	replayQuery  *ReplayQueryService
	graphID      string
	taskID       string
}

func TestWorkQueueExclusiveClaimAndFenceRejectsStaleWorkerCommit(t *testing.T) {
	clock := NewManualClock(time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC))
	queue := NewInMemoryWorkQueueStore()
	item := WorkItem{
		ID:            "work-1",
		Kind:          WorkItemKindExecuteReadyTask,
		Status:        WorkItemStatusQueued,
		GraphID:       "graph-1",
		TaskID:        "task-1",
		WorkflowID:    "workflow-1",
		AvailableAt:   clock.Now(),
		LastUpdatedAt: clock.Now(),
		Reason:        "unit test",
	}
	if _, err := queue.Enqueue(item); err != nil {
		t.Fatalf("enqueue work item: %v", err)
	}
	claimsA, err := queue.ClaimReady(WorkerID("worker-a"), 1, clock.Now(), 5*time.Second)
	if err != nil {
		t.Fatalf("claim by worker-a: %v", err)
	}
	if len(claimsA) != 1 {
		t.Fatalf("expected one claim for worker-a, got %+v", claimsA)
	}
	claimsB, err := queue.ClaimReady(WorkerID("worker-b"), 1, clock.Now(), 5*time.Second)
	if err != nil {
		t.Fatalf("claim by worker-b while leased: %v", err)
	}
	if len(claimsB) != 0 {
		t.Fatalf("expected exclusive lease, got %+v", claimsB)
	}
	clock.Advance(6 * time.Second)
	reclaimed, err := queue.ReclaimExpired(clock.Now())
	if err != nil {
		t.Fatalf("reclaim expired lease: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0].WorkItemID != item.ID {
		t.Fatalf("expected reclaimed lease result, got %+v", reclaimed)
	}
	claimsB, err = queue.ClaimReady(WorkerID("worker-b"), 1, clock.Now(), 5*time.Second)
	if err != nil {
		t.Fatalf("claim by worker-b after reclaim: %v", err)
	}
	if len(claimsB) != 1 {
		t.Fatalf("expected one claim for worker-b, got %+v", claimsB)
	}
	err = queue.Complete(FenceValidation{
		WorkItemID:   claimsA[0].WorkItem.ID,
		LeaseID:      claimsA[0].LeaseID,
		FencingToken: claimsA[0].FencingToken,
		WorkerID:     claimsA[0].WorkerID,
	}, clock.Now())
	if err == nil || !IsConflict(err) {
		t.Fatalf("expected stale worker completion to fail with conflict, got %v", err)
	}
	if err := queue.Complete(FenceValidation{
		WorkItemID:   claimsB[0].WorkItem.ID,
		LeaseID:      claimsB[0].LeaseID,
		FencingToken: claimsB[0].FencingToken,
		WorkerID:     claimsB[0].WorkerID,
	}, clock.Now()); err != nil {
		t.Fatalf("complete with current fence: %v", err)
	}
}

func TestSchedulerDeferredWakeupEnqueuesReevaluation(t *testing.T) {
	now := time.Date(2026, 3, 30, 9, 30, 0, 0, time.UTC)
	harness := newAsyncHarness(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
		},
	})
	snapshot, err := harness.service.loadTaskGraph(harness.graphID)
	if err != nil {
		t.Fatalf("load graph: %v", err)
	}
	deferredAt := harness.clock.Now().Add(10 * time.Minute)
	snapshot.RegisteredTasks[0].Status = TaskQueueStatusDeferred
	snapshot.RegisteredTasks[0].Metadata.DueWindow.NotBefore = &deferredAt
	if _, err := harness.service.runtime.saveUpdatedTaskGraphSnapshot(snapshot, snapshot.Version); err != nil {
		t.Fatalf("persist deferred graph: %v", err)
	}
	scheduler := harness.service.schedulerService(DefaultAutoExecutionPolicy())
	firstTick, err := scheduler.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick scheduler before due: %v", err)
	}
	if len(firstTick.SavedWakeups) == 0 || len(firstTick.DispatchedWakeups) != 0 {
		t.Fatalf("expected saved-but-not-dispatched due wakeup, got %+v", firstTick)
	}
	harness.clock.Set(deferredAt)
	secondTick, err := scheduler.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick scheduler at due time: %v", err)
	}
	if len(secondTick.EnqueuedWorkKinds) == 0 || secondTick.EnqueuedWorkKinds[0] != WorkItemKindReevaluateTaskGraph {
		t.Fatalf("expected reevaluate work item after due wakeup, got %+v", secondTick)
	}
	view, err := harness.replayQuery.Query(context.Background(), observability.ReplayQuery{TaskGraphID: harness.graphID})
	if err != nil {
		t.Fatalf("query replay view after due wakeup: %v", err)
	}
	if !containsString(view.Summary.AsyncRuntimeSummary, "scheduler=") {
		t.Fatalf("expected async runtime summary to include scheduler dispatch, got %+v", view.Summary)
	}
}

func TestApproveTaskAsyncEnqueuesResumeAndDifferentWorkerCompletes(t *testing.T) {
	now := time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC)
	harness := newAsyncWaitingApprovalHarness(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			return waitingApprovalWithPayloadResult("workflow-child-"+spec.ID, spec.ID, current), nil
		},
		resume: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState, _ CheckpointRecord, _ ResumeToken, _ CheckpointPayloadEnvelope) (FollowUpWorkflowRunResult, error) {
			return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
		},
	})
	approval, ok, err := harness.service.runtime.Approvals.LoadByTask(harness.graphID, harness.taskID)
	if err != nil || !ok {
		t.Fatalf("load approval: %v %+v", err, approval)
	}
	result, err := harness.operator.ApproveTask(context.Background(), ApproveTaskCommand{
		RequestID:  "approve-async-1",
		ApprovalID: approval.ApprovalID,
		Actor:      "operator",
		Roles:      []string{"operator"},
		Note:       "async approve",
	})
	if err != nil {
		t.Fatalf("approve task async: %v", err)
	}
	if !result.AsyncDispatchAccepted || len(result.EnqueuedWorkKinds) != 1 || result.EnqueuedWorkKinds[0] != WorkItemKindResumeApprovedCheckpoint {
		t.Fatalf("expected async resume dispatch, got %+v", result)
	}
	workerResult, err := harness.service.RunAsyncWorkerOnce(context.Background(), DefaultAutoExecutionPolicy(), WorkerRunOptions{
		WorkerID:   WorkerID("worker-b"),
		Role:       WorkerRoleWorker,
		LeaseTTL:   30 * time.Second,
		ClaimBatch: 2,
	}, false)
	if err != nil {
		t.Fatalf("run worker-b async pass: %v", err)
	}
	if len(workerResult.ResumedTasks) != 1 || workerResult.ResumedTasks[0] != harness.taskID {
		t.Fatalf("expected different worker to resume approved task, got %+v", workerResult)
	}
	latest, ok, err := harness.service.runtime.Executions.LoadLatestByTask(harness.graphID, harness.taskID)
	if err != nil || !ok {
		t.Fatalf("load latest execution: %v %+v", err, latest)
	}
	if latest.Status != TaskQueueStatusCompleted {
		t.Fatalf("expected completed execution after async resume, got %+v", latest)
	}
	view, err := harness.replayQuery.Query(context.Background(), observability.ReplayQuery{TaskGraphID: harness.graphID})
	if err != nil {
		t.Fatalf("query replay view: %v", err)
	}
	if len(view.Summary.AsyncRuntimeSummary) == 0 {
		t.Fatalf("expected async runtime summary in replay view, got %+v", view)
	}
	if !containsString(view.Explanation.WhyAsyncRuntime, "worker-b") {
		t.Fatalf("expected replay explanation to mention worker-b, got %+v", view.Explanation)
	}
}

func TestTransientFailureRetryBackoffLaterWorkerCompletes(t *testing.T) {
	now := time.Date(2026, 3, 30, 10, 30, 0, 0, time.UTC)
	attempts := 0
	harness := newAsyncHarness(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			attempts++
			if attempts == 1 {
				return FollowUpWorkflowRunResult{}, &FollowUpExecutionError{
					Category: FailureCategoryTransient,
					Summary:  "temporary upstream failure",
					Err:      errors.New("temporary upstream failure"),
				}
			}
			return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
		},
	})
	if _, err := harness.service.workQueue.Enqueue(WorkItem{
		ID:            "work-initial-execute",
		Kind:          WorkItemKindExecuteReadyTask,
		Status:        WorkItemStatusQueued,
		DedupeKey:     "execute:" + harness.graphID,
		GraphID:       harness.graphID,
		TaskID:        harness.taskID,
		WorkflowID:    harness.graphID,
		AvailableAt:   harness.clock.Now(),
		LastUpdatedAt: harness.clock.Now(),
		Reason:        "initial async execution",
	}); err != nil {
		t.Fatalf("enqueue initial execute work: %v", err)
	}
	firstWorker, err := harness.service.RunAsyncWorkerOnce(context.Background(), DefaultAutoExecutionPolicy(), WorkerRunOptions{
		WorkerID:   WorkerID("worker-a"),
		Role:       WorkerRoleWorker,
		LeaseTTL:   30 * time.Second,
		ClaimBatch: 1,
	}, false)
	if err != nil && !strings.Contains(err.Error(), "temporary upstream failure") {
		t.Fatalf("first worker pass: %v", err)
	}
	if len(firstWorker.Executed) != 1 || firstWorker.Executed[0].Status != TaskQueueStatusFailed {
		t.Fatalf("expected failed execution record on first attempt, got %+v", firstWorker)
	}
	harness.clock.Advance(6 * time.Second)
	tick, err := harness.service.schedulerService(DefaultAutoExecutionPolicy()).Tick(context.Background())
	if err != nil {
		t.Fatalf("scheduler tick for retry: %v", err)
	}
	if len(tick.EnqueuedWorkKinds) == 0 || tick.EnqueuedWorkKinds[len(tick.EnqueuedWorkKinds)-1] != WorkItemKindRetryFailedExecution {
		t.Fatalf("expected retry work item after backoff, got %+v", tick)
	}
	secondWorker, err := harness.service.RunAsyncWorkerOnce(context.Background(), DefaultAutoExecutionPolicy(), WorkerRunOptions{
		WorkerID:   WorkerID("worker-b"),
		Role:       WorkerRoleWorker,
		LeaseTTL:   30 * time.Second,
		ClaimBatch: 2,
	}, false)
	if err != nil {
		t.Fatalf("second worker pass: %v", err)
	}
	if len(secondWorker.CompletedWorkItemIDs) == 0 {
		t.Fatalf("expected later worker to complete retry, got %+v", secondWorker)
	}
	latest, ok, err := harness.service.runtime.Executions.LoadLatestByTask(harness.graphID, harness.taskID)
	if err != nil || !ok {
		t.Fatalf("load latest execution after retry: %v %+v", err, latest)
	}
	if latest.Status != TaskQueueStatusCompleted {
		t.Fatalf("expected completed status after retry, got %+v", latest)
	}
}

func newAsyncHarness(t *testing.T, now time.Time, capability runtimeTestCapability) *asyncRuntimeHarness {
	t.Helper()
	clock := NewManualClock(now)
	service := NewService(ServiceOptions{
		CheckpointStore: NewInMemoryCheckpointStore(),
		TaskGraphs:      NewInMemoryTaskGraphStore(),
		Executions:      NewInMemoryTaskExecutionStore(),
		Approvals:       NewInMemoryApprovalStateStore(),
		OperatorActions: NewInMemoryOperatorActionStore(),
		Replay:          NewInMemoryReplayStore(),
		Artifacts:       NewInMemoryArtifactMetadataStore(),
		WorkQueue:       NewInMemoryWorkQueueStore(),
		WorkAttempts:    NewInMemoryWorkAttemptStore(),
		Workers:         NewInMemoryWorkerRegistryStore(),
		Scheduler:       NewInMemorySchedulerStore(),
		Controller:      DefaultWorkflowController{},
		Clock:           clock,
		BackendProfile:  "test-async",
	})
	service.SetCapabilities(StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
		},
		Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: capability,
		},
	})
	workflowRuns := NewInMemoryWorkflowRunStore()
	projections := NewInMemoryReplayProjectionStore()
	rebuilder := NewReplayProjectionRebuilder(service, workflowRuns, projections, service.runtime.Artifacts, service.runtime.Replay, clock.Now)
	service.SetReplayProjectionWriter(rebuilder)
	graphID := "graph-runtime-test"
	taskID := "task-tax-async"
	base := runtimeTestState(now, 1)
	graph := runtimeTestGraph(now, runtimeGeneratedTask(now, taskID, taskspec.UserIntentTaxOptimization, 1))
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := service.runtime.RegisterFollowUpTasks(execCtx, graph, base); err != nil {
		t.Fatalf("register async follow-up tasks: %v", err)
	}
	return &asyncRuntimeHarness{
		service:      service,
		operator:     NewOperatorService(service),
		clock:        clock,
		workflowRuns: workflowRuns,
		projections:  projections,
		replayQuery:  NewReplayQueryService(service, workflowRuns, projections, service.runtime.Artifacts, service.runtime.Replay),
		graphID:      graphID,
		taskID:       taskID,
	}
}

func newAsyncWaitingApprovalHarness(t *testing.T, now time.Time, capability runtimeTestCapability) *asyncRuntimeHarness {
	t.Helper()
	harness := newAsyncHarness(t, now, capability)
	execCtx := ExecutionContext{WorkflowID: "workflow-life-event-rt", TaskID: "task-life-event-rt", CorrelationID: "workflow-life-event-rt", Attempt: 1}
	if _, err := harness.service.runtime.ExecuteReadyFollowUps(context.Background(), execCtx, harness.graphID, DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("seed waiting approval execution: %v", err)
	}
	return harness
}

func containsString(values []string, fragment string) bool {
	for _, item := range values {
		if strings.Contains(item, fragment) {
			return true
		}
	}
	return false
}
