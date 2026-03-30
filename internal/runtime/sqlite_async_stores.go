package runtime

import (
	"database/sql"
	"fmt"
	"time"
)

type sqliteWorkQueueStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteWorkQueueStore) Enqueue(item WorkItem) error {
	if item.ID == "" {
		return fmt.Errorf("work item id is required")
	}
	if item.Status == "" {
		item.Status = WorkItemStatusQueued
	}
	if item.LastUpdatedAt.IsZero() {
		item.LastUpdatedAt = item.AvailableAt
	}
	if item.AttemptCount < 0 {
		item.AttemptCount = 0
	}
	if item.DedupeKey != "" {
		var existing string
		err := s.db.db.QueryRow(`
			SELECT work_item_id
			FROM work_items
			WHERE dedupe_key = ?
			  AND status NOT IN (?, ?, ?)
			ORDER BY available_at ASC
			LIMIT 1
		`, item.DedupeKey, string(WorkItemStatusCompleted), string(WorkItemStatusFailed), string(WorkItemStatusAbandoned)).Scan(&existing)
		if err == nil {
			return nil
		}
		if !errorsIsNoRows(err) {
			return err
		}
	}
	_, err := s.db.db.Exec(`
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
		item.LastUpdatedAt.UTC().Format(time.RFC3339Nano), nullString(item.Reason), nullString(string(item.WakeupKind)),
		nullableTime(item.RetryNotBefore), item.AttemptCount, nullString(item.LeaseID), item.FencingToken, nullString(item.ClaimToken),
		nullString(string(item.ClaimedByWorkerID)), nullableTime(item.LeaseExpiresAt))
	return err
}

func (s *sqliteWorkQueueStore) ClaimReady(workerID WorkerID, limit int, now time.Time, leaseTTL time.Duration) ([]WorkClaim, error) {
	if limit <= 0 {
		limit = 1
	}
	tx, err := s.db.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(`
		SELECT work_item_id, kind, status, dedupe_key, graph_id, task_id, execution_id, approval_id, checkpoint_id,
		       workflow_id, available_at, claimed_at, completed_at, failed_at, last_updated_at, reason, wakeup_kind,
		       retry_not_before, attempt_count, lease_id, fencing_token, claim_token, claimed_by_worker_id, lease_expires_at
		FROM work_items
		WHERE status = ?
		  AND available_at <= ?
		ORDER BY available_at ASC, work_item_id ASC
		LIMIT ?
	`, string(WorkItemStatusQueued), now.UTC().Format(time.RFC3339Nano), limit)
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
		result, err := tx.Exec(`
			UPDATE work_items
			SET status = ?, claimed_at = ?, last_updated_at = ?, attempt_count = ?, lease_id = ?, fencing_token = ?, claim_token = ?, claimed_by_worker_id = ?, lease_expires_at = ?
			WHERE work_item_id = ? AND status = ?
		`, string(item.Status), claimedAt.Format(time.RFC3339Nano), item.LastUpdatedAt.Format(time.RFC3339Nano), item.AttemptCount, item.LeaseID, item.FencingToken, item.ClaimToken, string(workerID), expires.Format(time.RFC3339Nano), item.ID, string(WorkItemStatusQueued))
		if err != nil {
			return nil, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected != 1 {
			continue
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

func (s *sqliteWorkQueueStore) Heartbeat(heartbeat LeaseHeartbeat) error {
	return s.withFence(heartbeat.WorkItemID, heartbeat.LeaseID, heartbeat.FencingToken, heartbeat.WorkerID, func(item WorkItem) error {
		_, err := s.db.db.Exec(`
			UPDATE work_items
			SET last_updated_at = ?, lease_expires_at = ?
			WHERE work_item_id = ?
		`, heartbeat.RecordedAt.UTC().Format(time.RFC3339Nano), heartbeat.LeaseExpiresAt.UTC().Format(time.RFC3339Nano), heartbeat.WorkItemID)
		return err
	})
}

func (s *sqliteWorkQueueStore) Complete(fence FenceValidation, now time.Time) error {
	return s.finish(fence, WorkItemStatusCompleted, "", now)
}

func (s *sqliteWorkQueueStore) Fail(fence FenceValidation, summary string, now time.Time) error {
	return s.finish(fence, WorkItemStatusFailed, summary, now)
}

func (s *sqliteWorkQueueStore) Requeue(fence FenceValidation, nextAvailableAt time.Time, reason string, now time.Time) error {
	return s.withFence(fence.WorkItemID, fence.LeaseID, fence.FencingToken, fence.WorkerID, func(item WorkItem) error {
		_, err := s.db.db.Exec(`
			UPDATE work_items
			SET status = ?, available_at = ?, last_updated_at = ?, reason = ?, claimed_at = NULL,
			    lease_id = NULL, claim_token = NULL, claimed_by_worker_id = NULL, lease_expires_at = NULL
			WHERE work_item_id = ?
		`, string(WorkItemStatusQueued), nextAvailableAt.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), reason, fence.WorkItemID)
		return err
	})
}

func (s *sqliteWorkQueueStore) ReclaimExpired(now time.Time) ([]LeaseReclaimResult, error) {
	rows, err := s.db.db.Query(`
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
		_, err = s.db.db.Exec(`
			UPDATE work_items
			SET status = ?, reason = ?, last_updated_at = ?, claimed_at = NULL, lease_id = NULL, claim_token = NULL,
			    claimed_by_worker_id = NULL, lease_expires_at = NULL
			WHERE work_item_id = ? AND status = ? AND fencing_token = ?
		`, string(WorkItemStatusQueued), "reclaimed after lease expiry", now.UTC().Format(time.RFC3339Nano), item.ID, string(WorkItemStatusClaimed), item.FencingToken)
		if err != nil {
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

func (s *sqliteWorkQueueStore) Load(workItemID string) (WorkItem, bool, error) {
	row := s.db.db.QueryRow(`
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

func (s *sqliteWorkQueueStore) ListByGraph(graphID string) ([]WorkItem, error) {
	rows, err := s.db.db.Query(`
		SELECT work_item_id, kind, status, dedupe_key, graph_id, task_id, execution_id, approval_id, checkpoint_id,
		       workflow_id, available_at, claimed_at, completed_at, failed_at, last_updated_at, reason, wakeup_kind,
		       retry_not_before, attempt_count, lease_id, fencing_token, claim_token, claimed_by_worker_id, lease_expires_at
		FROM work_items
		WHERE graph_id = ?
		ORDER BY available_at ASC, work_item_id ASC
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

func (s *sqliteWorkQueueStore) ValidateFence(fence FenceValidation) error {
	return s.withFence(fence.WorkItemID, fence.LeaseID, fence.FencingToken, fence.WorkerID, func(_ WorkItem) error { return nil })
}

func (s *sqliteWorkQueueStore) finish(fence FenceValidation, status WorkItemStatus, summary string, now time.Time) error {
	return s.withFence(fence.WorkItemID, fence.LeaseID, fence.FencingToken, fence.WorkerID, func(item WorkItem) error {
		query := `
			UPDATE work_items
			SET status = ?, reason = ?, last_updated_at = ?, claimed_at = NULL, lease_id = NULL, claim_token = NULL,
			    claimed_by_worker_id = NULL, lease_expires_at = NULL, completed_at = ?, failed_at = ?
			WHERE work_item_id = ?
		`
		var completedAt any
		var failedAt any
		if status == WorkItemStatusCompleted {
			completedAt = now.UTC().Format(time.RFC3339Nano)
		}
		if status == WorkItemStatusFailed {
			failedAt = now.UTC().Format(time.RFC3339Nano)
		}
		_, err := s.db.db.Exec(query, string(status), summary, now.UTC().Format(time.RFC3339Nano), completedAt, failedAt, item.ID)
		return err
	})
}

func (s *sqliteWorkQueueStore) withFence(workItemID string, leaseID string, fencingToken int64, workerID WorkerID, fn func(WorkItem) error) error {
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

type sqliteWorkAttemptStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteWorkAttemptStore) SaveAttempt(attempt ExecutionAttempt) error {
	raw, err := marshalJSON(attempt)
	if err != nil {
		return err
	}
	_, err = s.db.db.Exec(`
		INSERT INTO work_attempts (
			attempt_id, work_item_id, work_item_kind, graph_id, task_id, execution_id, approval_id, worker_id,
			lease_id, fencing_token, status, failure_category, failure_summary, started_at, finished_at, checkpoint_id,
			produced_artifact_ids_json, record_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, attempt.AttemptID, attempt.WorkItemID, string(attempt.WorkItemKind), nullString(attempt.GraphID), nullString(attempt.TaskID),
		nullString(attempt.ExecutionID), nullString(attempt.ApprovalID), string(attempt.WorkerID), attempt.LeaseID, attempt.FencingToken,
		string(attempt.Status), nullString(string(attempt.FailureCategory)), nullString(attempt.FailureSummary),
		attempt.StartedAt.UTC().Format(time.RFC3339Nano), nullableTime(attempt.FinishedAt), nullString(attempt.CheckpointID),
		mustMarshalStrings(attempt.ProducedArtifactIDs), raw)
	return err
}

func (s *sqliteWorkAttemptStore) UpdateAttempt(attempt ExecutionAttempt) error {
	raw, err := marshalJSON(attempt)
	if err != nil {
		return err
	}
	result, err := s.db.db.Exec(`
		UPDATE work_attempts
		SET status = ?, failure_category = ?, failure_summary = ?, finished_at = ?, checkpoint_id = ?,
		    produced_artifact_ids_json = ?, record_json = ?
		WHERE attempt_id = ?
	`, string(attempt.Status), nullString(string(attempt.FailureCategory)), nullString(attempt.FailureSummary), nullableTime(attempt.FinishedAt),
		nullString(attempt.CheckpointID), mustMarshalStrings(attempt.ProducedArtifactIDs), raw, attempt.AttemptID)
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

func (s *sqliteWorkAttemptStore) ListAttempts(workItemID string) ([]ExecutionAttempt, error) {
	rows, err := s.db.db.Query(`SELECT record_json FROM work_attempts WHERE work_item_id = ? ORDER BY started_at ASC, attempt_id ASC`, workItemID)
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

type sqliteWorkerRegistryStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteWorkerRegistryStore) Register(worker WorkerRegistration) error {
	_, err := s.db.db.Exec(`
		INSERT INTO worker_registrations (worker_id, role, backend_profile, started_at, last_heartbeat)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(worker_id) DO UPDATE SET
			role = excluded.role,
			backend_profile = excluded.backend_profile,
			started_at = excluded.started_at,
			last_heartbeat = excluded.last_heartbeat
	`, string(worker.WorkerID), string(worker.Role), nullString(worker.BackendProfile), worker.StartedAt.UTC().Format(time.RFC3339Nano), worker.LastHeartbeat.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteWorkerRegistryStore) Heartbeat(workerID WorkerID, now time.Time) error {
	result, err := s.db.db.Exec(`UPDATE worker_registrations SET last_heartbeat = ? WHERE worker_id = ?`, now.UTC().Format(time.RFC3339Nano), string(workerID))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return &NotFoundError{Resource: "worker", ID: string(workerID)}
	}
	return nil
}

func (s *sqliteWorkerRegistryStore) Load(workerID WorkerID) (WorkerRegistration, bool, error) {
	row := s.db.db.QueryRow(`SELECT role, backend_profile, started_at, last_heartbeat FROM worker_registrations WHERE worker_id = ?`, string(workerID))
	var (
		record   WorkerRegistration
		role     string
		profile  sql.NullString
		started  string
		lastBeat string
	)
	record.WorkerID = workerID
	if err := row.Scan(&role, &profile, &started, &lastBeat); err != nil {
		if errorsIsNoRows(err) {
			return WorkerRegistration{}, false, nil
		}
		return WorkerRegistration{}, false, err
	}
	record.Role = WorkerRole(role)
	record.BackendProfile = profile.String
	record.StartedAt = mustParseTime(started)
	record.LastHeartbeat = mustParseTime(lastBeat)
	return record, true, nil
}

func (s *sqliteWorkerRegistryStore) List() ([]WorkerRegistration, error) {
	rows, err := s.db.db.Query(`SELECT worker_id, role, backend_profile, started_at, last_heartbeat FROM worker_registrations ORDER BY worker_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]WorkerRegistration, 0)
	for rows.Next() {
		var (
			record  WorkerRegistration
			role    string
			profile sql.NullString
			started string
			beat    string
		)
		if err := rows.Scan(&record.WorkerID, &role, &profile, &started, &beat); err != nil {
			return nil, err
		}
		record.Role = WorkerRole(role)
		record.BackendProfile = profile.String
		record.StartedAt = mustParseTime(started)
		record.LastHeartbeat = mustParseTime(beat)
		result = append(result, record)
	}
	return result, rows.Err()
}

type sqliteSchedulerStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteSchedulerStore) SaveWakeup(wakeup SchedulerWakeup) error {
	_, err := s.db.db.Exec(`
		INSERT INTO scheduler_wakeups (wakeup_id, graph_id, task_id, execution_id, approval_id, kind, available_at, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(wakeup_id) DO UPDATE SET
			graph_id = excluded.graph_id,
			task_id = excluded.task_id,
			execution_id = excluded.execution_id,
			approval_id = excluded.approval_id,
			kind = excluded.kind,
			available_at = excluded.available_at,
			reason = excluded.reason
	`, wakeup.ID, nullString(wakeup.GraphID), nullString(wakeup.TaskID), nullString(wakeup.ExecutionID), nullString(wakeup.ApprovalID), string(wakeup.Kind), wakeup.AvailableAt.UTC().Format(time.RFC3339Nano), nullString(wakeup.Reason))
	return err
}

func (s *sqliteSchedulerStore) ListDueWakeups(now time.Time) ([]SchedulerWakeup, error) {
	rows, err := s.db.db.Query(`
		SELECT wakeup_id, graph_id, task_id, execution_id, approval_id, kind, available_at, reason
		FROM scheduler_wakeups
		WHERE available_at <= ?
		ORDER BY available_at ASC, wakeup_id ASC
	`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SchedulerWakeup, 0)
	for rows.Next() {
		var (
			wakeup      SchedulerWakeup
			graphID     sql.NullString
			taskID      sql.NullString
			executionID sql.NullString
			approvalID  sql.NullString
			kind        string
			availableAt string
			reason      sql.NullString
		)
		if err := rows.Scan(&wakeup.ID, &graphID, &taskID, &executionID, &approvalID, &kind, &availableAt, &reason); err != nil {
			return nil, err
		}
		wakeup.GraphID = graphID.String
		wakeup.TaskID = taskID.String
		wakeup.ExecutionID = executionID.String
		wakeup.ApprovalID = approvalID.String
		wakeup.Kind = SchedulerWakeupKind(kind)
		wakeup.AvailableAt = mustParseTime(availableAt)
		wakeup.Reason = reason.String
		result = append(result, wakeup)
	}
	return result, rows.Err()
}

func (s *sqliteSchedulerStore) MarkWakeupDispatched(id string, _ time.Time) error {
	_, err := s.db.db.Exec(`DELETE FROM scheduler_wakeups WHERE wakeup_id = ?`, id)
	return err
}

func scanWorkItem(scanner interface{ Scan(dest ...any) error }) (WorkItem, error) {
	var (
		item           WorkItem
		kind           string
		status         string
		dedupeKey      sql.NullString
		graphID        sql.NullString
		taskID         sql.NullString
		executionID    sql.NullString
		approvalID     sql.NullString
		checkpointID   sql.NullString
		workflowID     sql.NullString
		availableAt    string
		claimedAt      sql.NullString
		completedAt    sql.NullString
		failedAt       sql.NullString
		lastUpdatedAt  string
		reason         sql.NullString
		wakeupKind     sql.NullString
		retryNotBefore sql.NullString
		leaseID        sql.NullString
		claimToken     sql.NullString
		workerID       sql.NullString
		leaseExpiresAt sql.NullString
	)
	if err := scanner.Scan(
		&item.ID, &kind, &status, &dedupeKey, &graphID, &taskID, &executionID, &approvalID, &checkpointID,
		&workflowID, &availableAt, &claimedAt, &completedAt, &failedAt, &lastUpdatedAt, &reason, &wakeupKind,
		&retryNotBefore, &item.AttemptCount, &leaseID, &item.FencingToken, &claimToken, &workerID, &leaseExpiresAt,
	); err != nil {
		return WorkItem{}, err
	}
	item.Kind = WorkItemKind(kind)
	item.Status = WorkItemStatus(status)
	item.DedupeKey = dedupeKey.String
	item.GraphID = graphID.String
	item.TaskID = taskID.String
	item.ExecutionID = executionID.String
	item.ApprovalID = approvalID.String
	item.CheckpointID = checkpointID.String
	item.WorkflowID = workflowID.String
	item.AvailableAt = mustParseTime(availableAt)
	item.LastUpdatedAt = mustParseTime(lastUpdatedAt)
	item.Reason = reason.String
	item.WakeupKind = SchedulerWakeupKind(wakeupKind.String)
	if claimedAt.Valid {
		value := mustParseTime(claimedAt.String)
		item.ClaimedAt = &value
	}
	if completedAt.Valid {
		value := mustParseTime(completedAt.String)
		item.CompletedAt = &value
	}
	if failedAt.Valid {
		value := mustParseTime(failedAt.String)
		item.FailedAt = &value
	}
	if retryNotBefore.Valid {
		value := mustParseTime(retryNotBefore.String)
		item.RetryNotBefore = &value
	}
	item.LeaseID = leaseID.String
	item.ClaimToken = claimToken.String
	item.ClaimedByWorkerID = WorkerID(workerID.String)
	if leaseExpiresAt.Valid {
		value := mustParseTime(leaseExpiresAt.String)
		item.LeaseExpiresAt = &value
	}
	return item, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func mustMarshalStrings(values []string) string {
	raw, _ := marshalJSON(values)
	return raw
}
