package runtime

import (
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type followUpExecutionReplayDetails struct {
	Status          TaskQueueStatus `json:"status"`
	Intent          string          `json:"intent"`
	FailureCategory FailureCategory `json:"failure_category,omitempty"`
	FailureSummary  string          `json:"failure_summary,omitempty"`
	ApprovalID      string          `json:"approval_id,omitempty"`
	CheckpointID    string          `json:"checkpoint_id,omitempty"`
	ResumeToken     string          `json:"resume_token,omitempty"`
	Committed       bool            `json:"committed"`
	UpdatedStateRef string          `json:"updated_state_ref,omitempty"`
	ArtifactIDs     []string        `json:"artifact_ids,omitempty"`
}

func (r LocalWorkflowRuntime) loadTaskGraphSnapshot(graphID string) (TaskGraphSnapshot, error) {
	if r.TaskGraphs == nil {
		return TaskGraphSnapshot{}, fmt.Errorf("task graph store is required")
	}
	snapshot, ok, err := r.TaskGraphs.Load(graphID)
	if err != nil {
		return TaskGraphSnapshot{}, err
	}
	if !ok {
		return TaskGraphSnapshot{}, &NotFoundError{Resource: "task_graph", ID: graphID}
	}
	if r.Executions != nil {
		executions, err := r.Executions.ListByGraph(graphID)
		if err != nil {
			return TaskGraphSnapshot{}, err
		}
		sortExecutions(executions)
		snapshot.ExecutedTasks = executions
	}
	return snapshot, nil
}

func (r LocalWorkflowRuntime) saveNewTaskGraphSnapshot(snapshot TaskGraphSnapshot) (TaskGraphSnapshot, error) {
	if snapshot.Version == 0 {
		snapshot.Version = 1
	}
	return snapshot, r.TaskGraphs.Save(snapshot)
}

func (r LocalWorkflowRuntime) saveUpdatedTaskGraphSnapshot(snapshot TaskGraphSnapshot, expectedVersion int64) (TaskGraphSnapshot, error) {
	snapshot.Version = expectedVersion + 1
	return snapshot, r.TaskGraphs.Update(snapshot, expectedVersion)
}

func (r LocalWorkflowRuntime) persistExecutionOutcome(snapshot TaskGraphSnapshot, record *TaskExecutionRecord, result FollowUpWorkflowRunResult) error {
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
		if err := r.CheckpointStore.Save(*result.Checkpoint); err != nil {
			return err
		}
	}
	if r.CheckpointStore != nil && result.ResumeToken != nil {
		if err := r.CheckpointStore.SaveResumeToken(*result.ResumeToken); err != nil {
			return err
		}
	}
	if r.CheckpointStore != nil && result.Checkpoint != nil && result.CheckpointPayload != nil {
		if err := r.CheckpointStore.SavePayload(result.Checkpoint.ID, *result.CheckpointPayload); err != nil {
			return err
		}
	}
	if r.Approvals != nil && result.PendingApproval != nil {
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
			if err := r.Artifacts.SaveArtifact(record.WorkflowID, record.TaskID, artifact); err != nil {
				return err
			}
		}
	}
	if r.Executions != nil {
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
		if err := r.Replay.Append(ReplayEventRecord{
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
			CommittedStateRef: committedStateRef(record, snapshot),
			UpdatedStateRef:   record.UpdatedStateSnapshotRef,
			ArtifactIDs:       append([]string{}, record.ArtifactIDs...),
			CheckpointID:      record.CheckpointID,
			ResumeToken:       record.ResumeToken,
		}); err != nil {
			return err
		}
	}
	return nil
}

func committedStateRef(record *TaskExecutionRecord, snapshot TaskGraphSnapshot) string {
	if record == nil || !record.Committed {
		return snapshot.LatestCommittedStateRef
	}
	if record.UpdatedStateSnapshotRef != "" {
		return record.UpdatedStateSnapshotRef
	}
	return snapshot.LatestCommittedStateRef
}

func (r LocalWorkflowRuntime) loadStateSnapshot(snapshotRef string) (state.StateSnapshot, error) {
	if r.TaskGraphs == nil {
		return state.StateSnapshot{}, fmt.Errorf("task graph store is required")
	}
	snapshot, ok, err := r.TaskGraphs.LoadStateSnapshot(snapshotRef)
	if err != nil {
		return state.StateSnapshot{}, err
	}
	if !ok {
		return state.StateSnapshot{}, &NotFoundError{Resource: "state_snapshot", ID: snapshotRef}
	}
	return snapshot, nil
}
