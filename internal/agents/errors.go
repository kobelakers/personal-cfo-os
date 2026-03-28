package agents

import (
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/protocol"
)

type AgentExecutionError struct {
	Recipient string
	Kind      protocol.MessageKind
	Failure   protocol.AgentFailure
	Cause     error
}

func (e *AgentExecutionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s %s failed: %s: %v", e.Recipient, e.Kind, e.Failure.Message, e.Cause)
	}
	return fmt.Sprintf("%s %s failed: %s", e.Recipient, e.Kind, e.Failure.Message)
}

func (e *AgentExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *AgentExecutionError) AgentFailure() protocol.AgentFailure {
	if e == nil {
		return protocol.AgentFailure{}
	}
	return e.Failure
}
