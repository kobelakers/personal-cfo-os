package runtime

import "time"

type WorkflowController interface {
	HandleFailure(current WorkflowExecutionState, category FailureCategory) (WorkflowExecutionState, RecoveryStrategy, error)
	PauseForApproval(current WorkflowExecutionState, pending HumanApprovalPending) (WorkflowExecutionState, error)
	Resume(checkpoint CheckpointRecord, token ResumeToken, now time.Time) (WorkflowExecutionState, error)
}

type CheckpointStore interface {
	Save(checkpoint CheckpointRecord) error
	Load(workflowID string, checkpointID string) (CheckpointRecord, error)
}

type ApprovalGate interface {
	RequestApproval(pending HumanApprovalPending) error
}

type RecoveryPlanner interface {
	StrategyFor(category FailureCategory) (RecoveryStrategy, error)
}
