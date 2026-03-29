package verification

import (
	"context"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type Pipeline struct {
	CoverageChecker        EvidenceCoverageChecker
	DeterministicValidator DeterministicValidator
	BusinessValidator      BusinessValidator
	SuccessChecker         SuccessCriteriaChecker
	Oracle                 TrajectoryOracle
	Now                    func() time.Time
}

type PipelineResult struct {
	CoverageReport EvidenceCoverageReport          `json:"coverage_report"`
	BlockResults   map[string][]VerificationResult `json:"block_results,omitempty"`
	FinalResults   []VerificationResult            `json:"final_results,omitempty"`
	Results        []VerificationResult            `json:"results"`
	OracleVerdict  OracleVerdict                   `json:"oracle_verdict"`
}

func (p Pipeline) VerifyMonthlyReview(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	plan planning.ExecutionPlan,
	blockResults []analysis.BlockResultEnvelope,
	blockVerificationContexts []contextview.BlockVerificationContext,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
) (PipelineResult, error) {
	return p.verify(
		ctx,
		"monthly-review",
		spec,
		currentState,
		evidence,
		memories,
		plan,
		blockResults,
		blockVerificationContexts,
		finalVerificationContext,
		output,
		true,
	)
}

func (p Pipeline) VerifyDebtDecision(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	plan planning.ExecutionPlan,
	blockResults []analysis.BlockResultEnvelope,
	blockVerificationContexts []contextview.BlockVerificationContext,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
) (PipelineResult, error) {
	return p.verify(
		ctx,
		"debt-vs-invest",
		spec,
		currentState,
		evidence,
		memories,
		plan,
		blockResults,
		blockVerificationContexts,
		finalVerificationContext,
		output,
		false,
	)
}

func (p Pipeline) verify(
	ctx context.Context,
	scenario string,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	plan planning.ExecutionPlan,
	blockResults []analysis.BlockResultEnvelope,
	blockVerificationContexts []contextview.BlockVerificationContext,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
	monthlyReview bool,
) (PipelineResult, error) {
	coverageChecker := p.coverageChecker()
	successChecker := p.successChecker()
	oracle := p.oracle()

	if err := plan.Validate(); err != nil {
		return PipelineResult{}, err
	}

	coverageReport, err := coverageChecker.Check(spec, evidence)
	if err != nil {
		return PipelineResult{}, err
	}
	coverageResult := withScope(CoverageToVerificationResult(spec, coverageReport, p.now()), VerificationScopeFinal, "", "")

	blockMap, severeFailure, err := p.verifyBlocks(spec, currentState, plan, blockResults, blockVerificationContexts)
	if err != nil {
		return PipelineResult{}, err
	}

	flattened := []VerificationResult{coverageResult}
	flattened = append(flattened, flattenBlockResults(plan, blockMap)...)

	if severeFailure {
		shortCircuit := VerificationResult{
			Status:                  VerificationStatusNeedsReplan,
			Scope:                   VerificationScopeFinal,
			Validator:               "final_validation_short_circuit",
			Message:                 "block-level verification failed; final report validation skipped",
			FailedRules:             []string{"block_verification_short_circuit"},
			RecommendedReplanAction: "repair failed block outputs before regenerating the final report",
			Severity:                "high",
			Details: map[string]any{
				"plan_id":               plan.PlanID,
				"selected_memory_count": len(memories),
			},
			EvidenceCoverage: fullCoverage(spec.ID),
			CheckedAt:        p.now(),
		}
		finalResults := []VerificationResult{shortCircuit}
		flattened = append(flattened, finalResults...)
		verdict, err := oracle.Evaluate(ctx, scenario, flattened)
		if err != nil {
			return PipelineResult{}, err
		}
		return PipelineResult{
			CoverageReport: coverageReport,
			BlockResults:   blockMap,
			FinalResults:   finalResults,
			Results:        flattened,
			OracleVerdict:  verdict,
		}, nil
	}

	finalResults, err := p.verifyFinal(ctx, spec, currentState, evidence, finalVerificationContext, output, monthlyReview)
	if err != nil {
		return PipelineResult{}, err
	}
	flattened = append(flattened, finalResults...)

	successResult, err := successChecker.Check(spec, flattened, output)
	if err != nil {
		return PipelineResult{}, err
	}
	successResult = withScope(successResult, VerificationScopeFinal, "", "")
	finalResults = append(finalResults, successResult)
	flattened = append(flattened, successResult)

	verdict, err := oracle.Evaluate(ctx, scenario, flattened)
	if err != nil {
		return PipelineResult{}, err
	}
	return PipelineResult{
		CoverageReport: coverageReport,
		BlockResults:   blockMap,
		FinalResults:   finalResults,
		Results:        flattened,
		OracleVerdict:  verdict,
	}, nil
}

func (p Pipeline) verifyBlocks(
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	plan planning.ExecutionPlan,
	results []analysis.BlockResultEnvelope,
	contexts []contextview.BlockVerificationContext,
) (map[string][]VerificationResult, bool, error) {
	resultByID := make(map[string]analysis.BlockResultEnvelope, len(results))
	for _, item := range results {
		if err := item.Validate(); err != nil {
			return nil, false, err
		}
		resultByID[item.BlockID] = item
	}
	contextByID := make(map[string]contextview.BlockVerificationContext, len(contexts))
	for _, item := range contexts {
		contextByID[item.BlockID] = item
	}

	blockResults := make(map[string][]VerificationResult, len(plan.Blocks))
	severe := false
	for _, block := range plan.Blocks {
		envelope, ok := resultByID[string(block.ID)]
		if !ok {
			result := VerificationResult{
				Status:                  VerificationStatusFail,
				Scope:                   VerificationScopeBlock,
				BlockID:                 string(block.ID),
				BlockKind:               string(block.Kind),
				Validator:               "block_presence_validator",
				Message:                 "required block result is missing",
				FailedRules:             []string{"schema_invalid"},
				RecommendedReplanAction: "dispatch the missing analysis block before drafting the report",
				Severity:                "high",
				EvidenceCoverage:        fullCoverage(spec.ID),
				CheckedAt:               p.now(),
			}
			blockResults[string(block.ID)] = []VerificationResult{result}
			severe = true
			continue
		}
		verificationContext, ok := contextByID[string(block.ID)]
		if !ok {
			result := VerificationResult{
				Status:                  VerificationStatusFail,
				Scope:                   VerificationScopeBlock,
				BlockID:                 string(block.ID),
				BlockKind:               string(block.Kind),
				Validator:               "block_context_validator",
				Message:                 "block verification context is missing",
				FailedRules:             []string{"schema_invalid"},
				RecommendedReplanAction: "assemble block verification context before verifying the report",
				Severity:                "high",
				EvidenceCoverage:        fullCoverage(spec.ID),
				CheckedAt:               p.now(),
			}
			blockResults[string(block.ID)] = []VerificationResult{result}
			severe = true
			continue
		}

		var blockResult VerificationResult
		switch {
		case envelope.Cashflow != nil:
			blockResult = CashflowBlockValidator{}.Validate(spec, block, *envelope.Cashflow, verificationContext, currentState)
		case envelope.Debt != nil:
			blockResult = DebtBlockValidator{}.Validate(spec, block, *envelope.Debt, verificationContext, currentState)
		default:
			blockResult = VerificationResult{
				Status:                  VerificationStatusFail,
				Scope:                   VerificationScopeBlock,
				BlockID:                 string(block.ID),
				BlockKind:               string(block.Kind),
				Validator:               "block_schema_validator",
				Message:                 "typed block result is missing",
				FailedRules:             []string{"schema_invalid"},
				RecommendedReplanAction: "rerun block analysis and return a typed block result",
				Severity:                "high",
				EvidenceCoverage:        fullCoverage(spec.ID),
				CheckedAt:               p.now(),
			}
		}
		blockResults[string(block.ID)] = []VerificationResult{blockResult}
		severe = severe || severeBlockFailure(blockResult)
	}
	return blockResults, severe, nil
}

func (p Pipeline) verifyFinal(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
	monthlyReview bool,
) ([]VerificationResult, error) {
	finalResults := make([]VerificationResult, 0, 4)

	contextResult := VerificationResult{
		Status:                  VerificationStatusPass,
		Scope:                   VerificationScopeFinal,
		Validator:               "final_verification_context_validator",
		Message:                 "final verification context is valid",
		RecommendedReplanAction: "rebuild final verification context with grounded evidence",
		Severity:                "low",
		Details: map[string]any{
			"selected_memory_ids":   finalVerificationContext.SelectedMemoryIDs,
			"selected_evidence_ids": finalVerificationContext.SelectedEvidenceIDs,
			"selected_state_blocks": finalVerificationContext.SelectedStateBlocks,
		},
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        p.now(),
	}
	if len(finalVerificationContext.SelectedEvidenceIDs) == 0 || finalVerificationContext.ResultSummary == "" {
		contextResult.Status = VerificationStatusNeedsReplan
		contextResult.Message = "final verification context is incomplete"
		contextResult.FailedRules = []string{"final_context_incomplete"}
		contextResult.Severity = "medium"
	}
	finalResults = append(finalResults, contextResult)

	if monthlyReview {
		deterministicResult, err := p.deterministicValidator().Validate(ctx, spec, currentState, evidence, output)
		if err != nil {
			return nil, err
		}
		finalResults = append(finalResults, withScope(deterministicResult, VerificationScopeFinal, "", ""))
		businessResult, err := p.businessValidator(true).Validate(ctx, spec, currentState, evidence, output)
		if err != nil {
			return nil, err
		}
		finalResults = append(finalResults, withScope(businessResult, VerificationScopeFinal, "", ""))
		return finalResults, nil
	}

	businessResult, err := p.businessValidator(false).Validate(ctx, spec, currentState, evidence, output)
	if err != nil {
		return nil, err
	}
	finalResults = append(finalResults, withScope(businessResult, VerificationScopeFinal, "", ""))
	return finalResults, nil
}

func NeedsReplan(results []VerificationResult) bool {
	for _, result := range results {
		if result.Status == VerificationStatusFail || result.Status == VerificationStatusNeedsReplan {
			return true
		}
	}
	return false
}

func CoverageToVerificationResult(spec taskspec.TaskSpec, report EvidenceCoverageReport, checkedAt time.Time) VerificationResult {
	status := VerificationStatusPass
	message := "required evidence coverage satisfied"
	missing := make([]string, 0)
	for i, item := range report.Items {
		if !item.Covered && i < len(spec.RequiredEvidence) && spec.RequiredEvidence[i].Mandatory {
			status = VerificationStatusNeedsReplan
			message = "mandatory evidence missing: " + item.RequirementID
			missing = append(missing, item.RequirementID)
		}
	}
	result := VerificationResult{
		Status:                  status,
		Scope:                   VerificationScopeFinal,
		Validator:               "evidence_coverage_checker",
		Message:                 message,
		MissingEvidence:         missing,
		RecommendedReplanAction: "collect missing mandatory evidence",
		Severity:                "medium",
		EvidenceCoverage:        report,
		CheckedAt:               checkedAt,
	}
	if len(missing) > 0 {
		result.FailedRules = []string{"required_evidence"}
		result.Details = map[string]any{"missing_requirement_ids": missing}
	}
	return result
}

func flattenBlockResults(plan planning.ExecutionPlan, byBlock map[string][]VerificationResult) []VerificationResult {
	result := make([]VerificationResult, 0, len(byBlock))
	for _, block := range plan.Blocks {
		result = append(result, byBlock[string(block.ID)]...)
	}
	return result
}

func withScope(result VerificationResult, scope VerificationScope, blockID string, blockKind string) VerificationResult {
	result.Scope = scope
	result.BlockID = blockID
	result.BlockKind = blockKind
	return result
}

func (p Pipeline) coverageChecker() EvidenceCoverageChecker {
	if p.CoverageChecker != nil {
		return p.CoverageChecker
	}
	return DefaultEvidenceCoverageChecker{}
}

func (p Pipeline) deterministicValidator() DeterministicValidator {
	if p.DeterministicValidator != nil {
		return p.DeterministicValidator
	}
	return MonthlyReviewDeterministicValidator{}
}

func (p Pipeline) businessValidator(monthlyReview bool) BusinessValidator {
	if p.BusinessValidator != nil {
		return p.BusinessValidator
	}
	if monthlyReview {
		return MonthlyReviewBusinessValidator{}
	}
	return DebtDecisionBusinessValidator{}
}

func (p Pipeline) successChecker() SuccessCriteriaChecker {
	if p.SuccessChecker != nil {
		return p.SuccessChecker
	}
	return DefaultSuccessCriteriaChecker{}
}

func (p Pipeline) oracle() TrajectoryOracle {
	if p.Oracle != nil {
		return p.Oracle
	}
	return BaselineTrajectoryOracle{}
}

func (p Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}
