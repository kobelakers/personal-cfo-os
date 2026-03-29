package observability

import (
	"sync"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
)

type EmbeddingCallTraceRecord = memory.EmbeddingCallRecord
type EmbeddingUsageTraceRecord = memory.EmbeddingUsageRecord

type EmbeddingCallLog struct {
	mu      sync.Mutex
	records []memory.EmbeddingCallRecord
}

func (l *EmbeddingCallLog) RecordEmbeddingCall(record memory.EmbeddingCallRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
}

func (l *EmbeddingCallLog) Records() []memory.EmbeddingCallRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]memory.EmbeddingCallRecord, len(l.records))
	copy(result, l.records)
	return result
}

type EmbeddingUsageLog struct {
	mu      sync.Mutex
	records []memory.EmbeddingUsageRecord
}

func (l *EmbeddingUsageLog) RecordEmbeddingUsage(record memory.EmbeddingUsageRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
}

func (l *EmbeddingUsageLog) Records() []memory.EmbeddingUsageRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]memory.EmbeddingUsageRecord, len(l.records))
	copy(result, l.records)
	return result
}

