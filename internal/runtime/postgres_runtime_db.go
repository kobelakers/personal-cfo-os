package runtime

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresRuntimeDB struct {
	db  *sql.DB
	dsn string
}

type PostgresRuntimeStores struct {
	DB               *PostgresRuntimeDB
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

func NewPostgresRuntimeStores(dsn string) (*PostgresRuntimeStores, error) {
	db, err := NewPostgresRuntimeDB(dsn)
	if err != nil {
		return nil, err
	}
	return &PostgresRuntimeStores{
		DB:               db,
		WorkflowRuns:     &postgresWorkflowRunStore{db: db},
		TaskGraphs:       &postgresTaskGraphStore{db: db},
		Executions:       &postgresTaskExecutionStore{db: db},
		SkillExecutions:  &postgresSkillExecutionStore{db: db},
		Approvals:        &postgresApprovalStateStore{db: db},
		OperatorActions:  &postgresOperatorActionStore{db: db},
		Checkpoints:      &postgresCheckpointStore{db: db},
		Replay:           &postgresReplayStore{db: db},
		ReplayProjection: &postgresReplayProjectionStore{db: db},
		ReplayQuery:      &postgresReplayProjectionStore{db: db},
		Artifacts:        &postgresArtifactMetadataStore{db: db},
		WorkQueue:        &postgresWorkQueueStore{db: db},
		WorkAttempts:     &postgresWorkAttemptStore{db: db},
		Workers:          &postgresWorkerRegistryStore{db: db},
		Scheduler:        &postgresSchedulerStore{db: db},
	}, nil
}

func NewPostgresRuntimeDB(dsn string) (*PostgresRuntimeDB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("postgres runtime dsn is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	runtimeDB := &PostgresRuntimeDB{db: db, dsn: dsn}
	if err := runtimeDB.EnsureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return runtimeDB, nil
}

func (db *PostgresRuntimeDB) Close() error {
	if db == nil || db.db == nil {
		return nil
	}
	return db.db.Close()
}

func (db *PostgresRuntimeDB) EnsureSchema() error {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(7015001)`); err != nil {
		return err
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS runtime_schema_migrations (
			version BIGINT PRIMARY KEY,
			applied_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS task_graphs (
			graph_id TEXT PRIMARY KEY,
			version BIGINT NOT NULL,
			snapshot_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS state_snapshots (
			snapshot_ref TEXT PRIMARY KEY,
			graph_id TEXT,
			workflow_id TEXT,
			task_id TEXT,
			kind TEXT NOT NULL,
			state_version BIGINT NOT NULL,
			reason TEXT NOT NULL,
			captured_at TEXT NOT NULL,
			state_json TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS task_executions (
			execution_id TEXT PRIMARY KEY,
			graph_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			started_at TEXT NOT NULL,
			version BIGINT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_task_executions_task_started
			ON task_executions(graph_id, task_id, started_at);`,
		`CREATE TABLE IF NOT EXISTS skill_executions (
			execution_id TEXT PRIMARY KEY,
			workflow_id TEXT,
			task_id TEXT,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_skill_executions_workflow_updated
			ON skill_executions(workflow_id, updated_at DESC, execution_id ASC);`,
		`CREATE TABLE IF NOT EXISTS approvals (
			approval_id TEXT PRIMARY KEY,
			graph_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			status TEXT NOT NULL,
			version BIGINT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_approvals_graph_task
			ON approvals(graph_id, task_id);`,
		`CREATE TABLE IF NOT EXISTS operator_actions (
			action_id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL UNIQUE,
			graph_id TEXT,
			task_id TEXT,
			record_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_operator_actions_task
			ON operator_actions(graph_id, task_id);`,
		`CREATE TABLE IF NOT EXISTS checkpoints (
			checkpoint_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			record_json TEXT NOT NULL,
			payload_kind TEXT,
			payload_json TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS resume_tokens (
			token TEXT PRIMARY KEY,
			record_json TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS replay_events (
			event_id TEXT PRIMARY KEY,
			workflow_id TEXT,
			graph_id TEXT,
			task_id TEXT,
			approval_id TEXT,
			execution_id TEXT,
			occurred_at TEXT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_replay_events_graph_time
			ON replay_events(graph_id, occurred_at);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_replay_events_task_time
			ON replay_events(task_id, occurred_at);`,
		`CREATE TABLE IF NOT EXISTS workflow_runs (
			workflow_id TEXT PRIMARY KEY,
			updated_at TEXT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS replay_projection_builds (
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			record_json TEXT NOT NULL,
			PRIMARY KEY (scope_kind, scope_id)
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_replay_projections (
			workflow_id TEXT PRIMARY KEY,
			projection_freshness TEXT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS task_graph_replay_projections (
			graph_id TEXT PRIMARY KEY,
			projection_freshness TEXT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS replay_provenance_nodes (
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			record_json TEXT NOT NULL,
			PRIMARY KEY (scope_kind, scope_id, node_id)
		);`,
		`CREATE TABLE IF NOT EXISTS replay_provenance_edges (
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			edge_id TEXT NOT NULL,
			record_json TEXT NOT NULL,
			PRIMARY KEY (scope_kind, scope_id, edge_id)
		);`,
		`CREATE TABLE IF NOT EXISTS replay_execution_attributions (
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			execution_id TEXT NOT NULL,
			category TEXT NOT NULL,
			record_json TEXT NOT NULL,
			PRIMARY KEY (scope_kind, scope_id, execution_id, category)
		);`,
		`CREATE TABLE IF NOT EXISTS replay_failure_attributions (
			scope_kind TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			attribution_id TEXT NOT NULL,
			record_json TEXT NOT NULL,
			PRIMARY KEY (scope_kind, scope_id, attribution_id)
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_artifacts (
			artifact_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			storage_ref TEXT,
			record_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_workflow_artifacts_task_created
			ON workflow_artifacts(task_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_workflow_artifacts_workflow_created
			ON workflow_artifacts(workflow_id, created_at);`,
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
			attempt_count BIGINT NOT NULL,
			lease_id TEXT,
			fencing_token BIGINT NOT NULL,
			claim_token TEXT,
			claimed_by_worker_id TEXT,
			lease_expires_at TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_work_items_status_available
			ON work_items(status, available_at);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_pg_work_items_active_dedupe
			ON work_items(dedupe_key)
			WHERE dedupe_key IS NOT NULL AND status IN ('queued', 'claimed');`,
		`CREATE TABLE IF NOT EXISTS work_attempts (
			attempt_id TEXT PRIMARY KEY,
			work_item_id TEXT NOT NULL,
			started_at TEXT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_work_attempts_work_item_started
			ON work_attempts(work_item_id, started_at);`,
		`CREATE TABLE IF NOT EXISTS worker_registrations (
			worker_id TEXT PRIMARY KEY,
			last_heartbeat TEXT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS scheduler_wakeups (
			wakeup_id TEXT PRIMARY KEY,
			available_at TEXT NOT NULL,
			record_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pg_scheduler_wakeups_available
			ON scheduler_wakeups(available_at);`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(rewritePositionalSQL(`INSERT INTO runtime_schema_migrations(version, applied_at) VALUES (?, ?) ON CONFLICT(version) DO NOTHING`), ReplayProjectionSchemaVersion, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *PostgresRuntimeDB) exec(query string, args ...any) (sql.Result, error) {
	return db.db.Exec(rewritePositionalSQL(query), args...)
}

func (db *PostgresRuntimeDB) query(query string, args ...any) (*sql.Rows, error) {
	return db.db.Query(rewritePositionalSQL(query), args...)
}

func (db *PostgresRuntimeDB) queryRow(query string, args ...any) *sql.Row {
	return db.db.QueryRow(rewritePositionalSQL(query), args...)
}

func rewritePositionalSQL(query string) string {
	var b strings.Builder
	index := 1
	for _, ch := range query {
		if ch == '?' {
			b.WriteString(fmt.Sprintf("$%d", index))
			index++
			continue
		}
		b.WriteRune(ch)
	}
	return b.String()
}

func resolvePostgresDSN(profile, runtimeDSN string) (string, error) {
	if strings.TrimSpace(runtimeDSN) != "" {
		return runtimeDSN, nil
	}
	switch profile {
	case "", "local-lite":
		return "", fmt.Errorf("postgres runtime profile requires runtime dsn")
	case "runtime-promotion":
		if env := os.Getenv("PERSONAL_CFO_RUNTIME_DSN"); strings.TrimSpace(env) != "" {
			return env, nil
		}
		return "", fmt.Errorf("runtime-promotion profile requires PERSONAL_CFO_RUNTIME_DSN or runtime dsn")
	default:
		return "", fmt.Errorf("unsupported runtime profile %q", profile)
	}
}

func defaultRuntimePromotionBlobRoot() string {
	return filepath.Join(".", "var", "runtime-blobs")
}
