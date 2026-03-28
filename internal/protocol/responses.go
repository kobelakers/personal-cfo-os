package protocol

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type AgentFailureCategory string

const (
	AgentFailureValidation         AgentFailureCategory = "validation"
	AgentFailurePolicy             AgentFailureCategory = "policy"
	AgentFailureTransient          AgentFailureCategory = "transient"
	AgentFailureUnrecoverable      AgentFailureCategory = "unrecoverable"
	AgentFailureUnsupportedMessage AgentFailureCategory = "unsupported_message"
	AgentFailureBadPayload         AgentFailureCategory = "bad_payload"
)

type AgentFailure struct {
	Category AgentFailureCategory `json:"category"`
	Message  string               `json:"message"`
	Details  map[string]string    `json:"details,omitempty"`
}

type CategorizedAgentError interface {
	error
	AgentFailure() AgentFailure
}

type AgentResponse struct {
	Metadata              ProtocolMetadata                  `json:"metadata"`
	Sender                string                            `json:"sender"`
	Recipient             string                            `json:"recipient"`
	Task                  taskspec.TaskSpec                 `json:"task"`
	StateRef              StateReference                    `json:"state_ref"`
	RequiredEvidence      []taskspec.RequiredEvidenceRef    `json:"required_evidence"`
	Deadline              *time.Time                        `json:"deadline,omitempty"`
	RiskLevel             taskspec.RiskLevel                `json:"risk_level"`
	Kind                  MessageKind                       `json:"kind"`
	Success               bool                              `json:"success"`
	Failure               *AgentFailure                     `json:"failure,omitempty"`
	Body                  AgentResultBody                   `json:"body,omitempty"`
	ProducedArtifacts     []reporting.WorkflowArtifact      `json:"produced_artifacts,omitempty"`
	ProducedMemories      []string                          `json:"produced_memories,omitempty"`
	VerificationResults   []verification.VerificationResult `json:"verification_results,omitempty"`
	EmittedWorkflowEvents []WorkflowEvent                   `json:"emitted_workflow_events,omitempty"`
}
