package runtime

import "github.com/kobelakers/personal-cfo-os/internal/observability"

type ObservabilityReplayStore struct {
	Store ReplayStore
}

func NewObservabilityReplayStore(store ReplayStore) ObservabilityReplayStore {
	return ObservabilityReplayStore{Store: store}
}

func (s ObservabilityReplayStore) ListByGraph(graphID string) ([]observability.ReplayEvent, error) {
	if s.Store == nil {
		return nil, nil
	}
	events, err := s.Store.ListByGraph(graphID)
	if err != nil {
		return nil, err
	}
	return convertReplayEvents(events), nil
}

func (s ObservabilityReplayStore) ListByTask(taskID string) ([]observability.ReplayEvent, error) {
	if s.Store == nil {
		return nil, nil
	}
	events, err := s.Store.ListByTask(taskID)
	if err != nil {
		return nil, err
	}
	return convertReplayEvents(events), nil
}

func (s ObservabilityReplayStore) ListByWorkflow(workflowID string) ([]observability.ReplayEvent, error) {
	if s.Store == nil {
		return nil, nil
	}
	events, err := s.Store.ListByWorkflow(workflowID)
	if err != nil {
		return nil, err
	}
	return convertReplayEvents(events), nil
}

func convertReplayEvents(events []ReplayEventRecord) []observability.ReplayEvent {
	result := make([]observability.ReplayEvent, 0, len(events))
	for _, event := range events {
		result = append(result, observability.ReplayEvent{
			EventID:           event.EventID,
			RootCorrelationID: event.RootCorrelationID,
			ParentWorkflowID:  event.ParentWorkflowID,
			WorkflowID:        event.WorkflowID,
			GraphID:           event.GraphID,
			TaskID:            event.TaskID,
			ApprovalID:        event.ApprovalID,
			ExecutionID:       event.ExecutionID,
			ActionType:        event.ActionType,
			Summary:           event.Summary,
			OccurredAt:        event.OccurredAt,
			DetailsJSON:       event.DetailsJSON,
			CommittedStateRef: event.CommittedStateRef,
			UpdatedStateRef:   event.UpdatedStateRef,
			ArtifactIDs:       append([]string{}, event.ArtifactIDs...),
			OperatorActionID:  event.OperatorActionID,
			CheckpointID:      event.CheckpointID,
			ResumeToken:       event.ResumeToken,
		})
	}
	return result
}
