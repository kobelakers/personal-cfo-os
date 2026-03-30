package verification

import (
	"context"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type DeterministicValidator interface {
	Validate(ctx context.Context, spec taskspec.TaskSpec, currentState state.FinancialWorldState, evidence []observation.EvidenceRecord, output any) (VerificationResult, error)
}

type BusinessValidator interface {
	Validate(ctx context.Context, spec taskspec.TaskSpec, currentState state.FinancialWorldState, evidence []observation.EvidenceRecord, output any) (VerificationResult, error)
}

type GroundingValidator interface {
	Validate(
		ctx context.Context,
		spec taskspec.TaskSpec,
		currentState state.FinancialWorldState,
		evidence []observation.EvidenceRecord,
		memories []memory.MemoryRecord,
		finalContext contextview.BlockVerificationContext,
		output any,
	) ([]VerificationResult, error)
}

type NumericValidator interface {
	Validate(
		ctx context.Context,
		spec taskspec.TaskSpec,
		currentState state.FinancialWorldState,
		evidence []observation.EvidenceRecord,
		memories []memory.MemoryRecord,
		finalContext contextview.BlockVerificationContext,
		output any,
	) ([]VerificationResult, error)
}

type TrustBusinessValidator interface {
	Validate(
		ctx context.Context,
		spec taskspec.TaskSpec,
		currentState state.FinancialWorldState,
		evidence []observation.EvidenceRecord,
		memories []memory.MemoryRecord,
		finalContext contextview.BlockVerificationContext,
		output any,
	) ([]VerificationResult, error)
}

type EvidenceCoverageChecker interface {
	Check(spec taskspec.TaskSpec, evidence []observation.EvidenceRecord) (EvidenceCoverageReport, error)
}

type SuccessCriteriaChecker interface {
	Check(spec taskspec.TaskSpec, results []VerificationResult, output any) (VerificationResult, error)
}

type TrajectoryOracle interface {
	Evaluate(ctx context.Context, scenario string, results []VerificationResult) (OracleVerdict, error)
}
