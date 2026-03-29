package observability

import (
	"sync"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
)

type MemoryAccessRecord = memory.MemoryAccessAudit
type MemoryQueryTraceRecord = memory.MemoryQueryRecord
type MemoryRetrievalTraceRecord = memory.MemoryRetrievalRecord
type MemorySelectionTraceRecord = memory.MemorySelectionRecord

type MemoryTraceLog struct {
	mu         sync.Mutex
	queries    []memory.MemoryQueryRecord
	retrievals []memory.MemoryRetrievalRecord
	selections []memory.MemorySelectionRecord
}

func (l *MemoryTraceLog) RecordMemoryQuery(record memory.MemoryQueryRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.queries = append(l.queries, record)
}

func (l *MemoryTraceLog) RecordMemoryRetrieval(record memory.MemoryRetrievalRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.retrievals = append(l.retrievals, record)
}

func (l *MemoryTraceLog) RecordMemorySelection(record memory.MemorySelectionRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.selections = append(l.selections, record)
}

func (l *MemoryTraceLog) QueryRecords() []memory.MemoryQueryRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]memory.MemoryQueryRecord, len(l.queries))
	copy(result, l.queries)
	return result
}

func (l *MemoryTraceLog) RetrievalRecords() []memory.MemoryRetrievalRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]memory.MemoryRetrievalRecord, len(l.retrievals))
	copy(result, l.retrievals)
	return result
}

func (l *MemoryTraceLog) SelectionRecords() []memory.MemorySelectionRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]memory.MemorySelectionRecord, len(l.selections))
	copy(result, l.selections)
	return result
}

func MemoryAccessRecords(entries []memory.MemoryAccessAudit) []memory.MemoryAccessAudit {
	result := make([]memory.MemoryAccessAudit, len(entries))
	copy(result, entries)
	return result
}

