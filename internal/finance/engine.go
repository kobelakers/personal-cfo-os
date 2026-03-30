package finance

import (
	"math"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type Engine interface {
	Cashflow(current state.FinancialWorldState, evidence []observation.EvidenceRecord, asOf time.Time) CashflowMetricBundle
	DebtDecision(current state.FinancialWorldState, evidence []observation.EvidenceRecord, asOf time.Time) DebtDecisionMetricBundle
	Tax(current state.FinancialWorldState, evidence []observation.EvidenceRecord, asOf time.Time) TaxMetricBundle
	Portfolio(current state.FinancialWorldState, evidence []observation.EvidenceRecord, asOf time.Time) PortfolioMetricBundle
}

type DeterministicEngine struct{}

func (DeterministicEngine) Cashflow(current state.FinancialWorldState, evidence []observation.EvidenceRecord, asOf time.Time) CashflowMetricBundle {
	metrics := CashflowDeterministicMetrics{
		MonthlyInflowCents:         current.CashflowState.MonthlyInflowCents,
		MonthlyOutflowCents:        current.CashflowState.MonthlyOutflowCents,
		MonthlyNetIncomeCents:      current.CashflowState.MonthlyNetIncomeCents,
		SavingsRate:                current.CashflowState.SavingsRate,
		DuplicateSubscriptionCount: current.BehaviorState.DuplicateSubscriptionCount,
		LateNightSpendingFrequency: current.BehaviorState.LateNightSpendingFrequency,
	}
	savingsQuality := clamp01(metrics.SavingsRate / 0.20)
	debtPressure := debtPressureScore(current)
	emergencyMonths := current.PortfolioState.EmergencyFundMonths
	subscriptionBurden := 0.0
	if len(current.BehaviorState.RecurringSubscriptions) > 0 {
		subscriptionBurden = float64(metrics.DuplicateSubscriptionCount) / float64(len(current.BehaviorState.RecurringSubscriptions))
	} else if metrics.DuplicateSubscriptionCount > 0 {
		subscriptionBurden = math.Min(float64(metrics.DuplicateSubscriptionCount)/3.0, 1.0)
	}
	recurringSignal := 0.0
	if current.CashflowState.MonthlyOutflowCents > 0 {
		recurringSignal = clamp01(float64(current.CashflowState.MonthlyFixedExpenseCents) / float64(current.CashflowState.MonthlyOutflowCents))
	}
	records := []MetricRecord{
		intMetric("monthly_inflow_cents", "cashflow", "monthly_inflow_cents", metrics.MonthlyInflowCents, "cents", asOf, evidence, "copied from reducer cashflow state"),
		intMetric("monthly_outflow_cents", "cashflow", "monthly_outflow_cents", metrics.MonthlyOutflowCents, "cents", asOf, evidence, "copied from reducer cashflow state"),
		intMetric("monthly_net_income_cents", "cashflow", "monthly_net_income_cents", metrics.MonthlyNetIncomeCents, "cents", asOf, evidence, "monthly inflow minus outflow"),
		floatMetric("savings_rate", "cashflow", "savings_rate", metrics.SavingsRate, "ratio", asOf, evidence, "copied from reducer cashflow state"),
		floatMetric("savings_rate_quality_score", "cashflow", "savings_rate_quality_score", savingsQuality, "score", asOf, evidence, "normalized against 20% savings target"),
		floatMetric("debt_pressure_score", "cashflow", "debt_pressure_score", debtPressure, "score", asOf, evidence, "derived from debt burden, minimum payment pressure, and APR"),
		floatMetric("emergency_fund_coverage_months", "cashflow", "emergency_fund_coverage_months", emergencyMonths, "months", asOf, evidence, "copied from portfolio emergency fund coverage"),
		floatMetric("liquidity_buffer_months", "cashflow", "liquidity_buffer_months", emergencyMonths, "months", asOf, evidence, "current emergency fund coverage reused as liquidity buffer metric"),
		floatMetric("subscription_burden_ratio", "cashflow", "subscription_burden_ratio", clamp01(subscriptionBurden), "ratio", asOf, evidence, "duplicate subscriptions relative to recurring subscription set"),
		floatMetric("recurring_expense_signal", "cashflow", "recurring_expense_signal", recurringSignal, "ratio", asOf, evidence, "fixed expenses relative to monthly outflow"),
		intMetric("duplicate_subscription_count", "cashflow", "duplicate_subscription_count", int64(metrics.DuplicateSubscriptionCount), "count", asOf, evidence, "copied from reducer behavior state"),
		floatMetric("late_night_spending_frequency", "cashflow", "late_night_spending_frequency", metrics.LateNightSpendingFrequency, "ratio", asOf, evidence, "copied from reducer behavior state"),
	}
	return CashflowMetricBundle{Metrics: metrics, Records: records}
}

func (DeterministicEngine) DebtDecision(current state.FinancialWorldState, evidence []observation.EvidenceRecord, asOf time.Time) DebtDecisionMetricBundle {
	metrics := DebtDeterministicMetrics{
		DebtBurdenRatio:        current.LiabilityState.DebtBurdenRatio,
		MinimumPaymentPressure: current.LiabilityState.MinimumPaymentPressure,
		AverageAPR:             current.LiabilityState.AverageAPR,
		MonthlyNetIncomeCents:  current.CashflowState.MonthlyNetIncomeCents,
		MaxAllocationDrift:     maxAllocationDrift(current.PortfolioState.AllocationDrift),
		OverallRisk:            current.RiskState.OverallRisk,
	}
	investableSurplus := max64(metrics.MonthlyNetIncomeCents, 0)
	cashBufferImpact := 0.0
	if current.CashflowState.MonthlyOutflowCents > 0 {
		cashBufferImpact = clamp01(float64(investableSurplus) / float64(current.CashflowState.MonthlyOutflowCents))
	}
	liquidityAfterPaydown := current.PortfolioState.EmergencyFundMonths - cashBufferImpact
	if liquidityAfterPaydown < 0 {
		liquidityAfterPaydown = 0
	}
	debtPressure := debtPressureScore(current)
	effectiveTradeoff := clampSigned((clamp01(float64(investableSurplus)/float64(max64(current.CashflowState.MonthlyOutflowCents, 1))) - debtPressure))
	records := []MetricRecord{
		floatMetric("debt_burden_ratio", "debt_decision", "debt_burden_ratio", metrics.DebtBurdenRatio, "ratio", asOf, evidence, "copied from reducer liability state"),
		floatMetric("minimum_payment_pressure", "debt_decision", "minimum_payment_pressure", metrics.MinimumPaymentPressure, "ratio", asOf, evidence, "copied from reducer liability state"),
		floatMetric("average_apr", "debt_decision", "average_apr", metrics.AverageAPR, "ratio", asOf, evidence, "copied from reducer liability state"),
		intMetric("monthly_net_income_cents", "debt_decision", "monthly_net_income_cents", metrics.MonthlyNetIncomeCents, "cents", asOf, evidence, "copied from reducer cashflow state"),
		floatMetric("debt_payoff_pressure", "debt_decision", "debt_payoff_pressure", debtPressure, "score", asOf, evidence, "derived from debt burden, minimum payment pressure, and APR"),
		floatMetric("liquidity_after_paydown", "debt_decision", "liquidity_after_paydown", liquidityAfterPaydown, "months", asOf, evidence, "emergency fund months after allocating one month surplus to debt paydown"),
		intMetric("investable_surplus_cents", "debt_decision", "investable_surplus_cents", investableSurplus, "cents", asOf, evidence, "max(monthly net income, 0)"),
		floatMetric("cash_buffer_impact", "debt_decision", "cash_buffer_impact", cashBufferImpact, "ratio", asOf, evidence, "one-month surplus relative to monthly outflow"),
		floatMetric("effective_tradeoff_score", "debt_decision", "effective_tradeoff_score", effectiveTradeoff, "score", asOf, evidence, "investable surplus signal minus debt payoff pressure"),
		floatMetric("emergency_fund_coverage_months", "debt_decision", "emergency_fund_coverage_months", current.PortfolioState.EmergencyFundMonths, "months", asOf, evidence, "copied from reducer portfolio state"),
		floatMetric("max_allocation_drift", "debt_decision", "max_allocation_drift", metrics.MaxAllocationDrift, "ratio", asOf, evidence, "max absolute allocation drift"),
		stringMetric("overall_risk", "debt_decision", "overall_risk", metrics.OverallRisk, asOf, evidence, "copied from reducer risk state"),
	}
	return DebtDecisionMetricBundle{Metrics: metrics, Records: records}
}

func (DeterministicEngine) Tax(current state.FinancialWorldState, evidence []observation.EvidenceRecord, asOf time.Time) TaxMetricBundle {
	metrics := TaxDeterministicMetrics{
		EffectiveTaxRate:               current.TaxState.EffectiveTaxRate,
		TaxAdvantagedContributionCents: current.TaxState.TaxAdvantagedContributionCents,
		ChildcareTaxSignal:             current.TaxState.ChildcareTaxSignal,
		UpcomingDeadlineCount:          len(current.TaxState.UpcomingDeadlines),
	}
	records := []MetricRecord{
		floatMetric("effective_tax_rate", "tax", "effective_tax_rate", metrics.EffectiveTaxRate, "ratio", asOf, evidence, "copied from reducer tax state"),
		intMetric("tax_advantaged_contribution_cents", "tax", "tax_advantaged_contribution_cents", metrics.TaxAdvantagedContributionCents, "cents", asOf, evidence, "copied from reducer tax state"),
		boolMetric("childcare_tax_signal", "tax", "childcare_tax_signal", metrics.ChildcareTaxSignal, asOf, evidence, "copied from reducer tax state"),
		intMetric("tax_deadline_risk", "tax", "tax_deadline_risk", int64(metrics.UpcomingDeadlineCount), "count", asOf, evidence, "upcoming tax deadlines count"),
		floatMetric("withholding_gap_signal", "tax", "withholding_gap_signal", boolAsFloat(hasNote(current.TaxState.FamilyTaxNotes, "withholding_review_required")), "score", asOf, evidence, "derived from withholding review note"),
	}
	return TaxMetricBundle{Metrics: metrics, Records: records}
}

func (DeterministicEngine) Portfolio(current state.FinancialWorldState, evidence []observation.EvidenceRecord, asOf time.Time) PortfolioMetricBundle {
	metrics := PortfolioDeterministicMetrics{
		TotalInvestableAssetsCents: current.PortfolioState.TotalInvestableAssetsCents,
		EmergencyFundMonths:        current.PortfolioState.EmergencyFundMonths,
		MaxAllocationDrift:         maxAllocationDrift(current.PortfolioState.AllocationDrift),
		CashAllocation:             current.PortfolioState.AssetAllocations["cash"],
	}
	records := []MetricRecord{
		intMetric("total_investable_assets_cents", "portfolio", "total_investable_assets_cents", metrics.TotalInvestableAssetsCents, "cents", asOf, evidence, "copied from reducer portfolio state"),
		floatMetric("emergency_fund_months", "portfolio", "emergency_fund_months", metrics.EmergencyFundMonths, "months", asOf, evidence, "copied from reducer portfolio state"),
		floatMetric("portfolio_drift_score", "portfolio", "portfolio_drift_score", metrics.MaxAllocationDrift, "ratio", asOf, evidence, "max absolute allocation drift"),
		floatMetric("rebalance_pressure", "portfolio", "rebalance_pressure", metrics.MaxAllocationDrift, "ratio", asOf, evidence, "max allocation drift reused as rebalance pressure"),
		floatMetric("cash_allocation", "portfolio", "cash_allocation", metrics.CashAllocation, "ratio", asOf, evidence, "cash asset allocation"),
		floatMetric("portfolio_liquidity_impact", "portfolio", "portfolio_liquidity_impact", clamp01(1-metrics.CashAllocation), "score", asOf, evidence, "lower cash allocation increases liquidity impact of rebalance"),
	}
	return PortfolioMetricBundle{Metrics: metrics, Records: records}
}

func debtPressureScore(current state.FinancialWorldState) float64 {
	debt := current.LiabilityState.DebtBurdenRatio
	minPay := current.LiabilityState.MinimumPaymentPressure
	apr := current.LiabilityState.AverageAPR
	return clamp01(maxFloat(
		debt/0.35,
		minPay/0.20,
		apr/0.25,
	))
}

func maxAllocationDrift(items map[string]float64) float64 {
	maximum := 0.0
	for _, item := range items {
		if item < 0 {
			item = -item
		}
		if item > maximum {
			maximum = item
		}
	}
	return maximum
}

func hasNote(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func boolAsFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func clampSigned(value float64) float64 {
	if value < -1 {
		return -1
	}
	if value > 1 {
		return 1
	}
	return value
}

func maxFloat(items ...float64) float64 {
	maximum := 0.0
	for index, item := range items {
		if index == 0 || item > maximum {
			maximum = item
		}
	}
	return maximum
}

func max64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
