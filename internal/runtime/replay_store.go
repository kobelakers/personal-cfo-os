package runtime

import (
	"slices"
	"sync"
)

type InMemoryReplayStore struct {
	mu     sync.RWMutex
	events []ReplayEventRecord
}

func NewInMemoryReplayStore() *InMemoryReplayStore {
	return &InMemoryReplayStore{}
}

func (s *InMemoryReplayStore) Append(event ReplayEventRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *InMemoryReplayStore) ListByGraph(graphID string) ([]ReplayEventRecord, error) {
	return s.filter(func(item ReplayEventRecord) bool { return item.GraphID == graphID })
}

func (s *InMemoryReplayStore) ListByTask(taskID string) ([]ReplayEventRecord, error) {
	return s.filter(func(item ReplayEventRecord) bool { return item.TaskID == taskID })
}

func (s *InMemoryReplayStore) ListByWorkflow(workflowID string) ([]ReplayEventRecord, error) {
	return s.filter(func(item ReplayEventRecord) bool {
		return item.WorkflowID == workflowID || item.ParentWorkflowID == workflowID
	})
}

func (s *InMemoryReplayStore) filter(keep func(ReplayEventRecord) bool) ([]ReplayEventRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ReplayEventRecord, 0)
	for _, event := range s.events {
		if keep(event) {
			result = append(result, event)
		}
	}
	slices.SortFunc(result, func(a, b ReplayEventRecord) int {
		if a.OccurredAt.Before(b.OccurredAt) {
			return -1
		}
		if a.OccurredAt.After(b.OccurredAt) {
			return 1
		}
		return 0
	})
	return result, nil
}
