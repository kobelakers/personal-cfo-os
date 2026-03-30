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
	case taskspec.UserIntentBehaviorIntervention:
		return []string{"behavior_intervention"}
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
