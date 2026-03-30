package governance

import (
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
)

const (
	PolicyRuleApprovalHighRisk                     = "approval.high_risk"
	PolicyRuleApprovalSensitiveRecommendation      = "approval.sensitive_recommendation"
	PolicyRuleApprovalLowLiquidityAggressiveInvest = "approval.low_liquidity_aggressive_invest"
	PolicyRuleApprovalHighDebtPressureInvest       = "approval.high_debt_pressure_invest"
	PolicyRuleDenyMissingDisclosure                = "deny.missing_disclosure"
	PolicyRuleDenyInsufficientActorRole            = "deny.insufficient_actor_role"
)

func approvalRulesForAction(request ActionRequest, toolPolicy *ToolExecutionPolicy) (PolicyDecisionOutcome, string, []string) {
	outcome := PolicyDecisionAllow
	reason := "action allowed by default"
	ruleRefs := make([]string, 0, 4)

	if toolPolicy != nil {
		if len(toolPolicy.AllowedRoles) > 0 && !hasIntersection(toolPolicy.AllowedRoles, request.ActorRoles) {
			return PolicyDecisionDeny, "actor roles do not satisfy tool execution policy", []string{PolicyRuleDenyInsufficientActorRole}
		}
		if compareRisk(request.RiskLevel, toolPolicy.RequiresApprovalAbove) >= 0 {
			outcome = PolicyDecisionRequireApproval
			reason = "tool execution policy requires approval"
			ruleRefs = append(ruleRefs, PolicyRuleApprovalHighRisk)
		}
	}

	for _, recommendation := range request.Recommendations {
		switch recommendation.Type {
		case analysis.RecommendationTypeInvestMore:
			if recommendation.ApprovalRequired {
				outcome = PolicyDecisionRequireApproval
				reason = firstNonEmpty(recommendation.ApprovalReason, "aggressive investment recommendation requires approval")
				ruleRefs = append(ruleRefs, PolicyRuleApprovalLowLiquidityAggressiveInvest)
			}
		case analysis.RecommendationTypePortfolioRebalance, analysis.RecommendationTypeDebtRestructure, analysis.RecommendationTypeTaxAction:
			if recommendationTypeSensitive(recommendation.Type) && compareRisk(recommendationRiskLevel(recommendation.RiskLevel), ActionRiskHigh) >= 0 {
				outcome = PolicyDecisionRequireApproval
				reason = firstNonEmpty(recommendation.ApprovalReason, "sensitive recommendation requires approval")
				ruleRefs = append(ruleRefs, PolicyRuleApprovalSensitiveRecommendation)
			}
		}
		if recommendationTypeSensitive(recommendation.Type) && len(recommendation.Caveats) == 0 && !request.DisclosureReady {
			return PolicyDecisionDeny, "sensitive recommendation is missing disclosure-ready caveats", []string{PolicyRuleDenyMissingDisclosure}
		}
	}

	if request.ApprovalRequired && outcome == PolicyDecisionAllow {
		outcome = PolicyDecisionRequireApproval
		reason = firstNonEmpty(request.ApprovalReason, "report marked recommendation as approval required")
		ruleRefs = append(ruleRefs, PolicyRuleApprovalSensitiveRecommendation)
	}

	if strings.Contains(strings.ToLower(request.Action), "single_stock") && compareRisk(request.RiskLevel, ActionRiskHigh) >= 0 && outcome == PolicyDecisionAllow {
		outcome = PolicyDecisionRequireApproval
		reason = "single-stock high-risk actions cannot auto-execute"
		ruleRefs = append(ruleRefs, PolicyRuleApprovalSensitiveRecommendation)
	}

	return outcome, reason, uniqueStrings(ruleRefs)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
