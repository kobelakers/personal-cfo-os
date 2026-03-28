package runtime

import (
	"errors"

	"github.com/kobelakers/personal-cfo-os/internal/protocol"
)

func FailureCategoryFromAgentError(err error) (FailureCategory, bool) {
	var categorized protocol.CategorizedAgentError
	if !errors.As(err, &categorized) {
		return "", false
	}
	switch categorized.AgentFailure().Category {
	case protocol.AgentFailureValidation:
		return FailureCategoryValidation, true
	case protocol.AgentFailurePolicy:
		return FailureCategoryPolicy, true
	case protocol.AgentFailureTransient:
		return FailureCategoryTransient, true
	case protocol.AgentFailureUnsupportedMessage, protocol.AgentFailureBadPayload:
		return FailureCategoryProtocol, true
	case protocol.AgentFailureUnrecoverable:
		return FailureCategoryUnrecoverable, true
	default:
		return FailureCategoryUnrecoverable, true
	}
}

func HandleAgentExecutionFailure(rt WorkflowRuntime, ctx ExecutionContext, current WorkflowExecutionState, err error, summary string) (WorkflowExecutionState, RecoveryStrategy, error) {
	category, ok := FailureCategoryFromAgentError(err)
	if !ok {
		return "", "", err
	}
	return rt.HandleFailure(ctx, current, category, summary)
}
