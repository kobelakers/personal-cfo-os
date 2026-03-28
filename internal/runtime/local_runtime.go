package runtime

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
)

type WorkflowRuntime interface {
	Checkpoint(ctx ExecutionContext, current WorkflowExecutionState, resumeState WorkflowExecutionState, stateVersion uint64, summary string) (CheckpointRecord, ResumeToken, error)
	Resume(ctx ExecutionContext, checkpointID string, token ResumeToken) (WorkflowExecutionState, error)
	HandleFailure(ctx ExecutionContext, current WorkflowExecutionState, category FailureCategory, summary string) (WorkflowExecutionState, RecoveryStrategy, error)
	PauseForApproval(ctx ExecutionContext, current WorkflowExecutionState, pending HumanApprovalPending) (WorkflowExecutionState, error)
}

type LocalRuntimeOptions struct {
	Controller      WorkflowController
	CheckpointStore CheckpointStore
	Journal         *CheckpointJournal
	Timeline        *WorkflowTimeline
	EventLog        *observability.EventLog
	Now             func() time.Time
}

type LocalWorkflowRuntime struct {
	Controller      WorkflowController
	CheckpointStore CheckpointStore
	Journal         *CheckpointJournal
	Timeline        *WorkflowTimeline
	EventLog        *observability.EventLog
	Now             func() time.Time
}

func NewLocalWorkflowRuntime(workflowID string, options LocalRuntimeOptions) *LocalWorkflowRuntime {
	controller := options.Controller
	if controller == nil {
		controller = DefaultWorkflowController{}
	}
	store := options.CheckpointStore
	if store == nil {
		store = NewInMemoryCheckpointStore()
	}
	timeline := options.Timeline
	if timeline == nil {
		timeline = &WorkflowTimeline{WorkflowID: workflowID, TraceID: workflowID}
	}
	journal := options.Journal
	if journal == nil {
		journal = &CheckpointJournal{}
	}
	return &LocalWorkflowRuntime{
		Controller:      controller,
		CheckpointStore: store,
		Journal:         journal,
		Timeline:        timeline,
		EventLog:        options.EventLog,
		Now:             options.Now,
	}
}

func ResolveWorkflowRuntime(current WorkflowRuntime, workflowID string, now func() time.Time) WorkflowRuntime {
	if current != nil {
		return current
	}
	return NewLocalWorkflowRuntime(workflowID, LocalRuntimeOptions{
		Now:      now,
		EventLog: &observability.EventLog{},
	})
}

func (r LocalWorkflowRuntime) Checkpoint(
	ctx ExecutionContext,
	current WorkflowExecutionState,
	resumeState WorkflowExecutionState,
	stateVersion uint64,
	summary string,
) (CheckpointRecord, ResumeToken, error) {
	now := r.now()
	checkpoint := CheckpointRecord{
		ID:           makeID(ctx.WorkflowID, summary, now),
		WorkflowID:   ctx.WorkflowID,
		State:        current,
		ResumeState:  resumeState,
		StateVersion: stateVersion,
		Summary:      summary,
		CapturedAt:   now,
	}
	if r.CheckpointStore == nil {
		r.CheckpointStore = NewInMemoryCheckpointStore()
	}
	if err := r.CheckpointStore.Save(checkpoint); err != nil {
		return CheckpointRecord{}, ResumeToken{}, err
	}
	if r.Journal != nil {
		r.Journal.Append(checkpoint)
	}
	if r.Timeline != nil {
		r.Timeline.Append(current, "checkpoint_created", summary, now)
	}
	r.log(ctx, "checkpoint", summary, map[string]string{
		"checkpoint_id": checkpoint.ID,
		"state":         string(current),
	})

	token := ResumeToken{
		Token:        makeID(ctx.WorkflowID, checkpoint.ID, now),
		WorkflowID:   ctx.WorkflowID,
		CheckpointID: checkpoint.ID,
		IssuedAt:     now,
		ExpiresAt:    now.Add(24 * time.Hour),
	}
	return checkpoint, token, nil
}

func (r LocalWorkflowRuntime) Resume(ctx ExecutionContext, checkpointID string, token ResumeToken) (WorkflowExecutionState, error) {
	if r.CheckpointStore == nil {
		return "", fmt.Errorf("checkpoint store is required")
	}
	checkpoint, err := r.CheckpointStore.Load(ctx.WorkflowID, checkpointID)
	if err != nil {
		return "", err
	}
	controller := r.Controller
	if controller == nil {
		controller = DefaultWorkflowController{}
	}
	next, err := controller.Resume(checkpoint, token, r.now())
	if err != nil {
		return "", err
	}
	if r.Timeline != nil {
		r.Timeline.Append(next, "resumed", checkpoint.Summary, r.now())
	}
	r.log(ctx, "resume", checkpoint.Summary, map[string]string{
		"checkpoint_id": checkpoint.ID,
		"next_state":    string(next),
	})
	return next, nil
}

func (r LocalWorkflowRuntime) HandleFailure(ctx ExecutionContext, current WorkflowExecutionState, category FailureCategory, summary string) (WorkflowExecutionState, RecoveryStrategy, error) {
	controller := r.Controller
	if controller == nil {
		controller = DefaultWorkflowController{}
	}
	next, strategy, err := controller.HandleFailure(current, category)
	if err != nil {
		return "", "", err
	}
	if r.Timeline != nil {
		r.Timeline.Append(next, "failure_handled", summary, r.now())
	}
	r.log(ctx, "failure", summary, map[string]string{
		"category": string(category),
		"strategy": string(strategy),
		"next":     string(next),
	})
	return next, strategy, nil
}

func (r LocalWorkflowRuntime) PauseForApproval(ctx ExecutionContext, current WorkflowExecutionState, pending HumanApprovalPending) (WorkflowExecutionState, error) {
	controller := r.Controller
	if controller == nil {
		controller = DefaultWorkflowController{}
	}
	next, err := controller.PauseForApproval(current, pending)
	if err != nil {
		return "", err
	}
	if r.Timeline != nil {
		r.Timeline.Append(next, "approval_required", pending.RequestedAction, r.now())
	}
	r.log(ctx, "approval", pending.RequestedAction, map[string]string{
		"approval_id": pending.ApprovalID,
		"next_state":  string(next),
	})
	return next, nil
}

func (r LocalWorkflowRuntime) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func (r LocalWorkflowRuntime) log(ctx ExecutionContext, category string, message string, details map[string]string) {
	if r.EventLog == nil {
		return
	}
	r.EventLog.Append(observability.LogEntry{
		TraceID:       ctx.CorrelationID,
		CorrelationID: ctx.CorrelationID,
		Category:      category,
		Message:       message,
		Details:       details,
		OccurredAt:    r.now(),
	})
}

func makeID(parts ...any) string {
	hash := sha1.New()
	for _, part := range parts {
		fmt.Fprint(hash, part)
	}
	return hex.EncodeToString(hash.Sum(nil))[:16]
}
