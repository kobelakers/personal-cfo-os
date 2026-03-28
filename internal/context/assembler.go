package context

import (
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type DefaultContextAssembler struct {
	Budget    ContextBudget
	Compactor ContextCompactor
}

func (a DefaultContextAssembler) Assemble(
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	memories []memory.MemoryRecord,
	evidence []observation.EvidenceRecord,
	view ContextView,
) (ContextSlice, error) {
	budget := resolveBudget(a.Budget, view)
	slice := ContextSlice{
		View:           view,
		TaskID:         spec.ID,
		Goal:           spec.Goal,
		Budget:         budget,
		RequiredSkills: requiredSkillsForIntent(spec.UserIntentType),
	}

	stateBlocks, err := selectStateBlocks(currentState, view, budget)
	if err != nil {
		return ContextSlice{}, err
	}
	slice.StateBlocks = stateBlocks

	slice.MemoryBlocks = selectMemoryBlocks(memories, view, budget)
	slice.MemoryIDs = make([]string, 0, len(slice.MemoryBlocks))
	for _, block := range slice.MemoryBlocks {
		slice.MemoryIDs = append(slice.MemoryIDs, block.MemoryID)
	}

	slice.EvidenceBlocks = selectEvidenceBlocks(evidence, view, budget)
	slice.EvidenceIDs = make([]observation.EvidenceID, 0, len(slice.EvidenceBlocks))
	for _, block := range slice.EvidenceBlocks {
		slice.EvidenceIDs = append(slice.EvidenceIDs, block.EvidenceID)
	}

	slice.SkillBlocks = selectSkillBlocks(spec.UserIntentType)
	compactor := a.Compactor
	if compactor == nil {
		compactor = StateAwareCompactor{}
	}
	return compactor.Compact(slice, strategyForView(view))
}

func strategyForView(view ContextView) CompactionStrategy {
	switch view {
	case ContextViewExecution:
		return CompactionStrategyEvidenceFocused
	case ContextViewVerification:
		return CompactionStrategyVerificationLean
	default:
		return CompactionStrategyStateAware
	}
}
