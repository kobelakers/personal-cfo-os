package runtime

import (
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/protocol"
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

func TestLocalWorkflowRuntimeCheckpointAndResume(t *testing.T) {
	now := time.Date(2026, 3, 28, 15, 0, 0, 0, time.UTC)
	local := LocalWorkflowRuntime{
		Controller:      DefaultWorkflowController{},
		CheckpointStore: NewInMemoryCheckpointStore(),
		Journal:         &CheckpointJournal{},
		Timeline:        &WorkflowTimeline{WorkflowID: "workflow-1", TraceID: "trace-1"},
		Now:             func() time.Time { return now },
	}
	execCtx := ExecutionContext{
		WorkflowID:    "workflow-1",
		TaskID:        "task-1",
		CorrelationID: "corr-1",
		Attempt:       1,
	}
	checkpoint, token, err := local.Checkpoint(execCtx, WorkflowStatePlanning, WorkflowStateActing, 2, "before act")
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	next, err := local.Resume(execCtx, checkpoint.ID, token)
	if err != nil {
		t.Fatalf("resume workflow: %v", err)
	}
	if next != WorkflowStateActing {
		t.Fatalf("expected acting state after resume, got %q", next)
	}
	if len(local.Journal.Checkpoints) != 1 || len(local.Timeline.Entries) < 2 {
		t.Fatalf("expected checkpoint journal and timeline to record events")
	}
}

func TestResolveWorkflowRuntimeProvidesDefaultRuntime(t *testing.T) {
	resolved := ResolveWorkflowRuntime(nil, "workflow-default", func() time.Time {
		return time.Date(2026, 3, 28, 16, 0, 0, 0, time.UTC)
	})
	local, ok := resolved.(*LocalWorkflowRuntime)
	if !ok {
		t.Fatalf("expected resolved runtime to be local runtime")
	}
	if local.CheckpointStore == nil || local.Timeline == nil || local.Journal == nil {
		t.Fatalf("expected default runtime dependencies to be initialized")
	}
}

func TestHandleAgentExecutionFailureMapsTypedCategory(t *testing.T) {
	now := time.Date(2026, 3, 28, 17, 0, 0, 0, time.UTC)
	local := NewLocalWorkflowRuntime("workflow-1", LocalRuntimeOptions{
		Now: func() time.Time { return now },
	})
	execCtx := ExecutionContext{
		WorkflowID:    "workflow-1",
		TaskID:        "task-1",
		CorrelationID: "corr-1",
		Attempt:       1,
	}
	next, strategy, err := HandleAgentExecutionFailure(local, execCtx, WorkflowStateVerifying, categorizedAgentError{
		failure: protocol.AgentFailure{Category: protocol.AgentFailurePolicy, Message: "governance requires approval"},
	}, "governance agent failed")
	if err != nil {
		t.Fatalf("map categorized agent error: %v", err)
	}
	if next != WorkflowStateWaitingApproval || strategy != RecoveryStrategyWaitForApproval {
		t.Fatalf("unexpected runtime mapping: %q %q", next, strategy)
	}
}

type categorizedAgentError struct {
	failure protocol.AgentFailure
}

func (e categorizedAgentError) Error() string {
	return e.failure.Message
}

func (e categorizedAgentError) AgentFailure() protocol.AgentFailure {
	return e.failure
}
