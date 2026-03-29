package observability

import (
	"sync"

	"github.com/kobelakers/personal-cfo-os/internal/model"
)

type UsageTraceRecord = model.UsageRecord

type UsageTraceLog struct {
	mu      sync.Mutex
	records []model.UsageRecord
}

func (l *UsageTraceLog) RecordUsage(record model.UsageRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
}

func (l *UsageTraceLog) Records() []model.UsageRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]model.UsageRecord, len(l.records))
	copy(result, l.records)
	return result
}
