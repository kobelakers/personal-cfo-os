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
