package reporting

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

func TestMonthlyReviewAggregatorRequiresDomainBlockResults(t *testing.T) {
	now := time.Date(2026, 3, 29, 9, 0, 0, 0, time.UTC)
	spec := taskspec.DeterministicIntakeService{Now: func() time.Time { return now }}.Parse("请帮我做一份月度财务复盘")
	if spec.TaskSpec == nil {
		t.Fatalf("expected monthly review task spec")
	}
	plan := sampleMonthlyReviewPlan(now, *spec.TaskSpec)
	aggregator := MonthlyReviewAggregator{
		TaxSignals: tools.ComputeTaxSignalTool{},
		Now:        func() time.Time { return now },
	}
	_, err := aggregator.Aggregate(*spec.TaskSpec, "workflow-1", DraftInput{
		Plan:         plan,
		BlockResults: []analysis.BlockResultEnvelope{sampleCashflowBlockResult()},
		CurrentState: sampleReportingState(now),
	})
	if err == nil {
		t.Fatalf("expected aggregate to fail when debt block result is missing")
	}
}

func TestDebtDecisionAggregatorUsesPlanBlockOrder(t *testing.T) {
	now := time.Date(2026, 3, 29, 9, 0, 0, 0, time.UTC)
	spec := taskspec.DeterministicIntakeService{Now: func() time.Time { return now }}.Parse("提前还贷还是继续投资更合适")
	if spec.TaskSpec == nil {
		t.Fatalf("expected debt-vs-invest task spec")
	}
	plan := planning.ExecutionPlan{
		WorkflowID: "workflow-2",
		TaskID:     spec.TaskSpec.ID,
		PlanID:     "plan-2",
		CreatedAt:  now,
		Blocks: []planning.ExecutionBlock{
			{
				ID:                "debt-tradeoff",
				Kind:              planning.ExecutionBlockKindDebtTradeoff,
				AssignedRecipient: planning.BlockRecipientDebtAgent,
				Goal:              "debt",
				RequiredEvidenceRefs: []planning.ExecutionBlockRequirement{
					{RequirementID: "debt", Type: "debt_obligation_snapshot", Mandatory: true},
				},
				ExecutionContextView: "execution",
				SuccessCriteria:      []planning.ExecutionBlockSuccessCriteria{{ID: "ok", Description: "ok"}},
				VerificationHints:    []planning.ExecutionBlockVerificationHint{{Rule: "grounding", Description: "grounding"}},
			},
			{
				ID:                "cashflow-liquidity",
				Kind:              planning.ExecutionBlockKindCashflowLiquidity,
				AssignedRecipient: planning.BlockRecipientCashflowAgent,
				Goal:              "cashflow",
				RequiredEvidenceRefs: []planning.ExecutionBlockRequirement{
					{RequirementID: "tx", Type: "transaction_batch", Mandatory: true},
				},
				ExecutionContextView: "execution",
				SuccessCriteria:      []planning.ExecutionBlockSuccessCriteria{{ID: "ok", Description: "ok"}},
				VerificationHints:    []planning.ExecutionBlockVerificationHint{{Rule: "grounding", Description: "grounding"}},
			},
		},
	}
	aggregator := DebtDecisionAggregator{Now: func() time.Time { return now }}
	report, err := aggregator.Aggregate(*spec.TaskSpec, "workflow-2", DraftInput{
		Plan: plan,
		BlockResults: []analysis.BlockResultEnvelope{
			sampleCashflowLiquidityBlockResult(),
			sampleDebtBlockResult(),
		},
		CurrentState: sampleReportingState(now),
	})
	if err != nil {
		t.Fatalf("aggregate debt decision: %v", err)
	}
	if len(report.SourceBlockIDs) != 2 || report.SourceBlockIDs[0] != "debt-tradeoff" {
		t.Fatalf("expected report to preserve plan block order, got %+v", report.SourceBlockIDs)
	}
	if report.Conclusion == "" {
		t.Fatalf("expected non-empty conclusion")
	}
}

func sampleMonthlyReviewPlan(now time.Time, spec taskspec.TaskSpec) planning.ExecutionPlan {
	return planning.ExecutionPlan{
		WorkflowID: "workflow-1",
		TaskID:     spec.ID,
		PlanID:     "plan-1",
		CreatedAt:  now,
		Blocks: []planning.ExecutionBlock{
			{
				ID:                "cashflow-review",
				Kind:              planning.ExecutionBlockKindCashflowReview,
				AssignedRecipient: planning.BlockRecipientCashflowAgent,
				Goal:              "cashflow",
				RequiredEvidenceRefs: []planning.ExecutionBlockRequirement{
					{RequirementID: "tx", Type: "transaction_batch", Mandatory: true},
				},
				ExecutionContextView: "execution",
				SuccessCriteria:      []planning.ExecutionBlockSuccessCriteria{{ID: "ok", Description: "ok"}},
				VerificationHints:    []planning.ExecutionBlockVerificationHint{{Rule: "grounding", Description: "grounding"}},
			},
			{
				ID:                "debt-review",
				Kind:              planning.ExecutionBlockKindDebtReview,
				AssignedRecipient: planning.BlockRecipientDebtAgent,
				Goal:              "debt",
				RequiredEvidenceRefs: []planning.ExecutionBlockRequirement{
					{RequirementID: "debt", Type: "debt_obligation_snapshot", Mandatory: true},
				},
				ExecutionContextView: "execution",
				SuccessCriteria:      []planning.ExecutionBlockSuccessCriteria{{ID: "ok", Description: "ok"}},
				VerificationHints:    []planning.ExecutionBlockVerificationHint{{Rule: "grounding", Description: "grounding"}},
			},
		},
	}
}

func sampleCashflowBlockResult() analysis.BlockResultEnvelope {
	return analysis.BlockResultEnvelope{
		BlockID:           "cashflow-review",
		BlockKind:         "cashflow_review_block",
		AssignedRecipient: "cashflow_agent",
		Cashflow: &analysis.CashflowBlockResult{
			BlockID: "cashflow-review",
			Summary: "现金流块结论：本月结余稳定。",
			DeterministicMetrics: analysis.CashflowDeterministicMetrics{
				MonthlyInflowCents:         1000000,
				MonthlyOutflowCents:        600000,
				MonthlyNetIncomeCents:      400000,
				SavingsRate:                0.4,
				DuplicateSubscriptionCount: 1,
				LateNightSpendingFrequency: 0.05,
			},
			EvidenceIDs:   []observation.EvidenceID{"ev-tx"},
			MemoryIDsUsed: []string{"mem-1"},
			RiskFlags: []analysis.RiskFlag{
				{Code: "subscription", Severity: "low", Detail: "有可清理订阅", EvidenceIDs: []observation.EvidenceID{"ev-tx"}},
			},
			Recommendations: []skills.SkillItem{
				{Title: "清理订阅", Detail: "先清理低使用率订阅。", EvidenceIDs: []observation.EvidenceID{"ev-tx"}},
			},
			Confidence: 0.88,
		},
	}
}

func sampleDebtBlockResult() analysis.BlockResultEnvelope {
	return analysis.BlockResultEnvelope{
		BlockID:           "debt-tradeoff",
		BlockKind:         "debt_tradeoff_block",
		AssignedRecipient: "debt_agent",
		Debt: &analysis.DebtBlockResult{
			BlockID: "debt-tradeoff",
			Summary: "债务块结论：当前债务压力可控，但需持续复核高息敞口。",
			DeterministicMetrics: analysis.DebtDeterministicMetrics{
				DebtBurdenRatio:        0.12,
				MinimumPaymentPressure: 0.07,
				AverageAPR:             0.08,
				MonthlyNetIncomeCents:  400000,
				MaxAllocationDrift:     0.02,
				OverallRisk:            "medium",
			},
			EvidenceIDs:   []observation.EvidenceID{"ev-debt"},
			MemoryIDsUsed: []string{"mem-2"},
			RiskFlags: []analysis.RiskFlag{
				{Code: "debt_caveat", Severity: "low", Detail: "债务压力可控但需持续关注。", EvidenceIDs: []observation.EvidenceID{"ev-debt"}},
			},
			Recommendations: []skills.SkillItem{
				{Title: "持续复核债务", Detail: "维持最低还款覆盖。", EvidenceIDs: []observation.EvidenceID{"ev-debt"}},
			},
			Confidence: 0.9,
		},
	}
}

func sampleCashflowLiquidityBlockResult() analysis.BlockResultEnvelope {
	result := sampleCashflowBlockResult()
	result.BlockID = "cashflow-liquidity"
	result.BlockKind = "cashflow_liquidity_block"
	result.Cashflow.BlockID = "cashflow-liquidity"
	return result
}

func sampleReportingState(now time.Time) state.FinancialWorldState {
	return state.FinancialWorldState{
		UserID: "user-1",
		RiskState: state.RiskState{
			OverallRisk: "medium",
		},
		TaxState: state.TaxState{
			ChildcareTaxSignal: true,
		},
		Version: state.StateVersion{
			Sequence:   1,
			SnapshotID: "snap-1",
			UpdatedAt:  now,
		},
	}
}
