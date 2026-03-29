package context

import (
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type BlockVerificationContext struct {
	View                 ContextView              `json:"view"`
	PlanID               string                   `json:"plan_id"`
	BlockID              string                   `json:"block_id"`
	BlockKind            string                   `json:"block_kind"`
	SelectionReason      ContextSelectionReason   `json:"selection_reason"`
	SelectedMemoryIDs    []string                 `json:"selected_memory_ids,omitempty"`
	SelectedEvidenceIDs  []observation.EvidenceID `json:"selected_evidence_ids,omitempty"`
	SelectedStateBlocks  []string                 `json:"selected_state_blocks,omitempty"`
	EstimatedInputTokens int                      `json:"estimated_input_tokens,omitempty"`
	VerificationRules    []string                 `json:"verification_rules,omitempty"`
	ResultSummary        string                   `json:"result_summary"`
	Slice                ContextSlice             `json:"slice"`
}

type VerificationContextAssembler struct {
	Budget    ContextBudget
	Compactor ContextCompactor
	Estimator TokenEstimator
}

func (a VerificationContextAssembler) AssembleBlock(
	spec BlockContextSpec,
	result analysis.BlockResultEnvelope,
	currentState state.FinancialWorldState,
	memories []memory.MemoryRecord,
	evidence []observation.EvidenceRecord,
) (BlockVerificationContext, error) {
	budget := resolveBudget(a.Budget, ContextViewVerification)
	stateBlocks, err := selectStateBlocks(currentState, ContextViewVerification, budget)
	if err != nil {
		return BlockVerificationContext{}, err
	}
	stateBlocks = filterStateBlocksByNames(stateBlocks, spec.RequiredStateBlocks)
	memoryBlocks := filterMemoryBlocksForSpec(selectMemoryBlocks(memories, ContextViewVerification, budget), spec)
	evidenceBlocks := filterEvidenceBlocksByTypes(selectEvidenceBlocks(filterEvidenceRecordsByIDs(evidence, result.EvidenceIDs()), ContextViewVerification, budget), spec.RequiredEvidenceRefs)

	slice := ContextSlice{
		View:           ContextViewVerification,
		TaskID:         spec.BlockID,
		Goal:           spec.Goal,
		Budget:         budget,
		TokenBudget:    tokenBudgetFor(budget),
		StateBlocks:    stateBlocks,
		MemoryBlocks:   memoryBlocks,
		EvidenceBlocks: evidenceBlocks,
	}
	slice.MemoryIDs = collectMemoryIDs(memoryBlocks)
	slice.EvidenceIDs = collectEvidenceIDs(evidenceBlocks)
	compactor := a.Compactor
	if compactor == nil {
		compactor = StateAwareCompactor{Estimator: a.Estimator}
	}
	compacted, err := compactor.Compact(slice, CompactionStrategyVerificationLean)
	if err != nil {
		return BlockVerificationContext{}, err
	}
	if len(compacted.EvidenceIDs) == 0 {
		return BlockVerificationContext{}, fmt.Errorf("verification context for block %q requires selected evidence", spec.BlockID)
	}
	return BlockVerificationContext{
		View:                 ContextViewVerification,
		PlanID:               spec.PlanID,
		BlockID:              spec.BlockID,
		BlockKind:            spec.BlockKind,
		SelectionReason:      ContextSelectionVerificationGap,
		SelectedMemoryIDs:    compacted.MemoryIDs,
		SelectedEvidenceIDs:  compacted.EvidenceIDs,
		SelectedStateBlocks:  stateBlockNames(compacted.StateBlocks),
		EstimatedInputTokens: compacted.BudgetDecision.EstimatedInputTokens,
		VerificationRules:    spec.VerificationRules,
		ResultSummary:        result.Summary(),
		Slice:                compacted,
	}, nil
}

func (a VerificationContextAssembler) AssembleFinal(
	planID string,
	reportSummary string,
	currentState state.FinancialWorldState,
	memories []memory.MemoryRecord,
	evidence []observation.EvidenceRecord,
) (BlockVerificationContext, error) {
	budget := resolveBudget(a.Budget, ContextViewVerification)
	stateBlocks, err := selectStateBlocks(currentState, ContextViewVerification, budget)
	if err != nil {
		return BlockVerificationContext{}, err
	}
	slice := ContextSlice{
		View:           ContextViewVerification,
		TaskID:         planID,
		Goal:           "final_report_verification",
		Budget:         budget,
		TokenBudget:    tokenBudgetFor(budget),
		StateBlocks:    stateBlocks,
		MemoryBlocks:   selectMemoryBlocks(memories, ContextViewVerification, budget),
		EvidenceBlocks: selectEvidenceBlocks(evidence, ContextViewVerification, budget),
	}
	slice.MemoryIDs = collectMemoryIDs(slice.MemoryBlocks)
	slice.EvidenceIDs = collectEvidenceIDs(slice.EvidenceBlocks)
	compactor := a.Compactor
	if compactor == nil {
		compactor = StateAwareCompactor{Estimator: a.Estimator}
	}
	compacted, err := compactor.Compact(slice, CompactionStrategyVerificationLean)
	if err != nil {
		return BlockVerificationContext{}, err
	}
	return BlockVerificationContext{
		View:                 ContextViewVerification,
		PlanID:               planID,
		BlockID:              "final-report",
		BlockKind:            "final_report",
		SelectionReason:      ContextSelectionVerificationGap,
		SelectedMemoryIDs:    compacted.MemoryIDs,
		SelectedEvidenceIDs:  compacted.EvidenceIDs,
		SelectedStateBlocks:  stateBlockNames(compacted.StateBlocks),
		EstimatedInputTokens: compacted.BudgetDecision.EstimatedInputTokens,
		VerificationRules:    []string{"final_report_schema", "final_report_grounding"},
		ResultSummary:        reportSummary,
		Slice:                compacted,
	}, nil
}

func filterEvidenceRecordsByIDs(records []observation.EvidenceRecord, ids []observation.EvidenceID) []observation.EvidenceRecord {
	if len(ids) == 0 {
		return records
	}
	allowed := make(map[observation.EvidenceID]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	filtered := make([]observation.EvidenceRecord, 0, len(records))
	for _, record := range records {
		if _, ok := allowed[record.ID]; ok {
			filtered = append(filtered, record)
		}
	}
	return filtered
}
