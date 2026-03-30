package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

type Server struct {
	query  *runtime.QueryService
	replay *runtime.ReplayQueryService
	operator *runtime.OperatorService
	service  *runtime.Service
	mux      *http.ServeMux
}

func NewServer(query *runtime.QueryService, replay *runtime.ReplayQueryService, operator *runtime.OperatorService, service *runtime.Service) *Server {
	server := &Server{
		query:    query,
		replay:   replay,
		operator: operator,
		service:  service,
		mux:      http.NewServeMux(),
	}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /task-graphs", s.handleListTaskGraphs)
	s.mux.HandleFunc("GET /task-graphs/{id}", s.handleGetTaskGraph)
	s.mux.HandleFunc("GET /replay/workflows/{id}", s.handleGetWorkflowReplay)
	s.mux.HandleFunc("GET /replay/task-graphs/{id}", s.handleGetTaskGraphReplay)
	s.mux.HandleFunc("GET /replay/executions/{id}", s.handleGetExecutionReplay)
	s.mux.HandleFunc("GET /replay/approvals/{id}", s.handleGetApprovalReplay)
	s.mux.HandleFunc("GET /replay/compare", s.handleCompareReplay)
	s.mux.HandleFunc("GET /approvals/pending", s.handleListPendingApprovals)
	s.mux.HandleFunc("POST /approvals/{id}/approve", s.handleApproveTask)
	s.mux.HandleFunc("POST /approvals/{id}/deny", s.handleDenyTask)
	s.mux.HandleFunc("POST /follow-ups/{task_id}/resume", s.handleResumeFollowUp)
	s.mux.HandleFunc("POST /follow-ups/{task_id}/retry", s.handleRetryFollowUp)
	s.mux.HandleFunc("POST /task-graphs/{id}/reevaluate", s.handleReevaluateTaskGraph)
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

func (s *Server) handleListTaskGraphs(w http.ResponseWriter, r *http.Request) {
	views, err := s.query.ListTaskGraphs(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleGetTaskGraph(w http.ResponseWriter, r *http.Request) {
	view, err := s.query.GetTaskGraph(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetWorkflowReplay(w http.ResponseWriter, r *http.Request) {
	view, err := s.replay.Query(r.Context(), runtimeReplayQueryWorkflow(r.PathValue("id")))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetTaskGraphReplay(w http.ResponseWriter, r *http.Request) {
	view, err := s.replay.Query(r.Context(), runtimeReplayQueryTaskGraph(r.PathValue("id")))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetExecutionReplay(w http.ResponseWriter, r *http.Request) {
	view, err := s.replay.Query(r.Context(), runtimeReplayQueryExecution(r.PathValue("id")))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetApprovalReplay(w http.ResponseWriter, r *http.Request) {
	view, err := s.replay.Query(r.Context(), runtimeReplayQueryApproval(r.PathValue("id")))
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
	comparison, err := s.replay.Compare(r.Context(), left, right)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, comparison)
}

func (s *Server) handleListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	items, err := s.query.ListPendingApprovals(r.Context())
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
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
	result, err := s.operator.ApproveTask(r.Context(), runtime.ApproveTaskCommand{
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
	result, err := s.operator.DenyTask(r.Context(), runtime.DenyTaskCommand{
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
	result, err := s.operator.ResumeFollowUpTask(r.Context(), runtime.ResumeFollowUpTaskCommand{
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
	result, err := s.operator.RetryFailedFollowUpTask(r.Context(), runtime.RetryFailedFollowUpTaskCommand{
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
	activation, result, err := s.service.ReevaluateTaskGraph(r.Context(), runtime.ReevaluateTaskGraphCommand{
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

func runtimeReplayQueryWorkflow(id string) observability.ReplayQuery {
	return observability.ReplayQuery{WorkflowID: id}
}

func runtimeReplayQueryTaskGraph(id string) observability.ReplayQuery {
	return observability.ReplayQuery{TaskGraphID: id}
}

func runtimeReplayQueryExecution(id string) observability.ReplayQuery {
	return observability.ReplayQuery{ExecutionID: id}
}

func runtimeReplayQueryApproval(id string) observability.ReplayQuery {
	return observability.ReplayQuery{ApprovalID: id}
}

func parseReplayCompareQuery(raw string) (observability.ReplayQuery, error) {
	scope, id, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || strings.TrimSpace(id) == "" {
		return observability.ReplayQuery{}, fmt.Errorf("replay compare query must use scope:id")
	}
	switch strings.TrimSpace(scope) {
	case "workflow":
		return runtimeReplayQueryWorkflow(strings.TrimSpace(id)), nil
	case "task_graph":
		return runtimeReplayQueryTaskGraph(strings.TrimSpace(id)), nil
	case "execution":
		return runtimeReplayQueryExecution(strings.TrimSpace(id)), nil
	case "approval":
		return runtimeReplayQueryApproval(strings.TrimSpace(id)), nil
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
