package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/model"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/prompt"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/structured"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type CashflowReasoner interface {
	Analyze(ctx context.Context, input CashflowReasonerInput) (analysis.CashflowBlockResult, error)
}

type CashflowReasonerInput struct {
	WorkflowID       string
	TaskID           string
	TraceID          string
	CurrentState     state.FinancialWorldState
	RelevantMemories []memory.MemoryRecord
	RelevantEvidence []observation.EvidenceRecord
	Block            planning.ExecutionBlock
	ExecutionContext contextview.BlockExecutionContext
}

type DeterministicCashflowReasoner struct {
	MetricsTool tools.ComputeCashflowMetricsTool
	Engine      finance.Engine
}

func (r DeterministicCashflowReasoner) Analyze(_ context.Context, input CashflowReasonerInput) (analysis.CashflowBlockResult, error) {
	bundle := r.engine().Cashflow(input.CurrentState, input.RelevantEvidence, metricAsOf(input.CurrentState))
	metrics := bundle.Metrics
	evidenceIDs := collectEvidenceIDs(input.RelevantEvidence)
	memoryIDs := collectMemoryIDsFromRecords(input.RelevantMemories)
	keyFindings := []string{
		fmt.Sprintf("本期流入 %d 分，流出 %d 分，净结余 %d 分。", metrics.MonthlyInflowCents, metrics.MonthlyOutflowCents, metrics.MonthlyNetIncomeCents),
		fmt.Sprintf("储蓄率 %.2f，流动性缓冲 %.2f 月，应急金覆盖 %.2f 月。", metrics.SavingsRate, refMetric(bundle.Records, "liquidity_buffer_months").Float64Value, refMetric(bundle.Records, "emergency_fund_coverage_months").Float64Value),
		fmt.Sprintf("重复订阅 %d，深夜消费频率 %.2f，订阅负担 %.2f。", metrics.DuplicateSubscriptionCount, metrics.LateNightSpendingFrequency, refMetric(bundle.Records, "subscription_burden_ratio").Float64Value),
	}
	riskFlags := make([]analysis.RiskFlag, 0, 3)
	recommendations := make([]analysis.Recommendation, 0, 3)
	caveats := []string{
		"所有金额、比率和压力指标都以 Finance Engine 的 deterministic metric records 为准。",
	}
	metricRefs := bundle.Refs()
	groundingRefs := prefixedRefs("metric", metricRefs)

	if metrics.SavingsRate < 0.15 || refMetric(bundle.Records, "liquidity_buffer_months").Float64Value < 3 {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "cashflow_pressure",
			Severity:    "medium",
			Detail:      "储蓄率偏低，现金流缓冲较弱。",
			EvidenceIDs: evidenceIDs,
			MetricRefs:  []string{"savings_rate", "liquidity_buffer_months"},
			MemoryRefs:  append([]string{}, memoryIDs...),
			Caveats:     caveats,
		})
		recommendations = append(recommendations, analysis.Recommendation{
			ID:            string(input.Block.ID) + ":cashflow-adjustment",
			Type:          analysis.RecommendationTypeCashflowAdjustment,
			Title:         "优先修复月度结余",
			Detail:        "先控制可变支出并修复现金流缓冲，确保结余能覆盖后续债务或投资决策。",
			RiskLevel:     taskspec.RiskLevelMedium,
			GroundingRefs: []string{"metric:savings_rate", "metric:liquidity_buffer_months"},
			MetricRefs:    []string{"savings_rate", "liquidity_buffer_months", "monthly_net_income_cents"},
			EvidenceRefs:  evidenceIDsToString(evidenceIDs),
			MemoryRefs:    append([]string{}, memoryIDs...),
			Caveats:       caveats,
		})
	}
	if metrics.DuplicateSubscriptionCount > 0 || hasMemoryKeyword(input.RelevantMemories, "subscription") {
		recommendations = append(recommendations, analysis.Recommendation{
			ID:            string(input.Block.ID) + ":expense-reduction",
			Type:          analysis.RecommendationTypeExpenseReduction,
			Title:         "梳理经常性订阅",
			Detail:        "订阅负担和重复订阅信号说明现金流存在可优化项，先清理低使用率订阅。",
			RiskLevel:     taskspec.RiskLevelLow,
			GroundingRefs: []string{"metric:duplicate_subscription_count", "metric:subscription_burden_ratio"},
			MetricRefs:    []string{"duplicate_subscription_count", "subscription_burden_ratio", "recurring_expense_signal"},
			EvidenceRefs:  evidenceIDsToString(evidenceIDs),
			MemoryRefs:    append([]string{}, memoryIDs...),
			Caveats:       caveats,
		})
	}
	if metrics.LateNightSpendingFrequency >= 0.2 || hasMemoryKeyword(input.RelevantMemories, "late-night") {
		riskFlags = append(riskFlags, analysis.RiskFlag{
			Code:        "late_night_spending",
			Severity:    "medium",
			Detail:      "深夜消费波动偏高，建议把这部分支出单独复核。",
			EvidenceIDs: evidenceIDs,
			MetricRefs:  []string{"late_night_spending_frequency"},
			MemoryRefs:  append([]string{}, memoryIDs...),
			Caveats:     caveats,
		})
	}
	summary := fmt.Sprintf("现金流块结论：当前月度结余 %d 分，储蓄率 %.2f。", metrics.MonthlyNetIncomeCents, metrics.SavingsRate)
	if hasMemoryKeyword(input.RelevantMemories, "decision") && strings.Contains(string(input.Block.Kind), "liquidity") {
		summary += " 检索到历史债务决策记忆，本次更强调流动性缓冲与现金流稳定性。"
	}
	return analysis.CashflowBlockResult{
		BlockID:              string(input.Block.ID),
		Summary:              summary,
		KeyFindings:          keyFindings,
		DeterministicMetrics: metrics,
		MetricRecords:        append([]finance.MetricRecord{}, bundle.Records...),
		EvidenceIDs:          evidenceIDs,
		MemoryIDsUsed:        memoryIDs,
		MetricRefs:           metricRefs,
		GroundingRefs:        groundingRefs,
		RiskFlags:            riskFlags,
		Recommendations:      recommendations,
		Caveats:              caveats,
		ApprovalRequired:     recommendationsRequireApproval(recommendations),
		ApprovalReason:       approvalReasonFromRecommendations(recommendations),
		PolicyRuleRefs:       collectRecommendationPolicyRefs(recommendations),
		Confidence:           confidenceFromEvidence(input.RelevantEvidence),
	}, nil
}

func (r DeterministicCashflowReasoner) engine() finance.Engine {
	if r.Engine != nil {
		return r.Engine
	}
	return finance.DeterministicEngine{}
}

type ProviderBackedCashflowReasoner struct {
	Base           DeterministicCashflowReasoner
	PromptRenderer prompt.PromptRenderer
	Generator      model.StructuredGenerator
	TraceRecorder  structured.TraceRecorder
}

func (r ProviderBackedCashflowReasoner) Analyze(ctx context.Context, input CashflowReasonerInput) (analysis.CashflowBlockResult, error) {
	fallbackResult, fallbackErr := r.Base.Analyze(ctx, input)
	if fallbackErr != nil {
		return analysis.CashflowBlockResult{}, fallbackErr
	}
	rendered, err := r.PromptRenderer.Render("cashflow.monthly_review.v1", struct {
		Goal            string
		BlockID         string
		BlockKind       string
		MetricsSummary  string
		EvidenceSummary string
		MemorySummary   string
		ContextSummary  string
	}{
		Goal:            input.Block.Goal,
		BlockID:         string(input.Block.ID),
		BlockKind:       string(input.Block.Kind),
		MetricsSummary:  mustJSON(fallbackResult.DeterministicMetrics),
		EvidenceSummary: evidenceSummaryText(input.RelevantEvidence),
		MemorySummary:   memorySummaryText(input.RelevantMemories),
		ContextSummary:  contextview.ContextSummary(input.ExecutionContext.Slice),
	}, prompt.PromptTraceInput{
		SelectedStateBlocks:  append([]string{}, input.ExecutionContext.SelectedStateBlocks...),
		SelectedMemoryIDs:    append([]string{}, input.ExecutionContext.SelectedMemoryIDs...),
		SelectedEvidenceIDs:  evidenceIDsToString(input.ExecutionContext.SelectedEvidenceIDs),
		SelectedSkillNames:   contextview.SkillNames(input.ExecutionContext.Slice),
		ExcludedBlockRefs:    contextview.ExcludedBlockRefs(input.ExecutionContext.Slice),
		CompactionDecisions:  contextview.CompactionNotes(input.ExecutionContext.Slice),
		EstimatedInputTokens: input.ExecutionContext.EstimatedInputTokens,
	})
	if err != nil {
		return fallbackWithReason(fallbackResult, "prompt_render_failed", err.Error()), nil
	}
	schema := structured.Schema[analysis.CashflowStructuredCandidate]{
		Name:   "CashflowAnalysisSchema",
		Parser: structured.JSONParser[analysis.CashflowStructuredCandidate]{},
		Validator: structured.ValidatorFunc[analysis.CashflowStructuredCandidate](func(value analysis.CashflowStructuredCandidate) []string {
			return verification.ValidateCashflowStructuredCandidate(value, allowedCashflowMetricRefs(), input.ExecutionContext.SelectedEvidenceIDs, fallbackResult.DeterministicMetrics)
		}),
	}
	pipeline := structured.Pipeline[analysis.CashflowStructuredCandidate]{
		Schema:        schema,
		Generator:     r.Generator,
		TraceRecorder: r.TraceRecorder,
		RepairPolicy:  structured.DefaultRepairPolicy(),
		FallbackPolicy: structured.FallbackPolicy[analysis.CashflowStructuredCandidate]{
			Name: "deterministic_cashflow_reasoner",
			Execute: func() (analysis.CashflowStructuredCandidate, error) {
				return cashflowCandidateFromFallback(fallbackResult), nil
			},
		},
	}
	result, err := pipeline.Execute(ctx, model.StructuredGenerationRequest{
		ModelRequest: model.ModelRequest{
			Profile:         model.ModelProfileCashflowFast,
			Messages:        rendered.Messages(),
			ResponseFormat:  model.ResponseFormat{Type: model.ResponseFormatJSONObject},
			MaxOutputTokens: 700,
			Temperature:     0.1,
			WorkflowID:      input.WorkflowID,
			TaskID:          input.TaskID,
			TraceID:         input.TraceID,
			Agent:           "cashflow_agent",
			PromptID:        rendered.ID,
			PromptVersion:   string(rendered.Version),
		},
	})
	if err != nil {
		return fallbackWithReason(fallbackResult, string(structured.FailureCategoryFallbackFailed), err.Error()), nil
	}
	candidate := result.Value
	if err := verification.RunCashflowGroundingPrecheck(candidate, fallbackResult.DeterministicMetrics, input.ExecutionContext.SelectedEvidenceIDs); err != nil {
		return fallbackWithReason(fallbackResult, string(structured.FailureCategoryGroundingInvalid), err.Error()), nil
	}
	return mergeCashflowCandidate(fallbackResult, candidate), nil
}

func allowedCashflowMetricRefs() []string {
	return []string{
		"monthly_inflow_cents",
		"monthly_outflow_cents",
		"monthly_net_income_cents",
		"savings_rate",
		"savings_rate_quality_score",
		"debt_pressure_score",
		"emergency_fund_coverage_months",
		"liquidity_buffer_months",
		"subscription_burden_ratio",
		"recurring_expense_signal",
		"duplicate_subscription_count",
		"late_night_spending_frequency",
	}
}

func cashflowCandidateFromFallback(result analysis.CashflowBlockResult) analysis.CashflowStructuredCandidate {
	return analysis.CashflowStructuredCandidate{
		Summary:                 result.Summary,
		KeyFindings:             append([]string{}, result.KeyFindings...),
		RiskFlags:               append([]analysis.RiskFlag{}, result.RiskFlags...),
		MetricRefs:              append([]string{}, result.MetricRefs...),
		GroundingRefs:           append([]string{}, result.GroundingRefs...),
		EvidenceRefs:            evidenceIDsToString(result.EvidenceIDs),
		MemoryRefs:              append([]string{}, result.MemoryIDsUsed...),
		Confidence:              result.Confidence,
		Caveats:                 append([]string{}, result.Caveats...),
		GroundedRecommendations: append([]analysis.Recommendation{}, result.Recommendations...),
		ApprovalRequired:        result.ApprovalRequired,
		ApprovalReason:          result.ApprovalReason,
		PolicyRuleRefs:          append([]string{}, result.PolicyRuleRefs...),
	}
}

func mergeCashflowCandidate(base analysis.CashflowBlockResult, candidate analysis.CashflowStructuredCandidate) analysis.CashflowBlockResult {
	base.Summary = candidate.Summary
	base.KeyFindings = append([]string{}, candidate.KeyFindings...)
	base.RiskFlags = append([]analysis.RiskFlag{}, candidate.RiskFlags...)
	base.MetricRefs = append([]string{}, candidate.MetricRefs...)
	base.GroundingRefs = append([]string{}, candidate.GroundingRefs...)
	base.EvidenceIDs = stringRefsToEvidenceIDs(candidate.EvidenceRefs)
	base.MemoryIDsUsed = append([]string{}, candidate.MemoryRefs...)
	base.Recommendations = append([]analysis.Recommendation{}, candidate.GroundedRecommendations...)
	base.Confidence = candidate.Confidence
	base.Caveats = append([]string{}, candidate.Caveats...)
	base.ApprovalRequired = candidate.ApprovalRequired
	base.ApprovalReason = candidate.ApprovalReason
	base.PolicyRuleRefs = append([]string{}, candidate.PolicyRuleRefs...)
	return base
}

func evidenceIDsToString(ids []observation.EvidenceID) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		result = append(result, string(id))
	}
	return result
}

func stringRefsToEvidenceIDs(items []string) []observation.EvidenceID {
	result := make([]observation.EvidenceID, 0, len(items))
	for _, item := range items {
		result = append(result, observation.EvidenceID(item))
	}
	return result
}

func evidenceSummaryText(records []observation.EvidenceRecord) string {
	lines := make([]string, 0, len(records))
	for _, record := range records {
		lines = append(lines, fmt.Sprintf("%s|%s|%s", record.ID, record.Type, record.Summary))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func memorySummaryText(records []memory.MemoryRecord) string {
	lines := make([]string, 0, len(records))
	for _, record := range records {
		lines = append(lines, fmt.Sprintf("%s|%s|%s", record.ID, record.Kind, record.Summary))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func mustJSON(value any) string {
	payload, _ := json.MarshalIndent(value, "", "  ")
	return string(payload)
}

func fallbackWithReason(base analysis.CashflowBlockResult, category string, reason string) analysis.CashflowBlockResult {
	if reason == "" {
		return base
	}
	base.Caveats = append(base.Caveats, fmt.Sprintf("provider-backed cashflow reasoning fallback: %s (%s)", category, reason))
	return base
}
