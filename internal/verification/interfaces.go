package verification

import (
	"context"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type DeterministicValidator interface {
	Validate(ctx context.Context, spec taskspec.TaskSpec, currentState state.FinancialWorldState, evidence []observation.EvidenceRecord) (VerificationResult, error)
}

type BusinessValidator interface {
	Validate(ctx context.Context, spec taskspec.TaskSpec, currentState state.FinancialWorldState, evidence []observation.EvidenceRecord) (VerificationResult, error)
}

type EvidenceCoverageChecker interface {
	Check(spec taskspec.TaskSpec, evidence []observation.EvidenceRecord) (EvidenceCoverageReport, error)
}

type SuccessCriteriaChecker interface {
	Check(spec taskspec.TaskSpec, result VerificationResult) error
}

type TrajectoryOracle interface {
	Evaluate(ctx context.Context, scenario string, result VerificationResult) (OracleVerdict, error)
}
