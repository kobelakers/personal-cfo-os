package runtime

import (
	"errors"
)

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
