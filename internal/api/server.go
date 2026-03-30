package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/product"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

type ServerOptions struct {
	RuntimeProfile          string
	RuntimeBackend          string
	BlobBackend             string
	BenchmarkCatalogDir     string
	BenchmarkArtifacts      runtime.ArtifactMetadataStore
	BenchmarkWorkflowRuns   runtime.WorkflowRunStore
	SupportedSchemaVersions []string
	UIDistDir               string
	UIMode                  string
}

type Server struct {
	surface   *product.OperatorSurfaceService
	mux       *http.ServeMux
	uiDistDir string
	uiHandler http.Handler
}

func NewServer(query *runtime.QueryService, replay *runtime.ReplayQueryService, operator *runtime.OperatorService, service *runtime.Service, opts ...ServerOptions) *Server {
	var options ServerOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	supportedVersions := append([]string{}, options.SupportedSchemaVersions...)
	if len(supportedVersions) == 0 {
		supportedVersions = []string{"v1"}
	}
	surface := product.NewOperatorSurfaceService(product.OperatorSurfaceOptions{
		Query:    query,
		Replay:   replay,
		Operator: operator,
		Service:  service,
		Benchmarks: product.NewBenchmarkSurfaceService(product.BenchmarkSurfaceOptions{
			SampleDir:    options.BenchmarkCatalogDir,
			Artifacts:    options.BenchmarkArtifacts,
			WorkflowRuns: options.BenchmarkWorkflowRuns,
		}),
		RuntimeProfile:          options.RuntimeProfile,
		RuntimeBackend:          options.RuntimeBackend,
		BlobBackend:             options.BlobBackend,
		UIMode:                  firstNonEmpty(strings.TrimSpace(options.UIMode), uiModeForDist(options.UIDistDir)),
		SupportedSchemaVersions: supportedVersions,
	})
	server := &Server{
		surface:   surface,
		mux:       http.NewServeMux(),
		uiDistDir: strings.TrimSpace(options.UIDistDir),
	}
	server.uiHandler = newStaticUIHandler(server.uiDistDir)
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	// canonical /api/v1 routes
	s.handle("GET", "/api/v1/meta/profile", s.handleGetMetaProfile)
	s.handle("GET", "/api/v1/task-graphs", s.handleListTaskGraphs)
	s.handle("GET", "/api/v1/task-graphs/{id}", s.handleGetTaskGraph)
	s.handle("GET", "/api/v1/approvals/pending", s.handleListPendingApprovals)
	s.handle("GET", "/api/v1/approvals/{id}", s.handleGetApprovalDetail)
	s.handle("POST", "/api/v1/approvals/{id}/approve", s.handleApproveTask)
	s.handle("POST", "/api/v1/approvals/{id}/deny", s.handleDenyTask)
	s.handle("POST", "/api/v1/follow-ups/{task_id}/resume", s.handleResumeFollowUp)
	s.handle("POST", "/api/v1/follow-ups/{task_id}/retry", s.handleRetryFollowUp)
	s.handle("POST", "/api/v1/task-graphs/{id}/reevaluate", s.handleReevaluateTaskGraph)
	s.handle("GET", "/api/v1/replay/workflows/{id}", s.handleGetWorkflowReplay)
	s.handle("GET", "/api/v1/replay/task-graphs/{id}", s.handleGetTaskGraphReplay)
	s.handle("GET", "/api/v1/replay/tasks/{id}", s.handleGetTaskReplay)
	s.handle("GET", "/api/v1/replay/executions/{id}", s.handleGetExecutionReplay)
	s.handle("GET", "/api/v1/replay/approvals/{id}", s.handleGetApprovalReplay)
	s.handle("GET", "/api/v1/replay/compare", s.handleCompareReplay)
	s.handle("GET", "/api/v1/artifacts/{id}", s.handleGetArtifact)
	s.handle("GET", "/api/v1/artifacts/{id}/content", s.handleGetArtifactContent)
	s.handle("GET", "/api/v1/benchmarks/runs", s.handleListBenchmarkRuns)
	s.handle("GET", "/api/v1/benchmarks/runs/{id}", s.handleGetBenchmarkRun)
	s.handle("GET", "/api/v1/benchmarks/compare", s.handleCompareBenchmarks)
	s.handle("GET", "/api/v1/benchmarks/exports/{id}", s.handleExportBenchmarkRun)

	// compatibility aliases that must reuse the same handlers
	s.handle("GET", "/task-graphs", s.handleListTaskGraphs)
	s.handle("GET", "/task-graphs/{id}", s.handleGetTaskGraph)
	s.handle("GET", "/approvals/pending", s.handleListPendingApprovals)
	s.handle("GET", "/approvals/{id}", s.handleGetApprovalDetail)
	s.handle("POST", "/approvals/{id}/approve", s.handleApproveTask)
	s.handle("POST", "/approvals/{id}/deny", s.handleDenyTask)
	s.handle("POST", "/follow-ups/{task_id}/resume", s.handleResumeFollowUp)
	s.handle("POST", "/follow-ups/{task_id}/retry", s.handleRetryFollowUp)
	s.handle("POST", "/task-graphs/{id}/reevaluate", s.handleReevaluateTaskGraph)
	s.handle("GET", "/replay/workflows/{id}", s.handleGetWorkflowReplay)
	s.handle("GET", "/replay/task-graphs/{id}", s.handleGetTaskGraphReplay)
	s.handle("GET", "/replay/tasks/{id}", s.handleGetTaskReplay)
	s.handle("GET", "/replay/executions/{id}", s.handleGetExecutionReplay)
	s.handle("GET", "/replay/approvals/{id}", s.handleGetApprovalReplay)
	s.handle("GET", "/replay/compare", s.handleCompareReplay)
	s.handle("GET", "/artifacts/{id}", s.handleGetArtifact)
	s.handle("GET", "/artifacts/{id}/content", s.handleGetArtifactContent)
	s.handle("GET", "/benchmarks/runs", s.handleListBenchmarkRuns)
	s.handle("GET", "/benchmarks/runs/{id}", s.handleGetBenchmarkRun)
	s.handle("GET", "/benchmarks/compare", s.handleCompareBenchmarks)
	s.handle("GET", "/benchmarks/exports/{id}", s.handleExportBenchmarkRun)

	if s.uiHandler != nil {
		s.mux.Handle("/", s.uiHandler)
	}
}

func (s *Server) handle(method string, path string, handler http.HandlerFunc) {
	s.mux.HandleFunc(method+" "+path, handler)
}

type actionRequest struct {
	RequestID       string   `json:"request_id"`
	Actor           string   `json:"actor"`
	Roles           []string `json:"roles,omitempty"`
	Note            string   `json:"note,omitempty"`
	ExpectedVersion int64    `json:"expected_version,omitempty"`
	GraphID         string   `json:"graph_id,omitempty"`
}

type reevaluateResponse struct {
	Activation runtime.TaskActivationResult `json:"activation"`
	Command    runtime.TaskCommandResult    `json:"command"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type badRequestError struct {
	message string
}

func (e badRequestError) Error() string { return e.message }

func (s *Server) handleGetMetaProfile(w http.ResponseWriter, r *http.Request) {
	meta, err := s.surface.Meta(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) handleListTaskGraphs(w http.ResponseWriter, r *http.Request) {
	views, err := s.surface.ListTaskGraphs(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleGetTaskGraph(w http.ResponseWriter, r *http.Request) {
	view, err := s.surface.GetTaskGraph(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetWorkflowReplay(w http.ResponseWriter, r *http.Request) {
	view, err := s.surface.QueryReplay(r.Context(), observability.ReplayQuery{WorkflowID: r.PathValue("id")})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetTaskGraphReplay(w http.ResponseWriter, r *http.Request) {
	view, err := s.surface.QueryReplay(r.Context(), observability.ReplayQuery{TaskGraphID: r.PathValue("id")})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetTaskReplay(w http.ResponseWriter, r *http.Request) {
	view, err := s.surface.QueryReplay(r.Context(), observability.ReplayQuery{TaskID: r.PathValue("id")})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetExecutionReplay(w http.ResponseWriter, r *http.Request) {
	view, err := s.surface.QueryReplay(r.Context(), observability.ReplayQuery{ExecutionID: r.PathValue("id")})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetApprovalReplay(w http.ResponseWriter, r *http.Request) {
	view, err := s.surface.QueryReplay(r.Context(), observability.ReplayQuery{ApprovalID: r.PathValue("id")})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleCompareReplay(w http.ResponseWriter, r *http.Request) {
	left, err := parseReplayCompareQuery(r.URL.Query().Get("left"))
	if err != nil {
		writeAPIError(w, badRequestError{message: err.Error()})
		return
	}
	right, err := parseReplayCompareQuery(r.URL.Query().Get("right"))
	if err != nil {
		writeAPIError(w, badRequestError{message: err.Error()})
		return
	}
	comparison, err := s.surface.CompareReplay(r.Context(), left, right)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, comparison)
}

func (s *Server) handleListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	items, err := s.surface.ListPendingApprovals(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleGetApprovalDetail(w http.ResponseWriter, r *http.Request) {
	item, err := s.surface.GetApprovalDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleApproveTask(w http.ResponseWriter, r *http.Request) {
	var req actionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Actor) == "" {
		writeAPIError(w, badRequestError{message: "request_id and actor are required"})
		return
	}
	result, err := s.surface.Approve(r.Context(), runtime.ApproveTaskCommand{
		RequestID:       req.RequestID,
		ApprovalID:      r.PathValue("id"),
		Actor:           req.Actor,
		Roles:           append([]string{}, req.Roles...),
		Note:            req.Note,
		ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDenyTask(w http.ResponseWriter, r *http.Request) {
	var req actionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Actor) == "" {
		writeAPIError(w, badRequestError{message: "request_id and actor are required"})
		return
	}
	result, err := s.surface.Deny(r.Context(), runtime.DenyTaskCommand{
		RequestID:       req.RequestID,
		ApprovalID:      r.PathValue("id"),
		Actor:           req.Actor,
		Roles:           append([]string{}, req.Roles...),
		Note:            req.Note,
		ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleResumeFollowUp(w http.ResponseWriter, r *http.Request) {
	var req actionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Actor) == "" {
		writeAPIError(w, badRequestError{message: "request_id and actor are required"})
		return
	}
	result, err := s.surface.Resume(r.Context(), runtime.ResumeFollowUpTaskCommand{
		RequestID:       req.RequestID,
		GraphID:         req.GraphID,
		TaskID:          r.PathValue("task_id"),
		Actor:           req.Actor,
		Roles:           append([]string{}, req.Roles...),
		Note:            req.Note,
		ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRetryFollowUp(w http.ResponseWriter, r *http.Request) {
	var req actionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Actor) == "" {
		writeAPIError(w, badRequestError{message: "request_id and actor are required"})
		return
	}
	result, err := s.surface.Retry(r.Context(), runtime.RetryFailedFollowUpTaskCommand{
		RequestID:       req.RequestID,
		GraphID:         req.GraphID,
		TaskID:          r.PathValue("task_id"),
		Actor:           req.Actor,
		Roles:           append([]string{}, req.Roles...),
		Note:            req.Note,
		ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleReevaluateTaskGraph(w http.ResponseWriter, r *http.Request) {
	var req actionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeAPIError(w, err)
		return
	}
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Actor) == "" {
		writeAPIError(w, badRequestError{message: "request_id and actor are required"})
		return
	}
	activation, result, err := s.surface.Reevaluate(r.Context(), runtime.ReevaluateTaskGraphCommand{
		RequestID: req.RequestID,
		GraphID:   r.PathValue("id"),
		Actor:     req.Actor,
		Roles:     append([]string{}, req.Roles...),
		Note:      req.Note,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reevaluateResponse{Activation: activation, Command: result})
}

func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	view, err := s.surface.LoadArtifact(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view.Artifact)
}

func (s *Server) handleGetArtifactContent(w http.ResponseWriter, r *http.Request) {
	view, err := s.surface.LoadArtifact(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleListBenchmarkRuns(w http.ResponseWriter, r *http.Request) {
	bench := s.surface.Benchmarks()
	if bench == nil {
		writeJSON(w, http.StatusOK, []product.BenchmarkRunSummary{})
		return
	}
	items, err := bench.ListRuns()
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleGetBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	bench := s.surface.Benchmarks()
	if bench == nil {
		writeAPIError(w, badRequestError{message: "benchmark surface is not configured"})
		return
	}
	item, err := bench.GetRun(r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleCompareBenchmarks(w http.ResponseWriter, r *http.Request) {
	bench := s.surface.Benchmarks()
	if bench == nil {
		writeAPIError(w, badRequestError{message: "benchmark surface is not configured"})
		return
	}
	leftID := strings.TrimSpace(r.URL.Query().Get("left"))
	rightID := strings.TrimSpace(r.URL.Query().Get("right"))
	if leftID == "" || rightID == "" {
		writeAPIError(w, badRequestError{message: "left and right benchmark ids are required"})
		return
	}
	view, err := bench.Compare(leftID, rightID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleExportBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	bench := s.surface.Benchmarks()
	if bench == nil {
		writeAPIError(w, badRequestError{message: "benchmark surface is not configured"})
		return
	}
	runID := r.PathValue("id")
	format := firstNonEmpty(strings.TrimSpace(r.URL.Query().Get("format")), "json")
	switch format {
	case "json":
		item, err := bench.GetRun(runID)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	case "md", "markdown", "summary":
		payload, err := bench.ExportRunMarkdown(runID)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	default:
		writeAPIError(w, badRequestError{message: fmt.Sprintf("unsupported export format %q", format)})
	}
}

func decodeJSONBody(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return badRequestError{message: fmt.Sprintf("invalid request body: %v", err)}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return badRequestError{message: "request body must contain a single JSON object"}
	}
	return nil
}

func parseReplayCompareQuery(raw string) (observability.ReplayQuery, error) {
	scope, id, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || strings.TrimSpace(id) == "" {
		return observability.ReplayQuery{}, fmt.Errorf("replay compare query must use scope:id")
	}
	switch strings.TrimSpace(scope) {
	case "workflow":
		return observability.ReplayQuery{WorkflowID: strings.TrimSpace(id)}, nil
	case "task_graph":
		return observability.ReplayQuery{TaskGraphID: strings.TrimSpace(id)}, nil
	case "task":
		return observability.ReplayQuery{TaskID: strings.TrimSpace(id)}, nil
	case "execution":
		return observability.ReplayQuery{ExecutionID: strings.TrimSpace(id)}, nil
	case "approval":
		return observability.ReplayQuery{ApprovalID: strings.TrimSpace(id)}, nil
	default:
		return observability.ReplayQuery{}, fmt.Errorf("unsupported replay scope %q", scope)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAPIError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var badReq badRequestError
	switch {
	case errors.As(err, &badReq):
		status = http.StatusBadRequest
	case runtime.IsNotFound(err):
		status = http.StatusNotFound
	case runtime.IsConflict(err):
		status = http.StatusConflict
	}
	writeJSON(w, status, errorResponse{Error: err.Error()})
}

func Shutdown(ctx context.Context, server *http.Server) error {
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func newStaticUIHandler(distDir string) http.Handler {
	root := strings.TrimSpace(distDir)
	if root == "" {
		return nil
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil
	}
	fileServer := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
		if path == "." || path == "" {
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}
		candidate := filepath.Join(root, path)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}

func uiModeForDist(distDir string) string {
	if root := strings.TrimSpace(distDir); root != "" {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return "served-dist"
		}
	}
	return "api-only"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
