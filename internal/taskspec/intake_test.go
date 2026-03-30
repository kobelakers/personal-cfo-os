package taskspec

import (
	"testing"
	"time"
)

func TestDeterministicIntakeServiceAcceptsMonthlyReview(t *testing.T) {
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	service := DeterministicIntakeService{
		Now: func() time.Time { return now },
	}

	result := service.Parse("请帮我做一份月度财务复盘")
	if !result.Accepted {
		t.Fatalf("expected intake to accept monthly review, got %+v", result)
	}
	if result.TaskSpec == nil || result.TaskSpec.UserIntentType != UserIntentMonthlyReview {
		t.Fatalf("expected monthly review task spec, got %+v", result.TaskSpec)
	}
	if result.Confidence != TaskIntakeConfidenceHigh {
		t.Fatalf("expected high confidence, got %q", result.Confidence)
	}
}

func TestDeterministicIntakeServiceRejectsUnsupportedIntent(t *testing.T) {
	service := DeterministicIntakeService{}

	result := service.Parse("帮我写一首歌")
	if result.Accepted {
		t.Fatalf("expected unsupported intent to be rejected")
	}
	if result.RejectionReason != TaskRejectionOutOfScope {
		t.Fatalf("expected out-of-scope rejection, got %q", result.RejectionReason)
	}
	if result.FailureReason != TaskIntakeFailureUnsupportedInput {
		t.Fatalf("expected unsupported input failure, got %q", result.FailureReason)
	}
}

func TestDeterministicIntakeServiceAcceptsBehaviorIntervention(t *testing.T) {
	now := time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC)
	service := DeterministicIntakeService{
		Now: func() time.Time { return now },
	}
	result := service.Parse("请帮我做一次消费护栏和支出行为复盘")
	if !result.Accepted {
		t.Fatalf("expected intake to accept behavior intervention, got %+v", result)
	}
	if result.TaskSpec == nil || result.TaskSpec.UserIntentType != UserIntentBehaviorIntervention {
		t.Fatalf("expected behavior intervention task spec, got %+v", result.TaskSpec)
	}
	if got := result.TaskSpec.Scope.Areas; len(got) != 2 || got[0] != "behavior" || got[1] != "cashflow" {
		t.Fatalf("unexpected scope areas: %+v", got)
	}
	if result.TaskSpec.ApprovalRequirement != ApprovalRequirementRecommended {
		t.Fatalf("expected recommended approval requirement, got %q", result.TaskSpec.ApprovalRequirement)
	}
	required := result.TaskSpec.RequiredEvidence
	if len(required) == 0 || required[0].Type != "transaction_batch" || !required[0].Mandatory {
		t.Fatalf("expected behavior intervention intake to require transaction_batch evidence, got %+v", required)
	}
}

func TestDeterministicIntakeServiceAcceptsBehaviorInterventionEnglishKeyword(t *testing.T) {
	now := time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC)
	service := DeterministicIntakeService{
		Now: func() time.Time { return now },
	}
	result := service.Parse("please run a spending behavior review")
	if !result.Accepted {
		t.Fatalf("expected english behavior keyword to be accepted, got %+v", result)
	}
	if result.TaskSpec == nil || result.TaskSpec.UserIntentType != UserIntentBehaviorIntervention {
		t.Fatalf("expected behavior intervention task spec, got %+v", result.TaskSpec)
	}
}
