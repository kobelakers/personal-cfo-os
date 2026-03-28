package taskspec

import (
	"testing"
	"time"
)

func TestTaskSpecValidateRejectsMissingSuccessCriteria(t *testing.T) {
	now := time.Now().UTC()
	spec := TaskSpec{
		ID:                  "task-monthly-review",
		Goal:                "Run monthly review",
		Scope:               TaskScope{Areas: []string{"cashflow"}},
		Constraints:         ConstraintSet{Hard: []string{"do not auto-trade"}},
		RiskLevel:           RiskLevelMedium,
		ApprovalRequirement: ApprovalRequirementRecommended,
		Deadline:            &now,
		UserIntentType:      UserIntentMonthlyReview,
		CreatedAt:           now,
	}

	if err := spec.Validate(); err == nil {
		t.Fatalf("expected validation error for missing success criteria")
	}
}

func TestTaskSpecNormalizeAppliesDefaults(t *testing.T) {
	spec := TaskSpec{
		ID:        "task-debt-vs-invest",
		Goal:      "  Compare debt vs invest  ",
		Scope:     TaskScope{Areas: []string{" debt ", "", "debt", " portfolio "}},
		RiskLevel: RiskLevel("not-real"),
		SuccessCriteria: []SuccessCriteria{
			{ID: " coverage ", Description: "  evidence complete "},
		},
		RequiredEvidence: []RequiredEvidenceRef{
			{Type: " ledger ", Reason: " cashflow coverage "},
		},
		ApprovalRequirement: ApprovalRequirement("unknown"),
		UserIntentType:      UserIntentType("missing"),
	}

	normalized := spec.Normalize()

	if normalized.Goal != "Compare debt vs invest" {
		t.Fatalf("unexpected normalized goal: %q", normalized.Goal)
	}
	if len(normalized.Scope.Areas) != 2 {
		t.Fatalf("expected deduplicated scope areas, got %v", normalized.Scope.Areas)
	}
	if normalized.RiskLevel != RiskLevelUnknown {
		t.Fatalf("expected unknown risk level, got %q", normalized.RiskLevel)
	}
	if normalized.ApprovalRequirement != ApprovalRequirementNone {
		t.Fatalf("expected default approval requirement, got %q", normalized.ApprovalRequirement)
	}
	if normalized.UserIntentType != UserIntentUnknown {
		t.Fatalf("expected default user intent type, got %q", normalized.UserIntentType)
	}
}
