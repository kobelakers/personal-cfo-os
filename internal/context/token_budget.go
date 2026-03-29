package context

func tokenBudgetFor(budget ContextBudget) TokenBudget {
	return TokenBudget{
		MaxInputTokens:       budget.MaxInputTokens,
		ReservedOutputTokens: budget.ReservedOutputTokens,
		HardTokenLimit:       budget.HardTokenLimit,
	}
}

func tokenTarget(budget ContextBudget) int {
	target := budget.MaxInputTokens
	if target <= 0 {
		return 0
	}
	if budget.ReservedOutputTokens > 0 {
		target -= budget.ReservedOutputTokens
	}
	if budget.HardTokenLimit > 0 && target > budget.HardTokenLimit {
		target = budget.HardTokenLimit
	}
	if target < 0 {
		target = 0
	}
	return target
}
