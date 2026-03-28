package runtime

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
)

type WorkflowTimelineEntry struct {
	State      WorkflowExecutionState `json:"state"`
	Event      string                 `json:"event"`
	Summary    string                 `json:"summary"`
	OccurredAt time.Time              `json:"occurred_at"`
}

type WorkflowTimeline struct {
	WorkflowID string                  `json:"workflow_id"`
	TraceID    string                  `json:"trace_id"`
	Entries    []WorkflowTimelineEntry `json:"entries"`
}

func (t *WorkflowTimeline) Append(state WorkflowExecutionState, event string, summary string, occurredAt time.Time) {
	t.Entries = append(t.Entries, WorkflowTimelineEntry{
		State:      state,
		Event:      event,
		Summary:    summary,
		OccurredAt: occurredAt,
	})
}

type CheckpointJournal struct {
	mu          sync.Mutex
	Checkpoints []CheckpointRecord `json:"checkpoints"`
}

func (j *CheckpointJournal) Append(checkpoint CheckpointRecord) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Checkpoints = append(j.Checkpoints, checkpoint)
}

type InMemoryCheckpointStore struct {
	mu          sync.RWMutex
	checkpoints map[string]map[string]CheckpointRecord
}

func NewInMemoryCheckpointStore() *InMemoryCheckpointStore {
	return &InMemoryCheckpointStore{
		checkpoints: make(map[string]map[string]CheckpointRecord),
	}
}

func (s *InMemoryCheckpointStore) Save(checkpoint CheckpointRecord) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.checkpoints[checkpoint.WorkflowID]; !ok {
		s.checkpoints[checkpoint.WorkflowID] = make(map[string]CheckpointRecord)
	}
	s.checkpoints[checkpoint.WorkflowID][checkpoint.ID] = checkpoint
	return nil
}

func (s *InMemoryCheckpointStore) Load(workflowID string, checkpointID string) (CheckpointRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byWorkflow, ok := s.checkpoints[workflowID]
	if !ok {
		return CheckpointRecord{}, fmt.Errorf("workflow %q not found", workflowID)
	}
	checkpoint, ok := byWorkflow[checkpointID]
	if !ok {
		return CheckpointRecord{}, fmt.Errorf("checkpoint %q not found", checkpointID)
	}
	return checkpoint, nil
}

type RetryPlanner struct{}

func (RetryPlanner) StrategyFor(category FailureCategory) (RecoveryStrategy, error) {
	switch category {
	case FailureCategoryTransient, FailureCategoryTimeout:
		return RecoveryStrategyRetry, nil
	case FailureCategoryValidation:
		return RecoveryStrategyReplan, nil
	case FailureCategoryPolicy:
		return RecoveryStrategyWaitForApproval, nil
	case FailureCategoryUnrecoverable:
		return RecoveryStrategyAbort, nil
	default:
		return "", fmt.Errorf("unsupported failure category %q", category)
	}
}

type LocalWorkflowRuntime struct {
	Controller      WorkflowController
	CheckpointStore *InMemoryCheckpointStore
	Journal         *CheckpointJournal
	Timeline        *WorkflowTimeline
	EventLog        *observability.EventLog
	Now             func() time.Time
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
