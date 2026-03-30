package skills

import (
	"fmt"
	"strings"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type SelectionInput struct {
	Task                 taskspec.TaskSpec
	AllowedFamilies      []string
	SelectionHint        string
	CurrentState         state.FinancialWorldState
	Evidence             []observation.EvidenceRecord
	ProceduralMemories   []ProceduralMemoryContextRecord
}

type SkillSelector interface {
	Select(input SelectionInput) (SkillSelection, error)
}

type DeterministicBehaviorSkillSelector struct {
	Catalog SkillCatalog
}

func (s DeterministicBehaviorSkillSelector) Select(input SelectionInput) (SkillSelection, error) {
	if s.Catalog == nil {
		return SkillSelection{}, fmt.Errorf("skill selector requires catalog")
	}
	allowed := normalizeFamilies(input.AllowedFamilies)
	chosen := SkillFamilyDiscretionaryGuardrail
	reasons := make([]SkillSelectionReason, 0, 4)
	stateRefs := []string{"behavior_state", "cashflow_state"}
	evidenceRefs := evidenceRefs(input.Evidence)
	memoryRefs := proceduralMemoryRefs(input.ProceduralMemories)
	policyRefs := make([]string, 0)
	lowerHint := strings.ToLower(strings.TrimSpace(input.SelectionHint))
	repeatedGuardrail := hasGuardrailProceduralMemory(input.ProceduralMemories)

	if strings.Contains(lowerHint, "subscription") && familyAllowed(allowed, SkillFamilySubscriptionCleanup) {
		chosen = SkillFamilySubscriptionCleanup
		reasons = append(reasons, SkillSelectionReason{
			Code:         "selection_hint_subscription",
			Detail:       "the explicit intervention request focused on subscriptions, so the selector chose the subscription cleanup family",
			EvidenceRefs: evidenceRefs,
			StateRefs:    stateRefs,
		})
	} else if strings.Contains(lowerHint, "guardrail") || strings.Contains(lowerHint, "hard_cap") || strings.Contains(lowerHint, "spending_cap") {
		chosen = SkillFamilyDiscretionaryGuardrail
		reasons = append(reasons, SkillSelectionReason{
			Code:         "selection_hint_guardrail",
			Detail:       "the explicit intervention request focused on a spending guardrail, so the selector chose the discretionary guardrail family",
			EvidenceRefs: evidenceRefs,
			StateRefs:    stateRefs,
		})
	} else if strings.Contains(lowerHint, "late_night") && familyAllowed(allowed, SkillFamilyLateNightSpendNudge) {
		chosen = SkillFamilyLateNightSpendNudge
		reasons = append(reasons, SkillSelectionReason{
			Code:         "selection_hint_late_night",
			Detail:       "the explicit intervention request focused on late-night spending, so the selector chose the late-night nudge family",
			EvidenceRefs: evidenceRefs,
			StateRefs:    stateRefs,
		})
	} else if repeatedGuardrail && familyAllowed(allowed, SkillFamilyDiscretionaryGuardrail) {
		chosen = SkillFamilyDiscretionaryGuardrail
		reasons = append(reasons, SkillSelectionReason{
			Code:       "procedural_memory_repeat_anomaly",
			Detail:     "retrieved procedural memory shows a prior guardrail already ran against a similar anomaly, so the selector kept the guardrail family and escalated the intervention",
			MemoryRefs: memoryRefs,
			StateRefs:  stateRefs,
		})
	} else if input.CurrentState.BehaviorState.DuplicateSubscriptionCount >= 2 && familyAllowed(allowed, SkillFamilySubscriptionCleanup) {
		chosen = SkillFamilySubscriptionCleanup
		reasons = append(reasons, SkillSelectionReason{
			Code:         "duplicate_subscriptions",
			Detail:       "duplicate subscriptions are the dominant anomaly, so subscription cleanup takes priority",
			EvidenceRefs: evidenceRefs,
			StateRefs:    stateRefs,
		})
	} else if input.CurrentState.BehaviorState.LateNightSpendingFrequency >= 0.30 && familyAllowed(allowed, SkillFamilyLateNightSpendNudge) {
		chosen = SkillFamilyLateNightSpendNudge
		reasons = append(reasons, SkillSelectionReason{
			Code:         "late_night_spike",
			Detail:       "late-night discretionary spend spike dominates, so the late-night nudge skill is selected",
			EvidenceRefs: evidenceRefs,
			StateRefs:    stateRefs,
		})
	} else if familyAllowed(allowed, SkillFamilyDiscretionaryGuardrail) {
		chosen = SkillFamilyDiscretionaryGuardrail
		reasons = append(reasons, SkillSelectionReason{
			Code:         "discretionary_pressure",
			Detail:       "discretionary pressure is the dominant signal, so the guardrail family is selected",
			EvidenceRefs: evidenceRefs,
			StateRefs:    stateRefs,
		})
	}

	manifest, ok := s.Catalog.Latest(chosen)
	if !ok {
		return SkillSelection{}, fmt.Errorf("missing default manifest for family %s", chosen)
	}
	recipeID := defaultRecipeID(chosen)
	if strings.Contains(strings.ToLower(input.SelectionHint), "hard_cap") {
		recipeID = "hard_cap.v1"
	}
	if chosen == SkillFamilyDiscretionaryGuardrail {
		recipeID = pickGuardrailRecipe(input)
		if recipeID == "budget_guardrail.v1" && len(memoryRefs) > 0 {
			reasons = append(reasons, SkillSelectionReason{
				Code:       "procedural_memory_escalation",
				Detail:     "retrieved procedural memory shows a prior lower-intensity guardrail already ran against a similar anomaly, so the selector escalated to budget_guardrail.v1",
				MemoryRefs: memoryRefs,
				StateRefs:  stateRefs,
			})
		}
		if recipeID == "hard_cap.v1" {
			policyRefs = append(policyRefs, "behavior.guardrail.hard_cap.approval_required")
			reasons = append(reasons, SkillSelectionReason{
				Code:           "high_intensity_guardrail",
				Detail:         "repeated discretionary-pressure anomaly plus prior procedural memory escalated the recipe to hard_cap.v1",
				MemoryRefs:     memoryRefs,
				StateRefs:      append([]string{}, stateRefs...),
				PolicyRuleRefs: []string{"behavior.guardrail.hard_cap.approval_required"},
			})
		}
	}
	recipe, ok := recipeForID(manifest, recipeID)
	if !ok {
		return SkillSelection{}, fmt.Errorf("selected recipe %q missing in manifest %s:%s", recipeID, manifest.Family, manifest.Version)
	}
	policyRefs = append(policyRefs, recipe.PolicyRuleRefs...)
	policyRefs = append(policyRefs, manifest.Policy.PolicyRuleRefs...)
	return SkillSelection{
		Family:                chosen,
		Version:               manifest.Version,
		RecipeID:              recipe.ID,
		Reasons:               reasons,
		EvidenceRefs:          evidenceRefs,
		StateRefs:             stateRefs,
		MemoryRefs:            memoryRefs,
		PolicyRuleRefs:        uniqueStrings(policyRefs),
		InterventionIntensity: recipe.InterventionIntensity,
	}, nil
}

func normalizeFamilies(items []string) map[SkillFamily]struct{} {
	result := make(map[SkillFamily]struct{}, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result[SkillFamily(trimmed)] = struct{}{}
		}
	}
	return result
}

func familyAllowed(allowed map[SkillFamily]struct{}, family SkillFamily) bool {
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[family]
	return ok
}

func defaultRecipeID(family SkillFamily) string {
	switch family {
	case SkillFamilySubscriptionCleanup:
		return "subscription_cleanup.v1"
	case SkillFamilyLateNightSpendNudge:
		return "late_night_spend_nudge.v1"
	default:
		return "soft_nudge.v1"
	}
}

func evidenceRefs(records []observation.EvidenceRecord) []string {
	result := make([]string, 0, len(records))
	for _, item := range records {
		result = append(result, string(item.ID))
	}
	return uniqueStrings(result)
}

func proceduralMemoryRefs(records []ProceduralMemoryContextRecord) []string {
	result := make([]string, 0, len(records))
	for _, item := range records {
		if item.Kind != "procedural" {
			continue
		}
		result = append(result, item.ID)
	}
	return uniqueStrings(result)
}

func pickGuardrailRecipe(input SelectionInput) string {
	repeated := hasGuardrailProceduralMemory(input.ProceduralMemories)
	lowerHint := strings.ToLower(strings.TrimSpace(input.SelectionHint))
	highPressure := input.CurrentState.BehaviorState.LateNightSpendingFrequency >= 0.45 || input.CurrentState.CashflowState.MonthlyNetIncomeCents < 0
	if strings.Contains(lowerHint, "hard_cap") && repeated {
		return "hard_cap.v1"
	}
	if repeated && highPressure {
		return "hard_cap.v1"
	}
	if repeated || input.CurrentState.BehaviorState.LateNightSpendingFrequency >= 0.35 {
		return "budget_guardrail.v1"
	}
	return "soft_nudge.v1"
}

func hasGuardrailProceduralMemory(records []ProceduralMemoryContextRecord) bool {
	for _, item := range records {
		if item.Kind != "procedural" {
			continue
		}
		if item.Facts["skill_family"] == string(SkillFamilyDiscretionaryGuardrail) {
			return true
		}
	}
	return false
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
