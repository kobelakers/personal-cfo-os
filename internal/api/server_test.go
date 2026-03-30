package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/eval"
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

func TestAPIV1MetaProfileEndpointReturnsCanonicalProfile(t *testing.T) {
	server, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/meta/profile", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode profile meta: %v", err)
	}
	if payload["runtime_profile"] != "test-product" {
		t.Fatalf("expected runtime_profile=test-product, got %v", payload["runtime_profile"])
	}
	if payload["runtime_backend"] != "sqlite" {
		t.Fatalf("expected runtime_backend=sqlite, got %v", payload["runtime_backend"])
	}
	if payload["ui_mode"] != "api-only" {
		t.Fatalf("expected ui_mode=api-only, got %v", payload["ui_mode"])
	}
}

func TestAPIV1TaskGraphsMatchesCompatibilityAlias(t *testing.T) {
	server, _, _ := newTestServer(t)
	v1 := performRequest(t, server, http.MethodGet, "/api/v1/task-graphs", nil)
	alias := performRequest(t, server, http.MethodGet, "/task-graphs", nil)
	if v1.Code != http.StatusOK || alias.Code != http.StatusOK {
		t.Fatalf("expected 200 for both endpoints, got v1=%d alias=%d", v1.Code, alias.Code)
	}
	if normalizeJSON(v1.Body.Bytes()) != normalizeJSON(alias.Body.Bytes()) {
		t.Fatalf("expected canonical /api/v1 and alias response bodies to match")
	}
}

func TestApprovalDetailEndpointReturnsApprovalAndTaskGraph(t *testing.T) {
	server, approvalID, graphID := newTestServer(t)
	resp := performRequest(t, server, http.MethodGet, "/api/v1/approvals/"+approvalID, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for approval detail, got %d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Approval struct {
			ApprovalID string `json:"approval_id"`
			GraphID    string `json:"graph_id"`
		} `json:"approval"`
		TaskGraph *struct {
			Snapshot struct {
				Graph struct {
					GraphID string `json:"graph_id"`
				} `json:"graph"`
			} `json:"snapshot"`
		} `json:"task_graph"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode approval detail: %v", err)
	}
	if payload.Approval.ApprovalID != approvalID {
		t.Fatalf("expected approval id %s, got %s", approvalID, payload.Approval.ApprovalID)
	}
	if payload.TaskGraph == nil || payload.TaskGraph.Snapshot.Graph.GraphID != graphID {
		t.Fatalf("expected approval detail to include task graph %s, got %+v", graphID, payload.TaskGraph)
	}
}

func TestTaskReplayEndpointSupportsAPIV1(t *testing.T) {
	server, _, _ := newTestServer(t)
	resp := performRequest(t, server, http.MethodGet, "/api/v1/replay/tasks/task-api-test", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for task replay, got %d body=%s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode task replay: %v", err)
	}
	scope := payload["scope"].(map[string]any)
	if scope["kind"] != "task" {
		t.Fatalf("expected task replay scope, got %v", scope)
	}
}

func TestArtifactEndpointsReturnStructuredContent(t *testing.T) {
	server, _, _ := newTestServer(t)
	metadata := performRequest(t, server, http.MethodGet, "/api/v1/artifacts/artifact-api-test", nil)
	if metadata.Code != http.StatusOK {
		t.Fatalf("expected 200 for artifact metadata, got %d body=%s", metadata.Code, metadata.Body.String())
	}
	content := performRequest(t, server, http.MethodGet, "/api/v1/artifacts/artifact-api-test/content", nil)
	if content.Code != http.StatusOK {
		t.Fatalf("expected 200 for artifact content, got %d body=%s", content.Code, content.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(content.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode artifact content: %v", err)
	}
	if payload["structured"] == nil {
		t.Fatalf("expected structured artifact content, got %v", payload)
	}
}

func TestBenchmarkEndpointsUseConfiguredCatalog(t *testing.T) {
	server, _, _ := newTestServer(t)
	list := performRequest(t, server, http.MethodGet, "/api/v1/benchmarks/runs", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("expected 200 for benchmark list, got %d body=%s", list.Code, list.Body.String())
	}
	var items []map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode benchmark list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected sample + artifact benchmark entries, got %v", items)
	}
	if items[0]["id"] != "artifact-artifact-benchmark-api-test" && items[1]["id"] != "artifact-artifact-benchmark-api-test" {
		t.Fatalf("expected artifact-backed benchmark entry, got %v", items)
	}
	if items[0]["id"] != "sample-phase7b-benchmark" && items[1]["id"] != "sample-phase7b-benchmark" {
		t.Fatalf("expected seeded benchmark catalog, got %v", items)
	}
	detail := performRequest(t, server, http.MethodGet, "/api/v1/benchmarks/runs/sample-phase7b-benchmark", nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("expected 200 for benchmark detail, got %d body=%s", detail.Code, detail.Body.String())
	}
	compare := performRequest(t, server, http.MethodGet, "/api/v1/benchmarks/compare?left=sample-phase7b-benchmark&right=sample-phase7b-benchmark", nil)
	if compare.Code != http.StatusOK {
		t.Fatalf("expected 200 for benchmark compare, got %d body=%s", compare.Code, compare.Body.String())
	}
	export := performRequest(t, server, http.MethodGet, "/api/v1/benchmarks/exports/sample-phase7b-benchmark?format=md", nil)
	if export.Code != http.StatusOK {
		t.Fatalf("expected 200 for benchmark markdown export, got %d body=%s", export.Code, export.Body.String())
	}
	if !strings.Contains(export.Body.String(), "# Benchmark") {
		t.Fatalf("expected markdown benchmark export, got %s", export.Body.String())
	}
}

func TestServesStaticUIBundleWhenDistExists(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html><body>operator ui</body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	server, _, _ := newTestServerWithOptions(t, ServerOptions{UIDistDir: dist})
	resp := performRequest(t, server, http.MethodGet, "/", nil)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "operator ui") {
		t.Fatalf("expected ui shell, got %d body=%s", resp.Code, resp.Body.String())
	}
	asset := performRequest(t, server, http.MethodGet, "/assets/app.js", nil)
	if asset.Code != http.StatusOK || !strings.Contains(asset.Body.String(), "console.log") {
		t.Fatalf("expected static asset, got %d body=%s", asset.Code, asset.Body.String())
	}
}

func newTestServer(t *testing.T) (*Server, string, string) {
	return newTestServerWithOptions(t, ServerOptions{})
}

func newTestServerWithOptions(t *testing.T, options ServerOptions) (*Server, string, string) {
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
	if err := artifactStore.SaveArtifact(graph.ParentWorkflowID, task.ID, reporting.WorkflowArtifact{
		ID:         "artifact-api-test",
		Kind:       reporting.ArtifactKindReplayBundle,
		WorkflowID: graph.ParentWorkflowID,
		TaskID:     task.ID,
		ProducedBy: "api_test",
		ContentJSON: `{
  "scenario": "api-test",
  "trace": {
    "workflow_id": "workflow-life-event-api",
    "trace_id": "trace-api-test",
    "usage": [
      {
        "provider": "openai",
        "model": "gpt-5.4",
        "prompt_id": "planner",
        "prompt_tokens": 120,
        "completion_tokens": 80,
        "total_tokens": 200,
        "estimated_cost_usd": 0.12,
        "recorded_at": "2026-03-29T15:00:00Z"
      }
    ],
    "llm_calls": [
      {
        "provider": "openai",
        "model": "gpt-5.4",
        "prompt_id": "planner",
        "latency_ms": 320,
        "started_at": "2026-03-29T15:00:00Z",
        "completed_at": "2026-03-29T15:00:00Z"
      }
    ]
  },
  "generated_at": "2026-03-29T15:00:00Z"
}`,
		Ref: reporting.ArtifactRef{
			ID:       "artifact-api-test",
			Summary:  "api test replay bundle",
			Location: "artifact-api-test.json",
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	approval, ok, err := service.Runtime().Approvals.LoadByTask(graph.GraphID, task.ID)
	if err != nil || !ok {
		t.Fatalf("load api test approval: %v %+v", err, approval)
	}
	if err := workflowRuns.Save(runtime.WorkflowRunRecord{
		WorkflowID:   "workflow-benchmark-artifact",
		TaskID:       "task-benchmark-artifact",
		Intent:       "benchmark_catalog_seed",
		RuntimeState: runtime.WorkflowStateCompleted,
		StartedAt:    now,
		UpdatedAt:    now,
		Summary:      "benchmark artifact seed",
	}); err != nil {
		t.Fatalf("seed workflow run for benchmark artifact: %v", err)
	}
	if err := artifactStore.SaveArtifact("workflow-benchmark-artifact", "task-benchmark-artifact", reporting.WorkflowArtifact{
		ID:         "artifact-benchmark-api-test",
		Kind:       reporting.ArtifactKindEvalRunResult,
		WorkflowID: "workflow-benchmark-artifact",
		TaskID:     "task-benchmark-artifact",
		ProducedBy: "api_test",
		ContentJSON: `{
  "run_id": "artifact-benchmark-run",
  "corpus_id": "phase7b-artifact",
  "deterministic_only": true,
  "started_at": "2026-03-29T15:00:00Z",
  "completed_at": "2026-03-29T15:00:00Z",
  "results": [
    {
      "scenario_id": "artifact-benchmark-scenario",
      "category": "artifact_seed",
      "description": "artifact-backed benchmark run",
      "deterministic": true,
      "passed": true,
      "runtime_state": "completed",
      "token_usage": 320,
      "duration_milliseconds": 60
    }
  ],
  "score": {
    "scenario_count": 1,
    "passed_count": 1,
    "failed_count": 0,
    "task_success_rate": 1,
    "validator_pass_rate": 1,
    "approval_frequency": 0,
    "average_latency_milliseconds": 60,
    "total_token_usage": 320
  },
  "summary": {
    "corpus_id": "phase7b-artifact",
    "deterministic_only": true,
    "passed_scenarios": ["artifact-benchmark-scenario"],
    "summary_lines": ["artifact-backed benchmark"]
  },
  "cost_summary": {
    "usd": 0.03125,
    "precision": "recorded_exact",
    "source": "artifact_payload"
  }
}`,
		Ref: reporting.ArtifactRef{
			ID:       "artifact-benchmark-api-test",
			Summary:  "artifact benchmark seed",
			Location: "artifact-benchmark-api-test.json",
		},
		CreatedAt: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("seed benchmark artifact: %v", err)
	}
	replayQuery := runtime.NewReplayQueryService(service, workflowRuns, replayProjections, artifactStore, replayStore)
	benchmarkDir := t.TempDir()
	writeBenchmarkSample(t, benchmarkDir, now)
	options.RuntimeProfile = firstNonEmpty(options.RuntimeProfile, "test-product")
	options.RuntimeBackend = firstNonEmpty(options.RuntimeBackend, "sqlite")
	options.BlobBackend = firstNonEmpty(options.BlobBackend, "localfs")
	options.BenchmarkCatalogDir = firstNonEmpty(options.BenchmarkCatalogDir, benchmarkDir)
	options.BenchmarkArtifacts = artifactStore
	options.BenchmarkWorkflowRuns = workflowRuns
	options.SupportedSchemaVersions = []string{"v1"}
	return NewServer(runtime.NewQueryService(service), replayQuery, runtime.NewOperatorService(service), service, options), approval.ApprovalID, graph.GraphID
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

func performRequest(t *testing.T, server *Server, method string, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	return resp
}

func writeBenchmarkSample(t *testing.T, dir string, now time.Time) {
	t.Helper()
	run := eval.EvalRun{
		RunID:             "sample-phase7b-benchmark-run",
		CorpusID:          "phase6b-default",
		DeterministicOnly: true,
		StartedAt:         now,
		CompletedAt:       now,
		Results: []eval.EvalResult{
			{
				ScenarioID:           "behavior_intervention_happy_path",
				Category:             "behavior_intervention",
				Description:          "seeded benchmark run",
				Deterministic:        true,
				Passed:               true,
				RuntimeState:         "completed",
				TokenUsage:           240,
				DurationMilliseconds: 50,
			},
		},
		Score: eval.EvalScore{
			ScenarioCount:              1,
			PassedCount:                1,
			FailedCount:                0,
			TaskSuccessRate:            1,
			ValidatorPassRate:          1,
			ApprovalFrequency:          0,
			AverageLatencyMilliseconds: 50,
			TotalTokenUsage:            240,
		},
		Summary: eval.EvalSummary{
			CorpusID:          "phase6b-default",
			DeterministicOnly: true,
			PassedScenarios:   []string{"behavior_intervention_happy_path"},
			SummaryLines:      []string{"phase7b benchmark seeded for operator surface"},
		},
	}
	payload, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		t.Fatalf("marshal benchmark sample: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample-phase7b-benchmark.json"), payload, 0o644); err != nil {
		t.Fatalf("write benchmark sample: %v", err)
	}
}

func normalizeJSON(payload []byte) string {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return string(payload)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return string(payload)
	}
	return string(normalized)
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
