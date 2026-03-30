package runtime

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) reevaluateTaskGraphAsync(ctx context.Context, graphID string, workerID WorkerID, fence FenceValidation) error {
	if err := s.workQueue.ValidateFence(fence); err != nil {
		return err
	}
	activation, err := s.runtime.ReevaluateTaskGraph(ExecutionContext{
		WorkflowID:    graphID,
		CorrelationID: graphID,
		Attempt:       1,
	}, graphID)
	if err != nil {
		return err
	}
	for _, taskID := range activation.ReadyTaskIDs {
		if err := s.workQueue.Enqueue(WorkItem{
			ID:            makeID("work", WorkItemKindExecuteReadyTask, graphID, taskID),
			Kind:          WorkItemKindExecuteReadyTask,
			Status:        WorkItemStatusQueued,
			DedupeKey:     fmt.Sprintf("execute:%s", graphID),
			GraphID:       graphID,
			TaskID:        taskID,
			WorkflowID:    graphID,
			AvailableAt:   s.now(),
			LastUpdatedAt: s.now(),
			Reason:        "ready task discovered by async reevaluation",
		}); err != nil {
			return err
		}
	}
	if err := s.recordAsyncReplayEvent(ReplayEventRecord{
		EventID:           makeID("replay", "reevaluate-task-graph", graphID, s.now()),
		RootCorrelationID: graphID,
		WorkflowID:        graphID,
		GraphID:           graphID,
		ActionType:        "reevaluate_task_graph",
		Summary:           "worker reevaluated task graph",
		OccurredAt:        s.now(),
		DetailsJSON: mustMarshalReplayDetails(AsyncReplayEventDetails{
			WorkerID:            string(workerID),
			WorkItemID:          fence.WorkItemID,
			WorkItemKind:        string(WorkItemKindReevaluateTaskGraph),
			LeaseID:             fence.LeaseID,
			FencingToken:        fence.FencingToken,
			StoreBackendProfile: s.backendProfile,
			GraphID:             graphID,
			ReadyTaskIDs:        append([]string{}, activation.ReadyTaskIDs...),
			SchedulerDecision:   "reevaluate_task_graph",
		}),
	}); err != nil {
		return err
	}
	return s.rebuildTaskGraphProjection(ctx, graphID)
}

func (s *Service) executeReadyTasksAsync(ctx context.Context, graphID string, policy AutoExecutionPolicy, workerID WorkerID, fence FenceValidation) (FollowUpExecutionBatchResult, error) {
	if err := s.workQueue.ValidateFence(fence); err != nil {
		return FollowUpExecutionBatchResult{}, err
	}
	policy.MaxAttempts = 1
	result, err := s.runtime.executeReadyFollowUps(ctx, ExecutionContext{
		WorkflowID:    graphID,
		CorrelationID: graphID,
		Attempt:       1,
	}, graphID, policy, &fence)
	if err != nil {
		return FollowUpExecutionBatchResult{}, err
	}
	if err := s.recordAsyncReplayEvent(ReplayEventRecord{
		EventID:           makeID("replay", "execute-ready-task", graphID, s.now()),
		RootCorrelationID: graphID,
		WorkflowID:        graphID,
		GraphID:           graphID,
		ActionType:        "execute_ready_task",
		Summary:           "worker executed ready tasks",
		OccurredAt:        s.now(),
		DetailsJSON: mustMarshalReplayDetails(AsyncReplayEventDetails{
			WorkerID:            string(workerID),
			WorkItemID:          fence.WorkItemID,
			WorkItemKind:        string(WorkItemKindExecuteReadyTask),
			LeaseID:             fence.LeaseID,
			FencingToken:        fence.FencingToken,
			StoreBackendProfile: s.backendProfile,
			GraphID:             graphID,
			ExecutedTaskIDs:     executionTaskIDs(result.ExecutedTasks),
		}),
	}); err != nil {
		return FollowUpExecutionBatchResult{}, err
	}
	if err := s.workQueue.ValidateFence(fence); err != nil {
		return FollowUpExecutionBatchResult{}, err
	}
	if err := s.rebuildTaskGraphProjection(ctx, graphID); err != nil {
		return FollowUpExecutionBatchResult{}, err
	}
	return result, nil
}

func (s *Service) handleSchedulerWakeup(ctx context.Context, item WorkItem) error {
	_ = ctx
	switch item.WakeupKind {
	case SchedulerWakeupDueWindow, SchedulerWakeupDependency, SchedulerWakeupCapability, SchedulerWakeupOperator:
		return s.workQueue.Enqueue(WorkItem{
			ID:            makeID("work", WorkItemKindReevaluateTaskGraph, item.GraphID, item.WakeupKind),
			Kind:          WorkItemKindReevaluateTaskGraph,
			Status:        WorkItemStatusQueued,
			DedupeKey:     fmt.Sprintf("reevaluate:%s", item.GraphID),
			GraphID:       item.GraphID,
			WorkflowID:    item.WorkflowID,
			AvailableAt:   s.now(),
			LastUpdatedAt: s.now(),
			Reason:        item.Reason,
			WakeupKind:    item.WakeupKind,
		})
	case SchedulerWakeupApproval:
		return s.workQueue.Enqueue(WorkItem{
			ID:            makeID("work", WorkItemKindResumeApprovedCheckpoint, item.ApprovalID),
			Kind:          WorkItemKindResumeApprovedCheckpoint,
			Status:        WorkItemStatusQueued,
			DedupeKey:     fmt.Sprintf("resume:%s:%s", item.GraphID, item.TaskID),
			GraphID:       item.GraphID,
			TaskID:        item.TaskID,
			ApprovalID:    item.ApprovalID,
			AvailableAt:   s.now(),
			LastUpdatedAt: s.now(),
			Reason:        item.Reason,
			WakeupKind:    item.WakeupKind,
		})
	case SchedulerWakeupRetry:
		return s.workQueue.Enqueue(WorkItem{
			ID:            makeID("work", WorkItemKindRetryFailedExecution, item.ExecutionID),
			Kind:          WorkItemKindRetryFailedExecution,
			Status:        WorkItemStatusQueued,
			DedupeKey:     fmt.Sprintf("retry:%s", item.ExecutionID),
			GraphID:       item.GraphID,
			TaskID:        item.TaskID,
			ExecutionID:   item.ExecutionID,
			AvailableAt:   s.now(),
			LastUpdatedAt: s.now(),
			Reason:        item.Reason,
			WakeupKind:    item.WakeupKind,
		})
	default:
		return fmt.Errorf("unsupported scheduler wakeup kind %q", item.WakeupKind)
	}
}

func (s *Service) resumeApprovedTaskAsync(ctx context.Context, graphID string, taskID string, approvalID string, workerID WorkerID, fence FenceValidation) (TaskCommandResult, error) {
	if err := s.workQueue.ValidateFence(fence); err != nil {
		return TaskCommandResult{}, err
	}
	snapshot, task, err := s.locateTask(graphID, taskID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if task.Status != TaskQueueStatusWaitingApproval {
		return TaskCommandResult{}, &InvalidTransitionError{Resource: "follow_up_task", ID: task.Task.ID, From: string(task.Status), To: string(TaskQueueStatusExecuting), Reason: "task is not waiting approval"}
	}
	approval, ok, err := s.runtime.Approvals.LoadByTask(snapshot.Graph.GraphID, task.Task.ID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if !ok || approval.Status != ApprovalStatusApproved {
		return TaskCommandResult{}, &InvalidTransitionError{Resource: "approval", ID: approvalID, From: string(approval.Status), To: string(ApprovalStatusApproved), Reason: "task cannot resume before approval resolution"}
	}
	latest, ok, err := s.runtime.Executions.LoadLatestByTask(snapshot.Graph.GraphID, task.Task.ID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if !ok {
		return TaskCommandResult{}, &NotFoundError{Resource: "task_execution", ID: task.Task.ID}
	}
	checkpoint, err := s.runtime.CheckpointStore.Load(latest.WorkflowID, latest.CheckpointID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	token, err := s.runtime.CheckpointStore.LoadResumeToken(latest.ResumeToken)
	if err != nil {
		return TaskCommandResult{}, err
	}
	payload, err := s.runtime.CheckpointStore.LoadPayload(latest.CheckpointID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if payload.FollowUpFinalizeResume == nil {
		return TaskCommandResult{}, &ConflictError{Resource: "checkpoint_payload", ID: latest.CheckpointID, Reason: "follow-up finalize resume payload is missing"}
	}
	snapshotRef := firstNonEmpty(latest.UpdatedStateSnapshotRef, payload.FollowUpFinalizeResume.PendingStateSnapshotRef)
	pendingState, err := s.runtime.loadStateSnapshot(snapshotRef)
	if err != nil {
		return TaskCommandResult{}, err
	}
	workflowCapability, ok, reason := s.runtime.Capabilities.ResolveWorkflow(task.Task)
	if !ok || workflowCapability == nil {
		return TaskCommandResult{}, &ConflictError{Resource: "follow_up_task", ID: task.Task.ID, Reason: firstNonEmpty(reason, "workflow capability is not available")}
	}
	now := s.now()
	snapshot = updateTaskStatus(snapshot, task.Task.ID, TaskQueueStatusExecuting, nil, nil, now)
	updatedSnapshot, err := s.runtime.saveUpdatedTaskGraphSnapshotGuarded(snapshot, snapshot.Version, &fence)
	if err != nil {
		return TaskCommandResult{}, err
	}
	snapshot = updatedSnapshot
	expectedExecutionVersion := latest.Version
	latest.Status = TaskQueueStatusExecuting
	latest.PendingApproval = false
	latest.LastTransitionAt = now
	if err := s.workQueue.ValidateFence(fence); err != nil {
		return TaskCommandResult{}, err
	}
	if err := s.runtime.Executions.Update(latest, expectedExecutionVersion); err != nil {
		return TaskCommandResult{}, err
	}
	resumed, err := workflowCapability.Resume(ctx, task.Task, buildActivationContext(snapshot.Graph, task), pendingState.State, checkpoint, token, payload)
	if err != nil {
		return TaskCommandResult{}, err
	}
	finalized := finalizeTaskExecutionRecord(latest, resumed, s.now())
	if err := s.runtime.persistExecutionOutcomeGuarded(snapshot, &finalized, resumed, &fence); err != nil {
		return TaskCommandResult{}, err
	}
	snapshot = applyExecutionRecord(snapshot, finalized, resumed)
	if updatedSnapshot, err = s.runtime.saveUpdatedTaskGraphSnapshotGuarded(snapshot, snapshot.Version, &fence); err != nil {
		return TaskCommandResult{}, err
	}
	snapshot = updatedSnapshot
	if err := s.recordAsyncReplayEvent(ReplayEventRecord{
		EventID:           makeID("replay", "resume-approved-checkpoint", finalized.ExecutionID, s.now()),
		RootCorrelationID: finalized.RootCorrelationID,
		ParentWorkflowID:  finalized.ParentWorkflowID,
		WorkflowID:        finalized.WorkflowID,
		GraphID:           snapshot.Graph.GraphID,
		TaskID:            task.Task.ID,
		ApprovalID:        latest.ApprovalID,
		ExecutionID:       finalized.ExecutionID,
		ActionType:        "resume_approved_checkpoint",
		Summary:           "worker resumed approved checkpoint",
		OccurredAt:        s.now(),
		DetailsJSON: mustMarshalReplayDetails(AsyncReplayEventDetails{
			WorkerID:            string(workerID),
			WorkItemID:          fence.WorkItemID,
			WorkItemKind:        string(WorkItemKindResumeApprovedCheckpoint),
			LeaseID:             fence.LeaseID,
			FencingToken:        fence.FencingToken,
			StoreBackendProfile: s.backendProfile,
			GraphID:             snapshot.Graph.GraphID,
			TaskID:              task.Task.ID,
			ApprovalID:          latest.ApprovalID,
			ExecutionID:         finalized.ExecutionID,
			SchedulerDecision:   string(SchedulerWakeupApproval),
		}),
	}); err != nil {
		return TaskCommandResult{}, err
	}
	if err := s.rebuildTaskGraphProjection(ctx, snapshot.Graph.GraphID); err != nil {
		return TaskCommandResult{}, err
	}
	return TaskCommandResult{
		GraphID:     snapshot.Graph.GraphID,
		TaskID:      task.Task.ID,
		ApprovalID:  latest.ApprovalID,
		ExecutionID: finalized.ExecutionID,
		Status:      finalized.Status,
	}, nil
}

func (s *Service) retryFailedTaskAsync(ctx context.Context, graphID string, taskID string, executionID string, workerID WorkerID, fence FenceValidation) (TaskCommandResult, error) {
	if err := s.workQueue.ValidateFence(fence); err != nil {
		return TaskCommandResult{}, err
	}
	snapshot, task, err := s.locateTask(graphID, taskID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if task.Status != TaskQueueStatusFailed {
		return TaskCommandResult{}, &InvalidTransitionError{Resource: "follow_up_task", ID: task.Task.ID, From: string(task.Status), To: string(TaskQueueStatusExecuting), Reason: "task is not failed"}
	}
	workflowCapability, ok, reason := s.runtime.Capabilities.ResolveWorkflow(task.Task)
	if !ok || workflowCapability == nil {
		return TaskCommandResult{}, &ConflictError{Resource: "follow_up_task", ID: task.Task.ID, Reason: firstNonEmpty(reason, "workflow capability is not available")}
	}
	now := s.now()
	snapshot = updateTaskStatus(snapshot, task.Task.ID, TaskQueueStatusExecuting, nil, nil, now)
	updatedSnapshot, err := s.runtime.saveUpdatedTaskGraphSnapshotGuarded(snapshot, snapshot.Version, &fence)
	if err != nil {
		return TaskCommandResult{}, err
	}
	snapshot = updatedSnapshot
	inputState := snapshot.LatestCommittedStateSnapshot.State
	if inputState.Version.Sequence == 0 {
		inputState = snapshot.BaseStateSnapshot.State
	}
	record, childResult, err := s.runtime.executeSingleFollowUp(ctx, ExecutionContext{
		WorkflowID:    snapshot.Graph.ParentWorkflowID,
		TaskID:        task.Task.ID,
		CorrelationID: snapshot.Graph.ParentWorkflowID,
		Attempt:       1,
	}, snapshot, task, workflowCapability, inputState, 1)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if executionID != "" {
		record.CausationID = executionID
	}
	if err := s.runtime.persistExecutionOutcomeGuarded(snapshot, &record, childResult, &fence); err != nil {
		return TaskCommandResult{}, err
	}
	snapshot = applyExecutionRecord(snapshot, record, childResult)
	if updatedSnapshot, err = s.runtime.saveUpdatedTaskGraphSnapshotGuarded(snapshot, snapshot.Version, &fence); err != nil {
		return TaskCommandResult{}, err
	}
	snapshot = updatedSnapshot
	if err := s.recordAsyncReplayEvent(ReplayEventRecord{
		EventID:           makeID("replay", "retry-failed-execution", record.ExecutionID, s.now()),
		RootCorrelationID: record.RootCorrelationID,
		ParentWorkflowID:  record.ParentWorkflowID,
		WorkflowID:        record.WorkflowID,
		GraphID:           snapshot.Graph.GraphID,
		TaskID:            task.Task.ID,
		ExecutionID:       record.ExecutionID,
		ActionType:        "retry_failed_execution",
		Summary:           "worker retried failed execution",
		OccurredAt:        s.now(),
		DetailsJSON: mustMarshalReplayDetails(AsyncReplayEventDetails{
			WorkerID:             string(workerID),
			WorkItemID:           fence.WorkItemID,
			WorkItemKind:         string(WorkItemKindRetryFailedExecution),
			LeaseID:              fence.LeaseID,
			FencingToken:         fence.FencingToken,
			StoreBackendProfile:  s.backendProfile,
			GraphID:              snapshot.Graph.GraphID,
			TaskID:               task.Task.ID,
			ExecutionID:          record.ExecutionID,
			RetryOfExecutionID:   executionID,
			RetryBackoffDecision: "retry_failed_execution",
		}),
	}); err != nil {
		return TaskCommandResult{}, err
	}
	if err := s.rebuildTaskGraphProjection(ctx, snapshot.Graph.GraphID); err != nil {
		return TaskCommandResult{}, err
	}
	return TaskCommandResult{
		GraphID:     snapshot.Graph.GraphID,
		TaskID:      task.Task.ID,
		ExecutionID: record.ExecutionID,
		Status:      record.Status,
	}, nil
}

func (s *Service) handleLeaseReclaim(ctx context.Context, reclaim LeaseReclaimResult, policy AutoExecutionPolicy) error {
	item, ok, err := s.workQueue.Load(reclaim.WorkItemID)
	if err != nil || !ok {
		return err
	}
	switch item.Kind {
	case WorkItemKindExecuteReadyTask, WorkItemKindRetryFailedExecution:
		if item.ExecutionID != "" {
			record, ok, err := s.runtime.Executions.Load(item.ExecutionID)
			if err != nil {
				return err
			}
			if ok && record.Status == TaskQueueStatusExecuting {
				record.Status = TaskQueueStatusFailed
				record.FailureCategory = FailureCategoryTimeout
				record.FailureSummary = "lease reclaimed after worker heartbeat expired"
				record.FailedAt = s.now()
				record.LastTransitionAt = s.now()
				if err := s.runtime.Executions.Update(record, record.Version); err != nil {
					return err
				}
			}
		}
		if item.GraphID != "" {
			wakeup := SchedulerWakeup{
				ID:          makeID("wakeup", SchedulerWakeupRetry, item.GraphID, item.TaskID, s.now().UTC()),
				GraphID:     item.GraphID,
				TaskID:      item.TaskID,
				ExecutionID: item.ExecutionID,
				Kind:        SchedulerWakeupRetry,
				AvailableAt: s.now().Add(nextRetryTime(TaskExecutionRecord{RetryCount: 1, FailedAt: s.now()}, RetryBackoffPolicy{BaseDelay: 2 * time.Second, MaxDelay: 1 * time.Minute}, s.now()).Sub(s.now())),
				Reason:      reclaim.Reason,
			}
			if s.scheduler != nil {
				if err := s.scheduler.SaveWakeup(wakeup); err != nil {
					return err
				}
			}
		}
	case WorkItemKindResumeApprovedCheckpoint, WorkItemKindReevaluateTaskGraph, WorkItemKindSchedulerWakeup:
		// these are safe to simply be re-queued by reclaim semantics
	}
	_ = ctx
	_ = policy
	return nil
}

func (s *Service) recordAsyncReplayEvent(event ReplayEventRecord) error {
	if s.runtime == nil || s.runtime.Replay == nil {
		return nil
	}
	return s.runtime.Replay.Append(event)
}

func executionTaskIDs(records []TaskExecutionRecord) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		if record.TaskID != "" {
			result = append(result, record.TaskID)
		}
	}
	return result
}

func mustMarshalReplayDetails(value any) string {
	raw, err := marshalJSON(value)
	if err != nil {
		return fmt.Sprintf(`{"marshal_error":%q}`, err.Error())
	}
	return raw
}
