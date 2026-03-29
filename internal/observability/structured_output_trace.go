package observability

import (
	"sync"

	"github.com/kobelakers/personal-cfo-os/internal/structured"
)

type StructuredOutputTraceRecord = structured.TraceRecord

type StructuredOutputTraceLog struct {
	mu      sync.Mutex
	records []structured.TraceRecord
}

func (l *StructuredOutputTraceLog) RecordStructuredOutput(record structured.TraceRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
}

func (l *StructuredOutputTraceLog) Records() []structured.TraceRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]structured.TraceRecord, len(l.records))
	copy(result, l.records)
	return result
}
