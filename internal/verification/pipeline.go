package verification

import (
	"context"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
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
	CoverageReport EvidenceCoverageReport `json:"coverage_report"`
	Results        []VerificationResult   `json:"results"`
	OracleVerdict  OracleVerdict          `json:"oracle_verdict"`
}

func (p Pipeline) VerifyMonthlyReview(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	output any,
) (PipelineResult, error) {
	coverageChecker := p.coverageChecker()
	deterministicValidator := p.deterministicValidator()
	businessValidator := p.businessValidator(true)
	successChecker := p.successChecker()
	oracle := p.oracle()

	coverageReport, err := coverageChecker.Check(spec, evidence)
	if err != nil {
		return PipelineResult{}, err
	}
	coverageResult := CoverageToVerificationResult(spec, coverageReport, p.now())
	deterministicResult, err := deterministicValidator.Validate(ctx, spec, currentState, evidence, output)
	if err != nil {
		return PipelineResult{}, err
	}
	businessResult, err := businessValidator.Validate(ctx, spec, currentState, evidence, output)
	if err != nil {
		return PipelineResult{}, err
	}
	results := []VerificationResult{coverageResult, deterministicResult, businessResult}
	successResult, err := successChecker.Check(spec, results, output)
	if err != nil {
		return PipelineResult{}, err
	}
	results = append(results, successResult)
	verdict, err := oracle.Evaluate(ctx, "monthly-review", results)
	if err != nil {
		return PipelineResult{}, err
	}
	return PipelineResult{
		CoverageReport: coverageReport,
		Results:        results,
		OracleVerdict:  verdict,
	}, nil
}

func (p Pipeline) VerifyDebtDecision(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	output any,
) (PipelineResult, error) {
	coverageChecker := p.coverageChecker()
	businessValidator := p.businessValidator(false)
	successChecker := p.successChecker()
	oracle := p.oracle()

	coverageReport, err := coverageChecker.Check(spec, evidence)
	if err != nil {
		return PipelineResult{}, err
	}
	coverageResult := CoverageToVerificationResult(spec, coverageReport, p.now())
	businessResult, err := businessValidator.Validate(ctx, spec, currentState, evidence, output)
	if err != nil {
		return PipelineResult{}, err
	}
	results := []VerificationResult{coverageResult, businessResult}
	successResult, err := successChecker.Check(spec, results, output)
	if err != nil {
		return PipelineResult{}, err
	}
	results = append(results, successResult)
	verdict, err := oracle.Evaluate(ctx, "debt-vs-invest", results)
	if err != nil {
		return PipelineResult{}, err
	}
	return PipelineResult{
		CoverageReport: coverageReport,
		Results:        results,
		OracleVerdict:  verdict,
	}, nil
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
