package workflows

import (
	"github.com/kobelakers/personal-cfo-os/internal/analysis"
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
	ArtifactKindMonthlyReviewReport      = reporting.ArtifactKindMonthlyReviewReport
	ArtifactKindDebtDecisionReport       = reporting.ArtifactKindDebtDecisionReport
	ArtifactKindLifeEventAssessment      = reporting.ArtifactKindLifeEventAssessment
	ArtifactKindTaxOptimizationReport    = reporting.ArtifactKindTaxOptimizationReport
	ArtifactKindPortfolioRebalanceReport = reporting.ArtifactKindPortfolioRebalanceReport
	ArtifactKindWorkflowTimeline         = reporting.ArtifactKindWorkflowTimeline
	ArtifactKindVerificationReport       = reporting.ArtifactKindVerificationReport
	ArtifactKindCheckpointDump           = reporting.ArtifactKindCheckpointDump
	ArtifactKindApprovalRequest          = reporting.ArtifactKindApprovalRequest
)

type ArtifactRef = reporting.ArtifactRef
type WorkflowArtifact = reporting.WorkflowArtifact
type ArtifactProducer = reporting.ArtifactProducer
type ArtifactConsumer = reporting.ArtifactConsumer
type ArtifactService = reporting.ArtifactService
type StaticArtifactProducer = reporting.StaticArtifactProducer
type MonthlyReviewReport = reporting.MonthlyReviewReport
type DebtDecisionReport = reporting.DebtDecisionReport
type LifeEventAssessmentReport = reporting.LifeEventAssessmentReport
type TaxOptimizationReport = reporting.TaxOptimizationReport
type PortfolioRebalanceReport = reporting.PortfolioRebalanceReport

type MonthlyReviewRunResult struct {
	WorkflowID        string                              `json:"workflow_id"`
	Intake            taskspec.TaskIntakeResult           `json:"intake"`
	TaskSpec          taskspec.TaskSpec                   `json:"task_spec"`
	Plan              planning.ExecutionPlan              `json:"plan"`
	Evidence          []observation.EvidenceRecord        `json:"evidence"`
	BlockResults      []analysis.BlockResultEnvelope      `json:"block_results,omitempty"`
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
	WorkflowID         string                              `json:"workflow_id"`
	Intake             taskspec.TaskIntakeResult           `json:"intake"`
	TaskSpec           taskspec.TaskSpec                   `json:"task_spec"`
	Plan               planning.ExecutionPlan              `json:"plan"`
	Evidence           []observation.EvidenceRecord        `json:"evidence"`
	BlockResults       []analysis.BlockResultEnvelope      `json:"block_results,omitempty"`
	UpdatedState       state.FinancialWorldState           `json:"updated_state"`
	DraftPayload       reporting.ReportPayload             `json:"draft_payload"`
	DisclosureDecision governance.PolicyDecision           `json:"disclosure_decision"`
	Report             DebtDecisionReport                  `json:"report"`
	Artifacts          []WorkflowArtifact                  `json:"artifacts"`
	CoverageReport     verification.EvidenceCoverageReport `json:"coverage_report"`
	Verification       []verification.VerificationResult   `json:"verification"`
	Oracle             verification.OracleVerdict          `json:"oracle"`
	RiskAssessment     governance.RiskAssessment           `json:"risk_assessment"`
	ApprovalDecision   *governance.PolicyDecision          `json:"approval_decision,omitempty"`
	ApprovalAudit      *governance.AuditEvent              `json:"approval_audit,omitempty"`
	Checkpoint         *runtime.CheckpointRecord           `json:"checkpoint,omitempty"`
	ResumeToken        *runtime.ResumeToken                `json:"resume_token,omitempty"`
	PendingApproval    *runtime.HumanApprovalPending       `json:"pending_approval,omitempty"`
	RuntimeState       runtime.WorkflowExecutionState      `json:"runtime_state"`
}

type LifeEventTriggerRunResult struct {
	WorkflowID           string                               `json:"workflow_id"`
	Intake               taskspec.TaskIntakeResult            `json:"intake"`
	TaskSpec             taskspec.TaskSpec                    `json:"task_spec"`
	Plan                 planning.ExecutionPlan               `json:"plan"`
	EventEvidence        []observation.EvidenceRecord         `json:"event_evidence,omitempty"`
	DeadlineEvidence     []observation.EvidenceRecord         `json:"deadline_evidence,omitempty"`
	Evidence             []observation.EvidenceRecord         `json:"evidence"`
	BlockResults         []analysis.BlockResultEnvelope       `json:"block_results,omitempty"`
	UpdatedState         state.FinancialWorldState            `json:"updated_state"`
	StateDiff            state.StateDiff                      `json:"state_diff"`
	GeneratedMemories    []string                             `json:"generated_memories,omitempty"`
	TaskGraph            taskspec.TaskGraph                   `json:"task_graph"`
	FollowUpTasks        runtime.FollowUpRegistrationResult   `json:"follow_up_tasks"`
	FollowUpExecution    runtime.FollowUpExecutionBatchResult `json:"follow_up_execution,omitempty"`
	Report               LifeEventAssessmentReport            `json:"report"`
	Artifacts            []WorkflowArtifact                   `json:"artifacts,omitempty"`
	CoverageReport       verification.EvidenceCoverageReport  `json:"coverage_report"`
	AnalysisVerification []verification.VerificationResult    `json:"analysis_verification,omitempty"`
	FinalVerification    []verification.VerificationResult    `json:"final_verification,omitempty"`
	Oracle               verification.OracleVerdict           `json:"oracle"`
	RiskAssessment       governance.RiskAssessment            `json:"risk_assessment"`
	ApprovalDecision     *governance.PolicyDecision           `json:"approval_decision,omitempty"`
	ApprovalAudit        *governance.AuditEvent               `json:"approval_audit,omitempty"`
	RuntimeState         runtime.WorkflowExecutionState       `json:"runtime_state"`
}

type TaxOptimizationRunResult struct {
	WorkflowID         string                              `json:"workflow_id"`
	TaskSpec           taskspec.TaskSpec                   `json:"task_spec"`
	Plan               planning.ExecutionPlan              `json:"plan"`
	Evidence           []observation.EvidenceRecord        `json:"evidence"`
	BlockResults       []analysis.BlockResultEnvelope      `json:"block_results,omitempty"`
	UpdatedState       state.FinancialWorldState           `json:"updated_state"`
	DraftPayload       reporting.ReportPayload             `json:"draft_payload"`
	DisclosureDecision governance.PolicyDecision           `json:"disclosure_decision"`
	Report             TaxOptimizationReport               `json:"report"`
	Artifacts          []WorkflowArtifact                  `json:"artifacts,omitempty"`
	CoverageReport     verification.EvidenceCoverageReport `json:"coverage_report"`
	Verification       []verification.VerificationResult   `json:"verification"`
	Oracle             verification.OracleVerdict          `json:"oracle"`
	RiskAssessment     governance.RiskAssessment           `json:"risk_assessment"`
	ApprovalDecision   *governance.PolicyDecision          `json:"approval_decision,omitempty"`
	ApprovalAudit      *governance.AuditEvent              `json:"approval_audit,omitempty"`
	Checkpoint         *runtime.CheckpointRecord           `json:"checkpoint,omitempty"`
	ResumeToken        *runtime.ResumeToken                `json:"resume_token,omitempty"`
	PendingApproval    *runtime.HumanApprovalPending       `json:"pending_approval,omitempty"`
	RuntimeState       runtime.WorkflowExecutionState      `json:"runtime_state"`
}

type PortfolioRebalanceRunResult struct {
	WorkflowID         string                              `json:"workflow_id"`
	TaskSpec           taskspec.TaskSpec                   `json:"task_spec"`
	Plan               planning.ExecutionPlan              `json:"plan"`
	Evidence           []observation.EvidenceRecord        `json:"evidence"`
	BlockResults       []analysis.BlockResultEnvelope      `json:"block_results,omitempty"`
	UpdatedState       state.FinancialWorldState           `json:"updated_state"`
	DraftPayload       reporting.ReportPayload             `json:"draft_payload"`
	DisclosureDecision governance.PolicyDecision           `json:"disclosure_decision"`
	Report             PortfolioRebalanceReport            `json:"report"`
	Artifacts          []WorkflowArtifact                  `json:"artifacts,omitempty"`
	CoverageReport     verification.EvidenceCoverageReport `json:"coverage_report"`
	Verification       []verification.VerificationResult   `json:"verification"`
	Oracle             verification.OracleVerdict          `json:"oracle"`
	RiskAssessment     governance.RiskAssessment           `json:"risk_assessment"`
	ApprovalDecision   *governance.PolicyDecision          `json:"approval_decision,omitempty"`
	ApprovalAudit      *governance.AuditEvent              `json:"approval_audit,omitempty"`
	Checkpoint         *runtime.CheckpointRecord           `json:"checkpoint,omitempty"`
	ResumeToken        *runtime.ResumeToken                `json:"resume_token,omitempty"`
	PendingApproval    *runtime.HumanApprovalPending       `json:"pending_approval,omitempty"`
	RuntimeState       runtime.WorkflowExecutionState      `json:"runtime_state"`
}
