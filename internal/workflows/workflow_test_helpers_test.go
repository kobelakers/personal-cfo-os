package workflows

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/agents"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/reducers"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type phase2Deps struct {
	LedgerAdapter        observation.LedgerObservationAdapter
	StructuredDocAdapter observation.StructuredDocumentObservationAdapter
	AgenticDocAdapter    observation.AgenticDocumentObservationAdapterStub
	Store                *memory.InMemoryMemoryStore
	Writer               memory.DefaultMemoryWriter
	Retriever            memory.HybridMemoryRetriever
	EventLog             *observability.EventLog
	AgentTrace           *observability.AgentTraceLog
	Now                  time.Time
}

func buildPhase2Deps(t *testing.T, holdingsCSV string, includeDebt bool, includeTax bool) phase2Deps {
	t.Helper()
	now := time.Date(2026, 3, 28, 14, 0, 0, 0, time.UTC)
	transactions, err := observation.LoadLedgerTransactionsCSV(readWorkflowFixture(t, "ledger_transactions_2026-03.csv"))
	if err != nil {
		t.Fatalf("load transactions: %v", err)
	}
	holdings, err := observation.LoadHoldingRecordsCSV([]byte(holdingsCSV))
	if err != nil {
		t.Fatalf("load holdings: %v", err)
	}
	var debts []observation.DebtRecord
	if includeDebt {
		debts, err = observation.LoadDebtRecordsCSV(readWorkflowFixture(t, "debts_2026-03.csv"))
		if err != nil {
			t.Fatalf("load debts: %v", err)
		}
	}

	store := memory.NewInMemoryMemoryStore()
	auditLog := &memory.MemoryAccessAuditLog{}
	ledgerAdapter := observation.LedgerObservationAdapter{
		Transactions: transactions,
		Debts:        debts,
		Holdings:     holdings,
		Now:          func() time.Time { return now },
	}
	structuredAdapter := observation.StructuredDocumentObservationAdapter{
		Artifacts: []observation.DocumentArtifact{
			{
				ID:         "doc-payslip",
				UserID:     "user-1",
				Kind:       observation.DocumentKindPayslip,
				Filename:   "payslip_2026-03.csv",
				MediaType:  "text/csv",
				Content:    readWorkflowFixture(t, "payslip_2026-03.csv"),
				ObservedAt: now,
			},
			{
				ID:         "doc-credit-card",
				UserID:     "user-1",
				Kind:       observation.DocumentKindCreditCardStatement,
				Filename:   "credit_card_2026-03.csv",
				MediaType:  "text/csv",
				Content:    readWorkflowFixture(t, "credit_card_2026-03.csv"),
				ObservedAt: now,
			},
		},
	}
	agenticArtifacts := []observation.DocumentArtifact{}
	if includeTax {
		agenticArtifacts = append(agenticArtifacts, observation.DocumentArtifact{
			ID:         "doc-tax",
			UserID:     "user-1",
			Kind:       observation.DocumentKindTaxDocument,
			Filename:   "tax_2026.txt",
			MediaType:  "text/plain",
			Content:    readWorkflowFixture(t, "tax_2026.txt"),
			ObservedAt: now,
		})
	}
	agenticAdapter := observation.AgenticDocumentObservationAdapterStub{Artifacts: agenticArtifacts}
	writer := memory.DefaultMemoryWriter{
		Store:                      store,
		AuditLog:                   auditLog,
		MinConfidence:              0.7,
		LowConfidenceEpisodicFloor: 0.55,
	}
	retriever := memory.HybridMemoryRetriever{
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
	}
	return phase2Deps{
		LedgerAdapter:        ledgerAdapter,
		StructuredDocAdapter: structuredAdapter,
		AgenticDocAdapter:    agenticAdapter,
		Store:                store,
		Writer:               writer,
		Retriever:            retriever,
		EventLog:             &observability.EventLog{},
		AgentTrace:           &observability.AgentTraceLog{},
		Now:                  now,
	}
}

func buildMonthlyReviewWorkflow(t *testing.T, deps phase2Deps) MonthlyReviewWorkflow {
	t.Helper()
	return MonthlyReviewWorkflow{
		Intake: taskspecIntake(deps.Now),
		ReviewService: MonthlyReviewService{
			QueryTransaction: tools.QueryTransactionTool{Adapter: deps.LedgerAdapter},
			QueryLiability:   tools.QueryLiabilityTool{Adapter: deps.LedgerAdapter},
			QueryPortfolio:   tools.QueryPortfolioTool{LedgerAdapter: deps.LedgerAdapter},
			ParseDocument: tools.ParseDocumentTool{
				Structured: deps.StructuredDocAdapter,
				Agentic:    deps.AgenticDocAdapter,
			},
			ReducerEngine: reducers.DeterministicReducerEngine{Now: func() time.Time { return deps.Now }},
		},
		SystemSteps: buildSystemStepBus(t, deps, governance.MemoryWritePolicy{
			MinConfidence:   0.7,
			RequireEvidence: false,
			AllowKinds: []memory.MemoryKind{
				memory.MemoryKindEpisodic,
				memory.MemoryKindSemantic,
				memory.MemoryKindProcedural,
			},
		}),
		Now: func() time.Time { return deps.Now },
	}
}

func buildDebtWorkflow(t *testing.T, deps phase2Deps) DebtVsInvestWorkflow {
	t.Helper()
	return DebtVsInvestWorkflow{
		Intake: taskspecIntake(deps.Now),
		DecisionService: DebtVsInvestService{
			QueryTransaction: tools.QueryTransactionTool{Adapter: deps.LedgerAdapter},
			QueryLiability:   tools.QueryLiabilityTool{Adapter: deps.LedgerAdapter},
			QueryPortfolio:   tools.QueryPortfolioTool{LedgerAdapter: deps.LedgerAdapter},
			ReducerEngine:    reducers.DeterministicReducerEngine{Now: func() time.Time { return deps.Now }},
		},
		SystemSteps: buildSystemStepBus(t, deps, governance.MemoryWritePolicy{
			MinConfidence:   0.7,
			RequireEvidence: false,
			AllowKinds: []memory.MemoryKind{
				memory.MemoryKindEpisodic,
				memory.MemoryKindSemantic,
				memory.MemoryKindProcedural,
			},
		}),
		Now: func() time.Time { return deps.Now },
	}
}

func buildSystemStepBus(t *testing.T, deps phase2Deps, memoryWritePolicy governance.MemoryWritePolicy) agents.SystemStepBus {
	t.Helper()
	memoryService := memory.WorkflowMemoryService{
		Writer:    deps.Writer,
		Retriever: deps.Retriever,
		Gate: governance.MemoryWriteGateService{
			PolicyEngine:  governance.StaticPolicyEngine{},
			Policy:        memoryWritePolicy,
			CorrelationID: "agent-memory-gate",
		},
		Now: func() time.Time { return deps.Now },
	}
	reportService := reporting.Service{
		MonthlyReviewBuilder: reporting.MonthlyReviewDraftBuilder{
			Skill:           skills.MonthlyReviewSkill{},
			CashflowMetrics: tools.ComputeCashflowMetricsTool{},
			TaxSignals:      tools.ComputeTaxSignalTool{},
			Now:             func() time.Time { return deps.Now },
		},
		DebtDecisionBuilder: reporting.DebtDecisionDraftBuilder{
			Skill:          skills.DebtOptimizationSkill{},
			ComputeMetrics: tools.ComputeDebtDecisionMetricsTool{},
			Now:            func() time.Time { return deps.Now },
		},
		Artifacts: reporting.ArtifactService{
			Tool:     tools.GenerateTaskArtifactTool{},
			Producer: reporting.StaticArtifactProducer{Now: func() time.Time { return deps.Now }},
			Now:      func() time.Time { return deps.Now },
		},
	}
	verificationPipeline := verification.Pipeline{
		CoverageChecker:        verification.DefaultEvidenceCoverageChecker{},
		DeterministicValidator: verification.MonthlyReviewDeterministicValidator{},
		BusinessValidator:      verification.MonthlyReviewBusinessValidator{},
		SuccessChecker:         verification.DefaultSuccessCriteriaChecker{},
		Oracle:                 verification.BaselineTrajectoryOracle{},
		Now:                    func() time.Time { return deps.Now },
	}
	approvalService := governance.ApprovalService{
		Classifier:   governance.DefaultRiskClassifier{},
		Decider:      governance.ApprovalDecider{},
		PolicyEngine: governance.StaticPolicyEngine{},
		ApprovalPolicy: governance.ApprovalPolicy{
			Name:          "system-agent-approval",
			MinRiskLevel:  governance.ActionRiskHigh,
			RequiredRoles: []string{"operator"},
			AutoApprove:   false,
		},
		ReportPolicy: governance.ReportDisclosurePolicy{Audience: "user", AllowPII: false},
	}

	registry := agents.NewInMemoryAgentRegistry()
	registered := []agents.RegisteredSystemAgent{
		agents.PlannerAgentHandler{
			Assembler: contextview.DefaultContextAssembler{},
			Planner:   &planning.DeterministicPlanner{Now: func() time.Time { return deps.Now }},
		},
		agents.MemoryStewardHandler{Service: memoryService},
		agents.ReportDraftAgentHandler{Service: reportService},
		agents.ReportFinalizeAgentHandler{Service: reportService},
		agents.VerificationAgentHandler{Pipeline: verificationPipeline},
		agents.GovernanceAgentHandler{Service: approvalService},
	}
	for _, agent := range registered {
		if err := registry.Register(agent); err != nil {
			t.Fatalf("register system agent: %v", err)
		}
	}

	dispatcher := agents.NewLocalAgentDispatcher(agents.LocalDispatcherOptions{
		Registry:   registry,
		Executor:   agents.LocalAgentExecutor{},
		AgentTrace: deps.AgentTrace,
		EventLog:   deps.EventLog,
		Now:        func() time.Time { return deps.Now },
	})
	return agents.NewLocalSystemStepBus(agents.SystemStepBusOptions{
		Dispatcher: dispatcher,
		Now:        func() time.Time { return deps.Now },
	})
}

func readWorkflowFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func taskspecIntake(now time.Time) taskspec.DeterministicIntakeService {
	return taskspec.DeterministicIntakeService{
		Now: func() time.Time { return now },
	}
}
