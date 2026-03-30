package governance

import "github.com/kobelakers/personal-cfo-os/internal/analysis"

type SensitiveRecommendationType string

const (
	SensitiveRecommendationInvestMore         SensitiveRecommendationType = "invest_more"
	SensitiveRecommendationPortfolioRebalance SensitiveRecommendationType = "portfolio_rebalance"
	SensitiveRecommendationDebtRestructure    SensitiveRecommendationType = "debt_restructure"
	SensitiveRecommendationTaxAction          SensitiveRecommendationType = "tax_action"
	SensitiveRecommendationBehaviorGuardrail  SensitiveRecommendationType = "behavior_guardrail"
)

func recommendationTypeSensitive(kind analysis.RecommendationType) bool {
	switch kind {
	case analysis.RecommendationTypeInvestMore,
		analysis.RecommendationTypePortfolioRebalance,
		analysis.RecommendationTypeDebtRestructure,
		analysis.RecommendationTypeTaxAction,
		analysis.RecommendationTypeBehaviorGuardrail:
		return true
	default:
		return false
	}
}
