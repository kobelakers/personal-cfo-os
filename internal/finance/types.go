package finance

import (
	"time"
)

// Finance Engine is the load-bearing numeric truth source for 5D.
// It keeps deterministic metric computation separate from model reasoning so
// validators, governance, and reports all consume the same financial substrate.

type MetricValueType string

const (
	MetricValueTypeInt64   MetricValueType = "int64"
	MetricValueTypeFloat64 MetricValueType = "float64"
	MetricValueTypeBool    MetricValueType = "bool"
	MetricValueTypeString  MetricValueType = "string"
)

type MetricRecord struct {
	Ref          string          `json:"ref"`
	Domain       string          `json:"domain"`
	Name         string          `json:"name"`
	ValueType    MetricValueType `json:"value_type"`
	Int64Value   int64           `json:"int64_value,omitempty"`
	Float64Value float64         `json:"float64_value,omitempty"`
	BoolValue    bool            `json:"bool_value,omitempty"`
	StringValue  string          `json:"string_value,omitempty"`
	Unit         string          `json:"unit,omitempty"`
	AsOf         time.Time       `json:"as_of"`
	SourceRefs   []string        `json:"source_refs,omitempty"`
	EvidenceRefs []string        `json:"evidence_refs,omitempty"`
	Derivation   string          `json:"derivation,omitempty"`
}

type CashflowDeterministicMetrics struct {
	MonthlyInflowCents         int64   `json:"monthly_inflow_cents"`
	MonthlyOutflowCents        int64   `json:"monthly_outflow_cents"`
	MonthlyNetIncomeCents      int64   `json:"monthly_net_income_cents"`
	SavingsRate                float64 `json:"savings_rate"`
	DuplicateSubscriptionCount int     `json:"duplicate_subscription_count"`
	LateNightSpendingFrequency float64 `json:"late_night_spending_frequency"`
}

type DebtDeterministicMetrics struct {
	DebtBurdenRatio        float64 `json:"debt_burden_ratio"`
	MinimumPaymentPressure float64 `json:"minimum_payment_pressure"`
	AverageAPR             float64 `json:"average_apr"`
	MonthlyNetIncomeCents  int64   `json:"monthly_net_income_cents"`
	MaxAllocationDrift     float64 `json:"max_allocation_drift"`
	OverallRisk            string  `json:"overall_risk"`
}

type TaxDeterministicMetrics struct {
	EffectiveTaxRate               float64 `json:"effective_tax_rate"`
	TaxAdvantagedContributionCents int64   `json:"tax_advantaged_contribution_cents"`
	ChildcareTaxSignal             bool    `json:"childcare_tax_signal"`
	UpcomingDeadlineCount          int     `json:"upcoming_deadline_count"`
}

type PortfolioDeterministicMetrics struct {
	TotalInvestableAssetsCents int64   `json:"total_investable_assets_cents"`
	EmergencyFundMonths        float64 `json:"emergency_fund_months"`
	MaxAllocationDrift         float64 `json:"max_allocation_drift"`
	CashAllocation             float64 `json:"cash_allocation"`
}

type CashflowMetricBundle struct {
	Metrics CashflowDeterministicMetrics `json:"metrics"`
	Records []MetricRecord               `json:"records,omitempty"`
}

type DebtDecisionMetricBundle struct {
	Metrics DebtDeterministicMetrics `json:"metrics"`
	Records []MetricRecord           `json:"records,omitempty"`
}

type TaxMetricBundle struct {
	Metrics TaxDeterministicMetrics `json:"metrics"`
	Records []MetricRecord          `json:"records,omitempty"`
}

type PortfolioMetricBundle struct {
	Metrics PortfolioDeterministicMetrics `json:"metrics"`
	Records []MetricRecord                `json:"records,omitempty"`
}

func (b CashflowMetricBundle) Refs() []string     { return refsFromRecords(b.Records) }
func (b DebtDecisionMetricBundle) Refs() []string { return refsFromRecords(b.Records) }
func (b TaxMetricBundle) Refs() []string          { return refsFromRecords(b.Records) }
func (b PortfolioMetricBundle) Refs() []string    { return refsFromRecords(b.Records) }

func refsFromRecords(records []MetricRecord) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		result = append(result, record.Ref)
	}
	return result
}
