package workflows

import (
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type ArtifactKind = reporting.ArtifactKind

const (
	ArtifactKindMonthlyReviewReport = reporting.ArtifactKindMonthlyReviewReport
	ArtifactKindDebtDecisionReport  = reporting.ArtifactKindDebtDecisionReport
	ArtifactKindWorkflowTimeline    = reporting.ArtifactKindWorkflowTimeline
	ArtifactKindVerificationReport  = reporting.ArtifactKindVerificationReport
	ArtifactKindCheckpointDump      = reporting.ArtifactKindCheckpointDump
	ArtifactKindApprovalRequest     = reporting.ArtifactKindApprovalRequest
)

type ArtifactRef = reporting.ArtifactRef
type WorkflowArtifact = reporting.WorkflowArtifact
type ArtifactProducer = reporting.ArtifactProducer
type ArtifactConsumer = reporting.ArtifactConsumer
type ArtifactService = reporting.ArtifactService
type StaticArtifactProducer = reporting.StaticArtifactProducer
type MonthlyReviewReport = reporting.MonthlyReviewReport
type DebtDecisionReport = reporting.DebtDecisionReport

type MonthlyReviewRunResult struct {
	WorkflowID        string                              `json:"workflow_id"`
	Intake            taskspec.TaskIntakeResult           `json:"intake"`
	TaskSpec          taskspec.TaskSpec                   `json:"task_spec"`
	Plan              planning.ExecutionPlan              `json:"plan"`
	Evidence          []observation.EvidenceRecord        `json:"evidence"`
	UpdatedState      state.FinancialWorldState           `json:"updated_state"`
	Report            MonthlyReviewReport                 `json:"report"`
	Artifacts         []WorkflowArtifact                  `json:"artifacts"`
	GeneratedMemories []string                            `json:"generated_memories,omitempty"`
	CoverageReport    verification.EvidenceCoverageReport `json:"coverage_report"`
	Verification      []verification.VerificationResult   `json:"verification"`
	Oracle            verification.OracleVerdict          `json:"oracle"`
	RiskAssessment    governance.RiskAssessment           `json:"risk_assessment"`
	ApprovalDecision  *governance.PolicyDecision          `json:"approval_decision,omitempty"`
	ApprovalAudit     *governance.AuditEvent              `json:"approval_audit,omitempty"`
	RuntimeState      runtime.WorkflowExecutionState      `json:"runtime_state"`
}

type DebtDecisionRunResult struct {
	WorkflowID       string                              `json:"workflow_id"`
	Intake           taskspec.TaskIntakeResult           `json:"intake"`
	TaskSpec         taskspec.TaskSpec                   `json:"task_spec"`
	Plan             planning.ExecutionPlan              `json:"plan"`
	Evidence         []observation.EvidenceRecord        `json:"evidence"`
	UpdatedState     state.FinancialWorldState           `json:"updated_state"`
	Report           DebtDecisionReport                  `json:"report"`
	Artifacts        []WorkflowArtifact                  `json:"artifacts"`
	CoverageReport   verification.EvidenceCoverageReport `json:"coverage_report"`
	Verification     []verification.VerificationResult   `json:"verification"`
	Oracle           verification.OracleVerdict          `json:"oracle"`
	RiskAssessment   governance.RiskAssessment           `json:"risk_assessment"`
	ApprovalDecision *governance.PolicyDecision          `json:"approval_decision,omitempty"`
	ApprovalAudit    *governance.AuditEvent              `json:"approval_audit,omitempty"`
	RuntimeState     runtime.WorkflowExecutionState      `json:"runtime_state"`
}
