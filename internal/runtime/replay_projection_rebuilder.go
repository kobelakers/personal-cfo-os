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

type ReplayProjectionRebuilder struct {
	service      *Service
	workflowRuns WorkflowRunStore
	projections  ReplayProjectionStore
	artifacts    ArtifactMetadataStore
	replay       ReplayStore
	now          func() time.Time
}

func NewReplayProjectionRebuilder(
	service *Service,
	workflowRuns WorkflowRunStore,
	projections ReplayProjectionStore,
	artifacts ArtifactMetadataStore,
	replay ReplayStore,
	now func() time.Time,
) *ReplayProjectionRebuilder {
	return &ReplayProjectionRebuilder{
		service:      service,
		workflowRuns: workflowRuns,
		projections:  projections,
		artifacts:    artifacts,
		replay:       replay,
		now:          now,
	}
}

func (r *ReplayProjectionRebuilder) RebuildWorkflow(_ context.Context, workflowID string) (ReplayProjectionBuildRecord, error) {
	if r.workflowRuns == nil || r.projections == nil {
		return ReplayProjectionBuildRecord{}, fmt.Errorf("workflow run store and replay projection store are required")
	}
	record, ok, err := r.workflowRuns.Load(workflowID)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	if !ok {
		return ReplayProjectionBuildRecord{}, &NotFoundError{Resource: "workflow_run", ID: workflowID}
	}
	artifacts, err := r.listArtifactsByWorkflow(workflowID)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	events, err := r.listReplayEventsByWorkflow(workflowID)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}

	summary := observability.ReplaySummary{FinalState: string(record.RuntimeState)}
	explanation := observability.ReplayExplanation{}
	provenance := bestEffortWorkflowProvenance(record, convertArtifactRefs(artifacts))
	failureAttrs := make([]observability.FailureAttribution, 0)
	if record.FailureCategory != "" {
		failureAttrs = append(failureAttrs, observability.FailureAttribution{
			FailureCategory: string(record.FailureCategory),
			Summary:         firstNonEmpty(record.FailureSummary, "workflow ended with failure state"),
			RelatedKind:     "workflow",
			RelatedID:       record.WorkflowID,
		})
	}

	status := ReplayProjectionStatusComplete
	degradationReasons := make([]string, 0)
	bundleArtifactID := ""
	summaryArtifactID := findArtifactIDByKind(artifacts, reporting.ArtifactKindReplaySummary)

	if bundle, ok, artifactID, err := r.loadWorkflowReplayBundle(artifacts); err != nil {
		return ReplayProjectionBuildRecord{}, err
	} else if ok {
		bundleArtifactID = artifactID
		summary = observability.BuildReplaySummaryFromTrace(bundle.Trace, string(record.RuntimeState))
		explanation = observability.BuildReplayExplanationFromTrace(bundle.Trace, string(record.RuntimeState))
		provenance = workflowProvenanceFromTrace(record, convertArtifactRefs(artifacts), bundle.Trace)
		failureAttrs = workflowFailureAttributionsFromTrace(record, bundle.Trace)
	} else {
		status = ReplayProjectionStatusPartial
		degradationReasons = append(degradationReasons, "workflow replay bundle artifact is missing; projection was rebuilt from authoritative runtime truth only")
	}
	if summary.GoalSummary == "" {
		summary.GoalSummary = firstNonEmpty(record.Intent, record.Summary)
	}

	summaryJSON, err := marshalJSON(summary)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	explanationJSON, err := marshalJSON(explanation)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	compareInputJSON, err := marshalJSON(summary)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	projection := WorkflowReplayProjection{
		WorkflowID:          record.WorkflowID,
		TaskID:              record.TaskID,
		Intent:              record.Intent,
		RuntimeState:        record.RuntimeState,
		FailureCategory:     record.FailureCategory,
		ApprovalID:          record.ApprovalID,
		BundleArtifactID:    bundleArtifactID,
		SummaryArtifactID:   summaryArtifactID,
		ProjectionStatus:    status,
		SchemaVersion:       ReplayProjectionSchemaVersion,
		DegradationReasons:  degradationReasons,
		SummaryJSON:         summaryJSON,
		ExplanationJSON:     explanationJSON,
		CompareInputJSON:    compareInputJSON,
		UpdatedAt:           r.nowUTC(),
		ProjectionFreshness: record.UpdatedAt,
	}
	scope := ReplayProjectionScope{ScopeKind: ReplayScopeWorkflow, ScopeID: workflowID}
	build := ReplayProjectionBuildRecord{
		ScopeKind:           scope.ScopeKind,
		ScopeID:             scope.ScopeID,
		SchemaVersion:       ReplayProjectionSchemaVersion,
		Status:              status,
		DegradationReasons:  append([]string{}, degradationReasons...),
		BuiltAt:             r.nowUTC(),
		SourceEventCount:    len(events),
		SourceArtifactCount: len(artifacts),
	}
	if err := r.projections.SaveWorkflowProjection(projection); err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	nodes, edges, err := provenanceRecords(scope, provenance)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	if err := r.projections.ReplaceProvenance(scope, nodes, edges); err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	if err := r.projections.ReplaceExecutionAttributions(scope, nil); err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	failureRecords, err := failureAttributionRecords(scope, failureAttrs)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	if err := r.projections.ReplaceFailureAttributions(scope, failureRecords); err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	if err := r.projections.SaveBuild(build); err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	return build, nil
}

func (r *ReplayProjectionRebuilder) RebuildTaskGraph(ctx context.Context, graphID string) (ReplayProjectionBuildRecord, error) {
	if r.service == nil || r.projections == nil {
		return ReplayProjectionBuildRecord{}, fmt.Errorf("runtime service and replay projection store are required")
	}
	query := NewQueryService(r.service)
	view, err := query.GetTaskGraph(ctx, graphID)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	events, err := r.listReplayEventsByGraph(graphID)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	status := ReplayProjectionStatusComplete
	degradationReasons := make([]string, 0)
	summary := observability.ReplaySummary{
		GoalSummary:          firstNonEmpty(view.Snapshot.Graph.GenerationSummary, string(view.Snapshot.Graph.TriggerSource)),
		ChildWorkflowSummary: childWorkflowSummary(view.Executions),
		FinalState:           summarizeGraphState(view),
	}
	if pending := pendingApprovalID(view.PendingApproval); pending != "" {
		summary.GovernanceSummary = []string{"approval_required"}
	}
	for _, task := range view.Snapshot.RegisteredTasks {
		switch task.Status {
		case TaskQueueStatusQueuedPendingCapability, TaskQueueStatusDependencyBlocked, TaskQueueStatusDeferred, TaskQueueStatusFailed:
			summary.ValidatorSummary = append(summary.ValidatorSummary, fmt.Sprintf("%s:%s", task.Task.ID, task.Status))
		}
	}
	explanation := observability.ReplayExplanation{
		WhyGeneratedTask: taskGenerationSummary(view.Snapshot.RegisteredTasks),
		WhyChildExecuted: childExecutionExplanation(view.Executions),
	}
	if view.PendingApproval != nil {
		explanation.WhyWaitingApproval = firstNonEmpty(view.PendingApproval.RequestedAction, "follow-up task is waiting for approval")
	}
	provenance := bestEffortTaskGraphProvenance(view)
	execAttrs := bestEffortExecutionAttributions(view.Executions)
	failureAttrs := bestEffortGraphFailureAttributions(view)
	bundleArtifactID := findArtifactMetaIDByKind(view.Artifacts, string(reporting.ArtifactKindReplayBundle))
	summaryArtifactID := findArtifactMetaIDByKind(view.Artifacts, string(reporting.ArtifactKindReplaySummary))
	if bundleArtifactID == "" {
		status = ReplayProjectionStatusPartial
		degradationReasons = append(degradationReasons, "task-graph replay bundle artifact is missing; projection was rebuilt from authoritative runtime truth only")
	}
	summaryJSON, err := marshalJSON(summary)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	explanationJSON, err := marshalJSON(explanation)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	compareInputJSON, err := marshalJSON(summary)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	projection := TaskGraphReplayProjection{
		GraphID:             graphID,
		ParentWorkflowID:    view.Snapshot.Graph.ParentWorkflowID,
		ParentTaskID:        view.Snapshot.Graph.ParentTaskID,
		RuntimeState:        WorkflowExecutionState(summary.FinalState),
		PendingApprovalID:   pendingApprovalID(view.PendingApproval),
		BundleArtifactID:    bundleArtifactID,
		SummaryArtifactID:   summaryArtifactID,
		ProjectionStatus:    status,
		SchemaVersion:       ReplayProjectionSchemaVersion,
		DegradationReasons:  degradationReasons,
		SummaryJSON:         summaryJSON,
		ExplanationJSON:     explanationJSON,
		CompareInputJSON:    compareInputJSON,
		UpdatedAt:           r.nowUTC(),
		ProjectionFreshness: freshestGraphTimestamp(view, events),
	}
	scope := ReplayProjectionScope{ScopeKind: ReplayScopeTaskGraph, ScopeID: graphID}
	build := ReplayProjectionBuildRecord{
		ScopeKind:           scope.ScopeKind,
		ScopeID:             scope.ScopeID,
		SchemaVersion:       ReplayProjectionSchemaVersion,
		Status:              status,
		DegradationReasons:  append([]string{}, degradationReasons...),
		BuiltAt:             r.nowUTC(),
		SourceEventCount:    len(events),
		SourceArtifactCount: len(view.Artifacts),
	}
	if err := r.projections.SaveTaskGraphProjection(projection); err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	nodes, edges, err := provenanceRecords(scope, provenance)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	if err := r.projections.ReplaceProvenance(scope, nodes, edges); err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	executionRecords, err := executionAttributionRecords(scope, execAttrs)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	if err := r.projections.ReplaceExecutionAttributions(scope, executionRecords); err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	failureRecords, err := failureAttributionRecords(scope, failureAttrs)
	if err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	if err := r.projections.ReplaceFailureAttributions(scope, failureRecords); err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	if err := r.projections.SaveBuild(build); err != nil {
		return ReplayProjectionBuildRecord{}, err
	}
	return build, nil
}

func (r *ReplayProjectionRebuilder) RebuildAll(ctx context.Context) ([]ReplayProjectionBuildRecord, error) {
	builds := make([]ReplayProjectionBuildRecord, 0)
	if r.workflowRuns != nil {
		runs, err := r.workflowRuns.List()
		if err != nil {
			return nil, err
		}
		for _, run := range runs {
			build, err := r.RebuildWorkflow(ctx, run.WorkflowID)
			if err != nil {
				return nil, err
			}
			builds = append(builds, build)
		}
	}
	if r.service != nil && r.service.runtime != nil && r.service.runtime.TaskGraphs != nil {
		graphs, err := r.service.runtime.TaskGraphs.List()
		if err != nil {
			return nil, err
		}
		for _, graph := range graphs {
			build, err := r.RebuildTaskGraph(ctx, graph.Graph.GraphID)
			if err != nil {
				return nil, err
			}
			builds = append(builds, build)
		}
	}
	return builds, nil
}

func (r *ReplayProjectionRebuilder) BackfillWorkflow(ctx context.Context, workflowID string) (ReplayProjectionBuildRecord, error) {
	return r.RebuildWorkflow(ctx, workflowID)
}

func (r *ReplayProjectionRebuilder) BackfillTaskGraph(ctx context.Context, graphID string) (ReplayProjectionBuildRecord, error) {
	return r.RebuildTaskGraph(ctx, graphID)
}

func (r *ReplayProjectionRebuilder) BackfillAll(ctx context.Context) ([]ReplayProjectionBuildRecord, error) {
	return r.RebuildAll(ctx)
}

func (r *ReplayProjectionRebuilder) listArtifactsByWorkflow(workflowID string) ([]reporting.WorkflowArtifact, error) {
	if r.artifacts == nil {
		return nil, nil
	}
	return r.artifacts.ListArtifactsByWorkflow(workflowID)
}

func (r *ReplayProjectionRebuilder) listReplayEventsByWorkflow(workflowID string) ([]ReplayEventRecord, error) {
	if r.replay == nil {
		return nil, nil
	}
	return r.replay.ListByWorkflow(workflowID)
}

func (r *ReplayProjectionRebuilder) listReplayEventsByGraph(graphID string) ([]ReplayEventRecord, error) {
	if r.replay == nil {
		return nil, nil
	}
	return r.replay.ListByGraph(graphID)
}

func (r *ReplayProjectionRebuilder) loadWorkflowReplayBundle(artifacts []reporting.WorkflowArtifact) (observability.ReplayBundle, bool, string, error) {
	if r.artifacts == nil {
		return observability.ReplayBundle{}, false, "", nil
	}
	artifactID := findArtifactIDByKind(artifacts, reporting.ArtifactKindReplayBundle)
	if artifactID == "" {
		return observability.ReplayBundle{}, false, "", nil
	}
	artifact, ok, err := r.artifacts.LoadArtifact(artifactID)
	if err != nil {
		return observability.ReplayBundle{}, false, "", err
	}
	if !ok || strings.TrimSpace(artifact.ContentJSON) == "" {
		return observability.ReplayBundle{}, false, "", nil
	}
	var bundle observability.ReplayBundle
	if err := json.Unmarshal([]byte(artifact.ContentJSON), &bundle); err != nil {
		return observability.ReplayBundle{}, false, "", err
	}
	return bundle, true, artifactID, nil
}

func provenanceRecords(scope ReplayProjectionScope, graph observability.ProvenanceGraph) ([]ProvenanceNodeRecord, []ProvenanceEdgeRecord, error) {
	nodes := make([]ProvenanceNodeRecord, 0, len(graph.Nodes))
	for _, item := range graph.Nodes {
		attrsJSON, err := marshalJSON(item.Attributes)
		if err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, ProvenanceNodeRecord{
			ScopeKind:      scope.ScopeKind,
			ScopeID:        scope.ScopeID,
			NodeID:         item.ID,
			NodeType:       item.Type,
			RefID:          item.RefID,
			Label:          item.Label,
			Summary:        item.Summary,
			AttributesJSON: attrsJSON,
		})
	}
	edges := make([]ProvenanceEdgeRecord, 0, len(graph.Edges))
	for _, item := range graph.Edges {
		attrsJSON, err := marshalJSON(item.Attributes)
		if err != nil {
			return nil, nil, err
		}
		edges = append(edges, ProvenanceEdgeRecord{
			ScopeKind:      scope.ScopeKind,
			ScopeID:        scope.ScopeID,
			EdgeID:         item.ID,
			FromNodeID:     item.FromNodeID,
			ToNodeID:       item.ToNodeID,
			EdgeType:       item.Type,
			Reason:         item.Reason,
			AttributesJSON: attrsJSON,
		})
	}
	return nodes, edges, nil
}

func executionAttributionRecords(scope ReplayProjectionScope, items []observability.ExecutionAttribution) ([]ExecutionAttributionRecord, error) {
	result := make([]ExecutionAttributionRecord, 0, len(items))
	for _, item := range items {
		sourceRefsJSON, err := marshalJSON(item.SourceRefs)
		if err != nil {
			return nil, err
		}
		detailsJSON, err := marshalJSON(item.Details)
		if err != nil {
			return nil, err
		}
		result = append(result, ExecutionAttributionRecord{
			ScopeKind:      scope.ScopeKind,
			ScopeID:        scope.ScopeID,
			ExecutionID:    item.ExecutionID,
			Category:       item.Category,
			Summary:        item.Summary,
			SourceRefsJSON: sourceRefsJSON,
			DetailsJSON:    detailsJSON,
		})
	}
	return result, nil
}

func failureAttributionRecords(scope ReplayProjectionScope, items []observability.FailureAttribution) ([]FailureAttributionRecord, error) {
	result := make([]FailureAttributionRecord, 0, len(items))
	for i, item := range items {
		sourceRefsJSON, err := marshalJSON(item.SourceRefs)
		if err != nil {
			return nil, err
		}
		detailsJSON, err := marshalJSON(item.Details)
		if err != nil {
			return nil, err
		}
		attributionID := fmt.Sprintf("%s:%d", scope.ScopeID, i)
		if item.RelatedID != "" {
			attributionID = fmt.Sprintf("%s:%s:%d", scope.ScopeID, item.RelatedID, i)
		}
		result = append(result, FailureAttributionRecord{
			ScopeKind:       scope.ScopeKind,
			ScopeID:         scope.ScopeID,
			AttributionID:   attributionID,
			FailureCategory: item.FailureCategory,
			ReasonCode:      item.ReasonCode,
			Summary:         item.Summary,
			RelatedKind:     item.RelatedKind,
			RelatedID:       item.RelatedID,
			SourceRefsJSON:  sourceRefsJSON,
			DetailsJSON:     detailsJSON,
		})
	}
	return result, nil
}

func workflowProvenanceFromTrace(record WorkflowRunRecord, artifacts []observability.ReplayArtifactRef, trace observability.WorkflowTraceDump) observability.ProvenanceGraph {
	graph := bestEffortWorkflowProvenance(record, artifacts)
	workflowNodeID := "workflow:" + record.WorkflowID
	for _, selection := range trace.MemorySelections {
		for _, memoryID := range selection.SelectedMemoryIDs {
			nodeID := "memory:" + memoryID
			graph.Nodes = append(graph.Nodes, observability.ProvenanceNode{
				ID:    nodeID,
				Type:  "memory",
				RefID: memoryID,
				Label: memoryID,
			})
			graph.Edges = append(graph.Edges, observability.ProvenanceEdge{
				ID:         "workflow-memory-selected:" + record.WorkflowID + ":" + memoryID,
				FromNodeID: workflowNodeID,
				ToNodeID:   nodeID,
				Type:       "selected_memory",
			})
		}
		for _, memoryID := range selection.RejectedMemoryIDs {
			nodeID := "memory:" + memoryID
			graph.Nodes = append(graph.Nodes, observability.ProvenanceNode{
				ID:    nodeID,
				Type:  "memory",
				RefID: memoryID,
				Label: memoryID,
			})
			graph.Edges = append(graph.Edges, observability.ProvenanceEdge{
				ID:         "workflow-memory-rejected:" + record.WorkflowID + ":" + memoryID,
				FromNodeID: workflowNodeID,
				ToNodeID:   nodeID,
				Type:       "rejected_memory",
			})
		}
	}
	addValidatorNodes := func(prefix string, items []observability.PolicyDecisionRecord) {
		for i, item := range items {
			nodeID := fmt.Sprintf("%s:%s:%d", prefix, record.WorkflowID, i)
			graph.Nodes = append(graph.Nodes, observability.ProvenanceNode{
				ID:      nodeID,
				Type:    prefix,
				Label:   firstNonEmpty(item.Action, prefix),
				Summary: item.Reason,
			})
			graph.Edges = append(graph.Edges, observability.ProvenanceEdge{
				ID:         fmt.Sprintf("workflow-%s:%s:%d", prefix, record.WorkflowID, i),
				FromNodeID: workflowNodeID,
				ToNodeID:   nodeID,
				Type:       "blocked_by_policy",
			})
		}
	}
	for i, item := range trace.GroundingVerdicts {
		nodeID := fmt.Sprintf("validator:%s:grounding:%d", record.WorkflowID, i)
		graph.Nodes = append(graph.Nodes, observability.ProvenanceNode{
			ID:      nodeID,
			Type:    "validator_verdict",
			Label:   item.Validator,
			Summary: item.Message,
		})
		graph.Edges = append(graph.Edges, observability.ProvenanceEdge{
			ID:         fmt.Sprintf("workflow-validator:%s:grounding:%d", record.WorkflowID, i),
			FromNodeID: workflowNodeID,
			ToNodeID:   nodeID,
			Type:       "validated_by",
		})
	}
	for i, item := range trace.NumericValidationVerdicts {
		nodeID := fmt.Sprintf("validator:%s:numeric:%d", record.WorkflowID, i)
		graph.Nodes = append(graph.Nodes, observability.ProvenanceNode{
			ID:      nodeID,
			Type:    "validator_verdict",
			Label:   item.Validator,
			Summary: item.Message,
		})
		graph.Edges = append(graph.Edges, observability.ProvenanceEdge{
			ID:         fmt.Sprintf("workflow-validator:%s:numeric:%d", record.WorkflowID, i),
			FromNodeID: workflowNodeID,
			ToNodeID:   nodeID,
			Type:       "validated_by",
		})
	}
	for i, item := range trace.BusinessRuleVerdicts {
		nodeID := fmt.Sprintf("validator:%s:business:%d", record.WorkflowID, i)
		graph.Nodes = append(graph.Nodes, observability.ProvenanceNode{
			ID:      nodeID,
			Type:    "validator_verdict",
			Label:   item.Validator,
			Summary: item.Message,
		})
		graph.Edges = append(graph.Edges, observability.ProvenanceEdge{
			ID:         fmt.Sprintf("workflow-validator:%s:business:%d", record.WorkflowID, i),
			FromNodeID: workflowNodeID,
			ToNodeID:   nodeID,
			Type:       "validated_by",
		})
	}
	addValidatorNodes("policy_decision", trace.PolicyDecisions)
	return observability.ProvenanceGraph{
		Scope: graph.Scope,
		Nodes: dedupeNodes(graph.Nodes),
		Edges: dedupeEdges(graph.Edges),
	}
}

func workflowFailureAttributionsFromTrace(record WorkflowRunRecord, trace observability.WorkflowTraceDump) []observability.FailureAttribution {
	result := make([]observability.FailureAttribution, 0)
	for _, item := range trace.GroundingVerdicts {
		if string(item.Status) != "fail" && string(item.Status) != "needs_replan" {
			continue
		}
		result = append(result, observability.FailureAttribution{
			FailureCategory: "grounding",
			Summary:         fmt.Sprintf("%s: %s", item.Validator, item.Message),
			RelatedKind:     "workflow",
			RelatedID:       record.WorkflowID,
		})
	}
	for _, item := range trace.NumericValidationVerdicts {
		if string(item.Status) != "fail" && string(item.Status) != "needs_replan" {
			continue
		}
		result = append(result, observability.FailureAttribution{
			FailureCategory: "numeric",
			Summary:         fmt.Sprintf("%s: %s", item.Validator, item.Message),
			RelatedKind:     "workflow",
			RelatedID:       record.WorkflowID,
		})
	}
	for _, item := range trace.BusinessRuleVerdicts {
		if string(item.Status) != "fail" && string(item.Status) != "needs_replan" {
			continue
		}
		result = append(result, observability.FailureAttribution{
			FailureCategory: "business",
			Summary:         fmt.Sprintf("%s: %s", item.Validator, item.Message),
			RelatedKind:     "workflow",
			RelatedID:       record.WorkflowID,
		})
	}
	if len(result) == 0 && record.FailureCategory != "" {
		result = append(result, observability.FailureAttribution{
			FailureCategory: string(record.FailureCategory),
			Summary:         firstNonEmpty(record.FailureSummary, "workflow ended with failure state"),
			RelatedKind:     "workflow",
			RelatedID:       record.WorkflowID,
		})
	}
	return result
}

func taskGenerationSummary(tasks []FollowUpTaskRecord) []string {
	result := make([]string, 0, len(tasks))
	for _, item := range tasks {
		result = append(result, fmt.Sprintf("%s:%s", item.Task.ID, item.Task.UserIntentType))
	}
	return result
}

func childExecutionExplanation(executions []TaskExecutionRecord) []string {
	result := make([]string, 0, len(executions))
	for _, item := range executions {
		result = append(result, fmt.Sprintf("%s executed child workflow %s with status %s", item.TaskID, item.WorkflowID, item.Status))
	}
	return result
}

func freshestGraphTimestamp(view TaskGraphView, events []ReplayEventRecord) time.Time {
	freshest := time.Time{}
	for _, execution := range view.Executions {
		if execution.LastTransitionAt.After(freshest) {
			freshest = execution.LastTransitionAt
		}
	}
	if view.PendingApproval != nil && view.PendingApproval.RequestedAt.After(freshest) {
		freshest = view.PendingApproval.RequestedAt
	}
	for _, action := range view.Actions {
		if action.RequestedAt.After(freshest) {
			freshest = action.RequestedAt
		}
		if action.AppliedAt != nil && action.AppliedAt.After(freshest) {
			freshest = *action.AppliedAt
		}
	}
	for _, event := range events {
		if event.OccurredAt.After(freshest) {
			freshest = event.OccurredAt
		}
	}
	if freshest.IsZero() {
		return time.Now().UTC()
	}
	return freshest.UTC()
}

func findArtifactIDByKind(items []reporting.WorkflowArtifact, kind reporting.ArtifactKind) string {
	for _, item := range items {
		if item.Kind == kind {
			return item.ID
		}
	}
	return ""
}

func findArtifactMetaIDByKind(items []WorkflowArtifactMeta, kind string) string {
	for _, item := range items {
		if item.Kind == kind {
			return item.ID
		}
	}
	return ""
}

func (r *ReplayProjectionRebuilder) nowUTC() time.Time {
	if r != nil && r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}
