package planning

import (
	"testing"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestDeterministicPlannerCreatesMonthlyReviewBlocks(t *testing.T) {
	now := time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC)
	planner := DeterministicPlanner{Now: func() time.Time { return now }}
	spec := taskspec.TaskSpec{
		ID:             "task-1",
		UserIntentType: taskspec.UserIntentMonthlyReview,
		RequiredEvidence: []taskspec.RequiredEvidenceRef{
			{Type: "transaction_batch", Mandatory: true},
			{Type: "debt_obligation_snapshot", Mandatory: true},
			{Type: "credit_card_statement", Mandatory: false},
		},
	}
	plan := planner.CreatePlan(spec, contextview.ContextSlice{MemoryBlocks: []contextview.MemoryBlock{
		{MemoryID: "m1", Kind: memory.MemoryKindProcedural, Summary: "monthly review should always cover cashflow, debt, portfolio, tax, behavior, and risk blocks"},
	}}, "workflow-1")
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate plan: %v", err)
	}
	if len(plan.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(plan.Blocks))
	}
	if plan.Blocks[0].Kind != ExecutionBlockKindCashflowReview || plan.Blocks[1].Kind != ExecutionBlockKindDebtReview {
		t.Fatalf("unexpected block order: %+v", plan.Blocks)
	}
}

func TestDeterministicPlannerUsesDebtMemoryToReorderBlocks(t *testing.T) {
	now := time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC)
	planner := DeterministicPlanner{Now: func() time.Time { return now }}
	spec := taskspec.TaskSpec{
		ID:             "task-2",
		UserIntentType: taskspec.UserIntentDebtVsInvest,
		RequiredEvidence: []taskspec.RequiredEvidenceRef{
			{Type: "transaction_batch", Mandatory: true},
			{Type: "debt_obligation_snapshot", Mandatory: true},
			{Type: "portfolio_allocation_snapshot", Mandatory: true},
		},
	}
	plan := planner.CreatePlan(spec, contextview.ContextSlice{MemoryBlocks: []contextview.MemoryBlock{
		{MemoryID: "m-debt", Kind: memory.MemoryKindSemantic, Summary: "Debt-versus-invest decisions should include debt burden, minimum payment pressure, and liquidity coverage."},
	}}, "workflow-2")
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate plan: %v", err)
	}
	if len(plan.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(plan.Blocks))
	}
	if plan.Blocks[0].Kind != ExecutionBlockKindDebtTradeoff {
		t.Fatalf("expected debt block to be prioritized, got %+v", plan.Blocks)
	}
}

func TestExecutionPlanRejectsInvalidDependencyGraph(t *testing.T) {
	plan := ExecutionPlan{
		WorkflowID: "workflow-3",
		TaskID:     "task-3",
		CreatedAt:  time.Date(2026, 3, 29, 8, 0, 0, 0, time.UTC),
		Blocks: []ExecutionBlock{
			{
				ID:                "cashflow",
				Kind:              ExecutionBlockKindCashflowReview,
				AssignedRecipient: BlockRecipientCashflowAgent,
				Goal:              "cashflow",
				RequiredEvidenceRefs: []ExecutionBlockRequirement{
					{RequirementID: "tx", Type: "transaction_batch", Mandatory: true},
				},
				ExecutionContextView: contextview.ContextViewExecution,
				SuccessCriteria:      []ExecutionBlockSuccessCriteria{{ID: "ok", Description: "ok"}},
				VerificationHints:    []ExecutionBlockVerificationHint{{Rule: "grounding", Description: "grounding"}},
				DependsOn:            []ExecutionBlockDependency{{BlockID: "debt"}},
			},
			{
				ID:                "debt",
				Kind:              ExecutionBlockKindDebtReview,
				AssignedRecipient: BlockRecipientDebtAgent,
				Goal:              "debt",
				RequiredEvidenceRefs: []ExecutionBlockRequirement{
					{RequirementID: "debt", Type: "debt_obligation_snapshot", Mandatory: true},
				},
				ExecutionContextView: contextview.ContextViewExecution,
				SuccessCriteria:      []ExecutionBlockSuccessCriteria{{ID: "ok", Description: "ok"}},
				VerificationHints:    []ExecutionBlockVerificationHint{{Rule: "grounding", Description: "grounding"}},
				DependsOn:            []ExecutionBlockDependency{{BlockID: "cashflow"}},
			},
		},
	}
	if err := plan.Validate(); err == nil {
		t.Fatalf("expected cyclic dependency validation to fail")
	}
}
