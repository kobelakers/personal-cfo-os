package product

import (
	"context"
	"fmt"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

type OperatorSurfaceOptions struct {
	Query                   *runtime.QueryService
	Replay                  *runtime.ReplayQueryService
	Operator                *runtime.OperatorService
	Service                 *runtime.Service
	Benchmarks              *BenchmarkSurfaceService
	RuntimeProfile          string
	RuntimeBackend          string
	BlobBackend             string
	UIMode                  string
	SupportedSchemaVersions []string
}

type OperatorSurfaceService struct {
	query    *runtime.QueryService
	replay   *runtime.ReplayQueryService
	operator *runtime.OperatorService
	service  *runtime.Service
	meta     ProfileMeta
	bench    *BenchmarkSurfaceService
}

func NewOperatorSurfaceService(options OperatorSurfaceOptions) *OperatorSurfaceService {
	return &OperatorSurfaceService{
		query:    options.Query,
		replay:   options.Replay,
		operator: options.Operator,
		service:  options.Service,
		bench:    options.Benchmarks,
		meta: ProfileMeta{
			RuntimeProfile:          firstNonEmpty(options.RuntimeProfile, "local-lite"),
			RuntimeBackend:          firstNonEmpty(options.RuntimeBackend, "sqlite"),
			BlobBackend:             firstNonEmpty(options.BlobBackend, "localfs"),
			UIMode:                  firstNonEmpty(options.UIMode, "api-only"),
			SupportedSchemaVersions: append([]string{}, options.SupportedSchemaVersions...),
		},
	}
}

func (s *OperatorSurfaceService) Meta(ctx context.Context) (ProfileMeta, error) {
	meta := s.meta
	if s.bench != nil {
		runs, err := s.bench.ListRuns()
		if err != nil {
			return ProfileMeta{}, err
		}
		meta.BenchmarkCatalog = make([]string, 0, len(runs))
		for _, item := range runs {
			meta.BenchmarkCatalog = append(meta.BenchmarkCatalog, item.ID)
		}
	}
	return meta, nil
}

func (s *OperatorSurfaceService) ListTaskGraphs(ctx context.Context) ([]runtime.TaskGraphView, error) {
	return s.query.ListTaskGraphs(ctx)
}

func (s *OperatorSurfaceService) GetTaskGraph(ctx context.Context, graphID string) (runtime.TaskGraphView, error) {
	return s.query.GetTaskGraph(ctx, graphID)
}

func (s *OperatorSurfaceService) ListPendingApprovals(ctx context.Context) ([]runtime.ApprovalStateRecord, error) {
	return s.query.ListPendingApprovals(ctx)
}

func (s *OperatorSurfaceService) GetApprovalDetail(ctx context.Context, approvalID string) (ApprovalDetail, error) {
	approval, err := s.query.GetApproval(ctx, approvalID)
	if err != nil {
		return ApprovalDetail{}, err
	}
	var graph *runtime.TaskGraphView
	if strings.TrimSpace(approval.GraphID) != "" {
		view, err := s.query.GetTaskGraph(ctx, approval.GraphID)
		if err == nil {
			graph = &view
		}
	}
	return ApprovalDetail{
		Approval:   approval,
		TaskGraph:  graph,
		ReplayHint: observability.ReplayQuery{ApprovalID: approval.ApprovalID},
	}, nil
}

func (s *OperatorSurfaceService) QueryReplay(ctx context.Context, query observability.ReplayQuery) (observability.ReplayView, error) {
	return s.replay.Query(ctx, query)
}

func (s *OperatorSurfaceService) CompareReplay(ctx context.Context, left observability.ReplayQuery, right observability.ReplayQuery) (observability.ReplayComparison, error) {
	return s.replay.Compare(ctx, left, right)
}

func (s *OperatorSurfaceService) LoadArtifact(ctx context.Context, artifactID string) (ArtifactContentView, error) {
	artifact, err := s.query.GetArtifact(ctx, artifactID)
	if err != nil {
		return ArtifactContentView{}, err
	}
	return BuildArtifactContentView(artifact), nil
}

func (s *OperatorSurfaceService) Approve(ctx context.Context, cmd runtime.ApproveTaskCommand) (runtime.TaskCommandResult, error) {
	if s.operator == nil {
		return runtime.TaskCommandResult{}, fmt.Errorf("operator service is required")
	}
	return s.operator.ApproveTask(ctx, cmd)
}

func (s *OperatorSurfaceService) Deny(ctx context.Context, cmd runtime.DenyTaskCommand) (runtime.TaskCommandResult, error) {
	if s.operator == nil {
		return runtime.TaskCommandResult{}, fmt.Errorf("operator service is required")
	}
	return s.operator.DenyTask(ctx, cmd)
}

func (s *OperatorSurfaceService) Resume(ctx context.Context, cmd runtime.ResumeFollowUpTaskCommand) (runtime.TaskCommandResult, error) {
	if s.operator == nil {
		return runtime.TaskCommandResult{}, fmt.Errorf("operator service is required")
	}
	return s.operator.ResumeFollowUpTask(ctx, cmd)
}

func (s *OperatorSurfaceService) Retry(ctx context.Context, cmd runtime.RetryFailedFollowUpTaskCommand) (runtime.TaskCommandResult, error) {
	if s.operator == nil {
		return runtime.TaskCommandResult{}, fmt.Errorf("operator service is required")
	}
	return s.operator.RetryFailedFollowUpTask(ctx, cmd)
}

func (s *OperatorSurfaceService) Reevaluate(ctx context.Context, cmd runtime.ReevaluateTaskGraphCommand) (runtime.TaskActivationResult, runtime.TaskCommandResult, error) {
	if s.service == nil {
		return runtime.TaskActivationResult{}, runtime.TaskCommandResult{}, fmt.Errorf("runtime service is required")
	}
	return s.service.ReevaluateTaskGraph(ctx, cmd)
}

func (s *OperatorSurfaceService) Benchmarks() *BenchmarkSurfaceService {
	return s.bench
}
