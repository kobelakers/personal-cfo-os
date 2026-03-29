package context

import (
	"fmt"
	"strings"
)

type StateAwareCompactor struct {
	Estimator TokenEstimator
}

func (c StateAwareCompactor) Compact(slice ContextSlice, strategy CompactionStrategy) (ContextSlice, error) {
	compacted := slice
	switch strategy {
	case CompactionStrategyNone:
		return compacted, nil
	case CompactionStrategyStateAware, CompactionStrategyEvidenceFocused, CompactionStrategyVerificationLean:
	default:
		return ContextSlice{}, fmt.Errorf("unsupported compaction strategy %q", strategy)
	}

	estimator := c.Estimator
	if estimator == nil {
		estimator = HeuristicTokenEstimator{}
	}
	if strategy == CompactionStrategyVerificationLean && len(compacted.MemoryBlocks) > 1 {
		compacted.MemoryBlocks = compacted.MemoryBlocks[:1]
	}
	if strategy == CompactionStrategyEvidenceFocused && len(compacted.StateBlocks) > 3 {
		compacted.StateBlocks = compacted.StateBlocks[:3]
	}

	initialTokens := estimateSliceTokens(compacted, estimator)
	targetTokens := tokenTarget(compacted.Budget)
	excluded := make([]ContextBlockDecision, 0)
	notes := make([]string, 0)
	if targetTokens > 0 && initialTokens > targetTokens {
		var reduced ContextSlice
		reduced, excluded, notes = excludeBlocksToFitBudget(compacted, strategy, targetTokens, estimator)
		compacted = reduced
	} else if targetTokens == 0 && compacted.Budget.MaxCharacters > 0 {
		charBudget := compacted.Budget.MaxCharacters
		totalChars := 0
		trimStateBlocks := make([]InjectedStateBlock, 0, len(compacted.StateBlocks))
		for _, block := range compacted.StateBlocks {
			totalChars += len(block.DataJSON)
			if totalChars > charBudget {
				excluded = append(excluded, ContextBlockDecision{
					Source:          ContextBlockSourceState,
					Ref:             block.Name,
					EstimatedTokens: estimateStateBlockTokens(block, estimator),
					Reason:          "trimmed_to_fit_character_budget",
				})
				notes = append(notes, "trimmed state block "+block.Name+" to fit legacy character budget")
				continue
			}
			trimStateBlocks = append(trimStateBlocks, block)
		}
		compacted.StateBlocks = trimStateBlocks
	}
	compacted.BudgetDecision = buildBudgetDecision(compacted, excluded, targetTokens, estimator)
	compacted.Compaction = ContextCompactionResult{
		Strategy:             strategy,
		InitialTokenEstimate: initialTokens,
		FinalTokenEstimate:   compacted.BudgetDecision.EstimatedInputTokens,
		Notes:                notes,
	}
	compacted.Compacted = true
	return compacted, nil
}

func estimateSliceTokens(slice ContextSlice, estimator TokenEstimator) int {
	total := 0
	for _, block := range slice.StateBlocks {
		total += estimateStateBlockTokens(block, estimator)
	}
	for _, block := range slice.MemoryBlocks {
		total += estimateMemoryBlockTokens(block, estimator)
	}
	for _, block := range slice.EvidenceBlocks {
		total += estimateEvidenceBlockTokens(block, estimator)
	}
	for _, block := range slice.SkillBlocks {
		total += estimateSkillBlockTokens(block, estimator)
	}
	return total
}

func estimateStateBlockTokens(block InjectedStateBlock, estimator TokenEstimator) int {
	return estimator.EstimateText(block.Name + "\n" + block.DataJSON)
}

func estimateMemoryBlockTokens(block MemoryBlock, estimator TokenEstimator) int {
	return estimator.EstimateText(block.MemoryID + "\n" + block.Summary)
}

func estimateEvidenceBlockTokens(block EvidenceSummaryBlock, estimator TokenEstimator) int {
	return estimator.EstimateText(string(block.EvidenceID) + "\n" + block.Summary)
}

func estimateSkillBlockTokens(block SkillBlock, estimator TokenEstimator) int {
	return estimator.EstimateText(block.SkillName + "\n" + block.Description)
}

func excludeBlocksToFitBudget(slice ContextSlice, strategy CompactionStrategy, targetTokens int, estimator TokenEstimator) (ContextSlice, []ContextBlockDecision, []string) {
	compacted := slice
	excluded := make([]ContextBlockDecision, 0)
	notes := make([]string, 0)
	for estimateSliceTokens(compacted, estimator) > targetTokens {
		switch {
		case len(compacted.SkillBlocks) > 0:
			block := compacted.SkillBlocks[len(compacted.SkillBlocks)-1]
			compacted.SkillBlocks = compacted.SkillBlocks[:len(compacted.SkillBlocks)-1]
			excluded = append(excluded, ContextBlockDecision{Source: ContextBlockSourceSkill, Ref: block.SkillName, EstimatedTokens: estimateSkillBlockTokens(block, estimator), Reason: "trimmed_to_fit_token_budget"})
			notes = append(notes, "trimmed skill block "+block.SkillName+" to fit token budget")
		case len(compacted.MemoryBlocks) > 0 && strategy != CompactionStrategyEvidenceFocused:
			block := compacted.MemoryBlocks[len(compacted.MemoryBlocks)-1]
			compacted.MemoryBlocks = compacted.MemoryBlocks[:len(compacted.MemoryBlocks)-1]
			compacted.MemoryIDs = collectMemoryIDs(compacted.MemoryBlocks)
			excluded = append(excluded, ContextBlockDecision{Source: ContextBlockSourceMemory, Ref: block.MemoryID, EstimatedTokens: estimateMemoryBlockTokens(block, estimator), Reason: "trimmed_to_fit_token_budget"})
			notes = append(notes, "trimmed memory block "+block.MemoryID+" to fit token budget")
		case len(compacted.StateBlocks) > 1:
			block := compacted.StateBlocks[len(compacted.StateBlocks)-1]
			compacted.StateBlocks = compacted.StateBlocks[:len(compacted.StateBlocks)-1]
			excluded = append(excluded, ContextBlockDecision{Source: ContextBlockSourceState, Ref: block.Name, EstimatedTokens: estimateStateBlockTokens(block, estimator), Reason: "trimmed_to_fit_token_budget"})
			notes = append(notes, "trimmed state block "+block.Name+" to fit token budget")
		case len(compacted.EvidenceBlocks) > 1:
			block := compacted.EvidenceBlocks[len(compacted.EvidenceBlocks)-1]
			compacted.EvidenceBlocks = compacted.EvidenceBlocks[:len(compacted.EvidenceBlocks)-1]
			compacted.EvidenceIDs = collectEvidenceIDs(compacted.EvidenceBlocks)
			excluded = append(excluded, ContextBlockDecision{Source: ContextBlockSourceEvidence, Ref: string(block.EvidenceID), EstimatedTokens: estimateEvidenceBlockTokens(block, estimator), Reason: "trimmed_to_fit_token_budget"})
			notes = append(notes, "trimmed evidence block "+string(block.EvidenceID)+" to fit token budget")
		case len(compacted.MemoryBlocks) > 0:
			block := compacted.MemoryBlocks[len(compacted.MemoryBlocks)-1]
			compacted.MemoryBlocks = compacted.MemoryBlocks[:len(compacted.MemoryBlocks)-1]
			compacted.MemoryIDs = collectMemoryIDs(compacted.MemoryBlocks)
			excluded = append(excluded, ContextBlockDecision{Source: ContextBlockSourceMemory, Ref: block.MemoryID, EstimatedTokens: estimateMemoryBlockTokens(block, estimator), Reason: "trimmed_to_fit_token_budget"})
			notes = append(notes, "trimmed memory block "+block.MemoryID+" to fit token budget")
		default:
			return compacted, excluded, append(notes, "token budget exhausted; no additional block can be safely trimmed")
		}
	}
	return compacted, excluded, notes
}

func buildBudgetDecision(slice ContextSlice, excluded []ContextBlockDecision, targetTokens int, estimator TokenEstimator) ContextBudgetDecision {
	decision := ContextBudgetDecision{
		EstimatedInputTokens: estimateSliceTokens(slice, estimator),
		TargetInputTokens:    targetTokens,
		Included:             make([]ContextBlockDecision, 0, len(slice.StateBlocks)+len(slice.MemoryBlocks)+len(slice.EvidenceBlocks)+len(slice.SkillBlocks)),
		Excluded:             excluded,
	}
	for _, block := range slice.StateBlocks {
		decision.Included = append(decision.Included, ContextBlockDecision{Source: ContextBlockSourceState, Ref: block.Name, EstimatedTokens: estimateStateBlockTokens(block, estimator), Reason: string(block.SelectionReason)})
	}
	for _, block := range slice.MemoryBlocks {
		decision.Included = append(decision.Included, ContextBlockDecision{Source: ContextBlockSourceMemory, Ref: block.MemoryID, EstimatedTokens: estimateMemoryBlockTokens(block, estimator), Reason: string(block.SelectionReason)})
	}
	for _, block := range slice.EvidenceBlocks {
		decision.Included = append(decision.Included, ContextBlockDecision{Source: ContextBlockSourceEvidence, Ref: string(block.EvidenceID), EstimatedTokens: estimateEvidenceBlockTokens(block, estimator), Reason: string(block.SelectionReason)})
	}
	for _, block := range slice.SkillBlocks {
		decision.Included = append(decision.Included, ContextBlockDecision{Source: ContextBlockSourceSkill, Ref: block.SkillName, EstimatedTokens: estimateSkillBlockTokens(block, estimator), Reason: string(block.SelectionReason)})
	}
	return decision
}

func ExcludedBlockRefs(slice ContextSlice) []string {
	refs := make([]string, 0, len(slice.BudgetDecision.Excluded))
	for _, item := range slice.BudgetDecision.Excluded {
		refs = append(refs, string(item.Source)+":"+item.Ref)
	}
	return refs
}

func CompactionNotes(slice ContextSlice) []string {
	if len(slice.Compaction.Notes) == 0 {
		return nil
	}
	return append([]string{}, slice.Compaction.Notes...)
}

func SkillNames(slice ContextSlice) []string {
	result := make([]string, 0, len(slice.SkillBlocks))
	for _, block := range slice.SkillBlocks {
		result = append(result, block.SkillName)
	}
	return result
}

func ContextSummary(slice ContextSlice) string {
	parts := make([]string, 0, len(slice.StateBlocks)+len(slice.MemoryBlocks)+len(slice.EvidenceBlocks)+len(slice.SkillBlocks))
	for _, block := range slice.StateBlocks {
		parts = append(parts, "[state] "+block.Name+": "+block.DataJSON)
	}
	for _, block := range slice.MemoryBlocks {
		parts = append(parts, "[memory] "+block.MemoryID+": "+block.Summary)
	}
	for _, block := range slice.EvidenceBlocks {
		parts = append(parts, "[evidence] "+string(block.EvidenceID)+": "+block.Summary)
	}
	for _, block := range slice.SkillBlocks {
		parts = append(parts, "[skill] "+block.SkillName+": "+block.Description)
	}
	return strings.Join(parts, "\n")
}
