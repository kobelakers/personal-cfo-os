package observability

import (
	"encoding/json"
	"sync"
	"time"
)

type LogEntry struct {
	TraceID       string            `json:"trace_id"`
	CorrelationID string            `json:"correlation_id"`
	Category      string            `json:"category"`
	Message       string            `json:"message"`
	Details       map[string]string `json:"details,omitempty"`
	OccurredAt    time.Time         `json:"occurred_at"`
}

type EventLog struct {
	mu      sync.Mutex
	entries []LogEntry
}

func (l *EventLog) Append(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (l *EventLog) Entries() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]LogEntry, len(l.entries))
	copy(result, l.entries)
	return result
}

func (l *EventLog) JSONDump() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	payload, err := json.MarshalIndent(l.entries, "", "  ")
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
