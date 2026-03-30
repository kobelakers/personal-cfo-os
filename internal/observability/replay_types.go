package observability

import "time"

type ReplayQuery struct {
	WorkflowID  string `json:"workflow_id,omitempty"`
	TaskGraphID string `json:"task_graph_id,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	ExecutionID string `json:"execution_id,omitempty"`
	ApprovalID  string `json:"approval_id,omitempty"`
}

type ReplayDegradationReason string

const (
	ReplayDegradationProjectionMissing    ReplayDegradationReason = "projection_missing"
	ReplayDegradationProjectionIncomplete ReplayDegradationReason = "projection_incomplete"
	ReplayDegradationProjectionStale      ReplayDegradationReason = "projection_stale"
	ReplayDegradationBestEffortAssembly   ReplayDegradationReason = "best_effort_assembly"
)

type ReplayDegradation struct {
	Reason  ReplayDegradationReason `json:"reason"`
	Message string                  `json:"message"`
}

type ReplayArtifactRef struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	WorkflowID string    `json:"workflow_id,omitempty"`
	TaskID     string    `json:"task_id,omitempty"`
	Location   string    `json:"location,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
}

type ReplayScope struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type WorkflowReplayView struct {
	WorkflowID       string             `json:"workflow_id"`
	TaskID           string             `json:"task_id,omitempty"`
	Intent           string             `json:"intent,omitempty"`
	RuntimeState     string             `json:"runtime_state"`
	FailureCategory  string             `json:"failure_category,omitempty"`
	FailureSummary   string             `json:"failure_summary,omitempty"`
	ApprovalID       string             `json:"approval_id,omitempty"`
	TaskGraphID      string             `json:"task_graph_id,omitempty"`
	RootCorrelationID string            `json:"root_correlation_id,omitempty"`
	Summary          string             `json:"summary,omitempty"`
	Artifacts        []ReplayArtifactRef `json:"artifacts,omitempty"`
}

type TaskGraphReplayView struct {
	TaskGraphID       string             `json:"task_graph_id"`
	ParentWorkflowID  string             `json:"parent_workflow_id,omitempty"`
	ParentTaskID      string             `json:"parent_task_id,omitempty"`
	PendingApprovalID string             `json:"pending_approval_id,omitempty"`
	TaskIDs           []string           `json:"task_ids,omitempty"`
	ExecutionIDs      []string           `json:"execution_ids,omitempty"`
	Artifacts         []ReplayArtifactRef `json:"artifacts,omitempty"`
}

type ApprovalReplayView struct {
	ApprovalID      string    `json:"approval_id"`
	WorkflowID      string    `json:"workflow_id,omitempty"`
	TaskGraphID     string    `json:"task_graph_id,omitempty"`
	TaskID          string    `json:"task_id,omitempty"`
	Status          string    `json:"status"`
	RequestedAction string    `json:"requested_action,omitempty"`
	RequestedAt     time.Time `json:"requested_at,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy      string    `json:"resolved_by,omitempty"`
}

type ProvenanceNode struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	RefID      string            `json:"ref_id,omitempty"`
	Label      string            `json:"label"`
	Summary    string            `json:"summary,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type ProvenanceEdge struct {
	ID         string            `json:"id"`
	FromNodeID string            `json:"from_node_id"`
	ToNodeID   string            `json:"to_node_id"`
	Type       string            `json:"type"`
	Reason     string            `json:"reason,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type ProvenanceGraph struct {
	Scope ReplayScope      `json:"scope"`
	Nodes []ProvenanceNode `json:"nodes,omitempty"`
	Edges []ProvenanceEdge `json:"edges,omitempty"`
}

type ExecutionAttribution struct {
	ExecutionID string            `json:"execution_id"`
	Category    string            `json:"category"`
	Summary     string            `json:"summary"`
	SourceRefs  []string          `json:"source_refs,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
}

type FailureAttribution struct {
	FailureCategory string            `json:"failure_category"`
	ReasonCode      string            `json:"reason_code,omitempty"`
	Summary         string            `json:"summary"`
	RelatedKind     string            `json:"related_kind,omitempty"`
	RelatedID       string            `json:"related_id,omitempty"`
	SourceRefs      []string          `json:"source_refs,omitempty"`
	Details         map[string]string `json:"details,omitempty"`
}

type ReplaySummary struct {
	GoalSummary          string   `json:"goal_summary,omitempty"`
	PlanSummary          []string `json:"plan_summary,omitempty"`
	MemorySummary        []string `json:"memory_summary,omitempty"`
	ValidatorSummary     []string `json:"validator_summary,omitempty"`
	GovernanceSummary    []string `json:"governance_summary,omitempty"`
	ChildWorkflowSummary []string `json:"child_workflow_summary,omitempty"`
	FinalState           string   `json:"final_state,omitempty"`
}

type ReplayExplanation struct {
	WhyFailed          string   `json:"why_failed,omitempty"`
	WhyWaitingApproval string   `json:"why_waiting_approval,omitempty"`
	WhyGeneratedTask   []string `json:"why_generated_task,omitempty"`
	WhyChildExecuted   []string `json:"why_child_executed,omitempty"`
	WhyMemoryDecision  []string `json:"why_memory_decision,omitempty"`
	WhyValidationFailed []string `json:"why_validation_failed,omitempty"`
}

type ReplayView struct {
	Query                ReplayQuery          `json:"query"`
	Scope                ReplayScope          `json:"scope"`
	Workflow             *WorkflowReplayView  `json:"workflow,omitempty"`
	TaskGraph            *TaskGraphReplayView `json:"task_graph,omitempty"`
	Approval             *ApprovalReplayView  `json:"approval,omitempty"`
	Provenance           ProvenanceGraph      `json:"provenance"`
	ExecutionAttributions []ExecutionAttribution `json:"execution_attributions,omitempty"`
	FailureAttributions   []FailureAttribution   `json:"failure_attributions,omitempty"`
	Summary              ReplaySummary       `json:"summary"`
	Explanation          ReplayExplanation   `json:"explanation"`
	Degraded             bool                `json:"degraded"`
	DegradationReasons   []ReplayDegradation `json:"degradation_reasons,omitempty"`
	ProjectionStatus     string              `json:"projection_status,omitempty"`
	ProjectionVersion    int                 `json:"projection_version,omitempty"`
}

type ReplayComparisonDiff struct {
	Category string   `json:"category"`
	Field    string   `json:"field"`
	Left     []string `json:"left,omitempty"`
	Right    []string `json:"right,omitempty"`
	Summary  string   `json:"summary"`
}

type ReplayComparison struct {
	Left    ReplayScope           `json:"left"`
	Right   ReplayScope           `json:"right"`
	Diffs   []ReplayComparisonDiff `json:"diffs,omitempty"`
	Summary []string              `json:"summary,omitempty"`
}

type DebugSummary struct {
	WorkflowID        string   `json:"workflow_id,omitempty"`
	TaskGraphID       string   `json:"task_graph_id,omitempty"`
	FinalRuntimeState string   `json:"final_runtime_state,omitempty"`
	Goal              string   `json:"goal,omitempty"`
	PlanSummary       []string `json:"plan_summary,omitempty"`
	MemorySummary     []string `json:"memory_summary,omitempty"`
	ValidatorSummary  []string `json:"validator_summary,omitempty"`
	GovernanceSummary []string `json:"governance_summary,omitempty"`
	ChildWorkflows    []string `json:"child_workflows,omitempty"`
	Explanation       []string `json:"explanation,omitempty"`
}

