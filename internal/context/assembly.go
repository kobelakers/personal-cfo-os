package context

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type DefaultContextAssembler struct {
	Budget ContextBudget
}

func (a DefaultContextAssembler) Assemble(
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	memories []memory.MemoryRecord,
	evidence []observation.EvidenceRecord,
	view ContextView,
) (ContextSlice, error) {
	budget := a.budgetForView(view)
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
	compacted, err := StateAwareCompactor{}.Compact(slice, CompactionStrategyStateAware)
	if err != nil {
		return ContextSlice{}, err
	}
	return compacted, nil
}

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

func (a DefaultContextAssembler) budgetForView(view ContextView) ContextBudget {
	if a.Budget.MaxStateBlocks != 0 || a.Budget.MaxMemoryBlocks != 0 || a.Budget.MaxEvidenceItems != 0 || a.Budget.MaxCharacters != 0 {
		return a.Budget
	}
	switch view {
	case ContextViewPlanning:
		return ContextBudget{MaxStateBlocks: 4, MaxMemoryBlocks: 3, MaxEvidenceItems: 4, MaxCharacters: 2200}
	case ContextViewExecution:
		return ContextBudget{MaxStateBlocks: 6, MaxMemoryBlocks: 4, MaxEvidenceItems: 6, MaxCharacters: 3200}
	case ContextViewVerification:
		return ContextBudget{MaxStateBlocks: 4, MaxMemoryBlocks: 2, MaxEvidenceItems: 6, MaxCharacters: 2400}
	default:
		return ContextBudget{MaxStateBlocks: 4, MaxMemoryBlocks: 3, MaxEvidenceItems: 4, MaxCharacters: 2200}
	}
}

func selectStateBlocks(currentState state.FinancialWorldState, view ContextView, budget ContextBudget) ([]InjectedStateBlock, error) {
	blocks := make([]InjectedStateBlock, 0, budget.MaxStateBlocks)
	type candidate struct {
		name   string
		data   any
		reason ContextSelectionReason
	}

	var candidates []candidate
	switch view {
	case ContextViewPlanning:
		candidates = []candidate{
			{name: "cashflow_state", data: currentState.CashflowState, reason: ContextSelectionRequiredEvidence},
			{name: "liability_state", data: currentState.LiabilityState, reason: ContextSelectionRiskSignal},
			{name: "risk_state", data: currentState.RiskState, reason: ContextSelectionRiskSignal},
			{name: "workflow_state", data: currentState.WorkflowState, reason: ContextSelectionRecentState},
		}
	case ContextViewExecution:
		candidates = []candidate{
			{name: "cashflow_state", data: currentState.CashflowState, reason: ContextSelectionRequiredEvidence},
			{name: "liability_state", data: currentState.LiabilityState, reason: ContextSelectionRequiredEvidence},
			{name: "portfolio_state", data: currentState.PortfolioState, reason: ContextSelectionRequiredEvidence},
			{name: "tax_state", data: currentState.TaxState, reason: ContextSelectionRequiredEvidence},
			{name: "behavior_state", data: currentState.BehaviorState, reason: ContextSelectionRiskSignal},
			{name: "risk_state", data: currentState.RiskState, reason: ContextSelectionRiskSignal},
		}
	case ContextViewVerification:
		candidates = []candidate{
			{name: "workflow_state", data: currentState.WorkflowState, reason: ContextSelectionVerificationGap},
			{name: "risk_state", data: currentState.RiskState, reason: ContextSelectionRiskSignal},
			{name: "cashflow_state", data: currentState.CashflowState, reason: ContextSelectionVerificationGap},
			{name: "tax_state", data: currentState.TaxState, reason: ContextSelectionVerificationGap},
		}
	default:
		return nil, fmt.Errorf("unsupported context view %q", view)
	}

	for _, item := range candidates {
		payload, err := json.Marshal(item.data)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, InjectedStateBlock{
			Name:            item.name,
			Version:         currentState.Version.Sequence,
			DataJSON:        string(payload),
			Source:          "financial_world_state",
			Mandatory:       true,
			BlockSource:     ContextBlockSourceState,
			SelectionReason: item.reason,
		})
		if budget.MaxStateBlocks > 0 && len(blocks) >= budget.MaxStateBlocks {
			break
		}
	}
	return blocks, nil
}

func selectMemoryBlocks(memories []memory.MemoryRecord, view ContextView, budget ContextBudget) []MemoryBlock {
	blocks := make([]MemoryBlock, 0, budget.MaxMemoryBlocks)
	for _, item := range memories {
		reason := ContextSelectionMemoryRelevance
		if view == ContextViewVerification && item.Kind == memory.MemoryKindPolicy {
			reason = ContextSelectionVerificationGap
		}
		blocks = append(blocks, MemoryBlock{
			MemoryID:        item.ID,
			Kind:            item.Kind,
			Summary:         item.Summary,
			BlockSource:     ContextBlockSourceMemory,
			SelectionReason: reason,
		})
		if budget.MaxMemoryBlocks > 0 && len(blocks) >= budget.MaxMemoryBlocks {
			break
		}
	}
	return blocks
}

func selectEvidenceBlocks(evidence []observation.EvidenceRecord, view ContextView, budget ContextBudget) []EvidenceSummaryBlock {
	blocks := make([]EvidenceSummaryBlock, 0, budget.MaxEvidenceItems)
	for _, item := range evidence {
		reason := ContextSelectionRequiredEvidence
		if view == ContextViewVerification {
			reason = ContextSelectionVerificationGap
		}
		blocks = append(blocks, EvidenceSummaryBlock{
			EvidenceID:      item.ID,
			Type:            item.Type,
			Summary:         compactSummary(item.Summary, 180),
			BlockSource:     ContextBlockSourceEvidence,
			SelectionReason: reason,
		})
		if budget.MaxEvidenceItems > 0 && len(blocks) >= budget.MaxEvidenceItems {
			break
		}
	}
	return blocks
}

func selectSkillBlocks(intent taskspec.UserIntentType) []SkillBlock {
	skills := requiredSkillsForIntent(intent)
	blocks := make([]SkillBlock, 0, len(skills))
	for _, skill := range skills {
		blocks = append(blocks, SkillBlock{
			SkillName:       skill,
			Description:     "skill injected because it matches task intent and output contract",
			BlockSource:     ContextBlockSourceSkill,
			SelectionReason: ContextSelectionSkillRequirement,
		})
	}
	return blocks
}

func requiredSkillsForIntent(intent taskspec.UserIntentType) []string {
	switch intent {
	case taskspec.UserIntentMonthlyReview:
		return []string{"monthly_review"}
	case taskspec.UserIntentDebtVsInvest:
		return []string{"debt_optimization"}
	default:
		return nil
	}
}

func compactSummary(summary string, maxLen int) string {
	summary = strings.TrimSpace(summary)
	if len(summary) <= maxLen {
		return summary
	}
	return summary[:maxLen] + "..."
}
