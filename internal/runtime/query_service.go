package runtime

import (
	"context"
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
)

type QueryService struct {
	service *Service
}

func NewQueryService(service *Service) *QueryService {
	return &QueryService{service: service}
}

func (s *QueryService) ListTaskGraphs(ctx context.Context) ([]TaskGraphView, error) {
	if s.service.runtime.TaskGraphs == nil {
		return nil, fmt.Errorf("task graph store is required")
	}
	snapshots, err := s.service.runtime.TaskGraphs.List()
	if err != nil {
		return nil, err
	}
	result := make([]TaskGraphView, 0, len(snapshots))
	for _, item := range snapshots {
		view, err := s.GetTaskGraph(ctx, item.Graph.GraphID)
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, nil
}

func (s *QueryService) GetTaskGraph(_ context.Context, graphID string) (TaskGraphView, error) {
	snapshot, err := s.service.loadTaskGraph(graphID)
	if err != nil {
		return TaskGraphView{}, err
	}
	var pending *ApprovalStateRecord
	for _, task := range snapshot.RegisteredTasks {
		if approval, ok, err := s.service.runtime.Approvals.LoadByTask(graphID, task.Task.ID); err == nil && ok && approval.Status == ApprovalStatusPending {
			copy := approval
			pending = &copy
			break
		}
	}
	artifacts := make([]reporting.WorkflowArtifact, 0)
	if s.service.runtime.Artifacts != nil {
		for _, task := range snapshot.RegisteredTasks {
			items, err := s.service.runtime.Artifacts.ListArtifactsByTask(task.Task.ID)
			if err != nil {
				return TaskGraphView{}, err
			}
			artifacts = append(artifacts, items...)
		}
	}
	actions := make([]OperatorActionRecord, 0)
	if s.service.runtime.OperatorActions != nil {
		for _, task := range snapshot.RegisteredTasks {
			items, err := s.service.runtime.OperatorActions.ListByTask(graphID, task.Task.ID)
			if err != nil {
				return TaskGraphView{}, err
			}
			actions = append(actions, items...)
		}
	}
	return taskGraphView(snapshot, snapshot.ExecutedTasks, pending, artifacts, sortedTaskActions(actions)), nil
}

func (s *QueryService) ListPendingApprovals(context.Context) ([]ApprovalStateRecord, error) {
	if s.service.runtime.Approvals == nil {
		return nil, fmt.Errorf("approval state store is required")
	}
	return s.service.runtime.Approvals.ListPending()
}

func (s *QueryService) GetApproval(_ context.Context, approvalID string) (ApprovalStateRecord, error) {
	if s.service.runtime.Approvals == nil {
		return ApprovalStateRecord{}, fmt.Errorf("approval state store is required")
	}
	record, ok, err := s.service.runtime.Approvals.Load(approvalID)
	if err != nil {
		return ApprovalStateRecord{}, err
	}
	if !ok {
		return ApprovalStateRecord{}, &NotFoundError{Resource: "approval", ID: approvalID}
	}
	return record, nil
}

func (s *QueryService) GetArtifact(_ context.Context, artifactID string) (reporting.WorkflowArtifact, error) {
	if s.service.runtime.Artifacts == nil {
		return reporting.WorkflowArtifact{}, fmt.Errorf("artifact metadata store is required")
	}
	artifact, ok, err := s.service.runtime.Artifacts.LoadArtifact(artifactID)
	if err != nil {
		return reporting.WorkflowArtifact{}, err
	}
	if !ok {
		return reporting.WorkflowArtifact{}, &NotFoundError{Resource: "artifact", ID: artifactID}
	}
	return artifact, nil
}

func (s *QueryService) GetExecutionRecord(_ context.Context, query ExecutionQuery) (TaskExecutionRecord, error) {
	switch {
	case query.ExecutionID != "":
		record, ok, err := s.service.runtime.Executions.Load(query.ExecutionID)
		if err != nil {
			return TaskExecutionRecord{}, err
		}
		if !ok {
			return TaskExecutionRecord{}, &NotFoundError{Resource: "task_execution", ID: query.ExecutionID}
		}
		return record, nil
	case query.GraphID != "" && query.TaskID != "":
		record, ok, err := s.service.runtime.Executions.LoadLatestByTask(query.GraphID, query.TaskID)
		if err != nil {
			return TaskExecutionRecord{}, err
		}
		if !ok {
			return TaskExecutionRecord{}, &NotFoundError{Resource: "task_execution", ID: query.TaskID}
		}
		return record, nil
	default:
		return TaskExecutionRecord{}, fmt.Errorf("execution query requires execution_id or graph_id+task_id")
	}
}
