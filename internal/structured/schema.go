package structured

type FailureCategory string

const (
	FailureCategoryParseFailed      FailureCategory = "parse_failed"
	FailureCategorySchemaInvalid    FailureCategory = "schema_invalid"
	FailureCategoryGroundingInvalid FailureCategory = "grounding_invalid"
	FailureCategoryRepairFailed     FailureCategory = "repair_failed"
	FailureCategoryFallbackUsed     FailureCategory = "fallback_used"
	FailureCategoryFallbackFailed   FailureCategory = "fallback_failed"
)

type Schema[T any] struct {
	Name      string
	Parser    Parser[T]
	Validator Validator[T]
}
