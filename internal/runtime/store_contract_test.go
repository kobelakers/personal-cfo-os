package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
)

func TestSQLiteRuntimeCoreStoreContract(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runtime-contract.db")
	stores, err := NewSQLiteRuntimeStores(dbPath)
	if err != nil {
		t.Fatalf("create sqlite runtime stores: %v", err)
	}
	defer func() { _ = stores.DB.Close() }()
	exerciseRuntimeCoreStores(t, BundleFromSQLite(stores), "sqlite")
}

func TestPostgresRuntimeCoreStoreContract(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PERSONAL_CFO_RUNTIME_DSN"))
	if dsn == "" {
		t.Skip("set PERSONAL_CFO_RUNTIME_DSN to run postgres runtime store contract test")
	}
	stores, err := NewPostgresRuntimeStores(dsn)
	if err != nil {
		t.Fatalf("create postgres runtime stores: %v", err)
	}
	defer func() { _ = stores.DB.Close() }()
	exerciseRuntimeCoreStores(t, BundleFromPostgres(stores), "postgres")
}

func TestRuntimePromotionProfileContract(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PERSONAL_CFO_RUNTIME_DSN"))
	endpoint := strings.TrimSpace(os.Getenv("MINIO_TEST_ENDPOINT"))
	bucket := strings.TrimSpace(os.Getenv("MINIO_TEST_BUCKET"))
	accessKey := strings.TrimSpace(os.Getenv("MINIO_TEST_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("MINIO_TEST_SECRET_KEY"))
	if dsn == "" || endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		t.Skip("set PERSONAL_CFO_RUNTIME_DSN and MINIO_TEST_* env vars to run runtime-promotion profile contract test")
	}
	bundle, _, err := OpenStoreBundle(StoreFactoryOptions{
		RuntimeProfile: "runtime-promotion",
		RuntimeBackend: "postgres",
		PostgresDSN:    dsn,
		BlobBackend:    "minio",
		BlobEndpoint:   endpoint,
		BlobBucket:     bucket,
		BlobAccessKey:  accessKey,
		BlobSecretKey:  secretKey,
		Now: func() time.Time {
			return time.Date(2026, 3, 30, 14, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("open runtime-promotion store bundle: %v", err)
	}
	defer func() { _ = bundle.Close() }()
	exerciseRuntimeCoreStores(t, bundle, "runtime-promotion")
}

func exerciseRuntimeCoreStores(t *testing.T, stores *StoreBundle, backend string) {
	t.Helper()
	now := time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC)
	suffix := backend + "-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))

	workflow := WorkflowRunRecord{
		WorkflowID:   "workflow-" + suffix,
		TaskID:       "task-root-" + suffix,
		Intent:       "runtime_contract",
		RuntimeState: WorkflowStateCompleted,
		Summary:      "contract workflow run",
		StartedAt:    now,
		UpdatedAt:    now,
	}
	if err := stores.WorkflowRuns.Save(workflow); err != nil {
		t.Fatalf("save workflow run: %v", err)
	}
	if loaded, ok, err := stores.WorkflowRuns.Load(workflow.WorkflowID); err != nil || !ok || loaded.WorkflowID != workflow.WorkflowID {
		t.Fatalf("load workflow run: ok=%t err=%v record=%+v", ok, err, loaded)
	}

	graph := runtimeTestGraph(now, runtimeGeneratedTask(now, "task-"+suffix, "tax_optimization", 1))
	baseState := runtimeTestState(now, 1)
	baseSnapshot := baseState.Snapshot("contract", now)
	graphSnapshot := TaskGraphSnapshot{
		Graph:   graph,
		Version: 1,
		RegisteredTasks: []FollowUpTaskRecord{{
			Task:          graph.GeneratedTasks[0].Task,
			Metadata:      graph.GeneratedTasks[0].Metadata,
			Status:        TaskQueueStatusReady,
			RegisteredAt:  now,
			LastUpdatedAt: now,
		}},
		BaseStateSnapshot:            baseSnapshot,
		BaseStateSnapshotRef:         baseSnapshot.State.Version.SnapshotID,
		LatestCommittedStateSnapshot: baseSnapshot,
		LatestCommittedStateRef:      baseSnapshot.State.Version.SnapshotID,
		RegisteredAt:                 now,
	}
	if err := stores.TaskGraphs.Save(graphSnapshot); err != nil {
		t.Fatalf("save task graph: %v", err)
	}
	graphSnapshot.Version = 2
	if _, err := stores.TaskGraphs.SaveStateSnapshot(graph.GraphID, graph.ParentWorkflowID, graph.GeneratedTasks[0].Task.ID, "contract", baseSnapshot); err != nil {
		t.Fatalf("save state snapshot: %v", err)
	}
	if err := stores.TaskGraphs.Update(graphSnapshot, 1); err != nil {
		t.Fatalf("update task graph: %v", err)
	}
	if loaded, ok, err := stores.TaskGraphs.Load(graph.GraphID); err != nil || !ok || loaded.Version != 2 {
		t.Fatalf("load task graph: ok=%t err=%v snapshot=%+v", ok, err, loaded)
	}

	execution := TaskExecutionRecord{
		ExecutionID:       "execution-" + suffix,
		TaskID:            graph.GeneratedTasks[0].Task.ID,
		Intent:            graph.GeneratedTasks[0].Task.UserIntentType,
		ParentGraphID:     graph.GraphID,
		ParentWorkflowID:  graph.ParentWorkflowID,
		RootTaskID:        graph.ParentTaskID,
		TriggeredByTaskID: graph.GeneratedTasks[0].Task.ID,
		WorkflowID:        "workflow-child-" + suffix,
		Status:            TaskQueueStatusExecuting,
		Version:           1,
		StartedAt:         now,
		LastTransitionAt:  now,
		InputStateVersion: 1,
	}
	if err := stores.Executions.Save(execution); err != nil {
		t.Fatalf("save execution: %v", err)
	}
	execution.Status = TaskQueueStatusCompleted
	execution.LastTransitionAt = now.Add(time.Minute)
	if err := stores.Executions.Update(execution, 1); err != nil {
		t.Fatalf("update execution: %v", err)
	}
	if loaded, ok, err := stores.Executions.LoadLatestByTask(graph.GraphID, graph.GeneratedTasks[0].Task.ID); err != nil || !ok || loaded.Status != TaskQueueStatusCompleted {
		t.Fatalf("load latest execution: ok=%t err=%v record=%+v", ok, err, loaded)
	}

	approval := ApprovalStateRecord{
		ApprovalID:      "approval-" + suffix,
		GraphID:         graph.GraphID,
		TaskID:          graph.GeneratedTasks[0].Task.ID,
		WorkflowID:      workflow.WorkflowID,
		RequestedAction: "resume",
		RequestedAt:     now,
		Status:          ApprovalStatusPending,
		Version:         1,
	}
	if err := stores.Approvals.Save(approval); err != nil {
		t.Fatalf("save approval: %v", err)
	}
	approval.Status = ApprovalStatusApproved
	if err := stores.Approvals.Update(approval, 1); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	action := OperatorActionRecord{
		ActionID:    "action-" + suffix,
		RequestID:   "request-" + suffix,
		ActionType:  OperatorActionApprove,
		Actor:       "operator",
		GraphID:     graph.GraphID,
		TaskID:      graph.GeneratedTasks[0].Task.ID,
		ApprovalID:  approval.ApprovalID,
		Status:      OperatorActionStatusApplied,
		RequestedAt: now,
	}
	if err := stores.OperatorActions.Save(action); err != nil {
		t.Fatalf("save operator action: %v", err)
	}
	if loaded, ok, err := stores.OperatorActions.LoadByRequestID(action.RequestID); err != nil || !ok || loaded.ActionID != action.ActionID {
		t.Fatalf("load operator action: ok=%t err=%v record=%+v", ok, err, loaded)
	}

	checkpoint := CheckpointRecord{
		ID:           "checkpoint-" + suffix,
		WorkflowID:   workflow.WorkflowID,
		State:        WorkflowStateWaitingApproval,
		ResumeState:  WorkflowStateActing,
		StateVersion: 1,
		Summary:      "contract checkpoint",
		CapturedAt:   now,
	}
	if err := stores.Checkpoints.Save(checkpoint); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	token := ResumeToken{
		Token:        "resume-" + suffix,
		WorkflowID:   workflow.WorkflowID,
		CheckpointID: checkpoint.ID,
		IssuedAt:     now,
		ExpiresAt:    now.Add(24 * time.Hour),
	}
	if err := stores.Checkpoints.SaveResumeToken(token); err != nil {
		t.Fatalf("save resume token: %v", err)
	}
	payload := CheckpointPayloadEnvelope{
		Kind: CheckpointPayloadKindFollowUpFinalizeResume,
		FollowUpFinalizeResume: &FollowUpFinalizeResumePayload{
			GraphID:                 graph.GraphID,
			TaskID:                  graph.GeneratedTasks[0].Task.ID,
			WorkflowID:              workflow.WorkflowID,
			ArtifactKind:            reporting.ArtifactKindTaxOptimizationReport,
			DraftReport:             reporting.ReportPayload{TaxOptimization: &reporting.TaxOptimizationReport{TaskID: graph.GeneratedTasks[0].Task.ID, WorkflowID: workflow.WorkflowID, Summary: "contract", DeterministicMetrics: map[string]any{"rate": 0.2}, Confidence: 0.8, GeneratedAt: now}},
			PendingStateSnapshotRef: baseSnapshot.State.Version.SnapshotID,
		},
	}
	if err := stores.Checkpoints.SavePayload(checkpoint.ID, payload); err != nil {
		t.Fatalf("save checkpoint payload: %v", err)
	}
	if loaded, err := stores.Checkpoints.LoadPayload(checkpoint.ID); err != nil || loaded.Kind != payload.Kind {
		t.Fatalf("load checkpoint payload: err=%v payload=%+v", err, loaded)
	}

	event := ReplayEventRecord{
		EventID:     "event-" + suffix,
		WorkflowID:  workflow.WorkflowID,
		GraphID:     graph.GraphID,
		TaskID:      graph.GeneratedTasks[0].Task.ID,
		ExecutionID: execution.ExecutionID,
		ActionType:  "contract",
		Summary:     "contract replay event",
		OccurredAt:  now,
	}
	if err := stores.Replay.Append(event); err != nil {
		t.Fatalf("append replay event: %v", err)
	}

	projection := TaskGraphReplayProjection{
		GraphID:             graph.GraphID,
		RuntimeState:        WorkflowStateCompleted,
		ProjectionStatus:    ReplayProjectionStatusComplete,
		SchemaVersion:       ReplayProjectionSchemaVersion,
		SummaryJSON:         `{"final_state":"completed"}`,
		ExplanationJSON:     `{}`,
		CompareInputJSON:    `{"final_state":"completed"}`,
		UpdatedAt:           now,
		ProjectionFreshness: now,
	}
	if err := stores.ReplayProjection.SaveTaskGraphProjection(projection); err != nil {
		t.Fatalf("save replay projection: %v", err)
	}
	if _, ok, err := stores.ReplayQuery.LoadTaskGraphProjection(graph.GraphID); err != nil || !ok {
		t.Fatalf("load replay projection: ok=%t err=%v", ok, err)
	}

	artifact := reporting.WorkflowArtifact{
		ID:         "artifact-" + suffix,
		WorkflowID: workflow.WorkflowID,
		TaskID:     graph.GeneratedTasks[0].Task.ID,
		Kind:       reporting.ArtifactKindReplayBundle,
		ProducedBy: "contract-test",
		Ref: reporting.ArtifactRef{
			Kind:    reporting.ArtifactKindReplayBundle,
			ID:      "artifact-" + suffix,
			Summary: "contract artifact",
		},
		ContentJSON: `{"ok":true}`,
		CreatedAt:   now,
	}
	if err := stores.Artifacts.SaveArtifact(workflow.WorkflowID, graph.GeneratedTasks[0].Task.ID, artifact); err != nil {
		t.Fatalf("save artifact: %v", err)
	}
	if loaded, ok, err := stores.Artifacts.LoadArtifact(artifact.ID); err != nil || !ok || loaded.ID != artifact.ID {
		t.Fatalf("load artifact: ok=%t err=%v artifact=%+v", ok, err, loaded)
	}

	workItem := WorkItem{
		ID:            "work-" + suffix,
		Kind:          WorkItemKindExecuteReadyTask,
		Status:        WorkItemStatusQueued,
		GraphID:       graph.GraphID,
		TaskID:        graph.GeneratedTasks[0].Task.ID,
		WorkflowID:    workflow.WorkflowID,
		AvailableAt:   now,
		LastUpdatedAt: now,
		Reason:        "contract work item",
	}
	if err := stores.WorkQueue.Enqueue(workItem); err != nil {
		t.Fatalf("enqueue work item: %v", err)
	}
	claims, err := stores.WorkQueue.ClaimReady(WorkerID("worker-"+suffix), 1, now, 30*time.Second)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim work item: err=%v claims=%+v", err, claims)
	}
	attempt := ExecutionAttempt{
		AttemptID:    "attempt-" + suffix,
		WorkItemID:   workItem.ID,
		WorkItemKind: workItem.Kind,
		WorkerID:     WorkerID("worker-" + suffix),
		LeaseID:      claims[0].LeaseID,
		FencingToken: claims[0].FencingToken,
		Status:       ExecutionAttemptStatusStarted,
		StartedAt:    now,
	}
	if err := stores.WorkAttempts.SaveAttempt(attempt); err != nil {
		t.Fatalf("save attempt: %v", err)
	}
	attempt.Status = ExecutionAttemptStatusSucceeded
	finished := now.Add(time.Minute)
	attempt.FinishedAt = &finished
	if err := stores.WorkAttempts.UpdateAttempt(attempt); err != nil {
		t.Fatalf("update attempt: %v", err)
	}
	if err := stores.Workers.Register(WorkerRegistration{WorkerID: WorkerID("worker-" + suffix), Role: WorkerRoleWorker, StartedAt: now, LastHeartbeat: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	wakeup := SchedulerWakeup{
		ID:          "wakeup-" + suffix,
		GraphID:     graph.GraphID,
		Kind:        SchedulerWakeupDependency,
		AvailableAt: now,
		Reason:      "contract wakeup",
	}
	if err := stores.Scheduler.SaveWakeup(wakeup); err != nil {
		t.Fatalf("save wakeup: %v", err)
	}
	due, err := stores.Scheduler.ListDueWakeups(now)
	if err != nil || len(due) == 0 {
		t.Fatalf("list due wakeups: err=%v due=%+v", err, due)
	}
	if err := stores.Scheduler.MarkWakeupDispatched(wakeup.ID, now); err != nil {
		t.Fatalf("mark wakeup dispatched: %v", err)
	}
}
