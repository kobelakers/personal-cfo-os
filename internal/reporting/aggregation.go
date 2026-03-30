package reporting

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

type MonthlyReviewAggregator struct {
	TaxSignals tools.ComputeTaxSignalTool
	Engine     finance.Engine
	Now        func() time.Time
}

type DebtDecisionAggregator struct {
	Now func() time.Time
}

type LifeEventAssessmentAggregator struct {
	Now func() time.Time
}

type TaxOptimizationAggregator struct {
	Now func() time.Time
}

type PortfolioRebalanceAggregator struct {
	Now func() time.Time
}

type BehaviorInterventionAggregator struct {
	Now func() time.Time
}

func (a MonthlyReviewAggregator) Aggregate(spec taskspec.TaskSpec, workflowID string, input DraftInput) (MonthlyReviewReport, error) {
	ordered, err := orderedBlockResults(input.Plan, input.BlockResults)
	if err != nil {
		return MonthlyReviewReport{}, err
	}
	var (
		cashflow *analysis.CashflowBlockResult
		debt     *analysis.DebtBlockResult
	)
	for _, item := range ordered {
		switch {
		case item.Cashflow != nil:
			cashflow = item.Cashflow
		case item.Debt != nil:
			debt = item.Debt
		}
	}
	if cashflow == nil || debt == nil {
		return MonthlyReviewReport{}, fmt.Errorf("monthly review draft requires both cashflow and debt block results")
	}

	sourceBlockIDs, sourceMemoryIDs, sourceEvidenceIDs := collectProvenance(ordered)
	riskFlags := append(append([]analysis.RiskFlag{}, cashflow.RiskFlags...), debt.RiskFlags...)
	recommendations := append(append([]analysis.Recommendation{}, cashflow.Recommendations...), debt.Recommendations...)
	riskItems := riskFlagsToSkillItems(riskFlags)
	optimizationSuggestions := recommendationsToSkillItems(recommendations)
	todoItems := deriveTodoItems(recommendations)
	summaryParts := make([]string, 0, len(ordered))
	for _, item := range ordered {
		summaryParts = append(summaryParts, item.Summary())
	}
	engine := a.engine()
	taxSignals := taxMetricsMap(engine.Tax(input.CurrentState, input.Evidence, a.now()).Metrics)
	metricRecords := appendMetricRecords(cashflow.MetricRecords, debt.MetricRecords)
	overallRiskApproval := input.CurrentState.RiskState.OverallRisk == "high" || input.CurrentState.RiskState.OverallRisk == "critical"
	return MonthlyReviewReport{
		TaskID:                  spec.ID,
		WorkflowID:              workflowID,
		Summary:                 strings.Join(summaryParts, " "),
		CashflowMetrics:         cashflowMetricsMap(cashflow.DeterministicMetrics),
		TaxSignals:              taxSignals,
		MetricRecords:           metricRecords,
		RiskItems:               riskItems,
		RiskFlags:               riskFlags,
		OptimizationSuggestions: optimizationSuggestions,
		Recommendations:         recommendations,
		TodoItems:               todoItems,
		SourceBlockIDs:          sourceBlockIDs,
		SourceMemoryIDs:         sourceMemoryIDs,
		SourceEvidenceIDs:       sourceEvidenceIDs,
		GroundingRefs:           appendGroundingRefs(cashflow.GroundingRefs, debt.GroundingRefs),
		Caveats:                 appendCaveats(cashflow.Caveats, debt.Caveats),
		ApprovalRequired:        overallRiskApproval || cashflow.ApprovalRequired || debt.ApprovalRequired,
		ApprovalReason:          firstNonEmpty(cashflow.ApprovalReason, debt.ApprovalReason, overallRiskApprovalReason(overallRiskApproval)),
		PolicyRuleRefs:          appendPolicyRuleRefs(cashflow.PolicyRuleRefs, debt.PolicyRuleRefs),
		Confidence:              averageConfidence(ordered),
		GeneratedAt:             a.now(),
	}, nil
}

func (a DebtDecisionAggregator) Aggregate(spec taskspec.TaskSpec, workflowID string, input DraftInput) (DebtDecisionReport, error) {
	ordered, err := orderedBlockResults(input.Plan, input.BlockResults)
	if err != nil {
		return DebtDecisionReport{}, err
	}
	var (
		cashflow *analysis.CashflowBlockResult
		debt     *analysis.DebtBlockResult
	)
	for _, item := range ordered {
		switch {
		case item.Cashflow != nil:
			cashflow = item.Cashflow
		case item.Debt != nil:
			debt = item.Debt
		}
	}
	if cashflow == nil || debt == nil {
		return DebtDecisionReport{}, fmt.Errorf("debt decision draft requires both cashflow and debt block results")
	}

	sourceBlockIDs, sourceMemoryIDs, sourceEvidenceIDs := collectProvenance(ordered)
	reasons := append([]string{}, cashflow.KeyFindings...)
	reasons = append(reasons, debt.KeyFindings...)
	recommendations := append(append([]analysis.Recommendation{}, cashflow.Recommendations...), debt.Recommendations...)
	actions := recommendationsToSkillItems(recommendations)
	conclusionParts := make([]string, 0, len(ordered))
	for _, item := range ordered {
		conclusionParts = append(conclusionParts, item.Summary())
	}
	overallRiskApproval := input.CurrentState.RiskState.OverallRisk == "high" || input.CurrentState.RiskState.OverallRisk == "critical"
	return DebtDecisionReport{
		TaskID:            spec.ID,
		WorkflowID:        workflowID,
		Conclusion:        strings.Join(conclusionParts, " "),
		Reasons:           reasons,
		Actions:           actions,
		Recommendations:   recommendations,
		RiskFlags:         append(append([]analysis.RiskFlag{}, cashflow.RiskFlags...), debt.RiskFlags...),
		Metrics:           debtDecisionMetricsMap(cashflow.DeterministicMetrics, debt.DeterministicMetrics),
		MetricRecords:     appendMetricRecords(cashflow.MetricRecords, debt.MetricRecords),
		EvidenceIDs:       sourceEvidenceIDs,
		SourceBlockIDs:    sourceBlockIDs,
		SourceMemoryIDs:   sourceMemoryIDs,
		SourceEvidenceIDs: sourceEvidenceIDs,
		GroundingRefs:     appendGroundingRefs(cashflow.GroundingRefs, debt.GroundingRefs),
		Caveats:           appendCaveats(cashflow.Caveats, debt.Caveats),
		ApprovalRequired:  overallRiskApproval || cashflow.ApprovalRequired || debt.ApprovalRequired,
		ApprovalReason:    firstNonEmpty(cashflow.ApprovalReason, debt.ApprovalReason, overallRiskApprovalReason(overallRiskApproval)),
		PolicyRuleRefs:    appendPolicyRuleRefs(cashflow.PolicyRuleRefs, debt.PolicyRuleRefs),
		Confidence:        averageConfidence(ordered),
		GeneratedAt:       a.now(),
	}, nil
}

func (a LifeEventAssessmentAggregator) Aggregate(spec taskspec.TaskSpec, workflowID string, input DraftInput) (LifeEventAssessmentReport, error) {
	ordered, err := orderedBlockResults(input.Plan, input.BlockResults)
	if err != nil {
		return LifeEventAssessmentReport{}, err
	}
	if input.TaskGraph == nil {
		return LifeEventAssessmentReport{}, fmt.Errorf("life event assessment draft requires generated task graph")
	}
	if err := input.TaskGraph.Validate(); err != nil {
		return LifeEventAssessmentReport{}, err
	}
	sourceBlockIDs, sourceMemoryIDs, sourceEvidenceIDs := collectProvenance(ordered)
	generatedTaskIDs := make([]string, 0, len(input.TaskGraph.GeneratedTasks))
	generatedTaskStatuses := make(map[string]string, len(input.TaskGraph.GeneratedTasks))
	requiredCapabilities := make(map[string]string, len(input.TaskGraph.GeneratedTasks))
	missingCapabilities := make(map[string]string, len(input.TaskGraph.GeneratedTasks))
	for _, item := range input.TaskGraph.GeneratedTasks {
		generatedTaskIDs = append(generatedTaskIDs, item.Task.ID)
		generatedTaskStatuses[item.Task.ID] = "generated"
		requiredCapabilities[item.Task.ID] = string(item.Task.UserIntentType) + "_workflow"
	}
	eventSummaryParts := make([]string, 0, len(ordered))
	for _, item := range ordered {
		eventSummaryParts = append(eventSummaryParts, item.Summary())
	}
	for _, note := range input.TaskGraph.SuppressionNotes {
		eventSummaryParts = append(eventSummaryParts, note)
	}
	return LifeEventAssessmentReport{
		TaskID:                spec.ID,
		WorkflowID:            workflowID,
		EventSummary:          strings.Join(eventSummaryParts, " "),
		StateDiffSummary:      append([]string{}, input.StateDiff...),
		MemoryUpdateSummary:   sourceMemoryIDs,
		GeneratedTaskIDs:      generatedTaskIDs,
		GeneratedTaskStatuses: generatedTaskStatuses,
		RequiredCapabilities:  requiredCapabilities,
		MissingCapabilities:   missingCapabilities,
		SourceBlockIDs:        sourceBlockIDs,
		SourceMemoryIDs:       sourceMemoryIDs,
		SourceEvidenceIDs:     sourceEvidenceIDs,
		GeneratedAt:           a.now(),
	}, nil
}

func (a TaxOptimizationAggregator) Aggregate(spec taskspec.TaskSpec, workflowID string, input DraftInput) (TaxOptimizationReport, error) {
	ordered, err := orderedBlockResults(input.Plan, input.BlockResults)
	if err != nil {
		return TaxOptimizationReport{}, err
	}
	if len(ordered) != 1 || ordered[0].Tax == nil {
		return TaxOptimizationReport{}, fmt.Errorf("tax optimization draft requires exactly one tax block result")
	}
	sourceBlockIDs, sourceMemoryIDs, sourceEvidenceIDs := collectProvenance(ordered)
	result := ordered[0].Tax
	return TaxOptimizationReport{
		TaskID:               spec.ID,
		WorkflowID:           workflowID,
		Summary:              result.Summary,
		DeterministicMetrics: taxMetricsMap(result.DeterministicMetrics),
		RecommendedActions:   recommendationsToSkillItems(result.Recommendations),
		Recommendations:      append([]analysis.Recommendation{}, result.Recommendations...),
		MetricRecords:        append([]finance.MetricRecord{}, result.MetricRecords...),
		SourceBlockIDs:       sourceBlockIDs,
		SourceMemoryIDs:      sourceMemoryIDs,
		SourceEvidenceIDs:    sourceEvidenceIDs,
		RiskFlags:            append([]analysis.RiskFlag{}, result.RiskFlags...),
		GroundingRefs:        append([]string{}, result.GroundingRefs...),
		Caveats:              append([]string{}, result.Caveats...),
		ApprovalRequired:     result.ApprovalRequired || input.CurrentState.RiskState.OverallRisk == "high" || input.CurrentState.RiskState.OverallRisk == "critical",
		ApprovalReason:       firstNonEmpty(result.ApprovalReason, overallRiskApprovalReason(input.CurrentState.RiskState.OverallRisk == "high" || input.CurrentState.RiskState.OverallRisk == "critical")),
		PolicyRuleRefs:       append([]string{}, result.PolicyRuleRefs...),
		Confidence:           result.Confidence,
		GeneratedAt:          a.now(),
	}, nil
}

func (a PortfolioRebalanceAggregator) Aggregate(spec taskspec.TaskSpec, workflowID string, input DraftInput) (PortfolioRebalanceReport, error) {
	ordered, err := orderedBlockResults(input.Plan, input.BlockResults)
	if err != nil {
		return PortfolioRebalanceReport{}, err
	}
	if len(ordered) != 1 || ordered[0].Portfolio == nil {
		return PortfolioRebalanceReport{}, fmt.Errorf("portfolio rebalance draft requires exactly one portfolio block result")
	}
	sourceBlockIDs, sourceMemoryIDs, sourceEvidenceIDs := collectProvenance(ordered)
	result := ordered[0].Portfolio
	return PortfolioRebalanceReport{
		TaskID:               spec.ID,
		WorkflowID:           workflowID,
		Summary:              result.Summary,
		DeterministicMetrics: portfolioMetricsMap(result.DeterministicMetrics),
		RecommendedActions:   recommendationsToSkillItems(result.Recommendations),
		Recommendations:      append([]analysis.Recommendation{}, result.Recommendations...),
		MetricRecords:        append([]finance.MetricRecord{}, result.MetricRecords...),
		SourceBlockIDs:       sourceBlockIDs,
		SourceMemoryIDs:      sourceMemoryIDs,
		SourceEvidenceIDs:    sourceEvidenceIDs,
		RiskFlags:            append([]analysis.RiskFlag{}, result.RiskFlags...),
		GroundingRefs:        append([]string{}, result.GroundingRefs...),
		Caveats:              append([]string{}, result.Caveats...),
		ApprovalRequired:     result.ApprovalRequired || input.CurrentState.RiskState.OverallRisk == "high" || input.CurrentState.RiskState.OverallRisk == "critical",
		ApprovalReason:       firstNonEmpty(result.ApprovalReason, overallRiskApprovalReason(input.CurrentState.RiskState.OverallRisk == "high" || input.CurrentState.RiskState.OverallRisk == "critical")),
		PolicyRuleRefs:       append([]string{}, result.PolicyRuleRefs...),
		Confidence:           result.Confidence,
		GeneratedAt:          a.now(),
	}, nil
}

func (a BehaviorInterventionAggregator) Aggregate(spec taskspec.TaskSpec, workflowID string, input DraftInput) (BehaviorInterventionReport, error) {
	ordered, err := orderedBlockResults(input.Plan, input.BlockResults)
	if err != nil {
		return BehaviorInterventionReport{}, err
	}
	if len(ordered) != 1 || ordered[0].Behavior == nil {
		return BehaviorInterventionReport{}, fmt.Errorf("behavior intervention draft requires exactly one behavior block result")
	}
	sourceBlockIDs, sourceMemoryIDs, sourceEvidenceIDs := collectProvenance(ordered)
	result := ordered[0].Behavior
	return BehaviorInterventionReport{
		TaskID:                spec.ID,
		WorkflowID:            workflowID,
		Summary:               result.Summary,
		DeterministicMetrics: map[string]any{
			"duplicate_subscription_count": result.DeterministicMetrics.DuplicateSubscriptionCount,
			"late_night_spend_count":       result.DeterministicMetrics.LateNightSpendCount,
			"late_night_spend_ratio":       result.DeterministicMetrics.LateNightSpendRatio,
			"discretionary_pressure_score": result.DeterministicMetrics.DiscretionaryPressureScore,
			"recurring_subscription_count": result.DeterministicMetrics.RecurringSubscriptionCount,
		},
		SelectedSkillFamily:   string(result.SelectedSkill.Family),
		SelectedSkillVersion:  string(result.SelectedSkill.Version),
		SelectedRecipeID:      result.SelectedSkill.RecipeID,
		SkillSelectionReasons: append([]string{}, result.SkillSelectionReasons...),
		Recommendations:       append([]analysis.Recommendation{}, result.Recommendations...),
		MetricRecords:         append([]finance.MetricRecord{}, result.MetricRecords...),
		SourceBlockIDs:        sourceBlockIDs,
		SourceMemoryIDs:       sourceMemoryIDs,
		SourceEvidenceIDs:     sourceEvidenceIDs,
		RiskFlags:             append([]analysis.RiskFlag{}, result.RiskFlags...),
		GroundingRefs:         append([]string{}, result.GroundingRefs...),
		Caveats:               append([]string{}, result.Caveats...),
		ApprovalRequired:      result.ApprovalRequired,
		ApprovalReason:        result.ApprovalReason,
		PolicyRuleRefs:        append([]string{}, result.PolicyRuleRefs...),
		Confidence:            result.Confidence,
		GeneratedAt:           a.now(),
	}, nil
}

func orderedBlockResults(plan planning.ExecutionPlan, results []analysis.BlockResultEnvelope) ([]analysis.BlockResultEnvelope, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	byID := make(map[string]analysis.BlockResultEnvelope, len(results))
	for _, result := range results {
		if err := result.Validate(); err != nil {
			return nil, err
		}
		byID[result.BlockID] = result
	}
	ordered := make([]analysis.BlockResultEnvelope, 0, len(plan.Blocks))
	for _, block := range plan.Blocks {
		result, ok := byID[string(block.ID)]
		if !ok {
			return nil, fmt.Errorf("missing block result for %s", block.ID)
		}
		ordered = append(ordered, result)
	}
	return ordered, nil
}

func collectProvenance(results []analysis.BlockResultEnvelope) ([]string, []string, []observation.EvidenceID) {
	blockIDs := make([]string, 0, len(results))
	memoryIDSet := make(map[string]struct{})
	evidenceIDSet := make(map[observation.EvidenceID]struct{})
	for _, result := range results {
		blockIDs = append(blockIDs, result.BlockID)
		for _, id := range result.MemoryIDsUsed() {
			memoryIDSet[id] = struct{}{}
		}
		for _, id := range result.EvidenceIDs() {
			evidenceIDSet[id] = struct{}{}
		}
	}
	memoryIDs := make([]string, 0, len(memoryIDSet))
	for id := range memoryIDSet {
		memoryIDs = append(memoryIDs, id)
	}
	sort.Strings(memoryIDs)
	evidenceIDs := make([]observation.EvidenceID, 0, len(evidenceIDSet))
	for id := range evidenceIDSet {
		evidenceIDs = append(evidenceIDs, id)
	}
	sort.Slice(evidenceIDs, func(i, j int) bool { return evidenceIDs[i] < evidenceIDs[j] })
	return blockIDs, memoryIDs, evidenceIDs
}

func riskFlagsToSkillItems(flags []analysis.RiskFlag) []skills.SkillItem {
	result := make([]skills.SkillItem, 0, len(flags))
	for _, flag := range flags {
		result = append(result, skills.SkillItem{
			Title:       flag.Code,
			Detail:      flag.Detail,
			Severity:    flag.Severity,
			EvidenceIDs: flag.EvidenceIDs,
		})
	}
	return result
}

func recommendationsToSkillItems(items []analysis.Recommendation) []skills.SkillItem {
	result := make([]skills.SkillItem, 0, len(items))
	for _, item := range items {
		result = append(result, skills.SkillItem{
			Title:    item.Title,
			Detail:   item.Detail,
			Severity: string(item.RiskLevel),
		})
	}
	return result
}

func deriveTodoItems(items []analysis.Recommendation) []skills.SkillItem {
	result := make([]skills.SkillItem, 0, len(items))
	for _, item := range items {
		result = append(result, skills.SkillItem{
			Title:    item.Title,
			Detail:   item.Detail,
			Severity: string(item.RiskLevel),
		})
	}
	return result
}

func cashflowMetricsMap(metrics analysis.CashflowDeterministicMetrics) map[string]any {
	return map[string]any{
		"monthly_inflow_cents":          metrics.MonthlyInflowCents,
		"monthly_outflow_cents":         metrics.MonthlyOutflowCents,
		"monthly_net_income_cents":      metrics.MonthlyNetIncomeCents,
		"savings_rate":                  metrics.SavingsRate,
		"duplicate_subscription_count":  metrics.DuplicateSubscriptionCount,
		"late_night_spending_frequency": metrics.LateNightSpendingFrequency,
	}
}

func debtDecisionMetricsMap(cashflow analysis.CashflowDeterministicMetrics, debt analysis.DebtDeterministicMetrics) map[string]any {
	return map[string]any{
		"monthly_net_income_cents":      cashflow.MonthlyNetIncomeCents,
		"savings_rate":                  cashflow.SavingsRate,
		"debt_burden_ratio":             debt.DebtBurdenRatio,
		"minimum_payment_pressure":      debt.MinimumPaymentPressure,
		"average_apr":                   debt.AverageAPR,
		"max_allocation_drift":          debt.MaxAllocationDrift,
		"overall_risk":                  debt.OverallRisk,
		"duplicate_subscription_count":  cashflow.DuplicateSubscriptionCount,
		"late_night_spending_frequency": cashflow.LateNightSpendingFrequency,
	}
}

func taxMetricsMap(metrics analysis.TaxDeterministicMetrics) map[string]any {
	return map[string]any{
		"effective_tax_rate":                metrics.EffectiveTaxRate,
		"tax_advantaged_contribution_cents": metrics.TaxAdvantagedContributionCents,
		"childcare_tax_signal":              metrics.ChildcareTaxSignal,
		"upcoming_deadline_count":           metrics.UpcomingDeadlineCount,
	}
}

func portfolioMetricsMap(metrics analysis.PortfolioDeterministicMetrics) map[string]any {
	return map[string]any{
		"total_investable_assets_cents": metrics.TotalInvestableAssetsCents,
		"emergency_fund_months":         metrics.EmergencyFundMonths,
		"max_allocation_drift":          metrics.MaxAllocationDrift,
		"cash_allocation":               metrics.CashAllocation,
	}
}

func appendMetricRecords(groups ...[]finance.MetricRecord) []finance.MetricRecord {
	seen := make(map[string]struct{})
	result := make([]finance.MetricRecord, 0)
	for _, group := range groups {
		for _, record := range group {
			if record.Ref == "" {
				continue
			}
			if _, ok := seen[record.Ref]; ok {
				continue
			}
			seen[record.Ref] = struct{}{}
			result = append(result, record)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Ref < result[j].Ref })
	return result
}

func appendGroundingRefs(groups ...[]string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, group := range groups {
		for _, item := range group {
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	sort.Strings(result)
	return result
}

func appendPolicyRuleRefs(groups ...[]string) []string {
	return appendGroundingRefs(groups...)
}

func appendCaveats(groups ...[]string) []string {
	return appendGroundingRefs(groups...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func overallRiskApprovalReason(active bool) string {
	if !active {
		return ""
	}
	return "当前总体风险已处于高位，需要治理审批后再发布最终建议"
}

func (a MonthlyReviewAggregator) engine() finance.Engine {
	if a.Engine != nil {
		return a.Engine
	}
	return finance.DeterministicEngine{}
}

func averageConfidence(results []analysis.BlockResultEnvelope) float64 {
	if len(results) == 0 {
		return 0
	}
	total := 0.0
	for _, item := range results {
		switch {
		case item.Cashflow != nil:
			total += item.Cashflow.Confidence
		case item.Debt != nil:
			total += item.Debt.Confidence
		case item.Tax != nil:
			total += item.Tax.Confidence
		case item.Portfolio != nil:
			total += item.Portfolio.Confidence
		}
	}
	return total / float64(len(results))
}

func (a MonthlyReviewAggregator) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func (a DebtDecisionAggregator) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func (a LifeEventAssessmentAggregator) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func (a TaxOptimizationAggregator) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func (a PortfolioRebalanceAggregator) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func (a BehaviorInterventionAggregator) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}
