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
	if !e.Kind.IsRequest() {
		return fmt.Errorf("agent envelope kind %q is not a request kind", e.Kind)
	}
	if err := e.Payload.ValidateForKind(e.Kind); err != nil {
		return err
	}
	return nil
}

func (r AgentResponse) Validate() error {
	if err := r.Metadata.Validate(); err != nil {
		return err
	}
	if r.Sender == "" || r.Recipient == "" {
		return errors.New("agent response sender and recipient are required")
	}
	if err := r.Task.Validate(); err != nil {
		return err
	}
	if r.StateRef.UserID == "" {
		return errors.New("agent response state_ref user_id is required")
	}
	if r.StateRef.Version == 0 {
		return errors.New("agent response state_ref version is required")
	}
	if !r.Kind.IsResult() {
		return fmt.Errorf("agent response kind %q is not a result kind", r.Kind)
	}
	if !r.Success {
		if r.Failure == nil {
			return errors.New("failed agent response requires failure details")
		}
		return nil
	}
	if err := r.Body.ValidateForKind(r.Kind); err != nil {
		return err
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

func (b AgentRequestBody) ValidateForKind(kind MessageKind) error {
	count := 0
	var matched bool
	if b.PlanRequest != nil {
		count++
		matched = kind == MessageKindPlanRequest
	}
	if b.MemorySyncRequest != nil {
		count++
		matched = matched || kind == MessageKindMemorySyncRequest
	}
	if b.ReportDraftRequest != nil {
		count++
		matched = matched || kind == MessageKindReportDraftRequest
	}
	if b.VerificationRequest != nil {
		count++
		matched = matched || kind == MessageKindVerificationRequest
	}
	if b.GovernanceEvaluationRequest != nil {
		count++
		matched = matched || kind == MessageKindGovernanceEvaluationRequest
	}
	if b.ReportFinalizeRequest != nil {
		count++
		matched = matched || kind == MessageKindReportFinalizeRequest
	}
	if count != 1 {
		return fmt.Errorf("agent request body must set exactly one typed field, got %d", count)
	}
	if !matched {
		return fmt.Errorf("agent request body does not match request kind %q", kind)
	}
	switch kind {
	case MessageKindPlanRequest:
		if b.PlanRequest == nil || len(b.PlanRequest.Evidence) == 0 {
			return errors.New("plan request requires evidence")
		}
	case MessageKindMemorySyncRequest:
		if b.MemorySyncRequest == nil || len(b.MemorySyncRequest.Evidence) == 0 {
			return errors.New("memory sync request requires evidence")
		}
	case MessageKindReportDraftRequest:
		if b.ReportDraftRequest == nil {
			return errors.New("report draft request payload is required")
		}
	case MessageKindVerificationRequest:
		if b.VerificationRequest == nil {
			return errors.New("verification request payload is required")
		}
		if err := b.VerificationRequest.Report.Validate(); err != nil {
			return err
		}
	case MessageKindGovernanceEvaluationRequest:
		if b.GovernanceEvaluationRequest == nil {
			return errors.New("governance evaluation request payload is required")
		}
		if err := b.GovernanceEvaluationRequest.Report.Validate(); err != nil {
			return err
		}
	case MessageKindReportFinalizeRequest:
		if b.ReportFinalizeRequest == nil {
			return errors.New("report finalize request payload is required")
		}
		if err := b.ReportFinalizeRequest.Draft.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (b AgentResultBody) ValidateForKind(kind MessageKind) error {
	count := 0
	var matched bool
	if b.PlanResult != nil {
		count++
		matched = kind == MessageKindPlanResult
	}
	if b.MemorySyncResult != nil {
		count++
		matched = matched || kind == MessageKindMemorySyncResult
	}
	if b.ReportDraftResult != nil {
		count++
		matched = matched || kind == MessageKindReportDraftResult
	}
	if b.VerificationResult != nil {
		count++
		matched = matched || kind == MessageKindVerificationResult
	}
	if b.GovernanceEvaluationResult != nil {
		count++
		matched = matched || kind == MessageKindGovernanceEvaluationResult
	}
	if b.ReportFinalizeResult != nil {
		count++
		matched = matched || kind == MessageKindReportFinalizeResult
	}
	if count != 1 {
		return fmt.Errorf("agent result body must set exactly one typed field, got %d", count)
	}
	if !matched {
		return fmt.Errorf("agent result body does not match result kind %q", kind)
	}
	switch kind {
	case MessageKindReportDraftResult:
		if b.ReportDraftResult == nil {
			return errors.New("report draft result payload is required")
		}
		if err := b.ReportDraftResult.Draft.Validate(); err != nil {
			return err
		}
	case MessageKindReportFinalizeResult:
		if b.ReportFinalizeResult == nil {
			return errors.New("report finalize result payload is required")
		}
		if err := b.ReportFinalizeResult.Report.Validate(); err != nil {
			return err
		}
	}
	return nil
}
