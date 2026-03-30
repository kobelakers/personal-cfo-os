package runtime

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type sqliteTaskGraphStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteTaskGraphStore) Save(snapshot TaskGraphSnapshot) error {
	if snapshot.Version == 0 {
		snapshot.Version = 1
	}
	tx, err := s.db.begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := saveTaskGraphTx(tx, s.db, snapshot, false, 0); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqliteTaskGraphStore) Update(snapshot TaskGraphSnapshot, expectedVersion int64) error {
	tx, err := s.db.begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := saveTaskGraphTx(tx, s.db, snapshot, true, expectedVersion); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqliteTaskGraphStore) Load(graphID string) (TaskGraphSnapshot, bool, error) {
	row := s.db.db.QueryRow(`
		SELECT parent_workflow_id, parent_task_id, trigger_source, generated_at, version,
		       base_state_snapshot_ref, latest_committed_state_ref, graph_json, registered_at
		FROM task_graphs
		WHERE graph_id = ?
	`, graphID)
	var (
		parentWorkflowID string
		parentTaskID     string
		triggerSource    string
		generatedAt      string
		version          int64
		baseRef          string
		committedRef     string
		graphJSON        string
		registeredAt     string
	)
	if err := row.Scan(&parentWorkflowID, &parentTaskID, &triggerSource, &generatedAt, &version, &baseRef, &committedRef, &graphJSON, &registeredAt); err != nil {
		if errorsIsNoRows(err) {
			return TaskGraphSnapshot{}, false, nil
		}
		return TaskGraphSnapshot{}, false, err
	}
	var graph taskspec.TaskGraph
	if err := unmarshalJSON(graphJSON, &graph); err != nil {
		return TaskGraphSnapshot{}, false, err
	}
	records, err := loadFollowUpTaskRecords(s.db.db, graphID)
	if err != nil {
		return TaskGraphSnapshot{}, false, err
	}
	graph.Dependencies, err = loadTaskDependencies(s.db.db, graphID)
	if err != nil {
		return TaskGraphSnapshot{}, false, err
	}
	baseSnapshot, ok, err := s.db.loadStateSnapshot(baseRef)
	if err != nil {
		return TaskGraphSnapshot{}, false, err
	}
	if !ok {
		return TaskGraphSnapshot{}, false, &NotFoundError{Resource: "state_snapshot", ID: baseRef}
	}
	committedSnapshot, ok, err := s.db.loadStateSnapshot(committedRef)
	if err != nil {
		return TaskGraphSnapshot{}, false, err
	}
	if !ok {
		return TaskGraphSnapshot{}, false, &NotFoundError{Resource: "state_snapshot", ID: committedRef}
	}
	spawned, deferred := deriveSpawnedAndDeferred(records, mustParseTime(registeredAt))
	return TaskGraphSnapshot{
		Graph:                        graph,
		Version:                      version,
		RegisteredTasks:              records,
		Spawned:                      spawned,
		Deferred:                     deferred,
		BaseStateSnapshot:            baseSnapshot,
		BaseStateSnapshotRef:         baseRef,
		LatestCommittedStateSnapshot: committedSnapshot,
		LatestCommittedStateRef:      committedRef,
		RegisteredAt:                 mustParseTime(registeredAt),
	}, true, nil
}

func (s *sqliteTaskGraphStore) List() ([]TaskGraphSnapshot, error) {
	rows, err := s.db.db.Query(`SELECT graph_id FROM task_graphs ORDER BY registered_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TaskGraphSnapshot, 0)
	for rows.Next() {
		var graphID string
		if err := rows.Scan(&graphID); err != nil {
			return nil, err
		}
		snapshot, ok, err := s.Load(graphID)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, snapshot)
		}
	}
	return result, rows.Err()
}

func (s *sqliteTaskGraphStore) SaveStateSnapshot(graphID string, workflowID string, taskID string, kind string, snapshot state.StateSnapshot) (string, error) {
	return s.db.saveStateSnapshot(graphID, workflowID, taskID, kind, snapshot)
}

func (s *sqliteTaskGraphStore) LoadStateSnapshot(snapshotRef string) (state.StateSnapshot, bool, error) {
	return s.db.loadStateSnapshot(snapshotRef)
}

type sqliteTaskExecutionStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteTaskExecutionStore) Save(record TaskExecutionRecord) error {
	if record.ExecutionID == "" {
		return fmt.Errorf("task execution requires execution_id")
	}
	if record.Version == 0 {
		record.Version = 1
	}
	recordJSON, err := marshalJSON(record)
	if err != nil {
		return err
	}
	_, err = s.db.db.Exec(`
		INSERT INTO task_executions (
			execution_id, graph_id, task_id, workflow_id, status, started_at, last_transition_at, version, record_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ExecutionID, record.ParentGraphID, record.TaskID, record.WorkflowID, string(record.Status), record.StartedAt.UTC().Format(time.RFC3339Nano), record.LastTransitionAt.UTC().Format(time.RFC3339Nano), record.Version, recordJSON)
	return mapSQLiteConstraint(err, "task_execution", record.ExecutionID)
}

func (s *sqliteTaskExecutionStore) Update(record TaskExecutionRecord, expectedVersion int64) error {
	record.Version = expectedVersion + 1
	recordJSON, err := marshalJSON(record)
	if err != nil {
		return err
	}
	result, err := s.db.db.Exec(`
		UPDATE task_executions
		SET graph_id = ?, task_id = ?, workflow_id = ?, status = ?, started_at = ?, last_transition_at = ?, version = ?, record_json = ?
		WHERE execution_id = ? AND version = ?
	`, record.ParentGraphID, record.TaskID, record.WorkflowID, string(record.Status), record.StartedAt.UTC().Format(time.RFC3339Nano), record.LastTransitionAt.UTC().Format(time.RFC3339Nano), record.Version, recordJSON, record.ExecutionID, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return &ConflictError{Resource: "task_execution", ID: record.ExecutionID, Reason: fmt.Sprintf("expected version %d", expectedVersion)}
	}
	return nil
}

func (s *sqliteTaskExecutionStore) Load(executionID string) (TaskExecutionRecord, bool, error) {
	row := s.db.db.QueryRow(`SELECT record_json FROM task_executions WHERE execution_id = ?`, executionID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return TaskExecutionRecord{}, false, nil
		}
		return TaskExecutionRecord{}, false, err
	}
	var record TaskExecutionRecord
	if err := unmarshalJSON(raw, &record); err != nil {
		return TaskExecutionRecord{}, false, err
	}
	return record, true, nil
}

func (s *sqliteTaskExecutionStore) LoadLatestByTask(graphID string, taskID string) (TaskExecutionRecord, bool, error) {
	row := s.db.db.QueryRow(`
		SELECT record_json
		FROM task_executions
		WHERE graph_id = ? AND task_id = ?
		ORDER BY started_at DESC, execution_id DESC
		LIMIT 1
	`, graphID, taskID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return TaskExecutionRecord{}, false, nil
		}
		return TaskExecutionRecord{}, false, err
	}
	var record TaskExecutionRecord
	if err := unmarshalJSON(raw, &record); err != nil {
		return TaskExecutionRecord{}, false, err
	}
	return record, true, nil
}

func (s *sqliteTaskExecutionStore) ListByGraph(graphID string) ([]TaskExecutionRecord, error) {
	rows, err := s.db.db.Query(`
		SELECT record_json
		FROM task_executions
		WHERE graph_id = ?
		ORDER BY started_at ASC, execution_id ASC
	`, graphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TaskExecutionRecord, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record TaskExecutionRecord
		if err := unmarshalJSON(raw, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

type sqliteApprovalStateStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteApprovalStateStore) Save(record ApprovalStateRecord) error {
	if record.Version == 0 {
		record.Version = 1
	}
	requiredRolesJSON, err := marshalJSON(record.RequiredRoles)
	if err != nil {
		return err
	}
	_, err = s.db.db.Exec(`
		INSERT INTO approvals (
			approval_id, graph_id, task_id, workflow_id, execution_id, requested_action, required_roles_json,
			requested_at, deadline, status, resolved_at, resolved_by, resolution_note, version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ApprovalID, record.GraphID, record.TaskID, record.WorkflowID, record.ExecutionID, record.RequestedAction, requiredRolesJSON, record.RequestedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.Deadline), string(record.Status), nullableTime(record.ResolvedAt), record.ResolvedBy, record.ResolutionNote, record.Version)
	return mapSQLiteConstraint(err, "approval", record.ApprovalID)
}

func (s *sqliteApprovalStateStore) Update(record ApprovalStateRecord, expectedVersion int64) error {
	record.Version = expectedVersion + 1
	requiredRolesJSON, err := marshalJSON(record.RequiredRoles)
	if err != nil {
		return err
	}
	result, err := s.db.db.Exec(`
		UPDATE approvals
		SET graph_id = ?, task_id = ?, workflow_id = ?, execution_id = ?, requested_action = ?, required_roles_json = ?,
		    requested_at = ?, deadline = ?, status = ?, resolved_at = ?, resolved_by = ?, resolution_note = ?, version = ?
		WHERE approval_id = ? AND version = ?
	`, record.GraphID, record.TaskID, record.WorkflowID, record.ExecutionID, record.RequestedAction, requiredRolesJSON, record.RequestedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.Deadline), string(record.Status), nullableTime(record.ResolvedAt), record.ResolvedBy, record.ResolutionNote, record.Version, record.ApprovalID, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return &ConflictError{Resource: "approval", ID: record.ApprovalID, Reason: fmt.Sprintf("expected version %d", expectedVersion)}
	}
	return nil
}

func (s *sqliteApprovalStateStore) Load(approvalID string) (ApprovalStateRecord, bool, error) {
	row := s.db.db.QueryRow(`
		SELECT graph_id, task_id, workflow_id, execution_id, requested_action, required_roles_json, requested_at,
		       deadline, status, resolved_at, resolved_by, resolution_note, version
		FROM approvals
		WHERE approval_id = ?
	`, approvalID)
	var (
		record            ApprovalStateRecord
		requiredRolesJSON string
		requestedAt       string
		deadline          sql.NullString
		resolvedAt        sql.NullString
		status            string
	)
	record.ApprovalID = approvalID
	if err := row.Scan(&record.GraphID, &record.TaskID, &record.WorkflowID, &record.ExecutionID, &record.RequestedAction, &requiredRolesJSON, &requestedAt, &deadline, &status, &resolvedAt, &record.ResolvedBy, &record.ResolutionNote, &record.Version); err != nil {
		if errorsIsNoRows(err) {
			return ApprovalStateRecord{}, false, nil
		}
		return ApprovalStateRecord{}, false, err
	}
	if err := unmarshalJSON(requiredRolesJSON, &record.RequiredRoles); err != nil {
		return ApprovalStateRecord{}, false, err
	}
	record.RequestedAt = mustParseTime(requestedAt)
	record.Status = ApprovalStatus(status)
	if deadline.Valid {
		parsed := mustParseTime(deadline.String)
		record.Deadline = &parsed
	}
	if resolvedAt.Valid {
		parsed := mustParseTime(resolvedAt.String)
		record.ResolvedAt = &parsed
	}
	return record, true, nil
}

func (s *sqliteApprovalStateStore) ListPending() ([]ApprovalStateRecord, error) {
	rows, err := s.db.db.Query(`SELECT approval_id FROM approvals WHERE status = ? ORDER BY requested_at ASC`, string(ApprovalStatusPending))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ApprovalStateRecord, 0)
	for rows.Next() {
		var approvalID string
		if err := rows.Scan(&approvalID); err != nil {
			return nil, err
		}
		record, ok, err := s.Load(approvalID)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, record)
		}
	}
	return result, rows.Err()
}

func (s *sqliteApprovalStateStore) LoadByTask(graphID string, taskID string) (ApprovalStateRecord, bool, error) {
	row := s.db.db.QueryRow(`
		SELECT approval_id
		FROM approvals
		WHERE graph_id = ? AND task_id = ?
		ORDER BY requested_at DESC, approval_id DESC
		LIMIT 1
	`, graphID, taskID)
	var approvalID string
	if err := row.Scan(&approvalID); err != nil {
		if errorsIsNoRows(err) {
			return ApprovalStateRecord{}, false, nil
		}
		return ApprovalStateRecord{}, false, err
	}
	return s.Load(approvalID)
}

type sqliteOperatorActionStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteOperatorActionStore) Save(record OperatorActionRecord) error {
	rolesJSON, err := marshalJSON(record.Roles)
	if err != nil {
		return err
	}
	_, err = s.db.db.Exec(`
		INSERT INTO operator_actions (
			action_id, request_id, action_type, actor, roles_json, graph_id, task_id, approval_id, workflow_id,
			status, note, requested_at, applied_at, failure_summary, expected_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ActionID, record.RequestID, string(record.ActionType), record.Actor, rolesJSON, record.GraphID, record.TaskID, record.ApprovalID, record.WorkflowID, string(record.Status), record.Note, record.RequestedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.AppliedAt), record.FailureSummary, record.ExpectedVersion)
	return mapSQLiteConstraint(err, "operator_action_request", record.RequestID)
}

func (s *sqliteOperatorActionStore) LoadByRequestID(requestID string) (OperatorActionRecord, bool, error) {
	row := s.db.db.QueryRow(`
		SELECT action_id, action_type, actor, roles_json, graph_id, task_id, approval_id, workflow_id,
		       status, note, requested_at, applied_at, failure_summary, expected_version
		FROM operator_actions
		WHERE request_id = ?
	`, requestID)
	var (
		record      OperatorActionRecord
		rolesJSON   string
		actionType  string
		status      string
		requestedAt string
		appliedAt   sql.NullString
	)
	record.RequestID = requestID
	if err := row.Scan(&record.ActionID, &actionType, &record.Actor, &rolesJSON, &record.GraphID, &record.TaskID, &record.ApprovalID, &record.WorkflowID, &status, &record.Note, &requestedAt, &appliedAt, &record.FailureSummary, &record.ExpectedVersion); err != nil {
		if errorsIsNoRows(err) {
			return OperatorActionRecord{}, false, nil
		}
		return OperatorActionRecord{}, false, err
	}
	if err := unmarshalJSON(rolesJSON, &record.Roles); err != nil {
		return OperatorActionRecord{}, false, err
	}
	record.ActionType = OperatorActionType(actionType)
	record.Status = OperatorActionStatus(status)
	record.RequestedAt = mustParseTime(requestedAt)
	if appliedAt.Valid {
		parsed := mustParseTime(appliedAt.String)
		record.AppliedAt = &parsed
	}
	return record, true, nil
}

func (s *sqliteOperatorActionStore) ListByTask(graphID string, taskID string) ([]OperatorActionRecord, error) {
	rows, err := s.db.db.Query(`
		SELECT request_id
		FROM operator_actions
		WHERE graph_id = ? AND task_id = ?
		ORDER BY requested_at ASC, request_id ASC
	`, graphID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]OperatorActionRecord, 0)
	for rows.Next() {
		var requestID string
		if err := rows.Scan(&requestID); err != nil {
			return nil, err
		}
		record, ok, err := s.LoadByRequestID(requestID)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, record)
		}
	}
	return result, rows.Err()
}

type sqliteCheckpointStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteCheckpointStore) Save(checkpoint CheckpointRecord) error {
	_, err := s.db.db.Exec(`
		INSERT INTO checkpoints (
			checkpoint_id, workflow_id, state, resume_state, state_version, summary, captured_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(checkpoint_id) DO UPDATE SET
			workflow_id = excluded.workflow_id,
			state = excluded.state,
			resume_state = excluded.resume_state,
			state_version = excluded.state_version,
			summary = excluded.summary,
			captured_at = excluded.captured_at
	`, checkpoint.ID, checkpoint.WorkflowID, string(checkpoint.State), string(checkpoint.ResumeState), checkpoint.StateVersion, checkpoint.Summary, checkpoint.CapturedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteCheckpointStore) Load(workflowID string, checkpointID string) (CheckpointRecord, error) {
	row := s.db.db.QueryRow(`
		SELECT workflow_id, state, resume_state, state_version, summary, captured_at
		FROM checkpoints
		WHERE checkpoint_id = ?
	`, checkpointID)
	var (
		record      CheckpointRecord
		stateValue  string
		resumeState string
		capturedAt  string
	)
	record.ID = checkpointID
	if err := row.Scan(&record.WorkflowID, &stateValue, &resumeState, &record.StateVersion, &record.Summary, &capturedAt); err != nil {
		if errorsIsNoRows(err) {
			return CheckpointRecord{}, &NotFoundError{Resource: "checkpoint", ID: checkpointID}
		}
		return CheckpointRecord{}, err
	}
	if workflowID != "" && record.WorkflowID != workflowID {
		return CheckpointRecord{}, &ConflictError{Resource: "checkpoint", ID: checkpointID, Reason: "workflow id mismatch"}
	}
	record.State = WorkflowExecutionState(stateValue)
	record.ResumeState = WorkflowExecutionState(resumeState)
	record.CapturedAt = mustParseTime(capturedAt)
	return record, nil
}

func (s *sqliteCheckpointStore) SaveResumeToken(token ResumeToken) error {
	_, err := s.db.db.Exec(`
		INSERT INTO resume_tokens (
			token, workflow_id, checkpoint_id, issued_at, expires_at
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(token) DO UPDATE SET
			workflow_id = excluded.workflow_id,
			checkpoint_id = excluded.checkpoint_id,
			issued_at = excluded.issued_at,
			expires_at = excluded.expires_at
	`, token.Token, token.WorkflowID, token.CheckpointID, token.IssuedAt.UTC().Format(time.RFC3339Nano), token.ExpiresAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteCheckpointStore) LoadResumeToken(token string) (ResumeToken, error) {
	row := s.db.db.QueryRow(`
		SELECT workflow_id, checkpoint_id, issued_at, expires_at
		FROM resume_tokens
		WHERE token = ?
	`, token)
	var (
		record    ResumeToken
		issuedAt  string
		expiresAt string
	)
	record.Token = token
	if err := row.Scan(&record.WorkflowID, &record.CheckpointID, &issuedAt, &expiresAt); err != nil {
		if errorsIsNoRows(err) {
			return ResumeToken{}, &NotFoundError{Resource: "resume_token", ID: token}
		}
		return ResumeToken{}, err
	}
	record.IssuedAt = mustParseTime(issuedAt)
	record.ExpiresAt = mustParseTime(expiresAt)
	return record, nil
}

func (s *sqliteCheckpointStore) SavePayload(checkpointID string, payload CheckpointPayloadEnvelope) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	payloadJSON, err := marshalJSON(payload)
	if err != nil {
		return err
	}
	result, err := s.db.db.Exec(`
		UPDATE checkpoints
		SET payload_kind = ?, payload_json = ?
		WHERE checkpoint_id = ?
	`, string(payload.Kind), payloadJSON, checkpointID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return &NotFoundError{Resource: "checkpoint", ID: checkpointID}
	}
	return nil
}

func (s *sqliteCheckpointStore) LoadPayload(checkpointID string) (CheckpointPayloadEnvelope, error) {
	row := s.db.db.QueryRow(`SELECT payload_json FROM checkpoints WHERE checkpoint_id = ?`, checkpointID)
	var raw sql.NullString
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return CheckpointPayloadEnvelope{}, &NotFoundError{Resource: "checkpoint", ID: checkpointID}
		}
		return CheckpointPayloadEnvelope{}, err
	}
	if !raw.Valid || raw.String == "" {
		return CheckpointPayloadEnvelope{}, ErrCheckpointPayloadAbsent
	}
	var payload CheckpointPayloadEnvelope
	if err := unmarshalJSON(raw.String, &payload); err != nil {
		return CheckpointPayloadEnvelope{}, err
	}
	return payload, nil
}

type sqliteReplayStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteReplayStore) Append(event ReplayEventRecord) error {
	artifactIDsJSON, err := marshalJSON(event.ArtifactIDs)
	if err != nil {
		return err
	}
	_, err = s.db.db.Exec(`
		INSERT INTO replay_events (
			event_id, root_correlation_id, parent_workflow_id, workflow_id, graph_id, task_id, approval_id,
			execution_id, action_type, summary, occurred_at, details_json, committed_state_ref, updated_state_ref,
			artifact_ids_json, operator_action_id, checkpoint_id, resume_token
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.EventID, event.RootCorrelationID, event.ParentWorkflowID, event.WorkflowID, event.GraphID, event.TaskID, event.ApprovalID, event.ExecutionID, event.ActionType, event.Summary, event.OccurredAt.UTC().Format(time.RFC3339Nano), event.DetailsJSON, event.CommittedStateRef, event.UpdatedStateRef, artifactIDsJSON, event.OperatorActionID, event.CheckpointID, event.ResumeToken)
	return mapSQLiteConstraint(err, "replay_event", event.EventID)
}

func (s *sqliteReplayStore) ListByGraph(graphID string) ([]ReplayEventRecord, error) {
	return s.list(`SELECT event_id FROM replay_events WHERE graph_id = ? ORDER BY occurred_at ASC, event_id ASC`, graphID)
}

func (s *sqliteReplayStore) ListByTask(taskID string) ([]ReplayEventRecord, error) {
	return s.list(`SELECT event_id FROM replay_events WHERE task_id = ? ORDER BY occurred_at ASC, event_id ASC`, taskID)
}

func (s *sqliteReplayStore) ListByWorkflow(workflowID string) ([]ReplayEventRecord, error) {
	return s.list(`SELECT event_id FROM replay_events WHERE workflow_id = ? ORDER BY occurred_at ASC, event_id ASC`, workflowID)
}

func (s *sqliteReplayStore) list(query string, arg string) ([]ReplayEventRecord, error) {
	rows, err := s.db.db.Query(query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ReplayEventRecord, 0)
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return nil, err
		}
		row := s.db.db.QueryRow(`
			SELECT root_correlation_id, parent_workflow_id, workflow_id, graph_id, task_id, approval_id, execution_id,
			       action_type, summary, occurred_at, details_json, committed_state_ref, updated_state_ref,
			       artifact_ids_json, operator_action_id, checkpoint_id, resume_token
			FROM replay_events
			WHERE event_id = ?
		`, eventID)
		var (
			record          ReplayEventRecord
			occurredAt      string
			artifactIDsJSON string
		)
		record.EventID = eventID
		if err := row.Scan(&record.RootCorrelationID, &record.ParentWorkflowID, &record.WorkflowID, &record.GraphID, &record.TaskID, &record.ApprovalID, &record.ExecutionID, &record.ActionType, &record.Summary, &occurredAt, &record.DetailsJSON, &record.CommittedStateRef, &record.UpdatedStateRef, &artifactIDsJSON, &record.OperatorActionID, &record.CheckpointID, &record.ResumeToken); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(artifactIDsJSON, &record.ArtifactIDs); err != nil {
			return nil, err
		}
		record.OccurredAt = mustParseTime(occurredAt)
		result = append(result, record)
	}
	return result, rows.Err()
}

type sqliteWorkflowRunStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteWorkflowRunStore) Save(record WorkflowRunRecord) error {
	_, err := s.db.db.Exec(`
		INSERT INTO workflow_runs (
			workflow_id, task_id, intent, runtime_state, failure_category, failure_summary, approval_id,
			checkpoint_id, resume_token, task_graph_id, root_correlation_id, summary, started_at, updated_at, record_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workflow_id) DO UPDATE SET
			task_id = excluded.task_id,
			intent = excluded.intent,
			runtime_state = excluded.runtime_state,
			failure_category = excluded.failure_category,
			failure_summary = excluded.failure_summary,
			approval_id = excluded.approval_id,
			checkpoint_id = excluded.checkpoint_id,
			resume_token = excluded.resume_token,
			task_graph_id = excluded.task_graph_id,
			root_correlation_id = excluded.root_correlation_id,
			summary = excluded.summary,
			started_at = excluded.started_at,
			updated_at = excluded.updated_at,
			record_json = excluded.record_json
	`, record.WorkflowID, record.TaskID, record.Intent, string(record.RuntimeState), string(record.FailureCategory), record.FailureSummary, record.ApprovalID, record.CheckpointID, record.ResumeToken, record.TaskGraphID, record.RootCorrelationID, record.Summary, record.StartedAt.UTC().Format(time.RFC3339Nano), record.UpdatedAt.UTC().Format(time.RFC3339Nano), record.RecordJSON)
	return err
}

func (s *sqliteWorkflowRunStore) Load(workflowID string) (WorkflowRunRecord, bool, error) {
	row := s.db.db.QueryRow(`
		SELECT task_id, intent, runtime_state, failure_category, failure_summary, approval_id, checkpoint_id,
		       resume_token, task_graph_id, root_correlation_id, summary, started_at, updated_at, record_json
		FROM workflow_runs
		WHERE workflow_id = ?
	`, workflowID)
	var (
		record    WorkflowRunRecord
		state     string
		failure   string
		startedAt string
		updatedAt string
	)
	record.WorkflowID = workflowID
	if err := row.Scan(&record.TaskID, &record.Intent, &state, &failure, &record.FailureSummary, &record.ApprovalID, &record.CheckpointID, &record.ResumeToken, &record.TaskGraphID, &record.RootCorrelationID, &record.Summary, &startedAt, &updatedAt, &record.RecordJSON); err != nil {
		if errorsIsNoRows(err) {
			return WorkflowRunRecord{}, false, nil
		}
		return WorkflowRunRecord{}, false, err
	}
	record.RuntimeState = WorkflowExecutionState(state)
	record.FailureCategory = FailureCategory(failure)
	record.StartedAt = mustParseTime(startedAt)
	record.UpdatedAt = mustParseTime(updatedAt)
	return record, true, nil
}

func (s *sqliteWorkflowRunStore) List() ([]WorkflowRunRecord, error) {
	rows, err := s.db.db.Query(`SELECT workflow_id FROM workflow_runs ORDER BY updated_at ASC, workflow_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]WorkflowRunRecord, 0)
	for rows.Next() {
		var workflowID string
		if err := rows.Scan(&workflowID); err != nil {
			return nil, err
		}
		record, ok, err := s.Load(workflowID)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, record)
		}
	}
	return result, rows.Err()
}

type sqliteReplayProjectionStore struct {
	db *SQLiteRuntimeDB
}

func marshalStringSlice(values []string) (string, error) {
	if len(values) == 0 {
		return "[]", nil
	}
	return marshalJSON(values)
}

func unmarshalStringSlice(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var values []string
	if err := unmarshalJSON(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (s *sqliteReplayProjectionStore) SaveWorkflowProjection(record WorkflowReplayProjection) error {
	reasonsJSON, err := marshalStringSlice(record.DegradationReasons)
	if err != nil {
		return err
	}
	_, err = s.db.db.Exec(`
		INSERT INTO workflow_replay_projections (
			workflow_id, task_id, intent, runtime_state, failure_category, approval_id, bundle_artifact_id, summary_artifact_id,
			projection_status, schema_version, degradation_reasons_json, summary_json, explanation_json, compare_input_json,
			updated_at, projection_freshness
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workflow_id) DO UPDATE SET
			task_id = excluded.task_id,
			intent = excluded.intent,
			runtime_state = excluded.runtime_state,
			failure_category = excluded.failure_category,
			approval_id = excluded.approval_id,
			bundle_artifact_id = excluded.bundle_artifact_id,
			summary_artifact_id = excluded.summary_artifact_id,
			projection_status = excluded.projection_status,
			schema_version = excluded.schema_version,
			degradation_reasons_json = excluded.degradation_reasons_json,
			summary_json = excluded.summary_json,
			explanation_json = excluded.explanation_json,
			compare_input_json = excluded.compare_input_json,
			updated_at = excluded.updated_at,
			projection_freshness = excluded.projection_freshness
	`, record.WorkflowID, record.TaskID, record.Intent, string(record.RuntimeState), string(record.FailureCategory), record.ApprovalID, record.BundleArtifactID, record.SummaryArtifactID, string(record.ProjectionStatus), record.SchemaVersion, reasonsJSON, record.SummaryJSON, record.ExplanationJSON, record.CompareInputJSON, record.UpdatedAt.UTC().Format(time.RFC3339Nano), record.ProjectionFreshness.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteReplayProjectionStore) SaveTaskGraphProjection(record TaskGraphReplayProjection) error {
	reasonsJSON, err := marshalStringSlice(record.DegradationReasons)
	if err != nil {
		return err
	}
	_, err = s.db.db.Exec(`
		INSERT INTO task_graph_replay_projections (
			graph_id, parent_workflow_id, parent_task_id, runtime_state, pending_approval_id, bundle_artifact_id, summary_artifact_id,
			projection_status, schema_version, degradation_reasons_json, summary_json, explanation_json, compare_input_json,
			updated_at, projection_freshness
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(graph_id) DO UPDATE SET
			parent_workflow_id = excluded.parent_workflow_id,
			parent_task_id = excluded.parent_task_id,
			runtime_state = excluded.runtime_state,
			pending_approval_id = excluded.pending_approval_id,
			bundle_artifact_id = excluded.bundle_artifact_id,
			summary_artifact_id = excluded.summary_artifact_id,
			projection_status = excluded.projection_status,
			schema_version = excluded.schema_version,
			degradation_reasons_json = excluded.degradation_reasons_json,
			summary_json = excluded.summary_json,
			explanation_json = excluded.explanation_json,
			compare_input_json = excluded.compare_input_json,
			updated_at = excluded.updated_at,
			projection_freshness = excluded.projection_freshness
	`, record.GraphID, record.ParentWorkflowID, record.ParentTaskID, string(record.RuntimeState), record.PendingApprovalID, record.BundleArtifactID, record.SummaryArtifactID, string(record.ProjectionStatus), record.SchemaVersion, reasonsJSON, record.SummaryJSON, record.ExplanationJSON, record.CompareInputJSON, record.UpdatedAt.UTC().Format(time.RFC3339Nano), record.ProjectionFreshness.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteReplayProjectionStore) SaveBuild(record ReplayProjectionBuildRecord) error {
	reasonsJSON, err := marshalStringSlice(record.DegradationReasons)
	if err != nil {
		return err
	}
	_, err = s.db.db.Exec(`
		INSERT INTO replay_projection_builds (
			scope_kind, scope_id, schema_version, status, degradation_reasons_json, built_at, source_event_count, source_artifact_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_kind, scope_id) DO UPDATE SET
			schema_version = excluded.schema_version,
			status = excluded.status,
			degradation_reasons_json = excluded.degradation_reasons_json,
			built_at = excluded.built_at,
			source_event_count = excluded.source_event_count,
			source_artifact_count = excluded.source_artifact_count
	`, string(record.ScopeKind), record.ScopeID, record.SchemaVersion, string(record.Status), reasonsJSON, record.BuiltAt.UTC().Format(time.RFC3339Nano), record.SourceEventCount, record.SourceArtifactCount)
	return err
}

func (s *sqliteReplayProjectionStore) ReplaceProvenance(scope ReplayProjectionScope, nodes []ProvenanceNodeRecord, edges []ProvenanceEdgeRecord) error {
	tx, err := s.db.begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM replay_provenance_nodes WHERE scope_kind = ? AND scope_id = ?`, string(scope.ScopeKind), scope.ScopeID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM replay_provenance_edges WHERE scope_kind = ? AND scope_id = ?`, string(scope.ScopeKind), scope.ScopeID); err != nil {
		return err
	}
	for _, node := range nodes {
		if _, err := tx.Exec(`
			INSERT INTO replay_provenance_nodes(scope_kind, scope_id, node_id, node_type, ref_id, label, summary, attributes_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, string(scope.ScopeKind), scope.ScopeID, node.NodeID, node.NodeType, node.RefID, node.Label, node.Summary, node.AttributesJSON); err != nil {
			return err
		}
	}
	for _, edge := range edges {
		if _, err := tx.Exec(`
			INSERT INTO replay_provenance_edges(scope_kind, scope_id, edge_id, from_node_id, to_node_id, edge_type, reason, attributes_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, string(scope.ScopeKind), scope.ScopeID, edge.EdgeID, edge.FromNodeID, edge.ToNodeID, edge.EdgeType, edge.Reason, edge.AttributesJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteReplayProjectionStore) ReplaceExecutionAttributions(scope ReplayProjectionScope, records []ExecutionAttributionRecord) error {
	tx, err := s.db.begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM replay_execution_attributions WHERE scope_kind = ? AND scope_id = ?`, string(scope.ScopeKind), scope.ScopeID); err != nil {
		return err
	}
	for _, item := range records {
		if _, err := tx.Exec(`
			INSERT INTO replay_execution_attributions(scope_kind, scope_id, execution_id, category, summary, source_refs_json, details_json)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, string(scope.ScopeKind), scope.ScopeID, item.ExecutionID, item.Category, item.Summary, item.SourceRefsJSON, item.DetailsJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteReplayProjectionStore) ReplaceFailureAttributions(scope ReplayProjectionScope, records []FailureAttributionRecord) error {
	tx, err := s.db.begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM replay_failure_attributions WHERE scope_kind = ? AND scope_id = ?`, string(scope.ScopeKind), scope.ScopeID); err != nil {
		return err
	}
	for _, item := range records {
		if _, err := tx.Exec(`
			INSERT INTO replay_failure_attributions(scope_kind, scope_id, attribution_id, failure_category, reason_code, summary, related_kind, related_id, source_refs_json, details_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, string(scope.ScopeKind), scope.ScopeID, item.AttributionID, item.FailureCategory, item.ReasonCode, item.Summary, item.RelatedKind, item.RelatedID, item.SourceRefsJSON, item.DetailsJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteReplayProjectionStore) LoadWorkflowProjection(workflowID string) (WorkflowReplayProjection, bool, error) {
	row := s.db.db.QueryRow(`
		SELECT task_id, intent, runtime_state, failure_category, approval_id, bundle_artifact_id, summary_artifact_id,
		       projection_status, schema_version, degradation_reasons_json, summary_json, explanation_json, compare_input_json,
		       updated_at, projection_freshness
		FROM workflow_replay_projections
		WHERE workflow_id = ?
	`, workflowID)
	var (
		record         WorkflowReplayProjection
		state          string
		failure        string
		status         string
		reasonsJSON    string
		updatedAt      string
		freshness      string
	)
	record.WorkflowID = workflowID
	if err := row.Scan(&record.TaskID, &record.Intent, &state, &failure, &record.ApprovalID, &record.BundleArtifactID, &record.SummaryArtifactID, &status, &record.SchemaVersion, &reasonsJSON, &record.SummaryJSON, &record.ExplanationJSON, &record.CompareInputJSON, &updatedAt, &freshness); err != nil {
		if errorsIsNoRows(err) {
			return WorkflowReplayProjection{}, false, nil
		}
		return WorkflowReplayProjection{}, false, err
	}
	record.RuntimeState = WorkflowExecutionState(state)
	record.FailureCategory = FailureCategory(failure)
	record.ProjectionStatus = ReplayProjectionStatus(status)
	reasons, err := unmarshalStringSlice(reasonsJSON)
	if err != nil {
		return WorkflowReplayProjection{}, false, err
	}
	record.DegradationReasons = reasons
	record.UpdatedAt = mustParseTime(updatedAt)
	record.ProjectionFreshness = mustParseTime(freshness)
	return record, true, nil
}

func (s *sqliteReplayProjectionStore) LoadTaskGraphProjection(graphID string) (TaskGraphReplayProjection, bool, error) {
	row := s.db.db.QueryRow(`
		SELECT parent_workflow_id, parent_task_id, runtime_state, pending_approval_id, bundle_artifact_id, summary_artifact_id,
		       projection_status, schema_version, degradation_reasons_json, summary_json, explanation_json, compare_input_json,
		       updated_at, projection_freshness
		FROM task_graph_replay_projections
		WHERE graph_id = ?
	`, graphID)
	var (
		record      TaskGraphReplayProjection
		state       string
		status      string
		reasonsJSON string
		updatedAt   string
		freshness   string
	)
	record.GraphID = graphID
	if err := row.Scan(&record.ParentWorkflowID, &record.ParentTaskID, &state, &record.PendingApprovalID, &record.BundleArtifactID, &record.SummaryArtifactID, &status, &record.SchemaVersion, &reasonsJSON, &record.SummaryJSON, &record.ExplanationJSON, &record.CompareInputJSON, &updatedAt, &freshness); err != nil {
		if errorsIsNoRows(err) {
			return TaskGraphReplayProjection{}, false, nil
		}
		return TaskGraphReplayProjection{}, false, err
	}
	record.RuntimeState = WorkflowExecutionState(state)
	record.ProjectionStatus = ReplayProjectionStatus(status)
	reasons, err := unmarshalStringSlice(reasonsJSON)
	if err != nil {
		return TaskGraphReplayProjection{}, false, err
	}
	record.DegradationReasons = reasons
	record.UpdatedAt = mustParseTime(updatedAt)
	record.ProjectionFreshness = mustParseTime(freshness)
	return record, true, nil
}

func (s *sqliteReplayProjectionStore) LoadBuild(scope ReplayProjectionScope) (ReplayProjectionBuildRecord, bool, error) {
	row := s.db.db.QueryRow(`
		SELECT schema_version, status, degradation_reasons_json, built_at, source_event_count, source_artifact_count
		FROM replay_projection_builds
		WHERE scope_kind = ? AND scope_id = ?
	`, string(scope.ScopeKind), scope.ScopeID)
	var (
		record      ReplayProjectionBuildRecord
		status      string
		reasonsJSON string
		builtAt     string
	)
	record.ScopeKind = scope.ScopeKind
	record.ScopeID = scope.ScopeID
	if err := row.Scan(&record.SchemaVersion, &status, &reasonsJSON, &builtAt, &record.SourceEventCount, &record.SourceArtifactCount); err != nil {
		if errorsIsNoRows(err) {
			return ReplayProjectionBuildRecord{}, false, nil
		}
		return ReplayProjectionBuildRecord{}, false, err
	}
	record.Status = ReplayProjectionStatus(status)
	reasons, err := unmarshalStringSlice(reasonsJSON)
	if err != nil {
		return ReplayProjectionBuildRecord{}, false, err
	}
	record.DegradationReasons = reasons
	record.BuiltAt = mustParseTime(builtAt)
	return record, true, nil
}

func (s *sqliteReplayProjectionStore) ListProvenance(scope ReplayProjectionScope) ([]ProvenanceNodeRecord, []ProvenanceEdgeRecord, error) {
	nodes := make([]ProvenanceNodeRecord, 0)
	nodeRows, err := s.db.db.Query(`
		SELECT node_id, node_type, ref_id, label, summary, attributes_json
		FROM replay_provenance_nodes
		WHERE scope_kind = ? AND scope_id = ?
		ORDER BY node_id ASC
	`, string(scope.ScopeKind), scope.ScopeID)
	if err != nil {
		return nil, nil, err
	}
	defer nodeRows.Close()
	for nodeRows.Next() {
		var node ProvenanceNodeRecord
		node.ScopeKind = scope.ScopeKind
		node.ScopeID = scope.ScopeID
		if err := nodeRows.Scan(&node.NodeID, &node.NodeType, &node.RefID, &node.Label, &node.Summary, &node.AttributesJSON); err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, node)
	}
	if err := nodeRows.Err(); err != nil {
		return nil, nil, err
	}
	edges := make([]ProvenanceEdgeRecord, 0)
	edgeRows, err := s.db.db.Query(`
		SELECT edge_id, from_node_id, to_node_id, edge_type, reason, attributes_json
		FROM replay_provenance_edges
		WHERE scope_kind = ? AND scope_id = ?
		ORDER BY edge_id ASC
	`, string(scope.ScopeKind), scope.ScopeID)
	if err != nil {
		return nil, nil, err
	}
	defer edgeRows.Close()
	for edgeRows.Next() {
		var edge ProvenanceEdgeRecord
		edge.ScopeKind = scope.ScopeKind
		edge.ScopeID = scope.ScopeID
		if err := edgeRows.Scan(&edge.EdgeID, &edge.FromNodeID, &edge.ToNodeID, &edge.EdgeType, &edge.Reason, &edge.AttributesJSON); err != nil {
			return nil, nil, err
		}
		edges = append(edges, edge)
	}
	return nodes, edges, edgeRows.Err()
}

func (s *sqliteReplayProjectionStore) ListExecutionAttributions(scope ReplayProjectionScope) ([]ExecutionAttributionRecord, error) {
	rows, err := s.db.db.Query(`
		SELECT execution_id, category, summary, source_refs_json, details_json
		FROM replay_execution_attributions
		WHERE scope_kind = ? AND scope_id = ?
		ORDER BY execution_id ASC, category ASC
	`, string(scope.ScopeKind), scope.ScopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ExecutionAttributionRecord, 0)
	for rows.Next() {
		var record ExecutionAttributionRecord
		record.ScopeKind = scope.ScopeKind
		record.ScopeID = scope.ScopeID
		if err := rows.Scan(&record.ExecutionID, &record.Category, &record.Summary, &record.SourceRefsJSON, &record.DetailsJSON); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *sqliteReplayProjectionStore) ListFailureAttributions(scope ReplayProjectionScope) ([]FailureAttributionRecord, error) {
	rows, err := s.db.db.Query(`
		SELECT attribution_id, failure_category, reason_code, summary, related_kind, related_id, source_refs_json, details_json
		FROM replay_failure_attributions
		WHERE scope_kind = ? AND scope_id = ?
		ORDER BY attribution_id ASC
	`, string(scope.ScopeKind), scope.ScopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]FailureAttributionRecord, 0)
	for rows.Next() {
		var record FailureAttributionRecord
		record.ScopeKind = scope.ScopeKind
		record.ScopeID = scope.ScopeID
		if err := rows.Scan(&record.AttributionID, &record.FailureCategory, &record.ReasonCode, &record.Summary, &record.RelatedKind, &record.RelatedID, &record.SourceRefsJSON, &record.DetailsJSON); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

type sqliteArtifactMetadataStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteArtifactMetadataStore) SaveArtifact(workflowID string, taskID string, artifact reporting.WorkflowArtifact) error {
	storageRef := artifact.Ref.Location
	if artifact.ContentJSON != "" {
		storageRef = filepath.Join(s.db.artifactDir, artifact.ID+".json")
		if err := os.WriteFile(storageRef, []byte(artifact.ContentJSON), 0o644); err != nil {
			return err
		}
	}
	ref := artifact.Ref
	ref.Location = storageRef
	refJSON, err := marshalJSON(ref)
	if err != nil {
		return err
	}
	_, err = s.db.db.Exec(`
		INSERT INTO workflow_artifacts (
			artifact_id, kind, workflow_id, task_id, produced_by, summary, storage_ref, created_at, ref_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(artifact_id) DO UPDATE SET
			kind = excluded.kind,
			workflow_id = excluded.workflow_id,
			task_id = excluded.task_id,
			produced_by = excluded.produced_by,
			summary = excluded.summary,
			storage_ref = excluded.storage_ref,
			created_at = excluded.created_at,
			ref_json = excluded.ref_json
	`, artifact.ID, string(artifact.Kind), workflowID, taskID, artifact.ProducedBy, artifactSummary(artifact), storageRef, artifact.CreatedAt.UTC().Format(time.RFC3339Nano), refJSON)
	return err
}

func (s *sqliteArtifactMetadataStore) ListArtifactsByTask(taskID string) ([]reporting.WorkflowArtifact, error) {
	rows, err := s.db.db.Query(`
		SELECT artifact_id, kind, workflow_id, task_id, produced_by, summary, storage_ref, created_at, ref_json
		FROM workflow_artifacts
		WHERE task_id = ?
		ORDER BY created_at ASC, artifact_id ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]reporting.WorkflowArtifact, 0)
	for rows.Next() {
		var (
			artifact   reporting.WorkflowArtifact
			kind       string
			summary    string
			storageRef sql.NullString
			createdAt  string
			refJSON    string
		)
		if err := rows.Scan(&artifact.ID, &kind, &artifact.WorkflowID, &artifact.TaskID, &artifact.ProducedBy, &summary, &storageRef, &createdAt, &refJSON); err != nil {
			return nil, err
		}
		artifact.Kind = reporting.ArtifactKind(kind)
		artifact.CreatedAt = mustParseTime(createdAt)
		if err := unmarshalJSON(refJSON, &artifact.Ref); err != nil {
			return nil, err
		}
		if artifact.Ref.Summary == "" {
			artifact.Ref.Summary = summary
		}
		if storageRef.Valid {
			artifact.Ref.Location = storageRef.String
		}
		result = append(result, artifact)
	}
	return result, rows.Err()
}

func (s *sqliteArtifactMetadataStore) ListArtifactsByWorkflow(workflowID string) ([]reporting.WorkflowArtifact, error) {
	rows, err := s.db.db.Query(`
		SELECT artifact_id, kind, workflow_id, task_id, produced_by, summary, storage_ref, created_at, ref_json
		FROM workflow_artifacts
		WHERE workflow_id = ?
		ORDER BY created_at ASC, artifact_id ASC
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]reporting.WorkflowArtifact, 0)
	for rows.Next() {
		var (
			artifact   reporting.WorkflowArtifact
			kind       string
			summary    string
			storageRef sql.NullString
			createdAt  string
			refJSON    string
		)
		if err := rows.Scan(&artifact.ID, &kind, &artifact.WorkflowID, &artifact.TaskID, &artifact.ProducedBy, &summary, &storageRef, &createdAt, &refJSON); err != nil {
			return nil, err
		}
		artifact.Kind = reporting.ArtifactKind(kind)
		artifact.CreatedAt = mustParseTime(createdAt)
		if err := unmarshalJSON(refJSON, &artifact.Ref); err != nil {
			return nil, err
		}
		if artifact.Ref.Summary == "" {
			artifact.Ref.Summary = summary
		}
		if storageRef.Valid {
			artifact.Ref.Location = storageRef.String
		}
		result = append(result, artifact)
	}
	return result, rows.Err()
}

func (s *sqliteArtifactMetadataStore) LoadArtifact(artifactID string) (reporting.WorkflowArtifact, bool, error) {
	row := s.db.db.QueryRow(`
		SELECT kind, workflow_id, task_id, produced_by, summary, storage_ref, created_at, ref_json
		FROM workflow_artifacts
		WHERE artifact_id = ?
	`, artifactID)
	var (
		artifact   reporting.WorkflowArtifact
		kind       string
		summary    string
		storageRef sql.NullString
		createdAt  string
		refJSON    string
	)
	artifact.ID = artifactID
	if err := row.Scan(&kind, &artifact.WorkflowID, &artifact.TaskID, &artifact.ProducedBy, &summary, &storageRef, &createdAt, &refJSON); err != nil {
		if errorsIsNoRows(err) {
			return reporting.WorkflowArtifact{}, false, nil
		}
		return reporting.WorkflowArtifact{}, false, err
	}
	artifact.Kind = reporting.ArtifactKind(kind)
	artifact.CreatedAt = mustParseTime(createdAt)
	if err := unmarshalJSON(refJSON, &artifact.Ref); err != nil {
		return reporting.WorkflowArtifact{}, false, err
	}
	if artifact.Ref.Summary == "" {
		artifact.Ref.Summary = summary
	}
	if storageRef.Valid {
		artifact.Ref.Location = storageRef.String
		payload, err := os.ReadFile(storageRef.String)
		if err == nil {
			artifact.ContentJSON = string(payload)
		}
	}
	return artifact, true, nil
}

func saveTaskGraphTx(tx *sql.Tx, db *SQLiteRuntimeDB, snapshot TaskGraphSnapshot, update bool, expectedVersion int64) error {
	graphJSON, err := marshalJSON(snapshot.Graph)
	if err != nil {
		return err
	}
	if _, err := db.saveStateSnapshotTx(tx, snapshot.Graph.GraphID, "", "", "base", snapshot.BaseStateSnapshot); err != nil {
		return err
	}
	if _, err := db.saveStateSnapshotTx(tx, snapshot.Graph.GraphID, "", "", "committed", snapshot.LatestCommittedStateSnapshot); err != nil {
		return err
	}
	if update {
		result, err := tx.Exec(`
			UPDATE task_graphs
			SET parent_workflow_id = ?, parent_task_id = ?, trigger_source = ?, generated_at = ?, version = ?,
			    base_state_snapshot_ref = ?, latest_committed_state_ref = ?, graph_json = ?, registered_at = ?
			WHERE graph_id = ? AND version = ?
		`, snapshot.Graph.ParentWorkflowID, snapshot.Graph.ParentTaskID, string(snapshot.Graph.TriggerSource), snapshot.Graph.GeneratedAt.UTC().Format(time.RFC3339Nano), snapshot.Version, snapshot.BaseStateSnapshotRef, snapshot.LatestCommittedStateRef, graphJSON, snapshot.RegisteredAt.UTC().Format(time.RFC3339Nano), snapshot.Graph.GraphID, expectedVersion)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return &ConflictError{Resource: "task_graph", ID: snapshot.Graph.GraphID, Reason: fmt.Sprintf("expected version %d", expectedVersion)}
		}
	} else {
		if _, err := tx.Exec(`
			INSERT INTO task_graphs (
				graph_id, parent_workflow_id, parent_task_id, trigger_source, generated_at, version,
				base_state_snapshot_ref, latest_committed_state_ref, graph_json, registered_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(graph_id) DO UPDATE SET
				parent_workflow_id = excluded.parent_workflow_id,
				parent_task_id = excluded.parent_task_id,
				trigger_source = excluded.trigger_source,
				generated_at = excluded.generated_at,
				version = excluded.version,
				base_state_snapshot_ref = excluded.base_state_snapshot_ref,
				latest_committed_state_ref = excluded.latest_committed_state_ref,
				graph_json = excluded.graph_json,
				registered_at = excluded.registered_at
		`, snapshot.Graph.GraphID, snapshot.Graph.ParentWorkflowID, snapshot.Graph.ParentTaskID, string(snapshot.Graph.TriggerSource), snapshot.Graph.GeneratedAt.UTC().Format(time.RFC3339Nano), snapshot.Version, snapshot.BaseStateSnapshotRef, snapshot.LatestCommittedStateRef, graphJSON, snapshot.RegisteredAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM follow_up_tasks WHERE graph_id = ?`, snapshot.Graph.GraphID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM task_dependencies WHERE graph_id = ?`, snapshot.Graph.GraphID); err != nil {
		return err
	}
	for _, item := range snapshot.RegisteredTasks {
		taskJSON, err := marshalJSON(item.Task)
		if err != nil {
			return err
		}
		metadataJSON, err := marshalJSON(item.Metadata)
		if err != nil {
			return err
		}
		blockingJSON, err := marshalJSON(item.BlockingReasons)
		if err != nil {
			return err
		}
		suppressedJSON, err := marshalJSON(item.SuppressedReasons)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO follow_up_tasks (
				graph_id, task_id, version, task_json, metadata_json, status, required_capability, missing_capability_reason,
				blocking_reasons_json, suppressed_reasons_json, registered_at, registration_order, last_updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, snapshot.Graph.GraphID, item.Task.ID, item.Version, taskJSON, metadataJSON, string(item.Status), item.RequiredCapability, item.MissingCapabilityReason, blockingJSON, suppressedJSON, item.RegisteredAt.UTC().Format(time.RFC3339Nano), item.RegistrationOrder, item.LastUpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	for _, dep := range snapshot.Graph.Dependencies {
		if _, err := tx.Exec(`
			INSERT INTO task_dependencies (graph_id, upstream_task_id, downstream_task_id, reason, mandatory)
			VALUES (?, ?, ?, ?, ?)
		`, snapshot.Graph.GraphID, dep.UpstreamTaskID, dep.DownstreamTaskID, dep.Reason, boolToInt(dep.Mandatory)); err != nil {
			return err
		}
	}
	return nil
}

func loadFollowUpTaskRecords(db *sql.DB, graphID string) ([]FollowUpTaskRecord, error) {
	rows, err := db.Query(`
		SELECT task_json, metadata_json, version, status, required_capability, missing_capability_reason,
		       blocking_reasons_json, suppressed_reasons_json, registered_at, registration_order, last_updated_at
		FROM follow_up_tasks
		WHERE graph_id = ?
		ORDER BY registration_order ASC
	`, graphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]FollowUpTaskRecord, 0)
	for rows.Next() {
		var (
			record         FollowUpTaskRecord
			taskJSON       string
			metadataJSON   string
			status         string
			blockingJSON   string
			suppressedJSON string
			registeredAt   string
			lastUpdatedAt  string
		)
		if err := rows.Scan(&taskJSON, &metadataJSON, &record.Version, &status, &record.RequiredCapability, &record.MissingCapabilityReason, &blockingJSON, &suppressedJSON, &registeredAt, &record.RegistrationOrder, &lastUpdatedAt); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(taskJSON, &record.Task); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(metadataJSON, &record.Metadata); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(blockingJSON, &record.BlockingReasons); err != nil {
			return nil, err
		}
		if err := unmarshalJSON(suppressedJSON, &record.SuppressedReasons); err != nil {
			return nil, err
		}
		record.Status = TaskQueueStatus(status)
		record.RegisteredAt = mustParseTime(registeredAt)
		record.LastUpdatedAt = mustParseTime(lastUpdatedAt)
		result = append(result, record)
	}
	return result, rows.Err()
}

func loadTaskDependencies(db *sql.DB, graphID string) ([]taskspec.TaskDependency, error) {
	rows, err := db.Query(`
		SELECT upstream_task_id, downstream_task_id, reason, mandatory
		FROM task_dependencies
		WHERE graph_id = ?
		ORDER BY upstream_task_id ASC, downstream_task_id ASC
	`, graphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]taskspec.TaskDependency, 0)
	for rows.Next() {
		var (
			dep       taskspec.TaskDependency
			mandatory int
		)
		if err := rows.Scan(&dep.UpstreamTaskID, &dep.DownstreamTaskID, &dep.Reason, &mandatory); err != nil {
			return nil, err
		}
		dep.Mandatory = mandatory == 1
		result = append(result, dep)
	}
	return result, rows.Err()
}

func sortExecutions(records []TaskExecutionRecord) {
	slices.SortFunc(records, func(a TaskExecutionRecord, b TaskExecutionRecord) int {
		switch {
		case a.StartedAt.Before(b.StartedAt):
			return -1
		case a.StartedAt.After(b.StartedAt):
			return 1
		case a.ExecutionID < b.ExecutionID:
			return -1
		case a.ExecutionID > b.ExecutionID:
			return 1
		default:
			return 0
		}
	})
}
