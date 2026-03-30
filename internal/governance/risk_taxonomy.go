package governance

import "github.com/kobelakers/personal-cfo-os/internal/analysis"

type SensitiveRecommendationType string

const (
	SensitiveRecommendationInvestMore         SensitiveRecommendationType = "invest_more"
	SensitiveRecommendationPortfolioRebalance SensitiveRecommendationType = "portfolio_rebalance"
	SensitiveRecommendationDebtRestructure    SensitiveRecommendationType = "debt_restructure"
	SensitiveRecommendationTaxAction          SensitiveRecommendationType = "tax_action"
)

func recommendationTypeSensitive(kind analysis.RecommendationType) bool {
	switch kind {
	case analysis.RecommendationTypeInvestMore,
		analysis.RecommendationTypePortfolioRebalance,
		analysis.RecommendationTypeDebtRestructure,
		analysis.RecommendationTypeTaxAction:
		return true
	default:
		return false
	}
}
