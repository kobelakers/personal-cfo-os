package runtime

import (
	"slices"
	"sync"
)

type InMemoryOperatorActionStore struct {
	mu        sync.RWMutex
	byID      map[string]OperatorActionRecord
	byRequest map[string]string
}

func NewInMemoryOperatorActionStore() *InMemoryOperatorActionStore {
	return &InMemoryOperatorActionStore{
		byID:      make(map[string]OperatorActionRecord),
		byRequest: make(map[string]string),
	}
}

func (s *InMemoryOperatorActionStore) Save(record OperatorActionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record.ActionID == "" {
		return &ConflictError{Resource: "operator_action", ID: "", Reason: "action id is required"}
	}
	if record.RequestID == "" {
		return &ConflictError{Resource: "operator_action", ID: record.ActionID, Reason: "request id is required"}
	}
	if existingID, exists := s.byRequest[record.RequestID]; exists {
		return &ConflictError{Resource: "operator_action_request", ID: existingID, Reason: ErrDuplicateRequest.Error()}
	}
	s.byID[record.ActionID] = record
	s.byRequest[record.RequestID] = record.ActionID
	return nil
}

func (s *InMemoryOperatorActionStore) LoadByRequestID(requestID string) (OperatorActionRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	actionID, ok := s.byRequest[requestID]
	if !ok {
		return OperatorActionRecord{}, false, nil
	}
	record, ok := s.byID[actionID]
	return record, ok, nil
}

func (s *InMemoryOperatorActionStore) ListByTask(graphID string, taskID string) ([]OperatorActionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]OperatorActionRecord, 0)
	for _, record := range s.byID {
		if record.GraphID == graphID && record.TaskID == taskID {
			result = append(result, record)
		}
	}
	slices.SortFunc(result, func(a, b OperatorActionRecord) int {
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
