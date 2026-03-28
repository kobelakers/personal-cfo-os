package workflows

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type ArtifactKind string

const (
	ArtifactKindMonthlyReviewReport ArtifactKind = "monthly_review_report"
	ArtifactKindDebtDecisionReport  ArtifactKind = "debt_decision_report"
	ArtifactKindWorkflowTimeline    ArtifactKind = "workflow_timeline"
	ArtifactKindVerificationReport  ArtifactKind = "verification_report"
	ArtifactKindCheckpointDump      ArtifactKind = "checkpoint_dump"
	ArtifactKindApprovalRequest     ArtifactKind = "approval_request"
)

type ArtifactRef struct {
	Kind     ArtifactKind `json:"kind"`
	ID       string       `json:"id"`
	Location string       `json:"location,omitempty"`
	Summary  string       `json:"summary,omitempty"`
}

type WorkflowArtifact struct {
	ID          string       `json:"id"`
	WorkflowID  string       `json:"workflow_id"`
	TaskID      string       `json:"task_id"`
	Kind        ArtifactKind `json:"kind"`
	ProducedBy  string       `json:"produced_by"`
	ConsumedBy  []string     `json:"consumed_by,omitempty"`
	Ref         ArtifactRef  `json:"ref"`
	ContentJSON string       `json:"content_json"`
	CreatedAt   time.Time    `json:"created_at"`
}

type ArtifactProducer interface {
	ProduceArtifact(workflowID string, taskID string, kind ArtifactKind, contentJSON string, summary string, producedBy string) WorkflowArtifact
}

type ArtifactConsumer interface {
	ConsumeArtifact(artifact WorkflowArtifact) error
}

type MonthlyReviewReport struct {
	TaskID                  string             `json:"task_id"`
	WorkflowID              string             `json:"workflow_id"`
	Summary                 string             `json:"summary"`
	CashflowMetrics         map[string]any     `json:"cashflow_metrics"`
	TaxSignals              map[string]any     `json:"tax_signals"`
	RiskItems               []skills.SkillItem `json:"risk_items"`
	OptimizationSuggestions []skills.SkillItem `json:"optimization_suggestions"`
	TodoItems               []skills.SkillItem `json:"todo_items"`
	ApprovalRequired        bool               `json:"approval_required"`
	Confidence              float64            `json:"confidence"`
	GeneratedAt             time.Time          `json:"generated_at"`
}

type DebtDecisionReport struct {
	TaskID           string                   `json:"task_id"`
	WorkflowID       string                   `json:"workflow_id"`
	Conclusion       string                   `json:"conclusion"`
	Reasons          []string                 `json:"reasons"`
	Actions          []skills.SkillItem       `json:"actions"`
	Metrics          map[string]any           `json:"metrics"`
	EvidenceIDs      []observation.EvidenceID `json:"evidence_ids"`
	ApprovalRequired bool                     `json:"approval_required"`
	Confidence       float64                  `json:"confidence"`
	GeneratedAt      time.Time                `json:"generated_at"`
}

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

type StaticArtifactProducer struct {
	Now func() time.Time
}

func (p StaticArtifactProducer) ProduceArtifact(workflowID string, taskID string, kind ArtifactKind, contentJSON string, summary string, producedBy string) WorkflowArtifact {
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now().UTC()
	}
	id := workflowID + "-" + string(kind)
	return WorkflowArtifact{
		ID:          id,
		WorkflowID:  workflowID,
		TaskID:      taskID,
		Kind:        kind,
		ProducedBy:  producedBy,
		Ref:         ArtifactRef{Kind: kind, ID: id, Summary: summary},
		ContentJSON: contentJSON,
		CreatedAt:   now,
	}
}
