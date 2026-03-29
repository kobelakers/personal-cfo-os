package observability

import (
	"fmt"
	"slices"
)

type ReplayService struct {
	Store ReplayQueryStore
}

func NewReplayService(store ReplayQueryStore) *ReplayService {
	return &ReplayService{Store: store}
}

func (s *ReplayService) ByGraph(graphID string) (RuntimeReplayTrace, error) {
	if s == nil || s.Store == nil {
		return RuntimeReplayTrace{}, fmt.Errorf("replay store is required")
	}
	events, err := s.Store.ListByGraph(graphID)
	if err != nil {
		return RuntimeReplayTrace{}, err
	}
	return buildRuntimeReplayTrace("graph", graphID, events), nil
}

func (s *ReplayService) ByTask(taskID string) (RuntimeReplayTrace, error) {
	if s == nil || s.Store == nil {
		return RuntimeReplayTrace{}, fmt.Errorf("replay store is required")
	}
	events, err := s.Store.ListByTask(taskID)
	if err != nil {
		return RuntimeReplayTrace{}, err
	}
	return buildRuntimeReplayTrace("task", taskID, events), nil
}

func (s *ReplayService) ByWorkflow(workflowID string) (RuntimeReplayTrace, error) {
	if s == nil || s.Store == nil {
		return RuntimeReplayTrace{}, fmt.Errorf("replay store is required")
	}
	events, err := s.Store.ListByWorkflow(workflowID)
	if err != nil {
		return RuntimeReplayTrace{}, err
	}
	return buildRuntimeReplayTrace("workflow", workflowID, events), nil
}

func buildRuntimeReplayTrace(scope string, scopeID string, events []ReplayEvent) RuntimeReplayTrace {
	trace := RuntimeReplayTrace{
		Scope:      scope,
		ScopeID:    scopeID,
		Events:     append([]ReplayEvent{}, events...),
		Provenance: ProvenanceChain{},
	}
	workflowSet := make(map[string]struct{})
	taskSet := make(map[string]struct{})
	approvalSet := make(map[string]struct{})
	executionSet := make(map[string]struct{})
	artifactSet := make(map[string]struct{})
	checkpointSet := make(map[string]struct{})
	resumeSet := make(map[string]struct{})
	actionSet := make(map[string]struct{})
	for _, event := range events {
		if trace.Provenance.RootCorrelationID == "" && event.RootCorrelationID != "" {
			trace.Provenance.RootCorrelationID = event.RootCorrelationID
		}
		if trace.Provenance.ParentWorkflowID == "" && event.ParentWorkflowID != "" {
			trace.Provenance.ParentWorkflowID = event.ParentWorkflowID
		}
		if event.WorkflowID != "" {
			workflowSet[event.WorkflowID] = struct{}{}
		}
		if event.TaskID != "" {
			taskSet[event.TaskID] = struct{}{}
		}
		if event.ApprovalID != "" {
			approvalSet[event.ApprovalID] = struct{}{}
		}
		if event.ExecutionID != "" {
			executionSet[event.ExecutionID] = struct{}{}
		}
		if event.CheckpointID != "" {
			checkpointSet[event.CheckpointID] = struct{}{}
		}
		if event.ResumeToken != "" {
			resumeSet[event.ResumeToken] = struct{}{}
		}
		for _, artifactID := range event.ArtifactIDs {
			if artifactID != "" {
				artifactSet[artifactID] = struct{}{}
			}
		}
		if event.ActionType != "" {
			actionSet[event.ActionType] = struct{}{}
		}
		if event.ActionType == "follow_up_execution" && event.ExecutionID != "" {
			trace.Attempts++
		}
		if event.ActionType == "retry" {
			trace.RetryEvents++
		}
		if event.ActionType == "approve" || event.ActionType == "deny" || event.ActionType == "resume" {
			trace.ApprovalActions++
		}
		if event.CommittedStateRef != "" && event.CommittedStateRef == event.UpdatedStateRef {
			trace.CommittedAdvances++
		}
		trace.CurrentSummary = event.Summary
	}
	trace.Provenance.WorkflowIDs = sortedKeys(workflowSet)
	trace.Provenance.TaskIDs = sortedKeys(taskSet)
	trace.Provenance.ApprovalIDs = sortedKeys(approvalSet)
	trace.Provenance.ExecutionIDs = sortedKeys(executionSet)
	trace.Provenance.ArtifactIDs = sortedKeys(artifactSet)
	trace.Provenance.CheckpointIDs = sortedKeys(checkpointSet)
	trace.Provenance.ResumeTokens = sortedKeys(resumeSet)
	trace.ActionTypes = sortedKeys(actionSet)
	return trace
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
