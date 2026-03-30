package runtime

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type postgresWorkflowRunStore struct{ db *PostgresRuntimeDB }

func (s *postgresWorkflowRunStore) Save(record WorkflowRunRecord) error {
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO workflow_runs (workflow_id, updated_at, record_json)
		VALUES (?, ?, ?)
		ON CONFLICT(workflow_id) DO UPDATE SET
			updated_at = excluded.updated_at,
			record_json = excluded.record_json
	`, record.WorkflowID, record.UpdatedAt.UTC().Format(time.RFC3339Nano), raw)
	return err
}

func (s *postgresWorkflowRunStore) Load(workflowID string) (WorkflowRunRecord, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM workflow_runs WHERE workflow_id = ?`, workflowID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return WorkflowRunRecord{}, false, nil
		}
		return WorkflowRunRecord{}, false, err
	}
	var record WorkflowRunRecord
	if err := unmarshalJSON(raw, &record); err != nil {
		return WorkflowRunRecord{}, false, err
	}
	return record, true, nil
}

func (s *postgresWorkflowRunStore) List() ([]WorkflowRunRecord, error) {
	rows, err := s.db.query(`SELECT record_json FROM workflow_runs ORDER BY updated_at ASC, workflow_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]WorkflowRunRecord, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record WorkflowRunRecord
		if err := unmarshalJSON(raw, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

type postgresTaskGraphStore struct{ db *PostgresRuntimeDB }

func (s *postgresTaskGraphStore) Save(snapshot TaskGraphSnapshot) error {
	raw, err := marshalJSON(snapshot)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO task_graphs (graph_id, version, snapshot_json, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(graph_id) DO UPDATE SET
			version = excluded.version,
			snapshot_json = excluded.snapshot_json,
			updated_at = excluded.updated_at
	`, snapshot.Graph.GraphID, snapshot.Version, raw, snapshot.RegisteredAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *postgresTaskGraphStore) Update(snapshot TaskGraphSnapshot, expectedVersion int64) error {
	raw, err := marshalJSON(snapshot)
	if err != nil {
		return err
	}
	result, err := s.db.exec(`
		UPDATE task_graphs
		SET version = ?, snapshot_json = ?, updated_at = ?
		WHERE graph_id = ? AND version = ?
	`, snapshot.Version, raw, snapshot.RegisteredAt.UTC().Format(time.RFC3339Nano), snapshot.Graph.GraphID, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return &ConflictError{Resource: "task_graph", ID: snapshot.Graph.GraphID, Reason: fmt.Sprintf("expected version %d", expectedVersion)}
	}
	return nil
}

func (s *postgresTaskGraphStore) Load(graphID string) (TaskGraphSnapshot, bool, error) {
	row := s.db.queryRow(`SELECT snapshot_json FROM task_graphs WHERE graph_id = ?`, graphID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return TaskGraphSnapshot{}, false, nil
		}
		return TaskGraphSnapshot{}, false, err
	}
	var snapshot TaskGraphSnapshot
	if err := unmarshalJSON(raw, &snapshot); err != nil {
		return TaskGraphSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func (s *postgresTaskGraphStore) List() ([]TaskGraphSnapshot, error) {
	rows, err := s.db.query(`SELECT snapshot_json FROM task_graphs ORDER BY graph_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TaskGraphSnapshot, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var snapshot TaskGraphSnapshot
		if err := unmarshalJSON(raw, &snapshot); err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}
	return result, rows.Err()
}

func (s *postgresTaskGraphStore) SaveStateSnapshot(graphID string, workflowID string, taskID string, kind string, snapshot state.StateSnapshot) (string, error) {
	ref := snapshotRefFor(snapshot)
	if ref == "" {
		return "", fmt.Errorf("state snapshot requires snapshot id")
	}
	raw, err := marshalJSON(snapshot.State)
	if err != nil {
		return "", err
	}
	_, err = s.db.exec(`
		INSERT INTO state_snapshots (snapshot_ref, graph_id, workflow_id, task_id, kind, state_version, reason, captured_at, state_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(snapshot_ref) DO UPDATE SET
			graph_id = excluded.graph_id,
			workflow_id = excluded.workflow_id,
			task_id = excluded.task_id,
			kind = excluded.kind,
			state_version = excluded.state_version,
			reason = excluded.reason,
			captured_at = excluded.captured_at,
			state_json = excluded.state_json
	`, ref, nullString(graphID), nullString(workflowID), nullString(taskID), kind, snapshot.State.Version.Sequence, snapshot.Reason, snapshot.CapturedAt.UTC().Format(time.RFC3339Nano), raw)
	return ref, err
}

func (s *postgresTaskGraphStore) LoadStateSnapshot(snapshotRef string) (state.StateSnapshot, bool, error) {
	row := s.db.queryRow(`SELECT kind, state_version, reason, captured_at, state_json FROM state_snapshots WHERE snapshot_ref = ?`, snapshotRef)
	var (
		kind       string
		version    uint64
		reason     string
		capturedAt string
		raw        string
	)
	if err := row.Scan(&kind, &version, &reason, &capturedAt, &raw); err != nil {
		if errorsIsNoRows(err) {
			return state.StateSnapshot{}, false, nil
		}
		return state.StateSnapshot{}, false, err
	}
	var world state.FinancialWorldState
	if err := unmarshalJSON(raw, &world); err != nil {
		return state.StateSnapshot{}, false, err
	}
	return state.StateSnapshot{
		State:      world,
		Reason:     reason,
		CapturedAt: mustParseTime(capturedAt),
	}, true, nil
}

type postgresTaskExecutionStore struct{ db *PostgresRuntimeDB }

func (s *postgresTaskExecutionStore) Save(record TaskExecutionRecord) error {
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO task_executions (execution_id, graph_id, task_id, started_at, version, record_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`, record.ExecutionID, record.ParentGraphID, record.TaskID, record.StartedAt.UTC().Format(time.RFC3339Nano), record.Version, raw)
	return err
}

func (s *postgresTaskExecutionStore) Update(record TaskExecutionRecord, expectedVersion int64) error {
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	result, err := s.db.exec(`
		UPDATE task_executions
		SET version = ?, record_json = ?
		WHERE execution_id = ? AND version = ?
	`, record.Version+1, raw, record.ExecutionID, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return &ConflictError{Resource: "task_execution", ID: record.ExecutionID, Reason: "version mismatch"}
	}
	record.Version = expectedVersion + 1
	return nil
}

func (s *postgresTaskExecutionStore) Load(executionID string) (TaskExecutionRecord, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM task_executions WHERE execution_id = ?`, executionID)
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

func (s *postgresTaskExecutionStore) LoadLatestByTask(graphID string, taskID string) (TaskExecutionRecord, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM task_executions WHERE graph_id = ? AND task_id = ? ORDER BY started_at DESC, execution_id DESC LIMIT 1`, graphID, taskID)
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

func (s *postgresTaskExecutionStore) ListByGraph(graphID string) ([]TaskExecutionRecord, error) {
	rows, err := s.db.query(`SELECT record_json FROM task_executions WHERE graph_id = ? ORDER BY started_at ASC, execution_id ASC`, graphID)
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

type postgresApprovalStateStore struct{ db *PostgresRuntimeDB }

func (s *postgresApprovalStateStore) Save(record ApprovalStateRecord) error {
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO approvals (approval_id, graph_id, task_id, status, version, record_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(approval_id) DO UPDATE SET
			graph_id = excluded.graph_id,
			task_id = excluded.task_id,
			status = excluded.status,
			version = excluded.version,
			record_json = excluded.record_json
	`, record.ApprovalID, record.GraphID, record.TaskID, string(record.Status), record.Version, raw)
	return err
}

func (s *postgresApprovalStateStore) Update(record ApprovalStateRecord, expectedVersion int64) error {
	record.Version = expectedVersion + 1
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	result, err := s.db.exec(`
		UPDATE approvals
		SET status = ?, version = ?, record_json = ?
		WHERE approval_id = ? AND version = ?
	`, string(record.Status), record.Version, raw, record.ApprovalID, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return &ConflictError{Resource: "approval", ID: record.ApprovalID, Reason: "version mismatch"}
	}
	return nil
}

func (s *postgresApprovalStateStore) Load(approvalID string) (ApprovalStateRecord, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM approvals WHERE approval_id = ?`, approvalID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return ApprovalStateRecord{}, false, nil
		}
		return ApprovalStateRecord{}, false, err
	}
	var record ApprovalStateRecord
	if err := unmarshalJSON(raw, &record); err != nil {
		return ApprovalStateRecord{}, false, err
	}
	return record, true, nil
}

func (s *postgresApprovalStateStore) ListPending() ([]ApprovalStateRecord, error) {
	rows, err := s.db.query(`SELECT record_json FROM approvals WHERE status = ? ORDER BY approval_id ASC`, string(ApprovalStatusPending))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ApprovalStateRecord, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record ApprovalStateRecord
		if err := unmarshalJSON(raw, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *postgresApprovalStateStore) LoadByTask(graphID string, taskID string) (ApprovalStateRecord, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM approvals WHERE graph_id = ? AND task_id = ? ORDER BY approval_id DESC LIMIT 1`, graphID, taskID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return ApprovalStateRecord{}, false, nil
		}
		return ApprovalStateRecord{}, false, err
	}
	var record ApprovalStateRecord
	if err := unmarshalJSON(raw, &record); err != nil {
		return ApprovalStateRecord{}, false, err
	}
	return record, true, nil
}

type postgresOperatorActionStore struct{ db *PostgresRuntimeDB }

func (s *postgresOperatorActionStore) Save(record OperatorActionRecord) error {
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO operator_actions (action_id, request_id, graph_id, task_id, record_json)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(action_id) DO UPDATE SET
			request_id = excluded.request_id,
			graph_id = excluded.graph_id,
			task_id = excluded.task_id,
			record_json = excluded.record_json
	`, record.ActionID, record.RequestID, nullString(record.GraphID), nullString(record.TaskID), raw)
	if err != nil && strings.Contains(err.Error(), "request_id") {
		return &ConflictError{Resource: "operator_action", ID: record.RequestID, Reason: "duplicate request id"}
	}
	return err
}

func (s *postgresOperatorActionStore) LoadByRequestID(requestID string) (OperatorActionRecord, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM operator_actions WHERE request_id = ?`, requestID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return OperatorActionRecord{}, false, nil
		}
		return OperatorActionRecord{}, false, err
	}
	var record OperatorActionRecord
	if err := unmarshalJSON(raw, &record); err != nil {
		return OperatorActionRecord{}, false, err
	}
	return record, true, nil
}

func (s *postgresOperatorActionStore) ListByTask(graphID string, taskID string) ([]OperatorActionRecord, error) {
	rows, err := s.db.query(`SELECT record_json FROM operator_actions WHERE graph_id = ? AND task_id = ? ORDER BY action_id ASC`, graphID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]OperatorActionRecord, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record OperatorActionRecord
		if err := unmarshalJSON(raw, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

type postgresCheckpointStore struct{ db *PostgresRuntimeDB }

func (s *postgresCheckpointStore) Save(checkpoint CheckpointRecord) error {
	raw, err := marshalJSON(checkpoint)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO checkpoints (checkpoint_id, workflow_id, record_json)
		VALUES (?, ?, ?)
		ON CONFLICT(checkpoint_id) DO UPDATE SET
			workflow_id = excluded.workflow_id,
			record_json = excluded.record_json
	`, checkpoint.ID, checkpoint.WorkflowID, raw)
	return err
}

func (s *postgresCheckpointStore) Load(workflowID string, checkpointID string) (CheckpointRecord, error) {
	row := s.db.queryRow(`SELECT workflow_id, record_json FROM checkpoints WHERE checkpoint_id = ?`, checkpointID)
	var (
		recordWorkflowID string
		raw              string
	)
	if err := row.Scan(&recordWorkflowID, &raw); err != nil {
		if errorsIsNoRows(err) {
			return CheckpointRecord{}, &NotFoundError{Resource: "checkpoint", ID: checkpointID}
		}
		return CheckpointRecord{}, err
	}
	var record CheckpointRecord
	if err := unmarshalJSON(raw, &record); err != nil {
		return CheckpointRecord{}, err
	}
	if workflowID != "" && recordWorkflowID != workflowID {
		return CheckpointRecord{}, &ConflictError{Resource: "checkpoint", ID: checkpointID, Reason: "workflow id mismatch"}
	}
	return record, nil
}

func (s *postgresCheckpointStore) SaveResumeToken(token ResumeToken) error {
	raw, err := marshalJSON(token)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO resume_tokens (token, record_json)
		VALUES (?, ?)
		ON CONFLICT(token) DO UPDATE SET record_json = excluded.record_json
	`, token.Token, raw)
	return err
}

func (s *postgresCheckpointStore) LoadResumeToken(token string) (ResumeToken, error) {
	row := s.db.queryRow(`SELECT record_json FROM resume_tokens WHERE token = ?`, token)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return ResumeToken{}, &NotFoundError{Resource: "resume_token", ID: token}
		}
		return ResumeToken{}, err
	}
	var record ResumeToken
	if err := unmarshalJSON(raw, &record); err != nil {
		return ResumeToken{}, err
	}
	return record, nil
}

func (s *postgresCheckpointStore) SavePayload(checkpointID string, payload CheckpointPayloadEnvelope) error {
	raw, err := marshalJSON(payload)
	if err != nil {
		return err
	}
	result, err := s.db.exec(`UPDATE checkpoints SET payload_kind = ?, payload_json = ? WHERE checkpoint_id = ?`, string(payload.Kind), raw, checkpointID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return &NotFoundError{Resource: "checkpoint", ID: checkpointID}
	}
	return nil
}

func (s *postgresCheckpointStore) LoadPayload(checkpointID string) (CheckpointPayloadEnvelope, error) {
	row := s.db.queryRow(`SELECT payload_json FROM checkpoints WHERE checkpoint_id = ?`, checkpointID)
	var raw sql.NullString
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return CheckpointPayloadEnvelope{}, &NotFoundError{Resource: "checkpoint_payload", ID: checkpointID}
		}
		return CheckpointPayloadEnvelope{}, err
	}
	if !raw.Valid || raw.String == "" {
		return CheckpointPayloadEnvelope{}, &NotFoundError{Resource: "checkpoint_payload", ID: checkpointID}
	}
	var payload CheckpointPayloadEnvelope
	if err := unmarshalJSON(raw.String, &payload); err != nil {
		return CheckpointPayloadEnvelope{}, err
	}
	return payload, nil
}

type postgresReplayStore struct{ db *PostgresRuntimeDB }

func (s *postgresReplayStore) Append(event ReplayEventRecord) error {
	raw, err := marshalJSON(event)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO replay_events (event_id, workflow_id, graph_id, task_id, approval_id, execution_id, occurred_at, record_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, event.EventID, nullString(event.WorkflowID), nullString(event.GraphID), nullString(event.TaskID), nullString(event.ApprovalID), nullString(event.ExecutionID), event.OccurredAt.UTC().Format(time.RFC3339Nano), raw)
	return err
}

func (s *postgresReplayStore) ListByGraph(graphID string) ([]ReplayEventRecord, error) {
	return s.list(`SELECT record_json FROM replay_events WHERE graph_id = ? ORDER BY occurred_at ASC, event_id ASC`, graphID)
}

func (s *postgresReplayStore) ListByTask(taskID string) ([]ReplayEventRecord, error) {
	return s.list(`SELECT record_json FROM replay_events WHERE task_id = ? ORDER BY occurred_at ASC, event_id ASC`, taskID)
}

func (s *postgresReplayStore) ListByWorkflow(workflowID string) ([]ReplayEventRecord, error) {
	return s.list(`SELECT record_json FROM replay_events WHERE workflow_id = ? ORDER BY occurred_at ASC, event_id ASC`, workflowID)
}

func (s *postgresReplayStore) list(query string, arg string) ([]ReplayEventRecord, error) {
	rows, err := s.db.query(query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ReplayEventRecord, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record ReplayEventRecord
		if err := unmarshalJSON(raw, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

type postgresReplayProjectionStore struct{ db *PostgresRuntimeDB }

func (s *postgresReplayProjectionStore) SaveWorkflowProjection(record WorkflowReplayProjection) error {
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO workflow_replay_projections (workflow_id, projection_freshness, record_json)
		VALUES (?, ?, ?)
		ON CONFLICT(workflow_id) DO UPDATE SET
			projection_freshness = excluded.projection_freshness,
			record_json = excluded.record_json
	`, record.WorkflowID, record.ProjectionFreshness.UTC().Format(time.RFC3339Nano), raw)
	return err
}

func (s *postgresReplayProjectionStore) SaveTaskGraphProjection(record TaskGraphReplayProjection) error {
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO task_graph_replay_projections (graph_id, projection_freshness, record_json)
		VALUES (?, ?, ?)
		ON CONFLICT(graph_id) DO UPDATE SET
			projection_freshness = excluded.projection_freshness,
			record_json = excluded.record_json
	`, record.GraphID, record.ProjectionFreshness.UTC().Format(time.RFC3339Nano), raw)
	return err
}

func (s *postgresReplayProjectionStore) SaveBuild(record ReplayProjectionBuildRecord) error {
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO replay_projection_builds (scope_kind, scope_id, record_json)
		VALUES (?, ?, ?)
		ON CONFLICT(scope_kind, scope_id) DO UPDATE SET
			record_json = excluded.record_json
	`, string(record.ScopeKind), record.ScopeID, raw)
	return err
}

func (s *postgresReplayProjectionStore) ReplaceProvenance(scope ReplayProjectionScope, nodes []ProvenanceNodeRecord, edges []ProvenanceEdgeRecord) error {
	if _, err := s.db.exec(`DELETE FROM replay_provenance_nodes WHERE scope_kind = ? AND scope_id = ?`, string(scope.ScopeKind), scope.ScopeID); err != nil {
		return err
	}
	if _, err := s.db.exec(`DELETE FROM replay_provenance_edges WHERE scope_kind = ? AND scope_id = ?`, string(scope.ScopeKind), scope.ScopeID); err != nil {
		return err
	}
	for _, node := range nodes {
		raw, err := marshalJSON(node)
		if err != nil {
			return err
		}
		if _, err := s.db.exec(`INSERT INTO replay_provenance_nodes(scope_kind, scope_id, node_id, record_json) VALUES (?, ?, ?, ?)`, string(scope.ScopeKind), scope.ScopeID, node.NodeID, raw); err != nil {
			return err
		}
	}
	for _, edge := range edges {
		raw, err := marshalJSON(edge)
		if err != nil {
			return err
		}
		if _, err := s.db.exec(`INSERT INTO replay_provenance_edges(scope_kind, scope_id, edge_id, record_json) VALUES (?, ?, ?, ?)`, string(scope.ScopeKind), scope.ScopeID, edge.EdgeID, raw); err != nil {
			return err
		}
	}
	return nil
}

func (s *postgresReplayProjectionStore) ReplaceExecutionAttributions(scope ReplayProjectionScope, records []ExecutionAttributionRecord) error {
	if _, err := s.db.exec(`DELETE FROM replay_execution_attributions WHERE scope_kind = ? AND scope_id = ?`, string(scope.ScopeKind), scope.ScopeID); err != nil {
		return err
	}
	for _, record := range records {
		raw, err := marshalJSON(record)
		if err != nil {
			return err
		}
		if _, err := s.db.exec(`INSERT INTO replay_execution_attributions(scope_kind, scope_id, execution_id, category, record_json) VALUES (?, ?, ?, ?, ?)`, string(scope.ScopeKind), scope.ScopeID, record.ExecutionID, record.Category, raw); err != nil {
			return err
		}
	}
	return nil
}

func (s *postgresReplayProjectionStore) ReplaceFailureAttributions(scope ReplayProjectionScope, records []FailureAttributionRecord) error {
	if _, err := s.db.exec(`DELETE FROM replay_failure_attributions WHERE scope_kind = ? AND scope_id = ?`, string(scope.ScopeKind), scope.ScopeID); err != nil {
		return err
	}
	for _, record := range records {
		raw, err := marshalJSON(record)
		if err != nil {
			return err
		}
		if _, err := s.db.exec(`INSERT INTO replay_failure_attributions(scope_kind, scope_id, attribution_id, record_json) VALUES (?, ?, ?, ?)`, string(scope.ScopeKind), scope.ScopeID, record.AttributionID, raw); err != nil {
			return err
		}
	}
	return nil
}

func (s *postgresReplayProjectionStore) LoadWorkflowProjection(workflowID string) (WorkflowReplayProjection, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM workflow_replay_projections WHERE workflow_id = ?`, workflowID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return WorkflowReplayProjection{}, false, nil
		}
		return WorkflowReplayProjection{}, false, err
	}
	var record WorkflowReplayProjection
	if err := unmarshalJSON(raw, &record); err != nil {
		return WorkflowReplayProjection{}, false, err
	}
	return record, true, nil
}

func (s *postgresReplayProjectionStore) LoadTaskGraphProjection(graphID string) (TaskGraphReplayProjection, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM task_graph_replay_projections WHERE graph_id = ?`, graphID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return TaskGraphReplayProjection{}, false, nil
		}
		return TaskGraphReplayProjection{}, false, err
	}
	var record TaskGraphReplayProjection
	if err := unmarshalJSON(raw, &record); err != nil {
		return TaskGraphReplayProjection{}, false, err
	}
	return record, true, nil
}

func (s *postgresReplayProjectionStore) LoadBuild(scope ReplayProjectionScope) (ReplayProjectionBuildRecord, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM replay_projection_builds WHERE scope_kind = ? AND scope_id = ?`, string(scope.ScopeKind), scope.ScopeID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return ReplayProjectionBuildRecord{}, false, nil
		}
		return ReplayProjectionBuildRecord{}, false, err
	}
	var record ReplayProjectionBuildRecord
	if err := unmarshalJSON(raw, &record); err != nil {
		return ReplayProjectionBuildRecord{}, false, err
	}
	return record, true, nil
}

func (s *postgresReplayProjectionStore) ListProvenance(scope ReplayProjectionScope) ([]ProvenanceNodeRecord, []ProvenanceEdgeRecord, error) {
	nodeRows, err := s.db.query(`SELECT record_json FROM replay_provenance_nodes WHERE scope_kind = ? AND scope_id = ? ORDER BY node_id ASC`, string(scope.ScopeKind), scope.ScopeID)
	if err != nil {
		return nil, nil, err
	}
	defer nodeRows.Close()
	nodes := make([]ProvenanceNodeRecord, 0)
	for nodeRows.Next() {
		var raw string
		if err := nodeRows.Scan(&raw); err != nil {
			return nil, nil, err
		}
		var node ProvenanceNodeRecord
		if err := unmarshalJSON(raw, &node); err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, node)
	}
	edgeRows, err := s.db.query(`SELECT record_json FROM replay_provenance_edges WHERE scope_kind = ? AND scope_id = ? ORDER BY edge_id ASC`, string(scope.ScopeKind), scope.ScopeID)
	if err != nil {
		return nil, nil, err
	}
	defer edgeRows.Close()
	edges := make([]ProvenanceEdgeRecord, 0)
	for edgeRows.Next() {
		var raw string
		if err := edgeRows.Scan(&raw); err != nil {
			return nil, nil, err
		}
		var edge ProvenanceEdgeRecord
		if err := unmarshalJSON(raw, &edge); err != nil {
			return nil, nil, err
		}
		edges = append(edges, edge)
	}
	return nodes, edges, nil
}

func (s *postgresReplayProjectionStore) ListExecutionAttributions(scope ReplayProjectionScope) ([]ExecutionAttributionRecord, error) {
	rows, err := s.db.query(`SELECT record_json FROM replay_execution_attributions WHERE scope_kind = ? AND scope_id = ? ORDER BY execution_id ASC, category ASC`, string(scope.ScopeKind), scope.ScopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ExecutionAttributionRecord, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record ExecutionAttributionRecord
		if err := unmarshalJSON(raw, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *postgresReplayProjectionStore) ListFailureAttributions(scope ReplayProjectionScope) ([]FailureAttributionRecord, error) {
	rows, err := s.db.query(`SELECT record_json FROM replay_failure_attributions WHERE scope_kind = ? AND scope_id = ? ORDER BY attribution_id ASC`, string(scope.ScopeKind), scope.ScopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]FailureAttributionRecord, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record FailureAttributionRecord
		if err := unmarshalJSON(raw, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

type postgresArtifactMetadataStore struct{ db *PostgresRuntimeDB }

func (s *postgresArtifactMetadataStore) SaveArtifact(workflowID string, taskID string, artifact reporting.WorkflowArtifact) error {
	ref := artifact.Ref
	raw, err := marshalJSON(artifact)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO workflow_artifacts (artifact_id, workflow_id, task_id, created_at, storage_ref, record_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(artifact_id) DO UPDATE SET
			workflow_id = excluded.workflow_id,
			task_id = excluded.task_id,
			created_at = excluded.created_at,
			storage_ref = excluded.storage_ref,
			record_json = excluded.record_json
	`, artifact.ID, workflowID, taskID, artifact.CreatedAt.UTC().Format(time.RFC3339Nano), nullString(ref.Location), raw)
	return err
}

func (s *postgresArtifactMetadataStore) ListArtifactsByTask(taskID string) ([]reporting.WorkflowArtifact, error) {
	return s.list(`SELECT record_json FROM workflow_artifacts WHERE task_id = ? ORDER BY created_at ASC, artifact_id ASC`, taskID)
}

func (s *postgresArtifactMetadataStore) ListArtifactsByWorkflow(workflowID string) ([]reporting.WorkflowArtifact, error) {
	return s.list(`SELECT record_json FROM workflow_artifacts WHERE workflow_id = ? ORDER BY created_at ASC, artifact_id ASC`, workflowID)
}

func (s *postgresArtifactMetadataStore) LoadArtifact(artifactID string) (reporting.WorkflowArtifact, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM workflow_artifacts WHERE artifact_id = ?`, artifactID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return reporting.WorkflowArtifact{}, false, nil
		}
		return reporting.WorkflowArtifact{}, false, err
	}
	var artifact reporting.WorkflowArtifact
	if err := unmarshalJSON(raw, &artifact); err != nil {
		return reporting.WorkflowArtifact{}, false, err
	}
	return artifact, true, nil
}

func (s *postgresArtifactMetadataStore) list(query string, arg string) ([]reporting.WorkflowArtifact, error) {
	rows, err := s.db.query(query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]reporting.WorkflowArtifact, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var artifact reporting.WorkflowArtifact
		if err := unmarshalJSON(raw, &artifact); err != nil {
			return nil, err
		}
		result = append(result, artifact)
	}
	return result, rows.Err()
}

type postgresWorkQueueStore struct{ db *PostgresRuntimeDB }

func (s *postgresWorkQueueStore) Enqueue(item WorkItem) error {
	if item.ID == "" {
		return fmt.Errorf("work item id is required")
	}
	if item.DedupeKey != "" {
		row := s.db.queryRow(`
			SELECT work_item_id
			FROM work_items
			WHERE dedupe_key = ?
			  AND status NOT IN (?, ?, ?)
			ORDER BY available_at ASC
			LIMIT 1
		`, item.DedupeKey, string(WorkItemStatusCompleted), string(WorkItemStatusFailed), string(WorkItemStatusAbandoned))
		var existing string
		if err := row.Scan(&existing); err == nil {
			return nil
		} else if !errorsIsNoRows(err) {
			return err
		}
	}
	if item.Status == "" {
		item.Status = WorkItemStatusQueued
	}
	if item.LastUpdatedAt.IsZero() {
		item.LastUpdatedAt = item.AvailableAt
	}
	_, err := s.db.exec(`
		INSERT INTO work_items (
			work_item_id, kind, status, dedupe_key, graph_id, task_id, execution_id, approval_id, checkpoint_id,
			workflow_id, available_at, claimed_at, completed_at, failed_at, last_updated_at, reason, wakeup_kind,
			retry_not_before, attempt_count, lease_id, fencing_token, claim_token, claimed_by_worker_id, lease_expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(work_item_id) DO UPDATE SET
			kind = excluded.kind,
			status = excluded.status,
			dedupe_key = excluded.dedupe_key,
			graph_id = excluded.graph_id,
			task_id = excluded.task_id,
			execution_id = excluded.execution_id,
			approval_id = excluded.approval_id,
			checkpoint_id = excluded.checkpoint_id,
			workflow_id = excluded.workflow_id,
			available_at = excluded.available_at,
			claimed_at = excluded.claimed_at,
			completed_at = excluded.completed_at,
			failed_at = excluded.failed_at,
			last_updated_at = excluded.last_updated_at,
			reason = excluded.reason,
			wakeup_kind = excluded.wakeup_kind,
			retry_not_before = excluded.retry_not_before,
			attempt_count = excluded.attempt_count,
			lease_id = excluded.lease_id,
			fencing_token = excluded.fencing_token,
			claim_token = excluded.claim_token,
			claimed_by_worker_id = excluded.claimed_by_worker_id,
			lease_expires_at = excluded.lease_expires_at
	`, item.ID, string(item.Kind), string(item.Status), nullString(item.DedupeKey), nullString(item.GraphID), nullString(item.TaskID),
		nullString(item.ExecutionID), nullString(item.ApprovalID), nullString(item.CheckpointID), nullString(item.WorkflowID),
		item.AvailableAt.UTC().Format(time.RFC3339Nano), nullableTime(item.ClaimedAt), nullableTime(item.CompletedAt), nullableTime(item.FailedAt),
		item.LastUpdatedAt.UTC().Format(time.RFC3339Nano), nullString(item.Reason), nullString(string(item.WakeupKind)), nullableTime(item.RetryNotBefore), item.AttemptCount,
		nullString(item.LeaseID), item.FencingToken, nullString(item.ClaimToken), nullString(string(item.ClaimedByWorkerID)), nullableTime(item.LeaseExpiresAt))
	return err
}

func (s *postgresWorkQueueStore) ClaimReady(workerID WorkerID, limit int, now time.Time, leaseTTL time.Duration) ([]WorkClaim, error) {
	if limit <= 0 {
		limit = 1
	}
	tx, err := s.db.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(rewritePositionalSQL(`
		SELECT work_item_id, kind, status, dedupe_key, graph_id, task_id, execution_id, approval_id, checkpoint_id,
		       workflow_id, available_at, claimed_at, completed_at, failed_at, last_updated_at, reason, wakeup_kind,
		       retry_not_before, attempt_count, lease_id, fencing_token, claim_token, claimed_by_worker_id, lease_expires_at
		FROM work_items
		WHERE status = ?
		  AND available_at <= ?
		ORDER BY available_at ASC, work_item_id ASC
		FOR UPDATE SKIP LOCKED
		LIMIT ?
	`), string(WorkItemStatusQueued), now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	claims := make([]WorkClaim, 0)
	for rows.Next() {
		item, err := scanWorkItem(rows)
		if err != nil {
			return nil, err
		}
		item.Status = WorkItemStatusClaimed
		item.ClaimedByWorkerID = workerID
		item.FencingToken++
		item.AttemptCount++
		item.LeaseID = makeID("lease", item.ID, item.FencingToken, workerID, now)
		item.ClaimToken = makeID("claim", item.ID, item.FencingToken, now)
		claimedAt := now.UTC()
		item.ClaimedAt = &claimedAt
		expires := now.UTC().Add(leaseTTL)
		item.LeaseExpiresAt = &expires
		item.LastUpdatedAt = claimedAt
		if _, err := tx.Exec(rewritePositionalSQL(`
			UPDATE work_items
			SET status = ?, claimed_at = ?, last_updated_at = ?, attempt_count = ?, lease_id = ?, fencing_token = ?, claim_token = ?, claimed_by_worker_id = ?, lease_expires_at = ?
			WHERE work_item_id = ?
		`), string(item.Status), claimedAt.Format(time.RFC3339Nano), item.LastUpdatedAt.Format(time.RFC3339Nano), item.AttemptCount, item.LeaseID, item.FencingToken, item.ClaimToken, string(workerID), expires.Format(time.RFC3339Nano), item.ID); err != nil {
			return nil, err
		}
		claims = append(claims, WorkClaim{
			WorkItem:       item,
			WorkerID:       workerID,
			LeaseID:        item.LeaseID,
			ClaimToken:     item.ClaimToken,
			FencingToken:   item.FencingToken,
			ClaimedAt:      claimedAt,
			LeaseExpiresAt: expires,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claims, nil
}

func (s *postgresWorkQueueStore) Heartbeat(heartbeat LeaseHeartbeat) error {
	return s.withFence(heartbeat.WorkItemID, heartbeat.LeaseID, heartbeat.FencingToken, heartbeat.WorkerID, func(item WorkItem) error {
		_, err := s.db.exec(`UPDATE work_items SET last_updated_at = ?, lease_expires_at = ? WHERE work_item_id = ?`, heartbeat.RecordedAt.UTC().Format(time.RFC3339Nano), heartbeat.LeaseExpiresAt.UTC().Format(time.RFC3339Nano), item.ID)
		return err
	})
}

func (s *postgresWorkQueueStore) Complete(fence FenceValidation, now time.Time) error {
	return s.finish(fence, WorkItemStatusCompleted, "", now)
}

func (s *postgresWorkQueueStore) Fail(fence FenceValidation, summary string, now time.Time) error {
	return s.finish(fence, WorkItemStatusFailed, summary, now)
}

func (s *postgresWorkQueueStore) Requeue(fence FenceValidation, nextAvailableAt time.Time, reason string, now time.Time) error {
	return s.withFence(fence.WorkItemID, fence.LeaseID, fence.FencingToken, fence.WorkerID, func(item WorkItem) error {
		_, err := s.db.exec(`
			UPDATE work_items
			SET status = ?, available_at = ?, last_updated_at = ?, reason = ?, claimed_at = NULL,
			    lease_id = NULL, claim_token = NULL, claimed_by_worker_id = NULL, lease_expires_at = NULL
			WHERE work_item_id = ?
		`, string(WorkItemStatusQueued), nextAvailableAt.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), reason, item.ID)
		return err
	})
}

func (s *postgresWorkQueueStore) ReclaimExpired(now time.Time) ([]LeaseReclaimResult, error) {
	rows, err := s.db.query(`
		SELECT work_item_id, kind, status, dedupe_key, graph_id, task_id, execution_id, approval_id, checkpoint_id,
		       workflow_id, available_at, claimed_at, completed_at, failed_at, last_updated_at, reason, wakeup_kind,
		       retry_not_before, attempt_count, lease_id, fencing_token, claim_token, claimed_by_worker_id, lease_expires_at
		FROM work_items
		WHERE status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?
		ORDER BY lease_expires_at ASC, work_item_id ASC
	`, string(WorkItemStatusClaimed), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]LeaseReclaimResult, 0)
	for rows.Next() {
		item, err := scanWorkItem(rows)
		if err != nil {
			return nil, err
		}
		if _, err := s.db.exec(`
			UPDATE work_items
			SET status = ?, reason = ?, last_updated_at = ?, claimed_at = NULL, lease_id = NULL, claim_token = NULL,
			    claimed_by_worker_id = NULL, lease_expires_at = NULL
			WHERE work_item_id = ? AND status = ? AND fencing_token = ?
		`, string(WorkItemStatusQueued), "reclaimed after lease expiry", now.UTC().Format(time.RFC3339Nano), item.ID, string(WorkItemStatusClaimed), item.FencingToken); err != nil {
			return nil, err
		}
		result = append(result, LeaseReclaimResult{
			WorkItemID:   item.ID,
			LeaseID:      item.LeaseID,
			WorkerID:     item.ClaimedByWorkerID,
			FencingToken: item.FencingToken,
			Reason:       "lease_expired",
			ReclaimedAt:  now.UTC(),
		})
	}
	return result, rows.Err()
}

func (s *postgresWorkQueueStore) Load(workItemID string) (WorkItem, bool, error) {
	row := s.db.queryRow(`
		SELECT work_item_id, kind, status, dedupe_key, graph_id, task_id, execution_id, approval_id, checkpoint_id,
		       workflow_id, available_at, claimed_at, completed_at, failed_at, last_updated_at, reason, wakeup_kind,
		       retry_not_before, attempt_count, lease_id, fencing_token, claim_token, claimed_by_worker_id, lease_expires_at
		FROM work_items WHERE work_item_id = ?
	`, workItemID)
	item, err := scanWorkItem(row)
	if err != nil {
		if errorsIsNoRows(err) {
			return WorkItem{}, false, nil
		}
		return WorkItem{}, false, err
	}
	return item, true, nil
}

func (s *postgresWorkQueueStore) ListByGraph(graphID string) ([]WorkItem, error) {
	rows, err := s.db.query(`
		SELECT work_item_id, kind, status, dedupe_key, graph_id, task_id, execution_id, approval_id, checkpoint_id,
		       workflow_id, available_at, claimed_at, completed_at, failed_at, last_updated_at, reason, wakeup_kind,
		       retry_not_before, attempt_count, lease_id, fencing_token, claim_token, claimed_by_worker_id, lease_expires_at
		FROM work_items WHERE graph_id = ? ORDER BY available_at ASC, work_item_id ASC
	`, graphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]WorkItem, 0)
	for rows.Next() {
		item, err := scanWorkItem(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *postgresWorkQueueStore) ValidateFence(fence FenceValidation) error {
	return s.withFence(fence.WorkItemID, fence.LeaseID, fence.FencingToken, fence.WorkerID, func(_ WorkItem) error { return nil })
}

func (s *postgresWorkQueueStore) finish(fence FenceValidation, status WorkItemStatus, summary string, now time.Time) error {
	return s.withFence(fence.WorkItemID, fence.LeaseID, fence.FencingToken, fence.WorkerID, func(item WorkItem) error {
		var completedAt any
		var failedAt any
		if status == WorkItemStatusCompleted {
			completedAt = now.UTC().Format(time.RFC3339Nano)
		}
		if status == WorkItemStatusFailed {
			failedAt = now.UTC().Format(time.RFC3339Nano)
		}
		_, err := s.db.exec(`
			UPDATE work_items
			SET status = ?, reason = ?, last_updated_at = ?, claimed_at = NULL, lease_id = NULL, claim_token = NULL,
			    claimed_by_worker_id = NULL, lease_expires_at = NULL, completed_at = ?, failed_at = ?
			WHERE work_item_id = ?
		`, string(status), summary, now.UTC().Format(time.RFC3339Nano), completedAt, failedAt, item.ID)
		return err
	})
}

func (s *postgresWorkQueueStore) withFence(workItemID string, leaseID string, fencingToken int64, workerID WorkerID, fn func(WorkItem) error) error {
	item, ok, err := s.Load(workItemID)
	if err != nil {
		return err
	}
	if !ok {
		return &NotFoundError{Resource: "work_item", ID: workItemID}
	}
	if err := validateFenceAgainstItem(item, FenceValidation{
		WorkItemID:   workItemID,
		LeaseID:      leaseID,
		FencingToken: fencingToken,
		WorkerID:     workerID,
	}); err != nil {
		return err
	}
	return fn(item)
}

type postgresWorkAttemptStore struct{ db *PostgresRuntimeDB }

func (s *postgresWorkAttemptStore) SaveAttempt(attempt ExecutionAttempt) error {
	raw, err := marshalJSON(attempt)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`INSERT INTO work_attempts (attempt_id, work_item_id, started_at, record_json) VALUES (?, ?, ?, ?)`, attempt.AttemptID, attempt.WorkItemID, attempt.StartedAt.UTC().Format(time.RFC3339Nano), raw)
	return err
}

func (s *postgresWorkAttemptStore) UpdateAttempt(attempt ExecutionAttempt) error {
	raw, err := marshalJSON(attempt)
	if err != nil {
		return err
	}
	result, err := s.db.exec(`UPDATE work_attempts SET record_json = ? WHERE attempt_id = ?`, raw, attempt.AttemptID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return &NotFoundError{Resource: "execution_attempt", ID: attempt.AttemptID}
	}
	return nil
}

func (s *postgresWorkAttemptStore) ListAttempts(workItemID string) ([]ExecutionAttempt, error) {
	rows, err := s.db.query(`SELECT record_json FROM work_attempts WHERE work_item_id = ? ORDER BY started_at ASC, attempt_id ASC`, workItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ExecutionAttempt, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var attempt ExecutionAttempt
		if err := unmarshalJSON(raw, &attempt); err != nil {
			return nil, err
		}
		result = append(result, attempt)
	}
	return result, rows.Err()
}

type postgresWorkerRegistryStore struct{ db *PostgresRuntimeDB }

func (s *postgresWorkerRegistryStore) Register(worker WorkerRegistration) error {
	raw, err := marshalJSON(worker)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`INSERT INTO worker_registrations(worker_id, last_heartbeat, record_json) VALUES (?, ?, ?) ON CONFLICT(worker_id) DO UPDATE SET last_heartbeat = excluded.last_heartbeat, record_json = excluded.record_json`, string(worker.WorkerID), worker.LastHeartbeat.UTC().Format(time.RFC3339Nano), raw)
	return err
}

func (s *postgresWorkerRegistryStore) Heartbeat(workerID WorkerID, now time.Time) error {
	record, ok, err := s.Load(workerID)
	if err != nil {
		return err
	}
	if !ok {
		return &NotFoundError{Resource: "worker", ID: string(workerID)}
	}
	record.LastHeartbeat = now
	return s.Register(record)
}

func (s *postgresWorkerRegistryStore) Load(workerID WorkerID) (WorkerRegistration, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM worker_registrations WHERE worker_id = ?`, string(workerID))
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return WorkerRegistration{}, false, nil
		}
		return WorkerRegistration{}, false, err
	}
	var record WorkerRegistration
	if err := unmarshalJSON(raw, &record); err != nil {
		return WorkerRegistration{}, false, err
	}
	return record, true, nil
}

func (s *postgresWorkerRegistryStore) List() ([]WorkerRegistration, error) {
	rows, err := s.db.query(`SELECT record_json FROM worker_registrations ORDER BY worker_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]WorkerRegistration, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record WorkerRegistration
		if err := unmarshalJSON(raw, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

type postgresSchedulerStore struct{ db *PostgresRuntimeDB }

func (s *postgresSchedulerStore) SaveWakeup(wakeup SchedulerWakeup) error {
	raw, err := marshalJSON(wakeup)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`INSERT INTO scheduler_wakeups(wakeup_id, available_at, record_json) VALUES (?, ?, ?) ON CONFLICT(wakeup_id) DO UPDATE SET available_at = excluded.available_at, record_json = excluded.record_json`, wakeup.ID, wakeup.AvailableAt.UTC().Format(time.RFC3339Nano), raw)
	return err
}

func (s *postgresSchedulerStore) ListDueWakeups(now time.Time) ([]SchedulerWakeup, error) {
	rows, err := s.db.query(`SELECT record_json FROM scheduler_wakeups WHERE available_at <= ? ORDER BY available_at ASC, wakeup_id ASC`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SchedulerWakeup, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var wakeup SchedulerWakeup
		if err := unmarshalJSON(raw, &wakeup); err != nil {
			return nil, err
		}
		result = append(result, wakeup)
	}
	return result, rows.Err()
}

func (s *postgresSchedulerStore) MarkWakeupDispatched(id string, _ time.Time) error {
	_, err := s.db.exec(`DELETE FROM scheduler_wakeups WHERE wakeup_id = ?`, id)
	return err
}
