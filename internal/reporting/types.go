package reporting

import (
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
)

type ArtifactKind string

const (
	ArtifactKindMonthlyReviewReport ArtifactKind = "monthly_review_report"
	ArtifactKindDebtDecisionReport  ArtifactKind = "debt_decision_report"
	ArtifactKindLifeEventAssessment ArtifactKind = "life_event_assessment_report"
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
	TaskID                  string                   `json:"task_id"`
	WorkflowID              string                   `json:"workflow_id"`
	Summary                 string                   `json:"summary"`
	CashflowMetrics         map[string]any           `json:"cashflow_metrics"`
	TaxSignals              map[string]any           `json:"tax_signals"`
	RiskItems               []skills.SkillItem       `json:"risk_items"`
	OptimizationSuggestions []skills.SkillItem       `json:"optimization_suggestions"`
	TodoItems               []skills.SkillItem       `json:"todo_items"`
	SourceBlockIDs          []string                 `json:"source_block_ids,omitempty"`
	SourceMemoryIDs         []string                 `json:"source_memory_ids,omitempty"`
	SourceEvidenceIDs       []observation.EvidenceID `json:"source_evidence_ids,omitempty"`
	ApprovalRequired        bool                     `json:"approval_required"`
	Confidence              float64                  `json:"confidence"`
	GeneratedAt             time.Time                `json:"generated_at"`
}

type DebtDecisionReport struct {
	TaskID            string                   `json:"task_id"`
	WorkflowID        string                   `json:"workflow_id"`
	Conclusion        string                   `json:"conclusion"`
	Reasons           []string                 `json:"reasons"`
	Actions           []skills.SkillItem       `json:"actions"`
	Metrics           map[string]any           `json:"metrics"`
	EvidenceIDs       []observation.EvidenceID `json:"evidence_ids"`
	SourceBlockIDs    []string                 `json:"source_block_ids,omitempty"`
	SourceMemoryIDs   []string                 `json:"source_memory_ids,omitempty"`
	SourceEvidenceIDs []observation.EvidenceID `json:"source_evidence_ids,omitempty"`
	ApprovalRequired  bool                     `json:"approval_required"`
	Confidence        float64                  `json:"confidence"`
	GeneratedAt       time.Time                `json:"generated_at"`
}

type LifeEventAssessmentReport struct {
	TaskID                string                   `json:"task_id"`
	WorkflowID            string                   `json:"workflow_id"`
	EventSummary          string                   `json:"event_summary"`
	StateDiffSummary      []string                 `json:"state_diff_summary,omitempty"`
	MemoryUpdateSummary   []string                 `json:"memory_update_summary,omitempty"`
	GeneratedTaskIDs      []string                 `json:"generated_task_ids,omitempty"`
	GeneratedTaskStatuses map[string]string        `json:"generated_task_statuses,omitempty"`
	RequiredCapabilities  map[string]string        `json:"required_capabilities,omitempty"`
	MissingCapabilities   map[string]string        `json:"missing_capabilities,omitempty"`
	SourceBlockIDs        []string                 `json:"source_block_ids,omitempty"`
	SourceMemoryIDs       []string                 `json:"source_memory_ids,omitempty"`
	SourceEvidenceIDs     []observation.EvidenceID `json:"source_evidence_ids,omitempty"`
	VerificationNotes     []string                 `json:"verification_notes,omitempty"`
	GovernanceNotes       []string                 `json:"governance_notes,omitempty"`
	GeneratedAt           time.Time                `json:"generated_at"`
}

type ReportPayload struct {
	MonthlyReview       *MonthlyReviewReport       `json:"monthly_review,omitempty"`
	DebtDecision        *DebtDecisionReport        `json:"debt_decision,omitempty"`
	LifeEventAssessment *LifeEventAssessmentReport `json:"life_event_assessment,omitempty"`
}

func (p ReportPayload) Validate() error {
	count := 0
	if p.MonthlyReview != nil {
		count++
	}
	if p.DebtDecision != nil {
		count++
	}
	if p.LifeEventAssessment != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("report payload must set exactly one typed report, got %d", count)
	}
	return nil
}

func (p ReportPayload) Summary() string {
	switch {
	case p.MonthlyReview != nil:
		return p.MonthlyReview.Summary
	case p.DebtDecision != nil:
		return p.DebtDecision.Conclusion
	case p.LifeEventAssessment != nil:
		return p.LifeEventAssessment.EventSummary
	default:
		return ""
	}
}

func (p ReportPayload) ArtifactKind() ArtifactKind {
	switch {
	case p.MonthlyReview != nil:
		return ArtifactKindMonthlyReviewReport
	case p.DebtDecision != nil:
		return ArtifactKindDebtDecisionReport
	case p.LifeEventAssessment != nil:
		return ArtifactKindLifeEventAssessment
	default:
		return ""
	}
}

func (p ReportPayload) ProducedAt() time.Time {
	switch {
	case p.MonthlyReview != nil:
		return p.MonthlyReview.GeneratedAt
	case p.DebtDecision != nil:
		return p.DebtDecision.GeneratedAt
	case p.LifeEventAssessment != nil:
		return p.LifeEventAssessment.GeneratedAt
	default:
		return time.Time{}
	}
}

func (p ReportPayload) WorkflowID() string {
	switch {
	case p.MonthlyReview != nil:
		return p.MonthlyReview.WorkflowID
	case p.DebtDecision != nil:
		return p.DebtDecision.WorkflowID
	case p.LifeEventAssessment != nil:
		return p.LifeEventAssessment.WorkflowID
	default:
		return ""
	}
}
