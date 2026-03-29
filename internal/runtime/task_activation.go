package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func (r LocalWorkflowRuntime) ExecuteReadyFollowUps(ctx context.Context, execCtx ExecutionContext, graphID string, policy AutoExecutionPolicy) (FollowUpExecutionBatchResult, error) {
	if r.TaskGraphs == nil {
		return FollowUpExecutionBatchResult{}, fmt.Errorf("task graph store is required")
	}
	if policy.MaxExecutionDepth <= 0 {
		policy = DefaultAutoExecutionPolicy()
	}
	snapshot, err := r.loadTaskGraphSnapshot(graphID)
	if err != nil {
		return FollowUpExecutionBatchResult{}, err
	}
	expectedVersion := snapshot.Version
	executed := make([]TaskExecutionRecord, 0)

	for {
		updated, _, err := ReevaluateFollowUpTaskGraph(snapshot, r.Capabilities, r.now())
		if err != nil {
			return FollowUpExecutionBatchResult{}, err
		}
		dirty := false
		if graphMutationChanged(snapshot, updated) {
			snapshot, err = r.saveUpdatedTaskGraphSnapshot(updated, expectedVersion)
			if err != nil {
				return FollowUpExecutionBatchResult{}, err
			}
			expectedVersion = snapshot.Version
		} else {
			snapshot = updated
		}
		progressed := false
		for _, task := range ReadyTasksInExecutionOrder(snapshot) {
			allowed, suppressedReason := policy.allows(task)
			if !allowed {
				snapshot = updateTaskSuppression(snapshot, task.Task.ID, suppressedReason, r.now())
				dirty = true
				r.log(execCtx, "follow_up_task_suppressed", fmt.Sprintf("suppressed auto-run for task %s", task.Task.ID), map[string]string{
					"graph_id":           graphID,
					"task_id":            task.Task.ID,
					"intent":             string(task.Task.UserIntentType),
					"suppressed_reasons": suppressedReason,
				})
				continue
			}
			snapshot = clearTaskSuppression(snapshot, task.Task.ID, r.now())
			dirty = true
			workflowCapability, ok, reason := r.Capabilities.ResolveWorkflow(task.Task)
			if !ok || workflowCapability == nil {
				snapshot = updateTaskSuppression(snapshot, task.Task.ID, reason, r.now())
				dirty = true
				continue
			}
			inputState := snapshot.LatestCommittedStateSnapshot.State
			if inputState.Version.Sequence == 0 {
				inputState = snapshot.BaseStateSnapshot.State
			}
			record, childResult, err := r.executeSingleFollowUp(ctx, execCtx, snapshot, task, workflowCapability, inputState, policy.MaxAttempts)
			if err != nil {
				return FollowUpExecutionBatchResult{}, err
			}
			if err := r.persistExecutionOutcome(snapshot, &record, childResult); err != nil {
				return FollowUpExecutionBatchResult{}, err
			}
			executed = append(executed, record)
			snapshot = applyExecutionRecord(snapshot, record, childResult)
			snapshot, err = r.saveUpdatedTaskGraphSnapshot(snapshot, expectedVersion)
			if err != nil {
				return FollowUpExecutionBatchResult{}, err
			}
			expectedVersion = snapshot.Version
			dirty = false
			progressed = true
			break
		}
		if !progressed {
			if dirty {
				snapshot, err = r.saveUpdatedTaskGraphSnapshot(snapshot, expectedVersion)
				if err != nil {
					return FollowUpExecutionBatchResult{}, err
				}
				expectedVersion = snapshot.Version
			}
			break
		}
	}
	return FollowUpExecutionBatchResult{
		GraphID:                      graphID,
		RegisteredTasks:              append([]FollowUpTaskRecord{}, snapshot.RegisteredTasks...),
		ExecutedTasks:                executed,
		LatestCommittedStateSnapshot: snapshot.LatestCommittedStateSnapshot,
		ExecutedAt:                   r.now(),
	}, nil
}

func (r LocalWorkflowRuntime) executeSingleFollowUp(
	ctx context.Context,
	parentExecCtx ExecutionContext,
	snapshot TaskGraphSnapshot,
	task FollowUpTaskRecord,
	capability FollowUpWorkflowCapability,
	inputState state.FinancialWorldState,
	maxAttempts int,
) (TaskExecutionRecord, FollowUpWorkflowRunResult, error) {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	startedAt := r.now()
	activation := buildActivationContext(snapshot.Graph, task)
	record := TaskExecutionRecord{
		ExecutionID:           makeID(snapshot.Graph.GraphID, task.Task.ID, startedAt),
		TaskID:                task.Task.ID,
		Intent:                task.Task.UserIntentType,
		ParentGraphID:         snapshot.Graph.GraphID,
		ParentWorkflowID:      snapshot.Graph.ParentWorkflowID,
		RootTaskID:            activation.RootTaskID,
		TriggeredByTaskID:     activation.TriggeredByTaskID,
		ExecutionDepth:        activation.ExecutionDepth,
		Capability:            task.RequiredCapability,
		RootCorrelationID:     activation.RootCorrelationID,
		CorrelationID:         activation.RootCorrelationID,
		CausationID:           task.Task.ID,
		Status:                TaskQueueStatusExecuting,
		Version:               1,
		Attempt:               1,
		LastTransitionAt:      startedAt,
		StartedAt:             startedAt,
		InputStateVersion:     inputState.Version.Sequence,
		InputStateSnapshotRef: inputState.Version.SnapshotID,
	}
	snapshot = updateTaskStatus(snapshot, task.Task.ID, TaskQueueStatusExecuting, nil, nil, startedAt)
	r.log(parentExecCtx, "follow_up_task_execution_started", fmt.Sprintf("executing follow-up task %s", task.Task.ID), map[string]string{
		"graph_id":            snapshot.Graph.GraphID,
		"task_id":             task.Task.ID,
		"intent":              string(task.Task.UserIntentType),
		"required_capability": task.RequiredCapability,
	})

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		record.Attempt = attempt
		childResult, err := capability.Execute(ctx, task.Task, activation, inputState)
		if err == nil {
			finalized := finalizeTaskExecutionRecord(record, childResult, r.now())
			logCategory := "follow_up_task_execution_completed"
			logMessage := fmt.Sprintf("completed follow-up task %s", task.Task.ID)
			if finalized.Status == TaskQueueStatusWaitingApproval {
				logCategory = "follow_up_task_execution_waiting_approval"
				logMessage = fmt.Sprintf("follow-up task %s is waiting approval", task.Task.ID)
			}
			r.log(parentExecCtx, logCategory, logMessage, map[string]string{
				"graph_id":     snapshot.Graph.GraphID,
				"task_id":      task.Task.ID,
				"workflow_id":  finalized.WorkflowID,
				"status":       string(finalized.Status),
				"artifact_ids": strings.Join(finalized.ArtifactIDs, ","),
			})
			return finalized, childResult, nil
		}
		lastErr = err
		category := failureCategoryFromExecutionError(err)
		next, strategy, handleErr := r.HandleFailure(ExecutionContext{
			WorkflowID:    snapshot.Graph.ParentWorkflowID,
			TaskID:        task.Task.ID,
			CorrelationID: activation.RootCorrelationID,
			Attempt:       attempt,
		}, WorkflowStateActing, category, err.Error())
		if handleErr != nil {
			return TaskExecutionRecord{}, FollowUpWorkflowRunResult{}, handleErr
		}
		record.LastRecoveryStrategy = strategy
		record.LastTransitionAt = r.now()
		if strategy == RecoveryStrategyRetry && attempt < maxAttempts {
			record.RetryCount++
			r.log(parentExecCtx, "follow_up_task_retry", fmt.Sprintf("retrying follow-up task %s", task.Task.ID), map[string]string{
				"graph_id":          snapshot.Graph.GraphID,
				"task_id":           task.Task.ID,
				"attempt":           fmt.Sprintf("%d", attempt+1),
				"failure_category":  string(category),
				"recovery_strategy": string(strategy),
			})
			continue
		}
		record.Status = TaskQueueStatusFailed
		record.FailureCategory = category
		record.FailureSummary = err.Error()
		record.FailedAt = r.now()
		record.LastTransitionAt = r.now()
		if next == WorkflowStateWaitingApproval {
			record.Status = TaskQueueStatusWaitingApproval
		}
		break
	}
	if lastErr != nil {
		logCategory := "follow_up_task_execution_failed"
		if record.Status == TaskQueueStatusWaitingApproval {
			logCategory = "follow_up_task_execution_waiting_approval"
		}
		r.log(parentExecCtx, logCategory, fmt.Sprintf("follow-up task %s ended with %s", task.Task.ID, record.Status), map[string]string{
			"graph_id":          snapshot.Graph.GraphID,
			"task_id":           task.Task.ID,
			"status":            string(record.Status),
			"failure_category":  string(record.FailureCategory),
			"failure_summary":   record.FailureSummary,
			"recovery_strategy": string(record.LastRecoveryStrategy),
		})
		return record, FollowUpWorkflowRunResult{}, nil
	}
	return record, FollowUpWorkflowRunResult{}, nil
}

func buildActivationContext(graph taskspec.TaskGraph, task FollowUpTaskRecord) FollowUpActivationContext {
	sourceEvidenceIDs := make([]string, 0)
	lifeEventID := ""
	lifeEventKind := ""
	for _, reason := range task.Metadata.GenerationReasons {
		if reason.LifeEventID != "" && lifeEventID == "" {
			lifeEventID = reason.LifeEventID
		}
		if reason.LifeEventKind != "" && lifeEventKind == "" {
			lifeEventKind = reason.LifeEventKind
		}
		sourceEvidenceIDs = append(sourceEvidenceIDs, reason.EvidenceIDs...)
		sourceEvidenceIDs = append(sourceEvidenceIDs, reason.DeadlineEvidence...)
	}
	return FollowUpActivationContext{
		GraphID:           graph.GraphID,
		ParentGraphID:     graph.GraphID,
		ParentWorkflowID:  graph.ParentWorkflowID,
		RootTaskID:        graph.ParentTaskID,
		TriggeredByTaskID: task.Task.ID,
		RootCorrelationID: firstNonEmpty(task.Metadata.RootCorrelationID, graph.ParentWorkflowID),
		TriggerSource:     task.Metadata.TriggerSource,
		GenerationReasons: append([]taskspec.TaskGenerationReason{}, task.Metadata.GenerationReasons...),
		LifeEventID:       lifeEventID,
		LifeEventKind:     lifeEventKind,
		SourceEvidenceIDs: uniqueStrings(sourceEvidenceIDs),
		ExecutionDepth:    task.Metadata.ExecutionDepth,
	}
}

func finalizeTaskExecutionRecord(record TaskExecutionRecord, result FollowUpWorkflowRunResult, now time.Time) TaskExecutionRecord {
	record.WorkflowID = result.WorkflowID
	if result.LastRecoveryStrategy != "" {
		record.LastRecoveryStrategy = result.LastRecoveryStrategy
	}
	record.LastTransitionAt = now
	record.UpdatedStateVersion = result.UpdatedState.Version.Sequence
	record.UpdatedStateSnapshotRef = result.UpdatedState.Version.SnapshotID
	record.ArtifactIDs = artifactIDs(result.Artifacts)
	switch result.RuntimeState {
	case WorkflowStateCompleted:
		record.Status = TaskQueueStatusCompleted
		record.Committed = true
		record.CompletedAt = now
	case WorkflowStateWaitingApproval:
		record.Status = TaskQueueStatusWaitingApproval
		record.PendingApproval = result.PendingApproval != nil
		record.Committed = false
		if result.PendingApproval != nil {
			record.ApprovalID = result.PendingApproval.ApprovalID
		}
		if result.Checkpoint != nil {
			record.CheckpointID = result.Checkpoint.ID
			record.ResumeState = result.Checkpoint.ResumeState
		}
		if result.ResumeToken != nil {
			record.ResumeToken = result.ResumeToken.Token
		}
	case WorkflowStateFailed, WorkflowStateReplanning, WorkflowStateRetrying:
		record.Status = TaskQueueStatusFailed
		record.FailureCategory = result.FailureCategory
		record.FailureSummary = result.FailureSummary
		record.FailedAt = now
	default:
		record.Status = TaskQueueStatusFailed
		record.FailureSummary = firstNonEmpty(result.FailureSummary, "follow-up workflow returned an unsupported terminal state")
		record.FailedAt = now
	}
	return record
}

func applyExecutionRecord(snapshot TaskGraphSnapshot, record TaskExecutionRecord, result FollowUpWorkflowRunResult) TaskGraphSnapshot {
	snapshot.ExecutedTasks = append(snapshot.ExecutedTasks, record)
	switch record.Status {
	case TaskQueueStatusCompleted:
		snapshot.LatestCommittedStateSnapshot = result.UpdatedState.Snapshot("follow_up_task_committed", record.CompletedAt)
		snapshot.LatestCommittedStateRef = firstNonEmpty(record.UpdatedStateSnapshotRef, snapshotRefFor(snapshot.LatestCommittedStateSnapshot))
	case TaskQueueStatusWaitingApproval, TaskQueueStatusFailed:
		// Intentionally do not advance committed state for non-terminal success outcomes.
	}
	snapshot = updateTaskStatus(snapshot, record.TaskID, record.Status, executionBlockingReasons(record), nil, record.LastTransitionAt)
	return snapshot
}

func graphMutationChanged(before TaskGraphSnapshot, after TaskGraphSnapshot) bool {
	return !reflect.DeepEqual(before.RegisteredTasks, after.RegisteredTasks) ||
		!reflect.DeepEqual(before.Spawned, after.Spawned) ||
		!reflect.DeepEqual(before.Deferred, after.Deferred) ||
		before.LatestCommittedStateRef != after.LatestCommittedStateRef
}

func updateTaskStatus(
	snapshot TaskGraphSnapshot,
	taskID string,
	status TaskQueueStatus,
	blocking []string,
	suppressed []string,
	at time.Time,
) TaskGraphSnapshot {
	for i := range snapshot.RegisteredTasks {
		if snapshot.RegisteredTasks[i].Task.ID != taskID {
			continue
		}
		snapshot.RegisteredTasks[i].Status = status
		snapshot.RegisteredTasks[i].Version++
		if blocking != nil {
			snapshot.RegisteredTasks[i].BlockingReasons = append([]string{}, blocking...)
		}
		if suppressed != nil {
			snapshot.RegisteredTasks[i].SuppressedReasons = append([]string{}, suppressed...)
		}
		snapshot.RegisteredTasks[i].LastUpdatedAt = at
	}
	snapshot.Spawned, snapshot.Deferred = deriveSpawnedAndDeferred(snapshot.RegisteredTasks, at)
	return snapshot
}

func updateTaskSuppression(snapshot TaskGraphSnapshot, taskID string, reason string, at time.Time) TaskGraphSnapshot {
	for i := range snapshot.RegisteredTasks {
		if snapshot.RegisteredTasks[i].Task.ID != taskID {
			continue
		}
		snapshot.RegisteredTasks[i].Version++
		snapshot.RegisteredTasks[i].SuppressedReasons = uniqueStrings(append(snapshot.RegisteredTasks[i].SuppressedReasons, reason))
		snapshot.RegisteredTasks[i].LastUpdatedAt = at
	}
	return snapshot
}

func clearTaskSuppression(snapshot TaskGraphSnapshot, taskID string, at time.Time) TaskGraphSnapshot {
	for i := range snapshot.RegisteredTasks {
		if snapshot.RegisteredTasks[i].Task.ID != taskID {
			continue
		}
		snapshot.RegisteredTasks[i].Version++
		snapshot.RegisteredTasks[i].SuppressedReasons = nil
		snapshot.RegisteredTasks[i].LastUpdatedAt = at
	}
	return snapshot
}

func artifactIDs(items []reporting.WorkflowArtifact) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}

func executionBlockingReasons(record TaskExecutionRecord) []string {
	switch record.Status {
	case TaskQueueStatusWaitingApproval:
		if record.ApprovalID != "" {
			return []string{fmt.Sprintf("waiting for approval %s", record.ApprovalID)}
		}
		return []string{"waiting for human approval"}
	case TaskQueueStatusFailed:
		if record.FailureSummary != "" {
			return []string{record.FailureSummary}
		}
		return []string{"follow-up execution failed"}
	default:
		return nil
	}
}

func failureCategoryFromExecutionError(err error) FailureCategory {
	if err == nil {
		return ""
	}
	var categorized *FollowUpExecutionError
	if errors.As(err, &categorized) && categorized != nil {
		if categorized.Category != "" {
			return categorized.Category
		}
	}
	if category, ok := FailureCategoryFromAgentError(err); ok {
		return category
	}
	return FailureCategoryUnrecoverable
}

func firstNonEmpty(values ...string) string {
	for _, item := range values {
		if item != "" {
			return item
		}
	}
	return ""
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
