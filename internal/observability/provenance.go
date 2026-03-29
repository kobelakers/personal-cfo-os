package observability

type ProvenanceChain struct {
	RootCorrelationID string   `json:"root_correlation_id,omitempty"`
	ParentWorkflowID  string   `json:"parent_workflow_id,omitempty"`
	WorkflowIDs       []string `json:"workflow_ids,omitempty"`
	TaskIDs           []string `json:"task_ids,omitempty"`
	ApprovalIDs       []string `json:"approval_ids,omitempty"`
	ExecutionIDs      []string `json:"execution_ids,omitempty"`
	ArtifactIDs       []string `json:"artifact_ids,omitempty"`
	CheckpointIDs     []string `json:"checkpoint_ids,omitempty"`
	ResumeTokens      []string `json:"resume_tokens,omitempty"`
}
