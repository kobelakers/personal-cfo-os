package runtime

import "fmt"

type RetryPlanner struct{}

func (RetryPlanner) StrategyFor(category FailureCategory) (RecoveryStrategy, error) {
	switch category {
	case FailureCategoryTransient, FailureCategoryTimeout:
		return RecoveryStrategyRetry, nil
	case FailureCategoryValidation:
		return RecoveryStrategyReplan, nil
	case FailureCategoryPolicy:
		return RecoveryStrategyWaitForApproval, nil
	case FailureCategoryUnrecoverable:
		return RecoveryStrategyAbort, nil
	default:
		return "", fmt.Errorf("unsupported failure category %q", category)
	}
}
