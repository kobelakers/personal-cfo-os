package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/agents"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
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

type MonthlyReview5BOptions struct {
	FixtureDir       string
	HoldingsFixture  string
	Now              func() time.Time
	ChatModel        model.ChatModel
	ChatModelFactory func(callRecorder model.CallRecorder, usageRecorder model.UsageRecorder) model.ChatModel
	PromptRegistry   *prompt.PromptRegistry
	EventLog         *observability.EventLog
	AgentTrace       *observability.AgentTraceLog
	PromptTrace      *observability.PromptTraceLog
	LLMTrace         *observability.LLMCallLog
	UsageTrace       *observability.UsageTraceLog
	StructuredTrace  *observability.StructuredOutputTraceLog
}

type MonthlyReview5BEnvironment struct {
	Workflow        workflows.MonthlyReviewWorkflow
	EventLog        *observability.EventLog
	AgentTrace      *observability.AgentTraceLog
	PromptTrace     *observability.PromptTraceLog
	LLMTrace        *observability.LLMCallLog
	UsageTrace      *observability.UsageTraceLog
	StructuredTrace *observability.StructuredOutputTraceLog
	Timeline        *runtimepkg.WorkflowTimeline
	Journal         *runtimepkg.CheckpointJournal
	FixtureDir      string
}

func OpenMonthlyReview5BEnvironment(options MonthlyReview5BOptions) (*MonthlyReview5BEnvironment, error) {
	nowFn := options.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	fixtureDir := options.FixtureDir
	if fixtureDir == "" {
		fixtureDir = filepath.Join("tests", "fixtures")
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
	eventLog := options.EventLog
	if eventLog == nil {
		eventLog = &observability.EventLog{}
	}
	agentTrace := options.AgentTrace
	if agentTrace == nil {
		agentTrace = &observability.AgentTraceLog{}
	}
	promptTrace := options.PromptTrace
	if promptTrace == nil {
		promptTrace = &observability.PromptTraceLog{}
	}
	llmTrace := options.LLMTrace
	if llmTrace == nil {
		llmTrace = &observability.LLMCallLog{}
	}
	usageTrace := options.UsageTrace
	if usageTrace == nil {
		usageTrace = &observability.UsageTraceLog{}
	}
	structuredTrace := options.StructuredTrace
	if structuredTrace == nil {
		structuredTrace = &observability.StructuredOutputTraceLog{}
	}
	chatModel := options.ChatModel
	if options.ChatModelFactory != nil {
		chatModel = options.ChatModelFactory(llmTrace, usageTrace)
	} else if chatModel == nil {
		chatModel = NewMockMonthlyReviewChatModelWithTrace(llmTrace, usageTrace)
	}
	timeline := &runtimepkg.WorkflowTimeline{}
	journal := &runtimepkg.CheckpointJournal{}
	systemSteps, err := buildMonthlyReview5BStepBus(monthlyReview5BWiring{
		deps:            deps,
		registry:        registry,
		chatModel:       chatModel,
		eventLog:        eventLog,
		agentTrace:      agentTrace,
		promptTrace:     promptTrace,
		llmTrace:        llmTrace,
		usageTrace:      usageTrace,
		structuredTrace: structuredTrace,
	})
	if err != nil {
		return nil, err
	}
	workflow := workflows.MonthlyReviewWorkflow{
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
		Runtime: runtimepkg.NewLocalWorkflowRuntime("monthly-review-5b", runtimepkg.LocalRuntimeOptions{
			EventLog: eventLog,
			Timeline: timeline,
			Journal:  journal,
			Now:      deps.LedgerAdapter.Now,
		}),
		Now: deps.LedgerAdapter.Now,
	}
	return &MonthlyReview5BEnvironment{
		Workflow:        workflow,
		EventLog:        eventLog,
		AgentTrace:      agentTrace,
		PromptTrace:     promptTrace,
		LLMTrace:        llmTrace,
		UsageTrace:      usageTrace,
		StructuredTrace: structuredTrace,
		Timeline:        timeline,
		Journal:         journal,
		FixtureDir:      fixtureDir,
	}, nil
}

type MonthlyReview5BRunOutput struct {
	Result workflows.MonthlyReviewRunResult `json:"result"`
	Trace  observability.WorkflowTraceDump  `json:"trace"`
}

func (e *MonthlyReview5BEnvironment) Run(ctx context.Context, userID string, rawInput string, current state.FinancialWorldState) (MonthlyReview5BRunOutput, error) {
	result, err := e.Workflow.Run(ctx, userID, rawInput, current)
	if err != nil {
		return MonthlyReview5BRunOutput{}, err
	}
	trace := observability.BuildWorkflowTraceDumpWithIntelligence(
		result.WorkflowID,
		result.WorkflowID,
		e.Timeline.Records(),
		e.Journal.Records(),
		e.AgentTrace.Records(),
		e.EventLog.Entries(),
		nil,
		nil,
		e.PromptTrace.Records(),
		e.LLMTrace.Records(),
		e.UsageTrace.Records(),
		e.StructuredTrace.Records(),
	)
	return MonthlyReview5BRunOutput{Result: result, Trace: trace}, nil
}

func (o MonthlyReview5BRunOutput) WriteArtifact(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(o.Result.Report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func (o MonthlyReview5BRunOutput) WriteTrace(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(o.Trace, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

type monthlyReview5BWiring struct {
	deps            fixtureDeps
	registry        *prompt.PromptRegistry
	chatModel       model.ChatModel
	eventLog        *observability.EventLog
	agentTrace      *observability.AgentTraceLog
	promptTrace     *observability.PromptTraceLog
	llmTrace        *observability.LLMCallLog
	usageTrace      *observability.UsageTraceLog
	structuredTrace *observability.StructuredOutputTraceLog
}

func buildMonthlyReview5BStepBus(w monthlyReview5BWiring) (agents.SystemStepBus, error) {
	memoryService := memory.WorkflowMemoryService{
		Writer:    w.deps.Writer,
		Retriever: w.deps.Retriever,
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
			CorrelationID: "monthly-review-5b-memory-gate",
		},
		Now: w.deps.LedgerAdapter.Now,
	}
	reportService := reporting.Service{
		MonthlyReviewAggregator: reporting.MonthlyReviewAggregator{
			TaxSignals: tools.ComputeTaxSignalTool{},
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
		SuccessChecker:         verification.DefaultSuccessCriteriaChecker{},
		Oracle:                 verification.BaselineTrajectoryOracle{},
		Now:                    w.deps.LedgerAdapter.Now,
	}
	approvalService := governance.ApprovalService{
		Classifier:   governance.DefaultRiskClassifier{},
		Decider:      governance.ApprovalDecider{},
		PolicyEngine: governance.StaticPolicyEngine{},
		ApprovalPolicy: governance.ApprovalPolicy{
			Name:          "monthly-review-5b",
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
	structuredGenerator := model.DefaultStructuredGenerator{
		Model: w.chatModel,
	}
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
		Base:           agents.DeterministicCashflowReasoner{MetricsTool: tools.ComputeCashflowMetricsTool{}},
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
			MetricsTool: tools.ComputeCashflowMetricsTool{},
			Reasoner:    cashflowReasoner,
		},
		agents.DebtAgentHandler{MetricsTool: tools.ComputeDebtDecisionMetricsTool{}},
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
