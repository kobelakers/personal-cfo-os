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
	case FailureCategoryDeniedByOp, FailureCategoryProtocol, FailureCategoryUnrecoverable:
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
