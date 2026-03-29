package context

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestDefaultContextAssemblerBuildsDifferentViews(t *testing.T) {
	assembler := DefaultContextAssembler{
		Budget: ContextBudget{
			MaxStateBlocks:   3,
			MaxMemoryBlocks:  2,
			MaxEvidenceItems: 2,
			MaxCharacters:    512,
		},
	}
	spec := taskspec.TaskSpec{
		ID:             "task-1",
		Goal:           "Monthly review",
		UserIntentType: taskspec.UserIntentMonthlyReview,
	}
	current := state.FinancialWorldState{
		UserID: "user-1",
		CashflowState: state.CashflowState{
			MonthlyInflowCents: 100000,
		},
		Version: state.StateVersion{Sequence: 2},
	}
	memories := []memory.MemoryRecord{
		{ID: "memory-1", Kind: memory.MemoryKindSemantic, Summary: "recurring subscriptions", Facts: []memory.MemoryFact{{Key: "x", Value: "y"}}, Source: memory.MemorySource{TaskID: "task-1"}, Confidence: memory.MemoryConfidence{Score: 0.9, Rationale: "direct"}},
	}
	evidence := []observation.EvidenceRecord{
		{
			ID:            "evidence-1",
			Type:          observation.EvidenceTypeTransactionBatch,
			Summary:       "transaction batch",
			Claims:        []observation.EvidenceClaim{{Subject: "cashflow", Predicate: "monthly_inflow_cents", Object: "month", ValueJSON: "100000"}},
			Source:        observation.EvidenceSource{Kind: "ledger", Adapter: "test", Reference: "r1", Provenance: "p1"},
			TimeRange:     observation.EvidenceTimeRange{ObservedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
			Confidence:    observation.EvidenceConfidence{Score: 0.9, Reason: "test"},
			Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
		},
	}

	planningSlice, err := assembler.Assemble(spec, current, memories, evidence, ContextViewPlanning)
	if err != nil {
		t.Fatalf("assemble planning context: %v", err)
	}
	verificationSlice, err := assembler.Assemble(spec, current, memories, evidence, ContextViewVerification)
	if err != nil {
		t.Fatalf("assemble verification context: %v", err)
	}
	if planningSlice.View == verificationSlice.View {
		t.Fatalf("expected different context views")
	}
	if len(planningSlice.StateBlocks) > assembler.Budget.MaxStateBlocks {
		t.Fatalf("expected budgeted state blocks")
	}
	if len(verificationSlice.MemoryBlocks) > assembler.Budget.MaxMemoryBlocks {
		t.Fatalf("expected budgeted memory blocks")
	}
	if planningSlice.Compacted != true || verificationSlice.Compacted != true {
		t.Fatalf("expected assembled contexts to be compacted")
	}
}

func TestContextBudgetAndSelectionReasonsAreApplied(t *testing.T) {
	assembler := DefaultContextAssembler{
		Budget: ContextBudget{
			MaxStateBlocks:   2,
			MaxMemoryBlocks:  1,
			MaxEvidenceItems: 1,
			MaxCharacters:    256,
		},
	}
	spec := taskspec.TaskSpec{
		ID:             "task-2",
		Goal:           "Debt decision",
		UserIntentType: taskspec.UserIntentDebtVsInvest,
	}
	current := state.FinancialWorldState{
		UserID: "user-1",
		CashflowState: state.CashflowState{
			MonthlyInflowCents: 100000,
		},
		LiabilityState: state.LiabilityState{
			DebtBurdenRatio: 0.25,
		},
		Version: state.StateVersion{Sequence: 3},
	}
	memories := []memory.MemoryRecord{
		{ID: "memory-1", Kind: memory.MemoryKindSemantic, Summary: "debt pressure observed", Facts: []memory.MemoryFact{{Key: "x", Value: "y"}}, Source: memory.MemorySource{TaskID: "task-2"}, Confidence: memory.MemoryConfidence{Score: 0.9, Rationale: "direct"}},
		{ID: "memory-2", Kind: memory.MemoryKindPolicy, Summary: "high risk actions need approval", Facts: []memory.MemoryFact{{Key: "policy", Value: "approval"}}, Source: memory.MemorySource{TaskID: "task-2"}, Confidence: memory.MemoryConfidence{Score: 0.95, Rationale: "policy"}},
	}
	evidence := []observation.EvidenceRecord{
		{
			ID:            "evidence-1",
			Type:          observation.EvidenceTypeDebtObligationSnapshot,
			Summary:       "debt snapshot",
			Claims:        []observation.EvidenceClaim{{Subject: "debt", Predicate: "debt_burden_ratio", Object: "month", ValueJSON: "0.25"}},
			Source:        observation.EvidenceSource{Kind: "ledger", Adapter: "test", Reference: "r1", Provenance: "p1"},
			TimeRange:     observation.EvidenceTimeRange{ObservedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
			Confidence:    observation.EvidenceConfidence{Score: 0.9, Reason: "test"},
			Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
		},
		{
			ID:            "evidence-2",
			Type:          observation.EvidenceTypeTransactionBatch,
			Summary:       "cashflow batch",
			Claims:        []observation.EvidenceClaim{{Subject: "cashflow", Predicate: "monthly_inflow_cents", Object: "month", ValueJSON: "100000"}},
			Source:        observation.EvidenceSource{Kind: "ledger", Adapter: "test", Reference: "r2", Provenance: "p2"},
			TimeRange:     observation.EvidenceTimeRange{ObservedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
			Confidence:    observation.EvidenceConfidence{Score: 0.9, Reason: "test"},
			Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
		},
	}

	slice, err := assembler.Assemble(spec, current, memories, evidence, ContextViewVerification)
	if err != nil {
		t.Fatalf("assemble verification context: %v", err)
	}
	if len(slice.StateBlocks) != 2 || len(slice.MemoryBlocks) != 1 || len(slice.EvidenceBlocks) != 1 {
		t.Fatalf("expected budget limits to apply, got %+v", slice)
	}
	if slice.StateBlocks[0].SelectionReason == "" || slice.MemoryBlocks[0].BlockSource == "" || slice.EvidenceBlocks[0].SelectionReason == "" {
		t.Fatalf("expected selection reasons and block sources on all blocks")
	}
}

func TestStateAwareCompactorUsesStructuredCompaction(t *testing.T) {
	compactor := StateAwareCompactor{}
	slice := ContextSlice{
		View: ContextViewVerification,
		Budget: ContextBudget{
			MaxCharacters: 20,
		},
		StateBlocks: []InjectedStateBlock{
			{Name: "cashflow_state", DataJSON: `{"a":"0123456789"}`},
			{Name: "tax_state", DataJSON: `{"b":"0123456789"}`},
		},
		MemoryBlocks: []MemoryBlock{
			{MemoryID: "m1"},
			{MemoryID: "m2"},
		},
	}
	compacted, err := compactor.Compact(slice, CompactionStrategyVerificationLean)
	if err != nil {
		t.Fatalf("compact context: %v", err)
	}
	if len(compacted.StateBlocks) >= len(slice.StateBlocks) {
		t.Fatalf("expected state-aware compaction to trim state blocks")
	}
	if len(compacted.MemoryBlocks) != 1 {
		t.Fatalf("expected verification lean compaction to reduce memory blocks")
	}
}

func TestExecutionContextAssemblerBuildsDifferentBlockContexts(t *testing.T) {
	current := state.FinancialWorldState{
		UserID: "user-1",
		CashflowState: state.CashflowState{
			MonthlyInflowCents: 100000,
		},
		LiabilityState: state.LiabilityState{
			DebtBurdenRatio: 0.22,
		},
		RiskState: state.RiskState{
			OverallRisk: "high",
		},
		Version: state.StateVersion{Sequence: 4},
	}
	memories := []memory.MemoryRecord{
		{ID: "memory-cashflow", Kind: memory.MemoryKindSemantic, Summary: "subscription cleanup memory", Source: memory.MemorySource{TaskID: "task-1"}, Confidence: memory.MemoryConfidence{Score: 0.9, Rationale: "direct"}},
		{ID: "memory-debt", Kind: memory.MemoryKindSemantic, Summary: "debt pressure history", Source: memory.MemorySource{TaskID: "task-1"}, Confidence: memory.MemoryConfidence{Score: 0.92, Rationale: "direct"}},
	}
	evidence := []observation.EvidenceRecord{
		{
			ID:            "evidence-tx",
			Type:          observation.EvidenceTypeTransactionBatch,
			Summary:       "transactions",
			Source:        observation.EvidenceSource{Kind: "ledger", Adapter: "test", Reference: "r1", Provenance: "p1"},
			TimeRange:     observation.EvidenceTimeRange{ObservedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
			Confidence:    observation.EvidenceConfidence{Score: 0.9, Reason: "test"},
			Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
		},
		{
			ID:            "evidence-debt",
			Type:          observation.EvidenceTypeDebtObligationSnapshot,
			Summary:       "debt snapshot",
			Source:        observation.EvidenceSource{Kind: "ledger", Adapter: "test", Reference: "r2", Provenance: "p2"},
			TimeRange:     observation.EvidenceTimeRange{ObservedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
			Confidence:    observation.EvidenceConfidence{Score: 0.9, Reason: "test"},
			Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
		},
	}
	assembler := ExecutionContextAssembler{}
	cashflowContext, err := assembler.Assemble(BlockContextSpec{
		PlanID:               "plan-1",
		BlockID:              "cashflow-review",
		BlockKind:            "cashflow_review_block",
		AssignedRecipient:    "cashflow_agent",
		Goal:                 "cashflow",
		RequiredEvidenceRefs: []string{"transaction_batch"},
		RequiredStateBlocks:  []string{"cashflow_state", "risk_state"},
		ExecutionView:        ContextViewExecution,
	}, current, memories, evidence)
	if err != nil {
		t.Fatalf("assemble cashflow context: %v", err)
	}
	debtContext, err := assembler.Assemble(BlockContextSpec{
		PlanID:               "plan-1",
		BlockID:              "debt-review",
		BlockKind:            "debt_review_block",
		AssignedRecipient:    "debt_agent",
		Goal:                 "debt",
		RequiredEvidenceRefs: []string{"debt_obligation_snapshot"},
		RequiredStateBlocks:  []string{"liability_state", "risk_state"},
		ExecutionView:        ContextViewExecution,
	}, current, memories, evidence)
	if err != nil {
		t.Fatalf("assemble debt context: %v", err)
	}
	if cashflowContext.BlockKind == debtContext.BlockKind {
		t.Fatalf("expected different block kinds")
	}
	if len(cashflowContext.SelectedEvidenceIDs) == 0 || len(debtContext.SelectedEvidenceIDs) == 0 {
		t.Fatalf("expected selected evidence ids for both block contexts")
	}
	if cashflowContext.SelectedEvidenceIDs[0] == debtContext.SelectedEvidenceIDs[0] {
		t.Fatalf("expected block-specific evidence selection, got %+v and %+v", cashflowContext.SelectedEvidenceIDs, debtContext.SelectedEvidenceIDs)
	}
}

func TestVerificationContextDiffersFromExecutionContext(t *testing.T) {
	current := state.FinancialWorldState{
		UserID: "user-1",
		CashflowState: state.CashflowState{
			MonthlyInflowCents:    100000,
			MonthlyOutflowCents:   60000,
			MonthlyNetIncomeCents: 40000,
			SavingsRate:           0.4,
		},
		RiskState: state.RiskState{OverallRisk: "medium"},
		Version:   state.StateVersion{Sequence: 2},
	}
	memories := []memory.MemoryRecord{
		{ID: "memory-cashflow", Kind: memory.MemoryKindSemantic, Summary: "subscription cleanup memory", Source: memory.MemorySource{TaskID: "task-2"}, Confidence: memory.MemoryConfidence{Score: 0.9, Rationale: "direct"}},
	}
	evidence := []observation.EvidenceRecord{
		{
			ID:            "evidence-tx",
			Type:          observation.EvidenceTypeTransactionBatch,
			Summary:       "transactions",
			Source:        observation.EvidenceSource{Kind: "ledger", Adapter: "test", Reference: "r1", Provenance: "p1"},
			TimeRange:     observation.EvidenceTimeRange{ObservedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
			Confidence:    observation.EvidenceConfidence{Score: 0.9, Reason: "test"},
			Normalization: observation.EvidenceNormalizationResult{Status: observation.EvidenceNormalizationNormalized},
		},
	}
	spec := BlockContextSpec{
		PlanID:               "plan-2",
		BlockID:              "cashflow-review",
		BlockKind:            "cashflow_review_block",
		AssignedRecipient:    "cashflow_agent",
		Goal:                 "cashflow",
		RequiredEvidenceRefs: []string{"transaction_batch"},
		RequiredStateBlocks:  []string{"cashflow_state", "risk_state"},
		ExecutionView:        ContextViewExecution,
		VerificationRules:    []string{"cashflow_grounding"},
	}
	execContext, err := ExecutionContextAssembler{}.Assemble(spec, current, memories, evidence)
	if err != nil {
		t.Fatalf("assemble execution context: %v", err)
	}
	verifyContext, err := VerificationContextAssembler{}.AssembleBlock(spec, analysis.BlockResultEnvelope{
		BlockID:           "cashflow-review",
		BlockKind:         "cashflow_review_block",
		AssignedRecipient: "cashflow_agent",
		Cashflow: &analysis.CashflowBlockResult{
			BlockID: "cashflow-review",
			Summary: "cashflow ok",
			DeterministicMetrics: analysis.CashflowDeterministicMetrics{
				MonthlyInflowCents:         100000,
				MonthlyOutflowCents:        60000,
				MonthlyNetIncomeCents:      40000,
				SavingsRate:                0.4,
				DuplicateSubscriptionCount: 1,
				LateNightSpendingFrequency: 0.1,
			},
			EvidenceIDs:   []observation.EvidenceID{"evidence-tx"},
			MemoryIDsUsed: []string{"memory-cashflow"},
			Confidence:    0.9,
		},
	}, current, memories, evidence)
	if err != nil {
		t.Fatalf("assemble verification context: %v", err)
	}
	if execContext.View == verifyContext.View {
		t.Fatalf("expected execution and verification contexts to use different views")
	}
	if len(verifyContext.VerificationRules) == 0 || verifyContext.ResultSummary == "" {
		t.Fatalf("expected verification context to carry verification hooks, got %+v", verifyContext)
	}
}
