package workflows

import (
	"context"
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/reducers"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

type FollowUpObservationResult struct {
	EventEvidence      []observation.EvidenceRecord `json:"event_evidence,omitempty"`
	DeadlineEvidence   []observation.EvidenceRecord `json:"deadline_evidence,omitempty"`
	SupportingEvidence []observation.EvidenceRecord `json:"supporting_evidence,omitempty"`
	Evidence           []observation.EvidenceRecord `json:"evidence"`
	UpdatedState       state.FinancialWorldState    `json:"updated_state"`
	Diff               state.StateDiff              `json:"diff"`
}

type FollowUpWorkflowService struct {
	QueryEvent            tools.QueryEventTool
	QueryCalendarDeadline tools.QueryCalendarDeadlineTool
	QueryTransaction      tools.QueryTransactionTool
	QueryPortfolio        tools.QueryPortfolioTool
	ParseDocument         tools.ParseDocumentTool
	ReducerEngine         reducers.DeterministicReducerEngine
	StateReducer          state.DefaultStateReducer
	EventLog              *observability.EventLog
}

func (s FollowUpWorkflowService) ObserveAndReduce(
	ctx context.Context,
	spec taskspec.TaskSpec,
	activation runtimepkg.FollowUpActivationContext,
	workflowID string,
	current state.FinancialWorldState,
) (FollowUpObservationResult, error) {
	if activation.LifeEventID == "" || activation.LifeEventKind == "" || len(activation.SourceEvidenceIDs) == 0 {
		return FollowUpObservationResult{}, fmt.Errorf("follow-up workflow requires activation context with life event seed and source evidence ids")
	}
	input := observationInput(spec, current.UserID)
	input["event_id"] = activation.LifeEventID
	input["event_kind"] = activation.LifeEventKind
	input["source_evidence_ids"] = joinStrings(activation.SourceEvidenceIDs)

	eventEvidence, err := s.QueryEvent.QueryEvidence(ctx, input)
	if err != nil {
		return FollowUpObservationResult{}, err
	}
	if len(eventEvidence) == 0 {
		return FollowUpObservationResult{}, fmt.Errorf("follow-up workflow requires life event evidence for activation seed %q", activation.LifeEventID)
	}
	deadlineEvidence, err := s.QueryCalendarDeadline.QueryEvidence(ctx, input)
	if err != nil {
		return FollowUpObservationResult{}, err
	}

	supporting := make([]observation.EvidenceRecord, 0, 6)
	transactionEvidence, err := s.QueryTransaction.QueryEvidence(ctx, input)
	if err != nil {
		return FollowUpObservationResult{}, err
	}
	supporting = append(supporting, transactionEvidence...)
	switch spec.UserIntentType {
	case taskspec.UserIntentTaxOptimization:
		documentEvidence, err := s.ParseDocument.ParseEvidence(ctx, input)
		if err != nil {
			return FollowUpObservationResult{}, err
		}
		supporting = append(supporting, documentEvidence...)
	case taskspec.UserIntentPortfolioRebalance:
		portfolioEvidence, err := s.QueryPortfolio.QueryEvidence(ctx, input)
		if err != nil {
			return FollowUpObservationResult{}, err
		}
		supporting = append(supporting, portfolioEvidence...)
	}

	evidence := dedupeEvidence(append(append(eventEvidence, deadlineEvidence...), supporting...))
	patch, err := s.ReducerEngine.BuildPatch(current, evidence, spec.ID, workflowID, "follow_up_observed")
	if err != nil {
		return FollowUpObservationResult{}, err
	}
	updatedState, diff, err := s.StateReducer.ApplyEvidencePatch(current, patch)
	if err != nil {
		return FollowUpObservationResult{}, err
	}
	appendWorkflowLog(s.EventLog, activation.RootCorrelationID, "follow_up_observation", "observed follow-up workflow evidence from activation seed", map[string]string{
		"workflow_id":             workflowID,
		"task_id":                 spec.ID,
		"intent":                  string(spec.UserIntentType),
		"life_event_id":           activation.LifeEventID,
		"life_event_kind":         activation.LifeEventKind,
		"source_evidence_ids":     joinStrings(activation.SourceEvidenceIDs),
		"event_evidence_ids":      joinEvidenceIDs(eventEvidence),
		"deadline_evidence_ids":   joinEvidenceIDs(deadlineEvidence),
		"supporting_evidence_ids": joinEvidenceIDs(supporting),
	}, spec.CreatedAt)
	return FollowUpObservationResult{
		EventEvidence:      eventEvidence,
		DeadlineEvidence:   deadlineEvidence,
		SupportingEvidence: supporting,
		Evidence:           evidence,
		UpdatedState:       updatedState,
		Diff:               diff,
	}, nil
}
