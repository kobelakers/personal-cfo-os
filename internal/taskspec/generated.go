package taskspec

import (
	"errors"
	"time"
)

type TaskTriggerSource string

const (
	TaskTriggerSourceLifeEvent  TaskTriggerSource = "life_event"
	TaskTriggerSourceStateDiff  TaskTriggerSource = "state_diff"
	TaskTriggerSourceMemory     TaskTriggerSource = "memory"
	TaskTriggerSourceDeadline   TaskTriggerSource = "calendar_deadline"
	TaskTriggerSourcePolicyHint TaskTriggerSource = "policy_hint"
)

type TaskGenerationReasonCode string

const (
	TaskGenerationReasonLifeEventImpact  TaskGenerationReasonCode = "life_event_impact"
	TaskGenerationReasonStateDiff        TaskGenerationReasonCode = "state_diff"
	TaskGenerationReasonRetrievedMemory  TaskGenerationReasonCode = "retrieved_memory"
	TaskGenerationReasonCalendarDeadline TaskGenerationReasonCode = "calendar_deadline"
)

type TaskPriority string

const (
	TaskPriorityLow      TaskPriority = "low"
	TaskPriorityMedium   TaskPriority = "medium"
	TaskPriorityHigh     TaskPriority = "high"
	TaskPriorityCritical TaskPriority = "critical"
)

type TaskDueWindow struct {
	NotBefore *time.Time `json:"not_before,omitempty"`
	NotAfter  *time.Time `json:"not_after,omitempty"`
}

type TaskGenerationReason struct {
	Code             TaskGenerationReasonCode `json:"code"`
	Description      string                   `json:"description"`
	LifeEventID      string                   `json:"life_event_id,omitempty"`
	LifeEventKind    string                   `json:"life_event_kind,omitempty"`
	EvidenceIDs      []string                 `json:"evidence_ids,omitempty"`
	MemoryIDs        []string                 `json:"memory_ids,omitempty"`
	StateDiffFields  []string                 `json:"state_diff_fields,omitempty"`
	DeadlineEvidence []string                 `json:"deadline_evidence,omitempty"`
}

type GeneratedTaskMetadata struct {
	GeneratedAt       time.Time              `json:"generated_at"`
	ParentWorkflowID  string                 `json:"parent_workflow_id"`
	ParentTaskID      string                 `json:"parent_task_id"`
	TriggerSource     TaskTriggerSource      `json:"trigger_source"`
	Priority          TaskPriority           `json:"priority"`
	DueWindow         TaskDueWindow          `json:"due_window"`
	RequiresApproval  bool                   `json:"requires_approval"`
	GenerationReasons []TaskGenerationReason `json:"generation_reasons,omitempty"`
}

// GeneratedTaskSpec keeps TaskSpec as the only executable goal contract.
// The wrapper only adds provenance and scheduling metadata for spawned tasks.
type GeneratedTaskSpec struct {
	Task     TaskSpec              `json:"task"`
	Metadata GeneratedTaskMetadata `json:"metadata"`
}

func (g GeneratedTaskSpec) Validate() error {
	if err := g.Task.Validate(); err != nil {
		return err
	}
	if g.Metadata.GeneratedAt.IsZero() {
		return errMissingGeneratedAt
	}
	if g.Metadata.ParentWorkflowID == "" {
		return errMissingParentWorkflowID
	}
	if g.Metadata.ParentTaskID == "" {
		return errMissingParentTaskID
	}
	if !validTaskTriggerSource(g.Metadata.TriggerSource) {
		return errMissingTriggerSource
	}
	if !validTaskPriority(g.Metadata.Priority) {
		return errInvalidTaskPriority
	}
	if g.Metadata.DueWindow.NotBefore != nil && g.Metadata.DueWindow.NotAfter != nil && g.Metadata.DueWindow.NotBefore.After(*g.Metadata.DueWindow.NotAfter) {
		return errInvalidDueWindow
	}
	return nil
}

func (g GeneratedTaskSpec) ToTaskSpec() TaskSpec {
	return g.Task
}

var (
	errMissingGeneratedAt      = errors.New("generated task metadata generated_at is required")
	errMissingParentWorkflowID = errors.New("generated task metadata parent_workflow_id is required")
	errMissingParentTaskID     = errors.New("generated task metadata parent_task_id is required")
	errMissingTriggerSource    = errors.New("generated task metadata trigger_source is required")
	errInvalidTaskPriority     = errors.New("generated task metadata priority is invalid")
	errInvalidDueWindow        = errors.New("generated task metadata due_window not_before must be before not_after")
)

func validTaskPriority(priority TaskPriority) bool {
	switch priority {
	case TaskPriorityLow, TaskPriorityMedium, TaskPriorityHigh, TaskPriorityCritical:
		return true
	default:
		return false
	}
}

func validTaskTriggerSource(source TaskTriggerSource) bool {
	switch source {
	case TaskTriggerSourceLifeEvent, TaskTriggerSourceStateDiff, TaskTriggerSourceMemory, TaskTriggerSourceDeadline, TaskTriggerSourcePolicyHint:
		return true
	default:
		return false
	}
}
