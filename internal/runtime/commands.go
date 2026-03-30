package runtime

type ApproveTaskCommand struct {
	RequestID       string   `json:"request_id"`
	ApprovalID      string   `json:"approval_id"`
	Actor           string   `json:"actor"`
	Roles           []string `json:"roles,omitempty"`
	Note            string   `json:"note,omitempty"`
	ExpectedVersion int64    `json:"expected_version,omitempty"`
}

type DenyTaskCommand struct {
	RequestID       string   `json:"request_id"`
	ApprovalID      string   `json:"approval_id"`
	Actor           string   `json:"actor"`
	Roles           []string `json:"roles,omitempty"`
	Note            string   `json:"note,omitempty"`
	ExpectedVersion int64    `json:"expected_version,omitempty"`
}

type ResumeFollowUpTaskCommand struct {
	RequestID       string   `json:"request_id"`
	GraphID         string   `json:"graph_id,omitempty"`
	TaskID          string   `json:"task_id"`
	Actor           string   `json:"actor"`
	Roles           []string `json:"roles,omitempty"`
	Note            string   `json:"note,omitempty"`
	ExpectedVersion int64    `json:"expected_version,omitempty"`
}

type RetryFailedFollowUpTaskCommand struct {
	RequestID       string   `json:"request_id"`
	GraphID         string   `json:"graph_id,omitempty"`
	TaskID          string   `json:"task_id"`
	Actor           string   `json:"actor"`
	Roles           []string `json:"roles,omitempty"`
	Note            string   `json:"note,omitempty"`
	ExpectedVersion int64    `json:"expected_version,omitempty"`
}

type ReevaluateTaskGraphCommand struct {
	RequestID string   `json:"request_id"`
	GraphID   string   `json:"graph_id"`
	Actor     string   `json:"actor"`
	Roles     []string `json:"roles,omitempty"`
	Note      string   `json:"note,omitempty"`
}

type ExecutionQuery struct {
	ExecutionID string `json:"execution_id,omitempty"`
	GraphID     string `json:"graph_id,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
}

type TaskGraphView struct {
	Snapshot        TaskGraphSnapshot      `json:"snapshot"`
	Executions      []TaskExecutionRecord  `json:"executions,omitempty"`
	PendingApproval *ApprovalStateRecord   `json:"pending_approval,omitempty"`
	Artifacts       []WorkflowArtifactMeta `json:"artifacts,omitempty"`
	Actions         []OperatorActionRecord `json:"actions,omitempty"`
}

type WorkflowArtifactMeta struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	WorkflowID string `json:"workflow_id"`
	TaskID     string `json:"task_id"`
	StorageRef string `json:"storage_ref,omitempty"`
	Summary    string `json:"summary,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

type TaskCommandResult struct {
	Action                OperatorActionRecord `json:"action"`
	GraphID               string               `json:"graph_id"`
	TaskID                string               `json:"task_id,omitempty"`
	ApprovalID            string               `json:"approval_id,omitempty"`
	ExecutionID           string               `json:"execution_id,omitempty"`
	Status                TaskQueueStatus      `json:"status,omitempty"`
	AutoResumeTried       bool                 `json:"auto_resume_tried,omitempty"`
	AutoResumeApplied     bool                 `json:"auto_resume_applied,omitempty"`
	FailureSummary        string               `json:"failure_summary,omitempty"`
	EnqueueResults        []WorkEnqueueResult  `json:"enqueue_results,omitempty"`
	EnqueuedWorkItemIDs   []string             `json:"enqueued_work_item_ids,omitempty"`
	EnqueuedWorkKinds     []WorkItemKind       `json:"enqueued_work_kinds,omitempty"`
	AsyncDispatchAccepted bool                 `json:"async_dispatch_accepted,omitempty"`
}

type WorkerPassResult struct {
	WorkerID             string                `json:"worker_id,omitempty"`
	ScannedGraphs        int                   `json:"scanned_graphs"`
	Reevaluated          []string              `json:"reevaluated,omitempty"`
	Executed             []TaskExecutionRecord `json:"executed,omitempty"`
	ResumedTasks         []string              `json:"resumed_tasks,omitempty"`
	SkippedTasks         []string              `json:"skipped_tasks,omitempty"`
	ClaimedWorkItemIDs   []string              `json:"claimed_work_item_ids,omitempty"`
	CompletedWorkItemIDs []string              `json:"completed_work_item_ids,omitempty"`
	FailedWorkItemIDs    []string              `json:"failed_work_item_ids,omitempty"`
	ReclaimedWorkItemIDs []string              `json:"reclaimed_work_item_ids,omitempty"`
	SchedulerWakeups     []string              `json:"scheduler_wakeups,omitempty"`
	CompletedAt          string                `json:"completed_at"`
	DryRun               bool                  `json:"dry_run"`
}
