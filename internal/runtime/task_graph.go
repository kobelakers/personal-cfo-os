package runtime

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type TaskQueueStatus string

const (
	TaskQueueStatusReady                   TaskQueueStatus = "ready"
	TaskQueueStatusDependencyBlocked       TaskQueueStatus = "dependency_blocked"
	TaskQueueStatusDeferred                TaskQueueStatus = "deferred"
	TaskQueueStatusWaitingApproval         TaskQueueStatus = "waiting_approval"
	TaskQueueStatusQueuedPendingCapability TaskQueueStatus = "queued_pending_capability"
)

type TaskCapabilityResolver interface {
	Resolve(spec taskspec.TaskSpec) (requiredCapability string, available bool, missingReason string)
}

type StaticTaskCapabilityResolver struct {
	Capabilities map[taskspec.UserIntentType]string
}

func (r StaticTaskCapabilityResolver) Resolve(spec taskspec.TaskSpec) (string, bool, string) {
	capability, ok := r.Capabilities[spec.UserIntentType]
	if ok {
		return capability, true, ""
	}
	required := string(spec.UserIntentType) + "_workflow"
	return required, false, "workflow entrypoint for this intent is not registered in the local runtime"
}

type FollowUpTaskRecord struct {
	Task                    taskspec.TaskSpec              `json:"task"`
	Metadata                taskspec.GeneratedTaskMetadata `json:"metadata"`
	Status                  TaskQueueStatus                `json:"status"`
	RequiredCapability      string                         `json:"required_capability,omitempty"`
	MissingCapabilityReason string                         `json:"missing_capability_reason,omitempty"`
	Dependencies            []taskspec.TaskDependency      `json:"dependencies,omitempty"`
	RegisteredAt            time.Time                      `json:"registered_at"`
}

type SpawnedTaskRecord struct {
	TaskID                  string          `json:"task_id"`
	Status                  TaskQueueStatus `json:"status"`
	RequiredCapability      string          `json:"required_capability,omitempty"`
	MissingCapabilityReason string          `json:"missing_capability_reason,omitempty"`
	RegisteredAt            time.Time       `json:"registered_at"`
}

type DeferredTaskRecord struct {
	TaskID     string          `json:"task_id"`
	Status     TaskQueueStatus `json:"status"`
	NotBefore  *time.Time      `json:"not_before,omitempty"`
	NotAfter   *time.Time      `json:"not_after,omitempty"`
	Reason     string          `json:"reason"`
	DeferredAt time.Time       `json:"deferred_at"`
}

type TaskGraphSnapshot struct {
	Graph           taskspec.TaskGraph   `json:"graph"`
	RegisteredTasks []FollowUpTaskRecord `json:"registered_tasks,omitempty"`
	Spawned         []SpawnedTaskRecord  `json:"spawned,omitempty"`
	Deferred        []DeferredTaskRecord `json:"deferred,omitempty"`
	RegisteredAt    time.Time            `json:"registered_at"`
}

type FollowUpRegistrationResult struct {
	Graph           taskspec.TaskGraph   `json:"graph"`
	RegisteredTasks []FollowUpTaskRecord `json:"registered_tasks,omitempty"`
	Spawned         []SpawnedTaskRecord  `json:"spawned,omitempty"`
	Deferred        []DeferredTaskRecord `json:"deferred,omitempty"`
}

type InMemoryTaskGraphStore struct {
	mu        sync.RWMutex
	snapshots map[string]TaskGraphSnapshot
}

func NewInMemoryTaskGraphStore() *InMemoryTaskGraphStore {
	return &InMemoryTaskGraphStore{snapshots: make(map[string]TaskGraphSnapshot)}
}

func (s *InMemoryTaskGraphStore) Save(snapshot TaskGraphSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshots == nil {
		s.snapshots = make(map[string]TaskGraphSnapshot)
	}
	s.snapshots[snapshot.Graph.GraphID] = snapshot
	return nil
}

func (s *InMemoryTaskGraphStore) Load(graphID string) (TaskGraphSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, ok := s.snapshots[graphID]
	return snapshot, ok
}

func (s *InMemoryTaskGraphStore) List() []TaskGraphSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]TaskGraphSnapshot, 0, len(s.snapshots))
	for _, snapshot := range s.snapshots {
		result = append(result, snapshot)
	}
	return result
}

func RegisterFollowUpTaskGraph(
	graph taskspec.TaskGraph,
	resolver TaskCapabilityResolver,
	now time.Time,
) (FollowUpRegistrationResult, error) {
	if err := graph.Validate(); err != nil {
		return FollowUpRegistrationResult{}, err
	}
	if resolver == nil {
		resolver = StaticTaskCapabilityResolver{
			Capabilities: map[taskspec.UserIntentType]string{
				taskspec.UserIntentMonthlyReview: "monthly_review_workflow",
				taskspec.UserIntentDebtVsInvest:  "debt_vs_invest_workflow",
			},
		}
	}

	downstreamDeps := make(map[string][]taskspec.TaskDependency)
	for _, dep := range graph.Dependencies {
		downstreamDeps[dep.DownstreamTaskID] = append(downstreamDeps[dep.DownstreamTaskID], dep)
	}

	registered := make([]FollowUpTaskRecord, 0, len(graph.GeneratedTasks))
	spawned := make([]SpawnedTaskRecord, 0, len(graph.GeneratedTasks))
	deferred := make([]DeferredTaskRecord, 0)
	for _, generated := range graph.GeneratedTasks {
		requiredCapability, available, missingReason := resolver.Resolve(generated.Task)
		status := TaskQueueStatusReady
		if len(downstreamDeps[generated.Task.ID]) > 0 {
			status = TaskQueueStatusDependencyBlocked
		}
		if generated.Metadata.RequiresApproval {
			status = TaskQueueStatusWaitingApproval
		}
		if generated.Metadata.DueWindow.NotBefore != nil && generated.Metadata.DueWindow.NotBefore.After(now) {
			status = TaskQueueStatusDeferred
			deferred = append(deferred, DeferredTaskRecord{
				TaskID:     generated.Task.ID,
				Status:     status,
				NotBefore:  generated.Metadata.DueWindow.NotBefore,
				NotAfter:   generated.Metadata.DueWindow.NotAfter,
				Reason:     "task due window has not opened yet",
				DeferredAt: now,
			})
		}
		if !available {
			status = TaskQueueStatusQueuedPendingCapability
		}

		record := FollowUpTaskRecord{
			Task:                    generated.Task,
			Metadata:                generated.Metadata,
			Status:                  status,
			RequiredCapability:      requiredCapability,
			MissingCapabilityReason: missingReason,
			Dependencies:            downstreamDeps[generated.Task.ID],
			RegisteredAt:            now,
		}
		if status == TaskQueueStatusQueuedPendingCapability && strings.TrimSpace(record.MissingCapabilityReason) == "" {
			record.MissingCapabilityReason = "required capability is not available in the current runtime"
		}
		registered = append(registered, record)
		spawned = append(spawned, SpawnedTaskRecord{
			TaskID:                  generated.Task.ID,
			Status:                  status,
			RequiredCapability:      record.RequiredCapability,
			MissingCapabilityReason: record.MissingCapabilityReason,
			RegisteredAt:            now,
		})
	}
	return FollowUpRegistrationResult{
		Graph:           graph,
		RegisteredTasks: registered,
		Spawned:         spawned,
		Deferred:        deferred,
	}, nil
}

func (r FollowUpTaskRecord) Validate() error {
	if err := r.Task.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.RequiredCapability) == "" {
		return fmt.Errorf("follow-up task %q requires required_capability", r.Task.ID)
	}
	if r.RegisteredAt.IsZero() {
		return fmt.Errorf("follow-up task %q requires registered_at", r.Task.ID)
	}
	if r.Status == TaskQueueStatusQueuedPendingCapability && strings.TrimSpace(r.MissingCapabilityReason) == "" {
		return fmt.Errorf("follow-up task %q requires missing_capability_reason when queued_pending_capability", r.Task.ID)
	}
	return nil
}
