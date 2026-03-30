package runtime

import (
	"fmt"
	"sync"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/skills"
)

type SkillExecutionRecord = skills.SkillExecutionRecord

type SkillExecutionStore interface {
	Save(record SkillExecutionRecord) error
	Load(executionID string) (SkillExecutionRecord, bool, error)
	ListByWorkflow(workflowID string) ([]SkillExecutionRecord, error)
}

type InMemorySkillExecutionStore struct {
	mu      sync.RWMutex
	records map[string]SkillExecutionRecord
}

func NewInMemorySkillExecutionStore() *InMemorySkillExecutionStore {
	return &InMemorySkillExecutionStore{records: make(map[string]SkillExecutionRecord)}
}

func (s *InMemorySkillExecutionStore) Save(record SkillExecutionRecord) error {
	if record.ExecutionID == "" {
		return fmt.Errorf("skill execution requires execution_id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.ExecutionID] = record
	return nil
}

func (s *InMemorySkillExecutionStore) Load(executionID string) (SkillExecutionRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[executionID]
	return record, ok, nil
}

func (s *InMemorySkillExecutionStore) ListByWorkflow(workflowID string) ([]SkillExecutionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]SkillExecutionRecord, 0)
	for _, record := range s.records {
		if record.WorkflowID == workflowID {
			result = append(result, record)
		}
	}
	return result, nil
}

type sqliteSkillExecutionStore struct {
	db *SQLiteRuntimeDB
}

func (s *sqliteSkillExecutionStore) Save(record SkillExecutionRecord) error {
	if record.ExecutionID == "" {
		return fmt.Errorf("skill execution requires execution_id")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	_, err = s.db.db.Exec(`
		INSERT INTO skill_executions (
			execution_id, workflow_id, task_id, status, created_at, updated_at, record_json
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(execution_id) DO UPDATE SET
			workflow_id = excluded.workflow_id,
			task_id = excluded.task_id,
			status = excluded.status,
			updated_at = excluded.updated_at,
			record_json = excluded.record_json
	`, record.ExecutionID, record.WorkflowID, record.TaskID, string(record.Status), record.CreatedAt.UTC().Format(time.RFC3339Nano), record.UpdatedAt.UTC().Format(time.RFC3339Nano), raw)
	return err
}

func (s *sqliteSkillExecutionStore) Load(executionID string) (SkillExecutionRecord, bool, error) {
	row := s.db.db.QueryRow(`SELECT record_json FROM skill_executions WHERE execution_id = ?`, executionID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return SkillExecutionRecord{}, false, nil
		}
		return SkillExecutionRecord{}, false, err
	}
	var record SkillExecutionRecord
	if err := unmarshalJSON(raw, &record); err != nil {
		return SkillExecutionRecord{}, false, err
	}
	return record, true, nil
}

func (s *sqliteSkillExecutionStore) ListByWorkflow(workflowID string) ([]SkillExecutionRecord, error) {
	rows, err := s.db.db.Query(`
		SELECT record_json
		FROM skill_executions
		WHERE workflow_id = ?
		ORDER BY updated_at ASC, execution_id ASC
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SkillExecutionRecord, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record SkillExecutionRecord
		if err := unmarshalJSON(raw, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

type postgresSkillExecutionStore struct {
	db *PostgresRuntimeDB
}

func (s *postgresSkillExecutionStore) Save(record SkillExecutionRecord) error {
	if record.ExecutionID == "" {
		return fmt.Errorf("skill execution requires execution_id")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	raw, err := marshalJSON(record)
	if err != nil {
		return err
	}
	_, err = s.db.exec(`
		INSERT INTO skill_executions (
			execution_id, workflow_id, task_id, status, created_at, updated_at, record_json
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(execution_id) DO UPDATE SET
			workflow_id = excluded.workflow_id,
			task_id = excluded.task_id,
			status = excluded.status,
			updated_at = excluded.updated_at,
			record_json = excluded.record_json
	`, record.ExecutionID, record.WorkflowID, record.TaskID, string(record.Status), record.CreatedAt.UTC().Format(time.RFC3339Nano), record.UpdatedAt.UTC().Format(time.RFC3339Nano), raw)
	return err
}

func (s *postgresSkillExecutionStore) Load(executionID string) (SkillExecutionRecord, bool, error) {
	row := s.db.queryRow(`SELECT record_json FROM skill_executions WHERE execution_id = ?`, executionID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errorsIsNoRows(err) {
			return SkillExecutionRecord{}, false, nil
		}
		return SkillExecutionRecord{}, false, err
	}
	var record SkillExecutionRecord
	if err := unmarshalJSON(raw, &record); err != nil {
		return SkillExecutionRecord{}, false, err
	}
	return record, true, nil
}

func (s *postgresSkillExecutionStore) ListByWorkflow(workflowID string) ([]SkillExecutionRecord, error) {
	rows, err := s.db.query(`
		SELECT record_json
		FROM skill_executions
		WHERE workflow_id = ?
		ORDER BY updated_at ASC, execution_id ASC
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SkillExecutionRecord, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record SkillExecutionRecord
		if err := unmarshalJSON(raw, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}
