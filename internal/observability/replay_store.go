package observability

import "time"

type ReplayEvent struct {
	EventID           string    `json:"event_id"`
	RootCorrelationID string    `json:"root_correlation_id,omitempty"`
	ParentWorkflowID  string    `json:"parent_workflow_id,omitempty"`
	WorkflowID        string    `json:"workflow_id,omitempty"`
	GraphID           string    `json:"graph_id,omitempty"`
	TaskID            string    `json:"task_id,omitempty"`
	ApprovalID        string    `json:"approval_id,omitempty"`
	ExecutionID       string    `json:"execution_id,omitempty"`
	ActionType        string    `json:"action_type"`
	Summary           string    `json:"summary"`
	OccurredAt        time.Time `json:"occurred_at"`
	DetailsJSON       string    `json:"details_json,omitempty"`
	CommittedStateRef string    `json:"committed_state_ref,omitempty"`
	UpdatedStateRef   string    `json:"updated_state_ref,omitempty"`
	ArtifactIDs       []string  `json:"artifact_ids,omitempty"`
	OperatorActionID  string    `json:"operator_action_id,omitempty"`
	CheckpointID      string    `json:"checkpoint_id,omitempty"`
	ResumeToken       string    `json:"resume_token,omitempty"`
}

type ReplayQueryStore interface {
	ListByGraph(graphID string) ([]ReplayEvent, error)
	ListByTask(taskID string) ([]ReplayEvent, error)
	ListByWorkflow(workflowID string) ([]ReplayEvent, error)
}
