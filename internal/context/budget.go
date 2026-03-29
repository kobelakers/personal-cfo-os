package context

func DefaultBudgetForView(view ContextView) ContextBudget {
	switch view {
	case ContextViewPlanning:
		return ContextBudget{MaxStateBlocks: 4, MaxMemoryBlocks: 3, MaxEvidenceItems: 4, MaxCharacters: 2200, MaxInputTokens: 1800, ReservedOutputTokens: 700, HardTokenLimit: 2200}
	case ContextViewExecution:
		return ContextBudget{MaxStateBlocks: 6, MaxMemoryBlocks: 4, MaxEvidenceItems: 6, MaxCharacters: 3200, MaxInputTokens: 2200, ReservedOutputTokens: 800, HardTokenLimit: 2800}
	case ContextViewVerification:
		return ContextBudget{MaxStateBlocks: 4, MaxMemoryBlocks: 2, MaxEvidenceItems: 6, MaxCharacters: 2400, MaxInputTokens: 1600, ReservedOutputTokens: 400, HardTokenLimit: 2000}
	default:
		return ContextBudget{MaxStateBlocks: 4, MaxMemoryBlocks: 3, MaxEvidenceItems: 4, MaxCharacters: 2200, MaxInputTokens: 1800, ReservedOutputTokens: 700, HardTokenLimit: 2200}
	}
}

func resolveBudget(explicit ContextBudget, view ContextView) ContextBudget {
	if explicit.MaxStateBlocks != 0 || explicit.MaxMemoryBlocks != 0 || explicit.MaxEvidenceItems != 0 || explicit.MaxCharacters != 0 {
		return explicit
	}
	return DefaultBudgetForView(view)
}
