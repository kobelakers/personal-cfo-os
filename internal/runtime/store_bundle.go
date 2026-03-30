package runtime

type StoreBundle struct {
	CloseFn          func() error
	Backend          string
	Profile          string
	WorkflowRuns     WorkflowRunStore
	TaskGraphs       TaskGraphStore
	Executions       TaskExecutionStore
	SkillExecutions  SkillExecutionStore
	Approvals        ApprovalStateStore
	OperatorActions  OperatorActionStore
	Checkpoints      CheckpointStore
	Replay           ReplayStore
	ReplayProjection ReplayProjectionStore
	ReplayQuery      ReplayProjectionQueryStore
	Artifacts        ArtifactMetadataStore
	WorkQueue        WorkQueueStore
	WorkAttempts     WorkAttemptStore
	Workers          WorkerRegistryStore
	Scheduler        SchedulerStore
}

func (b *StoreBundle) Close() error {
	if b == nil || b.CloseFn == nil {
		return nil
	}
	return b.CloseFn()
}

func BundleFromSQLite(stores *SQLiteRuntimeStores) *StoreBundle {
	if stores == nil {
		return nil
	}
	return &StoreBundle{
		CloseFn:          stores.DB.Close,
		Backend:          "sqlite",
		Profile:          "local-lite",
		WorkflowRuns:     stores.WorkflowRuns,
		TaskGraphs:       stores.TaskGraphs,
		Executions:       stores.Executions,
		SkillExecutions:  stores.SkillExecutions,
		Approvals:        stores.Approvals,
		OperatorActions:  stores.OperatorActions,
		Checkpoints:      stores.Checkpoints,
		Replay:           stores.Replay,
		ReplayProjection: stores.ReplayProjection,
		ReplayQuery:      stores.ReplayQuery,
		Artifacts:        stores.Artifacts,
		WorkQueue:        stores.WorkQueue,
		WorkAttempts:     stores.WorkAttempts,
		Workers:          stores.Workers,
		Scheduler:        stores.Scheduler,
	}
}
