package agents

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/protocol"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type LocalDispatcherOptions struct {
	Registry   AgentRegistry
	Executor   AgentExecutor
	AgentTrace *observability.AgentTraceLog
	EventLog   *observability.EventLog
	Now        func() time.Time
}

type LocalAgentDispatcher struct {
	Registry   AgentRegistry
	Executor   AgentExecutor
	AgentTrace *observability.AgentTraceLog
	EventLog   *observability.EventLog
	Now        func() time.Time
}

type SystemStepBusOptions struct {
	Dispatcher AgentDispatcher
	Now        func() time.Time
}

type LocalSystemStepBus struct {
	Dispatcher AgentDispatcher
	Now        func() time.Time
}

func NewLocalAgentDispatcher(options LocalDispatcherOptions) *LocalAgentDispatcher {
	registry := options.Registry
	if registry == nil {
		registry = NewInMemoryAgentRegistry()
	}
	executor := options.Executor
	if executor == nil {
		executor = LocalAgentExecutor{}
	}
	return &LocalAgentDispatcher{
		Registry:   registry,
		Executor:   executor,
		AgentTrace: options.AgentTrace,
		EventLog:   options.EventLog,
		Now:        options.Now,
	}
}

func NewLocalSystemStepBus(options SystemStepBusOptions) *LocalSystemStepBus {
	dispatcher := options.Dispatcher
	if dispatcher == nil {
		dispatcher = NewLocalAgentDispatcher(LocalDispatcherOptions{Now: options.Now})
	}
	return &LocalSystemStepBus{
		Dispatcher: dispatcher,
		Now:        options.Now,
	}
}

func (d *LocalAgentDispatcher) Dispatch(ctx context.Context, envelope protocol.AgentEnvelope) (AgentDispatchResult, error) {
	if err := envelope.Validate(); err != nil {
		execErr := &AgentExecutionError{
			Recipient: envelope.Recipient,
			Kind:      envelope.Kind,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureBadPayload,
				Message:  "invalid agent envelope",
			},
			Cause: err,
		}
		return AgentDispatchResult{}, execErr
	}
	dispatchID := makeID(envelope.Metadata.MessageID, envelope.Recipient, d.now())
	d.appendAgentTrace(observability.AgentExecutionRecord{
		DispatchID:       dispatchID,
		TraceID:          envelope.Metadata.CorrelationID,
		Recipient:        envelope.Recipient,
		RequestKind:      string(envelope.Kind),
		Lifecycle:        observability.AgentLifecycleDispatched,
		CorrelationID:    envelope.Metadata.CorrelationID,
		CausationID:      envelope.Metadata.CausationID,
		OccurredAt:       d.now(),
		RequestMessageID: envelope.Metadata.MessageID,
	})

	agent, err := d.Registry.Resolve(envelope.Recipient, envelope.Kind)
	if err != nil {
		execErr := coerceExecutionError(err, envelope.Recipient, envelope.Kind, protocol.AgentFailureUnsupportedMessage, "agent resolution failed")
		d.appendAgentTrace(observability.AgentExecutionRecord{
			DispatchID:       dispatchID,
			TraceID:          envelope.Metadata.CorrelationID,
			Recipient:        envelope.Recipient,
			RequestKind:      string(envelope.Kind),
			Lifecycle:        observability.AgentLifecycleFailed,
			CorrelationID:    envelope.Metadata.CorrelationID,
			CausationID:      envelope.Metadata.CausationID,
			OccurredAt:       d.now(),
			RequestMessageID: envelope.Metadata.MessageID,
			ErrorCategory:    string(execErr.Failure.Category),
			Summary:          execErr.Failure.Message,
		})
		return AgentDispatchResult{}, execErr
	}

	d.appendAgentTrace(observability.AgentExecutionRecord{
		DispatchID:       dispatchID,
		TraceID:          envelope.Metadata.CorrelationID,
		Recipient:        envelope.Recipient,
		RequestKind:      string(envelope.Kind),
		Lifecycle:        observability.AgentLifecycleStarted,
		CorrelationID:    envelope.Metadata.CorrelationID,
		CausationID:      envelope.Metadata.CausationID,
		OccurredAt:       d.now(),
		RequestMessageID: envelope.Metadata.MessageID,
	})

	result, err := d.Executor.Execute(AgentHandlerContext{
		Context:    ctx,
		DispatchID: dispatchID,
		TraceID:    envelope.Metadata.CorrelationID,
		Now:        d.Now,
	}, agent, envelope)
	if err != nil {
		execErr := coerceExecutionError(err, envelope.Recipient, envelope.Kind, protocol.AgentFailureUnrecoverable, "agent handler failed")
		response := d.failedResponse(envelope, execErr)
		d.appendAgentTrace(observability.AgentExecutionRecord{
			DispatchID:       dispatchID,
			TraceID:          envelope.Metadata.CorrelationID,
			Recipient:        envelope.Recipient,
			RequestKind:      string(envelope.Kind),
			Lifecycle:        observability.AgentLifecycleFailed,
			CorrelationID:    envelope.Metadata.CorrelationID,
			CausationID:      envelope.Metadata.CausationID,
			OccurredAt:       d.now(),
			RequestMessageID: envelope.Metadata.MessageID,
			ResultMessageID:  response.Metadata.MessageID,
			ResultKind:       string(response.Kind),
			ErrorCategory:    string(execErr.Failure.Category),
			Summary:          execErr.Failure.Message,
		})
		return AgentDispatchResult{DispatchID: dispatchID, Request: envelope, Response: response}, execErr
	}
	if result.Kind != protocol.ExpectedResultKind(envelope.Kind) {
		execErr := &AgentExecutionError{
			Recipient: envelope.Recipient,
			Kind:      envelope.Kind,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureBadPayload,
				Message:  fmt.Sprintf("agent returned mismatched result kind %q for request %q", result.Kind, envelope.Kind),
			},
		}
		response := d.failedResponse(envelope, execErr)
		d.appendAgentTrace(observability.AgentExecutionRecord{
			DispatchID:       dispatchID,
			TraceID:          envelope.Metadata.CorrelationID,
			Recipient:        envelope.Recipient,
			RequestKind:      string(envelope.Kind),
			Lifecycle:        observability.AgentLifecycleFailed,
			CorrelationID:    envelope.Metadata.CorrelationID,
			CausationID:      envelope.Metadata.CausationID,
			OccurredAt:       d.now(),
			RequestMessageID: envelope.Metadata.MessageID,
			ResultMessageID:  response.Metadata.MessageID,
			ResultKind:       string(response.Kind),
			ErrorCategory:    string(execErr.Failure.Category),
			Summary:          execErr.Failure.Message,
		})
		return AgentDispatchResult{DispatchID: dispatchID, Request: envelope, Response: response}, execErr
	}

	response := d.successResponse(envelope, result)
	if err := response.Validate(); err != nil {
		execErr := &AgentExecutionError{
			Recipient: envelope.Recipient,
			Kind:      envelope.Kind,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureBadPayload,
				Message:  "agent handler returned invalid response body",
			},
			Cause: err,
		}
		d.appendAgentTrace(observability.AgentExecutionRecord{
			DispatchID:       dispatchID,
			TraceID:          envelope.Metadata.CorrelationID,
			Recipient:        envelope.Recipient,
			RequestKind:      string(envelope.Kind),
			Lifecycle:        observability.AgentLifecycleFailed,
			CorrelationID:    envelope.Metadata.CorrelationID,
			CausationID:      envelope.Metadata.CausationID,
			OccurredAt:       d.now(),
			RequestMessageID: envelope.Metadata.MessageID,
			ErrorCategory:    string(execErr.Failure.Category),
			Summary:          execErr.Failure.Message,
		})
		return AgentDispatchResult{DispatchID: dispatchID, Request: envelope, Response: response}, execErr
	}

	d.appendAgentTrace(observability.AgentExecutionRecord{
		DispatchID:         dispatchID,
		TraceID:            envelope.Metadata.CorrelationID,
		Recipient:          envelope.Recipient,
		RequestKind:        string(envelope.Kind),
		ResultKind:         string(response.Kind),
		Lifecycle:          observability.AgentLifecycleCompleted,
		CorrelationID:      envelope.Metadata.CorrelationID,
		CausationID:        envelope.Metadata.CausationID,
		OccurredAt:         d.now(),
		RequestMessageID:   envelope.Metadata.MessageID,
		ResultMessageID:    response.Metadata.MessageID,
		WorkflowEventTypes: eventTypes(response.EmittedWorkflowEvents),
	})
	return AgentDispatchResult{DispatchID: dispatchID, Request: envelope, Response: response}, nil
}

func (b *LocalSystemStepBus) DispatchPlan(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, memories []memory.MemoryRecord, evidence []observation.EvidenceRecord) (PlanStepResult, error) {
	envelope := b.newEnvelope(meta, RecipientPlannerAgent, protocol.MessageKindPlanRequest, protocol.AgentRequestBody{
		PlanRequest: &protocol.PlanRequestPayload{
			CurrentState: current,
			Memories:     memories,
			Evidence:     evidence,
			PlanningView: contextview.ContextViewPlanning,
		},
	})
	dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
	if err != nil {
		return PlanStepResult{}, err
	}
	result := dispatched.Response.Body.PlanResult
	if result == nil {
		return PlanStepResult{}, &AgentExecutionError{
			Recipient: RecipientPlannerAgent,
			Kind:      protocol.MessageKindPlanRequest,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureBadPayload,
				Message:  "plan result body is missing",
			},
		}
	}
	return PlanStepResult{
		Metadata: stepMetadata(dispatched),
		Plan:     result.Plan,
	}, nil
}

func (b *LocalSystemStepBus) DispatchMemorySync(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, evidence []observation.EvidenceRecord, conclusionHint string) (MemorySyncStepResult, error) {
	envelope := b.newEnvelope(meta, RecipientMemorySteward, protocol.MessageKindMemorySyncRequest, protocol.AgentRequestBody{
		MemorySyncRequest: &protocol.MemorySyncRequestPayload{
			CurrentState:   current,
			Evidence:       evidence,
			ConclusionHint: conclusionHint,
		},
	})
	dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
	if err != nil {
		return MemorySyncStepResult{}, err
	}
	result := dispatched.Response.Body.MemorySyncResult
	if result == nil {
		return MemorySyncStepResult{}, &AgentExecutionError{
			Recipient: RecipientMemorySteward,
			Kind:      protocol.MessageKindMemorySyncRequest,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureBadPayload,
				Message:  "memory sync result body is missing",
			},
		}
	}
	return MemorySyncStepResult{
		Metadata:  stepMetadata(dispatched),
		Result:    result.Result,
		AuditRefs: result.AuditRefs,
	}, nil
}

func (b *LocalSystemStepBus) DispatchReportDraft(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, memories []memory.MemoryRecord, evidence []observation.EvidenceRecord, plan planning.ExecutionPlan) (ReportDraftStepResult, error) {
	envelope := b.newEnvelope(meta, RecipientReportAgent, protocol.MessageKindReportDraftRequest, protocol.AgentRequestBody{
		ReportDraftRequest: &protocol.ReportDraftRequestPayload{
			CurrentState: current,
			Memories:     memories,
			Evidence:     evidence,
			Plan:         plan,
		},
	})
	dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
	if err != nil {
		return ReportDraftStepResult{}, err
	}
	result := dispatched.Response.Body.ReportDraftResult
	if result == nil {
		return ReportDraftStepResult{}, &AgentExecutionError{
			Recipient: RecipientReportAgent,
			Kind:      protocol.MessageKindReportDraftRequest,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureBadPayload,
				Message:  "report draft result body is missing",
			},
		}
	}
	return ReportDraftStepResult{
		Metadata: stepMetadata(dispatched),
		Draft:    result.Draft,
	}, nil
}

func (b *LocalSystemStepBus) DispatchVerification(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, evidence []observation.EvidenceRecord, report reporting.ReportPayload) (VerificationStepResult, error) {
	envelope := b.newEnvelope(meta, RecipientVerificationAgent, protocol.MessageKindVerificationRequest, protocol.AgentRequestBody{
		VerificationRequest: &protocol.VerificationRequestPayload{
			CurrentState: current,
			Evidence:     evidence,
			Report:       report,
		},
	})
	dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
	if err != nil {
		return VerificationStepResult{}, err
	}
	result := dispatched.Response.Body.VerificationResult
	if result == nil {
		return VerificationStepResult{}, &AgentExecutionError{
			Recipient: RecipientVerificationAgent,
			Kind:      protocol.MessageKindVerificationRequest,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureBadPayload,
				Message:  "verification result body is missing",
			},
		}
	}
	return VerificationStepResult{
		Metadata: stepMetadata(dispatched),
		Result:   result.Result,
	}, nil
}

func (b *LocalSystemStepBus) DispatchGovernance(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, report reporting.ReportPayload) (GovernanceStepResult, error) {
	envelope := b.newEnvelope(meta, RecipientGovernanceAgent, protocol.MessageKindGovernanceEvaluationRequest, protocol.AgentRequestBody{
		GovernanceEvaluationRequest: &protocol.GovernanceEvaluationRequestPayload{
			CurrentState:    current,
			Report:          report,
			RequestedAction: requestedAction(meta.Task),
			Actor:           RecipientGovernanceAgent,
			ActorRoles:      []string{"analyst"},
			ContainsPII:     false,
			Audience:        "user",
			ForceApproval:   reportRequiresApproval(report),
		},
	})
	dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
	if err != nil {
		return GovernanceStepResult{}, err
	}
	result := dispatched.Response.Body.GovernanceEvaluationResult
	if result == nil {
		return GovernanceStepResult{}, &AgentExecutionError{
			Recipient: RecipientGovernanceAgent,
			Kind:      protocol.MessageKindGovernanceEvaluationRequest,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureBadPayload,
				Message:  "governance result body is missing",
			},
		}
	}
	return GovernanceStepResult{
		Metadata:          stepMetadata(dispatched),
		Approval:          result.Approval,
		Disclosure:        result.Disclosure,
		RedactionRequired: result.RedactionRequired,
	}, nil
}

func (b *LocalSystemStepBus) DispatchReportFinalize(ctx context.Context, meta SystemStepMetadata, draft reporting.ReportPayload, disclosureDecision governance.PolicyDecision) (ReportFinalizeStepResult, error) {
	envelope := b.newEnvelope(meta, RecipientReportAgent, protocol.MessageKindReportFinalizeRequest, protocol.AgentRequestBody{
		ReportFinalizeRequest: &protocol.ReportFinalizeRequestPayload{
			Draft:              draft,
			DisclosureDecision: disclosureDecision,
		},
	})
	dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
	if err != nil {
		return ReportFinalizeStepResult{}, err
	}
	result := dispatched.Response.Body.ReportFinalizeResult
	if result == nil {
		return ReportFinalizeStepResult{}, &AgentExecutionError{
			Recipient: RecipientReportAgent,
			Kind:      protocol.MessageKindReportFinalizeRequest,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureBadPayload,
				Message:  "report finalize result body is missing",
			},
		}
	}
	return ReportFinalizeStepResult{
		Metadata:  stepMetadata(dispatched),
		Report:    result.Report,
		Artifacts: result.Artifacts,
	}, nil
}

func (b *LocalSystemStepBus) newEnvelope(meta SystemStepMetadata, recipient string, kind protocol.MessageKind, body protocol.AgentRequestBody) protocol.AgentEnvelope {
	now := b.now()
	return protocol.AgentEnvelope{
		Metadata: protocol.ProtocolMetadata{
			MessageID:     makeID(meta.WorkflowID, recipient, kind, now),
			CorrelationID: meta.CorrelationID,
			CausationID:   meta.CausationID,
			EmittedAt:     now,
		},
		Sender:           meta.Sender,
		Recipient:        recipient,
		Task:             meta.Task,
		StateRef:         meta.StateRef,
		RequiredEvidence: meta.Task.RequiredEvidence,
		Deadline:         meta.Task.Deadline,
		RiskLevel:        meta.Task.RiskLevel,
		Kind:             kind,
		Payload:          body,
	}
}

func (d *LocalAgentDispatcher) successResponse(envelope protocol.AgentEnvelope, result AgentHandlerResult) protocol.AgentResponse {
	now := d.now()
	return protocol.AgentResponse{
		Metadata: protocol.ProtocolMetadata{
			MessageID:     makeID(envelope.Metadata.MessageID, result.Kind, now),
			CorrelationID: envelope.Metadata.CorrelationID,
			CausationID:   envelope.Metadata.MessageID,
			EmittedAt:     now,
		},
		Sender:                envelope.Recipient,
		Recipient:             envelope.Sender,
		Task:                  envelope.Task,
		StateRef:              envelope.StateRef,
		RequiredEvidence:      envelope.RequiredEvidence,
		Deadline:              envelope.Deadline,
		RiskLevel:             envelope.RiskLevel,
		Kind:                  result.Kind,
		Success:               true,
		Body:                  result.Body,
		ProducedArtifacts:     result.ProducedArtifacts,
		ProducedMemories:      result.ProducedMemories,
		VerificationResults:   result.VerificationResults,
		EmittedWorkflowEvents: result.EmittedWorkflowEvents,
	}
}

func (d *LocalAgentDispatcher) failedResponse(envelope protocol.AgentEnvelope, execErr *AgentExecutionError) protocol.AgentResponse {
	now := d.now()
	return protocol.AgentResponse{
		Metadata: protocol.ProtocolMetadata{
			MessageID:     makeID(envelope.Metadata.MessageID, "failure", now),
			CorrelationID: envelope.Metadata.CorrelationID,
			CausationID:   envelope.Metadata.MessageID,
			EmittedAt:     now,
		},
		Sender:           envelope.Recipient,
		Recipient:        envelope.Sender,
		Task:             envelope.Task,
		StateRef:         envelope.StateRef,
		RequiredEvidence: envelope.RequiredEvidence,
		Deadline:         envelope.Deadline,
		RiskLevel:        envelope.RiskLevel,
		Kind:             protocol.ExpectedResultKind(envelope.Kind),
		Success:          false,
		Failure:          &execErr.Failure,
	}
}

func stepMetadata(dispatched AgentDispatchResult) StepDispatchMetadata {
	return StepDispatchMetadata{
		RequestMetadata:  dispatched.Request.Metadata,
		ResponseMetadata: dispatched.Response.Metadata,
		EmittedEvents:    dispatched.Response.EmittedWorkflowEvents,
	}
}

func requestedAction(spec taskspec.TaskSpec) string {
	switch spec.UserIntentType {
	case taskspec.UserIntentMonthlyReview:
		return "monthly_review_report"
	case taskspec.UserIntentDebtVsInvest:
		return "debt_vs_invest_recommendation"
	default:
		return "workflow_output"
	}
}

func reportRequiresApproval(report reporting.ReportPayload) bool {
	switch {
	case report.MonthlyReview != nil:
		return report.MonthlyReview.ApprovalRequired
	case report.DebtDecision != nil:
		return report.DebtDecision.ApprovalRequired
	default:
		return false
	}
}

func eventTypes(events []protocol.WorkflowEvent) []string {
	result := make([]string, 0, len(events))
	for _, event := range events {
		result = append(result, string(event.Type))
	}
	return result
}

func (d *LocalAgentDispatcher) appendAgentTrace(record observability.AgentExecutionRecord) {
	if d.AgentTrace != nil {
		d.AgentTrace.Append(record)
	}
	if d.EventLog != nil {
		d.EventLog.Append(observability.LogEntry{
			TraceID:       record.TraceID,
			CorrelationID: record.CorrelationID,
			Category:      "agent",
			Message:       string(record.Lifecycle),
			Details: map[string]string{
				"dispatch_id":  record.DispatchID,
				"recipient":    record.Recipient,
				"request_kind": string(record.RequestKind),
				"result_kind":  string(record.ResultKind),
			},
			OccurredAt: record.OccurredAt,
		})
	}
}

func (d *LocalAgentDispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}

func (b *LocalSystemStepBus) now() time.Time {
	if b.Now != nil {
		return b.Now().UTC()
	}
	return time.Now().UTC()
}

func makeID(parts ...any) string {
	hash := sha1.New()
	for _, part := range parts {
		fmt.Fprint(hash, part)
	}
	return hex.EncodeToString(hash.Sum(nil))[:16]
}

func coerceExecutionError(err error, recipient string, kind protocol.MessageKind, fallback protocol.AgentFailureCategory, message string) *AgentExecutionError {
	var typed *AgentExecutionError
	if ok := errors.As(err, &typed); ok {
		return typed
	}
	var categorized protocol.CategorizedAgentError
	if ok := errors.As(err, &categorized); ok {
		return &AgentExecutionError{
			Recipient: recipient,
			Kind:      kind,
			Failure:   categorized.AgentFailure(),
			Cause:     err,
		}
	}
	return &AgentExecutionError{
		Recipient: recipient,
		Kind:      kind,
		Failure: protocol.AgentFailure{
			Category: fallback,
			Message:  message,
		},
		Cause: err,
	}
}
