package runtime

import (
	"slices"
	"sync"
)

type InMemoryTaskExecutionStore struct {
	mu      sync.RWMutex
	records map[string]TaskExecutionRecord
}

func NewInMemoryTaskExecutionStore() *InMemoryTaskExecutionStore {
	return &InMemoryTaskExecutionStore{records: make(map[string]TaskExecutionRecord)}
}

func (s *InMemoryTaskExecutionStore) Save(record TaskExecutionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record.ExecutionID == "" {
		return &ConflictError{Resource: "task_execution", ID: "", Reason: "execution id is required"}
	}
	if _, exists := s.records[record.ExecutionID]; exists {
		return &ConflictError{Resource: "task_execution", ID: record.ExecutionID, Reason: "execution already exists"}
	}
	if record.Version == 0 {
		record.Version = 1
	}
	s.records[record.ExecutionID] = record
	return nil
}

func (s *InMemoryTaskExecutionStore) Update(record TaskExecutionRecord, expectedVersion int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.records[record.ExecutionID]
	if !ok {
		return &NotFoundError{Resource: "task_execution", ID: record.ExecutionID}
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return &ConflictError{Resource: "task_execution", ID: record.ExecutionID, Reason: "version mismatch"}
	}
	record.Version = current.Version + 1
	s.records[record.ExecutionID] = record
	return nil
}

func (s *InMemoryTaskExecutionStore) Load(executionID string) (TaskExecutionRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[executionID]
	return record, ok, nil
}

func (s *InMemoryTaskExecutionStore) LoadLatestByTask(graphID string, taskID string) (TaskExecutionRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]TaskExecutionRecord, 0)
	for _, record := range s.records {
		if record.ParentGraphID == graphID && record.TaskID == taskID {
			items = append(items, record)
		}
	}
	if len(items) == 0 {
		return TaskExecutionRecord{}, false, nil
	}
	slices.SortFunc(items, func(a, b TaskExecutionRecord) int {
		if a.StartedAt.Before(b.StartedAt) {
			return 1
		}
		if a.StartedAt.After(b.StartedAt) {
			return -1
		}
		return 0
	})
	return items[0], true, nil
}

func (s *InMemoryTaskExecutionStore) ListByGraph(graphID string) ([]TaskExecutionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]TaskExecutionRecord, 0)
	for _, record := range s.records {
		if record.ParentGraphID == graphID {
			result = append(result, record)
		}
	}
	slices.SortFunc(result, func(a, b TaskExecutionRecord) int {
		if a.StartedAt.Before(b.StartedAt) {
			return -1
		}
		if a.StartedAt.After(b.StartedAt) {
			return 1
		}
		return 0
	})
	return result, nil
}
