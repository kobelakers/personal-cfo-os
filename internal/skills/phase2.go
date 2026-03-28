package skills

import (
	"fmt"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type SkillItem struct {
	Title       string                   `json:"title"`
	Detail      string                   `json:"detail"`
	Severity    string                   `json:"severity,omitempty"`
	EvidenceIDs []observation.EvidenceID `json:"evidence_ids,omitempty"`
}

type MonthlyReviewSkillOutput struct {
	Summary     string      `json:"summary"`
	RiskItems   []SkillItem `json:"risk_items"`
	Suggestions []SkillItem `json:"suggestions"`
	TodoItems   []SkillItem `json:"todo_items"`
	Confidence  float64     `json:"confidence"`
}

type DebtOptimizationSkillOutput struct {
	Conclusion string      `json:"conclusion"`
	Reasons    []string    `json:"reasons"`
	Actions    []SkillItem `json:"actions"`
	Confidence float64     `json:"confidence"`
}

type MonthlyReviewSkill struct{}

func (MonthlyReviewSkill) Name() string { return "monthly_review" }

func (MonthlyReviewSkill) Trigger() TriggerCondition {
	return TriggerCondition{
		IntentType: taskspec.UserIntentMonthlyReview,
		Keywords:   []string{"monthly review", "月度财务复盘", "月度复盘"},
	}
}

func (MonthlyReviewSkill) RequiredContext() []string {
	return []string{
		"cashflow_state",
		"liability_state",
		"portfolio_state",
		"tax_state",
		"behavior_state",
		"risk_state",
		"memory_blocks",
		"evidence_blocks",
	}
}

func (MonthlyReviewSkill) SuccessCriteriaTemplate() []taskspec.SuccessCriteria {
	return []taskspec.SuccessCriteria{
		{ID: "coverage", Description: "required evidence coverage is met"},
		{ID: "report", Description: "monthly review output includes risks, suggestions, and tasks"},
		{ID: "traceability", Description: "recommendations remain evidence-backed"},
	}
}

func (MonthlyReviewSkill) OutputContract() string {
	return "monthly_review_report"
}

func (MonthlyReviewSkill) Generate(current state.FinancialWorldState, evidence []observation.EvidenceRecord) MonthlyReviewSkillOutput {
	riskItems := make([]SkillItem, 0, 4)
	suggestions := make([]SkillItem, 0, 4)
	todoItems := make([]SkillItem, 0, 4)

	if current.LiabilityState.DebtBurdenRatio >= 0.2 {
		riskItems = append(riskItems, SkillItem{
			Title:       "债务负担偏高",
			Detail:      fmt.Sprintf("当前债务负担率为 %.2f，最低还款压力为 %.2f。", current.LiabilityState.DebtBurdenRatio, current.LiabilityState.MinimumPaymentPressure),
			Severity:    severityFromRatio(current.LiabilityState.DebtBurdenRatio),
			EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeDebtObligationSnapshot),
		})
		todoItems = append(todoItems, SkillItem{
			Title:       "复核高利率债务清单",
			Detail:      "优先确认信用卡与高 APR 债务是否存在提前偿还空间。",
			EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeDebtObligationSnapshot),
		})
	}

	if current.BehaviorState.DuplicateSubscriptionCount >= 2 {
		riskItems = append(riskItems, SkillItem{
			Title:       "重复订阅较多",
			Detail:      fmt.Sprintf("检测到 %d 个经常性订阅商户：%s。", current.BehaviorState.DuplicateSubscriptionCount, strings.Join(current.BehaviorState.RecurringSubscriptions, "、")),
			Severity:    "medium",
			EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeRecurringSubscription),
		})
		suggestions = append(suggestions, SkillItem{
			Title:       "执行订阅清理",
			Detail:      "优先梳理流媒体和工具类订阅，避免低使用率支出继续滚动。",
			EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeRecurringSubscription),
		})
	}

	if current.BehaviorState.LateNightSpendingFrequency >= 0.2 {
		suggestions = append(suggestions, SkillItem{
			Title:       "关注深夜消费波动",
			Detail:      fmt.Sprintf("深夜消费频率为 %.2f，建议复核高冲动支出场景。", current.BehaviorState.LateNightSpendingFrequency),
			EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypeLateNightSpendingSignal),
		})
	}

	if maxDrift(current.PortfolioState.AllocationDrift) >= 0.08 {
		riskItems = append(riskItems, SkillItem{
			Title:       "资产配置出现偏离",
			Detail:      fmt.Sprintf("当前最大配置偏离为 %.2f，说明仓位已偏离目标配置。", maxDrift(current.PortfolioState.AllocationDrift)),
			Severity:    "medium",
			EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypePortfolioAllocationSnap, observation.EvidenceTypeBrokerStatement),
		})
		suggestions = append(suggestions, SkillItem{
			Title:       "准备再平衡方案",
			Detail:      "先确认再平衡是否会影响流动性缓冲，再决定是否执行。",
			EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypePortfolioAllocationSnap, observation.EvidenceTypeBrokerStatement),
		})
	}

	if current.TaxState.ChildcareTaxSignal {
		suggestions = append(suggestions, SkillItem{
			Title:       "复核育儿相关税务优惠",
			Detail:      "工资单和税务单据都出现了家庭/育儿相关信号，建议确认抵扣和税优账户是否用满。",
			EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypePayslipStatement, observation.EvidenceTypeTaxDocument),
		})
		todoItems = append(todoItems, SkillItem{
			Title:       "补充家庭税务核查",
			Detail:      "确认 childcare credit / dependent care FSA 等是否已经正确申报。",
			EvidenceIDs: evidenceIDsByType(evidence, observation.EvidenceTypePayslipStatement, observation.EvidenceTypeTaxDocument),
		})
	}

	summary := fmt.Sprintf(
		"本月净结余 %d 分，储蓄率 %.2f，债务负担 %.2f，整体风险 %s。",
		current.CashflowState.MonthlyNetIncomeCents,
		current.CashflowState.SavingsRate,
		current.LiabilityState.DebtBurdenRatio,
		fallbackRisk(current.RiskState.OverallRisk),
	)

	confidence := 0.86
	if len(riskItems) == 0 {
		confidence = 0.8
	}
	return MonthlyReviewSkillOutput{
		Summary:     summary,
		RiskItems:   riskItems,
		Suggestions: suggestions,
		TodoItems:   todoItems,
		Confidence:  confidence,
	}
}

type DebtOptimizationSkill struct{}

func (DebtOptimizationSkill) Name() string { return "debt_optimization" }

func (DebtOptimizationSkill) Trigger() TriggerCondition {
	return TriggerCondition{
		IntentType: taskspec.UserIntentDebtVsInvest,
		Keywords:   []string{"提前还贷", "debt vs invest", "还贷还是投资"},
	}
}

func (DebtOptimizationSkill) RequiredContext() []string {
	return []string{"cashflow_state", "liability_state", "portfolio_state", "risk_state"}
}

func (DebtOptimizationSkill) SuccessCriteriaTemplate() []taskspec.SuccessCriteria {
	return []taskspec.SuccessCriteria{
		{ID: "comparison", Description: "debt and investment paths are compared with deterministic metrics"},
		{ID: "risk", Description: "liquidity and risk tradeoffs are explicit"},
	}
}

func (DebtOptimizationSkill) OutputContract() string {
	return "debt_decision_report"
}

func (DebtOptimizationSkill) Analyze(current state.FinancialWorldState) DebtOptimizationSkillOutput {
	conclusion := "继续投资"
	reasons := []string{
		fmt.Sprintf("平均债务利率 %.2f，债务负担率 %.2f。", current.LiabilityState.AverageAPR, current.LiabilityState.DebtBurdenRatio),
		fmt.Sprintf("现金流净结余 %d 分，流动性风险 %s。", current.CashflowState.MonthlyNetIncomeCents, fallbackRisk(current.RiskState.LiquidityRisk)),
	}
	actions := []SkillItem{
		{Title: "维持最低还款覆盖", Detail: "先确保最低还款压力不会挤占现金缓冲。"},
	}
	confidence := 0.72

	if current.LiabilityState.AverageAPR >= 0.12 || current.LiabilityState.MinimumPaymentPressure >= 0.15 {
		conclusion = "优先提前还高息债务"
		actions = append(actions, SkillItem{
			Title:    "优先偿还高息债务",
			Detail:   "高 APR 与最低还款压力已经明显抬升，先降债务风险更稳。",
			Severity: "high",
		})
		confidence = 0.82
	}
	return DebtOptimizationSkillOutput{
		Conclusion: conclusion,
		Reasons:    reasons,
		Actions:    actions,
		Confidence: confidence,
	}
}

func evidenceIDsByType(evidence []observation.EvidenceRecord, types ...observation.EvidenceType) []observation.EvidenceID {
	allowed := make(map[observation.EvidenceType]struct{}, len(types))
	for _, evidenceType := range types {
		allowed[evidenceType] = struct{}{}
	}
	result := make([]observation.EvidenceID, 0)
	for _, record := range evidence {
		if _, ok := allowed[record.Type]; ok {
			result = append(result, record.ID)
		}
	}
	return result
}

func severityFromRatio(value float64) string {
	switch {
	case value >= 0.35:
		return "high"
	case value >= 0.2:
		return "medium"
	default:
		return "low"
	}
}

func maxDrift(drift map[string]float64) float64 {
	maximum := 0.0
	for _, value := range drift {
		if value < 0 {
			value = -value
		}
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func fallbackRisk(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
