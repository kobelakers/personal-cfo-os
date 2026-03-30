package runtime

import (
	"context"
	"fmt"
	"time"
)

type WorkerRunOptions struct {
	WorkerID          WorkerID
	Role              WorkerRole
	LeaseTTL          time.Duration
	HeartbeatInterval time.Duration
	ClaimBatch        int
}

func (s *Service) RunWorkerPass(ctx context.Context, policy AutoExecutionPolicy, dryRun bool) (WorkerPassResult, error) {
	return s.RunAsyncWorkerOnce(ctx, policy, WorkerRunOptions{
		WorkerID:   WorkerID("worker-pass"),
		Role:       WorkerRoleAll,
		LeaseTTL:   30 * time.Second,
		ClaimBatch: 4,
	}, dryRun)
}

func (s *Service) RunAsyncWorkerOnce(ctx context.Context, policy AutoExecutionPolicy, options WorkerRunOptions, dryRun bool) (WorkerPassResult, error) {
	if s.workQueue != nil && s.scheduler != nil && s.workers != nil {
		worker := AsyncWorker{
			ID:                firstNonEmptyWorker(options.WorkerID, WorkerID("worker-pass")),
			Role:              firstNonEmptyWorkerRole(options.Role, WorkerRoleAll),
			Service:           s,
			Scheduler:         s.schedulerService(policy),
			Policy:            policy,
			Clock:             s.clock,
			LeaseTTL:          options.LeaseTTL,
			HeartbeatInterval: options.HeartbeatInterval,
			ClaimBatch:        options.ClaimBatch,
			BackendProfile:    s.backendProfile,
		}
		return worker.RunOnce(ctx, dryRun)
	}
	return s.runLegacyWorkerPass(ctx, policy, dryRun)
}

func firstNonEmptyWorker(value WorkerID, fallback WorkerID) WorkerID {
	if value != "" {
		return value
	}
	return fallback
}

func firstNonEmptyWorkerRole(value WorkerRole, fallback WorkerRole) WorkerRole {
	if value != "" {
		return value
	}
	return fallback
}

func (s *Service) runLegacyWorkerPass(ctx context.Context, policy AutoExecutionPolicy, dryRun bool) (WorkerPassResult, error) {
	if policy.MaxExecutionDepth <= 0 {
		policy = DefaultAutoExecutionPolicy()
	}
	query := NewQueryService(s)
	operator := NewOperatorService(s)
	graphs, err := query.ListTaskGraphs(ctx)
	if err != nil {
		return WorkerPassResult{}, err
	}
	result := WorkerPassResult{
		ScannedGraphs: len(graphs),
		DryRun:        dryRun,
	}
	for _, graph := range graphs {
		result.Reevaluated = append(result.Reevaluated, graph.Snapshot.Graph.GraphID)
		if dryRun {
			continue
		}
		_, _, err := s.ReevaluateTaskGraph(ctx, ReevaluateTaskGraphCommand{
			RequestID: makeID("worker", "reevaluate", graph.Snapshot.Graph.GraphID, s.now()),
			GraphID:   graph.Snapshot.Graph.GraphID,
			Actor:     "worker",
			Roles:     []string{"worker"},
			Note:      "worker reevaluate pass",
		})
		if err != nil && !IsConflict(err) {
			return WorkerPassResult{}, err
		}
		batch, err := s.ExecuteAutoReadyFollowUps(ctx, graph.Snapshot.Graph.GraphID, policy)
		if err != nil && !IsNotFound(err) {
			return WorkerPassResult{}, err
		}
		result.Executed = append(result.Executed, batch.ExecutedTasks...)
		refreshed, err := s.loadTaskGraph(graph.Snapshot.Graph.GraphID)
		if err != nil {
			return WorkerPassResult{}, err
		}
		for _, task := range refreshed.RegisteredTasks {
			if task.Status != TaskQueueStatusWaitingApproval {
				continue
			}
			approval, ok, err := s.runtime.Approvals.LoadByTask(refreshed.Graph.GraphID, task.Task.ID)
			if err != nil {
				return WorkerPassResult{}, err
			}
			if !ok || approval.Status != ApprovalStatusApproved {
				continue
			}
			if _, err := operator.resumeTaskInternal(ctx, ResumeFollowUpTaskCommand{
				RequestID: makeID("worker", "resume", refreshed.Graph.GraphID, task.Task.ID, s.now()),
				GraphID:   refreshed.Graph.GraphID,
				TaskID:    task.Task.ID,
				Actor:     "worker",
				Roles:     []string{"worker"},
				Note:      "worker resume approved follow-up task",
			}, false); err != nil {
				if IsConflict(err) {
					result.SkippedTasks = append(result.SkippedTasks, fmt.Sprintf("%s: %v", task.Task.ID, err))
					continue
				}
				return WorkerPassResult{}, err
			}
			result.ResumedTasks = append(result.ResumedTasks, task.Task.ID)
		}
	}
	result.CompletedAt = s.now().Format(time.RFC3339Nano)
	return result, nil
}

func (s *Service) schedulerService(policy AutoExecutionPolicy) SchedulerService {
	return SchedulerService{
		TaskGraphs:   s.runtime.TaskGraphs,
		Executions:   s.runtime.Executions,
		Approvals:    s.runtime.Approvals,
		WorkQueue:    s.workQueue,
		Wakeups:      s.scheduler,
		Capabilities: s.runtime.Capabilities,
		Replay:       s.runtime.Replay,
		Clock:        s.clock,
		DuePolicy:    DueWindowPolicy{},
		RetryPolicy: RetryBackoffPolicy{
			BaseDelay: 5 * time.Second,
			MaxDelay:  5 * time.Minute,
		},
	}
}
