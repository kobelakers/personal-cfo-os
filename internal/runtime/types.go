package runtime

import "time"

type WorkflowExecutionState string

const (
	WorkflowStateCreated         WorkflowExecutionState = "created"
	WorkflowStatePlanning        WorkflowExecutionState = "planning"
	WorkflowStateActing          WorkflowExecutionState = "acting"
	WorkflowStateVerifying       WorkflowExecutionState = "verifying"
	WorkflowStateReplanning      WorkflowExecutionState = "replanning"
	WorkflowStateWaitingApproval WorkflowExecutionState = "waiting_approval"
	WorkflowStatePaused          WorkflowExecutionState = "paused"
	WorkflowStateRetrying        WorkflowExecutionState = "retrying"
	WorkflowStateCompleted       WorkflowExecutionState = "completed"
	WorkflowStateFailed          WorkflowExecutionState = "failed"
	WorkflowStateAborted         WorkflowExecutionState = "aborted"
)

type FailureCategory string

const (
	FailureCategoryTransient     FailureCategory = "transient"
	FailureCategoryTimeout       FailureCategory = "timeout"
	FailureCategoryValidation    FailureCategory = "validation"
	FailureCategoryPolicy        FailureCategory = "policy"
	FailureCategoryUnrecoverable FailureCategory = "unrecoverable"
)

type RecoveryStrategy string

const (
	RecoveryStrategyRetry           RecoveryStrategy = "retry"
	RecoveryStrategyReplan          RecoveryStrategy = "replan"
	RecoveryStrategyWaitForApproval RecoveryStrategy = "wait_for_approval"
	RecoveryStrategyAbort           RecoveryStrategy = "abort"
	RecoveryStrategyEscalate        RecoveryStrategy = "escalate"
)

type CheckpointRecord struct {
	ID           string                 `json:"id"`
	WorkflowID   string                 `json:"workflow_id"`
	State        WorkflowExecutionState `json:"state"`
	ResumeState  WorkflowExecutionState `json:"resume_state"`
	StateVersion uint64                 `json:"state_version"`
	Summary      string                 `json:"summary"`
	CapturedAt   time.Time              `json:"captured_at"`
}

type ResumeToken struct {
	Token        string    `json:"token"`
	WorkflowID   string    `json:"workflow_id"`
	CheckpointID string    `json:"checkpoint_id"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type HumanApprovalPending struct {
	ApprovalID      string     `json:"approval_id"`
	WorkflowID      string     `json:"workflow_id"`
	RequestedAction string     `json:"requested_action"`
	RequiredRoles   []string   `json:"required_roles,omitempty"`
	RequestedAt     time.Time  `json:"requested_at"`
	Deadline        *time.Time `json:"deadline,omitempty"`
}

type ExecutionContext struct {
	WorkflowID    string `json:"workflow_id"`
	TaskID        string `json:"task_id"`
	CorrelationID string `json:"correlation_id"`
	Attempt       int    `json:"attempt"`
}
