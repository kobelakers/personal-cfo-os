package runtime

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type TaskQueueStatus string

const (
	TaskQueueStatusReady                   TaskQueueStatus = "ready"
	TaskQueueStatusDependencyBlocked       TaskQueueStatus = "dependency_blocked"
	TaskQueueStatusDeferred                TaskQueueStatus = "deferred"
	TaskQueueStatusWaitingApproval         TaskQueueStatus = "waiting_approval"
	TaskQueueStatusQueuedPendingCapability TaskQueueStatus = "queued_pending_capability"
	TaskQueueStatusExecuting               TaskQueueStatus = "executing"
	TaskQueueStatusCompleted               TaskQueueStatus = "completed"
	TaskQueueStatusFailed                  TaskQueueStatus = "failed"
)

type TaskCapabilityResolver interface {
	Resolve(spec taskspec.TaskSpec) (requiredCapability string, available bool, missingReason string)
	ResolveWorkflow(spec taskspec.TaskSpec) (FollowUpWorkflowCapability, bool, string)
}

type StaticTaskCapabilityResolver struct {
	Capabilities map[taskspec.UserIntentType]string
	Workflows    map[taskspec.UserIntentType]FollowUpWorkflowCapability
}

func (r StaticTaskCapabilityResolver) Resolve(spec taskspec.TaskSpec) (string, bool, string) {
	capability, ok := r.Capabilities[spec.UserIntentType]
	if ok {
		return capability, true, ""
	}
	required := string(spec.UserIntentType) + "_workflow"
	return required, false, "workflow entrypoint for this intent is not registered in the local runtime"
}

func (r StaticTaskCapabilityResolver) ResolveWorkflow(spec taskspec.TaskSpec) (FollowUpWorkflowCapability, bool, string) {
	capability, available, missingReason := r.Resolve(spec)
	if !available {
		return nil, false, missingReason
	}
	workflow, ok := r.Workflows[spec.UserIntentType]
	if !ok || workflow == nil {
		return nil, false, fmt.Sprintf("follow-up workflow capability %q is not registered for runtime execution", capability)
	}
	return workflow, true, ""
}

type FollowUpTaskRecord struct {
	Task                    taskspec.TaskSpec              `json:"task"`
	Metadata                taskspec.GeneratedTaskMetadata `json:"metadata"`
	Status                  TaskQueueStatus                `json:"status"`
	RequiredCapability      string                         `json:"required_capability,omitempty"`
	MissingCapabilityReason string                         `json:"missing_capability_reason,omitempty"`
	Dependencies            []taskspec.TaskDependency      `json:"dependencies,omitempty"`
	BlockingReasons         []string                       `json:"blocking_reasons,omitempty"`
	SuppressedReasons       []string                       `json:"suppressed_reasons,omitempty"`
	RegisteredAt            time.Time                      `json:"registered_at"`
	RegistrationOrder       int                            `json:"registration_order"`
	LastUpdatedAt           time.Time                      `json:"last_updated_at"`
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
	Graph                        taskspec.TaskGraph    `json:"graph"`
	RegisteredTasks              []FollowUpTaskRecord  `json:"registered_tasks,omitempty"`
	Spawned                      []SpawnedTaskRecord   `json:"spawned,omitempty"`
	Deferred                     []DeferredTaskRecord  `json:"deferred,omitempty"`
	BaseStateSnapshot            state.StateSnapshot   `json:"base_state_snapshot"`
	LatestCommittedStateSnapshot state.StateSnapshot   `json:"latest_committed_state_snapshot"`
	ExecutedTasks                []TaskExecutionRecord `json:"executed_tasks,omitempty"`
	RegisteredAt                 time.Time             `json:"registered_at"`
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
	registered, err := initialFollowUpTaskRecords(graph, resolver, now)
	if err != nil {
		return FollowUpRegistrationResult{}, err
	}
	spawned, deferred := deriveSpawnedAndDeferred(registered, now)
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

func defaultTaskCapabilityResolver() TaskCapabilityResolver {
	return StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentMonthlyReview: "monthly_review_workflow",
			taskspec.UserIntentDebtVsInvest:  "debt_vs_invest_workflow",
		},
	}
}

func initialFollowUpTaskRecords(
	graph taskspec.TaskGraph,
	resolver TaskCapabilityResolver,
	now time.Time,
) ([]FollowUpTaskRecord, error) {
	if resolver == nil {
		resolver = defaultTaskCapabilityResolver()
	}
	depsByTask := downstreamDependencies(graph.Dependencies)
	registered := make([]FollowUpTaskRecord, 0, len(graph.GeneratedTasks))
	for index, generated := range graph.GeneratedTasks {
		requiredCapability, available, missingReason := resolver.Resolve(generated.Task)
		record := FollowUpTaskRecord{
			Task:                    generated.Task,
			Metadata:                generated.Metadata,
			RequiredCapability:      requiredCapability,
			MissingCapabilityReason: missingReason,
			Dependencies:            depsByTask[generated.Task.ID],
			RegisteredAt:            now,
			RegistrationOrder:       index,
			LastUpdatedAt:           now,
		}
		record.Status, record.BlockingReasons = evaluatePreExecutionStatus(record, available, now)
		if record.Status == TaskQueueStatusQueuedPendingCapability && strings.TrimSpace(record.MissingCapabilityReason) == "" {
			record.MissingCapabilityReason = "required capability is not available in the current runtime"
		}
		if err := record.Validate(); err != nil {
			return nil, err
		}
		registered = append(registered, record)
	}
	return registered, nil
}

func ReevaluateFollowUpTaskGraph(
	snapshot TaskGraphSnapshot,
	resolver TaskCapabilityResolver,
	now time.Time,
) (TaskGraphSnapshot, TaskActivationResult, error) {
	if resolver == nil {
		resolver = defaultTaskCapabilityResolver()
	}
	if err := snapshot.Graph.Validate(); err != nil {
		return TaskGraphSnapshot{}, TaskActivationResult{}, err
	}
	updated := snapshot
	byTask := make(map[string]TaskExecutionRecord, len(snapshot.ExecutedTasks))
	for _, item := range snapshot.ExecutedTasks {
		byTask[item.TaskID] = item
	}
	for i := range updated.RegisteredTasks {
		record := updated.RegisteredTasks[i]
		record.BlockingReasons = nil
		record.LastUpdatedAt = now
		if execution, ok := byTask[record.Task.ID]; ok {
			switch execution.Status {
			case TaskQueueStatusCompleted, TaskQueueStatusFailed, TaskQueueStatusExecuting:
				record.Status = execution.Status
			case TaskQueueStatusWaitingApproval:
				record.Status = TaskQueueStatusWaitingApproval
				record.BlockingReasons = append(record.BlockingReasons, fmt.Sprintf("waiting for approval %s", execution.ApprovalID))
			}
			updated.RegisteredTasks[i] = record
			continue
		}

		requiredCapability, available, missingReason := resolver.Resolve(record.Task)
		record.RequiredCapability = requiredCapability
		record.MissingCapabilityReason = missingReason
		record.Status, record.BlockingReasons = evaluatePreExecutionStatus(record, available, now)
		if record.Status == TaskQueueStatusDependencyBlocked {
			record.BlockingReasons = dependencyBlockingReasons(updated, record.Task.ID)
			if len(record.BlockingReasons) == 0 {
				switch {
				case !available:
					record.Status = TaskQueueStatusQueuedPendingCapability
					if strings.TrimSpace(record.MissingCapabilityReason) != "" {
						record.BlockingReasons = []string{record.MissingCapabilityReason}
					}
				default:
					record.Status = TaskQueueStatusReady
					record.BlockingReasons = nil
				}
			}
		}
		if record.Status == TaskQueueStatusQueuedPendingCapability && strings.TrimSpace(record.MissingCapabilityReason) == "" {
			record.MissingCapabilityReason = "required capability is not available in the current runtime"
		}
		updated.RegisteredTasks[i] = record
	}
	updated.Spawned, updated.Deferred = deriveSpawnedAndDeferred(updated.RegisteredTasks, now)
	activation := TaskActivationResult{
		GraphID:         updated.Graph.GraphID,
		RegisteredTasks: append([]FollowUpTaskRecord{}, updated.RegisteredTasks...),
		EvaluatedAt:     now,
	}
	for _, item := range updated.RegisteredTasks {
		if item.Status == TaskQueueStatusReady {
			activation.ReadyTaskIDs = append(activation.ReadyTaskIDs, item.Task.ID)
		}
	}
	return updated, activation, nil
}

func ReadyTasksInExecutionOrder(snapshot TaskGraphSnapshot) []FollowUpTaskRecord {
	order := topologicalTaskOrder(snapshot.Graph, snapshot.RegisteredTasks)
	result := make([]FollowUpTaskRecord, 0, len(order))
	byID := taskRecordMap(snapshot.RegisteredTasks)
	for _, taskID := range order {
		record, ok := byID[taskID]
		if !ok || record.Status != TaskQueueStatusReady {
			continue
		}
		result = append(result, record)
	}
	return result
}

func evaluatePreExecutionStatus(record FollowUpTaskRecord, capabilityAvailable bool, now time.Time) (TaskQueueStatus, []string) {
	reasons := make([]string, 0, 2)
	switch {
	case record.Metadata.RequiresApproval:
		reasons = append(reasons, "approval required before task execution")
		return TaskQueueStatusWaitingApproval, reasons
	case record.Metadata.DueWindow.NotBefore != nil && record.Metadata.DueWindow.NotBefore.After(now):
		reasons = append(reasons, fmt.Sprintf("due window not open until %s", record.Metadata.DueWindow.NotBefore.UTC().Format(time.RFC3339)))
		return TaskQueueStatusDeferred, reasons
	case len(record.Dependencies) > 0:
		return TaskQueueStatusDependencyBlocked, reasons
	case !capabilityAvailable:
		reasons = append(reasons, record.MissingCapabilityReason)
		return TaskQueueStatusQueuedPendingCapability, reasons
	default:
		return TaskQueueStatusReady, reasons
	}
}

func dependencyBlockingReasons(snapshot TaskGraphSnapshot, taskID string) []string {
	upstream := make([]string, 0)
	depsByTask := downstreamDependencies(snapshot.Graph.Dependencies)
	statusByID := make(map[string]TaskQueueStatus, len(snapshot.RegisteredTasks))
	for _, item := range snapshot.RegisteredTasks {
		statusByID[item.Task.ID] = item.Status
	}
	for _, dep := range depsByTask[taskID] {
		if statusByID[dep.UpstreamTaskID] == TaskQueueStatusCompleted {
			continue
		}
		upstream = append(upstream, fmt.Sprintf("blocked by upstream task %s", dep.UpstreamTaskID))
	}
	return upstream
}

func downstreamDependencies(deps []taskspec.TaskDependency) map[string][]taskspec.TaskDependency {
	result := make(map[string][]taskspec.TaskDependency)
	for _, dep := range deps {
		result[dep.DownstreamTaskID] = append(result[dep.DownstreamTaskID], dep)
	}
	return result
}

func taskRecordMap(records []FollowUpTaskRecord) map[string]FollowUpTaskRecord {
	result := make(map[string]FollowUpTaskRecord, len(records))
	for _, item := range records {
		result[item.Task.ID] = item
	}
	return result
}

func deriveSpawnedAndDeferred(records []FollowUpTaskRecord, now time.Time) ([]SpawnedTaskRecord, []DeferredTaskRecord) {
	spawned := make([]SpawnedTaskRecord, 0, len(records))
	deferred := make([]DeferredTaskRecord, 0)
	for _, item := range records {
		spawned = append(spawned, SpawnedTaskRecord{
			TaskID:                  item.Task.ID,
			Status:                  item.Status,
			RequiredCapability:      item.RequiredCapability,
			MissingCapabilityReason: item.MissingCapabilityReason,
			RegisteredAt:            item.RegisteredAt,
		})
		if item.Status != TaskQueueStatusDeferred {
			continue
		}
		reason := "task due window has not opened yet"
		if len(item.BlockingReasons) > 0 {
			reason = item.BlockingReasons[0]
		}
		deferred = append(deferred, DeferredTaskRecord{
			TaskID:     item.Task.ID,
			Status:     item.Status,
			NotBefore:  item.Metadata.DueWindow.NotBefore,
			NotAfter:   item.Metadata.DueWindow.NotAfter,
			Reason:     reason,
			DeferredAt: now,
		})
	}
	return spawned, deferred
}

func topologicalTaskOrder(graph taskspec.TaskGraph, records []FollowUpTaskRecord) []string {
	orderByRegistration := make(map[string]int, len(records))
	for _, item := range records {
		orderByRegistration[item.Task.ID] = item.RegistrationOrder
	}
	indegree := make(map[string]int, len(records))
	downstream := make(map[string][]string)
	for _, item := range records {
		indegree[item.Task.ID] = 0
	}
	for _, dep := range graph.Dependencies {
		indegree[dep.DownstreamTaskID]++
		downstream[dep.UpstreamTaskID] = append(downstream[dep.UpstreamTaskID], dep.DownstreamTaskID)
	}
	zero := make([]string, 0, len(records))
	for taskID, value := range indegree {
		if value == 0 {
			zero = append(zero, taskID)
		}
	}
	sort.Slice(zero, func(i, j int) bool {
		return orderByRegistration[zero[i]] < orderByRegistration[zero[j]]
	})
	result := make([]string, 0, len(records))
	for len(zero) > 0 {
		taskID := zero[0]
		zero = zero[1:]
		result = append(result, taskID)
		children := downstream[taskID]
		sort.Slice(children, func(i, j int) bool {
			return orderByRegistration[children[i]] < orderByRegistration[children[j]]
		})
		for _, child := range children {
			indegree[child]--
			if indegree[child] == 0 {
				zero = append(zero, child)
				sort.Slice(zero, func(i, j int) bool {
					return orderByRegistration[zero[i]] < orderByRegistration[zero[j]]
				})
			}
		}
	}
	return result
}
