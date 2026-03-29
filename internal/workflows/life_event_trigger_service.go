package workflows

import (
	"context"
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/reducers"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

type LifeEventObservationResult struct {
	EventEvidence      []observation.EvidenceRecord `json:"event_evidence"`
	DeadlineEvidence   []observation.EvidenceRecord `json:"deadline_evidence"`
	SupportingEvidence []observation.EvidenceRecord `json:"supporting_evidence,omitempty"`
	Evidence           []observation.EvidenceRecord `json:"evidence"`
	UpdatedState       state.FinancialWorldState    `json:"updated_state"`
	Diff               state.StateDiff              `json:"diff"`
}

type LifeEventWorkflowService struct {
	QueryEvent            tools.QueryEventTool
	QueryCalendarDeadline tools.QueryCalendarDeadlineTool
	QueryTransaction      tools.QueryTransactionTool
	QueryLiability        tools.QueryLiabilityTool
	QueryPortfolio        tools.QueryPortfolioTool
	ParseDocument         tools.ParseDocumentTool
	ReducerEngine         reducers.DeterministicReducerEngine
	StateReducer          state.DefaultStateReducer
	EventLog              *observability.EventLog
}

func (s LifeEventWorkflowService) ObserveAndReduce(
	ctx context.Context,
	spec taskspec.TaskSpec,
	event observation.LifeEventRecord,
	workflowID string,
	current state.FinancialWorldState,
) (LifeEventObservationResult, error) {
	userID := event.UserID
	if userID == "" {
		userID = current.UserID
	}
	input := observationInput(spec, userID)
	input["event_id"] = event.ID
	input["event_kind"] = string(event.Kind)

	eventEvidence, err := s.QueryEvent.QueryEvidence(ctx, input)
	if err != nil {
		return LifeEventObservationResult{}, err
	}
	if len(eventEvidence) == 0 {
		return LifeEventObservationResult{}, fmt.Errorf("life event workflow requires normalized event evidence")
	}
	deadlineEvidence, err := s.QueryCalendarDeadline.QueryEvidence(ctx, input)
	if err != nil {
		return LifeEventObservationResult{}, err
	}

	supporting := make([]observation.EvidenceRecord, 0, 8)
	transactionEvidence, err := s.QueryTransaction.QueryEvidence(ctx, input)
	if err != nil {
		return LifeEventObservationResult{}, err
	}
	supporting = append(supporting, transactionEvidence...)
	if taskHasArea(spec, "debt") || taskHasArea(spec, "liability") {
		liabilityEvidence, err := s.QueryLiability.QueryEvidence(ctx, input)
		if err != nil {
			return LifeEventObservationResult{}, err
		}
		supporting = append(supporting, liabilityEvidence...)
	}
	if taskHasArea(spec, "portfolio") {
		portfolioEvidence, err := s.QueryPortfolio.QueryEvidence(ctx, input)
		if err != nil {
			return LifeEventObservationResult{}, err
		}
		supporting = append(supporting, portfolioEvidence...)
	}
	if taskHasArea(spec, "tax") {
		documentEvidence, err := s.ParseDocument.ParseEvidence(ctx, input)
		if err != nil {
			return LifeEventObservationResult{}, err
		}
		supporting = append(supporting, documentEvidence...)
	}

	evidence := dedupeEvidence(append(append(eventEvidence, deadlineEvidence...), supporting...))
	appendWorkflowLog(s.EventLog, workflowID, "life_event_ingestion", "ingested typed life event evidence", map[string]string{
		"event_id":                event.ID,
		"event_kind":              string(event.Kind),
		"event_evidence_ids":      joinEvidenceIDs(eventEvidence),
		"deadline_evidence_ids":   joinEvidenceIDs(deadlineEvidence),
		"supporting_evidence_ids": joinEvidenceIDs(supporting),
	}, spec.CreatedAt)
	patch, err := s.ReducerEngine.BuildPatch(current, evidence, spec.ID, workflowID, "life_event_observed")
	if err != nil {
		return LifeEventObservationResult{}, err
	}
	updatedState, diff, err := s.StateReducer.ApplyEvidencePatch(current, patch)
	if err != nil {
		return LifeEventObservationResult{}, err
	}
	appendWorkflowLog(s.EventLog, workflowID, "life_event_state_diff", "applied evidence patch after life event ingestion", map[string]string{
		"from_version":   fmt.Sprintf("%d", diff.FromVersion),
		"to_version":     fmt.Sprintf("%d", diff.ToVersion),
		"changed_fields": joinStrings(diff.ChangedFields),
		"evidence_ids":   joinStrings(observationIDsToStrings(diff.EvidenceIDs)),
	}, spec.CreatedAt)
	return LifeEventObservationResult{
		EventEvidence:      eventEvidence,
		DeadlineEvidence:   deadlineEvidence,
		SupportingEvidence: supporting,
		Evidence:           evidence,
		UpdatedState:       updatedState,
		Diff:               diff,
	}, nil
}

func observationIDsToStrings(ids []observation.EvidenceID) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		result = append(result, string(id))
	}
	return result
}
