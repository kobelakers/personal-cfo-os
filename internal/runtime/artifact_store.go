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

func (s *InMemoryArtifactMetadataStore) ListArtifactsByWorkflow(workflowID string) ([]reporting.WorkflowArtifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]reporting.WorkflowArtifact, 0)
	for _, artifact := range s.artifacts {
		if artifact.WorkflowID == workflowID {
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

func (s *InMemoryArtifactMetadataStore) LoadArtifact(artifactID string) (reporting.WorkflowArtifact, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	artifact, ok := s.artifacts[artifactID]
	if !ok {
		return reporting.WorkflowArtifact{}, false, nil
	}
	return artifact, true, nil
}

type InMemoryWorkflowRunStore struct {
	mu    sync.RWMutex
	items map[string]WorkflowRunRecord
}

func NewInMemoryWorkflowRunStore() *InMemoryWorkflowRunStore {
	return &InMemoryWorkflowRunStore{items: make(map[string]WorkflowRunRecord)}
}

func (s *InMemoryWorkflowRunStore) Save(record WorkflowRunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[record.WorkflowID] = record
	return nil
}

func (s *InMemoryWorkflowRunStore) Load(workflowID string) (WorkflowRunRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.items[workflowID]
	return record, ok, nil
}

func (s *InMemoryWorkflowRunStore) List() ([]WorkflowRunRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]WorkflowRunRecord, 0, len(s.items))
	for _, item := range s.items {
		result = append(result, item)
	}
	slices.SortFunc(result, func(a, b WorkflowRunRecord) int {
		if a.UpdatedAt.Before(b.UpdatedAt) {
			return -1
		}
		if a.UpdatedAt.After(b.UpdatedAt) {
			return 1
		}
		return 0
	})
	return result, nil
}

type InMemoryReplayProjectionStore struct {
	mu                  sync.RWMutex
	workflow            map[string]WorkflowReplayProjection
	taskGraphs          map[string]TaskGraphReplayProjection
	builds              map[string]ReplayProjectionBuildRecord
	nodes               map[string][]ProvenanceNodeRecord
	edges               map[string][]ProvenanceEdgeRecord
	executionAttr       map[string][]ExecutionAttributionRecord
	failureAttr         map[string][]FailureAttributionRecord
}

func NewInMemoryReplayProjectionStore() *InMemoryReplayProjectionStore {
	return &InMemoryReplayProjectionStore{
		workflow:      make(map[string]WorkflowReplayProjection),
		taskGraphs:    make(map[string]TaskGraphReplayProjection),
		builds:        make(map[string]ReplayProjectionBuildRecord),
		nodes:         make(map[string][]ProvenanceNodeRecord),
		edges:         make(map[string][]ProvenanceEdgeRecord),
		executionAttr: make(map[string][]ExecutionAttributionRecord),
		failureAttr:   make(map[string][]FailureAttributionRecord),
	}
}

func replayScopeKey(scope ReplayProjectionScope) string {
	return string(scope.ScopeKind) + ":" + scope.ScopeID
}

func (s *InMemoryReplayProjectionStore) SaveWorkflowProjection(record WorkflowReplayProjection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflow[record.WorkflowID] = record
	return nil
}

func (s *InMemoryReplayProjectionStore) SaveTaskGraphProjection(record TaskGraphReplayProjection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskGraphs[record.GraphID] = record
	return nil
}

func (s *InMemoryReplayProjectionStore) SaveBuild(record ReplayProjectionBuildRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.builds[replayScopeKey(ReplayProjectionScope{ScopeKind: record.ScopeKind, ScopeID: record.ScopeID})] = record
	return nil
}

func (s *InMemoryReplayProjectionStore) ReplaceProvenance(scope ReplayProjectionScope, nodes []ProvenanceNodeRecord, edges []ProvenanceEdgeRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := replayScopeKey(scope)
	s.nodes[key] = append([]ProvenanceNodeRecord{}, nodes...)
	s.edges[key] = append([]ProvenanceEdgeRecord{}, edges...)
	return nil
}

func (s *InMemoryReplayProjectionStore) ReplaceExecutionAttributions(scope ReplayProjectionScope, records []ExecutionAttributionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executionAttr[replayScopeKey(scope)] = append([]ExecutionAttributionRecord{}, records...)
	return nil
}

func (s *InMemoryReplayProjectionStore) ReplaceFailureAttributions(scope ReplayProjectionScope, records []FailureAttributionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failureAttr[replayScopeKey(scope)] = append([]FailureAttributionRecord{}, records...)
	return nil
}

func (s *InMemoryReplayProjectionStore) LoadWorkflowProjection(workflowID string) (WorkflowReplayProjection, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.workflow[workflowID]
	return record, ok, nil
}

func (s *InMemoryReplayProjectionStore) LoadTaskGraphProjection(graphID string) (TaskGraphReplayProjection, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.taskGraphs[graphID]
	return record, ok, nil
}

func (s *InMemoryReplayProjectionStore) LoadBuild(scope ReplayProjectionScope) (ReplayProjectionBuildRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.builds[replayScopeKey(scope)]
	return record, ok, nil
}

func (s *InMemoryReplayProjectionStore) ListProvenance(scope ReplayProjectionScope) ([]ProvenanceNodeRecord, []ProvenanceEdgeRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := replayScopeKey(scope)
	return append([]ProvenanceNodeRecord{}, s.nodes[key]...), append([]ProvenanceEdgeRecord{}, s.edges[key]...), nil
}

func (s *InMemoryReplayProjectionStore) ListExecutionAttributions(scope ReplayProjectionScope) ([]ExecutionAttributionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]ExecutionAttributionRecord{}, s.executionAttr[replayScopeKey(scope)]...), nil
}

func (s *InMemoryReplayProjectionStore) ListFailureAttributions(scope ReplayProjectionScope) ([]FailureAttributionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]FailureAttributionRecord{}, s.failureAttr[replayScopeKey(scope)]...), nil
}
