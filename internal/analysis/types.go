package analysis

import (
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
)

type RiskFlag struct {
	Code        string                   `json:"code"`
	Severity    string                   `json:"severity"`
	Detail      string                   `json:"detail"`
	EvidenceIDs []observation.EvidenceID `json:"evidence_ids,omitempty"`
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

type CashflowBlockResult struct {
	BlockID              string                       `json:"block_id"`
	Summary              string                       `json:"summary"`
	KeyFindings          []string                     `json:"key_findings,omitempty"`
	DeterministicMetrics CashflowDeterministicMetrics `json:"deterministic_metrics"`
	EvidenceIDs          []observation.EvidenceID     `json:"evidence_ids,omitempty"`
	MemoryIDsUsed        []string                     `json:"memory_ids_used,omitempty"`
	RiskFlags            []RiskFlag                   `json:"risk_flags,omitempty"`
	Recommendations      []skills.SkillItem           `json:"recommendations,omitempty"`
	Confidence           float64                      `json:"confidence"`
}

type DebtBlockResult struct {
	BlockID              string                   `json:"block_id"`
	Summary              string                   `json:"summary"`
	KeyFindings          []string                 `json:"key_findings,omitempty"`
	DeterministicMetrics DebtDeterministicMetrics `json:"deterministic_metrics"`
	EvidenceIDs          []observation.EvidenceID `json:"evidence_ids,omitempty"`
	MemoryIDsUsed        []string                 `json:"memory_ids_used,omitempty"`
	RiskFlags            []RiskFlag               `json:"risk_flags,omitempty"`
	Recommendations      []skills.SkillItem       `json:"recommendations,omitempty"`
	Confidence           float64                  `json:"confidence"`
}

type TaxBlockResult struct {
	BlockID              string                   `json:"block_id"`
	Summary              string                   `json:"summary"`
	KeyFindings          []string                 `json:"key_findings,omitempty"`
	DeterministicMetrics TaxDeterministicMetrics  `json:"deterministic_metrics"`
	EvidenceIDs          []observation.EvidenceID `json:"evidence_ids,omitempty"`
	MemoryIDsUsed        []string                 `json:"memory_ids_used,omitempty"`
	RiskFlags            []RiskFlag               `json:"risk_flags,omitempty"`
	Recommendations      []skills.SkillItem       `json:"recommendations,omitempty"`
	Confidence           float64                  `json:"confidence"`
}

type PortfolioBlockResult struct {
	BlockID              string                        `json:"block_id"`
	Summary              string                        `json:"summary"`
	KeyFindings          []string                      `json:"key_findings,omitempty"`
	DeterministicMetrics PortfolioDeterministicMetrics `json:"deterministic_metrics"`
	EvidenceIDs          []observation.EvidenceID      `json:"evidence_ids,omitempty"`
	MemoryIDsUsed        []string                      `json:"memory_ids_used,omitempty"`
	RiskFlags            []RiskFlag                    `json:"risk_flags,omitempty"`
	Recommendations      []skills.SkillItem            `json:"recommendations,omitempty"`
	Confidence           float64                       `json:"confidence"`
}

// BlockResultEnvelope is the typed handoff from domain agents into reporting and verification.
type BlockResultEnvelope struct {
	BlockID           string                `json:"block_id"`
	BlockKind         string                `json:"block_kind"`
	AssignedRecipient string                `json:"assigned_recipient"`
	Cashflow          *CashflowBlockResult  `json:"cashflow,omitempty"`
	Debt              *DebtBlockResult      `json:"debt,omitempty"`
	Tax               *TaxBlockResult       `json:"tax,omitempty"`
	Portfolio         *PortfolioBlockResult `json:"portfolio,omitempty"`
}

func (e BlockResultEnvelope) Validate() error {
	count := 0
	if e.Cashflow != nil {
		count++
		if e.BlockID == "" {
			e.BlockID = e.Cashflow.BlockID
		}
	}
	if e.Debt != nil {
		count++
		if e.BlockID == "" {
			e.BlockID = e.Debt.BlockID
		}
	}
	if e.Tax != nil {
		count++
		if e.BlockID == "" {
			e.BlockID = e.Tax.BlockID
		}
	}
	if e.Portfolio != nil {
		count++
		if e.BlockID == "" {
			e.BlockID = e.Portfolio.BlockID
		}
	}
	if count != 1 {
		return fmt.Errorf("block result envelope must contain exactly one typed result, got %d", count)
	}
	if e.BlockID == "" || e.BlockKind == "" || e.AssignedRecipient == "" {
		return fmt.Errorf("block result envelope requires block id, block kind, and assigned recipient")
	}
	switch {
	case e.Cashflow != nil && e.BlockKind != "cashflow_review_block" && e.BlockKind != "cashflow_liquidity_block":
		if e.BlockKind != "cashflow_event_impact_block" {
			return fmt.Errorf("cashflow result cannot be attached to block kind %q", e.BlockKind)
		}
	case e.Debt != nil && e.BlockKind != "debt_review_block" && e.BlockKind != "debt_tradeoff_block":
		if e.BlockKind != "debt_housing_impact_block" {
			return fmt.Errorf("debt result cannot be attached to block kind %q", e.BlockKind)
		}
	case e.Tax != nil && e.BlockKind != "tax_event_impact_block":
		return fmt.Errorf("tax result cannot be attached to block kind %q", e.BlockKind)
	case e.Portfolio != nil && e.BlockKind != "portfolio_event_impact_block":
		return fmt.Errorf("portfolio result cannot be attached to block kind %q", e.BlockKind)
	}
	return nil
}

func (e BlockResultEnvelope) Summary() string {
	switch {
	case e.Cashflow != nil:
		return e.Cashflow.Summary
	case e.Debt != nil:
		return e.Debt.Summary
	case e.Tax != nil:
		return e.Tax.Summary
	case e.Portfolio != nil:
		return e.Portfolio.Summary
	default:
		return ""
	}
}

func (e BlockResultEnvelope) EvidenceIDs() []observation.EvidenceID {
	switch {
	case e.Cashflow != nil:
		return e.Cashflow.EvidenceIDs
	case e.Debt != nil:
		return e.Debt.EvidenceIDs
	case e.Tax != nil:
		return e.Tax.EvidenceIDs
	case e.Portfolio != nil:
		return e.Portfolio.EvidenceIDs
	default:
		return nil
	}
}

func (e BlockResultEnvelope) MemoryIDsUsed() []string {
	switch {
	case e.Cashflow != nil:
		return e.Cashflow.MemoryIDsUsed
	case e.Debt != nil:
		return e.Debt.MemoryIDsUsed
	case e.Tax != nil:
		return e.Tax.MemoryIDsUsed
	case e.Portfolio != nil:
		return e.Portfolio.MemoryIDsUsed
	default:
		return nil
	}
}
