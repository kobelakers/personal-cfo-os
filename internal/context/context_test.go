package context

import (
	"testing"
	"time"

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
