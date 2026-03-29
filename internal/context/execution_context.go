package context

import (
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type BlockExecutionContext struct {
	View                ContextView              `json:"view"`
	PlanID              string                   `json:"plan_id"`
	BlockID             string                   `json:"block_id"`
	BlockKind           string                   `json:"block_kind"`
	AssignedRecipient   string                   `json:"assigned_recipient"`
	Goal                string                   `json:"goal"`
	SelectionReason     ContextSelectionReason   `json:"selection_reason"`
	SelectedMemoryIDs   []string                 `json:"selected_memory_ids,omitempty"`
	SelectedEvidenceIDs []observation.EvidenceID `json:"selected_evidence_ids,omitempty"`
	SelectedStateBlocks []string                 `json:"selected_state_blocks,omitempty"`
	Slice               ContextSlice             `json:"slice"`
}

type ExecutionContextAssembler struct {
	Budget    ContextBudget
	Compactor ContextCompactor
}

func (a ExecutionContextAssembler) Assemble(
	spec BlockContextSpec,
	currentState state.FinancialWorldState,
	memories []memory.MemoryRecord,
	evidence []observation.EvidenceRecord,
) (BlockExecutionContext, error) {
	view := spec.ExecutionView
	if view == "" {
		view = ContextViewExecution
	}
	budget := resolveBudget(a.Budget, view)

	stateBlocks, err := selectStateBlocks(currentState, view, budget)
	if err != nil {
		return BlockExecutionContext{}, err
	}
	stateBlocks = filterStateBlocksByNames(stateBlocks, spec.RequiredStateBlocks)
	memoryBlocks := filterMemoryBlocksForSpec(selectMemoryBlocks(memories, view, budget), spec)
	evidenceBlocks := filterEvidenceBlocksByTypes(selectEvidenceBlocks(evidence, view, budget), spec.RequiredEvidenceRefs)

	slice := ContextSlice{
		View:           view,
		TaskID:         spec.BlockID,
		Goal:           spec.Goal,
		Budget:         budget,
		StateBlocks:    stateBlocks,
		MemoryBlocks:   memoryBlocks,
		EvidenceBlocks: evidenceBlocks,
		RequiredSkills: nil,
	}
	slice.MemoryIDs = collectMemoryIDs(memoryBlocks)
	slice.EvidenceIDs = collectEvidenceIDs(evidenceBlocks)
	compactor := a.Compactor
	if compactor == nil {
		compactor = StateAwareCompactor{}
	}
	compacted, err := compactor.Compact(slice, CompactionStrategyEvidenceFocused)
	if err != nil {
		return BlockExecutionContext{}, err
	}
	result := BlockExecutionContext{
		View:                view,
		PlanID:              spec.PlanID,
		BlockID:             spec.BlockID,
		BlockKind:           spec.BlockKind,
		AssignedRecipient:   spec.AssignedRecipient,
		Goal:                spec.Goal,
		SelectionReason:     blockSelectionReason(spec.BlockKind),
		SelectedMemoryIDs:   compacted.MemoryIDs,
		SelectedEvidenceIDs: compacted.EvidenceIDs,
		SelectedStateBlocks: stateBlockNames(compacted.StateBlocks),
		Slice:               compacted,
	}
	if len(result.SelectedEvidenceIDs) == 0 {
		return BlockExecutionContext{}, fmt.Errorf("execution context for block %q requires selected evidence", spec.BlockID)
	}
	return result, nil
}

func collectMemoryIDs(blocks []MemoryBlock) []string {
	result := make([]string, 0, len(blocks))
	for _, block := range blocks {
		result = append(result, block.MemoryID)
	}
	return result
}

func collectEvidenceIDs(blocks []EvidenceSummaryBlock) []observation.EvidenceID {
	result := make([]observation.EvidenceID, 0, len(blocks))
	for _, block := range blocks {
		result = append(result, block.EvidenceID)
	}
	return result
}

func stateBlockNames(blocks []InjectedStateBlock) []string {
	result := make([]string, 0, len(blocks))
	for _, block := range blocks {
		result = append(result, block.Name)
	}
	return result
}

func blockSelectionReason(blockKind string) ContextSelectionReason {
	switch {
	case blockKind == "cashflow_review_block" || blockKind == "cashflow_liquidity_block" || blockKind == "cashflow_event_impact_block":
		return ContextSelectionRequiredEvidence
	case blockKind == "debt_review_block" || blockKind == "debt_tradeoff_block" || blockKind == "debt_housing_impact_block":
		return ContextSelectionRiskSignal
	case blockKind == "tax_event_impact_block" || blockKind == "portfolio_event_impact_block":
		return ContextSelectionMemoryRelevance
	default:
		return ContextSelectionRecentState
	}
}
