package runtime

import (
	"errors"
	"fmt"
	"time"
)

type DefaultWorkflowController struct{}

func (DefaultWorkflowController) HandleFailure(current WorkflowExecutionState, category FailureCategory) (WorkflowExecutionState, RecoveryStrategy, error) {
	if !validWorkflowState(current) {
		return "", "", fmt.Errorf("invalid workflow state %q", current)
	}
	switch category {
	case FailureCategoryTransient, FailureCategoryTimeout:
		return WorkflowStateRetrying, RecoveryStrategyRetry, nil
	case FailureCategoryValidation:
		return WorkflowStateReplanning, RecoveryStrategyReplan, nil
	case FailureCategoryPolicy:
		return WorkflowStateWaitingApproval, RecoveryStrategyWaitForApproval, nil
	case FailureCategoryUnrecoverable:
		return WorkflowStateFailed, RecoveryStrategyAbort, nil
	default:
		return "", "", fmt.Errorf("unsupported failure category %q", category)
	}
}

func (DefaultWorkflowController) PauseForApproval(current WorkflowExecutionState, pending HumanApprovalPending) (WorkflowExecutionState, error) {
	if pending.ApprovalID == "" || pending.WorkflowID == "" || pending.RequestedAction == "" {
		return "", errors.New("human approval pending requires ids and requested action")
	}
	if current != WorkflowStateActing && current != WorkflowStateVerifying && current != WorkflowStateReplanning {
		return "", fmt.Errorf("workflow state %q cannot pause for approval", current)
	}
	return WorkflowStateWaitingApproval, nil
}

func (DefaultWorkflowController) Resume(checkpoint CheckpointRecord, token ResumeToken, now time.Time) (WorkflowExecutionState, error) {
	if err := checkpoint.Validate(); err != nil {
		return "", err
	}
	if err := token.Validate(); err != nil {
		return "", err
	}
	if token.WorkflowID != checkpoint.WorkflowID {
		return "", errors.New("resume token workflow does not match checkpoint")
	}
	if token.CheckpointID != checkpoint.ID {
		return "", errors.New("resume token checkpoint does not match checkpoint")
	}
	currentTime := now.UTC()
	if currentTime.After(token.ExpiresAt) {
		return "", errors.New("resume token has expired")
	}
	return checkpoint.ResumeState, nil
}

func (c CheckpointRecord) Validate() error {
	if c.ID == "" {
		return errors.New("checkpoint id is required")
	}
	if c.WorkflowID == "" {
		return errors.New("checkpoint workflow id is required")
	}
	if !validWorkflowState(c.State) || !validWorkflowState(c.ResumeState) {
		return errors.New("checkpoint states must be valid")
	}
	if c.CapturedAt.IsZero() {
		return errors.New("checkpoint captured_at is required")
	}
	return nil
}

func (r ResumeToken) Validate() error {
	if r.Token == "" {
		return errors.New("resume token is required")
	}
	if r.WorkflowID == "" || r.CheckpointID == "" {
		return errors.New("resume token must bind workflow and checkpoint")
	}
	if r.IssuedAt.IsZero() || r.ExpiresAt.IsZero() {
		return errors.New("resume token timestamps are required")
	}
	if r.ExpiresAt.Before(r.IssuedAt) {
		return errors.New("resume token expiry must be after issue time")
	}
	return nil
}

func validWorkflowState(state WorkflowExecutionState) bool {
	switch state {
	case WorkflowStateCreated, WorkflowStatePlanning, WorkflowStateActing, WorkflowStateVerifying, WorkflowStateReplanning, WorkflowStateWaitingApproval, WorkflowStatePaused, WorkflowStateRetrying, WorkflowStateCompleted, WorkflowStateFailed, WorkflowStateAborted:
		return true
	default:
		return false
	}
}
