package runtime

import (
	"slices"
	"sync"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
)

type InMemoryArtifactMetadataStore struct {
	mu        sync.RWMutex
	artifacts map[string]reporting.WorkflowArtifact
}

func NewInMemoryArtifactMetadataStore() *InMemoryArtifactMetadataStore {
	return &InMemoryArtifactMetadataStore{artifacts: make(map[string]reporting.WorkflowArtifact)}
}

func (s *InMemoryArtifactMetadataStore) SaveArtifact(_ string, _ string, artifact reporting.WorkflowArtifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts[artifact.ID] = artifact
	return nil
}

func (s *InMemoryArtifactMetadataStore) ListArtifactsByTask(taskID string) ([]reporting.WorkflowArtifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]reporting.WorkflowArtifact, 0)
	for _, artifact := range s.artifacts {
		if artifact.TaskID == taskID {
			result = append(result, artifact)
		}
	}
	slices.SortFunc(result, func(a, b reporting.WorkflowArtifact) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return 0
	})
	return result, nil
}
