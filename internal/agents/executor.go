package agents

import "github.com/kobelakers/personal-cfo-os/internal/protocol"

type LocalAgentExecutor struct{}

func (LocalAgentExecutor) Execute(handlerCtx AgentHandlerContext, agent RegisteredSystemAgent, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	return agent.Handle(handlerCtx, envelope)
}
