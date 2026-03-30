package verification

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type DefaultEvidenceCoverageChecker struct{}

func (DefaultEvidenceCoverageChecker) Check(spec taskspec.TaskSpec, evidence []observation.EvidenceRecord) (EvidenceCoverageReport, error) {
	items := make([]EvidenceCoverageItem, 0, len(spec.RequiredEvidence))
	coveredCount := 0
	for _, requirement := range spec.RequiredEvidence {
		evidenceIDs := matchingEvidenceIDs(requirement.Type, evidence)
		covered := len(evidenceIDs) > 0
		if covered {
			coveredCount++
		}
		item := EvidenceCoverageItem{
			RequirementID: requirement.Type,
			Covered:       covered,
			EvidenceIDs:   evidenceIDs,
		}
		if !covered {
			if requirement.Mandatory {
				item.GapReason = "mandatory evidence missing"
			} else {
				item.GapReason = "optional evidence missing"
			}
		}
		items = append(items, item)
	}
	coverageRatio := 1.0
	if len(spec.RequiredEvidence) > 0 {
		coverageRatio = float64(coveredCount) / float64(len(spec.RequiredEvidence))
	}
	return EvidenceCoverageReport{
		TaskID:        spec.ID,
		CoverageRatio: coverageRatio,
		Items:         items,
	}, nil
}

type MonthlyReviewDeterministicValidator struct{}

func (MonthlyReviewDeterministicValidator) Validate(_ context.Context, spec taskspec.TaskSpec, currentState state.FinancialWorldState, _ []observation.EvidenceRecord, output any) (VerificationResult, error) {
	payload, err := toMap(output)
	if err != nil {
		return VerificationResult{}, err
	}
	requiredKeys := []string{"summary", "risk_items", "optimization_suggestions", "todo_items", "cashflow_metrics"}
	missing := missingKeys(payload, requiredKeys)
	status := VerificationStatusPass
	message := "monthly review output structure is valid"
	details := map[string]any{
		"required_keys": requiredKeys,
	}
	failedRules := []string{}
	if len(missing) > 0 {
		status = VerificationStatusFail
		message = "missing report keys: " + strings.Join(missing, ", ")
		failedRules = append(failedRules, "required_output_keys")
		details["missing_keys"] = missing
	}
	if currentState.CashflowState.MonthlyInflowCents == 0 || currentState.CashflowState.MonthlyOutflowCents == 0 {
		status = VerificationStatusNeedsReplan
		message = "cashflow metrics are incomplete"
		failedRules = append(failedRules, "cashflow_metrics_complete")
		details["cashflow_complete"] = false
	}
	return VerificationResult{
		Status:                  status,
		Validator:               "monthly_review_deterministic_validator",
		Message:                 message,
		Details:                 details,
		FailedRules:             failedRules,
		RecommendedReplanAction: "repair report structure and recompute missing cashflow metrics",
		Severity:                severityForStatus(status),
		EvidenceCoverage:        fullCoverage(spec.ID),
		CheckedAt:               time.Now().UTC(),
	}, nil
}

type MonthlyReviewBusinessValidator struct{}

func (MonthlyReviewBusinessValidator) Validate(_ context.Context, spec taskspec.TaskSpec, currentState state.FinancialWorldState, _ []observation.EvidenceRecord, output any) (VerificationResult, error) {
	payload, err := toMap(output)
	if err != nil {
		return VerificationResult{}, err
	}
	reportText := flattenOutput(payload)

	status := VerificationStatusPass
	message := "monthly review business checks passed"
	failedRules := []string{}
	missingEvidence := []string{}
	switch {
	case currentState.LiabilityState.DebtBurdenRatio >= 0.2 && !containsAny(reportText, "债务", "debt"):
		status = VerificationStatusFail
		message = "high-risk debt signal was omitted from report"
		failedRules = append(failedRules, "debt_signal_coverage")
		missingEvidence = append(missingEvidence, "debt_obligation_snapshot")
	case currentState.TaxState.ChildcareTaxSignal && !containsAny(reportText, "税", "tax", "childcare", "育儿"):
		status = VerificationStatusFail
		message = "obvious tax signal was omitted from report"
		failedRules = append(failedRules, "tax_signal_coverage")
		missingEvidence = append(missingEvidence, "tax_document")
	case currentState.RiskState.OverallRisk == "high" && !extractBool(payload, "approval_required"):
		status = VerificationStatusNeedsReplan
		message = "high overall risk should have triggered approval_required"
		failedRules = append(failedRules, "approval_required_high_risk")
	case currentState.CashflowState.SavingsRate < 0.1 && !containsAny(reportText, "现金流", "cashflow", "储蓄率", "savings"):
		status = VerificationStatusNeedsReplan
		message = "cashflow constraint explanation is missing"
		failedRules = append(failedRules, "cashflow_constraint_explanation")
	}
	return VerificationResult{
		Status:                  status,
		Validator:               "monthly_review_business_validator",
		Message:                 message,
		FailedRules:             failedRules,
		MissingEvidence:         missingEvidence,
		RecommendedReplanAction: "augment report with missing business risk explanations",
		Severity:                severityForStatus(status),
		EvidenceCoverage:        fullCoverage(spec.ID),
		CheckedAt:               time.Now().UTC(),
	}, nil
}

type DebtDecisionBusinessValidator struct{}

func (DebtDecisionBusinessValidator) Validate(_ context.Context, spec taskspec.TaskSpec, currentState state.FinancialWorldState, _ []observation.EvidenceRecord, output any) (VerificationResult, error) {
	payload, err := toMap(output)
	if err != nil {
		return VerificationResult{}, err
	}
	reportText := flattenOutput(payload)
	status := VerificationStatusPass
	message := "debt decision business checks passed"
	failedRules := []string{}
	switch {
	case extractString(payload, "conclusion") == "":
		status = VerificationStatusFail
		message = "debt decision conclusion is missing"
		failedRules = append(failedRules, "decision_conclusion_required")
	case currentState.LiabilityState.DebtBurdenRatio >= 0.2 && !containsAny(reportText, "债务", "debt", "还"):
		status = VerificationStatusFail
		message = "high debt burden is not reflected in decision output"
		failedRules = append(failedRules, "debt_burden_coverage")
	case currentState.RiskState.OverallRisk == "high" && !extractBool(payload, "approval_required"):
		status = VerificationStatusNeedsReplan
		message = "high-risk debt decision should require approval"
		failedRules = append(failedRules, "approval_required_high_risk")
	}
	return VerificationResult{
		Status:                  status,
		Validator:               "debt_decision_business_validator",
		Message:                 message,
		FailedRules:             failedRules,
		RecommendedReplanAction: "recompute decision narrative with explicit debt and approval reasoning",
		Severity:                severityForStatus(status),
		EvidenceCoverage:        fullCoverage(spec.ID),
		CheckedAt:               time.Now().UTC(),
	}, nil
}

type TaxOptimizationDeterministicValidator struct{}

func (TaxOptimizationDeterministicValidator) Validate(_ context.Context, spec taskspec.TaskSpec, _ state.FinancialWorldState, _ []observation.EvidenceRecord, output any) (VerificationResult, error) {
	payload, err := toMap(output)
	if err != nil {
		return VerificationResult{}, err
	}
	requiredKeys := []string{"summary", "deterministic_metrics", "recommended_actions", "risk_flags"}
	missing := missingKeys(payload, requiredKeys)
	status := VerificationStatusPass
	message := "tax optimization output structure is valid"
	failedRules := []string{}
	if len(missing) > 0 {
		status = VerificationStatusFail
		message = "missing tax optimization keys: " + strings.Join(missing, ", ")
		failedRules = append(failedRules, "required_output_keys")
	}
	return VerificationResult{
		Status:                  status,
		Validator:               "tax_optimization_deterministic_validator",
		Message:                 message,
		FailedRules:             failedRules,
		RecommendedReplanAction: "repair tax optimization output structure and regenerate the report",
		Severity:                severityForStatus(status),
		EvidenceCoverage:        fullCoverage(spec.ID),
		CheckedAt:               time.Now().UTC(),
	}, nil
}

type TaxOptimizationBusinessValidator struct{}

func (TaxOptimizationBusinessValidator) Validate(_ context.Context, spec taskspec.TaskSpec, currentState state.FinancialWorldState, _ []observation.EvidenceRecord, output any) (VerificationResult, error) {
	payload, err := toMap(output)
	if err != nil {
		return VerificationResult{}, err
	}
	reportText := flattenOutput(payload)
	status := VerificationStatusPass
	message := "tax optimization business checks passed"
	failedRules := []string{}
	switch {
	case currentState.TaxState.ChildcareTaxSignal && !containsAny(reportText, "tax", "税", "childcare", "育儿"):
		status = VerificationStatusFail
		message = "childcare tax signal is not reflected in tax optimization output"
		failedRules = append(failedRules, "childcare_tax_signal_coverage")
	case len(currentState.TaxState.UpcomingDeadlines) > 0 && !containsAny(reportText, "deadline", "截止", "预扣", "withholding"):
		status = VerificationStatusNeedsReplan
		message = "deadline-sensitive or withholding caveat is missing"
		failedRules = append(failedRules, "deadline_caveat_required")
	case currentState.RiskState.OverallRisk == "high" && extractListLen(payload, "risk_flags") == 0:
		status = VerificationStatusNeedsReplan
		message = "high-risk tax optimization requires explicit structured risk flags"
		failedRules = append(failedRules, "risk_flags_required_high_risk")
	}
	return VerificationResult{
		Status:                  status,
		Validator:               "tax_optimization_business_validator",
		Message:                 message,
		FailedRules:             failedRules,
		RecommendedReplanAction: "rebuild the tax optimization report with explicit deadline, withholding, and approval reasoning",
		Severity:                severityForStatus(status),
		EvidenceCoverage:        fullCoverage(spec.ID),
		CheckedAt:               time.Now().UTC(),
	}, nil
}

type PortfolioRebalanceDeterministicValidator struct{}

func (PortfolioRebalanceDeterministicValidator) Validate(_ context.Context, spec taskspec.TaskSpec, _ state.FinancialWorldState, _ []observation.EvidenceRecord, output any) (VerificationResult, error) {
	payload, err := toMap(output)
	if err != nil {
		return VerificationResult{}, err
	}
	requiredKeys := []string{"summary", "deterministic_metrics", "recommended_actions", "risk_flags"}
	missing := missingKeys(payload, requiredKeys)
	status := VerificationStatusPass
	message := "portfolio rebalance output structure is valid"
	failedRules := []string{}
	if len(missing) > 0 {
		status = VerificationStatusFail
		message = "missing portfolio rebalance keys: " + strings.Join(missing, ", ")
		failedRules = append(failedRules, "required_output_keys")
	}
	return VerificationResult{
		Status:                  status,
		Validator:               "portfolio_rebalance_deterministic_validator",
		Message:                 message,
		FailedRules:             failedRules,
		RecommendedReplanAction: "repair portfolio rebalance output structure and regenerate the report",
		Severity:                severityForStatus(status),
		EvidenceCoverage:        fullCoverage(spec.ID),
		CheckedAt:               time.Now().UTC(),
	}, nil
}

type PortfolioRebalanceBusinessValidator struct{}
type BehaviorInterventionDeterministicValidator struct{}
type BehaviorInterventionBusinessValidator struct{}

func (PortfolioRebalanceBusinessValidator) Validate(_ context.Context, spec taskspec.TaskSpec, currentState state.FinancialWorldState, _ []observation.EvidenceRecord, output any) (VerificationResult, error) {
	payload, err := toMap(output)
	if err != nil {
		return VerificationResult{}, err
	}
	reportText := flattenOutput(payload)
	status := VerificationStatusPass
	message := "portfolio rebalance business checks passed"
	failedRules := []string{}
	switch {
	case currentState.PortfolioState.EmergencyFundMonths < 3 && !containsAny(reportText, "liquidity", "流动性", "现金", "应急金"):
		status = VerificationStatusNeedsReplan
		message = "liquidity caveat is missing from portfolio rebalance output"
		failedRules = append(failedRules, "liquidity_caveat_required")
	case len(currentState.PortfolioState.AllocationDrift) > 0 && !containsAny(reportText, "rebalance", "再平衡", "drift", "配置"):
		status = VerificationStatusNeedsReplan
		message = "rebalance caveat is missing from portfolio output"
		failedRules = append(failedRules, "rebalance_caveat_required")
	case currentState.RiskState.OverallRisk == "high" && extractListLen(payload, "risk_flags") == 0:
		status = VerificationStatusNeedsReplan
		message = "high-risk portfolio rebalance requires explicit structured risk flags"
		failedRules = append(failedRules, "risk_flags_required_high_risk")
	}
	return VerificationResult{
		Status:                  status,
		Validator:               "portfolio_rebalance_business_validator",
		Message:                 message,
		FailedRules:             failedRules,
		RecommendedReplanAction: "rebuild the portfolio rebalance report with explicit liquidity and rebalance caveats",
		Severity:                severityForStatus(status),
		EvidenceCoverage:        fullCoverage(spec.ID),
		CheckedAt:               time.Now().UTC(),
	}, nil
}

type DefaultSuccessCriteriaChecker struct{}

func (DefaultSuccessCriteriaChecker) Check(spec taskspec.TaskSpec, results []VerificationResult, output any) (VerificationResult, error) {
	_ = output
	failures := make([]string, 0)
	for _, result := range results {
		if result.Status == VerificationStatusFail || result.Status == VerificationStatusBlocked {
			failures = append(failures, result.Validator)
		}
	}
	status := VerificationStatusPass
	message := "success criteria satisfied"
	if len(failures) > 0 {
		status = VerificationStatusNeedsReplan
		message = "validators failed: " + strings.Join(failures, ", ")
	}
	return VerificationResult{
		Status:                  status,
		Validator:               "success_criteria_checker",
		Message:                 message,
		FailedRules:             failures,
		RecommendedReplanAction: "rerun the plan with verification gaps resolved",
		Severity:                severityForStatus(status),
		EvidenceCoverage:        fullCoverage(spec.ID),
		CheckedAt:               time.Now().UTC(),
	}, nil
}

type BaselineTrajectoryOracle struct{}

func (BaselineTrajectoryOracle) Evaluate(_ context.Context, scenario string, results []VerificationResult) (OracleVerdict, error) {
	if len(results) == 0 {
		return OracleVerdict{
			Scenario:  scenario,
			Passed:    false,
			Score:     0,
			Reasons:   []string{"no verification results provided"},
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	passedCount := 0
	reasons := make([]string, 0)
	for _, result := range results {
		if result.Status == VerificationStatusPass {
			passedCount++
			continue
		}
		reasons = append(reasons, result.Validator+": "+result.Message)
	}
	score := float64(passedCount) / float64(len(results))
	return OracleVerdict{
		Scenario:  scenario,
		Passed:    passedCount == len(results),
		Score:     score,
		Reasons:   reasons,
		CheckedAt: time.Now().UTC(),
	}, nil
}

func matchingEvidenceIDs(requirementType string, evidence []observation.EvidenceRecord) []observation.EvidenceID {
	ids := make([]observation.EvidenceID, 0)
	for _, record := range evidence {
		if string(record.Type) == requirementType {
			ids = append(ids, record.ID)
		}
	}
	return ids
}

func fullCoverage(taskID string) EvidenceCoverageReport {
	return EvidenceCoverageReport{
		TaskID:        taskID,
		CoverageRatio: 1,
		Items:         nil,
	}
}

func (BehaviorInterventionDeterministicValidator) Validate(_ context.Context, spec taskspec.TaskSpec, _ state.FinancialWorldState, _ []observation.EvidenceRecord, output any) (VerificationResult, error) {
	payload, err := toMap(output)
	if err != nil {
		return VerificationResult{}, err
	}
	requiredKeys := []string{"summary", "deterministic_metrics", "selected_skill_family", "selected_recipe_id", "recommendations", "risk_flags"}
	missing := missingKeys(payload, requiredKeys)
	status := VerificationStatusPass
	message := "behavior intervention output structure is valid"
	failedRules := []string{}
	if len(missing) > 0 {
		status = VerificationStatusFail
		message = "missing behavior intervention keys: " + strings.Join(missing, ", ")
		failedRules = append(failedRules, "required_output_keys")
	}
	return VerificationResult{
		Status:                  status,
		Validator:               "behavior_intervention_deterministic_validator",
		Message:                 message,
		FailedRules:             failedRules,
		RecommendedReplanAction: "repair behavior report structure and regenerate the intervention output",
		Severity:                severityForStatus(status),
		EvidenceCoverage:        fullCoverage(spec.ID),
		CheckedAt:               time.Now().UTC(),
	}, nil
}

func (BehaviorInterventionBusinessValidator) Validate(_ context.Context, spec taskspec.TaskSpec, currentState state.FinancialWorldState, _ []observation.EvidenceRecord, output any) (VerificationResult, error) {
	payload, err := toMap(output)
	if err != nil {
		return VerificationResult{}, err
	}
	reportText := flattenOutput(payload)
	status := VerificationStatusPass
	message := "behavior intervention business checks passed"
	failedRules := []string{}
	switch {
	case currentState.BehaviorState.DuplicateSubscriptionCount >= 2 && !containsAny(reportText, "订阅", "subscription"):
		status = VerificationStatusFail
		message = "duplicate subscription anomaly is not reflected in behavior intervention output"
		failedRules = append(failedRules, "duplicate_subscription_coverage")
	case currentState.BehaviorState.LateNightSpendingFrequency >= 0.30 && !containsAny(reportText, "深夜", "late-night", "spend"):
		status = VerificationStatusFail
		message = "late-night spending anomaly is not reflected in behavior intervention output"
		failedRules = append(failedRules, "late_night_signal_coverage")
	case extractString(payload, "selected_recipe_id") == "hard_cap.v1" && !extractBool(payload, "approval_required"):
		status = VerificationStatusFail
		message = "hard_cap.v1 must require approval"
		failedRules = append(failedRules, "hard_cap_requires_approval")
	}
	return VerificationResult{
		Status:                  status,
		Validator:               "behavior_intervention_business_validator",
		Message:                 message,
		FailedRules:             failedRules,
		RecommendedReplanAction: "rebuild behavior intervention output with explicit anomaly justification and approval semantics",
		Severity:                severityForStatus(status),
		EvidenceCoverage:        fullCoverage(spec.ID),
		CheckedAt:               time.Now().UTC(),
	}, nil
}

func toMap(output any) (map[string]any, error) {
	payload, err := json.Marshal(output)
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func missingKeys(payload map[string]any, keys []string) []string {
	missing := make([]string, 0)
	for _, key := range keys {
		if _, ok := payload[key]; !ok {
			missing = append(missing, key)
		}
	}
	return missing
}

func extractString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return str
}

func extractBool(payload map[string]any, key string) bool {
	value, ok := payload[key]
	if !ok {
		return false
	}
	boolean, ok := value.(bool)
	return ok && boolean
}

func extractListLen(payload map[string]any, key string) int {
	value, ok := payload[key]
	if !ok {
		return 0
	}
	items, ok := value.([]any)
	if !ok {
		return 0
	}
	return len(items)
}

func flattenOutput(payload map[string]any) string {
	data, _ := json.Marshal(payload)
	return strings.ToLower(string(data))
}

func containsAny(text string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(text, strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}

func severityForStatus(status VerificationStatus) string {
	switch status {
	case VerificationStatusFail:
		return "high"
	case VerificationStatusNeedsReplan:
		return "medium"
	case VerificationStatusBlocked:
		return "high"
	default:
		return "low"
	}
}
