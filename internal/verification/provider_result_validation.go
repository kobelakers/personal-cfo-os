package verification

import "github.com/kobelakers/personal-cfo-os/internal/structured"

func StructuredOutputFailureNeedsFallback(category structured.FailureCategory) bool {
	switch category {
	case structured.FailureCategoryParseFailed,
		structured.FailureCategorySchemaInvalid,
		structured.FailureCategoryGroundingInvalid,
		structured.FailureCategoryRepairFailed,
		structured.FailureCategoryFallbackUsed,
		structured.FailureCategoryFallbackFailed:
		return true
	default:
		return false
	}
}
