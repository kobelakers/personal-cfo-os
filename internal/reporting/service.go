package reporting

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

type ArtifactService struct {
	Tool     tools.GenerateTaskArtifactTool
	Producer ArtifactProducer
	Now      func() time.Time
}

func (s ArtifactService) Produce(
	workflowID string,
	taskID string,
	kind ArtifactKind,
	content any,
	summary string,
	producedBy string,
) (WorkflowArtifact, error) {
	contentJSON, err := s.Tool.Generate(content)
	if err != nil {
		return WorkflowArtifact{}, err
	}
	producer := s.Producer
	if producer == nil {
		producer = StaticArtifactProducer{Now: s.Now}
	}
	return producer.ProduceArtifact(workflowID, taskID, kind, contentJSON, summary, producedBy), nil
}

func (s ArtifactService) ProduceJSON(
	workflowID string,
	taskID string,
	kind ArtifactKind,
	content any,
	summary string,
	producedBy string,
) (WorkflowArtifact, error) {
	payload, err := json.Marshal(content)
	if err != nil {
		return WorkflowArtifact{}, err
	}
	producer := s.Producer
	if producer == nil {
		producer = StaticArtifactProducer{Now: s.Now}
	}
	return producer.ProduceArtifact(workflowID, taskID, kind, string(payload), summary, producedBy), nil
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

type Service struct {
	MonthlyReviewBuilder MonthlyReviewBuilder
	DebtDecisionBuilder  DebtDecisionBuilder
	Artifacts            ArtifactService
}

type MonthlyReviewBuilder interface {
	Name() string
	Draft(workflowID string, taskID string, current state.FinancialWorldState, evidence []observation.EvidenceRecord) (MonthlyReviewReport, error)
}

type DebtDecisionBuilder interface {
	Name() string
	Draft(workflowID string, taskID string, current state.FinancialWorldState, evidence []observation.EvidenceRecord) (DebtDecisionReport, error)
}

type MonthlyReviewDraftBuilder struct {
	Skill           skills.MonthlyReviewSkill
	CashflowMetrics tools.ComputeCashflowMetricsTool
	TaxSignals      tools.ComputeTaxSignalTool
	Now             func() time.Time
}

func (b MonthlyReviewDraftBuilder) Name() string { return "monthly_review_draft_builder" }

func (b MonthlyReviewDraftBuilder) Draft(workflowID string, taskID string, current state.FinancialWorldState, evidence []observation.EvidenceRecord) (MonthlyReviewReport, error) {
	generatedAt := time.Now().UTC()
	if b.Now != nil {
		generatedAt = b.Now().UTC()
	}
	output := b.Skill.Generate(current, evidence)
	return MonthlyReviewReport{
		TaskID:                  taskID,
		WorkflowID:              workflowID,
		Summary:                 output.Summary,
		CashflowMetrics:         b.CashflowMetrics.Compute(current),
		TaxSignals:              b.TaxSignals.Compute(current),
		RiskItems:               output.RiskItems,
		OptimizationSuggestions: output.Suggestions,
		TodoItems:               output.TodoItems,
		ApprovalRequired:        current.RiskState.OverallRisk == "high",
		Confidence:              output.Confidence,
		GeneratedAt:             generatedAt,
	}, nil
}

type DebtDecisionDraftBuilder struct {
	Skill          skills.DebtOptimizationSkill
	ComputeMetrics tools.ComputeDebtDecisionMetricsTool
	Now            func() time.Time
}

func (b DebtDecisionDraftBuilder) Name() string { return "debt_decision_draft_builder" }

func (b DebtDecisionDraftBuilder) Draft(workflowID string, taskID string, current state.FinancialWorldState, evidence []observation.EvidenceRecord) (DebtDecisionReport, error) {
	generatedAt := time.Now().UTC()
	if b.Now != nil {
		generatedAt = b.Now().UTC()
	}
	output := b.Skill.Analyze(current)
	return DebtDecisionReport{
		TaskID:           taskID,
		WorkflowID:       workflowID,
		Conclusion:       output.Conclusion,
		Reasons:          output.Reasons,
		Actions:          output.Actions,
		Metrics:          b.ComputeMetrics.Compute(current),
		EvidenceIDs:      collectEvidenceIDs(evidence),
		ApprovalRequired: current.RiskState.OverallRisk == "high",
		Confidence:       output.Confidence,
		GeneratedAt:      generatedAt,
	}, nil
}

func (s Service) Draft(spec taskspec.TaskSpec, workflowID string, current state.FinancialWorldState, evidence []observation.EvidenceRecord) (ReportPayload, error) {
	switch spec.UserIntentType {
	case taskspec.UserIntentMonthlyReview:
		if s.MonthlyReviewBuilder == nil {
			return ReportPayload{}, fmt.Errorf("monthly review report builder is required")
		}
		report, err := s.MonthlyReviewBuilder.Draft(workflowID, spec.ID, current, evidence)
		if err != nil {
			return ReportPayload{}, err
		}
		return ReportPayload{MonthlyReview: &report}, nil
	case taskspec.UserIntentDebtVsInvest:
		if s.DebtDecisionBuilder == nil {
			return ReportPayload{}, fmt.Errorf("debt decision report builder is required")
		}
		report, err := s.DebtDecisionBuilder.Draft(workflowID, spec.ID, current, evidence)
		if err != nil {
			return ReportPayload{}, err
		}
		return ReportPayload{DebtDecision: &report}, nil
	default:
		return ReportPayload{}, fmt.Errorf("unsupported intent type %q for report draft", spec.UserIntentType)
	}
}

func (s Service) Finalize(workflowID string, taskID string, draft ReportPayload, disclosureDecision governance.PolicyDecision) (ReportPayload, []WorkflowArtifact, error) {
	switch disclosureDecision.Outcome {
	case governance.PolicyDecisionAllow, governance.PolicyDecisionRedact:
	default:
		return ReportPayload{}, nil, fmt.Errorf("report finalization requires allow or redact disclosure outcome, got %q", disclosureDecision.Outcome)
	}

	switch {
	case draft.MonthlyReview != nil:
		report := *draft.MonthlyReview
		if disclosureDecision.Outcome == governance.PolicyDecisionRedact {
			report.Summary = "[REDACTED] " + report.Summary
		}
		artifact, err := s.Artifacts.Produce(workflowID, taskID, ArtifactKindMonthlyReviewReport, report, report.Summary, "report_agent")
		if err != nil {
			return ReportPayload{}, nil, err
		}
		return ReportPayload{MonthlyReview: &report}, []WorkflowArtifact{artifact}, nil
	case draft.DebtDecision != nil:
		report := *draft.DebtDecision
		if disclosureDecision.Outcome == governance.PolicyDecisionRedact {
			report.Conclusion = "[REDACTED] " + report.Conclusion
		}
		artifact, err := s.Artifacts.Produce(workflowID, taskID, ArtifactKindDebtDecisionReport, report, report.Conclusion, "report_agent")
		if err != nil {
			return ReportPayload{}, nil, err
		}
		return ReportPayload{DebtDecision: &report}, []WorkflowArtifact{artifact}, nil
	default:
		return ReportPayload{}, nil, fmt.Errorf("report draft payload is empty")
	}
}

func collectEvidenceIDs(records []observation.EvidenceRecord) []observation.EvidenceID {
	result := make([]observation.EvidenceID, 0, len(records))
	for _, record := range records {
		result = append(result, record.ID)
	}
	return result
}
