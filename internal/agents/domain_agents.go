package agents

import (
	"fmt"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/protocol"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

type CashflowAgentHandler struct {
	MetricsTool tools.ComputeCashflowMetricsTool
}

func (CashflowAgentHandler) Name() string      { return RecipientCashflowAgent }
func (CashflowAgentHandler) Recipient() string { return RecipientCashflowAgent }
func (CashflowAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindCashflowAnalysisRequest
}

func (a CashflowAgentHandler) Handle(_ AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.CashflowAnalysisRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientCashflowAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "cashflow analysis request payload is required"},
		}
	}
	metrics := toCashflowMetrics(a.MetricsTool.Compute(payload.CurrentState))
	evidenceIDs := collectEvidenceIDs(payload.RelevantEvidence)
	memoryIDs := collectMemoryIDsFromRecords(payload.RelevantMemories)
	keyFindings := []string{
		fmt.Sprintf("本期流入 %d 分，流出 %d 分，净结余 %d 分。", metrics.MonthlyInflowCents, metrics.MonthlyOutflowCents, metrics.MonthlyNetIncomeCents),
		fmt.Sprintf("储蓄率 %.2f，重复订阅 %d，深夜消费频率 %.2f。", metrics.SavingsRate, metrics.DuplicateSubscriptionCount, metrics.LateNightSpendingFrequency),
	}
	riskFlags := make([]analysis.RiskFlag, 0, 3)
	recommendations := make([]skills.SkillItem, 0, 3)

	if metrics.SavingsRate < 0.15 {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "cashflow_pressure",
			Severity:    "medium",
			Detail:      "储蓄率偏低，现金流缓冲较弱。",
			EvidenceIDs: evidenceIDs,
		})
		recommendations = append(recommendations, skills.SkillItem{
			Title:       "优先修复月度结余",
			Detail:      "先控制可变支出，确保结余可以覆盖后续债务或投资决策。",
			Severity:    "medium",
			EvidenceIDs: evidenceIDs,
		})
	}
	if metrics.DuplicateSubscriptionCount > 0 || hasMemoryKeyword(payload.RelevantMemories, "subscription") {
		recommendations = append(recommendations, skills.SkillItem{
			Title:       "梳理经常性订阅",
			Detail:      "订阅信号说明现金流里存在可优化项，先清理低使用率订阅。",
			Severity:    "low",
			EvidenceIDs: evidenceIDs,
		})
	}
	if metrics.LateNightSpendingFrequency >= 0.2 || hasMemoryKeyword(payload.RelevantMemories, "late-night") {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "late_night_spending",
			Severity:    "medium",
			Detail:      "深夜消费波动偏高，建议把这部分支出单独复核。",
			EvidenceIDs: evidenceIDs,
		})
	}

	summary := fmt.Sprintf("现金流块结论：当前月度结余 %d 分，储蓄率 %.2f。", metrics.MonthlyNetIncomeCents, metrics.SavingsRate)
	if hasMemoryKeyword(payload.RelevantMemories, "decision") && strings.Contains(string(payload.Block.Kind), "liquidity") {
		summary += " 检索到历史债务决策记忆，本次更强调流动性缓冲与现金流稳定性。"
	}

	result := analysis.CashflowBlockResult{
		BlockID:              string(payload.Block.ID),
		Summary:              summary,
		KeyFindings:          keyFindings,
		DeterministicMetrics: metrics,
		EvidenceIDs:          evidenceIDs,
		MemoryIDsUsed:        memoryIDs,
		RiskFlags:            riskFlags,
		Recommendations:      recommendations,
		Confidence:           confidenceFromEvidence(payload.RelevantEvidence),
	}
	return AgentHandlerResult{
		Kind: protocol.MessageKindCashflowAnalysisResult,
		Body: protocol.AgentResultBody{
			CashflowAnalysisResult: &protocol.CashflowAnalysisResultPayload{Result: result},
		},
	}, nil
}

type DebtAgentHandler struct {
	MetricsTool tools.ComputeDebtDecisionMetricsTool
}

func (DebtAgentHandler) Name() string      { return RecipientDebtAgent }
func (DebtAgentHandler) Recipient() string { return RecipientDebtAgent }
func (DebtAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindDebtAnalysisRequest
}

func (a DebtAgentHandler) Handle(_ AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.DebtAnalysisRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientDebtAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "debt analysis request payload is required"},
		}
	}
	metrics := toDebtMetrics(a.MetricsTool.Compute(payload.CurrentState))
	evidenceIDs := collectEvidenceIDs(payload.RelevantEvidence)
	memoryIDs := collectMemoryIDsFromRecords(payload.RelevantMemories)
	keyFindings := []string{
		fmt.Sprintf("债务负担率 %.2f，最低还款压力 %.2f，平均 APR %.2f。", metrics.DebtBurdenRatio, metrics.MinimumPaymentPressure, metrics.AverageAPR),
		fmt.Sprintf("当前整体风险 %s，月度净结余 %d 分。", metrics.OverallRisk, metrics.MonthlyNetIncomeCents),
	}
	riskFlags := make([]analysis.RiskFlag, 0, 2)
	recommendations := make([]skills.SkillItem, 0, 2)

	if metrics.DebtBurdenRatio >= 0.2 || metrics.MinimumPaymentPressure >= 0.15 {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "debt_pressure",
			Severity:    "high",
			Detail:      "债务负担与最低还款压力已经进入高关注区间。",
			EvidenceIDs: evidenceIDs,
		})
		recommendations = append(recommendations, skills.SkillItem{
			Title:       "优先压降高息债务",
			Detail:      "债务压力已较高，优先释放最低还款压力比继续增加风险资产更稳。",
			Severity:    "high",
			EvidenceIDs: evidenceIDs,
		})
	} else {
		recommendations = append(recommendations, skills.SkillItem{
			Title:       "保持债务覆盖并比较投资机会成本",
			Detail:      "当前债务压力可控，但仍需先确保现金流缓冲和最低还款覆盖。",
			Severity:    "medium",
			EvidenceIDs: evidenceIDs,
		})
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "debt_caveat",
			Severity:    "low",
			Detail:      "当前债务压力可控，但仍需持续关注最低还款覆盖和流动性缓冲。",
			EvidenceIDs: evidenceIDs,
		})
	}
	if hasMemoryKeyword(payload.RelevantMemories, "debt-versus-invest") || hasMemoryKeyword(payload.RelevantMemories, "debt pressure") {
		keyFindings = append(keyFindings, "历史记忆提示债务压力曾是主要风险源，本轮分析已提高债务风险权重。")
	}

	summary := fmt.Sprintf("债务块结论：债务负担率 %.2f，最低还款压力 %.2f。", metrics.DebtBurdenRatio, metrics.MinimumPaymentPressure)
	result := analysis.DebtBlockResult{
		BlockID:              string(payload.Block.ID),
		Summary:              summary,
		KeyFindings:          keyFindings,
		DeterministicMetrics: metrics,
		EvidenceIDs:          evidenceIDs,
		MemoryIDsUsed:        memoryIDs,
		RiskFlags:            riskFlags,
		Recommendations:      recommendations,
		Confidence:           confidenceFromEvidence(payload.RelevantEvidence),
	}
	return AgentHandlerResult{
		Kind: protocol.MessageKindDebtAnalysisResult,
		Body: protocol.AgentResultBody{
			DebtAnalysisResult: &protocol.DebtAnalysisResultPayload{Result: result},
		},
	}, nil
}

type TaxAgentHandler struct {
	MetricsTool tools.ComputeTaxSignalTool
}

func (TaxAgentHandler) Name() string      { return RecipientTaxAgent }
func (TaxAgentHandler) Recipient() string { return RecipientTaxAgent }
func (TaxAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindTaxAnalysisRequest
}

func (a TaxAgentHandler) Handle(_ AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.TaxAnalysisRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientTaxAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "tax analysis request payload is required"},
		}
	}
	values := a.MetricsTool.Compute(payload.CurrentState)
	metrics := analysis.TaxDeterministicMetrics{
		EffectiveTaxRate:               asFloat64(values["effective_tax_rate"]),
		TaxAdvantagedContributionCents: asInt64(values["tax_advantaged_contribution"]),
		ChildcareTaxSignal:             asBool(values["childcare_tax_signal"]),
		UpcomingDeadlineCount:          len(payload.CurrentState.TaxState.UpcomingDeadlines),
	}
	evidenceIDs := collectEvidenceIDs(payload.RelevantEvidence)
	memoryIDs := collectMemoryIDsFromRecords(payload.RelevantMemories)
	keyFindings := []string{
		fmt.Sprintf("当前有效税率 %.2f，税优贡献 %d 分。", metrics.EffectiveTaxRate, metrics.TaxAdvantagedContributionCents),
		fmt.Sprintf("deadline 数量 %d，childcare tax signal=%t。", metrics.UpcomingDeadlineCount, metrics.ChildcareTaxSignal),
	}
	riskFlags := make([]analysis.RiskFlag, 0, 2)
	recommendations := make([]skills.SkillItem, 0, 2)
	if metrics.ChildcareTaxSignal || hasMemoryKeyword(payload.RelevantMemories, "tax signal") {
		recommendations = append(recommendations, skills.SkillItem{
			Title:       "跟进税务优惠与预扣",
			Detail:      "事件已触发税务后续动作，优先复核预扣和可用税收优惠。",
			Severity:    "medium",
			EvidenceIDs: evidenceIDs,
		})
	}
	if metrics.UpcomingDeadlineCount > 0 {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "tax_deadline",
			Severity:    "medium",
			Detail:      "事件关联 deadline 已进入跟进窗口。",
			EvidenceIDs: evidenceIDs,
		})
	}
	summary := fmt.Sprintf("税务块结论：当前有效税率 %.2f，后续税务/预扣事项需要跟进。", metrics.EffectiveTaxRate)
	result := analysis.TaxBlockResult{
		BlockID:              string(payload.Block.ID),
		Summary:              summary,
		KeyFindings:          keyFindings,
		DeterministicMetrics: metrics,
		EvidenceIDs:          evidenceIDs,
		MemoryIDsUsed:        memoryIDs,
		RiskFlags:            riskFlags,
		Recommendations:      recommendations,
		Confidence:           confidenceFromEvidence(payload.RelevantEvidence),
	}
	return AgentHandlerResult{
		Kind: protocol.MessageKindTaxAnalysisResult,
		Body: protocol.AgentResultBody{
			TaxAnalysisResult: &protocol.TaxAnalysisResultPayload{Result: result},
		},
	}, nil
}

type PortfolioAgentHandler struct {
	MetricsTool tools.ComputePortfolioImpactMetricsTool
}

func (PortfolioAgentHandler) Name() string      { return RecipientPortfolioAgent }
func (PortfolioAgentHandler) Recipient() string { return RecipientPortfolioAgent }
func (PortfolioAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindPortfolioAnalysisRequest
}

func (a PortfolioAgentHandler) Handle(_ AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.PortfolioAnalysisRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientPortfolioAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "portfolio analysis request payload is required"},
		}
	}
	values := a.MetricsTool.Compute(payload.CurrentState)
	metrics := analysis.PortfolioDeterministicMetrics{
		TotalInvestableAssetsCents: asInt64(values["total_investable_assets_cents"]),
		EmergencyFundMonths:        asFloat64(values["emergency_fund_months"]),
		MaxAllocationDrift:         asFloat64(values["max_allocation_drift"]),
		CashAllocation:             asFloat64(values["cash_allocation"]),
	}
	evidenceIDs := collectEvidenceIDs(payload.RelevantEvidence)
	memoryIDs := collectMemoryIDsFromRecords(payload.RelevantMemories)
	keyFindings := []string{
		fmt.Sprintf("可投资资产 %d 分，应急金月数 %.2f。", metrics.TotalInvestableAssetsCents, metrics.EmergencyFundMonths),
		fmt.Sprintf("最大配置漂移 %.2f，现金配置 %.2f。", metrics.MaxAllocationDrift, metrics.CashAllocation),
	}
	riskFlags := make([]analysis.RiskFlag, 0, 2)
	recommendations := make([]skills.SkillItem, 0, 2)
	if metrics.EmergencyFundMonths < 3 {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "liquidity_buffer",
			Severity:    "high",
			Detail:      "应急金月数偏低，不宜直接扩大风险敞口。",
			EvidenceIDs: evidenceIDs,
		})
	}
	if metrics.MaxAllocationDrift >= 0.08 {
		recommendations = append(recommendations, skills.SkillItem{
			Title:       "复核再平衡窗口",
			Detail:      "事件后配置漂移已上升，建议进入组合再平衡 follow-up。",
			Severity:    "medium",
			EvidenceIDs: evidenceIDs,
		})
	}
	summary := fmt.Sprintf("配置块结论：应急金月数 %.2f，最大配置漂移 %.2f。", metrics.EmergencyFundMonths, metrics.MaxAllocationDrift)
	result := analysis.PortfolioBlockResult{
		BlockID:              string(payload.Block.ID),
		Summary:              summary,
		KeyFindings:          keyFindings,
		DeterministicMetrics: metrics,
		EvidenceIDs:          evidenceIDs,
		MemoryIDsUsed:        memoryIDs,
		RiskFlags:            riskFlags,
		Recommendations:      recommendations,
		Confidence:           confidenceFromEvidence(payload.RelevantEvidence),
	}
	return AgentHandlerResult{
		Kind: protocol.MessageKindPortfolioAnalysisResult,
		Body: protocol.AgentResultBody{
			PortfolioAnalysisResult: &protocol.PortfolioAnalysisResultPayload{Result: result},
		},
	}, nil
}

func toCashflowMetrics(values map[string]any) analysis.CashflowDeterministicMetrics {
	return analysis.CashflowDeterministicMetrics{
		MonthlyInflowCents:         asInt64(values["monthly_inflow_cents"]),
		MonthlyOutflowCents:        asInt64(values["monthly_outflow_cents"]),
		MonthlyNetIncomeCents:      asInt64(values["monthly_net_income_cents"]),
		SavingsRate:                asFloat64(values["savings_rate"]),
		DuplicateSubscriptionCount: int(asInt64(values["duplicate_subscription_count"])),
		LateNightSpendingFrequency: asFloat64(values["late_night_spending_frequency"]),
	}
}

func toDebtMetrics(values map[string]any) analysis.DebtDeterministicMetrics {
	return analysis.DebtDeterministicMetrics{
		DebtBurdenRatio:        asFloat64(values["debt_burden_ratio"]),
		MinimumPaymentPressure: asFloat64(values["minimum_payment_pressure"]),
		AverageAPR:             asFloat64(values["average_apr"]),
		MonthlyNetIncomeCents:  asInt64(values["monthly_net_income_cents"]),
		MaxAllocationDrift:     maxAllocationDrift(values["allocation_drift"]),
		OverallRisk:            asString(values["overall_risk"]),
	}
}

func collectEvidenceIDs(records []observation.EvidenceRecord) []observation.EvidenceID {
	result := make([]observation.EvidenceID, 0, len(records))
	for _, record := range records {
		result = append(result, record.ID)
	}
	return result
}

func collectMemoryIDsFromRecords(records []memory.MemoryRecord) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		result = append(result, record.ID)
	}
	return result
}

func hasMemoryKeyword(records []memory.MemoryRecord, keyword string) bool {
	keyword = strings.ToLower(keyword)
	for _, record := range records {
		if strings.Contains(strings.ToLower(record.Summary), keyword) {
			return true
		}
		for _, fact := range record.Facts {
			if strings.Contains(strings.ToLower(fact.Key), keyword) || strings.Contains(strings.ToLower(fact.Value), keyword) {
				return true
			}
		}
	}
	return false
}

func confidenceFromEvidence(records []observation.EvidenceRecord) float64 {
	if len(records) == 0 {
		return 0.4
	}
	total := 0.0
	for _, record := range records {
		total += record.Confidence.Score
	}
	return total / float64(len(records))
}

func maxAllocationDrift(value any) float64 {
	switch typed := value.(type) {
	case map[string]float64:
		maximum := 0.0
		for _, item := range typed {
			if item < 0 {
				item = -item
			}
			if item > maximum {
				maximum = item
			}
		}
		return maximum
	case map[string]any:
		maximum := 0.0
		for _, item := range typed {
			drift := asFloat64(item)
			if drift < 0 {
				drift = -drift
			}
			if drift > maximum {
				maximum = drift
			}
		}
		return maximum
	default:
		return 0
	}
}

func asInt64(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	default:
		return 0
	}
}

func asFloat64(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	default:
		return 0
	}
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func asBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}
