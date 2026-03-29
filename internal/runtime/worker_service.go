package runtime

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) RunWorkerPass(ctx context.Context, policy AutoExecutionPolicy, dryRun bool) (WorkerPassResult, error) {
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
