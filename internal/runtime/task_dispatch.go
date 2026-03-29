package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type FollowUpWorkflowCapability interface {
	CapabilityName() string
	Execute(ctx context.Context, spec taskspec.TaskSpec, activation FollowUpActivationContext, current state.FinancialWorldState) (FollowUpWorkflowRunResult, error)
	Resume(
		ctx context.Context,
		spec taskspec.TaskSpec,
		activation FollowUpActivationContext,
		current state.FinancialWorldState,
		checkpoint CheckpointRecord,
		token ResumeToken,
		payload CheckpointPayloadEnvelope,
	) (FollowUpWorkflowRunResult, error)
}

type FollowUpActivationContext struct {
	GraphID           string                          `json:"graph_id"`
	ParentGraphID     string                          `json:"parent_graph_id"`
	ParentWorkflowID  string                          `json:"parent_workflow_id"`
	RootTaskID        string                          `json:"root_task_id"`
	TriggeredByTaskID string                          `json:"triggered_by_task_id"`
	RootCorrelationID string                          `json:"root_correlation_id"`
	TriggerSource     taskspec.TaskTriggerSource      `json:"trigger_source"`
	GenerationReasons []taskspec.TaskGenerationReason `json:"generation_reasons,omitempty"`
	LifeEventID       string                          `json:"life_event_id,omitempty"`
	LifeEventKind     string                          `json:"life_event_kind,omitempty"`
	SourceEvidenceIDs []string                        `json:"source_evidence_ids,omitempty"`
	ExecutionDepth    int                             `json:"execution_depth"`
}

type FollowUpWorkflowRunResult struct {
	WorkflowID           string                       `json:"workflow_id"`
	RuntimeState         WorkflowExecutionState       `json:"runtime_state"`
	UpdatedState         state.FinancialWorldState    `json:"updated_state"`
	Artifacts            []reporting.WorkflowArtifact `json:"artifacts,omitempty"`
	Checkpoint           *CheckpointRecord            `json:"checkpoint,omitempty"`
	ResumeToken          *ResumeToken                 `json:"resume_token,omitempty"`
	CheckpointPayload    *CheckpointPayloadEnvelope   `json:"checkpoint_payload,omitempty"`
	PendingApproval      *HumanApprovalPending        `json:"pending_approval,omitempty"`
	FailureCategory      FailureCategory              `json:"failure_category,omitempty"`
	FailureSummary       string                       `json:"failure_summary,omitempty"`
	LastRecoveryStrategy RecoveryStrategy             `json:"last_recovery_strategy,omitempty"`
}

type TaskExecutionRecord struct {
	ExecutionID             string                  `json:"execution_id"`
	TaskID                  string                  `json:"task_id"`
	Intent                  taskspec.UserIntentType `json:"intent"`
	ParentGraphID           string                  `json:"parent_graph_id"`
	ParentWorkflowID        string                  `json:"parent_workflow_id"`
	RootTaskID              string                  `json:"root_task_id"`
	TriggeredByTaskID       string                  `json:"triggered_by_task_id"`
	ExecutionDepth          int                     `json:"execution_depth"`
	Capability              string                  `json:"capability"`
	WorkflowID              string                  `json:"workflow_id"`
	RootCorrelationID       string                  `json:"root_correlation_id"`
	CorrelationID           string                  `json:"correlation_id"`
	CausationID             string                  `json:"causation_id"`
	Status                  TaskQueueStatus         `json:"status"`
	Version                 int64                   `json:"version"`
	Attempt                 int                     `json:"attempt"`
	RetryCount              int                     `json:"retry_count"`
	LastRecoveryStrategy    RecoveryStrategy        `json:"last_recovery_strategy,omitempty"`
	LastTransitionAt        time.Time               `json:"last_transition_at"`
	StartedAt               time.Time               `json:"started_at"`
	CompletedAt             time.Time               `json:"completed_at,omitempty"`
	FailedAt                time.Time               `json:"failed_at,omitempty"`
	InputStateVersion       uint64                  `json:"input_state_version"`
	UpdatedStateVersion     uint64                  `json:"updated_state_version,omitempty"`
	InputStateSnapshotRef   string                  `json:"input_state_snapshot_ref,omitempty"`
	UpdatedStateSnapshotRef string                  `json:"updated_state_snapshot_ref,omitempty"`
	Committed               bool                    `json:"committed"`
	CheckpointID            string                  `json:"checkpoint_id,omitempty"`
	ResumeToken             string                  `json:"resume_token,omitempty"`
	ResumeState             WorkflowExecutionState  `json:"resume_state,omitempty"`
	ApprovalID              string                  `json:"approval_id,omitempty"`
	PendingApproval         bool                    `json:"pending_approval,omitempty"`
	FailureCategory         FailureCategory         `json:"failure_category,omitempty"`
	FailureSummary          string                  `json:"failure_summary,omitempty"`
	ArtifactIDs             []string                `json:"artifact_ids,omitempty"`
}

type AutoExecutionPolicy struct {
	AllowedIntents                map[taskspec.UserIntentType]struct{} `json:"allowed_intents,omitempty"`
	MaxExecutionDepth             int                                  `json:"max_execution_depth"`
	MaxAttempts                   int                                  `json:"max_attempts"`
	ContinueIndependentOnFailure  bool                                 `json:"continue_independent_on_failure"`
	ContinueIndependentOnApproval bool                                 `json:"continue_independent_on_approval"`
}

type TaskActivationResult struct {
	GraphID         string               `json:"graph_id"`
	RegisteredTasks []FollowUpTaskRecord `json:"registered_tasks,omitempty"`
	ReadyTaskIDs    []string             `json:"ready_task_ids,omitempty"`
	EvaluatedAt     time.Time            `json:"evaluated_at"`
}

type FollowUpExecutionBatchResult struct {
	GraphID                      string                `json:"graph_id"`
	RegisteredTasks              []FollowUpTaskRecord  `json:"registered_tasks,omitempty"`
	ExecutedTasks                []TaskExecutionRecord `json:"executed_tasks,omitempty"`
	LatestCommittedStateSnapshot state.StateSnapshot   `json:"latest_committed_state_snapshot"`
	ExecutedAt                   time.Time             `json:"executed_at"`
}

type FollowUpExecutionError struct {
	Category FailureCategory
	Summary  string
	Err      error
}

func (e *FollowUpExecutionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Summary != "" {
		return e.Summary
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "follow-up execution failed"
}

func (e *FollowUpExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func DefaultAutoExecutionPolicy() AutoExecutionPolicy {
	return AutoExecutionPolicy{
		AllowedIntents: map[taskspec.UserIntentType]struct{}{
			taskspec.UserIntentTaxOptimization:    {},
			taskspec.UserIntentPortfolioRebalance: {},
		},
		MaxExecutionDepth:             1,
		MaxAttempts:                   2,
		ContinueIndependentOnFailure:  true,
		ContinueIndependentOnApproval: true,
	}
}

func (p AutoExecutionPolicy) allows(task FollowUpTaskRecord) (bool, string) {
	if p.MaxExecutionDepth <= 0 {
		p.MaxExecutionDepth = 1
	}
	if task.Metadata.ExecutionDepth > p.MaxExecutionDepth {
		return false, fmt.Sprintf("suppressed because execution_depth > %d", p.MaxExecutionDepth)
	}
	if len(p.AllowedIntents) == 0 {
		return false, "not auto-run: no intents are allowlisted"
	}
	if _, ok := p.AllowedIntents[task.Task.UserIntentType]; !ok {
		return false, "not auto-run: intent not in allowlist"
	}
	return true, ""
}
