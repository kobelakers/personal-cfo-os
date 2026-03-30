package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/agents"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/prompt"
	"github.com/kobelakers/personal-cfo-os/internal/reducers"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
	"github.com/kobelakers/personal-cfo-os/internal/workflows"
)

type Phase5DOptions struct {
	FixtureDir                   string
	HoldingsFixture              string
	MemoryDBPath                 string
	EmbeddingModel               string
	Now                          func() time.Time
	ChatModel                    model.ChatModel
	ChatModelFactory             func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel
	EmbeddingProvider            memory.EmbeddingProvider
	EmbeddingProviderFactory     func(callRecorder memory.EmbeddingCallRecorder, usageRecorder memory.EmbeddingUsageRecorder) memory.EmbeddingProvider
	VerificationPipelineOverride func(base verification.Pipeline) verification.Pipeline
	PromptRegistry               *prompt.PromptRegistry
	EventLog                     *observability.EventLog
	AgentTrace                   *observability.AgentTraceLog
	PromptTrace                  *observability.PromptTraceLog
	LLMTrace                     *observability.LLMCallLog
	UsageTrace                   *observability.UsageTraceLog
	StructuredTrace              *observability.StructuredOutputTraceLog
	MemoryTrace                  *observability.MemoryTraceLog
	EmbeddingCallTrace           *observability.EmbeddingCallLog
	EmbeddingUsageTrace          *observability.EmbeddingUsageLog
}

type Phase5DEnvironment struct {
	MonthlyReview       workflows.MonthlyReviewWorkflow
	DebtVsInvest        workflows.DebtVsInvestWorkflow
	EventLog            *observability.EventLog
	AgentTrace          *observability.AgentTraceLog
	PromptTrace         *observability.PromptTraceLog
	LLMTrace            *observability.LLMCallLog
	UsageTrace          *observability.UsageTraceLog
	StructuredTrace     *observability.StructuredOutputTraceLog
	MemoryTrace         *observability.MemoryTraceLog
	EmbeddingCallTrace  *observability.EmbeddingCallLog
	EmbeddingUsageTrace *observability.EmbeddingUsageLog
	Timeline            *runtimepkg.WorkflowTimeline
	Journal             *runtimepkg.CheckpointJournal
	FixtureDir          string
	MemoryStores        *memory.SQLiteMemoryStores
	MemoryIndexer       memory.MemoryIndexer
	MemoryAuditLog      *memory.MemoryAccessAuditLog
}

type MonthlyReview5DRunOutput struct {
	Result workflows.MonthlyReviewRunResult `json:"result"`
	Trace  observability.WorkflowTraceDump  `json:"trace"`
}

type DebtVsInvest5DRunOutput struct {
	Result workflows.DebtDecisionRunResult `json:"result"`
	Trace  observability.WorkflowTraceDump `json:"trace"`
}

func OpenPhase5DEnvironment(options Phase5DOptions) (*Phase5DEnvironment, error) {
	nowFn := options.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	fixtureDir := options.FixtureDir
	if fixtureDir == "" {
		fixtureDir = filepath.Join("tests", "fixtures")
	}
	if options.MemoryDBPath == "" {
		return nil, fmt.Errorf("phase 5d environment requires injected memory db path")
	}
	deps, err := loadFixtureDeps(fixtureDir, nowFn)
	if err != nil {
		return nil, err
	}
	if options.HoldingsFixture != "" {
		holdingsCSV, err := readFixture(fixtureDir, options.HoldingsFixture)
		if err != nil {
			return nil, err
		}
		holdings, err := observation.LoadHoldingRecordsCSV(holdingsCSV)
		if err != nil {
			return nil, fmt.Errorf("load holdings override fixture: %w", err)
		}
		deps.LedgerAdapter.Holdings = holdings
	}

	registry := options.PromptRegistry
	if registry == nil {
		registry, err = prompt.NewRegistry()
		if err != nil {
			return nil, err
		}
	}
	eventLog := valueOrDefault(options.EventLog, &observability.EventLog{})
	agentTrace := valueOrDefault(options.AgentTrace, &observability.AgentTraceLog{})
	promptTrace := valueOrDefault(options.PromptTrace, &observability.PromptTraceLog{})
	llmTrace := valueOrDefault(options.LLMTrace, &observability.LLMCallLog{})
	usageTrace := valueOrDefault(options.UsageTrace, &observability.UsageTraceLog{})
	structuredTrace := valueOrDefault(options.StructuredTrace, &observability.StructuredOutputTraceLog{})
	memoryTrace := valueOrDefault(options.MemoryTrace, &observability.MemoryTraceLog{})
	embeddingCallTrace := valueOrDefault(options.EmbeddingCallTrace, &observability.EmbeddingCallLog{})
	embeddingUsageTrace := valueOrDefault(options.EmbeddingUsageTrace, &observability.EmbeddingUsageLog{})

	chatModel := options.ChatModel
	if options.ChatModelFactory != nil {
		chatModel = options.ChatModelFactory(llmTrace, usageTrace)
	} else if chatModel == nil {
		chatModel = NewMockMonthlyReviewChatModelWithTrace(llmTrace, usageTrace)
	}
	embeddingProvider := options.EmbeddingProvider
	if options.EmbeddingProviderFactory != nil {
		embeddingProvider = options.EmbeddingProviderFactory(embeddingCallTrace, embeddingUsageTrace)
	} else if embeddingProvider == nil {
		embeddingProvider = NewMockMonthlyReviewEmbeddingProvider(embeddingCallTrace, embeddingUsageTrace)
	}
	embeddingModel := strings.TrimSpace(options.EmbeddingModel)
	if embeddingModel == "" {
		if live := memory.OpenAIEmbeddingConfigFromEnv().EmbeddingModel; live != "" {
			embeddingModel = live
		} else {
			embeddingModel = "mock-embedding-model"
		}
	}
	stores, err := memory.NewSQLiteMemoryStores(memory.SQLiteStoreConfig{DSN: options.MemoryDBPath})
	if err != nil {
		return nil, err
	}
	memoryAuditLog := &memory.MemoryAccessAuditLog{}
	memoryWriter := memory.DefaultMemoryWriter{
		Store:                      stores.Store,
		Relations:                  stores.Relations,
		AuditStore:                 stores.Audit,
		WriteEventStore:            stores.WriteEvents,
		EmbeddingStore:             stores.Embeddings,
		EmbeddingProvider:          embeddingProvider,
		EmbeddingModel:             embeddingModel,
		AuditLog:                   memoryAuditLog,
		MinConfidence:              0.7,
		LowConfidenceEpisodicFloor: 0.55,
		Now:                        nowFn,
	}
	retriever := memory.HybridMemoryRetriever{
		Lexical: memory.LexicalRetriever{
			Query:    stores.Query,
			Audit:    stores.Audit,
			AuditLog: memoryAuditLog,
		},
		Semantic: memory.SemanticRetriever{
			Store: stores.Store,
			Backend: memory.EmbeddedSemanticSearchBackend{
				Provider:   embeddingProvider,
				Embeddings: stores.Embeddings,
				Model:      embeddingModel,
				Scorer:     memory.DefaultRetrievalScorer{},
			},
			Audit:    stores.Audit,
			AuditLog: memoryAuditLog,
		},
		Fusion:   memory.ReciprocalRankFusion{},
		Reranker: memory.BaselineReranker{},
		RejectionPolicy: memory.ThresholdRejectionPolicy{
			MinScore: 0.01,
			DefaultPolicy: memory.RetrievalPolicy{
				Name: "phase5d_default",
				FreshnessPolicy: memory.FreshnessPolicy{
					Name:                  "phase5d_default",
					EpisodicMaxAge:        90 * 24 * time.Hour,
					RejectLowConfidence:   true,
					LowConfidenceFloor:    0.7,
					MinAcceptedFusedScore: 0.01,
				},
			},
			Policies: map[string]memory.RetrievalPolicy{
				"monthly_review_planning": {
					Name: "monthly_review_planning",
					FreshnessPolicy: memory.FreshnessPolicy{
						Name:                  "planning_recent_bias",
						EpisodicMaxAge:        90 * 24 * time.Hour,
						RejectLowConfidence:   true,
						LowConfidenceFloor:    0.7,
						MinAcceptedFusedScore: 0.01,
					},
				},
				"monthly_review_cashflow": {
					Name: "monthly_review_cashflow",
					FreshnessPolicy: memory.FreshnessPolicy{
						Name:                  "cashflow_signal_bias",
						EpisodicMaxAge:        90 * 24 * time.Hour,
						RejectLowConfidence:   true,
						LowConfidenceFloor:    0.7,
						MinAcceptedFusedScore: 0.01,
					},
				},
				"debt_vs_invest_planning": {
					Name: "debt_vs_invest_planning",
					FreshnessPolicy: memory.FreshnessPolicy{
						Name:                  "debt_tradeoff_bias",
						EpisodicMaxAge:        90 * 24 * time.Hour,
						RejectLowConfidence:   true,
						LowConfidenceFloor:    0.7,
						MinAcceptedFusedScore: 0.01,
					},
				},
			},
			Now: nowFn,
		},
	}
	indexer := memory.DefaultMemoryIndexer{
		Store:      stores.Store,
		Embeddings: stores.Embeddings,
		Writer:     stores.WriteEvents,
		Provider:   embeddingProvider,
		Model:      embeddingModel,
		Now:        nowFn,
		WorkflowID: "phase-5d-indexer",
		TaskID:     "phase-5d-indexer",
		TraceID:    "trace-phase-5d-indexer",
	}
	timeline := &runtimepkg.WorkflowTimeline{}
	journal := &runtimepkg.CheckpointJournal{}
	engine := finance.DeterministicEngine{}

	systemSteps, err := buildPhase5DStepBus(phase5DWiring{
		deps:                         deps,
		registry:                     registry,
		chatModel:                    chatModel,
		eventLog:                     eventLog,
		agentTrace:                   agentTrace,
		promptTrace:                  promptTrace,
		llmTrace:                     llmTrace,
		usageTrace:                   usageTrace,
		structuredTrace:              structuredTrace,
		memoryWriter:                 memoryWriter,
		retriever:                    retriever,
		memoryTrace:                  memoryTrace,
		financeEngine:                engine,
		verificationPipelineOverride: options.VerificationPipelineOverride,
	})
	if err != nil {
		_ = stores.DB.Close()
		return nil, err
	}

	monthlyWorkflow := workflows.MonthlyReviewWorkflow{
		Intake: taskspec.DeterministicIntakeService{Now: deps.LedgerAdapter.Now},
		ReviewService: workflows.MonthlyReviewService{
			QueryTransaction: tools.QueryTransactionTool{Adapter: deps.LedgerAdapter},
			QueryLiability:   tools.QueryLiabilityTool{Adapter: deps.LedgerAdapter},
			QueryPortfolio:   tools.QueryPortfolioTool{LedgerAdapter: deps.LedgerAdapter},
			ParseDocument: tools.ParseDocumentTool{
				Structured: deps.StructuredDocAdapter,
				Agentic:    deps.AgenticDocAdapter,
			},
			ReducerEngine: reducers.DeterministicReducerEngine{Now: deps.LedgerAdapter.Now},
		},
		SystemSteps: systemSteps,
		Runtime: runtimepkg.NewLocalWorkflowRuntime("monthly-review-5d", runtimepkg.LocalRuntimeOptions{
			EventLog: eventLog,
			Timeline: timeline,
			Journal:  journal,
			Now:      deps.LedgerAdapter.Now,
		}),
		Now: deps.LedgerAdapter.Now,
	}
	debtWorkflow := workflows.DebtVsInvestWorkflow{
		Intake: taskspec.DeterministicIntakeService{Now: deps.LedgerAdapter.Now},
		DecisionService: workflows.DebtVsInvestService{
			QueryTransaction: tools.QueryTransactionTool{Adapter: deps.LedgerAdapter},
			QueryLiability:   tools.QueryLiabilityTool{Adapter: deps.LedgerAdapter},
			QueryPortfolio:   tools.QueryPortfolioTool{LedgerAdapter: deps.LedgerAdapter},
			ReducerEngine:    reducers.DeterministicReducerEngine{Now: deps.LedgerAdapter.Now},
		},
		SystemSteps: systemSteps,
		Runtime: runtimepkg.NewLocalWorkflowRuntime("debt-vs-invest-5d", runtimepkg.LocalRuntimeOptions{
			EventLog: eventLog,
			Timeline: timeline,
			Journal:  journal,
			Now:      deps.LedgerAdapter.Now,
		}),
		Now: deps.LedgerAdapter.Now,
	}

	return &Phase5DEnvironment{
		MonthlyReview:       monthlyWorkflow,
		DebtVsInvest:        debtWorkflow,
		EventLog:            eventLog,
		AgentTrace:          agentTrace,
		PromptTrace:         promptTrace,
		LLMTrace:            llmTrace,
		UsageTrace:          usageTrace,
		StructuredTrace:     structuredTrace,
		MemoryTrace:         memoryTrace,
		EmbeddingCallTrace:  embeddingCallTrace,
		EmbeddingUsageTrace: embeddingUsageTrace,
		Timeline:            timeline,
		Journal:             journal,
		FixtureDir:          fixtureDir,
		MemoryStores:        stores,
		MemoryIndexer:       indexer,
		MemoryAuditLog:      memoryAuditLog,
	}, nil
}

type phase5DWiring struct {
	deps                         fixtureDeps
	registry                     *prompt.PromptRegistry
	chatModel                    model.ChatModel
	eventLog                     *observability.EventLog
	agentTrace                   *observability.AgentTraceLog
	promptTrace                  *observability.PromptTraceLog
	llmTrace                     *observability.LLMCallLog
	usageTrace                   *observability.UsageTraceLog
	structuredTrace              *observability.StructuredOutputTraceLog
	memoryWriter                 memory.DefaultMemoryWriter
	retriever                    memory.HybridMemoryRetriever
	memoryTrace                  *observability.MemoryTraceLog
	financeEngine                finance.Engine
	verificationPipelineOverride func(base verification.Pipeline) verification.Pipeline
}

func buildPhase5DStepBus(w phase5DWiring) (agents.SystemStepBus, error) {
	memoryService := memory.WorkflowMemoryService{
		Writer:               w.memoryWriter,
		Retriever:            w.retriever,
		PlannerQueryBuilder:  memory.PlannerMemoryQueryBuilder{},
		CashflowQueryBuilder: memory.CashflowMemoryQueryBuilder{},
		TraceRecorder:        w.memoryTrace,
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
			CorrelationID: "phase-5d-memory-gate",
		},
		Now: w.deps.LedgerAdapter.Now,
	}
	reportService := reporting.Service{
		MonthlyReviewAggregator: reporting.MonthlyReviewAggregator{
			TaxSignals: tools.ComputeTaxSignalTool{},
			Engine:     w.financeEngine,
			Now:        w.deps.LedgerAdapter.Now,
		},
		DebtDecisionAggregator:       reporting.DebtDecisionAggregator{Now: w.deps.LedgerAdapter.Now},
		LifeEventAggregator:          reporting.LifeEventAssessmentAggregator{Now: w.deps.LedgerAdapter.Now},
		TaxOptimizationAggregator:    reporting.TaxOptimizationAggregator{Now: w.deps.LedgerAdapter.Now},
		PortfolioRebalanceAggregator: reporting.PortfolioRebalanceAggregator{Now: w.deps.LedgerAdapter.Now},
		Artifacts: reporting.ArtifactService{
			Tool:     tools.GenerateTaskArtifactTool{},
			Producer: reporting.StaticArtifactProducer{Now: w.deps.LedgerAdapter.Now},
			Now:      w.deps.LedgerAdapter.Now,
		},
	}
	verificationPipeline := verification.Pipeline{
		CoverageChecker:        verification.DefaultEvidenceCoverageChecker{},
		DeterministicValidator: verification.MonthlyReviewDeterministicValidator{},
		BusinessValidator:      verification.MonthlyReviewBusinessValidator{},
		GroundingValidator:     verification.FinancialGroundingValidator{},
		NumericValidator:       verification.FinancialNumericConsistencyValidator{},
		TrustBusinessValidator: verification.TrustBusinessRuleValidator{},
		SuccessChecker:         verification.DefaultSuccessCriteriaChecker{},
		Oracle:                 verification.BaselineTrajectoryOracle{},
		Now:                    w.deps.LedgerAdapter.Now,
	}
	if w.verificationPipelineOverride != nil {
		verificationPipeline = w.verificationPipelineOverride(verificationPipeline)
	}
	approvalService := governance.ApprovalService{
		Classifier:   governance.DefaultRiskClassifier{},
		Decider:      governance.ApprovalDecider{},
		PolicyEngine: governance.StaticPolicyEngine{},
		ApprovalPolicy: governance.ApprovalPolicy{
			Name:          "trustworthy-finance-5d",
			MinRiskLevel:  governance.ActionRiskHigh,
			RequiredRoles: []string{"operator"},
			AutoApprove:   false,
		},
		ReportPolicy: governance.ReportDisclosurePolicy{Audience: "operator", AllowPII: false},
	}
	renderer := prompt.PromptRenderer{
		Registry:      w.registry,
		TraceRecorder: w.promptTrace,
	}
	structuredGenerator := model.DefaultStructuredGenerator{Model: w.chatModel}
	plannerEngine := planning.ProviderBackedPlanner{
		PromptRenderer: renderer,
		Generator:      structuredGenerator,
		TraceRecorder:  w.structuredTrace,
		CatalogBuilder: planning.CandidatePlanCatalogBuilder{
			Planner: &planning.DeterministicPlanner{Now: w.deps.LedgerAdapter.Now},
		},
		Compiler: planning.PlanCompiler{},
		Fallback: planning.DeterministicFallbackPlanner{
			Planner: &planning.DeterministicPlanner{Now: w.deps.LedgerAdapter.Now},
		},
		Now: w.deps.LedgerAdapter.Now,
	}
	cashflowReasoner := agents.ProviderBackedCashflowReasoner{
		Base:           agents.DeterministicCashflowReasoner{Engine: w.financeEngine},
		PromptRenderer: renderer,
		Generator:      structuredGenerator,
		TraceRecorder:  w.structuredTrace,
	}
	registry := agents.NewInMemoryAgentRegistry()
	registered := []agents.RegisteredSystemAgent{
		agents.PlannerAgentHandler{
			Assembler: contextview.DefaultContextAssembler{Estimator: contextview.HeuristicTokenEstimator{}},
			Planner:   plannerEngine,
		},
		agents.MemoryStewardHandler{Service: memoryService},
		agents.CashflowAgentHandler{
			Engine:   w.financeEngine,
			Reasoner: cashflowReasoner,
		},
		agents.DebtAgentHandler{Engine: w.financeEngine},
		agents.TaxAgentHandler{Engine: w.financeEngine},
		agents.PortfolioAgentHandler{Engine: w.financeEngine},
		agents.ReportDraftAgentHandler{Service: reportService},
		agents.ReportFinalizeAgentHandler{Service: reportService},
		agents.VerificationAgentHandler{Pipeline: verificationPipeline},
		agents.GovernanceAgentHandler{Service: approvalService},
	}
	for _, agent := range registered {
		if err := registry.Register(agent); err != nil {
			return nil, err
		}
	}
	dispatcher := agents.NewLocalAgentDispatcher(agents.LocalDispatcherOptions{
		Registry:   registry,
		Executor:   agents.LocalAgentExecutor{},
		AgentTrace: w.agentTrace,
		EventLog:   w.eventLog,
		Now:        w.deps.LedgerAdapter.Now,
	})
	return agents.NewLocalSystemStepBus(agents.SystemStepBusOptions{
		Dispatcher: dispatcher,
		Now:        w.deps.LedgerAdapter.Now,
	}), nil
}

func (e *Phase5DEnvironment) RunMonthlyReview(ctx context.Context, userID string, rawInput string, current state.FinancialWorldState) (MonthlyReview5DRunOutput, error) {
	result, err := e.MonthlyReview.Run(ctx, userID, rawInput, current)
	if err != nil {
		return MonthlyReview5DRunOutput{}, err
	}
	trace := e.buildTrace(result.WorkflowID, result.Verification, result.Report.MetricRecords, result.ApprovalAudit)
	return MonthlyReview5DRunOutput{Result: result, Trace: trace}, nil
}

func (e *Phase5DEnvironment) RunDebtVsInvest(ctx context.Context, userID string, rawInput string, current state.FinancialWorldState) (DebtVsInvest5DRunOutput, error) {
	result, err := e.DebtVsInvest.Run(ctx, userID, rawInput, current)
	if err != nil {
		return DebtVsInvest5DRunOutput{}, err
	}
	trace := e.buildTrace(result.WorkflowID, result.Verification, result.Report.MetricRecords, result.ApprovalAudit)
	return DebtVsInvest5DRunOutput{Result: result, Trace: trace}, nil
}

func (e *Phase5DEnvironment) ResumeDebtVsInvestAfterApproval(
	ctx context.Context,
	result workflows.DebtDecisionRunResult,
) (DebtVsInvest5DRunOutput, error) {
	if result.Checkpoint == nil || result.ResumeToken == nil {
		return DebtVsInvest5DRunOutput{}, fmt.Errorf("debt-vs-invest result does not contain approval resume anchors")
	}
	resumed, err := e.DebtVsInvest.ResumeAfterApproval(
		ctx,
		result.TaskSpec,
		runtimepkg.FollowUpActivationContext{
			RootCorrelationID: result.WorkflowID,
			ParentGraphID:     result.WorkflowID,
			TriggeredByTaskID: result.TaskSpec.ID,
		},
		result.UpdatedState,
		*result.Checkpoint,
		*result.ResumeToken,
		result.DraftPayload,
		result.DisclosureDecision,
	)
	if err != nil {
		return DebtVsInvest5DRunOutput{}, err
	}
	trace := e.buildTrace(resumed.WorkflowID, resumed.Verification, resumed.Report.MetricRecords, resumed.ApprovalAudit)
	return DebtVsInvest5DRunOutput{Result: resumed, Trace: trace}, nil
}

func (e *Phase5DEnvironment) RebuildMemoryIndexes(ctx context.Context) (memory.IndexBuildSummary, error) {
	if e.MemoryIndexer == nil {
		return memory.IndexBuildSummary{}, fmt.Errorf("phase 5d environment has no memory indexer")
	}
	return e.MemoryIndexer.RebuildIndexes(ctx)
}

func (e *Phase5DEnvironment) Close() error {
	if e == nil || e.MemoryStores == nil || e.MemoryStores.DB == nil {
		return nil
	}
	return e.MemoryStores.DB.Close()
}

func (e *Phase5DEnvironment) buildTrace(
	workflowID string,
	verificationResults []verification.VerificationResult,
	metricRecords []finance.MetricRecord,
	approvalAudit *governance.AuditEvent,
) observability.WorkflowTraceDump {
	var policyRecords []observability.PolicyDecisionRecord
	if approvalAudit != nil {
		policyRecords = governance.ToObservabilityRecords([]governance.AuditEvent{*approvalAudit})
	}
	return observability.BuildWorkflowTraceDumpWithTrust(
		workflowID,
		workflowID,
		e.Timeline.Records(),
		e.Journal.Records(),
		e.AgentTrace.Records(),
		e.EventLog.Entries(),
		observability.MemoryAccessRecords(e.MemoryAuditLog.Entries()),
		e.MemoryTrace.QueryRecords(),
		e.MemoryTrace.RetrievalRecords(),
		e.MemoryTrace.SelectionRecords(),
		e.EmbeddingCallTrace.Records(),
		e.EmbeddingUsageTrace.Records(),
		policyRecords,
		e.PromptTrace.Records(),
		e.LLMTrace.Records(),
		e.UsageTrace.Records(),
		e.StructuredTrace.Records(),
		observability.TrustTraceBundle{
			FinanceMetrics:            metricRecords,
			GroundingVerdicts:         observability.FilterVerificationResultsByCategory(verificationResults, verification.ValidationCategoryGrounding),
			NumericValidationVerdicts: observability.FilterVerificationResultsByCategory(verificationResults, verification.ValidationCategoryNumeric),
			BusinessRuleVerdicts:      observability.FilterVerificationResultsByCategory(verificationResults, verification.ValidationCategoryBusiness),
			PolicyRuleHits:            observability.PolicyRuleHitsFromDecisions(policyRecords),
			ApprovalTriggers:          observability.ApprovalTriggersFromDecisions(policyRecords),
		},
	)
}

func (o MonthlyReview5DRunOutput) WriteArtifact(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(o.Result.Report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func (o MonthlyReview5DRunOutput) WriteTrace(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(o.Trace, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func (o DebtVsInvest5DRunOutput) WriteArtifact(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(o.Result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func (o DebtVsInvest5DRunOutput) WriteTrace(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(o.Trace, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func valueOrDefault[T any](value *T, fallback *T) *T {
	if value != nil {
		return value
	}
	return fallback
}
