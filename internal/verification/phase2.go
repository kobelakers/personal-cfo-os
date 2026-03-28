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
	if len(missing) > 0 {
		status = VerificationStatusFail
		message = "missing report keys: " + strings.Join(missing, ", ")
	}
	if currentState.CashflowState.MonthlyInflowCents == 0 || currentState.CashflowState.MonthlyOutflowCents == 0 {
		status = VerificationStatusNeedsReplan
		message = "cashflow metrics are incomplete"
	}
	return VerificationResult{
		Status:           status,
		Validator:        "monthly_review_deterministic_validator",
		Message:          message,
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        time.Now().UTC(),
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
	switch {
	case currentState.LiabilityState.DebtBurdenRatio >= 0.2 && !containsAny(reportText, "债务", "debt"):
		status = VerificationStatusFail
		message = "high-risk debt signal was omitted from report"
	case currentState.TaxState.ChildcareTaxSignal && !containsAny(reportText, "税", "tax", "childcare", "育儿"):
		status = VerificationStatusFail
		message = "obvious tax signal was omitted from report"
	case currentState.RiskState.OverallRisk == "high" && !extractBool(payload, "approval_required"):
		status = VerificationStatusNeedsReplan
		message = "high overall risk should have triggered approval_required"
	case currentState.CashflowState.SavingsRate < 0.1 && !containsAny(reportText, "现金流", "cashflow", "储蓄率", "savings"):
		status = VerificationStatusNeedsReplan
		message = "cashflow constraint explanation is missing"
	}
	return VerificationResult{
		Status:           status,
		Validator:        "monthly_review_business_validator",
		Message:          message,
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        time.Now().UTC(),
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
	switch {
	case extractString(payload, "conclusion") == "":
		status = VerificationStatusFail
		message = "debt decision conclusion is missing"
	case currentState.LiabilityState.DebtBurdenRatio >= 0.2 && !containsAny(reportText, "债务", "debt", "还"):
		status = VerificationStatusFail
		message = "high debt burden is not reflected in decision output"
	case currentState.RiskState.OverallRisk == "high" && !extractBool(payload, "approval_required"):
		status = VerificationStatusNeedsReplan
		message = "high-risk debt decision should require approval"
	}
	return VerificationResult{
		Status:           status,
		Validator:        "debt_decision_business_validator",
		Message:          message,
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        time.Now().UTC(),
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
		Status:           status,
		Validator:        "success_criteria_checker",
		Message:          message,
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        time.Now().UTC(),
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
