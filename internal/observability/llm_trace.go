package observability

import (
	"sync"

	"github.com/kobelakers/personal-cfo-os/internal/model"
)

type LLMCallRecord = model.CallRecord

type LLMCallLog struct {
	mu      sync.Mutex
	records []model.CallRecord
}

func (l *LLMCallLog) RecordCall(record model.CallRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
}

func (l *LLMCallLog) Records() []model.CallRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]model.CallRecord, len(l.records))
	copy(result, l.records)
	return result
}
