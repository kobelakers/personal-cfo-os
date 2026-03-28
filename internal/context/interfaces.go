package context

import (
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type ContextAssembler interface {
	Assemble(spec taskspec.TaskSpec, currentState state.FinancialWorldState, memories []memory.MemoryRecord, evidence []observation.EvidenceRecord, view ContextView) (ContextSlice, error)
}

type ContextCompactor interface {
	Compact(slice ContextSlice, strategy CompactionStrategy) (ContextSlice, error)
}
