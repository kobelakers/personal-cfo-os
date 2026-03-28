package protocol

import (
	"errors"
	"fmt"
)

func (m ProtocolMetadata) Validate() error {
	if m.MessageID == "" {
		return errors.New("protocol metadata message_id is required")
	}
	if m.CorrelationID == "" {
		return errors.New("protocol metadata correlation_id is required")
	}
	if m.EmittedAt.IsZero() {
		return errors.New("protocol metadata emitted_at is required")
	}
	return nil
}

func (e AgentEnvelope) Validate() error {
	if err := e.Metadata.Validate(); err != nil {
		return err
	}
	if e.Sender == "" || e.Recipient == "" {
		return errors.New("agent envelope sender and recipient are required")
	}
	if err := e.Task.Validate(); err != nil {
		return err
	}
	if e.StateRef.UserID == "" {
		return errors.New("state_ref user_id is required")
	}
	if e.StateRef.Version == 0 {
		return errors.New("state_ref version is required")
	}
	return nil
}

func (e WorkflowEvent) Validate() error {
	if err := e.Metadata.Validate(); err != nil {
		return err
	}
	if e.WorkflowID == "" {
		return errors.New("workflow event workflow_id is required")
	}
	if e.TaskID == "" {
		return errors.New("workflow event task_id is required")
	}
	switch e.Type {
	case WorkflowEventPlanCreated, WorkflowEventStateUpdated, WorkflowEventToolCalled, WorkflowEventApprovalRequired, WorkflowEventVerificationFailed, WorkflowEventReportReady:
		return nil
	default:
		return fmt.Errorf("invalid workflow event type %q", e.Type)
	}
}
