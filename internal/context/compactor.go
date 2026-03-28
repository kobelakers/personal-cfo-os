package context

import "fmt"

type StateAwareCompactor struct{}

func (StateAwareCompactor) Compact(slice ContextSlice, strategy CompactionStrategy) (ContextSlice, error) {
	compacted := slice
	switch strategy {
	case CompactionStrategyNone:
		return compacted, nil
	case CompactionStrategyStateAware, CompactionStrategyEvidenceFocused, CompactionStrategyVerificationLean:
	default:
		return ContextSlice{}, fmt.Errorf("unsupported compaction strategy %q", strategy)
	}

	charBudget := slice.Budget.MaxCharacters
	if charBudget <= 0 {
		charBudget = 2400
	}

	totalChars := 0
	trimStateBlocks := make([]InjectedStateBlock, 0, len(compacted.StateBlocks))
	for _, block := range compacted.StateBlocks {
		totalChars += len(block.DataJSON)
		if totalChars > charBudget {
			break
		}
		trimStateBlocks = append(trimStateBlocks, block)
	}
	compacted.StateBlocks = trimStateBlocks

	if strategy == CompactionStrategyVerificationLean && len(compacted.MemoryBlocks) > 1 {
		compacted.MemoryBlocks = compacted.MemoryBlocks[:1]
	}
	if strategy == CompactionStrategyEvidenceFocused && len(compacted.StateBlocks) > 3 {
		compacted.StateBlocks = compacted.StateBlocks[:3]
	}
	compacted.Compacted = true
	return compacted, nil
}
