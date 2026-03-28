package runtime

import "github.com/kobelakers/personal-cfo-os/internal/observability"

func (t WorkflowTimeline) Records() []observability.TimelineRecord {
	records := make([]observability.TimelineRecord, 0, len(t.Entries))
	for _, entry := range t.Entries {
		records = append(records, observability.TimelineRecord{
			State:      string(entry.State),
			Event:      entry.Event,
			Summary:    entry.Summary,
			OccurredAt: entry.OccurredAt,
		})
	}
	return records
}

func (j CheckpointJournal) Records() []observability.CheckpointRecord {
	records := make([]observability.CheckpointRecord, 0, len(j.Checkpoints))
	for _, checkpoint := range j.Checkpoints {
		records = append(records, observability.CheckpointRecord{
			ID:           checkpoint.ID,
			WorkflowID:   checkpoint.WorkflowID,
			State:        string(checkpoint.State),
			ResumeState:  string(checkpoint.ResumeState),
			StateVersion: checkpoint.StateVersion,
			Summary:      checkpoint.Summary,
			CapturedAt:   checkpoint.CapturedAt,
		})
	}
	return records
}
