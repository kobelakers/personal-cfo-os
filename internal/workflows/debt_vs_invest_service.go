package workflows

import (
	"context"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/reducers"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

type DebtVsInvestObservationResult struct {
	Evidence     []observation.EvidenceRecord `json:"evidence"`
	UpdatedState state.FinancialWorldState    `json:"updated_state"`
	Diff         state.StateDiff              `json:"diff"`
}

type DebtVsInvestService struct {
	QueryTransaction tools.QueryTransactionTool
	QueryLiability   tools.QueryLiabilityTool
	QueryPortfolio   tools.QueryPortfolioTool
	ReducerEngine    reducers.DeterministicReducerEngine
	StateReducer     state.DefaultStateReducer
}

func (s DebtVsInvestService) ObserveAndReduce(
	ctx context.Context,
	spec taskspec.TaskSpec,
	userID string,
	workflowID string,
	current state.FinancialWorldState,
) (DebtVsInvestObservationResult, error) {
	input := observationInput(spec, userID)
	transactionEvidence, err := s.QueryTransaction.QueryEvidence(ctx, input)
	if err != nil {
		return DebtVsInvestObservationResult{}, err
	}
	liabilityEvidence, err := s.QueryLiability.QueryEvidence(ctx, input)
	if err != nil {
		return DebtVsInvestObservationResult{}, err
	}
	portfolioEvidence, err := s.QueryPortfolio.QueryEvidence(ctx, input)
	if err != nil {
		return DebtVsInvestObservationResult{}, err
	}
	evidence := dedupeEvidence(append(append(transactionEvidence, liabilityEvidence...), portfolioEvidence...))
	patch, err := s.ReducerEngine.BuildPatch(current, evidence, spec.ID, workflowID, "observed")
	if err != nil {
		return DebtVsInvestObservationResult{}, err
	}
	updatedState, diff, err := s.StateReducer.ApplyEvidencePatch(current, patch)
	if err != nil {
		return DebtVsInvestObservationResult{}, err
	}
	return DebtVsInvestObservationResult{
		Evidence:     evidence,
		UpdatedState: updatedState,
		Diff:         diff,
	}, nil
}
