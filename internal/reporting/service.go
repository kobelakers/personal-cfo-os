package reporting

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
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

type DraftInput struct {
	Plan         planning.ExecutionPlan         `json:"plan"`
	BlockResults []analysis.BlockResultEnvelope `json:"block_results"`
	CurrentState state.FinancialWorldState      `json:"current_state"`
	Evidence     []observation.EvidenceRecord   `json:"evidence"`
	Memories     []memory.MemoryRecord          `json:"memories,omitempty"`
	StateDiff    []string                       `json:"state_diff,omitempty"`
	TaskGraph    *taskspec.TaskGraph            `json:"task_graph,omitempty"`
}

type Service struct {
	MonthlyReviewAggregator MonthlyReviewAggregator
	DebtDecisionAggregator  DebtDecisionAggregator
	LifeEventAggregator     LifeEventAssessmentAggregator
	Artifacts               ArtifactService
}

func (s Service) Draft(spec taskspec.TaskSpec, workflowID string, input DraftInput) (ReportPayload, error) {
	if err := input.Plan.Validate(); err != nil {
		return ReportPayload{}, err
	}
	for _, result := range input.BlockResults {
		if err := result.Validate(); err != nil {
			return ReportPayload{}, err
		}
	}
	switch spec.UserIntentType {
	case taskspec.UserIntentMonthlyReview:
		report, err := s.MonthlyReviewAggregator.Aggregate(spec, workflowID, input)
		if err != nil {
			return ReportPayload{}, err
		}
		return ReportPayload{MonthlyReview: &report}, nil
	case taskspec.UserIntentDebtVsInvest:
		report, err := s.DebtDecisionAggregator.Aggregate(spec, workflowID, input)
		if err != nil {
			return ReportPayload{}, err
		}
		return ReportPayload{DebtDecision: &report}, nil
	case taskspec.UserIntentLifeEventTrigger:
		report, err := s.LifeEventAggregator.Aggregate(spec, workflowID, input)
		if err != nil {
			return ReportPayload{}, err
		}
		return ReportPayload{LifeEventAssessment: &report}, nil
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
	case draft.LifeEventAssessment != nil:
		report := *draft.LifeEventAssessment
		if disclosureDecision.Outcome == governance.PolicyDecisionRedact {
			report.EventSummary = "[REDACTED] " + report.EventSummary
		}
		artifact, err := s.Artifacts.Produce(workflowID, taskID, ArtifactKindLifeEventAssessment, report, report.EventSummary, "report_agent")
		if err != nil {
			return ReportPayload{}, nil, err
		}
		return ReportPayload{LifeEventAssessment: &report}, []WorkflowArtifact{artifact}, nil
	default:
		return ReportPayload{}, nil, fmt.Errorf("report draft payload is empty")
	}
}
