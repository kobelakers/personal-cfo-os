package observability

import (
	"sync"
	"time"
)

type AgentLifecycle string

const (
	AgentLifecycleDispatched AgentLifecycle = "agent_dispatched"
	AgentLifecycleStarted    AgentLifecycle = "agent_handler_started"
	AgentLifecycleCompleted  AgentLifecycle = "agent_handler_completed"
	AgentLifecycleFailed     AgentLifecycle = "agent_handler_failed"
)

type AgentExecutionRecord struct {
	DispatchID         string         `json:"dispatch_id"`
	TraceID            string         `json:"trace_id"`
	Recipient          string         `json:"recipient"`
	RequestKind        string         `json:"request_kind"`
	ResultKind         string         `json:"result_kind,omitempty"`
	Lifecycle          AgentLifecycle `json:"lifecycle"`
	CorrelationID      string         `json:"correlation_id"`
	CausationID        string         `json:"causation_id"`
	RequestMessageID   string         `json:"request_message_id"`
	ResultMessageID    string         `json:"result_message_id,omitempty"`
	ErrorCategory      string         `json:"error_category,omitempty"`
	WorkflowEventTypes []string       `json:"workflow_event_types,omitempty"`
	Summary            string         `json:"summary,omitempty"`
	OccurredAt         time.Time      `json:"occurred_at"`
}

type AgentTraceLog struct {
	mu      sync.Mutex
	records []AgentExecutionRecord
}

func (l *AgentTraceLog) Append(record AgentExecutionRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
}

func (l *AgentTraceLog) Records() []AgentExecutionRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]AgentExecutionRecord, len(l.records))
	copy(result, l.records)
	return result
}
