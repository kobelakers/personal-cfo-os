package runtime

import (
	"slices"
	"sync"
)

type InMemoryApprovalStateStore struct {
	mu      sync.RWMutex
	records map[string]ApprovalStateRecord
}

func NewInMemoryApprovalStateStore() *InMemoryApprovalStateStore {
	return &InMemoryApprovalStateStore{records: make(map[string]ApprovalStateRecord)}
}

func (s *InMemoryApprovalStateStore) Save(record ApprovalStateRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record.ApprovalID == "" {
		return &ConflictError{Resource: "approval", ID: "", Reason: "approval id is required"}
	}
	if _, exists := s.records[record.ApprovalID]; exists {
		return &ConflictError{Resource: "approval", ID: record.ApprovalID, Reason: "approval already exists"}
	}
	if record.Version == 0 {
		record.Version = 1
	}
	s.records[record.ApprovalID] = record
	return nil
}

func (s *InMemoryApprovalStateStore) Update(record ApprovalStateRecord, expectedVersion int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.records[record.ApprovalID]
	if !ok {
		return &NotFoundError{Resource: "approval", ID: record.ApprovalID}
	}
	if expectedVersion > 0 && current.Version != expectedVersion {
		return &ConflictError{Resource: "approval", ID: record.ApprovalID, Reason: "version mismatch"}
	}
	record.Version = current.Version + 1
	s.records[record.ApprovalID] = record
	return nil
}

func (s *InMemoryApprovalStateStore) Load(approvalID string) (ApprovalStateRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[approvalID]
	return record, ok, nil
}

func (s *InMemoryApprovalStateStore) ListPending() ([]ApprovalStateRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ApprovalStateRecord, 0)
	for _, record := range s.records {
		if record.Status == ApprovalStatusPending {
			result = append(result, record)
		}
	}
	slices.SortFunc(result, func(a, b ApprovalStateRecord) int {
		if a.RequestedAt.Before(b.RequestedAt) {
			return -1
		}
		if a.RequestedAt.After(b.RequestedAt) {
			return 1
		}
		return 0
	})
	return result, nil
}

func (s *InMemoryApprovalStateStore) LoadByTask(graphID string, taskID string) (ApprovalStateRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.records {
		if record.GraphID == graphID && record.TaskID == taskID {
			return record, true, nil
		}
	}
	return ApprovalStateRecord{}, false, nil
}
