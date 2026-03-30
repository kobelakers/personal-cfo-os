package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

func TestApproveEndpointReturns400ForBadPayload(t *testing.T) {
	server, approvalID, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/approvals/"+approvalID+"/approve", bytes.NewBufferString("{"))
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad payload, got %d", resp.Code)
	}
}

func TestApproveEndpointReturns404ForMissingApproval(t *testing.T) {
	server, _, _ := newTestServer(t)
	resp := performJSONRequest(t, server, http.MethodPost, "/approvals/missing/approve", map[string]any{
		"request_id": "approve-missing",
		"actor":      "operator",
	})
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing approval, got %d", resp.Code)
	}
}

func TestApproveEndpointReturns409ForInvalidStateTransition(t *testing.T) {
	server, approvalID, _ := newTestServer(t)
	first := performJSONRequest(t, server, http.MethodPost, "/approvals/"+approvalID+"/approve", map[string]any{
		"request_id": "approve-once",
		"actor":      "operator",
		"roles":      []string{"operator"},
	})
	if first.Code != http.StatusOK {
		t.Fatalf("expected first approval to succeed, got %d", first.Code)
	}
	second := performJSONRequest(t, server, http.MethodPost, "/approvals/"+approvalID+"/approve", map[string]any{
		"request_id": "approve-twice",
		"actor":      "operator",
		"roles":      []string{"operator"},
	})
	if second.Code != http.StatusConflict {
		t.Fatalf("expected 409 for invalid transition, got %d body=%s", second.Code, second.Body.String())
	}
}

func TestApproveEndpointReturns409ForVersionConflict(t *testing.T) {
	server, approvalID, _ := newTestServer(t)
	resp := performJSONRequest(t, server, http.MethodPost, "/approvals/"+approvalID+"/approve", map[string]any{
		"request_id":       "approve-conflict",
		"actor":            "operator",
		"roles":            []string{"operator"},
		"expected_version": 99,
	})
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409 for CAS conflict, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestReplayTaskGraphEndpointReturnsStructuredView(t *testing.T) {
	server, _, graphID := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/replay/task-graphs/"+graphID, nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for replay task graph endpoint, got %d body=%s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if payload["task_graph"] == nil {
		t.Fatalf("expected replay payload to include task_graph view, got %v", payload)
	}
}

func newTestServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	now := time.Date(2026, 3, 29, 15, 0, 0, 0, time.UTC)
	workflowRuns := runtime.NewInMemoryWorkflowRunStore()
	replayProjections := runtime.NewInMemoryReplayProjectionStore()
	artifactStore := runtime.NewInMemoryArtifactMetadataStore()
	replayStore := runtime.NewInMemoryReplayStore()
	service := runtime.NewService(runtime.ServiceOptions{
		CheckpointStore: runtime.NewInMemoryCheckpointStore(),
		TaskGraphs:      runtime.NewInMemoryTaskGraphStore(),
		Executions:      runtime.NewInMemoryTaskExecutionStore(),
		Approvals:       runtime.NewInMemoryApprovalStateStore(),
		OperatorActions: runtime.NewInMemoryOperatorActionStore(),
		Replay:          replayStore,
		Artifacts:       artifactStore,
		Controller:      runtime.DefaultWorkflowController{},
		Now:             func() time.Time { return now },
	})
	rebuilder := runtime.NewReplayProjectionRebuilder(service, workflowRuns, replayProjections, artifactStore, replayStore, func() time.Time { return now })
	service.SetReplayProjectionWriter(rebuilder)
	service.SetCapabilities(runtime.StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
		},
		Workflows: map[taskspec.UserIntentType]runtime.FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: apiTestCapability{
				execute: func(_ context.Context, spec taskspec.TaskSpec, _ runtime.FollowUpActivationContext, current state.FinancialWorldState) (runtime.FollowUpWorkflowRunResult, error) {
					return apiWaitingApprovalResult("workflow-child-"+spec.ID, spec.ID, current), nil
				},
				resume: func(_ context.Context, spec taskspec.TaskSpec, _ runtime.FollowUpActivationContext, current state.FinancialWorldState, _ runtime.CheckpointRecord, _ runtime.ResumeToken, _ runtime.CheckpointPayloadEnvelope) (runtime.FollowUpWorkflowRunResult, error) {
					return runtime.FollowUpWorkflowRunResult{
						WorkflowID:   "workflow-child-" + spec.ID,
						RuntimeState: runtime.WorkflowStateCompleted,
						UpdatedState: apiTestState(now, current.Version.Sequence+1),
					}, nil
				},
			},
		},
	})
	task := taskspec.TaskSpec{
		ID:    "task-api-test",
		Goal:  "api follow-up test",
		Scope: taskspec.TaskScope{Areas: []string{"tax"}},
		Constraints: taskspec.ConstraintSet{
			Hard: []string{"must remain evidence-backed"},
		},
		RiskLevel:       taskspec.RiskLevelMedium,
		UserIntentType:  taskspec.UserIntentTaxOptimization,
		CreatedAt:       now,
		SuccessCriteria: []taskspec.SuccessCriteria{{ID: "ok", Description: "complete the follow-up path"}},
		RequiredEvidence: []taskspec.RequiredEvidenceRef{
			{Type: "event_signal", Reason: "api test seed", Mandatory: true},
		},
		ApprovalRequirement: taskspec.ApprovalRequirementRecommended,
	}
	graph := taskspec.TaskGraph{
		GraphID:          "graph-api-test",
		ParentWorkflowID: "workflow-life-event-api",
		ParentTaskID:     "task-life-event-api",
		TriggerSource:    taskspec.TaskTriggerSourceLifeEvent,
		GeneratedAt:      now,
		GeneratedTasks: []taskspec.GeneratedTaskSpec{
			{
				Task: task,
				Metadata: taskspec.GeneratedTaskMetadata{
					GeneratedAt:       now,
					ParentWorkflowID:  "workflow-life-event-api",
					ParentTaskID:      "task-life-event-api",
					RootCorrelationID: "workflow-life-event-api",
					TriggerSource:     taskspec.TaskTriggerSourceLifeEvent,
					Priority:          taskspec.TaskPriorityHigh,
					ExecutionDepth:    1,
					GenerationReasons: []taskspec.TaskGenerationReason{{
						Code:          taskspec.TaskGenerationReasonLifeEventImpact,
						Description:   "api test seed",
						LifeEventID:   "event-api",
						LifeEventKind: "salary_change",
						EvidenceIDs:   []string{"evidence-life-event-event-api"},
					}},
				},
			},
		},
	}
	execCtx := runtime.ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := service.Runtime().RegisterFollowUpTasks(execCtx, graph, apiTestState(now, 1)); err != nil {
		t.Fatalf("register api test graph: %v", err)
	}
	if _, err := service.Runtime().ExecuteReadyFollowUps(context.Background(), execCtx, graph.GraphID, runtime.DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("execute api test follow-up: %v", err)
	}
	if _, err := rebuilder.RebuildTaskGraph(context.Background(), graph.GraphID); err != nil {
		t.Fatalf("rebuild api test replay projection: %v", err)
	}
	approval, ok, err := service.Runtime().Approvals.LoadByTask(graph.GraphID, task.ID)
	if err != nil || !ok {
		t.Fatalf("load api test approval: %v %+v", err, approval)
	}
	replayQuery := runtime.NewReplayQueryService(service, workflowRuns, replayProjections, artifactStore, replayStore)
	return NewServer(runtime.NewQueryService(service), replayQuery, runtime.NewOperatorService(service), service), approval.ApprovalID, graph.GraphID
}

func performJSONRequest(t *testing.T, server *Server, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	return resp
}

type apiTestCapability struct {
	execute func(context.Context, taskspec.TaskSpec, runtime.FollowUpActivationContext, state.FinancialWorldState) (runtime.FollowUpWorkflowRunResult, error)
	resume  func(context.Context, taskspec.TaskSpec, runtime.FollowUpActivationContext, state.FinancialWorldState, runtime.CheckpointRecord, runtime.ResumeToken, runtime.CheckpointPayloadEnvelope) (runtime.FollowUpWorkflowRunResult, error)
}

func (c apiTestCapability) CapabilityName() string { return "tax_optimization_workflow" }

func (c apiTestCapability) Execute(ctx context.Context, spec taskspec.TaskSpec, activation runtime.FollowUpActivationContext, current state.FinancialWorldState) (runtime.FollowUpWorkflowRunResult, error) {
	return c.execute(ctx, spec, activation, current)
}

func (c apiTestCapability) Resume(ctx context.Context, spec taskspec.TaskSpec, activation runtime.FollowUpActivationContext, current state.FinancialWorldState, checkpoint runtime.CheckpointRecord, token runtime.ResumeToken, payload runtime.CheckpointPayloadEnvelope) (runtime.FollowUpWorkflowRunResult, error) {
	return c.resume(ctx, spec, activation, current, checkpoint, token, payload)
}

func apiWaitingApprovalResult(workflowID string, taskID string, current state.FinancialWorldState) runtime.FollowUpWorkflowRunResult {
	result := runtime.FollowUpWorkflowRunResult{
		WorkflowID:   workflowID,
		RuntimeState: runtime.WorkflowStateWaitingApproval,
		UpdatedState: current,
		Checkpoint: &runtime.CheckpointRecord{
			ID:           workflowID + "-checkpoint",
			WorkflowID:   workflowID,
			State:        runtime.WorkflowStateVerifying,
			ResumeState:  runtime.WorkflowStateVerifying,
			StateVersion: current.Version.Sequence,
			Summary:      "waiting approval",
			CapturedAt:   current.Version.UpdatedAt,
		},
		ResumeToken: &runtime.ResumeToken{
			Token:        workflowID + "-resume",
			WorkflowID:   workflowID,
			CheckpointID: workflowID + "-checkpoint",
			IssuedAt:     current.Version.UpdatedAt,
			ExpiresAt:    current.Version.UpdatedAt.Add(24 * time.Hour),
		},
		PendingApproval: &runtime.HumanApprovalPending{
			ApprovalID:      workflowID + "-approval",
			WorkflowID:      workflowID,
			RequestedAction: "follow_up_execution",
			RequestedAt:     current.Version.UpdatedAt,
		},
	}
	result.CheckpointPayload = &runtime.CheckpointPayloadEnvelope{
		Kind: runtime.CheckpointPayloadKindFollowUpFinalizeResume,
		FollowUpFinalizeResume: &runtime.FollowUpFinalizeResumePayload{
			GraphID:      "graph-api-test",
			TaskID:       taskID,
			WorkflowID:   workflowID,
			ArtifactKind: reporting.ArtifactKindTaxOptimizationReport,
			DraftReport: reporting.ReportPayload{
				TaxOptimization: &reporting.TaxOptimizationReport{
					TaskID:               taskID,
					WorkflowID:           workflowID,
					Summary:              "draft api test report",
					DeterministicMetrics: map[string]any{"effective_tax_rate": 0.21},
					RiskFlags:            []analysis.RiskFlag{{Code: "deadline", Severity: "medium", Detail: "api test"}},
					ApprovalRequired:     true,
					Confidence:           0.8,
					GeneratedAt:          current.Version.UpdatedAt,
				},
			},
			PendingStateSnapshotRef: current.Version.SnapshotID,
		},
	}
	return result
}

func apiTestState(now time.Time, version uint64) state.FinancialWorldState {
	return state.FinancialWorldState{
		UserID: "user-1",
		Version: state.StateVersion{
			Sequence:   version,
			SnapshotID: fmt.Sprintf("api-state-v%d", version),
			UpdatedAt:  now,
		},
	}
}
