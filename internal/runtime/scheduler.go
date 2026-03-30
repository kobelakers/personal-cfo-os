package runtime

import (
	"context"
	"fmt"
	"time"
)

// SchedulerService belongs to the runtime layer. It promotes deferred,
// approval-resume, dependency/capability reevaluation, and retry timing into a
// durable subsystem instead of leaving them as incidental worker-pass logic.
type SchedulerService struct {
	TaskGraphs   TaskGraphStore
	Executions   TaskExecutionStore
	Approvals    ApprovalStateStore
	WorkQueue    WorkQueueStore
	Wakeups      SchedulerStore
	Capabilities TaskCapabilityResolver
	Replay       ReplayStore
	Clock        Clock
	DuePolicy    DueWindowPolicy
	RetryPolicy  RetryBackoffPolicy
}

type SchedulerTickResult struct {
	ScannedGraphs       int               `json:"scanned_graphs"`
	SavedWakeups        []SchedulerWakeup `json:"saved_wakeups,omitempty"`
	DispatchedWakeups   []SchedulerWakeup `json:"dispatched_wakeups,omitempty"`
	EnqueuedWorkItemIDs []string          `json:"enqueued_work_item_ids,omitempty"`
	EnqueuedWorkKinds   []WorkItemKind    `json:"enqueued_work_kinds,omitempty"`
	TickedAt            time.Time         `json:"ticked_at"`
}

func (s SchedulerService) Tick(ctx context.Context) (SchedulerTickResult, error) {
	now := s.now()
	result := SchedulerTickResult{TickedAt: now}
	if s.TaskGraphs != nil {
		graphs, err := s.TaskGraphs.List()
		if err != nil {
			return SchedulerTickResult{}, err
		}
		result.ScannedGraphs = len(graphs)
		for _, snapshot := range graphs {
			saved, err := s.scheduleGraph(snapshot, now)
			if err != nil {
				return SchedulerTickResult{}, err
			}
			result.SavedWakeups = append(result.SavedWakeups, saved...)
		}
	}
	due, err := s.Wakeups.ListDueWakeups(now)
	if err != nil {
		return SchedulerTickResult{}, err
	}
	for _, wakeup := range due {
		item, ok := s.toWorkItem(wakeup)
		if !ok {
			continue
		}
		if err := s.WorkQueue.Enqueue(item); err != nil {
			return SchedulerTickResult{}, err
		}
		if err := s.Wakeups.MarkWakeupDispatched(wakeup.ID, now); err != nil {
			return SchedulerTickResult{}, err
		}
		result.DispatchedWakeups = append(result.DispatchedWakeups, wakeup)
		result.EnqueuedWorkItemIDs = append(result.EnqueuedWorkItemIDs, item.ID)
		result.EnqueuedWorkKinds = append(result.EnqueuedWorkKinds, item.Kind)
		if err := s.appendReplay(ReplayEventRecord{
			EventID:           makeID("replay", "scheduler-dispatch", wakeup.ID, now),
			RootCorrelationID: firstNonEmpty(wakeup.GraphID, wakeup.ExecutionID, wakeup.ApprovalID),
			WorkflowID:        "",
			GraphID:           wakeup.GraphID,
			TaskID:            wakeup.TaskID,
			ApprovalID:        wakeup.ApprovalID,
			ExecutionID:       wakeup.ExecutionID,
			ActionType:        "scheduler_wakeup_dispatch",
			Summary:           fmt.Sprintf("scheduler dispatched %s wakeup", wakeup.Kind),
			OccurredAt:        now,
			DetailsJSON:       fmt.Sprintf(`{"wakeup_id":%q,"work_item_id":%q,"work_item_kind":%q}`, wakeup.ID, item.ID, item.Kind),
		}); err != nil {
			return SchedulerTickResult{}, err
		}
	}
	_ = ctx
	return result, nil
}

func (s SchedulerService) scheduleGraph(snapshot TaskGraphSnapshot, now time.Time) ([]SchedulerWakeup, error) {
	if s.Wakeups == nil {
		return nil, nil
	}
	saved := make([]SchedulerWakeup, 0)
	updated, activation, err := ReevaluateFollowUpTaskGraph(snapshot, s.Capabilities, now)
	if err != nil {
		return nil, err
	}
	for _, task := range snapshot.RegisteredTasks {
		switch {
		case task.Status == TaskQueueStatusDeferred && task.Metadata.DueWindow.NotBefore != nil:
			wakeup := SchedulerWakeup{
				ID:          makeID("wakeup", SchedulerWakeupDueWindow, snapshot.Graph.GraphID, task.Task.ID, task.Metadata.DueWindow.NotBefore.UTC()),
				GraphID:     snapshot.Graph.GraphID,
				TaskID:      task.Task.ID,
				Kind:        SchedulerWakeupDueWindow,
				AvailableAt: task.Metadata.DueWindow.NotBefore.UTC(),
				Reason:      "due window reached",
			}
			if err := s.Wakeups.SaveWakeup(wakeup); err != nil {
				return nil, err
			}
			saved = append(saved, wakeup)
		case task.Status == TaskQueueStatusWaitingApproval:
			approval, ok, err := s.Approvals.LoadByTask(snapshot.Graph.GraphID, task.Task.ID)
			if err != nil {
				return nil, err
			}
			if ok && approval.Status == ApprovalStatusApproved {
				wakeup := SchedulerWakeup{
					ID:          makeID("wakeup", SchedulerWakeupApproval, approval.ApprovalID),
					GraphID:     snapshot.Graph.GraphID,
					TaskID:      task.Task.ID,
					ApprovalID:  approval.ApprovalID,
					Kind:        SchedulerWakeupApproval,
					AvailableAt: now,
					Reason:      "approval resolved to approved",
				}
				if err := s.Wakeups.SaveWakeup(wakeup); err != nil {
					return nil, err
				}
				saved = append(saved, wakeup)
			}
		}
	}
	for _, task := range updated.RegisteredTasks {
		if task.Status == TaskQueueStatusReady {
			item := WorkItem{
				ID:            makeID("work", WorkItemKindExecuteReadyTask, snapshot.Graph.GraphID),
				Kind:          WorkItemKindExecuteReadyTask,
				Status:        WorkItemStatusQueued,
				DedupeKey:     fmt.Sprintf("execute:%s", snapshot.Graph.GraphID),
				GraphID:       snapshot.Graph.GraphID,
				WorkflowID:    snapshot.Graph.ParentWorkflowID,
				AvailableAt:   now,
				LastUpdatedAt: now,
				Reason:        "ready task detected during scheduler scan",
			}
			if err := s.WorkQueue.Enqueue(item); err != nil {
				return nil, err
			}
		}
	}
	if statusChangesRequireReevaluation(snapshot, updated) || len(activation.ReadyTaskIDs) > 0 {
		wakeup := SchedulerWakeup{
			ID:          makeID("wakeup", SchedulerWakeupDependency, snapshot.Graph.GraphID, now.Truncate(time.Second)),
			GraphID:     snapshot.Graph.GraphID,
			Kind:        SchedulerWakeupDependency,
			AvailableAt: now,
			Reason:      "graph status changed under reevaluation",
		}
		if err := s.Wakeups.SaveWakeup(wakeup); err != nil {
			return nil, err
		}
		saved = append(saved, wakeup)
	}
	if s.Executions != nil {
		records, err := s.Executions.ListByGraph(snapshot.Graph.GraphID)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			if record.Status != TaskQueueStatusFailed || record.LastRecoveryStrategy != RecoveryStrategyRetry {
				continue
			}
			next := nextRetryTime(record, s.retryPolicy(), now)
			wakeup := SchedulerWakeup{
				ID:          makeID("wakeup", SchedulerWakeupRetry, record.ExecutionID, next.UTC()),
				GraphID:     snapshot.Graph.GraphID,
				TaskID:      record.TaskID,
				ExecutionID: record.ExecutionID,
				Kind:        SchedulerWakeupRetry,
				AvailableAt: next,
				Reason:      "retry backoff reached",
			}
			if err := s.Wakeups.SaveWakeup(wakeup); err != nil {
				return nil, err
			}
			saved = append(saved, wakeup)
		}
	}
	return saved, nil
}

func (s SchedulerService) toWorkItem(wakeup SchedulerWakeup) (WorkItem, bool) {
	switch wakeup.Kind {
	case SchedulerWakeupDueWindow, SchedulerWakeupDependency, SchedulerWakeupCapability, SchedulerWakeupOperator:
		return WorkItem{
			ID:            makeID("work", WorkItemKindReevaluateTaskGraph, wakeup.GraphID, wakeup.Kind, wakeup.AvailableAt.UTC()),
			Kind:          WorkItemKindReevaluateTaskGraph,
			Status:        WorkItemStatusQueued,
			DedupeKey:     fmt.Sprintf("reevaluate:%s", wakeup.GraphID),
			GraphID:       wakeup.GraphID,
			AvailableAt:   wakeup.AvailableAt,
			LastUpdatedAt: wakeup.AvailableAt,
			Reason:        wakeup.Reason,
			WakeupKind:    wakeup.Kind,
		}, true
	case SchedulerWakeupApproval:
		return WorkItem{
			ID:            makeID("work", WorkItemKindResumeApprovedCheckpoint, wakeup.ApprovalID),
			Kind:          WorkItemKindResumeApprovedCheckpoint,
			Status:        WorkItemStatusQueued,
			DedupeKey:     fmt.Sprintf("resume:%s:%s", wakeup.GraphID, wakeup.TaskID),
			GraphID:       wakeup.GraphID,
			TaskID:        wakeup.TaskID,
			ApprovalID:    wakeup.ApprovalID,
			AvailableAt:   wakeup.AvailableAt,
			LastUpdatedAt: wakeup.AvailableAt,
			Reason:        wakeup.Reason,
			WakeupKind:    wakeup.Kind,
		}, true
	case SchedulerWakeupRetry:
		return WorkItem{
			ID:            makeID("work", WorkItemKindRetryFailedExecution, wakeup.ExecutionID, wakeup.AvailableAt.UTC()),
			Kind:          WorkItemKindRetryFailedExecution,
			Status:        WorkItemStatusQueued,
			DedupeKey:     fmt.Sprintf("retry:%s", wakeup.ExecutionID),
			GraphID:       wakeup.GraphID,
			TaskID:        wakeup.TaskID,
			ExecutionID:   wakeup.ExecutionID,
			AvailableAt:   wakeup.AvailableAt,
			LastUpdatedAt: wakeup.AvailableAt,
			Reason:        wakeup.Reason,
			WakeupKind:    wakeup.Kind,
		}, true
	default:
		return WorkItem{}, false
	}
}

func (s SchedulerService) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now()
	}
	return time.Now().UTC()
}

func (s SchedulerService) retryPolicy() RetryBackoffPolicy {
	if s.RetryPolicy.BaseDelay <= 0 {
		s.RetryPolicy.BaseDelay = 5 * time.Second
	}
	if s.RetryPolicy.MaxDelay <= 0 {
		s.RetryPolicy.MaxDelay = 5 * time.Minute
	}
	return s.RetryPolicy
}

func nextRetryTime(record TaskExecutionRecord, policy RetryBackoffPolicy, now time.Time) time.Time {
	delay := policy.BaseDelay
	if delay <= 0 {
		delay = 5 * time.Second
	}
	for i := 1; i < max(record.RetryCount, 1); i++ {
		delay *= 2
		if policy.MaxDelay > 0 && delay > policy.MaxDelay {
			delay = policy.MaxDelay
			break
		}
	}
	base := record.FailedAt
	if base.IsZero() {
		base = now
	}
	return base.Add(delay)
}

func statusChangesRequireReevaluation(before TaskGraphSnapshot, after TaskGraphSnapshot) bool {
	if len(before.RegisteredTasks) != len(after.RegisteredTasks) {
		return true
	}
	byID := make(map[string]FollowUpTaskRecord, len(before.RegisteredTasks))
	for _, item := range before.RegisteredTasks {
		byID[item.Task.ID] = item
	}
	for _, item := range after.RegisteredTasks {
		previous, ok := byID[item.Task.ID]
		if !ok {
			return true
		}
		if previous.Status != item.Status {
			return true
		}
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s SchedulerService) appendReplay(event ReplayEventRecord) error {
	if s.Replay == nil {
		return nil
	}
	return s.Replay.Append(event)
}
