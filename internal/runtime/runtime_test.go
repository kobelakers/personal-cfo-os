package runtime

import (
	"testing"
	"time"
)

func TestRuntimeFailureTransitions(t *testing.T) {
	controller := DefaultWorkflowController{}
	next, strategy, err := controller.HandleFailure(WorkflowStateActing, FailureCategoryTransient)
	if err != nil {
		t.Fatalf("transient failure should be handled: %v", err)
	}
	if next != WorkflowStateRetrying || strategy != RecoveryStrategyRetry {
		t.Fatalf("unexpected retry transition: %q %q", next, strategy)
	}

	next, strategy, err = controller.HandleFailure(WorkflowStateVerifying, FailureCategoryValidation)
	if err != nil {
		t.Fatalf("validation failure should be handled: %v", err)
	}
	if next != WorkflowStateReplanning || strategy != RecoveryStrategyReplan {
		t.Fatalf("unexpected replan transition: %q %q", next, strategy)
	}
}

func TestRuntimeResumeRejectsInvalidToken(t *testing.T) {
	controller := DefaultWorkflowController{}
	now := time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC)
	checkpoint := CheckpointRecord{
		ID:           "checkpoint-1",
		WorkflowID:   "workflow-1",
		State:        WorkflowStateWaitingApproval,
		ResumeState:  WorkflowStateActing,
		StateVersion: 2,
		Summary:      "waiting for approval",
		CapturedAt:   now.Add(-time.Minute),
	}
	token := ResumeToken{
		Token:        "token-1",
		WorkflowID:   "workflow-1",
		CheckpointID: "checkpoint-2",
		IssuedAt:     now.Add(-time.Minute),
		ExpiresAt:    now.Add(time.Minute),
	}
	if _, err := controller.Resume(checkpoint, token, now); err == nil {
		t.Fatalf("expected resume with mismatched checkpoint to fail")
	}
}
