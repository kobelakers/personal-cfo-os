package observability

import (
	"testing"
	"time"
)

type replayServiceTestStore struct {
	graphEvents    map[string][]ReplayEvent
	taskEvents     map[string][]ReplayEvent
	workflowEvents map[string][]ReplayEvent
}

func (s replayServiceTestStore) ListByGraph(graphID string) ([]ReplayEvent, error) {
	return append([]ReplayEvent{}, s.graphEvents[graphID]...), nil
}

func (s replayServiceTestStore) ListByTask(taskID string) ([]ReplayEvent, error) {
	return append([]ReplayEvent{}, s.taskEvents[taskID]...), nil
}

func (s replayServiceTestStore) ListByWorkflow(workflowID string) ([]ReplayEvent, error) {
	return append([]ReplayEvent{}, s.workflowEvents[workflowID]...), nil
}

func TestReplayServiceBuildsQueryableRuntimeTrace(t *testing.T) {
	now := time.Date(2026, 3, 29, 16, 0, 0, 0, time.UTC)
	events := []ReplayEvent{
		{
			EventID:           "event-1",
			RootCorrelationID: "workflow-life-event-1",
			ParentWorkflowID:  "workflow-life-event-1",
			WorkflowID:        "workflow-child-1",
			GraphID:           "graph-1",
			TaskID:            "task-1",
			ExecutionID:       "execution-1",
			ActionType:        "follow_up_execution",
			Summary:           "child execution waiting approval",
			OccurredAt:        now,
			CheckpointID:      "checkpoint-1",
			ResumeToken:       "resume-1",
		},
		{
			EventID:           "event-2",
			RootCorrelationID: "workflow-life-event-1",
			ParentWorkflowID:  "workflow-life-event-1",
			WorkflowID:        "workflow-child-1",
			GraphID:           "graph-1",
			TaskID:            "task-1",
			ApprovalID:        "approval-1",
			ActionType:        "approve",
			Summary:           "operator approved task",
			OccurredAt:        now.Add(time.Minute),
			OperatorActionID:  "action-approve-1",
		},
		{
			EventID:           "event-3",
			RootCorrelationID: "workflow-life-event-1",
			ParentWorkflowID:  "workflow-life-event-1",
			WorkflowID:        "workflow-child-1",
			GraphID:           "graph-1",
			TaskID:            "task-1",
			ExecutionID:       "execution-1",
			ActionType:        "follow_up_execution",
			Summary:           "child execution completed",
			OccurredAt:        now.Add(2 * time.Minute),
			CommittedStateRef: "state-v2",
			UpdatedStateRef:   "state-v2",
			ArtifactIDs:       []string{"artifact-1"},
		},
	}
	service := NewReplayService(replayServiceTestStore{
		graphEvents:    map[string][]ReplayEvent{"graph-1": events},
		taskEvents:     map[string][]ReplayEvent{"task-1": events},
		workflowEvents: map[string][]ReplayEvent{"workflow-child-1": events},
	})
	trace, err := service.ByGraph("graph-1")
	if err != nil {
		t.Fatalf("query replay by graph: %v", err)
	}
	if trace.Provenance.RootCorrelationID != "workflow-life-event-1" {
		t.Fatalf("expected root correlation id, got %+v", trace.Provenance)
	}
	if trace.Attempts != 2 || trace.ApprovalActions != 1 || trace.CommittedAdvances != 1 {
		t.Fatalf("expected attempts/approvals/commits to be summarized, got %+v", trace)
	}
	if len(trace.Provenance.ExecutionIDs) != 1 || len(trace.Provenance.ArtifactIDs) != 1 {
		t.Fatalf("expected provenance chain to include execution and artifact ids, got %+v", trace.Provenance)
	}
}
