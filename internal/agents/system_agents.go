package agents

import (
	"fmt"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/protocol"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type PlannerAgentHandler struct {
	Assembler contextview.ContextAssembler
	Planner   *planning.DeterministicPlanner
}

func (PlannerAgentHandler) Name() string      { return RecipientPlannerAgent }
func (PlannerAgentHandler) Recipient() string { return RecipientPlannerAgent }
func (PlannerAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindPlanRequest
}

func (a PlannerAgentHandler) Handle(_ AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.PlanRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientPlannerAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "plan request payload is required"},
		}
	}
	assembler := a.Assembler
	if assembler == nil {
		assembler = contextview.DefaultContextAssembler{}
	}
	planner := a.Planner
	if planner == nil {
		planner = &planning.DeterministicPlanner{}
	}
	slice, err := assembler.Assemble(envelope.Task, payload.CurrentState, payload.Memories, payload.Evidence, payload.PlanningView)
	if err != nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientPlannerAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureValidation, Message: "planning context assembly failed"},
			Cause:     err,
		}
	}
	plan := planner.CreatePlan(envelope.Task, slice, envelope.Metadata.CorrelationID)
	return AgentHandlerResult{
		Kind: protocol.MessageKindPlanResult,
		Body: protocol.AgentResultBody{
			PlanResult: &protocol.PlanResultPayload{Plan: plan},
		},
		EmittedWorkflowEvents: []protocol.WorkflowEvent{
			makeWorkflowEvent(envelope, protocol.WorkflowEventPlanCreated, "plan created by planner agent"),
		},
	}, nil
}

type MemoryStewardHandler struct {
	Service memory.WorkflowMemoryService
}

func (MemoryStewardHandler) Name() string      { return RecipientMemorySteward }
func (MemoryStewardHandler) Recipient() string { return RecipientMemorySteward }
func (MemoryStewardHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindMemorySyncRequest
}

func (a MemoryStewardHandler) Handle(handlerCtx AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.MemorySyncRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientMemorySteward,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "memory sync request payload is required"},
		}
	}
	service := a.Service
	if service.Now == nil {
		service.Now = handlerCtx.Now
	}
	var (
		result memory.WorkflowMemoryResult
		err    error
	)
	switch envelope.Task.UserIntentType {
	case taskspec.UserIntentMonthlyReview:
		result, err = service.SyncMonthlyReview(handlerCtx.Context, envelope.Task, envelope.Metadata.CorrelationID, payload.CurrentState, payload.Evidence)
	case taskspec.UserIntentDebtVsInvest:
		result, err = service.SyncDebtDecision(handlerCtx.Context, envelope.Task, envelope.Metadata.CorrelationID, payload.CurrentState, payload.Evidence, payload.ConclusionHint)
	default:
		err = fmt.Errorf("unsupported intent %q for memory sync", envelope.Task.UserIntentType)
	}
	if err != nil {
		category := protocol.AgentFailureUnrecoverable
		if memory.IsPolicyDenied(err) {
			category = protocol.AgentFailurePolicy
		}
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientMemorySteward,
			Kind:      envelope.Kind,
			Failure: protocol.AgentFailure{
				Category: category,
				Message:  "memory steward sync failed",
			},
			Cause: err,
		}
	}
	return AgentHandlerResult{
		Kind:             protocol.MessageKindMemorySyncResult,
		ProducedMemories: result.GeneratedIDs,
		Body: protocol.AgentResultBody{
			MemorySyncResult: &protocol.MemorySyncResultPayload{
				Result:    result,
				AuditRefs: result.GeneratedIDs,
			},
		},
	}, nil
}

type ReportDraftAgentHandler struct {
	Service reporting.Service
}

func (ReportDraftAgentHandler) Name() string      { return RecipientReportAgent }
func (ReportDraftAgentHandler) Recipient() string { return RecipientReportAgent }

func (ReportDraftAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindReportDraftRequest
}

func (a ReportDraftAgentHandler) Handle(_ AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.ReportDraftRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientReportAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "report draft request payload is required"},
		}
	}
	draft, err := a.Service.Draft(envelope.Task, envelope.Metadata.CorrelationID, payload.CurrentState, payload.Evidence)
	if err != nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientReportAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureValidation, Message: "report draft generation failed"},
			Cause:     err,
		}
	}
	return AgentHandlerResult{
		Kind: protocol.MessageKindReportDraftResult,
		Body: protocol.AgentResultBody{
			ReportDraftResult: &protocol.ReportDraftResultPayload{Draft: draft},
		},
	}, nil
}

type ReportFinalizeAgentHandler struct {
	Service reporting.Service
}

func (ReportFinalizeAgentHandler) Name() string      { return RecipientReportAgent }
func (ReportFinalizeAgentHandler) Recipient() string { return RecipientReportAgent }
func (ReportFinalizeAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindReportFinalizeRequest
}

func (a ReportFinalizeAgentHandler) Handle(_ AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.ReportFinalizeRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientReportAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "report finalize request payload is required"},
		}
	}
	report, artifacts, err := a.Service.Finalize(envelope.Metadata.CorrelationID, envelope.Task.ID, payload.Draft, payload.DisclosureDecision)
	if err != nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientReportAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailurePolicy, Message: "report finalization denied by disclosure policy"},
			Cause:     err,
		}
	}
	return AgentHandlerResult{
		Kind:              protocol.MessageKindReportFinalizeResult,
		ProducedArtifacts: artifacts,
		Body: protocol.AgentResultBody{
			ReportFinalizeResult: &protocol.ReportFinalizeResultPayload{
				Report:    report,
				Artifacts: artifacts,
			},
		},
		EmittedWorkflowEvents: []protocol.WorkflowEvent{
			makeWorkflowEvent(envelope, protocol.WorkflowEventReportReady, "final report artifact generated"),
		},
	}, nil
}

type VerificationAgentHandler struct {
	Pipeline verification.Pipeline
}

func (VerificationAgentHandler) Name() string      { return RecipientVerificationAgent }
func (VerificationAgentHandler) Recipient() string { return RecipientVerificationAgent }
func (VerificationAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindVerificationRequest
}

func (a VerificationAgentHandler) Handle(handlerCtx AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.VerificationRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientVerificationAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "verification request payload is required"},
		}
	}
	var (
		result verification.PipelineResult
		err    error
	)
	switch envelope.Task.UserIntentType {
	case taskspec.UserIntentMonthlyReview:
		if payload.Report.MonthlyReview == nil {
			return AgentHandlerResult{}, &AgentExecutionError{
				Recipient: RecipientVerificationAgent,
				Kind:      envelope.Kind,
				Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "monthly review verification requires monthly review report payload"},
			}
		}
		result, err = a.Pipeline.VerifyMonthlyReview(handlerCtx.Context, envelope.Task, payload.CurrentState, payload.Evidence, *payload.Report.MonthlyReview)
	case taskspec.UserIntentDebtVsInvest:
		if payload.Report.DebtDecision == nil {
			return AgentHandlerResult{}, &AgentExecutionError{
				Recipient: RecipientVerificationAgent,
				Kind:      envelope.Kind,
				Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "debt decision verification requires debt decision report payload"},
			}
		}
		result, err = a.Pipeline.VerifyDebtDecision(handlerCtx.Context, envelope.Task, payload.CurrentState, payload.Evidence, *payload.Report.DebtDecision)
	default:
		err = fmt.Errorf("unsupported verification intent %q", envelope.Task.UserIntentType)
	}
	if err != nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientVerificationAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureValidation, Message: "verification pipeline failed"},
			Cause:     err,
		}
	}
	events := []protocol.WorkflowEvent{}
	if verification.NeedsReplan(result.Results) {
		events = append(events, makeWorkflowEvent(envelope, protocol.WorkflowEventVerificationFailed, "verification requested replanning"))
	}
	return AgentHandlerResult{
		Kind:                protocol.MessageKindVerificationResult,
		VerificationResults: result.Results,
		Body: protocol.AgentResultBody{
			VerificationResult: &protocol.VerificationResultPayload{Result: result},
		},
		EmittedWorkflowEvents: events,
	}, nil
}

type GovernanceAgentHandler struct {
	Service governance.ApprovalService
}

func (GovernanceAgentHandler) Name() string      { return RecipientGovernanceAgent }
func (GovernanceAgentHandler) Recipient() string { return RecipientGovernanceAgent }
func (GovernanceAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindGovernanceEvaluationRequest
}

func (a GovernanceAgentHandler) Handle(_ AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.GovernanceEvaluationRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientGovernanceAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "governance evaluation request payload is required"},
		}
	}
	approval, err := a.Service.EvaluateAction(payload.CurrentState, envelope.Metadata.CorrelationID, payload.RequestedAction, envelope.Task.ID, payload.Actor, payload.ActorRoles, payload.ForceApproval)
	if err != nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientGovernanceAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailurePolicy, Message: "approval evaluation failed"},
			Cause:     err,
		}
	}
	disclosure, err := a.Service.EvaluateReport(envelope.Metadata.CorrelationID, RecipientReportAgent, payload.Audience, payload.ContainsPII)
	if err != nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientGovernanceAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailurePolicy, Message: "report disclosure evaluation failed"},
			Cause:     err,
		}
	}
	events := []protocol.WorkflowEvent{}
	if approval.Decision != nil && approval.Decision.Outcome == governance.PolicyDecisionRequireApproval {
		events = append(events, makeWorkflowEvent(envelope, protocol.WorkflowEventApprovalRequired, "governance requires human approval"))
	}
	return AgentHandlerResult{
		Kind: protocol.MessageKindGovernanceEvaluationResult,
		Body: protocol.AgentResultBody{
			GovernanceEvaluationResult: &protocol.GovernanceEvaluationResultPayload{
				Approval:          approval,
				Disclosure:        disclosure,
				RedactionRequired: disclosure.Decision.Outcome == governance.PolicyDecisionRedact,
			},
		},
		EmittedWorkflowEvents: events,
	}, nil
}

func makeWorkflowEvent(envelope protocol.AgentEnvelope, eventType protocol.WorkflowEventType, summary string) protocol.WorkflowEvent {
	now := envelope.Metadata.EmittedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return protocol.WorkflowEvent{
		Metadata: protocol.ProtocolMetadata{
			MessageID:     envelope.Metadata.MessageID + "-" + string(eventType),
			CorrelationID: envelope.Metadata.CorrelationID,
			CausationID:   envelope.Metadata.MessageID,
			EmittedAt:     now,
		},
		WorkflowID: envelope.Metadata.CorrelationID,
		TaskID:     envelope.Task.ID,
		Type:       eventType,
		Summary:    summary,
		StateRef:   envelope.StateRef,
	}
}
