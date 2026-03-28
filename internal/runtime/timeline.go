package runtime

import (
	"sync"
	"time"
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
