package runtime

import (
	"fmt"
	"sync"
)

type InMemoryCheckpointStore struct {
	mu          sync.RWMutex
	checkpoints map[string]map[string]CheckpointRecord
}

func NewInMemoryCheckpointStore() *InMemoryCheckpointStore {
	return &InMemoryCheckpointStore{
		checkpoints: make(map[string]map[string]CheckpointRecord),
	}
}

func (s *InMemoryCheckpointStore) Save(checkpoint CheckpointRecord) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.checkpoints[checkpoint.WorkflowID]; !ok {
		s.checkpoints[checkpoint.WorkflowID] = make(map[string]CheckpointRecord)
	}
	s.checkpoints[checkpoint.WorkflowID][checkpoint.ID] = checkpoint
	return nil
}

func (s *InMemoryCheckpointStore) Load(workflowID string, checkpointID string) (CheckpointRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byWorkflow, ok := s.checkpoints[workflowID]
	if !ok {
		return CheckpointRecord{}, fmt.Errorf("workflow %q not found", workflowID)
	}
	checkpoint, ok := byWorkflow[checkpointID]
	if !ok {
		return CheckpointRecord{}, fmt.Errorf("checkpoint %q not found", checkpointID)
	}
	return checkpoint, nil
}
