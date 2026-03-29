package observability

import (
	"sync"

	"github.com/kobelakers/personal-cfo-os/internal/prompt"
)

type PromptTraceRecord = prompt.PromptRenderTrace

type PromptTraceLog struct {
	mu      sync.Mutex
	records []prompt.PromptRenderTrace
}

func (l *PromptTraceLog) RecordPromptRender(trace prompt.PromptRenderTrace) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, trace)
}

func (l *PromptTraceLog) Records() []prompt.PromptRenderTrace {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]prompt.PromptRenderTrace, len(l.records))
	copy(result, l.records)
	return result
}
