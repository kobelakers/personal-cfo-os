package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AsyncWorker is the runtime-layer execution facade for 7A. It upgrades the
// old worker pass into claim/lease/reclaim semantics while still delegating
// business execution to the existing typed runtime services.
type AsyncWorker struct {
	ID                 WorkerID
	Role               WorkerRole
	Service            *Service
	Scheduler          SchedulerService
	Policy             AutoExecutionPolicy
	Clock              Clock
	LeaseTTL           time.Duration
	HeartbeatInterval  time.Duration
	ClaimBatch         int
	BackendProfile     string
	LeaseTickerFactory LeaseTickerFactory
}

func (w AsyncWorker) RunOnce(ctx context.Context, dryRun bool) (WorkerPassResult, error) {
	now := w.now()
	result := WorkerPassResult{
		WorkerID:    string(w.ID),
		DryRun:      dryRun,
		CompletedAt: now.Format(time.RFC3339Nano),
	}
	if w.Service == nil {
		return result, fmt.Errorf("service is required")
	}
	if w.Service.workers != nil {
		_ = w.Service.workers.Register(WorkerRegistration{
			WorkerID:       w.ID,
			Role:           w.Role,
			BackendProfile: firstNonEmpty(w.BackendProfile, w.Service.backendProfile),
			StartedAt:      now,
			LastHeartbeat:  now,
		})
	}
	if !dryRun && (w.Role == WorkerRoleScheduler || w.Role == WorkerRoleAll) {
		tick, err := w.Scheduler.Tick(ctx)
		if err != nil {
			return result, err
		}
		result.ScannedGraphs = tick.ScannedGraphs
		for _, wakeup := range tick.DispatchedWakeups {
			result.SchedulerWakeups = append(result.SchedulerWakeups, wakeup.ID)
		}
	}
	if !dryRun && w.Service.workQueue != nil {
		reclaimed, err := w.Service.workQueue.ReclaimExpired(now)
		if err != nil {
			return result, err
		}
		for _, reclaim := range reclaimed {
			result.ReclaimedWorkItemIDs = append(result.ReclaimedWorkItemIDs, reclaim.WorkItemID)
			if err := w.Service.recordAsyncReplayEvent(ReplayEventRecord{
				EventID:    makeID("replay", "lease-reclaimed", reclaim.WorkItemID, reclaim.ReclaimedAt),
				GraphID:    "",
				TaskID:     "",
				ActionType: "lease_reclaimed",
				Summary:    reclaim.Reason,
				OccurredAt: reclaim.ReclaimedAt,
				DetailsJSON: mustMarshalReplayDetails(AsyncReplayEventDetails{
					WorkerID:            string(reclaim.WorkerID),
					WorkItemID:          reclaim.WorkItemID,
					LeaseID:             reclaim.LeaseID,
					FencingToken:        reclaim.FencingToken,
					ReclaimReason:       reclaim.Reason,
					StoreBackendProfile: w.BackendProfile,
				}),
			}); err != nil {
				return result, err
			}
			if err := w.Service.handleLeaseReclaim(ctx, reclaim, w.Policy); err != nil {
				return result, err
			}
		}
	}
	if dryRun || w.Role == WorkerRoleScheduler {
		result.CompletedAt = w.now().Format(time.RFC3339Nano)
		return result, nil
	}
	claims, err := w.Service.workQueue.ClaimReady(w.ID, w.claimBatch(), now, w.leaseTTL())
	if err != nil {
		return result, err
	}
	for _, claim := range claims {
		result.ClaimedWorkItemIDs = append(result.ClaimedWorkItemIDs, claim.WorkItem.ID)
		if err := w.Service.recordAsyncReplayEvent(ReplayEventRecord{
			EventID:           makeID("replay", "work-claimed", claim.WorkItem.ID, claim.ClaimedAt),
			RootCorrelationID: firstNonEmpty(claim.WorkItem.GraphID, claim.WorkItem.WorkflowID),
			WorkflowID:        claim.WorkItem.WorkflowID,
			GraphID:           claim.WorkItem.GraphID,
			TaskID:            claim.WorkItem.TaskID,
			ApprovalID:        claim.WorkItem.ApprovalID,
			ExecutionID:       claim.WorkItem.ExecutionID,
			ActionType:        "work_claimed",
			Summary:           claim.WorkItem.Reason,
			OccurredAt:        claim.ClaimedAt,
			DetailsJSON: mustMarshalReplayDetails(AsyncReplayEventDetails{
				WorkerID:            string(claim.WorkerID),
				WorkItemID:          claim.WorkItem.ID,
				WorkItemKind:        string(claim.WorkItem.Kind),
				LeaseID:             claim.LeaseID,
				FencingToken:        claim.FencingToken,
				StoreBackendProfile: w.BackendProfile,
				GraphID:             claim.WorkItem.GraphID,
				TaskID:              claim.WorkItem.TaskID,
				ExecutionID:         claim.WorkItem.ExecutionID,
				ApprovalID:          claim.WorkItem.ApprovalID,
			}),
		}); err != nil {
			return result, err
		}
		if err := w.heartbeat(claim); err != nil {
			return result, err
		}
		if err := w.processClaim(ctx, claim, &result); err != nil {
			return result, err
		}
	}
	result.CompletedAt = w.now().Format(time.RFC3339Nano)
	return result, nil
}

func (w AsyncWorker) processClaim(ctx context.Context, claim WorkClaim, result *WorkerPassResult) error {
	claimCtx, cancel := context.WithCancel(ctx)
	renewalState, renewalStopped := w.startLeaseRenewal(claimCtx, cancel, claim)
	defer func() {
		cancel()
		<-renewalStopped
	}()

	attempt := ExecutionAttempt{
		AttemptID:    makeID("attempt", claim.WorkItem.ID, claim.FencingToken, w.now()),
		WorkItemID:   claim.WorkItem.ID,
		WorkItemKind: claim.WorkItem.Kind,
		GraphID:      claim.WorkItem.GraphID,
		TaskID:       claim.WorkItem.TaskID,
		ExecutionID:  claim.WorkItem.ExecutionID,
		ApprovalID:   claim.WorkItem.ApprovalID,
		WorkerID:     w.ID,
		LeaseID:      claim.LeaseID,
		FencingToken: claim.FencingToken,
		Status:       ExecutionAttemptStatusStarted,
		StartedAt:    w.now(),
	}
	if w.Service.workAttempts != nil {
		if err := w.Service.workAttempts.SaveAttempt(attempt); err != nil {
			return err
		}
	}
	fence := FenceValidation{
		WorkItemID:   claim.WorkItem.ID,
		LeaseID:      claim.LeaseID,
		FencingToken: claim.FencingToken,
		WorkerID:     w.ID,
	}
	var err error
	switch claim.WorkItem.Kind {
	case WorkItemKindSchedulerWakeup:
		err = w.Service.handleSchedulerWakeup(claimCtx, claim.WorkItem)
	case WorkItemKindReevaluateTaskGraph:
		err = w.Service.reevaluateTaskGraphAsync(claimCtx, claim.WorkItem.GraphID, w.ID, fence)
		if err == nil {
			result.Reevaluated = append(result.Reevaluated, claim.WorkItem.GraphID)
		}
	case WorkItemKindExecuteReadyTask:
		var batch FollowUpExecutionBatchResult
		batch, err = w.Service.executeReadyTasksAsync(claimCtx, claim.WorkItem.GraphID, w.Policy, w.ID, fence)
		if err == nil {
			result.Executed = append(result.Executed, batch.ExecutedTasks...)
		}
	case WorkItemKindResumeApprovedCheckpoint:
		var commandResult TaskCommandResult
		commandResult, err = w.Service.resumeApprovedTaskAsync(claimCtx, claim.WorkItem.GraphID, claim.WorkItem.TaskID, claim.WorkItem.ApprovalID, w.ID, fence)
		if err == nil && commandResult.TaskID != "" {
			result.ResumedTasks = append(result.ResumedTasks, commandResult.TaskID)
		}
	case WorkItemKindRetryFailedExecution:
		var commandResult TaskCommandResult
		commandResult, err = w.Service.retryFailedTaskAsync(claimCtx, claim.WorkItem.GraphID, claim.WorkItem.TaskID, claim.WorkItem.ExecutionID, w.ID, fence)
		if err == nil && commandResult.ExecutionID != "" {
			result.Executed = append(result.Executed, TaskExecutionRecord{
				ExecutionID:   commandResult.ExecutionID,
				TaskID:        commandResult.TaskID,
				ParentGraphID: commandResult.GraphID,
				Status:        commandResult.Status,
			})
		}
	default:
		err = fmt.Errorf("unsupported work item kind %q", claim.WorkItem.Kind)
	}
	cancel()
	<-renewalStopped
	renewalErr := renewalState.Err()
	if renewalErr != nil {
		err = renewalErr
	}
	finished := w.now()
	if err != nil {
		if renewalErr != nil {
			attempt.Status = ExecutionAttemptStatusAbandoned
			if isFenceConflict(renewalErr) || IsNotFound(renewalErr) {
				attempt.Status = ExecutionAttemptStatusReclaimed
			}
			attempt.FailureSummary = renewalErr.Error()
		} else {
			attempt.Status = ExecutionAttemptStatusFailed
			attempt.FailureCategory = FailureCategoryProtocol
			attempt.FailureSummary = err.Error()
			if isFenceConflict(err) {
				attempt.Status = ExecutionAttemptStatusReclaimed
			}
		}
		attempt.FinishedAt = &finished
		if w.Service.workAttempts != nil {
			if updateErr := w.Service.workAttempts.UpdateAttempt(attempt); updateErr != nil {
				return updateErr
			}
		}
		if renewalErr != nil {
			if isFenceConflict(renewalErr) || IsNotFound(renewalErr) {
				result.SkippedTasks = append(result.SkippedTasks, claim.WorkItem.ID)
				return nil
			}
			return renewalErr
		}
		if isFenceConflict(err) {
			result.SkippedTasks = append(result.SkippedTasks, claim.WorkItem.ID)
			return nil
		}
		if failErr := w.Service.workQueue.Fail(fence, err.Error(), finished); failErr != nil && !isFenceConflict(failErr) {
			return failErr
		}
		result.FailedWorkItemIDs = append(result.FailedWorkItemIDs, claim.WorkItem.ID)
		return err
	}
	attempt.Status = ExecutionAttemptStatusSucceeded
	attempt.FinishedAt = &finished
	if w.Service.workAttempts != nil {
		if updateErr := w.Service.workAttempts.UpdateAttempt(attempt); updateErr != nil {
			return updateErr
		}
	}
	if completeErr := w.Service.workQueue.Complete(fence, finished); completeErr != nil && !isFenceConflict(completeErr) {
		return completeErr
	}
	result.CompletedWorkItemIDs = append(result.CompletedWorkItemIDs, claim.WorkItem.ID)
	return nil
}

type leaseRenewalState struct {
	mu  sync.RWMutex
	err error
}

func (s *leaseRenewalState) Set(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

func (s *leaseRenewalState) Err() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (w AsyncWorker) startLeaseRenewal(ctx context.Context, cancel context.CancelFunc, claim WorkClaim) (*leaseRenewalState, <-chan struct{}) {
	state := &leaseRenewalState{}
	stopped := make(chan struct{})
	if w.Service == nil || w.Service.workQueue == nil {
		close(stopped)
		return state, stopped
	}
	ticker := w.leaseTickerFactory().New(w.heartbeatInterval())
	go func() {
		defer close(stopped)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C():
				if err := w.heartbeat(claim); err != nil {
					state.Set(err)
					cancel()
					return
				}
			}
		}
	}()
	return state, stopped
}

func (w AsyncWorker) heartbeat(claim WorkClaim) error {
	if w.Service.workQueue == nil {
		return nil
	}
	now := w.now()
	heartbeat := LeaseHeartbeat{
		WorkItemID:     claim.WorkItem.ID,
		WorkerID:       w.ID,
		LeaseID:        claim.LeaseID,
		FencingToken:   claim.FencingToken,
		RecordedAt:     now,
		LeaseExpiresAt: now.Add(w.leaseTTL()),
	}
	if err := w.Service.workQueue.Heartbeat(heartbeat); err != nil {
		return err
	}
	return w.Service.recordAsyncReplayEvent(ReplayEventRecord{
		EventID:           makeID("replay", "lease-heartbeat", claim.WorkItem.ID, now),
		RootCorrelationID: firstNonEmpty(claim.WorkItem.GraphID, claim.WorkItem.WorkflowID),
		WorkflowID:        claim.WorkItem.WorkflowID,
		GraphID:           claim.WorkItem.GraphID,
		TaskID:            claim.WorkItem.TaskID,
		ApprovalID:        claim.WorkItem.ApprovalID,
		ExecutionID:       claim.WorkItem.ExecutionID,
		ActionType:        "lease_heartbeat",
		Summary:           "worker heartbeat recorded",
		OccurredAt:        now,
		DetailsJSON: mustMarshalReplayDetails(AsyncReplayEventDetails{
			WorkerID:            string(w.ID),
			WorkItemID:          claim.WorkItem.ID,
			WorkItemKind:        string(claim.WorkItem.Kind),
			LeaseID:             claim.LeaseID,
			FencingToken:        claim.FencingToken,
			HeartbeatTimestamps: []string{now.UTC().Format(time.RFC3339Nano)},
			StoreBackendProfile: w.BackendProfile,
			GraphID:             claim.WorkItem.GraphID,
			TaskID:              claim.WorkItem.TaskID,
			ExecutionID:         claim.WorkItem.ExecutionID,
			ApprovalID:          claim.WorkItem.ApprovalID,
		}),
	})
}

func (w AsyncWorker) now() time.Time {
	if w.Clock != nil {
		return w.Clock.Now()
	}
	return time.Now().UTC()
}

func (w AsyncWorker) leaseTTL() time.Duration {
	if w.LeaseTTL <= 0 {
		return 30 * time.Second
	}
	return w.LeaseTTL
}

func (w AsyncWorker) claimBatch() int {
	if w.ClaimBatch <= 0 {
		return 1
	}
	return w.ClaimBatch
}

func (w AsyncWorker) heartbeatInterval() time.Duration {
	if w.HeartbeatInterval <= 0 {
		return 10 * time.Second
	}
	return w.HeartbeatInterval
}

func (w AsyncWorker) leaseTickerFactory() LeaseTickerFactory {
	if w.LeaseTickerFactory != nil {
		return w.LeaseTickerFactory
	}
	return systemLeaseTickerFactory{}
}

func isFenceConflict(err error) bool {
	return err != nil && IsConflict(err)
}
