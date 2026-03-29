package runtime

import (
	"fmt"
	"sync"
)

type InMemoryCheckpointStore struct {
	mu          sync.RWMutex
	checkpoints map[string]map[string]CheckpointRecord
	tokens      map[string]ResumeToken
	payloads    map[string]CheckpointPayloadEnvelope
}

func NewInMemoryCheckpointStore() *InMemoryCheckpointStore {
	return &InMemoryCheckpointStore{
		checkpoints: make(map[string]map[string]CheckpointRecord),
		tokens:      make(map[string]ResumeToken),
		payloads:    make(map[string]CheckpointPayloadEnvelope),
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
		return CheckpointRecord{}, &NotFoundError{Resource: "workflow", ID: workflowID}
	}
	checkpoint, ok := byWorkflow[checkpointID]
	if !ok {
		return CheckpointRecord{}, &NotFoundError{Resource: "checkpoint", ID: checkpointID}
	}
	return checkpoint, nil
}

func (s *InMemoryCheckpointStore) SaveResumeToken(token ResumeToken) error {
	if err := token.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token.Token] = token
	return nil
}

func (s *InMemoryCheckpointStore) LoadResumeToken(token string) (ResumeToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.tokens[token]
	if !ok {
		return ResumeToken{}, &NotFoundError{Resource: "resume_token", ID: token}
	}
	return record, nil
}

func (s *InMemoryCheckpointStore) SavePayload(checkpointID string, payload CheckpointPayloadEnvelope) error {
	if checkpointID == "" {
		return fmt.Errorf("checkpoint id is required")
	}
	if err := payload.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payloads[checkpointID] = payload
	return nil
}

func (s *InMemoryCheckpointStore) LoadPayload(checkpointID string) (CheckpointPayloadEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	payload, ok := s.payloads[checkpointID]
	if !ok {
		return CheckpointPayloadEnvelope{}, &NotFoundError{Resource: "checkpoint_payload", ID: checkpointID}
	}
	return payload, nil
}
