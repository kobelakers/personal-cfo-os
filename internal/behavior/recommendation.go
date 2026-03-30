package behavior

import (
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func Recommend(bundle BehaviorMetricBundle, selection skills.SkillSelection, evidenceRefs []string, memoryRefs []string) ([]BehaviorRecommendation, []BehaviorAnomaly, []BehaviorTrend) {
	metrics := bundle.Metrics
	anomalies := make([]BehaviorAnomaly, 0, 3)
	trends := make([]BehaviorTrend, 0, 2)
	metricRefs := refs(bundle)
	if metrics.DuplicateSubscriptionCount >= 2 {
		anomalies = append(anomalies, BehaviorAnomaly{
			Code:         "duplicate_subscriptions",
			Severity:     "medium",
			Detail:       "duplicate subscription overlap indicates cleanup opportunity",
			MetricRefs:   []string{"duplicate_subscription_count", "recurring_subscription_count"},
			EvidenceRefs: evidenceRefs,
		})
	}
	if metrics.LateNightSpendRatio >= 0.30 || metrics.LateNightSpendCount >= 4 {
		anomalies = append(anomalies, BehaviorAnomaly{
			Code:         "late_night_spend_spike",
			Severity:     "medium",
			Detail:       "late-night discretionary spending frequency is elevated",
			MetricRefs:   []string{"late_night_spend_count", "late_night_spend_ratio"},
			EvidenceRefs: evidenceRefs,
		})
	}
	if metrics.DiscretionaryPressureScore >= 0.55 {
		anomalies = append(anomalies, BehaviorAnomaly{
			Code:         "discretionary_pressure",
			Severity:     severityForPressure(metrics.DiscretionaryPressureScore),
			Detail:       "discretionary pressure score is elevated enough to justify intervention",
			MetricRefs:   []string{"discretionary_pressure_score"},
			EvidenceRefs: evidenceRefs,
		})
	}
	trends = append(trends, BehaviorTrend{
		Code:   "behavior_pressure_summary",
		Detail: fmt.Sprintf("行为压力分数 %.2f，深夜消费占比 %.2f，重复订阅数 %d。", metrics.DiscretionaryPressureScore, metrics.LateNightSpendRatio, metrics.DuplicateSubscriptionCount),
	})
	stateRefs := []string{"behavior_state", "cashflow_state"}

	rec := BehaviorRecommendation{
		EvidenceRefs: evidenceRefs,
		MetricRefs:   metricRefs,
		StateRefs:    stateRefs,
		MemoryRefs:   append([]string{}, memoryRefs...),
	}
	switch selection.Family {
	case skills.SkillFamilySubscriptionCleanup:
		rec.Type = BehaviorRecommendationSubscriptionCleanup
		rec.Title = "清理重复订阅"
		rec.Detail = "重复订阅异常主导当前行为问题，优先梳理重复扣费和低价值订阅。"
		rec.RiskLevel = taskspec.RiskLevelMedium
		rec.Caveats = []string{"优先取消重复和低价值订阅，不直接影响必要固定支出。"}
	case skills.SkillFamilyLateNightSpendNudge:
		rec.Type = BehaviorRecommendationSpendNudge
		rec.Title = "建立深夜消费提醒"
		rec.Detail = "深夜消费占比偏高，先用低摩擦提醒和延迟购买机制压缩冲动支出。"
		rec.RiskLevel = taskspec.RiskLevelMedium
		rec.Caveats = []string{"先用提醒和预算提示，不建议直接采用强限制。"}
	default:
		rec.Type = BehaviorRecommendationGuardrail
		rec.Title = "设置可选消费护栏"
		rec.Detail = "可选支出压力已进入需要干预的区间，应按当前 recipe 强度建立消费护栏。"
		rec.RiskLevel = riskForRecipe(selection.RecipeID)
		rec.Caveats = caveatsForRecipe(selection.RecipeID)
		rec.PolicyRuleRefs = append([]string{}, selection.PolicyRuleRefs...)
		if selection.RecipeID == "hard_cap.v1" {
			rec.ApprovalRequired = true
			rec.ApprovalReason = "高强度 discretionary guardrail 需要治理审批"
		}
	}
	return []BehaviorRecommendation{rec}, anomalies, trends
}

func severityForPressure(score float64) string {
	if score >= 0.75 {
		return "high"
	}
	return "medium"
}

func riskForRecipe(recipeID string) taskspec.RiskLevel {
	switch recipeID {
	case "hard_cap.v1":
		return taskspec.RiskLevelHigh
	case "budget_guardrail.v1":
		return taskspec.RiskLevelMedium
	default:
		return taskspec.RiskLevelLow
	}
}

func caveatsForRecipe(recipeID string) []string {
	switch recipeID {
	case "hard_cap.v1":
		return []string{"高强度消费护栏属于强干预建议，必须经治理审批后再执行。"}
	case "budget_guardrail.v1":
		return []string{"先以预算护栏和强提醒为主，不做外部支付限制。"}
	default:
		return []string{"优先采用低摩擦引导，不直接限制支付能力。"}
	}
}

func refs(bundle BehaviorMetricBundle) []string {
	result := make([]string, 0, len(bundle.Records))
	for _, record := range bundle.Records {
		result = append(result, record.Ref)
	}
	return result
}
