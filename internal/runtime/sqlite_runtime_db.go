package runtime

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	_ "modernc.org/sqlite"
)

type SQLiteRuntimeDB struct {
	db          *sql.DB
	dbPath      string
	artifactDir string
}

type SQLiteRuntimeStores struct {
	DB               *SQLiteRuntimeDB
	WorkflowRuns     WorkflowRunStore
	TaskGraphs       TaskGraphStore
	Executions       TaskExecutionStore
	SkillExecutions  SkillExecutionStore
	Approvals        ApprovalStateStore
	OperatorActions  OperatorActionStore
	Checkpoints      CheckpointStore
	Replay           ReplayStore
	ReplayProjection ReplayProjectionStore
	ReplayQuery      ReplayProjectionQueryStore
	Artifacts        ArtifactMetadataStore
	WorkQueue        WorkQueueStore
	WorkAttempts     WorkAttemptStore
	Workers          WorkerRegistryStore
	Scheduler        SchedulerStore
}

func NewSQLiteRuntimeStores(dbPath string) (*SQLiteRuntimeStores, error) {
	db, err := NewSQLiteRuntimeDB(dbPath)
	if err != nil {
		return nil, err
	}
	return &SQLiteRuntimeStores{
		DB:               db,
		WorkflowRuns:     &sqliteWorkflowRunStore{db: db},
		TaskGraphs:       &sqliteTaskGraphStore{db: db},
		Executions:       &sqliteTaskExecutionStore{db: db},
		SkillExecutions:  &sqliteSkillExecutionStore{db: db},
		Approvals:        &sqliteApprovalStateStore{db: db},
		OperatorActions:  &sqliteOperatorActionStore{db: db},
		Checkpoints:      &sqliteCheckpointStore{db: db},
		Replay:           &sqliteReplayStore{db: db},
		ReplayProjection: &sqliteReplayProjectionStore{db: db},
		ReplayQuery:      &sqliteReplayProjectionStore{db: db},
		Artifacts:        &sqliteArtifactMetadataStore{db: db},
		WorkQueue:        &sqliteWorkQueueStore{db: db},
		WorkAttempts:     &sqliteWorkAttemptStore{db: db},
		Workers:          &sqliteWorkerRegistryStore{db: db},
		Scheduler:        &sqliteSchedulerStore{db: db},
	}, nil
}

func NewSQLiteRuntimeDB(dbPath string) (*SQLiteRuntimeDB, error) {
	if strings.TrimSpace(dbPath) == "" {
		dbPath = filepath.Join(".", "var", "runtime.db")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	runtimeDB := &SQLiteRuntimeDB{
		db:          db,
		dbPath:      dbPath,
		artifactDir: filepath.Join(filepath.Dir(dbPath), "artifacts"),
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.MkdirAll(runtimeDB.artifactDir, 0o755); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := runtimeDB.EnsureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return runtimeDB, nil
}

func (db *SQLiteRuntimeDB) Close() error {
	if db == nil || db.db == nil {
		return nil
	}
	return db.db.Close()
}

func (db *SQLiteRuntimeDB) EnsureSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS runtime_schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS task_graphs (
			graph_id TEXT PRIMARY KEY,
			parent_workflow_id TEXT NOT NULL,
			parent_task_id TEXT NOT NULL,
			trigger_source TEXT NOT NULL,
			generated_at TEXT NOT NULL,
			version INTEGER NOT NULL,
			base_state_snapshot_ref TEXT NOT NULL,
			latest_committed_state_ref TEXT NOT NULL,
			graph_json TEXT NOT NULL,
			registered_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS follow_up_tasks (
			graph_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			task_json TEXT NOT NULL,
			metadata_json TEXT NOT NULL,
			status TEXT NOT NULL,
			required_capability TEXT NOT NULL,
			missing_capability_reason TEXT,
			blocking_reasons_json TEXT,
			suppressed_reasons_json TEXT,
			registered_at TEXT NOT NULL,
			registration_order INTEGER NOT NULL,
			last_updated_at TEXT NOT NULL,
			PRIMARY KEY (graph_id, task_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_follow_up_tasks_graph_status
			ON follow_up_tasks(graph_id, status);`,
		`CREATE TABLE IF NOT EXISTS task_dependencies (
			graph_id TEXT NOT NULL,
			upstream_task_id TEXT NOT NULL,
			downstream_task_id TEXT NOT NULL,
			reason TEXT NOT NULL,
			mandatory INTEGER NOT NULL,
			PRIMARY KEY (graph_id, upstream_task_id, downstream_task_id)
		);`,
		`CREATE TABLE IF NOT EXISTS state_snapshots (
			snapshot_ref TEXT PRIMARY KEY,
			graph_id TEXT,
			workflow_id TEXT,
			task_id TEXT,
			kind TEXT NOT NULL,
			state_version INTEGER NOT NULL,
			reason TEXT NOT NULL,
			captured_at TEXT NOT NULL,
			state_json TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS task_executions (
			execution_id TEXT PRIMARY KEY,
			graph_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			workflow_id TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			last_transition_at TEXT NOT NULL,
			version INTEGER NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS skill_executions (
			execution_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_skill_executions_workflow
			ON skill_executions(workflow_id, updated_at DESC);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_task_executions_execution_id
			ON task_executions(execution_id);`,
		`CREATE INDEX IF NOT EXISTS idx_task_executions_task_started
			ON task_executions(task_id, started_at DESC);`,
		`CREATE TABLE IF NOT EXISTS checkpoints (
			checkpoint_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			state TEXT NOT NULL,
			resume_state TEXT NOT NULL,
			state_version INTEGER NOT NULL,
			summary TEXT NOT NULL,
			captured_at TEXT NOT NULL,
			payload_kind TEXT,
			payload_json TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS resume_tokens (
			token TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			checkpoint_id TEXT NOT NULL,
			issued_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_resume_tokens_token
			ON resume_tokens(token);`,
		`CREATE TABLE IF NOT EXISTS approvals (
			approval_id TEXT PRIMARY KEY,
			graph_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			workflow_id TEXT NOT NULL,
			execution_id TEXT,
			requested_action TEXT NOT NULL,
			required_roles_json TEXT,
			requested_at TEXT NOT NULL,
			deadline TEXT,
			status TEXT NOT NULL,
			resolved_at TEXT,
			resolved_by TEXT,
			resolution_note TEXT,
			version INTEGER NOT NULL
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_approvals_approval_id
			ON approvals(approval_id);`,
		`CREATE TABLE IF NOT EXISTS operator_actions (
			action_id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL,
			action_type TEXT NOT NULL,
			actor TEXT NOT NULL,
			roles_json TEXT,
			graph_id TEXT,
			task_id TEXT,
			approval_id TEXT,
			workflow_id TEXT,
			status TEXT NOT NULL,
			note TEXT,
			requested_at TEXT NOT NULL,
			applied_at TEXT,
			failure_summary TEXT,
			expected_version INTEGER
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_operator_actions_request_id
			ON operator_actions(request_id);`,
		`CREATE TABLE IF NOT EXISTS replay_events (
			event_id TEXT PRIMARY KEY,
			root_correlation_id TEXT,
			parent_workflow_id TEXT,
			workflow_id TEXT,
			graph_id TEXT,
			task_id TEXT,
			approval_id TEXT,
			execution_id TEXT,
			action_type TEXT NOT NULL,
			summary TEXT NOT NULL,
			occurred_at TEXT NOT NULL,
			details_json TEXT,
			committed_state_ref TEXT,
			updated_state_ref TEXT,
			artifact_ids_json TEXT,
			operator_action_id TEXT,
			checkpoint_id TEXT,
			resume_token TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_replay_events_graph_time
			ON replay_events(graph_id, occurred_at);`,
		`CREATE INDEX IF NOT EXISTS idx_replay_events_task_time
			ON replay_events(task_id, occurred_at);`,
		`CREATE TABLE IF NOT EXISTS workflow_runs (
			workflow_id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			intent TEXT NOT NULL,
			runtime_state TEXT NOT NULL,
			failure_category TEXT,
			failure_summary TEXT,
			approval_id TEXT,
			checkpoint_id TEXT,
			resume_token TEXT,
			task_graph_id TEXT,
			root_correlation_id TEXT,
			summary TEXT,
			started_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			record_json TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS replay_projection_builds (
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			status TEXT NOT NULL,
			degradation_reasons_json TEXT,
			built_at TEXT NOT NULL,
			source_event_count INTEGER NOT NULL,
			source_artifact_count INTEGER NOT NULL,
			PRIMARY KEY (scope_kind, scope_id)
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_replay_projections (
			workflow_id TEXT PRIMARY KEY,
			task_id TEXT,
			intent TEXT,
			runtime_state TEXT NOT NULL,
			failure_category TEXT,
			approval_id TEXT,
			bundle_artifact_id TEXT,
			summary_artifact_id TEXT,
			projection_status TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			degradation_reasons_json TEXT,
			summary_json TEXT,
			explanation_json TEXT,
			compare_input_json TEXT,
			updated_at TEXT NOT NULL,
			projection_freshness TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS task_graph_replay_projections (
			graph_id TEXT PRIMARY KEY,
			parent_workflow_id TEXT,
			parent_task_id TEXT,
			runtime_state TEXT NOT NULL,
			pending_approval_id TEXT,
			bundle_artifact_id TEXT,
			summary_artifact_id TEXT,
			projection_status TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			degradation_reasons_json TEXT,
			summary_json TEXT,
			explanation_json TEXT,
			compare_input_json TEXT,
			updated_at TEXT NOT NULL,
			projection_freshness TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS replay_provenance_nodes (
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			node_type TEXT NOT NULL,
			ref_id TEXT,
			label TEXT NOT NULL,
			summary TEXT,
			attributes_json TEXT,
			PRIMARY KEY (scope_kind, scope_id, node_id)
		);`,
		`CREATE TABLE IF NOT EXISTS replay_provenance_edges (
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			edge_id TEXT NOT NULL,
			from_node_id TEXT NOT NULL,
			to_node_id TEXT NOT NULL,
			edge_type TEXT NOT NULL,
			reason TEXT,
			attributes_json TEXT,
			PRIMARY KEY (scope_kind, scope_id, edge_id)
		);`,
		`CREATE TABLE IF NOT EXISTS replay_execution_attributions (
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			execution_id TEXT NOT NULL,
			category TEXT NOT NULL,
			summary TEXT NOT NULL,
			source_refs_json TEXT,
			details_json TEXT,
			PRIMARY KEY (scope_kind, scope_id, execution_id, category)
		);`,
		`CREATE TABLE IF NOT EXISTS replay_failure_attributions (
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			attribution_id TEXT NOT NULL,
			failure_category TEXT NOT NULL,
			reason_code TEXT,
			summary TEXT NOT NULL,
			related_kind TEXT,
			related_id TEXT,
			source_refs_json TEXT,
			details_json TEXT,
			PRIMARY KEY (scope_kind, scope_id, attribution_id)
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_artifacts (
			artifact_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			workflow_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			produced_by TEXT NOT NULL,
			summary TEXT,
			storage_ref TEXT,
			created_at TEXT NOT NULL,
			ref_json TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS work_items (
			work_item_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			dedupe_key TEXT,
			graph_id TEXT,
			task_id TEXT,
			execution_id TEXT,
			approval_id TEXT,
			checkpoint_id TEXT,
			workflow_id TEXT,
			available_at TEXT NOT NULL,
			claimed_at TEXT,
			completed_at TEXT,
			failed_at TEXT,
			last_updated_at TEXT NOT NULL,
			reason TEXT,
			wakeup_kind TEXT,
			retry_not_before TEXT,
			attempt_count INTEGER NOT NULL,
			lease_id TEXT,
			fencing_token INTEGER NOT NULL,
			claim_token TEXT,
			claimed_by_worker_id TEXT,
			lease_expires_at TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_work_items_status_available
			ON work_items(status, available_at);`,
		`CREATE INDEX IF NOT EXISTS idx_work_items_graph
			ON work_items(graph_id, available_at);`,
		`CREATE TABLE IF NOT EXISTS work_attempts (
			attempt_id TEXT PRIMARY KEY,
			work_item_id TEXT NOT NULL,
			work_item_kind TEXT NOT NULL,
			graph_id TEXT,
			task_id TEXT,
			execution_id TEXT,
			approval_id TEXT,
			worker_id TEXT NOT NULL,
			lease_id TEXT NOT NULL,
			fencing_token INTEGER NOT NULL,
			status TEXT NOT NULL,
			failure_category TEXT,
			failure_summary TEXT,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			checkpoint_id TEXT,
			produced_artifact_ids_json TEXT,
			record_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_work_attempts_work_item_started
			ON work_attempts(work_item_id, started_at ASC);`,
		`CREATE TABLE IF NOT EXISTS worker_registrations (
			worker_id TEXT PRIMARY KEY,
			role TEXT NOT NULL,
			backend_profile TEXT,
			started_at TEXT NOT NULL,
			last_heartbeat TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS scheduler_wakeups (
			wakeup_id TEXT PRIMARY KEY,
			graph_id TEXT,
			task_id TEXT,
			execution_id TEXT,
			approval_id TEXT,
			kind TEXT NOT NULL,
			available_at TEXT NOT NULL,
			reason TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_scheduler_wakeups_available
			ON scheduler_wakeups(available_at);`,
	}
	for _, stmt := range statements {
		if _, err := db.db.Exec(stmt); err != nil {
			return err
		}
	}
	_, err := db.db.Exec(`INSERT OR IGNORE INTO runtime_schema_migrations(version, applied_at) VALUES (?, ?);`, ReplayProjectionSchemaVersion, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (db *SQLiteRuntimeDB) begin() (*sql.Tx, error) {
	return db.db.Begin()
}

func (db *SQLiteRuntimeDB) saveStateSnapshotTx(tx *sql.Tx, graphID string, workflowID string, taskID string, kind string, snapshot state.StateSnapshot) (string, error) {
	ref := snapshotRefFor(snapshot)
	if ref == "" {
		return "", fmt.Errorf("state snapshot requires snapshot id")
	}
	stateJSON, err := json.Marshal(snapshot.State)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(`
		INSERT INTO state_snapshots (
			snapshot_ref, graph_id, workflow_id, task_id, kind, state_version, reason, captured_at, state_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(snapshot_ref) DO UPDATE SET
			graph_id=excluded.graph_id,
			workflow_id=excluded.workflow_id,
			task_id=excluded.task_id,
			kind=excluded.kind,
			state_version=excluded.state_version,
			reason=excluded.reason,
			captured_at=excluded.captured_at,
			state_json=excluded.state_json
	`, ref, graphID, workflowID, taskID, kind, snapshot.State.Version.Sequence, snapshot.Reason, snapshot.CapturedAt.UTC().Format(time.RFC3339Nano), string(stateJSON)); err != nil {
		return "", err
	}
	return ref, nil
}

func (db *SQLiteRuntimeDB) saveStateSnapshot(graphID string, workflowID string, taskID string, kind string, snapshot state.StateSnapshot) (string, error) {
	tx, err := db.begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	ref, err := db.saveStateSnapshotTx(tx, graphID, workflowID, taskID, kind, snapshot)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return ref, nil
}

func (db *SQLiteRuntimeDB) loadStateSnapshot(snapshotRef string) (state.StateSnapshot, bool, error) {
	row := db.db.QueryRow(`
		SELECT reason, captured_at, state_json
		FROM state_snapshots
		WHERE snapshot_ref = ?
	`, snapshotRef)
	var (
		reason     string
		capturedAt string
		stateJSON  string
	)
	if err := row.Scan(&reason, &capturedAt, &stateJSON); err != nil {
		if errorsIsNoRows(err) {
			return state.StateSnapshot{}, false, nil
		}
		return state.StateSnapshot{}, false, err
	}
	var world state.FinancialWorldState
	if err := json.Unmarshal([]byte(stateJSON), &world); err != nil {
		return state.StateSnapshot{}, false, err
	}
	return state.StateSnapshot{
		State:      world,
		Reason:     reason,
		CapturedAt: mustParseTime(capturedAt),
	}, true, nil
}

func marshalJSON(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalJSON(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}

func mustParseTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func nullableTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}

func mapSQLiteConstraint(err error, resource string, id string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if strings.Contains(message, "UNIQUE constraint failed") || strings.Contains(message, "constraint failed") {
		return &ConflictError{Resource: resource, ID: id, Reason: message}
	}
	return err
}

func artifactSummary(artifact reporting.WorkflowArtifact) string {
	if artifact.Ref.Summary != "" {
		return artifact.Ref.Summary
	}
	return string(artifact.Kind)
}
