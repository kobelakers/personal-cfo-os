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
