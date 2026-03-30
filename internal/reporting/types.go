package reporting

import (
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
)

type ArtifactKind string

const (
	ArtifactKindMonthlyReviewReport      ArtifactKind = "monthly_review_report"
	ArtifactKindDebtDecisionReport       ArtifactKind = "debt_decision_report"
	ArtifactKindLifeEventAssessment      ArtifactKind = "life_event_assessment_report"
	ArtifactKindTaxOptimizationReport    ArtifactKind = "tax_optimization_report"
	ArtifactKindPortfolioRebalanceReport ArtifactKind = "portfolio_rebalance_report"
	ArtifactKindWorkflowTimeline         ArtifactKind = "workflow_timeline"
	ArtifactKindVerificationReport       ArtifactKind = "verification_report"
	ArtifactKindCheckpointDump           ArtifactKind = "checkpoint_dump"
	ArtifactKindApprovalRequest          ArtifactKind = "approval_request"
	ArtifactKindReplayBundle            ArtifactKind = "replay_bundle"
	ArtifactKindReplaySummary           ArtifactKind = "replay_summary"
	ArtifactKindEvalRunResult           ArtifactKind = "eval_run_result"
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
	TaskID                  string                    `json:"task_id"`
	WorkflowID              string                    `json:"workflow_id"`
	Summary                 string                    `json:"summary"`
	CashflowMetrics         map[string]any            `json:"cashflow_metrics"`
	TaxSignals              map[string]any            `json:"tax_signals"`
	MetricRecords           []finance.MetricRecord    `json:"metric_records,omitempty"`
	RiskItems               []skills.SkillItem        `json:"risk_items"`
	RiskFlags               []analysis.RiskFlag       `json:"risk_flags,omitempty"`
	OptimizationSuggestions []skills.SkillItem        `json:"optimization_suggestions"`
	Recommendations         []analysis.Recommendation `json:"recommendations,omitempty"`
	TodoItems               []skills.SkillItem        `json:"todo_items"`
	SourceBlockIDs          []string                  `json:"source_block_ids,omitempty"`
	SourceMemoryIDs         []string                  `json:"source_memory_ids,omitempty"`
	SourceEvidenceIDs       []observation.EvidenceID  `json:"source_evidence_ids,omitempty"`
	GroundingRefs           []string                  `json:"grounding_refs,omitempty"`
	Caveats                 []string                  `json:"caveats,omitempty"`
	ApprovalRequired        bool                      `json:"approval_required"`
	ApprovalReason          string                    `json:"approval_reason,omitempty"`
	PolicyRuleRefs          []string                  `json:"policy_rule_refs,omitempty"`
	Confidence              float64                   `json:"confidence"`
	GeneratedAt             time.Time                 `json:"generated_at"`
}

type DebtDecisionReport struct {
	TaskID            string                    `json:"task_id"`
	WorkflowID        string                    `json:"workflow_id"`
	Conclusion        string                    `json:"conclusion"`
	Reasons           []string                  `json:"reasons"`
	Actions           []skills.SkillItem        `json:"actions"`
	Recommendations   []analysis.Recommendation `json:"recommendations,omitempty"`
	RiskFlags         []analysis.RiskFlag       `json:"risk_flags,omitempty"`
	Metrics           map[string]any            `json:"metrics"`
	MetricRecords     []finance.MetricRecord    `json:"metric_records,omitempty"`
	EvidenceIDs       []observation.EvidenceID  `json:"evidence_ids"`
	SourceBlockIDs    []string                  `json:"source_block_ids,omitempty"`
	SourceMemoryIDs   []string                  `json:"source_memory_ids,omitempty"`
	SourceEvidenceIDs []observation.EvidenceID  `json:"source_evidence_ids,omitempty"`
	GroundingRefs     []string                  `json:"grounding_refs,omitempty"`
	Caveats           []string                  `json:"caveats,omitempty"`
	ApprovalRequired  bool                      `json:"approval_required"`
	ApprovalReason    string                    `json:"approval_reason,omitempty"`
	PolicyRuleRefs    []string                  `json:"policy_rule_refs,omitempty"`
	Confidence        float64                   `json:"confidence"`
	GeneratedAt       time.Time                 `json:"generated_at"`
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

type TaxOptimizationReport struct {
	TaskID               string                    `json:"task_id"`
	WorkflowID           string                    `json:"workflow_id"`
	Summary              string                    `json:"summary"`
	DeterministicMetrics map[string]any            `json:"deterministic_metrics"`
	RecommendedActions   []skills.SkillItem        `json:"recommended_actions"`
	Recommendations      []analysis.Recommendation `json:"recommendations,omitempty"`
	MetricRecords        []finance.MetricRecord    `json:"metric_records,omitempty"`
	SourceBlockIDs       []string                  `json:"source_block_ids,omitempty"`
	SourceMemoryIDs      []string                  `json:"source_memory_ids,omitempty"`
	SourceEvidenceIDs    []observation.EvidenceID  `json:"source_evidence_ids,omitempty"`
	RiskFlags            []analysis.RiskFlag       `json:"risk_flags"`
	GroundingRefs        []string                  `json:"grounding_refs,omitempty"`
	Caveats              []string                  `json:"caveats,omitempty"`
	ApprovalRequired     bool                      `json:"approval_required"`
	ApprovalReason       string                    `json:"approval_reason,omitempty"`
	PolicyRuleRefs       []string                  `json:"policy_rule_refs,omitempty"`
	Confidence           float64                   `json:"confidence"`
	GeneratedAt          time.Time                 `json:"generated_at"`
}

type PortfolioRebalanceReport struct {
	TaskID               string                    `json:"task_id"`
	WorkflowID           string                    `json:"workflow_id"`
	Summary              string                    `json:"summary"`
	DeterministicMetrics map[string]any            `json:"deterministic_metrics"`
	RecommendedActions   []skills.SkillItem        `json:"recommended_actions"`
	Recommendations      []analysis.Recommendation `json:"recommendations,omitempty"`
	MetricRecords        []finance.MetricRecord    `json:"metric_records,omitempty"`
	SourceBlockIDs       []string                  `json:"source_block_ids,omitempty"`
	SourceMemoryIDs      []string                  `json:"source_memory_ids,omitempty"`
	SourceEvidenceIDs    []observation.EvidenceID  `json:"source_evidence_ids,omitempty"`
	RiskFlags            []analysis.RiskFlag       `json:"risk_flags"`
	GroundingRefs        []string                  `json:"grounding_refs,omitempty"`
	Caveats              []string                  `json:"caveats,omitempty"`
	ApprovalRequired     bool                      `json:"approval_required"`
	ApprovalReason       string                    `json:"approval_reason,omitempty"`
	PolicyRuleRefs       []string                  `json:"policy_rule_refs,omitempty"`
	Confidence           float64                   `json:"confidence"`
	GeneratedAt          time.Time                 `json:"generated_at"`
}

type ReportPayload struct {
	MonthlyReview       *MonthlyReviewReport       `json:"monthly_review,omitempty"`
	DebtDecision        *DebtDecisionReport        `json:"debt_decision,omitempty"`
	LifeEventAssessment *LifeEventAssessmentReport `json:"life_event_assessment,omitempty"`
	TaxOptimization     *TaxOptimizationReport     `json:"tax_optimization,omitempty"`
	PortfolioRebalance  *PortfolioRebalanceReport  `json:"portfolio_rebalance,omitempty"`
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
	if p.TaxOptimization != nil {
		count++
	}
	if p.PortfolioRebalance != nil {
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
	case p.TaxOptimization != nil:
		return p.TaxOptimization.Summary
	case p.PortfolioRebalance != nil:
		return p.PortfolioRebalance.Summary
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
	case p.TaxOptimization != nil:
		return ArtifactKindTaxOptimizationReport
	case p.PortfolioRebalance != nil:
		return ArtifactKindPortfolioRebalanceReport
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
	case p.TaxOptimization != nil:
		return p.TaxOptimization.GeneratedAt
	case p.PortfolioRebalance != nil:
		return p.PortfolioRebalance.GeneratedAt
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
	case p.TaxOptimization != nil:
		return p.TaxOptimization.WorkflowID
	case p.PortfolioRebalance != nil:
		return p.PortfolioRebalance.WorkflowID
	default:
		return ""
	}
}

func (p ReportPayload) Recommendations() []analysis.Recommendation {
	switch {
	case p.MonthlyReview != nil:
		return append([]analysis.Recommendation{}, p.MonthlyReview.Recommendations...)
	case p.DebtDecision != nil:
		return append([]analysis.Recommendation{}, p.DebtDecision.Recommendations...)
	case p.TaxOptimization != nil:
		return append([]analysis.Recommendation{}, p.TaxOptimization.Recommendations...)
	case p.PortfolioRebalance != nil:
		return append([]analysis.Recommendation{}, p.PortfolioRebalance.Recommendations...)
	default:
		return nil
	}
}

func (p ReportPayload) RiskFlags() []analysis.RiskFlag {
	switch {
	case p.MonthlyReview != nil:
		return append([]analysis.RiskFlag{}, p.MonthlyReview.RiskFlags...)
	case p.DebtDecision != nil:
		return append([]analysis.RiskFlag{}, p.DebtDecision.RiskFlags...)
	case p.TaxOptimization != nil:
		return append([]analysis.RiskFlag{}, p.TaxOptimization.RiskFlags...)
	case p.PortfolioRebalance != nil:
		return append([]analysis.RiskFlag{}, p.PortfolioRebalance.RiskFlags...)
	default:
		return nil
	}
}

func (p ReportPayload) MetricRecords() []finance.MetricRecord {
	switch {
	case p.MonthlyReview != nil:
		return append([]finance.MetricRecord{}, p.MonthlyReview.MetricRecords...)
	case p.DebtDecision != nil:
		return append([]finance.MetricRecord{}, p.DebtDecision.MetricRecords...)
	case p.TaxOptimization != nil:
		return append([]finance.MetricRecord{}, p.TaxOptimization.MetricRecords...)
	case p.PortfolioRebalance != nil:
		return append([]finance.MetricRecord{}, p.PortfolioRebalance.MetricRecords...)
	default:
		return nil
	}
}

func (p ReportPayload) GroundingRefs() []string {
	switch {
	case p.MonthlyReview != nil:
		return append([]string{}, p.MonthlyReview.GroundingRefs...)
	case p.DebtDecision != nil:
		return append([]string{}, p.DebtDecision.GroundingRefs...)
	case p.TaxOptimization != nil:
		return append([]string{}, p.TaxOptimization.GroundingRefs...)
	case p.PortfolioRebalance != nil:
		return append([]string{}, p.PortfolioRebalance.GroundingRefs...)
	default:
		return nil
	}
}

func (p ReportPayload) Caveats() []string {
	switch {
	case p.MonthlyReview != nil:
		return append([]string{}, p.MonthlyReview.Caveats...)
	case p.DebtDecision != nil:
		return append([]string{}, p.DebtDecision.Caveats...)
	case p.TaxOptimization != nil:
		return append([]string{}, p.TaxOptimization.Caveats...)
	case p.PortfolioRebalance != nil:
		return append([]string{}, p.PortfolioRebalance.Caveats...)
	default:
		return nil
	}
}

func (p ReportPayload) PolicyRuleRefs() []string {
	switch {
	case p.MonthlyReview != nil:
		return append([]string{}, p.MonthlyReview.PolicyRuleRefs...)
	case p.DebtDecision != nil:
		return append([]string{}, p.DebtDecision.PolicyRuleRefs...)
	case p.TaxOptimization != nil:
		return append([]string{}, p.TaxOptimization.PolicyRuleRefs...)
	case p.PortfolioRebalance != nil:
		return append([]string{}, p.PortfolioRebalance.PolicyRuleRefs...)
	default:
		return nil
	}
}

func (p ReportPayload) ApprovalRequired() bool {
	switch {
	case p.MonthlyReview != nil:
		return p.MonthlyReview.ApprovalRequired
	case p.DebtDecision != nil:
		return p.DebtDecision.ApprovalRequired
	case p.TaxOptimization != nil:
		return p.TaxOptimization.ApprovalRequired
	case p.PortfolioRebalance != nil:
		return p.PortfolioRebalance.ApprovalRequired
	default:
		return false
	}
}

func (p ReportPayload) ApprovalReason() string {
	switch {
	case p.MonthlyReview != nil:
		return p.MonthlyReview.ApprovalReason
	case p.DebtDecision != nil:
		return p.DebtDecision.ApprovalReason
	case p.TaxOptimization != nil:
		return p.TaxOptimization.ApprovalReason
	case p.PortfolioRebalance != nil:
		return p.PortfolioRebalance.ApprovalReason
	default:
		return ""
	}
}
