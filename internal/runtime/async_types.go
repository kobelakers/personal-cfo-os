package runtime

import "time"

type WorkerID string

type WorkerRole string

const (
	WorkerRoleWorker    WorkerRole = "worker"
	WorkerRoleScheduler WorkerRole = "scheduler"
	WorkerRoleAll       WorkerRole = "all"
)

type WorkerRegistration struct {
	WorkerID       WorkerID   `json:"worker_id"`
	Role           WorkerRole `json:"role"`
	BackendProfile string     `json:"backend_profile,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	LastHeartbeat  time.Time  `json:"last_heartbeat"`
}

type WorkItemKind string

const (
	WorkItemKindReevaluateTaskGraph      WorkItemKind = "reevaluate_task_graph"
	WorkItemKindExecuteReadyTask         WorkItemKind = "execute_ready_task"
	WorkItemKindResumeApprovedCheckpoint WorkItemKind = "resume_approved_checkpoint"
	WorkItemKindRetryFailedExecution     WorkItemKind = "retry_failed_execution"
	WorkItemKindSchedulerWakeup          WorkItemKind = "scheduler_wakeup"
)

type WorkItemStatus string

const (
	WorkItemStatusQueued    WorkItemStatus = "queued"
	WorkItemStatusClaimed   WorkItemStatus = "claimed"
	WorkItemStatusCompleted WorkItemStatus = "completed"
	WorkItemStatusFailed    WorkItemStatus = "failed"
	WorkItemStatusAbandoned WorkItemStatus = "abandoned"
)

type SchedulerWakeupKind string

const (
	SchedulerWakeupDueWindow  SchedulerWakeupKind = "due_window"
	SchedulerWakeupDependency SchedulerWakeupKind = "dependency_reevaluation"
	SchedulerWakeupCapability SchedulerWakeupKind = "capability_reevaluation"
	SchedulerWakeupApproval   SchedulerWakeupKind = "approval_resume"
	SchedulerWakeupRetry      SchedulerWakeupKind = "retry_backoff"
	SchedulerWakeupOperator   SchedulerWakeupKind = "operator_reevaluate"
)

type WorkItem struct {
	ID                string              `json:"id"`
	Kind              WorkItemKind        `json:"kind"`
	Status            WorkItemStatus      `json:"status"`
	DedupeKey         string              `json:"dedupe_key,omitempty"`
	GraphID           string              `json:"graph_id,omitempty"`
	TaskID            string              `json:"task_id,omitempty"`
	ExecutionID       string              `json:"execution_id,omitempty"`
	ApprovalID        string              `json:"approval_id,omitempty"`
	CheckpointID      string              `json:"checkpoint_id,omitempty"`
	WorkflowID        string              `json:"workflow_id,omitempty"`
	AvailableAt       time.Time           `json:"available_at"`
	ClaimedAt         *time.Time          `json:"claimed_at,omitempty"`
	CompletedAt       *time.Time          `json:"completed_at,omitempty"`
	FailedAt          *time.Time          `json:"failed_at,omitempty"`
	LastUpdatedAt     time.Time           `json:"last_updated_at"`
	Reason            string              `json:"reason,omitempty"`
	WakeupKind        SchedulerWakeupKind `json:"wakeup_kind,omitempty"`
	RetryNotBefore    *time.Time          `json:"retry_not_before,omitempty"`
	AttemptCount      int                 `json:"attempt_count"`
	LeaseID           string              `json:"lease_id,omitempty"`
	FencingToken      int64               `json:"fencing_token,omitempty"`
	ClaimToken        string              `json:"claim_token,omitempty"`
	ClaimedByWorkerID WorkerID            `json:"claimed_by_worker_id,omitempty"`
	LeaseExpiresAt    *time.Time          `json:"lease_expires_at,omitempty"`
}

type WorkClaim struct {
	WorkItem       WorkItem  `json:"work_item"`
	WorkerID       WorkerID  `json:"worker_id"`
	LeaseID        string    `json:"lease_id"`
	ClaimToken     string    `json:"claim_token"`
	FencingToken   int64     `json:"fencing_token"`
	ClaimedAt      time.Time `json:"claimed_at"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
}

type WorkLease struct {
	WorkItemID     string    `json:"work_item_id"`
	WorkerID       WorkerID  `json:"worker_id"`
	LeaseID        string    `json:"lease_id"`
	FencingToken   int64     `json:"fencing_token"`
	ClaimToken     string    `json:"claim_token"`
	ClaimedAt      time.Time `json:"claimed_at"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
}

type LeaseHeartbeat struct {
	WorkItemID     string    `json:"work_item_id"`
	WorkerID       WorkerID  `json:"worker_id"`
	LeaseID        string    `json:"lease_id"`
	FencingToken   int64     `json:"fencing_token"`
	RecordedAt     time.Time `json:"recorded_at"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
}

type ExecutionAttemptStatus string

const (
	ExecutionAttemptStatusStarted   ExecutionAttemptStatus = "started"
	ExecutionAttemptStatusSucceeded ExecutionAttemptStatus = "succeeded"
	ExecutionAttemptStatusFailed    ExecutionAttemptStatus = "failed"
	ExecutionAttemptStatusAbandoned ExecutionAttemptStatus = "abandoned"
	ExecutionAttemptStatusReclaimed ExecutionAttemptStatus = "reclaimed"
)

type ExecutionAttempt struct {
	AttemptID           string                 `json:"attempt_id"`
	WorkItemID          string                 `json:"work_item_id"`
	WorkItemKind        WorkItemKind           `json:"work_item_kind"`
	GraphID             string                 `json:"graph_id,omitempty"`
	TaskID              string                 `json:"task_id,omitempty"`
	ExecutionID         string                 `json:"execution_id,omitempty"`
	ApprovalID          string                 `json:"approval_id,omitempty"`
	WorkerID            WorkerID               `json:"worker_id"`
	LeaseID             string                 `json:"lease_id"`
	FencingToken        int64                  `json:"fencing_token"`
	Status              ExecutionAttemptStatus `json:"status"`
	FailureCategory     FailureCategory        `json:"failure_category,omitempty"`
	FailureSummary      string                 `json:"failure_summary,omitempty"`
	StartedAt           time.Time              `json:"started_at"`
	FinishedAt          *time.Time             `json:"finished_at,omitempty"`
	CheckpointID        string                 `json:"checkpoint_id,omitempty"`
	ProducedArtifactIDs []string               `json:"produced_artifact_ids,omitempty"`
}

type ClaimResult struct {
	Claims []WorkClaim `json:"claims,omitempty"`
}

type LeaseReclaimResult struct {
	WorkItemID   string    `json:"work_item_id"`
	LeaseID      string    `json:"lease_id"`
	WorkerID     WorkerID  `json:"worker_id"`
	FencingToken int64     `json:"fencing_token"`
	Reason       string    `json:"reason"`
	ReclaimedAt  time.Time `json:"reclaimed_at"`
}

type AsyncDispatchResult struct {
	WorkItemIDs []string       `json:"work_item_ids,omitempty"`
	WorkKinds   []WorkItemKind `json:"work_kinds,omitempty"`
	Accepted    bool           `json:"accepted"`
}

type RetryBackoffPolicy struct {
	BaseDelay time.Duration `json:"base_delay"`
	MaxDelay  time.Duration `json:"max_delay"`
}

type DueWindowPolicy struct {
	LeadTime time.Duration `json:"lead_time"`
}

type SchedulerWakeup struct {
	ID          string              `json:"id"`
	GraphID     string              `json:"graph_id,omitempty"`
	TaskID      string              `json:"task_id,omitempty"`
	ExecutionID string              `json:"execution_id,omitempty"`
	ApprovalID  string              `json:"approval_id,omitempty"`
	Kind        SchedulerWakeupKind `json:"kind"`
	AvailableAt time.Time           `json:"available_at"`
	Reason      string              `json:"reason,omitempty"`
}

type ApprovalResumeWakeup struct {
	ApprovalID   string    `json:"approval_id"`
	GraphID      string    `json:"graph_id"`
	TaskID       string    `json:"task_id"`
	ExecutionID  string    `json:"execution_id,omitempty"`
	CheckpointID string    `json:"checkpoint_id,omitempty"`
	AvailableAt  time.Time `json:"available_at"`
}

type DependencyReevaluation struct {
	GraphID     string    `json:"graph_id"`
	TriggeredBy string    `json:"triggered_by"`
	AvailableAt time.Time `json:"available_at"`
	Reason      string    `json:"reason"`
}

type CapabilityReevaluation struct {
	GraphID     string    `json:"graph_id"`
	TaskID      string    `json:"task_id,omitempty"`
	AvailableAt time.Time `json:"available_at"`
	Reason      string    `json:"reason"`
}

type TaskActivationDecision struct {
	GraphID           string         `json:"graph_id"`
	EnqueuedWorkKinds []WorkItemKind `json:"enqueued_work_kinds,omitempty"`
	EnqueuedWorkIDs   []string       `json:"enqueued_work_ids,omitempty"`
	ReadyTaskIDs      []string       `json:"ready_task_ids,omitempty"`
	EvaluatedAt       time.Time      `json:"evaluated_at"`
	Reason            string         `json:"reason,omitempty"`
}

type FenceValidation struct {
	WorkItemID   string   `json:"work_item_id"`
	LeaseID      string   `json:"lease_id"`
	FencingToken int64    `json:"fencing_token"`
	WorkerID     WorkerID `json:"worker_id"`
}

type FenceValidator interface {
	ValidateFence(fence FenceValidation) error
}

type WorkQueueStore interface {
	Enqueue(item WorkItem) error
	ClaimReady(workerID WorkerID, limit int, now time.Time, leaseTTL time.Duration) ([]WorkClaim, error)
	Heartbeat(heartbeat LeaseHeartbeat) error
	Complete(fence FenceValidation, now time.Time) error
	Fail(fence FenceValidation, summary string, now time.Time) error
	Requeue(fence FenceValidation, nextAvailableAt time.Time, reason string, now time.Time) error
	ReclaimExpired(now time.Time) ([]LeaseReclaimResult, error)
	Load(workItemID string) (WorkItem, bool, error)
	ListByGraph(graphID string) ([]WorkItem, error)
	ValidateFence(fence FenceValidation) error
}

type WorkAttemptStore interface {
	SaveAttempt(attempt ExecutionAttempt) error
	UpdateAttempt(attempt ExecutionAttempt) error
	ListAttempts(workItemID string) ([]ExecutionAttempt, error)
}

type WorkerRegistryStore interface {
	Register(worker WorkerRegistration) error
	Heartbeat(workerID WorkerID, now time.Time) error
	Load(workerID WorkerID) (WorkerRegistration, bool, error)
	List() ([]WorkerRegistration, error)
}

type SchedulerStore interface {
	SaveWakeup(wakeup SchedulerWakeup) error
	ListDueWakeups(now time.Time) ([]SchedulerWakeup, error)
	MarkWakeupDispatched(id string, now time.Time) error
}

type AsyncReplayEventDetails struct {
	WorkerID             string   `json:"worker_id,omitempty"`
	WorkItemID           string   `json:"work_item_id,omitempty"`
	WorkItemKind         string   `json:"work_item_kind,omitempty"`
	LeaseID              string   `json:"lease_id,omitempty"`
	FencingToken         int64    `json:"fencing_token,omitempty"`
	AttemptID            string   `json:"attempt_id,omitempty"`
	HeartbeatTimestamps  []string `json:"heartbeat_timestamps,omitempty"`
	ReclaimReason        string   `json:"reclaim_reason,omitempty"`
	SchedulerDecision    string   `json:"scheduler_decision,omitempty"`
	RetryBackoffDecision string   `json:"retry_backoff_decision,omitempty"`
	StoreBackendProfile  string   `json:"store_backend_profile,omitempty"`
	GraphID              string   `json:"graph_id,omitempty"`
	TaskID               string   `json:"task_id,omitempty"`
	ExecutionID          string   `json:"execution_id,omitempty"`
	ApprovalID           string   `json:"approval_id,omitempty"`
	ReadyTaskIDs         []string `json:"ready_task_ids,omitempty"`
	ExecutedTaskIDs      []string `json:"executed_task_ids,omitempty"`
	RetryOfExecutionID   string   `json:"retry_of_execution_id,omitempty"`
}

type operatorAsyncDispatchReplayDetails struct {
	WorkerAction        string `json:"worker_action"`
	WorkItemID          string `json:"work_item_id"`
	WorkItemKind        string `json:"work_item_kind"`
	SchedulerDecision   string `json:"scheduler_decision,omitempty"`
	StoreBackendProfile string `json:"store_backend_profile,omitempty"`
}
