package runtime

import "time"

const ReplayProjectionSchemaVersion = 1

type ReplayScopeKind string

const (
	ReplayScopeWorkflow  ReplayScopeKind = "workflow"
	ReplayScopeTaskGraph ReplayScopeKind = "task_graph"
	ReplayScopeTask      ReplayScopeKind = "task"
	ReplayScopeExecution ReplayScopeKind = "execution"
	ReplayScopeApproval  ReplayScopeKind = "approval"
)

type ReplayProjectionStatus string

const (
	ReplayProjectionStatusComplete ReplayProjectionStatus = "complete"
	ReplayProjectionStatusPartial  ReplayProjectionStatus = "partial"
	ReplayProjectionStatusStale    ReplayProjectionStatus = "stale"
)

type WorkflowRunRecord struct {
	WorkflowID        string                 `json:"workflow_id"`
	TaskID            string                 `json:"task_id"`
	Intent            string                 `json:"intent"`
	RuntimeState      WorkflowExecutionState `json:"runtime_state"`
	FailureCategory   FailureCategory        `json:"failure_category,omitempty"`
	FailureSummary    string                 `json:"failure_summary,omitempty"`
	ApprovalID        string                 `json:"approval_id,omitempty"`
	CheckpointID      string                 `json:"checkpoint_id,omitempty"`
	ResumeToken       string                 `json:"resume_token,omitempty"`
	TaskGraphID       string                 `json:"task_graph_id,omitempty"`
	RootCorrelationID string                 `json:"root_correlation_id,omitempty"`
	Summary           string                 `json:"summary,omitempty"`
	StartedAt         time.Time              `json:"started_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	RecordJSON        string                 `json:"record_json,omitempty"`
}

type ReplayProjectionScope struct {
	ScopeKind ReplayScopeKind `json:"scope_kind"`
	ScopeID   string          `json:"scope_id"`
}

type WorkflowReplayProjection struct {
	WorkflowID          string                 `json:"workflow_id"`
	TaskID              string                 `json:"task_id,omitempty"`
	Intent              string                 `json:"intent,omitempty"`
	RuntimeState        WorkflowExecutionState `json:"runtime_state"`
	FailureCategory     FailureCategory        `json:"failure_category,omitempty"`
	ApprovalID          string                 `json:"approval_id,omitempty"`
	BundleArtifactID    string                 `json:"bundle_artifact_id,omitempty"`
	SummaryArtifactID   string                 `json:"summary_artifact_id,omitempty"`
	ProjectionStatus    ReplayProjectionStatus `json:"projection_status"`
	SchemaVersion       int                    `json:"schema_version"`
	DegradationReasons  []string               `json:"degradation_reasons,omitempty"`
	SummaryJSON         string                 `json:"summary_json,omitempty"`
	ExplanationJSON     string                 `json:"explanation_json,omitempty"`
	CompareInputJSON    string                 `json:"compare_input_json,omitempty"`
	UpdatedAt           time.Time              `json:"updated_at"`
	ProjectionFreshness time.Time              `json:"projection_freshness"`
}

type TaskGraphReplayProjection struct {
	GraphID             string                 `json:"graph_id"`
	ParentWorkflowID    string                 `json:"parent_workflow_id,omitempty"`
	ParentTaskID        string                 `json:"parent_task_id,omitempty"`
	RuntimeState        WorkflowExecutionState `json:"runtime_state"`
	PendingApprovalID   string                 `json:"pending_approval_id,omitempty"`
	BundleArtifactID    string                 `json:"bundle_artifact_id,omitempty"`
	SummaryArtifactID   string                 `json:"summary_artifact_id,omitempty"`
	ProjectionStatus    ReplayProjectionStatus `json:"projection_status"`
	SchemaVersion       int                    `json:"schema_version"`
	DegradationReasons  []string               `json:"degradation_reasons,omitempty"`
	SummaryJSON         string                 `json:"summary_json,omitempty"`
	ExplanationJSON     string                 `json:"explanation_json,omitempty"`
	CompareInputJSON    string                 `json:"compare_input_json,omitempty"`
	UpdatedAt           time.Time              `json:"updated_at"`
	ProjectionFreshness time.Time              `json:"projection_freshness"`
}

type ProvenanceNodeRecord struct {
	ScopeKind      ReplayScopeKind `json:"scope_kind"`
	ScopeID        string          `json:"scope_id"`
	NodeID         string          `json:"node_id"`
	NodeType       string          `json:"node_type"`
	RefID          string          `json:"ref_id,omitempty"`
	Label          string          `json:"label"`
	Summary        string          `json:"summary,omitempty"`
	AttributesJSON string          `json:"attributes_json,omitempty"`
}

type ProvenanceEdgeRecord struct {
	ScopeKind      ReplayScopeKind `json:"scope_kind"`
	ScopeID        string          `json:"scope_id"`
	EdgeID         string          `json:"edge_id"`
	FromNodeID     string          `json:"from_node_id"`
	ToNodeID       string          `json:"to_node_id"`
	EdgeType       string          `json:"edge_type"`
	Reason         string          `json:"reason,omitempty"`
	AttributesJSON string          `json:"attributes_json,omitempty"`
}

type ExecutionAttributionRecord struct {
	ScopeKind      ReplayScopeKind `json:"scope_kind"`
	ScopeID        string          `json:"scope_id"`
	ExecutionID    string          `json:"execution_id"`
	Category       string          `json:"category"`
	Summary        string          `json:"summary"`
	SourceRefsJSON string          `json:"source_refs_json,omitempty"`
	DetailsJSON    string          `json:"details_json,omitempty"`
}

type FailureAttributionRecord struct {
	ScopeKind       ReplayScopeKind `json:"scope_kind"`
	ScopeID         string          `json:"scope_id"`
	AttributionID   string          `json:"attribution_id"`
	FailureCategory string          `json:"failure_category"`
	ReasonCode      string          `json:"reason_code,omitempty"`
	Summary         string          `json:"summary"`
	RelatedKind     string          `json:"related_kind,omitempty"`
	RelatedID       string          `json:"related_id,omitempty"`
	SourceRefsJSON  string          `json:"source_refs_json,omitempty"`
	DetailsJSON     string          `json:"details_json,omitempty"`
}

type ReplayProjectionBuildRecord struct {
	ScopeKind          ReplayScopeKind       `json:"scope_kind"`
	ScopeID            string                `json:"scope_id"`
	SchemaVersion      int                   `json:"schema_version"`
	Status             ReplayProjectionStatus `json:"status"`
	DegradationReasons []string              `json:"degradation_reasons,omitempty"`
	BuiltAt            time.Time             `json:"built_at"`
	SourceEventCount   int                   `json:"source_event_count"`
	SourceArtifactCount int                  `json:"source_artifact_count"`
}

type WorkflowRunStore interface {
	Save(record WorkflowRunRecord) error
	Load(workflowID string) (WorkflowRunRecord, bool, error)
	List() ([]WorkflowRunRecord, error)
}

type ReplayProjectionStore interface {
	SaveWorkflowProjection(record WorkflowReplayProjection) error
	SaveTaskGraphProjection(record TaskGraphReplayProjection) error
	SaveBuild(record ReplayProjectionBuildRecord) error
	ReplaceProvenance(scope ReplayProjectionScope, nodes []ProvenanceNodeRecord, edges []ProvenanceEdgeRecord) error
	ReplaceExecutionAttributions(scope ReplayProjectionScope, records []ExecutionAttributionRecord) error
	ReplaceFailureAttributions(scope ReplayProjectionScope, records []FailureAttributionRecord) error
}

type ReplayProjectionQueryStore interface {
	LoadWorkflowProjection(workflowID string) (WorkflowReplayProjection, bool, error)
	LoadTaskGraphProjection(graphID string) (TaskGraphReplayProjection, bool, error)
	LoadBuild(scope ReplayProjectionScope) (ReplayProjectionBuildRecord, bool, error)
	ListProvenance(scope ReplayProjectionScope) ([]ProvenanceNodeRecord, []ProvenanceEdgeRecord, error)
	ListExecutionAttributions(scope ReplayProjectionScope) ([]ExecutionAttributionRecord, error)
	ListFailureAttributions(scope ReplayProjectionScope) ([]FailureAttributionRecord, error)
}

