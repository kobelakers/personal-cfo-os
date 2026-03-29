package verification

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type CashflowBlockValidator struct{}
type DebtBlockValidator struct{}
type TaxBlockValidator struct{}
type PortfolioBlockValidator struct{}

func (CashflowBlockValidator) Validate(
	spec taskspec.TaskSpec,
	block planning.ExecutionBlock,
	result analysis.CashflowBlockResult,
	verificationContext contextview.BlockVerificationContext,
	currentState state.FinancialWorldState,
) VerificationResult {
	failedRules := make([]string, 0)
	missingEvidence := missingMandatoryEvidence(block, verificationContext)
	if len(missingEvidence) > 0 {
		failedRules = append(failedRules, "mandatory_evidence_gap")
	}
	if result.BlockID == "" || result.Summary == "" || len(result.EvidenceIDs) == 0 {
		failedRules = append(failedRules, "schema_invalid")
	}
	if !recommendationsGrounded(result.Recommendations, verificationContext.SelectedEvidenceIDs) {
		failedRules = append(failedRules, "grounding_failure")
	}
	if len(result.Caveats) == 0 {
		failedRules = append(failedRules, "caveat_missing")
	}
	if !cashflowMetricRefsConsistent(result.MetricRefs) {
		failedRules = append(failedRules, "metric_ref_invalid")
	}
	if !cashflowMetricsConsistent(result.DeterministicMetrics, currentState.CashflowState) {
		failedRules = append(failedRules, "metric_consistency")
	}

	status, message := verificationOutcome(failedRules, "cashflow block verification passed", "cashflow block validation failed")
	return VerificationResult{
		Status:                  status,
		Scope:                   VerificationScopeBlock,
		BlockID:                 string(block.ID),
		BlockKind:               string(block.Kind),
		Validator:               "cashflow_block_validator",
		Message:                 message,
		FailedRules:             failedRules,
		MissingEvidence:         missingEvidence,
		RecommendedReplanAction: "collect missing cashflow evidence or repair cashflow block grounding",
		Severity:                severityForStatus(status),
		Details: map[string]any{
			"task_id":               spec.ID,
			"selected_evidence_ids": verificationContext.SelectedEvidenceIDs,
			"selected_memory_ids":   verificationContext.SelectedMemoryIDs,
			"selected_state_blocks": verificationContext.SelectedStateBlocks,
		},
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        time.Now().UTC(),
	}
}

func cashflowMetricRefsConsistent(metricRefs []string) bool {
	allowed := map[string]struct{}{
		"monthly_inflow_cents":          {},
		"monthly_outflow_cents":         {},
		"monthly_net_income_cents":      {},
		"savings_rate":                  {},
		"duplicate_subscription_count":  {},
		"late_night_spending_frequency": {},
	}
	for _, ref := range metricRefs {
		if _, ok := allowed[ref]; !ok {
			return false
		}
	}
	return true
}

func (DebtBlockValidator) Validate(
	spec taskspec.TaskSpec,
	block planning.ExecutionBlock,
	result analysis.DebtBlockResult,
	verificationContext contextview.BlockVerificationContext,
	currentState state.FinancialWorldState,
) VerificationResult {
	failedRules := make([]string, 0)
	missingEvidence := missingMandatoryEvidence(block, verificationContext)
	if len(missingEvidence) > 0 {
		failedRules = append(failedRules, "mandatory_evidence_gap")
	}
	if result.BlockID == "" || result.Summary == "" || len(result.EvidenceIDs) == 0 {
		failedRules = append(failedRules, "schema_invalid")
	}
	if !recommendationsGrounded(result.Recommendations, verificationContext.SelectedEvidenceIDs) {
		failedRules = append(failedRules, "grounding_failure")
	}
	if len(result.RiskFlags) == 0 {
		failedRules = append(failedRules, "risk_caveat_missing")
	}
	if !debtMetricsConsistent(result.DeterministicMetrics, currentState.LiabilityState, currentState.RiskState.OverallRisk) {
		failedRules = append(failedRules, "metric_consistency")
	}

	status, message := verificationOutcome(failedRules, "debt block verification passed", "debt block validation failed")
	return VerificationResult{
		Status:                  status,
		Scope:                   VerificationScopeBlock,
		BlockID:                 string(block.ID),
		BlockKind:               string(block.Kind),
		Validator:               "debt_block_validator",
		Message:                 message,
		FailedRules:             failedRules,
		MissingEvidence:         missingEvidence,
		RecommendedReplanAction: "collect missing debt evidence or repair debt tradeoff grounding",
		Severity:                severityForStatus(status),
		Details: map[string]any{
			"task_id":               spec.ID,
			"selected_evidence_ids": verificationContext.SelectedEvidenceIDs,
			"selected_memory_ids":   verificationContext.SelectedMemoryIDs,
			"selected_state_blocks": verificationContext.SelectedStateBlocks,
		},
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        time.Now().UTC(),
	}
}

func (TaxBlockValidator) Validate(
	spec taskspec.TaskSpec,
	block planning.ExecutionBlock,
	result analysis.TaxBlockResult,
	verificationContext contextview.BlockVerificationContext,
	currentState state.FinancialWorldState,
) VerificationResult {
	failedRules := make([]string, 0)
	missingEvidence := missingMandatoryEvidence(block, verificationContext)
	if len(missingEvidence) > 0 {
		failedRules = append(failedRules, "mandatory_evidence_gap")
	}
	if result.BlockID == "" || result.Summary == "" || len(result.EvidenceIDs) == 0 {
		failedRules = append(failedRules, "schema_invalid")
	}
	if !recommendationsGrounded(result.Recommendations, verificationContext.SelectedEvidenceIDs) {
		failedRules = append(failedRules, "grounding_failure")
	}
	if !taxMetricsConsistent(result.DeterministicMetrics, currentState.TaxState) {
		failedRules = append(failedRules, "metric_consistency")
	}

	status, message := verificationOutcome(failedRules, "tax block verification passed", "tax block validation failed")
	return VerificationResult{
		Status:                  status,
		Scope:                   VerificationScopeBlock,
		BlockID:                 string(block.ID),
		BlockKind:               string(block.Kind),
		Validator:               "tax_block_validator",
		Message:                 message,
		FailedRules:             failedRules,
		MissingEvidence:         missingEvidence,
		RecommendedReplanAction: "collect missing tax evidence or repair tax grounding before generating follow-up tasks",
		Severity:                severityForStatus(status),
		Details: map[string]any{
			"task_id":               spec.ID,
			"selected_evidence_ids": verificationContext.SelectedEvidenceIDs,
			"selected_memory_ids":   verificationContext.SelectedMemoryIDs,
			"selected_state_blocks": verificationContext.SelectedStateBlocks,
		},
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        time.Now().UTC(),
	}
}

func (PortfolioBlockValidator) Validate(
	spec taskspec.TaskSpec,
	block planning.ExecutionBlock,
	result analysis.PortfolioBlockResult,
	verificationContext contextview.BlockVerificationContext,
	currentState state.FinancialWorldState,
) VerificationResult {
	failedRules := make([]string, 0)
	missingEvidence := missingMandatoryEvidence(block, verificationContext)
	if len(missingEvidence) > 0 {
		failedRules = append(failedRules, "mandatory_evidence_gap")
	}
	if result.BlockID == "" || result.Summary == "" || len(result.EvidenceIDs) == 0 {
		failedRules = append(failedRules, "schema_invalid")
	}
	if !recommendationsGrounded(result.Recommendations, verificationContext.SelectedEvidenceIDs) {
		failedRules = append(failedRules, "grounding_failure")
	}
	if !portfolioMetricsConsistent(result.DeterministicMetrics, currentState.PortfolioState) {
		failedRules = append(failedRules, "metric_consistency")
	}

	status, message := verificationOutcome(failedRules, "portfolio block verification passed", "portfolio block validation failed")
	return VerificationResult{
		Status:                  status,
		Scope:                   VerificationScopeBlock,
		BlockID:                 string(block.ID),
		BlockKind:               string(block.Kind),
		Validator:               "portfolio_block_validator",
		Message:                 message,
		FailedRules:             failedRules,
		MissingEvidence:         missingEvidence,
		RecommendedReplanAction: "collect missing portfolio evidence or repair portfolio grounding before generating follow-up tasks",
		Severity:                severityForStatus(status),
		Details: map[string]any{
			"task_id":               spec.ID,
			"selected_evidence_ids": verificationContext.SelectedEvidenceIDs,
			"selected_memory_ids":   verificationContext.SelectedMemoryIDs,
			"selected_state_blocks": verificationContext.SelectedStateBlocks,
		},
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        time.Now().UTC(),
	}
}

func missingMandatoryEvidence(block planning.ExecutionBlock, verificationContext contextview.BlockVerificationContext) []string {
	typeSet := make(map[string]struct{}, len(verificationContext.Slice.EvidenceBlocks))
	for _, item := range verificationContext.Slice.EvidenceBlocks {
		typeSet[string(item.Type)] = struct{}{}
	}
	missing := make([]string, 0)
	for _, req := range block.RequiredEvidenceRefs {
		if !req.Mandatory {
			continue
		}
		if _, ok := typeSet[req.Type]; !ok {
			missing = append(missing, req.Type)
		}
	}
	return missing
}

func recommendationsGrounded(items []skills.SkillItem, selected []observation.EvidenceID) bool {
	allowed := make(map[string]struct{}, len(selected))
	for _, id := range selected {
		allowed[string(id)] = struct{}{}
	}
	for _, item := range items {
		if len(item.EvidenceIDs) == 0 {
			return false
		}
		grounded := false
		for _, id := range item.EvidenceIDs {
			if _, ok := allowed[string(id)]; ok {
				grounded = true
				break
			}
		}
		if !grounded {
			return false
		}
	}
	return true
}

func cashflowMetricsConsistent(metrics analysis.CashflowDeterministicMetrics, current state.CashflowState) bool {
	return metrics.MonthlyInflowCents == current.MonthlyInflowCents &&
		metrics.MonthlyOutflowCents == current.MonthlyOutflowCents &&
		metrics.MonthlyNetIncomeCents == current.MonthlyNetIncomeCents &&
		floatClose(metrics.SavingsRate, current.SavingsRate)
}

func debtMetricsConsistent(metrics analysis.DebtDeterministicMetrics, current state.LiabilityState, overallRisk string) bool {
	return floatClose(metrics.DebtBurdenRatio, current.DebtBurdenRatio) &&
		floatClose(metrics.MinimumPaymentPressure, current.MinimumPaymentPressure) &&
		floatClose(metrics.AverageAPR, current.AverageAPR) &&
		metrics.OverallRisk == overallRisk
}

func taxMetricsConsistent(metrics analysis.TaxDeterministicMetrics, current state.TaxState) bool {
	return floatClose(metrics.EffectiveTaxRate, current.EffectiveTaxRate) &&
		metrics.TaxAdvantagedContributionCents == current.TaxAdvantagedContributionCents &&
		metrics.ChildcareTaxSignal == current.ChildcareTaxSignal &&
		metrics.UpcomingDeadlineCount == len(current.UpcomingDeadlines)
}

func portfolioMetricsConsistent(metrics analysis.PortfolioDeterministicMetrics, current state.PortfolioState) bool {
	return metrics.TotalInvestableAssetsCents == current.TotalInvestableAssetsCents &&
		floatClose(metrics.EmergencyFundMonths, current.EmergencyFundMonths) &&
		floatClose(metrics.MaxAllocationDrift, currentPortfolioMaxDrift(current)) &&
		floatClose(metrics.CashAllocation, currentPortfolioCashAllocation(current))
}

func currentPortfolioMaxDrift(current state.PortfolioState) float64 {
	maximum := 0.0
	for _, drift := range current.AllocationDrift {
		if drift < 0 {
			drift = -drift
		}
		if drift > maximum {
			maximum = drift
		}
	}
	return maximum
}

func currentPortfolioCashAllocation(current state.PortfolioState) float64 {
	if current.AssetAllocations == nil {
		return 0
	}
	return current.AssetAllocations["cash"]
}

func floatClose(left float64, right float64) bool {
	return math.Abs(left-right) <= 0.0001
}

func verificationOutcome(failedRules []string, passMessage string, failMessage string) (VerificationStatus, string) {
	if len(failedRules) == 0 {
		return VerificationStatusPass, passMessage
	}
	severeRules := []string{"mandatory_evidence_gap", "schema_invalid", "grounding_failure"}
	for _, rule := range severeRules {
		if slices.Contains(failedRules, rule) {
			return VerificationStatusFail, fmt.Sprintf("%s: %s", failMessage, rule)
		}
	}
	return VerificationStatusNeedsReplan, fmt.Sprintf("%s: %v", failMessage, failedRules)
}

func severeBlockFailure(result VerificationResult) bool {
	if result.Scope != VerificationScopeBlock || result.Status != VerificationStatusFail {
		return false
	}
	for _, rule := range result.FailedRules {
		switch rule {
		case "mandatory_evidence_gap", "schema_invalid", "grounding_failure":
			return true
		}
	}
	return false
}
