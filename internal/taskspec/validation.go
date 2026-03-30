package taskspec

import (
	"errors"
	"fmt"
	"strings"
)

func (t TaskSpec) Normalize() TaskSpec {
	normalized := t
	normalized.Goal = strings.TrimSpace(normalized.Goal)
	normalized.Scope.Areas = trimNonEmpty(normalized.Scope.Areas)
	normalized.Scope.Notes = trimNonEmpty(normalized.Scope.Notes)
	normalized.Constraints.Hard = trimNonEmpty(normalized.Constraints.Hard)
	normalized.Constraints.Soft = trimNonEmpty(normalized.Constraints.Soft)

	if !validRiskLevel(normalized.RiskLevel) {
		normalized.RiskLevel = RiskLevelUnknown
	}
	if !validApprovalRequirement(normalized.ApprovalRequirement) {
		normalized.ApprovalRequirement = ApprovalRequirementNone
	}
	if !validUserIntentType(normalized.UserIntentType) {
		normalized.UserIntentType = UserIntentUnknown
	}

	for i := range normalized.SuccessCriteria {
		normalized.SuccessCriteria[i].ID = strings.TrimSpace(normalized.SuccessCriteria[i].ID)
		normalized.SuccessCriteria[i].Description = strings.TrimSpace(normalized.SuccessCriteria[i].Description)
	}

	for i := range normalized.RequiredEvidence {
		normalized.RequiredEvidence[i].Type = strings.TrimSpace(normalized.RequiredEvidence[i].Type)
		normalized.RequiredEvidence[i].Reason = strings.TrimSpace(normalized.RequiredEvidence[i].Reason)
	}

	return normalized
}

func (t TaskSpec) Validate() error {
	var errs []error

	if strings.TrimSpace(t.ID) == "" {
		errs = append(errs, errors.New("task spec id is required"))
	}
	if strings.TrimSpace(t.Goal) == "" {
		errs = append(errs, errors.New("task spec goal is required"))
	}
	if len(t.SuccessCriteria) == 0 {
		errs = append(errs, errors.New("at least one success criteria is required"))
	}
	if !validRiskLevel(t.RiskLevel) {
		errs = append(errs, fmt.Errorf("invalid risk level %q", t.RiskLevel))
	}
	if !validApprovalRequirement(t.ApprovalRequirement) {
		errs = append(errs, fmt.Errorf("invalid approval requirement %q", t.ApprovalRequirement))
	}
	if !validUserIntentType(t.UserIntentType) {
		errs = append(errs, fmt.Errorf("invalid user intent type %q", t.UserIntentType))
	}
	if t.Scope.Start != nil && t.Scope.End != nil && t.Scope.Start.After(*t.Scope.End) {
		errs = append(errs, errors.New("task scope start must be before end"))
	}
	for i, criteria := range t.SuccessCriteria {
		if strings.TrimSpace(criteria.ID) == "" {
			errs = append(errs, fmt.Errorf("success criteria %d must have an id", i))
		}
		if strings.TrimSpace(criteria.Description) == "" {
			errs = append(errs, fmt.Errorf("success criteria %d must have a description", i))
		}
	}
	for i, requirement := range t.RequiredEvidence {
		if strings.TrimSpace(requirement.Type) == "" {
			errs = append(errs, fmt.Errorf("required evidence %d must have a type", i))
		}
		if strings.TrimSpace(requirement.Reason) == "" {
			errs = append(errs, fmt.Errorf("required evidence %d must have a reason", i))
		}
	}

	return errors.Join(errs...)
}

func validRiskLevel(level RiskLevel) bool {
	switch level {
	case RiskLevelUnknown, RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelCritical:
		return true
	default:
		return false
	}
}

func validApprovalRequirement(requirement ApprovalRequirement) bool {
	switch requirement {
	case ApprovalRequirementNone, ApprovalRequirementNotify, ApprovalRequirementRecommended, ApprovalRequirementMandatory:
		return true
	default:
		return false
	}
}

func validUserIntentType(intent UserIntentType) bool {
	switch intent {
	case UserIntentUnknown, UserIntentMonthlyReview, UserIntentDebtVsInvest, UserIntentBehaviorIntervention, UserIntentLifeEventTrigger, UserIntentTaxOptimization, UserIntentPortfolioRebalance:
		return true
	default:
		return false
	}
}

func trimNonEmpty(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}
