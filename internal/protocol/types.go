package protocol

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type ProtocolMetadata struct {
	MessageID     string    `json:"message_id"`
	CorrelationID string    `json:"correlation_id"`
	CausationID   string    `json:"causation_id"`
	EmittedAt     time.Time `json:"emitted_at"`
}

type StateReference struct {
	UserID     string `json:"user_id"`
	SnapshotID string `json:"snapshot_id"`
	Version    uint64 `json:"version"`
}

type AgentEnvelope struct {
	Metadata         ProtocolMetadata               `json:"metadata"`
	Sender           string                         `json:"sender"`
	Recipient        string                         `json:"recipient"`
	Task             taskspec.TaskSpec              `json:"task"`
	StateRef         StateReference                 `json:"state_ref"`
	RequiredEvidence []taskspec.RequiredEvidenceRef `json:"required_evidence"`
	Deadline         *time.Time                     `json:"deadline,omitempty"`
	RiskLevel        taskspec.RiskLevel             `json:"risk_level"`
}

type WorkflowEventType string

const (
	WorkflowEventPlanCreated        WorkflowEventType = "plan_created"
	WorkflowEventStateUpdated       WorkflowEventType = "state_updated"
	WorkflowEventToolCalled         WorkflowEventType = "tool_called"
	WorkflowEventApprovalRequired   WorkflowEventType = "approval_required"
	WorkflowEventVerificationFailed WorkflowEventType = "verification_failed"
	WorkflowEventReportReady        WorkflowEventType = "report_ready"
)

type WorkflowEvent struct {
	Metadata   ProtocolMetadata  `json:"metadata"`
	WorkflowID string            `json:"workflow_id"`
	TaskID     string            `json:"task_id"`
	Type       WorkflowEventType `json:"type"`
	Summary    string            `json:"summary"`
	StateRef   StateReference    `json:"state_ref"`
	Details    map[string]string `json:"details,omitempty"`
}
