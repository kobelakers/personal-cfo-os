package runtime

import (
	"fmt"
	"slices"
	"sync"
	"time"
)

type InMemoryWorkQueueStore struct {
	mu    sync.RWMutex
	items map[string]WorkItem
}

func NewInMemoryWorkQueueStore() *InMemoryWorkQueueStore {
	return &InMemoryWorkQueueStore{items: make(map[string]WorkItem)}
}

func (s *InMemoryWorkQueueStore) Enqueue(item WorkItem) (WorkEnqueueResult, error) {
	if item.ID == "" {
		return WorkEnqueueResult{}, fmt.Errorf("work item id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.items {
		if item.DedupeKey != "" && existing.DedupeKey == item.DedupeKey && existing.Status != WorkItemStatusCompleted && existing.Status != WorkItemStatusFailed && existing.Status != WorkItemStatusAbandoned {
			return WorkEnqueueResult{
				Kind:               item.Kind,
				Disposition:        WorkEnqueueDispositionDuplicateSuppressed,
				DedupeKey:          item.DedupeKey,
				ExistingWorkItemID: existing.ID,
			}, nil
		}
	}
	if item.Status == "" {
		item.Status = WorkItemStatusQueued
	}
	item.LastUpdatedAt = item.AvailableAt
	if current, ok := s.items[item.ID]; ok {
		item.FencingToken = current.FencingToken
	}
	s.items[item.ID] = item
	return WorkEnqueueResult{
		WorkItemID:  item.ID,
		Kind:        item.Kind,
		Disposition: WorkEnqueueDispositionEnqueued,
		DedupeKey:   item.DedupeKey,
	}, nil
}

func (s *InMemoryWorkQueueStore) ClaimReady(workerID WorkerID, limit int, now time.Time, leaseTTL time.Duration) ([]WorkClaim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 1
	}
	ready := make([]WorkItem, 0)
	for _, item := range s.items {
		if item.Status != WorkItemStatusQueued {
			continue
		}
		if item.AvailableAt.After(now) {
			continue
		}
		ready = append(ready, item)
	}
	slices.SortFunc(ready, func(a, b WorkItem) int {
		if a.AvailableAt.Before(b.AvailableAt) {
			return -1
		}
		if a.AvailableAt.After(b.AvailableAt) {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	if len(ready) > limit {
		ready = ready[:limit]
	}
	claims := make([]WorkClaim, 0, len(ready))
	for _, item := range ready {
		item.Status = WorkItemStatusClaimed
		item.ClaimedByWorkerID = workerID
		item.FencingToken++
		item.LeaseID = makeID("lease", item.ID, item.FencingToken, workerID, now)
		item.ClaimToken = makeID("claim", item.ID, item.FencingToken, now)
		item.AttemptCount++
		claimedAt := now
		item.ClaimedAt = &claimedAt
		expires := now.Add(leaseTTL)
		item.LeaseExpiresAt = &expires
		item.LastUpdatedAt = now
		s.items[item.ID] = item
		claims = append(claims, WorkClaim{
			WorkItem:       item,
			WorkerID:       workerID,
			LeaseID:        item.LeaseID,
			ClaimToken:     item.ClaimToken,
			FencingToken:   item.FencingToken,
			ClaimedAt:      claimedAt,
			LeaseExpiresAt: expires,
		})
	}
	return claims, nil
}

func (s *InMemoryWorkQueueStore) Heartbeat(heartbeat LeaseHeartbeat) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[heartbeat.WorkItemID]
	if !ok {
		return &NotFoundError{Resource: "work_item", ID: heartbeat.WorkItemID}
	}
	if err := validateFenceAgainstItem(item, FenceValidation{
		WorkItemID:   heartbeat.WorkItemID,
		LeaseID:      heartbeat.LeaseID,
		FencingToken: heartbeat.FencingToken,
		WorkerID:     heartbeat.WorkerID,
	}); err != nil {
		return err
	}
	recorded := heartbeat.RecordedAt
	item.LastUpdatedAt = recorded
	expires := heartbeat.LeaseExpiresAt
	item.LeaseExpiresAt = &expires
	s.items[item.ID] = item
	return nil
}

func (s *InMemoryWorkQueueStore) Complete(fence FenceValidation, now time.Time) error {
	return s.finish(fence, WorkItemStatusCompleted, "", now)
}

func (s *InMemoryWorkQueueStore) Fail(fence FenceValidation, summary string, now time.Time) error {
	return s.finish(fence, WorkItemStatusFailed, summary, now)
}

func (s *InMemoryWorkQueueStore) Requeue(fence FenceValidation, nextAvailableAt time.Time, reason string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[fence.WorkItemID]
	if !ok {
		return &NotFoundError{Resource: "work_item", ID: fence.WorkItemID}
	}
	if err := validateFenceAgainstItem(item, fence); err != nil {
		return err
	}
	item.Status = WorkItemStatusQueued
	item.Reason = reason
	item.AvailableAt = nextAvailableAt
	item.LastUpdatedAt = now
	item.LeaseID = ""
	item.ClaimToken = ""
	item.ClaimedByWorkerID = ""
	item.LeaseExpiresAt = nil
	item.ClaimedAt = nil
	s.items[item.ID] = item
	return nil
}

func (s *InMemoryWorkQueueStore) ReclaimExpired(now time.Time) ([]LeaseReclaimResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]LeaseReclaimResult, 0)
	for id, item := range s.items {
		if item.Status != WorkItemStatusClaimed || item.LeaseExpiresAt == nil || item.LeaseExpiresAt.After(now) {
			continue
		}
		result = append(result, LeaseReclaimResult{
			WorkItemID:   item.ID,
			LeaseID:      item.LeaseID,
			WorkerID:     item.ClaimedByWorkerID,
			FencingToken: item.FencingToken,
			Reason:       "lease_expired",
			ReclaimedAt:  now,
		})
		item.Status = WorkItemStatusQueued
		item.Reason = "reclaimed after lease expiry"
		item.LeaseID = ""
		item.ClaimToken = ""
		item.ClaimedByWorkerID = ""
		item.LeaseExpiresAt = nil
		item.ClaimedAt = nil
		item.LastUpdatedAt = now
		s.items[id] = item
	}
	return result, nil
}

func (s *InMemoryWorkQueueStore) Load(workItemID string) (WorkItem, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[workItemID]
	return item, ok, nil
}

func (s *InMemoryWorkQueueStore) ListByGraph(graphID string) ([]WorkItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]WorkItem, 0)
	for _, item := range s.items {
		if item.GraphID == graphID {
			result = append(result, item)
		}
	}
	slices.SortFunc(result, func(a, b WorkItem) int {
		if a.AvailableAt.Before(b.AvailableAt) {
			return -1
		}
		if a.AvailableAt.After(b.AvailableAt) {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return result, nil
}

func (s *InMemoryWorkQueueStore) ValidateFence(fence FenceValidation) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[fence.WorkItemID]
	if !ok {
		return &NotFoundError{Resource: "work_item", ID: fence.WorkItemID}
	}
	return validateFenceAgainstItem(item, fence)
}

func (s *InMemoryWorkQueueStore) finish(fence FenceValidation, status WorkItemStatus, summary string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[fence.WorkItemID]
	if !ok {
		return &NotFoundError{Resource: "work_item", ID: fence.WorkItemID}
	}
	if err := validateFenceAgainstItem(item, fence); err != nil {
		return err
	}
	item.Status = status
	item.Reason = summary
	item.LastUpdatedAt = now
	item.LeaseExpiresAt = nil
	item.ClaimedByWorkerID = ""
	item.ClaimedAt = nil
	item.ClaimToken = ""
	item.LeaseID = ""
	if status == WorkItemStatusCompleted {
		completed := now
		item.CompletedAt = &completed
	}
	if status == WorkItemStatusFailed {
		failed := now
		item.FailedAt = &failed
	}
	s.items[item.ID] = item
	return nil
}

func validateFenceAgainstItem(item WorkItem, fence FenceValidation) error {
	if item.ID != fence.WorkItemID {
		return &ConflictError{Resource: "work_item", ID: fence.WorkItemID, Reason: "work item mismatch"}
	}
	if item.Status != WorkItemStatusClaimed {
		return &ConflictError{Resource: "work_item", ID: fence.WorkItemID, Reason: "work item is not actively leased"}
	}
	if item.LeaseID != fence.LeaseID {
		return &ConflictError{Resource: "work_item", ID: fence.WorkItemID, Reason: "lease ownership lost"}
	}
	if item.FencingToken != fence.FencingToken {
		return &ConflictError{Resource: "work_item", ID: fence.WorkItemID, Reason: "fencing token mismatch"}
	}
	if item.ClaimedByWorkerID != fence.WorkerID {
		return &ConflictError{Resource: "work_item", ID: fence.WorkItemID, Reason: "worker ownership mismatch"}
	}
	return nil
}

type InMemoryWorkAttemptStore struct {
	mu       sync.RWMutex
	attempts map[string]ExecutionAttempt
}

func NewInMemoryWorkAttemptStore() *InMemoryWorkAttemptStore {
	return &InMemoryWorkAttemptStore{attempts: make(map[string]ExecutionAttempt)}
}

func (s *InMemoryWorkAttemptStore) SaveAttempt(attempt ExecutionAttempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[attempt.AttemptID] = attempt
	return nil
}

func (s *InMemoryWorkAttemptStore) UpdateAttempt(attempt ExecutionAttempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.attempts[attempt.AttemptID]; !ok {
		return &NotFoundError{Resource: "execution_attempt", ID: attempt.AttemptID}
	}
	s.attempts[attempt.AttemptID] = attempt
	return nil
}

func (s *InMemoryWorkAttemptStore) ListAttempts(workItemID string) ([]ExecutionAttempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ExecutionAttempt, 0)
	for _, attempt := range s.attempts {
		if attempt.WorkItemID == workItemID {
			result = append(result, attempt)
		}
	}
	slices.SortFunc(result, func(a, b ExecutionAttempt) int {
		if a.StartedAt.Before(b.StartedAt) {
			return -1
		}
		if a.StartedAt.After(b.StartedAt) {
			return 1
		}
		if a.AttemptID < b.AttemptID {
			return -1
		}
		if a.AttemptID > b.AttemptID {
			return 1
		}
		return 0
	})
	return result, nil
}

type InMemoryWorkerRegistryStore struct {
	mu      sync.RWMutex
	workers map[WorkerID]WorkerRegistration
}

func NewInMemoryWorkerRegistryStore() *InMemoryWorkerRegistryStore {
	return &InMemoryWorkerRegistryStore{workers: make(map[WorkerID]WorkerRegistration)}
}

func (s *InMemoryWorkerRegistryStore) Register(worker WorkerRegistration) error {
	if worker.WorkerID == "" {
		return fmt.Errorf("worker id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers[worker.WorkerID] = worker
	return nil
}

func (s *InMemoryWorkerRegistryStore) Heartbeat(workerID WorkerID, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	worker, ok := s.workers[workerID]
	if !ok {
		return &NotFoundError{Resource: "worker", ID: string(workerID)}
	}
	worker.LastHeartbeat = now
	s.workers[workerID] = worker
	return nil
}

func (s *InMemoryWorkerRegistryStore) Load(workerID WorkerID) (WorkerRegistration, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	worker, ok := s.workers[workerID]
	return worker, ok, nil
}

func (s *InMemoryWorkerRegistryStore) List() ([]WorkerRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]WorkerRegistration, 0, len(s.workers))
	for _, worker := range s.workers {
		result = append(result, worker)
	}
	slices.SortFunc(result, func(a, b WorkerRegistration) int {
		if a.WorkerID < b.WorkerID {
			return -1
		}
		if a.WorkerID > b.WorkerID {
			return 1
		}
		return 0
	})
	return result, nil
}

type inMemorySchedulerStore struct {
	mu      sync.RWMutex
	wakeups map[string]SchedulerWakeup
}

func NewInMemorySchedulerStore() SchedulerStore {
	return &inMemorySchedulerStore{wakeups: make(map[string]SchedulerWakeup)}
}

func (s *inMemorySchedulerStore) SaveWakeup(wakeup SchedulerWakeup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wakeups[wakeup.ID] = wakeup
	return nil
}

func (s *inMemorySchedulerStore) ListDueWakeups(now time.Time) ([]SchedulerWakeup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]SchedulerWakeup, 0)
	for _, wakeup := range s.wakeups {
		if !wakeup.AvailableAt.After(now) {
			result = append(result, wakeup)
		}
	}
	slices.SortFunc(result, func(a, b SchedulerWakeup) int {
		if a.AvailableAt.Before(b.AvailableAt) {
			return -1
		}
		if a.AvailableAt.After(b.AvailableAt) {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return result, nil
}

func (s *inMemorySchedulerStore) MarkWakeupDispatched(id string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.wakeups, id)
	return nil
}
