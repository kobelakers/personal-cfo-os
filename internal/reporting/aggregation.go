package reporting

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

type MonthlyReviewAggregator struct {
	TaxSignals tools.ComputeTaxSignalTool
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
	riskItems := append(riskFlagsToSkillItems(cashflow.RiskFlags), riskFlagsToSkillItems(debt.RiskFlags)...)
	optimizationSuggestions := append(append([]skills.SkillItem{}, cashflow.Recommendations...), debt.Recommendations...)
	todoItems := deriveTodoItems(optimizationSuggestions)
	summaryParts := make([]string, 0, len(ordered))
	for _, item := range ordered {
		summaryParts = append(summaryParts, item.Summary())
	}
	taxSignals := a.TaxSignals.Compute(input.CurrentState)
	return MonthlyReviewReport{
		TaskID:                  spec.ID,
		WorkflowID:              workflowID,
		Summary:                 strings.Join(summaryParts, " "),
		CashflowMetrics:         cashflowMetricsMap(cashflow.DeterministicMetrics),
		TaxSignals:              taxSignals,
		RiskItems:               riskItems,
		OptimizationSuggestions: optimizationSuggestions,
		TodoItems:               todoItems,
		SourceBlockIDs:          sourceBlockIDs,
		SourceMemoryIDs:         sourceMemoryIDs,
		SourceEvidenceIDs:       sourceEvidenceIDs,
		ApprovalRequired:        input.CurrentState.RiskState.OverallRisk == "high",
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
	actions := append(append([]skills.SkillItem{}, cashflow.Recommendations...), debt.Recommendations...)
	conclusionParts := make([]string, 0, len(ordered))
	for _, item := range ordered {
		conclusionParts = append(conclusionParts, item.Summary())
	}
	return DebtDecisionReport{
		TaskID:            spec.ID,
		WorkflowID:        workflowID,
		Conclusion:        strings.Join(conclusionParts, " "),
		Reasons:           reasons,
		Actions:           actions,
		Metrics:           debtDecisionMetricsMap(cashflow.DeterministicMetrics, debt.DeterministicMetrics),
		EvidenceIDs:       sourceEvidenceIDs,
		SourceBlockIDs:    sourceBlockIDs,
		SourceMemoryIDs:   sourceMemoryIDs,
		SourceEvidenceIDs: sourceEvidenceIDs,
		ApprovalRequired:  input.CurrentState.RiskState.OverallRisk == "high",
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
		RecommendedActions:   append([]skills.SkillItem{}, result.Recommendations...),
		SourceBlockIDs:       sourceBlockIDs,
		SourceMemoryIDs:      sourceMemoryIDs,
		SourceEvidenceIDs:    sourceEvidenceIDs,
		RiskFlags:            append([]analysis.RiskFlag{}, result.RiskFlags...),
		ApprovalRequired:     input.CurrentState.RiskState.OverallRisk == "high",
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
		RecommendedActions:   append([]skills.SkillItem{}, result.Recommendations...),
		SourceBlockIDs:       sourceBlockIDs,
		SourceMemoryIDs:      sourceMemoryIDs,
		SourceEvidenceIDs:    sourceEvidenceIDs,
		RiskFlags:            append([]analysis.RiskFlag{}, result.RiskFlags...),
		ApprovalRequired:     input.CurrentState.RiskState.OverallRisk == "high",
		Confidence:           result.Confidence,
		GeneratedAt:          a.now(),
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

func deriveTodoItems(items []skills.SkillItem) []skills.SkillItem {
	result := make([]skills.SkillItem, 0, len(items))
	for _, item := range items {
		result = append(result, skills.SkillItem{
			Title:       item.Title,
			Detail:      item.Detail,
			Severity:    item.Severity,
			EvidenceIDs: item.EvidenceIDs,
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
