package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/agents"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/reducers"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
	"github.com/kobelakers/personal-cfo-os/internal/workflows"
)

type RuntimePlaneOptions struct {
	DBPath     string
	FixtureDir string
	Now        func() time.Time
}

type RuntimePlane struct {
	Stores          *runtime.SQLiteRuntimeStores
	Service         *runtime.Service
	Operator        *runtime.OperatorService
	Query           *runtime.QueryService
	ReplayQuery     *runtime.ReplayQueryService
	ReplayRebuilder *runtime.ReplayProjectionRebuilder
	LifeEvent       workflows.LifeEventTriggerWorkflow
	EventLog        *observability.EventLog
	AgentTrace      *observability.AgentTraceLog
	FixtureDir      string
	runtimeDB       *runtime.SQLiteRuntimeDB
}

func OpenRuntimePlane(options RuntimePlaneOptions) (*RuntimePlane, error) {
	nowFn := options.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	fixtureDir := options.FixtureDir
	if fixtureDir == "" {
		fixtureDir = filepath.Join("tests", "fixtures")
	}
	stores, err := runtime.NewSQLiteRuntimeStores(options.DBPath)
	if err != nil {
		return nil, err
	}
	eventLog := &observability.EventLog{}
	agentTrace := &observability.AgentTraceLog{}
	deps, err := loadFixtureDeps(fixtureDir, nowFn)
	if err != nil {
		_ = stores.DB.Close()
		return nil, err
	}
	systemSteps, err := buildSystemStepBus(deps, eventLog, agentTrace)
	if err != nil {
		_ = stores.DB.Close()
		return nil, err
	}
	followUpService := workflows.FollowUpWorkflowService{
		QueryEvent:            tools.QueryEventTool{Adapter: deps.EventAdapter},
		QueryCalendarDeadline: tools.QueryCalendarDeadlineTool{Adapter: deps.DeadlineAdapter},
		QueryTransaction:      tools.QueryTransactionTool{Adapter: deps.LedgerAdapter},
		QueryPortfolio:        tools.QueryPortfolioTool{LedgerAdapter: deps.LedgerAdapter},
		ParseDocument: tools.ParseDocumentTool{
			Structured: deps.StructuredDocAdapter,
			Agentic:    deps.AgenticDocAdapter,
		},
		ReducerEngine: reducers.DeterministicReducerEngine{Now: nowFn},
		EventLog:      eventLog,
	}
	service := runtime.NewService(runtime.ServiceOptions{
		CheckpointStore: stores.Checkpoints,
		TaskGraphs:      stores.TaskGraphs,
		Executions:      stores.Executions,
		Approvals:       stores.Approvals,
		OperatorActions: stores.OperatorActions,
		Replay:          stores.Replay,
		Artifacts:       stores.Artifacts,
		Controller:      runtime.DefaultWorkflowController{},
		EventLog:        eventLog,
		Now:             nowFn,
	})
	local := service.Runtime()
	replayRebuilder := runtime.NewReplayProjectionRebuilder(service, stores.WorkflowRuns, stores.ReplayProjection, stores.Artifacts, stores.Replay, nowFn)
	service.SetReplayProjectionWriter(replayRebuilder)
	replayQuery := runtime.NewReplayQueryService(service, stores.WorkflowRuns, stores.ReplayQuery, stores.Artifacts, stores.Replay)
	lifeEventWorkflow := workflows.LifeEventTriggerWorkflow{
		Intake: taskspec.EventTriggeredIntakeService{Now: nowFn},
		TriggerService: workflows.LifeEventWorkflowService{
			QueryEvent:            tools.QueryEventTool{Adapter: deps.EventAdapter},
			QueryCalendarDeadline: tools.QueryCalendarDeadlineTool{Adapter: deps.DeadlineAdapter},
			QueryTransaction:      tools.QueryTransactionTool{Adapter: deps.LedgerAdapter},
			QueryLiability:        tools.QueryLiabilityTool{Adapter: deps.LedgerAdapter},
			QueryPortfolio:        tools.QueryPortfolioTool{LedgerAdapter: deps.LedgerAdapter},
			ParseDocument: tools.ParseDocumentTool{
				Structured: deps.StructuredDocAdapter,
				Agentic:    deps.AgenticDocAdapter,
			},
			ReducerEngine: reducers.DeterministicReducerEngine{Now: nowFn},
			EventLog:      eventLog,
		},
		SystemSteps: systemSteps,
		Runtime:     local,
		EventLog:    eventLog,
		Now:         nowFn,
	}
	taxWorkflow := workflows.TaxOptimizationWorkflow{
		Service:     followUpService,
		SystemSteps: systemSteps,
		Runtime:     local,
		EventLog:    eventLog,
		Now:         nowFn,
	}
	portfolioWorkflow := workflows.PortfolioRebalanceWorkflow{
		Service:     followUpService,
		SystemSteps: systemSteps,
		Runtime:     local,
		EventLog:    eventLog,
		Now:         nowFn,
	}
	resolver := runtime.StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentMonthlyReview:      "monthly_review_workflow",
			taskspec.UserIntentDebtVsInvest:       "debt_vs_invest_workflow",
			taskspec.UserIntentTaxOptimization:    "tax_optimization_workflow",
			taskspec.UserIntentPortfolioRebalance: "portfolio_rebalance_workflow",
		},
		Workflows: map[taskspec.UserIntentType]runtime.FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization:    workflows.TaxOptimizationWorkflowCapability{Workflow: taxWorkflow},
			taskspec.UserIntentPortfolioRebalance: workflows.PortfolioRebalanceWorkflowCapability{Workflow: portfolioWorkflow},
		},
	}
	service.SetCapabilities(resolver)
	return &RuntimePlane{
		Stores:          stores,
		Service:         service,
		Operator:        runtime.NewOperatorService(service),
		Query:           runtime.NewQueryService(service),
		ReplayQuery:     replayQuery,
		ReplayRebuilder: replayRebuilder,
		LifeEvent:       lifeEventWorkflow,
		EventLog:        eventLog,
		AgentTrace:      agentTrace,
		FixtureDir:      fixtureDir,
		runtimeDB:       stores.DB,
	}, nil
}

func (p *RuntimePlane) Close() error {
	if p == nil || p.runtimeDB == nil {
		return nil
	}
	return p.runtimeDB.Close()
}

type fixtureDeps struct {
	LedgerAdapter        observation.LedgerObservationAdapter
	StructuredDocAdapter observation.StructuredDocumentObservationAdapter
	AgenticDocAdapter    observation.AgenticDocumentObservationAdapterStub
	EventAdapter         observation.EventObservationAdapter
	DeadlineAdapter      observation.CalendarDeadlineObservationAdapter
	Writer               memory.DefaultMemoryWriter
	Retriever            memory.HybridMemoryRetriever
}

func loadFixtureDeps(fixtureDir string, nowFn func() time.Time) (fixtureDeps, error) {
	now := nowFn().UTC()
	transactionsCSV, err := readFixture(fixtureDir, "ledger_transactions_2026-03.csv")
	if err != nil {
		return fixtureDeps{}, err
	}
	transactions, err := observation.LoadLedgerTransactionsCSV(transactionsCSV)
	if err != nil {
		return fixtureDeps{}, fmt.Errorf("load transaction fixture: %w", err)
	}
	holdingsCSV, err := readFixture(fixtureDir, "holdings_2026-03.csv")
	if err != nil {
		return fixtureDeps{}, err
	}
	holdings, err := observation.LoadHoldingRecordsCSV(holdingsCSV)
	if err != nil {
		return fixtureDeps{}, fmt.Errorf("load holdings fixture: %w", err)
	}
	debtsCSV, err := readFixture(fixtureDir, "debts_2026-03.csv")
	if err != nil {
		return fixtureDeps{}, err
	}
	debts, err := observation.LoadDebtRecordsCSV(debtsCSV)
	if err != nil {
		return fixtureDeps{}, fmt.Errorf("load debt fixture: %w", err)
	}
	payslipCSV, err := readFixture(fixtureDir, "payslip_2026-03.csv")
	if err != nil {
		return fixtureDeps{}, err
	}
	creditCSV, err := readFixture(fixtureDir, "credit_card_2026-03.csv")
	if err != nil {
		return fixtureDeps{}, err
	}
	taxDoc, err := readFixture(fixtureDir, "tax_2026.txt")
	if err != nil {
		return fixtureDeps{}, err
	}
	store := memory.NewInMemoryMemoryStore()
	auditLog := &memory.MemoryAccessAuditLog{}
	return fixtureDeps{
		LedgerAdapter: observation.LedgerObservationAdapter{
			Transactions: transactions,
			Debts:        debts,
			Holdings:     holdings,
			Now:          nowFn,
		},
		StructuredDocAdapter: observation.StructuredDocumentObservationAdapter{
			Artifacts: []observation.DocumentArtifact{
				{
					ID:         "doc-payslip",
					UserID:     "user-1",
					Kind:       observation.DocumentKindPayslip,
					Filename:   "payslip_2026-03.csv",
					MediaType:  "text/csv",
					Content:    payslipCSV,
					ObservedAt: now,
				},
				{
					ID:         "doc-credit-card",
					UserID:     "user-1",
					Kind:       observation.DocumentKindCreditCardStatement,
					Filename:   "credit_card_2026-03.csv",
					MediaType:  "text/csv",
					Content:    creditCSV,
					ObservedAt: now,
				},
			},
		},
		AgenticDocAdapter: observation.AgenticDocumentObservationAdapterStub{
			Artifacts: []observation.DocumentArtifact{
				{
					ID:         "doc-tax",
					UserID:     "user-1",
					Kind:       observation.DocumentKindTaxDocument,
					Filename:   "tax_2026.txt",
					MediaType:  "text/plain",
					Content:    taxDoc,
					ObservedAt: now,
				},
			},
		},
		EventAdapter: observation.EventObservationAdapter{
			AdapterName: "local-durable-runtime-events",
			Events:      []observation.LifeEventRecord{sampleSalaryChangeEvent(now), sampleJobChangeEvent(now)},
			Now:         nowFn,
		},
		DeadlineAdapter: observation.CalendarDeadlineObservationAdapter{
			AdapterName: "local-durable-runtime-deadlines",
			Deadlines: []observation.CalendarDeadlineRecord{
				sampleSalaryDeadline(now, sampleSalaryChangeEvent(now)),
				sampleJobDeadline(now, sampleJobChangeEvent(now)),
			},
			Now: nowFn,
		},
		Writer: memory.DefaultMemoryWriter{
			Store:                      store,
			AuditLog:                   auditLog,
			MinConfidence:              0.7,
			LowConfidenceEpisodicFloor: 0.55,
		},
		Retriever: memory.HybridMemoryRetriever{
			Lexical: memory.LexicalRetriever{
				Store:    store,
				AuditLog: auditLog,
			},
			Semantic: memory.SemanticRetriever{
				Store: store,
				Backend: memory.FakeSemanticSearchBackend{
					Provider: memory.KeywordEmbeddingProvider{Dimensions: 12},
					Index:    memory.NewInMemoryVectorIndex(),
					Scorer:   memory.DefaultRetrievalScorer{},
				},
				AuditLog: auditLog,
			},
			Fusion:          memory.ReciprocalRankFusion{},
			Reranker:        memory.BaselineReranker{},
			RejectionPolicy: memory.ThresholdRejectionPolicy{MinScore: 0.01},
		},
	}, nil
}

func buildSystemStepBus(deps fixtureDeps, eventLog *observability.EventLog, agentTrace *observability.AgentTraceLog) (agents.SystemStepBus, error) {
	memoryService := memory.WorkflowMemoryService{
		Writer:    deps.Writer,
		Retriever: deps.Retriever,
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
			CorrelationID: "runtime-memory-gate",
		},
		Now: deps.LedgerAdapter.Now,
	}
	reportService := reporting.Service{
		MonthlyReviewAggregator: reporting.MonthlyReviewAggregator{
			TaxSignals: tools.ComputeTaxSignalTool{},
			Engine:     finance.DeterministicEngine{},
			Now:        deps.LedgerAdapter.Now,
		},
		DebtDecisionAggregator: reporting.DebtDecisionAggregator{
			Now: deps.LedgerAdapter.Now,
		},
		LifeEventAggregator: reporting.LifeEventAssessmentAggregator{
			Now: deps.LedgerAdapter.Now,
		},
		TaxOptimizationAggregator: reporting.TaxOptimizationAggregator{
			Now: deps.LedgerAdapter.Now,
		},
		PortfolioRebalanceAggregator: reporting.PortfolioRebalanceAggregator{
			Now: deps.LedgerAdapter.Now,
		},
		Artifacts: reporting.ArtifactService{
			Tool:     tools.GenerateTaskArtifactTool{},
			Producer: reporting.StaticArtifactProducer{Now: deps.LedgerAdapter.Now},
			Now:      deps.LedgerAdapter.Now,
		},
	}
	verificationPipeline := verification.Pipeline{
		CoverageChecker:        verification.DefaultEvidenceCoverageChecker{},
		DeterministicValidator: verification.MonthlyReviewDeterministicValidator{},
		BusinessValidator:      verification.MonthlyReviewBusinessValidator{},
		SuccessChecker:         verification.DefaultSuccessCriteriaChecker{},
		Oracle:                 verification.BaselineTrajectoryOracle{},
		Now:                    deps.LedgerAdapter.Now,
	}
	approvalService := governance.ApprovalService{
		Classifier:   governance.DefaultRiskClassifier{},
		Decider:      governance.ApprovalDecider{},
		PolicyEngine: governance.StaticPolicyEngine{},
		ApprovalPolicy: governance.ApprovalPolicy{
			Name:          "runtime-operator-plane",
			MinRiskLevel:  governance.ActionRiskHigh,
			RequiredRoles: []string{"operator"},
			AutoApprove:   false,
		},
		ReportPolicy: governance.ReportDisclosurePolicy{Audience: "operator", AllowPII: false},
	}
	registry := agents.NewInMemoryAgentRegistry()
	registered := []agents.RegisteredSystemAgent{
		agents.PlannerAgentHandler{
			Assembler: contextview.DefaultContextAssembler{},
			Planner:   &planning.DeterministicPlanner{Now: deps.LedgerAdapter.Now},
		},
		agents.MemoryStewardHandler{Service: memoryService},
		agents.CashflowAgentHandler{Engine: finance.DeterministicEngine{}},
		agents.DebtAgentHandler{Engine: finance.DeterministicEngine{}},
		agents.TaxAgentHandler{Engine: finance.DeterministicEngine{}},
		agents.PortfolioAgentHandler{Engine: finance.DeterministicEngine{}},
		agents.TaskGenerationAgentHandler{},
		agents.ReportDraftAgentHandler{Service: reportService},
		agents.ReportFinalizeAgentHandler{Service: reportService},
		agents.VerificationAgentHandler{Pipeline: verificationPipeline},
		agents.GovernanceAgentHandler{Service: approvalService},
	}
	for _, agent := range registered {
		if err := registry.Register(agent); err != nil {
			return nil, fmt.Errorf("register runtime system agent: %w", err)
		}
	}
	dispatcher := agents.NewLocalAgentDispatcher(agents.LocalDispatcherOptions{
		Registry:   registry,
		Executor:   agents.LocalAgentExecutor{},
		AgentTrace: agentTrace,
		EventLog:   eventLog,
		Now:        deps.LedgerAdapter.Now,
	})
	return agents.NewLocalSystemStepBus(agents.SystemStepBusOptions{
		Dispatcher: dispatcher,
		Now:        deps.LedgerAdapter.Now,
	}), nil
}

func readFixture(dir string, name string) ([]byte, error) {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", path, err)
	}
	return data, nil
}

func sampleSalaryChangeEvent(now time.Time) observation.LifeEventRecord {
	return observation.LifeEventRecord{
		ID:         "event-salary-follow-up",
		UserID:     "user-1",
		Kind:       observation.LifeEventSalaryChange,
		Source:     "fixture-hris",
		Provenance: "local runtime fixture salary change",
		ObservedAt: now,
		Confidence: 0.95,
		SalaryChange: &observation.SalaryChangeEventPayload{
			PreviousMonthlyIncomeCents: 1000000,
			NewMonthlyIncomeCents:      1250000,
			EffectiveAt:                now.AddDate(0, 0, -1),
		},
	}
}

func sampleSalaryDeadline(now time.Time, event observation.LifeEventRecord) observation.CalendarDeadlineRecord {
	return observation.CalendarDeadlineRecord{
		ID:               "deadline-salary-follow-up",
		UserID:           "user-1",
		Kind:             "withholding_review",
		RelatedEventID:   event.ID,
		RelatedEventKind: event.Kind,
		Source:           "fixture-calendar",
		Provenance:       "local runtime fixture salary deadline",
		ObservedAt:       now,
		DeadlineAt:       now.Add(14 * 24 * time.Hour),
		Description:      "Review payroll withholding after salary change",
		Confidence:       0.9,
	}
}

func sampleJobChangeEvent(now time.Time) observation.LifeEventRecord {
	return observation.LifeEventRecord{
		ID:         "event-job-follow-up",
		UserID:     "user-1",
		Kind:       observation.LifeEventJobChange,
		Source:     "fixture-hris",
		Provenance: "local runtime fixture job change",
		ObservedAt: now,
		Confidence: 0.94,
		JobChange: &observation.JobChangeEventPayload{
			PreviousEmployer:             "OldCo",
			NewEmployer:                  "NextCo",
			PreviousMonthlyIncomeCents:   1000000,
			NewMonthlyIncomeCents:        1400000,
			BenefitsEnrollmentDeadlineAt: now.Add(7 * 24 * time.Hour),
		},
	}
}

func sampleJobDeadline(now time.Time, event observation.LifeEventRecord) observation.CalendarDeadlineRecord {
	return observation.CalendarDeadlineRecord{
		ID:               "deadline-job-follow-up",
		UserID:           "user-1",
		Kind:             "benefits_enrollment",
		RelatedEventID:   event.ID,
		RelatedEventKind: event.Kind,
		Source:           "fixture-calendar",
		Provenance:       "local runtime fixture job deadline",
		ObservedAt:       now,
		DeadlineAt:       now.Add(7 * 24 * time.Hour),
		Description:      "Complete benefits enrollment after job change",
		Confidence:       0.9,
	}
}
