package agents

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
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
	traceContext := traceRequestDetails(envelope)
	d.appendAgentTrace(observability.AgentExecutionRecord{
		DispatchID:          dispatchID,
		TraceID:             envelope.Metadata.CorrelationID,
		Recipient:           envelope.Recipient,
		RequestKind:         string(envelope.Kind),
		PlanID:              traceContext.PlanID,
		PlanBlockIDs:        traceContext.PlanBlockIDs,
		BlockID:             traceContext.BlockID,
		BlockKind:           traceContext.BlockKind,
		SelectedMemoryIDs:   traceContext.SelectedMemoryIDs,
		SelectedEvidenceIDs: traceContext.SelectedEvidenceIDs,
		SelectedStateBlocks: traceContext.SelectedStateBlocks,
		Lifecycle:           observability.AgentLifecycleDispatched,
		CorrelationID:       envelope.Metadata.CorrelationID,
		CausationID:         envelope.Metadata.CausationID,
		OccurredAt:          d.now(),
		RequestMessageID:    envelope.Metadata.MessageID,
	})

	agent, err := d.Registry.Resolve(envelope.Recipient, envelope.Kind)
	if err != nil {
		execErr := coerceExecutionError(err, envelope.Recipient, envelope.Kind, protocol.AgentFailureUnsupportedMessage, "agent resolution failed")
		d.appendAgentTrace(observability.AgentExecutionRecord{
			DispatchID:          dispatchID,
			TraceID:             envelope.Metadata.CorrelationID,
			Recipient:           envelope.Recipient,
			RequestKind:         string(envelope.Kind),
			PlanID:              traceContext.PlanID,
			PlanBlockIDs:        traceContext.PlanBlockIDs,
			BlockID:             traceContext.BlockID,
			BlockKind:           traceContext.BlockKind,
			SelectedMemoryIDs:   traceContext.SelectedMemoryIDs,
			SelectedEvidenceIDs: traceContext.SelectedEvidenceIDs,
			SelectedStateBlocks: traceContext.SelectedStateBlocks,
			Lifecycle:           observability.AgentLifecycleFailed,
			CorrelationID:       envelope.Metadata.CorrelationID,
			CausationID:         envelope.Metadata.CausationID,
			OccurredAt:          d.now(),
			RequestMessageID:    envelope.Metadata.MessageID,
			ErrorCategory:       string(execErr.Failure.Category),
			Summary:             execErr.Failure.Message,
		})
		return AgentDispatchResult{}, execErr
	}

	d.appendAgentTrace(observability.AgentExecutionRecord{
		DispatchID:          dispatchID,
		TraceID:             envelope.Metadata.CorrelationID,
		Recipient:           envelope.Recipient,
		RequestKind:         string(envelope.Kind),
		PlanID:              traceContext.PlanID,
		PlanBlockIDs:        traceContext.PlanBlockIDs,
		BlockID:             traceContext.BlockID,
		BlockKind:           traceContext.BlockKind,
		SelectedMemoryIDs:   traceContext.SelectedMemoryIDs,
		SelectedEvidenceIDs: traceContext.SelectedEvidenceIDs,
		SelectedStateBlocks: traceContext.SelectedStateBlocks,
		Lifecycle:           observability.AgentLifecycleStarted,
		CorrelationID:       envelope.Metadata.CorrelationID,
		CausationID:         envelope.Metadata.CausationID,
		OccurredAt:          d.now(),
		RequestMessageID:    envelope.Metadata.MessageID,
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
			DispatchID:          dispatchID,
			TraceID:             envelope.Metadata.CorrelationID,
			Recipient:           envelope.Recipient,
			RequestKind:         string(envelope.Kind),
			PlanID:              traceContext.PlanID,
			PlanBlockIDs:        traceContext.PlanBlockIDs,
			BlockID:             traceContext.BlockID,
			BlockKind:           traceContext.BlockKind,
			SelectedMemoryIDs:   traceContext.SelectedMemoryIDs,
			SelectedEvidenceIDs: traceContext.SelectedEvidenceIDs,
			SelectedStateBlocks: traceContext.SelectedStateBlocks,
			Lifecycle:           observability.AgentLifecycleFailed,
			CorrelationID:       envelope.Metadata.CorrelationID,
			CausationID:         envelope.Metadata.CausationID,
			OccurredAt:          d.now(),
			RequestMessageID:    envelope.Metadata.MessageID,
			ResultMessageID:     response.Metadata.MessageID,
			ResultKind:          string(response.Kind),
			ErrorCategory:       string(execErr.Failure.Category),
			Summary:             execErr.Failure.Message,
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
			DispatchID:          dispatchID,
			TraceID:             envelope.Metadata.CorrelationID,
			Recipient:           envelope.Recipient,
			RequestKind:         string(envelope.Kind),
			PlanID:              traceContext.PlanID,
			PlanBlockIDs:        traceContext.PlanBlockIDs,
			BlockID:             traceContext.BlockID,
			BlockKind:           traceContext.BlockKind,
			SelectedMemoryIDs:   traceContext.SelectedMemoryIDs,
			SelectedEvidenceIDs: traceContext.SelectedEvidenceIDs,
			SelectedStateBlocks: traceContext.SelectedStateBlocks,
			Lifecycle:           observability.AgentLifecycleFailed,
			CorrelationID:       envelope.Metadata.CorrelationID,
			CausationID:         envelope.Metadata.CausationID,
			OccurredAt:          d.now(),
			RequestMessageID:    envelope.Metadata.MessageID,
			ResultMessageID:     response.Metadata.MessageID,
			ResultKind:          string(response.Kind),
			ErrorCategory:       string(execErr.Failure.Category),
			Summary:             execErr.Failure.Message,
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
			DispatchID:          dispatchID,
			TraceID:             envelope.Metadata.CorrelationID,
			Recipient:           envelope.Recipient,
			RequestKind:         string(envelope.Kind),
			PlanID:              traceContext.PlanID,
			PlanBlockIDs:        traceContext.PlanBlockIDs,
			BlockID:             traceContext.BlockID,
			BlockKind:           traceContext.BlockKind,
			SelectedMemoryIDs:   traceContext.SelectedMemoryIDs,
			SelectedEvidenceIDs: traceContext.SelectedEvidenceIDs,
			SelectedStateBlocks: traceContext.SelectedStateBlocks,
			Lifecycle:           observability.AgentLifecycleFailed,
			CorrelationID:       envelope.Metadata.CorrelationID,
			CausationID:         envelope.Metadata.CausationID,
			OccurredAt:          d.now(),
			RequestMessageID:    envelope.Metadata.MessageID,
			ErrorCategory:       string(execErr.Failure.Category),
			Summary:             execErr.Failure.Message,
		})
		return AgentDispatchResult{DispatchID: dispatchID, Request: envelope, Response: response}, execErr
	}

	responseTrace := traceResponseDetails(response)
	d.appendAgentTrace(observability.AgentExecutionRecord{
		DispatchID:          dispatchID,
		TraceID:             envelope.Metadata.CorrelationID,
		Recipient:           envelope.Recipient,
		RequestKind:         string(envelope.Kind),
		ResultKind:          string(response.Kind),
		PlanID:              coalesceString(responseTrace.PlanID, traceContext.PlanID),
		PlanBlockIDs:        coalesceStrings(responseTrace.PlanBlockIDs, traceContext.PlanBlockIDs),
		BlockID:             coalesceString(responseTrace.BlockID, traceContext.BlockID),
		BlockKind:           coalesceString(responseTrace.BlockKind, traceContext.BlockKind),
		SelectedMemoryIDs:   traceContext.SelectedMemoryIDs,
		SelectedEvidenceIDs: traceContext.SelectedEvidenceIDs,
		SelectedStateBlocks: traceContext.SelectedStateBlocks,
		Lifecycle:           observability.AgentLifecycleCompleted,
		CorrelationID:       envelope.Metadata.CorrelationID,
		CausationID:         envelope.Metadata.CausationID,
		OccurredAt:          d.now(),
		RequestMessageID:    envelope.Metadata.MessageID,
		ResultMessageID:     response.Metadata.MessageID,
		WorkflowEventTypes:  eventTypes(response.EmittedWorkflowEvents),
		ResultSummary:       responseTrace.ResultSummary,
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

func (b *LocalSystemStepBus) DispatchAnalysisBlock(
	ctx context.Context,
	meta SystemStepMetadata,
	block planning.ExecutionBlock,
	current state.FinancialWorldState,
	memories []memory.MemoryRecord,
	evidence []observation.EvidenceRecord,
	executionContext contextview.BlockExecutionContext,
) (AnalysisBlockStepResult, error) {
	filteredMemories := filterMemoriesByIDs(memories, executionContext.SelectedMemoryIDs)
	filteredEvidence := filterEvidenceByIDs(evidence, executionContext.SelectedEvidenceIDs)
	switch block.AssignedRecipient {
	case RecipientCashflowAgent:
		envelope := b.newEnvelope(meta, RecipientCashflowAgent, protocol.MessageKindCashflowAnalysisRequest, protocol.AgentRequestBody{
			CashflowAnalysisRequest: &protocol.CashflowAnalysisRequestPayload{
				CurrentState:     current,
				RelevantMemories: filteredMemories,
				RelevantEvidence: filteredEvidence,
				Block:            block,
				ExecutionContext: executionContext,
			},
		})
		dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
		if err != nil {
			return AnalysisBlockStepResult{}, err
		}
		result := dispatched.Response.Body.CashflowAnalysisResult
		if result == nil {
			return AnalysisBlockStepResult{}, &AgentExecutionError{
				Recipient: RecipientCashflowAgent,
				Kind:      protocol.MessageKindCashflowAnalysisRequest,
				Failure: protocol.AgentFailure{
					Category: protocol.AgentFailureBadPayload,
					Message:  "cashflow analysis result body is missing",
				},
			}
		}
		envelopeResult := analysis.BlockResultEnvelope{
			BlockID:           string(block.ID),
			BlockKind:         string(block.Kind),
			AssignedRecipient: block.AssignedRecipient,
			Cashflow:          &result.Result,
		}
		return AnalysisBlockStepResult{Metadata: stepMetadata(dispatched), Block: block, Result: envelopeResult}, envelopeResult.Validate()
	case RecipientDebtAgent:
		envelope := b.newEnvelope(meta, RecipientDebtAgent, protocol.MessageKindDebtAnalysisRequest, protocol.AgentRequestBody{
			DebtAnalysisRequest: &protocol.DebtAnalysisRequestPayload{
				CurrentState:     current,
				RelevantMemories: filteredMemories,
				RelevantEvidence: filteredEvidence,
				Block:            block,
				ExecutionContext: executionContext,
			},
		})
		dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
		if err != nil {
			return AnalysisBlockStepResult{}, err
		}
		result := dispatched.Response.Body.DebtAnalysisResult
		if result == nil {
			return AnalysisBlockStepResult{}, &AgentExecutionError{
				Recipient: RecipientDebtAgent,
				Kind:      protocol.MessageKindDebtAnalysisRequest,
				Failure: protocol.AgentFailure{
					Category: protocol.AgentFailureBadPayload,
					Message:  "debt analysis result body is missing",
				},
			}
		}
		envelopeResult := analysis.BlockResultEnvelope{
			BlockID:           string(block.ID),
			BlockKind:         string(block.Kind),
			AssignedRecipient: block.AssignedRecipient,
			Debt:              &result.Result,
		}
		return AnalysisBlockStepResult{Metadata: stepMetadata(dispatched), Block: block, Result: envelopeResult}, envelopeResult.Validate()
	case RecipientTaxAgent:
		envelope := b.newEnvelope(meta, RecipientTaxAgent, protocol.MessageKindTaxAnalysisRequest, protocol.AgentRequestBody{
			TaxAnalysisRequest: &protocol.TaxAnalysisRequestPayload{
				CurrentState:     current,
				RelevantMemories: filteredMemories,
				RelevantEvidence: filteredEvidence,
				Block:            block,
				ExecutionContext: executionContext,
			},
		})
		dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
		if err != nil {
			return AnalysisBlockStepResult{}, err
		}
		result := dispatched.Response.Body.TaxAnalysisResult
		if result == nil {
			return AnalysisBlockStepResult{}, &AgentExecutionError{
				Recipient: RecipientTaxAgent,
				Kind:      protocol.MessageKindTaxAnalysisRequest,
				Failure: protocol.AgentFailure{
					Category: protocol.AgentFailureBadPayload,
					Message:  "tax analysis result body is missing",
				},
			}
		}
		envelopeResult := analysis.BlockResultEnvelope{
			BlockID:           string(block.ID),
			BlockKind:         string(block.Kind),
			AssignedRecipient: block.AssignedRecipient,
			Tax:               &result.Result,
		}
		return AnalysisBlockStepResult{Metadata: stepMetadata(dispatched), Block: block, Result: envelopeResult}, envelopeResult.Validate()
	case RecipientPortfolioAgent:
		envelope := b.newEnvelope(meta, RecipientPortfolioAgent, protocol.MessageKindPortfolioAnalysisRequest, protocol.AgentRequestBody{
			PortfolioAnalysisRequest: &protocol.PortfolioAnalysisRequestPayload{
				CurrentState:     current,
				RelevantMemories: filteredMemories,
				RelevantEvidence: filteredEvidence,
				Block:            block,
				ExecutionContext: executionContext,
			},
		})
		dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
		if err != nil {
			return AnalysisBlockStepResult{}, err
		}
		result := dispatched.Response.Body.PortfolioAnalysisResult
		if result == nil {
			return AnalysisBlockStepResult{}, &AgentExecutionError{
				Recipient: RecipientPortfolioAgent,
				Kind:      protocol.MessageKindPortfolioAnalysisRequest,
				Failure: protocol.AgentFailure{
					Category: protocol.AgentFailureBadPayload,
					Message:  "portfolio analysis result body is missing",
				},
			}
		}
		envelopeResult := analysis.BlockResultEnvelope{
			BlockID:           string(block.ID),
			BlockKind:         string(block.Kind),
			AssignedRecipient: block.AssignedRecipient,
			Portfolio:         &result.Result,
		}
		return AnalysisBlockStepResult{Metadata: stepMetadata(dispatched), Block: block, Result: envelopeResult}, envelopeResult.Validate()
	case RecipientBehaviorAgent:
		envelope := b.newEnvelope(meta, RecipientBehaviorAgent, protocol.MessageKindBehaviorAnalysisRequest, protocol.AgentRequestBody{
			BehaviorAnalysisRequest: &protocol.BehaviorAnalysisRequestPayload{
				CurrentState:     current,
				RelevantMemories: filteredMemories,
				RelevantEvidence: filteredEvidence,
				Block:            block,
				ExecutionContext: executionContext,
			},
		})
		dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
		if err != nil {
			return AnalysisBlockStepResult{}, err
		}
		result := dispatched.Response.Body.BehaviorAnalysisResult
		if result == nil {
			return AnalysisBlockStepResult{}, &AgentExecutionError{
				Recipient: RecipientBehaviorAgent,
				Kind:      protocol.MessageKindBehaviorAnalysisRequest,
				Failure: protocol.AgentFailure{
					Category: protocol.AgentFailureBadPayload,
					Message:  "behavior analysis result body is missing",
				},
			}
		}
		envelopeResult := analysis.BlockResultEnvelope{
			BlockID:           string(block.ID),
			BlockKind:         string(block.Kind),
			AssignedRecipient: block.AssignedRecipient,
			Behavior:          &result.Result,
		}
		return AnalysisBlockStepResult{Metadata: stepMetadata(dispatched), Block: block, Result: envelopeResult}, envelopeResult.Validate()
	default:
		return AnalysisBlockStepResult{}, &AgentExecutionError{
			Recipient: block.AssignedRecipient,
			Kind:      protocol.MessageKind("analysis_request"),
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureUnsupportedMessage,
				Message:  fmt.Sprintf("unsupported block recipient %q", block.AssignedRecipient),
			},
		}
	}
}

func (b *LocalSystemStepBus) DispatchReportDraft(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, memories []memory.MemoryRecord, evidence []observation.EvidenceRecord, plan planning.ExecutionPlan, blockResults []analysis.BlockResultEnvelope, diff state.StateDiff, taskGraph *taskspec.TaskGraph) (ReportDraftStepResult, error) {
	envelope := b.newEnvelope(meta, RecipientReportAgent, protocol.MessageKindReportDraftRequest, protocol.AgentRequestBody{
		ReportDraftRequest: &protocol.ReportDraftRequestPayload{
			CurrentState: current,
			Memories:     memories,
			Evidence:     evidence,
			Plan:         plan,
			BlockResults: blockResults,
			StateDiff:    diff,
			TaskGraph:    taskGraph,
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

func (b *LocalSystemStepBus) DispatchTaskGeneration(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, eventEvidence []observation.EvidenceRecord, memories []memory.MemoryRecord, diff state.StateDiff, plan planning.ExecutionPlan, validatedBlockResults []analysis.BlockResultEnvelope) (TaskGenerationStepResult, error) {
	envelope := b.newEnvelope(meta, RecipientTaskGenerationAgent, protocol.MessageKindTaskGenerationRequest, protocol.AgentRequestBody{
		TaskGenerationRequest: &protocol.TaskGenerationRequestPayload{
			CurrentState:          current,
			EventEvidence:         eventEvidence,
			Memories:              memories,
			StateDiff:             diff,
			Plan:                  plan,
			ValidatedBlockResults: validatedBlockResults,
		},
	})
	dispatched, err := b.Dispatcher.Dispatch(ctx, envelope)
	if err != nil {
		return TaskGenerationStepResult{}, err
	}
	result := dispatched.Response.Body.TaskGenerationResult
	if result == nil {
		return TaskGenerationStepResult{}, &AgentExecutionError{
			Recipient: RecipientTaskGenerationAgent,
			Kind:      protocol.MessageKindTaskGenerationRequest,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureBadPayload,
				Message:  "task generation result body is missing",
			},
		}
	}
	return TaskGenerationStepResult{
		Metadata:  stepMetadata(dispatched),
		TaskGraph: result.TaskGraph,
	}, nil
}

func (b *LocalSystemStepBus) DispatchVerification(ctx context.Context, meta SystemStepMetadata, input VerificationStepInput) (VerificationStepResult, error) {
	envelope := b.newEnvelope(meta, RecipientVerificationAgent, protocol.MessageKindVerificationRequest, protocol.AgentRequestBody{
		VerificationRequest: &protocol.VerificationRequestPayload{
			Stage:                     input.Stage,
			CurrentState:              input.CurrentState,
			Evidence:                  input.Evidence,
			Memories:                  input.Memories,
			Plan:                      input.Plan,
			BlockResults:              input.BlockResults,
			BlockVerificationContexts: input.BlockVerificationContexts,
			FinalVerificationContext:  input.FinalVerificationContext,
			TaskGraph:                 input.TaskGraph,
			Report:                    input.Report,
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

func (b *LocalSystemStepBus) DispatchGovernance(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, report reporting.ReportPayload, taskGraph *taskspec.TaskGraph) (GovernanceStepResult, error) {
	var generatedTasks []taskspec.GeneratedTaskSpec
	forceApproval := reportRequiresApproval(report)
	if taskGraph != nil {
		generatedTasks = append(generatedTasks, taskGraph.GeneratedTasks...)
		for _, item := range generatedTasks {
			if item.Metadata.RequiresApproval {
				forceApproval = true
				break
			}
		}
	}
	envelope := b.newEnvelope(meta, RecipientGovernanceAgent, protocol.MessageKindGovernanceEvaluationRequest, protocol.AgentRequestBody{
		GovernanceEvaluationRequest: &protocol.GovernanceEvaluationRequestPayload{
			CurrentState:    current,
			Report:          report,
			RequestedAction: requestedAction(meta.Task),
			Actor:           RecipientGovernanceAgent,
			ActorRoles:      []string{"analyst"},
			ContainsPII:     false,
			Audience:        "user",
			ForceApproval:   forceApproval,
			TaskGraph:       taskGraph,
			GeneratedTasks:  generatedTasks,
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
	case taskspec.UserIntentLifeEventTrigger:
		return "life_event_follow_up_registration"
	case taskspec.UserIntentTaxOptimization:
		return "tax_optimization_report"
	case taskspec.UserIntentPortfolioRebalance:
		return "portfolio_rebalance_report"
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
	case report.LifeEventAssessment != nil:
		return false
	case report.TaxOptimization != nil:
		return report.TaxOptimization.ApprovalRequired
	case report.PortfolioRebalance != nil:
		return report.PortfolioRebalance.ApprovalRequired
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

func filterMemoriesByIDs(memories []memory.MemoryRecord, ids []string) []memory.MemoryRecord {
	if len(ids) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	filtered := make([]memory.MemoryRecord, 0, len(memories))
	for _, item := range memories {
		if _, ok := allowed[item.ID]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterEvidenceByIDs(evidence []observation.EvidenceRecord, ids []observation.EvidenceID) []observation.EvidenceRecord {
	if len(ids) == 0 {
		return nil
	}
	allowed := make(map[observation.EvidenceID]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	filtered := make([]observation.EvidenceRecord, 0, len(evidence))
	for _, item := range evidence {
		if _, ok := allowed[item.ID]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
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

type traceContext struct {
	PlanID              string
	PlanBlockIDs        []string
	BlockID             string
	BlockKind           string
	SelectedMemoryIDs   []string
	SelectedEvidenceIDs []string
	SelectedStateBlocks []string
	ResultSummary       string
}

func traceRequestDetails(envelope protocol.AgentEnvelope) traceContext {
	switch envelope.Kind {
	case protocol.MessageKindReportDraftRequest:
		if payload := envelope.Payload.ReportDraftRequest; payload != nil {
			return traceContext{
				PlanID:       payload.Plan.PlanID,
				PlanBlockIDs: planBlockIDs(payload.Plan),
			}
		}
	case protocol.MessageKindVerificationRequest:
		if payload := envelope.Payload.VerificationRequest; payload != nil {
			return traceContext{
				PlanID:       payload.Plan.PlanID,
				PlanBlockIDs: planBlockIDs(payload.Plan),
			}
		}
	case protocol.MessageKindCashflowAnalysisRequest:
		if payload := envelope.Payload.CashflowAnalysisRequest; payload != nil {
			return traceContext{
				PlanID:              payload.ExecutionContext.PlanID,
				BlockID:             payload.ExecutionContext.BlockID,
				BlockKind:           payload.ExecutionContext.BlockKind,
				SelectedMemoryIDs:   payload.ExecutionContext.SelectedMemoryIDs,
				SelectedEvidenceIDs: evidenceIDsToStrings(payload.ExecutionContext.SelectedEvidenceIDs),
				SelectedStateBlocks: payload.ExecutionContext.SelectedStateBlocks,
			}
		}
	case protocol.MessageKindDebtAnalysisRequest:
		if payload := envelope.Payload.DebtAnalysisRequest; payload != nil {
			return traceContext{
				PlanID:              payload.ExecutionContext.PlanID,
				BlockID:             payload.ExecutionContext.BlockID,
				BlockKind:           payload.ExecutionContext.BlockKind,
				SelectedMemoryIDs:   payload.ExecutionContext.SelectedMemoryIDs,
				SelectedEvidenceIDs: evidenceIDsToStrings(payload.ExecutionContext.SelectedEvidenceIDs),
				SelectedStateBlocks: payload.ExecutionContext.SelectedStateBlocks,
			}
		}
	case protocol.MessageKindTaxAnalysisRequest:
		if payload := envelope.Payload.TaxAnalysisRequest; payload != nil {
			return traceContext{
				PlanID:              payload.ExecutionContext.PlanID,
				BlockID:             payload.ExecutionContext.BlockID,
				BlockKind:           payload.ExecutionContext.BlockKind,
				SelectedMemoryIDs:   payload.ExecutionContext.SelectedMemoryIDs,
				SelectedEvidenceIDs: evidenceIDsToStrings(payload.ExecutionContext.SelectedEvidenceIDs),
				SelectedStateBlocks: payload.ExecutionContext.SelectedStateBlocks,
			}
		}
	case protocol.MessageKindPortfolioAnalysisRequest:
		if payload := envelope.Payload.PortfolioAnalysisRequest; payload != nil {
			return traceContext{
				PlanID:              payload.ExecutionContext.PlanID,
				BlockID:             payload.ExecutionContext.BlockID,
				BlockKind:           payload.ExecutionContext.BlockKind,
				SelectedMemoryIDs:   payload.ExecutionContext.SelectedMemoryIDs,
				SelectedEvidenceIDs: evidenceIDsToStrings(payload.ExecutionContext.SelectedEvidenceIDs),
				SelectedStateBlocks: payload.ExecutionContext.SelectedStateBlocks,
			}
		}
	case protocol.MessageKindBehaviorAnalysisRequest:
		if payload := envelope.Payload.BehaviorAnalysisRequest; payload != nil {
			return traceContext{
				PlanID:              payload.ExecutionContext.PlanID,
				BlockID:             payload.ExecutionContext.BlockID,
				BlockKind:           payload.ExecutionContext.BlockKind,
				SelectedMemoryIDs:   payload.ExecutionContext.SelectedMemoryIDs,
				SelectedEvidenceIDs: evidenceIDsToStrings(payload.ExecutionContext.SelectedEvidenceIDs),
				SelectedStateBlocks: payload.ExecutionContext.SelectedStateBlocks,
			}
		}
	}
	return traceContext{}
}

func traceResponseDetails(response protocol.AgentResponse) traceContext {
	switch response.Kind {
	case protocol.MessageKindPlanResult:
		if payload := response.Body.PlanResult; payload != nil {
			return traceContext{
				PlanID:        payload.Plan.PlanID,
				PlanBlockIDs:  planBlockIDs(payload.Plan),
				ResultSummary: fmt.Sprintf("generated %d execution blocks", len(payload.Plan.Blocks)),
			}
		}
	case protocol.MessageKindCashflowAnalysisResult:
		if payload := response.Body.CashflowAnalysisResult; payload != nil {
			return traceContext{
				BlockID:       payload.Result.BlockID,
				ResultSummary: payload.Result.Summary,
			}
		}
	case protocol.MessageKindDebtAnalysisResult:
		if payload := response.Body.DebtAnalysisResult; payload != nil {
			return traceContext{
				BlockID:       payload.Result.BlockID,
				ResultSummary: payload.Result.Summary,
			}
		}
	case protocol.MessageKindTaxAnalysisResult:
		if payload := response.Body.TaxAnalysisResult; payload != nil {
			return traceContext{
				BlockID:       payload.Result.BlockID,
				ResultSummary: payload.Result.Summary,
			}
		}
	case protocol.MessageKindPortfolioAnalysisResult:
		if payload := response.Body.PortfolioAnalysisResult; payload != nil {
			return traceContext{
				BlockID:       payload.Result.BlockID,
				ResultSummary: payload.Result.Summary,
			}
		}
	case protocol.MessageKindBehaviorAnalysisResult:
		if payload := response.Body.BehaviorAnalysisResult; payload != nil {
			return traceContext{
				BlockID:       payload.Result.BlockID,
				ResultSummary: payload.Result.Summary,
			}
		}
	case protocol.MessageKindReportDraftResult:
		if payload := response.Body.ReportDraftResult; payload != nil {
			return traceContext{ResultSummary: payload.Draft.Summary()}
		}
	case protocol.MessageKindVerificationResult:
		if payload := response.Body.VerificationResult; payload != nil {
			return traceContext{ResultSummary: fmt.Sprintf("verification produced %d results", len(payload.Result.Results))}
		}
	case protocol.MessageKindGovernanceEvaluationResult:
		if payload := response.Body.GovernanceEvaluationResult; payload != nil {
			approvalOutcome := ""
			if payload.Approval.Decision != nil {
				approvalOutcome = string(payload.Approval.Decision.Outcome)
			}
			return traceContext{
				ResultSummary: fmt.Sprintf("approval=%s disclosure=%s", approvalOutcome, payload.Disclosure.Decision.Outcome),
			}
		}
	case protocol.MessageKindReportFinalizeResult:
		if payload := response.Body.ReportFinalizeResult; payload != nil {
			return traceContext{ResultSummary: payload.Report.Summary()}
		}
	}
	return traceContext{}
}

func planBlockIDs(plan planning.ExecutionPlan) []string {
	result := make([]string, 0, len(plan.Blocks))
	for _, block := range plan.Blocks {
		result = append(result, string(block.ID))
	}
	return result
}

func evidenceIDsToStrings(ids []observation.EvidenceID) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		result = append(result, string(id))
	}
	return result
}

func coalesceString(preferred string, fallback string) string {
	if preferred != "" {
		return preferred
	}
	return fallback
}

func coalesceStrings(preferred []string, fallback []string) []string {
	if len(preferred) > 0 {
		return preferred
	}
	return fallback
}
