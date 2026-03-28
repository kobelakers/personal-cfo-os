package workflows

import (
	"context"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/reducers"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
)

type MonthlyReviewObservationResult struct {
	Evidence     []observation.EvidenceRecord `json:"evidence"`
	UpdatedState state.FinancialWorldState    `json:"updated_state"`
	Diff         state.StateDiff              `json:"diff"`
}

type MonthlyReviewService struct {
	QueryTransaction tools.QueryTransactionTool
	QueryLiability   tools.QueryLiabilityTool
	QueryPortfolio   tools.QueryPortfolioTool
	ParseDocument    tools.ParseDocumentTool
	ReducerEngine    reducers.DeterministicReducerEngine
	StateReducer     state.DefaultStateReducer
}

func (s MonthlyReviewService) ObserveAndReduce(
	ctx context.Context,
	spec taskspec.TaskSpec,
	userID string,
	workflowID string,
	current state.FinancialWorldState,
) (MonthlyReviewObservationResult, error) {
	input := observationInput(spec, userID)
	transactionEvidence, err := s.QueryTransaction.QueryEvidence(ctx, input)
	if err != nil {
		return MonthlyReviewObservationResult{}, err
	}
	liabilityEvidence, err := s.QueryLiability.QueryEvidence(ctx, input)
	if err != nil {
		return MonthlyReviewObservationResult{}, err
	}
	portfolioEvidence, err := s.QueryPortfolio.QueryEvidence(ctx, input)
	if err != nil {
		return MonthlyReviewObservationResult{}, err
	}
	documentEvidence, err := s.ParseDocument.ParseEvidence(ctx, input)
	if err != nil {
		return MonthlyReviewObservationResult{}, err
	}
	evidence := dedupeEvidence(append(append(append(transactionEvidence, liabilityEvidence...), portfolioEvidence...), documentEvidence...))

	patch, err := s.ReducerEngine.BuildPatch(current, evidence, spec.ID, workflowID, "observed")
	if err != nil {
		return MonthlyReviewObservationResult{}, err
	}
	updatedState, diff, err := s.StateReducer.ApplyEvidencePatch(current, patch)
	if err != nil {
		return MonthlyReviewObservationResult{}, err
	}
	return MonthlyReviewObservationResult{
		Evidence:     evidence,
		UpdatedState: updatedState,
		Diff:         diff,
	}, nil
}
