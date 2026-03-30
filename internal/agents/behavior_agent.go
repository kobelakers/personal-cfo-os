package agents

import (
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/behavior"
	"github.com/kobelakers/personal-cfo-os/internal/protocol"
)

type BehaviorAgentHandler struct {
	Analyzer  behavior.Analyzer
	Validator behavior.BehaviorValidator
	Now       func() time.Time
}

func (BehaviorAgentHandler) Name() string      { return RecipientBehaviorAgent }
func (BehaviorAgentHandler) Recipient() string { return RecipientBehaviorAgent }
func (BehaviorAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindBehaviorAnalysisRequest
}

func (a BehaviorAgentHandler) Handle(_ AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.BehaviorAnalysisRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientBehaviorAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "behavior analysis request payload is required"},
		}
	}
	if payload.ExecutionContext.SelectedSkill == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientBehaviorAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "behavior analysis requires selected skill in execution context"},
		}
	}
	analyzer := a.Analyzer
	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}
	output, err := analyzer.Analyze(behavior.BehaviorEvidence{
		CurrentState: payload.CurrentState,
		Evidence:     payload.RelevantEvidence,
	}, *payload.ExecutionContext.SelectedSkill, collectMemoryIDsFromRecords(payload.RelevantMemories), now)
	if err != nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientBehaviorAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureValidation, Message: "behavior analysis failed"},
			Cause:     err,
		}
	}
	validator := a.Validator
	if err := validator.Validate(output); err != nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientBehaviorAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureValidation, Message: "behavior analysis output validation failed"},
			Cause:     err,
		}
	}
	result := output.ToBlockResult(
		string(payload.Block.ID),
		collectMemoryIDsFromRecords(payload.RelevantMemories),
		collectEvidenceIDs(payload.RelevantEvidence),
		confidenceFromEvidence(payload.RelevantEvidence),
	)
	if result.SelectedSkill.Family == "" {
		return AgentHandlerResult{}, fmt.Errorf("behavior block result missing selected skill")
	}
	return AgentHandlerResult{
		Kind: protocol.MessageKindBehaviorAnalysisResult,
		Body: protocol.AgentResultBody{
			BehaviorAnalysisResult: &protocol.BehaviorAnalysisResultPayload{Result: result},
		},
	}, nil
}
