package agents

import (
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/protocol"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

type CashflowAgentHandler struct {
	MetricsTool tools.ComputeCashflowMetricsTool
	Engine      finance.Engine
	Reasoner    CashflowReasoner
}

func (CashflowAgentHandler) Name() string      { return RecipientCashflowAgent }
func (CashflowAgentHandler) Recipient() string { return RecipientCashflowAgent }
func (CashflowAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindCashflowAnalysisRequest
}

func (a CashflowAgentHandler) Handle(handlerCtx AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.CashflowAnalysisRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientCashflowAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "cashflow analysis request payload is required"},
		}
	}
	reasoner := a.Reasoner
	if reasoner == nil {
		reasoner = DeterministicCashflowReasoner{Engine: a.engine()}
	}
	result, err := reasoner.Analyze(handlerCtx.Context, CashflowReasonerInput{
		WorkflowID:       envelope.Metadata.CorrelationID,
		TaskID:           envelope.Task.ID,
		TraceID:          envelope.Metadata.CorrelationID,
		CurrentState:     payload.CurrentState,
		RelevantMemories: payload.RelevantMemories,
		RelevantEvidence: payload.RelevantEvidence,
		Block:            payload.Block,
		ExecutionContext: payload.ExecutionContext,
	})
	if err != nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientCashflowAgent,
			Kind:      envelope.Kind,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureValidation,
				Message:  "cashflow reasoning failed",
			},
			Cause: err,
		}
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
	Engine      finance.Engine
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
	engine := a.engine()
	bundle := engine.DebtDecision(payload.CurrentState, payload.RelevantEvidence, metricAsOf(payload.CurrentState))
	metrics := bundle.Metrics
	evidenceIDs := collectEvidenceIDs(payload.RelevantEvidence)
	memoryIDs := collectMemoryIDsFromRecords(payload.RelevantMemories)
	keyFindings := []string{
		fmt.Sprintf("债务负担率 %.2f，最低还款压力 %.2f，平均 APR %.2f。", metrics.DebtBurdenRatio, metrics.MinimumPaymentPressure, metrics.AverageAPR),
		fmt.Sprintf("当前整体风险 %s，月度净结余 %d 分。", metrics.OverallRisk, metrics.MonthlyNetIncomeCents),
	}
	riskFlags := make([]analysis.RiskFlag, 0, 2)
	recommendations := make([]analysis.Recommendation, 0, 2)
	caveats := []string{
		"债务决策中的金额、压力指标和流动性指标均以 Finance Engine 的 deterministic metrics 为准。",
	}

	if metrics.DebtBurdenRatio >= 0.2 || metrics.MinimumPaymentPressure >= 0.15 {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "debt_pressure",
			Severity:    "high",
			Detail:      "债务负担与最低还款压力已经进入高关注区间。",
			EvidenceIDs: evidenceIDs,
			MetricRefs:  bundle.Refs(),
			MemoryRefs:  memoryIDs,
		})
		recommendations = append(recommendations, analysis.Recommendation{
			ID:            string(payload.Block.ID) + ":debt-paydown",
			Type:          analysis.RecommendationTypeDebtPaydown,
			Title:         "优先压降高息债务",
			Detail:        "债务压力已较高，优先释放最低还款压力比继续增加风险资产更稳。",
			RiskLevel:     taskspec.RiskLevelMedium,
			GroundingRefs: prefixedRefs("metric", bundle.Refs()),
			MetricRefs:    bundle.Refs(),
			EvidenceRefs:  evidenceIDsToString(evidenceIDs),
			MemoryRefs:    append([]string{}, memoryIDs...),
			Caveats:       caveats,
		})
	} else {
		recommendations = append(recommendations, analysis.Recommendation{
			ID:            string(payload.Block.ID) + ":debt-coverage",
			Type:          analysis.RecommendationTypeDebtPaydown,
			Title:         "保持债务覆盖并比较投资机会成本",
			Detail:        "当前债务压力可控，但仍需先确保现金流缓冲和最低还款覆盖。",
			RiskLevel:     taskspec.RiskLevelMedium,
			GroundingRefs: prefixedRefs("metric", bundle.Refs()),
			MetricRefs:    bundle.Refs(),
			EvidenceRefs:  evidenceIDsToString(evidenceIDs),
			MemoryRefs:    append([]string{}, memoryIDs...),
			Caveats:       caveats,
		})
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "debt_caveat",
			Severity:    "low",
			Detail:      "当前债务压力可控，但仍需持续关注最低还款覆盖和流动性缓冲。",
			EvidenceIDs: evidenceIDs,
			MetricRefs:  bundle.Refs(),
			MemoryRefs:  memoryIDs,
		})
	}
	if payload.Block.Kind == planning.ExecutionBlockKindDebtTradeoff && refMetric(bundle.Records, "investable_surplus_cents").Int64Value > 0 {
		highRiskInvest := refMetric(bundle.Records, "emergency_fund_coverage_months").Float64Value < 3 || refMetric(bundle.Records, "debt_payoff_pressure").Float64Value >= 0.7
		investRecommendation := analysis.Recommendation{
			ID:            string(payload.Block.ID) + ":invest-more",
			Type:          analysis.RecommendationTypeInvestMore,
			Title:         "在保留缓冲前提下评估继续投资",
			Detail:        "当前存在可投资结余，但如果继续增加风险资产，需要先确认现金缓冲和债务压力是否允许。",
			RiskLevel:     taskspec.RiskLevelMedium,
			GroundingRefs: prefixedRefs("metric", bundle.Refs()),
			MetricRefs:    []string{"investable_surplus_cents", "liquidity_after_paydown", "debt_payoff_pressure", "emergency_fund_coverage_months"},
			EvidenceRefs:  evidenceIDsToString(evidenceIDs),
			MemoryRefs:    append([]string{}, memoryIDs...),
			Caveats: []string{
				"若紧急备用金不足或债务压力偏高，不应在未复核风险的情况下直接扩大投资敞口。",
			},
		}
		if highRiskInvest {
			investRecommendation.RiskLevel = taskspec.RiskLevelHigh
			investRecommendation.ApprovalRequired = true
			investRecommendation.ApprovalReason = "低流动性或高债务压力下的 invest_more 建议需要治理审批"
			investRecommendation.PolicyRuleRefs = []string{"approval.invest_more.low_liquidity_or_high_debt"}
		}
		recommendations = append(recommendations, investRecommendation)
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
		MetricRecords:        append([]finance.MetricRecord{}, bundle.Records...),
		EvidenceIDs:          evidenceIDs,
		MemoryIDsUsed:        memoryIDs,
		MetricRefs:           bundle.Refs(),
		GroundingRefs:        prefixedRefs("metric", bundle.Refs()),
		RiskFlags:            riskFlags,
		Recommendations:      recommendations,
		Caveats:              caveats,
		ApprovalRequired:     recommendationsRequireApproval(recommendations),
		ApprovalReason:       approvalReasonFromRecommendations(recommendations),
		PolicyRuleRefs:       collectRecommendationPolicyRefs(recommendations),
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
	Engine      finance.Engine
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
	engine := a.engine()
	bundle := engine.Tax(payload.CurrentState, payload.RelevantEvidence, metricAsOf(payload.CurrentState))
	metrics := bundle.Metrics
	evidenceIDs := collectEvidenceIDs(payload.RelevantEvidence)
	memoryIDs := collectMemoryIDsFromRecords(payload.RelevantMemories)
	keyFindings := []string{
		fmt.Sprintf("当前有效税率 %.2f，税优贡献 %d 分。", metrics.EffectiveTaxRate, metrics.TaxAdvantagedContributionCents),
		fmt.Sprintf("deadline 数量 %d，childcare tax signal=%t。", metrics.UpcomingDeadlineCount, metrics.ChildcareTaxSignal),
	}
	riskFlags := make([]analysis.RiskFlag, 0, 2)
	recommendations := make([]analysis.Recommendation, 0, 2)
	caveats := []string{"税务类建议必须结合 deadline、withholding 和合规要求审慎执行。"}
	if metrics.ChildcareTaxSignal || hasMemoryKeyword(payload.RelevantMemories, "tax signal") {
		recommendations = append(recommendations, analysis.Recommendation{
			ID:            string(payload.Block.ID) + ":tax-action",
			Type:          analysis.RecommendationTypeTaxAction,
			Title:         "跟进税务优惠与预扣",
			Detail:        "事件已触发税务后续动作，优先复核预扣和可用税收优惠。",
			RiskLevel:     taskspec.RiskLevelMedium,
			GroundingRefs: prefixedRefs("metric", bundle.Refs()),
			MetricRefs:    bundle.Refs(),
			EvidenceRefs:  evidenceIDsToString(evidenceIDs),
			MemoryRefs:    append([]string{}, memoryIDs...),
			Caveats:       caveats,
		})
	}
	if metrics.UpcomingDeadlineCount > 0 {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "tax_deadline",
			Severity:    "medium",
			Detail:      "事件关联 deadline 已进入跟进窗口。",
			EvidenceIDs: evidenceIDs,
			MetricRefs:  bundle.Refs(),
			Caveats:     caveats,
		})
	}
	summary := fmt.Sprintf("税务块结论：当前有效税率 %.2f，后续税务/预扣事项需要跟进。", metrics.EffectiveTaxRate)
	result := analysis.TaxBlockResult{
		BlockID:              string(payload.Block.ID),
		Summary:              summary,
		KeyFindings:          keyFindings,
		DeterministicMetrics: metrics,
		MetricRecords:        append([]finance.MetricRecord{}, bundle.Records...),
		EvidenceIDs:          evidenceIDs,
		MemoryIDsUsed:        memoryIDs,
		MetricRefs:           bundle.Refs(),
		GroundingRefs:        prefixedRefs("metric", bundle.Refs()),
		RiskFlags:            riskFlags,
		Recommendations:      recommendations,
		Caveats:              caveats,
		ApprovalRequired:     recommendationsRequireApproval(recommendations),
		ApprovalReason:       approvalReasonFromRecommendations(recommendations),
		PolicyRuleRefs:       collectRecommendationPolicyRefs(recommendations),
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
	Engine      finance.Engine
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
	engine := a.engine()
	bundle := engine.Portfolio(payload.CurrentState, payload.RelevantEvidence, metricAsOf(payload.CurrentState))
	metrics := bundle.Metrics
	evidenceIDs := collectEvidenceIDs(payload.RelevantEvidence)
	memoryIDs := collectMemoryIDsFromRecords(payload.RelevantMemories)
	keyFindings := []string{
		fmt.Sprintf("可投资资产 %d 分，应急金月数 %.2f。", metrics.TotalInvestableAssetsCents, metrics.EmergencyFundMonths),
		fmt.Sprintf("最大配置漂移 %.2f，现金配置 %.2f。", metrics.MaxAllocationDrift, metrics.CashAllocation),
	}
	riskFlags := make([]analysis.RiskFlag, 0, 2)
	recommendations := make([]analysis.Recommendation, 0, 2)
	caveats := []string{"再平衡类建议必须同时披露漂移、流动性和潜在税务影响。"}
	if metrics.EmergencyFundMonths < 3 {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "liquidity_buffer",
			Severity:    "high",
			Detail:      "应急金月数偏低，不宜直接扩大风险敞口。",
			EvidenceIDs: evidenceIDs,
			MetricRefs:  bundle.Refs(),
			Caveats:     caveats,
		})
	}
	if metrics.MaxAllocationDrift >= 0.08 {
		recommendations = append(recommendations, analysis.Recommendation{
			ID:            string(payload.Block.ID) + ":portfolio-rebalance",
			Type:          analysis.RecommendationTypePortfolioRebalance,
			Title:         "复核再平衡窗口",
			Detail:        "事件后配置漂移已上升，建议进入组合再平衡 follow-up。",
			RiskLevel:     taskspec.RiskLevelMedium,
			GroundingRefs: prefixedRefs("metric", bundle.Refs()),
			MetricRefs:    bundle.Refs(),
			EvidenceRefs:  evidenceIDsToString(evidenceIDs),
			MemoryRefs:    append([]string{}, memoryIDs...),
			Caveats:       caveats,
		})
	}
	summary := fmt.Sprintf("配置块结论：应急金月数 %.2f，最大配置漂移 %.2f。", metrics.EmergencyFundMonths, metrics.MaxAllocationDrift)
	result := analysis.PortfolioBlockResult{
		BlockID:              string(payload.Block.ID),
		Summary:              summary,
		KeyFindings:          keyFindings,
		DeterministicMetrics: metrics,
		MetricRecords:        append([]finance.MetricRecord{}, bundle.Records...),
		EvidenceIDs:          evidenceIDs,
		MemoryIDsUsed:        memoryIDs,
		MetricRefs:           bundle.Refs(),
		GroundingRefs:        prefixedRefs("metric", bundle.Refs()),
		RiskFlags:            riskFlags,
		Recommendations:      recommendations,
		Caveats:              caveats,
		ApprovalRequired:     recommendationsRequireApproval(recommendations),
		ApprovalReason:       approvalReasonFromRecommendations(recommendations),
		PolicyRuleRefs:       collectRecommendationPolicyRefs(recommendations),
		Confidence:           confidenceFromEvidence(payload.RelevantEvidence),
	}
	return AgentHandlerResult{
		Kind: protocol.MessageKindPortfolioAnalysisResult,
		Body: protocol.AgentResultBody{
			PortfolioAnalysisResult: &protocol.PortfolioAnalysisResultPayload{Result: result},
		},
	}, nil
}

func (a CashflowAgentHandler) engine() finance.Engine {
	if a.Engine != nil {
		return a.Engine
	}
	return finance.DeterministicEngine{}
}

func (a DebtAgentHandler) engine() finance.Engine {
	if a.Engine != nil {
		return a.Engine
	}
	return finance.DeterministicEngine{}
}

func (a TaxAgentHandler) engine() finance.Engine {
	if a.Engine != nil {
		return a.Engine
	}
	return finance.DeterministicEngine{}
}

func (a PortfolioAgentHandler) engine() finance.Engine {
	if a.Engine != nil {
		return a.Engine
	}
	return finance.DeterministicEngine{}
}

func metricAsOf(current state.FinancialWorldState) time.Time {
	if !current.Version.UpdatedAt.IsZero() {
		return current.Version.UpdatedAt.UTC()
	}
	return time.Now().UTC()
}

func recommendationsRequireApproval(items []analysis.Recommendation) bool {
	for _, item := range items {
		if item.ApprovalRequired {
			return true
		}
	}
	return false
}

func approvalReasonFromRecommendations(items []analysis.Recommendation) string {
	for _, item := range items {
		if item.ApprovalRequired && item.ApprovalReason != "" {
			return item.ApprovalReason
		}
	}
	return ""
}

func collectRecommendationPolicyRefs(items []analysis.Recommendation) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, item := range items {
		for _, ref := range item.PolicyRuleRefs {
			if ref == "" {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			result = append(result, ref)
		}
	}
	return result
}

func prefixedRefs(prefix string, refs []string) []string {
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref == "" {
			continue
		}
		result = append(result, prefix+":"+ref)
	}
	return result
}

func refMetric(records []finance.MetricRecord, ref string) finance.MetricRecord {
	for _, record := range records {
		if record.Ref == ref {
			return record
		}
	}
	return finance.MetricRecord{}
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
