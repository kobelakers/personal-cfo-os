package runtime

import (
	"errors"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

var (
	ErrRuntimeConflict         = errors.New("runtime conflict")
	ErrRuntimeNotFound         = errors.New("runtime resource not found")
	ErrInvalidStateTransition  = errors.New("invalid runtime state transition")
	ErrDuplicateRequest        = errors.New("duplicate operator request")
	ErrCheckpointPayloadAbsent = errors.New("checkpoint payload not found")
)

type ConflictError struct {
	Resource string
	ID       string
	Reason   string
}

func (e *ConflictError) Error() string {
	if e == nil {
		return ErrRuntimeConflict.Error()
	}
	if e.Reason != "" {
		return fmt.Sprintf("%s %q conflict: %s", e.Resource, e.ID, e.Reason)
	}
	return fmt.Sprintf("%s %q conflict", e.Resource, e.ID)
}

func (e *ConflictError) Unwrap() error {
	return ErrRuntimeConflict
}

type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	if e == nil {
		return ErrRuntimeNotFound.Error()
	}
	return fmt.Sprintf("%s %q not found", e.Resource, e.ID)
}

func (e *NotFoundError) Unwrap() error {
	return ErrRuntimeNotFound
}

type InvalidTransitionError struct {
	Resource string
	ID       string
	From     string
	To       string
	Reason   string
}

func (e *InvalidTransitionError) Error() string {
	if e == nil {
		return ErrInvalidStateTransition.Error()
	}
	if e.Reason != "" {
		return fmt.Sprintf("%s %q cannot transition from %s to %s: %s", e.Resource, e.ID, e.From, e.To, e.Reason)
	}
	return fmt.Sprintf("%s %q cannot transition from %s to %s", e.Resource, e.ID, e.From, e.To)
}

func (e *InvalidTransitionError) Unwrap() error {
	return ErrInvalidStateTransition
}

func IsConflict(err error) bool {
	return errors.Is(err, ErrRuntimeConflict) || errors.Is(err, ErrInvalidStateTransition)
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrRuntimeNotFound)
}

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusDenied   ApprovalStatus = "denied"
)

type OperatorActionType string

const (
	OperatorActionApprove    OperatorActionType = "approve"
	OperatorActionDeny       OperatorActionType = "deny"
	OperatorActionResume     OperatorActionType = "resume"
	OperatorActionRetry      OperatorActionType = "retry"
	OperatorActionReevaluate OperatorActionType = "reevaluate"
)

type OperatorActionStatus string

const (
	OperatorActionStatusRequested OperatorActionStatus = "requested"
	OperatorActionStatusApplied   OperatorActionStatus = "applied"
	OperatorActionStatusFailed    OperatorActionStatus = "failed"
)

type ApprovalStateRecord struct {
	ApprovalID      string         `json:"approval_id"`
	GraphID         string         `json:"graph_id"`
	TaskID          string         `json:"task_id"`
	WorkflowID      string         `json:"workflow_id"`
	ExecutionID     string         `json:"execution_id,omitempty"`
	RequestedAction string         `json:"requested_action"`
	RequiredRoles   []string       `json:"required_roles,omitempty"`
	RequestedAt     time.Time      `json:"requested_at"`
	Deadline        *time.Time     `json:"deadline,omitempty"`
	Status          ApprovalStatus `json:"status"`
	ResolvedAt      *time.Time     `json:"resolved_at,omitempty"`
	ResolvedBy      string         `json:"resolved_by,omitempty"`
	ResolutionNote  string         `json:"resolution_note,omitempty"`
	Version         int64          `json:"version"`
}

type OperatorActionRecord struct {
	ActionID        string               `json:"action_id"`
	RequestID       string               `json:"request_id"`
	ActionType      OperatorActionType   `json:"action_type"`
	Actor           string               `json:"actor"`
	Roles           []string             `json:"roles,omitempty"`
	GraphID         string               `json:"graph_id,omitempty"`
	TaskID          string               `json:"task_id,omitempty"`
	ApprovalID      string               `json:"approval_id,omitempty"`
	WorkflowID      string               `json:"workflow_id,omitempty"`
	Status          OperatorActionStatus `json:"status"`
	Note            string               `json:"note,omitempty"`
	RequestedAt     time.Time            `json:"requested_at"`
	AppliedAt       *time.Time           `json:"applied_at,omitempty"`
	FailureSummary  string               `json:"failure_summary,omitempty"`
	ExpectedVersion int64                `json:"expected_version,omitempty"`
}

type ReplayEventRecord struct {
	EventID           string    `json:"event_id"`
	RootCorrelationID string    `json:"root_correlation_id,omitempty"`
	ParentWorkflowID  string    `json:"parent_workflow_id,omitempty"`
	WorkflowID        string    `json:"workflow_id,omitempty"`
	GraphID           string    `json:"graph_id,omitempty"`
	TaskID            string    `json:"task_id,omitempty"`
	ApprovalID        string    `json:"approval_id,omitempty"`
	ExecutionID       string    `json:"execution_id,omitempty"`
	ActionType        string    `json:"action_type"`
	Summary           string    `json:"summary"`
	OccurredAt        time.Time `json:"occurred_at"`
	DetailsJSON       string    `json:"details_json,omitempty"`
	CommittedStateRef string    `json:"committed_state_ref,omitempty"`
	UpdatedStateRef   string    `json:"updated_state_ref,omitempty"`
	ArtifactIDs       []string  `json:"artifact_ids,omitempty"`
	OperatorActionID  string    `json:"operator_action_id,omitempty"`
	CheckpointID      string    `json:"checkpoint_id,omitempty"`
	ResumeToken       string    `json:"resume_token,omitempty"`
}

type CheckpointPayloadKind string

const (
	CheckpointPayloadKindFollowUpFinalizeResume CheckpointPayloadKind = "follow_up_finalize_resume"
)

type FollowUpFinalizeResumePayload struct {
	GraphID                 string                    `json:"graph_id"`
	TaskID                  string                    `json:"task_id"`
	WorkflowID              string                    `json:"workflow_id"`
	ArtifactKind            reporting.ArtifactKind    `json:"artifact_kind"`
	DraftReport             reporting.ReportPayload   `json:"draft_report"`
	DisclosureDecision      governance.PolicyDecision `json:"disclosure_decision"`
	PendingStateSnapshotRef string                    `json:"pending_state_snapshot_ref"`
}

type CheckpointPayloadEnvelope struct {
	Kind                   CheckpointPayloadKind          `json:"kind"`
	FollowUpFinalizeResume *FollowUpFinalizeResumePayload `json:"follow_up_finalize_resume,omitempty"`
}

func (p CheckpointPayloadEnvelope) Validate() error {
	switch p.Kind {
	case CheckpointPayloadKindFollowUpFinalizeResume:
		if p.FollowUpFinalizeResume == nil {
			return errors.New("follow_up_finalize_resume payload is required")
		}
		if p.FollowUpFinalizeResume.GraphID == "" || p.FollowUpFinalizeResume.TaskID == "" || p.FollowUpFinalizeResume.WorkflowID == "" {
			return errors.New("follow_up_finalize_resume requires graph/task/workflow ids")
		}
		if err := p.FollowUpFinalizeResume.DraftReport.Validate(); err != nil {
			return fmt.Errorf("follow_up_finalize_resume draft report invalid: %w", err)
		}
		if p.FollowUpFinalizeResume.PendingStateSnapshotRef == "" {
			return errors.New("follow_up_finalize_resume requires pending state snapshot ref")
		}
		return nil
	default:
		return fmt.Errorf("unsupported checkpoint payload kind %q", p.Kind)
	}
}

type TaskGraphStore interface {
	Save(snapshot TaskGraphSnapshot) error
	Update(snapshot TaskGraphSnapshot, expectedVersion int64) error
	Load(graphID string) (TaskGraphSnapshot, bool, error)
	List() ([]TaskGraphSnapshot, error)
	SaveStateSnapshot(graphID string, workflowID string, taskID string, kind string, snapshot state.StateSnapshot) (string, error)
	LoadStateSnapshot(snapshotRef string) (state.StateSnapshot, bool, error)
}

type TaskExecutionStore interface {
	Save(record TaskExecutionRecord) error
	Update(record TaskExecutionRecord, expectedVersion int64) error
	Load(executionID string) (TaskExecutionRecord, bool, error)
	LoadLatestByTask(graphID string, taskID string) (TaskExecutionRecord, bool, error)
	ListByGraph(graphID string) ([]TaskExecutionRecord, error)
}

type ApprovalStateStore interface {
	Save(record ApprovalStateRecord) error
	Update(record ApprovalStateRecord, expectedVersion int64) error
	Load(approvalID string) (ApprovalStateRecord, bool, error)
	ListPending() ([]ApprovalStateRecord, error)
	LoadByTask(graphID string, taskID string) (ApprovalStateRecord, bool, error)
}

type OperatorActionStore interface {
	Save(record OperatorActionRecord) error
	LoadByRequestID(requestID string) (OperatorActionRecord, bool, error)
	ListByTask(graphID string, taskID string) ([]OperatorActionRecord, error)
}

type CheckpointStore interface {
	Save(checkpoint CheckpointRecord) error
	Load(workflowID string, checkpointID string) (CheckpointRecord, error)
	SaveResumeToken(token ResumeToken) error
	LoadResumeToken(token string) (ResumeToken, error)
	SavePayload(checkpointID string, payload CheckpointPayloadEnvelope) error
	LoadPayload(checkpointID string) (CheckpointPayloadEnvelope, error)
}

type ReplayStore interface {
	Append(event ReplayEventRecord) error
	ListByGraph(graphID string) ([]ReplayEventRecord, error)
	ListByTask(taskID string) ([]ReplayEventRecord, error)
	ListByWorkflow(workflowID string) ([]ReplayEventRecord, error)
}

type ArtifactMetadataStore interface {
	SaveArtifact(workflowID string, taskID string, artifact reporting.WorkflowArtifact) error
	ListArtifactsByTask(taskID string) ([]reporting.WorkflowArtifact, error)
}

type SchemaBootstrapper interface {
	EnsureSchema() error
}

type MigrationRunner interface {
	EnsureSchema() error
}

func snapshotRefFor(stateSnapshot state.StateSnapshot) string {
	return stateSnapshot.State.Version.SnapshotID
}
