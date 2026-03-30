package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type runtimePromotionTestEnv struct {
	DSN       string
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
}

type storeBackedAsyncHarness struct {
	service     *Service
	operator    *OperatorService
	clock       *ManualClock
	replayQuery *ReplayQueryService
	bundle      *StoreBundle
	graphID     string
	taskID      string
}

type heartbeatSpyWorkQueue struct {
	inner      WorkQueueStore
	heartbeats chan LeaseHeartbeat
}

func (q *heartbeatSpyWorkQueue) Enqueue(item WorkItem) (WorkEnqueueResult, error) {
	return q.inner.Enqueue(item)
}

func (q *heartbeatSpyWorkQueue) ClaimReady(workerID WorkerID, limit int, now time.Time, leaseTTL time.Duration) ([]WorkClaim, error) {
	return q.inner.ClaimReady(workerID, limit, now, leaseTTL)
}

func (q *heartbeatSpyWorkQueue) Heartbeat(heartbeat LeaseHeartbeat) error {
	if err := q.inner.Heartbeat(heartbeat); err != nil {
		return err
	}
	if q.heartbeats != nil {
		q.heartbeats <- heartbeat
	}
	return nil
}

func (q *heartbeatSpyWorkQueue) Complete(fence FenceValidation, now time.Time) error {
	return q.inner.Complete(fence, now)
}

func (q *heartbeatSpyWorkQueue) Fail(fence FenceValidation, summary string, now time.Time) error {
	return q.inner.Fail(fence, summary, now)
}

func (q *heartbeatSpyWorkQueue) Requeue(fence FenceValidation, nextAvailableAt time.Time, reason string, now time.Time) error {
	return q.inner.Requeue(fence, nextAvailableAt, reason, now)
}

func (q *heartbeatSpyWorkQueue) ReclaimExpired(now time.Time) ([]LeaseReclaimResult, error) {
	return q.inner.ReclaimExpired(now)
}

func (q *heartbeatSpyWorkQueue) Load(workItemID string) (WorkItem, bool, error) {
	return q.inner.Load(workItemID)
}

func (q *heartbeatSpyWorkQueue) ListByGraph(graphID string) ([]WorkItem, error) {
	return q.inner.ListByGraph(graphID)
}

func (q *heartbeatSpyWorkQueue) ValidateFence(fence FenceValidation) error {
	return q.inner.ValidateFence(fence)
}

func TestPostgresWorkQueueFenceRejectsStaleWorkerCommit(t *testing.T) {
	stores := newPostgresRuntimeStoresForTest(t)
	queue := stores.WorkQueue
	clock := NewManualClock(time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC))
	item := WorkItem{
		ID:            "work-fence-postgres",
		Kind:          WorkItemKindExecuteReadyTask,
		Status:        WorkItemStatusQueued,
		DedupeKey:     "dedupe-fence-postgres",
		GraphID:       "graph-fence-postgres",
		TaskID:        "task-fence-postgres",
		WorkflowID:    "workflow-fence-postgres",
		AvailableAt:   clock.Now(),
		LastUpdatedAt: clock.Now(),
		Reason:        "postgres fence proof",
	}
	enqueued, err := queue.Enqueue(item)
	if err != nil {
		t.Fatalf("enqueue work item: %v", err)
	}
	if enqueued.Disposition != WorkEnqueueDispositionEnqueued {
		t.Fatalf("expected real enqueue, got %+v", enqueued)
	}
	claimsA, err := queue.ClaimReady(WorkerID("worker-a"), 1, clock.Now(), 5*time.Second)
	if err != nil {
		t.Fatalf("claim by worker-a: %v", err)
	}
	if len(claimsA) != 1 {
		t.Fatalf("expected one claim for worker-a, got %+v", claimsA)
	}
	clock.Advance(6 * time.Second)
	reclaimed, err := queue.ReclaimExpired(clock.Now())
	if err != nil {
		t.Fatalf("reclaim expired lease: %v", err)
	}
	if len(reclaimed) != 1 {
		t.Fatalf("expected one reclaimed lease, got %+v", reclaimed)
	}
	claimsB, err := queue.ClaimReady(WorkerID("worker-b"), 1, clock.Now(), 5*time.Second)
	if err != nil {
		t.Fatalf("claim by worker-b after reclaim: %v", err)
	}
	if len(claimsB) != 1 {
		t.Fatalf("expected one claim for worker-b, got %+v", claimsB)
	}
	err = queue.Heartbeat(LeaseHeartbeat{
		WorkItemID:     claimsA[0].WorkItem.ID,
		WorkerID:       claimsA[0].WorkerID,
		LeaseID:        claimsA[0].LeaseID,
		FencingToken:   claimsA[0].FencingToken,
		RecordedAt:     clock.Now(),
		LeaseExpiresAt: clock.Now().Add(5 * time.Second),
	})
	if err == nil || !IsConflict(err) {
		t.Fatalf("expected stale worker heartbeat conflict, got %v", err)
	}
	err = queue.Fail(FenceValidation{
		WorkItemID:   claimsA[0].WorkItem.ID,
		LeaseID:      claimsA[0].LeaseID,
		FencingToken: claimsA[0].FencingToken,
		WorkerID:     claimsA[0].WorkerID,
	}, "stale failure", clock.Now())
	if err == nil || !IsConflict(err) {
		t.Fatalf("expected stale worker fail conflict, got %v", err)
	}
	if err := queue.Requeue(FenceValidation{
		WorkItemID:   claimsB[0].WorkItem.ID,
		LeaseID:      claimsB[0].LeaseID,
		FencingToken: claimsB[0].FencingToken,
		WorkerID:     claimsB[0].WorkerID,
	}, clock.Now().Add(time.Second), "requeue current owner", clock.Now()); err != nil {
		t.Fatalf("requeue with current fence: %v", err)
	}
	clock.Advance(2 * time.Second)
	claimsC, err := queue.ClaimReady(WorkerID("worker-c"), 1, clock.Now(), 5*time.Second)
	if err != nil {
		t.Fatalf("claim by worker-c after requeue: %v", err)
	}
	if len(claimsC) != 1 {
		t.Fatalf("expected one claim for worker-c, got %+v", claimsC)
	}
	err = queue.Requeue(FenceValidation{
		WorkItemID:   claimsB[0].WorkItem.ID,
		LeaseID:      claimsB[0].LeaseID,
		FencingToken: claimsB[0].FencingToken,
		WorkerID:     claimsB[0].WorkerID,
	}, clock.Now().Add(time.Second), "stale requeue", clock.Now())
	if err == nil || !IsConflict(err) {
		t.Fatalf("expected stale worker requeue conflict, got %v", err)
	}
	err = queue.Complete(FenceValidation{
		WorkItemID:   claimsA[0].WorkItem.ID,
		LeaseID:      claimsA[0].LeaseID,
		FencingToken: claimsA[0].FencingToken,
		WorkerID:     claimsA[0].WorkerID,
	}, clock.Now())
	if err == nil || !IsConflict(err) {
		t.Fatalf("expected stale worker completion conflict, got %v", err)
	}
	if err := queue.Complete(FenceValidation{
		WorkItemID:   claimsC[0].WorkItem.ID,
		LeaseID:      claimsC[0].LeaseID,
		FencingToken: claimsC[0].FencingToken,
		WorkerID:     claimsC[0].WorkerID,
	}, clock.Now()); err != nil {
		t.Fatalf("complete with current fence: %v", err)
	}
}

func TestPostgresWorkQueueDedupeIsAtomicUnderConcurrentEnqueue(t *testing.T) {
	stores := newPostgresRuntimeStoresForTest(t)
	queue := stores.WorkQueue
	now := time.Date(2026, 3, 31, 10, 15, 0, 0, time.UTC)
	const writers = 12
	results := make([]WorkEnqueueResult, writers)
	errs := make([]error, writers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = queue.Enqueue(WorkItem{
				ID:            fmt.Sprintf("work-dedupe-postgres-%02d", i),
				Kind:          WorkItemKindReevaluateTaskGraph,
				Status:        WorkItemStatusQueued,
				DedupeKey:     "dedupe:postgres:atomic",
				GraphID:       "graph-dedupe-postgres",
				WorkflowID:    "workflow-dedupe-postgres",
				AvailableAt:   now,
				LastUpdatedAt: now,
				Reason:        "concurrent dedupe proof",
			})
		}()
	}
	close(start)
	wg.Wait()
	enqueuedCount := 0
	suppressedCount := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("enqueue %d failed: %v", i, err)
		}
		switch results[i].Disposition {
		case WorkEnqueueDispositionEnqueued:
			enqueuedCount++
		case WorkEnqueueDispositionDuplicateSuppressed:
			suppressedCount++
		default:
			t.Fatalf("unexpected enqueue disposition: %+v", results[i])
		}
	}
	if enqueuedCount != 1 || suppressedCount != writers-1 {
		t.Fatalf("expected one real enqueue and %d duplicates, got enqueued=%d suppressed=%d results=%+v", writers-1, enqueuedCount, suppressedCount, results)
	}
	items, err := queue.ListByGraph("graph-dedupe-postgres")
	if err != nil {
		t.Fatalf("list work items by graph: %v", err)
	}
	active := 0
	for _, item := range items {
		if item.Status == WorkItemStatusQueued || item.Status == WorkItemStatusClaimed {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("expected exactly one active deduped work item, got %d items=%+v", active, items)
	}
}

func TestPostgresReclaimHasSingleWinner(t *testing.T) {
	stores := newPostgresRuntimeStoresForTest(t)
	queue := stores.WorkQueue
	clock := NewManualClock(time.Date(2026, 3, 31, 10, 30, 0, 0, time.UTC))
	if _, err := queue.Enqueue(WorkItem{
		ID:            "work-reclaim-postgres",
		Kind:          WorkItemKindExecuteReadyTask,
		Status:        WorkItemStatusQueued,
		DedupeKey:     "dedupe-reclaim-postgres",
		GraphID:       "graph-reclaim-postgres",
		TaskID:        "task-reclaim-postgres",
		WorkflowID:    "workflow-reclaim-postgres",
		AvailableAt:   clock.Now(),
		LastUpdatedAt: clock.Now(),
		Reason:        "reclaim proof",
	}); err != nil {
		t.Fatalf("enqueue work item: %v", err)
	}
	claims, err := queue.ClaimReady(WorkerID("worker-a"), 1, clock.Now(), 5*time.Second)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim initial lease: err=%v claims=%+v", err, claims)
	}
	clock.Advance(6 * time.Second)
	start := make(chan struct{})
	results := make([][]LeaseReclaimResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = queue.ReclaimExpired(clock.Now())
		}()
	}
	close(start)
	wg.Wait()
	total := 0
	nonEmpty := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("reclaim call %d failed: %v", i, err)
		}
		total += len(results[i])
		if len(results[i]) > 0 {
			nonEmpty++
		}
	}
	if total != 1 || nonEmpty != 1 {
		t.Fatalf("expected single reclaim winner, got total=%d non_empty=%d results=%+v", total, nonEmpty, results)
	}
}

func TestPostgresHeartbeatRenewsLeaseDuringLongExecution(t *testing.T) {
	clock := NewManualClock(time.Date(2026, 3, 31, 11, 0, 0, 0, time.UTC))
	started := make(chan struct{})
	release := make(chan struct{})
	spy := &heartbeatSpyWorkQueue{heartbeats: make(chan LeaseHeartbeat, 4)}
	harness := newPostgresAsyncHarness(t, clock, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(ctx context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return FollowUpWorkflowRunResult{}, ctx.Err()
			}
			return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(clock.Now(), current.Version.Sequence+1)), nil
		},
	}, spy)
	defer func() { _ = harness.bundle.Close() }()
	if _, err := harness.service.workQueue.Enqueue(WorkItem{
		ID:            "work-heartbeat-postgres",
		Kind:          WorkItemKindExecuteReadyTask,
		Status:        WorkItemStatusQueued,
		DedupeKey:     "execute:" + harness.graphID,
		GraphID:       harness.graphID,
		TaskID:        harness.taskID,
		WorkflowID:    harness.graphID,
		AvailableAt:   clock.Now(),
		LastUpdatedAt: clock.Now(),
		Reason:        "long-running heartbeat proof",
	}); err != nil {
		t.Fatalf("enqueue execute work: %v", err)
	}
	manualTicker := newManualLeaseTicker()
	done := make(chan struct{})
	var workerResult WorkerPassResult
	var workerErr error
	go func() {
		defer close(done)
		workerResult, workerErr = harness.service.RunAsyncWorkerOnce(context.Background(), DefaultAutoExecutionPolicy(), WorkerRunOptions{
			WorkerID:           WorkerID("worker-heartbeat"),
			Role:               WorkerRoleWorker,
			LeaseTTL:           5 * time.Second,
			HeartbeatInterval:  2 * time.Second,
			ClaimBatch:         1,
			leaseTickerFactory: staticLeaseTickerFactory{ticker: manualTicker},
		}, false)
	}()
	<-started
	select {
	case <-spy.heartbeats:
	default:
	}
	itemBefore, ok, err := harness.service.workQueue.Load("work-heartbeat-postgres")
	if err != nil || !ok || itemBefore.LeaseExpiresAt == nil {
		t.Fatalf("load claimed work item before renewal: ok=%t err=%v item=%+v", ok, err, itemBefore)
	}
	initialExpiry := *itemBefore.LeaseExpiresAt
	clock.Advance(2 * time.Second)
	manualTicker.Tick(clock.Now())
	heartbeat := <-spy.heartbeats
	itemAfterFirst, ok, err := harness.service.workQueue.Load("work-heartbeat-postgres")
	if err != nil || !ok || itemAfterFirst.LeaseExpiresAt == nil {
		t.Fatalf("load work item after first heartbeat: ok=%t err=%v item=%+v", ok, err, itemAfterFirst)
	}
	if !itemAfterFirst.LeaseExpiresAt.After(initialExpiry) {
		t.Fatalf("expected lease expiry to extend after heartbeat, before=%s after=%s heartbeat=%+v", initialExpiry, itemAfterFirst.LeaseExpiresAt, heartbeat)
	}
	clock.Advance(2 * time.Second)
	manualTicker.Tick(clock.Now())
	secondHeartbeat := <-spy.heartbeats
	if !secondHeartbeat.LeaseExpiresAt.After(heartbeat.LeaseExpiresAt) {
		t.Fatalf("expected later heartbeat to extend lease again, first=%+v second=%+v", heartbeat, secondHeartbeat)
	}
	close(release)
	<-done
	if workerErr != nil {
		t.Fatalf("worker run with periodic heartbeat: %v", workerErr)
	}
	if len(workerResult.CompletedWorkItemIDs) != 1 || workerResult.CompletedWorkItemIDs[0] != "work-heartbeat-postgres" {
		t.Fatalf("expected worker to complete long-running claim, got %+v", workerResult)
	}
}

func TestRuntimePromotionProfileApprovalResumeAcrossWorkers(t *testing.T) {
	now := time.Date(2026, 3, 31, 11, 30, 0, 0, time.UTC)
	harness := newRuntimePromotionHarness(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			return waitingApprovalWithPayloadResult("workflow-child-"+spec.ID, spec.ID, current), nil
		},
		resume: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState, _ CheckpointRecord, _ ResumeToken, _ CheckpointPayloadEnvelope) (FollowUpWorkflowRunResult, error) {
			return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(now, current.Version.Sequence+1)), nil
		},
	}, nil)
	defer func() { _ = harness.bundle.Close() }()
	if _, err := harness.service.runtime.ExecuteReadyFollowUps(context.Background(), ExecutionContext{
		WorkflowID:    harness.graphID,
		CorrelationID: harness.graphID,
		Attempt:       1,
	}, harness.graphID, DefaultAutoExecutionPolicy()); err != nil {
		t.Fatalf("seed waiting approval execution: %v", err)
	}
	approval, ok, err := harness.service.runtime.Approvals.LoadByTask(harness.graphID, harness.taskID)
	if err != nil || !ok {
		t.Fatalf("load approval: ok=%t err=%v approval=%+v", ok, err, approval)
	}
	result, err := harness.operator.ApproveTask(context.Background(), ApproveTaskCommand{
		RequestID:  "approve-profile-resume",
		ApprovalID: approval.ApprovalID,
		Actor:      "operator",
		Roles:      []string{"operator"},
		Note:       "runtime-promotion approval resume",
	})
	if err != nil {
		t.Fatalf("approve task: %v", err)
	}
	if !result.AsyncDispatchAccepted || len(result.EnqueuedWorkKinds) != 1 || result.EnqueuedWorkKinds[0] != WorkItemKindResumeApprovedCheckpoint {
		t.Fatalf("expected async resume enqueue, got %+v", result)
	}
	workerResult, err := harness.service.RunAsyncWorkerOnce(context.Background(), DefaultAutoExecutionPolicy(), WorkerRunOptions{
		WorkerID:   WorkerID("worker-profile-b"),
		Role:       WorkerRoleWorker,
		LeaseTTL:   30 * time.Second,
		ClaimBatch: 2,
	}, false)
	if err != nil {
		t.Fatalf("run worker across profile: %v", err)
	}
	if len(workerResult.ResumedTasks) != 1 || workerResult.ResumedTasks[0] != harness.taskID {
		t.Fatalf("expected different worker to resume approved task, got %+v", workerResult)
	}
	view, err := harness.replayQuery.Query(context.Background(), observability.ReplayQuery{TaskGraphID: harness.graphID})
	if err != nil {
		t.Fatalf("query replay view: %v", err)
	}
	if !containsString(view.Explanation.WhyAsyncRuntime, "worker-profile-b") {
		t.Fatalf("expected replay explanation to mention worker-profile-b, got %+v", view.Explanation)
	}
}

func TestRuntimePromotionProfileRetryBackoffAcrossWorkers(t *testing.T) {
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	attempts := 0
	var harness *storeBackedAsyncHarness
	harness = newRuntimePromotionHarness(t, now, runtimeTestCapability{
		name: "tax_optimization_workflow",
		execute: func(_ context.Context, spec taskspec.TaskSpec, _ FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error) {
			attempts++
			if attempts == 1 {
				return FollowUpWorkflowRunResult{}, &FollowUpExecutionError{
					Category: FailureCategoryTransient,
					Summary:  "temporary upstream failure",
					Err:      fmt.Errorf("temporary upstream failure"),
				}
			}
			return completedFollowUpResult("workflow-child-"+spec.ID, runtimeTestState(harness.clock.Now(), current.Version.Sequence+1)), nil
		},
	}, nil)
	defer func() { _ = harness.bundle.Close() }()
	if _, err := harness.service.workQueue.Enqueue(WorkItem{
		ID:            "work-profile-retry",
		Kind:          WorkItemKindExecuteReadyTask,
		Status:        WorkItemStatusQueued,
		DedupeKey:     "execute:" + harness.graphID,
		GraphID:       harness.graphID,
		TaskID:        harness.taskID,
		WorkflowID:    harness.graphID,
		AvailableAt:   harness.clock.Now(),
		LastUpdatedAt: harness.clock.Now(),
		Reason:        "runtime-promotion retry proof",
	}); err != nil {
		t.Fatalf("enqueue initial retry work: %v", err)
	}
	firstWorker, err := harness.service.RunAsyncWorkerOnce(context.Background(), DefaultAutoExecutionPolicy(), WorkerRunOptions{
		WorkerID:   WorkerID("worker-profile-a"),
		Role:       WorkerRoleWorker,
		LeaseTTL:   30 * time.Second,
		ClaimBatch: 1,
	}, false)
	if err != nil && !strings.Contains(err.Error(), "temporary upstream failure") {
		t.Fatalf("first worker pass: %v", err)
	}
	if len(firstWorker.Executed) != 1 || firstWorker.Executed[0].Status != TaskQueueStatusFailed {
		t.Fatalf("expected failed execution on first worker, got %+v", firstWorker)
	}
	harness.clock.Advance(6 * time.Second)
	tick, err := harness.service.schedulerService(DefaultAutoExecutionPolicy()).Tick(context.Background())
	if err != nil {
		t.Fatalf("scheduler tick for retry: %v", err)
	}
	if len(tick.EnqueuedWorkKinds) == 0 || tick.EnqueuedWorkKinds[len(tick.EnqueuedWorkKinds)-1] != WorkItemKindRetryFailedExecution {
		t.Fatalf("expected retry work item after backoff, got %+v", tick)
	}
	secondWorker, err := harness.service.RunAsyncWorkerOnce(context.Background(), DefaultAutoExecutionPolicy(), WorkerRunOptions{
		WorkerID:   WorkerID("worker-profile-b"),
		Role:       WorkerRoleWorker,
		LeaseTTL:   30 * time.Second,
		ClaimBatch: 2,
	}, false)
	if err != nil {
		t.Fatalf("second worker pass: %v", err)
	}
	if len(secondWorker.CompletedWorkItemIDs) == 0 {
		t.Fatalf("expected later worker to complete retry, got %+v", secondWorker)
	}
	view, err := harness.replayQuery.Query(context.Background(), observability.ReplayQuery{TaskGraphID: harness.graphID})
	if err != nil {
		t.Fatalf("query replay after retry: %v", err)
	}
	if !containsString(view.Explanation.WhyAsyncRuntime, "retry") {
		t.Fatalf("expected replay explanation to mention retry scheduling, got %+v", view.Explanation)
	}
}

func TestPostgresSkillExecutionStoreParity(t *testing.T) {
	stores := newPostgresRuntimeStoresForTest(t)
	record := SkillExecutionRecord{
		WorkflowID:  "workflow-skill-parity",
		TaskID:      "task-skill-parity",
		ExecutionID: "execution-skill-parity",
		Selection:   skillSelectionForParity(),
		Status:      "executed",
		CreatedAt:   time.Date(2026, 3, 31, 12, 30, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 3, 31, 12, 31, 0, 0, time.UTC),
	}
	if err := stores.SkillExecutions.Save(record); err != nil {
		t.Fatalf("save skill execution parity record: %v", err)
	}
	loaded, ok, err := stores.SkillExecutions.Load(record.ExecutionID)
	if err != nil || !ok {
		t.Fatalf("load skill execution parity record: ok=%t err=%v record=%+v", ok, err, loaded)
	}
	if loaded.Selection.RecipeID != record.Selection.RecipeID {
		t.Fatalf("expected saved recipe to round-trip, got %+v", loaded)
	}
	records, err := stores.SkillExecutions.ListByWorkflow(record.WorkflowID)
	if err != nil {
		t.Fatalf("list skill execution parity records: %v", err)
	}
	if len(records) != 1 || records[0].ExecutionID != record.ExecutionID {
		t.Fatalf("expected one workflow-scoped skill execution record, got %+v", records)
	}
}

func newPostgresRuntimeStoresForTest(t *testing.T) *PostgresRuntimeStores {
	t.Helper()
	dsn := requirePostgresRuntimeDSN(t)
	resetPostgresRuntimeDatabase(t, dsn)
	stores, err := NewPostgresRuntimeStores(dsn)
	if err != nil {
		t.Fatalf("create postgres runtime stores: %v", err)
	}
	t.Cleanup(func() { _ = stores.DB.Close() })
	return stores
}

func newPostgresAsyncHarness(t *testing.T, clock *ManualClock, capability runtimeTestCapability, queueOverride WorkQueueStore) *storeBackedAsyncHarness {
	t.Helper()
	stores := newPostgresRuntimeStoresForTest(t)
	return newStoreBackedHarness(t, clock, BundleFromPostgres(stores), capability, queueOverride)
}

func newRuntimePromotionHarness(t *testing.T, now time.Time, capability runtimeTestCapability, queueOverride WorkQueueStore) *storeBackedAsyncHarness {
	t.Helper()
	env := requireRuntimePromotionEnv(t)
	resetPostgresRuntimeDatabase(t, env.DSN)
	clock := NewManualClock(now)
	bundle, _, err := OpenStoreBundle(StoreFactoryOptions{
		RuntimeProfile: "runtime-promotion",
		RuntimeBackend: "postgres",
		PostgresDSN:    env.DSN,
		BlobBackend:    "minio",
		BlobEndpoint:   env.Endpoint,
		BlobBucket:     env.Bucket,
		BlobAccessKey:  env.AccessKey,
		BlobSecretKey:  env.SecretKey,
		Now:            clock.Now,
	})
	if err != nil {
		t.Fatalf("open runtime-promotion bundle: %v", err)
	}
	t.Cleanup(func() { _ = bundle.Close() })
	return newStoreBackedHarness(t, clock, bundle, capability, queueOverride)
}

func newStoreBackedHarness(t *testing.T, clock *ManualClock, bundle *StoreBundle, capability runtimeTestCapability, queueOverride WorkQueueStore) *storeBackedAsyncHarness {
	t.Helper()
	queue := bundle.WorkQueue
	if queueOverride != nil {
		if spy, ok := queueOverride.(*heartbeatSpyWorkQueue); ok && spy.inner == nil {
			spy.inner = bundle.WorkQueue
		}
		queue = queueOverride
	}
	service := NewService(ServiceOptions{
		CheckpointStore: bundle.Checkpoints,
		TaskGraphs:      bundle.TaskGraphs,
		Executions:      bundle.Executions,
		Approvals:       bundle.Approvals,
		OperatorActions: bundle.OperatorActions,
		Replay:          bundle.Replay,
		Artifacts:       bundle.Artifacts,
		WorkQueue:       queue,
		WorkAttempts:    bundle.WorkAttempts,
		Workers:         bundle.Workers,
		Scheduler:       bundle.Scheduler,
		Controller:      DefaultWorkflowController{},
		Clock:           clock,
		BackendProfile:  bundle.Profile,
	})
	service.SetCapabilities(StaticTaskCapabilityResolver{
		Capabilities: map[taskspec.UserIntentType]string{
			taskspec.UserIntentTaxOptimization: "tax_optimization_workflow",
		},
		Workflows: map[taskspec.UserIntentType]FollowUpWorkflowCapability{
			taskspec.UserIntentTaxOptimization: capability,
		},
	})
	rebuilder := NewReplayProjectionRebuilder(service, bundle.WorkflowRuns, bundle.ReplayProjection, service.runtime.Artifacts, service.runtime.Replay, clock.Now)
	service.SetReplayProjectionWriter(rebuilder)
	query := NewReplayQueryService(service, bundle.WorkflowRuns, bundle.ReplayQuery, service.runtime.Artifacts, service.runtime.Replay)

	suffix := sanitizeRuntimeTestSuffix(t.Name())
	graph, taskID := storeBackedRuntimeGraph(clock.Now(), suffix)
	execCtx := ExecutionContext{WorkflowID: graph.ParentWorkflowID, TaskID: graph.ParentTaskID, CorrelationID: graph.ParentWorkflowID, Attempt: 1}
	if _, err := service.runtime.RegisterFollowUpTasks(execCtx, graph, runtimeTestState(clock.Now(), 1)); err != nil {
		t.Fatalf("register store-backed follow-up tasks: %v", err)
	}

	return &storeBackedAsyncHarness{
		service:     service,
		operator:    NewOperatorService(service),
		clock:       clock,
		replayQuery: query,
		bundle:      bundle,
		graphID:     graph.GraphID,
		taskID:      taskID,
	}
}

func storeBackedRuntimeGraph(now time.Time, suffix string) (taskspec.TaskGraph, string) {
	taskID := "task-tax-" + suffix
	task := runtimeGeneratedTask(now, taskID, taskspec.UserIntentTaxOptimization, 1)
	graph := runtimeTestGraph(now, task)
	graph.GraphID = "graph-runtime-" + suffix
	graph.ParentWorkflowID = "workflow-life-event-" + suffix
	graph.ParentTaskID = "task-life-event-" + suffix
	for i := range graph.GeneratedTasks {
		graph.GeneratedTasks[i].Metadata.ParentWorkflowID = graph.ParentWorkflowID
		graph.GeneratedTasks[i].Metadata.ParentTaskID = graph.ParentTaskID
		graph.GeneratedTasks[i].Metadata.RootCorrelationID = graph.ParentWorkflowID
	}
	return graph, taskID
}

func sanitizeRuntimeTestSuffix(name string) string {
	replacer := strings.NewReplacer("/", "-", " ", "-", "_", "-", ":", "-", ".", "-")
	return strings.ToLower(replacer.Replace(name))
}

func requirePostgresRuntimeDSN(t *testing.T) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("PERSONAL_CFO_RUNTIME_DSN"))
	if dsn == "" {
		t.Skip("set PERSONAL_CFO_RUNTIME_DSN to run promoted postgres runtime tests")
	}
	return dsn
}

func requireRuntimePromotionEnv(t *testing.T) runtimePromotionTestEnv {
	t.Helper()
	env := runtimePromotionTestEnv{
		DSN:       strings.TrimSpace(os.Getenv("PERSONAL_CFO_RUNTIME_DSN")),
		Endpoint:  strings.TrimSpace(os.Getenv("MINIO_TEST_ENDPOINT")),
		Bucket:    strings.TrimSpace(os.Getenv("MINIO_TEST_BUCKET")),
		AccessKey: strings.TrimSpace(os.Getenv("MINIO_TEST_ACCESS_KEY")),
		SecretKey: strings.TrimSpace(os.Getenv("MINIO_TEST_SECRET_KEY")),
	}
	if env.DSN == "" || env.Endpoint == "" || env.Bucket == "" || env.AccessKey == "" || env.SecretKey == "" {
		t.Skip("set PERSONAL_CFO_RUNTIME_DSN and MINIO_TEST_* env vars to run runtime-promotion promoted-backend proofs")
	}
	return env
}

func resetPostgresRuntimeDatabase(t *testing.T, dsn string) {
	t.Helper()
	db, err := NewPostgresRuntimeDB(dsn)
	if err != nil {
		t.Fatalf("open postgres runtime db for reset: %v", err)
	}
	defer func() { _ = db.Close() }()
	_, err = db.exec(`
		TRUNCATE TABLE
			task_graphs,
			state_snapshots,
			task_executions,
			skill_executions,
			approvals,
			operator_actions,
			checkpoints,
			resume_tokens,
			replay_events,
			workflow_runs,
			replay_projection_builds,
			workflow_replay_projections,
			task_graph_replay_projections,
			replay_provenance_nodes,
			replay_provenance_edges,
			replay_execution_attributions,
			replay_failure_attributions,
			workflow_artifacts,
			work_items,
			work_attempts,
			worker_registrations,
			scheduler_wakeups
	`)
	if err != nil {
		t.Fatalf("truncate postgres runtime tables: %v", err)
	}
}

func skillSelectionForParity() skills.SkillSelection {
	return skills.SkillSelection{
		Family:   skills.SkillFamilyDiscretionaryGuardrail,
		Version:  "v1",
		RecipeID: "budget_guardrail.v1",
	}
}
