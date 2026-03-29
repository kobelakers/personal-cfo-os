package runtime

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type WorkflowRuntime interface {
	Checkpoint(ctx ExecutionContext, current WorkflowExecutionState, resumeState WorkflowExecutionState, stateVersion uint64, summary string) (CheckpointRecord, ResumeToken, error)
	Resume(ctx ExecutionContext, checkpointID string, token ResumeToken) (WorkflowExecutionState, error)
	HandleFailure(ctx ExecutionContext, current WorkflowExecutionState, category FailureCategory, summary string) (WorkflowExecutionState, RecoveryStrategy, error)
	PauseForApproval(ctx ExecutionContext, current WorkflowExecutionState, pending HumanApprovalPending) (WorkflowExecutionState, error)
	RegisterFollowUpTasks(ctx ExecutionContext, graph taskspec.TaskGraph, base state.FinancialWorldState) (FollowUpRegistrationResult, error)
	ReevaluateTaskGraph(ctx ExecutionContext, graphID string) (TaskActivationResult, error)
	ExecuteReadyFollowUps(ctx context.Context, execCtx ExecutionContext, graphID string, policy AutoExecutionPolicy) (FollowUpExecutionBatchResult, error)
}

type LocalRuntimeOptions struct {
	Controller      WorkflowController
	CheckpointStore CheckpointStore
	Journal         *CheckpointJournal
	Timeline        *WorkflowTimeline
	EventLog        *observability.EventLog
	TaskGraphs      TaskGraphStore
	Executions      TaskExecutionStore
	Approvals       ApprovalStateStore
	OperatorActions OperatorActionStore
	Replay          ReplayStore
	Artifacts       ArtifactMetadataStore
	Capabilities    TaskCapabilityResolver
	Now             func() time.Time
}

type LocalWorkflowRuntime struct {
	Controller      WorkflowController
	CheckpointStore CheckpointStore
	Journal         *CheckpointJournal
	Timeline        *WorkflowTimeline
	EventLog        *observability.EventLog
	TaskGraphs      TaskGraphStore
	Executions      TaskExecutionStore
	Approvals       ApprovalStateStore
	OperatorActions OperatorActionStore
	Replay          ReplayStore
	Artifacts       ArtifactMetadataStore
	Capabilities    TaskCapabilityResolver
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
	taskGraphs := options.TaskGraphs
	if taskGraphs == nil {
		taskGraphs = NewInMemoryTaskGraphStore()
	}
	executions := options.Executions
	if executions == nil {
		executions = NewInMemoryTaskExecutionStore()
	}
	approvals := options.Approvals
	if approvals == nil {
		approvals = NewInMemoryApprovalStateStore()
	}
	operatorActions := options.OperatorActions
	if operatorActions == nil {
		operatorActions = NewInMemoryOperatorActionStore()
	}
	replay := options.Replay
	if replay == nil {
		replay = NewInMemoryReplayStore()
	}
	artifacts := options.Artifacts
	if artifacts == nil {
		artifacts = NewInMemoryArtifactMetadataStore()
	}
	return &LocalWorkflowRuntime{
		Controller:      controller,
		CheckpointStore: store,
		Journal:         journal,
		Timeline:        timeline,
		EventLog:        options.EventLog,
		TaskGraphs:      taskGraphs,
		Executions:      executions,
		Approvals:       approvals,
		OperatorActions: operatorActions,
		Replay:          replay,
		Artifacts:       artifacts,
		Capabilities:    options.Capabilities,
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
	if err := r.CheckpointStore.SaveResumeToken(token); err != nil {
		return CheckpointRecord{}, ResumeToken{}, err
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
	storedToken, err := r.CheckpointStore.LoadResumeToken(token.Token)
	if err != nil {
		return "", err
	}
	controller := r.Controller
	if controller == nil {
		controller = DefaultWorkflowController{}
	}
	next, err := controller.Resume(checkpoint, storedToken, r.now())
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

func (r LocalWorkflowRuntime) RegisterFollowUpTasks(ctx ExecutionContext, graph taskspec.TaskGraph, base state.FinancialWorldState) (FollowUpRegistrationResult, error) {
	result, err := RegisterFollowUpTaskGraph(graph, r.Capabilities, r.now())
	if err != nil {
		return FollowUpRegistrationResult{}, err
	}
	if r.TaskGraphs == nil {
		r.TaskGraphs = NewInMemoryTaskGraphStore()
	}
	baseSnapshot := base.Snapshot("follow_up_task_graph_registered", r.now())
	snapshot := TaskGraphSnapshot{
		Graph:                        result.Graph,
		Version:                      1,
		RegisteredTasks:              result.RegisteredTasks,
		Spawned:                      result.Spawned,
		Deferred:                     result.Deferred,
		BaseStateSnapshot:            baseSnapshot,
		BaseStateSnapshotRef:         snapshotRefFor(baseSnapshot),
		LatestCommittedStateSnapshot: baseSnapshot,
		LatestCommittedStateRef:      snapshotRefFor(baseSnapshot),
		RegisteredAt:                 r.now(),
	}
	if snapshot, err = r.saveNewTaskGraphSnapshot(snapshot); err != nil {
		return FollowUpRegistrationResult{}, err
	}
	if r.Timeline != nil {
		r.Timeline.Append(WorkflowStateActing, "follow_up_tasks_registered", fmt.Sprintf("registered %d follow-up tasks", len(result.RegisteredTasks)), r.now())
	}
	r.log(ctx, "follow_up_tasks", fmt.Sprintf("registered %d generated tasks", len(result.RegisteredTasks)), map[string]string{
		"graph_id":    graph.GraphID,
		"task_count":  fmt.Sprintf("%d", len(result.RegisteredTasks)),
		"workflow_id": graph.ParentWorkflowID,
	})
	for _, item := range result.RegisteredTasks {
		r.log(ctx, "follow_up_task_registered", fmt.Sprintf("registered follow-up task %s", item.Task.ID), map[string]string{
			"graph_id":                  graph.GraphID,
			"task_id":                   item.Task.ID,
			"intent":                    string(item.Task.UserIntentType),
			"status":                    string(item.Status),
			"required_capability":       item.RequiredCapability,
			"missing_capability_reason": item.MissingCapabilityReason,
		})
	}
	return result, nil
}

func (r LocalWorkflowRuntime) ReevaluateTaskGraph(ctx ExecutionContext, graphID string) (TaskActivationResult, error) {
	if r.TaskGraphs == nil {
		return TaskActivationResult{}, fmt.Errorf("task graph store is required")
	}
	snapshot, err := r.loadTaskGraphSnapshot(graphID)
	if err != nil {
		return TaskActivationResult{}, err
	}
	expectedVersion := snapshot.Version
	updated, activation, err := ReevaluateFollowUpTaskGraph(snapshot, r.Capabilities, r.now())
	if err != nil {
		return TaskActivationResult{}, err
	}
	if updated, err = r.saveUpdatedTaskGraphSnapshot(updated, expectedVersion); err != nil {
		return TaskActivationResult{}, err
	}
	r.log(ctx, "follow_up_task_activation", fmt.Sprintf("reevaluated follow-up task graph %s", graphID), map[string]string{
		"graph_id":       graphID,
		"ready_task_ids": stringsJoin(activation.ReadyTaskIDs),
	})
	for _, item := range activation.RegisteredTasks {
		r.log(ctx, "follow_up_task_status", fmt.Sprintf("task %s reevaluated to %s", item.Task.ID, item.Status), map[string]string{
			"graph_id":            graphID,
			"task_id":             item.Task.ID,
			"intent":              string(item.Task.UserIntentType),
			"status":              string(item.Status),
			"blocking_reasons":    stringsJoin(item.BlockingReasons),
			"suppressed_reasons":  stringsJoin(item.SuppressedReasons),
			"required_capability": item.RequiredCapability,
		})
	}
	return activation, nil
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

func stringsJoin(items []string) string {
	return strings.Join(items, ",")
}
