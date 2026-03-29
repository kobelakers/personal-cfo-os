package agents

import (
	"errors"
	"strings"
	"testing"
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
	"github.com/kobelakers/personal-cfo-os/internal/tools"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

func TestAgentDispatcherRejectsUnknownRecipientAndBadPayload(t *testing.T) {
	now := fixedNow()
	task := monthlyTask(now)
	dispatcher, _ := testDispatcher(t, now)

	unknownEnvelope := agentEnvelope(now, task, protocol.MessageKindPlanRequest, "workflow", "unknown_agent", protocol.AgentRequestBody{
		PlanRequest: &protocol.PlanRequestPayload{
			CurrentState: sampleState(now),
			Evidence:     sampleMonthlyEvidence(now),
			PlanningView: contextview.ContextViewPlanning,
		},
	})
	_, err := dispatcher.Dispatch(t.Context(), unknownEnvelope)
	var execErr *AgentExecutionError
	if !errors.As(err, &execErr) || execErr.Failure.Category != protocol.AgentFailureUnsupportedMessage {
		t.Fatalf("expected unsupported_message error, got %v", err)
	}

	badPayloadEnvelope := agentEnvelope(now, task, protocol.MessageKindPlanRequest, "workflow", RecipientPlannerAgent, protocol.AgentRequestBody{})
	_, err = dispatcher.Dispatch(t.Context(), badPayloadEnvelope)
	if !errors.As(err, &execErr) || execErr.Failure.Category != protocol.AgentFailureBadPayload {
		t.Fatalf("expected bad_payload error, got %v", err)
	}
}

func TestAgentDispatcherPropagatesCorrelationAndLifecycle(t *testing.T) {
	now := fixedNow()
	task := monthlyTask(now)
	dispatcher, traceLog := testDispatcher(t, now)

	envelope := agentEnvelope(now, task, protocol.MessageKindPlanRequest, "workflow", RecipientPlannerAgent, protocol.AgentRequestBody{
		PlanRequest: &protocol.PlanRequestPayload{
			CurrentState: sampleState(now),
			Evidence:     sampleMonthlyEvidence(now),
			PlanningView: contextview.ContextViewPlanning,
		},
	})
	result, err := dispatcher.Dispatch(t.Context(), envelope)
	if err != nil {
		t.Fatalf("dispatch planner agent: %v", err)
	}
	if result.Response.Metadata.CorrelationID != envelope.Metadata.CorrelationID || result.Response.Metadata.CausationID != envelope.Metadata.MessageID {
		t.Fatalf("expected correlation/causation to be preserved, got %+v", result.Response.Metadata)
	}
	records := traceLog.Records()
	if len(records) < 3 {
		t.Fatalf("expected dispatch lifecycle records, got %+v", records)
	}
	if records[0].Lifecycle != observability.AgentLifecycleDispatched || records[1].Lifecycle != observability.AgentLifecycleStarted || records[len(records)-1].Lifecycle != observability.AgentLifecycleCompleted {
		t.Fatalf("unexpected lifecycle sequence: %+v", records)
	}
}

func TestMemoryStewardRequestResponse(t *testing.T) {
	now := fixedNow()
	task := monthlyTask(now)
	dispatcher, _ := testDispatcher(t, now)

	envelope := agentEnvelope(now, task, protocol.MessageKindMemorySyncRequest, "workflow", RecipientMemorySteward, protocol.AgentRequestBody{
		MemorySyncRequest: &protocol.MemorySyncRequestPayload{
			CurrentState: sampleState(now),
			Evidence:     sampleMonthlyEvidence(now),
		},
	})
	result, err := dispatcher.Dispatch(t.Context(), envelope)
	if err != nil {
		t.Fatalf("dispatch memory steward: %v", err)
	}
	if result.Response.Body.MemorySyncResult == nil || len(result.Response.Body.MemorySyncResult.Result.GeneratedIDs) == 0 {
		t.Fatalf("expected generated memories, got %+v", result.Response.Body.MemorySyncResult)
	}
}

func TestReportVerificationGovernanceAndFinalizeAgents(t *testing.T) {
	now := fixedNow()
	task := monthlyTask(now)
	dispatcher, _ := testDispatcher(t, now)
	current := sampleState(now)
	evidence := sampleMonthlyEvidence(now)
	plan := samplePlan(now, task, current, nil, evidence)
	blockResults, blockVerificationContexts := dispatchDomainBlocks(t, dispatcher, now, task, current, nil, evidence, plan)

	draftEnvelope := agentEnvelope(now, task, protocol.MessageKindReportDraftRequest, "workflow", RecipientReportAgent, protocol.AgentRequestBody{
		ReportDraftRequest: &protocol.ReportDraftRequestPayload{
			CurrentState: current,
			Memories:     nil,
			Evidence:     evidence,
			Plan:         plan,
			BlockResults: blockResults,
			StateDiff: state.StateDiff{
				FromVersion:   1,
				ToVersion:     current.Version.Sequence,
				ChangedFields: []string{"cashflow_state", "liability_state"},
				EvidenceIDs:   []observation.EvidenceID{"ev-tx", "ev-debt"},
			},
		},
	})
	draftResult, err := dispatcher.Dispatch(t.Context(), draftEnvelope)
	if err != nil {
		t.Fatalf("dispatch report draft: %v", err)
	}
	if draftResult.Response.Body.ReportDraftResult == nil || draftResult.Response.Body.ReportDraftResult.Draft.MonthlyReview == nil {
		t.Fatalf("expected monthly review draft payload")
	}

	finalVerificationContext, err := contextview.VerificationContextAssembler{}.AssembleFinal(plan.PlanID, draftResult.Response.Body.ReportDraftResult.Draft.Summary(), current, nil, evidence)
	if err != nil {
		t.Fatalf("assemble final verification context: %v", err)
	}
	verificationEnvelope := agentEnvelope(now, task, protocol.MessageKindVerificationRequest, "workflow", RecipientVerificationAgent, protocol.AgentRequestBody{
		VerificationRequest: &protocol.VerificationRequestPayload{
			CurrentState:              current,
			Evidence:                  evidence,
			Memories:                  nil,
			Plan:                      plan,
			BlockResults:              blockResults,
			BlockVerificationContexts: blockVerificationContexts,
			FinalVerificationContext:  finalVerificationContext,
			Report:                    draftResult.Response.Body.ReportDraftResult.Draft,
		},
	})
	verificationResult, err := dispatcher.Dispatch(t.Context(), verificationEnvelope)
	if err != nil {
		t.Fatalf("dispatch verification agent: %v", err)
	}
	if verificationResult.Response.Body.VerificationResult == nil || len(verificationResult.Response.Body.VerificationResult.Result.Results) == 0 {
		t.Fatalf("expected verification results")
	}

	governanceEnvelope := agentEnvelope(now, task, protocol.MessageKindGovernanceEvaluationRequest, "workflow", RecipientGovernanceAgent, protocol.AgentRequestBody{
		GovernanceEvaluationRequest: &protocol.GovernanceEvaluationRequestPayload{
			CurrentState:    current,
			Report:          draftResult.Response.Body.ReportDraftResult.Draft,
			RequestedAction: "monthly_review_report",
			Actor:           RecipientGovernanceAgent,
			ActorRoles:      []string{"analyst"},
			ContainsPII:     true,
			Audience:        "user",
			ForceApproval:   false,
		},
	})
	governanceResult, err := dispatcher.Dispatch(t.Context(), governanceEnvelope)
	if err != nil {
		t.Fatalf("dispatch governance agent: %v", err)
	}
	if governanceResult.Response.Body.GovernanceEvaluationResult == nil || governanceResult.Response.Body.GovernanceEvaluationResult.Disclosure.Decision.Outcome != governance.PolicyDecisionRedact {
		t.Fatalf("expected disclosure redaction result, got %+v", governanceResult.Response.Body.GovernanceEvaluationResult)
	}

	finalizeEnvelope := agentEnvelope(now, task, protocol.MessageKindReportFinalizeRequest, "workflow", RecipientReportAgent, protocol.AgentRequestBody{
		ReportFinalizeRequest: &protocol.ReportFinalizeRequestPayload{
			Draft:              draftResult.Response.Body.ReportDraftResult.Draft,
			DisclosureDecision: governanceResult.Response.Body.GovernanceEvaluationResult.Disclosure.Decision,
		},
	})
	finalizeResult, err := dispatcher.Dispatch(t.Context(), finalizeEnvelope)
	if err != nil {
		t.Fatalf("dispatch report finalize: %v", err)
	}
	if finalizeResult.Response.Body.ReportFinalizeResult == nil || len(finalizeResult.Response.Body.ReportFinalizeResult.Artifacts) != 1 {
		t.Fatalf("expected finalized artifact, got %+v", finalizeResult.Response.Body.ReportFinalizeResult)
	}
	if finalizeResult.Response.Body.ReportFinalizeResult.Report.MonthlyReview == nil || !strings.HasPrefix(finalizeResult.Response.Body.ReportFinalizeResult.Report.MonthlyReview.Summary, "[REDACTED]") {
		t.Fatalf("expected redacted monthly review summary, got %+v", finalizeResult.Response.Body.ReportFinalizeResult.Report.MonthlyReview)
	}
}

func testDispatcher(t *testing.T, now time.Time) (*LocalAgentDispatcher, *observability.AgentTraceLog) {
	t.Helper()
	traceLog := &observability.AgentTraceLog{}
	store := memory.NewInMemoryMemoryStore()
	auditLog := &memory.MemoryAccessAuditLog{}
	writer := memory.DefaultMemoryWriter{
		Store:                      store,
		AuditLog:                   auditLog,
		MinConfidence:              0.7,
		LowConfidenceEpisodicFloor: 0.55,
	}
	retriever := memory.HybridMemoryRetriever{
		Lexical: memory.LexicalRetriever{Store: store, AuditLog: auditLog},
		Semantic: memory.SemanticRetriever{
			Store: store,
			Backend: memory.FakeSemanticSearchBackend{
				Provider: memory.KeywordEmbeddingProvider{Dimensions: 8},
				Index:    memory.NewInMemoryVectorIndex(),
				Scorer:   memory.DefaultRetrievalScorer{},
			},
			AuditLog: auditLog,
		},
		Fusion:          memory.ReciprocalRankFusion{},
		Reranker:        memory.BaselineReranker{},
		RejectionPolicy: memory.ThresholdRejectionPolicy{MinScore: 0.01},
	}
	memoryService := memory.WorkflowMemoryService{
		Writer:    writer,
		Retriever: retriever,
		Gate: governance.MemoryWriteGateService{
			PolicyEngine: governance.StaticPolicyEngine{},
			Policy: governance.MemoryWritePolicy{
				MinConfidence:   0.7,
				RequireEvidence: false,
				AllowKinds: []memory.MemoryKind{
					memory.MemoryKindEpisodic,
					memory.MemoryKindSemantic,
					memory.MemoryKindProcedural,
				},
			},
			CorrelationID: "corr-1",
		},
		Now: func() time.Time { return now },
	}
	reportService := reporting.Service{
		MonthlyReviewAggregator: reporting.MonthlyReviewAggregator{
			TaxSignals: tools.ComputeTaxSignalTool{},
			Now:        func() time.Time { return now },
		},
		DebtDecisionAggregator: reporting.DebtDecisionAggregator{
			Now: func() time.Time { return now },
		},
		LifeEventAggregator: reporting.LifeEventAssessmentAggregator{
			Now: func() time.Time { return now },
		},
		Artifacts: reporting.ArtifactService{
			Tool:     tools.GenerateTaskArtifactTool{},
			Producer: reporting.StaticArtifactProducer{Now: func() time.Time { return now }},
			Now:      func() time.Time { return now },
		},
	}
	pipeline := verification.Pipeline{
		CoverageChecker:        verification.DefaultEvidenceCoverageChecker{},
		DeterministicValidator: verification.MonthlyReviewDeterministicValidator{},
		BusinessValidator:      verification.MonthlyReviewBusinessValidator{},
		SuccessChecker:         verification.DefaultSuccessCriteriaChecker{},
		Oracle:                 verification.BaselineTrajectoryOracle{},
		Now:                    func() time.Time { return now },
	}
	approvalService := governance.ApprovalService{
		Classifier:   governance.DefaultRiskClassifier{},
		Decider:      governance.ApprovalDecider{},
		PolicyEngine: governance.StaticPolicyEngine{},
		ApprovalPolicy: governance.ApprovalPolicy{
			Name:          "agent-test-approval",
			MinRiskLevel:  governance.ActionRiskHigh,
			RequiredRoles: []string{"operator"},
			AutoApprove:   false,
		},
		ReportPolicy: governance.ReportDisclosurePolicy{Audience: "user", AllowPII: false},
	}
	registry := NewInMemoryAgentRegistry()
	agents := []RegisteredSystemAgent{
		PlannerAgentHandler{Assembler: contextview.DefaultContextAssembler{}, Planner: &planning.DeterministicPlanner{Now: func() time.Time { return now }}},
		MemoryStewardHandler{Service: memoryService},
		CashflowAgentHandler{MetricsTool: tools.ComputeCashflowMetricsTool{}},
		DebtAgentHandler{MetricsTool: tools.ComputeDebtDecisionMetricsTool{}},
		TaxAgentHandler{MetricsTool: tools.ComputeTaxSignalTool{}},
		PortfolioAgentHandler{MetricsTool: tools.ComputePortfolioImpactMetricsTool{}},
		TaskGenerationAgentHandler{},
		ReportDraftAgentHandler{Service: reportService},
		ReportFinalizeAgentHandler{Service: reportService},
		VerificationAgentHandler{Pipeline: pipeline},
		GovernanceAgentHandler{Service: approvalService},
	}
	for _, agent := range agents {
		if err := registry.Register(agent); err != nil {
			t.Fatalf("register system agent: %v", err)
		}
	}
	dispatcher := NewLocalAgentDispatcher(LocalDispatcherOptions{
		Registry:   registry,
		Executor:   LocalAgentExecutor{},
		AgentTrace: traceLog,
		EventLog:   &observability.EventLog{},
		Now:        func() time.Time { return now },
	})
	return dispatcher, traceLog
}

func fixedNow() time.Time {
	return time.Date(2026, 3, 28, 18, 0, 0, 0, time.UTC)
}

func monthlyTask(now time.Time) taskspec.TaskSpec {
	result := taskspec.DeterministicIntakeService{Now: func() time.Time { return now }}.Parse("请帮我做一份月度财务复盘")
	if result.TaskSpec == nil {
		panic("monthly intake failed in test setup")
	}
	return *result.TaskSpec
}

func sampleState(now time.Time) state.FinancialWorldState {
	return state.FinancialWorldState{
		UserID: "user-1",
		CashflowState: state.CashflowState{
			MonthlyInflowCents:    1000000,
			MonthlyOutflowCents:   600000,
			MonthlyNetIncomeCents: 400000,
			SavingsRate:           0.4,
		},
		LiabilityState: state.LiabilityState{
			TotalDebtCents:         300000,
			AverageAPR:             0.06,
			DebtBurdenRatio:        0.1,
			MinimumPaymentPressure: 0.05,
		},
		PortfolioState: state.PortfolioState{
			AllocationDrift: map[string]float64{"equity": 0.02},
		},
		WorkflowState: state.WorkflowState{
			Phase:         "observed",
			LastUpdatedAt: now,
		},
		RiskState: state.RiskState{
			OverallRisk: "medium",
		},
		Version: state.StateVersion{
			Sequence:   2,
			SnapshotID: "snap-2",
			UpdatedAt:  now,
		},
	}
}

func samplePlan(now time.Time, task taskspec.TaskSpec, current state.FinancialWorldState, memories []memory.MemoryRecord, evidence []observation.EvidenceRecord) planning.ExecutionPlan {
	planningSlice, err := contextview.DefaultContextAssembler{}.Assemble(task, current, memories, evidence, contextview.ContextViewPlanning)
	if err != nil {
		panic(err)
	}
	return (&planning.DeterministicPlanner{Now: func() time.Time { return now }}).CreatePlan(task, planningSlice, "workflow-1")
}

func dispatchDomainBlocks(
	t *testing.T,
	dispatcher *LocalAgentDispatcher,
	now time.Time,
	task taskspec.TaskSpec,
	current state.FinancialWorldState,
	memories []memory.MemoryRecord,
	evidence []observation.EvidenceRecord,
	plan planning.ExecutionPlan,
) ([]analysis.BlockResultEnvelope, []contextview.BlockVerificationContext) {
	t.Helper()
	executionAssembler := contextview.ExecutionContextAssembler{}
	verificationAssembler := contextview.VerificationContextAssembler{}
	results := make([]analysis.BlockResultEnvelope, 0, len(plan.Blocks))
	verificationContexts := make([]contextview.BlockVerificationContext, 0, len(plan.Blocks))
	for _, block := range plan.Blocks {
		spec := blockContextSpecForTest(plan, block)
		executionContext, err := executionAssembler.Assemble(spec, current, memories, evidence)
		if err != nil {
			t.Fatalf("assemble execution context: %v", err)
		}
		dispatched, err := dispatcher.Dispatch(t.Context(), domainEnvelope(now, task, current, block, executionContext, memories, evidence))
		if err != nil {
			t.Fatalf("dispatch analysis block %s: %v", block.ID, err)
		}
		result := extractBlockResult(t, block, dispatched.Response)
		results = append(results, result)
		verificationContext, err := verificationAssembler.AssembleBlock(spec, result, current, memories, evidence)
		if err != nil {
			t.Fatalf("assemble verification context: %v", err)
		}
		verificationContexts = append(verificationContexts, verificationContext)
	}
	return results, verificationContexts
}

func blockContextSpecForTest(plan planning.ExecutionPlan, block planning.ExecutionBlock) contextview.BlockContextSpec {
	requirements := make([]string, 0, len(block.RequiredEvidenceRefs))
	for _, item := range block.RequiredEvidenceRefs {
		requirements = append(requirements, item.Type)
	}
	rules := make([]string, 0, len(block.VerificationHints))
	for _, item := range block.VerificationHints {
		rules = append(rules, item.Rule)
	}
	return contextview.BlockContextSpec{
		PlanID:               plan.PlanID,
		BlockID:              string(block.ID),
		BlockKind:            string(block.Kind),
		AssignedRecipient:    block.AssignedRecipient,
		Goal:                 block.Goal,
		RequiredEvidenceRefs: requirements,
		RequiredMemoryKinds:  block.RequiredMemoryKinds,
		RequiredStateBlocks:  block.RequiredStateBlocks,
		ExecutionView:        block.ExecutionContextView,
		VerificationRules:    rules,
	}
}

func domainEnvelope(
	now time.Time,
	task taskspec.TaskSpec,
	current state.FinancialWorldState,
	block planning.ExecutionBlock,
	executionContext contextview.BlockExecutionContext,
	memories []memory.MemoryRecord,
	evidence []observation.EvidenceRecord,
) protocol.AgentEnvelope {
	filteredMemories := filterMemoriesByIDs(memories, executionContext.SelectedMemoryIDs)
	filteredEvidence := filterEvidenceByIDs(evidence, executionContext.SelectedEvidenceIDs)
	switch block.AssignedRecipient {
	case RecipientCashflowAgent:
		return agentEnvelope(now, task, protocol.MessageKindCashflowAnalysisRequest, "workflow", RecipientCashflowAgent, protocol.AgentRequestBody{
			CashflowAnalysisRequest: &protocol.CashflowAnalysisRequestPayload{
				CurrentState:     current,
				RelevantMemories: filteredMemories,
				RelevantEvidence: filteredEvidence,
				Block:            block,
				ExecutionContext: executionContext,
			},
		})
	default:
		return agentEnvelope(now, task, protocol.MessageKindDebtAnalysisRequest, "workflow", RecipientDebtAgent, protocol.AgentRequestBody{
			DebtAnalysisRequest: &protocol.DebtAnalysisRequestPayload{
				CurrentState:     current,
				RelevantMemories: filteredMemories,
				RelevantEvidence: filteredEvidence,
				Block:            block,
				ExecutionContext: executionContext,
			},
		})
	}
}

func extractBlockResult(t *testing.T, block planning.ExecutionBlock, response protocol.AgentResponse) analysis.BlockResultEnvelope {
	t.Helper()
	switch block.AssignedRecipient {
	case RecipientCashflowAgent:
		if response.Body.CashflowAnalysisResult == nil {
			t.Fatalf("cashflow analysis result missing")
		}
		return analysis.BlockResultEnvelope{
			BlockID:           string(block.ID),
			BlockKind:         string(block.Kind),
			AssignedRecipient: block.AssignedRecipient,
			Cashflow:          &response.Body.CashflowAnalysisResult.Result,
		}
	default:
		if response.Body.DebtAnalysisResult == nil {
			t.Fatalf("debt analysis result missing")
		}
		return analysis.BlockResultEnvelope{
			BlockID:           string(block.ID),
			BlockKind:         string(block.Kind),
			AssignedRecipient: block.AssignedRecipient,
			Debt:              &response.Body.DebtAnalysisResult.Result,
		}
	}
}

func sampleMonthlyEvidence(now time.Time) []observation.EvidenceRecord {
	return []observation.EvidenceRecord{
		sampleEvidence(now, "ev-tx", observation.EvidenceTypeTransactionBatch, "transaction_batch"),
		sampleEvidence(now, "ev-debt", observation.EvidenceTypeDebtObligationSnapshot, "debt_obligation_snapshot"),
		sampleEvidence(now, "ev-portfolio", observation.EvidenceTypePortfolioAllocationSnap, "portfolio_allocation_snapshot"),
	}
}

func sampleEvidence(now time.Time, id string, evidenceType observation.EvidenceType, predicate string) observation.EvidenceRecord {
	start := now.Add(-24 * time.Hour)
	return observation.EvidenceRecord{
		ID:   observation.EvidenceID(id),
		Type: evidenceType,
		Source: observation.EvidenceSource{
			Kind:       "fixture",
			Adapter:    "agent-test",
			Reference:  id,
			Provenance: "agent test fixture",
		},
		TimeRange: observation.EvidenceTimeRange{
			ObservedAt: now,
			Start:      &start,
			End:        &now,
		},
		Confidence: observation.EvidenceConfidence{
			Score:  0.9,
			Reason: "fixture evidence",
		},
		Claims: []observation.EvidenceClaim{
			{Subject: "user-1", Predicate: predicate, Object: "true"},
		},
		Normalization: observation.EvidenceNormalizationResult{
			Status:        observation.EvidenceNormalizationNormalized,
			CanonicalUnit: "unitless",
		},
		Summary:   string(evidenceType),
		CreatedAt: now,
	}
}

func agentEnvelope(now time.Time, task taskspec.TaskSpec, kind protocol.MessageKind, sender string, recipient string, body protocol.AgentRequestBody) protocol.AgentEnvelope {
	return protocol.AgentEnvelope{
		Metadata: protocol.ProtocolMetadata{
			MessageID:     "msg-" + string(kind),
			CorrelationID: "corr-1",
			CausationID:   "cause-1",
			EmittedAt:     now,
		},
		Sender:           sender,
		Recipient:        recipient,
		Task:             task,
		StateRef:         protocol.StateReference{UserID: "user-1", SnapshotID: "snap-2", Version: 2},
		RequiredEvidence: task.RequiredEvidence,
		Deadline:         task.Deadline,
		RiskLevel:        task.RiskLevel,
		Kind:             kind,
		Payload:          body,
	}
}
