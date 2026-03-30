package runtime

import (
	"context"
	"fmt"
	"time"
)

type OperatorService struct {
	service *Service
}

func NewOperatorService(service *Service) *OperatorService {
	return &OperatorService{service: service}
}

func (s *OperatorService) ReevaluateDeferredOrQueuedTasks(ctx context.Context, cmd ReevaluateTaskGraphCommand) (TaskActivationResult, TaskCommandResult, error) {
	return s.service.ReevaluateTaskGraph(ctx, cmd)
}

func (s *OperatorService) ApproveTask(ctx context.Context, cmd ApproveTaskCommand) (TaskCommandResult, error) {
	approval, ok, err := s.service.runtime.Approvals.Load(cmd.ApprovalID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if !ok {
		return TaskCommandResult{}, &NotFoundError{Resource: "approval", ID: cmd.ApprovalID}
	}
	action, result, duplicate, err := s.service.beginAction(OperatorActionApprove, cmd.RequestID, cmd.Actor, cmd.Roles, approval.GraphID, approval.TaskID, approval.ApprovalID, approval.WorkflowID, cmd.Note, cmd.ExpectedVersion)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if duplicate {
		return result, nil
	}
	if approval.Status != ApprovalStatusPending {
		return s.service.failAction(action, result, &InvalidTransitionError{Resource: "approval", ID: approval.ApprovalID, From: string(approval.Status), To: string(ApprovalStatusApproved), Reason: "approval is not pending"})
	}
	expectedVersion := approval.Version
	if cmd.ExpectedVersion > 0 {
		expectedVersion = cmd.ExpectedVersion
	}
	now := s.service.now()
	approval.Status = ApprovalStatusApproved
	approval.ResolvedAt = &now
	approval.ResolvedBy = cmd.Actor
	approval.ResolutionNote = cmd.Note
	if err := s.service.runtime.Approvals.Update(approval, expectedVersion); err != nil {
		return TaskCommandResult{}, err
	}
	action.Status = OperatorActionStatusApplied
	action.AppliedAt = &now
	if err := s.service.runtime.OperatorActions.Save(action); err != nil {
		return TaskCommandResult{}, err
	}
	if err := s.service.appendReplay(ReplayEventRecord{
		EventID:           makeID("replay", "approve", approval.ApprovalID, now),
		RootCorrelationID: approval.GraphID,
		ParentWorkflowID:  approval.WorkflowID,
		WorkflowID:        approval.WorkflowID,
		GraphID:           approval.GraphID,
		TaskID:            approval.TaskID,
		ApprovalID:        approval.ApprovalID,
		ActionType:        string(OperatorActionApprove),
		Summary:           fmt.Sprintf("operator approved task %s", approval.TaskID),
		OccurredAt:        now,
		OperatorActionID:  action.ActionID,
	}); err != nil {
		return TaskCommandResult{}, err
	}
	if s.service.asyncDispatchEnabled() {
		item := WorkItem{
			ID:            makeID("work", WorkItemKindResumeApprovedCheckpoint, approval.ApprovalID),
			Kind:          WorkItemKindResumeApprovedCheckpoint,
			Status:        WorkItemStatusQueued,
			DedupeKey:     fmt.Sprintf("resume:%s:%s", approval.GraphID, approval.TaskID),
			GraphID:       approval.GraphID,
			TaskID:        approval.TaskID,
			ApprovalID:    approval.ApprovalID,
			WorkflowID:    approval.WorkflowID,
			AvailableAt:   now,
			LastUpdatedAt: now,
			Reason:        "approval resolved to approved",
			WakeupKind:    SchedulerWakeupApproval,
		}
		enqueued, err := s.service.enqueueWorkItem(item)
		if err != nil {
			return TaskCommandResult{}, err
		}
		if err := s.service.appendReplay(ReplayEventRecord{
			EventID:           makeID("replay", "approve-enqueue", approval.ApprovalID, now),
			RootCorrelationID: approval.GraphID,
			ParentWorkflowID:  approval.WorkflowID,
			WorkflowID:        approval.WorkflowID,
			GraphID:           approval.GraphID,
			TaskID:            approval.TaskID,
			ApprovalID:        approval.ApprovalID,
			ActionType:        "async_dispatch",
			Summary:           fmt.Sprintf("approval %s enqueued async resume", approval.ApprovalID),
			OccurredAt:        now,
			OperatorActionID:  action.ActionID,
			DetailsJSON:       mustMarshalReplayDetails(operatorAsyncDispatchReplayDetails{WorkerAction: string(OperatorActionApprove), WorkItemID: item.ID, WorkItemKind: string(item.Kind), SchedulerDecision: string(SchedulerWakeupApproval), StoreBackendProfile: s.service.backendProfile}),
		}); err != nil {
			return TaskCommandResult{}, err
		}
		if err := s.service.rebuildTaskGraphProjection(ctx, approval.GraphID); err != nil {
			return TaskCommandResult{}, err
		}
		enqueued.Action = action
		enqueued.AutoResumeTried = false
		return enqueued, nil
	}
	if err := s.service.rebuildTaskGraphProjection(ctx, approval.GraphID); err != nil {
		return TaskCommandResult{}, err
	}
	result = TaskCommandResult{
		Action:          action,
		GraphID:         approval.GraphID,
		TaskID:          approval.TaskID,
		ApprovalID:      approval.ApprovalID,
		AutoResumeTried: true,
	}
	resumeResult, err := s.resumeTaskInternal(ctx, ResumeFollowUpTaskCommand{
		RequestID: approval.ApprovalID + ":approve-resume:" + cmd.RequestID,
		GraphID:   approval.GraphID,
		TaskID:    approval.TaskID,
		Actor:     cmd.Actor,
		Roles:     cmd.Roles,
		Note:      "auto resume after approval",
	}, false)
	if err == nil {
		result.AutoResumeApplied = true
		result.ExecutionID = resumeResult.ExecutionID
		result.Status = resumeResult.Status
	} else {
		result.FailureSummary = err.Error()
	}
	return result, nil
}

func (s *OperatorService) DenyTask(ctx context.Context, cmd DenyTaskCommand) (TaskCommandResult, error) {
	approval, ok, err := s.service.runtime.Approvals.Load(cmd.ApprovalID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if !ok {
		return TaskCommandResult{}, &NotFoundError{Resource: "approval", ID: cmd.ApprovalID}
	}
	action, result, duplicate, err := s.service.beginAction(OperatorActionDeny, cmd.RequestID, cmd.Actor, cmd.Roles, approval.GraphID, approval.TaskID, approval.ApprovalID, approval.WorkflowID, cmd.Note, cmd.ExpectedVersion)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if duplicate {
		return result, nil
	}
	if approval.Status != ApprovalStatusPending {
		return s.service.failAction(action, result, &InvalidTransitionError{Resource: "approval", ID: approval.ApprovalID, From: string(approval.Status), To: string(ApprovalStatusDenied), Reason: "approval is not pending"})
	}
	snapshot, task, err := s.service.locateTask(approval.GraphID, approval.TaskID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	expectedApprovalVersion := approval.Version
	if cmd.ExpectedVersion > 0 {
		expectedApprovalVersion = cmd.ExpectedVersion
	}
	now := s.service.now()
	approval.Status = ApprovalStatusDenied
	approval.ResolvedAt = &now
	approval.ResolvedBy = cmd.Actor
	approval.ResolutionNote = cmd.Note
	if err := s.service.runtime.Approvals.Update(approval, expectedApprovalVersion); err != nil {
		return TaskCommandResult{}, err
	}
	if latest, ok, err := s.service.runtime.Executions.LoadLatestByTask(snapshot.Graph.GraphID, task.Task.ID); err != nil {
		return TaskCommandResult{}, err
	} else if ok {
		expectedExecutionVersion := latest.Version
		latest.Status = TaskQueueStatusFailed
		latest.PendingApproval = false
		latest.FailureCategory = FailureCategoryDeniedByOp
		latest.FailureSummary = firstNonEmpty(cmd.Note, "operator denied approval")
		latest.FailedAt = now
		latest.LastTransitionAt = now
		if err := s.service.runtime.Executions.Update(latest, expectedExecutionVersion); err != nil {
			return TaskCommandResult{}, err
		}
	}
	expectedGraphVersion := snapshot.Version
	snapshot = updateTaskStatus(snapshot, task.Task.ID, TaskQueueStatusFailed, []string{firstNonEmpty(cmd.Note, "operator denied approval")}, nil, now)
	if updated, err := s.service.runtime.saveUpdatedTaskGraphSnapshot(snapshot, expectedGraphVersion); err != nil {
		return TaskCommandResult{}, err
	} else {
		snapshot = updated
	}
	action.Status = OperatorActionStatusApplied
	action.AppliedAt = &now
	if err := s.service.runtime.OperatorActions.Save(action); err != nil {
		return TaskCommandResult{}, err
	}
	if err := s.service.appendReplay(ReplayEventRecord{
		EventID:           makeID("replay", "deny", approval.ApprovalID, now),
		RootCorrelationID: approval.GraphID,
		ParentWorkflowID:  approval.WorkflowID,
		WorkflowID:        approval.WorkflowID,
		GraphID:           approval.GraphID,
		TaskID:            approval.TaskID,
		ApprovalID:        approval.ApprovalID,
		ActionType:        string(OperatorActionDeny),
		Summary:           fmt.Sprintf("operator denied task %s", approval.TaskID),
		OccurredAt:        now,
		OperatorActionID:  action.ActionID,
	}); err != nil {
		return TaskCommandResult{}, err
	}
	if err := s.service.rebuildTaskGraphProjection(ctx, approval.GraphID); err != nil {
		return TaskCommandResult{}, err
	}
	return TaskCommandResult{
		Action:         action,
		GraphID:        approval.GraphID,
		TaskID:         approval.TaskID,
		ApprovalID:     approval.ApprovalID,
		Status:         TaskQueueStatusFailed,
		FailureSummary: firstNonEmpty(cmd.Note, "operator denied approval"),
	}, nil
}

func (s *OperatorService) ResumeFollowUpTask(ctx context.Context, cmd ResumeFollowUpTaskCommand) (TaskCommandResult, error) {
	return s.resumeTaskInternal(ctx, cmd, true)
}

func (s *OperatorService) RetryFailedFollowUpTask(ctx context.Context, cmd RetryFailedFollowUpTaskCommand) (TaskCommandResult, error) {
	snapshot, task, err := s.service.locateTask(cmd.GraphID, cmd.TaskID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	action, result, duplicate, err := s.service.beginAction(OperatorActionRetry, cmd.RequestID, cmd.Actor, cmd.Roles, snapshot.Graph.GraphID, task.Task.ID, "", "", cmd.Note, cmd.ExpectedVersion)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if duplicate {
		return result, nil
	}
	if task.Status != TaskQueueStatusFailed {
		return s.service.failAction(action, result, &InvalidTransitionError{Resource: "follow_up_task", ID: task.Task.ID, From: string(task.Status), To: string(TaskQueueStatusExecuting), Reason: "task is not failed"})
	}
	workflowCapability, ok, reason := s.service.runtime.Capabilities.ResolveWorkflow(task.Task)
	if !ok || workflowCapability == nil {
		return s.service.failAction(action, result, &ConflictError{Resource: "follow_up_task", ID: task.Task.ID, Reason: firstNonEmpty(reason, "workflow capability is not available")})
	}
	if s.service.asyncDispatchEnabled() {
		now := s.service.now()
		action.Status = OperatorActionStatusApplied
		action.AppliedAt = &now
		if err := s.service.runtime.OperatorActions.Save(action); err != nil {
			return TaskCommandResult{}, err
		}
		existing, ok, err := s.service.runtime.Executions.LoadLatestByTask(snapshot.Graph.GraphID, task.Task.ID)
		if err != nil {
			return TaskCommandResult{}, err
		}
		if !ok {
			return TaskCommandResult{}, &NotFoundError{Resource: "task_execution", ID: task.Task.ID}
		}
		item := WorkItem{
			ID:            makeID("work", WorkItemKindRetryFailedExecution, snapshot.Graph.GraphID, task.Task.ID, now),
			Kind:          WorkItemKindRetryFailedExecution,
			Status:        WorkItemStatusQueued,
			DedupeKey:     fmt.Sprintf("retry:%s:%s", snapshot.Graph.GraphID, task.Task.ID),
			GraphID:       snapshot.Graph.GraphID,
			TaskID:        task.Task.ID,
			ExecutionID:   existing.ExecutionID,
			WorkflowID:    snapshot.Graph.ParentWorkflowID,
			AvailableAt:   now,
			LastUpdatedAt: now,
			Reason:        firstNonEmpty(cmd.Note, "operator requested retry"),
			WakeupKind:    SchedulerWakeupRetry,
		}
		enqueued, err := s.service.enqueueWorkItem(item)
		if err != nil {
			return TaskCommandResult{}, err
		}
		if err := s.service.appendReplay(ReplayEventRecord{
			EventID:           makeID("replay", "retry-enqueue", task.Task.ID, now),
			RootCorrelationID: snapshot.Graph.ParentWorkflowID,
			ParentWorkflowID:  snapshot.Graph.ParentWorkflowID,
			GraphID:           snapshot.Graph.GraphID,
			TaskID:            task.Task.ID,
			ExecutionID:       existing.ExecutionID,
			ActionType:        "async_dispatch",
			Summary:           fmt.Sprintf("operator enqueued async retry for %s", task.Task.ID),
			OccurredAt:        now,
			OperatorActionID:  action.ActionID,
			DetailsJSON:       mustMarshalReplayDetails(operatorAsyncDispatchReplayDetails{WorkerAction: string(OperatorActionRetry), WorkItemID: item.ID, WorkItemKind: string(item.Kind), SchedulerDecision: string(SchedulerWakeupRetry), StoreBackendProfile: s.service.backendProfile}),
		}); err != nil {
			return TaskCommandResult{}, err
		}
		if err := s.service.rebuildTaskGraphProjection(ctx, snapshot.Graph.GraphID); err != nil {
			return TaskCommandResult{}, err
		}
		enqueued.Action = action
		return enqueued, nil
	}
	expectedGraphVersion := snapshot.Version
	now := s.service.now()
	snapshot = updateTaskStatus(snapshot, task.Task.ID, TaskQueueStatusExecuting, nil, nil, now)
	if updated, err := s.service.runtime.saveUpdatedTaskGraphSnapshot(snapshot, expectedGraphVersion); err != nil {
		return TaskCommandResult{}, err
	} else {
		snapshot = updated
	}
	inputState := snapshot.LatestCommittedStateSnapshot.State
	if inputState.Version.Sequence == 0 {
		inputState = snapshot.BaseStateSnapshot.State
	}
	record, childResult, err := s.service.runtime.executeSingleFollowUp(ctx, ExecutionContext{
		WorkflowID:    snapshot.Graph.ParentWorkflowID,
		TaskID:        task.Task.ID,
		CorrelationID: snapshot.Graph.ParentWorkflowID,
		Attempt:       1,
	}, snapshot, task, workflowCapability, inputState, DefaultAutoExecutionPolicy().MaxAttempts)
	if err != nil {
		return s.service.failAction(action, result, err)
	}
	if err := s.service.runtime.persistExecutionOutcome(snapshot, &record, childResult); err != nil {
		return TaskCommandResult{}, err
	}
	expectedGraphVersion = snapshot.Version
	snapshot = applyExecutionRecord(snapshot, record, childResult)
	if updated, err := s.service.runtime.saveUpdatedTaskGraphSnapshot(snapshot, expectedGraphVersion); err != nil {
		return TaskCommandResult{}, err
	} else {
		snapshot = updated
	}
	action.Status = OperatorActionStatusApplied
	action.AppliedAt = &now
	action.WorkflowID = record.WorkflowID
	if err := s.service.runtime.OperatorActions.Save(action); err != nil {
		return TaskCommandResult{}, err
	}
	if err := s.service.rebuildTaskGraphProjection(ctx, snapshot.Graph.GraphID); err != nil {
		return TaskCommandResult{}, err
	}
	return TaskCommandResult{
		Action:      action,
		GraphID:     snapshot.Graph.GraphID,
		TaskID:      task.Task.ID,
		ExecutionID: record.ExecutionID,
		Status:      record.Status,
	}, nil
}

func (s *OperatorService) resumeTaskInternal(ctx context.Context, cmd ResumeFollowUpTaskCommand, explicit bool) (TaskCommandResult, error) {
	snapshot, task, err := s.service.locateTask(cmd.GraphID, cmd.TaskID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	actionType := OperatorActionResume
	action, result, duplicate, err := s.service.beginAction(actionType, cmd.RequestID, cmd.Actor, cmd.Roles, snapshot.Graph.GraphID, task.Task.ID, "", "", cmd.Note, cmd.ExpectedVersion)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if duplicate {
		if latest, ok, err := s.service.runtime.Executions.LoadLatestByTask(snapshot.Graph.GraphID, task.Task.ID); err == nil && ok {
			result.GraphID = snapshot.Graph.GraphID
			result.TaskID = task.Task.ID
			result.ApprovalID = latest.ApprovalID
			result.ExecutionID = latest.ExecutionID
			result.Status = latest.Status
		}
		return result, nil
	}
	if task.Status != TaskQueueStatusWaitingApproval {
		return s.service.failAction(action, result, &InvalidTransitionError{Resource: "follow_up_task", ID: task.Task.ID, From: string(task.Status), To: string(TaskQueueStatusExecuting), Reason: "task is not waiting approval"})
	}
	approval, ok, err := s.service.runtime.Approvals.LoadByTask(snapshot.Graph.GraphID, task.Task.ID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if ok && approval.Status != ApprovalStatusApproved {
		return s.service.failAction(action, result, &InvalidTransitionError{Resource: "approval", ID: approval.ApprovalID, From: string(approval.Status), To: string(ApprovalStatusApproved), Reason: "task cannot resume before approval resolution"})
	}
	latest, ok, err := s.service.runtime.Executions.LoadLatestByTask(snapshot.Graph.GraphID, task.Task.ID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if !ok {
		return s.service.failAction(action, result, &NotFoundError{Resource: "task_execution", ID: task.Task.ID})
	}
	checkpoint, err := s.service.runtime.CheckpointStore.Load(latest.WorkflowID, latest.CheckpointID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	token, err := s.service.runtime.CheckpointStore.LoadResumeToken(latest.ResumeToken)
	if err != nil {
		return TaskCommandResult{}, err
	}
	payload, err := s.service.runtime.CheckpointStore.LoadPayload(latest.CheckpointID)
	if err != nil {
		return TaskCommandResult{}, err
	}
	if payload.FollowUpFinalizeResume == nil {
		return s.service.failAction(action, result, &ConflictError{Resource: "checkpoint_payload", ID: latest.CheckpointID, Reason: "follow-up finalize resume payload is missing"})
	}
	snapshotRef := firstNonEmpty(latest.UpdatedStateSnapshotRef, payload.FollowUpFinalizeResume.PendingStateSnapshotRef)
	pendingState, err := s.service.runtime.loadStateSnapshot(snapshotRef)
	if err != nil {
		return TaskCommandResult{}, err
	}
	workflowCapability, ok, reason := s.service.runtime.Capabilities.ResolveWorkflow(task.Task)
	if !ok || workflowCapability == nil {
		return s.service.failAction(action, result, &ConflictError{Resource: "follow_up_task", ID: task.Task.ID, Reason: firstNonEmpty(reason, "workflow capability is not available")})
	}
	expectedGraphVersion := snapshot.Version
	now := s.service.now()
	snapshot = updateTaskStatus(snapshot, task.Task.ID, TaskQueueStatusExecuting, nil, nil, now)
	if updated, err := s.service.runtime.saveUpdatedTaskGraphSnapshot(snapshot, expectedGraphVersion); err != nil {
		return TaskCommandResult{}, err
	} else {
		snapshot = updated
	}
	expectedExecutionVersion := latest.Version
	latest.Status = TaskQueueStatusExecuting
	latest.PendingApproval = false
	latest.LastTransitionAt = now
	if err := s.service.runtime.Executions.Update(latest, expectedExecutionVersion); err != nil {
		return TaskCommandResult{}, err
	}
	resumed, err := workflowCapability.Resume(ctx, task.Task, buildActivationContext(snapshot.Graph, task), pendingState.State, checkpoint, token, payload)
	if err != nil {
		if explicit {
			latest.Status = TaskQueueStatusFailed
			latest.FailureCategory = failureCategoryFromExecutionError(err)
			latest.FailureSummary = err.Error()
			latest.FailedAt = now
			latest.LastTransitionAt = now
			if updateErr := s.service.runtime.Executions.Update(latest, expectedExecutionVersion+1); updateErr != nil {
				return TaskCommandResult{}, updateErr
			}
			snapshot = applyExecutionRecord(snapshot, latest, FollowUpWorkflowRunResult{})
			if updated, updateErr := s.service.runtime.saveUpdatedTaskGraphSnapshot(snapshot, snapshot.Version); updateErr == nil {
				snapshot = updated
			}
		} else {
			latest.Status = TaskQueueStatusWaitingApproval
			latest.PendingApproval = true
			latest.LastTransitionAt = now
			latest.FailureCategory = ""
			latest.FailureSummary = ""
			latest.FailedAt = time.Time{}
			if updateErr := s.service.runtime.Executions.Update(latest, expectedExecutionVersion+1); updateErr != nil {
				return TaskCommandResult{}, updateErr
			}
			snapshot = updateTaskStatus(snapshot, task.Task.ID, TaskQueueStatusWaitingApproval, []string{"approved but automatic resume attempt failed; explicit resume required"}, nil, now)
			if updated, updateErr := s.service.runtime.saveUpdatedTaskGraphSnapshot(snapshot, snapshot.Version); updateErr == nil {
				snapshot = updated
			}
		}
		return s.service.failAction(action, result, err)
	}
	finalized := finalizeTaskExecutionRecord(latest, resumed, s.service.now())
	if err := s.service.runtime.persistExecutionOutcome(snapshot, &finalized, resumed); err != nil {
		return TaskCommandResult{}, err
	}
	snapshot = applyExecutionRecord(snapshot, finalized, resumed)
	if updated, err := s.service.runtime.saveUpdatedTaskGraphSnapshot(snapshot, snapshot.Version); err != nil {
		return TaskCommandResult{}, err
	} else {
		snapshot = updated
	}
	action.Status = OperatorActionStatusApplied
	action.AppliedAt = &now
	action.ApprovalID = latest.ApprovalID
	action.WorkflowID = finalized.WorkflowID
	if err := s.service.runtime.OperatorActions.Save(action); err != nil {
		return TaskCommandResult{}, err
	}
	if explicit {
		if err := s.service.appendReplay(ReplayEventRecord{
			EventID:           makeID("replay", "resume", finalized.ExecutionID, now),
			RootCorrelationID: finalized.RootCorrelationID,
			ParentWorkflowID:  finalized.ParentWorkflowID,
			WorkflowID:        finalized.WorkflowID,
			GraphID:           snapshot.Graph.GraphID,
			TaskID:            finalized.TaskID,
			ApprovalID:        finalized.ApprovalID,
			ExecutionID:       finalized.ExecutionID,
			ActionType:        string(OperatorActionResume),
			Summary:           fmt.Sprintf("operator resumed follow-up task %s", finalized.TaskID),
			OccurredAt:        now,
			OperatorActionID:  action.ActionID,
		}); err != nil {
			return TaskCommandResult{}, err
		}
	}
	if err := s.service.rebuildTaskGraphProjection(ctx, snapshot.Graph.GraphID); err != nil {
		return TaskCommandResult{}, err
	}
	return TaskCommandResult{
		Action:      action,
		GraphID:     snapshot.Graph.GraphID,
		TaskID:      task.Task.ID,
		ApprovalID:  latest.ApprovalID,
		ExecutionID: finalized.ExecutionID,
		Status:      finalized.Status,
	}, nil
}
