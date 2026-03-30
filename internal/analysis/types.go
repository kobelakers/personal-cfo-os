package analysis

import (
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type RiskFlag struct {
	Code           string                   `json:"code"`
	Severity       string                   `json:"severity"`
	Detail         string                   `json:"detail"`
	EvidenceIDs    []observation.EvidenceID `json:"evidence_ids,omitempty"`
	MetricRefs     []string                 `json:"metric_refs,omitempty"`
	MemoryRefs     []string                 `json:"memory_refs,omitempty"`
	Caveats        []string                 `json:"caveats,omitempty"`
	PolicyRuleRefs []string                 `json:"policy_rule_refs,omitempty"`
}

type RecommendationType string

const (
	RecommendationTypeCashflowAdjustment RecommendationType = "cashflow_adjustment"
	RecommendationTypeExpenseReduction   RecommendationType = "expense_reduction"
	RecommendationTypeDebtPaydown        RecommendationType = "debt_paydown"
	RecommendationTypeDebtRestructure    RecommendationType = "debt_restructure"
	RecommendationTypeInvestMore         RecommendationType = "invest_more"
	RecommendationTypePortfolioRebalance RecommendationType = "portfolio_rebalance"
	RecommendationTypeTaxAction          RecommendationType = "tax_action"
)

type Recommendation struct {
	ID               string             `json:"id,omitempty"`
	Type             RecommendationType `json:"type"`
	Title            string             `json:"title"`
	Detail           string             `json:"detail"`
	RiskLevel        taskspec.RiskLevel `json:"risk_level"`
	GroundingRefs    []string           `json:"grounding_refs,omitempty"`
	MetricRefs       []string           `json:"metric_refs,omitempty"`
	EvidenceRefs     []string           `json:"evidence_refs,omitempty"`
	MemoryRefs       []string           `json:"memory_refs,omitempty"`
	Caveats          []string           `json:"caveats,omitempty"`
	ApprovalRequired bool               `json:"approval_required,omitempty"`
	ApprovalReason   string             `json:"approval_reason,omitempty"`
	PolicyRuleRefs   []string           `json:"policy_rule_refs,omitempty"`
	Metadata         map[string]string  `json:"metadata,omitempty"`
}

type CashflowDeterministicMetrics = finance.CashflowDeterministicMetrics
type DebtDeterministicMetrics = finance.DebtDeterministicMetrics
type TaxDeterministicMetrics = finance.TaxDeterministicMetrics
type PortfolioDeterministicMetrics = finance.PortfolioDeterministicMetrics

type CashflowBlockResult struct {
	BlockID              string                       `json:"block_id"`
	Summary              string                       `json:"summary"`
	KeyFindings          []string                     `json:"key_findings,omitempty"`
	DeterministicMetrics CashflowDeterministicMetrics `json:"deterministic_metrics"`
	MetricRecords        []finance.MetricRecord       `json:"metric_records,omitempty"`
	EvidenceIDs          []observation.EvidenceID     `json:"evidence_ids,omitempty"`
	MemoryIDsUsed        []string                     `json:"memory_ids_used,omitempty"`
	MetricRefs           []string                     `json:"metric_refs,omitempty"`
	GroundingRefs        []string                     `json:"grounding_refs,omitempty"`
	RiskFlags            []RiskFlag                   `json:"risk_flags,omitempty"`
	Recommendations      []Recommendation             `json:"recommendations,omitempty"`
	Caveats              []string                     `json:"caveats,omitempty"`
	ApprovalRequired     bool                         `json:"approval_required,omitempty"`
	ApprovalReason       string                       `json:"approval_reason,omitempty"`
	PolicyRuleRefs       []string                     `json:"policy_rule_refs,omitempty"`
	Confidence           float64                      `json:"confidence"`
}

type CashflowStructuredCandidate struct {
	Summary                 string           `json:"summary"`
	KeyFindings             []string         `json:"key_findings,omitempty"`
	GroundedRecommendations []Recommendation `json:"grounded_recommendations,omitempty"`
	RiskFlags               []RiskFlag       `json:"risk_flags,omitempty"`
	MetricRefs              []string         `json:"metric_refs,omitempty"`
	GroundingRefs           []string         `json:"grounding_refs,omitempty"`
	EvidenceRefs            []string         `json:"evidence_refs,omitempty"`
	MemoryRefs              []string         `json:"memory_refs,omitempty"`
	Confidence              float64          `json:"confidence"`
	Caveats                 []string         `json:"caveats,omitempty"`
	ApprovalRequired        bool             `json:"approval_required,omitempty"`
	ApprovalReason          string           `json:"approval_reason,omitempty"`
	PolicyRuleRefs          []string         `json:"policy_rule_refs,omitempty"`
}

type DebtBlockResult struct {
	BlockID              string                   `json:"block_id"`
	Summary              string                   `json:"summary"`
	KeyFindings          []string                 `json:"key_findings,omitempty"`
	DeterministicMetrics DebtDeterministicMetrics `json:"deterministic_metrics"`
	MetricRecords        []finance.MetricRecord   `json:"metric_records,omitempty"`
	EvidenceIDs          []observation.EvidenceID `json:"evidence_ids,omitempty"`
	MemoryIDsUsed        []string                 `json:"memory_ids_used,omitempty"`
	MetricRefs           []string                 `json:"metric_refs,omitempty"`
	GroundingRefs        []string                 `json:"grounding_refs,omitempty"`
	RiskFlags            []RiskFlag               `json:"risk_flags,omitempty"`
	Recommendations      []Recommendation         `json:"recommendations,omitempty"`
	Caveats              []string                 `json:"caveats,omitempty"`
	ApprovalRequired     bool                     `json:"approval_required,omitempty"`
	ApprovalReason       string                   `json:"approval_reason,omitempty"`
	PolicyRuleRefs       []string                 `json:"policy_rule_refs,omitempty"`
	Confidence           float64                  `json:"confidence"`
}

type TaxBlockResult struct {
	BlockID              string                   `json:"block_id"`
	Summary              string                   `json:"summary"`
	KeyFindings          []string                 `json:"key_findings,omitempty"`
	DeterministicMetrics TaxDeterministicMetrics  `json:"deterministic_metrics"`
	MetricRecords        []finance.MetricRecord   `json:"metric_records,omitempty"`
	EvidenceIDs          []observation.EvidenceID `json:"evidence_ids,omitempty"`
	MemoryIDsUsed        []string                 `json:"memory_ids_used,omitempty"`
	MetricRefs           []string                 `json:"metric_refs,omitempty"`
	GroundingRefs        []string                 `json:"grounding_refs,omitempty"`
	RiskFlags            []RiskFlag               `json:"risk_flags,omitempty"`
	Recommendations      []Recommendation         `json:"recommendations,omitempty"`
	Caveats              []string                 `json:"caveats,omitempty"`
	ApprovalRequired     bool                     `json:"approval_required,omitempty"`
	ApprovalReason       string                   `json:"approval_reason,omitempty"`
	PolicyRuleRefs       []string                 `json:"policy_rule_refs,omitempty"`
	Confidence           float64                  `json:"confidence"`
}

type PortfolioBlockResult struct {
	BlockID              string                        `json:"block_id"`
	Summary              string                        `json:"summary"`
	KeyFindings          []string                      `json:"key_findings,omitempty"`
	DeterministicMetrics PortfolioDeterministicMetrics `json:"deterministic_metrics"`
	MetricRecords        []finance.MetricRecord        `json:"metric_records,omitempty"`
	EvidenceIDs          []observation.EvidenceID      `json:"evidence_ids,omitempty"`
	MemoryIDsUsed        []string                      `json:"memory_ids_used,omitempty"`
	MetricRefs           []string                      `json:"metric_refs,omitempty"`
	GroundingRefs        []string                      `json:"grounding_refs,omitempty"`
	RiskFlags            []RiskFlag                    `json:"risk_flags,omitempty"`
	Recommendations      []Recommendation              `json:"recommendations,omitempty"`
	Caveats              []string                      `json:"caveats,omitempty"`
	ApprovalRequired     bool                          `json:"approval_required,omitempty"`
	ApprovalReason       string                        `json:"approval_reason,omitempty"`
	PolicyRuleRefs       []string                      `json:"policy_rule_refs,omitempty"`
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
	case e.Tax != nil && e.BlockKind != "tax_event_impact_block" && e.BlockKind != "tax_optimization_block":
		return fmt.Errorf("tax result cannot be attached to block kind %q", e.BlockKind)
	case e.Portfolio != nil && e.BlockKind != "portfolio_event_impact_block" && e.BlockKind != "portfolio_rebalance_block":
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
