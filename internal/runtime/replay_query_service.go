package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
)

type ReplayQueryService struct {
	service      *Service
	workflowRuns WorkflowRunStore
	projections  ReplayProjectionQueryStore
	artifacts    ArtifactMetadataStore
	replay       ReplayStore
}

func NewReplayQueryService(service *Service, workflowRuns WorkflowRunStore, projections ReplayProjectionQueryStore, artifacts ArtifactMetadataStore, replay ReplayStore) *ReplayQueryService {
	return &ReplayQueryService{
		service:      service,
		workflowRuns: workflowRuns,
		projections:  projections,
		artifacts:    artifacts,
		replay:       replay,
	}
}

func (s *ReplayQueryService) Query(ctx context.Context, query observability.ReplayQuery) (observability.ReplayView, error) {
	switch {
	case query.WorkflowID != "":
		return s.byWorkflow(ctx, query.WorkflowID)
	case query.TaskGraphID != "":
		return s.byTaskGraph(ctx, query.TaskGraphID)
	case query.TaskID != "":
		return s.byTask(ctx, query.TaskID)
	case query.ExecutionID != "":
		return s.byExecution(ctx, query.ExecutionID)
	case query.ApprovalID != "":
		return s.byApproval(ctx, query.ApprovalID)
	default:
		return observability.ReplayView{}, fmt.Errorf("replay query requires workflow_id, task_graph_id, task_id, execution_id, or approval_id")
	}
}

func (s *ReplayQueryService) Compare(ctx context.Context, left observability.ReplayQuery, right observability.ReplayQuery) (observability.ReplayComparison, error) {
	leftView, err := s.Query(ctx, left)
	if err != nil {
		return observability.ReplayComparison{}, err
	}
	rightView, err := s.Query(ctx, right)
	if err != nil {
		return observability.ReplayComparison{}, err
	}
	return observability.BuildReplayComparison(leftView, rightView), nil
}

func (s *ReplayQueryService) byWorkflow(_ context.Context, workflowID string) (observability.ReplayView, error) {
	if s.workflowRuns == nil {
		return observability.ReplayView{}, fmt.Errorf("workflow run store is required")
	}
	record, ok, err := s.workflowRuns.Load(workflowID)
	if err != nil {
		return observability.ReplayView{}, err
	}
	if !ok {
		return observability.ReplayView{}, &NotFoundError{Resource: "workflow_run", ID: workflowID}
	}
	artifacts, err := s.artifactsByWorkflow(workflowID)
	if err != nil {
		return observability.ReplayView{}, err
	}
	events, err := s.listReplayEventsByWorkflow(workflowID)
	if err != nil {
		return observability.ReplayView{}, err
	}
	view := observability.ReplayView{
		Query: observability.ReplayQuery{WorkflowID: workflowID},
		Scope: observability.ReplayScope{Kind: string(ReplayScopeWorkflow), ID: workflowID},
		Workflow: &observability.WorkflowReplayView{
			WorkflowID:        record.WorkflowID,
			TaskID:            record.TaskID,
			Intent:            record.Intent,
			RuntimeState:      string(record.RuntimeState),
			FailureCategory:   string(record.FailureCategory),
			FailureSummary:    record.FailureSummary,
			ApprovalID:        record.ApprovalID,
			TaskGraphID:       record.TaskGraphID,
			RootCorrelationID: record.RootCorrelationID,
			Summary:           record.Summary,
			Artifacts:         artifacts,
		},
		Summary: observability.ReplaySummary{FinalState: string(record.RuntimeState)},
		Provenance: observability.ProvenanceGraph{
			Scope: observability.ReplayScope{Kind: string(ReplayScopeWorkflow), ID: workflowID},
		},
	}
	scope := ReplayProjectionScope{ScopeKind: ReplayScopeWorkflow, ScopeID: workflowID}
	degradations := make([]observability.ReplayDegradation, 0)
	if err := s.applyWorkflowProjection(&view, scope, record.UpdatedAt, &degradations); err != nil {
		return observability.ReplayView{}, err
	}
	if len(view.Provenance.Nodes) == 0 {
		view.Provenance = bestEffortWorkflowProvenance(record, artifacts)
		view.Provenance = augmentProvenanceWithAsyncEvents(view.Provenance, events)
		degradations = append(degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationBestEffortAssembly,
			Message: "workflow provenance graph was assembled from authoritative runtime truth because durable projection rows were unavailable",
		})
	}
	if len(view.FailureAttributions) == 0 && record.FailureCategory != "" {
		view.FailureAttributions = append(view.FailureAttributions, observability.FailureAttribution{
			FailureCategory: string(record.FailureCategory),
			Summary:         firstNonEmpty(record.FailureSummary, "workflow ended with failure state"),
			RelatedKind:     "workflow",
			RelatedID:       record.WorkflowID,
		})
	}
	view.Summary.FinalState = string(record.RuntimeState)
	if len(view.Summary.AsyncRuntimeSummary) == 0 {
		view.Summary.AsyncRuntimeSummary = asyncRuntimeSummaryFromEvents(events)
	}
	if len(view.Explanation.WhyAsyncRuntime) == 0 {
		view.Explanation.WhyAsyncRuntime = asyncRuntimeExplanationFromEvents(events)
	}
	view.Degraded = len(degradations) > 0
	view.DegradationReasons = degradations
	return view, nil
}

func (s *ReplayQueryService) byTaskGraph(ctx context.Context, graphID string) (observability.ReplayView, error) {
	if s.service == nil {
		return observability.ReplayView{}, fmt.Errorf("runtime service is required")
	}
	query := NewQueryService(s.service)
	graphView, err := query.GetTaskGraph(ctx, graphID)
	if err != nil {
		return observability.ReplayView{}, err
	}
	artifacts, err := s.artifactsByGraph(graphView)
	if err != nil {
		return observability.ReplayView{}, err
	}
	events, err := s.listReplayEventsByGraph(graphID)
	if err != nil {
		return observability.ReplayView{}, err
	}
	view := observability.ReplayView{
		Query: observability.ReplayQuery{TaskGraphID: graphID},
		Scope: observability.ReplayScope{Kind: string(ReplayScopeTaskGraph), ID: graphID},
		TaskGraph: &observability.TaskGraphReplayView{
			TaskGraphID:       graphID,
			ParentWorkflowID:  graphView.Snapshot.Graph.ParentWorkflowID,
			ParentTaskID:      graphView.Snapshot.Graph.ParentTaskID,
			PendingApprovalID: pendingApprovalID(graphView.PendingApproval),
			TaskIDs:           taskIDsFromRecords(graphView.Snapshot.RegisteredTasks),
			ExecutionIDs:      executionIDs(graphView.Executions),
			Artifacts:         artifacts,
		},
		Summary: observability.ReplaySummary{
			FinalState:           summarizeGraphState(graphView),
			ChildWorkflowSummary: childWorkflowSummary(graphView.Executions),
		},
		Provenance: observability.ProvenanceGraph{
			Scope: observability.ReplayScope{Kind: string(ReplayScopeTaskGraph), ID: graphID},
		},
	}
	scope := ReplayProjectionScope{ScopeKind: ReplayScopeTaskGraph, ScopeID: graphID}
	degradations := make([]observability.ReplayDegradation, 0)
	if err := s.applyTaskGraphProjection(&view, scope, freshestGraphTimestamp(graphView, nil), &degradations); err != nil {
		return observability.ReplayView{}, err
	}
	if len(view.Provenance.Nodes) == 0 {
		view.Provenance = bestEffortTaskGraphProvenance(graphView)
		view.Provenance = augmentProvenanceWithAsyncEvents(view.Provenance, events)
		degradations = append(degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationBestEffortAssembly,
			Message: "task-graph provenance graph was assembled from authoritative runtime truth because durable projection rows were unavailable",
		})
	}
	if len(view.ExecutionAttributions) == 0 {
		view.ExecutionAttributions = bestEffortExecutionAttributions(graphView.Executions)
		if len(view.ExecutionAttributions) > 0 {
			degradations = append(degradations, observability.ReplayDegradation{
				Reason:  observability.ReplayDegradationBestEffortAssembly,
				Message: "execution attributions were assembled from execution records because durable projection rows were unavailable",
			})
		}
	}
	if len(view.FailureAttributions) == 0 {
		view.FailureAttributions = bestEffortGraphFailureAttributions(graphView)
	}
	view.Summary.FinalState = summarizeGraphState(graphView)
	if len(view.Summary.AsyncRuntimeSummary) == 0 {
		view.Summary.AsyncRuntimeSummary = asyncRuntimeSummaryFromEvents(events)
	}
	if len(view.Explanation.WhyAsyncRuntime) == 0 {
		view.Explanation.WhyAsyncRuntime = asyncRuntimeExplanationFromEvents(events)
	}
	view.Degraded = len(degradations) > 0
	view.DegradationReasons = degradations
	return view, nil
}

func (s *ReplayQueryService) listReplayEventsByWorkflow(workflowID string) ([]ReplayEventRecord, error) {
	if s.replay == nil {
		return nil, nil
	}
	return s.replay.ListByWorkflow(workflowID)
}

func (s *ReplayQueryService) listReplayEventsByGraph(graphID string) ([]ReplayEventRecord, error) {
	if s.replay == nil {
		return nil, nil
	}
	return s.replay.ListByGraph(graphID)
}

func (s *ReplayQueryService) byTask(ctx context.Context, taskID string) (observability.ReplayView, error) {
	if s.service == nil {
		return observability.ReplayView{}, fmt.Errorf("runtime service is required")
	}
	snapshot, _, err := s.service.locateTask("", taskID)
	if err != nil {
		return observability.ReplayView{}, err
	}
	view, err := s.byTaskGraph(ctx, snapshot.Graph.GraphID)
	if err != nil {
		return observability.ReplayView{}, err
	}
	view.Query = observability.ReplayQuery{TaskID: taskID}
	view.Scope = observability.ReplayScope{Kind: string(ReplayScopeTask), ID: taskID}
	if view.TaskGraph != nil {
		view.TaskGraph.TaskIDs = []string{taskID}
		view.TaskGraph.ExecutionIDs = filterExecutionIDsForTask(view.TaskGraph.ExecutionIDs, snapshot.ExecutedTasks, taskID)
	}
	view.Provenance = filterProvenanceForTask(view.Provenance, taskID)
	view.ExecutionAttributions = filterExecutionAttributionsForTask(view.ExecutionAttributions, snapshot.ExecutedTasks, taskID)
	view.FailureAttributions = filterFailureAttributionsForTask(view.FailureAttributions, taskID)
	view.Summary.ChildWorkflowSummary = filterChildWorkflowSummaryForTask(view.Summary.ChildWorkflowSummary, taskID)
	return view, nil
}

func (s *ReplayQueryService) byExecution(ctx context.Context, executionID string) (observability.ReplayView, error) {
	if s.service == nil {
		return observability.ReplayView{}, fmt.Errorf("runtime service is required")
	}
	query := NewQueryService(s.service)
	record, err := query.GetExecutionRecord(ctx, ExecutionQuery{ExecutionID: executionID})
	if err != nil {
		return observability.ReplayView{}, err
	}
	view, err := s.byTask(ctx, record.TaskID)
	if err != nil {
		return observability.ReplayView{}, err
	}
	view.Query = observability.ReplayQuery{ExecutionID: executionID}
	view.Scope = observability.ReplayScope{Kind: string(ReplayScopeExecution), ID: executionID}
	view.ExecutionAttributions = filterExecutionAttributionsForExecution(view.ExecutionAttributions, executionID)
	view.Provenance = filterProvenanceForExecution(view.Provenance, executionID)
	return view, nil
}

func (s *ReplayQueryService) byApproval(ctx context.Context, approvalID string) (observability.ReplayView, error) {
	if s.service == nil || s.service.runtime.Approvals == nil {
		return observability.ReplayView{}, fmt.Errorf("approval state store is required")
	}
	approval, ok, err := s.service.runtime.Approvals.Load(approvalID)
	if err != nil {
		return observability.ReplayView{}, err
	}
	if !ok {
		return observability.ReplayView{}, &NotFoundError{Resource: "approval", ID: approvalID}
	}
	view, err := s.byTaskGraph(ctx, approval.GraphID)
	if err != nil {
		return observability.ReplayView{}, err
	}
	view.Query = observability.ReplayQuery{ApprovalID: approvalID}
	view.Scope = observability.ReplayScope{Kind: string(ReplayScopeApproval), ID: approvalID}
	view.Approval = &observability.ApprovalReplayView{
		ApprovalID:      approval.ApprovalID,
		WorkflowID:      approval.WorkflowID,
		TaskGraphID:     approval.GraphID,
		TaskID:          approval.TaskID,
		Status:          string(approval.Status),
		RequestedAction: approval.RequestedAction,
		RequestedAt:     approval.RequestedAt,
		ResolvedAt:      approval.ResolvedAt,
		ResolvedBy:      approval.ResolvedBy,
	}
	view.Provenance = filterProvenanceForApproval(view.Provenance, approvalID)
	return view, nil
}

func (s *ReplayQueryService) applyWorkflowProjection(view *observability.ReplayView, scope ReplayProjectionScope, authoritativeFreshness time.Time, degradations *[]observability.ReplayDegradation) error {
	if s.projections == nil {
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionMissing,
			Message: "workflow replay projection store is not configured",
		})
		return nil
	}
	build, buildOK, err := s.projections.LoadBuild(scope)
	if err != nil {
		return err
	}
	projectionFound := false
	if projection, ok, err := s.projections.LoadWorkflowProjection(scope.ScopeID); err != nil {
		return err
	} else if ok {
		projectionFound = true
		view.ProjectionStatus = string(projection.ProjectionStatus)
		view.ProjectionVersion = projection.SchemaVersion
		if err := applySummaryProjection(view, projection.SummaryJSON, projection.ExplanationJSON); err != nil {
			return err
		}
		applyProjectionFreshnessDegradation(projection.ProjectionFreshness, authoritativeFreshness, degradations)
	}
	nodes, edges, err := s.projections.ListProvenance(scope)
	if err != nil {
		return err
	}
	view.Provenance = convertProvenance(scope, nodes, edges)
	if execAttrs, err := s.projections.ListExecutionAttributions(scope); err != nil {
		return err
	} else {
		view.ExecutionAttributions = convertExecutionAttributions(execAttrs)
	}
	if failureAttrs, err := s.projections.ListFailureAttributions(scope); err != nil {
		return err
	} else {
		view.FailureAttributions = convertFailureAttributions(failureAttrs)
	}
	if !buildOK {
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionMissing,
			Message: "workflow replay projection build metadata is missing",
		})
		return nil
	}
	if !projectionFound {
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionMissing,
			Message: "workflow replay projection rows are missing",
		})
	}
	applyBuildDegradations(build, degradations)
	return nil
}

func (s *ReplayQueryService) applyTaskGraphProjection(view *observability.ReplayView, scope ReplayProjectionScope, authoritativeFreshness time.Time, degradations *[]observability.ReplayDegradation) error {
	if s.projections == nil {
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionMissing,
			Message: "task-graph replay projection store is not configured",
		})
		return nil
	}
	build, buildOK, err := s.projections.LoadBuild(scope)
	if err != nil {
		return err
	}
	projectionFound := false
	if projection, ok, err := s.projections.LoadTaskGraphProjection(scope.ScopeID); err != nil {
		return err
	} else if ok {
		projectionFound = true
		view.ProjectionStatus = string(projection.ProjectionStatus)
		view.ProjectionVersion = projection.SchemaVersion
		if err := applySummaryProjection(view, projection.SummaryJSON, projection.ExplanationJSON); err != nil {
			return err
		}
		applyProjectionFreshnessDegradation(projection.ProjectionFreshness, authoritativeFreshness, degradations)
	}
	nodes, edges, err := s.projections.ListProvenance(scope)
	if err != nil {
		return err
	}
	view.Provenance = convertProvenance(scope, nodes, edges)
	if execAttrs, err := s.projections.ListExecutionAttributions(scope); err != nil {
		return err
	} else {
		view.ExecutionAttributions = convertExecutionAttributions(execAttrs)
	}
	if failureAttrs, err := s.projections.ListFailureAttributions(scope); err != nil {
		return err
	} else {
		view.FailureAttributions = convertFailureAttributions(failureAttrs)
	}
	if !buildOK {
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionMissing,
			Message: "task-graph replay projection build metadata is missing",
		})
		return nil
	}
	if !projectionFound {
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionMissing,
			Message: "task-graph replay projection rows are missing",
		})
	}
	applyBuildDegradations(build, degradations)
	return nil
}

func applySummaryProjection(view *observability.ReplayView, summaryJSON string, explanationJSON string) error {
	if strings.TrimSpace(summaryJSON) != "" {
		if err := json.Unmarshal([]byte(summaryJSON), &view.Summary); err != nil {
			return err
		}
	}
	if strings.TrimSpace(explanationJSON) != "" {
		if err := json.Unmarshal([]byte(explanationJSON), &view.Explanation); err != nil {
			return err
		}
	}
	return nil
}

func applyBuildDegradations(build ReplayProjectionBuildRecord, degradations *[]observability.ReplayDegradation) {
	if build.SchemaVersion < ReplayProjectionSchemaVersion {
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionStale,
			Message: "replay projection schema version is older than the current runtime replay schema",
		})
	}
	switch build.Status {
	case ReplayProjectionStatusPartial:
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionIncomplete,
			Message: "replay projection is only partially available",
		})
	case ReplayProjectionStatusStale:
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionStale,
			Message: "replay projection is stale relative to durable runtime truth",
		})
	}
	for _, reason := range build.DegradationReasons {
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionIncomplete,
			Message: reason,
		})
	}
}

func applyProjectionFreshnessDegradation(projectionFreshness time.Time, authoritativeFreshness time.Time, degradations *[]observability.ReplayDegradation) {
	if projectionFreshness.IsZero() || authoritativeFreshness.IsZero() {
		return
	}
	if projectionFreshness.Before(authoritativeFreshness) {
		*degradations = append(*degradations, observability.ReplayDegradation{
			Reason:  observability.ReplayDegradationProjectionStale,
			Message: "replay projection freshness is older than authoritative runtime truth",
		})
	}
}

func convertProvenance(scope ReplayProjectionScope, nodes []ProvenanceNodeRecord, edges []ProvenanceEdgeRecord) observability.ProvenanceGraph {
	graph := observability.ProvenanceGraph{
		Scope: observability.ReplayScope{Kind: string(scope.ScopeKind), ID: scope.ScopeID},
		Nodes: make([]observability.ProvenanceNode, 0, len(nodes)),
		Edges: make([]observability.ProvenanceEdge, 0, len(edges)),
	}
	for _, item := range nodes {
		graph.Nodes = append(graph.Nodes, observability.ProvenanceNode{
			ID:         item.NodeID,
			Type:       item.NodeType,
			RefID:      item.RefID,
			Label:      item.Label,
			Summary:    item.Summary,
			Attributes: parseStringMap(item.AttributesJSON),
		})
	}
	for _, item := range edges {
		graph.Edges = append(graph.Edges, observability.ProvenanceEdge{
			ID:         item.EdgeID,
			FromNodeID: item.FromNodeID,
			ToNodeID:   item.ToNodeID,
			Type:       item.EdgeType,
			Reason:     item.Reason,
			Attributes: parseStringMap(item.AttributesJSON),
		})
	}
	return graph
}

func convertExecutionAttributions(items []ExecutionAttributionRecord) []observability.ExecutionAttribution {
	result := make([]observability.ExecutionAttribution, 0, len(items))
	for _, item := range items {
		result = append(result, observability.ExecutionAttribution{
			ExecutionID: item.ExecutionID,
			Category:    item.Category,
			Summary:     item.Summary,
			SourceRefs:  parseStringList(item.SourceRefsJSON),
			Details:     parseStringMap(item.DetailsJSON),
		})
	}
	return result
}

func convertFailureAttributions(items []FailureAttributionRecord) []observability.FailureAttribution {
	result := make([]observability.FailureAttribution, 0, len(items))
	for _, item := range items {
		result = append(result, observability.FailureAttribution{
			FailureCategory: item.FailureCategory,
			ReasonCode:      item.ReasonCode,
			Summary:         item.Summary,
			RelatedKind:     item.RelatedKind,
			RelatedID:       item.RelatedID,
			SourceRefs:      parseStringList(item.SourceRefsJSON),
			Details:         parseStringMap(item.DetailsJSON),
		})
	}
	return result
}

func parseStringList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{raw}
	}
	return values
}

func parseStringMap(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	result := make(map[string]string)
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]string{"raw": raw}
	}
	return result
}

func (s *ReplayQueryService) artifactsByWorkflow(workflowID string) ([]observability.ReplayArtifactRef, error) {
	if s.artifacts == nil {
		return nil, nil
	}
	items, err := s.artifacts.ListArtifactsByWorkflow(workflowID)
	if err != nil {
		return nil, err
	}
	return convertArtifactRefs(items), nil
}

func (s *ReplayQueryService) artifactsByGraph(view TaskGraphView) ([]observability.ReplayArtifactRef, error) {
	if len(view.Artifacts) > 0 {
		result := make([]observability.ReplayArtifactRef, 0, len(view.Artifacts))
		for _, artifact := range view.Artifacts {
			result = append(result, observability.ReplayArtifactRef{
				ID:         artifact.ID,
				Kind:       artifact.Kind,
				WorkflowID: artifact.WorkflowID,
				TaskID:     artifact.TaskID,
				Location:   artifact.StorageRef,
				Summary:    artifact.Summary,
			})
		}
		return result, nil
	}
	return nil, nil
}

func convertArtifactRefs(items []reporting.WorkflowArtifact) []observability.ReplayArtifactRef {
	result := make([]observability.ReplayArtifactRef, 0, len(items))
	for _, artifact := range items {
		result = append(result, observability.ReplayArtifactRef{
			ID:         artifact.ID,
			Kind:       string(artifact.Kind),
			WorkflowID: artifact.WorkflowID,
			TaskID:     artifact.TaskID,
			Location:   artifact.Ref.Location,
			Summary:    artifact.Ref.Summary,
			CreatedAt:  artifact.CreatedAt,
		})
	}
	return result
}

func pendingApprovalID(item *ApprovalStateRecord) string {
	if item == nil {
		return ""
	}
	return item.ApprovalID
}

func taskIDsFromRecords(records []FollowUpTaskRecord) []string {
	result := make([]string, 0, len(records))
	for _, item := range records {
		result = append(result, item.Task.ID)
	}
	return result
}

func executionIDs(records []TaskExecutionRecord) []string {
	result := make([]string, 0, len(records))
	for _, item := range records {
		result = append(result, item.ExecutionID)
	}
	return result
}

func summarizeGraphState(view TaskGraphView) string {
	if view.PendingApproval != nil {
		return string(WorkflowStateWaitingApproval)
	}
	failed := false
	executing := false
	for _, item := range view.Snapshot.RegisteredTasks {
		switch item.Status {
		case TaskQueueStatusFailed:
			failed = true
		case TaskQueueStatusExecuting:
			executing = true
		}
	}
	switch {
	case failed:
		return string(WorkflowStateFailed)
	case executing:
		return string(WorkflowStateActing)
	default:
		return string(WorkflowStateCompleted)
	}
}

func childWorkflowSummary(executions []TaskExecutionRecord) []string {
	result := make([]string, 0, len(executions))
	for _, item := range executions {
		result = append(result, fmt.Sprintf("%s:%s:%s", item.TaskID, item.Intent, item.Status))
	}
	return result
}

func bestEffortWorkflowProvenance(record WorkflowRunRecord, artifacts []observability.ReplayArtifactRef) observability.ProvenanceGraph {
	nodes := []observability.ProvenanceNode{{
		ID:      "workflow:" + record.WorkflowID,
		Type:    "workflow",
		RefID:   record.WorkflowID,
		Label:   record.WorkflowID,
		Summary: firstNonEmpty(record.Summary, string(record.RuntimeState)),
	}}
	edges := make([]observability.ProvenanceEdge, 0)
	if record.TaskID != "" {
		nodes = append(nodes, observability.ProvenanceNode{
			ID:    "task:" + record.TaskID,
			Type:  "task",
			RefID: record.TaskID,
			Label: record.TaskID,
		})
		edges = append(edges, observability.ProvenanceEdge{
			ID:         "workflow-task:" + record.WorkflowID + ":" + record.TaskID,
			FromNodeID: "workflow:" + record.WorkflowID,
			ToNodeID:   "task:" + record.TaskID,
			Type:       "executed_as",
		})
	}
	if record.TaskGraphID != "" {
		nodes = append(nodes, observability.ProvenanceNode{
			ID:    "task_graph:" + record.TaskGraphID,
			Type:  "task_graph",
			RefID: record.TaskGraphID,
			Label: record.TaskGraphID,
		})
		edges = append(edges, observability.ProvenanceEdge{
			ID:         "workflow-graph:" + record.WorkflowID + ":" + record.TaskGraphID,
			FromNodeID: "workflow:" + record.WorkflowID,
			ToNodeID:   "task_graph:" + record.TaskGraphID,
			Type:       "produced_task_graph",
		})
	}
	if record.ApprovalID != "" {
		nodes = append(nodes, observability.ProvenanceNode{
			ID:    "approval:" + record.ApprovalID,
			Type:  "approval",
			RefID: record.ApprovalID,
			Label: record.ApprovalID,
		})
		edges = append(edges, observability.ProvenanceEdge{
			ID:         "workflow-approval:" + record.WorkflowID + ":" + record.ApprovalID,
			FromNodeID: "workflow:" + record.WorkflowID,
			ToNodeID:   "approval:" + record.ApprovalID,
			Type:       "requested_approval",
		})
	}
	for _, artifact := range artifacts {
		nodeID := "artifact:" + artifact.ID
		nodes = append(nodes, observability.ProvenanceNode{
			ID:      nodeID,
			Type:    "artifact",
			RefID:   artifact.ID,
			Label:   artifact.Kind,
			Summary: artifact.Summary,
		})
		edges = append(edges, observability.ProvenanceEdge{
			ID:         "workflow-artifact:" + record.WorkflowID + ":" + artifact.ID,
			FromNodeID: "workflow:" + record.WorkflowID,
			ToNodeID:   nodeID,
			Type:       "produced_artifact",
		})
	}
	return observability.ProvenanceGraph{
		Scope: observability.ReplayScope{Kind: string(ReplayScopeWorkflow), ID: record.WorkflowID},
		Nodes: nodes,
		Edges: edges,
	}
}

func bestEffortTaskGraphProvenance(view TaskGraphView) observability.ProvenanceGraph {
	graphNodeID := "task_graph:" + view.Snapshot.Graph.GraphID
	nodes := []observability.ProvenanceNode{{
		ID:      graphNodeID,
		Type:    "task_graph",
		RefID:   view.Snapshot.Graph.GraphID,
		Label:   view.Snapshot.Graph.GraphID,
		Summary: firstNonEmpty(view.Snapshot.Graph.GenerationSummary, string(view.Snapshot.Graph.TriggerSource)),
	}}
	edges := make([]observability.ProvenanceEdge, 0)
	if view.Snapshot.Graph.ParentWorkflowID != "" {
		nodes = append(nodes, observability.ProvenanceNode{
			ID:    "workflow:" + view.Snapshot.Graph.ParentWorkflowID,
			Type:  "workflow",
			RefID: view.Snapshot.Graph.ParentWorkflowID,
			Label: view.Snapshot.Graph.ParentWorkflowID,
		})
		edges = append(edges, observability.ProvenanceEdge{
			ID:         "parent-workflow:" + view.Snapshot.Graph.ParentWorkflowID + ":" + view.Snapshot.Graph.GraphID,
			FromNodeID: "workflow:" + view.Snapshot.Graph.ParentWorkflowID,
			ToNodeID:   graphNodeID,
			Type:       "generated_task_graph",
		})
	}
	for _, task := range view.Snapshot.RegisteredTasks {
		taskNodeID := "task:" + task.Task.ID
		nodes = append(nodes, observability.ProvenanceNode{
			ID:      taskNodeID,
			Type:    "task",
			RefID:   task.Task.ID,
			Label:   string(task.Task.UserIntentType),
			Summary: string(task.Status),
			Attributes: map[string]string{
				"required_capability":       task.RequiredCapability,
				"missing_capability_reason": task.MissingCapabilityReason,
			},
		})
		edges = append(edges, observability.ProvenanceEdge{
			ID:         "graph-task:" + view.Snapshot.Graph.GraphID + ":" + task.Task.ID,
			FromNodeID: graphNodeID,
			ToNodeID:   taskNodeID,
			Type:       "generated_task",
		})
	}
	for _, execution := range view.Executions {
		execNodeID := "execution:" + execution.ExecutionID
		childWorkflowNodeID := "workflow:" + execution.WorkflowID
		nodes = append(nodes, observability.ProvenanceNode{
			ID:      execNodeID,
			Type:    "execution",
			RefID:   execution.ExecutionID,
			Label:   execution.WorkflowID,
			Summary: string(execution.Status),
		})
		if execution.WorkflowID != "" {
			nodes = append(nodes, observability.ProvenanceNode{
				ID:      childWorkflowNodeID,
				Type:    "workflow",
				RefID:   execution.WorkflowID,
				Label:   execution.WorkflowID,
				Summary: fmt.Sprintf("child workflow for %s", execution.TaskID),
			})
		}
		edges = append(edges, observability.ProvenanceEdge{
			ID:         "task-execution:" + execution.TaskID + ":" + execution.ExecutionID,
			FromNodeID: "task:" + execution.TaskID,
			ToNodeID:   execNodeID,
			Type:       "executed_as",
		})
		if execution.WorkflowID != "" {
			edges = append(edges, observability.ProvenanceEdge{
				ID:         "execution-workflow:" + execution.ExecutionID + ":" + execution.WorkflowID,
				FromNodeID: execNodeID,
				ToNodeID:   childWorkflowNodeID,
				Type:       "triggered_child_workflow",
			})
		}
		if execution.UpdatedStateSnapshotRef != "" {
			stateNodeID := "state:" + execution.UpdatedStateSnapshotRef
			nodes = append(nodes, observability.ProvenanceNode{
				ID:    stateNodeID,
				Type:  "state_snapshot",
				RefID: execution.UpdatedStateSnapshotRef,
				Label: execution.UpdatedStateSnapshotRef,
			})
			edges = append(edges, observability.ProvenanceEdge{
				ID:         "execution-state:" + execution.ExecutionID + ":" + execution.UpdatedStateSnapshotRef,
				FromNodeID: execNodeID,
				ToNodeID:   stateNodeID,
				Type:       "committed_state",
			})
		}
		if execution.ApprovalID != "" {
			approvalNodeID := "approval:" + execution.ApprovalID
			nodes = append(nodes, observability.ProvenanceNode{
				ID:    approvalNodeID,
				Type:  "approval",
				RefID: execution.ApprovalID,
				Label: execution.ApprovalID,
			})
			edges = append(edges, observability.ProvenanceEdge{
				ID:         "execution-approval:" + execution.ExecutionID + ":" + execution.ApprovalID,
				FromNodeID: execNodeID,
				ToNodeID:   approvalNodeID,
				Type:       "requested_approval",
			})
		}
		for _, artifactID := range execution.ArtifactIDs {
			artifactNodeID := "artifact:" + artifactID
			nodes = append(nodes, observability.ProvenanceNode{
				ID:    artifactNodeID,
				Type:  "artifact",
				RefID: artifactID,
				Label: artifactID,
			})
			edges = append(edges, observability.ProvenanceEdge{
				ID:         "execution-artifact:" + execution.ExecutionID + ":" + artifactID,
				FromNodeID: execNodeID,
				ToNodeID:   artifactNodeID,
				Type:       "produced_artifact",
			})
		}
	}
	for _, action := range view.Actions {
		actionNodeID := "operator_action:" + action.ActionID
		nodes = append(nodes, observability.ProvenanceNode{
			ID:      actionNodeID,
			Type:    "operator_action",
			RefID:   action.ActionID,
			Label:   string(action.ActionType),
			Summary: string(action.Status),
		})
		if action.ApprovalID != "" {
			edges = append(edges, observability.ProvenanceEdge{
				ID:         "approval-action:" + action.ApprovalID + ":" + action.ActionID,
				FromNodeID: "approval:" + action.ApprovalID,
				ToNodeID:   actionNodeID,
				Type:       "resolved_by_operator",
			})
		}
	}
	return observability.ProvenanceGraph{
		Scope: observability.ReplayScope{Kind: string(ReplayScopeTaskGraph), ID: view.Snapshot.Graph.GraphID},
		Nodes: dedupeNodes(nodes),
		Edges: dedupeEdges(edges),
	}
}

func bestEffortExecutionAttributions(records []TaskExecutionRecord) []observability.ExecutionAttribution {
	result := make([]observability.ExecutionAttribution, 0, len(records))
	for _, item := range records {
		result = append(result, observability.ExecutionAttribution{
			ExecutionID: item.ExecutionID,
			Category:    "task_execution",
			Summary:     fmt.Sprintf("%s ended as %s", item.TaskID, item.Status),
			SourceRefs: []string{
				item.TaskID,
				item.WorkflowID,
			},
			Details: map[string]string{
				"intent":                 string(item.Intent),
				"status":                 string(item.Status),
				"failure_category":       string(item.FailureCategory),
				"failure_summary":        item.FailureSummary,
				"last_recovery_strategy": string(item.LastRecoveryStrategy),
			},
		})
	}
	return result
}

func bestEffortGraphFailureAttributions(view TaskGraphView) []observability.FailureAttribution {
	result := make([]observability.FailureAttribution, 0)
	for _, task := range view.Snapshot.RegisteredTasks {
		switch task.Status {
		case TaskQueueStatusQueuedPendingCapability:
			result = append(result, observability.FailureAttribution{
				FailureCategory: "capability_blocked",
				Summary:         firstNonEmpty(task.MissingCapabilityReason, "task is blocked on a missing capability"),
				RelatedKind:     "task",
				RelatedID:       task.Task.ID,
			})
		case TaskQueueStatusDependencyBlocked:
			result = append(result, observability.FailureAttribution{
				FailureCategory: "dependency_blocked",
				Summary:         strings.Join(task.BlockingReasons, "; "),
				RelatedKind:     "task",
				RelatedID:       task.Task.ID,
			})
		case TaskQueueStatusDeferred:
			result = append(result, observability.FailureAttribution{
				FailureCategory: "deferred",
				Summary:         strings.Join(task.BlockingReasons, "; "),
				RelatedKind:     "task",
				RelatedID:       task.Task.ID,
			})
		case TaskQueueStatusFailed:
			result = append(result, observability.FailureAttribution{
				FailureCategory: "execution_failed",
				Summary:         strings.Join(task.BlockingReasons, "; "),
				RelatedKind:     "task",
				RelatedID:       task.Task.ID,
			})
		}
	}
	for _, execution := range view.Executions {
		if execution.FailureCategory == "" {
			continue
		}
		result = append(result, observability.FailureAttribution{
			FailureCategory: string(execution.FailureCategory),
			Summary:         firstNonEmpty(execution.FailureSummary, "execution failed"),
			RelatedKind:     "execution",
			RelatedID:       execution.ExecutionID,
		})
	}
	return result
}

func filterExecutionIDsForTask(ids []string, records []TaskExecutionRecord, taskID string) []string {
	result := make([]string, 0)
	for _, record := range records {
		if record.TaskID == taskID {
			result = append(result, record.ExecutionID)
		}
	}
	return result
}

func filterChildWorkflowSummaryForTask(items []string, taskID string) []string {
	result := make([]string, 0)
	for _, item := range items {
		if strings.HasPrefix(item, taskID+":") {
			result = append(result, item)
		}
	}
	return result
}

func filterExecutionAttributionsForTask(items []observability.ExecutionAttribution, records []TaskExecutionRecord, taskID string) []observability.ExecutionAttribution {
	allowed := make(map[string]struct{})
	for _, record := range records {
		if record.TaskID == taskID {
			allowed[record.ExecutionID] = struct{}{}
		}
	}
	result := make([]observability.ExecutionAttribution, 0)
	for _, item := range items {
		if _, ok := allowed[item.ExecutionID]; ok {
			result = append(result, item)
		}
	}
	return result
}

func filterExecutionAttributionsForExecution(items []observability.ExecutionAttribution, executionID string) []observability.ExecutionAttribution {
	result := make([]observability.ExecutionAttribution, 0)
	for _, item := range items {
		if item.ExecutionID == executionID {
			result = append(result, item)
		}
	}
	return result
}

func filterFailureAttributionsForTask(items []observability.FailureAttribution, taskID string) []observability.FailureAttribution {
	result := make([]observability.FailureAttribution, 0)
	for _, item := range items {
		if item.RelatedID == taskID {
			result = append(result, item)
		}
	}
	return result
}

func filterProvenanceForTask(graph observability.ProvenanceGraph, taskID string) observability.ProvenanceGraph {
	if taskID == "" {
		return graph
	}
	return filterProvenance(graph, func(node observability.ProvenanceNode) bool {
		return node.RefID == taskID || node.ID == "task:"+taskID || node.Type == "task_graph" || node.Type == "workflow" || strings.Contains(node.Summary, taskID)
	})
}

func filterProvenanceForExecution(graph observability.ProvenanceGraph, executionID string) observability.ProvenanceGraph {
	if executionID == "" {
		return graph
	}
	return filterProvenance(graph, func(node observability.ProvenanceNode) bool {
		return node.RefID == executionID || node.ID == "execution:"+executionID || node.Type == "task_graph" || node.Type == "workflow" || node.Type == "task"
	})
}

func filterProvenanceForApproval(graph observability.ProvenanceGraph, approvalID string) observability.ProvenanceGraph {
	if approvalID == "" {
		return graph
	}
	return filterProvenance(graph, func(node observability.ProvenanceNode) bool {
		return node.RefID == approvalID || node.ID == "approval:"+approvalID || node.Type == "task_graph" || node.Type == "workflow" || node.Type == "task" || node.Type == "operator_action"
	})
}

func filterProvenance(graph observability.ProvenanceGraph, keep func(node observability.ProvenanceNode) bool) observability.ProvenanceGraph {
	allowed := make(map[string]struct{})
	nodes := make([]observability.ProvenanceNode, 0)
	for _, node := range graph.Nodes {
		if keep(node) {
			nodes = append(nodes, node)
			allowed[node.ID] = struct{}{}
		}
	}
	edges := make([]observability.ProvenanceEdge, 0)
	for _, edge := range graph.Edges {
		_, fromOK := allowed[edge.FromNodeID]
		_, toOK := allowed[edge.ToNodeID]
		if fromOK && toOK {
			edges = append(edges, edge)
		}
	}
	graph.Nodes = nodes
	graph.Edges = edges
	return graph
}

func dedupeNodes(nodes []observability.ProvenanceNode) []observability.ProvenanceNode {
	seen := make(map[string]struct{})
	result := make([]observability.ProvenanceNode, 0, len(nodes))
	for _, node := range nodes {
		if _, ok := seen[node.ID]; ok {
			continue
		}
		seen[node.ID] = struct{}{}
		result = append(result, node)
	}
	return result
}

func dedupeEdges(edges []observability.ProvenanceEdge) []observability.ProvenanceEdge {
	seen := make(map[string]struct{})
	result := make([]observability.ProvenanceEdge, 0, len(edges))
	for _, edge := range edges {
		if _, ok := seen[edge.ID]; ok {
			continue
		}
		seen[edge.ID] = struct{}{}
		result = append(result, edge)
	}
	return result
}
