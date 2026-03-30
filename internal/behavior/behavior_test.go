package behavior

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestComputeMetricsAndRecommendations(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)
	evidence := BehaviorEvidence{
		CurrentState: state.FinancialWorldState{
			CashflowState: state.CashflowState{
				MonthlyOutflowCents:         100000,
				MonthlyVariableExpenseCents: 60000,
				MonthlyNetIncomeCents:       -5000,
			},
			BehaviorState: state.BehaviorState{
				DuplicateSubscriptionCount: 2,
				LateNightSpendingFrequency: 0.42,
				RecurringSubscriptions:     []string{"netflix", "spotify"},
			},
		},
		Evidence: []observation.EvidenceRecord{
			{ID: "tx", Type: observation.EvidenceTypeTransactionBatch},
			{ID: "sub", Type: observation.EvidenceTypeRecurringSubscription},
			{ID: "late", Type: observation.EvidenceTypeLateNightSpendingSignal},
		},
	}
	output, err := Analyzer{}.Analyze(evidence, skills.SkillSelection{
		Family:                skills.SkillFamilyDiscretionaryGuardrail,
		Version:               "v1",
		RecipeID:              "hard_cap.v1",
		InterventionIntensity: skills.InterventionIntensityHigh,
	}, []string{"memory-1"}, now)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if output.Metrics.Metrics.DiscretionaryPressureScore <= 0.55 {
		t.Fatalf("expected pressure score > 0.55, got %.2f", output.Metrics.Metrics.DiscretionaryPressureScore)
	}
	if len(output.Recommendations) != 1 || output.Recommendations[0].Type != BehaviorRecommendationGuardrail {
		t.Fatalf("expected guardrail recommendation, got %#v", output.Recommendations)
	}
	if output.Recommendations[0].RiskLevel != taskspec.RiskLevelHigh || !output.Recommendations[0].ApprovalRequired {
		t.Fatalf("expected hard cap recommendation to require approval, got %+v", output.Recommendations[0])
	}
	if err := (BehaviorValidator{}).Validate(output); err != nil {
		t.Fatalf("validate: %v", err)
	}
}
