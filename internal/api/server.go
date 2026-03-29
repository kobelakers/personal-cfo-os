package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/runtime"
)

type Server struct {
	query    *runtime.QueryService
	operator *runtime.OperatorService
	service  *runtime.Service
	mux      *http.ServeMux
}

func NewServer(query *runtime.QueryService, operator *runtime.OperatorService, service *runtime.Service) *Server {
	server := &Server{
		query:    query,
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
