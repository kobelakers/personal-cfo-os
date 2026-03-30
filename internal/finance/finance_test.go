package finance

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

func TestDeterministicEngineCashflowProducesTypedTruthSource(t *testing.T) {
	engine := DeterministicEngine{}
	asOf := time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC)
	current := state.FinancialWorldState{
		CashflowState: state.CashflowState{
			MonthlyInflowCents:       800000,
			MonthlyOutflowCents:      500000,
			MonthlyNetIncomeCents:    300000,
			MonthlyFixedExpenseCents: 250000,
			SavingsRate:              0.375,
		},
		LiabilityState: state.LiabilityState{
			DebtBurdenRatio:        0.25,
			MinimumPaymentPressure: 0.11,
			AverageAPR:             0.18,
		},
		PortfolioState: state.PortfolioState{
			EmergencyFundMonths: 2.5,
		},
		BehaviorState: state.BehaviorState{
			DuplicateSubscriptionCount: 2,
			LateNightSpendingFrequency: 0.16,
			RecurringSubscriptions:     []string{"a", "b", "c", "d"},
		},
	}
	evidence := []observation.EvidenceRecord{{ID: "evidence-1"}}

	bundle := engine.Cashflow(current, evidence, asOf)
	if bundle.Metrics.MonthlyNetIncomeCents != 300000 {
		t.Fatalf("expected deterministic monthly net income, got %+v", bundle.Metrics)
	}
	if len(bundle.Records) == 0 {
		t.Fatalf("expected metric records for finance provenance")
	}
	refs := bundle.Refs()
	if !containsString(refs, "emergency_fund_coverage_months") || !containsString(refs, "savings_rate_quality_score") {
		t.Fatalf("expected key finance metric refs, got %+v", refs)
	}
	record := findMetric(bundle.Records, "subscription_burden_ratio")
	if record.Ref == "" || record.Domain != "cashflow" || len(record.EvidenceRefs) == 0 {
		t.Fatalf("expected typed metric provenance, got %+v", record)
	}
}

func TestDeterministicEngineDebtDecisionProducesApprovalRelevantMetrics(t *testing.T) {
	engine := DeterministicEngine{}
	asOf := time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC)
	current := state.FinancialWorldState{
		CashflowState: state.CashflowState{
			MonthlyOutflowCents:   500000,
			MonthlyNetIncomeCents: 120000,
		},
		LiabilityState: state.LiabilityState{
			DebtBurdenRatio:        0.42,
			MinimumPaymentPressure: 0.24,
			AverageAPR:             0.19,
		},
		PortfolioState: state.PortfolioState{
			EmergencyFundMonths: 1.2,
			AllocationDrift:     map[string]float64{"equity": 0.11, "bond": -0.11},
		},
		RiskState: state.RiskState{
			OverallRisk: "high",
		},
	}

	bundle := engine.DebtDecision(current, []observation.EvidenceRecord{{ID: "evidence-1"}}, asOf)
	if findMetric(bundle.Records, "debt_payoff_pressure").Ref == "" {
		t.Fatalf("expected debt payoff pressure metric record, got %+v", bundle.Records)
	}
	if findMetric(bundle.Records, "liquidity_after_paydown").Ref == "" {
		t.Fatalf("expected liquidity-after-paydown metric record")
	}
	if findMetric(bundle.Records, "effective_tradeoff_score").Ref == "" {
		t.Fatalf("expected effective tradeoff metric record")
	}
}

func TestDeterministicEngineTaxAndPortfolioMinimalBundles(t *testing.T) {
	engine := DeterministicEngine{}
	asOf := time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC)
	current := state.FinancialWorldState{
		TaxState: state.TaxState{
			EffectiveTaxRate:               0.21,
			TaxAdvantagedContributionCents: 50000,
			ChildcareTaxSignal:             true,
			FamilyTaxNotes:                 []string{"withholding_review_required"},
			UpcomingDeadlines:              []string{"2026-04-15"},
		},
		PortfolioState: state.PortfolioState{
			TotalInvestableAssetsCents: 900000,
			EmergencyFundMonths:        2.1,
			AllocationDrift:            map[string]float64{"equity": 0.08, "bond": -0.08},
			AssetAllocations:           map[string]float64{"cash": 0.12},
		},
	}

	tax := engine.Tax(current, []observation.EvidenceRecord{{ID: "evidence-tax"}}, asOf)
	portfolio := engine.Portfolio(current, []observation.EvidenceRecord{{ID: "evidence-portfolio"}}, asOf)
	if findMetric(tax.Records, "tax_deadline_risk").Ref == "" || findMetric(tax.Records, "withholding_gap_signal").Ref == "" {
		t.Fatalf("expected minimal tax metric bundle, got %+v", tax.Records)
	}
	if findMetric(portfolio.Records, "portfolio_drift_score").Ref == "" || findMetric(portfolio.Records, "portfolio_liquidity_impact").Ref == "" {
		t.Fatalf("expected minimal portfolio metric bundle, got %+v", portfolio.Records)
	}
}

func findMetric(records []MetricRecord, ref string) MetricRecord {
	for _, record := range records {
		if record.Ref == ref {
			return record
		}
	}
	return MetricRecord{}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
