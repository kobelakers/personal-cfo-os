package protocol

import (
	"errors"
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/verification"
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
	if b.CashflowAnalysisRequest != nil {
		count++
		matched = matched || kind == MessageKindCashflowAnalysisRequest
	}
	if b.DebtAnalysisRequest != nil {
		count++
		matched = matched || kind == MessageKindDebtAnalysisRequest
	}
	if b.TaxAnalysisRequest != nil {
		count++
		matched = matched || kind == MessageKindTaxAnalysisRequest
	}
	if b.PortfolioAnalysisRequest != nil {
		count++
		matched = matched || kind == MessageKindPortfolioAnalysisRequest
	}
	if b.TaskGenerationRequest != nil {
		count++
		matched = matched || kind == MessageKindTaskGenerationRequest
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
		if err := b.ReportDraftRequest.Plan.Validate(); err != nil {
			return err
		}
		if len(b.ReportDraftRequest.BlockResults) == 0 {
			return errors.New("report draft request requires domain block results")
		}
		if b.ReportDraftRequest.StateDiff.ToVersion == 0 {
			return errors.New("report draft request requires state diff")
		}
		for _, result := range b.ReportDraftRequest.BlockResults {
			if err := result.Validate(); err != nil {
				return err
			}
		}
	case MessageKindVerificationRequest:
		if b.VerificationRequest == nil {
			return errors.New("verification request payload is required")
		}
		if err := b.VerificationRequest.Plan.Validate(); err != nil {
			return err
		}
		if len(b.VerificationRequest.BlockResults) == 0 {
			return errors.New("verification request requires block results")
		}
		for _, result := range b.VerificationRequest.BlockResults {
			if err := result.Validate(); err != nil {
				return err
			}
		}
		stage := b.VerificationRequest.Stage
		switch stage {
		case "", verification.VerificationStageFullReport:
			if len(b.VerificationRequest.BlockVerificationContexts) == 0 {
				return errors.New("verification request requires block verification contexts")
			}
			if len(b.VerificationRequest.FinalVerificationContext.SelectedEvidenceIDs) == 0 {
				return errors.New("verification request requires final verification context with selected evidence")
			}
			if err := b.VerificationRequest.Report.Validate(); err != nil {
				return err
			}
		case verification.VerificationStageAnalysisBlocks:
			if len(b.VerificationRequest.BlockVerificationContexts) == 0 {
				return errors.New("analysis-block verification requires block verification contexts")
			}
		case verification.VerificationStageGeneratedTasksAndFinal:
			if b.VerificationRequest.TaskGraph == nil {
				return errors.New("generated-task verification requires task graph")
			}
			if err := b.VerificationRequest.TaskGraph.Validate(); err != nil {
				return err
			}
			if err := b.VerificationRequest.Report.Validate(); err != nil {
				return err
			}
			if len(b.VerificationRequest.FinalVerificationContext.SelectedEvidenceIDs) == 0 {
				return errors.New("generated-task verification requires final verification context with selected evidence")
			}
		default:
			return fmt.Errorf("unsupported verification stage %q", stage)
		}
	case MessageKindGovernanceEvaluationRequest:
		if b.GovernanceEvaluationRequest == nil {
			return errors.New("governance evaluation request payload is required")
		}
		if err := b.GovernanceEvaluationRequest.Report.Validate(); err != nil {
			return err
		}
		if b.GovernanceEvaluationRequest.TaskGraph != nil {
			if err := b.GovernanceEvaluationRequest.TaskGraph.Validate(); err != nil {
				return err
			}
		}
		for _, task := range b.GovernanceEvaluationRequest.GeneratedTasks {
			if err := task.Validate(); err != nil {
				return err
			}
		}
	case MessageKindReportFinalizeRequest:
		if b.ReportFinalizeRequest == nil {
			return errors.New("report finalize request payload is required")
		}
		if err := b.ReportFinalizeRequest.Draft.Validate(); err != nil {
			return err
		}
	case MessageKindCashflowAnalysisRequest:
		if b.CashflowAnalysisRequest == nil {
			return errors.New("cashflow analysis request payload is required")
		}
		if err := b.CashflowAnalysisRequest.Block.Validate(); err != nil {
			return err
		}
		if b.CashflowAnalysisRequest.Block.AssignedRecipient != "cashflow_agent" {
			return fmt.Errorf("cashflow analysis request must target cashflow_agent, got %q", b.CashflowAnalysisRequest.Block.AssignedRecipient)
		}
		if len(b.CashflowAnalysisRequest.RelevantEvidence) == 0 {
			return errors.New("cashflow analysis request requires relevant evidence")
		}
	case MessageKindDebtAnalysisRequest:
		if b.DebtAnalysisRequest == nil {
			return errors.New("debt analysis request payload is required")
		}
		if err := b.DebtAnalysisRequest.Block.Validate(); err != nil {
			return err
		}
		if b.DebtAnalysisRequest.Block.AssignedRecipient != "debt_agent" {
			return fmt.Errorf("debt analysis request must target debt_agent, got %q", b.DebtAnalysisRequest.Block.AssignedRecipient)
		}
		if len(b.DebtAnalysisRequest.RelevantEvidence) == 0 {
			return errors.New("debt analysis request requires relevant evidence")
		}
	case MessageKindTaxAnalysisRequest:
		if b.TaxAnalysisRequest == nil {
			return errors.New("tax analysis request payload is required")
		}
		if err := b.TaxAnalysisRequest.Block.Validate(); err != nil {
			return err
		}
		if b.TaxAnalysisRequest.Block.AssignedRecipient != "tax_agent" {
			return fmt.Errorf("tax analysis request must target tax_agent, got %q", b.TaxAnalysisRequest.Block.AssignedRecipient)
		}
		if len(b.TaxAnalysisRequest.RelevantEvidence) == 0 {
			return errors.New("tax analysis request requires relevant evidence")
		}
	case MessageKindPortfolioAnalysisRequest:
		if b.PortfolioAnalysisRequest == nil {
			return errors.New("portfolio analysis request payload is required")
		}
		if err := b.PortfolioAnalysisRequest.Block.Validate(); err != nil {
			return err
		}
		if b.PortfolioAnalysisRequest.Block.AssignedRecipient != "portfolio_agent" {
			return fmt.Errorf("portfolio analysis request must target portfolio_agent, got %q", b.PortfolioAnalysisRequest.Block.AssignedRecipient)
		}
		if len(b.PortfolioAnalysisRequest.RelevantEvidence) == 0 {
			return errors.New("portfolio analysis request requires relevant evidence")
		}
	case MessageKindTaskGenerationRequest:
		if b.TaskGenerationRequest == nil {
			return errors.New("task generation request payload is required")
		}
		if err := b.TaskGenerationRequest.Plan.Validate(); err != nil {
			return err
		}
		if b.TaskGenerationRequest.StateDiff.ToVersion == 0 {
			return errors.New("task generation request requires state diff")
		}
		if len(b.TaskGenerationRequest.EventEvidence) == 0 {
			return errors.New("task generation request requires event evidence")
		}
		if len(b.TaskGenerationRequest.ValidatedBlockResults) == 0 {
			return errors.New("task generation request requires validated block results")
		}
		for _, result := range b.TaskGenerationRequest.ValidatedBlockResults {
			if err := result.Validate(); err != nil {
				return err
			}
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
	if b.CashflowAnalysisResult != nil {
		count++
		matched = matched || kind == MessageKindCashflowAnalysisResult
	}
	if b.DebtAnalysisResult != nil {
		count++
		matched = matched || kind == MessageKindDebtAnalysisResult
	}
	if b.TaxAnalysisResult != nil {
		count++
		matched = matched || kind == MessageKindTaxAnalysisResult
	}
	if b.PortfolioAnalysisResult != nil {
		count++
		matched = matched || kind == MessageKindPortfolioAnalysisResult
	}
	if b.TaskGenerationResult != nil {
		count++
		matched = matched || kind == MessageKindTaskGenerationResult
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
	case MessageKindCashflowAnalysisResult:
		if b.CashflowAnalysisResult == nil {
			return errors.New("cashflow analysis result payload is required")
		}
		if b.CashflowAnalysisResult.Result.BlockID == "" {
			return errors.New("cashflow analysis result requires block id")
		}
	case MessageKindDebtAnalysisResult:
		if b.DebtAnalysisResult == nil {
			return errors.New("debt analysis result payload is required")
		}
		if b.DebtAnalysisResult.Result.BlockID == "" {
			return errors.New("debt analysis result requires block id")
		}
	case MessageKindTaxAnalysisResult:
		if b.TaxAnalysisResult == nil {
			return errors.New("tax analysis result payload is required")
		}
		if b.TaxAnalysisResult.Result.BlockID == "" {
			return errors.New("tax analysis result requires block id")
		}
	case MessageKindPortfolioAnalysisResult:
		if b.PortfolioAnalysisResult == nil {
			return errors.New("portfolio analysis result payload is required")
		}
		if b.PortfolioAnalysisResult.Result.BlockID == "" {
			return errors.New("portfolio analysis result requires block id")
		}
	case MessageKindTaskGenerationResult:
		if b.TaskGenerationResult == nil {
			return errors.New("task generation result payload is required")
		}
		if err := b.TaskGenerationResult.TaskGraph.Validate(); err != nil {
			return err
		}
	}
	return nil
}
