package runtime

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
)

type ServiceOptions struct {
	CheckpointStore CheckpointStore
	TaskGraphs      TaskGraphStore
	Executions      TaskExecutionStore
	Approvals       ApprovalStateStore
	OperatorActions OperatorActionStore
	Replay          ReplayStore
	Artifacts       ArtifactMetadataStore
	WorkQueue       WorkQueueStore
	WorkAttempts    WorkAttemptStore
	Workers         WorkerRegistryStore
	Scheduler       SchedulerStore
	Capabilities    TaskCapabilityResolver
	Controller      WorkflowController
	EventLog        *observability.EventLog
	Now             func() time.Time
	Clock           Clock
	BackendProfile  string
}

type Service struct {
	runtime                *LocalWorkflowRuntime
	replayProjectionWriter ReplayProjectionWriter
	workQueue              WorkQueueStore
	workAttempts           WorkAttemptStore
	workers                WorkerRegistryStore
	scheduler              SchedulerStore
	clock                  Clock
	backendProfile         string
}

type ReplayProjectionWriter interface {
	RebuildTaskGraph(ctx context.Context, graphID string) (ReplayProjectionBuildRecord, error)
}

func NewService(options ServiceOptions) *Service {
	clock := options.Clock
	if clock == nil {
		if options.Now != nil {
			clock = funcClock{now: options.Now}
		} else {
			clock = SystemClock{}
		}
	}
	local := NewLocalWorkflowRuntime("runtime-service", LocalRuntimeOptions{
		Controller:      options.Controller,
		CheckpointStore: options.CheckpointStore,
		TaskGraphs:      options.TaskGraphs,
		Executions:      options.Executions,
		Approvals:       options.Approvals,
		OperatorActions: options.OperatorActions,
		Replay:          options.Replay,
		Artifacts:       options.Artifacts,
		Capabilities:    options.Capabilities,
		FenceValidator:  options.WorkQueue,
		EventLog:        options.EventLog,
		Now:             clock.Now,
	})
	return &Service{
		runtime:        local,
		workQueue:      options.WorkQueue,
		workAttempts:   options.WorkAttempts,
		workers:        options.Workers,
		scheduler:      options.Scheduler,
		clock:          clock,
		backendProfile: firstNonEmpty(options.BackendProfile, "local-lite"),
	}
}

func (s *Service) Runtime() *LocalWorkflowRuntime {
	return s.runtime
}

func (s *Service) SetReplayProjectionWriter(writer ReplayProjectionWriter) {
	s.replayProjectionWriter = writer
}

func (s *Service) SetCapabilities(resolver TaskCapabilityResolver) {
	if s.runtime != nil {
		s.runtime.Capabilities = resolver
	}
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock.Now()
	}
	return s.runtime.now()
}

type funcClock struct {
	now func() time.Time
}

func (c funcClock) Now() time.Time {
	return c.now().UTC()
}

func (s *Service) ReevaluateTaskGraph(ctx context.Context, cmd ReevaluateTaskGraphCommand) (TaskActivationResult, TaskCommandResult, error) {
	action, result, duplicate, err := s.beginAction(OperatorActionReevaluate, cmd.RequestID, cmd.Actor, cmd.Roles, cmd.GraphID, "", "", "", cmd.Note, 0)
	if err != nil {
		return TaskActivationResult{}, TaskCommandResult{}, err
	}
	if duplicate {
		return TaskActivationResult{}, result, nil
	}
	if s.asyncDispatchEnabled() {
		now := s.now()
		action.Status = OperatorActionStatusApplied
		action.AppliedAt = &now
		if err := s.runtime.OperatorActions.Save(action); err != nil {
			return TaskActivationResult{}, TaskCommandResult{}, err
		}
		item := WorkItem{
			ID:            makeID("work", WorkItemKindReevaluateTaskGraph, cmd.GraphID, now),
			Kind:          WorkItemKindReevaluateTaskGraph,
			Status:        WorkItemStatusQueued,
			DedupeKey:     fmt.Sprintf("reevaluate:%s", cmd.GraphID),
			GraphID:       cmd.GraphID,
			AvailableAt:   now,
			LastUpdatedAt: now,
			Reason:        firstNonEmpty(cmd.Note, "operator requested task-graph reevaluation"),
			WakeupKind:    SchedulerWakeupOperator,
		}
		enqueued, err := s.enqueueWorkItem(item)
		if err != nil {
			return TaskActivationResult{}, TaskCommandResult{}, err
		}
		if err := s.appendReplay(ReplayEventRecord{
			EventID:           makeID("replay", "reevaluate-enqueued", cmd.GraphID, now),
			RootCorrelationID: cmd.GraphID,
			ParentWorkflowID:  cmd.GraphID,
			GraphID:           cmd.GraphID,
			ActionType:        string(OperatorActionReevaluate),
			Summary:           fmt.Sprintf("enqueued task-graph reevaluation for %s", cmd.GraphID),
			OccurredAt:        now,
			OperatorActionID:  action.ActionID,
			DetailsJSON:       mustMarshalReplayDetails(operatorAsyncDispatchReplayDetails{WorkerAction: string(OperatorActionReevaluate), WorkItemID: item.ID, WorkItemKind: string(item.Kind), SchedulerDecision: "operator_reevaluate", StoreBackendProfile: s.backendProfile}),
		}); err != nil {
			return TaskActivationResult{}, TaskCommandResult{}, err
		}
		enqueued.Action = action
		return TaskActivationResult{GraphID: cmd.GraphID, EvaluatedAt: now}, enqueued, nil
	}
	activation, err := s.runtime.ReevaluateTaskGraph(ExecutionContext{
		WorkflowID:    firstNonEmpty(cmd.GraphID, "runtime-reevaluator"),
		CorrelationID: firstNonEmpty(cmd.GraphID, "runtime-reevaluator"),
		Attempt:       1,
	}, cmd.GraphID)
	if err != nil {
		failed, failureErr := s.failAction(action, result, err)
		return TaskActivationResult{}, failed, failureErr
	}
	action.Status = OperatorActionStatusApplied
	appliedAt := s.now()
	action.AppliedAt = &appliedAt
	if err := s.runtime.OperatorActions.Save(action); err != nil {
		return TaskActivationResult{}, TaskCommandResult{}, err
	}
	if err := s.appendReplay(ReplayEventRecord{
		EventID:           makeID("replay", "reevaluate", cmd.GraphID, appliedAt),
		RootCorrelationID: cmd.GraphID,
		ParentWorkflowID:  cmd.GraphID,
		GraphID:           cmd.GraphID,
		ActionType:        string(OperatorActionReevaluate),
		Summary:           fmt.Sprintf("reevaluated task graph %s", cmd.GraphID),
		OccurredAt:        appliedAt,
		OperatorActionID:  action.ActionID,
	}); err != nil {
		return TaskActivationResult{}, TaskCommandResult{}, err
	}
	if err := s.rebuildTaskGraphProjection(ctx, cmd.GraphID); err != nil {
		return TaskActivationResult{}, TaskCommandResult{}, err
	}
	return activation, TaskCommandResult{
		Action:  action,
		GraphID: cmd.GraphID,
		Status:  "",
	}, nil
}

func (s *Service) ExecuteAutoReadyFollowUps(ctx context.Context, graphID string, policy AutoExecutionPolicy) (FollowUpExecutionBatchResult, error) {
	result, err := s.runtime.ExecuteReadyFollowUps(ctx, ExecutionContext{
		WorkflowID:    graphID,
		CorrelationID: graphID,
		Attempt:       1,
	}, graphID, policy)
	if err != nil {
		return FollowUpExecutionBatchResult{}, err
	}
	if err := s.rebuildTaskGraphProjection(ctx, graphID); err != nil {
		return FollowUpExecutionBatchResult{}, err
	}
	return result, nil
}

func (s *Service) beginAction(actionType OperatorActionType, requestID string, actor string, roles []string, graphID string, taskID string, approvalID string, workflowID string, note string, expectedVersion int64) (OperatorActionRecord, TaskCommandResult, bool, error) {
	if strings.TrimSpace(requestID) == "" {
		return OperatorActionRecord{}, TaskCommandResult{}, false, fmt.Errorf("request_id is required")
	}
	if s.runtime.OperatorActions == nil {
		return OperatorActionRecord{}, TaskCommandResult{}, false, fmt.Errorf("operator action store is required")
	}
	if existing, ok, err := s.runtime.OperatorActions.LoadByRequestID(requestID); err != nil {
		return OperatorActionRecord{}, TaskCommandResult{}, false, err
	} else if ok {
		return existing, TaskCommandResult{
			Action:         existing,
			GraphID:        existing.GraphID,
			TaskID:         existing.TaskID,
			ApprovalID:     existing.ApprovalID,
			FailureSummary: existing.FailureSummary,
		}, true, nil
	}
	return OperatorActionRecord{
		ActionID:        makeID("operator-action", actionType, requestID),
		RequestID:       requestID,
		ActionType:      actionType,
		Actor:           actor,
		Roles:           append([]string{}, roles...),
		GraphID:         graphID,
		TaskID:          taskID,
		ApprovalID:      approvalID,
		WorkflowID:      workflowID,
		Status:          OperatorActionStatusRequested,
		Note:            note,
		RequestedAt:     s.now(),
		ExpectedVersion: expectedVersion,
	}, TaskCommandResult{}, false, nil
}

func (s *Service) failAction(action OperatorActionRecord, result TaskCommandResult, err error) (TaskCommandResult, error) {
	action.Status = OperatorActionStatusFailed
	now := s.now()
	action.AppliedAt = &now
	action.FailureSummary = err.Error()
	_ = s.runtime.OperatorActions.Save(action)
	_ = s.appendReplay(ReplayEventRecord{
		EventID:           makeID("replay", string(action.ActionType), action.RequestID, now),
		RootCorrelationID: firstNonEmpty(action.GraphID, action.WorkflowID),
		ParentWorkflowID:  action.WorkflowID,
		WorkflowID:        action.WorkflowID,
		GraphID:           action.GraphID,
		TaskID:            action.TaskID,
		ApprovalID:        action.ApprovalID,
		ActionType:        string(action.ActionType),
		Summary:           fmt.Sprintf("operator action %s failed", action.ActionType),
		OccurredAt:        now,
		DetailsJSON:       fmt.Sprintf(`{"failure_summary":%q}`, err.Error()),
		OperatorActionID:  action.ActionID,
	})
	result.Action = action
	result.FailureSummary = err.Error()
	return result, err
}

func (s *Service) appendReplay(event ReplayEventRecord) error {
	if s.runtime.Replay == nil {
		return nil
	}
	return s.runtime.Replay.Append(event)
}

func (s *Service) asyncDispatchEnabled() bool {
	return s != nil && s.workQueue != nil && s.scheduler != nil
}

func (s *Service) enqueueWorkItem(item WorkItem) (TaskCommandResult, error) {
	if s.workQueue == nil {
		return TaskCommandResult{}, fmt.Errorf("work queue store is required")
	}
	if err := s.workQueue.Enqueue(item); err != nil {
		return TaskCommandResult{}, err
	}
	return TaskCommandResult{
		GraphID:               item.GraphID,
		TaskID:                item.TaskID,
		ExecutionID:           item.ExecutionID,
		ApprovalID:            item.ApprovalID,
		AsyncDispatchAccepted: true,
		EnqueuedWorkItemIDs:   []string{item.ID},
		EnqueuedWorkKinds:     []WorkItemKind{item.Kind},
	}, nil
}

func (s *Service) rebuildTaskGraphProjection(ctx context.Context, graphID string) error {
	if s == nil || s.replayProjectionWriter == nil || strings.TrimSpace(graphID) == "" {
		return nil
	}
	_, err := s.replayProjectionWriter.RebuildTaskGraph(ctx, graphID)
	return err
}

func (s *Service) loadTaskGraph(graphID string) (TaskGraphSnapshot, error) {
	return s.runtime.loadTaskGraphSnapshot(graphID)
}

func (s *Service) locateTask(graphID string, taskID string) (TaskGraphSnapshot, FollowUpTaskRecord, error) {
	if strings.TrimSpace(taskID) == "" {
		return TaskGraphSnapshot{}, FollowUpTaskRecord{}, fmt.Errorf("task id is required")
	}
	if strings.TrimSpace(graphID) != "" {
		snapshot, err := s.loadTaskGraph(graphID)
		if err != nil {
			return TaskGraphSnapshot{}, FollowUpTaskRecord{}, err
		}
		record, ok := findTaskRecord(snapshot.RegisteredTasks, taskID)
		if !ok {
			return TaskGraphSnapshot{}, FollowUpTaskRecord{}, &NotFoundError{Resource: "follow_up_task", ID: taskID}
		}
		return snapshot, record, nil
	}
	if s.runtime.TaskGraphs == nil {
		return TaskGraphSnapshot{}, FollowUpTaskRecord{}, fmt.Errorf("task graph store is required")
	}
	snapshots, err := s.runtime.TaskGraphs.List()
	if err != nil {
		return TaskGraphSnapshot{}, FollowUpTaskRecord{}, err
	}
	matches := make([]struct {
		snapshot TaskGraphSnapshot
		record   FollowUpTaskRecord
	}, 0, 1)
	for _, snapshot := range snapshots {
		hydrated, err := s.loadTaskGraph(snapshot.Graph.GraphID)
		if err != nil {
			return TaskGraphSnapshot{}, FollowUpTaskRecord{}, err
		}
		record, ok := findTaskRecord(hydrated.RegisteredTasks, taskID)
		if ok {
			matches = append(matches, struct {
				snapshot TaskGraphSnapshot
				record   FollowUpTaskRecord
			}{snapshot: hydrated, record: record})
		}
	}
	if len(matches) == 0 {
		return TaskGraphSnapshot{}, FollowUpTaskRecord{}, &NotFoundError{Resource: "follow_up_task", ID: taskID}
	}
	if len(matches) > 1 {
		return TaskGraphSnapshot{}, FollowUpTaskRecord{}, &ConflictError{Resource: "follow_up_task", ID: taskID, Reason: "task id is not unique across graphs"}
	}
	return matches[0].snapshot, matches[0].record, nil
}

func findTaskRecord(records []FollowUpTaskRecord, taskID string) (FollowUpTaskRecord, bool) {
	for _, record := range records {
		if record.Task.ID == taskID {
			return record, true
		}
	}
	return FollowUpTaskRecord{}, false
}

func findTaskRecordIndex(records []FollowUpTaskRecord, taskID string) int {
	for i := range records {
		if records[i].Task.ID == taskID {
			return i
		}
	}
	return -1
}

func taskGraphView(snapshot TaskGraphSnapshot, executions []TaskExecutionRecord, pending *ApprovalStateRecord, artifacts []reporting.WorkflowArtifact, actions []OperatorActionRecord) TaskGraphView {
	meta := make([]WorkflowArtifactMeta, 0, len(artifacts))
	for _, artifact := range artifacts {
		meta = append(meta, WorkflowArtifactMeta{
			ID:         artifact.ID,
			Kind:       string(artifact.Kind),
			WorkflowID: artifact.WorkflowID,
			TaskID:     artifact.TaskID,
			StorageRef: artifact.Ref.Location,
			Summary:    artifact.Ref.Summary,
			CreatedAt:  artifact.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return TaskGraphView{
		Snapshot:        snapshot,
		Executions:      executions,
		PendingApproval: pending,
		Artifacts:       meta,
		Actions:         actions,
	}
}

func sortedTaskActions(actions []OperatorActionRecord) []OperatorActionRecord {
	items := append([]OperatorActionRecord{}, actions...)
	slices.SortFunc(items, func(a, b OperatorActionRecord) int {
		switch {
		case a.RequestedAt.Before(b.RequestedAt):
			return -1
		case a.RequestedAt.After(b.RequestedAt):
			return 1
		case a.ActionID < b.ActionID:
			return -1
		case a.ActionID > b.ActionID:
			return 1
		default:
			return 0
		}
	})
	return items
}
