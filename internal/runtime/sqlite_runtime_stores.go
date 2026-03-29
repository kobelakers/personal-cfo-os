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
