package runtime

import "fmt"

func (r LocalWorkflowRuntime) validateFence(fence *FenceValidation) error {
	if fence == nil || r.FenceValidator == nil {
		return nil
	}
	return r.FenceValidator.ValidateFence(*fence)
}

func (r LocalWorkflowRuntime) saveUpdatedTaskGraphSnapshotGuarded(snapshot TaskGraphSnapshot, expectedVersion int64, fence *FenceValidation) (TaskGraphSnapshot, error) {
	if err := r.validateFence(fence); err != nil {
		return TaskGraphSnapshot{}, err
	}
	return r.saveUpdatedTaskGraphSnapshot(snapshot, expectedVersion)
}

func (r LocalWorkflowRuntime) persistExecutionOutcomeGuarded(snapshot TaskGraphSnapshot, record *TaskExecutionRecord, result FollowUpWorkflowRunResult, fence *FenceValidation) error {
	if err := r.validateFence(fence); err != nil {
		return err
	}
	if record == nil {
		return nil
	}
	if record.ExecutionID == "" {
		return fmt.Errorf("task execution requires execution id")
	}
	if record.Version == 0 {
		record.Version = 1
	}
	if r.TaskGraphs != nil && result.UpdatedState.Version.SnapshotID != "" {
		if err := r.validateFence(fence); err != nil {
			return err
		}
		kind := "follow_up_pending"
		if record.Status == TaskQueueStatusCompleted {
			kind = "follow_up_committed"
		}
		ref, err := r.TaskGraphs.SaveStateSnapshot(
			snapshot.Graph.GraphID,
			record.WorkflowID,
			record.TaskID,
			kind,
			result.UpdatedState.Snapshot(kind, record.LastTransitionAt),
		)
		if err != nil {
			return err
		}
		record.UpdatedStateSnapshotRef = ref
	}
	if r.CheckpointStore != nil && result.Checkpoint != nil {
		if err := r.validateFence(fence); err != nil {
			return err
		}
		if err := r.CheckpointStore.Save(*result.Checkpoint); err != nil {
			return err
		}
	}
	if r.CheckpointStore != nil && result.ResumeToken != nil {
		if err := r.validateFence(fence); err != nil {
			return err
		}
		if err := r.CheckpointStore.SaveResumeToken(*result.ResumeToken); err != nil {
			return err
		}
	}
	if r.CheckpointStore != nil && result.Checkpoint != nil && result.CheckpointPayload != nil {
		if err := r.validateFence(fence); err != nil {
			return err
		}
		if err := r.CheckpointStore.SavePayload(result.Checkpoint.ID, *result.CheckpointPayload); err != nil {
			return err
		}
	}
	if r.Approvals != nil && result.PendingApproval != nil {
		if err := r.validateFence(fence); err != nil {
			return err
		}
		approval := ApprovalStateRecord{
			ApprovalID:      result.PendingApproval.ApprovalID,
			GraphID:         snapshot.Graph.GraphID,
			TaskID:          record.TaskID,
			WorkflowID:      result.PendingApproval.WorkflowID,
			ExecutionID:     record.ExecutionID,
			RequestedAction: result.PendingApproval.RequestedAction,
			RequiredRoles:   append([]string{}, result.PendingApproval.RequiredRoles...),
			RequestedAt:     result.PendingApproval.RequestedAt,
			Deadline:        result.PendingApproval.Deadline,
			Status:          ApprovalStatusPending,
			Version:         1,
		}
		if err := r.Approvals.Save(approval); err != nil && !IsConflict(err) {
			return err
		}
	}
	if r.Artifacts != nil {
		for _, artifact := range result.Artifacts {
			if err := r.validateFence(fence); err != nil {
				return err
			}
			if err := r.Artifacts.SaveArtifact(record.WorkflowID, record.TaskID, artifact); err != nil {
				return err
			}
		}
	}
	if r.Executions != nil {
		if err := r.validateFence(fence); err != nil {
			return err
		}
		if current, ok, err := r.Executions.Load(record.ExecutionID); err != nil {
			return err
		} else if ok {
			if err := r.Executions.Update(*record, current.Version); err != nil {
				return err
			}
		} else {
			if err := r.Executions.Save(*record); err != nil {
				return err
			}
		}
	}
	if r.Replay != nil {
		if err := r.validateFence(fence); err != nil {
			return err
		}
		return r.persistExecutionReplay(snapshot, *record)
	}
	return nil
}

func (r LocalWorkflowRuntime) persistExecutionReplay(snapshot TaskGraphSnapshot, record TaskExecutionRecord) error {
	details, err := marshalJSON(followUpExecutionReplayDetails{
		Status:          record.Status,
		Intent:          string(record.Intent),
		FailureCategory: record.FailureCategory,
		FailureSummary:  record.FailureSummary,
		ApprovalID:      record.ApprovalID,
		CheckpointID:    record.CheckpointID,
		ResumeToken:     record.ResumeToken,
		Committed:       record.Committed,
		UpdatedStateRef: record.UpdatedStateSnapshotRef,
		ArtifactIDs:     append([]string{}, record.ArtifactIDs...),
	})
	if err != nil {
		return err
	}
	return r.Replay.Append(ReplayEventRecord{
		EventID:           makeID("replay", snapshot.Graph.GraphID, record.ExecutionID, record.Status, record.LastTransitionAt),
		RootCorrelationID: record.RootCorrelationID,
		ParentWorkflowID:  record.ParentWorkflowID,
		WorkflowID:        record.WorkflowID,
		GraphID:           snapshot.Graph.GraphID,
		TaskID:            record.TaskID,
		ApprovalID:        record.ApprovalID,
		ExecutionID:       record.ExecutionID,
		ActionType:        "follow_up_execution",
		Summary:           fmt.Sprintf("follow-up task %s ended as %s", record.TaskID, record.Status),
		OccurredAt:        record.LastTransitionAt,
		DetailsJSON:       details,
		CommittedStateRef: committedStateRef(&record, snapshot),
		UpdatedStateRef:   record.UpdatedStateSnapshotRef,
		ArtifactIDs:       append([]string{}, record.ArtifactIDs...),
		CheckpointID:      record.CheckpointID,
		ResumeToken:       record.ResumeToken,
	})
}
