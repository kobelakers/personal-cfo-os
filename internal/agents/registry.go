package agents

import (
	"fmt"
	"sync"

	"github.com/kobelakers/personal-cfo-os/internal/protocol"
)

type InMemoryAgentRegistry struct {
	mu      sync.RWMutex
	entries map[string]RegisteredSystemAgent
}

func NewInMemoryAgentRegistry() *InMemoryAgentRegistry {
	return &InMemoryAgentRegistry{
		entries: make(map[string]RegisteredSystemAgent),
	}
}

func (r *InMemoryAgentRegistry) Register(agent RegisteredSystemAgent) error {
	if agent == nil {
		return fmt.Errorf("registered system agent cannot be nil")
	}
	key := registryKey(agent.Recipient(), agent.RequestKind())
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[key]; exists {
		return fmt.Errorf("agent already registered for %s", key)
	}
	r.entries[key] = agent
	return nil
}

func (r *InMemoryAgentRegistry) Resolve(recipient string, kind protocol.MessageKind) (RegisteredSystemAgent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.entries[registryKey(recipient, kind)]
	if !ok {
		return nil, &AgentExecutionError{
			Recipient: recipient,
			Kind:      kind,
			Failure: protocol.AgentFailure{
				Category: protocol.AgentFailureUnsupportedMessage,
				Message:  fmt.Sprintf("no registered agent for recipient %q and kind %q", recipient, kind),
			},
		}
	}
	return agent, nil
}

func registryKey(recipient string, kind protocol.MessageKind) string {
	return recipient + "::" + string(kind)
}
